//go:build windows

package proc

import (
	"os"
	"syscall"
)

var relayed = []os.Signal{os.Interrupt, syscall.SIGTERM}

func terminate(p *os.Process) error { return p.Kill() }

func isInterrupt(s os.Signal) bool { return s == os.Interrupt }

// Alive reports whether process pid is running: Windows opens a handle only
// to one that exists.
func Alive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	_ = p.Release()
	return true
}
