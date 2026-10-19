package schedule

import (
	"cmp"
	"encoding/xml"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/proc"
)

func plistPath(j Job) string {
	return filepath.Join(fsx.Home(), "Library", "LaunchAgents", j.label()+".plist")
}

func install(j Job) (string, error) {
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
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%[1]s</string>
	<key>ProgramArguments</key>
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
`, j.label(), args.String(), env.String(), int(j.Every.Seconds()), xmlText(cmp.Or(j.Log, "/dev/null")))
	path := plistPath(j)
	_, _, _ = uninstall(j)
	if err := fsx.WriteFile(path, []byte(plist), 0o644); err != nil {
		return path, err
	}
	if testing.Testing() {
		return path, nil
	}
	return path, launchctl("bootstrap", domain(), path)
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

func installed(j Job) (string, bool) {
	path := plistPath(j)
	_, err := os.Stat(path)
	return path, err == nil
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
