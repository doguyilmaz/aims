package ui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/ops"
	"github.com/doguyilmaz/aims/internal/profiles"
	"github.com/doguyilmaz/aims/internal/tool"
)

// State is the one-word state of a profile and its style.
func State(s *Styles, p ops.ProfileStatus) string {
	switch {
	case p.LoggedIn != nil && !*p.LoggedIn:
		return s.Bad.Render("not logged in")
	case p.Usable && p.LoggedIn == nil:
		return s.OK.Render("ready") + s.Dim.Render("?")
	case p.Usable:
		return s.OK.Render("ready")
	}
	for _, r := range p.Reasons {
		if strings.HasPrefix(r, "login") {
			return s.Bad.Render("login expired")
		}
	}
	if !p.Until.IsZero() {
		word := "limited "
		if p.Reason == "busy" {
			word = "busy "
		}
		return s.Warn.Render(word + profiles.Duration(time.Until(p.Until)))
	}
	if len(p.Reasons) > 0 {
		return s.Warn.Render("near limit") // a usage window is over the threshold; the bar shows which
	}
	return s.Warn.Render("unavailable")
}

// Account describes who a profile is logged in as.
func Account(s *Styles, p ops.ProfileStatus) string {
	switch {
	case p.Email != "" && p.Plan != "":
		return p.Email + s.Dim.Render(" · "+p.Plan)
	case p.Email != "":
		return p.Email
	case p.Method == "token" || p.Method == "api-key":
		return s.Dim.Render("API key")
	case p.LoggedIn != nil && !*p.LoggedIn:
		return s.Dim.Render("no login")
	}
	return s.Dim.Render("unknown account")
}

// usageSlots are the windows every row shows, filled or not, so columns line up.
var usageSlots = []string{"5h", "7d"}

// Usage renders the usage windows as small bars: always 5h and 7d (a dash
// when unknown), then any other window, then when the fullest one resets.
func Usage(s *Styles, ws []tool.Window, threshold float64) string {
	const cells = 6
	byLabel := map[string]tool.Window{}
	labels := slices.Clone(usageSlots)
	for _, w := range ws {
		byLabel[w.Label] = w
		if !slices.Contains(labels, w.Label) {
			labels = append(labels, w.Label)
		}
	}
	var parts []string
	var soonest *tool.Window
	for _, l := range labels {
		w, ok := byLabel[l]
		if !ok {
			parts = append(parts, s.Dim.Render(l+" ")+Bar(s, 0, threshold, cells)+s.Dim.Render("    –"))
			continue
		}
		parts = append(parts, s.Dim.Render(l+" ")+Bar(s, w.Percent, threshold, cells)+fmt.Sprintf(" %3.0f%%", w.Percent))
		if w.Percent >= 80 && w.ResetsAt.After(time.Now()) && (soonest == nil || w.Percent > soonest.Percent) {
			soonest = &w
		}
	}
	out := strings.Join(parts, "   ")
	if soonest != nil {
		out += s.Dim.Render("   " + soonest.Label + " resets " + profiles.Duration(time.Until(soonest.ResetsAt)))
	}
	return out
}

// Email is who a profile is logged in as, for the table.
func Email(s *Styles, p ops.ProfileStatus) string {
	switch {
	case p.Email != "":
		return p.Email
	case p.Method == "token" || p.Method == "api-key":
		return s.Dim.Render("API key")
	case p.LoggedIn != nil && !*p.LoggedIn:
		return s.Dim.Render("no login")
	}
	return s.Dim.Render("unknown account")
}

// ShortPlan names a plan in a word: "self_serve_business_prolite" is
// "business". The full name stays in the dashboard card and --json.
func ShortPlan(plan string) string {
	p := strings.ToLower(plan)
	for _, k := range []string{"enterprise", "business", "team", "edu", "max", "pro", "plus", "free"} {
		if strings.Contains(p, k) {
			return k
		}
	}
	p = strings.ReplaceAll(p, "_", " ")
	if r := []rune(p); len(r) > 10 {
		return string(r[:9]) + "…"
	}
	return p
}

// Layout is the table's column widths, measured over every tool so the
// sections line up.
type Layout struct{ name, email, plan, state int }

// NewLayout measures report.
func NewLayout(s *Styles, report []ops.ToolStatus) Layout {
	var l Layout
	for _, t := range report {
		for _, p := range t.Profiles {
			l.name = max(l.name, lipgloss.Width(p.Name))
			l.email = max(l.email, lipgloss.Width(Email(s, p)))
			l.plan = max(l.plan, lipgloss.Width(ShortPlan(p.Plan)))
			l.state = max(l.state, lipgloss.Width(State(s, p)))
		}
	}
	return l
}

// Row is a profile's columns after its marker: name, account, plan, state, usage.
func (l Layout) Row(s *Styles, p ops.ProfileStatus, name, state string, threshold float64) string {
	return Pad(name, l.name) + "   " + Pad(Email(s, p), l.email) + "   " + Pad(s.Dim.Render(ShortPlan(p.Plan)), l.plan) +
		"   " + Pad(state, max(l.state, lipgloss.Width(state))) + "   " + Usage(s, p.Usage, threshold)
}

// Next is the command that fixes a profile's problem, or "".
func Next(id tool.ID, p ops.ProfileStatus) string {
	if (p.LoggedIn != nil && !*p.LoggedIn) || hasLoginReason(p) {
		return "aims login " + string(id) + " " + p.Name
	}
	return ""
}

func hasLoginReason(p ops.ProfileStatus) bool {
	for _, r := range p.Reasons {
		if strings.HasPrefix(r, "login") {
			return true
		}
	}
	return false
}

// RenderStatus is the output of `aims status`.
func RenderStatus(s *Styles, report []ops.ToolStatus, threshold float64) string {
	var b strings.Builder
	l := NewLayout(s, report)
	for i, t := range report {
		if i > 0 {
			b.WriteString("\n")
		}
		head := s.Bold.Render(t.Title)
		switch {
		case !t.Installed:
			head += "  " + s.Dim.Render("not installed")
		case t.Hub != "":
			head += "  " + s.Dim.Render(fsx.Tildify(t.Hub))
		}
		b.WriteString(head + "\n")
		if len(t.Profiles) == 0 {
			b.WriteString("  " + s.Dim.Render("no accounts yet") + "\n")
			continue
		}
		for _, p := range t.Profiles {
			mark := "  "
			name := p.Name
			if p.Active {
				mark = s.Accent.Render("● ")
				name = s.Bold.Render(name)
			}
			line := "  " + mark + l.Row(s, p, name, State(s, p), threshold)
			var tags []string
			if p.Existing {
				tags = append(tags, "default login")
			}
			switch p.Share {
			case "none":
				tags = append(tags, "isolated")
			case "settings":
				tags = append(tags, "own history")
			}
			if p.Pinned {
				tags = append(tags, "pinned in this terminal")
			}
			if p.StandsIn != "" {
				tags = append(tags, "standing in for "+p.StandsIn)
			}
			if p.MarkedBy != "" {
				tags = append(tags, "marked by "+p.MarkedBy)
			}
			if len(tags) > 0 {
				line += "  " + s.Dim.Render(strings.Join(tags, ", "))
			}
			b.WriteString(strings.TrimRight(line, " ") + "\n")
			if next := Next(t.ID, p); next != "" {
				b.WriteString("      " + strings.Repeat(" ", l.name) + s.Dim.Render("→ ") + Kbd(s, next) + "\n")
			}
		}
	}
	return b.String()
}

// PlainStatus is the status without colour, for the MCP server.
func PlainStatus(report []ops.ToolStatus) string {
	var b strings.Builder
	for _, t := range report {
		fmt.Fprintf(&b, "%s", t.Title)
		if !t.Installed {
			b.WriteString(" (not installed)")
		}
		b.WriteString("\n")
		if len(t.Profiles) == 0 {
			b.WriteString("  no profiles\n")
		}
		for _, p := range t.Profiles {
			mark := " "
			if p.Active {
				mark = "*"
			}
			state := "ready"
			if !p.Usable {
				state = strings.Join(p.Reasons, ", ")
				if p.MarkedBy != "" {
					state += " (marked by " + p.MarkedBy + ")"
				}
			}
			acct := p.Email
			if acct == "" {
				acct = "unknown account"
			}
			if p.Plan != "" {
				acct += " (" + p.Plan + ")"
			}
			fmt.Fprintf(&b, "  %s %s: %s, %s", mark, p.Name, acct, state)
			for _, w := range p.Usage {
				if !slices.ContainsFunc(p.Reasons, func(r string) bool { return strings.HasPrefix(r, w.Label+" at ") }) {
					fmt.Fprintf(&b, ", %s %.0f%%", w.Label, w.Percent)
				}
			}
			if p.StandsIn != "" {
				fmt.Fprintf(&b, "; new sessions use it until %s works again", p.StandsIn)
			}
			b.WriteString("\n")
		}
	}
	b.WriteString("(* = active for new sessions)\n")
	return b.String()
}

// Until describes when a rest period ends: "30m, until 14:05".
func Until(t time.Time) string {
	d := time.Until(t)
	clock := t.Local().Format("15:04")
	if d > 20*time.Hour {
		clock = t.Local().Format("Mon 15:04")
	}
	return profiles.Duration(d) + ", until " + clock
}
