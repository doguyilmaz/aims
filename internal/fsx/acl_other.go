//go:build !windows && !linux && !darwin

package fsx

import "fmt"

type accessACL struct{}

func readAccessACL(string, bool) (accessACL, error) {
	return accessACL{}, fmt.Errorf("ACL preservation is unavailable on this platform")
}
func (accessACL) apply(string) error {
	return fmt.Errorf("ACL preservation is unavailable on this platform")
}
func clearAccessACL(string, bool) error {
	return fmt.Errorf("private ACL creation is unavailable on this platform")
}
