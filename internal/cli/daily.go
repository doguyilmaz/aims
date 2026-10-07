package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/launch"
	"github.com/doguyilmaz/aims/internal/ops"
	"github.com/doguyilmaz/aims/internal/profiles"
	"github.com/doguyilmaz/aims/internal/shell"
	"github.com/doguyilmaz/aims/internal/tool"
	"github.com/doguyilmaz/aims/internal/tools"
	"github.com/doguyilmaz/aims/internal/ui"
)

func newStatusCmd() *cobra.Command {
	var live, asJSON bool
	cmd := &cobra.Command{
		Use:     "status [tool]",
		Aliases: []string{"ls"},
		Short:   "Show every account: login, plan usage, cooldowns",
		Example: `aims status
aims status codex --live
aims status --json`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeTools,
		RunE: func(cmd *cobra.Command, args []string) error {
			var ids []tool.ID
			if len(args) == 1 {
				a, err := parseTool(args[0])
				if err != nil {
					return err
				}
				ids = []tool.ID{a.ID()}
			}
			if live {
				if err := checkLive(cmd.Context(), ids, asJSON); err != nil {
					return err
				}
			}
			return printStatus(cmd.Context(), cmd, ids, asJSON)
		},
	}
	cmd.Flags().BoolVar(&live, "live", false, "ask the providers (Codex: free account API; Claude: a one-word prompt)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	return cmd
}

func checkLive(ctx context.Context, ids []tool.ID, quiet bool) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		ids = tools.IDs()
	}
	for _, id := range ids {
		a := tools.Get(id)
		if profiles.Bin(a) == "" {
			continue
		}
		for _, n := range cfg.Tools[id].Order {
			var p tool.Probe
			run := func() { p = profiles.Live(ctx, cfg, a, n) }
			if quiet || !ui.Interactive() {
				run()
			} else {
				_ = ui.Spin(fmt.Sprintf("checking %s/%s", id, n), run)
			}
			if !quiet && p.Status != tool.ProbeOK {
				ui.Warn("%s/%s: %s%s", id, n, p.Status, suffix(p.Detail))
			}
		}
	}
	return nil
}

func suffix(s string) string {
	if s == "" {
		return ""
	}
	return " (" + s + ")"
}

func printStatus(ctx context.Context, cmd *cobra.Command, ids []tool.ID, asJSON bool) error {
	report, err := ops.Status(ctx, ids)
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	if asJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	s := ui.NewStyles(out)
	fmt.Fprint(out, ui.RenderStatus(s, report, cfg.Failover.Threshold))
	if !cfg.HasProfiles() {
		fmt.Fprintln(out, "\n"+s.Dim.Render("Set up your accounts with ")+ui.Kbd(s, "aims init"))
	}
	return nil
}

func newUseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "use [tool] <profile>",
		Short: "Make a profile the active account for new sessions",
		Long:  "Without a tool, every tool that has a profile with that name switches.",
		Example: `aims use work            # Claude Code and Codex
aims use claude personal`,
		Args: cobra.RangeArgs(1, 2),
		ValidArgsFunction: func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 0 {
				return append(tools.Names(), profileNames("")...), cobra.ShellCompDirectiveNoFileComp
			}
			if len(args) == 1 {
				return profileNames(args[0]), cobra.ShellCompDirectiveNoFileComp
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(_ *cobra.Command, args []string) error {
			var a tool.Adapter
			name := args[0]
			if len(args) == 2 {
				var err error
				if a, err = parseTool(args[0]); err != nil {
					return err
				}
				name = args[1]
			}
			changed, err := ops.Use(a, name)
			if err != nil {
				return err
			}
			var titles []string
			for _, c := range changed {
				titles = append(titles, c.Title())
				if pin := config.Pinned(c.ID()); pin != "" && pin != name {
					ui.Warn("this terminal is pinned to %s %q; %s to follow the active profile", c.ID(), pin, ui.Kbd(ui.Err, `eval "$(aims env --reset)"`))
				}
			}
			ui.Done("New %s sessions use %s", strings.Join(titles, " and "), ui.Err.Bold.Render(name))
			return nil
		},
	}
}

// newToolCmd is `aims claude ...` / `aims codex ...`: the tool itself, as the
// active profile. Every argument goes to the tool untouched.
func newToolCmd(a tool.Adapter) *cobra.Command {
	return &cobra.Command{
		Use:                string(a.ID()) + " [args...]",
		Short:              "Run " + a.Title() + " as the active profile (or " + string(a.ID()) + "@<profile>)",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTool(cmd.Context(), a, "", args)
		},
	}
}

func newRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run <tool>[@profile] [args...]",
		Short: "Run a tool as a profile; tool@profile works without `run`",
		Long: "Runs the tool with the profile's login. Without @profile it uses the active\n" +
			"profile and moves to the next one if that is limited or logged out.\n" +
			"Headless runs (claude -p, codex exec) that fail on a limit before doing\n" +
			"anything are retried on the next account.",
		Example: `aims claude@work
aims codex@personal exec "summarize the diff"
aims run claude -p "explain this error"`,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
				return cmd.Help()
			}
			name, profile, _ := strings.Cut(args[0], "@")
			if p, q, ok := strings.Cut(args[0], ":"); ok && profile == "" {
				name, profile = p, q
			}
			a, err := parseTool(name)
			if err != nil {
				return err
			}
			return runTool(cmd.Context(), a, profile, args[1:])
		},
	}
}

func runTool(ctx context.Context, a tool.Adapter, profile string, args []string) error {
	res, err := launch.Run(ctx, launch.Options{Tool: a, Profile: profile, Args: args, Notify: notify})
	if err != nil {
		return err
	}
	return exitCode(res.Code)
}

func newEnvCmd() *cobra.Command {
	var sh string
	var reset bool
	cmd := &cobra.Command{
		Use:   "env [tool] [profile]",
		Short: "Print exports that pin this terminal to a profile",
		Long: "Pins only the current terminal: it sets " + config.PinVar("claude") + " (and the tool's\n" +
			"config variable, so even the bare binary uses that login) until --reset.",
		Example: `eval "$(aims env work)"
eval "$(aims env claude personal)"
eval "$(aims env --reset)"`,
		Args:              cobra.MaximumNArgs(2),
		ValidArgsFunction: completeToolProfile,
		RunE: func(cmd *cobra.Command, args []string) error {
			s := shell.Detect()
			if sh != "" {
				var ok bool
				if s, ok = shell.Parse(sh); !ok {
					return fmt.Errorf("unknown shell %q", sh)
				}
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			var ids []tool.ID
			profile := ""
			if len(args) > 0 {
				if a, ok := tools.Parse(args[0]); ok {
					ids = []tool.ID{a.ID()}
					args = args[1:]
				}
			}
			if len(args) > 0 {
				profile = args[0]
			}
			out, err := shell.EnvLines(s, cfg, ids, profile, reset)
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), out)
			if ui.IsTerminal(os.Stdout) {
				ui.Hint(`this only prints; apply it with: eval "$(aims env %s)"`, strings.Join(append(idStrings(ids), profile), " "))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&sh, "shell", "", "bash, zsh, fish or powershell (default: detected)")
	cmd.Flags().BoolVar(&reset, "reset", false, "unpin this terminal")
	return cmd
}

func idStrings(ids []tool.ID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = string(id)
	}
	return out
}
