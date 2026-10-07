//go:build windows

package links

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// makeLink uses a symlink when Windows allows one (Developer Mode or admin),
// else a junction for directories (no rights needed) and a hard link for files.
func makeLink(target, link string, isDir bool) error {
	if err := os.Symlink(target, link); err == nil {
		return nil
	}
	if !isDir {
		return os.Link(target, link)
	}
	return junction(target, link)
}

// junction runs `mklink /J` with a command line built by hand: Go quotes only
// arguments with spaces, and cmd would run whatever follows an unquoted "&".
// Inside double quotes cmd treats & | ^ < > literally; % still expands, and
// '"' cannot occur in a Windows path.
func junction(target, link string) error {
	if strings.ContainsRune(target, '%') || strings.ContainsRune(link, '%') {
		return errors.New("cannot create a junction for a path containing %; turn on Developer Mode for symlinks")
	}
	cmd := exec.Command("cmd")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: fmt.Sprintf(`cmd /d /c mklink /J "%s" "%s"`, link, target)}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("mklink /J: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// removeLink deletes a symlink, junction or hard link without touching what
// it points to. Never RemoveAll here: that is one mistake away from the hub.
func removeLink(p string) error { return os.Remove(p) }

func isJunction(p string) bool {
	fi, err := os.Lstat(p)
	return err == nil && fi.Mode()&fs.ModeIrregular != 0
}

func sameFile(a, b fs.FileInfo) bool { return a.Mode().IsRegular() && os.SameFile(a, b) }

// errNotSameDevice is ERROR_NOT_SAME_DEVICE: a rename across volumes.
const errNotSameDevice = syscall.Errno(17)

func crossDevice(err error) bool { return errors.Is(err, errNotSameDevice) }
