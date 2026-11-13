package profiles

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/links"
	"github.com/doguyilmaz/aims/internal/tool"
)

// HomeConflict refuses ambiguous login homes without renaming profiles or
// touching credentials. In particular, old case-variant names may alias on a
// case-insensitive filesystem even though they are distinct config keys.
func HomeConflict(cfg *config.Config, a tool.Adapter, name string) error {
	if _, err := cfg.Profile(a.ID(), name); err != nil {
		return err
	}
	dir := cfg.Dir(a.ID(), name)
	fi := fsx.Stat(dir)
	for _, other := range cfg.Tools[a.ID()].Order {
		if other == name {
			continue
		}
		path := cfg.Dir(a.ID(), other)
		oi := fsx.Stat(path)
		if fsx.Real(dir) == fsx.Real(path) || fi != nil && oi != nil && os.SameFile(fi, oi) {
			return fmt.Errorf("%s profiles %q and %q use the same login folder; remove the duplicate profile label without --purge before continuing", a.ID(), name, other)
		}
	}
	return nil
}

// RequireIsolation checks that a stricter sharing policy is already effective.
// A skipped or failed repair must never silently start new private work through
// an old shared link. This check is read-only, including for bridge exports.
func RequireIsolation(cfg *config.Config, a tool.Adapter, name string) error {
	if err := HomeConflict(cfg, a, name); err != nil {
		return err
	}
	p := cfg.Tools[a.ID()].Profiles[name]
	if p.ShareMode() == config.ShareAll && len(p.Exclude) == 0 {
		return nil
	}
	l := a.Layout()
	private := Private(p, l)
	if p.Existing {
		// An adopted login lives in the hub itself. Detaching its peers cannot
		// give that same folder a separate private history or settings store.
		names := append([]string{}, l.SharedDirs...)
		for _, e := range l.Shared {
			if e.Name != "" {
				names = append(names, e.Name)
			}
		}
		entries, err := os.ReadDir(cfg.Dir(a.ID(), name))
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("cannot inspect %s/%s sharing: %w", a.ID(), name, err)
		}
		for _, e := range entries {
			names = append(names, e.Name())
		}
		for _, other := range cfg.Tools[a.ID()].Order {
			q := cfg.Tools[a.ID()].Profiles[other]
			if other == name || q == nil {
				continue
			}
			for _, entry := range names {
				if l.IsShared(entry) && private(entry) && !Private(q, l)(entry) {
					return fmt.Errorf("%s/%s lives in the shared hub and cannot keep %s private from %q; use a separate private profile or stop sharing that entry in the other profile", a.ID(), name, entry, other)
				}
			}
			for _, r := range CheckSharing(cfg, a, other) {
				if r.Action == links.Conflict && (private(r.Name) && Private(q, l)(r.Name) || filepath.IsAbs(r.Name)) {
					return fmt.Errorf("%s/%s hub privacy change is incomplete in peer %q: %s; close affected tools and run aims doctor --fix", a.ID(), name, other, r.Detail)
				}
			}
		}
		return nil
	}
	for _, r := range CheckSharing(cfg, a, name) {
		if r.Action == links.Conflict && (private(r.Name) || filepath.IsAbs(r.Name)) {
			return fmt.Errorf("%s/%s privacy change is incomplete: %s: %s; close affected tools and run aims doctor --fix before starting this profile", a.ID(), name, r.Name, r.Detail)
		}
	}
	return nil
}
