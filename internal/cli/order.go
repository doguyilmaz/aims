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
	return &cobra.Command{
		Use:   "order [tool] [profile...]",
		Short: "Set which account comes first and which stand in for it",
		Long: "The first profile becomes the active one: new sessions use it. While it is\n" +
			"limited or logged out they use the next usable one in this order, and come\n" +
			"back to the first once it works again.\n\n" +
			"Without a tool, every tool that has these profiles changes. Without profiles,\n" +
			"aims asks for the order, shows it and saves it once you confirm.",
		Example: `aims order
aims order personal work            # Claude Code and Codex
aims order claude personal work`,
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
			if len(args) == 0 {
				if !ui.Interactive() {
					return errors.New("which order? e.g. aims order personal work, or aims order claude personal work")
				}
				return askOrder(cmd.Context(), only)
			}
			targets, err := orderTargets(only, args)
			if err != nil {
				return err
			}
			for _, a := range targets {
				order, err := ops.SetOrder(a, args)
				if err != nil {
					return err
				}
				ui.Done("%s", orderSummary(ui.Err, a.Title(), order))
				warnPinned(a, order[0])
			}
			return nil
		},
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

// orderSummary says what an order means: "New Claude Code sessions use
// personal; while it is limited or logged out: work".
func orderSummary(s *ui.Styles, titles string, order []string) string {
	msg := fmt.Sprintf("New %s sessions use %s", titles, s.Bold.Render(order[0]))
	if len(order) > 1 {
		msg += "; while it is limited or logged out: " + strings.Join(order[1:], ", then ")
	}
	return msg
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
		lines = append(lines, "When "+order[0]+" works again, new sessions go back to it.")
	}
	return lines
}

// askOrder asks which tool and which account comes first (and next), shows
// the order and saves it once the user confirms.
func askOrder(ctx context.Context, only tool.Adapter) error {
	var report []ops.ToolStatus
	var err error
	ui.Spin("checking your accounts", func() { report, err = ops.Status(ctx, nil) })
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
			opts = append(opts, ui.Option{Label: t.Title, Value: string(t.ID), Hint: "now " + strings.Join(accountNames(t), " → ")})
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

	remaining := accountNames(picked[0])
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
				hint = accountHint(picked[0], n)
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
	if err := r.Confirm("Save this order for "+title+"?", strings.Join(orderLines(order), "\n"), &save); err != nil {
		return cancelled(r, err)
	}
	if !save {
		r.Outro(ui.Out.Dim.Render("Nothing changed."))
		return nil
	}
	for _, t := range picked {
		if _, err := ops.SetOrder(tools.Get(t.ID), order); err != nil {
			r.Fail(err.Error())
			return err
		}
	}
	r.Outro(ui.Out.OK.Render("Saved. ") + orderSummary(ui.Out, title, order))
	for _, t := range picked {
		warnPinned(tools.Get(t.ID), order[0])
	}
	return nil
}

// accountNames lists a tool's profiles in their current order.
func accountNames(t ops.ToolStatus) []string {
	out := make([]string, len(t.Profiles))
	for i, p := range t.Profiles {
		out[i] = p.Name
	}
	return out
}

// sameAccounts reports whether the tools have the same profile names, so one
// order fits them all.
func sameAccounts(ts []ops.ToolStatus) bool {
	first := accountNames(ts[0])
	slices.Sort(first)
	for _, t := range ts[1:] {
		names := accountNames(t)
		slices.Sort(names)
		if !slices.Equal(first, names) {
			return false
		}
	}
	return true
}

// accountHint is who a profile is and why it cannot be used, if it cannot.
func accountHint(t ops.ToolStatus, name string) string {
	for _, p := range t.Profiles {
		if p.Name != name {
			continue
		}
		var parts []string
		if p.Email != "" {
			parts = append(parts, p.Email)
		}
		if !p.Usable {
			parts = append(parts, strings.Join(p.Reasons, ", "))
		}
		return strings.Join(parts, " · ")
	}
	return ""
}
