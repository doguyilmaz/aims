package schedule

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/proc"
)

func unitDir() string {
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "systemd", "user")
	}
	return filepath.Join(fsx.Home(), ".config", "systemd", "user")
}

func unit(j Job) string { return "aims-" + j.Name }

func install(j Job) (string, error) {
	quoted := make([]string, len(j.Command))
	for i, a := range j.Command {
		quoted[i] = quote(a)
	}
	var env strings.Builder
	for _, k := range slices.Sorted(maps.Keys(j.Env)) {
		fmt.Fprintf(&env, "Environment=%s\n", strconv.Quote(k+"="+j.Env[k]))
	}
	service := fmt.Sprintf("[Unit]\nDescription=aims %s\n\n[Service]\nType=oneshot\n%sExecStart=%s\n", j.Name, env.String(), strings.Join(quoted, " "))
	timer := fmt.Sprintf("[Unit]\nDescription=aims %s\n\n[Timer]\nOnBootSec=5min\nOnUnitActiveSec=%ds\n\n[Install]\nWantedBy=timers.target\n", j.Name, int(j.Every.Seconds()))
	base := filepath.Join(unitDir(), unit(j))
	if err := fsx.WriteFile(base+".service", []byte(service), 0o644); err != nil {
		return base + ".timer", err
	}
	if err := fsx.WriteFile(base+".timer", []byte(timer), 0o644); err != nil {
		return base + ".timer", err
	}
	if testing.Testing() {
		return base + ".timer", nil
	}
	err := systemctl("daemon-reload")
	if err == nil {
		err = systemctl("enable", "--now", unit(j)+".timer")
	}
	if err != nil {
		// Without a user systemd (WSL, a container) the files would only
		// pretend the job is on.
		_ = os.Remove(base + ".timer")
		_ = os.Remove(base + ".service")
		return base + ".timer", fmt.Errorf("%w (no systemd user session? run `aims keepalive` from cron instead)", err)
	}
	return base + ".timer", nil
}

func uninstall(j Job) (string, bool, error) {
	base := filepath.Join(unitDir(), unit(j))
	if _, err := os.Stat(base + ".timer"); err != nil {
		return base + ".timer", false, nil
	}
	if !testing.Testing() {
		_ = systemctl("disable", "--now", unit(j)+".timer")
	}
	err := os.Remove(base + ".timer")
	_ = os.Remove(base + ".service")
	if !testing.Testing() {
		_ = systemctl("daemon-reload")
	}
	return base + ".timer", true, err
}

func installed(j Job) (string, bool) {
	path := filepath.Join(unitDir(), unit(j)+".timer")
	if _, err := os.Stat(path); err != nil {
		return path, false
	}
	return path, testing.Testing() || systemctl("is-enabled", "--quiet", unit(j)+".timer") == nil
}

func systemctl(args ...string) error {
	out, code, err := proc.CombinedOutput("systemctl", append([]string{"--user"}, args...), nil, defaultTimeout)
	if err == nil && code != 0 {
		err = fmt.Errorf("systemctl %s: %s", args[0], strings.TrimSpace(out))
	}
	return err
}
