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

// removeLink deletes a symlink, junction or hard link without touching what
// it points to. Never RemoveAll here: that is one mistake away from the hub.
func removeLink(p string) error { return os.Remove(p) }

func isJunction(p string) bool {
	fi, err := os.Lstat(p)
	return err == nil && fi.Mode()&fs.ModeIrregular != 0
}

func sameFile(a, b fs.FileInfo) bool { return a.Mode().IsRegular() && os.SameFile(a, b) }
