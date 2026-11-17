package statusline

import (
	"strings"
	"testing"
	"time"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/profiles"
	"github.com/doguyilmaz/aims/internal/tool"
)

func TestRender(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	cfg := &config.Config{Failover: config.Failover{Threshold: 95}}
	next := func() string { return "personal" }
	w := func(label string, pct float64, resets time.Duration) tool.Window {
		return tool.Window{Label: label, Percent: pct, ResetsAt: now.Add(resets)}
	}
	cases := []struct {
		windows []tool.Window
		ps      *config.ProfileState
		mode    profiles.Mode
		want    string
	}{
		{nil, nil, profiles.Watched, "work · auto → personal"},
		{[]tool.Window{{Label: "5h", PercentUnknown: true}}, nil, profiles.Watched, "work · 5h — · auto → personal"},
		{[]tool.Window{{Label: "5h", PercentUnknown: true, Limited: true}}, nil, profiles.Watched, "work · 5h limited → personal"},
		{[]tool.Window{w("5h", 0, time.Hour)}, nil, profiles.Watched, "work ▱▱▱▱▱  0% · auto → personal"},
		{[]tool.Window{w("5h", 11, time.Hour), w("7d", 47, 48*time.Hour)}, nil, profiles.Watched, "work ▰▱▱▱▱  11% · 7d 47% · auto → personal"},
		{[]tool.Window{w("5h", 82, 80*time.Minute), w("7d", 61, 48*time.Hour)}, nil, profiles.Watched, "work ▰▰▰▰▱  82% ↻1h20m · 7d 61% · auto → personal"},
		{[]tool.Window{w("5h", 97, 54*time.Minute), w("7d", 61, 48*time.Hour)}, nil, profiles.Watched, "work ▰▰▰▰▰  97% ↻54m · 7d 61% → personal"},
		{[]tool.Window{w("5h", 30, time.Hour), w("7d", 96, 50*time.Hour)}, nil, profiles.Watched, "work ▰▰▱▱▱  30% · 7d 96% ↻2d → personal"},
		{[]tool.Window{w("5h", 6, time.Hour)}, &config.ProfileState{Until: now.Add(2 * time.Hour)}, profiles.Watched, "work limited ↻2h ▱▱▱▱▱  6% → personal"},
		// A session that does not move says so, also at its limit.
		{[]tool.Window{w("5h", 97, time.Hour)}, nil, profiles.Pinned, "work ▰▰▰▰▰  97% ↻1h · pinned"},
		{[]tool.Window{w("5h", 11, time.Hour)}, nil, profiles.Unwatched, "work ▰▱▱▱▱  11% · auto off"},
		{[]tool.Window{w("5h", 11, time.Hour)}, nil, "", "work ▰▱▱▱▱  11% · not via aims"},
	}
	for _, c := range cases {
		if got := render(cfg, "work", c.windows, c.ps, c.mode, "personal", next, now, false); got != c.want {
			t.Errorf("got  %q\nwant %q", got, c.want)
		}
	}
	if got := render(cfg, "work", []tool.Window{w("5h", 97, time.Hour)}, nil, profiles.Watched, "", func() string { return "" }, now, false); got != "work ▰▰▰▰▰  97% ↻1h · auto" {
		t.Errorf("no other account: %q", got)
	}
}

// A fresh reading after a banked reset must agree with the launcher and
// `aims status`, without erasing explicit holds or treating a short retry as
// evidence that the plan's five-hour allowance is used up.
func TestRenderEffectiveCooldown(t *testing.T) {
	now := time.Date(2026, 11, 17, 5, 0, 0, 0, time.UTC)
	cfg := &config.Config{Failover: config.Failover{Threshold: 95}}
	for _, kind := range []string{"reset", "manual", "busy", "unknown-window", "other-window", "older", "failed", "unknown-percent", "still-limited", "expired"} {
		t.Run(kind, func(t *testing.T) {
			window := tool.Window{Label: "5h", Percent: 74, ResetsAt: now.Add(135 * time.Minute)}
			ps := &config.ProfileState{
				Until: now.Add(2 * time.Minute), Reason: "limit", Window: "5h",
				MarkedBy: "a session", MarkedAt: now.Add(-time.Minute),
				Usage: &config.Usage{At: now, Windows: []tool.Window{window}},
			}
			switch kind {
			case "manual":
				ps.MarkedBy = "you"
			case "busy":
				ps.Reason = "busy"
			case "unknown-window":
				ps.Window = ""
			case "other-window":
				ps.Window = "7d"
			case "older":
				ps.Usage.At = ps.MarkedAt
			case "failed":
				ps.UsageError = "unavailable"
				ps.UsageAttemptedAt = now.Add(time.Second)
			case "unknown-percent":
				ps.Usage.Windows[0].PercentUnknown = true
			case "still-limited":
				ps.Usage.Windows[0].Limited = true
			case "expired":
				ps.Until = now
			}
			for _, mode := range []profiles.Mode{profiles.Pinned, profiles.Watched} {
				nextCalled := false
				text := render(cfg, "work", []tool.Window{window}, ps, mode, "personal", func() string {
					nextCalled = true
					return "personal"
				}, now, false)
				wantLimited := kind != "reset" && kind != "busy" && kind != "expired"
				if strings.Contains(text, "limited") != wantLimited || strings.Contains(text, "retry in 2m") != (kind == "busy") {
					t.Fatalf("%s/%s: misleading cooldown: %q", kind, mode, text)
				}
				if nextCalled != (mode == profiles.Watched && kind != "reset" && kind != "expired") {
					t.Fatalf("%s/%s: wrong next-account lookup: %q", kind, mode, text)
				}
				if !strings.Contains(text, "74% ↻2h15m") {
					t.Fatalf("quota reading changed: %q", text)
				}
			}
			if ps.Until.IsZero() || ps.Usage.Windows[0].Percent != 74 {
				t.Fatal("rendering mutated saved routing or usage state")
			}
		})
	}
}
