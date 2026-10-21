//go:build !windows

package proc

import (
	"os"
	"syscall"
)

var relayed = []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT}

func terminate(p *os.Process) error { return p.Signal(syscall.SIGTERM) }

func isInterrupt(s os.Signal) bool { return s == os.Interrupt || s == syscall.SIGQUIT }

// Alive reports whether process pid is running.
func Alive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}
