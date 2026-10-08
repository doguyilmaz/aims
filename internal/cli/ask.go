package cli

// Commands take "[tool] [profile]" and ask for what is missing. Questions
// appear only on a terminal; a script gets an error with an example instead.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/ops"
	"github.com/doguyilmaz/aims/internal/profiles"
	"github.com/doguyilmaz/aims/internal/tool"
	"github.com/doguyilmaz/aims/internal/tools"
	"github.com/doguyilmaz/aims/internal/ui"
)

// account is one profile of one tool, as a picker shows it.
type account struct {
	t ops.ToolStatus
	p ops.ProfileStatus
}

func (x account) adapter() tool.Adapter { return tools.Get(x.t.ID) }

// target is the account a command acts on.
type target struct {
	a    tool.Adapter
	name string
}

func (t target) String() string { return string(t.a.ID()) + "/" + t.name }

// parseTarget reads "[tool] [profile]". A lone word that is not a tool is a
// profile name.
func parseTarget(args []string) (only tool.Adapter, name string, err error) {
	switch len(args) {
	case 0:
		return nil, "", nil
	case 1:
		if a, ok := tools.Parse(args[0]); ok {
			return a, "", nil
		}
		return nil, args[0], nil
	case 2:
		a, err := parseTool(args[0])
		return a, args[1], err
	}
	return nil, "", fmt.Errorf("too many arguments: expected [tool] [profile], got %q", strings.Join(args, " "))
}

// resolve turns "[tool] [profile]" into one account. A name alone means the
// tool that has it. Anything missing or ambiguous is asked for on a terminal,
// from the accounts keep accepts (nil: all); r is the rail the question went
// on, or nil when nothing was asked.
func resolve(ctx context.Context, args []string, question, example string, keep func(account) bool) (t target, r *ui.Rail, err error) {
	only, name, err := parseTarget(args)
	if err != nil {
		return t, nil, err
	}
	cfg, err := config.Load()
	if err != nil {
		return t, nil, err
	}
	if only != nil && name != "" {
		if _, err := cfg.Profile(only.ID(), name); err != nil {
			return t, nil, err
		}
		return target{only, name}, nil, nil
	}
	if name != "" {
		has := toolsWith(cfg, name)
		switch {
		case len(has) == 0:
			return t, nil, fmt.Errorf("no profile named %q (see: aims status)", name)
		case len(has) == 1:
			return target{has[0], name}, nil, nil
		case !ui.Interactive():
			return t, nil, fmt.Errorf("both %s have a profile %q; name the tool, e.g. %s", toolIDs(has), name, example)
		}
		keep = func(x account) bool { return x.p.Name == name }
	} else if !ui.Interactive() {
		return t, nil, fmt.Errorf("which account? e.g. %s", example)
	}
	report, err := loadAccounts(ctx)
	if err != nil {
		return t, nil, err
	}
	list := accountsOf(report, only, keep)
	if len(list) == 0 {
		return t, nil, errNoAccount
	}
	r = ui.NewRail()
	x, err := pickAccount(r, question, "", list, only == nil && toolsInUse(report) > 1, nil)
	if err != nil {
		return t, r, cancelled(r, err)
	}
	return target{x.adapter(), x.p.Name}, r, nil
}

// errNoAccount: no account fits the question; callers say why.
var errNoAccount = errors.New("no account to choose from")

// toolsWith lists the tools that have a profile called name.
func toolsWith(cfg *config.Config, name string) []tool.Adapter {
	var out []tool.Adapter
	for _, a := range tools.All() {
		if cfg.Tools[a.ID()].Profiles[name] != nil {
			out = append(out, a)
		}
	}
	return out
}

func toolIDs(as []tool.Adapter) string {
	ids := make([]string, len(as))
	for i, a := range as {
		ids[i] = string(a.ID())
	}
	return strings.Join(ids, " and ")
}

// toolsInUse counts the tools that have accounts.
func toolsInUse(report []ops.ToolStatus) int {
	n := 0
	for _, t := range report {
		if len(t.Profiles) > 0 {
			n++
		}
	}
	return n
}

// loadAccounts reads every tool's accounts behind a spinner.
func loadAccounts(ctx context.Context) ([]ops.ToolStatus, error) {
	var report []ops.ToolStatus
	var err error
	ui.Spin("checking your accounts", func() { report, err = ops.Status(ctx, nil) })
	return report, err
}

// accountsOf lists the accounts of one tool (every tool when only is nil)
// that keep accepts (all when keep is nil), in status order.
func accountsOf(report []ops.ToolStatus, only tool.Adapter, keep func(account) bool) []account {
	var out []account
	for _, t := range report {
		if only != nil && t.ID != only.ID() {
			continue
		}
		for _, p := range t.Profiles {
			if x := (account{t, p}); keep == nil || keep(x) {
				out = append(out, x)
			}
		}
	}
	return out
}

// accountOptions builds picker options. With accounts of several tools, or
// withTool, the tool leads each label; the hint says who it is and how it is
// doing.
func accountOptions(list []account, withTool bool) []ui.Option {
	multi, width := withTool, 0
	for _, x := range list {
		multi = multi || x.t.ID != list[0].t.ID
		width = max(width, len(x.t.Title))
	}
	opts := make([]ui.Option, len(list))
	for i, x := range list {
		label, short := x.p.Name, x.p.Name
		if multi {
			label, short = ui.Pad(x.t.Title, width)+"  "+x.p.Name, string(x.t.ID)+"/"+x.p.Name
		}
		opts[i] = ui.Option{Label: label, Value: strconv.Itoa(i), Hint: accountHint(x.p), Short: short}
	}
	return opts
}

// accountHint is who an account is and how it is doing:
// "me@gmail.com · limit for 2h10m, marked by a failed run · active".
func accountHint(p ops.ProfileStatus) string {
	var parts []string
	if p.Email != "" {
		parts = append(parts, p.Email)
	}
	state := "ready"
	switch {
	case p.LoggedIn != nil && !*p.LoggedIn:
		state = "not logged in"
	case !p.Usable:
		state = strings.Join(p.Reasons, ", ")
	}
	if p.MarkedBy != "" {
		state += ", marked by " + p.MarkedBy
	}
	parts = append(parts, state)
	if p.Active {
		parts = append(parts, "active")
	}
	return strings.Join(parts, " · ")
}

// pickAccount asks for one of list, starting on the first that start accepts.
func pickAccount(r *ui.Rail, question, description string, list []account, withTool bool, start func(account) bool) (account, error) {
	opts := accountOptions(list, withTool)
	pick := opts[0].Value
	if start != nil {
		if i := slices.IndexFunc(list, start); i >= 0 {
			pick = opts[i].Value
		}
	}
	if err := r.Select(question, description, opts, &pick); err != nil {
		return account{}, err
	}
	return list[optionIndex(pick)], nil
}

// optionIndex reads back the value accountOptions gave an option.
func optionIndex(value string) int {
	i, _ := strconv.Atoi(value)
	return i
}

// needsLogin reports whether an account cannot be used until it logs in.
func needsLogin(p ops.ProfileStatus) bool {
	return (p.LoggedIn != nil && !*p.LoggedIn) || slices.ContainsFunc(p.Reasons, func(r string) bool { return strings.HasPrefix(r, "login") })
}

// installedTools lists the tools on PATH, or only when it is set.
func installedTools(only tool.Adapter) []tool.Adapter {
	if only != nil {
		return []tool.Adapter{only}
	}
	var out []tool.Adapter
	for _, a := range tools.All() {
		if profiles.Bin(a) != "" {
			out = append(out, a)
		}
	}
	return out
}

// addAccount names a new account, sets what it shares, creates it in the
// chosen tools and offers to log it in. name may be given already.
func addAccount(ctx context.Context, r *ui.Rail, adapters []tool.Adapter, name string) error {
	if len(adapters) == 0 {
		return errNoTools
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if len(adapters) > 1 {
		pick := "_both"
		opts := []ui.Option{{Label: "Both", Value: "_both"}}
		for _, a := range adapters {
			opts = append(opts, ui.Option{Label: a.Title(), Value: string(a.ID())})
		}
		if err := r.Select("For which tool?", "", opts, &pick); err != nil {
			return cancelled(r, err)
		}
		if pick != "_both" {
			adapters = slices.DeleteFunc(slices.Clone(adapters), func(a tool.Adapter) bool { return string(a.ID()) != pick })
		}
	}
	var taken []string
	for _, a := range adapters {
		taken = append(taken, cfg.Tools[a.ID()].Order...)
	}
	if name == "" || nameRule(taken...)(name) != nil {
		if err := r.Input("Name the new account", "", "e.g. client-a", &name, nameRule(taken...)); err != nil {
			return cancelled(r, err)
		}
		name = strings.TrimSpace(name)
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
	for _, a := range adapters {
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
	for _, a := range adapters {
		if err := loginStep(ctx, r, a, name); err != nil {
			return cancelled(r, err)
		}
	}
	r.Outro(ui.Out.OK.Render("Added ") + name + ui.Out.Dim.Render(". Make it first with ") + ui.Kbd(ui.Out, "aims use "+name))
	return nil
}
