package fsx

import (
	"encoding/binary"
	"fmt"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

type accessACL []byte

// getattrlist/setattrlist round-trip the kernel's opaque kauth_filesec ACL.
// See Apple's getattrlist(2), ATTR_CMN_EXTENDED_SECURITY and sys/kauth.h.
func readAccessACL(path string, _ bool) (accessACL, error) {
	name, err := syscall.BytePtrFromString(path)
	if err != nil {
		return nil, err
	}
	attrs := unix.Attrlist{Bitmapcount: 5, Commonattr: unix.ATTR_CMN_EXTENDED_SECURITY}
	b := make([]byte, 64<<10)
	_, _, errno := syscall.Syscall6(unix.SYS_GETATTRLIST, uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&attrs)), uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)), unix.FSOPT_NOFOLLOW|unix.FSOPT_REPORT_FULLSIZE, 0)
	runtime.KeepAlive(name)
	runtime.KeepAlive(&attrs)
	runtime.KeepAlive(b)
	if errno != 0 {
		return nil, errno
	}
	size := uint64(binary.LittleEndian.Uint32(b))
	offset := int64(int32(binary.LittleEndian.Uint32(b[4:]))) + 4
	length := uint64(binary.LittleEndian.Uint32(b[8:]))
	if size < 12 || size > uint64(len(b)) {
		return nil, fmt.Errorf("invalid or oversized ACL attribute")
	}
	if length == 0 {
		return nil, nil
	}
	if offset < 12 || uint64(offset)+length > size {
		return nil, fmt.Errorf("invalid ACL attribute reference")
	}
	return accessACL(append([]byte(nil), b[int(offset):uint64(offset)+length]...)), nil
}

func (a accessACL) apply(path string) error {
	if len(a) == 0 {
		// A valid filesec with KAUTH_FILESEC_NOACL removes inherited ACLs.
		a = make([]byte, 44)
		binary.LittleEndian.PutUint32(a, 0x012cc16d)
		binary.LittleEndian.PutUint32(a[36:], ^uint32(0))
	}
	b := make([]byte, 8+len(a))
	binary.LittleEndian.PutUint32(b, 8)
	binary.LittleEndian.PutUint32(b[4:], uint32(len(a)))
	copy(b[8:], a)
	attrs := unix.Attrlist{Bitmapcount: 5, Commonattr: unix.ATTR_CMN_EXTENDED_SECURITY}
	return unix.Setattrlist(path, &attrs, b, unix.FSOPT_NOFOLLOW)
}

func clearAccessACL(path string, _ bool) error { return accessACL(nil).apply(path) }
