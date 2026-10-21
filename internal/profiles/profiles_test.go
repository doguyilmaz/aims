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
	if err := MarkLimited(cfg, "codex", "b", Limit{By: "a test", Detail: "You've hit your limit\nmore"}); err != nil {
		t.Fatal(err)
	}
	_ = MarkNeedsLogin("codex", "a", "a test", "")
	p := Choose(t.Context(), cfg, config.LoadState(), a, "b", nil)
	if p.Name != "c" || len(p.Skipped) != 2 || p.Skipped[0].Name != "b" {
		t.Fatalf("pick: %+v", p)
	}
	if !strings.Contains(strings.Join(p.Skipped[0].Reasons, ","), "limit for 5h") || p.Skipped[1].Reasons[0] != "login expired" {
		t.Fatalf("reasons: %+v", p.Skipped)
	}
	if ps := p.Skipped[0].State; ps.MarkedBy != "a test" || ps.Detail != "You've hit your limit" || ps.MarkedAt.IsZero() {
		t.Fatalf("mark not recorded: %+v", ps)
	}
	if p := Choose(t.Context(), cfg, config.LoadState(), a, "", []string{"c"}); p.Name != "" {
		t.Fatalf("excluded the only usable one: %+v", p)
	}
	_ = Clear("codex", "b")
	if p := Choose(t.Context(), cfg, config.LoadState(), a, "b", nil); p.Name != "b" {
		t.Fatalf("after clear: %+v", p)
	}
	if ps := config.LoadState().Get("codex", "b"); ps.MarkedBy != "" || ps.Detail != "" {
		t.Fatalf("clear kept the mark's origin: %+v", ps)
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

// When nothing is usable, Choose names the profile that works again first,
// counting only limits with a known end.
func TestSoonest(t *testing.T) {
	cfg, a := setup(t)
	_ = RecordUsage("codex", "a", []tool.Window{
		{Label: "5h", Percent: 100, ResetsAt: now.Add(200 * time.Minute)},
		{Label: "7d", Percent: 44, ResetsAt: now.Add(48 * time.Hour)},
	}, "test")
	_ = MarkLimited(cfg, "codex", "b", Limit{ResetsAt: now.Add(time.Hour), Window: "5h"})
	_ = MarkNeedsLogin("codex", "c", "a test", "")

	ev := Evaluate(t.Context(), cfg, config.LoadState(), a, "a")
	if ev.Usable || !slices.Equal(ev.Reasons, []string{"5h at 100% for 3h20m"}) || !ev.ReadyAt.Equal(now.Add(200*time.Minute)) {
		t.Fatalf("five-hour usage: %+v", ev)
	}
	p := Choose(t.Context(), cfg, config.LoadState(), a, "a", nil)
	if p.Name != "" || p.Soonest != "b" || !p.ReadyAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("soonest: %+v", p)
	}
	// Both windows full: ready when the later one resets.
	_ = RecordUsage("codex", "a", []tool.Window{
		{Label: "5h", Percent: 100, ResetsAt: now.Add(30 * time.Minute)},
		{Label: "7d", Percent: 100, ResetsAt: now.Add(48 * time.Hour)},
	}, "test")
	if ev := Evaluate(t.Context(), cfg, config.LoadState(), a, "a"); !ev.ReadyAt.Equal(now.Add(48 * time.Hour)) {
		t.Fatalf("both windows: %+v", ev)
	}
	// A dead login has no known end.
	if ev := Evaluate(t.Context(), cfg, config.LoadState(), a, "c"); !ev.ReadyAt.IsZero() {
		t.Fatalf("login: %+v", ev)
	}
}

func TestLimitWindow(t *testing.T) {
	cfg, _ := setup(t)
	short := tool.Window{Label: "5h", Percent: 100, ResetsAt: now.Add(time.Hour)}
	weekly := tool.Window{Label: "7d", Percent: 97, ResetsAt: now.Add(48 * time.Hour)}
	cases := []struct {
		detail  string
		windows []tool.Window
		want    string
	}{
		{"You've hit your session limit · resets 5pm", []tool.Window{weekly}, "5h"},
		{"You've hit your weekly limit", nil, "7d"},
		{"usage limit reached", []tool.Window{short}, "5h"},
		{"usage limit reached", []tool.Window{short, weekly}, "7d"},
		{"usage limit reached", []tool.Window{{Label: "5h", Percent: 100, ResetsAt: now.Add(-time.Minute)}}, ""},
		{"usage limit reached", nil, ""},
	}
	for _, c := range cases {
		if got := LimitWindow(cfg, c.detail, c.windows); got != c.want {
			t.Errorf("LimitWindow(%q, %v) = %q, want %q", c.detail, c.windows, got, c.want)
		}
	}
}

func TestMarkLimitedUsesKnownReset(t *testing.T) {
	cfg, _ := setup(t)
	_ = RecordUsage("codex", "a", []tool.Window{{Label: "5h", Percent: 100, ResetsAt: now.Add(70 * time.Minute)}}, "test")
	_ = MarkLimited(cfg, "codex", "a", Limit{})
	if until := config.LoadState().Get("codex", "a").Until; !until.Equal(now.Add(70 * time.Minute)) {
		t.Fatalf("until = %v", until)
	}
	// An existing cooldown is not stretched by a second report.
	_ = MarkLimited(cfg, "codex", "a", Limit{})
	if until := config.LoadState().Get("codex", "a").Until; !until.Equal(now.Add(70 * time.Minute)) {
		t.Fatalf("stretched to %v", until)
	}
	// A weekly limit replaces a five-hour mark, until the weekly reset.
	_ = RecordUsage("codex", "c", []tool.Window{
		{Label: "5h", Percent: 100, ResetsAt: now.Add(time.Hour)},
		{Label: "7d", Percent: 100, ResetsAt: now.Add(48 * time.Hour)},
	}, "test")
	_ = MarkLimited(cfg, "codex", "c", Limit{Window: "5h"})
	_ = MarkLimited(cfg, "codex", "c", Limit{Window: "7d"})
	if ps := config.LoadState().Get("codex", "c"); ps.Window != "7d" || !ps.Until.Equal(now.Add(48*time.Hour)) {
		t.Fatalf("weekly after five-hour: %+v", ps)
	}
	_ = MarkLimited(cfg, "codex", "c", Limit{Window: "5h"})
	if ps := config.LoadState().Get("codex", "c"); ps.Window != "7d" {
		t.Fatalf("five-hour replaced the weekly mark: %+v", ps)
	}
	// A limit outlasts a short rate limit.
	_ = MarkLimited(cfg, "codex", "b", Limit{Reason: "busy", For: 2 * time.Minute})
	_ = MarkLimited(cfg, "codex", "b", Limit{})
	if ps := config.LoadState().Get("codex", "b"); ps.Reason != "limit" || !ps.Until.Equal(now.Add(5*time.Hour)) {
		t.Fatalf("limit after busy: %+v", ps)
	}
	_ = Clear("codex", "b")
	_ = Clear("codex", "b")
	// An explicit duration wins.
	_ = MarkLimited(cfg, "codex", "b", Limit{For: 15 * time.Minute})
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

func TestBusyMarkIsShort(t *testing.T) {
	cfg, _ := setup(t)
	_ = MarkLimited(cfg, "codex", "a", Limit{Reason: "busy", For: 2 * time.Minute, By: "a failed run"})
	ps := config.LoadState().Get("codex", "a")
	if ps.Reason != "busy" || !ps.Until.Equal(now.Add(2*time.Minute)) {
		t.Fatalf("busy mark: %+v", ps)
	}
}

// Two logins of one account share its plan: a limit on one keeps the other
// out too, but a rate limit or a dead login is the login's own.
func TestSameAccount(t *testing.T) {
	cfg, a := setup(t)
	account := func(n, email, tenant string) {
		_ = config.UpdateState("codex", n, func(ps *config.ProfileState) {
			ps.Account = &config.Account{Email: email, Tenant: tenant}
		})
	}
	account("a", "me@corp.com", "ws-1")
	account("b", "ME@corp.com", "ws-1")
	account("c", "me@corp.com", "ws-2") // a personal plan and a team seat
	_ = MarkLimited(cfg, "codex", "a", Limit{ResetsAt: now.Add(2 * time.Hour)})
	p := Choose(t.Context(), cfg, config.LoadState(), a, "a", nil)
	if p.Name != "c" || len(p.Skipped) != 2 || !slices.Contains(p.Skipped[1].Reasons, "same account as a, limited for 2h") || !p.Skipped[1].ReadyAt.Equal(now.Add(2*time.Hour)) {
		t.Fatalf("b shares a's limit: %+v", p)
	}
	_ = Clear("codex", "a")
	_ = RecordUsage("codex", "a", []tool.Window{{Label: "5h", Percent: 99, ResetsAt: now.Add(time.Hour)}}, "test")
	if ev := Evaluate(t.Context(), cfg, config.LoadState(), a, "b"); ev.Usable || !slices.Contains(ev.Reasons, "same account as a, 5h at 99% for 1h") {
		t.Fatalf("b shares a's usage: %+v", ev)
	}
	_ = RecordUsage("codex", "a", nil, "test")
	_ = MarkLimited(cfg, "codex", "a", Limit{Reason: "busy", For: 2 * time.Minute})
	_ = MarkNeedsLogin("codex", "a", "a test", "")
	if ev := Evaluate(t.Context(), cfg, config.LoadState(), a, "b"); !ev.Usable {
		t.Fatalf("a's rate limit and login are its own: %+v", ev)
	}
}

// A login whose tenant aims cannot tell stops counting as the account it
// was stored as.
func TestSameAccountForgetsTenant(t *testing.T) {
	cfg, a := setup(t)
	for _, n := range []string{"a", "b"} {
		_ = config.UpdateState("codex", n, func(ps *config.ProfileState) {
			ps.Account = &config.Account{Email: "me@corp.com", Tenant: "ws-1"}
		})
	}
	_ = RecordUsage("codex", "b", []tool.Window{{Label: "7d", Percent: 99, ResetsAt: now.Add(time.Hour)}}, "test")
	if ev := Evaluate(t.Context(), cfg, config.LoadState(), a, "a"); ev.Usable {
		t.Fatalf("a shares b's usage: %+v", ev)
	}
	// b's login now names the e-mail but no workspace.
	testutil.Write(t, filepath.Join(config.Home(), "profiles", "codex", "b", "auth.json"), `{"tokens":{"id_token":"x.eyJlbWFpbCI6Im1lQGNvcnAuY29tIn0.sig","refresh_token":"r"}}`)
	if ev := Evaluate(t.Context(), cfg, config.LoadState(), a, "a"); !ev.Usable {
		t.Fatalf("b's tenant is unknown now: %+v", ev)
	}
}

func TestNames(t *testing.T) {
	for _, c := range []struct {
		in   []string
		want string
	}{
		{[]string{"work"}, "work"},
		{[]string{"a", "b", "c"}, "a, b, c"},
		{[]string{"a", "b", "c", "d", "e"}, "a, b +3 more"},
	} {
		if got := Names(c.in, ", "); got != c.want {
			t.Errorf("Names(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
