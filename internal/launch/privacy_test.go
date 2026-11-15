package launch

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/profiles"
	"github.com/doguyilmaz/aims/internal/testutil"
)

func TestSessionRefusesPendingPrivacyChangeWhenRepairLockBusy(t *testing.T) {
	o := setup(t)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	hub := cfg.EnsureHub("codex")
	source := filepath.Join(hub, "history.jsonl")
	testutil.Write(t, source, "old shared prompt\n")
	target := filepath.Join(cfg.Dir("codex", "a"), "history.jsonl")
	if err := os.Link(source, target); err != nil {
		t.Fatal(err)
	}
	cfg.Tools["codex"].Profiles["a"].Share = config.ShareSettings
	unlock, ok, err := fsx.Lock(filepath.Join(config.Home(), "codex-links"), time.Second)
	if err != nil || !ok {
		t.Fatalf("lock: %v", err)
	}
	defer unlock()
	if _, err := sessionEnv(o, cfg, "a"); err == nil {
		t.Fatal("started private work through a stale shared history link")
	}
	if !config.LoadState().Get("codex", "a").LastUsedAt.IsZero() {
		t.Fatal("failed privacy check recorded a launch")
	}
	// An unrelated sharing lock may still be busy after privacy was applied.
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if err := profiles.RequireIsolation(cfg, o.Tool, "a"); err != nil {
		t.Fatalf("already private profile rejected: %v", err)
	}
}
