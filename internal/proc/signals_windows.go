//go:build windows

package proc

import (
	"errors"
	"os"
	"syscall"

	"golang.org/x/sys/windows"
)

var relayed = []os.Signal{os.Interrupt, syscall.SIGTERM}

func terminate(p *os.Process) error { return p.Kill() }

func isInterrupt(s os.Signal) bool { return s == os.Interrupt }

// Alive checks the process's termination state. A handle can remain valid after
// termination while another process (for example a debugger) still holds it.
func Alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return errors.Is(err, windows.ERROR_ACCESS_DENIED)
	}
	defer windows.CloseHandle(handle)
	state, err := windows.WaitForSingleObject(handle, 0)
	return err != nil || state != windows.WAIT_OBJECT_0
}
