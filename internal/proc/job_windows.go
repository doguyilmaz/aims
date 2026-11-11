package proc

import (
	"fmt"
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var launcherJob struct {
	once   sync.Once
	handle windows.Handle
	err    error
}

// Join the launcher itself before CreateProcess, so every native descendant
// inherits membership atomically. The handle is non-inheritable and stays
// open until process exit; closing it here would also terminate this launcher.
func ownLauncherTree() error {
	launcherJob.once.Do(func() {
		job, err := windows.CreateJobObject(nil, nil)
		if err != nil {
			launcherJob.err = err
			return
		}
		var limits windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
		limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
		_, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits)))
		runtime.KeepAlive(&limits)
		if err == nil {
			err = windows.AssignProcessToJobObject(job, windows.CurrentProcess())
		}
		if err != nil {
			windows.CloseHandle(job)
			launcherJob.err = err
			return
		}
		launcherJob.handle = job
	})
	if launcherJob.err != nil {
		return fmt.Errorf("cannot own Windows launcher process tree: %w", launcherJob.err)
	}
	return nil
}
