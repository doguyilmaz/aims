package ops

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/profiles"
	"github.com/doguyilmaz/aims/internal/testutil"
	"github.com/doguyilmaz/aims/internal/tool"
	"github.com/doguyilmaz/aims/internal/tools"
)

type delayedProbe struct {
	tool.Adapter
	started, release chan struct{}
}

type generationAccount struct{ tool.Adapter }

func (generationAccount) Account(context.Context, tool.Home) tool.Account {
	return tool.Account{Email: "new@example.test", Plan: "plus"}
}

func TestSuccessfulLoginInvalidatesOlderObservations(t *testing.T) {
	testutil.Sandbox(t)
	a := generationAccount{tools.Get("codex")}
	if _, err := AddProfile(a, "work", AddOptions{Share: config.ShareNone}); err != nil {
		t.Fatal(err)
	}
	before, _ := config.Load()
	if _, err := RecordLogin(t.Context(), a, "work"); err != nil {
		t.Fatal(err)
	}
	after, _ := config.Load()
	if before.Tools["codex"].Profiles["work"].Generation == after.Tools["codex"].Profiles["work"].Generation {
		t.Fatal("login retained old observation identity")
	}
	if err := profiles.MarkNeedsLogin("codex", "work", "old process", "expired", before); err == nil {
		t.Fatal("pre-login process marked new login expired")
	}
	state := config.LoadStateFor(after).Get("codex", "work")
	if state.NeedsLogin || state.Account == nil || state.Account.Email != "new@example.test" {
		t.Fatalf("login state lost: %+v", state)
	}
}

func (a delayedProbe) Probe(context.Context, tool.Home) tool.Probe {
	close(a.started)
	<-a.release
	return tool.Probe{Status: tool.ProbeLimit, Email: "old@example.test", Plan: "old", UsageReported: true, Windows: []tool.Window{{Label: "7d", Percent: 100, ResetsAt: time.Now().Add(time.Hour)}}}
}

func TestDelayedProbeCannotOverwriteRecreatedOrPendingProfile(t *testing.T) {
	for _, passive := range []bool{false, true} {
		for _, pending := range []bool{false, true} {
			t.Run(map[bool]string{false: "live", true: "passive"}[passive]+map[bool]string{false: "/recreated", true: "/pending"}[pending], func(t *testing.T) {
				testutil.Sandbox(t)
				a := tools.Get("codex")
				if _, err := AddProfile(a, "work", AddOptions{Share: config.ShareNone}); err != nil {
					t.Fatal(err)
				}
				before, _ := config.Load()
				slow := delayedProbe{a, make(chan struct{}), make(chan struct{})}
				done := make(chan struct{})
				go func() {
					defer close(done)
					if passive {
						profiles.ReadUsage(t.Context(), before, slow, "work")
					} else {
						profiles.Live(t.Context(), before, slow, "work")
					}
				}()
				<-slow.started
				defer func() {
					select {
					case <-slow.release:
					default:
						close(slow.release)
					}
					<-done
				}()
				if pending {
					if _, err := config.Update(func(c *config.Config) error {
						c.Tools["codex"].Profiles["work"].Purge = &config.PurgeState{}
						return nil
					}); err != nil {
						t.Fatal(err)
					}
				} else {
					if _, err := RemoveProfile(t.Context(), a, "work", false, false); err != nil {
						t.Fatal(err)
					}
					if _, err := AddProfile(a, "work", AddOptions{Share: config.ShareNone}); err != nil {
						t.Fatal(err)
					}
					current, _ := config.Load()
					if current.Tools["codex"].Profiles["work"].Generation == before.Tools["codex"].Profiles["work"].Generation {
						t.Fatal("recreated registration retained its generation")
					}
				}
				if err := config.UpdateState("codex", "work", func(s *config.ProfileState) {
					s.Account = &config.Account{Email: "new@example.test"}
					s.UsageError = "current"
				}); err != nil {
					t.Fatal(err)
				}
				want, _ := json.Marshal(config.LoadState())
				close(slow.release)
				<-done
				got, _ := json.Marshal(config.LoadState())
				if string(got) != string(want) {
					t.Fatalf("old result changed current state: %s -> %s", want, got)
				}
			})
		}
	}
}

func TestLogoutRejectsObservationsStartedBeforeNativeLogout(t *testing.T) {
	testutil.Sandbox(t)
	before, _ := purgeFixture(t)
	a := logoutAdapter{tools.Get("codex")}
	if err := Logout(t.Context(), a, "work"); err != nil {
		t.Fatal(err)
	}
	after, _ := config.Load()
	if before.Tools["codex"].Profiles["work"].Generation == after.Tools["codex"].Profiles["work"].Generation {
		t.Fatal("logout retained old observation identity")
	}
	if err := profiles.RecordAccount("codex", "work", "old@example.test", "plus", before); err == nil {
		t.Fatal("old observation restored logged-out identity")
	}
	if state := config.LoadStateFor(after).Get("codex", "work"); state.Account != nil || state.Usage != nil {
		t.Fatalf("old account survived logout: %+v", state)
	}
}
