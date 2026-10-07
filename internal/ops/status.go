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
	LastUsed time.Time     `json:"lastUsed,omitzero"`
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
	for _, ev := range profiles.EvaluateAll(ctx, cfg, st, a) {
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
			row.Until = ps.Until
		}
		ts.Profiles = append(ts.Profiles, row)
	}
	return ts
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
