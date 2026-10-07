// Package codex is the Codex CLI adapter.
package codex

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"
	"time"

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
		Bin:         "codex",
		BinEnv:      "AIMS_CODEX_BIN",
		HomeEnv:     "CODEX_HOME",
		DefaultHome: ".codex",
		Shared: append(tool.Names(
			"sessions", // rollouts: `codex resume` works across accounts
			"archived_sessions", "history.jsonl", "config.toml", "AGENTS.md", "AGENTS.override.md",
			"prompts", "skills", "rules", "plugins", "memories",
		), tool.Entry{Pattern: regexp.MustCompile(`^(state|memories|goals|queue)_\d+\.sqlite$`)}),
		SharedDirs:     []string{"sessions", "archived_sessions", "prompts", "skills"},
		History:        append(tool.Names("sessions", "archived_sessions", "history.jsonl", "memories"), tool.Entry{Pattern: regexp.MustCompile(`\.sqlite$`)}),
		ConflictingEnv: []string{"CODEX_API_KEY"},
		SkillsDir:      "skills",
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

func (*Adapter) Account(_ context.Context, h tool.Home) tool.Account {
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
		email, plan := claims(auth.Tokens.IDToken)
		ok := auth.Tokens.RefreshToken != "" || auth.Tokens.AccessToken != ""
		return tool.Account{LoggedIn: tool.Known(ok), Method: "chatgpt", Email: email, Plan: plan}
	}
	// The login may live in the OS keyring (cli_auth_credentials_store).
	if h.Bin == "" {
		return tool.Account{}
	}
	_, code, err := proc.Output(h.Bin, []string{"login", "status"}, h.Env, 15*time.Second)
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

// claims reads the e-mail and plan from the ID token. The token is only
// decoded, never verified or sent anywhere: it is a label, not a credential check.
func claims(idToken string) (email, plan string) {
	parts := strings.Split(idToken, ".")
	if len(parts) < 2 {
		return "", ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return "", ""
	}
	var c struct {
		Email string `json:"email"`
		Auth  struct {
			Plan string `json:"chatgpt_plan_type"`
		} `json:"https://api.openai.com/auth"`
	}
	if json.Unmarshal(payload, &c) != nil {
		return "", ""
	}
	return c.Email, c.Auth.Plan
}

func (*Adapter) MCPScope() tool.MCPScope { return tool.MCPShared }

var (
	alreadyRe = regexp.MustCompile(`(?i)already exists`)
	missingRe = regexp.MustCompile(`(?i)no mcp server named|not found`)
)

func (*Adapter) RegisterMCP(_ context.Context, h tool.Home, name string, command []string) error {
	_, err := runMCP(h, append([]string{"mcp", "add", name, "--"}, command...), alreadyRe)
	return err
}

func (*Adapter) UnregisterMCP(_ context.Context, h tool.Home, name string) (bool, error) {
	return runMCP(h, []string{"mcp", "remove", name}, missingRe)
}

// runMCP runs an mcp subcommand. changed is false when it failed with an
// expected message (already there, nothing to remove).
func runMCP(h tool.Home, args []string, benign *regexp.Regexp) (changed bool, err error) {
	if h.Bin == "" {
		return false, &tool.CommandError{Output: "codex is not installed"}
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
