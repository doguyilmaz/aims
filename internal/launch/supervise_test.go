package launch

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/profiles"
	"github.com/doguyilmaz/aims/internal/testutil"
	"github.com/doguyilmaz/aims/internal/tool"
	"github.com/doguyilmaz/aims/internal/tools"
)

func TestMain(m *testing.M) { testutil.Main(m) }

var now = time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

// setup makes codex profiles a, b and c, all logged in, in that order.
func setup(t *testing.T) Options {
	t.Helper()
	home := testutil.Sandbox(t)
	old := config.Now
	config.Now = func() time.Time { return now }
	t.Cleanup(func() { config.Now = old })
	if _, err := config.Update(func(c *config.Config) error {
		for _, n := range []string{"a", "b", "c"} {
			c.Tools["codex"].Profiles[n] = &config.Profile{}
			c.Tools["codex"].Order = append(c.Tools["codex"].Order, n)
		}
		c.Tools["codex"].Active = "a"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"a", "b", "c"} {
		testutil.Write(t, filepath.Join(home, ".aims", "profiles", "codex", n, "auth.json"), `{"tokens":{"refresh_token":"r"}}`)
	}
	return Options{Tool: tools.Get("codex")}
}

// A profile's own copy of a database does not keep a conversation out:
// Codex reads the shared one (CODEX_SQLITE_HOME).
func TestOnFailureIgnoresPrivateDatabase(t *testing.T) {
	o := setup(t)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	testutil.Write(t, filepath.Join(cfg.Dir("codex", "b"), "thread_history_1.sqlite"), "private history")
	if next, _, _, _ := onFailure(t.Context(), o, "a", limited); next != "b" || !validMove("codex", "a", "b") {
		t.Fatalf("a private index kept the conversation out: %q", next)
	}
	// A database with data of its own still does: Codex keeps using it.
	testutil.Write(t, filepath.Join(cfg.EnsureHub("codex"), "goals_1.sqlite"), "shared goals")
	testutil.Write(t, filepath.Join(cfg.Dir("codex", "b"), "goals_1.sqlite"), "private goals")
	if validMove("codex", "a", "b") {
		t.Fatal("moved next to a database of its own")
	}
}

func TestOnFailureSkipsConflictingHistory(t *testing.T) {
	for _, entry := range []string{"thread-writer-locks", "sessions"} {
		t.Run(entry, func(t *testing.T) {
			o := setup(t)
			cfg, err := config.Load()
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(cfg.Dir("codex", "b"), entry)
			if err := os.MkdirAll(path, 0o755); err != nil {
				t.Fatal(err)
			}
			if next, _, _, _ := onFailure(t.Context(), o, "a", limited); next != "c" {
				t.Fatalf("moved into conflicting %s: %q", entry, next)
			}
			if validMove("codex", "a", "b") {
				t.Fatal("explicit move accepted conflicting history")
			}
			if next, _, _, _ := onFailure(t.Context(), o, "b", limited); next != "" {
				t.Fatalf("moved out of private %s: %q", entry, next)
			}
		})
	}
}

func limit(t *testing.T, name string, d time.Duration) {
	t.Helper()
	cfg, _ := config.Load()
	if err := profiles.MarkLimited(cfg, "codex", name, profiles.Limit{ResetsAt: now.Add(d), Window: "7d"}); err != nil {
		t.Fatal(err)
	}
}

var limited = tool.SessionEvent{Session: "s", Failure: tool.FailLimit, Text: "You've hit your usage limit"}

func TestOnFailure(t *testing.T) {
	o := setup(t)
	next, prompt, why, wake := onFailure(t.Context(), o, "a", limited)
	if next != "b" || prompt != continuePrompt || why != "a hit its usage limit; continuing this conversation on b" || wake != nil {
		t.Fatalf("to the next: %q %q %q %+v", next, prompt, why, wake)
	}
	if ps := config.LoadState().Get("codex", "a"); ps.MarkedBy != "a session" || !ps.Until.After(now) {
		t.Fatalf("not marked: %+v", ps)
	}

	// Every account limited: wait for the one that resets first, c here.
	limit(t, "b", 3*time.Hour)
	limit(t, "c", time.Hour)
	next, _, _, wake = onFailure(t.Context(), o, "b", limited)
	if next != "" || wake == nil || wake.Soonest != "c" || !wake.ReadyAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("wait: %q %+v", next, wake)
	}

}

func TestOnStop(t *testing.T) {
	o := setup(t)
	if next, _ := onStop(t.Context(), o, "a", ""); next != "" {
		t.Fatalf("stays on a usable first account: %q", next)
	}
	limit(t, "a", time.Hour)
	if next, _ := onStop(t.Context(), o, "b", ""); next != "" {
		t.Fatalf("stays on the stand-in while a is limited: %q", next)
	}
	if next, why := onStop(t.Context(), o, "a", ""); next != "b" || why != "a is about to run out; this conversation moves to b" {
		t.Fatalf("moves on before the limit: %q %q", next, why)
	}
	_ = profiles.Clear("codex", "a")
	if next, why := onStop(t.Context(), o, "b", ""); next != "a" || why != "a works again; this conversation goes back to it" {
		t.Fatalf("back: %q %q", next, why)
	}
}

// A conversation only moves to profiles that share its history.
func TestOnFailureKeepsHistory(t *testing.T) {
	o := setup(t)
	if _, err := config.Update(func(c *config.Config) error {
		c.Tools["codex"].Profiles["b"].Share = config.ShareSettings
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if next, _, _, _ := onFailure(t.Context(), o, "a", limited); next != "c" {
		t.Fatalf("skips b, whose history is its own: %q", next)
	}
	if next, _, _, _ := onFailure(t.Context(), o, "b", limited); next != "" {
		t.Fatalf("b's conversation is nowhere else: %q", next)
	}
}

// A rate limit that comes back soon after the last rests the account longer.
func TestBusyBackoff(t *testing.T) {
	o := setup(t)
	busy := tool.SessionEvent{Session: "s", Failure: tool.FailBusy}
	onFailure(t.Context(), o, "a", busy)
	if ps := config.LoadState().Get("codex", "a"); !ps.Until.Equal(now.Add(busyFor)) {
		t.Fatalf("first: %v", ps.Until)
	}
	_ = profiles.Clear("codex", "a")
	_ = config.UpdateState("codex", "a", func(ps *config.ProfileState) { ps.Reason, ps.MarkedAt = "busy", now.Add(-5*time.Minute) })
	onFailure(t.Context(), o, "a", busy)
	if ps := config.LoadState().Get("codex", "a"); !ps.Until.Equal(now.Add(busyAgain)) {
		t.Fatalf("again: %v", ps.Until)
	}
}
