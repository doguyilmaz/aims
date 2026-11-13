package ui

import (
	"github.com/doguyilmaz/aims/internal/tool"
	"testing"
	"time"
)

func TestUsageKeepsDistinctBucketsWithTheSameDuration(t *testing.T) {
	r := usageRow{slots: []string{"5h", "7d"}, windows: []tool.Window{{ID: "regular", Label: "5h", Percent: 10}, {ID: "custom", Label: "5h", Percent: 80}}}
	w := r.cells()
	if len(w) != 3 || w[0].ID != "regular" || w[2].ID != "custom" {
		t.Fatalf("a quota bucket disappeared: %+v", w)
	}
}

func TestPastResetDoesNotImplyFreshQuota(t *testing.T) {
	if got := resets(tool.Window{ResetsAt: time.Now().Add(-time.Minute)}); got != "reset passed; refresh" {
		t.Fatalf("past reset was silently treated as current: %q", got)
	}
}
