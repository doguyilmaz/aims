// Package tool defines what aims needs from an AI coding CLI. Each supported
// CLI is an Adapter in its own package (tool/claude, tool/codex); the rest of
// aims only talks to this interface, so adding a CLI means adding a package and
// listing it in package tools.
package tool

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
)

// ID names a tool ("claude", "codex").
type ID string

// Adapter is one AI coding CLI.
type Adapter interface {
	ID() ID
	// Title is the product name, e.g. "Claude Code".
	Title() string
	Layout() Layout
	Commands() Commands

	// IsHeadless reports whether args start a non-interactive run.
	IsHeadless(args []string) bool
	// HeadlessArgs builds a non-interactive run whose output shows tool calls,
	// so a failed attempt can be judged safe to retry. The prompt must follow
	// "--" so a prompt starting with "-" is not read as a flag.
	HeadlessArgs(HeadlessOptions) []string
	// NewAnalyzer reads the output of a headless run started with args.
	NewAnalyzer(args []string) Analyzer

	// Account inspects a login without network access and without refreshing
	// any token.
	Account(ctx context.Context, h Home) Account
	// Probe asks the provider whether the login works and how much of the plan
	// is used. It must be cheap: no paid request where an account API exists.
	Probe(ctx context.Context, h Home) Probe

	// MCPScope says where the tool keeps user-level MCP servers.
	MCPScope() MCPScope
	// RegisterMCP adds a user-level MCP server; one already there is fine.
	RegisterMCP(ctx context.Context, h Home, s MCPServer) error
	// UnregisterMCP removes it and reports whether it was there.
	UnregisterMCP(ctx context.Context, h Home, name string) (removed bool, err error)
}

// Optional capabilities, detected with a type assertion.

// Seeder prepares a new profile directory from the hub (settings that are not
// shared by linking but should not start empty either).
type Seeder interface {
	Seed(hub Home, profileDir string) error
}

// MCPSyncer copies user-level MCP servers from the hub's login into a profile
// whose tool stores them per login.
type MCPSyncer interface {
	SyncMCP(hub Home, profile Home) (added []string, err error)
}

// StatusLiner installs aims as the tool's status line command.
type StatusLiner interface {
	// InstallStatusLine sets command in the settings under home and returns
	// the command it replaced, if any.
	InstallStatusLine(home, command string) (previous string, err error)
	// RemoveStatusLine restores previous if command is still the one set,
	// and reports whether it changed anything.
	RemoveStatusLine(home, command, previous string) (removed bool, err error)
}

// Layout is where a tool keeps its state and which parts accounts can share.
type Layout struct {
	// Bin is the command name; BinEnv overrides it.
	Bin    string
	BinEnv string
	// HomeEnv points the tool at a config directory; DefaultHome is the
	// directory name under $HOME used when it is unset.
	HomeEnv     string
	DefaultHome string
	// Shared entries are linked from every profile to the hub. Anything not
	// listed, above all the login files, stays private to the profile.
	Shared []Entry
	// SharedDirs are created in the hub up front so a profile never forks them.
	SharedDirs []string
	// History is conversation data: a profile that stops sharing starts with
	// it empty rather than with a copy.
	History []Entry
	// ConflictingEnv overrides a profile's login; aims drops it from the child.
	ConflictingEnv []string
	// SkillsDir is where skills live inside the home ("" if unsupported).
	SkillsDir string
	// Markers are entries only the tool's own folder has. An existing folder
	// given as a profile's --dir must hold one, so a code repository (with its
	// own CLAUDE.md or plugins/) is never mistaken for a config folder.
	Markers []string
}

// IsShared reports whether a home entry is shared between profiles.
func (l Layout) IsShared(name string) bool { return matchAny(l.Shared, name) }

// IsHistory reports whether a home entry holds conversation data.
func (l Layout) IsHistory(name string) bool { return matchAny(l.History, name) }

func matchAny(es []Entry, name string) bool {
	return slices.ContainsFunc(es, func(e Entry) bool { return e.Match(name) })
}

// Entry is an exact name or a pattern.
type Entry struct {
	Name    string
	Pattern *regexp.Regexp
}

// Match reports whether a directory entry name matches.
func (e Entry) Match(name string) bool {
	if e.Pattern != nil {
		return e.Pattern.MatchString(name)
	}
	return e.Name == name
}

// Names builds exact-name entries.
func Names(ns ...string) []Entry {
	out := make([]Entry, len(ns))
	for i, n := range ns {
		out[i] = Entry{Name: n}
	}
	return out
}

// Commands are the tool's own subcommands aims runs for the user.
type Commands struct {
	Login  []string
	Logout []string
	// Resume continues the most recent conversation in the current directory.
	Resume []string
	// LoginTip is shown before a browser login.
	LoginTip string
}

// HeadlessOptions describes a non-interactive run started by aims itself.
type HeadlessOptions struct {
	Prompt   string
	Model    string
	Continue bool
	// ReadOnly asks the tool not to edit files or run changing commands,
	// whatever its settings allow.
	ReadOnly bool
}

// Home is one login of a tool, as the adapter sees it.
type Home struct {
	// Dir is the config directory.
	Dir string
	// EnvValue is the value of Layout.HomeEnv for this login ("" = unset).
	EnvValue string
	// Env is the complete environment to run the tool with for this login.
	Env []string
	// Bin is the resolved binary, "" when the tool is not installed.
	Bin string
	// Profile env vars given in aims' config (an API key account, say).
	ProfileEnv map[string]string
}

// Account is what can be known about a login without the network.
type Account struct {
	// LoggedIn is nil when it cannot be told offline.
	LoggedIn *bool
	Method   string
	Email    string
	Plan     string
	Org      string
}

// Known reports a definite login state.
func Known(b bool) *bool { return &b }

// Window is one plan usage window, e.g. "5h" or "7d".
type Window struct {
	Label    string    `json:"label"`
	Percent  float64   `json:"pct"`
	ResetsAt time.Time `json:"resetsAt,omitzero"`
}

// ProbeStatus is the outcome of a Probe.
type ProbeStatus string

const (
	ProbeOK    ProbeStatus = "ok"
	ProbeLogin ProbeStatus = "login"
	ProbeLimit ProbeStatus = "limit"
	ProbeError ProbeStatus = "error"
)

// Probe is what the provider said.
type Probe struct {
	Status  ProbeStatus
	Detail  string
	Email   string
	Plan    string
	Windows []Window
}

// MCPScope is where a tool keeps user-level MCP servers.
type MCPScope int

// MCPServer is a stdio MCP server to register.
type MCPServer struct {
	Name    string
	Command []string
	// Env names the variables the server reads. A tool that starts MCP
	// servers with only a few variables (HOME, PATH...) is told to pass
	// these on as well.
	Env []string
	// Timeout is how long one tool call may take (0: the tool's default).
	Timeout time.Duration
}

// MCPAuditor reports what an MCP entry registered by an older aims lacks;
// nil when it is complete or not there.
type MCPAuditor interface {
	MCPMissing(h Home, s MCPServer) []string
}

const (
	// MCPPerLogin: each login has its own list (Claude Code's .claude.json).
	MCPPerLogin MCPScope = iota
	// MCPShared: the list lives in a shared file (Codex config.toml).
	MCPShared
)

// CommandError is a tool subcommand that failed.
type CommandError struct {
	Output string
	Code   int
}

func (e *CommandError) Error() string {
	msg := strings.TrimSpace(e.Output)
	if i := strings.LastIndexByte(msg, '\n'); i >= 0 {
		msg = msg[i+1:]
	}
	if msg == "" {
		msg = fmt.Sprintf("exit status %d", e.Code)
	}
	return msg
}

// ParseTime reads epoch seconds, epoch milliseconds or an RFC 3339 string.
func ParseTime(v any) time.Time {
	switch t := v.(type) {
	case float64:
		if t <= 0 {
			return time.Time{}
		}
		if t < 1e12 {
			return time.Unix(int64(t), 0).UTC()
		}
		return time.UnixMilli(int64(t)).UTC()
	case int64:
		return ParseTime(float64(t))
	case string:
		if ts, err := time.Parse(time.RFC3339Nano, t); err == nil {
			return ts.UTC()
		}
	}
	return time.Time{}
}
