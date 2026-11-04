package codex

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/proc"
	"github.com/doguyilmaz/aims/internal/tool"
)

var _ tool.FolderTruster = (*Adapter)(nil)

// folderSettings are what Codex can load from a folder once it is trusted.
var folderSettings = []string{".codex", ".agents", ".mcp.json"}

// TrustArgs trusts the session's folder for this start only, when Codex would
// otherwise ask. It asks at start in a git repository with no entry of its
// own, so a session that began before `git init` reaches the question only on
// a restart. An entry in the config is the user's answer and stays, and so
// does the question for a folder with settings that trusting would load.
func (*Adapter) TrustArgs(h tool.Home, dir string, args []string) ([]string, error) {
	dir = fsx.Real(sessionDir(dir, args))
	out, code, err := proc.Output("git", []string{"-C", dir, "rev-parse", "--git-common-dir"}, nil, 5*time.Second)
	lines := proc.Lines(out)
	if err != nil || code != 0 || len(lines) != 1 {
		return nil, nil
	}
	common := lines[0]
	if !filepath.IsAbs(common) {
		common = filepath.Join(dir, common)
	}
	scopes := trustScopes(dir, fsx.Real(common))
	if hasFolderSettings(scopes) {
		return nil, nil
	}
	b, err := os.ReadFile(filepath.Join(h.Dir, "config.toml"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if hasProject(string(b), scopes) {
		return nil, nil
	}
	if !utf8.ValidString(dir) || strings.ContainsFunc(dir, unicode.IsControl) {
		return nil, fmt.Errorf("cannot name %s in a Codex option", dir)
	}
	return []string{"-c", "projects={" + strconv.Quote(dir) + "={trust_level=\"trusted\"}}"}, nil
}

// sessionDir is the folder Codex runs in: dir, unless args name another.
func sessionDir(dir string, args []string) string {
	for i, a := range args {
		cd, ok := "", false
		switch {
		case a == "--":
			return dir
		case a == "-C" || a == "--cd":
			if i+1 < len(args) {
				cd, ok = args[i+1], true
			}
		case strings.HasPrefix(a, "--cd="):
			cd, ok = a[len("--cd="):], true
		case strings.HasPrefix(a, "-C"):
			cd, ok = a[len("-C"):], true
		}
		if ok {
			if filepath.IsAbs(cd) {
				return cd
			}
			return filepath.Join(dir, cd)
		}
	}
	return dir
}

// trustScopes lists the folders whose entry Codex applies to dir: dir up to
// the top of its worktree, then the main worktree.
func trustScopes(dir, commonDir string) []string {
	scopes := []string{filepath.Dir(commonDir)}
	for p := dir; ; p = filepath.Dir(p) {
		scopes = append(scopes, p)
		if fsx.Lstat(filepath.Join(p, ".git")) != nil || p == filepath.Dir(p) {
			return scopes
		}
	}
}

func hasFolderSettings(folders []string) bool {
	for _, p := range folders {
		for _, name := range folderSettings {
			if fsx.Lstat(filepath.Join(p, name)) != nil {
				return true
			}
		}
	}
	return false
}

// hasProject finds an entry for one of paths, as a table, a dotted key or a
// key of an inline projects table.
func hasProject(toml string, paths []string) bool {
	for _, p := range paths {
		for _, q := range []string{strconv.Quote(p), "'" + p + "'"} {
			q = regexp.QuoteMeta(q)
			if regexp.MustCompile(`projects\s*\.\s*` + q + `|` + q + `\s*=\s*\{`).MatchString(toml) {
				return true
			}
		}
	}
	return false
}
