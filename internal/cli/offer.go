package cli

import (
	"os"
	"slices"
	"strings"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/ops"
	"github.com/doguyilmaz/aims/internal/shell"
	"github.com/doguyilmaz/aims/internal/ui"
)

// integration is one thing aims can add, as the setup checklist names it.
type integration struct {
	name, what string
	on         func(ops.Integrations) bool
}

var integrations = []integration{
	{"mcp", "Claude Code and Codex can check and switch accounts themselves", func(in ops.Integrations) bool { return in.MCP || in.Skill }},
	{"statusline", "the account and its usage in Claude Code's status line", func(in ops.Integrations) bool { return in.StatusLine }},
	{"shell", "plain `claude` and `codex` use the active account", func(in ops.Integrations) bool { return in.Shell != "" }},
	{"keepalive", "logins kept alive while an account is not used", func(in ops.Integrations) bool { return in.KeepAlive }},
	{"trusthooks", "Codex sessions aims restarts on their own run hooks without stopping for review", func(in ops.Integrations) bool { return in.TrustHooks }},
}

// markOffered records that the user has seen every integration.
func markOffered() {
	_, _ = config.Update(func(c *config.Config) error {
		for _, i := range integrations {
			if !slices.Contains(c.Offered, i.name) {
				c.Offered = append(c.Offered, i.name)
			}
		}
		return nil
	})
}

// offerNew points out, once, the integrations the user was never asked
// about and does not have: what a new version of aims added. It runs after
// a command, so a tool's screen cannot hide it.
func offerNew(args []string) {
	if !ui.IsTerminal(os.Stderr) || os.Getenv("AIMS_NO_UPDATE_CHECK") != "" {
		return
	}
	if len(args) > 0 && slices.Contains([]string{"mcp", "statusline", "hook", "shell-init", "env", "completion", "__complete", "__completeNoDesc", "init", "setup", "claude", "codex", "run"}, args[0]) {
		return
	}
	cfg, err := config.Load()
	if err != nil || !hasProfiles(cfg) {
		return
	}
	fresh := unoffered(cfg, ops.Installed(), shell.RCFile(shell.Detect()) != "")
	markOffered()
	if len(fresh) > 0 {
		ui.Info("aims can now add: %s. Choose with %s", strings.Join(fresh, "; "), ui.Kbd(ui.Err, "aims setup"))
	}
}

// unoffered is what aims can add that the user was never asked about and
// does not have.
func unoffered(cfg *config.Config, current ops.Integrations, hasRC bool) []string {
	var out []string
	for _, i := range integrations {
		if slices.Contains(cfg.Offered, i.name) || i.on(current) {
			continue
		}
		if (i.name == "statusline" && cfg.Tools["claude"].Hub == "") || (i.name == "shell" && !hasRC) || (i.name == "trusthooks" && (cfg.Tools["codex"] == nil || cfg.Tools["codex"].Hub == "")) {
			continue
		}
		out = append(out, i.what)
	}
	return out
}

func hasProfiles(cfg *config.Config) bool {
	for _, t := range cfg.Tools {
		if len(t.Profiles) > 0 {
			return true
		}
	}
	return false
}
