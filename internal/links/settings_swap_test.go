package links

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/doguyilmaz/aims/internal/testutil"
	"github.com/doguyilmaz/aims/internal/tool"
)

func settingsSwapFixture(t *testing.T, hub, dir, name string) Options {
	t.Helper()
	testutil.Write(t, filepath.Join(hub, name), "original shared settings")
	testutil.Write(t, filepath.Join(dir, name), "newer private settings")
	for path, when := range map[string]time.Time{
		filepath.Join(hub, name): time.Now().Add(-2 * time.Hour),
		filepath.Join(dir, name): time.Now().Add(-time.Hour),
	} {
		if err := os.Chtimes(path, when, when); err != nil {
			t.Fatal(err)
		}
	}
	return Options{Hub: hub, Dir: dir, Layout: tool.Layout{Shared: tool.Names(name)}, Label: "work"}
}

func assertUnchangedSettingsSwap(t *testing.T, hub, dir, name string) {
	t.Helper()
	if testutil.Read(t, filepath.Join(hub, name)) != "original shared settings" ||
		testutil.Read(t, filepath.Join(dir, name)) != "newer private settings" {
		t.Fatal("failed replacement changed or hid a settings copy")
	}
	backups, err := filepath.Glob(filepath.Join(hub, name) + ".aims-*")
	if err != nil || len(backups) != 0 {
		t.Fatalf("failed replacement left the original hub in a backup: %v, %v", backups, err)
	}
}

func TestSettingsSwapPreflightsCrossFilesystemReplacement(t *testing.T) {
	other := os.Getenv("AIMS_TEST_SECOND_FS")
	if other == "" {
		t.Skip("a second scratch filesystem is required")
	}
	for _, name := range []string{"config.toml", "settings.json"} {
		t.Run(name, func(t *testing.T) {
			hub := filepath.Join(t.TempDir(), "hub")
			dir, err := os.MkdirTemp(other, "aims-settings-swap-")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { os.RemoveAll(dir) })
			o := settingsSwapFixture(t, hub, dir, name)
			if same, err := sameFilesystem(filepath.Join(dir, name), hub); err != nil || same {
				t.Fatalf("fixture requires different filesystems: %v %v", same, err)
			}
			if _, err := ensureOne(o, name); err == nil || !strings.Contains(err.Error(), "cross-filesystem") {
				t.Fatalf("replacement was not refused before the swap: %v", err)
			}
			assertUnchangedSettingsSwap(t, hub, dir, name)
		})
	}
}

func TestSettingsSwapRestoresHubAfterReplacementFailure(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("fixture requires unprivileged POSIX directory permissions")
	}
	for _, name := range []string{"config.toml", "settings.json"} {
		t.Run(name, func(t *testing.T) {
			hub, dir := dirs(t)
			o := settingsSwapFixture(t, hub, dir, name)
			before, err := os.Stat(filepath.Join(hub, name))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(dir, 0o555); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { os.Chmod(dir, 0o700) })
			// The hub can be renamed, but the source directory forbids the
			// subsequent unlink. The original hub inode must be restored.
			if _, err := ensureOne(o, name); !errors.Is(err, fs.ErrPermission) {
				t.Fatalf("fixture did not reach the failed replacement: %v", err)
			}
			assertUnchangedSettingsSwap(t, hub, dir, name)
			after, err := os.Stat(filepath.Join(hub, name))
			if err != nil || !os.SameFile(before, after) {
				t.Fatal("rollback did not restore the original shared inode")
			}
		})
	}
}
