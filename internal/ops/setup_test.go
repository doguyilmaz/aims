package ops

import (
	"context"
	"path/filepath"
	"slices"
	"testing"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/testutil"
	"github.com/doguyilmaz/aims/internal/tool"
	"github.com/doguyilmaz/aims/internal/tools"
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
