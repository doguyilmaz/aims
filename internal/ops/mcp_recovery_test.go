package ops

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/testutil"
	"github.com/doguyilmaz/aims/internal/tool"
	"github.com/doguyilmaz/aims/internal/tools"
)

type failingMCP struct {
	tool.Adapter
	path string
}

func (a failingMCP) MCPConfigPath(tool.Home) string { return a.path }
func (a failingMCP) UnregisterMCP(context.Context, tool.Home, string) (bool, error) {
	return true, os.WriteFile(a.path, []byte("removed entry"), 0o600)
}
func (a failingMCP) RegisterMCP(context.Context, tool.Home, tool.MCPServer) error {
	return errors.New("registration failed")
}

func TestMCPFailureKeepsPrivateRecoverySettings(t *testing.T) {
	home := testutil.Sandbox(t)
	path := filepath.Join(home, ".codex", "config.toml")
	original := "unknown = 'keep me'\n[mcp_servers.aims]\ncommand = 'previous'\n"
	testutil.Write(t, path, original)
	a := failingMCP{Adapter: tools.Get("codex"), path: path}
	first, err := backupMCP(a, tool.Home{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := backupMCP(a, tool.Home{})
	if err != nil || first != second {
		t.Fatalf("recovery copy was not reused: %v", err)
	}
	if info, err := os.Stat(first); err != nil || runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatal("recovery copy is not private")
	}
	err = updateMCP(context.Background(), a, tool.Home{}, tool.MCPServer{Name: "aims"}, true)
	if err == nil || !strings.Contains(err.Error(), "registration failed") || !strings.Contains(err.Error(), "previous MCP settings kept at") {
		t.Fatalf("missing recovery guidance: %v", err)
	}
	if testutil.Read(t, first) != original {
		t.Fatal("previous MCP or unknown settings lost")
	}
	files, err := os.ReadDir(filepath.Join(config.Home(), "recovery", "mcp"))
	if err != nil || len(files) != 1 {
		t.Fatalf("duplicate recovery files: %v, %v", files, err)
	}
}
