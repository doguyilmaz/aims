package bridge

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/tool"
	"github.com/doguyilmaz/aims/internal/tools"
)

const manifestName = "manifest.json"

// Launcher contains executable routing only, never account or auth contents.
type Launcher struct {
	Name   string `json:"name"`
	Native string `json:"nativeExecutable"`
	Kind   string `json:"linkKind"`
	Digest string `json:"sha256,omitempty"`
}

// Launchers is a machine-local installation, not a portable credential store.
type Launchers struct {
	Owner      string               `json:"owner"`
	Version    string               `json:"schemaVersion"`
	Aims       string               `json:"aimsExecutable"`
	ConfigHome string               `json:"aimsHome"`
	Tools      map[tool.ID]Launcher `json:"tools"`
	Adapters   map[tool.ID]string   `json:"acpExecutables,omitempty"`
}

func LauncherRoot() string { return filepath.Join(config.Home(), "bridge") }
func LauncherBin() string  { return filepath.Join(LauncherRoot(), "bin") }

func TemporaryExecutable(path string) bool {
	p := filepath.ToSlash(path)
	return strings.Contains(p, "/go-build") && strings.Contains(p, "/exe/")
}

func executableName(a tool.Adapter) string {
	n := a.Layout().Bin
	if runtime.GOOS == "windows" {
		n += ".exe"
	}
	return n
}

// ReadLaunchers rejects links and malformed ownership metadata. It never writes.
func ReadLaunchers(root string) (*Launchers, error) {
	fi, err := os.Lstat(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !fi.IsDir() {
		return nil, fmt.Errorf("%s is not a real launcher directory", root)
	}
	p := filepath.Join(root, manifestName)
	fi, err = os.Lstat(p)
	if err != nil || !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%s has no regular aims launcher manifest; nothing was changed", root)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var m Launchers
	if json.Unmarshal(b, &m) != nil || m.Owner != "aims.bridge" || m.Version != "1.0.0" || !filepath.IsAbs(m.Aims) || !filepath.IsAbs(m.ConfigHome) || len(m.Tools) == 0 {
		return nil, fmt.Errorf("%s has an invalid or unsupported aims launcher manifest", root)
	}
	for id, l := range m.Tools {
		a, ok := tools.Parse(string(id))
		if !ok || l.Name != executableName(a) || !filepath.IsAbs(l.Native) || (l.Kind != "symlink" && l.Kind != "hardlink") || (l.Kind == "hardlink" && len(l.Digest) != 64) {
			return nil, fmt.Errorf("invalid launcher metadata for %q", id)
		}
	}
	for id, path := range m.Adapters {
		if _, ok := tools.Parse(string(id)); !ok || !filepath.IsAbs(path) {
			return nil, fmt.Errorf("invalid ACP executable metadata for %q", id)
		}
	}
	return &m, nil
}

// CheckLaunchers diagnoses an existing installation without running a binary
// or installing optional integrations. Ownership failures are never repaired.
func CheckLaunchers(root string) []string {
	m, err := ReadLaunchers(root)
	if err != nil {
		return []string{err.Error()}
	}
	if m == nil {
		return nil
	}
	if err := owned(root, m); err != nil {
		return []string{err.Error()}
	}
	var out []string
	if fsx.Real(m.ConfigHome) != fsx.Real(config.Home()) {
		out = append(out, "bridge launchers belong to another aims configuration home; review before reinstalling")
	}
	if _, err := exec.LookPath(m.Aims); err != nil {
		out = append(out, "bridge aims executable is missing; run aims bridge install from a persistent installation")
	}
	for _, a := range tools.All() {
		l, ok := m.Tools[a.ID()]
		if !ok {
			continue
		}
		if _, err := exec.LookPath(filepath.Join(root, "bin", l.Name)); err != nil {
			out = append(out, string(a.ID())+" bridge launcher is missing or not executable")
		}
		if _, err := exec.LookPath(l.Native); err != nil {
			out = append(out, string(a.ID())+" recorded native executable is missing; reinstall the launcher after upgrading the CLI")
		}
		if p := m.Adapters[a.ID()]; p != "" {
			if _, err := exec.LookPath(p); err != nil {
				out = append(out, string(a.ID())+" recorded ACP adapter is missing; reinstall the launcher after upgrading the adapter")
			}
		}
	}
	return out
}

// ACPExecutable preserves the installed adapter's choice of bundled runtime.
// A recorded absolute path also works when a desktop client's PATH is minimal.
func ACPExecutable(a tool.Adapter, m *Launchers) (string, error) {
	name := a.Layout().ACPBin
	if name == "" {
		return "", fmt.Errorf("%s has no configured upstream ACP adapter", a.ID())
	}
	if p, err := exec.LookPath(name); err == nil {
		return fsx.Abs(p), nil
	}
	if m != nil && m.Adapters[a.ID()] != "" {
		if p, err := exec.LookPath(m.Adapters[a.ID()]); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("%s is not installed; install its upstream ACP adapter first (see aims bridge commands)", name)
}

func digest(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// owned checks only files this installation created. Unknown or edited files
// are never overwritten or deleted, including during an upgrade/uninstall.
func owned(root string, m *Launchers) error {
	bin := filepath.Join(root, "bin")
	fi, err := os.Lstat(bin)
	if err != nil || !fi.IsDir() {
		return fmt.Errorf("%s is not a real launcher bin directory", bin)
	}
	for _, l := range m.Tools {
		p := filepath.Join(bin, l.Name)
		fi, err := os.Lstat(p)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if l.Kind == "symlink" && fi.Mode()&os.ModeSymlink != 0 {
			if target, err := os.Readlink(p); err == nil && target == m.Aims {
				continue
			}
		} else if l.Kind == "hardlink" && fi.Mode().IsRegular() {
			if samePath(p, m.Aims) {
				continue // An in-place executable upgrade updates both hard links.
			}
			if d, err := digest(p); err == nil && d == l.Digest {
				continue
			}
		}
		return fmt.Errorf("%s was changed outside aims; refusing to replace or remove it", p)
	}
	return nil
}

// NativeBinary bypasses aims' own launchers, avoiding PATH recursion. An
// explicit AIMS_*_BIN wins; recorded native paths are a fallback for GUI apps
// whose PATH lacks the original installation. No binary is executed here.
func NativeBinary(a tool.Adapter, root, self string, m *Launchers) (string, error) {
	valid := func(p string) string {
		p, err := exec.LookPath(p)
		if err != nil || fsx.Within(p, root) || fsx.Inside(p, root) || samePath(p, self) {
			return ""
		}
		if id, _, _ := LauncherInvocation(p); id != "" {
			return "" // Also skip launchers from another aims installation.
		}
		return fsx.Abs(p)
	}
	if v := os.Getenv(a.Layout().BinEnv); v != "" {
		if p := valid(v); p != "" {
			return p, nil
		}
		return "", fmt.Errorf("%s must name a real %s executable outside aims' launcher directory", a.Layout().BinEnv, a.ID())
	}
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		// Match os/exec's protection against implicit executables in cwd.
		if !filepath.IsAbs(dir) {
			continue
		}
		if p := valid(filepath.Join(dir, a.Layout().Bin)); p != "" {
			return p, nil
		}
	}
	if m != nil {
		if p := valid(m.Tools[a.ID()].Native); p != "" {
			return p, nil
		}
	}
	return "", fmt.Errorf("%s is not installed (or set %s)", a.ID(), a.Layout().BinEnv)
}

// InstallLaunchers creates links to one aims executable, never copies of
// credentials, sessions, or the executable. It does not modify PATH or editors.
// A staged installation is published only after every launcher is ready.
func InstallLaunchers(self string) (*Launchers, error) {
	self = fsx.Real(self)
	if _, err := exec.LookPath(self); err != nil {
		return nil, err
	}
	if TemporaryExecutable(self) {
		return nil, fmt.Errorf("build or install aims as a persistent executable first; go run's temporary executable cannot host launchers")
	}
	// Homebrew and version managers replace a versioned target on upgrade.
	// Keep the stable PATH entry when it resolves to this same executable.
	if onPath, err := exec.LookPath("aims"); err == nil && samePath(onPath, self) {
		self = fsx.Abs(onPath)
	}
	root := LauncherRoot()
	if fsx.Inside(self, root) {
		return nil, fmt.Errorf("aims itself must live outside %s", root)
	}
	var out *Launchers
	err := fsx.WithLock(root, func() error {
		old, err := ReadLaunchers(root)
		if err != nil {
			return err
		}
		if old != nil {
			if err := owned(root, old); err != nil {
				return err
			}
		}
		m := &Launchers{Owner: "aims.bridge", Version: "1.0.0", Aims: self, ConfigHome: config.Home(), Tools: map[tool.ID]Launcher{}, Adapters: map[tool.ID]string{}}
		for _, a := range tools.All() {
			bin, err := NativeBinary(a, root, self, old)
			if err != nil {
				if os.Getenv(a.Layout().BinEnv) != "" {
					return err
				}
				if old != nil && old.Tools[a.ID()].Name != "" {
					return err
				}
				continue
			}
			m.Tools[a.ID()] = Launcher{Name: executableName(a), Native: bin}
			if p, err := ACPExecutable(a, old); err == nil && !samePath(p, self) {
				m.Adapters[a.ID()] = p
			}
		}
		if len(m.Tools) == 0 {
			return fmt.Errorf("no native Claude Code or Codex executable found")
		}
		stage, err := os.MkdirTemp(config.Home(), ".bridge-install-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(stage)
		if err := os.Mkdir(filepath.Join(stage, "bin"), 0o700); err != nil {
			return err
		}
		for id, l := range m.Tools {
			p := filepath.Join(stage, "bin", l.Name)
			l.Kind = "symlink"
			if err := os.Symlink(self, p); err != nil {
				if err := os.Link(self, p); err != nil {
					return fmt.Errorf("cannot link %s: %w (on Windows keep aims and AIMS_HOME on the same volume)", l.Name, err)
				}
				l.Kind, l.Digest = "hardlink", ""
				l.Digest, err = digest(self)
				if err != nil {
					return err
				}
			}
			m.Tools[id] = l
		}
		if err := fsx.WriteJSON(filepath.Join(stage, manifestName), m, 0o600); err != nil {
			return err
		}
		if old == nil {
			if err := os.Rename(stage, root); err != nil {
				return err
			}
		} else {
			// Preflight every destination, including tools newly added to an
			// older installation; no user file can be replaced in a partial update.
			for id, l := range m.Tools {
				if old.Tools[id].Name == "" && fsx.Lstat(filepath.Join(root, "bin", l.Name)) != nil {
					return fmt.Errorf("%s already exists outside this installation", l.Name)
				}
			}
			// Keep old links, not executable copies, until the new manifest
			// commits. A failed update can restore the previous installation.
			backups := map[string]string{}
			for _, l := range m.Tools {
				dest := filepath.Join(root, "bin", l.Name)
				if fi := fsx.Lstat(dest); fi != nil {
					backup := filepath.Join(stage, "previous-"+l.Name)
					if fi.Mode()&os.ModeSymlink != 0 {
						target, err := os.Readlink(dest)
						if err != nil {
							return err
						}
						if err := os.Symlink(target, backup); err != nil {
							return err
						}
					} else if err := os.Link(dest, backup); err != nil {
						return err
					}
					backups[l.Name] = backup
				}
			}
			var applied []string
			rollback := func(cause error) error {
				for _, name := range applied {
					dest := filepath.Join(root, "bin", name)
					var err error
					if backup := backups[name]; backup != "" {
						err = os.Rename(backup, dest)
					} else {
						err = os.Remove(dest)
					}
					cause = errors.Join(cause, err)
				}
				return cause
			}
			for _, l := range m.Tools {
				if err := os.Rename(filepath.Join(stage, "bin", l.Name), filepath.Join(root, "bin", l.Name)); err != nil {
					return rollback(err)
				}
				applied = append(applied, l.Name)
			}
			if err := fsx.WriteJSON(filepath.Join(root, manifestName), m, 0o600); err != nil {
				return rollback(err)
			}
		}
		out = m
		return nil
	})
	return out, err
}

func UninstallLaunchers() error {
	root := LauncherRoot()
	if _, err := os.Lstat(root); os.IsNotExist(err) {
		return nil
	}
	return fsx.WithLock(root, func() error {
		m, err := ReadLaunchers(root)
		if err != nil || m == nil {
			return err
		}
		if err := owned(root, m); err != nil {
			return err
		}
		for _, l := range m.Tools {
			if err := os.Remove(filepath.Join(root, "bin", l.Name)); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		if err := os.Remove(filepath.Join(root, manifestName)); err != nil {
			return err
		}
		// Remove only empty directories; unrelated files are retained.
		_ = os.Remove(filepath.Join(root, "bin"))
		_ = os.Remove(root)
		return nil
	})
}

// LauncherInvocation recognizes only a CLI basename beside aims' own manifest.
// It works for symlinks and Windows .exe hardlinks; there is no shell quoting.
func LauncherInvocation(argv0 string) (tool.ID, *Launchers, error) {
	name := strings.TrimSuffix(filepath.Base(argv0), ".exe")
	a, ok := tools.Parse(name)
	if !ok || name != a.Layout().Bin {
		return "", nil, nil
	}
	p, err := exec.LookPath(argv0)
	if err != nil {
		return "", nil, err
	}
	root := filepath.Dir(filepath.Dir(fsx.Abs(p)))
	m, err := ReadLaunchers(root)
	if err != nil || m == nil {
		return "", nil, err
	}
	if m.Tools[a.ID()].Name != filepath.Base(p) || filepath.Base(filepath.Dir(p)) != "bin" {
		return "", nil, errors.New("this executable is not a registered aims launcher")
	}
	return a.ID(), m, nil
}
