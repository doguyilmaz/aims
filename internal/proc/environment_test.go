package proc

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestAttachedUsesPreparedPATH(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	name := "aims-isolated-child"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	bin := filepath.Join(dir, name)
	// A Windows image mapping locks every hard link to the executable,
	// including this alias while the parent test process is still running.
	if runtime.GOOS != "windows" {
		err = os.Link(self, bin)
	}
	if runtime.GOOS == "windows" || err != nil {
		b, e := os.ReadFile(self)
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(bin, b, 0o755); e != nil {
			t.Fatal(e)
		}
	}
	env := []string{"PATH=" + dir, "TMPDIR=" + os.TempDir(), "AIMS_PROC_PATH_HELPER=1"}
	resolved, err := LookPathEnv(name, env)
	if err != nil || resolved != bin {
		t.Fatalf("prepared PATH lookup: %s %v", resolved, err)
	}
	code, err := RunAttached(name, []string{"-test.run=^TestAttachedPATHHelper$"}, env, "")
	if err != nil || code != 23 {
		t.Fatalf("child did not use prepared PATH: %d %v", code, err)
	}
}

func TestAttachedPATHHelper(t *testing.T) {
	if os.Getenv("AIMS_PROC_PATH_HELPER") == "1" {
		os.Exit(23)
	}
}

func TestPrependPathPreservesExistingEntries(t *testing.T) {
	key := "PATH"
	if runtime.GOOS == "windows" {
		key = "Path"
	}
	env := []string{key + "=/native/tools", "OTHER=value"}
	out := PrependPath(env, "/aims/bridge")
	value, ok := EnvValue(out, "PATH")
	if !ok || value != "/aims/bridge"+string(os.PathListSeparator)+"/native/tools" {
		t.Fatalf("lost native PATH: %v", out)
	}
	if env[0] != key+"=/native/tools" {
		t.Fatal("changed parent environment")
	}
}
