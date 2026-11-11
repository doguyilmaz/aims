//go:build !windows

package links

import "os"

func createCopyDirectory(path string, mode os.FileMode) error       { return os.Mkdir(path, mode) }
func finishCopyPermissions(from, to string, mode os.FileMode) error { return os.Chmod(to, mode) }

func copyPermissionRestorer(from, to string, mode os.FileMode) (func() error, error) {
	return func() error { return os.Chmod(to, mode) }, nil
}
