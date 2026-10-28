package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
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
		Short:   "Show accounts, open sessions and next steps",
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
				ui.Spin(fmt.Sprintf("checking %s/%s", id, n), run)
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
	fmt.Fprint(out, ui.RenderStatus(s, report, cfg))
	return nil
}

func newUseCmd() *cobra.Command {
	var global bool
	cmd := &cobra.Command{
		Use:   "use [tool] <profile>",
		Short: "Choose the account new sessions start on",
		Long: "In the project aims runs in (the git repository, or the folder outside one),\n" +
			"unless --global. Without a tool, every tool that has a profile with that name\n" +
			"switches.",
		Example: `aims use work            # Claude Code and Codex, in this project
aims use claude personal
aims use --global work   # every folder without an order of its own`,
		Args: cobra.MaximumNArgs(2),
		ValidArgsFunction: func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 0 {
				return append(tools.Names(), profileNames("")...), cobra.ShellCompDirectiveNoFileComp
			}
			if len(args) == 1 {
				return profileNames(args[0]), cobra.ShellCompDirectiveNoFileComp
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			var a tool.Adapter
			var name string
			var err error
			switch len(args) {
			case 0:
				if !ui.Interactive() {
					return errors.New("which account? e.g. aims use work, or aims use claude personal")
				}
				if a, name, err = pickProfile(cmd.Context()); err != nil {
					return err
				}
			case 1:
				name = args[0]
			default:
				if a, err = parseTool(args[0]); err != nil {
					return err
				}
				name = args[1]
			}
			key, err := scopeKey(global)
			if err != nil {
				return err
			}
			changed, err := ops.Use(a, name, key)
			if err != nil {
				return err
			}
			var titles []string
			for _, c := range changed {
				titles = append(titles, c.Title())
				warnPinned(c, name)
			}
			ui.Done("%s", ui.OrderSummary(strings.Join(titles, " and "), key, []string{name}, ui.Err.Bold))
			afterScope(changed, key, "aims use --global "+name)
			return nil
		},
	}
	cmd.Flags().BoolVar(&global, "global", false, "for every folder without an order of its own")
	return cmd
}

// scopeKey is the project use and order change: the one aims runs in, or
// none (every folder) with --global.
func scopeKey(global bool) (string, error) {
	if global {
		return "", nil
	}
	key := config.Here()
	if key == "" {
		return "", errors.New("cannot tell which folder this is; --global changes every folder")
	}
	return key, nil
}

// afterScope says, after a change to key's order, what holds elsewhere:
// for a project, the other folders' order and the command for all of them;
// for every folder, the projects that keep their own.
func afterScope(changed []tool.Adapter, key, globalCmd string) {
	cfg, err := config.Load()
	if err != nil {
		return
	}
	for _, a := range changed {
		t := cfg.Tools[a.ID()]
		switch {
		case key != "":
			ui.Hint("%s elsewhere: %s; every folder: %s", a.Title(), strings.Join(t.Everywhere, " → "), ui.Kbd(ui.Err, globalCmd))
		case t.Project != "":
			ui.Warn("%s keeps its own %s order: %s; drop it with %s", ui.Where(t.Project), a.Title(), strings.Join(t.Order, " → "), ui.Kbd(ui.Err, "aims order --reset"))
		}
		others := len(t.Projects)
		if _, ok := t.Projects[t.Project]; ok {
			others--
		}
		if key == "" && others > 0 {
			ui.Hint("%d other folder(s) keep their own %s order (aims status)", others, a.Title())
		}
	}
}

// warnPinned says when this terminal will not follow a new active profile.
func warnPinned(a tool.Adapter, active string) {
	if pin := config.Pinned(a.ID()); pin != "" && pin != active {
		ui.Warn("this terminal is pinned to %s %q; %s to follow the active profile", a.ID(), pin, ui.Kbd(ui.Err, `eval "$(aims env --reset)"`))
	}
}

// pickProfile asks which tool (or every tool) and which account.
func pickProfile(ctx context.Context) (tool.Adapter, string, error) {
	report, err := ops.Status(ctx, nil)
	if err != nil {
		return nil, "", err
	}
	var withProfiles []ops.ToolStatus
	for _, t := range report {
		if len(t.Profiles) > 0 {
			withProfiles = append(withProfiles, t)
		}
	}
	if len(withProfiles) == 0 {
		return nil, "", errors.New("no accounts yet; set them up with: aims init")
	}
	r := ui.NewRail()
	toolID := string(withProfiles[0].ID)
	if len(withProfiles) > 1 {
		opts := []ui.Option{{Label: "Every tool", Value: "", Hint: "switch each tool that has the account"}}
		for _, t := range withProfiles {
			opts = append(opts, ui.Option{Label: t.Title, Value: string(t.ID), Hint: "now " + t.Active})
		}
		toolID = ""
		if err := r.Select("Switch which tool?", "", opts, &toolID); err != nil {
			return nil, "", err
		}
	}
	var opts []ui.Option
	pick := ""
	for _, t := range withProfiles {
		if toolID != "" && string(t.ID) != toolID {
			continue
		}
		for _, p := range t.Profiles {
			if slices.ContainsFunc(opts, func(o ui.Option) bool { return o.Value == p.Name }) {
				continue
			}
			hint := p.Email
			if !p.Usable {
				hint += " · " + strings.Join(p.Reasons, ", ")
			}
			if toolID == "" {
				hint = ""
			}
			opts = append(opts, ui.Option{Label: p.Name, Value: p.Name, Hint: hint})
			if p.Active && pick == "" {
				pick = p.Name
			}
		}
	}
	if err := r.Select("Which account for new sessions?", "", opts, &pick); err != nil {
		return nil, "", err
	}
	if toolID == "" {
		return nil, pick, nil
	}
	return tools.Get(tool.ID(toolID)), pick, nil
}

// newToolCmd is `aims claude ...` / `aims codex ...`: the tool itself, as the
// active profile. Every argument goes to the tool untouched.
func newToolCmd(a tool.Adapter) *cobra.Command {
	return &cobra.Command{
		Use:                string(a.ID()) + " [args...]",
		Short:              "Start " + a.Title() + " (" + string(a.ID()) + "@<profile> pins it)",
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
			"An interactive session moves on its own: when its account\n" +
			"hits a limit, aims continues the conversation on the next one, and moves\n" +
			"it back once the first works again. Headless runs (claude -p, codex exec)\n" +
			"that fail on a limit before doing anything are retried on the next account.",
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
	ops.Refresh(ctx, a)
	res, err := launch.Run(ctx, launch.Options{Tool: a, Profile: profile, Args: args, Notify: notify})
	if err != nil && res.Code > 1 {
		// An unknown profile (2) or a missing tool (127): say why, keep the code.
		ui.Error(err)
		return exitCode(res.Code)
	}
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
			given := strings.Join(args, " ")
			var ids []tool.ID
			profile := ""
			if len(args) > 0 {
				if a, ok := tools.Parse(args[0]); ok {
					ids = []tool.ID{a.ID()}
					args = args[1:]
				}
			}
			switch len(args) {
			case 0:
			case 1:
				profile = args[0]
			default:
				return fmt.Errorf("unknown tool %q (expected %s)", given[:strings.IndexByte(given, ' ')], strings.Join(tools.Names(), " or "))
			}
			out, err := shell.EnvLines(s, cfg, ids, profile, reset)
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), out)
			if ui.IsTerminal(os.Stdout) {
				line := strings.TrimSpace("aims env " + given)
				if reset {
					line += " --reset"
				}
				ui.Hint(`this only prints; apply it with: eval "$(%s)"`, line)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&sh, "shell", "", "bash, zsh, fish or powershell (default: detected)")
	cmd.Flags().BoolVar(&reset, "reset", false, "unpin this terminal")
	return cmd
}
