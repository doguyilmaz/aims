package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/doguyilmaz/aims"
	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/links"
	"github.com/doguyilmaz/aims/internal/ops"
	"github.com/doguyilmaz/aims/internal/proc"
	"github.com/doguyilmaz/aims/internal/profiles"
	"github.com/doguyilmaz/aims/internal/shell"
	"github.com/doguyilmaz/aims/internal/tool"
	"github.com/doguyilmaz/aims/internal/tools"
	"github.com/doguyilmaz/aims/internal/ui"
)

// toolVersion asks a binary for its version, first line only.
func toolVersion(bin string) string {
	out, _, err := proc.Output(bin, []string{"--version"}, os.Environ(), 10*time.Second)
	if err != nil {
		return ""
	}
	if l := proc.Lines(out); len(l) > 0 {
		return strings.TrimPrefix(strings.TrimSuffix(l[0], " (Claude Code)"), "codex-cli ")
	}
	return ""
}

func newDoctorCmd(version string) *cobra.Command {
	var fix bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check that every account and shared folder is in order",
		Long: "Checks the tools, each profile's login and shared folders, and the\n" +
			"integrations. It repairs shared folders as it goes; --fix also adopts\n" +
			"databases that look open.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s := ui.Out
			problems := 0
			section := func(t string) { fmt.Println("\n" + s.Bold.Render(t)) }
			ok := func(f string, a ...any) { fmt.Println("  " + s.OK.Render("✓") + " " + fmt.Sprintf(f, a...)) }
			bad := func(f string, a ...any) {
				problems++
				fmt.Println("  " + s.Warn.Render("!") + " " + fmt.Sprintf(f, a...))
			}
			fix1 := func(cmd string) { fmt.Println("    " + s.Dim.Render("→ ") + ui.Kbd(s, cmd)) }

			section("aims")
			exe, _ := os.Executable()
			ok("aims %s %s", version, s.Dim.Render(fsx.Tildify(exe)))
			switch onPath, err := exec.LookPath("aims"); {
			case err != nil:
				bad("aims is not on your PATH; Claude Code and Codex start it as %s", fsx.Tildify(ops.Command()[0]))
			case fsx.Real(onPath) != fsx.Real(exe):
				bad("the aims on your PATH is another copy (%s); Claude Code and Codex start that one", fsx.Tildify(onPath))
			}
			cfg, err := config.Load()
			if err != nil {
				bad("%v", err)
				return fmt.Errorf("fix the config first")
			}
			ok("config %s", s.Dim.Render(fsx.Tildify(filepath.Join(config.Home(), "config.json"))))

			for _, a := range tools.All() {
				id := a.ID()
				section(a.Title())
				bin := profiles.Bin(a)
				if bin == "" {
					if len(cfg.Tools[id].Profiles) > 0 {
						bad("not installed, but aims has profiles for it")
					} else {
						fmt.Println("  " + s.Dim.Render("not installed"))
					}
					continue
				}
				ok("%s %s", toolVersion(bin), s.Dim.Render(fsx.Tildify(bin)))
				for _, k := range a.Layout().ConflictingEnv {
					if os.Getenv(k) != "" {
						bad("%s is set in your shell; it overrides profile logins (aims drops it when it starts %s)", k, id)
					}
				}
				if v := os.Getenv(a.Layout().HomeEnv); v != "" {
					pin := ""
					if p := config.Pinned(id); p != "" {
						pin = " (this terminal is pinned to " + p + ")"
					}
					fmt.Println("  " + s.Dim.Render(a.Layout().HomeEnv+"="+v+pin))
				}
				if len(cfg.Tools[id].Profiles) == 0 {
					fmt.Println("  " + s.Dim.Render("no profiles yet"))
					fix1("aims init")
					continue
				}
				emails := map[string][]string{}
				for _, ev := range profiles.EvaluateAll(cmd.Context(), cfg, config.LoadState(), a) {
					name := ev.Name
					label := string(id) + "/" + name
					if ev.Account.Email != "" {
						label += " " + s.Dim.Render(ev.Account.Email)
						emails[strings.ToLower(ev.Account.Email)] = append(emails[strings.ToLower(ev.Account.Email)], name)
					}
					var conflicts []links.Report
					for _, r := range profiles.Sync(cfg, a, name, fix) {
						switch r.Action {
						case links.Conflict:
							conflicts = append(conflicts, r)
						case links.Merged, links.Adopted, links.Detached:
							fmt.Println("    " + s.Dim.Render("repaired "+r.Name+": "+string(r.Action)))
						}
					}
					switch {
					case ev.Account.LoggedIn != nil && !*ev.Account.LoggedIn:
						bad("%s: not logged in", label)
						fix1("aims login " + string(id) + " " + name)
					case !ev.Usable:
						bad("%s: %s", label, strings.Join(ev.Reasons, ", "))
						if strings.Contains(strings.Join(ev.Reasons, ","), "login") {
							fix1("aims login " + string(id) + " " + name)
						}
					default:
						ok("%s", label)
					}
					for _, c := range conflicts {
						if c.Quiet {
							fmt.Println("    " + s.Dim.Render(c.Name+": "+c.Detail))
							continue
						}
						bad("%s: %s: %s", label, c.Name, c.Detail)
					}
				}
				for email, names := range emails {
					if len(names) > 1 {
						bad("%s are the same account (%s)", strings.Join(names, " and "), email)
						fix1("aims login " + string(id) + " " + names[len(names)-1])
					}
				}
			}

			section("Integrations")
			sh := shell.Detect()
			if shell.Installed(sh) {
				ok("shell integration in %s", fsx.Tildify(shell.RCFile(sh)))
			} else {
				fmt.Println("  " + s.Dim.Render("no shell integration (plain `claude` and `codex` ignore the active profile)"))
				fix1("aims setup --shell")
			}
			for _, a := range tools.All() {
				hub := cfg.Tools[a.ID()].Hub
				if hub == "" {
					continue
				}
				if dir := a.Layout().SkillsDir; dir != "" {
					b, err := os.ReadFile(filepath.Join(hub, dir, "aims", "SKILL.md"))
					switch {
					case err != nil:
						fmt.Println("  " + s.Dim.Render(a.Title()+" skill not installed"))
					case string(b) != aims.Skill:
						bad("%s skill is from another aims version", a.Title())
						fix1("aims setup --no-mcp --tools " + string(a.ID()))
					default:
						ok("%s skill", a.Title())
					}
				}
				// Entries from an older aims can lack settings the tool needs
				// to run the server well.
				if au, isAuditor := a.(tool.MCPAuditor); isAuditor {
					if missing := au.MCPMissing(profiles.HubHome(cfg, a), ops.MCPServer()); len(missing) > 0 {
						bad("%s MCP server is missing %s", a.Title(), strings.Join(missing, " and "))
						fix1("aims setup --no-skill --tools " + string(a.ID()))
					}
				}
			}

			fmt.Println()
			if problems > 0 {
				return exitCode(1)
			}
			ui.Done("everything is in order")
			return nil
		},
	}
	cmd.Flags().BoolVar(&fix, "fix", false, "also adopt databases that look open (close the tools first)")
	return cmd
}
