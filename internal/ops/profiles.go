// Package ops implements what aims' commands and its MCP server do, without
// any terminal I/O.
package ops

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
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
	var out Added
	err := profileOperation(func() error {
		var err error
		out, err = addProfile(a, name, o)
		return err
	})
	return out, err
}

func profileOperation(fn func() error) error {
	return fsx.WithLock(filepath.Join(config.Home(), "profile-operations"), fn)
}

func addProfile(a tool.Adapter, name string, o AddOptions) (Added, error) {
	if !config.ValidName(name) {
		return Added{}, fmt.Errorf("%q is not a valid profile name (letters, digits, '.', '_' and '-'; up to 40)", name)
	}
	if o.Existing && (o.Share != "" && o.Share != config.ShareAll || len(o.Exclude) > 0) {
		return Added{}, fmt.Errorf("an adopted login lives in the shared hub; create a separate profile for private sharing instead of combining --existing with --share or --exclude")
	}
	var out Added
	_, err := config.Update(func(c *config.Config) error {
		id := a.ID()
		c.EnsureHub(id)
		t := c.Tools[id]
		for existing := range t.Profiles {
			if strings.EqualFold(existing, name) {
				return fmt.Errorf("%s already has a profile named %q (names must also differ in case-insensitive filesystems)", a.Title(), existing)
			}
		}
		if o.Share != "" && !config.ValidShare(o.Share) {
			return fmt.Errorf("share must be %s, %s or %s", config.ShareAll, config.ShareSettings, config.ShareNone)
		}
		p := &config.Profile{Generation: rand.Text(), CreatedAt: config.Now().UTC().Format(time.RFC3339), Exclude: o.Exclude}
		if o.Share != "" && o.Share != config.ShareAll {
			p.Share = o.Share
		}
		if o.Existing {
			if other := c.ExistingProfile(id); other != "" {
				return fmt.Errorf("%q already uses the login in %s", other, fsx.Tildify(t.Hub))
			}
			p = &config.Profile{Generation: p.Generation, CreatedAt: p.CreatedAt, Existing: true}
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
		out.Dir = c.Dir(id, name)
		// Never attribute a recreated label to the previous profile's cache.
		if err := config.RemoveState(id, name); err != nil {
			return err
		}
		if o.Existing {
			return nil
		}
		if err := fsx.MkdirAllPrivate(out.Dir); err != nil {
			return err
		}
		if s, ok := a.(tool.Seeder); ok && o.Share != config.ShareNone {
			if err := s.Seed(profiles.HubHome(c, a), out.Dir); err != nil {
				return err
			}
		}
		out.Reports = profiles.Sync(c, a, name, false)
		for _, report := range out.Reports {
			if report.Action == links.Conflict {
				return fmt.Errorf("profile setup incomplete at %s: %s: %s; files kept, fix the problem and retry adding the profile", fsx.Tildify(out.Dir), report.Name, report.Detail)
			}
		}
		return nil
	})
	if err != nil {
		if out.Dir != "" {
			return out, fmt.Errorf("could not finish adding profile; folder kept at %s, fix the error and retry: %w", fsx.Tildify(out.Dir), err)
		}
		return out, err
	}
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
// before a purge failed; registration and files are kept for retry.
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
	var out Removed
	err := profileOperation(func() error {
		var err error
		out, err = removeProfile(ctx, a, name, purge, force)
		return err
	})
	return out, err
}

func removeProfile(ctx context.Context, a tool.Adapter, name string, purge, force bool) (Removed, error) {
	if err := ctx.Err(); err != nil {
		return Removed{}, err
	}
	cfg, err := config.Load()
	if err != nil {
		return Removed{}, err
	}
	p, err := cfg.Profile(a.ID(), name)
	if err != nil {
		return Removed{}, err
	}
	out := Removed{Dir: cfg.Dir(a.ID(), name)}
	var journal *purgeJournal
	purge = purge && !p.Existing
	if purge {
		if err := profiles.HomeConflict(cfg, a, name); err != nil {
			return out, err
		}
		root := config.ProfilesRoot()
		if p.Dir != "" || !fsx.Within(out.Dir, root) || fsx.Abs(out.Dir) == fsx.Abs(root) {
			return out, fmt.Errorf("%s is a custom folder; remove the profile without --purge and delete it yourself", fsx.Tildify(out.Dir))
		}
		if fi := fsx.Lstat(out.Dir); fi != nil && (!fi.IsDir() || fi.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0) {
			return out, fmt.Errorf("%s is not a dedicated profile directory; remove only its label without --purge", fsx.Tildify(out.Dir))
		}
		if p.ShareMode() != config.ShareNone && fsx.Lstat(out.Dir) != nil {
			pending := p.Purge
			p.Purge = nil // explicit purge reconciliation, never a normal launch/doctor repair
			out.Reports = profiles.Sync(cfg, a, name, true)
			p.Purge = pending
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
		// Commit the removal intent before logout or deletion. A failed final
		// config write then leaves an explicitly unavailable, resumable profile,
		// never an active registration whose home silently disappeared.
		if p.Purge == nil || (!p.Purge.LoggedOut && p.Purge.Journal == "") {
			if err := ctx.Err(); err != nil {
				return out, err
			}
			journal, err = openPurgeJournal(a.ID(), name, p)
			if err != nil {
				return out, err
			}
			defer journal.file.Close()
			cfg, err = checkpointPurge(cfg, a, name, config.PurgeState{Journal: filepath.Base(journal.path)})
			if err != nil {
				journal.remove() // no logout occurred, so no recovery is needed
				return out, err
			}
			p = cfg.Tools[a.ID()].Profiles[name]
		}
		// The login may outlive even a missing folder in the keychain/keyring.
		// --force only overrides sharing conflicts, never an incomplete logout.
		if !p.Purge.LoggedOut {
			if journal == nil {
				journal, err = openPurgeJournal(a.ID(), name, p)
				if err != nil {
					return out, fmt.Errorf("folder kept; cannot read logout recovery: %w", err)
				}
				defer journal.file.Close()
			}
			if !journal.completed {
				if out.LogoutErr = nativeLogout(ctx, cfg, a, name); out.LogoutErr != nil {
					return out, fmt.Errorf("profile kept because logout did not complete: %w", out.LogoutErr)
				}
				if err := journal.complete(); err != nil {
					return out, fmt.Errorf("native logout completed but its recovery record could not be synced; folder kept: %w", err)
				}
			}
			state := *p.Purge
			state.LoggedOut = true
			cfg, err = checkpointPurge(cfg, a, name, state)
			if err != nil {
				return out, fmt.Errorf("logout completed; folder kept because its checkpoint could not be saved: %w", err)
			}
			p = cfg.Tools[a.ID()].Profiles[name]
		}
	}
	if _, err := config.Update(func(c *config.Config) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := sameProfile(cfg, c, a.ID(), name); err != nil {
			return err
		}
		if err := config.RemoveState(a.ID(), name); err != nil {
			return err
		}
		if purge {
			if err := links.RemoveLinks(out.Dir); err != nil {
				return err
			}
			if err := os.RemoveAll(out.Dir); err != nil {
				return err
			}
		}
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
		if purge {
			return out, fmt.Errorf("profile purge remains pending; retry aims rm %s %s --purge: %w", a.ID(), name, err)
		}
		return out, err
	}
	out.Purged = purge
	if journal == nil && p.Purge != nil && p.Purge.Journal != "" {
		journal, _ = openPurgeJournal(a.ID(), name, p)
		if journal != nil {
			defer journal.file.Close()
		}
	}
	if journal != nil {
		journal.remove()
	}
	return out, nil
}

func sameProfile(before, current *config.Config, id tool.ID, name string) error {
	if !reflect.DeepEqual(before.Tools[id].Profiles[name], current.Tools[id].Profiles[name]) ||
		before.Tools[id].Hub != current.Tools[id].Hub || before.Dir(id, name) != current.Dir(id, name) {
		return fmt.Errorf("%s/%s changed while the operation was in progress; stopped cleanup, inspect aims doctor --check before retrying", id, name)
	}
	return nil
}

func checkpointPurge(before *config.Config, a tool.Adapter, name string, state config.PurgeState) (*config.Config, error) {
	return config.Update(func(c *config.Config) error {
		if err := sameProfile(before, c, a.ID(), name); err != nil {
			return err
		}
		c.Tools[a.ID()].Profiles[name].Purge = &state
		return nil
	})
}

// Logout runs the tool's own logout for a profile, quietly. Purging a
// profile does this first: on macOS and with a keyring the login lives
// outside the folder and would otherwise outlive it.
func Logout(ctx context.Context, a tool.Adapter, name string) error {
	return profileOperation(func() error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if p, err := cfg.Profile(a.ID(), name); err != nil {
			return err
		} else if p.Purge != nil {
			return fmt.Errorf("profile purge is pending; retry aims rm --purge")
		}
		if err := nativeLogout(ctx, cfg, a, name); err != nil {
			return err
		}
		before := cfg
		cfg, err = config.Update(func(c *config.Config) error {
			if err := sameProfile(before, c, a.ID(), name); err != nil {
				return err
			}
			c.Tools[a.ID()].Profiles[name].Generation = rand.Text()
			return nil
		})
		if err != nil {
			return fmt.Errorf("native logout completed; observation identity could not be changed: %w", err)
		}
		if err := config.UpdateStateFor(cfg, a.ID(), name, func(ps *config.ProfileState) { ps.Account = nil }); err != nil {
			return fmt.Errorf("native logout completed; cached account could not be cleared: %w", err)
		}
		return nil
	})
}

func nativeLogout(ctx context.Context, cfg *config.Config, a tool.Adapter, name string) error {
	if err := profiles.HomeConflict(cfg, a, name); err != nil {
		return err
	}
	if v, ok := profiles.EnvOverride(cfg.Tools[a.ID()].Profiles[name], a.Layout().HomeEnv); ok && v != cfg.HomeEnv(a.ID(), name) {
		return fmt.Errorf("profile environment changes its login home; use the profile dir setting before logging out")
	}
	h := profiles.Home(cfg, a, name)
	if h.Bin == "" {
		return fmt.Errorf("%s is not installed", a.Title())
	}
	out, code, err := proc.CombinedOutputContext(ctx, h.Bin, a.Commands().Logout, h.Env, time.Minute)
	if err != nil {
		return err
	}
	if code != 0 {
		return &tool.CommandError{Output: out, Code: code}
	}
	return nil
}

// Use makes name the profile new sessions of a use in project key, or in
// every folder without an order of its own when key is "". With a nil, it
// does so for every tool that has a profile with that name.
func Use(a tool.Adapter, name, key string) ([]tool.Adapter, error) {
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
			if c.Tools[t.ID()].Profiles[name].Purge != nil {
				return fmt.Errorf("%s/%s has a pending purge; finish its removal before selecting it", t.ID(), name)
			}
			c.Tools[t.ID()].Arrange([]string{name}, key)
			changed = append(changed, t)
		}
		if len(changed) == 0 {
			return fmt.Errorf("no profile named %q (see: aims status)", name)
		}
		return nil
	})
	return changed, err
}

// SetOrder puts names first in the order of project key, or of every folder
// without one of its own when key is "", and returns that order. The first
// is the profile new sessions use; the others stand in for it, in order,
// while it is unusable.
func SetOrder(a tool.Adapter, names []string, key string) ([]string, error) {
	cfg, err := config.Update(func(c *config.Config) error {
		t := c.Tools[a.ID()]
		for _, n := range names {
			if t.Profiles[n] == nil {
				return fmt.Errorf("no %s profile %q", a.ID(), n)
			}
			if t.Profiles[n].Purge != nil {
				return fmt.Errorf("%s/%s has a pending purge; finish its removal before arranging it", a.ID(), n)
			}
		}
		t.Arrange(names, key)
		return nil
	})
	if err != nil {
		return nil, err
	}
	t := cfg.Tools[a.ID()]
	return config.OrderFor(t.Projects[key], t.Order), nil
}

// ForgetOrder drops project key's own order: the order of every other
// folder applies there again. It reports whether there was one.
func ForgetOrder(a tool.Adapter, key string) (bool, error) {
	had := false
	_, err := config.Update(func(c *config.Config) error {
		t := c.Tools[a.ID()]
		_, had = t.Projects[key]
		delete(t.Projects, key)
		return nil
	})
	return had, err
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
	// Switched: To is now the active profile. Otherwise it only stands in
	// until From works again.
	Switched bool `json:"switched,omitempty"`
	// FromLastUsed: From was chosen because it was used last.
	FromLastUsed bool `json:"-"`
}

// ErrNoTarget means every other profile is unusable too.
var ErrNoTarget = errors.New("no other usable profile")

// Failover marks a profile as limited (or logged out). New sessions skip it
// while the mark lasts and use To, the next usable profile, then come back to
// it on their own. With to, or with automatic failover off, To becomes the
// active profile instead, as with Use.
// It changes nothing when there is no profile to switch to.
// by names who asked ("aims failover", "the dashboard"), for the mark.
func Failover(ctx context.Context, a tool.Adapter, from, to string, d time.Duration, reason, by, key string) (FailoverResult, error) {
	cfg, err := config.Load()
	if err != nil {
		return FailoverResult{}, err
	}
	id := a.ID()
	t := cfg.Tools[id]
	res := FailoverResult{From: from, Pinned: config.Pinned(id)}
	if res.From == "" {
		res.From = LastUsed(cfg, config.LoadStateFor(cfg), id)
		res.FromLastUsed = res.From != ""
	}
	if res.From == "" {
		res.From, _ = cfg.Preferred(id, "")
	}
	if res.From == "" || t.Profiles[res.From] == nil {
		return res, fmt.Errorf("no %s profile to fail over from", id)
	}
	if t.Profiles[res.From].Purge != nil {
		return res, fmt.Errorf("%s/%s has a pending purge", id, res.From)
	}
	if to != "" {
		if t.Profiles[to] == nil {
			return res, fmt.Errorf("no %s profile %q", id, to)
		}
		if t.Profiles[to].Purge != nil {
			return res, fmt.Errorf("%s/%s has a pending purge; cannot fail over to it", id, to)
		}
		if to == res.From {
			return res, fmt.Errorf("%s is the profile being marked; --to names the one to use instead", to)
		}
		res.To, res.Switched = to, true
	} else {
		// The profile the next session gets: the preferred one, unless it is
		// the one being marked or unusable itself.
		preferred, _ := cfg.Preferred(id, "")
		pick := profiles.Choose(ctx, cfg, config.LoadStateFor(cfg), a, preferred, []string{res.From})
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
		// Without automatic failover nothing would skip the marked profile.
		res.Switched = !cfg.Failover.Auto
	}
	switch reason {
	case "", "limit":
		if err := profiles.MarkLimited(cfg, id, res.From, profiles.Limit{For: d, By: by}); err != nil {
			return res, err
		}
	case "login":
		// A dead login does not recover on its own; `aims login` clears this.
		if err := profiles.MarkNeedsLogin(id, res.From, by, "", cfg); err != nil {
			return res, err
		}
		res.Login = true
	default:
		return res, fmt.Errorf("reason must be limit or login, not %q", reason)
	}
	if res.Switched {
		if _, err := config.Update(func(c *config.Config) error { c.Tools[id].Arrange([]string{res.To}, key); return nil }); err != nil {
			return res, err
		}
	}
	res.Until = config.LoadStateFor(cfg).Get(id, res.From).Until
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
		if err := profiles.Clear(a.ID(), n, cfg); err != nil {
			return nil, err
		}
	}
	return names, nil
}
