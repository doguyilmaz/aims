package links

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/doguyilmaz/aims/internal/testutil"
)

func TestAdoptionPreservesDirectoryPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions")
	}
	hub, dir := dirs(t)
	private := filepath.Join(dir, "skills")
	testutil.Write(t, filepath.Join(private, "private", "SKILL.md"), "private instructions")
	for _, p := range []string{private, filepath.Join(private, "private")} {
		if err := os.Chmod(p, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if r := ensure(hub, dir)["skills"]; r.Action != Adopted {
		t.Fatalf("adoption: %+v", r)
	}
	for _, p := range []string{filepath.Join(hub, "skills"), filepath.Join(hub, "skills", "private")} {
		info, err := os.Stat(p)
		if err != nil || info.Mode().Perm() != 0o700 {
			t.Fatalf("private directory permissions widened: %s, %v", p, err)
		}
	}
}

func TestCopyPopulatesReadOnlyDirectoriesBeforeRestoringMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions")
	}
	for _, mode := range []os.FileMode{0o500, 0o555} {
		t.Run(mode.String(), func(t *testing.T) {
			root := t.TempDir()
			src, dst := filepath.Join(root, "src"), filepath.Join(root, "dst")
			testutil.Write(t, filepath.Join(src, "nested", "file"), "keep")
			for _, p := range []string{src, filepath.Join(src, "nested")} {
				if err := os.Chmod(p, mode); err != nil {
					t.Fatal(err)
				}
			}
			t.Cleanup(func() {
				for _, p := range []string{src, filepath.Join(src, "nested"), dst, filepath.Join(dst, "nested")} {
					os.Chmod(p, 0o700)
				}
			})
			if err := copyTree(src, dst); err != nil {
				t.Fatal(err)
			}
			if testutil.Read(t, filepath.Join(dst, "nested", "file")) != "keep" {
				t.Fatal("copy lost data")
			}
			for _, p := range []string{dst, filepath.Join(dst, "nested")} {
				info, err := os.Stat(p)
				if err != nil || info.Mode().Perm() != mode {
					t.Fatalf("mode not restored: %s %v", p, err)
				}
			}
		})
	}
}
