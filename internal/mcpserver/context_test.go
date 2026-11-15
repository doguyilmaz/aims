package mcpserver

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/proc"
	"github.com/doguyilmaz/aims/internal/tools"
)

func contextWorker() {
	if err := os.WriteFile(os.Getenv("AIMS_TEST_MCP_CONTEXT_MARKER"), []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		os.Exit(4)
	}
	for {
		time.Sleep(time.Hour)
	}
}

func TestSDKRequestCancellationStopsWorkerAndKeepsServer(t *testing.T) {
	for _, name := range []string{"aims_run", "aims_status"} {
		t.Run(name, func(t *testing.T) {
			setup(t)
			root := t.TempDir()
			first, second := filepath.Join(root, "first.pid"), filepath.Join(root, "second.pid")
			if _, err := config.Update(func(c *config.Config) error {
				for i, n := range c.Tools["claude"].Order {
					marker := first
					if i > 0 {
						marker = second
					}
					c.Tools["claude"].Profiles[n].Env = map[string]string{
						"AIMS_TEST_MCP_CONTEXT_WORKER": "1", "AIMS_TEST_MCP_CONTEXT_MARKER": marker,
					}
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			self, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			t.Setenv(tools.Get("claude").Layout().BinEnv, self)
			client := connect(t)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			args := map[string]any{"tool": "claude", "live": true}
			if name == "aims_run" {
				args = map[string]any{"tool": "claude", "profile": "personal", "prompt": "DUMMY", "write": false, "timeoutSeconds": 30}
			}
			done := make(chan error, 1)
			go func() {
				_, err := client.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
				done <- err
			}()
			var pid int
			for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
				if b, err := os.ReadFile(first); err == nil {
					pid, _ = strconv.Atoi(string(b))
					if pid > 0 {
						break
					}
				}
				time.Sleep(10 * time.Millisecond)
			}
			if pid == 0 {
				t.Fatal("SDK call did not start the dummy worker")
			}
			cancel()
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("client cancellation: %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("client call did not cancel")
			}
			for deadline := time.Now().Add(8 * time.Second); proc.Alive(pid) && time.Now().Before(deadline); {
				time.Sleep(10 * time.Millisecond)
			}
			if proc.Alive(pid) {
				t.Fatal("canceled SDK call left its dummy worker running")
			}
			if _, err := os.Stat(second); !os.IsNotExist(err) {
				t.Fatal("canceled status request started a second profile probe")
			}
			res, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: "aims_status", Arguments: map[string]any{"tool": "claude"}})
			if err != nil || res.IsError {
				t.Fatalf("individual cancellation stopped the MCP server: %v", err)
			}
		})
	}
}
