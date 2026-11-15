package meter

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/fsx"
)

type fixtureMeter struct {
	id     string
	result Reading
	calls  *int
}

func (f fixtureMeter) ID() string    { return f.id }
func (f fixtureMeter) Title() string { return f.id }
func (f fixtureMeter) Bin() string   { p, _ := os.Executable(); return p }
func (f fixtureMeter) Read(context.Context, string) Reading {
	if f.calls != nil {
		(*f.calls)++
	}
	return f.result
}

func TestFailedRefreshRetainsAgeOfPreviousReading(t *testing.T) {
	t.Setenv(config.HomeVar, t.TempDir())
	oldAll := all
	all = []Meter{fixtureMeter{id: "fixture", result: Reading{Error: "offline"}}}
	t.Cleanup(func() { all = oldAll })
	at := time.Now().Add(-24 * time.Hour).UTC()
	if err := fsx.WriteJSON(path(), map[string]Reading{"fixture": {At: at, Rows: []Row{{Name: "previous"}}}}, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Refresh(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Error != "offline" || !got[0].At.Equal(at) || !got[0].AttemptedAt.After(at) || got[0].Rows[0].Name != "previous" {
		t.Fatalf("failed refresh made stale data look fresh: %+v", got)
	}
}

func TestRefreshFilterDoesNotContactOtherProviders(t *testing.T) {
	t.Setenv(config.HomeVar, t.TempDir())
	a, b := 0, 0
	oldAll := all
	all = []Meter{fixtureMeter{id: "a", calls: &a}, fixtureMeter{id: "b", calls: &b}}
	t.Cleanup(func() { all = oldAll })
	if _, err := Refresh(t.Context(), "a"); err != nil {
		t.Fatal(err)
	}
	if a != 1 || b != 0 {
		t.Fatalf("unexpected provider reads: %d %d", a, b)
	}
}

func TestLegacyFailureDoesNotClaimAnObservationTime(t *testing.T) {
	r := observedReading(Reading{Error: "offline", At: time.Now(), Rows: []Row{{Name: "retained"}}})
	if !r.At.IsZero() || r.AttemptedAt.IsZero() || len(r.Rows) != 1 {
		t.Fatalf("legacy failure was presented as a successful fresh read: %+v", r)
	}
}

func TestOpenCodeKeepsGoMetricsWhenBudgetsAreAbsent(t *testing.T) {
	r := parseOpenCode([]byte(`{"data":{"workspace":{"name":"Fixture"},"go":{"weekly":{"percent":23,"limited":false,"resetsAt":1793491200000}}}}`))
	if r.Error != "" || len(r.Rows) != 1 || len(r.Rows[0].Windows) != 1 || r.Rows[0].Windows[0].Percent != 23 {
		t.Fatalf("available Go quota was discarded with absent Console budgets: %+v", r)
	}
}

func TestKiroReadsRootResetInSecondsOrMilliseconds(t *testing.T) {
	for _, epoch := range []string{"1793491200", "1793491200000"} {
		r := parseKiro([]byte(`{"nextDateReset":` + epoch + `,"usageBreakdownList":[{"displayNamePlural":"Credits","currentUsageWithPrecision":10,"usageLimitWithPrecision":50}]}`))
		if len(r.Rows) != 1 || !r.Rows[0].Windows[0].ResetsAt.Equal(time.Unix(1793491200, 0)) {
			t.Fatalf("root reset lost: %+v", r)
		}
	}
}

func TestOpenCodeKeepsZeroBudgetsGoAndPerModelLimits(t *testing.T) {
	r := parseOpenCode([]byte(`{"data":{"workspace":{"name":"Fixture"},"go":{"weekly":{"percent":20,"limited":false,"resetsAt":1793491200000}},"budgets":[{"id":"zero","limit":0,"spent":0,"per":"set"},{"id":"models","limit":10,"spent":15,"per":"model","models":[{"providerID":"a","modelID":"one","name":"One","spent":5},{"providerID":"b","modelID":"two","name":"Two"}]}]}}`))
	if r.Error != "" || len(r.Rows) != 4 {
		t.Fatalf("lost applicable limits: %+v", r)
	}
	if !r.Rows[1].Windows[0].PercentUnknown || !r.Rows[1].Windows[0].Limited {
		t.Fatal("$0 budget was hidden or presented as a valid ratio")
	}
	if r.Rows[2].Windows[0].Percent != 50 || !r.Rows[3].Windows[0].PercentUnknown {
		t.Fatal("per-model spend was guessed from an aggregate")
	}
}
