package profiles

import (
	"errors"
	"testing"
	"time"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/tool"
)

func TestAutomaticLimitYieldsToNewerQuotaForTheSameWindow(t *testing.T) {
	for _, change := range []string{"reset", "manual", "busy", "unknown-window", "other-window", "unknown-percent", "still-limited", "older", "failed", "reset-passed", "login"} {
		t.Run(change, func(t *testing.T) {
			cfg, a := setup(t)
			w := tool.Window{Label: "5h", Percent: 41, ResetsAt: now.Add(time.Hour)}
			ps := config.ProfileState{
				Until: now.Add(3 * time.Hour), Reason: "limit", Window: "5h", MarkedBy: "a session", MarkedAt: now,
				Usage: &config.Usage{At: now.Add(time.Minute), Windows: []tool.Window{w}},
			}
			switch change {
			case "manual":
				ps.MarkedBy = "you"
			case "busy":
				ps.Reason = "busy"
			case "unknown-window":
				ps.Window = ""
			case "other-window":
				ps.Window = "7d"
			case "unknown-percent":
				ps.Usage.Windows[0].PercentUnknown = true
			case "still-limited":
				ps.Usage.Windows[0].Limited = true
			case "older":
				ps.Usage.At = now
			case "failed":
				ps.UsageAttemptedAt = now.Add(2 * time.Minute)
				ps.UsageError = "unavailable"
			case "reset-passed":
				ps.Usage.Windows[0].ResetsAt = now.Add(-time.Minute)
			case "login":
				ps.NeedsLogin = true
			}
			if err := config.UpdateStateFor(cfg, a.ID(), "a", func(s *config.ProfileState) { ps.Generation = s.Generation; *s = ps }); err != nil {
				t.Fatal(err)
			}
			pick := Choose(t.Context(), cfg, config.LoadStateFor(cfg), a, "a", nil)
			if got, want := pick.Name == "a", change == "reset"; got != want {
				t.Fatalf("%s: chose %q, skipped %+v", change, pick.Name, pick.Skipped)
			}
			if state := config.LoadState().Get(a.ID(), "a"); !state.Until.Equal(ps.Until) || state.NeedsLogin != ps.NeedsLogin {
				t.Fatal("reading quotas rewrote routing marks")
			}
		})
	}
}

func TestSameAccountDoesNotReintroduceAContradictedLimit(t *testing.T) {
	cfg, a := setup(t)
	for _, name := range []string{"a", "b"} {
		_ = config.UpdateStateFor(cfg, a.ID(), name, func(s *config.ProfileState) {
			s.Account = &config.Account{Email: "same@example.test", Tenant: "workspace"}
		})
	}
	_ = MarkLimited(cfg, a.ID(), "a", Limit{Window: "5h", By: "a failed run", For: time.Hour})
	_ = config.UpdateStateFor(cfg, a.ID(), "a", func(s *config.ProfileState) {
		s.Usage = &config.Usage{At: now.Add(time.Minute), Windows: []tool.Window{{Label: "5h", Percent: 3}}}
	})
	if ev := Evaluate(t.Context(), cfg, config.LoadStateFor(cfg), a, "b"); !ev.Usable {
		t.Fatalf("stale sibling mark: %+v", ev)
	}
}

func TestSuccessfulWorkerClearsOnlyTheMarksItStartedWith(t *testing.T) {
	for _, change := range []string{"none", "new-failure", "manual-hold", "identity", "revision", "registration"} {
		t.Run(change, func(t *testing.T) {
			cfg, a := setup(t)
			_ = MarkNeedsLogin(a.ID(), "a", "a session", "expired", cfg)
			_ = MarkLimited(cfg, a.ID(), "a", Limit{By: "a session", For: time.Hour})
			before := config.LoadState().Get(a.ID(), "a")
			if change == "registration" {
				_, err := config.Update(func(c *config.Config) error { c.Tools[a.ID()].Profiles["a"].Generation = "replacement"; return nil })
				if err != nil {
					t.Fatal(err)
				}
			} else {
				_ = config.UpdateStateFor(cfg, a.ID(), "a", func(s *config.ProfileState) {
					s.LastUsedAt = now.Add(time.Minute)
					s.Usage = &config.Usage{At: now, Windows: []tool.Window{{Label: "5h", Percent: 41}}}
					switch change {
					case "new-failure":
						s.MarkedAt = now.Add(time.Minute)
					case "manual-hold":
						s.MarkedBy = "you"
					case "identity":
						s.Account = &config.Account{Email: "new@example.test"}
					case "revision":
						s.AccountRevision = "new-login"
					}
				})
			}
			err := ClearAfterSuccess(cfg, a.ID(), "a", before)
			if change == "registration" {
				if !errors.Is(err, config.ErrProfileChanged) {
					t.Fatalf("old registration accepted: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			ps := config.LoadState().Get(a.ID(), "a")
			if change == "none" {
				if ps.NeedsLogin || !ps.Until.IsZero() || ps.MarkedBy != "" {
					t.Fatalf("stale marks survived success: %+v", ps)
				}
			} else if !ps.NeedsLogin || ps.Until.IsZero() {
				t.Fatalf("success cleared changed marks: %+v", ps)
			}
			if ps.Usage == nil || ps.Usage.Windows[0].Percent != 41 || !ps.LastUsedAt.Equal(now.Add(time.Minute)) {
				t.Fatal("success removed usage or last-used metadata")
			}
		})
	}
}
