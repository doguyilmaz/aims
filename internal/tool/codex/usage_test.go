package codex

import (
	"testing"
	"time"

	"github.com/doguyilmaz/aims/internal/tool"
)

func TestBankedResetsRemainSeparateFromMainLimit(t *testing.T) {
	var p tool.Probe
	additionalUsage([]byte(`{"rateLimitResetCredits":{"availableCount":2,"credits":[{"id":"fixture","resetType":"codexRateLimits","status":"available","title":"Full reset","grantedAt":1794441600,"expiresAt":1795046400}]},"rateLimitsByLimitId":{"codex":{"primary":{"usedPercent":95}},"review":{"limitName":"Reviews","primary":{"usedPercent":100,"windowDurationMins":10080,"resetsAt":1795046400}}}}`), &p)
	if len(p.Windows) != 0 || p.Status != "" || p.Resets == nil || p.Resets.Available != 2 || len(p.Groups) != 1 {
		t.Fatalf("display metadata changed the main allowance: %+v", p)
	}
	if got := p.Resets.Credits[0]; got.Kind != "codexRateLimits" || !got.ExpiresAt.Equal(time.Unix(1795046400, 0)) {
		t.Fatalf("lost reset metadata: %+v", got)
	}
}

func TestResetCountUnknownEmptyAndDetailsAreDistinct(t *testing.T) {
	for _, tc := range []struct {
		raw     string
		known   bool
		details bool
	}{
		{`{}`, false, false},
		{`{"rateLimitResetCredits":null}`, false, false},
		{`{"rateLimitResetCredits":{"availableCount":0,"credits":null}}`, true, false},
		{`{"rateLimitResetCredits":{"availableCount":0,"credits":[]}}`, true, true},
	} {
		var p tool.Probe
		additionalUsage([]byte(tc.raw), &p)
		if (p.Resets != nil) != tc.known {
			t.Fatalf("known state lost for %s", tc.raw)
		}
		if p.Resets != nil && (p.Resets.Credits != nil) != tc.details {
			t.Fatalf("detail availability lost for %s", tc.raw)
		}
	}
}
