//go:build !darwin && !linux && !windows

package schedule

import (
	"errors"
	"runtime"
)

var errUnsupported = errors.New("no scheduler aims knows on " + runtime.GOOS + "; run `aims keepalive` from cron instead")

func install(Job) (string, error)         { return "", errUnsupported }
func uninstall(Job) (string, bool, error) { return "", false, nil }
func installed(Job) (string, bool)        { return "", false }
