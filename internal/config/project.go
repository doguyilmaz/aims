package config

import (
	"os"
	"path/filepath"
	"slices"

	"github.com/doguyilmaz/aims/internal/fsx"
)

// A project is the git repository a folder is in, or the folder itself
// outside one. Its own account order is kept in config.json, never in the
// project, and is in effect anywhere inside it.

// ProjectOf is the project dir belongs to. The home folder is never one
// repository's root, so a repository there does not take in every folder.
func ProjectOf(dir string) string {
	dir = fsx.Real(dir)
	home, _ := os.UserHomeDir()
	home = fsx.Real(home)
	for d := dir; d != home; d = filepath.Dir(d) {
		if _, err := os.Lstat(filepath.Join(d, ".git")); err == nil {
			return d
		}
		if filepath.Dir(d) == d {
			break
		}
	}
	return dir
}

// Here is the project of the working directory, "" when it is unknown.
func Here() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return ProjectOf(wd)
}

// here puts the order of project key in effect: its profiles first, then
// the others in the order used everywhere else.
func (c *Config) here(key string) {
	for _, t := range c.Tools {
		t.Everywhere = slices.Clone(t.Order)
		first := t.Projects[key]
		if key == "" || len(first) == 0 {
			continue
		}
		t.Project = key
		t.Order = OrderFor(first, t.Order)
		t.Active = t.Order[0]
	}
}

// OrderFor is names first, then the rest of order.
func OrderFor(names, order []string) []string {
	rest := slices.DeleteFunc(slices.Clone(order), func(n string) bool { return slices.Contains(names, n) })
	return append(slices.Clone(names), rest...)
}

// Arrange puts names first in the order of project key, or of every folder
// without one of its own when key is "". The first is the profile new
// sessions use there.
func (t *Tool) Arrange(names []string, key string) {
	if key == "" {
		t.Order, t.Active = OrderFor(names, t.Order), names[0]
		return
	}
	if t.Projects == nil {
		t.Projects = map[string][]string{}
	}
	t.Projects[key] = OrderFor(names, t.Projects[key])
}

// cleanProjects drops profiles that are gone from project orders, and the
// orders left empty.
func (t *Tool) cleanProjects() {
	for key, names := range t.Projects {
		var kept []string
		for _, n := range names {
			if t.Profiles[n] != nil && !slices.Contains(kept, n) {
				kept = append(kept, n)
			}
		}
		if len(kept) == 0 {
			delete(t.Projects, key)
		} else {
			t.Projects[key] = kept
		}
	}
	if len(t.Projects) == 0 {
		t.Projects = nil
	}
}
