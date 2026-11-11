package bridge

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/profiles"
	"github.com/doguyilmaz/aims/internal/testutil"
	"github.com/doguyilmaz/aims/internal/tool"
	"github.com/doguyilmaz/aims/internal/tools"
)

func TestMain(m *testing.M) { testutil.Main(m) }

func TestExportsRefusePendingPrivateHistoryWithoutRepair(t *testing.T) {
	cfg := setup(t)
	a := tools.Get("codex")
	cfg.Tools[a.ID()].Profiles["other"].Share = config.ShareSettings
	source := filepath.Join(cfg.EnsureHub(a.ID()), "history.jsonl")
	target := filepath.Join(cfg.Dir(a.ID(), "other"), "history.jsonl")
	testutil.Write(t, source, "shared prompt\n")
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(source, target); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Environment(cfg, nil, nil); err == nil {
		t.Fatal("application environment accepted stale shared history")
	}
	if _, _, err := AgentEnvironment(cfg, a, "other", nil); err == nil {
		t.Fatal("launcher environment accepted stale shared history")
	}
	if _, err := PlanT3(cfg, filepath.Join(t.TempDir(), "settings.json")); err == nil {
		t.Fatal("T3 export accepted stale shared history")
	}
	si, _ := os.Stat(source)
	ti, _ := os.Stat(target)
	if !os.SameFile(si, ti) {
		t.Fatal("read-only exports repaired the history link")
	}
}

func setup(t *testing.T) *config.Config {
	t.Helper()
	testutil.Sandbox(t)
	cfg, err := config.Update(func(c *config.Config) error {
		for _, a := range tools.All() {
			c.EnsureHub(a.ID())
			t := c.Tools[a.ID()]
			t.Profiles["default"] = &config.Profile{Existing: true}
			t.Profiles["other"] = &config.Profile{}
			t.Order, t.Active = []string{"other", "default"}, "other"
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestApplicationEnvironment(t *testing.T) {
	cfg := setup(t)
	claude := cfg.Tools["claude"].Profiles["other"]
	claude.Env = map[string]string{"ANTHROPIC_API_KEY": "profile-key"}
	// Fresh application launches must not accidentally report turns to the
	// supervisor of the terminal they were launched from.
	base := []string{"PATH=/bin", "CODEX_HOME=/t3/shadow", "ANTHROPIC_API_KEY=shell-key", "CODEX_API_KEY=shell-key", "CLAUDECODE=1", "CODEX_THREAD_ID=parent-thread",
		profiles.SessionVar + "=codex:parent", profiles.RunVar + "=/parent/run", profiles.ModeVar + "=watched"}
	env, selected, err := Environment(cfg, nil, base)
	if err != nil {
		t.Fatal(err)
	}
	m := envMap(env)
	if m["CLAUDE_CONFIG_DIR"] != cfg.Dir("claude", "other") || m["CODEX_HOME"] != cfg.Dir("codex", "other") || m["CODEX_SQLITE_HOME"] != cfg.EnsureHub("codex") {
		t.Fatalf("application homes: %v", m)
	}
	if m["ANTHROPIC_API_KEY"] != "profile-key" || m["CODEX_API_KEY"] != "" || m["PATH"] != "/bin" || m["AIMS_CODEX_PROFILE"] != "other" {
		t.Fatalf("account overrides: %v", m)
	}
	for _, k := range []string{profiles.SessionVar, profiles.RunVar, profiles.ModeVar, "CLAUDECODE", "CODEX_THREAD_ID"} {
		if _, ok := m[k]; ok {
			t.Errorf("inherited session marker %s", k)
		}
	}
	if len(selected) != 2 || selected[0].Profile != "other" || selected[1].Profile != "other" {
		t.Fatalf("selection: %v", selected)
	}
	if !slices.Contains(base, "CODEX_HOME=/t3/shadow") {
		t.Fatal("modified caller environment")
	}
	// A default Claude login needs the variable absent, not ~/.claude spelled
	// out: the latter is a different keychain entry and .claude.json.
	env, _, err = Environment(cfg, map[tool.ID]string{"claude": "default", "codex": "default"}, base)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := envMap(env)["CLAUDE_CONFIG_DIR"]; exists {
		t.Fatal("adopted login was changed to an explicit home")
	}
}

func TestEnvironmentRejectsUnknownProfileBeforeLaunching(t *testing.T) {
	cfg := setup(t)
	for _, pins := range []map[tool.ID]string{{"codex": "missing"}, {"unknown": "other"}} {
		if _, _, err := Environment(cfg, pins, nil); err == nil {
			t.Fatalf("accepted %v", pins)
		}
	}
}

func TestEnvironmentRejectsCrossProviderLoginOverrides(t *testing.T) {
	cfg := setup(t)
	cfg.Tools["codex"].Profiles["other"].Env = map[string]string{"CLAUDE_CONFIG_DIR": "/wrong-login"}
	if _, _, err := Environment(cfg, nil, nil); err == nil || !strings.Contains(err.Error(), "cross-provider") {
		t.Fatalf("misrouted selected Claude login: %v", err)
	}
}

func TestEnvironmentUsesProjectOrderThenTerminalPinThenExplicitProfile(t *testing.T) {
	setup(t)
	t.Chdir(t.TempDir())
	if _, err := config.Update(func(c *config.Config) error {
		c.Tools["codex"].Arrange([]string{"default"}, config.Here())
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	check := func(pins map[tool.ID]string, want string) {
		t.Helper()
		_, selected, err := Environment(cfg, pins, os.Environ())
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range selected {
			if s.Tool == "codex" && s.Profile != want {
				t.Fatalf("got %s, want %s", s.Profile, want)
			}
		}
	}
	check(nil, "default")
	t.Setenv(config.PinVar("codex"), "other")
	check(nil, "other")
	check(map[tool.ID]string{"codex": "default"}, "default")
}

func TestInspectExternalOverlayIsHistoryNotIdentity(t *testing.T) {
	cfg := setup(t)
	if os.PathSeparator != '/' {
		t.Skip("symlinks need Developer Mode on Windows")
	}
	hub := cfg.EnsureHub("codex")
	testutil.Write(t, filepath.Join(hub, "sessions", "one.jsonl"), "conversation")
	shadow := filepath.Join(t.TempDir(), "shadow")
	if err := os.MkdirAll(shadow, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(hub, "sessions"), filepath.Join(shadow, "sessions")); err != nil {
		t.Fatal(err)
	}
	before := snapshot(t, filepath.Dir(cfg.Dir("codex", "other")))
	reports := Inspect(cfg, []string{"CODEX_HOME=" + shadow, "CODEX_API_KEY=NEVER-PRINT-THIS"})
	for _, r := range reports {
		if r.Tool == "codex" {
			if !r.CurrentHistory.Shared || !r.CurrentHistory.Exists || r.CurrentProfile != "" || len(r.Warnings) != 1 {
				t.Fatalf("overlay mistaken for an aims login: %+v", r)
			}
			if len(r.ConflictingEnv) != 1 || r.ConflictingEnv[0] != "CODEX_API_KEY" {
				t.Fatal("missing API key diagnostic")
			}
		}
	}
	b, _ := json.Marshal(reports)
	if strings.Contains(string(b), "NEVER-PRINT-THIS") {
		t.Fatal("reported a credential")
	}
	if after := snapshot(t, filepath.Dir(cfg.Dir("codex", "other"))); !reflect.DeepEqual(before, after) {
		t.Fatal("inspection repaired or created profile entries")
	}
}

func TestInspectWarnsAboutExplicitDefaultClaudeHome(t *testing.T) {
	cfg := setup(t)
	for _, r := range Inspect(cfg, []string{"CLAUDE_CONFIG_DIR=" + cfg.EnsureHub("claude")}) {
		if r.Tool == "claude" && (r.CurrentProfile != "" || !r.CurrentHistory.Shared || len(r.Warnings) != 1 || !strings.Contains(r.Warnings[0], "keychain")) {
			t.Fatalf("explicit default mistaken for the adopted login: %+v", r)
		}
	}
}

func TestPrivateHistoryStaysPrivate(t *testing.T) {
	cfg := setup(t)
	cfg.Tools["codex"].Profiles["other"].Share = config.ShareSettings
	testutil.Write(t, filepath.Join(cfg.Dir("codex", "other"), "goals_1.sqlite"), "private database")
	env, _, err := Environment(cfg, nil, []string{"CODEX_SQLITE_HOME=/inherited/shared"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := envMap(env)["CODEX_SQLITE_HOME"]; ok {
		t.Fatal("private profile inherited a shared database")
	}
	before := snapshot(t, config.Home())
	for _, r := range Inspect(cfg, nil) {
		if r.Tool == "codex" && r.Profiles[0].History.Shared {
			t.Fatal("private history reported as shared")
		}
	}
	if !reflect.DeepEqual(before, snapshot(t, config.Home())) {
		t.Fatal("inspection modified private data")
	}
}

// snapshot records synthetic test files without following symlinks.
func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	m := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			m[p], err = os.Readlink(p)
			return err
		}
		if !d.IsDir() {
			b, e := os.ReadFile(p)
			m[p] = string(b)
			return e
		}
		m[p] = "<directory>"
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}
