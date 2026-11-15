package config

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/doguyilmaz/aims/internal/testutil"
	"github.com/doguyilmaz/aims/internal/tool"
)

func TestUsageObservationVersionAndProfileFence(t *testing.T) {
	for _, bad := range []string{`{`, `null`, `{}`, `{"schemaVersion":"2.0.0","profiles":{}}`} {
		t.Run(bad, func(t *testing.T) {
			testutil.Sandbox(t)
			cfg, err := Update(func(c *Config) error {
				c.Tools["codex"].Profiles["work"] = &Profile{Generation: "first"}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(Home(), "usage-observations.json")
			testutil.Write(t, path, bad)
			if err := UpdateUsageObservationFor(cfg, "codex", "work", func(*ProfileState) { t.Fatal("invalid cache reached mutation") }); err == nil {
				t.Fatal("invalid/newer cache was accepted")
			}
			if testutil.Read(t, path) != bad {
				t.Fatal("invalid/newer cache was overwritten")
			}
			if _, err := LoadStateView(cfg); err == nil {
				t.Fatal("unreadable cache was hidden from diagnostics")
			}
			if err := RemoveState("codex", "work"); err != nil || testutil.Read(t, path) != bad {
				t.Fatalf("optional invalid cache blocked profile bookkeeping or was modified: %v", err)
			}
		})
	}
	testutil.Sandbox(t)
	before, err := Update(func(c *Config) error {
		c.Tools["codex"].Profiles["work"] = &Profile{Generation: "first"}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	after, _ := Update(func(c *Config) error { c.Tools["codex"].Profiles["work"].Generation = "second"; return nil })
	if err := UpdateUsageObservationFor(before, "codex", "work", func(*ProfileState) { t.Fatal("stale registration mutated cache") }); err != ErrProfileChanged {
		t.Fatal(err)
	}
	if got := LoadStateFor(after).Get("codex", "work"); got.Usage != nil {
		t.Fatal("old registration leaked into new profile")
	}
}

func TestNativeUsageRetainsSeparateAPIOnlyMetadataAndFailureAge(t *testing.T) {
	testutil.Sandbox(t)
	cfg, _ := Update(func(c *Config) error { c.Tools["codex"].Profiles["work"] = &Profile{}; return nil })
	old := time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC)
	if err := UpdateUsageObservationFor(cfg, "codex", "work", func(s *ProfileState) {
		s.UsageAttemptedAt, s.UsageError = old, "API access expired"
		s.Usage = &Usage{At: old, Windows: []tool.Window{{Label: "7d", Percent: 100}}, Groups: []tool.QuotaGroup{{ID: "extra", Name: "Extra"}}, Resets: &tool.ResetBalance{Available: 2}}
	}); err != nil {
		t.Fatal(err)
	}
	if err := UpdateState("codex", "work", func(s *ProfileState) {
		s.Until, s.MarkedBy = old.Add(time.Hour), "owner"
		s.Usage = &Usage{At: old.Add(time.Minute), Windows: []tool.Window{{Label: "7d", Percent: 0}}}
	}); err != nil {
		t.Fatal(err)
	}
	s := LoadStateFor(cfg).Get("codex", "work")
	if s.Usage == nil || s.Usage.Windows[0].Percent != 0 || len(s.Usage.Groups) != 1 || s.Usage.Resets.Available != 2 || !s.Usage.At.Equal(old) || s.UsageError != "API access expired" || s.MarkedBy != "owner" || !s.Until.Equal(old.Add(time.Hour)) {
		t.Fatalf("new native window erased API detail/error or freshened old data: %+v", s)
	}
}

func TestOversizedUsageObservationKeepsPreviousCache(t *testing.T) {
	testutil.Sandbox(t)
	cfg, err := Update(func(c *Config) error { c.Tools["codex"].Profiles["work"] = &Profile{}; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := UpdateUsageObservationFor(cfg, "codex", "work", func(s *ProfileState) { s.UsageError = "previous" }); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(Home(), "usage-observations.json")
	before := testutil.Read(t, path)
	if err := UpdateUsageObservationFor(cfg, "codex", "work", func(s *ProfileState) { s.UsageError = strings.Repeat("x", maxUsageObservationBytes) }); err == nil {
		t.Fatal("oversized update was accepted")
	}
	if got := testutil.Read(t, path); got != before {
		t.Fatal("oversized update destroyed readable cache")
	}
}
