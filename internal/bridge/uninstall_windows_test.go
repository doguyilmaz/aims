package bridge

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/testutil"
	"github.com/doguyilmaz/aims/internal/tool"
	"golang.org/x/sys/windows"
)

func TestWindowsDeferredCleanupKeepsChangedInstallation(t *testing.T) {
	for _, changed := range []string{"manifest", "launcher"} {
		t.Run(changed, func(t *testing.T) {
			self := launcherFixture(t)
			m, err := InstallLaunchers(self)
			if err != nil {
				t.Fatal(err)
			}
			testutil.Write(t, filepath.Join(LauncherRoot(), "my-file"), "keep")
			parent, err := windows.CreateEvent(nil, 1, 0, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer windows.CloseHandle(parent)
			if err := windows.SetHandleInformation(parent, windows.HANDLE_FLAG_INHERIT, windows.HANDLE_FLAG_INHERIT); err != nil {
				t.Fatal(err)
			}
			cmd, err := startUninstallHelper(LauncherRoot(), m, parent)
			if err != nil {
				t.Fatal(err)
			}
			defer cmd.Process.Kill()
			if changed == "manifest" {
				testutil.Write(t, filepath.Join(LauncherRoot(), manifestName), `{"owner":"changed"}`)
			} else {
				path := filepath.Join(LauncherBin(), m.Tools["codex"].Name)
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				testutil.Write(t, path, "user replacement")
			}
			if err := windows.SetEvent(parent); err != nil {
				t.Fatal(err)
			}
			err = cmd.Wait()
			diagnostic := cmd.Stderr.(*cleanupDiagnostic).String()
			if err == nil || !strings.Contains(diagnostic, "changed") {
				t.Fatalf("cleanup did not reject the changed installation: %v\n%s", err, diagnostic)
			}
			for _, l := range m.Tools {
				if _, err := os.Lstat(filepath.Join(LauncherBin(), l.Name)); err != nil {
					t.Fatal("preflight failure removed an owned or replaced launcher")
				}
			}
			if _, err := os.Stat(filepath.Join(LauncherRoot(), manifestName)); err != nil {
				t.Fatal("preflight failure removed the manifest")
			}
			if testutil.Read(t, filepath.Join(LauncherRoot(), "my-file")) != "keep" ||
				testutil.Read(t, self) != "test-only executable content" {
				t.Fatal("preflight failure changed unrelated files or the original executable")
			}
		})
	}
}

func TestWindowsDeferredCleanupRechecksAfterBlockedDelete(t *testing.T) {
	for _, changed := range []string{"manifest", "launcher"} {
		t.Run(changed, func(t *testing.T) {
			self := launcherFixture(t)
			m, err := InstallLaunchers(self)
			if err != nil {
				t.Fatal(err)
			}
			l := m.Tools["codex"]
			path := filepath.Join(LauncherBin(), l.Name)
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Link(self, path); err != nil {
				t.Fatal(err)
			}
			l.Kind = "hardlink"
			m.Tools = map[tool.ID]Launcher{"codex": l}
			if err := fsx.WriteJSON(filepath.Join(LauncherRoot(), manifestName), m, 0o600); err != nil {
				t.Fatal(err)
			}
			name, err := windows.UTF16PtrFromString(path)
			if err != nil {
				t.Fatal(err)
			}
			// Permit hash reads and edits but prevent deletion until the test
			// observes the helper's first sharing violation.
			locked, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE,
				windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if locked != 0 {
					_ = windows.CloseHandle(locked)
				}
			}()
			parent, err := windows.CreateEvent(nil, 1, 0, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer windows.CloseHandle(parent)
			if err := windows.SetHandleInformation(parent, windows.HANDLE_FLAG_INHERIT, windows.HANDLE_FLAG_INHERIT); err != nil {
				t.Fatal(err)
			}
			cmd, err := startUninstallHelper(LauncherRoot(), m, parent)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
			if err := windows.SetEvent(parent); err != nil {
				t.Fatal(err)
			}
			diagnostic := cmd.Stderr.(*cleanupDiagnostic)
			select {
			case <-diagnostic.retry:
			case <-time.After(10 * time.Second):
				t.Fatalf("cleanup did not reach the locked deletion: %s", diagnostic.String())
			}
			changedPath := path
			if changed == "manifest" {
				changedPath = filepath.Join(LauncherRoot(), manifestName)
			}
			testutil.Write(t, changedPath, "user replacement after blocked deletion")
			if err := cmd.Wait(); err == nil || !strings.Contains(diagnostic.String(), "changed") {
				t.Fatalf("cleanup did not stop on the changed %s: %v\n%s", changed, err, diagnostic.String())
			}
			if got := testutil.Read(t, changedPath); got != "user replacement after blocked deletion" {
				t.Fatalf("cleanup changed the replacement: %q", got)
			}
			if _, err := os.Stat(path); err != nil {
				t.Fatal("cleanup removed the changed installation's launcher")
			}
			if _, err := os.Stat(filepath.Join(LauncherRoot(), manifestName)); err != nil {
				t.Fatal("cleanup removed the changed installation's manifest")
			}
		})
	}
}

func TestWindowsUninstallWaitsForMappedLauncherImage(t *testing.T) {
	testutil.Sandbox(t)
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	// The child must use a distinct image: the parent keeps its own test
	// executable mapped until all tests (including their cleanup) finish.
	child := filepath.Join(root, "uninstall-driver.exe")
	b, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(child, b, 0o700); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(root, "home with % and ' quotes")
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, child, "-test.run=^TestWindowsDeferredUninstallHelper$")
	cmd.Env = append(os.Environ(), "AIMS_TEST_UNINSTALL_HOME="+home)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("mapped-image uninstall: %v\n%s", err, out)
	}
	bridgeRoot := filepath.Join(home, "bridge")
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		_, manifestErr := os.Lstat(filepath.Join(bridgeRoot, manifestName))
		_, launcherErr := os.Lstat(filepath.Join(bridgeRoot, "bin", "codex.exe"))
		_, lockErr := os.Lstat(bridgeRoot + ".lock")
		if os.IsNotExist(manifestErr) && os.IsNotExist(launcherErr) && os.IsNotExist(lockErr) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("deferred removal did not complete: manifest=%v launcher=%v lock=%v", manifestErr, launcherErr, lockErr)
		}
	}
	if testutil.Read(t, filepath.Join(bridgeRoot, "my-file")) != "keep" ||
		testutil.Read(t, filepath.Join(home, "profiles", "auth.json")) != "DUMMY-CREDENTIAL" {
		t.Fatal("cleanup changed unrelated files or a login sentinel")
	}
	if _, err := os.Stat(child); err != nil {
		t.Fatal("cleanup removed the original executable")
	}
}

func TestWindowsDeferredUninstallHelper(t *testing.T) {
	home := os.Getenv("AIMS_TEST_UNINSTALL_HOME")
	if home == "" {
		return
	}
	t.Setenv(config.HomeVar, home)
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(LauncherBin(), "codex.exe")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(self, path); err != nil {
		t.Fatal(err)
	}
	d, err := digest(self)
	if err != nil {
		t.Fatal(err)
	}
	m := &Launchers{
		Owner: "aims.bridge", Version: "1.0.0", Aims: self, ConfigHome: home,
		Tools: map[tool.ID]Launcher{"codex": {Name: "codex.exe", Native: self, Kind: "hardlink", Digest: d}},
	}
	if err := fsx.WriteJSON(filepath.Join(LauncherRoot(), manifestName), m, 0o600); err != nil {
		t.Fatal(err)
	}
	testutil.Write(t, filepath.Join(LauncherRoot(), "my-file"), "keep")
	testutil.Write(t, filepath.Join(home, "profiles", "auth.json"), "DUMMY-CREDENTIAL")
	if err := UninstallLaunchers(); !errors.Is(err, ErrUninstallDeferred) {
		t.Fatalf("expected deferred mapped-image removal: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("launcher was removed while its image was still mapped")
	}
}
