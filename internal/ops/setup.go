package ops

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/doguyilmaz/aims"
	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/profiles"
	"github.com/doguyilmaz/aims/internal/schedule"
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
	env := []string{profiles.SessionVar, profiles.NestedVar, profiles.RunVar, config.HomeVar}
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
	// KeepAlive schedules `aims keepalive`, which renews logins before they
	// lapse unused.
	KeepAlive bool
	// TrustHooks runs Codex's hooks without review on the restarts aims
	// makes itself (config failover.trustHooks).
	TrustHooks bool
}

// Step is one change made (or attempted) by Setup or Uninstall.
type Step struct {
	Tool   tool.ID
	What   string
	Target string
	Err    error
}

// IntegrationHomes lists the hub and profiles keeping this specific entry
// private. An exclusion is as significant as a fully isolated profile.
func IntegrationHomes(cfg *config.Config, a tool.Adapter, entry string) []tool.Home {
	out := []tool.Home{profiles.HubHome(cfg, a)}
	for _, n := range cfg.Tools[a.ID()].Order {
		p := cfg.Tools[a.ID()].Profiles[n]
		if !p.Existing && profiles.Private(p, a.Layout())(entry) {
			out = append(out, profiles.Home(cfg, a, n))
		}
	}
	return out
}

// MCPHomes is where a tool's MCP servers must be registered: every login when
// the tool keeps them per login, otherwise the shared homes.
func MCPHomes(cfg *config.Config, a tool.Adapter) []tool.Home {
	if a.MCPScope() == tool.MCPShared {
		return IntegrationHomes(cfg, a, a.Layout().SettingsFile)
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
			for _, h := range IntegrationHomes(cfg, a, a.Layout().SkillsDir) {
				p := filepath.Join(h.Dir, a.Layout().SkillsDir, "aims", "SKILL.md")
				steps = append(steps, Step{Tool: id, What: "skill", Target: fsx.Tildify(p), Err: writeSkill(p, []byte(aims.Skill))})
			}
		}
		if in.MCP && installed {
			for _, h := range MCPHomes(cfg, a) {
				steps = append(steps, Step{Tool: id, What: "MCP server", Target: label(h), Err: updateMCP(ctx, a, h, server, true)})
			}
		}
		if sl, ok := a.(tool.StatusLiner); ok && in.StatusLine {
			cmd := shellJoin(append(command, "statusline"))
			for _, h := range IntegrationHomes(cfg, a, a.Layout().SettingsFile) {
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
	if in.KeepAlive {
		path, err := InstallKeepAlive()
		steps = append(steps, Step{What: "login keepalive", Target: fsx.Tildify(path), Err: err})
	}
	if in.TrustHooks {
		steps = append(steps, setTrustHooks(true))
	}
	return steps, nil
}

// setTrustHooks turns failover.trustHooks on or off.
func setTrustHooks(on bool) Step {
	_, err := config.Update(func(c *config.Config) error { c.Failover.TrustHooks = on; return nil })
	return Step{Tool: "codex", What: "hooks without review on aims restarts", Target: "failover.trustHooks", Err: err}
}

// Installed says which integrations are set up now, in any tool.
func Installed() Integrations {
	var in Integrations
	cfg, err := config.Load()
	if err != nil {
		return in
	}
	for _, a := range tools.All() {
		hub := cfg.Tools[a.ID()].Hub
		if hub == "" {
			continue
		}
		if dir := a.Layout().SkillsDir; dir != "" && fsx.Stat(filepath.Join(hub, dir, "aims", "SKILL.md")) != nil {
			in.Skill, in.MCP = true, true
		}
		if sl, ok := a.(tool.StatusLiner); ok && sl.HasStatusLine(hub) {
			in.StatusLine = true
		}
	}
	if sh := shell.Detect(); shell.Installed(sh) {
		in.Shell = sh
	}
	_, in.KeepAlive = schedule.Installed(KeepAliveName)
	in.TrustHooks = cfg.Failover.TrustHooks
	return in
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
			for _, h := range IntegrationHomes(cfg, a, a.Layout().SkillsDir) {
				dir := filepath.Join(h.Dir, a.Layout().SkillsDir, "aims")
				if fsx.Lstat(dir) == nil {
					continue
				}
				path := filepath.Join(dir, "SKILL.md")
				b, readErr := os.ReadFile(path)
				if readErr != nil || string(b) != aims.Skill || managedSkill(path) {
					steps = append(steps, Step{Tool: id, What: "skill", Target: fsx.Tildify(dir), Err: fmt.Errorf("skill was changed or is externally managed; kept it")})
					continue
				}
				err := os.Remove(path)
				if err == nil || errors.Is(err, os.ErrNotExist) {
					// Recovery copies and user additions stay. A nonempty folder
					// is a successful conservative uninstall, not a failed step.
					if entries, e := os.ReadDir(dir); e != nil {
						err = e
					} else if len(entries) == 0 {
						err = os.Remove(dir)
					} else {
						err = nil
					}
				}
				steps = append(steps, Step{Tool: id, What: "skill", Target: fsx.Tildify(dir), Err: err})
			}
		}
		if in.MCP && profiles.Bin(a) != "" {
			for _, h := range MCPHomes(cfg, a) {
				if _, err := backupMCP(a, h); err != nil {
					steps = append(steps, Step{Tool: id, What: "MCP server", Target: label(h), Err: err})
					continue
				}
				if removed, err := a.UnregisterMCP(ctx, h, MCPName); removed || err != nil {
					steps = append(steps, Step{Tool: id, What: "MCP server", Target: label(h), Err: err})
				}
			}
		}
		if sl, ok := a.(tool.StatusLiner); ok && in.StatusLine {
			cmd := shellJoin(append(command, "statusline"))
			for _, h := range IntegrationHomes(cfg, a, a.Layout().SettingsFile) {
				if removed, err := sl.RemoveStatusLine(h.Dir, cmd, cfg.Status.Chain); removed || err != nil {
					steps = append(steps, Step{Tool: id, What: "status line", Target: fsx.Tildify(filepath.Join(h.Dir, "settings.json")), Err: err})
				}
			}
		}
	}
	if in.KeepAlive {
		if path, found, err := UninstallKeepAlive(); found || err != nil {
			steps = append(steps, Step{What: "login keepalive", Target: fsx.Tildify(path), Err: err})
		}
	}
	if in.TrustHooks && cfg.Failover.TrustHooks {
		steps = append(steps, setTrustHooks(false))
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
