package fsx

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

func privateAttributes(directory bool) (*windows.SecurityAttributes, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	inherit := ""
	if directory {
		inherit = "OICI"
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;" + inherit + ";FA;;;" + user.User.Sid.String() + ")")
	if err != nil {
		return nil, err
	}
	return &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}, nil
}

func privatePath(path string) (*uint16, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(path, `\\?\`) {
		if strings.HasPrefix(path, `\\`) {
			path = `\\?\UNC\` + path[2:]
		} else {
			path = `\\?\` + path
		}
	}
	return windows.UTF16PtrFromString(path)
}

// CreatePrivateFile applies an owner-only protected DACL at creation time;
// Windows mode bits alone cannot prevent inherited read grants.
func CreatePrivateFile(path string) (*os.File, error) {
	sa, err := privateAttributes(false)
	if err != nil {
		return nil, err
	}
	name, err := privatePath(path)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(name, windows.GENERIC_WRITE, windows.FILE_SHARE_READ, sa, windows.CREATE_NEW, windows.FILE_ATTRIBUTE_NORMAL, 0)
	runtime.KeepAlive(sa)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	return os.NewFile(uintptr(h), path), nil
}

func MkdirPrivate(path string) error {
	sa, err := privateAttributes(true)
	if err != nil {
		return err
	}
	name, err := privatePath(path)
	if err != nil {
		return err
	}
	err = windows.CreateDirectory(name, sa)
	runtime.KeepAlive(sa)
	return err
}
