package schedule

import (
	"cmp"
	"encoding/xml"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/proc"
)

func plistPath(j Job) string {
	return filepath.Join(fsx.Home(), "Library", "LaunchAgents", j.label()+".plist")
}

// plist is the launchd job for j.
func plist(j Job) string {
	var args strings.Builder
	for _, a := range j.Command {
		args.WriteString("\t\t<string>")
		_ = xml.EscapeText(&args, []byte(a))
		args.WriteString("</string>\n")
	}
	var env strings.Builder
	for _, k := range slices.Sorted(maps.Keys(j.Env)) {
		env.WriteString("\t\t<key>")
		_ = xml.EscapeText(&env, []byte(k))
		env.WriteString("</key>\n\t\t<string>")
		_ = xml.EscapeText(&env, []byte(j.Env[k]))
		env.WriteString("</string>\n")
	}
	var app string
	if j.App != "" {
		app = "\t<key>AssociatedBundleIdentifiers</key>\n\t<array>\n\t\t<string>" + xmlText(j.App) + "</string>\n\t</array>\n"
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%[1]s</string>
%[6]s	<key>ProgramArguments</key>
	<array>
%[2]s	</array>
	<key>EnvironmentVariables</key>
	<dict>
%[3]s	</dict>
	<key>StartInterval</key>
	<integer>%[4]d</integer>
	<key>RunAtLoad</key>
	<true/>
	<key>StandardOutPath</key>
	<string>%[5]s</string>
	<key>StandardErrorPath</key>
	<string>%[5]s</string>
</dict>
</plist>
`, j.label(), args.String(), env.String(), int(j.Every.Seconds()), xmlText(cmp.Or(j.Log, "/dev/null")), app)
}

func install(j Job) (string, error) {
	path := plistPath(j)
	_, _, _ = uninstall(j)
	if err := fsx.WriteFile(path, []byte(plist(j)), 0o644); err != nil {
		return path, err
	}
	if testing.Testing() {
		return path, nil
	}
	// Turning it on by hand turns it on, also after Login Items turned it off.
	_ = launchctl("enable", domain()+"/"+j.label())
	// launchd finishes removing the old job in the background: a bootstrap
	// right after it can fail for a moment.
	var err error
	for range 20 {
		if err = launchctl("bootstrap", domain(), path); err == nil {
			return path, nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	_ = os.Remove(path)
	return path, err
}

func uninstall(j Job) (string, bool, error) {
	path := plistPath(j)
	if _, err := os.Stat(path); err != nil {
		return path, false, nil
	}
	if !testing.Testing() {
		_ = launchctl("bootout", domain()+"/"+j.label())
	}
	return path, true, os.Remove(path)
}

// installed asks launchd, not the file: a job whose bootstrap failed is not
// on.
func installed(j Job) (string, bool) {
	path := plistPath(j)
	if _, err := os.Stat(path); err != nil {
		return path, false
	}
	return path, testing.Testing() || launchctl("print", domain()+"/"+j.label()) == nil
}

// envBlock is a plist's environment, which differs from shell to shell
// (PATH) without the job needing to change.
var envBlock = regexp.MustCompile(`(?s)\t<key>EnvironmentVariables</key>\n\t<dict>\n.*?\t</dict>\n`)

func heal(j Job) (bool, error) {
	b, err := os.ReadFile(plistPath(j))
	if err == nil && envBlock.ReplaceAllString(string(b), "") == envBlock.ReplaceAllString(plist(j), "") || disabled(j) {
		return false, nil
	}
	_, err = install(j)
	return err == nil, err
}

// disabled reports whether the person turned the job off in Login Items,
// which launchd records as disabled ("true" on older macOS). It outlives the
// plist.
func disabled(j Job) bool {
	if testing.Testing() {
		return false
	}
	out, _, err := proc.Output("launchctl", []string{"print-disabled", domain()}, nil, defaultTimeout)
	if err != nil {
		return false
	}
	re := regexp.MustCompile(`(?m)^\s*"` + regexp.QuoteMeta(j.label()) + `"\s*=>\s*(disabled|true)\s*$`)
	return re.MatchString(out)
}

func xmlText(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func domain() string { return fmt.Sprintf("gui/%d", os.Getuid()) }

func launchctl(args ...string) error {
	out, code, err := proc.CombinedOutput("launchctl", args, nil, defaultTimeout)
	if err == nil && code != 0 {
		err = fmt.Errorf("launchctl %s: %s", args[0], strings.TrimSpace(out))
	}
	return err
}
