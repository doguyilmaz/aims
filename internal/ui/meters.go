package ui

import (
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/doguyilmaz/aims/internal/meter"
	"github.com/doguyilmaz/aims/internal/profiles"
)

// staleAfter is when a reading is old enough to say its age: past one
// keepalive run.
const staleAfter = 2 * time.Hour

// RenderMeters is the "Other tools" section: the limits of installed tools
// aims reads but does not run sessions on.
func RenderMeters(s *Styles, ms []meter.Status, threshold float64) string {
	if len(ms) == 0 {
		return ""
	}
	width := 0
	for _, m := range ms {
		for _, r := range m.Rows {
			width = max(width, lipgloss.Width(r.Name))
		}
	}
	var b strings.Builder
	b.WriteString(s.Bold.Render("Other tools") + "\n")
	for _, m := range ms {
		head := "  " + s.Bold.Render(m.Title)
		if m.Plan != "" {
			head += "  " + s.Dim.Render(ShortPlan(m.Plan))
		}
		switch {
		case m.At.IsZero():
			head += "  " + s.Dim.Render("not checked yet (aims status --live)")
		case time.Since(m.At) > staleAfter:
			head += "  " + s.Dim.Render("checked "+profiles.Duration(time.Since(m.At))+" ago")
		}
		if m.Error != "" {
			head += "  " + s.Warn.Render(m.Error)
		}
		b.WriteString(head + "\n")
		for _, r := range m.Rows {
			line := "    " + Pad(r.Name, width) + "   " + usage(s, r.Windows, threshold, nil)
			if r.Detail != "" {
				line += "   " + s.Dim.Render(r.Detail)
			}
			b.WriteString(line + "\n")
		}
	}
	return b.String()
}
