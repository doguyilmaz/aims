package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/testutil"
)

func TestMain(m *testing.M) { testutil.Main(m) }

func TestLoadDefaultsAndNormalize(t *testing.T) {
	home := testutil.Sandbox(t)
	testutil.Write(t, filepath.Join(home, ".aims", "config.json"), `{
	  "failover": {"auto": true, "threshold": 250},
	  "tools": {"claude": {"active": "gone", "order": ["b", "missing", "b"], "profiles": {"a": {}, "b": {}, "c": null}}}
	}`)
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Failover.Threshold != 95 || c.Failover.CooldownMinutes != 300 {
		t.Errorf("defaults: %+v", c.Failover)
	}
	tl := c.Tools["claude"]
	// An unknown active profile falls back to the first in order.
	if strings.Join(tl.Order, ",") != "b,a,c" || tl.Active != "b" || tl.Profiles["c"] == nil {
		t.Errorf("normalize: order=%v active=%q", tl.Order, tl.Active)
	}
	// The active profile always leads the order.
	tl.Active = "c"
	c.normalize()
	if strings.Join(tl.Order, ",") != "c,b,a" {
		t.Errorf("active first: order=%v", tl.Order)
	}
	if c.Tools["codex"] == nil || c.Tools["codex"].Profiles == nil {
		t.Error("every known tool gets an entry")
	}
}

func TestBrokenConfigIsNeverOverwritten(t *testing.T) {
	home := testutil.Sandbox(t)
	path := filepath.Join(home, ".aims", "config.json")
	testutil.Write(t, path, `{"tools": {`)
	if _, err := Load(); err == nil {
		t.Fatal("broken config loaded")
	}
	if _, err := Update(func(*Config) error { return nil }); err == nil {
		t.Fatal("broken config updated")
	}
	if testutil.Read(t, path) != `{"tools": {` {
		t.Fatal("broken config was rewritten")
	}
}

func TestUpdateRoundTrip(t *testing.T) {
	home := testutil.Sandbox(t)
	if _, err := Update(func(c *Config) error {
		c.Tools["claude"].Profiles["work"] = &Profile{Share: ShareSettings}
		c.Tools["claude"].Active = "work"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(filepath.Join(home, ".aims", "config.json"))
	if err != nil || (fi.Mode().Perm()&0o077 != 0 && os.PathSeparator == '/') {
		t.Fatalf("config must be private: %v %v", fi.Mode(), err)
	}
	if di, err := os.Stat(filepath.Join(home, ".aims")); err == nil && os.PathSeparator == '/' && di.Mode().Perm() != 0o700 {
		t.Fatalf("~/.aims is %v", di.Mode().Perm())
	}
	c, _ := Load()
	if c.Tools["claude"].Profiles["work"].ShareMode() != ShareSettings || c.Tools["claude"].Active != "work" {
		t.Fatal("round trip")
	}
}

func TestHubAndHomeEnv(t *testing.T) {
	home := testutil.Sandbox(t)
	c, _ := Load()
	c.Tools["claude"].Profiles["personal"] = &Profile{Existing: true}
	c.Tools["claude"].Profiles["work"] = &Profile{}
	c.Tools["claude"].Profiles["ext"] = &Profile{Dir: "~/ext"}
	c.normalize()

	if c.EnsureHub("claude") != filepath.Join(home, ".claude") {
		t.Fatal("hub")
	}
	// The adopted login runs with the variable unset, exactly as before.
	if c.HomeEnv("claude", "personal") != "" || c.Dir("claude", "personal") != filepath.Join(home, ".claude") {
		t.Fatal("existing profile")
	}
	if c.HomeEnv("claude", "work") != filepath.Join(home, ".aims", "profiles", "claude", "work") {
		t.Fatalf("work: %s", c.HomeEnv("claude", "work"))
	}
	if c.Dir("claude", "ext") != filepath.Join(home, "ext") {
		t.Fatal("custom dir")
	}
	for value, want := range map[string]string{
		"": "personal", filepath.Join(home, ".claude"): "personal",
		filepath.Join(home, ".aims", "profiles", "claude", "work") + string(filepath.Separator): "work",
		"/elsewhere": "",
	} {
		if got := c.ProfileForHomeEnv("claude", value); got != want {
			t.Errorf("ProfileForHomeEnv(%q) = %q, want %q", value, got, want)
		}
	}
}

func TestHubFromEnvIsKeptVerbatim(t *testing.T) {
	home := testutil.Sandbox(t)
	t.Setenv("CLAUDE_CONFIG_DIR", "~/cfg/claude")
	c, _ := Load()
	if c.EnsureHub("claude") != filepath.Join(home, "cfg", "claude") || c.Tools["claude"].HubEnv != "~/cfg/claude" {
		t.Fatalf("hub=%s env=%s", c.Tools["claude"].Hub, c.Tools["claude"].HubEnv)
	}
	// A profile's own directory in the variable is not mistaken for the hub.
	c2, _ := Load()
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".aims", "profiles", "claude", "work"))
	if c2.EnsureHub("claude") != filepath.Join(home, ".claude") {
		t.Fatal("a profile became the hub")
	}
}

func TestPreferred(t *testing.T) {
	testutil.Sandbox(t)
	c, _ := Load()
	tl := c.Tools["codex"]
	tl.Profiles["a"], tl.Profiles["b"] = &Profile{}, &Profile{}
	c.normalize()
	if p, _ := c.Preferred("codex", ""); p != "a" {
		t.Fatalf("first in order: %s", p)
	}
	tl.Active = "b"
	if p, _ := c.Preferred("codex", ""); p != "b" {
		t.Fatalf("active: %s", p)
	}
	t.Setenv(PinVar("codex"), "a")
	if p, _ := c.Preferred("codex", ""); p != "a" {
		t.Fatalf("pin: %s", p)
	}
	if _, err := c.Preferred("codex", "zzz"); err == nil {
		t.Fatal("unknown explicit profile")
	}
}

func TestValidName(t *testing.T) {
	for _, n := range []string{"work", "a.b-c_1", "W2"} {
		if !ValidName(n) {
			t.Errorf("%q rejected", n)
		}
	}
	for _, n := range []string{"", "../x", "a/b", ".hidden", "-flag", "a b", "work.", "work-", strings.Repeat("x", 41)} {
		if ValidName(n) {
			t.Errorf("%q accepted", n)
		}
	}
}

func TestStateUpdates(t *testing.T) {
	testutil.Sandbox(t)
	if err := UpdateState("claude", "work", func(ps *ProfileState) { ps.NeedsLogin = true }); err != nil {
		t.Fatal(err)
	}
	if !LoadState().Get("claude", "work").NeedsLogin {
		t.Fatal("state not saved")
	}
	if LoadState().Get("codex", "nobody") == nil {
		t.Fatal("Get must never return nil")
	}
	if err := RemoveState("claude", "work"); err != nil || LoadState().Get("claude", "work").NeedsLogin {
		t.Fatal("state not removed")
	}
}

// A project's own order is in effect inside it, the profiles it names
// first; elsewhere, and in what is saved, the order of every folder stays.
func TestProjectOrder(t *testing.T) {
	home := testutil.Sandbox(t)
	repo := filepath.Join(home, "src", "app")
	testutil.Write(t, filepath.Join(repo, ".git", "HEAD"), "ref: main\n")
	testutil.Write(t, filepath.Join(repo, "web", "x"), "")
	testutil.Write(t, filepath.Join(home, ".git", "HEAD"), "")
	if got := ProjectOf(filepath.Join(repo, "web")); got != fsx.Real(repo) {
		t.Errorf("subfolder: %q", got)
	}
	// A repository at home does not swallow every folder.
	if got, want := ProjectOf(filepath.Join(home, "src")), fsx.Real(filepath.Join(home, "src")); got != want {
		t.Errorf("home repository: %q, want %q", got, want)
	}

	if _, err := Update(func(c *Config) error {
		tl := c.Tools["claude"]
		for _, n := range []string{"a", "b", "c"} {
			tl.Profiles[n] = &Profile{}
		}
		tl.Order = []string{"a", "b", "c"}
		tl.Arrange([]string{"c"}, fsx.Real(repo))
		tl.Arrange([]string{"b"}, fsx.Real(repo))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	t.Chdir(filepath.Join(repo, "web"))
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	tl := c.Tools["claude"]
	if strings.Join(tl.Order, ",") != "b,c,a" || tl.Active != "b" || tl.Project != fsx.Real(repo) || strings.Join(tl.Everywhere, ",") != "a,b,c" {
		t.Errorf("in the project: order=%v active=%q project=%q everywhere=%v", tl.Order, tl.Active, tl.Project, tl.Everywhere)
	}
	t.Chdir(home)
	if c, _ = Load(); c.Tools["claude"].Active != "a" || c.Tools["claude"].Project != "" {
		t.Errorf("elsewhere: %+v", c.Tools["claude"])
	}

	// A profile that goes leaves the project orders; an empty one goes too.
	c, err = Update(func(c *Config) error {
		delete(c.Tools["claude"].Profiles, "b")
		delete(c.Tools["claude"].Profiles, "c")
		return nil
	})
	if err != nil || c.Tools["claude"].Projects != nil || strings.Join(c.Tools["claude"].Order, ",") != "a" {
		t.Errorf("cleaned: %v %+v", err, c.Tools["claude"])
	}
}
