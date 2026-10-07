// Package fsx holds the small file helpers every other package needs:
// home directory lookup, atomic writes, JSON reading, path containment and a
// cross-process lock.
package fsx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Home returns the user's home directory. HOME (USERPROFILE on Windows) is
// read on every call so tests can point it at a sandbox.
func Home() string {
	key := "HOME"
	if runtime.GOOS == "windows" {
		key = "USERPROFILE"
	}
	if h := os.Getenv(key); h != "" {
		return h
	}
	h, _ := os.UserHomeDir()
	return h
}

// ExpandHome turns a leading "~" into the home directory.
func ExpandHome(p string) string {
	if p == "~" {
		return Home()
	}
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		return filepath.Join(Home(), p[2:])
	}
	return p
}

// Tildify shortens a path under the home directory for display.
func Tildify(p string) string {
	h := Home()
	if h != "" && (p == h || strings.HasPrefix(p, h+string(filepath.Separator))) {
		return "~" + p[len(h):]
	}
	return p
}

// Abs resolves p to an absolute, cleaned path after expanding "~".
func Abs(p string) string {
	a, err := filepath.Abs(ExpandHome(p))
	if err != nil {
		return filepath.Clean(ExpandHome(p))
	}
	return a
}

// Within reports whether child is parent or below it.
func Within(child, parent string) bool {
	rel, err := filepath.Rel(Abs(parent), Abs(child))
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel))
}

// Inside reports whether child is parent or below it on disk: unlike Within
// it sees through symlinks and case-insensitive file systems, so ~/link-to-claude
// and ~/.Claude both count as inside ~/.claude. child need not exist yet.
func Inside(child, parent string) bool {
	if Within(Real(child), Real(parent)) {
		return true
	}
	pi := Stat(parent)
	if pi == nil {
		return false
	}
	for c := Abs(child); ; c = filepath.Dir(c) {
		if ci := Stat(c); ci != nil && os.SameFile(ci, pi) {
			return true
		}
		if filepath.Dir(c) == c {
			return false
		}
	}
}

// Real resolves symlinks in p, or in its longest existing prefix when p does
// not exist yet.
func Real(p string) string {
	p = Abs(p)
	rest := ""
	for {
		if r, err := filepath.EvalSymlinks(p); err == nil {
			return filepath.Join(r, rest)
		}
		parent := filepath.Dir(p)
		if parent == p {
			return filepath.Join(p, rest)
		}
		rest = filepath.Join(filepath.Base(p), rest)
		p = parent
	}
}

// Lstat returns nil when the path does not exist.
func Lstat(p string) fs.FileInfo {
	fi, err := os.Lstat(p)
	if err != nil {
		return nil
	}
	return fi
}

// Stat follows symlinks and returns nil when the target does not exist.
func Stat(p string) fs.FileInfo {
	fi, err := os.Stat(p)
	if err != nil {
		return nil
	}
	return fi
}

// WriteFile replaces a file atomically (temp file + rename). A symlinked
// target, such as a settings file managed by a dotfiles tool, is written
// through instead of being replaced by a regular file.
//
// An existing file keeps its permissions; perm applies to a new file.
func WriteFile(path string, data []byte, perm fs.FileMode) error {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	if fi := Stat(path); fi != nil && fi.Mode().IsRegular() {
		perm = fi.Mode().Perm()
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Chmod(name, perm); err != nil {
		os.Remove(name)
		return err
	}
	if err := rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

// WriteJSON writes v as indented JSON with a trailing newline.
func WriteJSON(path string, v any, perm fs.FileMode) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return WriteFile(path, append(b, '\n'), perm)
}

// ReadJSON decodes path into v. A missing file leaves v untouched and returns
// false. A file that exists but does not parse is an error: callers that write
// the file back must never turn a typo into an empty config.
func ReadJSON(path string, v any) (bool, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if len(strings.TrimSpace(string(b))) == 0 {
		return false, nil
	}
	if err := json.Unmarshal(b, v); err != nil {
		return false, fmt.Errorf("%s is not valid JSON (%v); fix it or move it away", Tildify(path), err)
	}
	return true, nil
}

// ReadJSONLenient decodes path into v and ignores every error. For caches and
// for files owned by other tools, where a bad read only costs information.
func ReadJSONLenient(path string, v any) bool {
	ok, err := ReadJSON(path, v)
	return ok && err == nil
}

// lockStale is how old a lock file may get before it is assumed to belong to a
// process that died while holding it.
const lockStale = 10 * time.Second

// Lock takes an exclusive lock file next to path, waiting up to wait. It
// returns the unlock function, or ok=false when another aims process held the
// lock the whole time.
func Lock(path string, wait time.Duration) (unlock func(), ok bool, err error) {
	lock := path + ".lock"
	if err := os.MkdirAll(filepath.Dir(lock), 0o700); err != nil {
		return nil, false, err
	}
	deadline := time.Now().Add(wait)
	for {
		f, err := os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			fmt.Fprint(f, strconv.Itoa(os.Getpid()))
			f.Close()
			return func() { os.Remove(lock) }, true, nil
		}
		// On Windows a lock file another process is deleting reads as
		// "access denied" for a moment, not "exists": wait for it too.
		if !errors.Is(err, fs.ErrExist) && !(busy(err) && time.Now().Before(deadline)) {
			return nil, false, err
		}
		if fi := Stat(lock); fi != nil && time.Since(fi.ModTime()) > lockStale {
			os.Remove(lock)
			continue
		}
		if time.Now().After(deadline) {
			return nil, false, nil
		}
		time.Sleep(15 * time.Millisecond)
	}
}

// WithLock runs fn while holding an exclusive lock file next to path, so two
// aims processes (a status line refresh and a launch, say) cannot interleave a
// read-modify-write of the same file.
func WithLock(path string, fn func() error) error {
	unlock, ok, err := Lock(path, 3*time.Second)
	if err != nil {
		return err
	}
	if ok {
		defer unlock()
	}
	// Without the lock after 3s: better to risk a lost update than to hang a
	// command forever.
	return fn()
}
