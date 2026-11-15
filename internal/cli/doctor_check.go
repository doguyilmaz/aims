package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/doguyilmaz/aims"
	"github.com/doguyilmaz/aims/internal/bridge"
	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/links"
	"github.com/doguyilmaz/aims/internal/ops"
	"github.com/doguyilmaz/aims/internal/profiles"
	"github.com/doguyilmaz/aims/internal/schedule"
	"github.com/doguyilmaz/aims/internal/shell"
	"github.com/doguyilmaz/aims/internal/tool"
	"github.com/doguyilmaz/aims/internal/tools"
)

type doctorCheck struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Message string `json:"message"`
	Command string `json:"command,omitempty"`
}

type doctorReport struct {
	SchemaVersion string        `json:"schemaVersion"`
	AimsVersion   string        `json:"aimsVersion"`
	Mode          string        `json:"mode"`
	Checks        []doctorCheck `json:"checks"`
	Repairs       []doctorCheck `json:"repairs"`
}

func runDoctorCheck(cmd *cobra.Command, version string, fix, asJSON bool) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	r := doctorReport{SchemaVersion: "1.0.0", AimsVersion: version, Mode: "check", Checks: []doctorCheck{}, Repairs: []doctorCheck{}}
	add := func(id, status, message, command string) {
		r.Checks = append(r.Checks, doctorCheck{id, status, message, command})
	}
	if fix {
		r.Mode = "fix"
		for _, a := range tools.All() {
			for _, name := range cfg.Tools[a.ID()].Order {
				p := cfg.Tools[a.ID()].Profiles[name]
				if p.Purge != nil {
					continue // only another explicitly confirmed rm --purge may delete this home
				}
				if v, ok := profiles.EnvOverride(p, a.Layout().HomeEnv); ok && v != cfg.HomeEnv(a.ID(), name) {
					continue
				}
				for _, rep := range profiles.RepairSharing(cfg, a, name) {
					if rep.Action != links.OK && rep.Action != links.Skipped {
						r.Repairs = append(r.Repairs, doctorCheck{string(a.ID()) + "/" + name + "/" + rep.Name, string(rep.Action), rep.Detail, ""})
					}
				}
			}
		}
		// Never install optional launchers implicitly. Only repair an existing,
		// owned installation, using the installer's preflight and rollback.
		if m, e := bridge.ReadLaunchers(bridge.LauncherRoot()); e == nil && m != nil {
			if problems := bridge.CheckLaunchers(bridge.LauncherRoot()); len(problems) > 0 {
				self, e := os.Executable()
				if fsx.Real(m.ConfigHome) != fsx.Real(config.Home()) {
					e = fmt.Errorf("launchers belong to another aims configuration home; kept them")
				} else if e == nil {
					_, e = bridge.InstallLaunchers(self)
				}
				status, message := "repaired", "updated owned bridge launchers"
				if e != nil {
					status, message = "conflict", e.Error()
				}
				r.Repairs = append(r.Repairs, doctorCheck{"bridge", status, message, ""})
			}
		}
	}
	add("aims", "ok", "aims "+version, "")
	for _, a := range tools.All() {
		id, l := a.ID(), a.Layout()
		t := cfg.Tools[id]
		if bin := profiles.Bin(a); bin != "" {
			add(string(id)+"/binary", "ok", a.Title()+" executable: "+fsx.Tildify(bin), "")
		} else if len(t.Profiles) > 0 {
			add(string(id)+"/binary", "problem", a.Title()+" is not installed, but profiles exist", "")
		}
		if len(t.Profiles) == 0 {
			add(string(id)+"/profiles", "info", "no "+a.Title()+" profiles yet", "aims init")
			continue
		}
		for _, name := range t.Order {
			key := string(id) + "/" + name
			p := t.Profiles[name]
			if p.Purge != nil {
				add(key+"/purge", "problem", key+": profile purge is pending; normal launches are disabled and doctor keeps its files", "aims rm "+string(id)+" "+name+" --purge")
				continue
			}
			if err := profiles.RequireIsolation(cfg, a, name); err != nil {
				add(key+"/isolation", "problem", err.Error(), "")
			}
			if v, ok := profiles.EnvOverride(p, l.HomeEnv); ok && v != cfg.HomeEnv(id, name) {
				add(key+"/home", "problem", "profile environment changes its login home; use the profile dir setting instead of overriding "+l.HomeEnv, "")
			}
			_, customState := profiles.EnvOverride(p, l.StateEnv)
			_, customMemory := profiles.EnvOverride(p, l.MemoryEnv)
			if l.StateEnv != "" && customState || l.MemoryEnv != "" && customMemory {
				add(key+"/storage", "info", key+": explicit persistent-storage routing; automatic moves require matching storage homes and shared history", "aims bridge")
			}
			for _, rep := range profiles.CheckSharing(cfg, a, name) {
				if rep.Action == links.Conflict || rep.Action == links.Missing {
					command := ""
					if rep.Repairable {
						command = "aims doctor --fix"
					}
					add(key+"/"+rep.Name, "problem", key+": "+rep.Name+": "+rep.Detail, command)
				} else if rep.Action == links.Unused {
					add(key+"/"+rep.Name, "info", key+": "+rep.Name+": "+rep.Detail, "aims doctor --fix")
				}
			}
		}
		if t.Hub != "" {
			if d, ok := a.(tool.Diagnoser); ok {
				for i, message := range d.Diagnose(profiles.HubHome(cfg, a)) {
					add(fmt.Sprintf("%s/settings/%d", id, i), "problem", message, "")
				}
			}
			if l.SkillsDir != "" {
				for i, h := range ops.IntegrationHomes(cfg, a, l.SkillsDir) {
					key := fmt.Sprintf("%s/skill/%d", id, i)
					b, e := os.ReadFile(filepath.Join(h.Dir, l.SkillsDir, "aims", "SKILL.md"))
					switch {
					case os.IsNotExist(e):
						add(key, "info", a.Title()+" skill not installed in "+fsx.Tildify(h.Dir), "aims setup --no-mcp --tools "+string(id))
					case e != nil:
						add(key, "problem", a.Title()+" skill cannot be read in "+fsx.Tildify(h.Dir), "")
					case string(b) != aims.Skill:
						add(key, "problem", a.Title()+" skill is from another aims version or has local edits in "+fsx.Tildify(h.Dir), "aims setup --no-mcp --tools "+string(id))
					default:
						add(key, "ok", a.Title()+" skill in "+fsx.Tildify(h.Dir), "")
					}
				}
			}
			if au, ok := a.(tool.MCPAuditor); ok {
				for i, h := range ops.MCPHomes(cfg, a) {
					if missing := au.MCPMissing(h, ops.MCPServer()); len(missing) > 0 {
						add(fmt.Sprintf("%s/mcp/%d", id, i), "problem", a.Title()+" MCP server is missing "+strings.Join(missing, " and ")+" in "+fsx.Tildify(h.Dir), "aims setup --no-skill --tools "+string(id))
					}
				}
			}
		}
	}
	for _, tr := range bridge.Inspect(cfg, os.Environ()) {
		for i, w := range tr.Warnings {
			add(fmt.Sprintf("%s/routing/%d", tr.Tool, i), "info", w, "aims bridge")
		}
		if len(tr.ConflictingEnv) > 0 {
			add(string(tr.Tool)+"/environment", "info", "inherited login overrides: "+strings.Join(tr.ConflictingEnv, ", ")+" (aims drops these for a selected profile)", "")
		}
	}
	for i, message := range bridge.CheckLaunchers(bridge.LauncherRoot()) {
		add(fmt.Sprintf("bridge/%d", i), "problem", message, "aims bridge install")
	}
	if shell.Installed(shell.Detect()) {
		add("shell", "ok", "shell integration", "")
	} else {
		add("shell", "info", "no shell integration", "aims setup --shell")
	}
	if _, ok := schedule.Installed(ops.KeepAliveName); ok {
		add("keepalive", "ok", "login keepalive installed", "")
	} else {
		add("keepalive", "info", "no login keepalive", "aims keepalive on")
	}
	problems := false
	for _, c := range r.Checks {
		problems = problems || c.Status == "problem"
	}
	for _, c := range r.Repairs {
		problems = problems || c.Status == string(links.Conflict)
	}
	if asJSON {
		if err := writeJSON(cmd, r); err != nil {
			return err
		}
	} else {
		out := cmd.OutOrStdout()
		for _, c := range r.Repairs {
			fmt.Fprintf(out, "repair %s: %s %s\n", c.ID, c.Status, c.Message)
		}
		for _, c := range r.Checks {
			fmt.Fprintf(out, "%s: %s\n", c.Status, c.Message)
			if c.Command != "" {
				fmt.Fprintln(out, "  → "+c.Command)
			}
		}
		fmt.Fprintln(out, "Login validity was not probed. Use aims doctor --logins for local login inspection.")
	}
	if problems {
		return exitCode(1)
	}
	return nil
}
