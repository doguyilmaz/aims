package meter

import (
	"testing"
	"time"
)

func TestParseAgy(t *testing.T) {
	r := parseAgy([]byte(`{"status":"SUCCESS","num_turns":0,"command":{"name":"usage","data":{"groups":[
		{"name":"Gemini Models","buckets":[
			{"id":"gemini-weekly","window":"weekly","remaining_fraction":0.9464,"reset_time":"2026-10-31T23:17:17Z"},
			{"id":"gemini-5h","window":"5h","remaining_fraction":1,"reset_time":"2026-10-31T20:16:38Z"}]},
		{"name":"Claude and GPT models","buckets":[
			{"id":"3p-weekly","window":"weekly","remaining_fraction":0,"reset_time":"2026-10-31T23:04:01Z"},
			{"id":"3p-5h","window":"5h","disabled":true,"remaining_fraction":1}]}]}}}`))
	if r.Error != "" || len(r.Rows) != 2 {
		t.Fatalf("%+v", r)
	}
	g := r.Rows[0].Windows
	if g[0].Label != "7d" || g[0].Percent != 5.4 || g[1].Label != "5h" || g[1].Percent != 0 {
		t.Errorf("gemini %+v", g)
	}
	if w := r.Rows[1].Windows; len(w) != 1 || w[0].Percent != 100 || !w[0].ResetsAt.Equal(time.Date(2026, 10, 31, 23, 4, 1, 0, time.UTC)) {
		t.Errorf("a disabled window was kept, or the weekly one is wrong: %+v", w)
	}
	if r := parseAgy([]byte("Error: not logged in\n")); r.Error == "" {
		t.Error("no error for unexpected output")
	}
}

func TestParseOpenCode(t *testing.T) {
	r := parseOpenCode([]byte(`{"data":{"workspace":{"name":"Acme"},"budgets":[{"limit":50,"spent":33.4996,"per":"set","resetsAt":1793491200000}]}}`))
	if len(r.Rows) != 1 {
		t.Fatalf("%+v", r)
	}
	row := r.Rows[0]
	if row.Name != "Acme" || row.Detail != "$33.50 of $50" || row.Windows[0].Percent != 67 || row.Windows[0].Label != "budget" {
		t.Errorf("%+v", row)
	}
	if !row.Windows[0].ResetsAt.Equal(time.Unix(1793491200, 0)) {
		t.Errorf("resets %v", row.Windows[0].ResetsAt)
	}
}

func TestParseGrok(t *testing.T) {
	r := parseGrok([]byte(`{"config":{"creditUsagePercent":26.0,"currentPeriod":{"type":"USAGE_PERIOD_TYPE_WEEKLY","start":"2026-10-26T20:07:27.562728+00:00","end":"2026-11-02T20:07:27.562728+00:00"}},"subscription_tier":"X Premium"}`))
	if r.Plan != "X Premium" || len(r.Rows) != 1 {
		t.Fatalf("%+v", r)
	}
	if w := r.Rows[0].Windows[0]; w.Label != "7d" || w.Percent != 26 || w.ResetsAt.Month() != time.November {
		t.Errorf("%+v", w)
	}
	if r := parseGrok([]byte(`{"config":{}}`)); r.Error == "" {
		t.Error("no error without a percentage")
	}
}

func TestParseKiro(t *testing.T) {
	r := parseKiro([]byte(`{"subscriptionInfo":{"subscriptionTitle":"KIRO PRO MAX"},"usageBreakdownList":[{"displayNamePlural":"Credits",
		"currentUsageWithPrecision":5000,"currentOveragesWithPrecision":0,"usageLimitWithPrecision":5000,"nextDateReset":1793491200.0}]}`))
	if r.Plan != "KIRO PRO MAX" || len(r.Rows) != 1 {
		t.Fatalf("%+v", r)
	}
	row := r.Rows[0]
	if row.Windows[0].Percent != 100 || row.Windows[0].Label != "month" || row.Detail != "5000 of 5000 credits" {
		t.Errorf("%+v", row)
	}
}

func TestKiroEndpointFollowsTheProfileRegion(t *testing.T) {
	for arn, want := range map[string]string{
		"arn:aws:codewhisperer:eu-central-1:1:profile/X": "https://q.eu-central-1.amazonaws.com/",
		"arn:aws:codewhisperer:us-east-1:1:profile/X":    "https://codewhisperer.us-east-1.amazonaws.com/",
		"garbage": "https://codewhisperer.us-east-1.amazonaws.com/",
		"arn:aws:codewhisperer:evil.example/x:1:profile/X": "https://codewhisperer.us-east-1.amazonaws.com/",
	} {
		if got := kiroEndpoint(arn); got != want {
			t.Errorf("%s: %s", arn, got)
		}
	}
}
