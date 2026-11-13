package links

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/doguyilmaz/aims/internal/testutil"
)

func TestMergeNormalizesAbsoluteInternalLinkBeforeAncestorWalk(t *testing.T) {
	root := t.TempDir()
	src, dst := filepath.Join(root, "source"), filepath.Join(root, "destination")
	testutil.Write(t, filepath.Join(src, "private.txt"), "keep private content")
	// filepath.Join would remove the very spelling that caused the loop.
	raw := src + string(filepath.Separator) + ".." + string(filepath.Separator) + filepath.Base(src)
	if err := os.Symlink(raw, filepath.Join(src, "root-link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, self, "-test.run=^TestMergeNoncanonicalLinkHelper$")
	cmd.Env = append(os.Environ(), "AIMS_TEST_LINK_SOURCE="+src, "AIMS_TEST_LINK_DEST="+dst)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("noncanonical link reconciliation did not finish: %v\n%s", err, out)
	}
	if got, err := os.Readlink(filepath.Join(dst, "root-link")); err != nil || got != dst {
		t.Fatalf("internal root link was not rebased: %q %v", got, err)
	}
	if testutil.Read(t, filepath.Join(dst, "private.txt")) != "keep private content" {
		t.Fatal("normalization lost content")
	}
	if _, err := os.Lstat(src); !os.IsNotExist(err) {
		t.Fatal("successful adoption kept the old source tree")
	}
}

func TestMergeNoncanonicalLinkHelper(t *testing.T) {
	src := os.Getenv("AIMS_TEST_LINK_SOURCE")
	if src == "" {
		return
	}
	if _, err := mergeDir(src, os.Getenv("AIMS_TEST_LINK_DEST"), "work"); err != nil {
		t.Fatal(err)
	}
}

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
