package desktop

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func fakeBundle(t *testing.T, root string) string {
	t.Helper()
	path := filepath.Join(root, "Aims Usage.app")
	for _, rel := range []string{"MacOS/AimsUsage", "Resources/aims"} {
		file := filepath.Join(path, "Contents", rel)
		if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte("fixture"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	info := `<plist><dict><key>CFBundleIdentifier</key><string>` + BundleID + `</string><key>CFBundleExecutable</key><string>AimsUsage</string><key>CFBundleShortVersionString</key><string>0.3.0</string></dict></plist>`
	if err := os.WriteFile(filepath.Join(path, "Contents", "Info.plist"), []byte(info), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestTerminalCommandPreservesExistingInstallations(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix command links")
	}
	bundle := fakeBundle(t, t.TempDir())
	for _, kind := range []string{"file", "link", "dangling link"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "aims")
			var err error
			if kind == "file" {
				err = os.WriteFile(path, []byte("existing CLI"), 0700)
			} else {
				target := filepath.Join(dir, "previous CLI")
				if kind == "link" {
					err = os.WriteFile(target, []byte("existing CLI"), 0700)
					if err != nil {
						t.Fatal(err)
					}
				}
				err = os.Symlink(target, path)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := linkCLI(bundle, dir); err == nil {
				t.Fatal("replaced existing command")
			}
			if kind == "file" {
				data, _ := os.ReadFile(path)
				if string(data) != "existing CLI" {
					t.Fatal("CLI changed")
				}
			} else {
				target, err := os.Readlink(path)
				if err != nil || target != filepath.Join(dir, "previous CLI") {
					t.Fatalf("link changed: %s, %v", target, err)
				}
			}
		})
	}
	dir := t.TempDir()
	first, err := linkCLI(bundle, dir)
	if err != nil {
		t.Fatal(err)
	}
	second, err := linkCLI(bundle, dir)
	if err != nil || first != second {
		t.Fatalf("not idempotent: %s %v", second, err)
	}
	if target, err := os.Readlink(first); err != nil || target != filepath.Join(bundle, "Contents", "Resources", "aims") {
		t.Fatalf("wrong target %s %v", target, err)
	}
}

func TestInvalidBundleDoesNotCreateCommand(t *testing.T) {
	bundle := fakeBundle(t, t.TempDir())
	if err := os.Remove(filepath.Join(bundle, "Contents", "Resources", "aims")); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "bin")
	if _, err := linkCLI(bundle, dir); err == nil {
		t.Fatal("accepted incomplete app")
	}
	if _, err := os.Lstat(dir); !os.IsNotExist(err) {
		t.Fatal("created output for invalid app")
	}
}
