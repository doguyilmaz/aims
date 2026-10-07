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
		return s.Warn.Render("limited " + profiles.Duration(time.Until(p.Until)))
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

// Usage renders the usage windows as small bars.
func Usage(s *Styles, ws []tool.Window, threshold float64) string {
	var parts []string
	for _, w := range ws {
		part := s.Dim.Render(w.Label+" ") + Bar(s, w.Percent, threshold, 8) + fmt.Sprintf(" %3.0f%%", w.Percent)
		if w.Percent >= 80 && w.ResetsAt.After(time.Now()) {
			part += s.Dim.Render(" resets " + profiles.Duration(time.Until(w.ResetsAt)))
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, "   ")
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
		nameW, acctW, stateW := 0, 0, 0
		for _, p := range t.Profiles {
			nameW = max(nameW, len(p.Name))
			acctW = max(acctW, lipgloss.Width(Account(s, p)))
			stateW = max(stateW, lipgloss.Width(State(s, p)))
		}
		for _, p := range t.Profiles {
			mark := "  "
			name := p.Name
			if p.Active {
				mark = s.Accent.Render("● ")
				name = s.Bold.Render(name)
			}
			line := "  " + mark + Pad(name, nameW) + "   " + Pad(Account(s, p), acctW) + "   " + Pad(State(s, p), stateW)
			if u := Usage(s, p.Usage, threshold); u != "" {
				line += "   " + u
			}
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
			if len(tags) > 0 {
				line += "  " + s.Dim.Render(strings.Join(tags, ", "))
			}
			b.WriteString(strings.TrimRight(line, " ") + "\n")
			if next := Next(t.ID, p); next != "" {
				b.WriteString("      " + strings.Repeat(" ", nameW) + s.Dim.Render("→ ") + Kbd(s, next) + "\n")
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
