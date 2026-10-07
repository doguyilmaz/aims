package fsx

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestHomePaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if ExpandHome("~/x") != filepath.Join(home, "x") || ExpandHome("~") != home || ExpandHome("a/~") != "a/~" {
		t.Fatal("ExpandHome")
	}
	if Tildify(filepath.Join(home, "a")) != "~"+string(filepath.Separator)+"a" || Tildify(home+"x") != home+"x" {
		t.Fatal("Tildify must stop at a path boundary")
	}
}

func TestWithin(t *testing.T) {
	root := t.TempDir()
	cases := []struct {
		child, parent string
		want          bool
	}{
		{root, root, true},
		{filepath.Join(root, "a", "b"), root, true},
		{root + "x", root, false},
		{filepath.Dir(root), root, false},
		{filepath.Join(root, "..", filepath.Base(root), "a"), root, true},
	}
	for _, c := range cases {
		if got := Within(c.child, c.parent); got != c.want {
			t.Errorf("Within(%q, %q) = %v", c.child, c.parent, got)
		}
	}
}

func TestWriteFileThroughSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need developer mode on Windows")
	}
	dir := t.TempDir()
	real := filepath.Join(dir, "dotfiles", "settings.json")
	os.MkdirAll(filepath.Dir(real), 0o755)
	os.WriteFile(real, []byte("old"), 0o644)
	link := filepath.Join(dir, "settings.json")
	os.Symlink(real, link)

	if err := WriteFile(link, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Lstat(link); fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("the symlink was replaced")
	}
	if b, _ := os.ReadFile(real); string(b) != "new" {
		t.Fatalf("target = %q", b)
	}
	// An existing file keeps its mode; a new one gets the requested mode.
	if fi, _ := os.Stat(real); fi.Mode().Perm() != 0o644 {
		t.Fatalf("perm = %v", fi.Mode().Perm())
	}
	fresh := filepath.Join(dir, "new.json")
	if err := WriteFile(fresh, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(fresh); fi.Mode().Perm() != 0o600 {
		t.Fatalf("new file perm = %v", fi.Mode().Perm())
	}
	if m, _ := filepath.Glob(filepath.Join(dir, "dotfiles", ".*.tmp")); len(m) != 0 {
		t.Fatalf("temp files left: %v", m)
	}
}

func TestReadJSON(t *testing.T) {
	dir := t.TempDir()
	var v map[string]int
	if ok, err := ReadJSON(filepath.Join(dir, "missing.json"), &v); ok || err != nil {
		t.Fatal("missing file")
	}
	bad := filepath.Join(dir, "bad.json")
	os.WriteFile(bad, []byte(`{"a":`), 0o644)
	if _, err := ReadJSON(bad, &v); err == nil {
		t.Fatal("broken JSON must be an error")
	}
	if ReadJSONLenient(bad, &v) {
		t.Fatal("lenient read reported success")
	}
	good := filepath.Join(dir, "good.json")
	if err := WriteJSON(good, map[string]int{"a": 1}, 0o600); err != nil {
		t.Fatal(err)
	}
	if ok, err := ReadJSON(good, &v); !ok || err != nil || v["a"] != 1 {
		t.Fatalf("round trip: %v %v %v", ok, err, v)
	}
}

func TestWithLockSerializes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "counter")
	os.WriteFile(path, []byte("0"), 0o644)
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = WithLock(path, func() error {
				b, _ := os.ReadFile(path)
				n, _ := strconv.Atoi(string(b))
				return WriteFile(path, []byte(strconv.Itoa(n+1)), 0o644)
			})
		}()
	}
	wg.Wait()
	if b, _ := os.ReadFile(path); string(b) != "20" {
		t.Fatalf("lost updates: %s", b)
	}
	if _, err := os.Stat(path + ".lock"); err == nil {
		t.Fatal("lock file left behind")
	}
}

func TestInsideSeesThroughLinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need developer mode on Windows")
	}
	root := t.TempDir()
	hub := filepath.Join(root, ".claude")
	os.MkdirAll(hub, 0o755)
	alias := filepath.Join(root, "alias")
	os.Symlink(hub, alias)
	for _, c := range []string{alias, filepath.Join(alias, "work"), filepath.Join(alias, "not", "yet")} {
		if !Inside(c, hub) {
			t.Errorf("%s is inside the hub", c)
		}
	}
	if Inside(filepath.Join(root, "elsewhere"), hub) || !Inside(hub, root) {
		t.Error("Inside is too eager")
	}
	if Real(filepath.Join(alias, "x", "y")) != filepath.Join(Real(hub), "x", "y") {
		t.Errorf("Real = %s", Real(filepath.Join(alias, "x", "y")))
	}
}

func TestLockTimesOut(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	unlock, ok, err := Lock(path, time.Second)
	if err != nil || !ok {
		t.Fatal("first lock")
	}
	if _, ok, _ := Lock(path, 50*time.Millisecond); ok {
		t.Fatal("lock taken twice")
	}
	unlock()
	if u, ok, _ := Lock(path, 50*time.Millisecond); !ok {
		t.Fatal("lock not released")
	} else {
		u()
	}
}
