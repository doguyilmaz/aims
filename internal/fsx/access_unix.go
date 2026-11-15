//go:build !windows

package fsx

import (
	"fmt"
	"os"
	"slices"
	"syscall"
)

// AccessRestorer captures access controls before a source can be moved away.
// New copies stay private until their ACL, ownership and final mode are ready.
func AccessRestorer(from, to string) (func() error, error) {
	fi, err := os.Stat(from)
	if err != nil {
		return nil, err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return nil, fmt.Errorf("cannot inspect ownership of %s", from)
	}
	uid, gid := int(st.Uid), int(st.Gid)
	if os.Geteuid() != 0 {
		groups, err := os.Getgroups()
		if err != nil {
			return nil, err
		}
		if uid != os.Geteuid() || gid != os.Getegid() && !slices.Contains(groups, gid) {
			return nil, fmt.Errorf("cannot preserve owner/group of %s as this user; kept it for explicit migration", from)
		}
	}
	acl, err := readAccessACL(from, fi.IsDir())
	if err != nil {
		return nil, fmt.Errorf("cannot preserve access controls of %s: %w", from, err)
	}
	mode := fi.Mode() & (os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky)
	return func() error {
		dest, err := os.Stat(to)
		if err != nil {
			return err
		}
		ds, ok := dest.Sys().(*syscall.Stat_t)
		if !ok {
			return fmt.Errorf("cannot inspect ownership of %s", to)
		}
		if int(ds.Uid) != uid || int(ds.Gid) != gid {
			if err := os.Chown(to, uid, gid); err != nil {
				return err
			}
		}
		// Install restrictive ACL entries before exposing the final mode bits.
		if err := acl.apply(to); err != nil {
			return err
		}
		return os.Chmod(to, mode)
	}, nil
}
