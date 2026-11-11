package fsx

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
)

func TestDarwinAccessRestorerPreservesDenyACLAndGroup(t *testing.T) {
	root := t.TempDir()
	source, destination := filepath.Join(root, "source"), filepath.Join(root, "destination")
	if err := os.Mkdir(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("/bin/chmod", "+a", "user:nobody deny list,search", source).CombinedOutput(); err != nil {
		t.Fatalf("set fixture ACL: %v %s", err, out)
	}
	before, err := readAccessACL(source, true)
	if err != nil || len(before) == 0 {
		t.Fatalf("read fixture ACL: %v", err)
	}
	si, err := os.Stat(source)
	if err != nil {
		t.Fatal(err)
	}
	restore, err := AccessRestorer(source, destination)
	if err != nil {
		t.Fatal(err)
	}
	if err := MkdirPrivate(destination); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	if err := restore(); err != nil {
		t.Fatal(err)
	}
	after, err := readAccessACL(destination, true)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("deny ACL changed after source removal: %v", err)
	}
	di, err := os.Stat(destination)
	if err != nil || si.Sys().(*syscall.Stat_t).Gid != di.Sys().(*syscall.Stat_t).Gid {
		t.Fatalf("group changed: %v", err)
	}
}

func TestDarwinPrivateCreationRemovesInheritedACLBeforeData(t *testing.T) {
	root := t.TempDir()
	if out, err := exec.Command("/bin/chmod", "+a", "everyone allow read,file_inherit,directory_inherit", root).CombinedOutput(); err != nil {
		t.Fatalf("set fixture inheritance: %v %s", err, out)
	}
	file, dir := filepath.Join(root, "export.json"), filepath.Join(root, "directory")
	f, err := CreatePrivateFile(file)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := MkdirPrivate(dir); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{file, dir} {
		acl, err := readAccessACL(path, path == dir)
		if err != nil || len(acl) != 0 {
			t.Fatalf("inherited ACL grants remain at %s: %v", path, err)
		}
	}
	if _, err := f.WriteString(`{"apiKey":"DUMMY-SECRET"}`); err != nil {
		t.Fatal(err)
	}
}
