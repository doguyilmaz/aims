package links

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/doguyilmaz/aims/internal/testutil"
	"github.com/doguyilmaz/aims/internal/tool"
)

func TestMain(m *testing.M) { testutil.Main(m) }

var layout = tool.Layout{
	Shared:     append(tool.Names("projects", "settings.json", "history.jsonl", "skills"), tool.Entry{Pattern: regexp.MustCompile(`^state_\d+\.sqlite$`)}),
	SharedDirs: []string{"projects", "skills"},
	History:    tool.Names("projects", "history.jsonl"),
}

func dirs(t *testing.T) (hub, dir string) {
	t.Helper()
	root := t.TempDir()
	return filepath.Join(root, "hub"), filepath.Join(root, "profile")
}

func ensure(hub, dir string) map[string]Report {
	out := map[string]Report{}
	for _, r := range Ensure(Options{Hub: hub, Dir: dir, Layout: layout, Label: "work"}) {
		out[r.Name] = r
	}
	return out
}

func isLink(t *testing.T, p string) bool {
	t.Helper()
	fi, err := os.Lstat(p)
	if err != nil {
		t.Fatal(err)
	}
	return fi.Mode()&fs.ModeSymlink != 0 || isJunction(p)
}

func TestEnsureLinksAndKeepsLoginPrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file links need developer mode on Windows")
	}
	hub, dir := dirs(t)
	testutil.Write(t, filepath.Join(hub, "settings.json"), `{"a":1}`)
	testutil.Write(t, filepath.Join(dir, ".credentials.json"), "secret")
	r := ensure(hub, dir)
	if r["settings.json"].Action != Linked || r["projects"].Action != Linked || r["history.jsonl"].Action != Skipped {
		t.Fatalf("reports: %+v", r)
	}
	if !isLink(t, filepath.Join(dir, "settings.json")) || !isLink(t, filepath.Join(dir, "projects")) {
		t.Fatal("not linked")
	}
	if _, ok := r[".credentials.json"]; ok || isLink(t, filepath.Join(dir, ".credentials.json")) {
		t.Fatal("the login was touched")
	}
	if _, err := os.Stat(filepath.Join(hub, ".credentials.json")); err == nil {
		t.Fatal("the login reached the hub")
	}
	// Running again changes nothing.
	for name, rep := range ensure(hub, dir) {
		if rep.Action != OK && rep.Action != Skipped {
			t.Errorf("%s: %s on the second run", name, rep.Action)
		}
	}
}

func TestEnsureAdoptsAndMerges(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file links need developer mode on Windows")
	}
	hub, dir := dirs(t)
	testutil.Write(t, filepath.Join(hub, "projects", "a", "1.jsonl"), "hub")
	testutil.Write(t, filepath.Join(hub, "projects", "same.jsonl"), "same")
	testutil.Write(t, filepath.Join(dir, "projects", "a", "1.jsonl"), "profile")
	testutil.Write(t, filepath.Join(dir, "projects", "b", "2.jsonl"), "new")
	testutil.Write(t, filepath.Join(dir, "projects", "same.jsonl"), "same")
	testutil.Write(t, filepath.Join(dir, "skills", "mine", "SKILL.md"), "skill")
	testutil.Write(t, filepath.Join(hub, "history.jsonl"), "h1\n")
	testutil.Write(t, filepath.Join(dir, "history.jsonl"), "p1\n")

	r := ensure(hub, dir)
	if r["projects"].Action != Merged || !strings.Contains(r["projects"].Detail, "1.jsonl.aims-work-") {
		t.Fatalf("projects: %+v", r["projects"])
	}
	if r["skills"].Action != Linked && r["skills"].Action != Merged && r["skills"].Action != Adopted {
		t.Fatalf("skills: %+v", r["skills"])
	}
	if testutil.Read(t, filepath.Join(hub, "projects", "a", "1.jsonl")) != "hub" || testutil.Read(t, filepath.Join(hub, "projects", "b", "2.jsonl")) != "new" {
		t.Fatal("merge lost or overwrote content")
	}
	matches, _ := filepath.Glob(filepath.Join(hub, "projects", "a", "1.jsonl.aims-work-*"))
	if len(matches) != 1 || testutil.Read(t, matches[0]) != "profile" {
		t.Fatalf("the profile's copy was not kept: %v", matches)
	}
	if got := testutil.Read(t, filepath.Join(hub, "history.jsonl")); got != "h1\np1\n" {
		t.Fatalf("history = %q", got)
	}
	if testutil.Read(t, filepath.Join(hub, "skills", "mine", "SKILL.md")) != "skill" {
		t.Fatal("skill not adopted")
	}
}

func TestEnsureNewerFileWins(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file links need developer mode on Windows")
	}
	hub, dir := dirs(t)
	testutil.Write(t, filepath.Join(hub, "settings.json"), "old")
	testutil.Write(t, filepath.Join(dir, "settings.json"), "new")
	past := time.Now().Add(-time.Hour)
	os.Chtimes(filepath.Join(hub, "settings.json"), past, past)

	r := ensure(hub, dir)
	if r["settings.json"].Action != Merged || testutil.Read(t, filepath.Join(hub, "settings.json")) != "new" {
		t.Fatalf("%+v", r["settings.json"])
	}
	backups, _ := filepath.Glob(filepath.Join(hub, "settings.json.aims-previous-*"))
	if len(backups) != 1 || testutil.Read(t, backups[0]) != "old" {
		t.Fatalf("old version not kept: %v", backups)
	}
}

func TestEnsureConflicts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file links need developer mode on Windows")
	}
	hub, dir := dirs(t)
	other := filepath.Join(t.TempDir(), "elsewhere")
	os.MkdirAll(other, 0o755)
	os.MkdirAll(dir, 0o755)
	os.Symlink(other, filepath.Join(dir, "projects"))
	testutil.Write(t, filepath.Join(hub, "settings.json", "x"), "a folder in the hub")
	testutil.Write(t, filepath.Join(dir, "settings.json"), "a file here")
	// An open database (non-empty WAL) is not moved.
	testutil.Write(t, filepath.Join(dir, "state_5.sqlite"), "db")
	testutil.Write(t, filepath.Join(dir, "state_5.sqlite-wal"), "wal")

	r := ensure(hub, dir)
	if r["projects"].Action != Conflict || r["settings.json"].Action != Conflict {
		t.Fatalf("%+v", r)
	}
	if r["state_5.sqlite"].Action != Conflict || !strings.Contains(r["state_5.sqlite"].Detail, "open") {
		t.Fatalf("sqlite: %+v", r["state_5.sqlite"])
	}
	if testutil.Read(t, filepath.Join(dir, "settings.json")) != "a file here" {
		t.Fatal("conflicting file changed")
	}
	// With Fix (tools closed) the database moves, sidecars too.
	Ensure(Options{Hub: hub, Dir: dir, Layout: layout, Fix: true})
	if testutil.Read(t, filepath.Join(hub, "state_5.sqlite-wal")) != "wal" || !isLink(t, filepath.Join(dir, "state_5.sqlite")) {
		t.Fatal("database not adopted with Fix")
	}
}

func TestPrivateAndDetach(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file links need developer mode on Windows")
	}
	hub, dir := dirs(t)
	testutil.Write(t, filepath.Join(hub, "settings.json"), `{"a":1}`)
	testutil.Write(t, filepath.Join(hub, "projects", "p", "t.jsonl"), "conversation")
	ensure(hub, dir)

	// The profile stops sharing everything: settings become a copy, history empty.
	private := func(string) bool { return true }
	reps := Detach(hub, dir, private, layout)
	if len(reps) != 3 { // projects, settings.json, skills
		t.Fatalf("detach: %+v", reps)
	}
	if isLink(t, filepath.Join(dir, "settings.json")) || testutil.Read(t, filepath.Join(dir, "settings.json")) != `{"a":1}` {
		t.Fatal("settings not copied")
	}
	if _, err := os.Lstat(filepath.Join(dir, "projects")); err == nil {
		t.Fatal("history was copied into the private profile")
	}
	if testutil.Read(t, filepath.Join(hub, "projects", "p", "t.jsonl")) != "conversation" {
		t.Fatal("detach touched the hub")
	}
	if r := Ensure(Options{Hub: hub, Dir: dir, Layout: layout, Private: private}); len(r) != 0 {
		t.Fatalf("private entries were linked: %+v", r)
	}
}

func TestRemoveLinksKeepsHub(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file links need developer mode on Windows")
	}
	hub, dir := dirs(t)
	testutil.Write(t, filepath.Join(hub, "projects", "keep.jsonl"), "keep")
	testutil.Write(t, filepath.Join(dir, "own.txt"), "own")
	ensure(hub, dir)
	if err := RemoveLinks(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if testutil.Read(t, filepath.Join(hub, "projects", "keep.jsonl")) != "keep" {
		t.Fatal("hub content deleted")
	}
}

func TestSameDirIsNoop(t *testing.T) {
	hub, _ := dirs(t)
	if r := Ensure(Options{Hub: hub, Dir: hub, Layout: layout}); r != nil {
		t.Fatalf("hub linked to itself: %+v", r)
	}
	if slices.Contains(readDir(filepath.Dir(hub)), "hub") {
		t.Fatal("created the hub for nothing")
	}
}

func TestDetachHardLink(t *testing.T) {
	hub, dir := dirs(t)
	testutil.Write(t, filepath.Join(hub, "settings.json"), "shared")
	os.MkdirAll(dir, 0o755)
	if err := os.Link(filepath.Join(hub, "settings.json"), filepath.Join(dir, "settings.json")); err != nil {
		t.Skip("no hard links here")
	}
	reps := Detach(hub, dir, func(string) bool { return true }, layout)
	if len(reps) != 1 || reps[0].Action != Detached {
		t.Fatalf("detach: %+v", reps)
	}
	testutil.Write(t, filepath.Join(dir, "settings.json"), "mine now")
	if testutil.Read(t, filepath.Join(hub, "settings.json")) != "shared" {
		t.Fatal("the profile still writes into the hub")
	}
}

// The profile folder is the hub under another name: nothing may be merged
// into itself (that would delete the history it holds).
func TestProfileAliasingHubIsRefused(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need developer mode on Windows")
	}
	hub, dir := dirs(t)
	testutil.Write(t, filepath.Join(hub, "projects", "p", "t.jsonl"), "conversation")
	if err := os.Symlink(hub, dir); err != nil {
		t.Fatal(err)
	}
	reps := Ensure(Options{Hub: hub, Dir: dir, Layout: layout})
	if len(reps) != 1 || reps[0].Action != Conflict || !strings.Contains(reps[0].Detail, "overlaps") {
		t.Fatalf("reports: %+v", reps)
	}
	if testutil.Read(t, filepath.Join(hub, "projects", "p", "t.jsonl")) != "conversation" {
		t.Fatal("history lost")
	}
	if _, err := mergeDir(filepath.Join(dir, "projects"), filepath.Join(hub, "projects"), "x"); err == nil {
		t.Fatal("merged a folder into itself")
	}
	if testutil.Read(t, filepath.Join(hub, "projects", "p", "t.jsonl")) != "conversation" {
		t.Fatal("history lost by mergeDir")
	}
}

// Two profiles repaired in the same second keep both old versions.
func TestBackupsNeverOverwrite(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file links need developer mode on Windows")
	}
	hub, dir := dirs(t)
	dir2 := dir + "2"
	past := time.Now().Add(-time.Hour)
	testutil.Write(t, filepath.Join(hub, "settings.json"), "hub")
	os.Chtimes(filepath.Join(hub, "settings.json"), past, past)
	testutil.Write(t, filepath.Join(dir, "settings.json"), "one")
	testutil.Write(t, filepath.Join(dir2, "settings.json"), "two")
	Ensure(Options{Hub: hub, Dir: dir, Layout: layout, Label: "one"})
	os.Chtimes(filepath.Join(hub, "settings.json"), past, past)
	Ensure(Options{Hub: hub, Dir: dir2, Layout: layout, Label: "two"})

	seen := map[string]bool{}
	files, _ := filepath.Glob(filepath.Join(hub, "settings.json*"))
	for _, f := range files {
		seen[testutil.Read(t, f)] = true
	}
	if !seen["hub"] || !seen["one"] || !seen["two"] {
		t.Fatalf("a version was lost: %v", files)
	}
}

func TestMoveNeverReplaces(t *testing.T) {
	root := t.TempDir()
	a, b := filepath.Join(root, "a"), filepath.Join(root, "b")
	testutil.Write(t, a, "a")
	testutil.Write(t, b, "b")
	if err := move(a, b); err == nil || testutil.Read(t, b) != "b" {
		t.Fatal("move replaced an existing file")
	}
	if unique(b) == b || unique(filepath.Join(root, "c")) != filepath.Join(root, "c") {
		t.Fatal("unique")
	}
}
