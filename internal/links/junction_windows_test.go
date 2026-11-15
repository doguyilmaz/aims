package links

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/doguyilmaz/aims/internal/testutil"
)

func TestCopyTreePreservesExternalJunctionReference(t *testing.T) {
	root := t.TempDir()
	src, dst, external := filepath.Join(root, "src"), filepath.Join(root, "dst"), filepath.Join(root, "external")
	testutil.Write(t, filepath.Join(external, "SKILL.md"), "managed elsewhere")
	if err := os.Mkdir(src, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := junction(external, filepath.Join(src, "linked-skill")); err != nil {
		t.Fatal(err)
	}
	if err := copyTree(src, dst); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dst, "linked-skill")
	info, err := os.Lstat(link)
	if err != nil || info.Mode()&os.ModeSymlink == 0 && !isJunction(link) {
		t.Fatal("external junction was copied as a real directory")
	}
	testutil.Write(t, filepath.Join(external, "SKILL.md"), "updated by owner")
	if testutil.Read(t, filepath.Join(link, "SKILL.md")) != "updated by owner" {
		t.Fatal("copy stopped following the external owner")
	}
}
