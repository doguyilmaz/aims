//go:build !windows

package fsx

import "os"

// CreatePrivateFile creates a new file exclusively. Inherited ACL grants are
// removed while it is still empty, before the caller can write private data.
func CreatePrivateFile(path string) (*os.File, error) {
	f, err := openPrivateFile(path)
	if err != nil {
		return nil, err
	}
	if err := clearAccessACL(path, false); err != nil {
		f.Close()
		os.Remove(path)
		return nil, err
	}
	return f, nil
}

// MkdirPrivate removes inherited grants before any children are populated.
func MkdirPrivate(path string) error {
	if err := os.Mkdir(path, 0o700); err != nil {
		return err
	}
	if err := clearAccessACL(path, true); err != nil {
		os.Remove(path)
		return err
	}
	return nil
}
