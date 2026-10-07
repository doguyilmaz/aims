//go:build windows

package links

import (
	"io/fs"
	"os"
	"os/exec"
)

// makeLink uses a junction for directories (no admin rights or Developer Mode
// needed) and falls back to a hard link for files.
func makeLink(target, link string, isDir bool) error {
	if isDir {
		if err := os.Symlink(target, link); err == nil {
			return nil
		}
		return exec.Command("cmd", "/c", "mklink", "/J", link, target).Run()
	}
	if err := os.Symlink(target, link); err == nil {
		return nil
	}
	return os.Link(target, link)
}

func removeLink(p string) error {
	if err := os.Remove(p); err != nil {
		return os.RemoveAll(p)
	}
	return nil
}

func isJunction(p string) bool {
	fi, err := os.Lstat(p)
	return err == nil && fi.Mode()&fs.ModeIrregular != 0
}

func sameFile(a, b fs.FileInfo) bool { return a.Mode().IsRegular() && os.SameFile(a, b) }
