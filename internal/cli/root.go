// Package cli wires aims' commands onto Cobra.
package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"runtime/debug"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/launch"
	"github.com/doguyilmaz/aims/internal/tool"
	"github.com/doguyilmaz/aims/internal/tools"
	"github.com/doguyilmaz/aims/internal/ui"
)

// exitError carries a child's exit status without printing anything.
type exitError struct{ code int }

func (e exitError) Error() string { return fmt.Sprintf("exit status %d", e.code) }

// Execute runs aims and returns the process exit code.
func Execute(version string) int {
	version = resolveVersion(version)
	ctx := context.Background()
	args := rewrite(os.Args[1:])
	root := newRoot(version)
	root.SetArgs(args)
	notice := startUpdateCheck(ctx, version, args)
	err := root.ExecuteContext(ctx)
	code := 0
	var ee exitError
	switch {
	case errors.As(err, &ee):
		code = ee.code
	case errors.Is(err, ui.ErrCancelled):
		code = 130
	case err != nil:
		ui.Error(err)
		code = 1
		if strings.Contains(err.Error(), "unknown command") || strings.Contains(err.Error(), "unknown flag") {
			code = 2
		}
	}
	notice.finish(version)
	return code
}

// resolveVersion fills in the module version for `go install` builds.
func resolveVersion(v string) string {
	if v != "" && v != "dev" {
		return v
	}
	// `go install ...@v1.2.3` records the tag; a local build records a
	// pseudo-version, which is no more useful than "dev".
	if bi, ok := debug.ReadBuildInfo(); ok && releaseTag.MatchString(bi.Main.Version) {
		return bi.Main.Version
	}
	return "dev"
}

var releaseTag = regexp.MustCompile(`^v\d+\.\d+\.\d+(-(rc|beta|alpha)\.?\d*)?$`)

var toolAt = regexp.MustCompile(`^([a-z][a-z0-9-]*)[@:]([A-Za-z0-9][A-Za-z0-9_.-]*)$`)

// rewrite turns `aims claude@work ...` into `aims run claude@work ...`.
func rewrite(args []string) []string {
	if len(args) > 0 {
		if m := toolAt.FindStringSubmatch(args[0]); m != nil {
			if _, ok := tools.Parse(m[1]); ok {
				return append([]string{"run"}, args...)
			}
		}
	}
	return args
}

func newRoot(version string) *cobra.Command {
	cobra.EnableCommandSorting = false
	root := &cobra.Command{
		Use:   "aims",
		Short: "Several Claude Code and Codex accounts on one machine",
		Long: "aims (AI multi-session) keeps one login per account for Claude Code and Codex,\n" +
			"switches between them instantly, shares history and settings across them, and\n" +
			"moves to the next account when one hits its usage limit or needs a new login.",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return home(cmd.Context(), cmd, version)
		},
	}
	root.SetVersionTemplate("aims {{.Version}}\n")
	root.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		return fmt.Errorf("%w (see: %s --help)", err, c.CommandPath())
	})
	root.AddGroup(
		&cobra.Group{ID: "start", Title: "Get started"},
		&cobra.Group{ID: "daily", Title: "Every day"},
		&cobra.Group{ID: "accounts", Title: "Accounts"},
		&cobra.Group{ID: "connect", Title: "Integrations"},
		&cobra.Group{ID: "maintain", Title: "Maintenance"},
	)
	add := func(group string, cmds ...*cobra.Command) {
		for _, c := range cmds {
			c.GroupID = group
			root.AddCommand(c)
		}
	}
	add("start", newInitCmd(version), newLoginCmd())
	add("daily", newStatusCmd(), newUseCmd(), newRunCmd(), newEnvCmd())
	for _, a := range tools.All() {
		add("daily", newToolCmd(a))
	}
	add("accounts", newAddCmd(), newFailoverCmd(), newClearCmd(), newOrderCmd(), newLogoutCmd(), newRemoveCmd())
	add("connect", newSetupCmd(), newUninstallCmd(), newSyncCmd(), newShellInitCmd(), newMCPCmd(version))
	add("maintain", newDoctorCmd(version), newUpgradeCmd(version))
	root.AddCommand(newStatusLineCmd())
	root.SetCompletionCommandGroupID("maintain")
	root.SetHelpCommandGroupID("maintain")
	installHelp(root)
	return root
}

// home is bare `aims`: the dashboard on a terminal, the status otherwise.
func home(ctx context.Context, cmd *cobra.Command, version string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if !ui.Interactive() {
		return printStatus(ctx, cmd, nil, false)
	}
	if !cfg.HasProfiles() {
		fmt.Println()
		fmt.Println("  " + ui.Out.Title.Render("aims") + "  " + ui.Out.Dim.Render("several Claude Code and Codex accounts on one machine"))
		fmt.Println()
		start := true
		if err := ui.NewRail().Confirm("No accounts yet. Set them up now?", "", &start); err != nil {
			return err
		}
		if !start {
			fmt.Println("  Run " + ui.Kbd(ui.Out, "aims init") + " whenever you are ready.")
			return nil
		}
		return runInit(ctx, initOptions{version: version})
	}
	req, err := ui.RunDashboard(ctx, version)
	if err != nil || req == nil {
		return err
	}
	res, err := launch.Run(ctx, launch.Options{Tool: tools.Get(req.Tool), Profile: req.Profile, Notify: notify})
	if err != nil {
		return err
	}
	return exitCode(res.Code)
}

func exitCode(code int) error {
	if code == 0 {
		return nil
	}
	return exitError{code}
}

// notify prints launch progress.
func notify(l launch.Level, msg string) {
	if l == launch.Warn {
		ui.Warn("%s", msg)
		return
	}
	ui.Info("%s", msg)
}

// --- argument helpers and completion ---

func parseTool(name string) (tool.Adapter, error) {
	a, ok := tools.Parse(name)
	if !ok {
		return nil, fmt.Errorf("unknown tool %q (expected %s)", name, strings.Join(tools.Names(), " or "))
	}
	return a, nil
}

func completeTools(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return tools.Names(), cobra.ShellCompDirectiveNoFileComp
}

// completeToolProfile completes "<tool> <profile>".
func completeToolProfile(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	switch len(args) {
	case 0:
		return tools.Names(), cobra.ShellCompDirectiveNoFileComp
	case 1:
		return profileNames(args[0]), cobra.ShellCompDirectiveNoFileComp
	}
	return nil, cobra.ShellCompDirectiveNoFileComp
}

func profileNames(toolName string) []string {
	cfg, err := config.Load()
	if err != nil {
		return nil
	}
	var out []string
	for _, a := range tools.All() {
		if toolName != "" && string(a.ID()) != toolName {
			continue
		}
		for _, n := range cfg.Tools[a.ID()].Order {
			if !slices.Contains(out, n) {
				out = append(out, n)
			}
		}
	}
	return out
}
