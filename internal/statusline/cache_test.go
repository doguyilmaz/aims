package statusline

import (
	"slices"
	"testing"
	"time"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/tool"
)

func TestUsageChangesIncludeRemovedWindowsAndQuotaState(t *testing.T) {
	windows := []tool.Window{{ID: "a", Label: "5h", Percent: 40}, {ID: "b", Label: "7d", Percent: 90}}
	previous := &config.Usage{At: time.Now(), Windows: windows}
	if !changed(previous, windows[:1]) {
		t.Fatal("removed weekly allowance remained cached")
	}
	reversed := slices.Clone(windows)
	slices.Reverse(reversed)
	if changed(previous, reversed) {
		t.Fatal("reordering unchanged quota windows caused another write")
	}
	for _, kind := range []string{"id", "unknown", "limited", "reset"} {
		next := slices.Clone(windows)
		switch kind {
		case "id":
			next[0].ID = "replacement"
		case "unknown":
			next[0].PercentUnknown = true
		case "limited":
			next[0].Limited = true
		case "reset":
			next[0].ResetsAt = time.Now().Add(time.Hour)
		}
		if !changed(previous, next) {
			t.Errorf("%s change was ignored", kind)
		}
	}
}
