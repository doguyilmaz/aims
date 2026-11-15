package profiles

import (
	"context"
	"github.com/doguyilmaz/aims/internal/fsx"
	"path/filepath"
	"testing"
	"time"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/tool"
)

type passiveUsageReader struct {
	tool.Adapter
	probeCalls, usageCalls int
	result                 tool.Probe
}

func (a *passiveUsageReader) Probe(context.Context, tool.Home) tool.Probe {
	a.probeCalls++
	return tool.Probe{Status: tool.ProbeOK}
}

func (a *passiveUsageReader) Usage(context.Context, tool.Home) tool.Probe {
	a.usageCalls++
	return a.result
}

func TestPassiveUsageReaderPreservesMarksAndFailedObservationAge(t *testing.T) {
	cfg, adapter := setup(t)
	a := &passiveUsageReader{Adapter: adapter, result: tool.Probe{Status: tool.ProbeError, UsageError: "Quota access expired; use the native CLI."}}
	until := now.Add(RefreshDue)
	_ = config.UpdateState(a.ID(), "a", func(ps *config.ProfileState) {
		ps.Until, ps.Reason, ps.MarkedBy = until, "busy", "owner"
		ps.Usage = &config.Usage{At: now, Windows: []tool.Window{{Label: "7d", Percent: 97}}}
	})
	ReadUsage(t.Context(), cfg, a, "a")
	ps := config.LoadStateFor(cfg).Get(a.ID(), "a")
	if a.probeCalls != 0 || a.usageCalls != 1 || ps.UsageError != a.result.UsageError || ps.Usage == nil || !ps.Usage.At.Equal(now) || ps.Usage.Windows[0].Percent != 97 || !ps.Until.Equal(until) || ps.MarkedBy != "owner" || ps.NeedsLogin {
		t.Fatalf("passive refresh changed routing, credential state or old measurement: %+v", ps)
	}
	a.result = tool.Probe{Status: tool.ProbeOK, UsageReported: true, Windows: []tool.Window{{Label: "7d", Percent: 2}}}
	ReadUsage(t.Context(), cfg, a, "a")
	ps = config.LoadStateFor(cfg).Get(a.ID(), "a")
	if a.probeCalls != 0 || a.usageCalls != 2 || ps.Usage.Windows[0].Percent != 2 || ps.UsageError != "" || !ps.Until.Equal(until) {
		t.Fatalf("successful passive observation was not isolated: %+v", ps)
	}
	Live(t.Context(), cfg, a, "a")
	if a.probeCalls != 1 || a.usageCalls != 2 {
		t.Fatal("explicit login probe unexpectedly used the passive quota reader")
	}
}

func TestNativeNoQuotaKeepsPassiveMeasurementAndNewFailure(t *testing.T) {
	cfg, adapter := setup(t)
	_ = RecordAccount(adapter.ID(), "a", "same@example.test", "plus", cfg)
	a := &passiveUsageReader{Adapter: adapter, result: tool.Probe{Status: tool.ProbeOK, UsageReported: true, Email: "same@example.test", Plan: "plus", Windows: []tool.Window{{Label: "7d", Percent: 80}}}}
	if _, err := ReadUsage(t.Context(), cfg, a, "a"); err != nil {
		t.Fatal(err)
	}
	old := now
	config.Now = func() time.Time { return old.Add(time.Minute) }
	record(cfg, adapter, "a", tool.Probe{Status: tool.ProbeOK, Email: "same@example.test", Plan: "plus"})
	ps := config.LoadStateFor(cfg).Get(adapter.ID(), "a")
	if ps.Usage == nil || ps.Usage.Windows[0].Percent != 80 || !ps.Usage.At.Equal(old) {
		t.Fatalf("native no-quota result hid passive measurement: %+v", ps)
	}
	config.Now = func() time.Time { return old.Add(2 * time.Minute) }
	record(cfg, adapter, "a", tool.Probe{Status: tool.ProbeError, UsageError: "new native failure"})
	ps = config.LoadStateFor(cfg).Get(adapter.ID(), "a")
	if ps.Usage == nil || ps.UsageError != "new native failure" || !ps.UsageAttemptedAt.Equal(config.Now()) {
		t.Fatalf("old success masked/rewound native failure: %+v", ps)
	}
}

type nativeIdentity struct {
	tool.Adapter
	account tool.Account
}

func (a nativeIdentity) Account(context.Context, tool.Home) tool.Account { return a.account }

func TestNativeIdentityCorrectionFencesPassiveUsageEvenWhenCleanupBusy(t *testing.T) {
	for _, tenant := range []bool{false, true} {
		t.Run(map[bool]string{false: "email", true: "workspace"}[tenant], func(t *testing.T) {
			cfg, adapter := setup(t)
			old := config.Account{Email: "old@example.test", Tenant: "workspace-old", Plan: "plus"}
			_ = config.UpdateStateFor(cfg, adapter.ID(), "a", func(s *config.ProfileState) { s.Account = &old })
			a := &passiveUsageReader{Adapter: adapter, result: tool.Probe{Status: tool.ProbeOK, Email: old.Email, Plan: old.Plan, UsageReported: true, Windows: []tool.Window{{Label: "7d", Percent: 99}}}}
			if _, err := ReadUsage(t.Context(), cfg, a, "a"); err != nil {
				t.Fatal(err)
			}
			target := tool.Account{Email: "new@example.test", Tenant: "workspace-new", Plan: "plus", LoggedIn: tool.Known(true)}
			if tenant {
				target.Email = old.Email
			}
			binding, revision := config.CaptureUsageAccount(adapter.ID(), "a")
			unlock, ok, err := fsx.Lock(filepath.Join(config.Home(), "usage-observations.json"), 0)
			if err != nil || !ok {
				t.Fatal(err)
			}
			defer unlock()
			view := config.LoadStateFor(cfg)
			ev := Evaluate(t.Context(), cfg, view, nativeIdentity{adapter, target}, "a")
			if ev.State.Usage != nil || !ev.Usable || ev.State.Account.Email != target.Email || ev.State.Account.Tenant != target.Tenant {
				t.Fatalf("current selection retained prior account quota: %+v", ev)
			}
			ps := config.LoadStateFor(cfg).Get(adapter.ID(), "a")
			if ps.Usage != nil || ps.Account.Email != target.Email || ps.Account.Tenant != target.Tenant || ps.AccountRevision == revision {
				t.Fatalf("cache undid native correction: %+v", ps)
			}
			// The old observation remains physically present while locked, but ignored.
			if len(config.UsageObservationNotes(cfg)) == 0 {
				t.Fatal("ignored row missing from doctor diagnostics")
			}
			unlock()
			if err := config.CommitUsageObservation(cfg, adapter.ID(), "a", binding, revision, func(*config.ProfileState) { t.Fatal("in-flight old account reached mutation") }); err == nil {
				t.Fatal("old in-flight usage committed after identity correction")
			}
		})
	}
}

func TestOptionalUsageCacheCleanupCannotBlockLogin(t *testing.T) {
	cfg, adapter := setup(t)
	a := &passiveUsageReader{Adapter: adapter, result: tool.Probe{Status: tool.ProbeOK, UsageReported: true, Windows: []tool.Window{{Label: "7d", Percent: 99}}}}
	if _, err := ReadUsage(t.Context(), cfg, a, "a"); err != nil {
		t.Fatal(err)
	}
	unlock, ok, err := fsx.Lock(filepath.Join(config.Home(), "usage-observations.json"), 0)
	if err != nil || !ok {
		t.Fatal(err)
	}
	defer unlock()
	if err := RecordLogin(adapter.ID(), "a", tool.Account{Email: "new@example.test"}, cfg); err != nil {
		t.Fatal(err)
	}
	if ps := config.LoadStateFor(cfg).Get(adapter.ID(), "a"); ps.Usage != nil || ps.Account.Email != "new@example.test" {
		t.Fatalf("busy cleanup resurrected prior login: %+v", ps)
	}
	if err := config.RemoveState(adapter.ID(), "a"); err != nil {
		t.Fatal(err)
	}
}

func TestNativeProbeCanCorrectCachedIdentityBackToRawIdentity(t *testing.T) {
	cfg, adapter := setup(t)
	_ = RecordAccount(adapter.ID(), "a", "native@example.test", "plus", cfg)
	a := &passiveUsageReader{Adapter: adapter, result: tool.Probe{Status: tool.ProbeOK, Email: "different@example.test", Plan: "plus", UsageReported: true, Windows: []tool.Window{{Label: "7d", Percent: 99}}}}
	if _, err := ReadUsage(t.Context(), cfg, a, "a"); err != nil {
		t.Fatal(err)
	}
	if ps := config.LoadStateFor(cfg).Get(adapter.ID(), "a"); ps.Account.Email != a.result.Email {
		t.Fatalf("fixture cache not visible: %+v", ps)
	}
	record(cfg, adapter, "a", tool.Probe{Status: tool.ProbeOK, Email: "native@example.test", Plan: "plus"})
	if ps := config.LoadStateFor(cfg).Get(adapter.ID(), "a"); ps.Account.Email != "native@example.test" || ps.Usage != nil {
		t.Fatalf("cached identity undid native reversion: %+v", ps)
	}
}
