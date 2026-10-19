// Package schedule runs an aims command at an interval in the background,
// with the system's own scheduler: launchd on macOS, a systemd user timer on
// Linux, the Task Scheduler on Windows.
package schedule

import (
	"strings"
	"time"
)

// Job is a command the system runs every Every.
type Job struct {
	// Name is unique to aims: "keepalive".
	Name    string
	Command []string
	Every   time.Duration
	// Env is what the job's environment needs beyond the scheduler's
	// minimal one, such as PATH to find claude and codex.
	Env map[string]string
	// Log receives the job's output where the scheduler does not keep it
	// itself (launchd); "" discards it.
	Log string
}

// label names the job in the scheduler.
func (j Job) label() string { return "com.doguyilmaz.aims." + j.Name }

// Install sets the job up, replacing an older one of the same name, and
// returns the file or task it wrote.
func Install(j Job) (string, error) { return install(j) }

// Uninstall removes the job; found is false when it was not set up.
func Uninstall(name string) (target string, found bool, err error) { return uninstall(Job{Name: name}) }

// Installed reports whether the job is set up, and where.
func Installed(name string) (string, bool) { return installed(Job{Name: name}) }

// quote makes s one word of a systemd ExecStart line, where % starts a
// specifier.
func quote(s string) string {
	s = strings.ReplaceAll(s, "%", "%%")
	if s != "" && !strings.ContainsAny(s, " \t'\"\\$`") {
		return s
	}
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "$", "$$").Replace(s) + `"`
}

const defaultTimeout = 30 * time.Second
