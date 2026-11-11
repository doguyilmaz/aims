package proc

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows"
)

func Writing(...int) (map[int][]string, bool) { return nil, false }
func Children(int) []int                      { return nil }

// Holding checks for incompatible existing handles without changing files or
// stopping their owners. A denied/incomplete inspection is always unknown.
func Holding(dir string) (held, ok bool)     { return holdingWindows(dir, false) }
func HoldingTree(dir string) (held, ok bool) { return holdingWindows(dir, true) }

func holdingWindows(dir string, recursive bool) (bool, bool) {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return false, false
	}
	var busy bool
	err = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path != dir && entry.IsDir() && !recursive {
			return filepath.SkipDir
		}
		name, err := windows.UTF16PtrFromString(path)
		if err != nil {
			return err
		}
		var h windows.Handle
		for attempt := range 10 {
			h, err = windows.CreateFile(name, windows.GENERIC_READ, 0, nil, windows.OPEN_EXISTING,
				windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
			if err == nil || !(errors.Is(err, windows.ERROR_SHARING_VIOLATION) || errors.Is(err, windows.ERROR_LOCK_VIOLATION) || errors.Is(err, windows.ERROR_ACCESS_DENIED)) {
				break
			}
			// Indexers and scanners may briefly open a newly changed path.
			// Retry only for a bounded interval; persistent handles or denied
			// inspection still prevent reconciliation.
			if attempt < 9 {
				time.Sleep(20 * time.Millisecond)
			}
		}
		if errors.Is(err, windows.ERROR_SHARING_VIOLATION) || errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			busy = true
			return fs.SkipAll
		}
		if err != nil {
			return err
		}
		if err := windows.CloseHandle(h); err != nil {
			return err
		}
		// A junction is an independent reference, not a subtree to reconcile.
		if path != dir && entry.Type()&os.ModeIrregular != 0 && entry.IsDir() {
			return filepath.SkipDir
		}
		return nil
	})
	if busy {
		return true, true
	}
	return false, err == nil
}
