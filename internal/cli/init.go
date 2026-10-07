package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/ops"
	"github.com/doguyilmaz/aims/internal/proc"
	"github.com/doguyilmaz/aims/internal/profiles"
	"github.com/doguyilmaz/aims/internal/shell"
	"github.com/doguyilmaz/aims/internal/tool"
	"github.com/doguyilmaz/aims/internal/tools"
	"github.com/doguyilmaz/aims/internal/ui"
)

type initOptions struct {
	version      string
	yes          bool
	tools        string
	current      string
	second       string
	share        string
	integrations string
}

func newInitCmd(version string) *cobra.Command {
	o := initOptions{version: version}
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Set up your accounts step by step",
		Long: "Finds Claude Code and Codex and the logins you already have, names your\n" +
			"accounts, decides what they share, connects the tools to aims and walks you\n" +
			"through the remaining logins. Run it again to add an account.",
		Example: `aims init
aims init --yes --current personal --second work --share settings`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return runInit(cmd.Context(), o) },
	}
	f := cmd.Flags()
	f.BoolVarP(&o.yes, "yes", "y", false, "no questions: use the flags and defaults (logins are left for later)")
	f.StringVar(&o.tools, "tools", "", "comma-separated tools (default: every installed one)")
	f.StringVar(&o.current, "current", "personal", "name for the login you already have")
	f.StringVar(&o.second, "second", "work", "name for your other account")
	f.StringVar(&o.share, "share", config.ShareAll, "what accounts share: all, settings or none")
	f.StringVar(&o.integrations, "integrations", "all", "skill, mcp, statusline, shell (comma-separated), all or none")
	return cmd
}

type found struct {
	a       tool.Adapter
	bin     string
	version string
	acct    tool.Account
}

func (f found) loggedIn() bool { return f.acct.LoggedIn != nil && *f.acct.LoggedIn }

func (f found) who() string {
	switch {
	case f.acct.Email != "":
		return f.acct.Email
	case f.loggedIn():
		return "an account without an e-mail (API key)"
	}
	return ""
}

func detect(ctx context.Context, cfg *config.Config, only []tool.ID) []found {
	var list []tool.Adapter
	for _, a := range tools.All() {
		if len(only) == 0 || slices.Contains(only, a.ID()) {
			list = append(list, a)
		}
	}
	out := make([]found, len(list))
	var wg sync.WaitGroup
	for i, a := range list {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f := found{a: a, bin: profiles.Bin(a)}
			if f.bin != "" {
				f.version = toolVersion(f.bin)
				f.acct = a.Account(ctx, profiles.HubHome(cfg, a))
			}
			out[i] = f
		}()
	}
	wg.Wait()
	return out
}

func installed(fs []found) []found {
	var out []found
	for _, f := range fs {
		if f.bin != "" {
			out = append(out, f)
		}
	}
	return out
}

func runInit(ctx context.Context, o initOptions) error {
	only, err := parseTools(o.tools)
	if err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if !ui.Interactive() && !o.yes {
		return errors.New("aims init asks questions; on a script use: aims init --yes [--current personal --second work --share all]")
	}
	if !config.ValidName(o.current) || !config.ValidName(o.second) || o.current == o.second {
		return errors.New("--current and --second must be two different valid names")
	}
	if !config.ValidShare(o.share) {
		return fmt.Errorf("--share must be %s, %s or %s", config.ShareAll, config.ShareSettings, config.ShareNone)
	}

	var all []found
	if o.yes {
		all = detect(ctx, cfg, only)
	} else {
		ui.Spin("looking for Claude Code and Codex", func() { all = detect(ctx, cfg, only) })
	}
	inst := installed(all)
	if len(inst) == 0 {
		return errNoTools
	}
	if o.yes {
		return initQuiet(ctx, cfg, inst, o)
	}

	r := ui.NewRail()
	r.Intro("aims", "accounts for Claude Code and Codex")
	var lines []string
	width := 0
	for _, f := range all {
		width = max(width, len(f.a.Title()+" "+f.version))
	}
	for _, f := range all {
		name := ui.Pad(f.a.Title()+" "+ui.Out.Dim.Render(f.version), width+1)
		switch {
		case f.bin == "":
			lines = append(lines, name+"   "+ui.Out.Dim.Render("not installed, skipped"))
		case f.who() != "":
			who := f.who()
			if f.acct.Plan != "" {
				who += ui.Out.Dim.Render(" · " + f.acct.Plan)
			}
			lines = append(lines, name+"   "+who)
		default:
			lines = append(lines, name+"   "+ui.Out.Dim.Render("not logged in"))
		}
	}
	r.Section("Found", lines...)

	if cfg.HasProfiles() {
		return initAgain(ctx, r, cfg, inst)
	}
	return initFresh(ctx, r, inst, o)
}

func complement(name string) string {
	switch name {
	case "personal":
		return "work"
	case "work":
		return "personal"
	}
	return "work"
}

func nameRule(taken ...string) func(string) error {
	return func(s string) error {
		s = strings.TrimSpace(s)
		switch {
		case !config.ValidName(s):
			return errors.New("letters, digits, '.', '_' and '-' only")
		case slices.Contains(taken, s):
			return fmt.Errorf("%q is already taken", s)
		}
		return nil
	}
}

// askWho asks which of the user's accounts a login belongs to.
func askWho(r *ui.Rail, question string, value *string) error {
	choice := "personal"
	opts := []ui.Option{
		{Label: "personal", Value: "personal"},
		{Label: "work", Value: "work"},
		{Label: "another name", Value: "_other"},
	}
	if err := r.Select(question, "", opts, &choice); err != nil {
		return err
	}
	if choice != "_other" {
		*value = choice
		return nil
	}
	*value = ""
	return r.Input("Name this account", "", "e.g. client-a", value, nameRule())
}

// plan is one tool's part of the setup.
type plan struct {
	f        found
	adopt    string   // profile that adopts the existing login
	newNames []string // profiles that need a login
}

func initFresh(ctx context.Context, r *ui.Rail, inst []found, o initOptions) error {
	var plans []plan
	var logged []found
	emails := map[string]bool{}
	for _, f := range inst {
		if f.loggedIn() {
			logged = append(logged, f)
			emails[strings.ToLower(f.acct.Email)] = true
		}
	}

	var current, second string
	switch {
	case len(logged) == 0:
		current = "personal"
		if err := r.Input("Name your first account", "Neither tool is logged in yet; you will log in to each account below.", "personal", &current, nameRule()); err != nil {
			return cancelled(r, err)
		}
	case len(emails) == 1:
		q := "Who is " + logged[0].who() + "?"
		if len(logged) > 1 {
			q = logged[0].who() + " is logged in to both tools. Which account is it?"
		}
		if err := askWho(r, q, &current); err != nil {
			return cancelled(r, err)
		}
	}

	perTool := map[tool.ID]string{}
	if len(emails) > 1 {
		for _, f := range logged {
			var n string
			if err := askWho(r, fmt.Sprintf("Who is %s on %s?", f.who(), f.a.Title()), &n); err != nil {
				return cancelled(r, err)
			}
			perTool[f.a.ID()] = n
		}
		current = perTool[logged[0].a.ID()]
	}

	second = complement(current)
	if err := r.Input("Name your other account", "", second, &second, nameRule(current)); err != nil {
		return cancelled(r, err)
	}

	for _, f := range inst {
		p := plan{f: f}
		first := current
		if n, ok := perTool[f.a.ID()]; ok {
			first = n
		}
		other := second
		if other == first {
			other = current
		}
		if f.loggedIn() {
			p.adopt = first
		} else {
			p.newNames = append(p.newNames, first)
		}
		p.newNames = append(p.newNames, other)
		plans = append(plans, p)
	}

	share := o.share
	shareOpts := []ui.Option{
		{Label: "Everything", Value: config.ShareAll, Hint: "history, settings, skills and MCP servers; continue a conversation on either account"},
		{Label: "Settings only", Value: config.ShareSettings, Hint: "each account keeps its own conversations"},
		{Label: "Nothing", Value: config.ShareNone, Hint: "fully separate"},
	}
	if err := r.Select(fmt.Sprintf("What should %s and %s share?", current, second), "", shareOpts, &share); err != nil {
		return cancelled(r, err)
	}

	in, err := askIntegrations(r, inst)
	if err != nil {
		return cancelled(r, err)
	}

	for _, p := range plans {
		a := p.f.a
		if p.adopt != "" {
			_ = r.Task(fmt.Sprintf("%s/%s", a.ID(), p.adopt), func() (string, error) {
				_, err := ops.AddProfile(a, p.adopt, ops.AddOptions{Existing: true})
				if err == nil {
					_, _ = ops.RecordLogin(ctx, a, p.adopt)
				}
				return "your current login, " + p.f.who(), err
			})
		}
		for _, n := range p.newNames {
			_ = r.Task(fmt.Sprintf("%s/%s", a.ID(), n), func() (string, error) {
				res, err := ops.AddProfile(a, n, ops.AddOptions{Share: share})
				return fsx.Tildify(res.Dir), err
			})
		}
	}
	applyIntegrations(ctx, r, in)
	r.Gap()

	for _, p := range plans {
		for _, n := range p.newNames {
			if err := loginStep(ctx, r, p.f.a, n); err != nil {
				return cancelled(r, err)
			}
		}
	}
	outro(r, in, current, second)
	return nil
}

func askIntegrations(r *ui.Rail, inst []found) (ops.Integrations, error) {
	sh := shell.Detect()
	var titles, bins []string
	for _, f := range inst {
		titles = append(titles, f.a.Title())
		bins = append(bins, "`"+f.a.Layout().Bin+"`")
	}
	opts := []ui.Option{{Label: "Let " + strings.Join(titles, " and ") + " switch accounts themselves", Value: "mcp", Hint: "MCP server and skill", Short: "MCP server and skill"}}
	for _, f := range inst {
		if _, ok := f.a.(tool.StatusLiner); ok {
			opts = append(opts, ui.Option{Label: "Show the account and plan usage in " + f.a.Title(), Value: "statusline", Hint: "status line", Short: "status line"})
			break
		}
	}
	rc := shell.RCFile(sh)
	if rc != "" {
		opts = append(opts, ui.Option{Label: "Make plain " + strings.Join(bins, " and ") + " follow the active account", Value: "shell", Hint: fsx.Tildify(rc), Short: "shell integration"})
	}
	picked := make([]string, len(opts))
	for i, o := range opts {
		picked[i] = o.Value
	}
	if err := r.MultiSelect("Connect aims to your tools", "Space toggles, Enter confirms.", opts, &picked); err != nil {
		return ops.Integrations{}, err
	}
	in := ops.Integrations{Skill: slices.Contains(picked, "mcp"), MCP: slices.Contains(picked, "mcp"), StatusLine: slices.Contains(picked, "statusline")}
	if slices.Contains(picked, "shell") {
		in.Shell = sh
	}
	return in, nil
}

func applyIntegrations(ctx context.Context, r *ui.Rail, in ops.Integrations) {
	if !in.Skill && !in.MCP && !in.StatusLine && in.Shell == "" {
		return
	}
	var steps []ops.Step
	_ = r.Task("connecting your tools", func() (string, error) {
		var err error
		steps, err = ops.Setup(ctx, nil, in)
		return "", err
	})
	done := map[string]int{}
	var order []string
	for _, s := range steps {
		if s.Err != nil {
			r.Fail(fmt.Sprintf("%s %s: %v", s.What, ui.Out.Dim.Render(s.Target), s.Err))
			continue
		}
		if done[s.What] == 0 {
			order = append(order, s.What)
		}
		done[s.What]++
	}
	for _, w := range order {
		if n := done[w]; n > 1 {
			r.OK(fmt.Sprintf("%s %s", w, ui.Out.Dim.Render(fmt.Sprintf("in %d places", n))))
		} else {
			r.OK(w)
		}
	}
}

// loginStep offers to log a profile in now, handing the terminal to the
// tool's own login, and checks the result.
func loginStep(ctx context.Context, r *ui.Rail, a tool.Adapter, name string) error {
	for attempt := 0; attempt < 3; attempt++ {
		now := true
		q := fmt.Sprintf("Log in to %s as %s now?", a.Title(), name)
		if attempt > 0 {
			q = "Try again with the other account?"
		}
		if err := r.Confirm(q, a.Commands().LoginTip, &now); err != nil {
			return err
		}
		if !now {
			r.Hint("later: " + ui.Kbd(ui.Out, "aims login "+string(a.ID())+" "+name))
			r.Gap()
			return nil
		}
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		env, _ := profiles.Env(cfg, a, name, os.Environ())
		code, err := proc.RunAttached(profiles.Bin(a), a.Commands().Login, env, "")
		if err != nil || code != 0 {
			r.Fail(fmt.Sprintf("%s/%s: the login did not finish", a.ID(), name))
			continue
		}
		res, err := ops.RecordLogin(ctx, a, name)
		if err != nil {
			return err
		}
		if len(res.SameAs) > 0 {
			r.Fail(fmt.Sprintf("%s/%s is %s, the same account as %s", a.ID(), name, res.Email, strings.Join(res.SameAs, ", ")))
			r.Hint("open the login link in a private window and sign in with the other account")
			continue
		}
		who := res.Email
		if who == "" {
			who = "logged in"
		}
		r.OK(fmt.Sprintf("%s/%s  %s", a.ID(), name, ui.Out.Dim.Render(who)))
		r.Gap()
		return nil
	}
	r.Hint("later: " + ui.Kbd(ui.Out, "aims login "+string(a.ID())+" "+name))
	r.Gap()
	return nil
}

func outro(r *ui.Rail, in ops.Integrations, first, second string) {
	s := ui.Out
	lines := []string{
		ui.Pad(ui.Kbd(s, "aims"), 22) + s.Dim.Render("your accounts, usage and quick actions"),
		ui.Pad(ui.Kbd(s, "aims use "+second), 22) + s.Dim.Render("new sessions use "+second),
	}
	if in.Shell != "" {
		lines = append(lines, ui.Pad(ui.Kbd(s, "claude"), 22)+s.Dim.Render("runs as the active account (in a new terminal)"))
	} else {
		lines = append(lines, ui.Pad(ui.Kbd(s, "aims claude"), 22)+s.Dim.Render("runs Claude Code as the active account"))
	}
	lines = append(lines, ui.Pad(ui.Kbd(s, "aims failover claude"), 22)+s.Dim.Render("hit a limit? continue on "+first+" or "+second))
	r.Box(lines...)
	r.Outro(s.OK.Render("Ready."))
}

func cancelled(r *ui.Rail, err error) error {
	if errors.Is(err, ui.ErrCancelled) {
		r.Cancel()
	}
	return err
}

// initAgain is `aims init` when profiles exist already.
func initAgain(ctx context.Context, r *ui.Rail, cfg *config.Config, inst []found) error {
	var have []string
	for _, f := range inst {
		if names := cfg.Tools[f.a.ID()].Order; len(names) > 0 {
			have = append(have, f.a.Title()+": "+strings.Join(names, ", "))
		}
	}
	r.Section("aims manages", have...)
	choice := "add"
	opts := []ui.Option{
		{Label: "Add an account", Value: "add"},
		{Label: "Connect the tools again", Value: "connect", Hint: "skill, MCP server, status line, shell"},
		{Label: "Nothing, show my accounts", Value: "status"},
	}
	if err := r.Select("What would you like to do?", "", opts, &choice); err != nil {
		return cancelled(r, err)
	}
	switch choice {
	case "connect":
		in, err := askIntegrations(r, inst)
		if err != nil {
			return cancelled(r, err)
		}
		applyIntegrations(ctx, r, in)
		r.Outro(ui.Out.OK.Render("Done."))
		return nil
	case "status":
		r.Outro("")
		report, err := ops.Status(ctx, nil)
		if err != nil {
			return err
		}
		fmt.Print(ui.RenderStatus(ui.Out, report, cfg.Failover.Threshold))
		return nil
	}

	target := inst[0]
	if len(inst) > 1 {
		pick := "_both"
		opts := []ui.Option{{Label: "Both", Value: "_both"}}
		for _, f := range inst {
			opts = append(opts, ui.Option{Label: f.a.Title(), Value: string(f.a.ID())})
		}
		if err := r.Select("For which tool?", "", opts, &pick); err != nil {
			return cancelled(r, err)
		}
		if pick != "_both" {
			for _, f := range inst {
				if string(f.a.ID()) == pick {
					inst = []found{f}
				}
			}
		}
		target = inst[0]
	}
	var taken []string
	for _, f := range inst {
		taken = append(taken, cfg.Tools[f.a.ID()].Order...)
	}
	name := ""
	if err := r.Input("Name the new account", "", "e.g. client-a", &name, nameRule(taken...)); err != nil {
		return cancelled(r, err)
	}
	share := config.ShareAll
	shareOpts := []ui.Option{
		{Label: "Everything", Value: config.ShareAll, Hint: "history, settings, skills and MCP servers"},
		{Label: "Settings only", Value: config.ShareSettings, Hint: "keeps its own conversations"},
		{Label: "Nothing", Value: config.ShareNone, Hint: "fully separate"},
	}
	if err := r.Select("What should "+name+" share with the others?", "", shareOpts, &share); err != nil {
		return cancelled(r, err)
	}
	_ = target
	for _, f := range inst {
		a := f.a
		if err := r.Task(fmt.Sprintf("%s/%s", a.ID(), name), func() (string, error) {
			res, err := ops.AddProfile(a, name, ops.AddOptions{Share: share})
			return fsx.Tildify(res.Dir), err
		}); err != nil {
			return err
		}
	}
	// Tools that keep MCP servers per login need aims registered in the new one too.
	applyIntegrations(ctx, r, ops.Integrations{MCP: true})
	r.Gap()
	for _, f := range inst {
		if err := loginStep(ctx, r, f.a, name); err != nil {
			return cancelled(r, err)
		}
	}
	r.Outro(ui.Out.OK.Render("Added ") + name + ui.Out.Dim.Render(". Switch to it with ") + ui.Kbd(ui.Out, "aims use "+name))
	return nil
}

// initQuiet is `aims init --yes`. Tools that already have profiles are left
// alone, so it is safe to run again (from a dotfiles script, say).
func initQuiet(ctx context.Context, cfg *config.Config, inst []found, o initOptions) error {
	in, err := parseIntegrations(o.integrations)
	if err != nil {
		return err
	}
	var hints []string
	for _, f := range inst {
		a := f.a
		if len(cfg.Tools[a.ID()].Profiles) > 0 {
			ui.Info("%s already has profiles: %s", a.ID(), strings.Join(cfg.Tools[a.ID()].Order, ", "))
			continue
		}
		if f.loggedIn() {
			if _, err := ops.AddProfile(a, o.current, ops.AddOptions{Existing: true}); err != nil {
				return err
			}
			_, _ = ops.RecordLogin(ctx, a, o.current)
			ui.Done("%s/%s uses your current login (%s)", a.ID(), o.current, f.who())
		} else {
			res, err := ops.AddProfile(a, o.current, ops.AddOptions{Share: o.share})
			if err != nil {
				return err
			}
			ui.Done("%s/%s %s", a.ID(), o.current, fsx.Tildify(res.Dir))
			hints = append(hints, fmt.Sprintf("aims login %s %s", a.ID(), o.current))
		}
		res, err := ops.AddProfile(a, o.second, ops.AddOptions{Share: o.share})
		if err != nil {
			return err
		}
		ui.Done("%s/%s %s", a.ID(), o.second, fsx.Tildify(res.Dir))
		hints = append(hints, fmt.Sprintf("aims login %s %s", a.ID(), o.second))
	}
	steps, err := ops.Setup(ctx, nil, in)
	if err != nil {
		return err
	}
	if err := reportSteps(steps, ""); err != nil {
		return err
	}
	for _, h := range hints {
		ui.Hint("log in: %s", h)
	}
	return nil
}

func parseIntegrations(list string) (ops.Integrations, error) {
	var in ops.Integrations
	for _, part := range strings.Split(list, ",") {
		switch strings.TrimSpace(part) {
		case "all":
			in = ops.Integrations{Skill: true, MCP: true, StatusLine: true, Shell: shell.Detect()}
		case "skill":
			in.Skill = true
		case "mcp":
			in.MCP = true
		case "statusline":
			in.StatusLine = true
		case "shell":
			in.Shell = shell.Detect()
		case "none", "":
		default:
			return in, fmt.Errorf("unknown integration %q (skill, mcp, statusline, shell, all or none)", part)
		}
	}
	return in, nil
}
