// Package proc runs the tools aims launches: attached to the terminal, or
// captured for headless runs, always passing termination signals on so a
// child never outlives aims.
package proc

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	activeMu sync.Mutex
	active   = map[*os.Process]bool{}
)

func track(p *os.Process)   { activeMu.Lock(); active[p] = true; activeMu.Unlock() }
func untrack(p *os.Process) { activeMu.Lock(); delete(active, p); activeMu.Unlock() }

// KillAll stops every child aims started, e.g. when the MCP client goes away.
func KillAll() {
	activeMu.Lock()
	defer activeMu.Unlock()
	for p := range active {
		_ = terminate(p)
	}
}

// ExitCode maps a finished command to a shell exit status (128+n for a signal).
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			return 128 + int(ws.Signal())
		}
		if c := ee.ExitCode(); c >= 0 {
			return c
		}
	}
	return 1
}

// relay passes termination signals to a started command until stop is called.
//
// Interrupts are passed on only when forwardInt is set. On a terminal, Ctrl+C
// and Ctrl+\ already reach the whole foreground process group, so an
// interactive tool would get them twice, and a second Ctrl+C makes Claude Code
// quit. aims itself ignores them while a child runs.
func relay(cmd *exec.Cmd, forwardInt bool) (stop func()) {
	sigs := make(chan os.Signal, 4)
	signal.Notify(sigs, relayed...)
	track(cmd.Process)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case s := <-sigs:
				if isInterrupt(s) && !forwardInt {
					continue
				}
				_ = cmd.Process.Signal(s)
			case <-done:
				return
			}
		}
	}()
	return func() {
		close(done)
		signal.Stop(sigs)
		untrack(cmd.Process)
	}
}

// RunAttached runs bin with the terminal attached and returns its exit status.
func RunAttached(bin string, args, env []string, dir string) (int, error) {
	cmd := exec.Command(bin, args...)
	cmd.Env, cmd.Dir = env, dir
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		return 127, err
	}
	stop := relay(cmd, false)
	defer stop()
	return ExitCode(cmd.Wait()), nil
}

// CommitAfter is how long headless stdout is held back. Limit and login errors
// arrive with the first request, well inside it.
const (
	CommitAfter      = 8 * time.Second
	commitAfterBytes = 256 << 10
)

// Captured describes a finished headless run.
type Captured struct {
	Code int
	// Committed: stdout was already passed through, too late to retry cleanly.
	Committed bool
	flush     func()
}

// Flush passes held-back stdout through.
func (c *Captured) Flush() { c.flush() }

// CaptureOptions configures RunCaptured.
type CaptureOptions struct {
	Env     []string
	Dir     string
	Stdin   []byte
	Stdout  io.Writer
	Stderr  io.Writer
	Timeout time.Duration
	// OnLine sees every complete line of output.
	OnLine func(stderr bool, line string)
}

// RunCaptured runs a headless command. stderr streams live; stdout is held
// back briefly so a failed attempt can be retried on another profile without
// mixing two runs' output.
func RunCaptured(bin string, args []string, o CaptureOptions) (*Captured, error) {
	cmd := exec.Command(bin, args...)
	cmd.Env, cmd.Dir = o.Env, o.Dir
	if len(o.Stdin) > 0 {
		cmd.Stdin = bytes.NewReader(o.Stdin)
	} // otherwise the null device: tools that read stdin see EOF at once
	// Own pipes rather than StdoutPipe: a background process the tool left
	// behind may keep them open, and aims must not wait for it (see below).
	outR, outW, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		outR.Close()
		outW.Close()
		return nil, err
	}
	defer outR.Close()
	defer errR.Close()
	cmd.Stdout, cmd.Stderr = outW, errW
	err = cmd.Start()
	outW.Close()
	errW.Close()
	if err != nil {
		return &Captured{Code: 127, flush: func() {}}, err
	}
	stop := relay(cmd, true)
	defer stop()

	var mu sync.Mutex
	var held [][]byte
	heldBytes, committed := 0, false
	commit := func() {
		mu.Lock()
		defer mu.Unlock()
		if committed {
			return
		}
		committed = true
		for _, b := range held {
			o.Stdout.Write(b)
		}
		held = nil
	}
	commitTimer := time.AfterFunc(CommitAfter, commit)
	defer commitTimer.Stop()
	if o.Timeout > 0 {
		exited := make(chan struct{})
		defer close(exited)
		go func() {
			select {
			case <-exited:
				return
			case <-time.After(o.Timeout):
			}
			io.WriteString(o.Stderr, "aims: stopped after "+o.Timeout.String()+"\n")
			_ = terminate(cmd.Process)
			select {
			case <-exited:
			case <-time.After(5 * time.Second):
				_ = cmd.Process.Kill()
			}
		}()
	}

	readers := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		pump(outR, func(chunk []byte) {
			mu.Lock()
			if committed {
				mu.Unlock()
				o.Stdout.Write(chunk)
				return
			}
			held = append(held, chunk)
			heldBytes += len(chunk)
			over := heldBytes > commitAfterBytes
			mu.Unlock()
			if over {
				commit()
			}
		}, func(line string) { emit(o.OnLine, false, line) })
	}()
	go func() {
		defer wg.Done()
		pump(errR, func(chunk []byte) { o.Stderr.Write(chunk) }, func(line string) { emit(o.OnLine, true, line) })
	}()
	go func() { wg.Wait(); close(readers) }()

	code := ExitCode(cmd.Wait())
	select {
	case <-readers:
	case <-time.After(pipeGrace):
		// The tool exited but something it started still holds the pipes.
		outR.Close()
		errR.Close()
		<-readers
	}
	mu.Lock()
	wasCommitted := committed
	mu.Unlock()
	return &Captured{Code: code, Committed: wasCommitted, flush: commit}, nil
}

// pipeGrace is how long output may keep arriving after the tool exits.
const pipeGrace = 2 * time.Second

var lineMu sync.Mutex

func emit(fn func(bool, string), stderr bool, line string) {
	if fn == nil {
		return
	}
	lineMu.Lock()
	defer lineMu.Unlock()
	fn(stderr, line)
}

// pump copies r chunk by chunk to onChunk and reports complete lines to onLine.
func pump(r io.Reader, onChunk func([]byte), onLine func(string)) {
	buf := make([]byte, 32<<10)
	var partial strings.Builder
	for {
		n, err := r.Read(buf)
		if n > 0 {
			chunk := append([]byte(nil), buf[:n]...)
			onChunk(chunk)
			data := chunk
			for {
				i := bytes.IndexByte(data, '\n')
				if i < 0 {
					partial.Write(data)
					break
				}
				partial.Write(data[:i])
				onLine(strings.TrimSuffix(partial.String(), "\r"))
				partial.Reset()
				data = data[i+1:]
			}
		}
		if err != nil {
			if partial.Len() > 0 {
				onLine(partial.String())
			}
			return
		}
	}
}

// Output runs a short helper command (`claude auth status`, `security`,
// `--version`) and returns its stdout. It never hangs past timeout.
func Output(bin string, args, env []string, timeout time.Duration) (string, int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.WaitDelay = time.Second
	err := cmd.Run()
	if ctx.Err() != nil {
		return out.String(), -1, ctx.Err()
	}
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return out.String(), ee.ExitCode(), nil
		}
		return out.String(), -1, err
	}
	return out.String(), 0, nil
}

// CombinedOutput is Output with stderr included.
func CombinedOutput(bin string, args, env []string, timeout time.Duration) (string, int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env
	cmd.WaitDelay = time.Second
	b, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return string(b), -1, ctx.Err()
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return string(b), ee.ExitCode(), nil
	}
	if err != nil {
		return string(b), -1, err
	}
	return string(b), 0, nil
}

// Lines splits text into trimmed, non-empty lines.
func Lines(s string) []string {
	var out []string
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		if t := strings.TrimSpace(sc.Text()); t != "" {
			out = append(out, t)
		}
	}
	return out
}
