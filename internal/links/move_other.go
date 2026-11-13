//go:build !darwin && !linux && !windows

package links

import (
	"fmt"
	"os"
)

func renameNoReplace(from, to string) error {
	fi, err := os.Lstat(from)
	if err != nil {
		return err
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("this platform cannot move %s without risking replacement; kept it", from)
	}
	if err := os.Link(from, to); err != nil {
		return err
	}
	return os.Remove(from)
}
