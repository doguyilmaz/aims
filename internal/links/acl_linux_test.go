package links

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/doguyilmaz/aims/internal/testutil"
	"golang.org/x/sys/unix"
)

func TestAdoptionPreservesUnixACLGroupAndSetgid(t *testing.T) {
	hub, dir := dirs(t)
	src := filepath.Join(dir, "skills")
	testutil.Write(t, filepath.Join(src, "private", "SKILL.md"), "private instructions")
	// A named user has no access even though ordinary mode bits are 0755.
	acl := []byte{2, 0, 0, 0}
	for _, e := range []struct {
		tag, perm uint16
		id        uint32
	}{
		{1, 7, ^uint32(0)}, {2, 0, 65533}, {4, 5, ^uint32(0)},
		{16, 5, ^uint32(0)}, {32, 5, ^uint32(0)},
	} {
		acl = binary.LittleEndian.AppendUint16(acl, e.tag)
		acl = binary.LittleEndian.AppendUint16(acl, e.perm)
		acl = binary.LittleEndian.AppendUint32(acl, e.id)
	}
	before := map[string]uint32{}
	for _, path := range []string{src, filepath.Join(src, "private")} {
		if err := unix.Setxattr(path, "system.posix_acl_access", acl, 0); errors.Is(err, unix.ENOTSUP) {
			t.Skip("fixture filesystem does not support POSIX ACLs")
		} else if err != nil {
			t.Fatal(err)
		}
		if err := unix.Setxattr(path, "system.posix_acl_default", acl, 0); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, 0o755|os.ModeSetgid); err != nil {
			t.Fatal(err)
		}
		fi, _ := os.Stat(path)
		before[filepath.Base(path)] = fi.Sys().(*syscall.Stat_t).Gid
	}
	if r := ensure(hub, dir)["skills"]; r.Action != Adopted {
		t.Fatalf("adoption: %+v", r)
	}
	for _, path := range []string{filepath.Join(hub, "skills"), filepath.Join(hub, "skills", "private")} {
		fi, err := os.Stat(path)
		if err != nil || fi.Mode()&os.ModeSetgid == 0 || fi.Sys().(*syscall.Stat_t).Gid != before[filepath.Base(path)] {
			t.Fatalf("group/setgid changed: %s %v", path, err)
		}
		for _, name := range []string{"system.posix_acl_access", "system.posix_acl_default"} {
			b := make([]byte, 4096)
			n, err := unix.Getxattr(path, name, b)
			if err != nil || !bytes.Equal(b[:max(n, 0)], acl) {
				t.Fatalf("%s not preserved at %s: %v", name, path, err)
			}
		}
	}
}
