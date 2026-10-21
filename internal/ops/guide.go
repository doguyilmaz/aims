package ops

import (
	"fmt"
	"path/filepath"
	"slices"
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
			case p.SameAs != "":
				out = append(out, Tip{fmt.Sprintf("%s/%s is logged in to the same account as %s; log it in to the other one", t.ID, p.Name, p.SameAs), "aims login " + string(t.ID) + " " + p.Name})
			case p.Stale:
				out = append(out, Tip{string(t.ID) + "/" + p.Name + "'s account details are out of date; log in again", "aims login " + string(t.ID) + " " + p.Name})
			}
		}
		if len(t.Profiles) == 1 {
			out = append(out, Tip{string(t.ID) + " has one account, nothing to move to", "aims login " + string(t.ID) + " <name>"})
		}
	}
	if sh := shell.Detect(); sh != "" && shell.RCFile(sh) != "" && !shell.Installed(sh) && len(bins) > 0 {
		skip := " skips aims"
		if len(bins) > 1 {
			skip = " skip aims"
		}
		out = append(out, Tip{"plain " + strings.Join(bins, " and ") + skip, "aims setup --shell " + string(sh)})
	}
	for _, t := range report {
		for _, s := range t.Sessions {
			if s.Mode == profiles.Pinned {
				out = append(out, Tip{fmt.Sprintf("%s in %s is pinned to %s; for auto, restart it with", t.ID, fsx.Brief(s.Dir), s.Profile),
					"aims " + string(t.ID) + " " + strings.Join(tools.Get(t.ID).Commands().Resume, " ")})
			}
		}
	}
	if fsx.Stat(keepAliveFailure()) != nil {
		out = append(out, Tip{"the login keepalive is gone and could not be set up again", "aims keepalive on"})
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

// Explain says in words what aims does with a new session, and what keeps
// an account out right now: the end of `aims status`.
func Explain(cfg *config.Config, report []ToolStatus) []string {
	type group struct {
		bins          []string
		first, others string
	}
	var groups []*group
	var now []string
	pin := ""
	for _, t := range report {
		if !t.Installed || len(t.Profiles) == 0 {
			continue
		}
		bin := tools.Get(t.ID).Layout().Bin
		var others, usable []string
		for _, p := range t.Profiles {
			if p.Name != t.Active {
				others = append(others, p.Name)
			}
			if p.Usable {
				usable = append(usable, p.Name)
			}
		}
		g := &group{first: t.Active, others: strings.Join(others, ", then ")}
		if i := slices.IndexFunc(groups, func(o *group) bool { return o.first == g.first && o.others == g.others }); i >= 0 {
			g = groups[i]
		} else {
			groups = append(groups, g)
		}
		g.bins = append(g.bins, bin)
		if pin == "" && len(others) > 0 {
			pin = fmt.Sprintf("aims %s@%s keeps a session on %s and never moves it.", bin, others[0], others[0])
		}
		for _, p := range t.Profiles {
			if p.Usable {
				continue
			}
			out := fmt.Sprintf("%s/%s cannot be used now (%s)", t.ID, p.Name, strings.Join(p.Reasons, ", "))
			switch {
			case !cfg.Failover.Auto:
			case len(usable) == 0:
				out += "; no " + bin + " account can, so a session waits"
			case p.Name == t.Active:
				out += "; new " + bin + " sessions start on " + usable[0]
			case len(usable) == 1 && usable[0] == t.Active:
				out += "; a " + bin + " session at its limit has nowhere to move"
			}
			now = append(now, out+".")
		}
	}
	var out []string
	for _, g := range groups {
		who, starts := g.bins[0], "starts"
		if len(g.bins) > 1 {
			who, starts = strings.Join(g.bins, " and "), "start"
		}
		switch {
		case g.others == "":
			out = append(out, fmt.Sprintf("%s %s on %s, the only account; at a limit a session waits for the reset.", who, starts, g.first))
		case !cfg.Failover.Auto:
			out = append(out, fmt.Sprintf("%s %s on %s and stays there; auto moves are off.", who, starts, g.first))
		default:
			out = append(out, fmt.Sprintf("%s %s on %s. At a limit or a dead login the conversation continues on %s, and goes back to %s once it works again.", who, starts, g.first, g.others, g.first))
		}
	}
	out = append(out, now...)
	if pin != "" {
		out = append(out, pin)
	}
	return out
}
