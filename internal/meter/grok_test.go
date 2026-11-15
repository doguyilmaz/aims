package meter

import "testing"

func TestGrokUnifiedBillingKeepsKnownResetWithoutInventingUsage(t *testing.T) {
	r := parseGrok([]byte(`{"config":{"currentPeriod":{"type":"USAGE_PERIOD_TYPE_WEEKLY","start":"2026-11-09T20:07:27Z","end":"2026-11-16T20:07:27Z"},"isUnifiedBillingUser":true},"subscription_tier":"fixture"}`))
	if r.Error != "" || len(r.Rows) != 1 {
		t.Fatalf("valid partial billing report rejected: %+v", r)
	}
	w := r.Rows[0].Windows[0]
	if !w.PercentUnknown || w.ResetsAt.IsZero() || w.Label != "7d" {
		t.Fatalf("unknown percentage presented as zero, or reset lost: %+v", w)
	}
}
