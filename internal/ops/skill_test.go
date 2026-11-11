package ops

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/testutil"
)

func TestSkillUpdateKeepsPreviousTextOnce(t *testing.T) {
	root := fsx.Real(t.TempDir())
	path := filepath.Join(root, "skills", "aims", "SKILL.md")
	testutil.Write(t, path, "local edits")
	for range 2 {
		if err := writeSkill(path, []byte("new skill")); err != nil {
			t.Fatal(err)
		}
	}
	backups, err := filepath.Glob(filepath.Join(root, "skills", ".aims-skill-*.bak"))
	if err != nil || len(backups) != 1 || testutil.Read(t, backups[0]) != "local edits" {
		t.Fatalf("previous skill not preserved once: %v, %v", backups, err)
	}
	if info, err := os.Stat(backups[0]); err != nil || runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatal("recovery copy must be private")
	}
	if testutil.Read(t, path) != "new skill" {
		t.Fatal("skill not updated")
	}
}

func TestSkillUpdateRejectsExternallyManagedPath(t *testing.T) {
	root := fsx.Real(t.TempDir())
	outside := filepath.Join(root, "owner", "SKILL.md")
	testutil.Write(t, outside, "owned by dotfiles")
	path := filepath.Join(root, "skills", "aims", "SKILL.md")
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.Symlink(outside, path); err != nil {
		t.Skip(err)
	}
	if err := writeSkill(path, []byte("new skill")); err == nil {
		t.Fatal("overwrote externally managed skill")
	}
	if testutil.Read(t, outside) != "owned by dotfiles" {
		t.Fatal("external skill changed")
	}
}

func TestSkillUpdateKeepsDanglingManagedLink(t *testing.T) {
	root := fsx.Real(t.TempDir())
	path := filepath.Join(root, "skills", "aims", "SKILL.md")
	target := filepath.Join(root, "manager", "missing-skill.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Skip(err)
	}
	if err := writeSkill(path, []byte("new skill")); err == nil {
		t.Fatal("replaced a dangling managed skill link")
	}
	if got, err := os.Readlink(path); err != nil || got != target {
		t.Fatalf("managed link changed: %q, %v", got, err)
	}
	if fsx.Lstat(target) != nil {
		t.Fatal("created an external manager's missing file")
	}
}
