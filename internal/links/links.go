// Package links keeps a profile directory's shared entries pointing at the hub.
//
// Tools sometimes replace a symlink with a real file (an atomic "write temp,
// rename"), or create an entry before aims linked it. Ensure repairs that on
// every run: newer content moves into the hub and the link comes back, so
// nothing written under one account is lost or hidden from the others.
package links

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/tool"
)

// Action is what happened to one shared entry.
type Action string

const (
	OK       Action = "ok"
	Linked   Action = "linked"
	Adopted  Action = "adopted"  // the profile's copy moved into the hub
	Merged   Action = "merged"   // both had content; combined in the hub
	Skipped  Action = "skipped"  // nothing to link yet
	Detached Action = "detached" // stopped sharing
	Conflict Action = "conflict" // needs a person
)

// Report describes one entry.
type Report struct {
	Name   string
	Action Action
	Detail string
	// Quiet conflicts cannot be fixed automatically and are shown only by
	// doctor and sync, not on every launch.
	Quiet bool
}

// Options configures Ensure.
type Options struct {
	Hub, Dir string
	Layout   tool.Layout
	// Private names entries that stay private to the profile.
	Private func(name string) bool
	// Label names kept duplicates, e.g. "settings.json.aims-work-<time>".
	Label string
	// Fix also adopts databases that look open.
	Fix bool
}

func readDir(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Name()
	}
	return out
}

// sharedNames expands the spec (names and patterns) against what is on disk.
func sharedNames(o Options) []string {
	var out []string
	add := func(n string) {
		if !slices.Contains(out, n) && (o.Private == nil || !o.Private(n)) {
			out = append(out, n)
		}
	}
	present := append(readDir(o.Hub), readDir(o.Dir)...)
	for _, e := range o.Layout.Shared {
		if e.Pattern == nil {
			add(e.Name)
			continue
		}
		for _, n := range present {
			if e.Match(n) {
				add(n)
			}
		}
	}
	return out
}

func stamp() string { return time.Now().UTC().Format("20060102T150405Z") }

// pointsTo reports whether the link at p resolves to target.
func pointsTo(p, target string) bool {
	a, err1 := filepath.EvalSymlinks(p)
	b, err2 := filepath.EvalSymlinks(target)
	if err1 == nil && err2 == nil {
		return a == b
	}
	raw, err := os.Readlink(p)
	if err != nil {
		return false
	}
	if !filepath.IsAbs(raw) {
		raw = filepath.Join(filepath.Dir(p), raw)
	}
	return filepath.Clean(raw) == filepath.Clean(target)
}

// move renames, falling back to copy and delete across file systems.
func move(from, to string) error {
	if err := os.Rename(from, to); err == nil {
		return nil
	}
	if err := copyTree(from, to); err != nil {
		return err
	}
	return os.RemoveAll(from)
}

func copyTree(from, to string) error {
	fi, err := os.Stat(from)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return copyFile(from, to, fi.Mode().Perm(), fi.ModTime())
	}
	return filepath.WalkDir(from, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(from, p)
		dst := filepath.Join(to, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		info, err := os.Stat(p) // follows symlinks: copies content, not links
		if err != nil {
			return nil // a dangling link inside is skipped
		}
		if info.IsDir() {
			return copyTree(p, dst)
		}
		return copyFile(p, dst, info.Mode().Perm(), info.ModTime())
	})
}

func copyFile(from, to string, perm fs.FileMode, mtime time.Time) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(to, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chtimes(to, mtime, mtime)
}

func sameContent(a, b string) bool {
	fa, fb := fsx.Stat(a), fsx.Stat(b)
	if fa == nil || fb == nil || fa.Size() != fb.Size() {
		return false
	}
	ba, err1 := os.ReadFile(a)
	bb, err2 := os.ReadFile(b)
	return err1 == nil && err2 == nil && bytes.Equal(ba, bb)
}

// mergeDir moves src's contents into dst without overwriting anything, then
// removes src. Returns the names of kept duplicates.
func mergeDir(src, dst, label string) ([]string, error) {
	var kept []string
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return nil, err
	}
	for _, name := range readDir(src) {
		s, d := filepath.Join(src, name), filepath.Join(dst, name)
		sfi, dfi := fsx.Lstat(s), fsx.Lstat(d)
		switch {
		case sfi == nil:
		case dfi == nil:
			if err := move(s, d); err != nil {
				return kept, err
			}
		case sfi.IsDir() && dfi.IsDir():
			sub, err := mergeDir(s, d, label)
			kept = append(kept, sub...)
			if err != nil {
				return kept, err
			}
		case sfi.Mode().IsRegular() && dfi.Mode().IsRegular() && sameContent(s, d):
			os.Remove(s)
		default:
			alt := d + ".aims-" + label + "-" + stamp()
			if err := move(s, alt); err != nil {
				return kept, err
			}
			kept = append(kept, filepath.Base(alt))
		}
	}
	return kept, os.RemoveAll(src)
}

var sqliteSidecars = []string{"-wal", "-shm", "-journal"}

// Ensure links every shared entry of a profile to the hub and reports what it did.
func Ensure(o Options) []Report {
	var out []Report
	if o.Label == "" {
		o.Label = "profile"
	}
	if fsx.Abs(o.Hub) == fsx.Abs(o.Dir) {
		return nil
	}
	if err := os.MkdirAll(o.Dir, 0o700); err != nil {
		return []Report{{Name: o.Dir, Action: Conflict, Detail: err.Error()}}
	}
	if err := os.MkdirAll(o.Hub, 0o755); err != nil {
		return []Report{{Name: o.Hub, Action: Conflict, Detail: err.Error()}}
	}
	for _, name := range sharedNames(o) {
		r, err := ensureOne(o, name)
		if err != nil {
			r = Report{Name: name, Action: Conflict, Detail: err.Error()}
		}
		out = append(out, r)
	}
	return out
}

func ensureOne(o Options, name string) (Report, error) {
	target := filepath.Join(o.Hub, name)
	link := filepath.Join(o.Dir, name)
	lfi := fsx.Lstat(link)
	hfi := fsx.Stat(target)
	r := Report{Name: name}

	switch {
	case lfi != nil && lfi.Mode()&fs.ModeSymlink != 0, lfi != nil && isJunction(link):
		if pointsTo(link, target) || (fsx.Stat(link) == nil && hfi == nil) {
			r.Action = OK
			return r, nil
		}
		raw, _ := os.Readlink(link)
		r.Action, r.Detail = Conflict, "points to "+raw+", not the shared folder"
		return r, nil

	case lfi == nil:
		switch {
		case hfi != nil:
			r.Action = Linked
			return r, makeLink(target, link, hfi.IsDir())
		case slices.Contains(o.Layout.SharedDirs, name):
			if err := os.MkdirAll(target, 0o755); err != nil {
				return r, err
			}
			r.Action = Linked
			return r, makeLink(target, link, true)
		}
		r.Action, r.Detail = Skipped, "not created yet"
		return r, nil

	case hfi != nil && sameFile(lfi, hfi):
		// A Windows hard link standing in for a file symlink.
		r.Action = OK
		return r, nil

	case lfi.IsDir():
		if hfi == nil {
			if err := move(link, target); err != nil {
				return r, err
			}
			r.Action = Adopted
			return r, makeLink(target, link, true)
		}
		kept, err := mergeDir(link, target, o.Label)
		if err != nil {
			return r, err
		}
		r.Action = Merged
		if len(kept) > 0 {
			r.Detail = "kept both copies of " + strings.Join(kept, ", ")
		}
		return r, makeLink(target, link, true)

	case strings.HasSuffix(name, ".sqlite"):
		// Never move a database that may be open; only adopt a cleanly closed one.
		busy := slices.ContainsFunc(sqliteSidecars, func(s string) bool {
			fi := fsx.Stat(link + s)
			return fi != nil && fi.Size() > 0
		})
		if hfi == nil && (!busy || o.Fix) {
			if err := move(link, target); err != nil {
				return r, err
			}
			for _, s := range sqliteSidecars {
				if fsx.Lstat(link+s) != nil {
					if err := move(link+s, target+s); err != nil {
						return r, err
					}
				}
			}
			r.Action = Adopted
			return r, makeLink(target, link, false)
		}
		r.Action, r.Quiet = Conflict, hfi != nil
		if hfi != nil {
			r.Detail = "this profile and the shared folder each have their own database; its threads stay with this profile"
		} else {
			r.Detail = "the database is open; close the tool and run `aims doctor --fix`"
		}
		return r, nil
	}

	// A regular file where the link should be.
	switch {
	case hfi != nil && hfi.IsDir():
		r.Action, r.Detail = Conflict, "a file here, a folder in the shared folder"
		return r, nil
	case hfi == nil:
		if err := move(link, target); err != nil {
			return r, err
		}
		r.Action = Adopted
	case sameContent(link, target):
		if err := os.Remove(link); err != nil {
			return r, err
		}
		r.Action = Linked
	case strings.HasSuffix(name, ".jsonl"):
		// Append-only logs (prompt history): keep both.
		if err := appendFile(target, link); err != nil {
			return r, err
		}
		if err := os.Remove(link); err != nil {
			return r, err
		}
		r.Action = Merged
	default:
		// The newer copy wins; the other is kept next to it.
		profileNewer := lfi.ModTime().After(hfi.ModTime())
		tag := o.Label
		if profileNewer {
			tag = "previous"
		}
		backup := target + ".aims-" + tag + "-" + stamp()
		if profileNewer {
			if err := move(target, backup); err != nil {
				return r, err
			}
			if err := move(link, target); err != nil {
				return r, err
			}
		} else if err := move(link, backup); err != nil {
			return r, err
		}
		r.Action, r.Detail = Merged, "other version saved as "+filepath.Base(backup)
	}
	return r, makeLink(target, link, false)
}

func appendFile(dst, src string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(dst, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// Detach stops sharing the entries private selects: the link is replaced by a
// private copy, or by nothing for conversation data.
func Detach(hub, dir string, private func(string) bool, layout tool.Layout) []Report {
	var out []Report
	hubAbs := fsx.Abs(hub)
	hubReal, err := filepath.EvalSymlinks(hub)
	if err != nil {
		hubReal = hubAbs
	}
	for _, name := range readDir(dir) {
		if private == nil || !private(name) {
			continue
		}
		link := filepath.Join(dir, name)
		fi := fsx.Lstat(link)
		if fi == nil {
			continue
		}
		var raw string
		switch hfi := fsx.Lstat(filepath.Join(hub, name)); {
		case fi.Mode()&fs.ModeSymlink != 0 || isJunction(link):
			// aims links to <hub>/<name>, and that entry may itself be a symlink
			// (dotfiles), so compare the link's own target, not its resolved path.
			target, err := os.Readlink(link)
			if err != nil {
				continue
			}
			if !filepath.IsAbs(target) {
				target = filepath.Join(dir, target)
			}
			target = strings.TrimPrefix(target, `\\?\`)
			if parent := filepath.Dir(filepath.Clean(target)); parent != hubAbs && parent != hubReal {
				continue
			}
			raw = target
		case hfi != nil && sameFile(fi, hfi):
			// A hard link, which Windows gets for files without Developer Mode.
			raw = filepath.Join(hub, name)
		default:
			continue
		}
		if err := removeLink(link); err != nil {
			out = append(out, Report{Name: name, Action: Conflict, Detail: err.Error()})
			continue
		}
		detail := "now private and empty"
		if !layout.IsHistory(name) && fsx.Stat(raw) != nil {
			if err := copyTree(raw, link); err != nil {
				out = append(out, Report{Name: name, Action: Conflict, Detail: err.Error()})
				continue
			}
			detail = "now a private copy"
		}
		out = append(out, Report{Name: name, Action: Detached, Detail: detail})
	}
	return out
}

// RemoveLinks deletes the links in dir without following them into the hub.
func RemoveLinks(dir string) error {
	var errs []error
	for _, name := range readDir(dir) {
		p := filepath.Join(dir, name)
		fi := fsx.Lstat(p)
		if fi != nil && (fi.Mode()&fs.ModeSymlink != 0 || isJunction(p)) {
			if err := removeLink(p); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", name, err))
			}
		}
	}
	return errors.Join(errs...)
}
