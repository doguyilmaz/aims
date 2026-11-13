package ops

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/profiles"
	"github.com/doguyilmaz/aims/internal/testutil"
	"github.com/doguyilmaz/aims/internal/tool"
	"github.com/doguyilmaz/aims/internal/tools"
)

type failingSeeder struct{ tool.Adapter }

type pendingProbeAdapter struct{ logoutAdapter }

func (pendingProbeAdapter) Probe(context.Context, tool.Home) tool.Probe {
	panic("pending purges must not contact a provider")
}

func (failingSeeder) Seed(_ tool.Home, dir string) error {
	if err := os.WriteFile(filepath.Join(dir, "partial-seed"), []byte("keep"), 0o600); err != nil {
		return err
	}
	return errors.New("fixture seed failure")
}

func TestAddDoesNotRegisterOrActivateAnIncompleteHome(t *testing.T) {
	for _, kind := range []string{"mkdir", "seed"} {
		t.Run(kind, func(t *testing.T) {
			testutil.Sandbox(t)
			var adapter tool.Adapter = failingSeeder{tools.Get("codex")}
			if kind == "mkdir" {
				testutil.Write(t, config.ProfilesRoot(), "not a directory")
			}
			added, err := AddProfile(adapter, "new", AddOptions{})
			if err == nil {
				t.Fatal("incomplete setup succeeded")
			}
			cfg, err := config.Load()
			if err != nil || len(cfg.Tools["codex"].Profiles) != 0 || cfg.Tools["codex"].Active != "" {
				t.Fatalf("failed setup committed active registration: %+v %v", cfg, err)
			}
			if kind == "seed" && testutil.Read(t, filepath.Join(added.Dir, "partial-seed")) != "keep" {
				t.Fatal("failed setup erased retained files")
			}
		})
	}
}

func TestStateFailureKeepsRegistrationAndRecreationClearsLateOldCache(t *testing.T) {
	testutil.Sandbox(t)
	a := tools.Get("codex")
	if _, err := AddProfile(a, "work", AddOptions{Share: config.ShareNone}); err != nil {
		t.Fatal(err)
	}
	if err := config.UpdateState("codex", "work", func(s *config.ProfileState) { s.NeedsLogin = true }); err != nil {
		t.Fatal(err)
	}
	unlock, ok, err := fsx.Lock(filepath.Join(config.Home(), "state.json"), 0)
	if err != nil || !ok {
		t.Fatal(err)
	}
	defer func() {
		if unlock != nil {
			unlock()
		}
	}()
	if _, err := RemoveProfile(t.Context(), a, "work", false, false); err == nil {
		t.Fatal("state cleanup failure was ignored")
	}
	cfg, err := config.Load()
	if err != nil || cfg.Tools["codex"].Profiles["work"] == nil {
		t.Fatal("failed state cleanup forgot the registration")
	}
	unlock()
	unlock = nil
	if _, err := RemoveProfile(t.Context(), a, "work", false, false); err != nil {
		t.Fatal(err)
	}
	// An older process can leave cached state after its label was forgotten.
	if err := config.UpdateState("codex", "work", func(s *config.ProfileState) { s.NeedsLogin = true }); err != nil {
		t.Fatal(err)
	}
	unlock, ok, err = fsx.Lock(filepath.Join(config.Home(), "state.json"), 0)
	if err != nil || !ok {
		t.Fatal(err)
	}
	if _, err := AddProfile(a, "work", AddOptions{Share: config.ShareNone}); err == nil {
		t.Fatal("state cleanup failure registered a recreated profile")
	}
	if loaded, err := config.Load(); err != nil || loaded.Tools["codex"].Profiles["work"] != nil {
		t.Fatal("failed recreated profile changed registration")
	}
	unlock()
	unlock = nil
	if _, err := AddProfile(a, "work", AddOptions{Share: config.ShareNone}); err != nil {
		t.Fatal(err)
	}
	if config.LoadState().Get("codex", "work").NeedsLogin {
		t.Fatal("recreated profile inherited a stale login mark")
	}
}

func purgeFixture(t *testing.T) (*config.Config, string) {
	t.Helper()
	marker := filepath.Join(t.TempDir(), "logout-marker")
	cfg, err := config.Update(func(c *config.Config) error {
		c.EnsureHub("codex")
		c.Tools["codex"].Profiles["work"] = &config.Profile{Share: config.ShareNone, Env: map[string]string{
			"AIMS_TEST_LOGOUT_MODE": "success", "AIMS_TEST_LOGOUT_MARKER": marker,
		}}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	testutil.Write(t, filepath.Join(cfg.Dir("codex", "work"), "private-history"), "keep until committed purge")
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("AIMS_TEST_LOGOUT_BIN", self)
	return cfg, marker
}

func TestPurgeCannotLogOutOrDeleteBeforeConfigCheckpoint(t *testing.T) {
	testutil.Sandbox(t)
	cfg, marker := purgeFixture(t)
	unlock, ok, err := fsx.Lock(filepath.Join(config.Home(), "config.json"), 0)
	if err != nil || !ok {
		t.Fatal(err)
	}
	defer unlock()
	if result, err := RemoveProfile(t.Context(), logoutAdapter{tools.Get("codex")}, "work", true, true); err == nil || result.Purged {
		t.Fatal("purge ignored config checkpoint failure")
	}
	if fsx.Stat(marker) != nil || fsx.Stat(filepath.Join(cfg.Dir("codex", "work"), "private-history")) == nil {
		t.Fatal("failed checkpoint logged out or deleted history")
	}
}

func TestPurgeKeepsHistoryWhenCheckpointFailsAfterLogoutAndCanResume(t *testing.T) {
	testutil.Sandbox(t)
	cfg, _ := purgeFixture(t)
	path := filepath.Join(config.Home(), "config.json")
	if _, err := config.Update(func(c *config.Config) error {
		c.Tools["codex"].Profiles["work"].Env["AIMS_TEST_LOGOUT_MODE"] = "break-config-once"
		c.Tools["codex"].Profiles["work"].Env["AIMS_TEST_CONFIG_PATH"] = path
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	a := logoutAdapter{tools.Get("codex")}
	result, err := RemoveProfile(t.Context(), a, "work", true, true)
	if err == nil || result.Purged || fsx.Stat(filepath.Join(cfg.Dir("codex", "work"), "private-history")) == nil {
		t.Fatalf("failed checkpoint lost history: %+v %v", result, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path+".saved", path); err != nil {
		t.Fatal(err)
	}
	loaded, err := config.Load()
	if err != nil || loaded.Tools["codex"].Profiles["work"].Purge == nil {
		t.Fatal("interrupted purge has no durable recovery state")
	}
	if err := profiles.RequireIsolation(loaded, a, "work"); err == nil {
		t.Fatal("interrupted purge remained launchable")
	}
	// The completion journal survives config replacement failure. A retry
	// must not require a provider command that already completed successfully.
	t.Setenv("AIMS_TEST_LOGOUT_BIN", filepath.Join(t.TempDir(), "no-provider"))
	if result, err = RemoveProfile(t.Context(), a, "work", true, true); err != nil || !result.Purged {
		t.Fatalf("explicit retry failed: %+v %v", result, err)
	}
}

func TestCompletedLogoutPurgeResumesWithoutProviderAndNeverRepairsItsHome(t *testing.T) {
	testutil.Sandbox(t)
	cfg, _ := purgeFixture(t)
	cfg, err := config.Update(func(c *config.Config) error {
		c.Tools["codex"].Profiles["work"].Purge = &config.PurgeState{LoggedOut: true}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	a := logoutAdapter{tools.Get("codex")}
	t.Setenv("AIMS_TEST_LOGOUT_BIN", filepath.Join(t.TempDir(), "no-provider"))
	if _, err := cfg.Preferred("codex", "work"); err == nil {
		t.Fatal("explicit launch selected pending purge")
	}
	if chosen, _ := cfg.Preferred("codex", ""); chosen != "" {
		t.Fatal("automatic launch selected pending purge")
	}
	t.Setenv(config.PinVar("codex"), "work")
	if _, err := cfg.Preferred("codex", ""); err == nil {
		t.Fatal("pending pin silently selected a different profile")
	}
	if _, err := Use(a, "work", ""); err == nil {
		t.Fatal("pending profile could become the selected account")
	}
	if _, err := SetOrder(a, []string{"work"}, ""); err == nil {
		t.Fatal("order selected a pending purge")
	}
	if _, err := Failover(t.Context(), a, "work", "work", 0, "", "", ""); err == nil {
		t.Fatal("failover selected a pending purge")
	}
	if _, err := Clean(t.Context()); err == nil {
		t.Fatal("clean destroyed a pending purge checkpoint")
	}
	if loaded, err := config.Load(); err != nil || loaded.Tools["codex"].Profiles["work"].Purge == nil {
		t.Fatal("clean lost the purge checkpoint")
	}
	if evaluation := profiles.Evaluate(t.Context(), cfg, config.LoadState(), a, "work"); evaluation.Usable {
		t.Fatal("pending purge is usable")
	}
	probeAdapter := pendingProbeAdapter{a}
	if probe := profiles.Live(t.Context(), cfg, probeAdapter, "work"); probe.Status != tool.ProbeError {
		t.Fatal("pending purge returned a live reading")
	}
	if probe := profiles.ReadUsage(t.Context(), cfg, probeAdapter, "work"); probe.Status != tool.ProbeError {
		t.Fatal("pending purge returned a fresh usage reading")
	}
	if _, _, renewed := profiles.KeepAlive(t.Context(), cfg, probeAdapter, "work"); renewed {
		t.Fatal("keepalive renewed a pending purge")
	}
	for _, home := range IntegrationHomes(cfg, a, a.Layout().SkillsDir) {
		if home.Dir == cfg.Dir("codex", "work") {
			t.Fatal("integration setup would recreate a pending purge home")
		}
	}
	for _, home := range MCPHomes(cfg, a) {
		if home.Dir == cfg.Dir("codex", "work") {
			t.Fatal("MCP setup would recreate a pending purge home")
		}
	}
	if err := os.RemoveAll(cfg.Dir("codex", "work")); err != nil {
		t.Fatal(err)
	}
	profiles.Sync(cfg, a, "work", true)
	profiles.RepairSharing(cfg, a, "work")
	if fsx.Stat(cfg.Dir("codex", "work")) != nil {
		t.Fatal("ordinary repair recreated a purged home")
	}
	if result, err := RemoveProfile(t.Context(), a, "work", true, true); err != nil || !result.Purged {
		t.Fatalf("completed logout could not finalize without provider: %+v %v", result, err)
	}
}
