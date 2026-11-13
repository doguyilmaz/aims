package usage

import (
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/meter"
	"github.com/doguyilmaz/aims/internal/testutil"
	"github.com/doguyilmaz/aims/internal/tool"
)

func TestSnapshotKeepsAllProvidersAccountsAndResetKindsTogether(t *testing.T) {
	testutil.Sandbox(t)
	t.Setenv("PATH", t.TempDir()) // Cached export must not need installed providers.
	observed := time.Now().UTC().Truncate(time.Second)
	reset := observed.Add(24 * time.Hour)
	window := tool.Window{ID: "weekly", Label: "7d", Percent: 75, ResetsAt: reset}
	if _, err := config.Update(func(c *config.Config) error {
		for _, id := range []tool.ID{"claude", "codex"} {
			c.Tools[id].Order = []string{"personal", "work"}
			for _, name := range c.Tools[id].Order {
				c.Tools[id].Profiles[name] = &config.Profile{}
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []tool.ID{"claude", "codex"} {
		for _, name := range []string{"personal", "work"} {
			if err := config.UpdateState(id, name, func(p *config.ProfileState) {
				p.Usage = &config.Usage{At: observed, Windows: []tool.Window{window}}
				if id == "codex" {
					p.Usage.Resets = &tool.ResetBalance{Credits: []tool.ResetCredit{}}
					if name == "personal" {
						p.Usage.Resets.Available = 2
						p.Usage.Resets.Credits = []tool.ResetCredit{{ID: "fixture", Status: "available", ExpiresAt: reset}}
					}
				}
			}); err != nil {
				t.Fatal(err)
			}
		}
	}
	meters := map[string]meter.Reading{}
	for _, id := range []string{"kiro", "grok", "opencode"} {
		meters[id] = meter.Reading{At: observed, Rows: []meter.Row{{ID: "credits", Name: "Fixture", Windows: []tool.Window{window}}}}
	}
	groups := []string{"gemini", "claude-gpt", "argon", "carbon"}
	reading := meter.Reading{At: observed}
	for _, id := range groups {
		reading.Rows = append(reading.Rows, meter.Row{ID: id, Name: id, Windows: []tool.Window{window}})
	}
	meters["agy"] = reading
	if err := fsx.WriteJSON(filepath.Join(config.Home(), "meters.json"), meters, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := AddCustom("agy", "carbon", "Carbon reminder", reset.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	snapshot, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(snapshot.Providers))
	for _, provider := range snapshot.Providers {
		ids = append(ids, provider.ID)
		wantRows := 1
		if provider.ID == "claude" || provider.ID == "codex" {
			wantRows = 2
		} else if provider.ID == "agy" {
			wantRows = 4
		}
		if len(provider.Rows) != wantRows {
			t.Fatalf("%s lost cached accounts or model groups: %+v", provider.ID, provider.Rows)
		}
		for _, row := range provider.Rows {
			if !row.ObservedAt.Equal(observed) || len(row.Windows) != 1 || row.Windows[0].Percent != 75 || !row.Windows[0].ResetsAt.Equal(reset) {
				t.Fatalf("%s/%s lost its observation or scheduled reset: %+v", provider.ID, row.ID, row)
			}
		}
	}
	if !slices.Equal(ids, []string{"claude", "codex", "kiro", "grok", "agy", "opencode"}) {
		t.Fatalf("provider order or cached providers lost: %v", ids)
	}
	for _, provider := range snapshot.Providers[:2] {
		if len(provider.Rows) != 2 || provider.Rows[0].ID != "personal" || provider.Rows[1].ID != "work" {
			t.Fatalf("%s accounts merged: %+v", provider.ID, provider.Rows)
		}
	}
	claude, codex, agy := snapshot.Providers[0], snapshot.Providers[1], snapshot.Providers[4]
	if claude.Rows[0].Resets != nil || codex.Rows[0].Resets == nil || codex.Rows[1].Resets == nil || codex.Rows[0].Resets.Available != 2 || codex.Rows[1].Resets.Available != 0 {
		t.Fatal("unreported, available and empty banked inventories were conflated")
	}
	if len(codex.Rows[0].Resets.Credits) != 1 || !codex.Rows[0].Resets.Credits[0].ExpiresAt.Equal(reset) {
		t.Fatal("capped banked reset details or their expiry were lost")
	}
	groupIDs := make([]string, 0, len(agy.Rows))
	for _, row := range agy.Rows {
		groupIDs = append(groupIDs, row.ID)
		if row.Resets != nil {
			t.Fatal("a custom reminder became an Antigravity banked reset")
		}
	}
	if !slices.Equal(groupIDs, groups) || len(snapshot.CustomResets) != 1 || snapshot.CustomResets[0].Provider != "agy" || snapshot.CustomResets[0].Row != "carbon" {
		t.Fatalf("model groups or group-specific reminder lost: %+v", snapshot)
	}
}
