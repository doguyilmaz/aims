//go:build windows

package links

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

// makeLink uses a symlink when Windows allows one (Developer Mode or admin),
// else a junction for directories (no rights needed) and a hard link for files.
func makeLink(target, link string, isDir bool) error {
	if err := os.Symlink(target, link); err == nil {
		return nil
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(link), target)
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

func sameFilesystem(from, to string) (bool, error) {
	volume := func(path string, follow bool) (string, error) {
		p, err := windows.UTF16PtrFromString(path)
		if err != nil {
			return "", err
		}
		flags := uint32(windows.FILE_FLAG_BACKUP_SEMANTICS)
		if !follow {
			flags |= windows.FILE_FLAG_OPEN_REPARSE_POINT
		}
		h, err := windows.CreateFile(p, windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, flags, 0)
		if err != nil {
			return "", err
		}
		defer windows.CloseHandle(h)
		buf := make([]uint16, 32768)
		n, err := windows.GetFinalPathNameByHandle(h, &buf[0], uint32(len(buf)), 1) // VOLUME_NAME_GUID
		if err == nil && n < uint32(len(buf)) {
			path := windows.UTF16ToString(buf)
			if end := strings.Index(path, `}\`); end >= 0 {
				return path[:end+1], nil
			}
		}
		// Remote volumes expose an UNC share rather than a local volume GUID.
		n, err = windows.GetFinalPathNameByHandle(h, &buf[0], uint32(len(buf)), 0)
		if err != nil {
			return "", err
		}
		path = windows.UTF16ToString(buf)
		if n < uint32(len(buf)) && strings.HasPrefix(strings.ToUpper(path), `\\?\UNC\`) {
			parts := strings.Split(strings.TrimPrefix(path, `\\?\UNC\`), `\`)
			if len(parts) >= 2 {
				return parts[0] + `\` + parts[1], nil
			}
		}
		return "", errors.New("cannot determine destination volume without changing source")
	}
	a, err := volume(from, false)
	if err != nil {
		return false, err
	}
	b, err := volume(to, true)
	return strings.EqualFold(a, b), err
}
