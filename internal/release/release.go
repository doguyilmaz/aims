// Package release compares versions, works out how the running binary was
// installed, and checks GitHub for a newer release at most once a day.
package release

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/doguyilmaz/aims/internal/fsx"
)

const (
	repo = "doguyilmaz/aims"
	// LatestURL redirects to the newest release (drafts and prereleases
	// excluded). github.com is used rather than the API, whose unauthenticated
	// limit is shared by everyone behind the same office NAT.
	LatestURL  = "https://github.com/" + repo + "/releases/latest"
	tagPrefix  = "https://github.com/" + repo + "/releases/tag/"
	ModulePath = "github.com/" + repo + "/cmd/aims"
)

// InstallLine is the one-line install for this system.
func InstallLine(goos string) string {
	if goos == "windows" {
		return "irm https://raw.githubusercontent.com/" + repo + "/main/scripts/install.ps1 | iex"
	}
	return "curl -fsSL https://raw.githubusercontent.com/" + repo + "/main/scripts/install.sh | sh"
}

// Compare orders versions numerically per segment ("v" ignored); a release
// sorts after its pre-releases.
func Compare(a, b string) int {
	an, ap := split(a)
	bn, bp := split(b)
	for i := 0; i < max(len(an), len(bn)); i++ {
		x, y := at(an, i), at(bn, i)
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	switch {
	case ap == bp:
		return 0
	case ap == "":
		return 1
	case bp == "":
		return -1
	}
	return strings.Compare(ap, bp)
}

func split(v string) ([]int, string) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	v, _, _ = strings.Cut(v, "+")
	core, pre, _ := strings.Cut(v, "-")
	var nums []int
	for _, s := range strings.Split(core, ".") {
		n, err := strconv.Atoi(s)
		if err != nil {
			n = 0
		}
		nums = append(nums, n)
	}
	return nums, pre
}

func at(s []int, i int) int {
	if i < len(s) {
		return s[i]
	}
	return 0
}

// Cask is aims' Homebrew cask, by its full name.
const Cask = "doguyilmaz/tap/aims"

// Method is how the running binary was installed.
type Method int

const (
	Manual Method = iota
	Homebrew
	GoInstall
	Npm
	Scoop
	// Script: where the install script puts aims; upgraded by running the
	// script again, which checks the download against the release's
	// checksums.
	Script
)

// Detect classifies an install from the binary's resolved path. aims never
// overwrites itself: a package manager that owns the file is asked to
// upgrade it, so its records stay true.
func Detect(exe, goBin, scriptDir string) Method {
	slash := filepath.ToSlash(exe)
	switch {
	case scriptDir != "" && filepath.Dir(exe) == filepath.Clean(scriptDir):
		return Script
	case strings.Contains(slash, "/Caskroom/") || strings.Contains(slash, "/Cellar/") || strings.Contains(slash, "/linuxbrew/"):
		return Homebrew
	case strings.Contains(slash, "/node_modules/"):
		return Npm
	case strings.Contains(strings.ToLower(slash), "/scoop/apps/"):
		return Scoop
	case goBin != "" && filepath.Dir(exe) == filepath.Clean(goBin):
		return GoInstall
	}
	return Manual
}

// ScriptDir is where the install script puts aims: AIMS_BIN_DIR, or its
// default for goos.
func ScriptDir(goos, home string) string {
	if d := os.Getenv("AIMS_BIN_DIR"); d != "" {
		return d
	}
	if goos == "windows" {
		if d := os.Getenv("LOCALAPPDATA"); d != "" {
			return filepath.Join(d, "Programs", "aims")
		}
		return ""
	}
	return filepath.Join(home, ".local", "bin")
}

// UpgradeCommand is the command that upgrades an install in dir, as argv.
func UpgradeCommand(m Method, goos, dir string) [][]string {
	switch m {
	case Homebrew:
		// `brew update` refreshes the tap clone; without it upgrade can report
		// "already installed" for hours after a release. The cask is named
		// in full: another tap may have one called aims too.
		return [][]string{{"brew", "update"}, {"brew", "upgrade", "--cask", Cask}}
	case Script:
		if goos == "windows" {
			return [][]string{{"powershell", "-NoProfile", "-Command", "$env:AIMS_BIN_DIR='" + strings.ReplaceAll(dir, "'", "''") + "'; " + InstallLine(goos)}}
		}
		return [][]string{{"sh", "-c", strings.Replace(InstallLine(goos), "| sh", "| AIMS_BIN_DIR='"+strings.ReplaceAll(dir, "'", `'\''`)+"' sh", 1)}}
	case GoInstall:
		return [][]string{{"go", "install", ModulePath + "@latest"}}
	case Npm:
		return [][]string{{"npm", "install", "-g", "@doguyilmaz/aims@latest"}}
	case Scoop:
		return [][]string{{"scoop", "update"}, {"scoop", "update", "aims"}}
	}
	return nil
}

// Checker finds the latest version, hitting the network at most once per TTL.
type Checker struct {
	CachePath string
	TTL       time.Duration
	Fetch     func(ctx context.Context) (location string, err error)
	Now       func() time.Time
}

type cache struct {
	CheckedAt time.Time `json:"checkedAt"`
	Latest    string    `json:"latest"`
}

// Default returns the checker aims uses.
func Default(cacheDir string) *Checker {
	return &Checker{CachePath: filepath.Join(cacheDir, "update.json"), TTL: 24 * time.Hour, Fetch: fetchLocation, Now: time.Now}
}

// Cached is the last version seen, without touching the network.
func (c *Checker) Cached() string {
	var cf cache
	fsx.ReadJSONLenient(c.CachePath, &cf)
	return cf.Latest
}

// Latest returns the newest version, from the cache unless it expired or force is set.
func (c *Checker) Latest(ctx context.Context, force bool) (string, error) {
	var cf cache
	fsx.ReadJSONLenient(c.CachePath, &cf)
	if !force && c.Now().Sub(cf.CheckedAt) < c.TTL {
		return cf.Latest, nil
	}
	// Stamp first: an offline laptop then fails once a day, not on every command.
	cf.CheckedAt = c.Now().UTC()
	_ = fsx.WriteJSON(c.CachePath, cf, 0o644)
	loc, err := c.Fetch(ctx)
	if err != nil {
		return cf.Latest, err
	}
	tag, ok := strings.CutPrefix(loc, tagPrefix)
	if !ok || tag == "" || strings.ContainsAny(tag, "/?#") {
		return cf.Latest, fmt.Errorf("unexpected release location %q", loc)
	}
	cf.Latest = tag
	_ = fsx.WriteJSON(c.CachePath, cf, 0o644)
	return tag, nil
}

// fetchLocation reads the redirect target without following it: the answer is
// in the header, and nothing at the other end is wanted.
func fetchLocation(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, LatestURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "aims")
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	resp.Body.Close()
	if resp.StatusCode < 300 || resp.StatusCode > 399 {
		return "", fmt.Errorf("update check returned %s", resp.Status)
	}
	loc := resp.Header.Get("Location")
	if loc == "" {
		return "", errors.New("update check returned no location")
	}
	return loc, nil
}

// CacheDir is where aims keeps the update check.
func CacheDir() string {
	if d := os.Getenv("XDG_CACHE_HOME"); d != "" {
		return filepath.Join(d, "aims")
	}
	if d, err := os.UserCacheDir(); err == nil {
		return filepath.Join(d, "aims")
	}
	return filepath.Join(fsx.Home(), ".cache", "aims")
}
