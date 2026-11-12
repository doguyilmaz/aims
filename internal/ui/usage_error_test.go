package ui

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/doguyilmaz/aims/internal/meter"
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

func TestUnknownUsageDoesNotRenderAnExhaustedBar(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	meters := []meter.Status{{Title: "OpenCode", Reading: meter.Reading{At: time.Now(), Rows: []meter.Row{{Name: "model", Windows: []tool.Window{{Label: "budget", Percent: 150, PercentUnknown: true}}}}}}}
	out := RenderUsage(NewStyles(&bytes.Buffer{}), nil, meters, 90, 0)
	if strings.Contains(out, "#") {
		t.Fatalf("unknown per-model spend rendered a filled usage bar: %s", out)
	}
}
