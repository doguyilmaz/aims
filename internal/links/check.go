package links

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/doguyilmaz/aims/internal/fsx"
)

// Check inspects the same sharing policy as Ensure without creating folders,
// acquiring locks, reading file contents, or invoking a provider. Reports name
// actions still needed, rather than pretending they have already happened.
func Check(o Options) []Report {
	if fsx.Abs(o.Hub) == fsx.Abs(o.Dir) {
		return nil
	}
	if fsx.Inside(o.Dir, o.Hub) || fsx.Inside(o.Hub, o.Dir) {
		return []Report{{Name: o.Dir, Action: Conflict, Detail: "the profile folder overlaps the shared folder " + o.Hub}}
	}
	var out []Report
	for _, dir := range []string{o.Hub, o.Dir} {
		if _, err := os.ReadDir(dir); err != nil && !errors.Is(err, os.ErrNotExist) {
			return []Report{{Name: dir, Action: Conflict, Detail: "cannot inspect folder: " + err.Error()}}
		}
	}
	// Inspect private links too: a changed share/exclude policy is not
	// effective until the old link has been detached.
	for _, name := range readDir(o.Dir) {
		if !o.Layout.IsShared(name) || o.Private == nil || !o.Private(name) {
			continue
		}
		p := filepath.Join(o.Dir, name)
		fi := fsx.Lstat(p)
		hfi := fsx.Lstat(filepath.Join(o.Hub, name))
		if fi != nil && (fi.Mode()&os.ModeSymlink != 0 || isJunction(p) || hfi != nil && sameFile(fi, hfi)) && pointsAtHub(p, o.Hub, name) {
			out = append(out, Report{Name: name, Action: Conflict, Repairable: true, Detail: "sharing policy changed; run aims doctor --fix to detach this entry (history stays in the hub)"})
		}
	}
	for _, name := range sharedNames(o) {
		path, target := filepath.Join(o.Dir, name), filepath.Join(o.Hub, name)
		fi, err := os.Lstat(path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			out = append(out, Report{Name: name, Action: Conflict, Detail: err.Error()})
			continue
		}
		hfi := fsx.Stat(target)
		r := Report{Name: name, Action: OK}
		switch {
		case hfi != nil && (!hfi.IsDir() && slices.Contains(o.Layout.SharedDirs, name) || hfi.IsDir() && name == o.Layout.SettingsFile):
			r.Action, r.Detail = Conflict, "shared target has the wrong file type; left untouched"
		case hfi != nil && !hfi.IsDir() && !hfi.Mode().IsRegular():
			r.Action, r.Detail = Conflict, "shared target is a special file; left untouched"
		case fi == nil:
			if hfi == nil && !slices.Contains(o.Layout.SharedDirs, name) {
				r.Action, r.Detail = Skipped, "not created yet"
			} else {
				r.Action, r.Detail = Missing, "shared link is missing; run aims doctor --fix"
			}
		case pointsTo(path, target), hfi != nil && sameFile(fi, hfi):
			if hfi == nil && slices.Contains(o.Layout.SharedDirs, name) {
				r.Action, r.Detail = Missing, "shared directory is missing; run aims doctor --fix"
			}
		case fi.Mode()&os.ModeSymlink != 0 || isJunction(path):
			// Do not reveal the arbitrary target: it may contain sensitive data.
			r.Action, r.Detail = Conflict, "link points outside the configured shared entry; left untouched"
		case hfi != nil && strings.HasSuffix(name, ".sqlite") && o.StateShared && o.Layout.IsDisposable(name):
			r.Action, r.Detail = Unused, "the tool uses the shared copy; aims doctor --fix archives this one for recovery"
		case hfi != nil && strings.HasSuffix(name, ".sqlite"):
			r.Action, r.Detail = Conflict, "this profile and the shared folder each have their own database; both are kept, including with --fix"
		default:
			r.Action, r.Detail = Conflict, "independent local entry; doctor keeps it; close affected tools and reconcile explicitly with aims sync"
		}
		r.Repairable = r.Action == Missing || r.Action == Unused || r.Action == Conflict && fi != nil && fi.IsDir() && slices.Contains(o.Layout.LockDirs, name)
		out = append(out, r)
	}
	// Codex can redirect databases that are not themselves part of the
	// shared layout. Keep check and repair in agreement about those copies.
	if o.StateShared {
		for _, name := range readDir(o.Dir) {
			fi := fsx.Lstat(filepath.Join(o.Dir, name))
			if fi == nil || !fi.Mode().IsRegular() || !o.Layout.IsDisposable(name) || o.Layout.IsShared(name) || o.Private != nil && o.Private(name) || fsx.Stat(filepath.Join(o.Hub, name)) == nil {
				continue
			}
			out = append(out, Report{Name: name, Action: Unused, Repairable: true, Detail: "the tool uses the shared copy; aims doctor --fix archives this one for recovery"})
		}
	}
	return out
}

func pointsAtHub(path, hub, name string) bool {
	fi, hi := fsx.Lstat(path), fsx.Lstat(filepath.Join(hub, name))
	if fi != nil && hi != nil && sameFile(fi, hi) {
		return true
	}
	return pointsTo(path, filepath.Join(hub, name))
}
