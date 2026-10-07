package profiles

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/testutil"
	"github.com/doguyilmaz/aims/internal/tool"
	"github.com/doguyilmaz/aims/internal/tools"
)

func TestMain(m *testing.M) { testutil.Main(m) }

var now = time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

// setup makes codex profiles a, b and c, all logged in.
func setup(t *testing.T) (*config.Config, tool.Adapter) {
	t.Helper()
	home := testutil.Sandbox(t)
	old := config.Now
	config.Now = func() time.Time { return now }
	t.Cleanup(func() { config.Now = old })
	cfg, err := config.Update(func(c *config.Config) error {
		for _, n := range []string{"a", "b", "c"} {
			c.Tools["codex"].Profiles[n] = &config.Profile{}
			c.Tools["codex"].Order = append(c.Tools["codex"].Order, n)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"a", "b", "c"} {
		testutil.Write(t, filepath.Join(home, ".aims", "profiles", "codex", n, "auth.json"), `{"tokens":{"refresh_token":"r"}}`)
	}
	return cfg, tools.Get("codex")
}

func TestChooseSkipsUnusable(t *testing.T) {
	cfg, a := setup(t)
	if p := Choose(t.Context(), cfg, config.LoadState(), a, "b", nil); p.Name != "b" {
		t.Fatalf("preferred first: %+v", p)
	}
	if err := MarkLimited(cfg, "codex", "b", "limit", time.Time{}, 0); err != nil {
		t.Fatal(err)
	}
	_ = MarkNeedsLogin("codex", "a")
	p := Choose(t.Context(), cfg, config.LoadState(), a, "b", nil)
	if p.Name != "c" || len(p.Skipped) != 2 || p.Skipped[0].Name != "b" {
		t.Fatalf("pick: %+v", p)
	}
	if !strings.Contains(strings.Join(p.Skipped[0].Reasons, ","), "limit for 5h") || p.Skipped[1].Reasons[0] != "login expired" {
		t.Fatalf("reasons: %+v", p.Skipped)
	}
	if p := Choose(t.Context(), cfg, config.LoadState(), a, "", []string{"c"}); p.Name != "" {
		t.Fatalf("excluded the only usable one: %+v", p)
	}
	_ = Clear("codex", "b")
	if p := Choose(t.Context(), cfg, config.LoadState(), a, "b", nil); p.Name != "b" {
		t.Fatalf("after clear: %+v", p)
	}
}

func TestUsageThreshold(t *testing.T) {
	cfg, a := setup(t)
	_ = RecordUsage("codex", "a", []tool.Window{
		{Label: "5h", Percent: 96, ResetsAt: now.Add(40 * time.Minute)},
		{Label: "7d", Percent: 50},
	}, "test")
	ev := Evaluate(t.Context(), cfg, config.LoadState(), a, "a")
	if ev.Usable || !slices.Contains(ev.Reasons, "5h at 96% for 40m") {
		t.Fatalf("over threshold: %+v", ev)
	}
	// A window that has already reset no longer counts.
	_ = RecordUsage("codex", "a", []tool.Window{{Label: "5h", Percent: 99, ResetsAt: now.Add(-time.Minute)}}, "test")
	if ev := Evaluate(t.Context(), cfg, config.LoadState(), a, "a"); !ev.Usable {
		t.Fatalf("stale usage: %+v", ev)
	}
}

func TestMarkLimitedUsesKnownReset(t *testing.T) {
	cfg, _ := setup(t)
	_ = RecordUsage("codex", "a", []tool.Window{{Label: "5h", Percent: 100, ResetsAt: now.Add(70 * time.Minute)}}, "test")
	_ = MarkLimited(cfg, "codex", "a", "", time.Time{}, 0)
	if until := config.LoadState().Get("codex", "a").Until; !until.Equal(now.Add(70 * time.Minute)) {
		t.Fatalf("until = %v", until)
	}
	// An existing cooldown is not stretched by a second report.
	_ = MarkLimited(cfg, "codex", "a", "limit", time.Time{}, 0)
	if until := config.LoadState().Get("codex", "a").Until; !until.Equal(now.Add(70 * time.Minute)) {
		t.Fatalf("stretched to %v", until)
	}
	// An explicit duration wins.
	_ = MarkLimited(cfg, "codex", "b", "limit", time.Time{}, 15*time.Minute)
	if until := config.LoadState().Get("codex", "b").Until; !until.Equal(now.Add(15 * time.Minute)) {
		t.Fatalf("explicit: %v", until)
	}
}

func TestEnv(t *testing.T) {
	cfg, a := setup(t)
	cfg.Tools["codex"].Profiles["b"].Env = map[string]string{"CODEX_API_KEY": "own"}
	base := []string{"PATH=/bin", "CODEX_API_KEY=shell", "CODEX_HOME=/somewhere"}

	env, dropped := Env(cfg, a, "a", base)
	if !slices.Contains(dropped, "CODEX_API_KEY") || slices.Contains(env, "CODEX_API_KEY=shell") {
		t.Fatalf("conflicting key kept: %v", env)
	}
	if !slices.Contains(env, "CODEX_HOME="+cfg.Dir("codex", "a")) || !slices.Contains(env, "AIMS_SESSION_PROFILE=codex:a") || !slices.Contains(env, "PATH=/bin") {
		t.Fatalf("env: %v", env)
	}
	env, dropped = Env(cfg, a, "b", base)
	if len(dropped) != 0 || !slices.Contains(env, "CODEX_API_KEY=own") {
		t.Fatalf("profile's own key: %v %v", env, dropped)
	}
}

func TestEnvExistingUnsetsHome(t *testing.T) {
	cfg, a := setup(t)
	cfg.Tools["codex"].Profiles["a"].Existing = true
	env, _ := Env(cfg, a, "a", []string{"CODEX_HOME=/somewhere/else"})
	for _, kv := range env {
		if strings.HasPrefix(kv, "CODEX_HOME=") {
			t.Fatalf("CODEX_HOME should be unset for the adopted login: %v", env)
		}
	}
}

func TestDuration(t *testing.T) {
	for d, want := range map[time.Duration]string{
		0: "now", 20 * time.Second: "1m", 45 * time.Minute: "45m", 2 * time.Hour: "2h",
		2*time.Hour + 10*time.Minute: "2h10m", 60 * time.Hour: "3d", 7 * 24 * time.Hour: "7d",
	} {
		if got := Duration(d); got != want {
			t.Errorf("Duration(%v) = %q, want %q", d, got, want)
		}
	}
}

// Launches running at the same moment take turns repairing links, so no
// version of a file is lost.
func TestConcurrentSyncKeepsEveryVersion(t *testing.T) {
	cfg, a := setup(t)
	hub := cfg.EnsureHub("codex")
	past := time.Now().Add(-time.Hour)
	for round := range 20 {
		dir := cfg.Dir("codex", "a")
		testutil.Write(t, filepath.Join(hub, "config.toml"), "hub")
		os.Chtimes(filepath.Join(hub, "config.toml"), past, past)
		os.Remove(filepath.Join(dir, "config.toml"))
		testutil.Write(t, filepath.Join(dir, "config.toml"), "profile")
		var wg sync.WaitGroup
		for range 2 {
			wg.Add(1)
			go func() { defer wg.Done(); Sync(cfg, a, "a", false) }()
		}
		wg.Wait()
		seen := map[string]bool{}
		files, _ := filepath.Glob(filepath.Join(hub, "config.toml*"))
		for _, f := range files {
			if b, err := os.ReadFile(f); err == nil {
				seen[string(b)] = true
			}
		}
		if !seen["hub"] || !seen["profile"] || testutil.Read(t, filepath.Join(hub, "config.toml")) != "profile" {
			t.Fatalf("round %d lost a version: %v", round, files)
		}
		for _, f := range files {
			if f != filepath.Join(hub, "config.toml") {
				os.Remove(f)
			}
		}
	}
}
