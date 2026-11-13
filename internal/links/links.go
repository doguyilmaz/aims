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
	"sync"
	"time"

	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/proc"
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
	Unused   Action = "unused"   // the profile's own copy, which the tool no longer reads
	Removed  Action = "removed"  // an unused copy deleted and linked to the hub
	Archived Action = "archived" // unused database kept in a recovery folder
	Detached Action = "detached" // stopped sharing
	Conflict Action = "conflict" // needs a person
	Missing  Action = "missing"  // read-only check: link/target still needs creation
)

// Report describes one entry.
type Report struct {
	Name   string
	Action Action
	Detail string
	// Repairable marks check results handled by the conservative doctor fix.
	Repairable bool
	// Quiet conflicts are shown only by doctor and sync, not on every launch.
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
	// Fix archives verified-unused disposable databases. It never forces an
	// open database or one with recovery sidecars to move.
	Fix bool
	// StateShared: the tool reads the profile's databases from the hub.
	StateShared bool
	// MissingOnly prevents doctor from reconciling independent content.
	MissingOnly bool
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

var (
	busyMu sync.Mutex
	busyAt = map[string]time.Time{}
	// BusyRecheck is how long a lock folder found in use is not asked about
	// again by the same process: a session that moves syncs on every restart.
	BusyRecheck = 10 * time.Minute
)

// busyLocks is holding for a lock folder, remembered for BusyRecheck when
// it is in use.
func busyLocks(dir string) bool {
	busyMu.Lock()
	defer busyMu.Unlock()
	if at, ok := busyAt[dir]; ok && time.Since(at) < BusyRecheck {
		return true
	}
	if holding(dir) {
		busyAt[dir] = time.Now()
		return true
	}
	delete(busyAt, dir)
	return false
}

// holding reports whether a process may have a file in dir open: true too
// when the system cannot tell.
func holding(dir string) bool {
	held, known := proc.Holding(fsx.Real(dir))
	return held || !known
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

// move renames without replacing anything at to. Cross-filesystem moves need
// an explicit offline copy: copying and recursively deleting a live source
// cannot preserve writes made after the copy.
func move(from, to string) error {
	if fsx.Lstat(to) != nil {
		return fmt.Errorf("%s already exists", to)
	}
	err := renameNoReplace(from, to)
	if err == nil || !crossDevice(err) {
		return err
	}
	return fmt.Errorf("cross-filesystem move refused; close affected tools and copy %s to %s explicitly: %w", from, to, err)
}

// unique returns base, or base with a counter, naming nothing that exists yet.
func unique(base string) string {
	p := base
	for i := 2; fsx.Lstat(p) != nil; i++ {
		p = fmt.Sprintf("%s-%d", base, i)
	}
	return p
}

func copyTree(from, to string) error {
	return copyTreeAt(from, to, from, to)
}

// rebasedLink preserves external references and moves internal absolute
// references with the tree. Relative external links become absolute so that
// moving a skill/plugin does not silently change what it loads.
func rebasedLink(raw, source, sourceRoot, destRoot string) string {
	absolute := raw
	if !filepath.IsAbs(raw) {
		absolute = filepath.Join(filepath.Dir(source), raw)
	}
	if fsx.Within(absolute, sourceRoot) {
		if !filepath.IsAbs(raw) {
			return raw
		}
		rel, _ := filepath.Rel(sourceRoot, absolute)
		return filepath.Join(destRoot, rel)
	}
	return absolute
}

func copyTreeAt(from, to, sourceRoot, destRoot string) error {
	fi, err := os.Lstat(from)
	if err != nil {
		return err
	}
	if fi.Mode()&fs.ModeSymlink != 0 || isJunction(from) {
		target, err := os.Readlink(from)
		if err != nil {
			return err
		}
		info := fsx.Stat(from)
		return makeLink(rebasedLink(target, from, sourceRoot, destRoot), to, isJunction(from) || info != nil && info.IsDir())
	}
	if !fi.IsDir() {
		if !fi.Mode().IsRegular() {
			return fmt.Errorf("refusing to copy special file %s", from)
		}
		return copyFile(from, to, fi.Mode().Perm(), fi.ModTime())
	}
	if err := createCopyDirectory(to, fi.Mode().Perm()|0o700); err != nil {
		return err
	}
	entries, err := os.ReadDir(from)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := copyTreeAt(filepath.Join(from, e.Name()), filepath.Join(to, e.Name()), sourceRoot, destRoot); err != nil {
			return err
		}
	}
	return finishCopyPermissions(from, to, fi.Mode().Perm())
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
	out, err := fsx.CreatePrivateFile(to)
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
	if err := os.Chtimes(to, mtime, mtime); err != nil {
		return err
	}
	return finishCopyPermissions(from, to, perm)
}

func sameContent(a, b string) bool {
	fa, fb := fsx.Stat(a), fsx.Stat(b)
	if fa == nil || fb == nil || fa.Size() != fb.Size() {
		return false
	}
	if os.SameFile(fa, fb) {
		return true
	}
	left, err := os.Open(a)
	if err != nil {
		return false
	}
	defer left.Close()
	right, err := os.Open(b)
	if err != nil {
		return false
	}
	defer right.Close()
	// Repairs can encounter multi-GB rollouts. Compare in fixed-size chunks
	// rather than keeping two entire histories in memory.
	ba, bb := make([]byte, 32<<10), make([]byte, 32<<10)
	for {
		na, ea := io.ReadFull(left, ba)
		nb, eb := io.ReadFull(right, bb)
		if na != nb || !bytes.Equal(ba[:na], bb[:nb]) {
			return false
		}
		if ea == io.EOF || ea == io.ErrUnexpectedEOF {
			return ea == eb
		}
		if ea != nil || eb != nil {
			return false
		}
	}
}

// mergeDir moves src's contents into dst without overwriting anything, then
// removes src. Returns the names of kept duplicates.
func mergeDir(src, dst, label string) ([]string, error) {
	// Check the entire move before removing even a symlink. A later EXDEV
	// must not strand earlier links in an otherwise unmigrated profile.
	if err := preflightTreeMove(src, dst); err != nil {
		return nil, err
	}
	return mergeDirAt(src, dst, label, src, dst)
}

func mergeDirAt(src, dst, label, sourceRoot, destRoot string) (kept []string, err error) {
	if si, di := fsx.Stat(src), fsx.Stat(dst); si != nil && di != nil && os.SameFile(si, di) {
		return nil, fmt.Errorf("%s and %s are the same folder", src, dst) // removing src would empty dst
	}
	si, err := os.Stat(src)
	if err != nil {
		return nil, err
	}
	if fsx.Lstat(dst) == nil {
		restore, e := copyPermissionRestorer(src, dst, si.Mode().Perm())
		if e != nil {
			return nil, e
		}
		if err := createCopyDirectory(dst, si.Mode().Perm()|0o700); err != nil {
			return nil, err
		}
		// Restore read-only modes after population, including on failure.
		defer func() { err = errors.Join(err, restore()) }()
	}
	sourceMode := si.Mode() & (os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky)
	if sourceMode.Perm()&0o700 != 0o700 {
		if err := os.Chmod(src, sourceMode|0o700); err != nil {
			return nil, err
		}
		defer func() {
			if e := os.Chmod(src, sourceMode); !errors.Is(e, os.ErrNotExist) {
				err = errors.Join(err, e)
			}
		}()
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		name := entry.Name()
		s, d := filepath.Join(src, name), filepath.Join(dst, name)
		sfi, dfi := fsx.Lstat(s), fsx.Lstat(d)
		switch {
		case sfi == nil:
		case sfi.Mode()&fs.ModeSymlink != 0 || isJunction(s):
			raw, err := os.Readlink(s)
			if err != nil {
				return kept, err
			}
			rebased := rebasedLink(raw, s, sourceRoot, destRoot)
			resolved := rebased
			if !filepath.IsAbs(resolved) {
				resolved = filepath.Join(filepath.Dir(d), resolved)
			}
			if dfi != nil && (dfi.Mode()&fs.ModeSymlink != 0 || isJunction(d)) && pointsTo(d, resolved) {
				if err := os.Remove(s); err != nil {
					return kept, err
				}
				continue
			}
			if dfi != nil {
				d = unique(d + ".aims-" + label + "-" + stamp())
				kept = append(kept, filepath.Base(d))
			}
			info := fsx.Stat(s)
			if err := makeLink(rebased, d, isJunction(s) || info != nil && info.IsDir()); err != nil {
				return kept, err
			}
			if err := os.Remove(s); err != nil {
				return kept, err
			}
		case sfi.IsDir() && dfi == nil:
			sub, err := mergeDirAt(s, d, label, sourceRoot, destRoot)
			kept = append(kept, sub...)
			if err != nil {
				return kept, err
			}
		case dfi == nil:
			if err := move(s, d); err != nil {
				return kept, err
			}
		case sfi.IsDir() && dfi.IsDir() && !isJunction(d):
			sub, err := mergeDirAt(s, d, label, sourceRoot, destRoot)
			kept = append(kept, sub...)
			if err != nil {
				return kept, err
			}
		default:
			// Keep even equal files: a writer may have opened the source
			// after the idle check, so its inode must remain reachable.
			alt := unique(d + ".aims-" + label + "-" + stamp())
			if err := move(s, alt); err != nil {
				return kept, err
			}
			kept = append(kept, filepath.Base(alt))
		}
	}
	// Never recursively remove the source: a concurrent writer or a failed
	// directory read must leave its remaining entries available for recovery.
	return kept, os.Remove(src)
}

func preflightTreeMove(src, dst string) error {
	src, dst = fsx.Abs(src), fsx.Abs(dst)
	return filepath.WalkDir(src, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		for fsx.Lstat(target) == nil {
			parent := filepath.Dir(target)
			if parent == target {
				return fmt.Errorf("cannot determine destination filesystem")
			}
			target = parent
		}
		same, err := sameFilesystem(path, target)
		if err != nil {
			return err
		}
		if !same {
			return fmt.Errorf("cross-filesystem move refused before changing source %s", src)
		}
		if entry.IsDir() && !isJunction(path) && fsx.Lstat(filepath.Join(dst, rel)) == nil {
			if _, err := copyPermissionRestorer(path, filepath.Join(dst, rel), 0); err != nil {
				return err
			}
		}
		if entry.Type()&fs.ModeSymlink != 0 || isJunction(path) {
			raw, err := os.Readlink(path)
			if err != nil {
				return err
			}
			referent := raw
			if !filepath.IsAbs(referent) {
				referent = filepath.Join(filepath.Dir(path), referent)
			}
			referent = filepath.Clean(referent)
			if fsx.Within(referent, src) {
				// A conflicting file will receive a backup suffix. Rebasing its
				// incoming link to the unsuffixed hub path would select another
				// file (or turn a dangling link into an unrelated live reference).
				// Check ancestors too: a directory/file collision renames the
				// whole source subtree rather than only its referenced leaf.
				for originalPath := referent; ; originalPath = filepath.Dir(originalPath) {
					rel, err := filepath.Rel(src, originalPath)
					if err != nil {
						return err
					}
					if rel == "." {
						break
					}
					if filepath.Dir(originalPath) == originalPath {
						return fmt.Errorf("internal link %s did not reach its source root; kept the source tree", path)
					}
					destPath := filepath.Join(dst, rel)
					original, destination := fsx.Lstat(originalPath), fsx.Lstat(destPath)
					if destination == nil {
						continue
					}
					same := original != nil && os.SameFile(original, destination)
					mergedDirs := original != nil && original.IsDir() && destination.IsDir() && !isJunction(originalPath) && !isJunction(destPath)
					if !same && !mergedDirs {
						return fmt.Errorf("internal link %s would select a conflicting destination; kept the source tree for explicit reconciliation", path)
					}
				}
			}
		}
		if entry.IsDir() && isJunction(path) {
			return filepath.SkipDir
		}
		return nil
	})
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
	// A profile folder that is the hub under another name (a symlink, another
	// letter case) or overlaps it would have its "shared" entries merged into
	// themselves.
	if fsx.Inside(o.Dir, o.Hub) || fsx.Inside(o.Hub, o.Dir) {
		return []Report{{Name: o.Dir, Action: Conflict, Detail: "the profile folder overlaps the shared folder " + o.Hub}}
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
	// Disposable databases that are not shared (Codex logs) get no link: with
	// shared state the tool writes them to the hub, so the profile copy is dead.
	if o.StateShared {
		entries, _ := os.ReadDir(o.Dir)
		for _, e := range entries {
			name := e.Name()
			if !e.Type().IsRegular() || !o.Layout.IsDisposable(name) || o.Layout.IsShared(name) || o.Private != nil && o.Private(name) || fsx.Stat(filepath.Join(o.Hub, name)) == nil {
				continue
			}
			r, err := dropUnused(o, name)
			if err != nil {
				r = Report{Name: name, Action: Conflict, Detail: err.Error()}
			}
			out = append(out, r)
		}
	}
	return out
}

// dropUnused archives a profile's unused database with all its sidecars.
// Recovery data is never deleted by a doctor repair.
func dropUnused(o Options, name string) (Report, error) {
	link := filepath.Join(o.Dir, name)
	r := Report{Name: name, Action: Unused, Detail: "the tool uses the shared copy; `aims doctor --fix` archives this one for recovery"}
	if !o.Fix {
		return r, nil
	}
	held, known := proc.Holding(fsx.Real(o.Dir))
	switch {
	case held:
		r.Detail = "a running session has this profile's folder open; close it and run `aims doctor --fix` again"
		return r, nil
	case !known:
		r.Detail = "aims cannot tell whether a session has this folder open, so it keeps the copy"
		return r, nil
	}
	archive, err := os.MkdirTemp(o.Dir, ".aims-recovery-")
	if err != nil {
		return r, err
	}
	var moved []string
	for _, s := range append([]string{""}, sqliteSidecars...) {
		if fsx.Lstat(link+s) == nil {
			continue
		}
		if err := move(link+s, filepath.Join(archive, name+s)); err != nil {
			for _, old := range moved {
				err = errors.Join(err, move(filepath.Join(archive, name+old), link+old))
			}
			return r, fmt.Errorf("database recovery folder %s: %w", archive, err)
		}
		moved = append(moved, s)
	}
	r.Action, r.Detail = Archived, "unused database and sidecars kept in "+archive
	return r, nil
}

func ensureOne(o Options, name string) (Report, error) {
	target := filepath.Join(o.Hub, name)
	link := filepath.Join(o.Dir, name)
	lfi := fsx.Lstat(link)
	hfi := fsx.Stat(target)
	r := Report{Name: name}
	if o.MissingOnly && lfi != nil && !pointsTo(link, target) && !(hfi != nil && sameFile(lfi, hfi)) && !slices.Contains(o.Layout.LockDirs, name) && !(o.StateShared && o.Layout.IsDisposable(name)) {
		return Report{Name: name, Action: Conflict, Detail: "independent entry kept; reconcile explicitly with aims sync after closing affected tools"}, nil
	}
	if lfi != nil && !lfi.IsDir() && !lfi.Mode().IsRegular() && lfi.Mode()&fs.ModeSymlink == 0 && !isJunction(link) {
		return Report{Name: name, Action: Conflict, Detail: "special file; left untouched"}, nil
	}
	if hfi != nil && !hfi.IsDir() && !hfi.Mode().IsRegular() {
		return Report{Name: name, Action: Conflict, Detail: "shared target is a special file; left untouched"}, nil
	}
	if hfi != nil && (!hfi.IsDir() && slices.Contains(o.Layout.SharedDirs, name) || hfi.IsDir() && name == o.Layout.SettingsFile) {
		return Report{Name: name, Action: Conflict, Detail: "shared target has the wrong file type; left untouched"}, nil
	}
	if tfi := fsx.Lstat(target); lfi != nil && tfi != nil && (tfi.Mode()&fs.ModeSymlink != 0 || isJunction(target)) && !pointsTo(link, target) && !(hfi != nil && sameFile(lfi, hfi)) {
		return Report{Name: name, Action: Conflict, Detail: "shared entry is externally managed; independent local copy kept"}, nil
	}
	// Correct links require no reconciliation. Real directory trees can have
	// active nested writers; merging/deduplicating them must wait until idle.
	if lfi != nil && lfi.IsDir() && !isJunction(link) && !slices.Contains(o.Layout.LockDirs, name) {
		if held, known := proc.HoldingTree(fsx.Real(link)); held || !known {
			return Report{Name: name, Action: Conflict, Quiet: true, Detail: "folder may be in use; close affected tools before syncing; contents kept"}, nil
		}
		if hfi != nil && hfi.IsDir() {
			if held, known := proc.HoldingTree(fsx.Real(target)); held || !known {
				return Report{Name: name, Action: Conflict, Quiet: true, Detail: "shared folder may be in use; close affected tools before syncing; contents kept"}, nil
			}
		}
	}
	if lfi != nil && lfi.Mode().IsRegular() && !strings.HasSuffix(name, ".sqlite") && !(hfi != nil && sameFile(lfi, hfi)) && (holding(o.Dir) || holding(o.Hub)) {
		return Report{Name: name, Action: Conflict, Quiet: true, Detail: "folder may be in use; close affected tools before reconciling files"}, nil
	}

	switch {
	case lfi != nil && lfi.Mode()&fs.ModeSymlink != 0, lfi != nil && isJunction(link):
		if pointsTo(link, target) {
			if hfi == nil && slices.Contains(o.Layout.SharedDirs, name) {
				if err := os.MkdirAll(target, 0o755); err != nil {
					return r, err
				}
				r.Action = Linked
				return r, nil
			}
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

	case lfi.IsDir() && hfi != nil && os.SameFile(fsx.Stat(link), hfi):
		r.Action, r.Detail = Conflict, "this is the shared folder itself, under another name"
		return r, nil

	case lfi.IsDir():
		if hfi != nil && !hfi.IsDir() {
			return Report{Name: name, Action: Conflict, Detail: "a folder here, a file in the shared folder"}, nil
		}
		if slices.Contains(o.Layout.LockDirs, name) {
			// Moving or deduplicating even an empty lock file changes its
			// inode and can let two processes own the same conversation:
			// only a folder no process has a file of open moves aside.
			if !busyLocks(link) {
				aside := unique(link + ".aims-" + o.Label + "-" + stamp())
				if err := move(link, aside); err != nil {
					return r, err
				}
				// A process that opened a lock between the look and the move.
				if holding(aside) {
					if err := move(aside, link); err != nil {
						return r, err
					}
				} else {
					if err := os.MkdirAll(target, 0o755); err != nil {
						return r, err
					}
					r.Action, r.Detail = Adopted, "the old one is kept as "+filepath.Base(aside)
					return r, makeLink(target, link, true)
				}
			}
			r.Action, r.Quiet, r.Detail = Conflict, true, "private writer locks still in use; aims shares them once no instance of the tool uses this account"
			return r, nil
		}
		if hfi == nil {
			// Walk the tree even for first adoption: a rename alone leaves
			// internal absolute links pointing into the old profile directory.
			if _, err := mergeDir(link, target, o.Label); err != nil {
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
		if hfi == nil && !busy && !holding(o.Dir) {
			// Recovery files in either home belong to that database. Never
			// overwrite or separate them, even when --fix was requested.
			for _, s := range sqliteSidecars {
				if fsx.Lstat(target+s) != nil || fsx.Lstat(link+s) != nil {
					return Report{Name: name, Action: Conflict, Detail: "database sidecars are present; close the tool and checkpoint or recover SQLite before syncing"}, nil
				}
			}
			if err := move(link, target); err != nil {
				return r, err
			}
			r.Action = Adopted
			return r, makeLink(target, link, false)
		}
		if hfi != nil && o.StateShared && o.Layout.IsDisposable(name) {
			r, err := dropUnused(o, name)
			if err != nil || r.Action != Archived {
				return r, err
			}
			return r, makeLink(target, link, false)
		}
		r.Action, r.Quiet = Conflict, hfi != nil
		if hfi != nil {
			r.Detail = "this profile and the shared folder each have their own database; history sharing is incomplete; both copies are kept, including with --fix"
		} else {
			r.Detail = "the database may be open or need recovery; close the tool, checkpoint SQLite, then run `aims doctor --fix`; --fix never forces it"
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
		backup := unique(link + ".aims-" + o.Label + "-" + stamp())
		if err := move(link, backup); err != nil {
			return r, err
		}
		r.Action, r.Detail = Linked, "source kept as "+filepath.Base(backup)
	case strings.HasSuffix(name, ".jsonl"):
		// Append-only logs (prompt history): keep both.
		if err := appendFile(target, link); err != nil {
			return r, err
		}
		backup := unique(link + ".aims-" + o.Label + "-" + stamp())
		if err := move(link, backup); err != nil {
			return r, err
		}
		r.Action, r.Detail = Merged, "source kept as "+filepath.Base(backup)
	default:
		// The newer copy wins; the other is kept next to it.
		profileNewer := lfi.ModTime().After(hfi.ModTime())
		tag := o.Label
		if profileNewer {
			tag = "previous"
		}
		backup := unique(target + ".aims-" + tag + "-" + stamp())
		if profileNewer {
			// Reject an impossible replacement before taking the settings used
			// by every sharing profile out of service.
			if same, err := sameFilesystem(link, filepath.Dir(target)); err != nil {
				return r, fmt.Errorf("cannot preflight shared settings replacement: %w", err)
			} else if !same {
				return r, fmt.Errorf("cross-filesystem settings replacement refused; both files kept; close affected tools and reconcile %s and %s explicitly", link, target)
			}
			if err := move(target, backup); err != nil {
				return r, err
			}
			if err := move(link, target); err != nil {
				// Permissions or a concurrent edit can still prevent the second
				// move. Restore the old hub without replacing any new occupant.
				return r, errors.Join(err, move(backup, target))
			}
		} else if err := move(link, backup); err != nil {
			return r, err
		}
		r.Action, r.Detail = Merged, "other version saved as "+filepath.Base(backup)
	}
	return r, makeLink(target, link, false)
}

// HistoryShared checks existing history entries without reading their contents
// or walking history directories. Missing entries can be linked by the next
// Sync; existing private copies must be reconciled before a conversation moves.
func HistoryShared(hub, dir string, layout tool.Layout, stateShared bool) bool {
	if fsx.Real(hub) == fsx.Real(dir) {
		return true
	}
	if fsx.Inside(dir, hub) || fsx.Inside(hub, dir) {
		return false
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return errors.Is(err, fs.ErrNotExist)
	}
	for _, entry := range entries {
		name := entry.Name()
		if !layout.IsShared(name) || !layout.IsHistory(name) || (stateShared && layout.IsDisposable(name)) {
			continue
		}
		path, target := filepath.Join(dir, name), filepath.Join(hub, name)
		if entry.Type()&fs.ModeSymlink != 0 || isJunction(path) {
			if !pointsTo(path, target) {
				return false
			}
			continue
		}
		local, err := os.Stat(path)
		if err != nil {
			return false
		}
		shared, err := os.Stat(target)
		if err != nil || !os.SameFile(local, shared) {
			return false
		}
	}
	return true
}

func appendFile(dst, src string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	f, err := os.OpenFile(dst, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, in); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// Detach stops sharing the entries private selects: the link is replaced by a
// private copy, or by nothing for conversation data.
func Detach(hub, dir string, private func(string) bool, layout tool.Layout) []Report {
	if fsx.Inside(dir, hub) || fsx.Inside(hub, dir) {
		return []Report{{Name: dir, Action: Conflict, Detail: "the profile folder overlaps the shared folder; nothing was detached"}}
	}
	var out []Report
	hubAbs := fsx.Abs(hub)
	hubReal, err := filepath.EvalSymlinks(hub)
	if err != nil {
		hubReal = hubAbs
	}
	for _, name := range readDir(dir) {
		if !layout.IsShared(name) || private == nil || !private(name) {
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
		detail := "now private and empty"
		if !layout.IsHistory(name) && fsx.Stat(raw) != nil {
			// Stage the complete private copy before removing the shared link.
			// A failed copy must leave access to the user's settings intact.
			stage, err := os.MkdirTemp(dir, ".aims-detach-")
			if err != nil {
				out = append(out, Report{Name: name, Action: Conflict, Detail: err.Error()})
				continue
			}
			copy := filepath.Join(stage, name)
			if err := copyTreeAt(fsx.Real(raw), copy, fsx.Real(raw), link); err != nil {
				os.RemoveAll(stage)
				out = append(out, Report{Name: name, Action: Conflict, Detail: err.Error()})
				continue
			}
			if err := removeLink(link); err != nil {
				os.RemoveAll(stage)
				out = append(out, Report{Name: name, Action: Conflict, Detail: err.Error()})
				continue
			}
			if err := os.Rename(copy, link); err != nil {
				// Keep the staged copy if even restoring the link fails.
				info := fsx.Stat(raw)
				restore := makeLink(raw, link, info != nil && info.IsDir())
				out = append(out, Report{Name: name, Action: Conflict, Detail: fmt.Sprintf("private copy kept at %s: %v", copy, errors.Join(err, restore))})
				continue
			}
			os.Remove(stage)
			detail = "now a private copy"
		} else if err := removeLink(link); err != nil {
			out = append(out, Report{Name: name, Action: Conflict, Detail: err.Error()})
			continue
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
