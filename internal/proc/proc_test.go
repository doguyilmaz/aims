package proc

import (
	"bytes"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// syncBuf is a goroutine-safe buffer.
type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.b.Write(p) }
func (s *syncBuf) String() string              { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

func sh(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh")
	}
	p, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh")
	}
	return p
}

func TestRunCapturedHoldsStdoutUntilFlush(t *testing.T) {
	bin := sh(t)
	var out, errOut syncBuf
	var lines []string
	c, err := RunCaptured(bin, []string{"-c", "echo one; echo err >&2; printf two; exit 3"}, CaptureOptions{
		Env: os.Environ(), Stdout: &out, Stderr: &errOut,
		OnLine: func(stderr bool, l string) {
			if !stderr {
				lines = append(lines, l)
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.Code != 3 || c.Committed {
		t.Fatalf("code=%d committed=%v", c.Code, c.Committed)
	}
	if out.String() != "" || errOut.String() != "err\n" {
		t.Fatalf("stdout must wait for Flush: %q / %q", out.String(), errOut.String())
	}
	c.Flush()
	if out.String() != "one\ntwo" || strings.Join(lines, ",") != "one,two" {
		t.Fatalf("after flush: %q lines=%v", out.String(), lines)
	}
}

func TestRunCapturedStdin(t *testing.T) {
	bin := sh(t)
	var out syncBuf
	c, err := RunCaptured(bin, []string{"-c", "cat"}, CaptureOptions{Stdin: []byte("piped"), Stdout: &out, Stderr: &out})
	if err != nil || c.Code != 0 {
		t.Fatal(err)
	}
	c.Flush()
	if out.String() != "piped" {
		t.Fatalf("stdin: %q", out.String())
	}
}

func TestRunCapturedTimeout(t *testing.T) {
	bin := sh(t)
	var out syncBuf
	start := time.Now()
	c, err := RunCaptured(bin, []string{"-c", "trap '' TERM; sleep 30"}, CaptureOptions{Stdout: &out, Stderr: &out, Timeout: 200 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if c.Code == 0 || time.Since(start) > 15*time.Second || !strings.Contains(out.String(), "stopped after") {
		t.Fatalf("code=%d after %v: %q", c.Code, time.Since(start), out.String())
	}
}

func TestRunCapturedDoesNotWaitForLeftovers(t *testing.T) {
	bin := sh(t)
	var out syncBuf
	start := time.Now()
	// The child exits at once; a background process keeps stdout open.
	c, err := RunCaptured(bin, []string{"-c", "echo done; (sleep 30) & exit 0"}, CaptureOptions{Stdout: &out, Stderr: &out})
	if err != nil || c.Code != 0 {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 10*time.Second {
		t.Fatalf("waited %v for a leftover process", d)
	}
	c.Flush()
	if out.String() != "done\n" {
		t.Fatalf("output: %q", out.String())
	}
}

func TestOutputTimeout(t *testing.T) {
	bin := sh(t)
	if _, code, err := Output(bin, []string{"-c", "sleep 10"}, nil, 100*time.Millisecond); err == nil || code != -1 {
		t.Fatalf("code=%d err=%v", code, err)
	}
	out, code, err := Output(bin, []string{"-c", "echo hi; exit 4"}, nil, 5*time.Second)
	if err != nil || code != 4 || out != "hi\n" {
		t.Fatalf("%q %d %v", out, code, err)
	}
	if _, _, err := Output("/nonexistent/bin", nil, nil, time.Second); err == nil {
		t.Fatal("missing binary")
	}
}

func TestLines(t *testing.T) {
	if got := Lines("  a \n\n b\r\n"); strings.Join(got, "|") != "a|b" {
		t.Fatalf("%q", got)
	}
}
