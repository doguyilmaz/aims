package ops

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/doguyilmaz/aims"
	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/profiles"
	"github.com/doguyilmaz/aims/internal/tool"
)

// Refresh brings the integrations already installed for a tool up to this
// version of aims: the keepalive job, the skill's text, and the settings of
// the MCP server entry. It runs before a session starts, so an upgrade needs
// no setup again. Only the aims on PATH does it, so two copies of different
// versions do not rewrite each other's files back and forth. The error is a
// keepalive that could not be set up again, told once.
func Refresh(ctx context.Context, a tool.Adapter) error {
	onPath, err := exec.LookPath("aims")
	self, err2 := os.Executable()
	if err != nil || err2 != nil || fsx.Real(onPath) != fsx.Real(self) {
		return nil
	}
	healErr := HealKeepAlive()
	cfg, err := config.Load()
	if err != nil || cfg.Tools[a.ID()].Hub == "" {
		return healErr
	}
	for _, h := range homes(cfg, a) {
		if dir := a.Layout().SkillsDir; dir != "" {
			p := filepath.Join(h.Dir, dir, "aims", "SKILL.md")
			if b, err := os.ReadFile(p); err == nil && string(b) != aims.Skill {
				_ = fsx.WriteFile(p, []byte(aims.Skill), 0o644)
			}
		}
	}
	if au, ok := a.(tool.MCPAuditor); ok && profiles.Bin(a) != "" {
		server := MCPServer()
		for _, h := range mcpHomes(cfg, a) {
			if len(au.MCPMissing(h, server)) > 0 {
				_ = a.RegisterMCP(ctx, h, server)
			}
		}
	}
	return healErr
}
