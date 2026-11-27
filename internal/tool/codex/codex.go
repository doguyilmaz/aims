// Package codex is the Codex CLI adapter.
package codex

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
	"github.com/pelletier/go-toml/v2/unstable"

	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/proc"
	"github.com/doguyilmaz/aims/internal/tool"
)

// ID is the tool id.
const ID tool.ID = "codex"

// Adapter implements tool.Adapter for Codex.
type Adapter struct{}

// New returns the adapter.
func New() *Adapter { return &Adapter{} }

var _ tool.Adapter = (*Adapter)(nil)

func (*Adapter) ID() tool.ID   { return ID }
func (*Adapter) Title() string { return "Codex" }

func (*Adapter) Layout() tool.Layout {
	return tool.Layout{
		Bin:           "codex",
		BinEnv:        "AIMS_CODEX_BIN",
		HomeEnv:       "CODEX_HOME",
		DefaultHome:   ".codex",
		TranscriptDir: "sessions",
		ACPBin:        "codex-acp",
		Shared: append(tool.Names(
			"sessions", "thread-writer-locks", // shared rollouts need shared writer ownership
			"archived_sessions", "history.jsonl", "config.toml", "AGENTS.md", "AGENTS.override.md",
			"prompts", "skills", "rules", "plugins", "memories", "session_index.jsonl", "packages",
			".hook-trust",
		), tool.Entry{Pattern: regexp.MustCompile(`^(state|memories|goals|queue|thread_history)_\d+\.sqlite$`)}),
		SharedDirs:     []string{"sessions", "archived_sessions", "prompts", "skills", "thread-writer-locks", ".hook-trust"},
		LockDirs:       []string{"thread-writer-locks"},
		StateEnv:       "CODEX_SQLITE_HOME",
		Disposable:     []tool.Entry{{Pattern: regexp.MustCompile(`^(thread_history|logs)_\d+\.sqlite$`)}},
		History:        append(tool.Names("sessions", "archived_sessions", "history.jsonl", "memories", "session_index.jsonl", "thread-writer-locks"), tool.Entry{Pattern: regexp.MustCompile(`\.sqlite$`)}),
		ConflictingEnv: []string{"CODEX_API_KEY", "OPENAI_API_KEY"},
		SkillsDir:      "skills",
		SettingsFile:   "config.toml",
		Markers:        []string{"auth.json", "config.toml", "sessions", "history.jsonl"},
	}
}

func (*Adapter) Commands() tool.Commands {
	return tool.Commands{
		Login:  []string{"login"},
		Logout: []string{"logout"},
		Resume: []string{"resume", "--last"},
		LoginTip: "The browser signs in whichever ChatGPT account it is already logged into. " +
			"Use a private window, or a device code: aims login codex <profile> -- --device-auth",
	}
}

// valueFlags are codex's global options that take a value, so the word after
// them is not the subcommand.
var valueFlags = map[string]bool{
	"-c": true, "--config": true, "--enable": true, "--disable": true, "--remote": true,
	"--remote-auth-token-env": true, "-i": true, "--image": true, "-m": true, "--model": true,
	"--local-provider": true, "-p": true, "--profile": true, "-s": true, "--sandbox": true,
	"-C": true, "--cd": true, "--add-dir": true, "-a": true, "--ask-for-approval": true,
}

func (*Adapter) IsHeadless(args []string) bool {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			return false
		case valueFlags[a]:
			i++
		case strings.HasPrefix(a, "-"):
		default:
			return a == "exec" || a == "e" || a == "review"
		}
	}
	return false
}

func (*Adapter) HeadlessArgs(o tool.HeadlessOptions) []string {
	args := []string{"exec"}
	if o.Continue {
		args = append(args, "resume", "--last")
	}
	args = append(args, "--json", "--skip-git-repo-check")
	if o.ReadOnly {
		args = append(args, "--sandbox", "read-only")
	}
	if o.Model != "" {
		args = append(args, "--model", o.Model)
	}
	return append(args, "--", o.Prompt)
}

func (*Adapter) Account(ctx context.Context, h tool.Home) tool.Account {
	if ctx.Err() != nil {
		return tool.Account{}
	}
	if h.ProfileEnv["CODEX_API_KEY"] != "" || h.ProfileEnv["OPENAI_API_KEY"] != "" {
		return tool.Account{LoggedIn: tool.Known(true), Method: "token"}
	}
	var auth struct {
		APIKey *string `json:"OPENAI_API_KEY"`
		Tokens *struct {
			IDToken      string `json:"id_token"`
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
		} `json:"tokens"`
	}
	if fsx.ReadJSONLenient(filepath.Join(h.Dir, "auth.json"), &auth) {
		if auth.APIKey != nil && *auth.APIKey != "" {
			return tool.Account{LoggedIn: tool.Known(true), Method: "api-key"}
		}
		if auth.Tokens == nil {
			return tool.Account{LoggedIn: tool.Known(false)}
		}
		email, plan, workspace := claims(auth.Tokens.IDToken)
		ok := auth.Tokens.RefreshToken != "" || auth.Tokens.AccessToken != ""
		return tool.Account{LoggedIn: tool.Known(ok), Method: "chatgpt", Email: email, Plan: plan, Tenant: workspace, Expires: expiry(auth.Tokens.AccessToken)}
	}
	// The login may live in the OS keyring (cli_auth_credentials_store).
	if h.Bin == "" {
		return tool.Account{}
	}
	_, code, err := proc.OutputContext(ctx, h.Bin, []string{"login", "status"}, h.Env, 15*time.Second)
	switch {
	case err != nil:
		return tool.Account{}
	case code == 0:
		return tool.Account{LoggedIn: tool.Known(true), Method: "keyring"}
	case code == 1:
		return tool.Account{LoggedIn: tool.Known(false)}
	}
	return tool.Account{}
}

// expiry reads when an access token runs out, decoded like claims.
func expiry(token string) time.Time {
	var c struct {
		Exp int64 `json:"exp"`
	}
	if !decode(token, &c) || c.Exp == 0 {
		return time.Time{}
	}
	return time.Unix(c.Exp, 0)
}

// claims reads the e-mail and plan from the ID token. The token is only
// decoded, never verified or sent anywhere: it is a label, not a credential check.
func claims(idToken string) (email, plan, workspace string) {
	var c struct {
		Email string `json:"email"`
		Auth  struct {
			Plan    string `json:"chatgpt_plan_type"`
			Account string `json:"chatgpt_account_id"`
		} `json:"https://api.openai.com/auth"`
	}
	if !decode(idToken, &c) {
		return "", "", ""
	}
	return c.Email, c.Auth.Plan, c.Auth.Account
}

// decode reads a JWT's payload into v.
func decode(token string, v any) bool {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return false
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	return err == nil && json.Unmarshal(payload, v) == nil
}

func (*Adapter) MCPScope() tool.MCPScope { return tool.MCPShared }

func (*Adapter) MCPConfigPath(h tool.Home) string { return filepath.Join(h.Dir, "config.toml") }

var (
	alreadyRe = regexp.MustCompile(`(?i)already exists`)
	missingRe = regexp.MustCompile(`(?i)no mcp server named|not found`)
)

// RegisterMCP uses Codex to add a new entry. For an existing entry, it only
// updates the environment allowlist and tool timeout managed by aims.
func (a *Adapter) RegisterMCP(ctx context.Context, h tool.Home, s tool.MCPServer) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path := a.MCPConfigPath(h)
	b, entry, err := readMCPConfig(path, s.Name)
	if err != nil {
		return err
	}
	if entry == nil {
		if _, err := runMCP(ctx, h, append([]string{"mcp", "add", s.Name, "--"}, s.Command...), alreadyRe); err != nil {
			return err
		}
		b, entry, err = readMCPConfig(path, s.Name)
		if err != nil {
			return err
		}
		if entry == nil {
			return fmt.Errorf("codex mcp add left no %s MCP entry in %s", s.Name, fsx.Tildify(path))
		}
	}
	if len(missingMCPKeys(entry, s)) == 0 {
		return nil
	}
	out, ok := completeMCP(string(b), s)
	if !ok {
		return fmt.Errorf("cannot update %s MCP entry without an explicit [mcp_servers.%s] table in %s", s.Name, s.Name, fsx.Tildify(path))
	}
	if _, err := parseMCPConfig([]byte(out), path, s.Name); err != nil {
		return fmt.Errorf("cannot update %s MCP entry in %s: managed keys conflict with its TOML structure", s.Name, fsx.Tildify(path))
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return fsx.WriteFile(path, []byte(out), 0o600)
}

// MCPMissing names missing or stale managed keys in an existing MCP entry.
func (a *Adapter) MCPMissing(h tool.Home, s tool.MCPServer) ([]string, error) {
	_, entry, err := readMCPConfig(a.MCPConfigPath(h), s.Name)
	return missingMCPKeys(entry, s), err
}

func readMCPConfig(path, name string) ([]byte, map[string]any, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("cannot read Codex MCP settings in %s", fsx.Tildify(path))
	}
	entry, err := parseMCPConfig(b, path, name)
	return b, entry, err
}

func parseMCPConfig(b []byte, path, name string) (map[string]any, error) {
	var cfg struct {
		Servers map[string]map[string]any `toml:"mcp_servers"`
	}
	if err := toml.Unmarshal(b, &cfg); err != nil {
		// TOML errors can include values from the settings file.
		var decode *toml.DecodeError
		if errors.As(err, &decode) {
			line, column := decode.Position()
			return nil, fmt.Errorf("invalid Codex MCP settings in %s (line %d, column %d)", fsx.Tildify(path), line, column)
		}
		return nil, fmt.Errorf("invalid Codex MCP settings in %s", fsx.Tildify(path))
	}
	return cfg.Servers[name], nil
}

func missingMCPKeys(entry map[string]any, s tool.MCPServer) []string {
	if entry == nil {
		return nil
	}
	var missing []string
	if env, _ := entry["env_vars"].([]any); len(s.Env) > 0 &&
		(len(env) != len(s.Env) || slices.ContainsFunc(s.Env, func(k string) bool { return !slices.Contains(env, any(k)) })) {
		missing = append(missing, "env_vars")
	}
	if seconds := int64(s.Timeout.Seconds()); s.Timeout > 0 &&
		entry["tool_timeout_sec"] != seconds && entry["tool_timeout_sec"] != float64(seconds) {
		missing = append(missing, "tool_timeout_sec")
	}
	return missing
}

// mcpKeys contains complete assignments for the keys aims manages.
func mcpKeys(s tool.MCPServer) []string {
	var out []string
	if len(s.Env) > 0 {
		q := make([]string, len(s.Env))
		for i, v := range s.Env {
			q[i] = strconv.Quote(v)
		}
		out = append(out, "env_vars = ["+strings.Join(q, ", ")+"]\n")
	}
	if s.Timeout > 0 {
		out = append(out, fmt.Sprintf("tool_timeout_sec = %d\n", int(s.Timeout.Seconds())))
	}
	return out
}

// completeMCP inserts managed settings at the top of the explicit server
// table and removes their old assignments. The parser supplies source ranges
// so quoted keys and multiline values leave surrounding settings intact.
func completeMCP(config string, s tool.MCPServer) (out string, ok bool) {
	var p unstable.Parser
	p.Reset([]byte(config))
	var result strings.Builder
	cursor := 0
	inTable := false
	for p.NextExpression() {
		n := p.Expression()
		switch n.Kind {
		case unstable.Table, unstable.ArrayTable:
			if inTable {
				result.WriteString(config[cursor:])
				return result.String(), true
			}
			keys := n.Key()
			if n.Kind != unstable.Table || !keys.Next() || string(keys.Node().Data) != "mcp_servers" ||
				!keys.Next() || string(keys.Node().Data) != s.Name || !keys.IsLast() {
				continue
			}
			cursor = nextTOMLLine(config, int(keys.Node().Raw.Offset))
			result.WriteString(config[:cursor])
			if config[cursor-1] != '\n' {
				result.WriteByte('\n')
			}
			for _, line := range mcpKeys(s) {
				result.WriteString(line)
			}
			inTable = true
		case unstable.KeyValue:
			if !inTable {
				continue
			}
			keys := n.Key()
			keys.Next()
			key := string(keys.Node().Data)
			if key != "env_vars" && key != "tool_timeout_sec" {
				continue
			}
			start := int(n.Raw.Offset)
			result.WriteString(config[cursor : strings.LastIndexByte(config[:start], '\n')+1])
			cursor = nextTOMLLine(config, start+int(n.Raw.Length))
		}
	}
	if p.Error() != nil || !inTable {
		return config, false
	}
	result.WriteString(config[cursor:])
	return result.String(), true
}

func nextTOMLLine(config string, offset int) int {
	if i := strings.IndexByte(config[offset:], '\n'); i >= 0 {
		return offset + i + 1
	}
	return len(config)
}

func (a *Adapter) UnregisterMCP(ctx context.Context, h tool.Home, name string) (bool, error) {
	if _, _, err := readMCPConfig(a.MCPConfigPath(h), name); err != nil {
		return false, err
	}
	return runMCP(ctx, h, []string{"mcp", "remove", name}, missingRe)
}

// runMCP runs an mcp subcommand. changed is false when it failed with an
// expected message (already there, nothing to remove).
func runMCP(ctx context.Context, h tool.Home, args []string, benign *regexp.Regexp) (changed bool, err error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if h.Bin == "" {
		return false, &tool.CommandError{Output: "codex is not installed"}
	}
	out, code, err := proc.CombinedOutputContext(ctx, h.Bin, args, h.Env, time.Minute)
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
