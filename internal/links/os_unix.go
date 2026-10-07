//go:build !windows

package links

import (
	"io/fs"
	"os"
	"syscall"
)

func makeLink(target, link string, _ bool) error { return os.Symlink(target, link) }

func removeLink(p string) error { return os.Remove(p) }

func isJunction(string) bool { return false }

func sameFile(a, b fs.FileInfo) bool {
	sa, ok1 := a.Sys().(*syscall.Stat_t)
	sb, ok2 := b.Sys().(*syscall.Stat_t)
	return ok1 && ok2 && a.Mode().IsRegular() && sa.Ino == sb.Ino && sa.Dev == sb.Dev
}
