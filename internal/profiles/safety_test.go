package profiles

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/testutil"
)

func TestHomeConflictDetectsExistingAliases(t *testing.T) {
	cfg, a := setup(t)
	// Explicit aliases model old configs and case-folded filesystem identities.
	cfg.Tools[a.ID()].Profiles["b"].Dir = cfg.Dir(a.ID(), "a")
	if err := HomeConflict(cfg, a, "a"); err == nil {
		t.Fatal("two registered labels can alter the same login")
	}
}

func TestIsolationCheckDoesNotRepairPrivateLinks(t *testing.T) {
	cfg, a := setup(t)
	p := cfg.Tools[a.ID()].Profiles["a"]
	p.Share = config.ShareSettings
	source := filepath.Join(cfg.EnsureHub(a.ID()), "history.jsonl")
	target := filepath.Join(cfg.Dir(a.ID(), "a"), "history.jsonl")
	testutil.Write(t, source, "shared prompt\n")
	if err := os.Link(source, target); err != nil {
		t.Fatal(err)
	}
	if err := RequireIsolation(cfg, a, "a"); err == nil {
		t.Fatal("pending private history accepted")
	}
	si, _ := os.Stat(source)
	ti, _ := os.Stat(target)
	if !os.SameFile(si, ti) {
		t.Fatal("read-only check changed history link")
	}
}

func TestAdoptedHubCannotPromisePrivateHistoryFromSharingPeers(t *testing.T) {
	cfg, a := setup(t)
	p := cfg.Tools[a.ID()].Profiles["a"]
	p.Existing, p.Share = true, config.ShareSettings
	if err := RequireIsolation(cfg, a, "a"); err == nil {
		t.Fatal("hub history still visible to all-sharing peers")
	}
	for _, other := range []string{"b", "c"} {
		cfg.Tools[a.ID()].Profiles[other].Share = config.ShareSettings
	}
	if err := RequireIsolation(cfg, a, "a"); err != nil {
		t.Fatalf("hub with private peers rejected: %v", err)
	}
}
