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
		return []Tip{{"no accounts yet", "aims init"}}
	}
	var out []Tip
	var bins []string
	for _, t := range report {
		if !t.Installed || len(t.Profiles) == 0 {
			continue
		}
		bins = append(bins, tools.Get(t.ID).Layout().Bin)
		for _, p := range t.Profiles {
			switch {
			case p.NeedsLogin():
				out = append(out, Tip{string(t.ID) + "/" + p.Name + " is not logged in", "aims login " + string(t.ID) + " " + p.Name})
			case p.Stale:
				out = append(out, Tip{string(t.ID) + "/" + p.Name + "'s account details are out of date; log in again", "aims login " + string(t.ID) + " " + p.Name})
			}
		}
		if len(t.Profiles) == 1 {
			out = append(out, Tip{string(t.ID) + " has one account, nothing to move to", "aims login " + string(t.ID) + " <name>"})
		}
	}
	if sh := shell.Detect(); sh != "" && shell.RCFile(sh) != "" && !shell.Installed(sh) && len(bins) > 0 {
		out = append(out, Tip{"plain " + strings.Join(bins, " and ") + " skip aims", "aims setup --shell " + string(sh)})
	}
	for _, t := range report {
		for _, s := range t.Sessions {
			if s.Mode == profiles.Pinned {
				out = append(out, Tip{fmt.Sprintf("%s in %s is pinned to %s; for auto, restart it with", t.ID, fsx.Brief(s.Dir), s.Profile),
					"aims " + string(t.ID) + " " + strings.Join(tools.Get(t.ID).Commands().Resume, " ")})
			}
		}
	}
	if !cfg.Failover.Auto {
		out = append(out, Tip{"auto moves are off", `"auto": true under "failover" in ` + fsx.Tildify(filepath.Join(config.Home(), "config.json"))})
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

// Plan says what a new session of the tool does: "personal (auto → work)".
func (t ToolStatus) Plan(auto bool) string {
	switch {
	case len(t.Profiles) == 0:
		return ""
	case len(t.Profiles) == 1:
		return "new sessions: " + t.Active + " (auto, one account)"
	case !auto:
		return "new sessions: " + t.Active + " (auto off)"
	}
	var others []string
	for _, p := range t.Profiles {
		if p.Name != t.Active {
			others = append(others, p.Name)
		}
	}
	return "new sessions: " + t.Active + " (auto → " + strings.Join(others, ", ") + ")"
}
