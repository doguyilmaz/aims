package proc

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestRequestContextWorker(t *testing.T) {
	path := os.Getenv("AIMS_TEST_CONTEXT_MARKER")
	if path == "" {
		return
	}
	if err := os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		os.Exit(4)
	}
	for {
		time.Sleep(time.Hour)
	}
}

func TestRequestCancellationStopsOwnWorker(t *testing.T) {
	for _, kind := range []string{"captured", "output", "combined"} {
		t.Run(kind, func(t *testing.T) {
			self, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			marker := filepath.Join(t.TempDir(), "worker.pid")
			env := append(os.Environ(), "AIMS_TEST_CONTEXT_MARKER="+marker)
			args := []string{"-test.run=^TestRequestContextWorker$"}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				var err error
				switch kind {
				case "captured":
					_, err = RunCaptured(self, args, CaptureOptions{Context: ctx, Env: env, Stdout: io.Discard, Stderr: io.Discard, Timeout: time.Minute})
				case "output":
					_, _, err = OutputContext(ctx, self, args, env, time.Minute)
				case "combined":
					_, _, err = CombinedOutputContext(ctx, self, args, env, time.Minute)
				}
				done <- err
			}()
			defer func() {
				cancel()
				select {
				case <-done:
				case <-time.After(10 * time.Second):
				}
			}()
			var pid int
			for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
				if b, err := os.ReadFile(marker); err == nil {
					pid, _ = strconv.Atoi(string(b))
					if pid > 0 {
						break
					}
				}
				time.Sleep(10 * time.Millisecond)
			}
			if pid == 0 {
				t.Fatal("dummy worker never started")
			}
			cancel()
			select {
			case err := <-done:
				// Leave a completion value for the deferred cleanup too.
				done <- err
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("request cancellation lost: %v", err)
				}
			case <-time.After(8 * time.Second):
				t.Fatal("worker ignored request cancellation")
			}
			if Alive(pid) {
				t.Fatal("dummy worker still active after request returned")
			}
		})
	}
}

func TestCanceledCaptureNeverStartsWorker(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := RunCaptured("/nonexistent/worker", nil, CaptureOptions{Context: ctx}); !errors.Is(err, context.Canceled) {
		t.Fatalf("a canceled request attempted executable lookup/start: %v", err)
	}
}
