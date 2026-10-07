//go:build !windows

package fsx

import "os"

func rename(from, to string) error { return os.Rename(from, to) }

func busy(error) bool { return false }
