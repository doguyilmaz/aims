package bridge

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/testutil"
	"github.com/doguyilmaz/aims/internal/tool"
)

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
