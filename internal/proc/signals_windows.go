//go:build windows

package proc

import (
	"os"
	"syscall"
)

var relayed = []os.Signal{os.Interrupt, syscall.SIGTERM}

func terminate(p *os.Process) error { return p.Kill() }

func isInterrupt(s os.Signal) bool { return s == os.Interrupt }
