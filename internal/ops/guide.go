package ops

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

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
	out = append(out, sharingTips(cfg)...)
	if fsx.Stat(keepAliveFailure()) != nil {
		out = append(out, Tip{"the login keepalive is gone and could not be set up again", "aims keepalive on"})
	}
	if !cfg.Failover.Auto {
		out = append(out, Tip{"auto moves are off", `"auto": true under "failover" in ` + fsx.Tildify(filepath.Join(config.Home(), "config.json"))})
	}
	return out
}

// sharingTips name a profile that cannot take part in moves yet because its
// writer locks are its own and still in use, and unused copies of
// databases a profile still keeps.
func sharingTips(cfg *config.Config) []Tip {
	var out []Tip
	for _, id := range tools.IDs() {
		t, l, hub := cfg.Tools[id], tools.Get(id).Layout(), cfg.EnsureHub(id)
		for _, n := range t.Order {
			p := t.Profiles[n]
			if p.Existing || p.ShareMode() != config.ShareAll {
				continue
			}
			dir := cfg.Dir(id, n)
			for _, d := range l.LockDirs {
				if fi := fsx.Lstat(filepath.Join(dir, d)); fi != nil && fi.IsDir() {
					out = append(out, Tip{fmt.Sprintf("%s/%s takes no part in moves until no %s uses it; aims then shares its writer locks", id, n, tools.Get(id).Title()), "aims doctor"})
				}
			}
			if !profiles.StateShared(cfg, tools.Get(id), n) {
				continue
			}
			var size int64
			entries, _ := os.ReadDir(dir)
			for _, e := range entries {
				if e.Type().IsRegular() && l.IsDisposable(e.Name()) && fsx.Stat(filepath.Join(hub, e.Name())) != nil {
					if fi, err := e.Info(); err == nil {
						size += fi.Size()
					}
				}
			}
			if size >= 1<<20 {
				out = append(out, Tip{fmt.Sprintf("%s/%s keeps %d MB of unused database copies", id, n, size>>20), "aims doctor --fix"})
			}
		}
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
	return "new sessions: " + t.Active + " (auto → " + profiles.Names(others, ", ") + ")"
}

// Outlook says what a session of the tool does now that an account is out:
// waits, has nowhere to move, or starts on another one. "" when nothing is.
func (t ToolStatus) Outlook(auto bool) string {
	var usable []string
	var soonest time.Time
	out := false
	for _, p := range t.Profiles {
		switch {
		case p.Usable:
			usable = append(usable, p.Name)
		default:
			out = true
			if !p.ReadyAt.IsZero() && (soonest.IsZero() || p.ReadyAt.Before(soonest)) {
				soonest = p.ReadyAt
			}
		}
	}
	switch {
	case !out || !auto && len(usable) > 0:
		return ""
	case len(usable) == 0 && !soonest.IsZero():
		return "no account works now; sessions wait " + profiles.Duration(time.Until(soonest))
	case len(usable) == 0:
		return "no account works now; sessions wait"
	case !slices.Contains(usable, t.Active):
		return "new sessions start on " + usable[0]
	case len(usable) == 1 && len(t.Profiles) > 1:
		return "a session at its limit has nowhere to move"
	}
	return ""
}
