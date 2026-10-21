package ops

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/profiles"
	"github.com/doguyilmaz/aims/internal/shell"
	"github.com/doguyilmaz/aims/internal/tools"
)

// Tip is a step `aims` suggests: what is still missing for sessions to
// move on at a limit on their own.
type Tip struct {
	Text    string
	Command string
}

// Tips lists those steps, most important first.
func Tips(cfg *config.Config, report []ToolStatus) []Tip {
	if !cfg.HasProfiles() {
		return []Tip{{"Set up your accounts", "aims init"}}
	}
	var out []Tip
	var bins []string
	for _, t := range report {
		if !t.Installed || len(t.Profiles) == 0 {
			continue
		}
		bins = append(bins, "`"+tools.Get(t.ID).Layout().Bin+"`")
		for _, p := range t.Profiles {
			if p.NeedsLogin() {
				out = append(out, Tip{fmt.Sprintf("Log %s in to %s", p.Name, t.Title), "aims login " + string(t.ID) + " " + p.Name})
			}
		}
		if len(t.Profiles) == 1 {
			out = append(out, Tip{fmt.Sprintf("Add a second %s account, so a session can move on at a limit", t.Title), "aims login " + string(t.ID) + " <name>"})
		}
	}
	if sh := shell.Detect(); sh != "" && shell.RCFile(sh) != "" && !shell.Installed(sh) && len(bins) > 0 {
		out = append(out, Tip{"Let plain " + strings.Join(bins, " and ") + " go through aims, so it can move them at a limit", "aims setup --shell " + string(sh)})
	}
	for _, t := range report {
		for _, s := range t.Sessions {
			if s.Mode == profiles.Pinned {
				a := tools.Get(t.ID)
				out = append(out, Tip{fmt.Sprintf("%s in %s is pinned to %s: it stays there at a limit. To let aims move it, end it and run", t.Title, fsx.Brief(s.Dir), s.Profile),
					"aims " + string(t.ID) + " " + strings.Join(a.Commands().Resume, " ")})
			}
		}
	}
	if !cfg.Failover.Auto {
		out = append(out, Tip{"Automatic moves are off: sessions stay on their account at a limit", `"auto": true under "failover" in ` + fsx.Tildify(filepath.Join(config.Home(), "config.json"))})
	}
	return out
}

// NeedsLogin reports whether the profile has to be logged in again.
func (p ProfileStatus) NeedsLogin() bool {
	if p.LoggedIn != nil && !*p.LoggedIn {
		return true
	}
	for _, r := range p.Reasons {
		if strings.HasPrefix(r, "login") {
			return true
		}
	}
	return false
}

// Plan says in a sentence what happens to a new session of the tool.
func (t ToolStatus) Plan(auto bool) string {
	switch {
	case len(t.Profiles) == 0:
		return ""
	case len(t.Profiles) == 1:
		return "sessions use " + t.Profiles[0].Name + "; at a limit they wait for the reset"
	case !auto:
		return "sessions use " + t.Active + "; automatic moves are off"
	}
	var others []string
	for _, p := range t.Profiles {
		if p.Name != t.Active {
			others = append(others, p.Name)
		}
	}
	return fmt.Sprintf("sessions start on %s; at a limit they move to %s, and back once %s works again", t.Active, strings.Join(others, ", then "), t.Active)
}
