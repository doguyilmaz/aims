// Package profiles turns configured profiles into something runnable (an
// environment, a tool.Home) and decides which profile a new session gets.
package profiles

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/links"
	"github.com/doguyilmaz/aims/internal/tool"
)

// SessionVar tells a running tool, and the MCP server it starts, which
// profile it runs as ("claude:work").
const SessionVar = "AIMS_SESSION_PROFILE"

// Bin resolves the tool's binary, honouring its override variable.
func Bin(a tool.Adapter) string {
	l := a.Layout()
	name := l.Bin
	if v := os.Getenv(l.BinEnv); v != "" {
		name = v
	}
	p, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return p
}

// envMap turns KEY=VALUE pairs into a map; later entries win.
func envMap(env []string) map[string]string {
	m := make(map[string]string, len(env))
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok {
			m[k] = v
		}
	}
	return m
}

func envList(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	slices.Sort(out)
	return out
}

// Env is the environment to run a tool as a profile: the home variable set
// (or unset for the adopted login), variables that would override the login
// dropped, the profile's own variables added. dropped lists what was removed.
func Env(cfg *config.Config, a tool.Adapter, name string, base []string) (env []string, dropped []string) {
	p := cfg.Tools[a.ID()].Profiles[name]
	m := envMap(base)
	l := a.Layout()
	for _, k := range l.ConflictingEnv {
		if _, own := p.Env[k]; m[k] != "" && !own {
			delete(m, k)
			dropped = append(dropped, k)
		}
	}
	if v := cfg.HomeEnv(a.ID(), name); v != "" {
		m[l.HomeEnv] = v
	} else {
		delete(m, l.HomeEnv)
	}
	for k, v := range p.Env {
		m[k] = v
	}
	m[SessionVar] = string(a.ID()) + ":" + name
	return envList(m), dropped
}

// Home describes a profile's login to its adapter.
func Home(cfg *config.Config, a tool.Adapter, name string) tool.Home {
	env, _ := Env(cfg, a, name, os.Environ())
	return tool.Home{
		Dir:        cfg.Dir(a.ID(), name),
		EnvValue:   cfg.HomeEnv(a.ID(), name),
		Env:        env,
		Bin:        Bin(a),
		ProfileEnv: cfg.Tools[a.ID()].Profiles[name].Env,
	}
}

// HubHome describes the login that lives in the tool's own home, whether or
// not a profile adopted it.
func HubHome(cfg *config.Config, a tool.Adapter) tool.Home {
	hub := cfg.EnsureHub(a.ID())
	t := cfg.Tools[a.ID()]
	m := envMap(os.Environ())
	if t.HubEnv != "" {
		m[a.Layout().HomeEnv] = t.HubEnv
	} else {
		delete(m, a.Layout().HomeEnv)
	}
	return tool.Home{Dir: hub, EnvValue: t.HubEnv, Env: envList(m), Bin: Bin(a)}
}

// Sync links a profile's shared entries to the hub, and detaches what it
// should no longer share (a sharing mode or exclude changed later).
func Sync(cfg *config.Config, a tool.Adapter, name string, fix bool) []links.Report {
	p := cfg.Tools[a.ID()].Profiles[name]
	if p == nil || p.Existing {
		return nil
	}
	hub := cfg.EnsureHub(a.ID())
	dir := cfg.Dir(a.ID(), name)
	// Profiles of a tool share one hub: two launches repairing links at once
	// could move the same file twice. A launch that cannot get the lock skips
	// the repair; the links it needs are almost always in place already.
	unlock, ok, err := fsx.Lock(filepath.Join(config.Home(), string(a.ID())+"-links"), 5*time.Second)
	if err != nil || !ok {
		return []links.Report{{Name: dir, Action: links.Conflict, Quiet: true, Detail: "another aims process is repairing the shared folders; skipped"}}
	}
	defer unlock()
	private := Private(p, a.Layout())
	out := links.Detach(hub, dir, private, a.Layout())
	if p.ShareMode() == config.ShareNone {
		return out
	}
	return append(out, links.Ensure(links.Options{Hub: hub, Dir: dir, Layout: a.Layout(), Private: private, Label: name, Fix: fix})...)
}

// Private reports which home entries a profile keeps to itself.
func Private(p *config.Profile, l tool.Layout) func(string) bool {
	mode := p.ShareMode()
	return func(name string) bool {
		switch {
		case mode == config.ShareNone:
			return true
		case mode == config.ShareSettings && l.IsHistory(name):
			return true
		}
		return slices.Contains(p.Exclude, name)
	}
}

// Evaluation says whether a new session can use a profile right now.
type Evaluation struct {
	Name    string
	Usable  bool
	Reasons []string
	Account tool.Account
	State   *config.ProfileState
}

// Evaluate checks login, cooldown and usage for one profile.
func Evaluate(ctx context.Context, cfg *config.Config, st config.State, a tool.Adapter, name string) Evaluation {
	ps := st.Get(a.ID(), name)
	acct := a.Account(ctx, Home(cfg, a, name))
	ev := Evaluation{Name: name, Account: acct, State: ps}
	now := config.Now()
	switch {
	case acct.LoggedIn != nil && !*acct.LoggedIn:
		ev.Reasons = append(ev.Reasons, "not logged in")
	case ps.NeedsLogin:
		ev.Reasons = append(ev.Reasons, "login expired")
	}
	if ps.Until.After(now) {
		reason := ps.Reason
		if reason == "" {
			reason = "limited"
		}
		ev.Reasons = append(ev.Reasons, reason+" for "+Duration(ps.Until.Sub(now)))
	}
	if ps.Usage != nil {
		for _, w := range ps.Usage.Windows {
			if w.Percent >= cfg.Failover.Threshold && (w.ResetsAt.IsZero() || w.ResetsAt.After(now)) {
				r := fmt.Sprintf("%s at %.0f%%", w.Label, w.Percent)
				if !w.ResetsAt.IsZero() {
					r += " for " + Duration(w.ResetsAt.Sub(now))
				}
				ev.Reasons = append(ev.Reasons, r)
			}
		}
	}
	ev.Usable = len(ev.Reasons) == 0
	return ev
}

// EvaluateAll checks every profile of a tool in parallel, in configured order.
func EvaluateAll(ctx context.Context, cfg *config.Config, st config.State, a tool.Adapter) []Evaluation {
	order := cfg.Tools[a.ID()].Order
	out := make([]Evaluation, len(order))
	var wg sync.WaitGroup
	for i, n := range order {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out[i] = Evaluate(ctx, cfg, st, a, n)
		}()
	}
	wg.Wait()
	return out
}

// Pick returns the first usable profile, trying preferred and then the
// configured order, skipping exclude. Name is "" when none is usable.
type Pick struct {
	Name    string
	Skipped []Evaluation
}

// Choose finds the profile a new session should use.
func Choose(ctx context.Context, cfg *config.Config, st config.State, a tool.Adapter, preferred string, exclude []string) Pick {
	t := cfg.Tools[a.ID()]
	var candidates []string
	for _, n := range append([]string{preferred}, t.Order...) {
		if n != "" && t.Profiles[n] != nil && !slices.Contains(candidates, n) && !slices.Contains(exclude, n) {
			candidates = append(candidates, n)
		}
	}
	var p Pick
	for _, n := range candidates {
		ev := Evaluate(ctx, cfg, st, a, n)
		if ev.Usable {
			p.Name = n
			return p
		}
		p.Skipped = append(p.Skipped, ev)
	}
	return p
}

// MarkLimited records that a profile hit a limit. Without an explicit reset
// time or duration it uses the soonest reset the provider reported, and never
// stretches a reset time that is already known.
func MarkLimited(cfg *config.Config, id tool.ID, name, reason string, resetsAt time.Time, d time.Duration) error {
	return config.UpdateState(id, name, func(ps *config.ProfileState) {
		now := config.Now()
		until := resetsAt
		if until.IsZero() && d == 0 {
			if ps.Until.After(now) {
				return
			}
			if ps.Usage != nil {
				for _, w := range ps.Usage.Windows {
					if w.Percent >= cfg.Failover.Threshold && w.ResetsAt.After(now) && (until.IsZero() || w.ResetsAt.Before(until)) {
						until = w.ResetsAt
					}
				}
			}
		}
		if until.IsZero() {
			if d == 0 {
				d = time.Duration(cfg.Failover.CooldownMinutes) * time.Minute
			}
			until = now.Add(d)
		}
		if reason == "" {
			reason = "limited"
		}
		ps.Until, ps.Reason = until.UTC(), reason
	})
}

// MarkNeedsLogin records that a profile's login stopped working.
func MarkNeedsLogin(id tool.ID, name string) error {
	return config.UpdateState(id, name, func(ps *config.ProfileState) { ps.NeedsLogin = true })
}

// Clear forgets cooldowns and login problems.
func Clear(id tool.ID, name string) error {
	return config.UpdateState(id, name, func(ps *config.ProfileState) {
		ps.Until, ps.Reason, ps.NeedsLogin = time.Time{}, "", false
	})
}

// RecordUsage stores a usage report.
func RecordUsage(id tool.ID, name string, windows []tool.Window, source string) error {
	return config.UpdateState(id, name, func(ps *config.ProfileState) {
		ps.Usage = &config.Usage{Windows: windows, Source: source, At: config.Now().UTC()}
	})
}

// RecordAccount stores who a profile is logged in as.
func RecordAccount(id tool.ID, name, email, plan string) error {
	if email == "" && plan == "" {
		return nil
	}
	return config.UpdateState(id, name, func(ps *config.ProfileState) {
		ps.Account = &config.Account{Email: email, Plan: plan}
	})
}

// Live asks the provider about a profile and stores what it learned.
func Live(ctx context.Context, cfg *config.Config, a tool.Adapter, name string) tool.Probe {
	p := a.Probe(ctx, Home(cfg, a, name))
	id := a.ID()
	if len(p.Windows) > 0 {
		_ = RecordUsage(id, name, p.Windows, "live")
	}
	switch p.Status {
	case tool.ProbeOK:
		_ = Clear(id, name)
	case tool.ProbeLogin:
		_ = MarkNeedsLogin(id, name)
	case tool.ProbeLimit:
		var resets time.Time
		for _, w := range p.Windows {
			if w.Percent >= cfg.Failover.Threshold && w.ResetsAt.After(resets) {
				resets = w.ResetsAt
			}
		}
		_ = MarkLimited(cfg, id, name, "limit", resets, 0)
	}
	_ = RecordAccount(id, name, p.Email, p.Plan)
	return p
}

// Duration formats a wait for people: "45m", "2h10m", "3d".
func Duration(d time.Duration) string {
	if d <= 0 {
		return "now"
	}
	m := int((d + 30*time.Second) / time.Minute)
	switch {
	case m < 60:
		return fmt.Sprintf("%dm", max(m, 1))
	case m < 48*60:
		if m%60 == 0 {
			return fmt.Sprintf("%dh", m/60)
		}
		return fmt.Sprintf("%dh%dm", m/60, m%60)
	}
	return fmt.Sprintf("%dd", (m+12*60)/(24*60))
}

// Short shortens a path for display.
func Short(p string) string { return fsx.Tildify(p) }
