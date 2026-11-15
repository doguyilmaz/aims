package links

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/testutil"
	"github.com/doguyilmaz/aims/internal/tool"
)

func TestCheckDoesNotCreateOrReconcile(t *testing.T) {
	hub, dir := dirs(t)
	reports := Check(Options{Hub: hub, Dir: dir, Layout: layout})
	if len(reports) == 0 || fsx.Lstat(hub) != nil || fsx.Lstat(dir) != nil {
		t.Fatal("checking missing links changed the filesystem")
	}
	testutil.Write(t, filepath.Join(hub, "settings.json"), "shared")
	testutil.Write(t, filepath.Join(dir, "settings.json"), "independent")
	for _, r := range Check(Options{Hub: hub, Dir: dir, Layout: layout}) {
		if r.Name == "settings.json" && (r.Action != Conflict || r.Repairable) {
			t.Fatalf("independent settings offered for automatic repair: %+v", r)
		}
	}
	if testutil.Read(t, filepath.Join(dir, "settings.json")) != "independent" || testutil.Read(t, filepath.Join(hub, "settings.json")) != "shared" {
		t.Fatal("check reconciled settings")
	}
}

func TestCheckUnsharedDisposableCopy(t *testing.T) {
	hub, dir := dirs(t)
	l := layout
	l.Disposable = []tool.Entry{{Pattern: regexp.MustCompile(`^logs_\d+\.sqlite$`)}}
	testutil.Write(t, filepath.Join(dir, "logs_2.sqlite"), "old")
	testutil.Write(t, filepath.Join(hub, "logs_2.sqlite"), "shared")
	for _, stateShared := range []bool{false, true} {
		found := false
		for _, r := range Check(Options{Hub: hub, Dir: dir, Layout: l, StateShared: stateShared}) {
			if r.Name == "logs_2.sqlite" {
				found = r.Action == Unused && r.Repairable
			}
		}
		if found != stateShared {
			t.Fatalf("unused database classification with shared state %v: %v", stateShared, found)
		}
	}
}

func TestAdoptionRebasesSkillLinks(t *testing.T) {
	hub, dir := dirs(t)
	source := filepath.Join(dir, "skills")
	testutil.Write(t, filepath.Join(source, "mine", "SKILL.md"), "skill")
	external := filepath.Join(filepath.Dir(dir), "external")
	testutil.Write(t, external, "external skill")
	for name, target := range map[string]string{"internal": filepath.Join(source, "mine"), "relative": "mine", "external": "../../external"} {
		if err := os.Symlink(target, filepath.Join(source, name)); err != nil {
			t.Skip(err)
		}
	}
	if r := ensure(hub, dir)["skills"]; r.Action != Adopted {
		t.Fatalf("adoption: %+v", r)
	}
	for _, name := range []string{"internal", "relative"} {
		if testutil.Read(t, filepath.Join(hub, "skills", name, "SKILL.md")) != "skill" {
			t.Fatal("internal link no longer reaches the skill")
		}
	}
	if testutil.Read(t, filepath.Join(hub, "skills", "external")) != "external skill" {
		t.Fatal("relative external link changed meaning")
	}
}

func TestMergeKeepsUnrelatedSymlink(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	testutil.Write(t, filepath.Join(src, "ours"), "ours")
	external := filepath.Join(root, "external")
	testutil.Write(t, filepath.Join(external, "keep"), "external")
	// The caller rejects a top-level unrelated link; merging a tree must not
	// recursively follow a nested destination link either.
	testutil.Write(t, filepath.Join(src, "nested", "ours"), "nested ours")
	os.MkdirAll(filepath.Join(root, "hub"), 0o755)
	if err := os.Symlink(external, filepath.Join(root, "hub", "nested")); err != nil {
		t.Fatal(err)
	}
	if _, err := mergeDir(src, filepath.Join(root, "hub"), "work"); err != nil {
		t.Fatal(err)
	}
	if fsx.Lstat(filepath.Join(external, "ours")) != nil || testutil.Read(t, filepath.Join(external, "keep")) != "external" {
		t.Fatal("merge traversed an unrelated destination symlink")
	}
}
