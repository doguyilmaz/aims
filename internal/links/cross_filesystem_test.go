package links

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/testutil"
)

func TestCrossFilesystemPreflightKeepsSourceLinks(t *testing.T) {
	other := os.Getenv("AIMS_TEST_SECOND_FS")
	if other == "" {
		t.Skip("a second scratch filesystem is required")
	}
	src := filepath.Join(t.TempDir(), "skills")
	testutil.Write(t, filepath.Join(src, "z-file"), "private skill")
	link := filepath.Join(src, "a-link")
	if err := os.Symlink(filepath.Join(src, "z-file"), link); err != nil {
		t.Skip(err)
	}
	root, err := os.MkdirTemp(other, "aims-crossfs-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	if same, err := sameFilesystem(src, root); err != nil || same {
		t.Fatalf("second filesystem unavailable: %v, %v", same, err)
	}
	dst := filepath.Join(root, "skills")
	if _, err := mergeDir(src, dst, "work"); err == nil {
		t.Fatal("cross-filesystem adoption was accepted")
	}
	if got, err := os.Readlink(link); err != nil || got != filepath.Join(src, "z-file") {
		t.Fatalf("source link changed before rejecting move: %q %v", got, err)
	}
	if testutil.Read(t, link) != "private skill" || fsx.Lstat(dst) != nil {
		t.Fatal("cross-filesystem preflight changed a source or destination")
	}
}
