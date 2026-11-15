package proc

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestWindowsLauncherOwnsChildAfterForcedTermination(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	pidfile := filepath.Join(t.TempDir(), "child.pid")
	cmd := exec.Command(self, "-test.run=^TestWindowsJobLauncherHelper$")
	cmd.Env = append(os.Environ(), "AIMS_TEST_JOB_ROLE=launcher", "AIMS_TEST_JOB_PIDFILE="+pidfile)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cmd.Process.Kill(); cmd.Wait() }()
	var pid int
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); {
		if b, e := os.ReadFile(pidfile); e == nil {
			pid, _ = strconv.Atoi(strings.TrimSpace(string(b)))
			if pid > 0 {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if pid == 0 {
		t.Fatal("launcher never started native child")
	}
	child, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(child)
	defer windows.TerminateProcess(child, 1)
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	state, err := windows.WaitForSingleObject(child, 5000)
	if err != nil || state != windows.WAIT_OBJECT_0 {
		t.Fatalf("native child outlived launcher: %v %v", state, err)
	}
}

func TestWindowsJobLauncherHelper(t *testing.T) {
	if os.Getenv("AIMS_TEST_JOB_ROLE") != "launcher" {
		return
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(), "AIMS_TEST_JOB_ROLE=child")
	if _, err := Replace(self, []string{"-test.run=^TestWindowsJobChildHelper$"}, env); err != nil {
		t.Fatal(err)
	}
}

func TestWindowsJobChildHelper(t *testing.T) {
	if os.Getenv("AIMS_TEST_JOB_ROLE") != "child" {
		return
	}
	if err := os.WriteFile(os.Getenv("AIMS_TEST_JOB_PIDFILE"), []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		t.Fatal(err)
	}
	for {
		time.Sleep(time.Hour)
	}
}
