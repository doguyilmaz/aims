package cli

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/ops"
	"github.com/doguyilmaz/aims/internal/tool"
	"github.com/doguyilmaz/aims/internal/tools"
	"github.com/doguyilmaz/aims/internal/ui"
)

// newOrderCmd is `aims order`: which account comes first and which stand in
// for it. Every argument is optional; on a terminal aims asks for the rest.
func newOrderCmd() *cobra.Command {
	var global, reset bool
	cmd := &cobra.Command{
		Use:   "order [tool] [profile...]",
		Short: "Set which account comes first and which stand in for it",
		Long: "The first profile becomes the active one: new sessions use it. While it is\n" +
			"limited or logged out they use the next usable one in this order, and come\n" +
			"back to the first once it works again. When every account is limited, they\n" +
			"wait on the one that resets first.\n\n" +
			"It is the order of the project aims runs in (the git repository, or the folder\n" +
			"outside one), unless --global. --reset drops the project's own order.\n\n" +
			"Without a tool, every tool that has these profiles changes. Without profiles,\n" +
			"aims asks for the order, shows it and saves it once you confirm.",
		Example: `aims order
aims order personal work            # Claude Code and Codex, in this project
aims order claude personal work
aims order --global personal work   # every folder without an order of its own
aims order --reset                  # this project follows that order again`,
		Args: cobra.ArbitraryArgs,
		ValidArgsFunction: func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
			var out []string
			toolName := ""
			if len(args) == 0 {
				out = tools.Names()
			} else if _, ok := tools.Parse(args[0]); ok {
				toolName = args[0]
			}
			for _, n := range profileNames(toolName) {
				if !slices.Contains(args, n) {
					out = append(out, n)
				}
			}
			return out, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			var only tool.Adapter
			if len(args) > 0 {
				if a, ok := tools.Parse(args[0]); ok {
					only, args = a, args[1:]
				}
			}
			key, err := scopeKey(global)
			if err != nil {
				return err
			}
			if reset {
				return resetOrder(only, key, args)
			}
			if len(args) == 0 {
				if !ui.Interactive() {
					return errors.New("which order? e.g. aims order personal work, or aims order claude personal work")
				}
				return askOrder(cmd.Context(), only, key)
			}
			targets, err := orderTargets(only, args)
			if err != nil {
				return err
			}
			for _, a := range targets {
				order, err := ops.SetOrder(a, args, key)
				if err != nil {
					return err
				}
				ui.Done("%s", ui.OrderSummary(a.Title(), key, order, ui.Err.Bold))
				warnPinned(a, order[0])
			}
			afterScope(targets, key, "aims order --global "+strings.Join(args, " "))
			return nil
		},
	}
	cmd.Flags().BoolVar(&global, "global", false, "the order of every folder without one of its own")
	cmd.Flags().BoolVar(&reset, "reset", false, "drop this project's own order")
	return cmd
}

// resetOrder drops the project's own order of one tool, or of every tool.
func resetOrder(only tool.Adapter, key string, args []string) error {
	switch {
	case len(args) > 0:
		return errors.New("--reset takes no profiles")
	case key == "":
		return errors.New("--reset is for a project's own order; --global has none to drop")
	}
	targets := tools.All()
	if only != nil {
		targets = []tool.Adapter{only}
	}
	dropped := false
	for _, a := range targets {
		had, err := ops.ForgetOrder(a, key)
		if err != nil {
			return err
		}
		if had {
			dropped = true
			ui.Done("%s in %s follows the order of every folder again", a.Title(), ui.Where(key))
		}
	}
	if !dropped {
		ui.Info("%s has no order of its own", ui.Where(key))
	}
	return nil
}

// limitLines say what happens when first hits a limit, after the order.
func limitLines(first string) []string {
	return []string{
		"When " + first + " works again, sessions go back to it.",
		"When every account is limited, sessions wait on the one that resets first.",
	}
}

// orderTargets is the tool given, or else every tool that has all of names.
func orderTargets(only tool.Adapter, names []string) ([]tool.Adapter, error) {
	if only != nil {
		return []tool.Adapter{only}, nil
	}
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	var out []tool.Adapter
	for _, a := range tools.All() {
		ps := cfg.Tools[a.ID()].Profiles
		if !slices.ContainsFunc(names, func(n string) bool { return ps[n] == nil }) {
			out = append(out, a)
		}
	}
	if len(out) > 0 {
		return out, nil
	}
	known := profileNames("")
	for i, n := range names {
		switch {
		case slices.Contains(known, n):
		case i == 0:
			return nil, fmt.Errorf("%q is neither a tool (%s) nor a profile (see: aims status)", n, strings.Join(tools.Names(), " or "))
		default:
			return nil, fmt.Errorf("no profile named %q (see: aims status)", n)
		}
	}
	return nil, fmt.Errorf("no tool has all of %s; name the tool, e.g. aims order %s %s", strings.Join(names, ", "), tools.Names()[0], strings.Join(names, " "))
}

// orderLines spells an order out, one account per line, for the confirmation.
func orderLines(order []string) []string {
	width := 0
	for _, n := range order {
		width = max(width, len(n))
	}
	lines := make([]string, len(order))
	for i, n := range order {
		role := "new sessions use it"
		switch {
		case i == 1:
			role = "while " + order[0] + " is limited or logged out"
		case i > 1:
			role = "while the ones above are limited or logged out"
		}
		lines[i] = fmt.Sprintf("%d. %-*s  %s", i+1, width, n, role)
	}
	if len(order) > 1 {
		lines = append(lines, limitLines(order[0])...)
	}
	return lines
}

// askOrder asks which tool and which account comes first (and next), shows
// the order and saves it once the user confirms.
func askOrder(ctx context.Context, only tool.Adapter, key string) error {
	report, err := loadAccounts(ctx)
	if err != nil {
		return err
	}
	// Only tools with two accounts or more have anything to order.
	var choices []ops.ToolStatus
	for _, t := range report {
		if (only == nil || t.ID == only.ID()) && len(t.Profiles) > 1 {
			choices = append(choices, t)
		}
	}
	if len(choices) == 0 {
		if only != nil {
			return fmt.Errorf("%s has one account, nothing to order; add another with: aims login %s <name>", only.Title(), only.ID())
		}
		return errors.New("no tool has two accounts yet, nothing to order; add one with: aims login <tool> <name>")
	}

	r := ui.NewRail()
	picked := choices
	if len(choices) > 1 {
		var opts []ui.Option
		if sameAccounts(choices) {
			label := "Both"
			if len(choices) > 2 {
				label = "Every tool"
			}
			opts = append(opts, ui.Option{Label: label, Value: "", Hint: "the same accounts, one order"})
		}
		for _, t := range choices {
			opts = append(opts, ui.Option{Label: t.Title, Value: string(t.ID), Hint: "now " + strings.Join(t.Names(), " → ")})
		}
		pick := opts[0].Value
		if err := r.Select("Order the accounts of which tool?", "", opts, &pick); err != nil {
			return cancelled(r, err)
		}
		if pick != "" {
			picked = slices.DeleteFunc(slices.Clone(choices), func(t ops.ToolStatus) bool { return string(t.ID) != pick })
		}
	}
	var titles []string
	for _, t := range picked {
		titles = append(titles, t.Title)
	}
	title := strings.Join(titles, " and ")

	remaining := picked[0].Names()
	var order []string
	for len(remaining) > 1 {
		q := "Which " + title + " account should new sessions use first?"
		desc := "The others stand in for it while it is limited or logged out."
		if len(picked) > 1 {
			q = "Which account should new sessions use first?"
			desc = "For " + title + ". " + desc
		}
		if len(order) > 0 {
			q = "Which one stands in first when " + strings.Join(order, " and ") + " cannot be used?"
			desc = ""
		}
		var opts []ui.Option
		for _, n := range remaining {
			hint := ""
			if len(picked) == 1 {
				if i := slices.IndexFunc(picked[0].Profiles, func(p ops.ProfileStatus) bool { return p.Name == n }); i >= 0 {
					hint = accountHint(picked[0].Profiles[i])
				}
			}
			opts = append(opts, ui.Option{Label: n, Value: n, Hint: hint})
		}
		pick := remaining[0]
		if err := r.Select(q, desc, opts, &pick); err != nil {
			return cancelled(r, err)
		}
		order = append(order, pick)
		remaining = slices.DeleteFunc(remaining, func(n string) bool { return n == pick })
	}
	order = append(order, remaining...)

	save := true
	scope := ""
	if key != "" {
		scope = " in " + ui.Where(key)
	}
	if err := r.Confirm("Save this order for "+title+scope+"?", strings.Join(orderLines(order), "\n"), &save); err != nil {
		return cancelled(r, err)
	}
	if !save {
		r.Outro(ui.Out.Dim.Render("Nothing changed."))
		return nil
	}
	for _, t := range picked {
		if _, err := ops.SetOrder(tools.Get(t.ID), order, key); err != nil {
			r.Fail(err.Error())
			return err
		}
	}
	r.Outro(ui.Out.OK.Render("Saved. ") + ui.OrderSummary(title, key, order, ui.Out.Bold))
	var changed []tool.Adapter
	for _, t := range picked {
		warnPinned(tools.Get(t.ID), order[0])
		changed = append(changed, tools.Get(t.ID))
	}
	afterScope(changed, key, "aims order --global")
	return nil
}

// sameAccounts reports whether the tools have the same profile names, so one
// order fits them all.
func sameAccounts(ts []ops.ToolStatus) bool {
	first := ts[0].Names()
	slices.Sort(first)
	for _, t := range ts[1:] {
		names := t.Names()
		slices.Sort(names)
		if !slices.Equal(first, names) {
			return false
		}
	}
	return true
}
