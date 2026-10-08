package ops

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/doguyilmaz/aims"
	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/profiles"
	"github.com/doguyilmaz/aims/internal/shell"
	"github.com/doguyilmaz/aims/internal/tool"
	"github.com/doguyilmaz/aims/internal/tools"
)

// MCPName is the name aims registers its MCP server under.
const MCPName = "aims"

// MaxRun is the longest an aims_run call may take.
const MaxRun = time.Hour

// MCPServer is the aims MCP server as the tools register it.
func MCPServer() tool.MCPServer {
	env := []string{profiles.SessionVar, profiles.NestedVar, config.HomeVar}
	for _, a := range tools.All() {
		l := a.Layout()
		env = append(env, config.PinVar(a.ID()), l.BinEnv, l.HomeEnv)
	}
	// A minute more than the longest aims_run, so the run reports its own timeout.
	return tool.MCPServer{Name: MCPName, Command: append(Command(), "mcp"), Env: env, Timeout: MaxRun + time.Minute}
}

// Command is how Claude Code and Codex start aims: plain "aims" when it is on
// PATH, otherwise this binary's full path.
func Command() []string {
	if _, err := exec.LookPath("aims"); err == nil {
		return []string{"aims"}
	}
	if self, err := os.Executable(); err == nil {
		if r, err := filepath.EvalSymlinks(self); err == nil {
			self = r
		}
		return []string{self}
	}
	return []string{"aims"}
}

// Integrations selects what Setup installs or Uninstall removes.
type Integrations struct {
	Skill      bool
	MCP        bool
	StatusLine bool
	Shell      shell.Shell // "" for none
}

// Step is one change made (or attempted) by Setup or Uninstall.
type Step struct {
	Tool   tool.ID
	What   string
	Target string
	Err    error
}

// homes returns the logins an integration goes into: the hub, plus isolated
// profiles, which do not see the hub's files.
func homes(cfg *config.Config, a tool.Adapter) []tool.Home {
	out := []tool.Home{profiles.HubHome(cfg, a)}
	for _, n := range cfg.Tools[a.ID()].Order {
		if cfg.Tools[a.ID()].Profiles[n].ShareMode() == config.ShareNone {
			out = append(out, profiles.Home(cfg, a, n))
		}
	}
	return out
}

// mcpHomes is where a tool's MCP servers must be registered: every login when
// the tool keeps them per login, otherwise the shared homes.
func mcpHomes(cfg *config.Config, a tool.Adapter) []tool.Home {
	if a.MCPScope() == tool.MCPShared {
		return homes(cfg, a)
	}
	var out []tool.Home
	if cfg.ExistingProfile(a.ID()) == "" {
		out = append(out, profiles.HubHome(cfg, a))
	}
	for _, n := range cfg.Tools[a.ID()].Order {
		out = append(out, profiles.Home(cfg, a, n))
	}
	return out
}

func label(h tool.Home) string { return fsx.Tildify(h.Dir) }

// Setup installs the chosen integrations into the given tools (all when empty).
func Setup(ctx context.Context, ids []tool.ID, in Integrations) ([]Step, error) {
	cfg, err := config.Update(func(c *config.Config) error {
		for _, id := range idsOrAll(ids) {
			c.EnsureHub(id)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	command := Command()
	server := MCPServer()
	var steps []Step
	for _, id := range idsOrAll(ids) {
		a := tools.Get(id)
		installed := profiles.Bin(a) != ""
		if in.Skill && a.Layout().SkillsDir != "" {
			for _, h := range homes(cfg, a) {
				p := filepath.Join(h.Dir, a.Layout().SkillsDir, "aims", "SKILL.md")
				steps = append(steps, Step{Tool: id, What: "skill", Target: fsx.Tildify(p), Err: fsx.WriteFile(p, []byte(aims.Skill), 0o644)})
			}
		}
		if in.MCP && installed {
			for _, h := range mcpHomes(cfg, a) {
				_, _ = a.UnregisterMCP(ctx, h, MCPName)
				steps = append(steps, Step{Tool: id, What: "MCP server", Target: label(h), Err: a.RegisterMCP(ctx, h, server)})
			}
		}
		if sl, ok := a.(tool.StatusLiner); ok && in.StatusLine {
			cmd := shellJoin(append(command, "statusline"))
			for _, h := range homes(cfg, a) {
				prev, err := sl.InstallStatusLine(h.Dir, cmd)
				if err == nil && prev != "" {
					_, err = config.Update(func(c *config.Config) error { c.Status.Chain = prev; return nil })
				}
				steps = append(steps, Step{Tool: id, What: "status line", Target: fsx.Tildify(filepath.Join(h.Dir, "settings.json")), Err: err})
			}
		}
	}
	if in.Shell != "" {
		path, err := shell.Install(in.Shell)
		steps = append(steps, Step{What: "shell integration", Target: fsx.Tildify(path), Err: err})
	}
	return steps, nil
}

// Uninstall removes what Setup added. Profiles and logins stay.
func Uninstall(ctx context.Context, in Integrations) ([]Step, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	command := Command()
	var steps []Step
	for _, a := range tools.All() {
		id := a.ID()
		if cfg.Tools[id].Hub == "" {
			continue
		}
		if in.Skill && a.Layout().SkillsDir != "" {
			for _, h := range homes(cfg, a) {
				dir := filepath.Join(h.Dir, a.Layout().SkillsDir, "aims")
				if fsx.Lstat(dir) == nil {
					continue
				}
				err := os.Remove(filepath.Join(dir, "SKILL.md"))
				if err == nil || errors.Is(err, os.ErrNotExist) {
					err = os.Remove(dir) // only if empty: never delete files aims did not write
				}
				steps = append(steps, Step{Tool: id, What: "skill", Target: fsx.Tildify(dir), Err: err})
			}
		}
		if in.MCP && profiles.Bin(a) != "" {
			for _, h := range mcpHomes(cfg, a) {
				if removed, err := a.UnregisterMCP(ctx, h, MCPName); removed || err != nil {
					steps = append(steps, Step{Tool: id, What: "MCP server", Target: label(h), Err: err})
				}
			}
		}
		if sl, ok := a.(tool.StatusLiner); ok && in.StatusLine {
			cmd := shellJoin(append(command, "statusline"))
			for _, h := range homes(cfg, a) {
				if removed, err := sl.RemoveStatusLine(h.Dir, cmd, cfg.Status.Chain); removed || err != nil {
					steps = append(steps, Step{Tool: id, What: "status line", Target: fsx.Tildify(filepath.Join(h.Dir, "settings.json")), Err: err})
				}
			}
		}
	}
	if in.Shell != "" {
		for _, sh := range shell.All {
			if path, found, err := shell.Uninstall(sh); found || err != nil {
				steps = append(steps, Step{What: "shell integration", Target: fsx.Tildify(path), Err: err})
			}
		}
	}
	return steps, nil
}

// SyncMCP copies MCP servers added to the hub's login into each profile of
// tools that keep them per login.
func SyncMCP(ctx context.Context) (map[string][]string, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	out := map[string][]string{}
	for _, a := range tools.All() {
		s, ok := a.(tool.MCPSyncer)
		if !ok {
			continue
		}
		hub := profiles.HubHome(cfg, a)
		for _, n := range cfg.Tools[a.ID()].Order {
			p := cfg.Tools[a.ID()].Profiles[n]
			if p.Existing || p.ShareMode() == config.ShareNone {
				continue
			}
			added, err := s.SyncMCP(hub, profiles.Home(cfg, a, n))
			if err != nil {
				return out, err
			}
			if len(added) > 0 {
				out[string(a.ID())+"/"+n] = added
			}
		}
	}
	return out, nil
}

func idsOrAll(ids []tool.ID) []tool.ID {
	if len(ids) == 0 {
		return tools.IDs()
	}
	return ids
}

// shellJoin quotes a command for a settings file that runs it through a
// shell. Anything outside a small safe set is single-quoted.
func shellJoin(parts []string) string {
	quoted := make([]string, len(parts))
	for i, p := range parts {
		if p == "" || strings.IndexFunc(p, unsafeRune) >= 0 {
			p = "'" + strings.ReplaceAll(p, "'", `'\''`) + "'"
		}
		quoted[i] = p
	}
	return strings.Join(quoted, " ")
}

func unsafeRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return false
	}
	return !strings.ContainsRune("/._-+:@%,=", r)
}
