package ops

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/testutil"
	"github.com/doguyilmaz/aims/internal/tool"
	"github.com/doguyilmaz/aims/internal/tools"
	"github.com/tidwall/gjson"
)

func TestMain(m *testing.M) { testutil.Main(m) }

func TestFailedCleanRetainsConfiguration(t *testing.T) {
	testutil.Sandbox(t)
	cfg, err := config.Update(func(c *config.Config) error {
		c.EnsureHub("codex")
		c.Tools["codex"].Profiles["work"] = &config.Profile{}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	testutil.Write(t, filepath.Join(cfg.EnsureHub("codex"), "skills", "aims", "SKILL.md"), "local changes")
	if _, err := Clean(context.Background()); err == nil {
		t.Fatal("clean ignored an uninstall conflict")
	}
	if fsx.Stat(filepath.Join(config.Home(), "config.json")) == nil {
		t.Fatal("failed clean forgot profiles")
	}
	loaded, err := config.Load()
	if err != nil || loaded.Tools["codex"].Profiles["work"] == nil {
		t.Fatal("profile no longer recoverable after failed clean")
	}
}

func TestIntegrationsHonorEntryExclusions(t *testing.T) {
	testutil.Sandbox(t)
	cfg, err := config.Update(func(c *config.Config) error {
		c.Tools["codex"].Profiles["work"] = &config.Profile{Exclude: []string{"skills", "config.toml"}}
		c.Tools["codex"].Profiles["shared"] = &config.Profile{}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	a := tools.Get("codex")
	for _, homes := range [][]tool.Home{IntegrationHomes(cfg, a, "skills"), MCPHomes(cfg, a)} {
		var dirs []string
		for _, h := range homes {
			dirs = append(dirs, h.Dir)
		}
		if len(dirs) != 2 || !slices.Contains(dirs, cfg.Dir("codex", "work")) {
			t.Fatalf("private integration home omitted: %v", dirs)
		}
	}
}

func TestShellJoin(t *testing.T) {
	cases := map[string][]string{
		"/usr/local/bin/aims statusline":                 {"/usr/local/bin/aims", "statusline"},
		`'/opt/my apps/aims' statusline`:                 {"/opt/my apps/aims", "statusline"},
		`'/opt/{a,b}/aims' mcp`:                          {"/opt/{a,b}/aims", "mcp"},
		`'/home/o'\''neil/[x]/aims#1' statusline`:        {"/home/o'neil/[x]/aims#1", "statusline"},
		`'C:\Users\me\aims.exe' statusline`:              {`C:\Users\me\aims.exe`, "statusline"},
		`'~/bin/aims' ''`:                                {"~/bin/aims", ""},
		"/Users/me/.local/bin/aims-2.0_x+y@z statusline": {"/Users/me/.local/bin/aims-2.0_x+y@z", "statusline"},
	}
	for want, parts := range cases {
		if got := shellJoin(parts); got != want {
			t.Errorf("shellJoin(%q) = %s, want %s", parts, got, want)
		}
	}
}

func TestStatusLineSetupRetainsEachHomeCommand(t *testing.T) {
	testutil.Sandbox(t)
	binDir := t.TempDir()
	bin := filepath.Join(binDir, "aims")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	testutil.Write(t, bin, "fixture; not executed")
	if err := os.Chmod(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	cfg, err := config.Update(func(c *config.Config) error {
		c.EnsureHub("claude")
		c.Tools["claude"].Profiles["personal"] = &config.Profile{Share: config.ShareNone}
		c.Tools["claude"].Profiles["work"] = &config.Profile{Share: config.ShareNone}
		c.Tools["claude"].Order = []string{"personal", "work"}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	homes := IntegrationHomes(cfg, tools.Get("claude"), "settings.json")
	commands := []string{"echo hub", "echo personal", "echo work"}
	for i, h := range homes {
		b, _ := json.Marshal(map[string]any{"statusLine": map[string]any{"type": "command", "command": commands[i], "padding": 3}})
		testutil.Write(t, filepath.Join(h.Dir, "settings.json"), string(b))
	}
	for range 2 {
		steps, err := Setup(t.Context(), []tool.ID{"claude"}, Integrations{StatusLine: true})
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range steps {
			if s.Err != nil {
				t.Fatal(s.Err)
			}
		}
	}
	cfg, _ = config.Load()
	if cfg.Status.Chain != "" {
		t.Fatal("setup overwrote global fallback")
	}
	for i, h := range homes {
		if got := cfg.Status.CommandForHome(h.Dir); got != commands[i] {
			t.Fatalf("home %s: %q", h.Dir, got)
		}
	}
	steps, err := Uninstall(t.Context(), Integrations{StatusLine: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range steps {
		if s.Err != nil {
			t.Fatal(s.Err)
		}
	}
	for i, h := range homes {
		raw := testutil.Read(t, filepath.Join(h.Dir, "settings.json"))
		if gjson.Get(raw, "statusLine.command").String() != commands[i] || gjson.Get(raw, "statusLine.padding").Int() != 3 {
			t.Fatalf("uninstall restored wrong home command or changed padding: %s", raw)
		}
	}
}
