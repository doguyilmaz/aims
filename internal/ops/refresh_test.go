package ops

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/testutil"
	"github.com/doguyilmaz/aims/internal/tool"
	"github.com/doguyilmaz/aims/internal/tools"
)

type pausedRefresh struct {
	tool.Adapter
	home                    string
	reached, release, wrote chan struct{}
}

func (a pausedRefresh) MCPMissing(h tool.Home, _ tool.MCPServer) []string {
	if h.Dir != a.home {
		return nil
	}
	close(a.reached)
	<-a.release
	return []string{"stale dummy registration"}
}
func (a pausedRefresh) RegisterMCP(_ context.Context, h tool.Home, _ tool.MCPServer) error {
	err := fsx.WriteFile(filepath.Join(h.Dir, "config.toml"), []byte("# dummy aims registration\n"), 0o600)
	if err == nil {
		close(a.wrote)
	}
	return err
}
func (a pausedRefresh) MCPConfigPath(h tool.Home) string { return filepath.Join(h.Dir, "config.toml") }
func TestAutomaticRefreshSerializesWithProfilePurge(t *testing.T) {
	testutil.Sandbox(t)
	cfg, _ := purgeFixture(t)
	cfg, err := config.Update(func(c *config.Config) error { c.Tools["codex"].Order = []string{"work"}; return nil })
	if err != nil {
		t.Fatal(err)
	}
	adapter := logoutAdapter{tools.Get("codex")}
	a := pausedRefresh{adapter, cfg.Dir("codex", "work"), make(chan struct{}), make(chan struct{}), make(chan struct{})}
	done := make(chan struct{})
	go func() { defer close(done); refreshInstalled(t.Context(), a) }()
	var once sync.Once
	release := func() { once.Do(func() { close(a.release) }) }
	defer func() { release(); <-done }()
	<-a.reached
	// The loaded Home is held inside the same lifecycle transaction as purge.
	unlock, ok, err := fsx.Lock(filepath.Join(config.Home(), "profile-operations"), 0)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		unlock()
		t.Fatal("automatic refresh is outside the profile lifecycle lock")
	}
	purged := make(chan error, 1)
	started := make(chan struct{})
	go func() {
		close(started)
		_, err := RemoveProfile(t.Context(), adapter, "work", true, true)
		purged <- err
	}()
	<-started
	release()
	<-done
	select {
	case <-a.wrote:
	default:
		t.Fatal("dummy MCP update did not reach the write sink")
	}
	if err := <-purged; err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(a.home); !os.IsNotExist(err) {
		t.Fatalf("refresh recreated purged home: %v", err)
	}
}
