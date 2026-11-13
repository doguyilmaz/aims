package bridge

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/testutil"
	"github.com/doguyilmaz/aims/internal/tools"
)

func launcherFixture(t *testing.T) string {
	t.Helper()
	testutil.Sandbox(t)
	dir := t.TempDir()
	self := filepath.Join(dir, "aims")
	if runtime.GOOS == "windows" {
		self += ".exe"
	}
	testutil.Write(t, self, "test-only executable content")
	if err := os.Chmod(self, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, a := range tools.All() {
		bin := filepath.Join(dir, executableName(a))
		testutil.Write(t, bin, "native "+string(a.ID()))
		if err := os.Chmod(bin, 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv(a.Layout().BinEnv, bin)
	}
	return self
}

func TestLaunchersInstallUpdateAndRemoveOnlyOwnedFiles(t *testing.T) {
	self := launcherFixture(t)
	for range 2 {
		m, err := InstallLaunchers(self)
		if err != nil {
			t.Fatal(err)
		}
		if len(m.Tools) != 2 || m.ConfigHome != config.Home() {
			t.Fatalf("manifest: %+v", m)
		}
		for id, l := range m.Tools {
			p := filepath.Join(LauncherBin(), l.Name)
			if !samePath(p, self) {
				t.Fatal("executable was copied rather than linked")
			}
			got, resolved, err := LauncherInvocation(p)
			if err != nil || got != id || !samePath(resolved.Aims, self) {
				t.Fatalf("invocation: %s %v", got, err)
			}
		}
	}
	testutil.Write(t, filepath.Join(LauncherRoot(), "my-file"), "keep")
	testutil.Write(t, filepath.Join(config.Home(), "profiles", "claude", "account", ".credentials.json"), "never-touch")
	if err := UninstallLaunchers(); err != nil {
		t.Fatal(err)
	}
	if testutil.Read(t, filepath.Join(LauncherRoot(), "my-file")) != "keep" || testutil.Read(t, filepath.Join(config.Home(), "profiles", "claude", "account", ".credentials.json")) != "never-touch" {
		t.Fatal("uninstall changed unrelated files")
	}
	if _, err := os.Stat(self); err != nil {
		t.Fatal("aims executable removed")
	}
}

func TestLaunchersRejectUnmanagedDirectoriesAndChangedExecutables(t *testing.T) {
	self := launcherFixture(t)
	testutil.Write(t, filepath.Join(LauncherRoot(), "unrelated"), "keep")
	before := snapshot(t, config.Home())
	if _, err := InstallLaunchers(self); err == nil {
		t.Fatal("adopted an unmanaged directory")
	}
	if !reflect.DeepEqual(before, snapshot(t, config.Home())) {
		t.Fatal("changed unmanaged directory")
	}
	if err := os.RemoveAll(LauncherRoot()); err != nil {
		t.Fatal(err)
	}
	m, err := InstallLaunchers(self)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(LauncherBin(), m.Tools["codex"].Name)
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	testutil.Write(t, p, "a user replacement")
	before = snapshot(t, config.Home())
	if _, err := InstallLaunchers(self); err == nil {
		t.Fatal("overwrote a changed executable")
	}
	if err := UninstallLaunchers(); err == nil {
		t.Fatal("removed a changed executable")
	}
	if !reflect.DeepEqual(before, snapshot(t, config.Home())) {
		t.Fatal("failed operation modified files")
	}
}

func TestNativeResolutionBypassesLaunchersAndPreservesExplicitOverrides(t *testing.T) {
	self := launcherFixture(t)
	m, err := InstallLaunchers(self)
	if err != nil {
		t.Fatal(err)
	}
	a := tools.Get("codex")
	t.Setenv("PATH", LauncherBin()) // A GUI may have no original CLI on PATH.
	t.Setenv(a.Layout().BinEnv, "")
	if p, err := NativeBinary(a, LauncherRoot(), self, m); err != nil || p != m.Tools["codex"].Native {
		t.Fatalf("GUI fallback: %q %v", p, err)
	}
	t.Setenv(a.Layout().BinEnv, filepath.Join(LauncherBin(), m.Tools["codex"].Name))
	if _, err := NativeBinary(a, LauncherRoot(), self, m); err == nil {
		t.Fatal("recursive executable override accepted")
	}
}

func TestLaunchersFollowTheStableAimsPathAcrossUpgrades(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("tests the symlink-based Homebrew/version-manager installation")
	}
	self := launcherFixture(t)
	bin := t.TempDir()
	stable := filepath.Join(bin, "aims")
	if err := os.Symlink(self, stable); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	m, err := InstallLaunchers(self)
	if err != nil {
		t.Fatal(err)
	}
	if m.Aims != stable {
		t.Fatalf("stored the versioned binary rather than its stable entry: %q", m.Aims)
	}
	upgraded := filepath.Join(t.TempDir(), "aims-new")
	testutil.Write(t, upgraded, "next executable version")
	os.Chmod(upgraded, 0o755)
	os.Remove(stable)
	if err := os.Symlink(upgraded, stable); err != nil {
		t.Fatal(err)
	}
	os.Remove(self)
	if !samePath(filepath.Join(LauncherBin(), m.Tools["codex"].Name), upgraded) {
		t.Fatal("the launcher broke when the old versioned executable was removed")
	}
	if err := UninstallLaunchers(); err != nil {
		t.Fatal(err)
	}
}

func TestHardlinkOwnershipSurvivesAnInPlaceExecutableUpgrade(t *testing.T) {
	self := launcherFixture(t)
	m, err := InstallLaunchers(self)
	if err != nil {
		t.Fatal(err)
	}
	// Exercise the Windows hardlink fallback even on a Unix test host.
	l := m.Tools["codex"]
	p := filepath.Join(LauncherBin(), l.Name)
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(self, p); err != nil {
		t.Fatal(err)
	}
	l.Kind, l.Digest = "hardlink", ""
	l.Digest, err = digest(self)
	if err != nil {
		t.Fatal(err)
	}
	m.Tools["codex"] = l
	if err := fsx.WriteJSON(filepath.Join(LauncherRoot(), manifestName), m, 0o600); err != nil {
		t.Fatal(err)
	}
	testutil.Write(t, self, "upgraded in place")
	if err := UninstallLaunchers(); err != nil {
		t.Fatal(err)
	}
	if testutil.Read(t, self) != "upgraded in place" {
		t.Fatal("uninstall deleted the original upgraded executable")
	}
}

func TestLaunchersRejectSymlinkRootAndCorruptManifest(t *testing.T) {
	self := launcherFixture(t)
	if os.PathSeparator == '/' {
		target := t.TempDir()
		if err := os.MkdirAll(config.Home(), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, LauncherRoot()); err != nil {
			t.Fatal(err)
		}
		if _, err := InstallLaunchers(self); err == nil {
			t.Fatal("followed an external root")
		}
		if files, _ := os.ReadDir(target); len(files) != 0 {
			t.Fatal("wrote into an external root")
		}
		os.Remove(LauncherRoot())
	}
	if _, err := InstallLaunchers(self); err != nil {
		t.Fatal(err)
	}
	testutil.Write(t, filepath.Join(LauncherRoot(), manifestName), `{"owner":"someone-else"}`)
	before := snapshot(t, config.Home())
	if err := UninstallLaunchers(); err == nil {
		t.Fatal("uninstalled without valid ownership metadata")
	}
	if !reflect.DeepEqual(before, snapshot(t, config.Home())) {
		t.Fatal("changed corrupt installation")
	}
}

func TestAgentEnvironmentUsesOnlyOneProviderAndPreservesBundledRuntimeOverrides(t *testing.T) {
	cfg := setup(t)
	cfg.Tools["claude"].Profiles["other"].Env = map[string]string{"ANTHROPIC_API_KEY": "profile-key"}
	env, name, err := AgentEnvironment(cfg, tools.Get("claude"), "other", []string{
		"CODEX_HOME=/another-app", "CLAUDECODE=1", "AIMS_RUN=/parent-run", "CLAUDE_CODE_EXECUTABLE=/adapter/pinned/claude", "CODEX_PATH=/adapter/pinned/codex", "ANTHROPIC_API_KEY=shell-key",
	})
	if err != nil {
		t.Fatal(err)
	}
	m := envMap(env)
	if name != "other" || m["CODEX_HOME"] != "/another-app" || m["CLAUDE_CODE_EXECUTABLE"] != "/adapter/pinned/claude" || m["CODEX_PATH"] != "/adapter/pinned/codex" || m["ANTHROPIC_API_KEY"] != "profile-key" {
		t.Fatal("changed another provider or pinned adapter runtime")
	}
	if m["AIMS_RUN"] != "" || m["CLAUDECODE"] != "" || m["AIMS_SESSION_PROFILE"] != "claude:other" || m["AIMS_SESSION_MODE"] != "unwatched" {
		t.Fatalf("host session markers: %v", m)
	}
	_, _, err = AgentEnvironment(cfg, tools.Get("codex"), "missing", nil)
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatal("unknown profile silently changed accounts")
	}
}
