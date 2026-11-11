package fsx

import (
	"errors"
	"fmt"

	"golang.org/x/sys/unix"
)

type accessACL struct {
	access, defaults []byte
	directory        bool
}

func readAccessACL(path string, directory bool) (accessACL, error) {
	a := accessACL{directory: directory}
	read := func(key string) ([]byte, error) {
		n, err := unix.Getxattr(path, key, nil)
		if errors.Is(err, unix.ENODATA) || errors.Is(err, unix.ENOTSUP) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		if n > 64<<10 {
			return nil, fmt.Errorf("ACL exceeds 64 KiB")
		}
		b := make([]byte, n)
		n, err = unix.Getxattr(path, key, b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
	var err error
	a.access, err = read("system.posix_acl_access")
	if err == nil && directory {
		a.defaults, err = read("system.posix_acl_default")
	}
	return a, err
}

func (a accessACL) apply(path string) error {
	set := func(key string, value []byte) error {
		if len(value) > 0 {
			return unix.Setxattr(path, key, value, 0)
		}
		err := unix.Removexattr(path, key)
		if errors.Is(err, unix.ENODATA) || errors.Is(err, unix.ENOTSUP) {
			return nil
		}
		return err
	}
	if err := set("system.posix_acl_access", a.access); err != nil {
		return err
	}
	if a.directory {
		return set("system.posix_acl_default", a.defaults)
	}
	return nil
}

func clearAccessACL(path string, directory bool) error {
	return (accessACL{directory: directory}).apply(path)
}
