package profiles

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/testutil"
	"github.com/doguyilmaz/aims/internal/tool"
	"github.com/doguyilmaz/aims/internal/tools"
)

func TestLoginInvalidatesOnlyDerivedUsageAndMarks(t *testing.T) {
	cfg, a := setup(t)
	credentials := filepath.Join(cfg.Dir(a.ID(), "a"), "auth.json")
	before := testutil.Read(t, credentials)
	_ = config.UpdateState(a.ID(), "a", func(ps *config.ProfileState) {
		ps.Usage = &config.Usage{Windows: []tool.Window{{Label: "7d", Percent: 100}}, Resets: &tool.ResetBalance{Available: 5}}
		ps.NeedsLogin = true
		ps.Until = now.Add(RefreshDue)
		ps.LastUsedAt = now
	})
	if err := RecordLogin(a.ID(), "a", tool.Account{Email: "new@example.test", Plan: "plus", Tenant: "new-tenant"}); err != nil {
		t.Fatal(err)
	}
	ps := config.LoadState().Get(a.ID(), "a")
	if ps.Usage != nil || ps.NeedsLogin || !ps.Until.IsZero() || !ps.LastUsedAt.Equal(now) || ps.Account.Tenant != "new-tenant" {
		t.Fatalf("login left stale limits or discarded unrelated state: %+v", ps)
	}
	if got := testutil.Read(t, credentials); got != before {
		t.Fatal("login bookkeeping changed native credentials")
	}
}

type identityProbe struct{ tool.Adapter }

func (identityProbe) Probe(context.Context, tool.Home) tool.Probe {
	return tool.Probe{Status: tool.ProbeOK}
}
func (identityProbe) Account(context.Context, tool.Home) tool.Account {
	return tool.Account{Email: "new@example.test", Plan: "pro"}
}

func TestClaudeLiveProbeBindsItsOfflineAccountIdentity(t *testing.T) {
	cfg, _ := setup(t)
	var err error
	cfg, err = config.Update(func(c *config.Config) error {
		c.Tools["claude"].Profiles["a"] = &config.Profile{}
		c.Tools["claude"].Order = []string{"a"}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = RecordAccount("claude", "a", "old@example.test", "pro")
	_ = RecordUsage("claude", "a", []tool.Window{{Label: "7d", Percent: 100}}, "fixture")
	p := Live(t.Context(), cfg, identityProbe{tools.Get("claude")}, "a")
	ps := config.LoadState().Get("claude", "a")
	if p.Email != "new@example.test" || ps.Usage != nil || ps.Account.Email != "new@example.test" {
		t.Fatalf("a changed Claude login retained old quota: %+v %+v", p, ps)
	}
}

type observationProbe struct {
	tool.Adapter
	status tool.ProbeStatus
}

func (a observationProbe) Probe(context.Context, tool.Home) tool.Probe {
	return tool.Probe{Status: a.status, UsageReported: true, Windows: []tool.Window{{Label: "7d", Percent: 20}}}
}

func TestUsageDisplayRefreshPreservesManualRoutingMarks(t *testing.T) {
	for _, status := range []tool.ProbeStatus{tool.ProbeOK, tool.ProbeLimit, tool.ProbeLogin} {
		t.Run(string(status), func(t *testing.T) {
			cfg, a := setup(t)
			until := now.Add(RefreshDue)
			_ = config.UpdateState(a.ID(), "a", func(ps *config.ProfileState) { ps.Until = until; ps.Reason = "busy"; ps.MarkedBy = "owner" })
			ReadUsage(t.Context(), cfg, observationProbe{a, status}, "a")
			ps := config.LoadState().Get(a.ID(), "a")
			if !ps.Until.Equal(until) || ps.Reason != "busy" || ps.MarkedBy != "owner" || ps.NeedsLogin {
				t.Fatalf("passive usage refresh changed manual routing: %+v", ps)
			}
			if ps.Usage == nil || ps.Usage.Windows[0].Percent != 20 {
				t.Fatal("observation was not cached")
			}
		})
	}
}

func TestAPIKeyLoginDoesNotInheritOAuthQuota(t *testing.T) {
	cfg, a := setup(t)
	_ = RecordAccount(a.ID(), "a", "old@example.test", "plus")
	_ = RecordUsage(a.ID(), "a", []tool.Window{{Label: "7d", Percent: 100}}, "fixture")
	record(cfg, a, "a", tool.Probe{Status: tool.ProbeOK, Plan: "apiKey", UsageError: "unavailable"})
	if u := config.LoadState().Get(a.ID(), "a").Usage; u != nil {
		t.Fatal("OAuth limits survived an API-key authentication mode change")
	}
}
