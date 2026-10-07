// Package ops implements what aims' commands and its MCP server do, without
// any terminal I/O.
package ops

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/links"
	"github.com/doguyilmaz/aims/internal/proc"
	"github.com/doguyilmaz/aims/internal/profiles"
	"github.com/doguyilmaz/aims/internal/tool"
	"github.com/doguyilmaz/aims/internal/tools"
)

// AddOptions configures a new profile.
type AddOptions struct {
	// Existing adopts the login already in the tool's own home.
	Existing bool
	// Share is the sharing mode (config.ShareAll when empty).
	Share string
	// Exclude keeps these shared entries private.
	Exclude []string
	// Dir overrides the profile directory.
	Dir string
}

// Added describes a new profile.
type Added struct {
	Dir     string
	Active  bool
	Reports []links.Report
}

// AddProfile creates a profile.
func AddProfile(a tool.Adapter, name string, o AddOptions) (Added, error) {
	if !config.ValidName(name) {
		return Added{}, fmt.Errorf("%q is not a valid profile name (letters, digits, '.', '_' and '-'; up to 40)", name)
	}
	var out Added
	cfg, err := config.Update(func(c *config.Config) error {
		id := a.ID()
		c.EnsureHub(id)
		t := c.Tools[id]
		if t.Profiles[name] != nil {
			return fmt.Errorf("%s already has a profile named %q", a.Title(), name)
		}
		if o.Share != "" && !config.ValidShare(o.Share) {
			return fmt.Errorf("share must be %s, %s or %s", config.ShareAll, config.ShareSettings, config.ShareNone)
		}
		p := &config.Profile{CreatedAt: config.Now().UTC().Format(time.RFC3339), Exclude: o.Exclude}
		if o.Share != "" && o.Share != config.ShareAll {
			p.Share = o.Share
		}
		if o.Existing {
			if other := c.ExistingProfile(id); other != "" {
				return fmt.Errorf("%q already uses the login in %s", other, fsx.Tildify(t.Hub))
			}
			p = &config.Profile{CreatedAt: p.CreatedAt, Existing: true}
		} else if o.Dir != "" {
			dir, err := checkCustomDir(c, a, fsx.Abs(o.Dir))
			if err != nil {
				return err
			}
			p.Dir = dir
		}
		t.Profiles[name] = p
		t.Order = append(t.Order, name)
		if t.Active == "" {
			t.Active = name
		}
		out.Active = t.Active == name
		return nil
	})
	if err != nil {
		return out, err
	}
	out.Dir = cfg.Dir(a.ID(), name)
	if o.Existing {
		return out, nil
	}
	if err := os.MkdirAll(out.Dir, 0o700); err != nil {
		return out, err
	}
	if s, ok := a.(tool.Seeder); ok && o.Share != config.ShareNone {
		if err := s.Seed(profiles.HubHome(cfg, a), out.Dir); err != nil {
			return out, err
		}
	}
	out.Reports = profiles.Sync(cfg, a, name, false)
	return out, nil
}

// checkCustomDir refuses a profile directory that overlaps something aims
// links into or might delete: home, a hub, ~/.aims or another profile.
func checkCustomDir(c *config.Config, a tool.Adapter, dir string) (string, error) {
	home := fsx.Home()
	protected := []string{config.Home()}
	for _, t := range tools.All() {
		protected = append(protected, filepath.Join(home, t.Layout().DefaultHome))
		if h := c.Tools[t.ID()].Hub; h != "" {
			protected = append(protected, h)
		}
	}
	// Inside sees through symlinks and letter case: ~/link-to-claude and
	// ~/.Claude are ~/.claude.
	if fsx.Inside(home, dir) {
		return "", fmt.Errorf("%s contains your home folder; pick a dedicated folder", dir)
	}
	for _, p := range protected {
		if fsx.Inside(p, dir) && fsx.Inside(dir, p) {
			return "", fmt.Errorf("%s is %s under another name", dir, fsx.Tildify(p))
		}
		if fsx.Inside(p, dir) {
			return "", fmt.Errorf("%s contains %s; pick a dedicated folder", dir, fsx.Tildify(p))
		}
		if fsx.Inside(dir, p) {
			return "", fmt.Errorf("%s is inside %s; pick a folder outside it", dir, fsx.Tildify(p))
		}
	}
	for _, t := range tools.All() {
		for _, n := range c.Tools[t.ID()].Order {
			if o := c.Dir(t.ID(), n); fsx.Inside(dir, o) || fsx.Inside(o, dir) {
				return "", fmt.Errorf("%s overlaps the %s profile %q (%s)", dir, t.ID(), n, fsx.Tildify(o))
			}
		}
	}
	// An existing folder must be a config folder of this tool: aims links and
	// merges its entries, which would rewrite a project's CLAUDE.md or plugins/.
	entries, err := os.ReadDir(dir)
	switch {
	case os.IsNotExist(err):
		return dir, nil
	case err != nil:
		return "", err
	case len(entries) == 0:
		return dir, nil
	case fsx.Lstat(filepath.Join(dir, ".git")) != nil:
		return "", fmt.Errorf("%s is a git repository; a profile needs a folder of its own", dir)
	}
	for _, m := range a.Layout().Markers {
		if fsx.Lstat(filepath.Join(dir, m)) != nil {
			return dir, nil
		}
	}
	return "", fmt.Errorf("%s is not empty and does not look like a %s folder; pick an empty or new folder", dir, a.Title())
}

// Removed describes a removed profile. LogoutErr is set when logging out
// before a purge failed; the purge goes ahead.
type Removed struct {
	Dir       string
	Purged    bool
	Reports   []links.Report
	LogoutErr error
}

// RemoveProfile forgets a profile. With purge it also deletes the profile
// directory, after moving anything shared that the tool wrote there into the
// hub; it never deletes a directory aims did not create.
func RemoveProfile(ctx context.Context, a tool.Adapter, name string, purge, force bool) (Removed, error) {
	cfg, err := config.Load()
	if err != nil {
		return Removed{}, err
	}
	p, err := cfg.Profile(a.ID(), name)
	if err != nil {
		return Removed{}, err
	}
	out := Removed{Dir: cfg.Dir(a.ID(), name)}
	purge = purge && !p.Existing && fsx.Lstat(out.Dir) != nil
	if purge {
		root := config.ProfilesRoot()
		if p.Dir != "" || !fsx.Within(out.Dir, root) || fsx.Abs(out.Dir) == fsx.Abs(root) {
			return out, fmt.Errorf("%s is a custom folder; remove the profile without --purge and delete it yourself", fsx.Tildify(out.Dir))
		}
		if p.ShareMode() != config.ShareNone {
			out.Reports = profiles.Sync(cfg, a, name, true)
			var stuck []string
			for _, r := range out.Reports {
				if r.Action == links.Conflict {
					stuck = append(stuck, fmt.Sprintf("%s (%s)", r.Name, r.Detail))
				}
			}
			if len(stuck) > 0 && !force {
				return out, fmt.Errorf("not deleting %s: %s; resolve it or add --force", fsx.Tildify(out.Dir), strings.Join(stuck, "; "))
			}
		}
		// The login may live in the keychain or a keyring, outside the folder.
		if acct := a.Account(ctx, profiles.Home(cfg, a, name)); profiles.Bin(a) != "" && (acct.LoggedIn == nil || *acct.LoggedIn) {
			out.LogoutErr = Logout(ctx, a, name)
		}
	}
	if _, err := config.Update(func(c *config.Config) error {
		t := c.Tools[a.ID()]
		delete(t.Profiles, name)
		t.Order = slices.DeleteFunc(t.Order, func(n string) bool { return n == name })
		if t.Active == name {
			t.Active = ""
			if len(t.Order) > 0 {
				t.Active = t.Order[0]
			}
		}
		return nil
	}); err != nil {
		return out, err
	}
	_ = config.RemoveState(a.ID(), name)
	if purge {
		if err := links.RemoveLinks(out.Dir); err != nil {
			return out, err
		}
		if err := os.RemoveAll(out.Dir); err != nil {
			return out, err
		}
		out.Purged = true
	}
	return out, nil
}

// Logout runs the tool's own logout for a profile, quietly. Purging a
// profile does this first: on macOS and with a keyring the login lives
// outside the folder and would otherwise outlive it.
func Logout(ctx context.Context, a tool.Adapter, name string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	h := profiles.Home(cfg, a, name)
	if h.Bin == "" {
		return fmt.Errorf("%s is not installed", a.Title())
	}
	out, code, err := proc.CombinedOutput(h.Bin, a.Commands().Logout, h.Env, time.Minute)
	if err != nil {
		return err
	}
	if code != 0 {
		return &tool.CommandError{Output: out, Code: code}
	}
	return config.UpdateState(a.ID(), name, func(ps *config.ProfileState) { ps.Account = nil })
}

// Use makes name the active profile of a, or of every tool that has a profile
// with that name when a is nil.
func Use(a tool.Adapter, name string) ([]tool.Adapter, error) {
	var changed []tool.Adapter
	_, err := config.Update(func(c *config.Config) error {
		targets := tools.All()
		if a != nil {
			targets = []tool.Adapter{a}
		}
		for _, t := range targets {
			if c.Tools[t.ID()].Profiles[name] == nil {
				if a != nil {
					return fmt.Errorf("no %s profile %q (see: aims status)", t.ID(), name)
				}
				continue
			}
			c.Tools[t.ID()].Active = name
			changed = append(changed, t)
		}
		if len(changed) == 0 {
			return fmt.Errorf("no profile named %q (see: aims status)", name)
		}
		return nil
	})
	return changed, err
}

// SetOrder puts names first in the failover order.
func SetOrder(a tool.Adapter, names []string) ([]string, error) {
	cfg, err := config.Update(func(c *config.Config) error {
		t := c.Tools[a.ID()]
		for _, n := range names {
			if t.Profiles[n] == nil {
				return fmt.Errorf("no %s profile %q", a.ID(), n)
			}
		}
		rest := slices.DeleteFunc(slices.Clone(t.Order), func(n string) bool { return slices.Contains(names, n) })
		t.Order = append(slices.Clone(names), rest...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return cfg.Tools[a.ID()].Order, nil
}

// LastUsed is the profile aims launched most recently. It can differ from the
// active one after an automatic skip, and it is the account a person means
// by "the one I was just using".
func LastUsed(cfg *config.Config, st config.State, id tool.ID) string {
	best, at := "", time.Time{}
	for _, n := range cfg.Tools[id].Order {
		if t := st.Get(id, n).LastUsedAt; t.After(at) {
			best, at = n, t
		}
	}
	return best
}

// FailoverResult describes a switch.
type FailoverResult struct {
	From string `json:"from"`
	To   string `json:"to"`
	// Until is when From may be used again; zero when it needs a new login.
	Until  time.Time `json:"until,omitzero"`
	Login  bool      `json:"needsLogin,omitempty"`
	Pinned string    `json:"pinned,omitempty"`
	// FromLastUsed: From was chosen because it was used last.
	FromLastUsed bool `json:"-"`
}

// ErrNoTarget means every other profile is unusable too.
var ErrNoTarget = errors.New("no other usable profile")

// Failover marks a profile as limited (or logged out) and makes the next
// usable one active.
// It changes nothing when there is no profile to switch to.
func Failover(ctx context.Context, a tool.Adapter, from, to string, d time.Duration, reason string) (FailoverResult, error) {
	cfg, err := config.Load()
	if err != nil {
		return FailoverResult{}, err
	}
	id := a.ID()
	t := cfg.Tools[id]
	res := FailoverResult{From: from, Pinned: config.Pinned(id)}
	if res.From == "" {
		res.From = LastUsed(cfg, config.LoadState(), id)
		res.FromLastUsed = res.From != ""
	}
	if res.From == "" {
		res.From, _ = cfg.Preferred(id, "")
	}
	if res.From == "" || t.Profiles[res.From] == nil {
		return res, fmt.Errorf("no %s profile to fail over from", id)
	}
	if to != "" {
		if t.Profiles[to] == nil {
			return res, fmt.Errorf("no %s profile %q", id, to)
		}
		res.To = to
	} else {
		pick := profiles.Choose(ctx, cfg, config.LoadState(), a, "", []string{res.From})
		if pick.Name == "" {
			var why []string
			for _, s := range pick.Skipped {
				why = append(why, s.Name+": "+strings.Join(s.Reasons, ", "))
			}
			if len(why) > 0 {
				return res, fmt.Errorf("%w (%s)", ErrNoTarget, strings.Join(why, "; "))
			}
			return res, ErrNoTarget
		}
		res.To = pick.Name
	}
	switch reason {
	case "", "limit":
		if err := profiles.MarkLimited(cfg, id, res.From, "limit", time.Time{}, d); err != nil {
			return res, err
		}
	case "login":
		// A dead login does not recover on its own; `aims login` clears this.
		if err := profiles.MarkNeedsLogin(id, res.From); err != nil {
			return res, err
		}
		res.Login = true
	default:
		return res, fmt.Errorf("reason must be limit or login, not %q", reason)
	}
	if _, err := config.Update(func(c *config.Config) error { c.Tools[id].Active = res.To; return nil }); err != nil {
		return res, err
	}
	res.Until = config.LoadState().Get(id, res.From).Until
	return res, nil
}

// ClearMarks forgets cooldowns and login problems of one or all profiles.
func ClearMarks(a tool.Adapter, name string) ([]string, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	names := cfg.Tools[a.ID()].Order
	if name != "" {
		if _, err := cfg.Profile(a.ID(), name); err != nil {
			return nil, err
		}
		names = []string{name}
	}
	for _, n := range names {
		if err := profiles.Clear(a.ID(), n); err != nil {
			return nil, err
		}
	}
	return names, nil
}
