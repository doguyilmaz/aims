package ops

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/testutil"
	"github.com/doguyilmaz/aims/internal/tool"
	"github.com/doguyilmaz/aims/internal/tools"
)

type logoutAdapter struct{ tool.Adapter }

func (a logoutAdapter) Layout() tool.Layout {
	l := a.Adapter.Layout()
	l.BinEnv = "AIMS_TEST_LOGOUT_BIN"
	return l
}

func (logoutAdapter) Commands() tool.Commands {
	return tool.Commands{Logout: []string{"-test.run=^TestPurgeLogoutHelper$"}}
}

func (logoutAdapter) Account(context.Context, tool.Home) tool.Account {
	panic("purge must confirm native logout, not infer keyring state from account inspection")
}

func TestPurgeLogoutHelper(t *testing.T) {
	mode := os.Getenv("AIMS_TEST_LOGOUT_MODE")
	if mode == "" {
		return
	}
	if mode == "fail" {
		os.Exit(3)
	}
	if err := os.WriteFile(os.Getenv("AIMS_TEST_LOGOUT_MARKER"), []byte("logged out"), 0o600); err != nil {
		os.Exit(4)
	}
	os.Exit(0)
}

func TestPurgeKeepsProfileUntilNativeLogoutCompletes(t *testing.T) {
	for _, mode := range []string{"missing-binary", "fail", "success-missing-folder"} {
		t.Run(mode, func(t *testing.T) {
			testutil.Sandbox(t)
			marker := filepath.Join(t.TempDir(), "logout-marker")
			cfg, err := config.Update(func(c *config.Config) error {
				c.EnsureHub("codex")
				c.Tools["codex"].Profiles["work"] = &config.Profile{Share: config.ShareNone, Env: map[string]string{
					"AIMS_TEST_LOGOUT_MODE": "success", "AIMS_TEST_LOGOUT_MARKER": marker,
				}}
				if mode == "fail" {
					c.Tools["codex"].Profiles["work"].Env["AIMS_TEST_LOGOUT_MODE"] = "fail"
				}
				c.Tools["codex"].Order = []string{"work"}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			bin, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			if mode == "missing-binary" {
				bin = filepath.Join(t.TempDir(), "missing-executable")
			}
			t.Setenv("AIMS_TEST_LOGOUT_BIN", bin)
			if mode == "fail" {
				testutil.Write(t, filepath.Join(cfg.Dir("codex", "work"), "private-data"), "keep")
			}
			res, err := RemoveProfile(t.Context(), logoutAdapter{tools.Get("codex")}, "work", true, true)
			loaded, loadErr := config.Load()
			if loadErr != nil {
				t.Fatal(loadErr)
			}
			if mode == "success-missing-folder" {
				if err != nil || !res.Purged || loaded.Tools["codex"].Profiles["work"] != nil || fsx.Stat(marker) == nil {
					t.Fatalf("missing home skipped logout: result=%+v err=%v", res, err)
				}
			} else {
				if err == nil || res.LogoutErr == nil || res.Purged || loaded.Tools["codex"].Profiles["work"] == nil {
					t.Fatalf("incomplete logout lost registration: result=%+v err=%v", res, err)
				}
				if mode == "fail" && fsx.Stat(filepath.Join(res.Dir, "private-data")) == nil {
					t.Fatal("failed logout deleted private data")
				}
			}
		})
	}
}

func TestAddProfileRejectsCaseVariantName(t *testing.T) {
	testutil.Sandbox(t)
	a := tools.Get("codex")
	if _, err := AddProfile(a, "Work", AddOptions{Share: config.ShareNone}); err != nil {
		t.Fatal(err)
	}
	if _, err := AddProfile(a, "work", AddOptions{Share: config.ShareNone}); err == nil {
		t.Fatal("case-variant name can alias another login after moving filesystems")
	}
	cfg, err := config.Load()
	if err != nil || len(cfg.Tools["codex"].Profiles) != 1 {
		t.Fatal("rejected add changed registration")
	}
}

func TestWipeRetainsConfigurationWhenLogoutUnavailable(t *testing.T) {
	testutil.Sandbox(t)
	if _, err := config.Update(func(c *config.Config) error {
		c.Tools["codex"].Profiles["work"] = &config.Profile{Share: config.ShareNone}
		c.Tools["codex"].Order = []string{"work"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	t.Setenv(tools.Get("codex").Layout().BinEnv, filepath.Join(t.TempDir(), "missing-executable"))
	if _, err := Wipe(t.Context(), true); err == nil {
		t.Fatal("wipe ignored incomplete logout")
	}
	cfg, err := config.Load()
	if err != nil || cfg.Tools["codex"].Profiles["work"] == nil {
		t.Fatal("failed wipe forgot profile")
	}
}

func TestPurgeRefusesRedirectedProfileRoot(t *testing.T) {
	testutil.Sandbox(t)
	cfg, err := config.Update(func(c *config.Config) error {
		c.Tools["codex"].Profiles["work"] = &config.Profile{Share: config.ShareNone}
		c.Tools["codex"].Order = []string{"work"}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	dir := cfg.Dir("codex", "work")
	external := t.TempDir()
	if err := os.MkdirAll(filepath.Dir(dir), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, dir); err != nil {
		t.Skipf("directory symlinks unavailable: %v", err)
	}
	res, err := RemoveProfile(t.Context(), logoutAdapter{tools.Get("codex")}, "work", true, true)
	if err == nil || !strings.Contains(err.Error(), "dedicated profile directory") || res.Purged {
		t.Fatalf("redirected root not refused before logout: %+v %v", res, err)
	}
	if fi := fsx.Lstat(dir); fi == nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("refusal changed the root link")
	}
}
