package proc

import (
	"os"
	"os/exec"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestAliveDistinguishesAnExitedProcessWithARetainedHandle(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(self, "-test.run=^TestWindowsAliveHelper$")
	cmd.Env = append(os.Environ(), "AIMS_TEST_ALIVE_HELPER=1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	pid := cmd.Process.Pid
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(handle)
	if !Alive(pid) {
		t.Fatal("running child reported dead")
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	if result, err := windows.WaitForSingleObject(handle, 5000); err != nil || result != windows.WAIT_OBJECT_0 {
		t.Fatalf("child did not terminate: %v %v", result, err)
	}
	if Alive(pid) {
		t.Fatal("retained process handle was mistaken for a running worker")
	}
	if Alive(0) || Alive(-1) {
		t.Fatal("invalid PID reported alive")
	}
}

func TestWindowsAliveHelper(t *testing.T) {
	if os.Getenv("AIMS_TEST_ALIVE_HELPER") != "1" {
		return
	}
	for {
		time.Sleep(time.Hour)
	}
}
