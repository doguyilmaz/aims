//go:build !windows

package links

import (
	"errors"
	"io/fs"
	"os"
	"syscall"
)

func makeLink(target, link string, _ bool) error { return os.Symlink(target, link) }

func removeLink(p string) error { return os.Remove(p) }

func isJunction(string) bool { return false }

func crossDevice(err error) bool { return errors.Is(err, syscall.EXDEV) }

func sameFilesystem(from, to string) (bool, error) {
	a, err := os.Lstat(from)
	if err != nil {
		return false, err
	}
	b, err := os.Stat(to)
	if err != nil {
		return false, err
	}
	sa, oka := a.Sys().(*syscall.Stat_t)
	sb, okb := b.Sys().(*syscall.Stat_t)
	if !oka || !okb {
		return false, errors.New("filesystem identity unavailable")
	}
	return sa.Dev == sb.Dev, nil
}

func sameFile(a, b fs.FileInfo) bool {
	sa, ok1 := a.Sys().(*syscall.Stat_t)
	sb, ok2 := b.Sys().(*syscall.Stat_t)
	return ok1 && ok2 && a.Mode().IsRegular() && sa.Ino == sb.Ino && sa.Dev == sb.Dev
}
