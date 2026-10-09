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
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/launch"
	"github.com/doguyilmaz/aims/internal/tool"
	"github.com/doguyilmaz/aims/internal/tools"
	"github.com/doguyilmaz/aims/internal/ui"
)

// appVersion is the running version, for commands that start the setup.
var appVersion = "dev"

// exitError carries a child's exit status without printing anything.
type exitError struct{ code int }

func (e exitError) Error() string { return fmt.Sprintf("exit status %d", e.code) }

// Execute runs aims and returns the process exit code.
func Execute(version string) int {
	version = resolveVersion(version)
	appVersion = version
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

// resolveVersion fills in what Go recorded when no version was set at build
// time: the tag for `go install ...@v1.2.3`, the last tag and commit for a
// build from a checkout.
func resolveVersion(v string) string {
	if v != "" && v != "dev" {
		return v
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return localVersion(bi.Main.Version)
	}
	return "dev"
}

// pseudoVersion matches Go's pseudo-versions: vX.0.0-TIME-REV without a tag,
// vX.Y.Z-PRE.0.TIME-REV after a pre-release, vX.Y.(Z+1)-0.TIME-REV after a release.
var pseudoVersion = regexp.MustCompile(`^(v\d+\.\d+\.)(\d+)-(.*?)\d{14}-([0-9a-f]{12})(\+dirty)?$`)

// localVersion names a pseudo-version by the tag it was built on and its
// commit: v0.1.4-0.20261009081856-e93a9a0fe529 is v0.1.3+e93a9a0.
func localVersion(v string) string {
	m := pseudoVersion.FindStringSubmatch(v)
	if m == nil {
		return v
	}
	base := "dev"
	switch patch, _ := strconv.Atoi(m[2]); {
	case m[3] == "0." && patch > 0:
		base = m[1] + strconv.Itoa(patch-1)
	case strings.HasSuffix(m[3], ".0."):
		base = m[1] + m[2] + "-" + strings.TrimSuffix(m[3], ".0.")
	}
	out := base + "+" + m[4][:7]
	if m[5] != "" {
		out += ".dirty"
	}
	return out
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
		Example: `aims init
aims claude@work
aims use work
aims status --live`,
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
	add("connect", newSetupCmd(), newSyncCmd(), newShellInitCmd(), newMCPCmd(version))
	add("maintain", newDoctorCmd(version), newCleanCmd(), newWipeCmd(), newUpgradeCmd(version))
	root.AddCommand(newStatusLineCmd())
	// Commands that act on an account offer the setup when there is none yet.
	needAccounts := []string{"use", "env", "failover", "clear", "order", "logout", "rm", "sync"}
	root.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		if !slices.Contains(needAccounts, cmd.Name()) || cmd.Parent() != root {
			return nil
		}
		_, err := offerInit(cmd.Context())
		return err
	}
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
		return runInit(ctx, defaultInit(version))
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
	switch l {
	case launch.Warn:
		ui.Warn("%s", msg)
	case launch.Hint:
		ui.Hint("%s", msg)
	default:
		ui.Info("%s", msg)
	}
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
