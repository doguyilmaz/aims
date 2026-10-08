package ops

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/profiles"
	"github.com/doguyilmaz/aims/internal/tool"
	"github.com/doguyilmaz/aims/internal/tools"
)

// ProfileStatus is one row of `aims status`.
type ProfileStatus struct {
	Name     string        `json:"name"`
	Active   bool          `json:"active"`
	Pinned   bool          `json:"pinned"`
	Existing bool          `json:"existing"`
	Share    string        `json:"share"`
	Dir      string        `json:"dir"`
	Email    string        `json:"email,omitempty"`
	Plan     string        `json:"plan,omitempty"`
	Method   string        `json:"method,omitempty"`
	LoggedIn *bool         `json:"loggedIn"`
	Usable   bool          `json:"usable"`
	Reasons  []string      `json:"reasons,omitempty"`
	Usage    []tool.Window `json:"usage,omitempty"`
	UsageAt  time.Time     `json:"usageAt,omitzero"`
	Until    time.Time     `json:"until,omitzero"`
	// Reason ("limit", "busy"), MarkedBy and Detail explain a mark.
	Reason   string    `json:"reason,omitempty"`
	MarkedBy string    `json:"markedBy,omitempty"`
	Detail   string    `json:"detail,omitempty"`
	MarkedAt time.Time `json:"markedAt,omitzero"`
	LastUsed time.Time `json:"lastUsed,omitzero"`
	// StandsIn names the preferred profile when it is unusable and new
	// sessions get this one instead.
	StandsIn string `json:"standsIn,omitempty"`
}

// ToolStatus is one tool in `aims status`.
type ToolStatus struct {
	ID        tool.ID         `json:"tool"`
	Title     string          `json:"title"`
	Installed bool            `json:"installed"`
	Hub       string          `json:"hub,omitempty"`
	Active    string          `json:"active,omitempty"`
	Pinned    string          `json:"pinned,omitempty"`
	Profiles  []ProfileStatus `json:"profiles"`
}

// Status reports every profile of the given tools (all when empty).
func Status(ctx context.Context, ids []tool.ID) ([]ToolStatus, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		ids = tools.IDs()
	}
	st := config.LoadState()
	out := make([]ToolStatus, len(ids))
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out[i] = toolStatus(ctx, cfg, st, tools.Get(id))
		}()
	}
	wg.Wait()
	return out, nil
}

func toolStatus(ctx context.Context, cfg *config.Config, st config.State, a tool.Adapter) ToolStatus {
	id := a.ID()
	t := cfg.Tools[id]
	ts := ToolStatus{ID: id, Title: a.Title(), Installed: profiles.Bin(a) != "", Hub: t.Hub, Active: t.Active, Pinned: config.Pinned(id)}
	now := config.Now()
	evs := profiles.EvaluateAll(ctx, cfg, st, a)
	standIn, preferred := "", ""
	if cfg.Failover.Auto {
		standIn, preferred = nextSession(cfg, id, evs)
	}
	for _, ev := range evs {
		p := t.Profiles[ev.Name]
		ps := ev.State
		row := ProfileStatus{
			Name: ev.Name, Active: t.Active == ev.Name, Pinned: ts.Pinned == ev.Name,
			Existing: p.Existing, Share: p.ShareMode(), Dir: cfg.Dir(id, ev.Name),
			Email: ev.Account.Email, Plan: ev.Account.Plan, Method: ev.Account.Method,
			LoggedIn: ev.Account.LoggedIn, Usable: ev.Usable, Reasons: ev.Reasons,
			LastUsed: ps.LastUsedAt,
		}
		if ps.Account != nil {
			if row.Email == "" {
				row.Email = ps.Account.Email
			}
			if row.Plan == "" {
				row.Plan = ps.Account.Plan
			}
		}
		if ps.Usage != nil {
			row.Usage, row.UsageAt = ps.Usage.Windows, ps.Usage.At
		}
		if ps.Until.After(now) {
			row.Until, row.Reason = ps.Until, ps.Reason
		}
		if row.Until.After(now) || ps.NeedsLogin {
			row.MarkedBy, row.Detail, row.MarkedAt = ps.MarkedBy, ps.Detail, ps.MarkedAt
		}
		if ev.Name == standIn && standIn != preferred {
			row.StandsIn = preferred
		}
		ts.Profiles = append(ts.Profiles, row)
	}
	return ts
}

// nextSession is the profile a new session gets, the way the launcher picks
// it: the preferred one, else the first usable one in order. It is "" when
// none is usable.
func nextSession(cfg *config.Config, id tool.ID, evs []profiles.Evaluation) (next, preferred string) {
	preferred, _ = cfg.Preferred(id, "")
	usable := map[string]bool{}
	for _, ev := range evs {
		usable[ev.Name] = ev.Usable
	}
	for _, n := range append([]string{preferred}, cfg.Tools[id].Order...) {
		if usable[n] {
			return n, preferred
		}
	}
	return "", preferred
}

// LoginResult is who a profile turned out to be logged in as.
type LoginResult struct {
	Email string
	Plan  string
	// SameAs lists other profiles logged in as the same account.
	SameAs []string
}

// RecordLogin clears the profile's marks after a login and checks that it did
// not land on an account another profile already uses, the usual mistake when
// the browser is still signed in to the first account.
func RecordLogin(ctx context.Context, a tool.Adapter, name string) (LoginResult, error) {
	cfg, err := config.Load()
	if err != nil {
		return LoginResult{}, err
	}
	_ = profiles.Clear(a.ID(), name)
	acct := a.Account(ctx, profiles.Home(cfg, a, name))
	res := LoginResult{Email: acct.Email, Plan: acct.Plan}
	_ = profiles.RecordAccount(a.ID(), name, acct.Email, acct.Plan)
	if acct.Email == "" {
		return res, nil
	}
	for _, other := range cfg.Tools[a.ID()].Order {
		if other == name {
			continue
		}
		if o := a.Account(ctx, profiles.Home(cfg, a, other)); strings.EqualFold(o.Email, acct.Email) {
			res.SameAs = append(res.SameAs, other)
		}
	}
	return res, nil
}
