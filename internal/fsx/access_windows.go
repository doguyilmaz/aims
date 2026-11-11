package fsx

import (
	"os"
	"runtime"

	"golang.org/x/sys/windows"
)

func AccessRestorer(from, to string) (func() error, error) {
	fi, err := os.Stat(from)
	if err != nil {
		return nil, err
	}
	sd, err := windows.GetNamedSecurityInfo(from, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return nil, err
	}
	acl, _, err := sd.DACL()
	if err != nil {
		return nil, err
	}
	return func() error {
		// Preserve the source's ACL instead of inheriting grants from the
		// destination directory. Do this before the copy leaves staging.
		err := windows.SetNamedSecurityInfo(to, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil)
		runtime.KeepAlive(sd)
		if err != nil {
			return err
		}
		return os.Chmod(to, fi.Mode().Perm())
	}, nil
}
