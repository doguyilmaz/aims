package codex

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/doguyilmaz/aims/internal/proc"
	"github.com/doguyilmaz/aims/internal/testutil"
	"github.com/doguyilmaz/aims/internal/tool"
)

func TestCanceledCodexRequestsStopTheirWorkers(t *testing.T) {
	for _, kind := range []string{"account", "register", "unregister"} {
		t.Run(kind, func(t *testing.T) {
			testutil.Sandbox(t)
			self, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			marker := filepath.Join(t.TempDir(), "worker.pid")
			home := tool.Home{Dir: t.TempDir(), Bin: self, Env: append(os.Environ(), "AIMS_TEST_CODEX_CONTEXT_WORKER="+marker)}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				switch kind {
				case "account":
					account := New().Account(ctx, home)
					if account.LoggedIn != nil {
						done <- errors.New("canceled account check invented login status")
						return
					}
					done <- ctx.Err()
				case "register":
					done <- New().RegisterMCP(ctx, home, tool.MCPServer{Name: "fixture"})
				case "unregister":
					_, err := New().UnregisterMCP(ctx, home, "fixture")
					done <- err
				}
			}()
			var pid int
			for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
				if data, err := os.ReadFile(marker); err == nil {
					pid, _ = strconv.Atoi(string(data))
					if pid > 0 {
						break
					}
				}
				time.Sleep(10 * time.Millisecond)
			}
			if pid == 0 {
				t.Fatal("dummy Codex worker never started")
			}
			cancel()
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("cancellation was lost: %v", err)
				}
			case <-time.After(8 * time.Second):
				if worker, err := os.FindProcess(pid); err == nil {
					_ = worker.Kill()
					_ = worker.Release()
				}
				select {
				case <-done:
				case <-time.After(3 * time.Second):
				}
				t.Fatal("worker continued after request cancellation")
			}
			if proc.Alive(pid) {
				t.Fatal("Codex worker survived the completed canceled request")
			}
		})
	}
}
