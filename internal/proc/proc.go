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
	"unicode"

	"golang.org/x/term"
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
	a, err := StartAttached(bin, args, env, dir)
	if err != nil {
		return 127, err
	}
	<-a.Done()
	return a.Code(), nil
}

// Attached is a tool running with the terminal attached.
type Attached struct {
	cmd   *exec.Cmd
	done  chan struct{}
	code  int
	term  *term.State
	stdin int
}

// StartAttached starts bin with the terminal attached.
func StartAttached(bin string, args, env []string, dir string) (*Attached, error) {
	resolved, err := LookPathEnv(bin, env)
	if err != nil {
		return nil, err
	}
	bin, args, err = attachedCommand(resolved, args, env)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(bin, args...)
	cmd.Env, cmd.Dir = env, dir
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	a := &Attached{cmd: cmd, done: make(chan struct{}), stdin: int(os.Stdin.Fd())}
	if term.IsTerminal(a.stdin) {
		a.term, _ = term.GetState(a.stdin)
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	stop := relay(cmd, !term.IsTerminal(a.stdin))
	go func() {
		a.code = ExitCode(cmd.Wait())
		stop()
		close(a.done)
	}()
	return a, nil
}

// Done is closed when the tool has exited.
func (a *Attached) Done() <-chan struct{} { return a.done }

// Code is the exit status, once Done is closed.
func (a *Attached) Code() int { return a.code }

// Pid is the tool's process id.
func (a *Attached) Pid() int { return a.cmd.Process.Pid }

// Notify asks the terminal for a desktop notification (OSC 9, which
// Ghostty, iTerm2, WezTerm and kitty show). Other terminals ignore the
// sequence, so it can go out while a tool draws on the screen.
func Notify(msg string) {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return
	}
	msg = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, msg)
	_, _ = io.WriteString(os.Stdout, "\x1b]9;aims: "+msg+"\x07")
}

// resetModes turns off what a full-screen tool may leave on:
// the alternate screen, hidden cursor, bracketed paste, focus and mouse
// reporting, the kitty keyboard protocol, colours.
const resetModes = "\x1b[?1049l\x1b[?25h\x1b[?2004l\x1b[?1004l\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l\x1b[<u\x1b[0m\r\n"

// Stop asks the tool to quit, kills it after grace, and puts the terminal
// back the way it was before the tool started: a tool stopped from outside
// may not clean up after itself.
func (a *Attached) Stop(grace time.Duration) {
	_ = terminate(a.cmd.Process)
	select {
	case <-a.done:
	case <-time.After(grace):
		_ = a.cmd.Process.Kill()
		<-a.done
	}
	if a.term != nil {
		_ = term.Restore(a.stdin, a.term)
		_, _ = io.WriteString(os.Stdout, resetModes)
	}
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
	// AnalysisIncomplete: an oversized line was skipped by OnLine. Raw output
	// is unchanged, but its analysis cannot establish that retrying is safe.
	AnalysisIncomplete bool
	flush              func()
}

// Flush passes held-back stdout through.
func (c *Captured) Flush() { c.flush() }

// CaptureOptions configures RunCaptured.
type CaptureOptions struct {
	// Context binds this worker to its caller, independently of its timeout.
	Context context.Context
	Env     []string
	Dir     string
	Stdin   []byte
	Stdout  io.Writer
	Stderr  io.Writer
	Timeout time.Duration
	// OnLine sees complete lines up to maxAnalysisLineBytes. Larger lines are
	// streamed unchanged but skipped for analysis; see AnalysisIncomplete.
	OnLine func(stderr bool, line string)
}

type serialWriter struct {
	mu *sync.Mutex
	w  io.Writer
}

func (s serialWriter) Write(b []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.w == nil {
		return len(b), nil
	}
	return s.w.Write(b)
}

// RunCaptured runs a headless command. stderr streams live; stdout is held
// back briefly so a failed attempt can be retried on another profile without
// mixing two runs' output.
func RunCaptured(bin string, args []string, o CaptureOptions) (*Captured, error) {
	ctx := o.Context
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return &Captured{Code: 127, flush: func() {}}, err
	}
	// The caller may use a buffer, including the same buffer for both streams.
	// Timer/cancellation messages and reader goroutines share this lock too.
	var streamMu sync.Mutex
	o.Stdout = serialWriter{&streamMu, o.Stdout}
	o.Stderr = serialWriter{&streamMu, o.Stderr}
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
	commitDone := make(chan struct{})
	commitTimer := time.AfterFunc(CommitAfter, func() { defer close(commitDone); commit() })
	defer commitTimer.Stop()
	if o.Timeout > 0 || ctx.Done() != nil {
		exited := make(chan struct{})
		defer close(exited)
		var timeout <-chan time.Time
		if o.Timeout > 0 {
			timer := time.NewTimer(o.Timeout)
			defer timer.Stop()
			timeout = timer.C
		}
		go func() {
			select {
			case <-exited:
				return
			case <-timeout:
				io.WriteString(o.Stderr, "aims: stopped after "+o.Timeout.String()+"\n")
			case <-ctx.Done():
				io.WriteString(o.Stderr, "aims: stopped: "+ctx.Err().Error()+"\n")
			}
			_ = terminate(cmd.Process)
			select {
			case <-exited:
			case <-time.After(5 * time.Second):
				_ = cmd.Process.Kill()
			}
		}()
	}

	readers := make(chan struct{})
	var outSkipped, errSkipped bool
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		outSkipped = pump(outR, func(chunk []byte) {
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
		errSkipped = pump(errR, func(chunk []byte) { o.Stderr.Write(chunk) }, func(line string) { emit(o.OnLine, true, line) })
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
	// AfterFunc.Stop does not join an already running callback. Finish any
	// commit before reporting whether stdout was exposed or returning buffers.
	if !commitTimer.Stop() {
		<-commitDone
	}
	mu.Lock()
	wasCommitted := committed
	mu.Unlock()
	return &Captured{Code: code, Committed: wasCommitted, AnalysisIncomplete: outSkipped || errSkipped, flush: commit}, ctx.Err()
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

// maxAnalysisLineBytes bounds the additional memory used to analyze one line.
// It does not limit or truncate the raw output passed to the caller.
const maxAnalysisLineBytes = 1 << 20

// pump copies r chunk by chunk to onChunk and reports bounded lines to onLine.
// An oversized line is discarded through its newline, never reported as
// fragments that could be mistaken for independent events. It returns whether
// any line was skipped, including an oversized final line without a newline.
func pump(r io.Reader, onChunk func([]byte), onLine func(string)) (skipped bool) {
	buf := make([]byte, 32<<10)
	var partial strings.Builder
	discarding := false
	for {
		n, err := r.Read(buf)
		if n > 0 {
			chunk := append([]byte(nil), buf[:n]...)
			onChunk(chunk)
			data := chunk
			for {
				i := bytes.IndexByte(data, '\n')
				part := data
				if i >= 0 {
					part = data[:i]
				}
				if !discarding {
					// Keep one possible CRLF terminator byte across reads. A
					// content byte beyond the limit still discards the line.
					if len(part) > maxAnalysisLineBytes+1-partial.Len() {
						skipped, discarding = true, true
						partial.Reset()
					} else {
						partial.Write(part)
						if partial.Len() > maxAnalysisLineBytes && !strings.HasSuffix(partial.String(), "\r") {
							skipped, discarding = true, true
							partial.Reset()
						}
					}
				}
				if i < 0 {
					break
				}
				if !discarding {
					onLine(strings.TrimSuffix(partial.String(), "\r"))
				}
				partial.Reset()
				discarding = false
				data = data[i+1:]
			}
		}
		if err != nil {
			if !discarding && partial.Len() > 0 {
				if partial.Len() > maxAnalysisLineBytes {
					// Without a newline, the pending CR is content.
					skipped = true
				} else {
					onLine(partial.String())
				}
			}
			return
		}
	}
}

// Output runs a short helper command (`claude auth status`, `security`,
// `--version`) and returns its stdout. It never hangs past timeout.
func Output(bin string, args, env []string, timeout time.Duration) (string, int, error) {
	return OutputContext(context.Background(), bin, args, env, timeout)
}

// OutputContext also stops the helper when the request is cancelled.
func OutputContext(ctx context.Context, bin string, args, env []string, timeout time.Duration) (string, int, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
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
	return CombinedOutputContext(context.Background(), bin, args, env, timeout)
}

func CombinedOutputContext(ctx context.Context, bin string, args, env []string, timeout time.Duration) (string, int, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
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
