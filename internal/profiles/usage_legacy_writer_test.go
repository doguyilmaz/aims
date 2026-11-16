package profiles

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/testutil"
	"github.com/doguyilmaz/aims/internal/tool"
)

// The v0.4.6 state/window schema intentionally cannot retain newer fields.
// This exercises a real read/mutate/rewrite cycle, not a current-state copy.
type legacyProfileState struct {
	Until      time.Time       `json:"until,omitzero"`
	Reason     string          `json:"reason,omitempty"`
	Window     string          `json:"window,omitempty"`
	NeedsLogin bool            `json:"needsLogin,omitempty"`
	MarkedBy   string          `json:"markedBy,omitempty"`
	Detail     string          `json:"detail,omitempty"`
	MarkedAt   time.Time       `json:"markedAt,omitzero"`
	Usage      *legacyUsage    `json:"usage,omitempty"`
	Account    *config.Account `json:"account,omitempty"`
	LastUsedAt time.Time       `json:"lastUsedAt,omitzero"`
}

type legacyUsage struct {
	Windows []struct {
		Label    string    `json:"label"`
		Percent  float64   `json:"pct"`
		ResetsAt time.Time `json:"resetsAt,omitzero"`
	} `json:"windows"`
	Source string    `json:"source,omitempty"`
	At     time.Time `json:"at,omitzero"`
}

func TestPassiveUsageSurvivesLegacyWriterWithoutTouchingRoutingOrCredentials(t *testing.T) {
	cfg, adapter := setup(t)
	a := &passiveUsageReader{Adapter: adapter, result: tool.Probe{
		Status: tool.ProbeOK, UsageReported: true, Email: "work@example.test", Plan: "plus",
		Windows: []tool.Window{{Label: "7d", Percent: 0}},
		Groups:  []tool.QuotaGroup{{ID: "extra", Name: "Extra", Windows: []tool.Window{{Label: "7d", Percent: 10}}}},
		Resets:  &tool.ResetBalance{Available: 3},
	}}
	for _, name := range []string{"a", "b"} {
		if err := config.UpdateState(a.ID(), name, func(s *config.ProfileState) {
			s.Until, s.Reason, s.Window = now.Add(time.Hour), "busy", "7d"
			s.NeedsLogin, s.MarkedBy, s.Detail, s.MarkedAt = true, "owner", "keep", now
			s.LastUsedAt, s.Account = now, &config.Account{Email: name + "@example.test", Tenant: "tenant"}
		}); err != nil {
			t.Fatal(err)
		}
	}
	statePath := filepath.Join(config.Home(), "state.json")
	configPath := filepath.Join(config.Home(), "config.json")
	credentials := filepath.Join(cfg.Dir(a.ID(), "a"), "auth.json")
	wantState, wantConfig, wantCredentials := testutil.Read(t, statePath), testutil.Read(t, configPath), testutil.Read(t, credentials)
	ReadUsage(t.Context(), cfg, a, "a")
	if testutil.Read(t, statePath) != wantState || testutil.Read(t, configPath) != wantConfig || testutil.Read(t, credentials) != wantCredentials {
		t.Fatal("passive API refresh changed routing, configuration or native credentials")
	}
	legacy := map[tool.ID]map[string]*legacyProfileState{}
	if err := json.Unmarshal([]byte(wantState), &legacy); err != nil {
		t.Fatal(err)
	}
	legacy[a.ID()]["b"].LastUsedAt = now.Add(time.Minute)
	b, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	testutil.Write(t, statePath, string(b))
	wantLegacy := testutil.Read(t, statePath)
	view := config.LoadStateFor(cfg).Get(a.ID(), "a")
	if view.Usage == nil || view.Usage.Resets == nil || view.Usage.Resets.Available != 3 || len(view.Usage.Groups) != 1 || view.Usage.Windows[0].Percent != 0 {
		t.Fatalf("legacy rewrite erased passive observations: %+v", view)
	}
	ReadUsage(t.Context(), cfg, a, "a")
	if testutil.Read(t, statePath) != wantLegacy || testutil.Read(t, configPath) != wantConfig || testutil.Read(t, credentials) != wantCredentials {
		t.Fatal("later app refresh rewrote legacy routing/context")
	}
	if got := config.LoadState().Get(a.ID(), "b"); !got.NeedsLogin || got.Reason != "busy" || !got.LastUsedAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("unrelated profile state lost: %+v", got)
	}
	if info, err := os.Stat(filepath.Join(config.Home(), "usage-observations.json")); err != nil || os.PathSeparator == '/' && info.Mode().Perm()&0077 != 0 {
		t.Fatalf("passive cache is not private: %v %v", info, err)
	}
	if err := RecordLogin(a.ID(), "a", tool.Account{Email: "new@example.test"}, cfg); err != nil {
		t.Fatal(err)
	}
	if got := config.LoadStateFor(cfg).Get(a.ID(), "a"); got.Usage != nil || got.Account.Email != "new@example.test" {
		t.Fatalf("login resurrected old passive usage: %+v", got)
	}
}
