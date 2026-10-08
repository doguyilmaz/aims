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
	"github.com/doguyilmaz/aims/internal/tools"
	"github.com/doguyilmaz/aims/internal/ui"
)

func newLoginCmd() *cobra.Command {
	var share string
	cmd := &cobra.Command{
		Use:   "login [tool] [profile] [-- tool login flags]",
		Short: "Log a profile in (creates it when new)",
		Long: "Runs the tool's own browser login for this profile. Logins are never copied\n" +
			"between profiles: each account logs in once, into its own folder.\n\n" +
			"Without a profile, aims asks which account to log in, or sets up a new one.",
		Example: `aims login
aims login claude work -- --email you@company.com
aims login codex work -- --device-auth`,
		Args:              cobra.ArbitraryArgs,
		ValidArgsFunction: completeToolProfile,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			pos, extra := args, []string(nil)
			if d := cmd.ArgsLenAtDash(); d >= 0 {
				pos, extra = args[:d], args[d:]
			}
			only, name, err := parseTarget(pos)
			if err != nil {
				return err
			}
			if only != nil && name != "" {
				return login(ctx, only, name, share, extra)
			}
			if name != "" {
				return loginByName(ctx, name, share, extra)
			}
			if !ui.Interactive() {
				return errors.New("which account? e.g. aims login claude work")
			}
			return askLogin(ctx, only, share, extra)
		},
	}
	cmd.Flags().StringVar(&share, "share", "", "for a new profile: all, settings or none (default all)")
	return cmd
}

// loginByName logs in the profile called name in the tool that has it,
// asking which when both do, or sets up a new account by that name.
func loginByName(ctx context.Context, name, share string, extra []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	has := toolsWith(cfg, name)
	switch {
	case len(has) == 1:
		return login(ctx, has[0], name, share, extra)
	case !ui.Interactive() && len(has) == 0:
		return fmt.Errorf("no profile named %q; create it with: aims login <tool> %s", name, name)
	case !ui.Interactive():
		return fmt.Errorf("both %s have a profile %q; name the tool, e.g. aims login %s %s", toolIDs(has), name, has[0].ID(), name)
	case len(has) == 0:
		return addAccount(ctx, ui.NewRail(), installedTools(nil), name)
	}
	r := ui.NewRail()
	pick := "_both"
	opts := []ui.Option{{Label: "Both", Value: "_both", Hint: "one after the other"}}
	for _, a := range has {
		opts = append(opts, ui.Option{Label: a.Title(), Value: string(a.ID())})
	}
	if err := r.Select("Log "+name+" in to which tool?", "", opts, &pick); err != nil {
		return cancelled(r, err)
	}
	r.Outro(ui.Out.Dim.Render("opening the login"))
	for _, a := range has {
		if pick != "_both" && string(a.ID()) != pick {
			continue
		}
		if err := login(ctx, a, name, share, extra); err != nil {
			return err
		}
	}
	return nil
}

// askLogin asks which account to log in; the ones that need it come first
// in line. "A new account" sets one up.
func askLogin(ctx context.Context, only tool.Adapter, share string, extra []string) error {
	report, err := loadAccounts(ctx)
	if err != nil {
		return err
	}
	r := ui.NewRail()
	list := accountsOf(report, only, func(x account) bool { return x.t.Installed })
	if len(list) == 0 {
		return addAccount(ctx, r, installedTools(only), "")
	}
	opts := append(accountOptions(list, only == nil && toolsInUse(report) > 1), ui.Option{Label: "A new account", Value: "new", Hint: "name it, then log in"})
	pick := "new"
	for i, x := range list {
		if needsLogin(x.p) {
			pick = opts[i].Value
			break
		}
	}
	if err := r.Select("Log in which account?", "", opts, &pick); err != nil {
		return cancelled(r, err)
	}
	if pick == "new" {
		return addAccount(ctx, r, installedTools(only), "")
	}
	x := list[optionIndex(pick)]
	r.Outro(ui.Out.Dim.Render("opening the " + x.t.Title + " login"))
	return login(ctx, x.adapter(), x.p.Name, share, extra)
}

// login runs the tool's login for a profile, creating the profile first if
// needed, and reports who it ended up logged in as.
func login(ctx context.Context, a tool.Adapter, name, share string, extra []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.Tools[a.ID()].Profiles[name] == nil {
		if ui.Interactive() {
			ok := false
			q := fmt.Sprintf("There is no %s profile %q. Create it?", a.ID(), name)
			if err := ui.NewRail().Confirm(q, "Existing: "+strings.Join(cfg.Tools[a.ID()].Order, ", "), &ok); err != nil || !ok {
				return err
			}
		}
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
		Use:   "logout [tool] [profile]",
		Short: "Log a profile out",
		Long:  "Without a profile, aims asks which logged-in account to log out, and checks first.",
		Example: `aims logout
aims logout claude work`,
		Args:              cobra.MaximumNArgs(2),
		ValidArgsFunction: completeToolProfile,
		RunE: func(cmd *cobra.Command, args []string) error {
			t, r, err := resolve(cmd.Context(), args, "Log which account out?", "aims logout claude work", func(x account) bool {
				return x.t.Installed && (x.p.LoggedIn == nil || *x.p.LoggedIn)
			})
			if errors.Is(err, errNoAccount) {
				return errors.New("no account is logged in")
			}
			if err != nil {
				return err
			}
			if r != nil {
				ok := false
				desc := "Log it in again later with: aims login " + string(t.a.ID()) + " " + t.name
				if err := r.Confirm("Log "+t.String()+" out?", desc, &ok); err != nil {
					return cancelled(r, err)
				}
				if !ok {
					r.Outro(ui.Out.Dim.Render("Nothing changed."))
					return nil
				}
				r.Outro(ui.Out.Dim.Render("logging " + t.String() + " out"))
			}
			return logout(t)
		},
	}
}

// logout runs the tool's own logout for a profile.
func logout(t target) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	bin := profiles.Bin(t.a)
	if bin == "" {
		return fmt.Errorf("%s is not installed", t.a.Title())
	}
	env, _ := profiles.Env(cfg, t.a, t.name, os.Environ())
	code, err := proc.RunAttached(bin, t.a.Commands().Logout, env, "")
	if err != nil || code != 0 {
		if err == nil {
			err = exitCode(code)
		}
		return err
	}
	_ = config.UpdateState(t.a.ID(), t.name, func(ps *config.ProfileState) { ps.Account = nil })
	ui.Done("%s is logged out", t)
	return nil
}

func newAddCmd() *cobra.Command {
	var existing, doLogin bool
	var share, dir string
	cmd := &cobra.Command{
		Use:   "add [tool] [profile]",
		Short: "Add a profile without logging in",
		Long: "Without a profile, aims asks for a name and what the account shares, and\n" +
			"offers to log it in.",
		Example: `aims add
aims add claude personal --existing   # adopt the login you already have
aims add codex work --share settings     # keep conversations separate`,
		Args:              cobra.MaximumNArgs(2),
		ValidArgsFunction: completeTools,
		RunE: func(cmd *cobra.Command, args []string) error {
			only, name, err := parseTarget(args)
			if err != nil {
				return err
			}
			if only == nil || name == "" {
				if !ui.Interactive() || cmd.Flags().NFlag() > 0 {
					return errors.New("which account? e.g. aims add claude work")
				}
				return addAccount(cmd.Context(), ui.NewRail(), installedTools(only), name)
			}
			a := only
			res, err := ops.AddProfile(a, name, ops.AddOptions{Existing: existing, Share: share, Dir: dir})
			if err != nil {
				return err
			}
			reportLinks(a.ID(), name, res.Reports)
			what := "in " + fsx.Tildify(res.Dir)
			if existing {
				what = "using the login in " + fsx.Tildify(res.Dir)
			}
			ui.Done("added %s profile %s %s", a.Title(), ui.Err.Bold.Render(name), ui.Err.Dim.Render(what))
			if res.Active {
				ui.Hint("it is the active profile")
			}
			if doLogin {
				return login(cmd.Context(), a, name, "", nil)
			}
			if existing {
				_, _ = ops.RecordLogin(cmd.Context(), a, name)
			} else {
				ui.Hint("next: %s", ui.Kbd(ui.Err, "aims login "+string(a.ID())+" "+name))
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
		Use:   "failover [tool]",
		Short: "Mark the current account as limited and switch to the next",
		Long: "Marks the account you used last as limited (until its reset time when aims\n" +
			"knows it). New sessions use the next usable account until then and come\n" +
			"back to the active one on their own. Conversations are shared, so --resume\n" +
			"continues the one you just left on the next account.\n\n" +
			"Without a tool, aims asks which account hit its limit and what happened,\n" +
			"and offers to continue the conversation.",
		Example: `aims failover
aims failover claude --resume
aims failover codex --to work --minutes 90`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeTools,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			ask := len(args) == 0 && ui.Interactive()
			var a tool.Adapter
			var r *ui.Rail
			var err error
			if len(args) == 1 {
				a, err = parseTool(args[0])
			} else {
				a, r, err = failoverTool()
			}
			if err != nil {
				return err
			}
			if ask && from == "" {
				if r == nil {
					r = ui.NewRail()
				}
				if from, reason, err = askFailover(ctx, r, a, reason); err != nil {
					return cancelled(r, err)
				}
			}
			res, err := ops.Failover(ctx, a, from, to, time.Duration(minutes)*time.Minute, reason, "aims failover")
			if err != nil {
				if errors.Is(err, ops.ErrNoTarget) {
					err = fmt.Errorf("%w; add one with: aims login %s <name>", err, a.ID())
				}
				if r != nil {
					r.Fail(err.Error())
				}
				return err
			}
			resumeCmd := "aims " + string(a.ID()) + " " + strings.Join(a.Commands().Resume, " ")
			if r == nil {
				ui.Done("%s", failoverText(ui.Err, a, res))
				for _, h := range failoverHints(a, res) {
					ui.Hint("%s", h)
				}
				if !resume {
					ui.Hint("continue where you were: %s", ui.Kbd(ui.Err, resumeCmd))
					return nil
				}
			} else {
				r.OK(failoverText(ui.Out, a, res))
				for _, h := range failoverHints(a, res) {
					r.Hint(h)
				}
				r.Gap()
				if !resume {
					resume = true
					if err := r.Confirm("Continue your last conversation on "+res.To+" now?", resumeCmd, &resume); err != nil {
						return cancelled(r, err)
					}
				}
				if !resume {
					r.Outro(ui.Out.Dim.Render("Later: ") + ui.Kbd(ui.Out, resumeCmd))
					return nil
				}
				r.Outro(ui.Out.Dim.Render("continuing on " + res.To))
			}
			lr, err := launch.Run(ctx, launch.Options{Tool: a, Profile: res.To, Args: a.Commands().Resume, Notify: notify})
			if err != nil {
				return err
			}
			return exitCode(lr.Code)
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "profile to mark (default: the one used last)")
	cmd.Flags().StringVar(&to, "to", "", "make this profile active instead (default: the next usable one, until the mark lifts)")
	cmd.Flags().IntVar(&minutes, "minutes", 0, "rest period when the reset time is unknown")
	cmd.Flags().StringVar(&reason, "reason", "", "limit (default) or login")
	cmd.Flags().BoolVar(&resume, "resume", false, "continue the last conversation on the new account")
	return cmd
}

// failoverTool is the tool `aims failover` acts on when none is named: the
// only one with a second account, or the one the user picks.
func failoverTool() (tool.Adapter, *ui.Rail, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, err
	}
	var candidates []tool.Adapter
	for _, a := range tools.All() {
		if len(cfg.Tools[a.ID()].Profiles) > 1 {
			candidates = append(candidates, a)
		}
	}
	switch {
	case len(candidates) == 0:
		return nil, nil, errors.New("no tool has a second account to fail over to; add one with: aims login")
	case len(candidates) == 1:
		return candidates[0], nil, nil
	case !ui.Interactive():
		return nil, nil, errors.New("which tool? e.g. aims failover claude")
	}
	r := ui.NewRail()
	opts := make([]ui.Option, len(candidates))
	for i, a := range candidates {
		opts[i] = ui.Option{Label: a.Title(), Value: string(a.ID()), Hint: "active: " + cfg.Tools[a.ID()].Active}
	}
	pick := opts[0].Value
	if err := r.Select("Which tool hit its limit?", "", opts, &pick); err != nil {
		return nil, r, cancelled(r, err)
	}
	return tools.Get(tool.ID(pick)), r, nil
}

// askFailover asks which account to mark, starting on the one used last,
// and why, unless reason is given.
func askFailover(ctx context.Context, r *ui.Rail, a tool.Adapter, reason string) (from, why string, err error) {
	cfg, err := config.Load()
	if err != nil {
		return "", "", err
	}
	var report []ops.ToolStatus
	ui.Spin("checking your accounts", func() { report, err = ops.Status(ctx, []tool.ID{a.ID()}) })
	if err != nil {
		return "", "", err
	}
	last := ops.LastUsed(cfg, config.LoadState(), a.ID())
	desc := "aims started " + last + " last."
	if last == "" {
		last = cfg.Tools[a.ID()].Active
		desc = ""
	}
	x, err := pickAccount(r, "Which "+a.Title()+" account hit its limit?", desc, accountsOf(report, a, nil), false, func(x account) bool { return x.p.Name == last })
	if err != nil {
		return "", "", err
	}
	if reason == "" {
		reason = "limit"
		opts := []ui.Option{
			{Label: "It hit its usage limit", Value: "limit", Hint: "it rests until the limit resets"},
			{Label: "Its login stopped working", Value: "login", Hint: "it rests until you log it in again"},
		}
		if err := r.Select("What happened to "+x.p.Name+"?", "", opts, &reason); err != nil {
			return "", "", err
		}
	}
	return x.p.Name, reason, nil
}

// failoverText says what a failover did.
func failoverText(s *ui.Styles, a tool.Adapter, res ops.FailoverResult) string {
	to := s.Bold.Render(res.To)
	switch {
	case res.Login && res.Switched:
		return fmt.Sprintf("%s now uses %s; %s is marked as logged out", a.Title(), to, res.From)
	case res.Login:
		return fmt.Sprintf("%s is marked as logged out; new %s sessions use %s until it is logged in again", res.From, a.Title(), to)
	case res.Switched:
		return fmt.Sprintf("%s now uses %s; %s rests (%s)", a.Title(), to, res.From, ui.Until(res.Until))
	}
	return fmt.Sprintf("%s rests (%s); new %s sessions use %s until then", res.From, ui.Until(res.Until), a.Title(), to)
}

// failoverHints are the follow-ups worth knowing after a failover.
func failoverHints(a tool.Adapter, res ops.FailoverResult) []string {
	var out []string
	if res.Login {
		out = append(out, fmt.Sprintf("log it in again: aims login %s %s", a.ID(), res.From))
	}
	if res.FromLastUsed {
		out = append(out, fmt.Sprintf("%s was marked because it was used last; another one? aims clear %s %s && aims failover %s --from <name>", res.From, a.ID(), res.From, a.ID()))
	}
	if res.Pinned != "" && res.Pinned != res.To {
		out = append(out, fmt.Sprintf("this terminal is pinned to %s; aims skips it while it rests", res.Pinned))
	}
	return out
}

func newClearCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clear [tool] [profile]",
		Short: "Forget cooldown and login marks",
		Long: "Without arguments, aims lists the marked accounts and clears the ones you\n" +
			"pick; in a script it clears every mark.",
		Example: `aims clear
aims clear claude
aims clear claude work`,
		Args:              cobra.MaximumNArgs(2),
		ValidArgsFunction: completeToolProfile,
		RunE: func(cmd *cobra.Command, args []string) error {
			only, name, err := parseTarget(args)
			if err != nil {
				return err
			}
			targets := []tool.Adapter{only}
			switch {
			case only != nil:
			case name != "":
				cfg, err := config.Load()
				if err != nil {
					return err
				}
				if targets = toolsWith(cfg, name); len(targets) == 0 {
					return fmt.Errorf("no profile named %q (see: aims status)", name)
				}
			case ui.Interactive():
				return askClear(cmd.Context())
			default:
				cfg, err := config.Load()
				if err != nil {
					return err
				}
				targets = nil
				for _, a := range tools.All() {
					if len(cfg.Tools[a.ID()].Profiles) > 0 {
						targets = append(targets, a)
					}
				}
			}
			for _, a := range targets {
				names, err := ops.ClearMarks(a, name)
				if err != nil {
					return err
				}
				ui.Done("cleared %s: %s", a.ID(), strings.Join(names, ", "))
			}
			return nil
		},
	}
}

// askClear lists the marked accounts, all picked, and clears the ones kept picked.
func askClear(ctx context.Context) error {
	report, err := loadAccounts(ctx)
	if err != nil {
		return err
	}
	marked := accountsOf(report, nil, func(x account) bool {
		return x.p.MarkedBy != "" || !x.p.Until.IsZero() || (needsLogin(x.p) && (x.p.LoggedIn == nil || *x.p.LoggedIn))
	})
	if len(marked) == 0 {
		ui.Done("no account is marked; nothing to clear")
		return nil
	}
	r := ui.NewRail()
	opts := accountOptions(marked, toolsInUse(report) > 1)
	picked := make([]string, len(opts))
	for i, o := range opts {
		picked[i] = o.Value
	}
	if err := r.MultiSelect("Clear which marks?", "A marked account is skipped until its mark lifts. Space toggles, Enter confirms.", opts, &picked); err != nil {
		return cancelled(r, err)
	}
	if len(picked) == 0 {
		r.Outro(ui.Out.Dim.Render("Nothing changed."))
		return nil
	}
	var done []string
	for _, v := range picked {
		x := marked[optionIndex(v)]
		if _, err := ops.ClearMarks(x.adapter(), x.p.Name); err != nil {
			r.Fail(err.Error())
			return err
		}
		done = append(done, string(x.t.ID)+"/"+x.p.Name)
	}
	r.Outro(ui.Out.OK.Render("Cleared ") + strings.Join(done, ", "))
	return nil
}

func newRemoveCmd() *cobra.Command {
	var purge, force, yes bool
	cmd := &cobra.Command{
		Use:     "rm [tool] [profile]",
		Aliases: []string{"remove"},
		Short:   "Remove a profile",
		Long: "Forgets the profile. With --purge it is logged out and its folder deleted,\n" +
			"after anything shared that was written there moved to the shared folder.\n" +
			"Shared history and settings are never deleted.\n\n" +
			"Without a profile, aims asks which account and what to do with it.",
		Example: `aims rm
aims rm codex client-a --purge`,
		Args:              cobra.MaximumNArgs(2),
		ValidArgsFunction: completeToolProfile,
		RunE: func(cmd *cobra.Command, args []string) error {
			t, r, err := resolve(cmd.Context(), args, "Remove which account?", "aims rm claude work", nil)
			if errors.Is(err, errNoAccount) {
				return errors.New("no account to remove")
			}
			if err != nil {
				return err
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			dir := fsx.Tildify(cfg.Dir(t.a.ID(), t.name))
			if r != nil && !purge {
				p := cfg.Tools[t.a.ID()].Profiles[t.name]
				how := "keep"
				opts := []ui.Option{
					{Label: "Keep it", Value: "keep", Hint: "change nothing"},
					{Label: "Forget it", Value: "forget", Hint: "aims stops using it; the login stays in " + dir},
				}
				if !p.Existing && p.Dir == "" {
					opts = append(opts, ui.Option{Label: "Log it out and delete its folder", Value: "purge", Hint: dir})
				}
				if err := r.Select("What should happen to "+t.String()+"?", "Shared history and settings are kept either way.", opts, &how); err != nil {
					return cancelled(r, err)
				}
				switch how {
				case "keep":
					r.Outro(ui.Out.Dim.Render("Nothing changed."))
					return nil
				case "purge":
					purge, yes = true, true
				}
				r.Outro(ui.Out.Dim.Render("removing " + t.String()))
			}
			if purge && !yes {
				if !ui.Interactive() {
					return errors.New("--purge deletes the profile folder; add --yes to confirm")
				}
				ok := false
				q := fmt.Sprintf("Delete %s and the login in it?", dir)
				if err := ui.NewRail().Confirm(q, "Shared history and settings are kept.", &ok); err != nil {
					return err
				}
				if !ok {
					return nil
				}
			}
			res, err := ops.RemoveProfile(cmd.Context(), t.a, t.name, purge, force)
			reportLinks(t.a.ID(), t.name, res.Reports)
			if res.LogoutErr != nil {
				ui.Warn("could not log %s out first (%v); its login may stay in the system keychain", t, res.LogoutErr)
			}
			if err != nil {
				return err
			}
			msg := "removed " + string(t.a.ID()) + " profile " + t.name
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
