package links

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/doguyilmaz/aims/internal/testutil"
	"github.com/doguyilmaz/aims/internal/tool"
)

func TestWindowsFixKeepsDatabaseWithOpenRecoverySidecar(t *testing.T) {
	hub, dir := dirs(t)
	const name = "logs_2.sqlite"
	l := layout
	l.Shared = append(l.Shared, tool.Names(name)...)
	l.Disposable = tool.Names(name)
	testutil.Write(t, filepath.Join(hub, name), "shared")
	files := map[string]string{
		name: "private database", name + "-wal": "private wal",
		name + "-shm": "private shm", name + "-journal": "private journal",
	}
	for base, contents := range files {
		testutil.Write(t, filepath.Join(dir, base), contents)
	}
	// Simulate a provider retaining its recovery handle while --fix runs.
	open, err := os.Open(filepath.Join(dir, name+"-wal"))
	if err != nil {
		t.Fatal(err)
	}
	defer open.Close()
	var found bool
	for _, r := range Ensure(Options{Hub: hub, Dir: dir, Layout: l, StateShared: true, Fix: true}) {
		if r.Name == name {
			found = true
			if r.Action != Unused {
				t.Fatalf("open recovery file did not prevent archival: %+v", r)
			}
		}
	}
	if !found {
		t.Fatal("missing database repair report")
	}
	for base, want := range files {
		if got := testutil.Read(t, filepath.Join(dir, base)); got != want {
			t.Fatalf("repair changed %s: %q", base, got)
		}
	}
	if got := testutil.Read(t, filepath.Join(hub, name)); got != "shared" {
		t.Fatal("repair changed the shared database")
	}
	archives, err := filepath.Glob(filepath.Join(dir, ".aims-recovery-*"))
	if err != nil || len(archives) != 0 {
		t.Fatalf("repair created an archive despite a live recovery handle: %v, %v", archives, err)
	}
}
