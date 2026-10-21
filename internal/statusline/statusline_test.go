package statusline

import (
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
