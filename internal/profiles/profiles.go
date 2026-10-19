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

// NestedVar marks a tool started by aims_run, so the aims MCP server it
// loads in turn refuses to start yet another run.
const NestedVar = "AIMS_RUN_NESTED"

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
	// ReadyAt is when the last limit keeping the profile unusable resets.
	// It is zero when the profile is usable, or when something without a
	// known end (a dead login, a limit without a reset time) keeps it out.
	ReadyAt time.Time
	Account tool.Account
	State   *config.ProfileState
}

// Busy reports whether a short rate limit is all that keeps the profile from
// being used.
func (e Evaluation) Busy() bool {
	return len(e.Reasons) == 1 && e.State != nil && e.State.Reason == "busy" && e.State.Until.After(config.Now())
}

// Evaluate checks login, cooldown and usage for one profile.
func Evaluate(ctx context.Context, cfg *config.Config, st config.State, a tool.Adapter, name string) Evaluation {
	ps := st.Get(a.ID(), name)
	acct := a.Account(ctx, Home(cfg, a, name))
	ev := Evaluation{Name: name, Account: acct, State: ps}
	now := config.Now()
	open := false // something keeps the profile out with no known end
	switch {
	case acct.LoggedIn != nil && !*acct.LoggedIn:
		ev.Reasons, open = append(ev.Reasons, "not logged in"), true
	case ps.NeedsLogin:
		ev.Reasons, open = append(ev.Reasons, "login expired"), true
	}
	if ps.Until.After(now) {
		reason := ps.Reason
		switch {
		case reason == "":
			reason = "limited"
		case reason == "limit" && ps.Window != "":
			reason = ps.Window + " limit"
		}
		ev.Reasons = append(ev.Reasons, reason+" for "+Duration(ps.Until.Sub(now)))
		ev.ReadyAt = later(ev.ReadyAt, ps.Until)
	}
	if ps.Usage != nil {
		for _, w := range ps.Usage.Windows {
			if w.Percent >= cfg.Failover.Threshold && (w.ResetsAt.IsZero() || w.ResetsAt.After(now)) {
				r := fmt.Sprintf("%s at %.0f%%", w.Label, w.Percent)
				if w.ResetsAt.IsZero() {
					open = true
				} else {
					r += " for " + Duration(w.ResetsAt.Sub(now))
					ev.ReadyAt = later(ev.ReadyAt, w.ResetsAt)
				}
				ev.Reasons = append(ev.Reasons, r)
			}
		}
	}
	ev.Usable = len(ev.Reasons) == 0
	if open {
		ev.ReadyAt = time.Time{}
	}
	return ev
}

func later(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
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

// Pick is the profile a session should use: the first usable one, trying
// preferred and then the configured order, skipping exclude. Name is "" when
// none is usable; Soonest is then the skipped profile that works again first,
// "" when no reset time is known.
type Pick struct {
	Name    string
	Skipped []Evaluation
	Soonest string
	ReadyAt time.Time
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
		if !ev.ReadyAt.IsZero() && (p.Soonest == "" || ev.ReadyAt.Before(p.ReadyAt)) {
			p.Soonest, p.ReadyAt = n, ev.ReadyAt
		}
	}
	return p
}

// Limit describes a limit mark.
type Limit struct {
	// Reason is "limit" (the plan's limit, the default) or "busy" (a short
	// rate limit).
	Reason string
	// ResetsAt is when the provider said the limit lifts, if it did.
	ResetsAt time.Time
	// For is how long the mark lasts when the reset time is unknown
	// (default: the configured cooldown).
	For time.Duration
	// Window is the usage window the limit is for ("5h", "7d"), if known.
	// A mark without one always counts; see LimitWindow.
	Window string
	// By names who set the mark ("a live check"); Detail is the provider's message.
	By, Detail string
}

// MarkLimited records that a profile hit a limit. Without a reset time or
// duration it uses the soonest reset the provider reported (of the limit's
// window, when known), and keeps a mark that is already set unless the new
// one weighs more: see outweighs.
func MarkLimited(cfg *config.Config, id tool.ID, name string, l Limit) error {
	return config.UpdateState(id, name, func(ps *config.ProfileState) {
		now := config.Now()
		until := l.ResetsAt
		if until.IsZero() && l.For == 0 {
			if ps.Until.After(now) && !outweighs(l, ps) {
				return
			}
			if ps.Usage != nil {
				for _, w := range ps.Usage.Windows {
					if (l.Window == "" || w.Label == l.Window) && w.Percent >= cfg.Failover.Threshold && w.ResetsAt.After(now) && (until.IsZero() || w.ResetsAt.Before(until)) {
						until = w.ResetsAt
					}
				}
			}
		}
		if until.IsZero() {
			d := l.For
			if d == 0 {
				d = time.Duration(cfg.Failover.CooldownMinutes) * time.Minute
			}
			until = now.Add(d)
		}
		reason := l.Reason
		if reason == "" {
			reason = "limit"
		}
		ps.Until, ps.Reason, ps.Window = until.UTC(), reason, l.Window
		ps.MarkedBy, ps.Detail, ps.MarkedAt = l.By, firstLine(l.Detail), now.UTC()
	})
}

// outweighs reports whether a new limit without a reset time replaces the
// live mark ps: a plan limit outlasts a short rate limit, a weekly limit a
// five-hour one.
func outweighs(l Limit, ps *config.ProfileState) bool {
	switch {
	case l.Reason == "busy":
		return false
	case ps.Reason == "busy":
		return true
	}
	return l.Window != "" && !tool.ShortWindow(l.Window) && tool.ShortWindow(ps.Window)
}

// MarkNeedsLogin records that a profile's login stopped working.
func MarkNeedsLogin(id tool.ID, name, by, detail string) error {
	return config.UpdateState(id, name, func(ps *config.ProfileState) {
		ps.NeedsLogin = true
		ps.MarkedBy, ps.Detail, ps.MarkedAt = by, firstLine(detail), config.Now().UTC()
	})
}

// Clear forgets cooldowns and login problems.
func Clear(id tool.ID, name string) error {
	return config.UpdateState(id, name, func(ps *config.ProfileState) {
		ps.Until, ps.Reason, ps.Window, ps.NeedsLogin = time.Time{}, "", "", false
		ps.MarkedBy, ps.Detail, ps.MarkedAt = "", "", time.Time{}
	})
}

// LimitWindow says which usage window a limit the provider reported is for:
// the one its message names, else the one the usage report shows used up
// (the weekly one when both are). "" when neither tells.
func LimitWindow(cfg *config.Config, detail string, windows []tool.Window) string {
	if w := tool.LimitWindow(detail); w != "" {
		return w
	}
	now := config.Now()
	found := ""
	for _, w := range windows {
		if w.Percent < cfg.Failover.Threshold || (!w.ResetsAt.IsZero() && !w.ResetsAt.After(now)) {
			continue
		}
		if !w.Short() {
			return w.Label
		}
		found = w.Label
	}
	return found
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if r := []rune(s); len(r) > 160 {
		s = string(r[:160]) + "…"
	}
	return s
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
	return record(cfg, a, name, a.Probe(ctx, Home(cfg, a, name)))
}

// RefreshDue is how close to running out a login's access token must be for
// KeepAlive to renew it.
const RefreshDue = 10 * time.Minute

// KeepAlive renews a login whose access token has run out or is about to, so
// a refresh token does not lapse while the account is not used. It makes no
// request for a login that is fresh, logged out or of unknown age, and
// reports whether it renewed this one.
func KeepAlive(ctx context.Context, cfg *config.Config, a tool.Adapter, name string) (tool.Probe, tool.Account, bool) {
	h := Home(cfg, a, name)
	acct := a.Account(ctx, h)
	if acct.LoggedIn == nil || !*acct.LoggedIn || acct.Expires.IsZero() || acct.Expires.After(config.Now().Add(RefreshDue)) {
		return tool.Probe{}, acct, false
	}
	var p tool.Probe
	if r, ok := a.(tool.Refresher); ok {
		p = r.Refresh(ctx, h)
	} else {
		p = a.Probe(ctx, h)
	}
	record(cfg, a, name, p)
	return p, a.Account(ctx, h), true
}

// record keeps what a probe found: usage, a limit, a dead login.
func record(cfg *config.Config, a tool.Adapter, name string, p tool.Probe) tool.Probe {
	id := a.ID()
	if len(p.Windows) > 0 {
		_ = RecordUsage(id, name, p.Windows, "live")
	}
	switch p.Status {
	case tool.ProbeOK:
		_ = Clear(id, name)
	case tool.ProbeLogin:
		_ = MarkNeedsLogin(id, name, "a live check", p.Detail)
	case tool.ProbeLimit:
		resets := tool.ResetTime(p.Detail, config.Now())
		for _, w := range p.Windows {
			if w.Percent >= cfg.Failover.Threshold && w.ResetsAt.After(resets) {
				resets = w.ResetsAt
			}
		}
		windows := p.Windows
		if u := config.LoadState().Get(id, name).Usage; len(windows) == 0 && u != nil {
			windows = u.Windows
		}
		_ = MarkLimited(cfg, id, name, Limit{ResetsAt: resets, Window: LimitWindow(cfg, p.Detail, windows), By: "a live check", Detail: p.Detail})
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
