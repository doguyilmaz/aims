package codex

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/doguyilmaz/aims/internal/links"
	"github.com/doguyilmaz/aims/internal/testutil"
)

// A sparse database represents large history without allocating its disk
// contents. Repeated synchronization must only inspect metadata and links.
func sharedHistoryFixture(t testing.TB, size int64) links.Options {
	t.Helper()
	hub, profile := t.TempDir(), t.TempDir()
	path := filepath.Join(hub, "thread_history_1.sqlite")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(size); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	for i := range 200 {
		testutil.Write(t, filepath.Join(hub, "sessions", "2026", "01", "01", fmt.Sprintf("rollout-%d.jsonl", i)), "history\n")
	}
	o := links.Options{Hub: hub, Dir: profile, Layout: New().Layout()}
	for _, r := range links.Ensure(o) {
		if r.Action == links.Conflict {
			t.Fatalf("setup: %+v", r)
		}
	}
	return o
}

func TestSharedHistorySyncDoesNotCopy(t *testing.T) {
	size := int64(1 << 30)
	if runtime.GOOS == "windows" {
		// Truncate need not produce a sparse file on Windows.
		size = 1 << 20
	}
	o := sharedHistoryFixture(t, size)
	path := filepath.Join(o.Hub, "thread_history_1.sqlite")
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	hubBefore, err := os.ReadDir(o.Hub)
	if err != nil {
		t.Fatal(err)
	}
	for range 5 {
		for _, r := range links.Ensure(o) {
			if r.Action != links.OK && r.Action != links.Skipped {
				t.Fatalf("warm sync modified shared data: %+v", r)
			}
		}
		if !links.HistoryShared(o.Hub, o.Dir, o.Layout, false) {
			t.Fatal("shared history rejected")
		}
	}
	linked, err := os.Stat(filepath.Join(o.Dir, "thread_history_1.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, linked) || before.Size() != linked.Size() || !before.ModTime().Equal(linked.ModTime()) {
		t.Fatal("database was copied or rewritten")
	}
	hubAfter, err := os.ReadDir(o.Hub)
	if err != nil {
		t.Fatal(err)
	}
	if len(hubBefore) != len(hubAfter) {
		t.Fatal("warm sync created extra files or backups")
	}
}

func BenchmarkSharedHistory(b *testing.B) {
	for _, size := range []int64{1024, 1 << 30} {
		b.Run(fmt.Sprintf("bytes=%d", size), func(b *testing.B) {
			if runtime.GOOS == "windows" && size > 1<<20 {
				b.Skip("large sparse fixture is only used on Unix")
			}
			o := sharedHistoryFixture(b, size)
			b.Run("sync", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					links.Ensure(o)
				}
			})
			b.Run("move-check", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if !links.HistoryShared(o.Hub, o.Dir, o.Layout, false) {
						b.Fatal("history not shared")
					}
				}
			})
		})
	}
}
