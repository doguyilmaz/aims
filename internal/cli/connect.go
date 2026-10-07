package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/mcpserver"
	"github.com/doguyilmaz/aims/internal/ops"
	"github.com/doguyilmaz/aims/internal/profiles"
	"github.com/doguyilmaz/aims/internal/shell"
	"github.com/doguyilmaz/aims/internal/statusline"
	"github.com/doguyilmaz/aims/internal/tool"
	"github.com/doguyilmaz/aims/internal/tools"
	"github.com/doguyilmaz/aims/internal/ui"
)

func newSetupCmd() *cobra.Command {
	var all, noSkill, noMCP, statusLine bool
	var sh, only string
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Connect aims to Claude Code and Codex (skill, MCP server, status line, shell)",
		Long: "Installs the aims skill and MCP server so Claude Code and Codex can check and\n" +
			"switch accounts themselves. --statusline shows the account and plan usage in\n" +
			"Claude Code; --shell makes plain `claude` and `codex` follow the active account.",
		Example: `aims setup --all
aims setup --statusline --shell zsh
aims setup --tools codex --no-skill`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ids, err := parseTools(only)
			if err != nil {
				return err
			}
			in := ops.Integrations{Skill: !noSkill, MCP: !noMCP, StatusLine: statusLine || all}
			if sh != "" || all {
				s := shell.Detect()
				if sh != "" && sh != "auto" {
					var ok bool
					if s, ok = shell.Parse(sh); !ok {
						return fmt.Errorf("unknown shell %q", sh)
					}
				}
				in.Shell = s
			}
			steps, err := ops.Setup(cmd.Context(), ids, in)
			if err != nil {
				return err
			}
			return reportSteps(steps, "restart running sessions to pick these up")
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "everything: skill, MCP server, status line and shell integration")
	cmd.Flags().BoolVar(&noSkill, "no-skill", false, "skip the skill")
	cmd.Flags().BoolVar(&noMCP, "no-mcp", false, "skip the MCP server")
	cmd.Flags().BoolVar(&statusLine, "statusline", false, "show account and plan usage in Claude Code's status line")
	cmd.Flags().StringVar(&sh, "shell", "", "add shell integration to this shell's rc file (auto, bash, zsh, fish)")
	cmd.Flags().Lookup("shell").NoOptDefVal = "auto"
	cmd.Flags().StringVar(&only, "tools", "", "comma-separated tools (default: all)")
	return cmd
}

func parseTools(list string) ([]tool.ID, error) {
	if list == "" {
		return nil, nil
	}
	var ids []tool.ID
	for _, n := range strings.Split(list, ",") {
		a, err := parseTool(strings.TrimSpace(n))
		if err != nil {
			return nil, err
		}
		ids = append(ids, a.ID())
	}
	return ids, nil
}

func reportSteps(steps []ops.Step, after string) error {
	var failed int
	for _, s := range steps {
		where := s.What
		if s.Tool != "" {
			where = string(s.Tool) + " " + s.What
		}
		if s.Err != nil {
			failed++
			ui.Warn("%s %s: %v", where, ui.Err.Dim.Render(s.Target), s.Err)
			continue
		}
		ui.Done("%s %s", where, ui.Err.Dim.Render(s.Target))
	}
	if len(steps) > 0 && after != "" {
		ui.Hint("%s", after)
	}
	if failed > 0 {
		return fmt.Errorf("%d step(s) failed", failed)
	}
	return nil
}

func newCleanCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:     "clean",
		Aliases: []string{"uninstall"},
		Short:   "Take aims out of your tools and forget its settings; logins stay",
		Long: "Removes the skill, MCP server, status line and shell line, and forgets aims'\n" +
			"settings and marks. Every login and account folder stays, so `aims init`\n" +
			"brings them back without logging in again. ~/.claude and ~/.codex are not touched.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !yes && ui.Interactive() {
				ok := false
				if err := ui.NewRail().Confirm("Take aims out of your tools and forget its settings?", "Logins, account folders, conversations and skills stay.", &ok); err != nil || !ok {
					return err
				}
			}
			steps, err := ops.Clean(cmd.Context())
			if err != nil {
				return err
			}
			if len(steps) == 0 {
				ui.Done("nothing to clean")
				return nil
			}
			return reportSteps(steps, "start again any time with: aims init")
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "do not ask")
	return cmd
}

func newWipeCmd() *cobra.Command {
	var yes, force bool
	cmd := &cobra.Command{
		Use:   "wipe",
		Short: "Clean, and also log out and delete the account folders aims created",
		Long: "Logs out every account aims created and deletes its folder (anything shared that\n" +
			"was written there moves to ~/.claude or ~/.codex first), then cleans. Your\n" +
			"conversations, sessions, skills, settings and main logins in ~/.claude and\n" +
			"~/.codex are never touched. Custom --dir folders are forgotten, not deleted.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			deleted, kept, err := ops.WipePlan()
			if err != nil {
				return err
			}
			if !yes {
				if !ui.Interactive() {
					return errors.New("wipe logs out and deletes account folders; add --yes to confirm")
				}
				lines := []string{"Conversations, skills, settings and main logins stay."}
				if len(deleted) > 0 {
					lines = append([]string{"Deletes: " + strings.Join(deleted, ", ")}, lines...)
				}
				if len(kept) > 0 {
					lines = append(lines, "Keeps (custom folders): "+strings.Join(kept, ", "))
				}
				ok := false
				if err := ui.NewRail().Confirm("Log out and delete the accounts aims created, then clean?", strings.Join(lines, "\n"), &ok); err != nil || !ok {
					return err
				}
			}
			steps, err := ops.Wipe(cmd.Context(), force)
			if len(steps) == 0 && err == nil {
				ui.Done("nothing to wipe")
				return nil
			}
			if rerr := reportSteps(steps, ""); err == nil {
				err = rerr
			}
			return err
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "do not ask")
	cmd.Flags().BoolVar(&force, "force", false, "delete account folders even if something in them is not shared")
	return cmd
}

func newSyncCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sync [tool]",
		Short: "Repair shared folders and copy new MCP servers into profiles",
		Long: "Runs on every launch for the profile being started; this does it for all of\n" +
			"them at once, and copies Claude Code MCP servers added to your main login.",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeTools,
		RunE: func(cmd *cobra.Command, args []string) error {
			ids := tools.IDs()
			if len(args) == 1 {
				a, err := parseTool(args[0])
				if err != nil {
					return err
				}
				ids = []tool.ID{a.ID()}
			}
			cfg, err := config.Update(func(c *config.Config) error {
				for _, id := range ids {
					if len(c.Tools[id].Profiles) > 0 {
						c.EnsureHub(id)
					}
				}
				return nil
			})
			if err != nil {
				return err
			}
			for _, id := range ids {
				for _, n := range cfg.Tools[id].Order {
					reportLinks(id, n, profiles.Sync(cfg, tools.Get(id), n, false))
				}
			}
			added, err := ops.SyncMCP(cmd.Context())
			for p, names := range added {
				ui.Info("%s: added MCP servers %s", p, strings.Join(names, ", "))
			}
			if err != nil {
				return err
			}
			ui.Done("shared folders are in order")
			return nil
		},
	}
}

func newShellInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "shell-init [shell]",
		Short: "Print the shell integration (wrappers and completion)",
		Long: "Makes plain `claude` and `codex` run as the active profile and adds completion\n" +
			"for aims. `aims setup --shell` adds this line to your rc file for you.",
		Example: `eval "$(aims shell-init zsh)"
aims shell-init fish | source`,
		Args:      cobra.MaximumNArgs(1),
		ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
		RunE: func(cmd *cobra.Command, args []string) error {
			s := shell.Detect()
			if len(args) == 1 {
				var ok bool
				if s, ok = shell.Parse(args[0]); !ok {
					return fmt.Errorf("unknown shell %q", args[0])
				}
			}
			fmt.Fprint(cmd.OutOrStdout(), shell.Init(s, ops.Command()))
			return nil
		},
	}
}

func newMCPCmd(version string) *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Run the MCP server (started by Claude Code and Codex)",
		Long: "Speaks MCP over stdio. `aims setup` registers it; you do not run it yourself.\n" +
			"Tools: aims_status, aims_switch, aims_failover, aims_clear, aims_run, aims_login_help.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if ui.Interactive() {
				ui.Info("this is the MCP server; it waits for a client on stdin (Ctrl+C to stop)")
			}
			return mcpserver.Serve(cmd.Context(), version)
		},
	}
}

func newStatusLineCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "statusline",
		Short:  "Claude Code status line command",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return statusline.Run(cmd.Context(), os.Stdout)
		},
	}
}

var errNoTools = errors.New("neither Claude Code nor Codex is installed")
