package fsx

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func openPrivateFile(path string) (*os.File, error) {
	// Darwin inherited ACL grants can override 0600. Build the empty file
	// inside a private directory, then publish its already-private inode by
	// an exclusive hard link. No other user can retain an early read handle.
	dir, err := os.MkdirTemp(filepath.Dir(path), ".aims-private-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	if err := clearAccessACL(dir, true); err != nil {
		return nil, err
	}
	inner := filepath.Join(dir, "file")
	f, err := os.OpenFile(inner, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fd, err := unix.FcntlInt(f.Fd(), unix.F_DUPFD_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	out := os.NewFile(uintptr(fd), path)
	if err := os.Link(inner, path); err != nil {
		out.Close()
		return nil, err
	}
	return out, nil
}
