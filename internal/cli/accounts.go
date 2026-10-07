package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/launch"
	"github.com/doguyilmaz/aims/internal/links"
	"github.com/doguyilmaz/aims/internal/ops"
	"github.com/doguyilmaz/aims/internal/proc"
	"github.com/doguyilmaz/aims/internal/profiles"
	"github.com/doguyilmaz/aims/internal/tool"
	"github.com/doguyilmaz/aims/internal/ui"
)

func newLoginCmd() *cobra.Command {
	var share string
	cmd := &cobra.Command{
		Use:   "login <tool> <profile> [-- tool login flags]",
		Short: "Log a profile in (creates it when new)",
		Long: "Runs the tool's own browser login for this profile. Logins are never copied\n" +
			"between profiles: each account logs in once, into its own folder.",
		Example: `aims login claude work -- --email you@company.com
aims login codex work -- --device-auth`,
		Args:              cobra.MinimumNArgs(2),
		ValidArgsFunction: completeToolProfile,
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := parseTool(args[0])
			if err != nil {
				return err
			}
			return login(cmd.Context(), a, args[1], share, args[2:])
		},
	}
	cmd.Flags().StringVar(&share, "share", "", "for a new profile: all, settings or none (default all)")
	return cmd
}

// login runs the tool's login for a profile, creating the profile first if
// needed, and reports who it ended up logged in as.
func login(ctx context.Context, a tool.Adapter, name, share string, extra []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.Tools[a.ID()].Profiles[name] == nil {
		added, err := ops.AddProfile(a, name, ops.AddOptions{Share: share})
		if err != nil {
			return err
		}
		reportLinks(a.ID(), name, added.Reports)
		ui.Done("created %s profile %s", a.Title(), name)
		if cfg, err = config.Load(); err != nil {
			return err
		}
	}
	bin := profiles.Bin(a)
	if bin == "" {
		return fmt.Errorf("%s is not installed", a.Title())
	}
	reportLinks(a.ID(), name, profiles.Sync(cfg, a, name, false))
	ui.Info("logging %s in to %s", ui.Err.Bold.Render(name), a.Title())
	ui.Hint("%s", a.Commands().LoginTip)
	env, _ := profiles.Env(cfg, a, name, os.Environ())
	code, err := proc.RunAttached(bin, append(a.Commands().Login, extra...), env, "")
	if err != nil {
		return err
	}
	if code != 0 {
		return exitCode(code)
	}
	res, err := ops.RecordLogin(ctx, a, name)
	if err != nil {
		return err
	}
	who := res.Email
	if who == "" {
		who = "an account aims cannot name"
	}
	if res.Plan != "" {
		who += " (" + res.Plan + ")"
	}
	ui.Done("%s/%s is logged in as %s", a.ID(), name, who)
	if len(res.SameAs) > 0 {
		ui.Warn("%s is already used by %s; log in again in a private browser window with the other account", res.Email, strings.Join(res.SameAs, ", "))
	}
	return nil
}

func newLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "logout <tool> <profile>",
		Short:             "Log a profile out",
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: completeToolProfile,
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := parseTool(args[0])
			if err != nil {
				return err
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if _, err := cfg.Profile(a.ID(), args[1]); err != nil {
				return err
			}
			bin := profiles.Bin(a)
			if bin == "" {
				return fmt.Errorf("%s is not installed", a.Title())
			}
			env, _ := profiles.Env(cfg, a, args[1], os.Environ())
			code, err := proc.RunAttached(bin, a.Commands().Logout, env, "")
			if err != nil || code != 0 {
				if err == nil {
					err = exitCode(code)
				}
				return err
			}
			_ = config.UpdateState(a.ID(), args[1], func(ps *config.ProfileState) { ps.Account = nil })
			ui.Done("%s/%s is logged out", a.ID(), args[1])
			return nil
		},
	}
}

func newAddCmd() *cobra.Command {
	var existing, doLogin bool
	var share, dir string
	cmd := &cobra.Command{
		Use:   "add <tool> <profile>",
		Short: "Add a profile without logging in",
		Example: `aims add claude personal --existing   # adopt the login you already have
aims add codex work --share settings     # keep conversations separate`,
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: completeTools,
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := parseTool(args[0])
			if err != nil {
				return err
			}
			res, err := ops.AddProfile(a, args[1], ops.AddOptions{Existing: existing, Share: share, Dir: dir})
			if err != nil {
				return err
			}
			reportLinks(a.ID(), args[1], res.Reports)
			what := "in " + fsx.Tildify(res.Dir)
			if existing {
				what = "using the login in " + fsx.Tildify(res.Dir)
			}
			ui.Done("added %s profile %s %s", a.Title(), ui.Err.Bold.Render(args[1]), ui.Err.Dim.Render(what))
			if res.Active {
				ui.Hint("it is the active profile")
			}
			if doLogin {
				return login(cmd.Context(), a, args[1], "", nil)
			}
			if existing {
				_, _ = ops.RecordLogin(cmd.Context(), a, args[1])
			} else {
				ui.Hint("next: %s", ui.Kbd(ui.Err, "aims login "+args[0]+" "+args[1]))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&existing, "existing", false, "adopt the login already in the tool's own folder")
	cmd.Flags().StringVar(&share, "share", "", "all, settings (own history) or none (default all)")
	cmd.Flags().StringVar(&dir, "dir", "", "use this folder instead of ~/.aims/profiles/<tool>/<profile>")
	cmd.Flags().BoolVar(&doLogin, "login", false, "log in right away")
	return cmd
}

func newFailoverCmd() *cobra.Command {
	var from, to, reason string
	var minutes int
	var resume bool
	cmd := &cobra.Command{
		Use:   "failover <tool>",
		Short: "Mark the current account as limited and switch to the next",
		Long: "Marks the account you used last as limited (until its reset time when aims\n" +
			"knows it) and makes the next usable account active. Conversations are\n" +
			"shared, so --resume continues the one you just left on the new account.",
		Example: `aims failover claude --resume
aims failover codex --to work --minutes 90`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeTools,
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := parseTool(args[0])
			if err != nil {
				return err
			}
			res, err := ops.Failover(cmd.Context(), a, from, to, time.Duration(minutes)*time.Minute, reason, "aims failover")
			if err != nil {
				if errors.Is(err, ops.ErrNoTarget) {
					return fmt.Errorf("%w; add one with: aims login %s <name>", err, a.ID())
				}
				return err
			}
			if res.Login {
				ui.Done("%s now uses %s; %s is marked as logged out", a.Title(), ui.Err.Bold.Render(res.To), res.From)
				ui.Hint("log it in again: aims login %s %s", a.ID(), res.From)
			} else {
				ui.Done("%s now uses %s; %s rests (%s)", a.Title(), ui.Err.Bold.Render(res.To), res.From, ui.Until(res.Until))
			}
			if res.FromLastUsed {
				ui.Hint("%s was marked because it was used last; another one? aims clear %s %s && aims failover %s --from <name>", res.From, a.ID(), res.From, a.ID())
			}
			if res.Pinned != "" && res.Pinned != res.To {
				ui.Hint("this terminal is pinned to %s; aims skips it while it rests", res.Pinned)
			}
			resumeArgs := a.Commands().Resume
			if resume {
				r, err := launch.Run(cmd.Context(), launch.Options{Tool: a, Profile: res.To, Args: resumeArgs, Notify: notify})
				if err != nil {
					return err
				}
				return exitCode(r.Code)
			}
			ui.Hint("continue where you were: %s", ui.Kbd(ui.Err, "aims "+string(a.ID())+" "+strings.Join(resumeArgs, " ")))
			return nil
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "profile to mark (default: the one used last)")
	cmd.Flags().StringVar(&to, "to", "", "profile to switch to (default: the next usable one)")
	cmd.Flags().IntVar(&minutes, "minutes", 0, "rest period when the reset time is unknown")
	cmd.Flags().StringVar(&reason, "reason", "", "limit (default) or login")
	cmd.Flags().BoolVar(&resume, "resume", false, "continue the last conversation on the new account")
	return cmd
}

func newClearCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "clear <tool> [profile]",
		Short:             "Forget cooldown and login marks",
		Args:              cobra.RangeArgs(1, 2),
		ValidArgsFunction: completeToolProfile,
		RunE: func(_ *cobra.Command, args []string) error {
			a, err := parseTool(args[0])
			if err != nil {
				return err
			}
			name := ""
			if len(args) == 2 {
				name = args[1]
			}
			names, err := ops.ClearMarks(a, name)
			if err != nil {
				return err
			}
			ui.Done("cleared %s: %s", a.ID(), strings.Join(names, ", "))
			return nil
		},
	}
}

func newOrderCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "order <tool> <profile>...",
		Short:   "Set the order failover tries profiles in",
		Example: `aims order claude work personal`,
		Args:    cobra.MinimumNArgs(2),
		ValidArgsFunction: func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 0 {
				return completeTools(nil, args, "")
			}
			return profileNames(args[0]), cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(_ *cobra.Command, args []string) error {
			a, err := parseTool(args[0])
			if err != nil {
				return err
			}
			order, err := ops.SetOrder(a, args[1:])
			if err != nil {
				return err
			}
			ui.Done("%s failover order: %s", a.Title(), strings.Join(order, " → "))
			return nil
		},
	}
}

func newRemoveCmd() *cobra.Command {
	var purge, force, yes bool
	cmd := &cobra.Command{
		Use:     "rm <tool> <profile>",
		Aliases: []string{"remove"},
		Short:   "Remove a profile",
		Long: "Forgets the profile. With --purge it is logged out and its folder deleted,\n" +
			"after anything shared that was written there moved to the shared folder.\n" +
			"Shared history and settings are never deleted.",
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: completeToolProfile,
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := parseTool(args[0])
			if err != nil {
				return err
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if _, err := cfg.Profile(a.ID(), args[1]); err != nil {
				return err
			}
			if purge && !yes {
				if !ui.Interactive() {
					return errors.New("--purge deletes the profile folder; add --yes to confirm")
				}
				ok := false
				q := fmt.Sprintf("Delete %s and the login in it?", fsx.Tildify(cfg.Dir(a.ID(), args[1])))
				if err := ui.NewRail().Confirm(q, "Shared history and settings are kept.", &ok); err != nil {
					return err
				}
				if !ok {
					return nil
				}
			}
			res, err := ops.RemoveProfile(cmd.Context(), a, args[1], purge, force)
			reportLinks(a.ID(), args[1], res.Reports)
			if res.LogoutErr != nil {
				ui.Warn("could not log %s/%s out first (%v); its login may stay in the system keychain", a.ID(), args[1], res.LogoutErr)
			}
			if err != nil {
				return err
			}
			msg := "removed " + string(a.ID()) + " profile " + args[1]
			if res.Purged {
				msg += " and deleted " + fsx.Tildify(res.Dir)
			}
			ui.Done("%s", msg)
			return nil
		},
	}
	cmd.Flags().BoolVar(&purge, "purge", false, "also delete the profile folder")
	cmd.Flags().BoolVar(&force, "force", false, "purge even if some shared entries could not be moved")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "do not ask")
	return cmd
}

// reportLinks mentions link repairs worth knowing about.
func reportLinks(id tool.ID, name string, reports []links.Report) {
	for _, r := range reports {
		switch r.Action {
		case links.Conflict:
			ui.Warn("%s/%s: %s: %s", id, name, r.Name, r.Detail)
		case links.Merged, links.Adopted:
			detail := ""
			if r.Detail != "" {
				detail = " (" + r.Detail + ")"
			}
			ui.Info("%s/%s: moved %s into the shared folder%s", id, name, r.Name, detail)
		case links.Detached:
			ui.Info("%s/%s: %s is %s", id, name, r.Name, r.Detail)
		}
	}
}
