// Package claude is the Claude Code adapter.
package claude

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/proc"
	"github.com/doguyilmaz/aims/internal/tool"
	"golang.org/x/text/unicode/norm"
)

// ID is the tool id.
const ID tool.ID = "claude"

// Adapter implements tool.Adapter for Claude Code.
type Adapter struct{}

// New returns the adapter.
func New() *Adapter { return &Adapter{} }

var (
	_ tool.Adapter     = (*Adapter)(nil)
	_ tool.Seeder      = (*Adapter)(nil)
	_ tool.MCPSyncer   = (*Adapter)(nil)
	_ tool.StatusLiner = (*Adapter)(nil)
)

func (*Adapter) ID() tool.ID   { return ID }
func (*Adapter) Title() string { return "Claude Code" }

func (*Adapter) Layout() tool.Layout {
	return tool.Layout{
		Bin:         "claude",
		BinEnv:      "AIMS_CLAUDE_BIN",
		HomeEnv:     "CLAUDE_CONFIG_DIR",
		DefaultHome: ".claude",
		Shared: tool.Names(
			"projects", // transcripts and auto memory: `claude --continue` works across accounts
			"file-history", "todos", "plans", "tasks", "history.jsonl",
			"settings.json", "CLAUDE.md", "keybindings.json",
			"commands", "agents", "skills", "output-styles", "hooks", "plugins", "rules",
		),
		SharedDirs:     []string{"projects", "file-history", "todos", "plans", "commands", "agents", "skills"},
		History:        tool.Names("projects", "file-history", "todos", "plans", "tasks", "history.jsonl"),
		ConflictingEnv: []string{"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "CLAUDE_CODE_OAUTH_TOKEN"},
		SkillsDir:      "skills",
		Markers:        []string{".credentials.json", ".claude.json", "projects", "settings.json"},
	}
}

func (*Adapter) Commands() tool.Commands {
	return tool.Commands{
		Login:  []string{"auth", "login"},
		Logout: []string{"auth", "logout"},
		Resume: []string{"--continue"},
		LoginTip: "The browser signs in whichever claude.ai account it is already logged into. " +
			"Use a private window, or pre-fill the address: aims login claude <profile> -- --email you@company.com",
	}
}

func (*Adapter) IsHeadless(args []string) bool {
	return tool.HasFlag(args, "-p") || tool.HasFlag(args, "--print")
}

func (*Adapter) HeadlessArgs(o tool.HeadlessOptions) []string {
	var args []string
	if o.Continue {
		args = append(args, "--continue")
	}
	args = append(args, "-p", "--output-format", "stream-json", "--verbose")
	if o.ReadOnly {
		args = append(args, "--permission-mode", "plan")
	}
	if o.Model != "" {
		args = append(args, "--model", o.Model)
	}
	return append(args, "--", o.Prompt)
}

// claudeJSON is where Claude Code keeps account, trust and user MCP settings:
// ~/.claude.json by default, inside CLAUDE_CONFIG_DIR when that is set.
func claudeJSON(h tool.Home) string {
	if h.EnvValue == "" {
		return filepath.Join(fsx.Home(), ".claude.json")
	}
	return filepath.Join(fsx.Abs(h.EnvValue), ".claude.json")
}

// KeychainService is the macOS keychain entry Claude Code uses for a
// CLAUDE_CONFIG_DIR value: a hash of the exact string is appended when set.
func KeychainService(envValue string) string {
	if envValue == "" {
		return "Claude Code-credentials"
	}
	sum := sha256.Sum256([]byte(nfc(envValue)))
	return "Claude Code-credentials-" + hex.EncodeToString(sum[:])[:8]
}

// nfc matches Claude Code, which normalizes the path before hashing it.
func nfc(s string) string { return norm.NFC.String(s) }

type oauth struct {
	AccessToken      string `json:"accessToken"`
	RefreshToken     string `json:"refreshToken"`
	SubscriptionType string `json:"subscriptionType"`
}

func keychainAccount() string {
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return "claude-code-user"
}

func readKeychain(envValue string) (found bool, o *oauth) {
	out, code, err := proc.Output("security", []string{"find-generic-password", "-a", keychainAccount(), "-w", "-s", KeychainService(envValue)}, nil, 5*time.Second)
	if err != nil || code != 0 {
		return false, nil
	}
	out = strings.TrimSpace(out)
	candidates := []string{out}
	if b, err := hex.DecodeString(out); err == nil {
		candidates = append(candidates, string(b))
	}
	for _, c := range candidates {
		var v struct {
			ClaudeAiOauth *oauth `json:"claudeAiOauth"`
		}
		if json.Unmarshal([]byte(c), &v) == nil {
			return true, v.ClaudeAiOauth
		}
	}
	return true, nil
}

func (*Adapter) Account(_ context.Context, h tool.Home) tool.Account {
	var cj struct {
		OAuthAccount struct {
			Email string `json:"emailAddress"`
			Org   string `json:"organizationName"`
		} `json:"oauthAccount"`
	}
	fsx.ReadJSONLenient(claudeJSON(h), &cj)
	acct := tool.Account{Email: cj.OAuthAccount.Email, Org: cj.OAuthAccount.Org}
	for _, k := range []string{"CLAUDE_CODE_OAUTH_TOKEN", "ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN"} {
		if h.ProfileEnv[k] != "" {
			acct.LoggedIn, acct.Method = tool.Known(true), "token"
			return acct
		}
	}
	var o *oauth
	present := false
	// Test binaries never read the keychain of the person running them.
	if runtime.GOOS == "darwin" && !testing.Testing() {
		present, o = readKeychain(h.EnvValue)
	}
	if !present {
		var f struct {
			ClaudeAiOauth *oauth `json:"claudeAiOauth"`
		}
		if fsx.ReadJSONLenient(filepath.Join(h.Dir, ".credentials.json"), &f) {
			present, o = true, f.ClaudeAiOauth
		}
	}
	acct.LoggedIn = tool.Known(present)
	if present {
		acct.Method = "claude.ai"
		if o != nil {
			acct.Plan = o.SubscriptionType
		}
	}
	return acct
}

// Probe sends a one-word prompt to the smallest model. Claude Code has no
// account API that works without refreshing the login, so this costs a few
// tokens; it runs only when asked for (aims status --live).
func (a *Adapter) Probe(ctx context.Context, h tool.Home) tool.Probe {
	if h.Bin == "" {
		return tool.Probe{Status: tool.ProbeError, Detail: "claude is not installed"}
	}
	args := []string{"-p", "--output-format", "json", "--no-session-persistence", "--model", "haiku", "--", "Reply with just: OK"}
	out, code, err := proc.Output(h.Bin, args, h.Env, 2*time.Minute)
	if err != nil {
		return tool.Probe{Status: tool.ProbeError, Detail: err.Error()}
	}
	var res struct {
		IsError bool   `json:"is_error"`
		Result  string `json:"result"`
	}
	lines := proc.Lines(out)
	parsed := len(lines) > 0 && json.Unmarshal([]byte(lines[len(lines)-1]), &res) == nil
	if code == 0 && parsed && !res.IsError {
		return tool.Probe{Status: tool.ProbeOK}
	}
	text := out
	if parsed {
		text = res.Result
	}
	switch tool.Classify(text) {
	case tool.FailLogin:
		return tool.Probe{Status: tool.ProbeLogin, Detail: firstLine(text)}
	case tool.FailLimit:
		return tool.Probe{Status: tool.ProbeLimit, Detail: firstLine(text)}
	}
	return tool.Probe{Status: tool.ProbeError, Detail: firstLine(text)}
}

func firstLine(s string) string {
	if l := proc.Lines(s); len(l) > 0 {
		return l[0]
	}
	return "no output"
}

func (*Adapter) MCPScope() tool.MCPScope { return tool.MCPPerLogin }

var alreadyRe = regexp.MustCompile(`(?i)already exists`)
var missingRe = regexp.MustCompile(`(?i)no (mcp )?server named|not found`)

func (*Adapter) RegisterMCP(_ context.Context, h tool.Home, name string, command []string) error {
	args := append([]string{"mcp", "add", "--scope", "user", name, "--"}, command...)
	_, err := runMCP(h, args, alreadyRe)
	return err
}

func (*Adapter) UnregisterMCP(_ context.Context, h tool.Home, name string) (bool, error) {
	return runMCP(h, []string{"mcp", "remove", "--scope", "user", name}, missingRe)
}

// runMCP runs an mcp subcommand. changed is false when it failed with an
// expected message (already there, nothing to remove).
func runMCP(h tool.Home, args []string, benign *regexp.Regexp) (changed bool, err error) {
	if h.Bin == "" {
		return false, errNotInstalled
	}
	out, code, err := proc.CombinedOutput(h.Bin, args, h.Env, time.Minute)
	switch {
	case err != nil:
		return false, err
	case benign.MatchString(out): // codex exits 0 after "No MCP server named ..."
		return false, nil
	case code == 0:
		return true, nil
	}
	return false, &tool.CommandError{Output: out, Code: code}
}

var errNotInstalled = &tool.CommandError{Output: "claude is not installed"}

// Seed gives a new profile the non-account parts of the hub's .claude.json:
// user MCP servers, per-project trust and allowed tools, theme. Account
// fields (oauthAccount, userID, caches) are never copied.
func (*Adapter) Seed(hub tool.Home, profileDir string) error {
	target := filepath.Join(profileDir, ".claude.json")
	if fsx.Lstat(target) != nil {
		return nil
	}
	src, err := os.ReadFile(claudeJSON(hub))
	if err != nil {
		return nil // nothing to seed from
	}
	out, err := seedJSON(src)
	if err != nil || out == nil {
		return err
	}
	return fsx.WriteFile(target, out, 0o600)
}

var (
	seedKeys        = []string{"theme", "editorMode", "mcpServers", "autoUpdates", "verbose", "preferredNotifChannel"}
	seedProjectKeys = []string{"allowedTools", "mcpServers", "enabledMcpjsonServers", "disabledMcpjsonServers", "hasTrustDialogAccepted", "hasCompletedProjectOnboarding"}
)

func (*Adapter) SyncMCP(hub tool.Home, profile tool.Home) ([]string, error) {
	src, err := os.ReadFile(claudeJSON(hub))
	if err != nil {
		return nil, nil
	}
	path := claudeJSON(profile)
	dst, err := os.ReadFile(path)
	if err != nil {
		return nil, nil // the profile has not been set up yet
	}
	out, added, err := mergeMCPServers(src, dst)
	if err != nil || len(added) == 0 {
		return nil, err
	}
	return added, fsx.WriteFile(path, out, 0o600)
}

func (*Adapter) InstallStatusLine(home, command string) (string, error) {
	path := filepath.Join(home, "settings.json")
	b, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	out, prev, err := setStatusLine(b, command)
	if err != nil {
		return "", err
	}
	return prev, fsx.WriteFile(path, out, 0o644)
}

func (*Adapter) RemoveStatusLine(home, command, previous string) (bool, error) {
	path := filepath.Join(home, "settings.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return false, nil
	}
	out, changed, err := unsetStatusLine(b, command, previous)
	if err != nil || !changed {
		return false, err
	}
	return true, fsx.WriteFile(path, out, 0o644)
}

// StatusInput is the part of Claude Code's status line JSON aims reads.
type StatusInput struct {
	RateLimits map[string]struct {
		UsedPercentage *float64 `json:"used_percentage"`
		Utilization    *float64 `json:"utilization"`
		Remaining      *float64 `json:"remaining"`
		Limit          *float64 `json:"limit"`
		ResetsAt       any      `json:"resets_at"`
	} `json:"rate_limits"`
}

// ParseStatusLine reads plan usage from the JSON Claude Code passes to a
// status line command.
func ParseStatusLine(raw []byte) []tool.Window {
	var in StatusInput
	if json.Unmarshal(raw, &in) != nil {
		return nil
	}
	var out []tool.Window
	for _, k := range []struct{ key, label string }{{"five_hour", "5h"}, {"seven_day", "7d"}} {
		w, ok := in.RateLimits[k.key]
		if !ok {
			continue
		}
		var pct float64
		switch {
		case w.UsedPercentage != nil:
			pct = *w.UsedPercentage
		case w.Utilization != nil:
			pct = *w.Utilization
		case w.Remaining != nil && w.Limit != nil && *w.Limit > 0:
			pct = 100 - *w.Remaining / *w.Limit * 100
		default:
			continue
		}
		out = append(out, tool.Window{Label: k.label, Percent: float64(int(pct*10+0.5)) / 10, ResetsAt: tool.ParseTime(w.ResetsAt)})
	}
	return out
}

// statusCommandRe recognizes a status line command aims installed.
var statusCommandRe = regexp.MustCompile(`\baims\b.*\bstatusline\b`)

// IsAimsStatusLine reports whether command is aims' own status line.
func IsAimsStatusLine(command string) bool { return statusCommandRe.MatchString(command) }

func contains(list []string, s string) bool { return slices.Contains(list, s) }
