package config

import (
	"os"
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

func TestErrorProvenanceAndNativeNoUsageOverlay(t *testing.T) {
	for _, c := range []struct {
		name, source, nativeError string
		nativeQuota               bool
		wantError                 string
	}{
		{"local window cannot settle API failure", "statusline", "", true, "API failed"},
		{"successful native quota settles older API failure", "live", "", true, ""},
		{"new native failure keeps its timestamp", "live", "new failure", false, "new failure"},
		{"native no quota retains older API failure", "live", "", false, "API failed"},
	} {
		t.Run(c.name, func(t *testing.T) {
			testutil.Sandbox(t)
			cfg, _ := Update(func(c *Config) error { c.Tools["codex"].Profiles["work"] = &Profile{}; return nil })
			old := time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC)
			newer := old.Add(time.Minute)
			if err := UpdateUsageObservationFor(cfg, "codex", "work", func(s *ProfileState) {
				s.Usage = &Usage{At: old, Source: "live", Windows: []tool.Window{{Label: "7d", Percent: 80}}}
				s.UsageError = "API failed"
				s.UsageAttemptedAt = old
			}); err != nil {
				t.Fatal(err)
			}
			if err := UpdateState("codex", "work", func(s *ProfileState) {
				s.UsageAttemptedAt = newer
				s.UsageError = c.nativeError
				if c.nativeQuota {
					s.Usage = &Usage{At: newer, Source: c.source, Windows: []tool.Window{{Label: "7d", Percent: 1}}}
				}
			}); err != nil {
				t.Fatal(err)
			}
			ps := LoadStateFor(cfg).Get("codex", "work")
			if ps.Usage == nil || ps.UsageError != c.wantError {
				t.Fatalf("wrong merge: %+v", ps)
			}
			if c.nativeError != "" && !ps.UsageAttemptedAt.Equal(newer) {
				t.Fatal("new failure timestamp rewound")
			}
			if !c.nativeQuota && !ps.Usage.At.Equal(old) {
				t.Fatal("no-quota attempt freshened old measurement")
			}
		})
	}
}

func TestEarlierUnboundCacheIsKeptForFreshObservation(t *testing.T) {
	testutil.Sandbox(t)
	cfg, _ := Update(func(c *Config) error { c.Tools["codex"].Profiles["work"] = &Profile{}; return nil })
	path := filepath.Join(Home(), "usage-observations.json")
	old := `{"schemaVersion":"1.0.0","profiles":{"codex":{"work":{"attemptedAt":"2026-11-16T00:00:00Z","usage":{"windows":[{"label":"7d","pct":99}],"at":"2026-11-16T00:00:00Z"}}}}}`
	testutil.Write(t, path, old)
	if got := LoadStateFor(cfg).Get("codex", "work"); got.Usage != nil {
		t.Fatal("unbound cache relabeled a native identity")
	}
	if len(UsageObservationNotes(cfg)) == 0 || testutil.Read(t, path) != old {
		t.Fatal("earlier cache not preserved/diagnosed")
	}
}

func TestDoctorExplainsLegacyAccountBindingMismatch(t *testing.T) {
	testutil.Sandbox(t)
	cfg, _ := Update(func(c *Config) error { c.Tools["codex"].Profiles["work"] = &Profile{}; return nil })
	_ = UpdateState("codex", "work", func(s *ProfileState) { s.Account = &Account{Email: "old@example.test"} })
	if err := UpdateUsageObservationFor(cfg, "codex", "work", func(s *ProfileState) { s.Usage = &Usage{Windows: []tool.Window{{Label: "7d", Percent: 99}}} }); err != nil {
		t.Fatal(err)
	}
	// Old writers do not retain accountRevision, but their identity still fences UI.
	_ = UpdateState("codex", "work", func(s *ProfileState) { s.Account = &Account{Email: "new@example.test"}; s.AccountRevision = "" })
	if LoadStateFor(cfg).Get("codex", "work").Usage != nil {
		t.Fatal("mismatched cache was visible")
	}
	if notes := strings.Join(UsageObservationNotes(cfg), " "); !strings.Contains(notes, "no longer match") {
		t.Fatalf("missing identity diagnostic: %q", notes)
	}
}

func TestLostIdentityFieldsRequireFreshObservation(t *testing.T) {
	for _, field := range []string{"tenant", "email"} {
		t.Run(field, func(t *testing.T) {
			testutil.Sandbox(t)
			cfg, _ := Update(func(c *Config) error { c.Tools["codex"].Profiles["work"] = &Profile{}; return nil })
			_ = UpdateState("codex", "work", func(s *ProfileState) { s.Account = &Account{Email: "same@example.test", Tenant: "workspace"} })
			if err := UpdateUsageObservationFor(cfg, "codex", "work", func(s *ProfileState) { s.Usage = &Usage{Windows: []tool.Window{{Label: "7d", Percent: 99}}} }); err != nil {
				t.Fatal(err)
			}
			_ = UpdateState("codex", "work", func(s *ProfileState) {
				if field == "tenant" {
					s.Account.Tenant = ""
				} else {
					s.Account.Email = ""
				}
			})
			if LoadStateFor(cfg).Get("codex", "work").Usage != nil {
				t.Fatal("old quota borrowed unknown identity")
			}
			if err := UpdateUsageObservationFor(cfg, "codex", "work", func(s *ProfileState) { s.Usage = &Usage{Windows: []tool.Window{{Label: "7d", Percent: 2}}} }); err != nil {
				t.Fatal(err)
			}
			if ps := LoadStateFor(cfg).Get("codex", "work"); ps.Usage == nil || ps.Usage.Windows[0].Percent != 2 {
				t.Fatal("fresh observation could not bind corrected identity")
			}
		})
	}
}

func TestUsageCacheRefusesManagedLink(t *testing.T) {
	testutil.Sandbox(t)
	cfg, _ := Update(func(c *Config) error { c.Tools["codex"].Profiles["work"] = &Profile{}; return nil })
	target := filepath.Join(t.TempDir(), "owner-file")
	testutil.Write(t, target, "keep")
	path := filepath.Join(Home(), "usage-observations.json")
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := LoadStateView(cfg); err == nil {
		t.Fatal("cache followed an external link")
	}
	if err := UpdateUsageObservationFor(cfg, "codex", "work", func(*ProfileState) { t.Fatal("external cache reached mutation") }); err == nil {
		t.Fatal("cache link was writable")
	}
	if testutil.Read(t, target) != "keep" {
		t.Fatal("cache changed external owner file")
	}
}

func TestOverlayDoesNotMixTwoNativeStateSnapshots(t *testing.T) {
	testutil.Sandbox(t)
	cfg, _ := Update(func(c *Config) error { c.Tools["codex"].Profiles["work"] = &Profile{}; return nil })
	_ = UpdateState("codex", "work", func(s *ProfileState) {
		s.Account = &Account{Email: "first@example.test"}
		s.AccountRevision = "first"
		s.Usage = &Usage{Windows: []tool.Window{{Label: "7d", Percent: 99}}}
	})
	raw := LoadState()
	copy := *raw.Get("codex", "work")
	view := State{"codex": {"work": &copy}}
	// Disk advances after the view's native snapshot was read.
	_ = UpdateState("codex", "work", func(s *ProfileState) {
		s.Account = &Account{Email: "second@example.test"}
		s.AccountRevision = "second"
		s.Usage = nil
	})
	if err := UpdateUsageObservationFor(cfg, "codex", "work", func(s *ProfileState) {
		s.Usage = &Usage{Windows: []tool.Window{{Label: "7d", Percent: 2}}, Groups: []tool.QuotaGroup{{ID: "second"}}}
	}); err != nil {
		t.Fatal(err)
	}
	if err := overlayUsageObservations(cfg, view, raw); err != nil {
		t.Fatal(err)
	}
	got := view.Get("codex", "work")
	if got.Account.Email != "first@example.test" || got.Usage.Windows[0].Percent != 99 || len(got.Usage.Groups) != 0 {
		t.Fatalf("cache mixed two native identities into one snapshot: %+v", got)
	}
	if fresh := LoadStateFor(cfg).Get("codex", "work"); fresh.Account.Email != "second@example.test" || fresh.Usage.Windows[0].Percent != 2 {
		t.Fatalf("fresh snapshot lost current identity: %+v", fresh)
	}
}
