package links

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/doguyilmaz/aims/internal/testutil"
)

func TestMergeKeepsInternalLinksBeforeConflictingTargetRename(t *testing.T) {
	for _, targetExists := range []bool{true, false} {
		t.Run(map[bool]string{true: "conflicting-file", false: "dangling-reference"}[targetExists], func(t *testing.T) {
			src, dst := filepath.Join(t.TempDir(), "source"), filepath.Join(t.TempDir(), "destination")
			if err := os.Mkdir(src, 0o700); err != nil {
				t.Fatal(err)
			}
			testutil.Write(t, filepath.Join(dst, "settings"), "hub settings")
			if targetExists {
				testutil.Write(t, filepath.Join(src, "settings"), "private settings")
			}
			link := filepath.Join(src, "a-link")
			if err := os.Symlink("settings", link); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
			if _, err := mergeDir(src, dst, "work"); err == nil {
				t.Fatal("merge silently retargeted internal link to another file")
			}
			if raw, err := os.Readlink(link); err != nil || raw != "settings" {
				t.Fatal("preflight refusal changed source link")
			}
			if testutil.Read(t, filepath.Join(dst, "settings")) != "hub settings" {
				t.Fatal("preflight refusal changed destination")
			}
			if targetExists && testutil.Read(t, filepath.Join(src, "settings")) != "private settings" {
				t.Fatal("preflight refusal moved private target")
			}
		})
	}
}
