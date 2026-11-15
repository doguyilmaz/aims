package desktop

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBinaryBundleMetadata(t *testing.T) {
	bundle := fakeBundle(t, t.TempDir())
	path := filepath.Join(bundle, "Contents", "Info.plist")
	if err := exec.Command("/usr/bin/plutil", "-convert", "binary1", path).Run(); err != nil {
		t.Fatal(err)
	}
	if version, err := readBundle(bundle); err != nil || version != "0.3.0" {
		t.Fatalf("binary metadata not recognized: %s %v", version, err)
	}
}

func TestInvalidSignatureLeavesNoAppOrStagingFiles(t *testing.T) {
	source := fakeBundle(t, t.TempDir())
	dir := t.TempDir()
	if err := installBundle(context.Background(), source, filepath.Join(dir, "Aims Usage.app")); err == nil {
		t.Fatal("accepted unsigned fixture")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("partial installation left behind: %v %v", entries, err)
	}
}

func TestLocalInstallKeepsExistingAppAndLinks(t *testing.T) {
	from := fakeBundle(t, t.TempDir())
	for _, link := range []bool{false, true} {
		dir := t.TempDir()
		dest := filepath.Join(dir, "Aims Usage.app")
		kept := filepath.Join(dir, "original")
		if err := os.MkdirAll(kept, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(kept, "keep"), []byte("original data"), 0600); err != nil {
			t.Fatal(err)
		}
		var err error
		if link {
			err = os.Symlink(kept, dest)
		} else {
			err = os.Rename(kept, dest)
			kept = dest
		}
		if err != nil {
			t.Fatal(err)
		}
		if err := installBundle(context.Background(), from, dest); err == nil {
			t.Fatal("replaced existing app")
		}
		data, _ := os.ReadFile(filepath.Join(kept, "keep"))
		if string(data) != "original data" {
			t.Fatal("existing data changed")
		}
	}
}

func TestBrewInstallKeepsExistingCLIInstaller(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "args")
	if err := os.WriteFile(filepath.Join(dir, "aims"), []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	// The fixture only records arguments; no package manager is run.
	if err := os.WriteFile(filepath.Join(dir, "brew"), []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$AIMS_TEST_BREW_ARGS\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("AIMS_TEST_BREW_ARGS", log)
	if err := Install(context.Background(), "", nil, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	args, _ := os.ReadFile(log)
	if string(args) != "install\n--cask\n--skip-cask-deps\n"+Cask+"\n" {
		t.Fatalf("unexpected brew args: %s", args)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "aims"))
	if !strings.Contains(string(data), "exit 0") {
		t.Fatal("existing CLI changed")
	}
}
