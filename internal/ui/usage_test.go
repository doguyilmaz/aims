package ui

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/help"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/doguyilmaz/aims/internal/meter"
	"github.com/doguyilmaz/aims/internal/ops"
	"github.com/doguyilmaz/aims/internal/tool"
)

func TestUsageColumnsLineUp(t *testing.T) {
	soon := time.Now().Add(3 * time.Hour)
	report := []ops.ToolStatus{{Title: "Claude Code", Profiles: []ops.ProfileStatus{
		{Name: "personal", Plan: "pro", Usage: []tool.Window{{Label: "7d", Percent: 22}, {Label: "5h", Percent: 16, ResetsAt: soon}}},
		{Name: "work", Plan: "team", Usage: []tool.Window{{Label: "7d", Percent: 98}}},
	}}}
	ms := []meter.Status{
		{Title: "Kiro", Reading: meter.Reading{At: time.Now(), Rows: []meter.Row{{Name: "credits", Detail: "5000 of 5000 credits", Windows: []tool.Window{{Label: "month", Percent: 100, ResetsAt: soon}}}}}},
		{Title: "Antigravity", Reading: meter.Reading{At: time.Now(), Rows: []meter.Row{{Name: "Gemini Models", Windows: []tool.Window{{Label: "7d", Percent: 5}, {Label: "5h"}}}}}},
	}
	s := NewStyles(&bytes.Buffer{})
	// columns are where bars start: every window in one column per slot.
	columns := func(out string) map[int]int {
		at := map[int]int{}
		for _, l := range strings.Split(out, "\n") {
			for i, r := range []rune(l) {
				if r == '[' {
					at[i]++
				}
			}
		}
		return at
	}
	if c := columns(RenderUsage(s, report, ms, 90, 0)); len(c) != 2 {
		t.Errorf("bars start in %v, want two columns", c)
	}
	out := RenderUsage(s, report, ms, 90, 60)
	for _, l := range strings.Split(out, "\n") {
		if lipgloss.Width(l) > 60 {
			t.Errorf("wider than 60: %q", l)
		}
	}
	if c := columns(out); len(c) != 1 || strings.Count(out, "[") != 7 {
		t.Errorf("wrapped windows out of line (%v):\n%s", c, out)
	}
}

func TestDashboardUsageView(t *testing.T) {
	m := &dashboard{width: 100, usage: true, checking: map[string]bool{}, help: help.New(),
		report: []ops.ToolStatus{{Title: "Codex", Profiles: []ops.ProfileStatus{{Name: "personal", Usage: []tool.Window{{Label: "5h", Percent: 40}}}}}},
		meters: []meter.Status{{Title: "Grok", Reading: meter.Reading{At: time.Now(), Rows: []meter.Row{{Name: "credits", Windows: []tool.Window{{Label: "7d", Percent: 26}}}}}}},
	}
	v := m.View()
	for _, want := range []string{"personal", "40%", "Grok", "26%", "accounts"} {
		if !strings.Contains(v, want) {
			t.Errorf("no %q in\n%s", want, v)
		}
	}
	m.key(tea.KeyMsg{Type: tea.KeyTab})
	if m.usage {
		t.Error("tab did not go back to the accounts")
	}
}
