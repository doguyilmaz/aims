package fsx

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

func TestPrivateCreationClearsInheritedACLBeforeData(t *testing.T) {
	root := t.TempDir()
	// owner/group/other all have full access in this inheritable default ACL.
	acl := []byte{2, 0, 0, 0, 1, 0, 7, 0, 255, 255, 255, 255, 4, 0, 7, 0, 255, 255, 255, 255, 32, 0, 7, 0, 255, 255, 255, 255}
	if err := unix.Setxattr(root, "system.posix_acl_default", acl, 0); errors.Is(err, unix.ENOTSUP) {
		t.Skip("fixture filesystem does not support POSIX ACLs")
	} else if err != nil {
		t.Fatal(err)
	}
	file, dir := filepath.Join(root, "private-file"), filepath.Join(root, "private-directory")
	f, err := CreatePrivateFile(file)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := MkdirPrivate(dir); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{file, dir} {
		fi, err := os.Stat(path)
		if err != nil || fi.Mode().Perm()&0o077 != 0 {
			t.Fatalf("private creation widened permissions: %s %v", path, err)
		}
		if _, err := unix.Getxattr(path, "system.posix_acl_access", nil); !errors.Is(err, unix.ENODATA) {
			t.Fatalf("inherited access ACL kept: %s %v", path, err)
		}
	}
	if _, err := unix.Getxattr(dir, "system.posix_acl_default", nil); !errors.Is(err, unix.ENODATA) {
		t.Fatalf("private directory still propagates another ACL: %v", err)
	}
	if _, err := f.WriteString("DUMMY-SECRET"); err != nil {
		t.Fatal(err)
	}
}

func TestAtomicWritePreservesUnixAccessControls(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	acl := []byte{2, 0, 0, 0}
	for _, e := range []struct {
		tag, perm uint16
		id        uint32
	}{
		{1, 6, ^uint32(0)}, {2, 0, 65533}, {4, 4, ^uint32(0)},
		{16, 4, ^uint32(0)}, {32, 4, ^uint32(0)},
	} {
		acl = binary.LittleEndian.AppendUint16(acl, e.tag)
		acl = binary.LittleEndian.AppendUint16(acl, e.perm)
		acl = binary.LittleEndian.AppendUint32(acl, e.id)
	}
	if err := unix.Setxattr(path, "system.posix_acl_access", acl, 0); errors.Is(err, unix.ENOTSUP) {
		t.Skip("fixture filesystem does not support POSIX ACLs")
	} else if err != nil {
		t.Fatal(err)
	}
	before, _ := os.Stat(path)
	if err := WriteFile(path, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(path)
	bs, as := before.Sys().(*syscall.Stat_t), after.Sys().(*syscall.Stat_t)
	if bs.Uid != as.Uid || bs.Gid != as.Gid || before.Mode() != after.Mode() {
		t.Fatal("atomic settings write changed ownership or mode")
	}
	b := make([]byte, 4096)
	n, err := unix.Getxattr(path, "system.posix_acl_access", b)
	if err != nil || !bytes.Equal(b[:max(n, 0)], acl) {
		t.Fatalf("atomic write discarded a named-user restriction: %v", err)
	}
	if content, err := os.ReadFile(path); err != nil || string(content) != "new" {
		t.Fatal("atomic write did not update content")
	}
}
