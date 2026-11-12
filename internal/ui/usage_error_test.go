package ui

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/doguyilmaz/aims/internal/ops"
	"github.com/doguyilmaz/aims/internal/tool"
)

func TestUsageShowsFailedCoreRefreshEvenForRecentData(t *testing.T) {
	report := []ops.ToolStatus{{Title: "Codex", Profiles: []ops.ProfileStatus{{Name: "personal", UsageAt: time.Now().Add(-time.Minute), UsageError: "unavailable", Usage: []tool.Window{{Label: "7d", Percent: 20}}}}}}
	out := RenderUsage(NewStyles(&bytes.Buffer{}), report, nil, 90, 0)
	if !strings.Contains(out, "last check: unavailable; previous reading retained") {
		t.Fatalf("recent cached quota hid its failed refresh: %s", out)
	}
}
