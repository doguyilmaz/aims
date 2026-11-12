package ui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/doguyilmaz/aims/internal/meter"
	"github.com/doguyilmaz/aims/internal/ops"
	"github.com/doguyilmaz/aims/internal/profiles"
	"github.com/doguyilmaz/aims/internal/tool"
)

// staleAfter is when a reading is old enough to say its age: past one
// keepalive run.
const staleAfter = 2 * time.Hour

const barCells = 6

type usageRow struct {
	name, plan, detail string
	windows            []tool.Window
	// slots are labels shown as a dash when the row has no such window.
	slots []string
}

type usageSection struct {
	head string
	rows []usageRow
}

// RenderUsage is the output of `aims usage`: every window of every account
// and of the other tools, with when it resets, in columns that line up
// across all of them. Windows wrap onto further lines past width.
func RenderUsage(s *Styles, report []ops.ToolStatus, ms []meter.Status, threshold float64, width int) string {
	var secs []usageSection
	for _, t := range report {
		if len(t.Profiles) == 0 {
			continue
		}
		sec := usageSection{head: s.Bold.Render(t.Title)}
		for _, p := range t.Profiles {
			detail := "banked resets: not reported"
			if p.BankedResets != nil {
				detail = fmt.Sprintf("banked resets: %d", p.BankedResets.Available)
				details := 0
				for _, credit := range p.BankedResets.Credits {
					if credit.Status == "available" {
						details++
					}
					if credit.Status == "available" && !credit.ExpiresAt.IsZero() {
						detail += " · expires " + credit.ExpiresAt.Local().Format("Jan 2 15:04 MST")
					}
					if credit.Status == "available" && credit.NoExpiry {
						detail += " · does not expire"
					}
				}
				if int64(details) < p.BankedResets.Available {
					detail += fmt.Sprintf(" · reset details %d/%d; other expiries not reported", details, p.BankedResets.Available)
				}
			}
			if p.UsageAt.IsZero() {
				detail += " · not checked"
			} else if time.Since(p.UsageAt) > staleAfter {
				detail += " · reported " + profiles.Duration(time.Since(p.UsageAt)) + " ago"
			}
			if p.UsageError != "" {
				detail += " · last check: " + p.UsageError + "; previous reading retained"
			}
			sec.rows = append(sec.rows, usageRow{name: p.Name, plan: ShortPlan(p.Plan), detail: detail, windows: p.Usage, slots: usageSlots})
			for _, g := range p.UsageGroups {
				sec.rows = append(sec.rows, usageRow{name: p.Name + " · " + g.Name, windows: g.Windows})
			}
		}
		secs = append(secs, sec)
	}
	for _, m := range ms {
		head := s.Bold.Render(m.Title)
		if m.Plan != "" {
			head += "  " + s.Dim.Render(ShortPlan(m.Plan))
		}
		switch {
		case m.At.IsZero():
			head += "  " + s.Dim.Render("not checked yet (aims usage --live)")
		case time.Since(m.At) > staleAfter:
			head += "  " + s.Dim.Render("checked "+profiles.Duration(time.Since(m.At))+" ago")
		}
		if m.Error != "" {
			head += "  " + s.Warn.Render(m.Error)
		}
		sec := usageSection{head: head}
		for _, r := range m.Rows {
			sec.rows = append(sec.rows, usageRow{name: r.Name, detail: r.Detail, windows: r.Windows})
		}
		secs = append(secs, sec)
	}

	var nameW, planW, labelW, resetW int
	for _, sec := range secs {
		for _, r := range sec.rows {
			nameW = max(nameW, lipgloss.Width(r.name))
			planW = max(planW, lipgloss.Width(r.plan))
			for _, w := range r.cells() {
				labelW = max(labelW, lipgloss.Width(w.Label))
				resetW = max(resetW, lipgloss.Width(resets(w)))
			}
		}
	}
	cell := func(w tool.Window, known bool) string {
		pct := "   –"
		if known && !w.PercentUnknown {
			pct = fmt.Sprintf("%3.0f%%", w.Percent)
		}
		barPercent := w.Percent
		if !known || w.PercentUnknown {
			barPercent = 0
		}
		out := s.Dim.Render(Pad(w.Label, labelW)) + " " + Bar(s, barPercent, threshold, barCells) + " " + pct
		if resetW > 0 {
			out += "  " + s.Dim.Render(Pad(resets(w), resetW))
		}
		return out
	}
	cellW := labelW + 1 + barCells + 5
	if resetW > 0 {
		cellW += 2 + resetW
	}
	lead := 2 + nameW + 2
	if planW > 0 {
		lead += planW + 2
	}
	perLine := 1 << 10
	if width > 0 {
		perLine = max(1, (width-lead+3)/(cellW+3))
	}

	var b strings.Builder
	for i, sec := range secs {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(sec.head + "\n")
		for _, r := range sec.rows {
			prefix := "  " + Pad(r.name, nameW) + "  "
			if planW > 0 {
				prefix += Pad(s.Dim.Render(r.plan), planW) + "  "
			}
			var cells []string
			for _, w := range r.cells() {
				cells = append(cells, cell(w, r.has(w.Label)))
			}
			if r.detail != "" && width == 0 {
				cells = append(cells, s.Dim.Render(r.detail))
			}
			for j := 0; j < len(cells) || j == 0; j += perLine {
				line := strings.Repeat(" ", lead)
				if j == 0 {
					line = prefix
				}
				if j < len(cells) {
					line += strings.Join(cells[j:min(j+perLine, len(cells))], "   ")
				}
				b.WriteString(strings.TrimRight(line, " ") + "\n")
			}
			if r.detail != "" && width > 0 {
				for _, line := range strings.Split(ansi.Wrap(r.detail, max(10, width-lead), ""), "\n") {
					b.WriteString(strings.Repeat(" ", lead) + s.Dim.Render(line) + "\n")
				}
			}
		}
	}
	return b.String()
}

// cells are the row's windows, with an empty one for each missing slot.
func (r usageRow) cells() []tool.Window {
	var out []tool.Window
	used := make([]bool, len(r.windows))
	for _, l := range r.slots {
		w := tool.Window{Label: l}
		for i, x := range r.windows {
			if !used[i] && x.Label == l {
				w = x
				used[i] = true
				break
			}
		}
		out = append(out, w)
	}
	var rest []tool.Window
	for i, w := range r.windows {
		if !used[i] {
			rest = append(rest, w)
		}
	}
	slices.SortStableFunc(rest, func(a, b tool.Window) int { return span(a.Label) - span(b.Label) })
	return append(out, rest...)
}

func (r usageRow) has(label string) bool {
	return slices.ContainsFunc(r.windows, func(w tool.Window) bool { return w.Label == label })
}

// span orders windows from the shortest: 5h, 1d, 7d, month, then the rest.
func span(label string) int {
	if i := slices.Index([]string{"5h", "1d", "7d", "month"}, label); i >= 0 {
		return i
	}
	return 4
}

func resets(w tool.Window) string {
	if w.ResetsAt.IsZero() {
		return ""
	}
	if !w.ResetsAt.After(time.Now()) {
		return "reset passed; refresh"
	}
	return "resets " + profiles.Duration(time.Until(w.ResetsAt))
}
