package links

import (
	"os"

	"github.com/doguyilmaz/aims/internal/fsx"
)

func createCopyDirectory(path string, _ os.FileMode) error { return fsx.MkdirPrivate(path) }

func finishCopyPermissions(from, to string, mode os.FileMode) error {
	restore, err := copyPermissionRestorer(from, to, mode)
	if err != nil {
		return err
	}
	return restore()
}

func copyPermissionRestorer(from, to string, mode os.FileMode) (func() error, error) {
	return fsx.AccessRestorer(from, to)
}
