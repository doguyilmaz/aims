package links

import (
	"os"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

func createCopyDirectory(path string, mode os.FileMode) error {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;" + user.User.Sid.String() + ")")
	if err != nil {
		return err
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	err = windows.CreateDirectory(name, &sa)
	runtime.KeepAlive(sd)
	return err
}

func finishCopyPermissions(from, to string, mode os.FileMode) error {
	restore, err := copyPermissionRestorer(from, to, mode)
	if err != nil {
		return err
	}
	return restore()
}

func copyPermissionRestorer(from, to string, mode os.FileMode) (func() error, error) {
	sd, err := windows.GetNamedSecurityInfo(from, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return nil, err
	}
	acl, _, err := sd.DACL()
	if err != nil {
		return nil, err
	}
	// Do not inherit a public hub ACL while preserving a private source.
	return func() error {
		if err := os.Chmod(to, mode); err != nil {
			return err
		}
		err := windows.SetNamedSecurityInfo(to, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil)
		runtime.KeepAlive(sd)
		return err
	}, nil
}
