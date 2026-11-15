package profiles

import (
	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/tool"
	"testing"
)

func TestCompleteEmptyUsageClearsAnOlderAllowance(t *testing.T) {
	cfg, a := setup(t)
	if err := RecordUsage(a.ID(), "a", []tool.Window{{Label: "7d", Percent: 100}}, "fixture"); err != nil {
		t.Fatal(err)
	}
	record(cfg, a, "a", tool.Probe{Status: tool.ProbeOK, UsageReported: true})
	u := config.LoadState().Get(a.ID(), "a").Usage
	if u == nil || len(u.Windows) != 0 || u.Resets != nil {
		t.Fatalf("old allowance survived a complete empty report: %+v", u)
	}
}

func TestUnavailableUsageRetainsAgeAndMarksTheReadFailure(t *testing.T) {
	cfg, a := setup(t)
	_ = RecordUsage(a.ID(), "a", []tool.Window{{Label: "7d", Percent: 80}}, "fixture")
	before := config.LoadState().Get(a.ID(), "a").Usage.At
	record(cfg, a, "a", tool.Probe{Status: tool.ProbeOK, UsageError: "unavailable"})
	ps := config.LoadState().Get(a.ID(), "a")
	if ps.UsageError != "unavailable" || ps.Usage == nil || !ps.Usage.At.Equal(before) || ps.Usage.Windows[0].Percent != 80 {
		t.Fatalf("failed read looked fresh: %+v", ps)
	}
}

func TestNewAccountDoesNotInheritPreviousLoginUsage(t *testing.T) {
	cfg, a := setup(t)
	_ = RecordAccount(a.ID(), "a", "old@example.test", "plus")
	_ = RecordUsage(a.ID(), "a", []tool.Window{{Label: "7d", Percent: 80}}, "fixture")
	record(cfg, a, "a", tool.Probe{Status: tool.ProbeOK, Email: "new@example.test", Plan: "plus", UsageError: "unavailable"})
	ps := config.LoadState().Get(a.ID(), "a")
	if ps.Usage != nil || ps.Account.Email != "new@example.test" {
		t.Fatalf("previous account's quota was assigned to the new login: %+v", ps)
	}
}

func TestExplicitLimitWithUnknownPercentIsNotTreatedAsZero(t *testing.T) {
	cfg, a := setup(t)
	w := tool.Window{Label: "5h", PercentUnknown: true, Limited: true, ResetsAt: now.Add(RefreshDue)}
	if err := RecordUsage(a.ID(), "a", []tool.Window{w}, "fixture", cfg); err != nil {
		t.Fatal(err)
	}
	ev := Evaluate(t.Context(), cfg, config.LoadStateFor(cfg), a, "a")
	if ev.Usable || len(ev.Reasons) != 1 || ev.Reasons[0] != "5h limit for 10m" {
		t.Fatalf("explicit limit was ignored or shown as zero: %+v", ev)
	}
	w.Limited = false
	if err := RecordUsage(a.ID(), "a", []tool.Window{w}, "fixture", cfg); err != nil {
		t.Fatal(err)
	}
	if ev := Evaluate(t.Context(), cfg, config.LoadStateFor(cfg), a, "a"); !ev.Usable {
		t.Fatalf("unknown alone became a false limit: %+v", ev)
	}
}

func TestUnknownResetLimitIsSharedBySameAccount(t *testing.T) {
	cfg, a := setup(t)
	for _, name := range []string{"a", "b"} {
		_ = config.UpdateStateFor(cfg, a.ID(), name, func(s *config.ProfileState) {
			s.Account = &config.Account{Email: "same@example.test", Tenant: "workspace"}
		})
	}
	_ = RecordUsage(a.ID(), "a", []tool.Window{{Label: "7d", PercentUnknown: true, Limited: true}}, "fixture", cfg)
	ev := Evaluate(t.Context(), cfg, config.LoadStateFor(cfg), a, "b")
	if ev.Usable || !ev.ReadyAt.IsZero() || len(ev.Reasons) != 1 || ev.Reasons[0] != "same account as a, 7d limit" {
		t.Fatalf("same account escaped unknown-reset limit: %+v", ev)
	}
}
