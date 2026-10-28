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

// OnPath reports whether this is the aims on PATH. Only that one brings
// what is installed up to its version, so two copies of different versions
// do not rewrite each other's files back and forth.
func OnPath() bool {
	onPath, err := exec.LookPath("aims")
	self, err2 := os.Executable()
	return err == nil && err2 == nil && fsx.Real(onPath) == fsx.Real(self)
}

// Refresh brings the integrations already installed for a tool up to this
// version of aims: the skill's text and the settings of the MCP server entry.
// It runs before a session starts, so an upgrade needs no setup again.
func Refresh(ctx context.Context, a tool.Adapter) {
	if !OnPath() {
		return
	}
	cfg, err := config.Load()
	if err != nil || cfg.Tools[a.ID()].Hub == "" {
		return
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
}
