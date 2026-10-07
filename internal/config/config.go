// Package config reads and writes aims' own files:
//
//	~/.aims/config.json   profiles and preferences (safe to edit by hand)
//	~/.aims/state.json    what aims learned: cooldowns, usage, accounts
//	~/.aims/profiles/<tool>/<name>/   one CLAUDE_CONFIG_DIR / CODEX_HOME per account
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/tool"
	"github.com/doguyilmaz/aims/internal/tools"
)

// Profile is one account of one tool.
type Profile struct {
	CreatedAt string `json:"createdAt,omitempty"`
	// Existing profiles use the tool's own home and the login already in it.
	Existing bool `json:"existing,omitempty"`
	// Share is what the profile shares with the hub: ShareAll (default),
	// ShareSettings (everything but conversation history) or ShareNone.
	Share string `json:"share,omitempty"`
	// Isolated is the older spelling of Share: "none".
	Isolated bool `json:"isolated,omitempty"`
	// Dir overrides the profile directory.
	Dir string `json:"dir,omitempty"`
	// Exclude keeps shared entries private to this profile.
	Exclude []string `json:"exclude,omitempty"`
	// Env is extra environment for this profile, e.g. an API key account.
	Env map[string]string `json:"env,omitempty"`
}

// Sharing modes.
const (
	ShareAll      = "all"
	ShareSettings = "settings"
	ShareNone     = "none"
)

// ShareMode is the profile's effective sharing mode.
func (p *Profile) ShareMode() string {
	switch {
	case p.Isolated || p.Share == ShareNone:
		return ShareNone
	case p.Share == ShareSettings:
		return ShareSettings
	}
	return ShareAll
}

// ValidShare reports whether s names a sharing mode.
func ValidShare(s string) bool { return s == ShareAll || s == ShareSettings || s == ShareNone }

// Tool is the configuration of one tool.
type Tool struct {
	// Hub is the tool's normal home. Shared entries of every profile point here.
	Hub string `json:"hub,omitempty"`
	// HubEnv is CLAUDE_CONFIG_DIR / CODEX_HOME exactly as the user had it, or
	// "" when it was unset.
	HubEnv   string              `json:"hubEnv,omitempty"`
	Active   string              `json:"active,omitempty"`
	Order    []string            `json:"order,omitempty"`
	Profiles map[string]*Profile `json:"profiles,omitempty"`
}

// Failover tunes automatic account switching.
type Failover struct {
	// Auto picks another profile when the chosen one is limited or logged out.
	Auto bool `json:"auto"`
	// CooldownMinutes applies when a limit's reset time is unknown.
	CooldownMinutes int `json:"cooldownMinutes"`
	// Threshold is the usage percentage treated as "limited".
	Threshold float64 `json:"threshold"`
}

// Config is ~/.aims/config.json.
type Config struct {
	Version  int               `json:"version"`
	Failover Failover          `json:"failover"`
	Status   StatusLine        `json:"statusline,omitzero"`
	Tools    map[tool.ID]*Tool `json:"tools"`
}

// StatusLine holds the user's previous Claude status line, shown before aims'.
type StatusLine struct {
	Chain string `json:"chain,omitempty"`
}

// Home is aims' own directory, ~/.aims or $AIMS_HOME.
func Home() string {
	if v := os.Getenv("AIMS_HOME"); v != "" {
		return fsx.Abs(v)
	}
	return filepath.Join(fsx.Home(), ".aims")
}

func configPath() string { return filepath.Join(Home(), "config.json") }
func statePath() string  { return filepath.Join(Home(), "state.json") }

// ProfilesRoot holds the profile directories aims creates.
func ProfilesRoot() string { return filepath.Join(Home(), "profiles") }

var nameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,39}$`)

// ValidName reports whether s can name a profile.
func ValidName(s string) bool { return nameRe.MatchString(s) }

// Load reads the config and fills defaults. A config that does not parse is an
// error, never an empty config: every command writes it back.
func Load() (*Config, error) {
	c := &Config{Version: 1, Failover: Failover{Auto: true, CooldownMinutes: 300, Threshold: 95}}
	if _, err := fsx.ReadJSON(configPath(), c); err != nil {
		return nil, err
	}
	c.normalize()
	return c, nil
}

func (c *Config) normalize() {
	if c.Tools == nil {
		c.Tools = map[tool.ID]*Tool{}
	}
	if c.Failover.CooldownMinutes <= 0 {
		c.Failover.CooldownMinutes = 300
	}
	if c.Failover.Threshold <= 0 || c.Failover.Threshold > 100 {
		c.Failover.Threshold = 95
	}
	for _, id := range tools.IDs() {
		t := c.Tools[id]
		if t == nil {
			t = &Tool{}
			c.Tools[id] = t
		}
		if t.Profiles == nil {
			t.Profiles = map[string]*Profile{}
		}
		for n, p := range t.Profiles {
			if p == nil {
				t.Profiles[n] = &Profile{}
			}
		}
		order := make([]string, 0, len(t.Profiles))
		for _, n := range t.Order {
			if t.Profiles[n] != nil && !slices.Contains(order, n) {
				order = append(order, n)
			}
		}
		var rest []string
		for n := range t.Profiles {
			if !slices.Contains(order, n) {
				rest = append(rest, n)
			}
		}
		sort.Strings(rest)
		t.Order = append(order, rest...)
		if t.Profiles[t.Active] == nil {
			t.Active = ""
		}
	}
}

// Save writes the config atomically.
func (c *Config) Save() error {
	return fsx.WithLock(configPath(), func() error { return fsx.WriteJSON(configPath(), c, 0o600) })
}

// Update loads the config, applies fn and saves, under the config lock.
func Update(fn func(*Config) error) (*Config, error) {
	var out *Config
	err := fsx.WithLock(configPath(), func() error {
		c, err := Load()
		if err != nil {
			return err
		}
		if err := fn(c); err != nil {
			return err
		}
		out = c
		return fsx.WriteJSON(configPath(), c, 0o600)
	})
	return out, err
}

// Tool returns the config of one tool.
func (c *Config) Tool(id tool.ID) *Tool { return c.Tools[id] }

// Profile returns a profile or an error naming it.
func (c *Config) Profile(id tool.ID, name string) (*Profile, error) {
	if p := c.Tools[id].Profiles[name]; p != nil {
		return p, nil
	}
	return nil, fmt.Errorf("no %s profile %q (see: aims status)", id, name)
}

// HasProfiles reports whether any tool has a profile.
func (c *Config) HasProfiles() bool {
	for _, t := range c.Tools {
		if len(t.Profiles) > 0 {
			return true
		}
	}
	return false
}

// EnsureHub records the tool's home the first time it is needed. The value is
// kept, so a shell that later exports CLAUDE_CONFIG_DIR for a profile does not
// move it.
func (c *Config) EnsureHub(id tool.ID) string {
	t := c.Tools[id]
	if t.Hub != "" {
		return t.Hub
	}
	layout := tools.Get(id).Layout()
	if v := os.Getenv(layout.HomeEnv); v != "" && !fsx.Within(fsx.Abs(v), ProfilesRoot()) {
		t.Hub = fsx.Abs(v)
		// Kept verbatim: Claude Code hashes the exact string into the name of
		// its macOS keychain entry.
		t.HubEnv = v
	} else {
		t.Hub = filepath.Join(fsx.Home(), layout.DefaultHome)
		t.HubEnv = ""
	}
	return t.Hub
}

// Dir is the profile's config directory.
func (c *Config) Dir(id tool.ID, name string) string {
	p := c.Tools[id].Profiles[name]
	switch {
	case p == nil:
		return ""
	case p.Existing:
		return c.EnsureHub(id)
	case p.Dir != "":
		return fsx.Abs(p.Dir)
	}
	return filepath.Join(ProfilesRoot(), string(id), name)
}

// HomeEnv is the value of CLAUDE_CONFIG_DIR / CODEX_HOME for a profile, or ""
// to leave it unset.
//
// The profile that adopted the existing login runs with the variable exactly
// as the user had it, usually unset. Claude Code derives the keychain entry
// and the location of .claude.json from it, so even ~/.claude spelled out
// would point at a different, empty login.
func (c *Config) HomeEnv(id tool.ID, name string) string {
	p := c.Tools[id].Profiles[name]
	if p == nil {
		return ""
	}
	if p.Existing {
		c.EnsureHub(id)
		return c.Tools[id].HubEnv
	}
	return c.Dir(id, name)
}

// ProfileForHomeEnv finds the profile that owns a CLAUDE_CONFIG_DIR /
// CODEX_HOME value ("" meaning unset).
func (c *Config) ProfileForHomeEnv(id tool.ID, value string) string {
	norm := func(v string) string {
		if v == "" {
			return ""
		}
		return fsx.Abs(v)
	}
	want := norm(value)
	t := c.Tools[id]
	for _, n := range t.Order {
		if norm(c.HomeEnv(id, n)) == want {
			return n
		}
	}
	if want != "" && t.Hub != "" && want == fsx.Abs(t.Hub) {
		for _, n := range t.Order {
			if t.Profiles[n].Existing {
				return n
			}
		}
	}
	return ""
}

// ExistingProfile returns the profile that adopted the tool's own login.
func (c *Config) ExistingProfile(id tool.ID) string {
	for _, n := range c.Tools[id].Order {
		if c.Tools[id].Profiles[n].Existing {
			return n
		}
	}
	return ""
}

// Pinned is the profile this terminal is pinned to by `aims env`.
func Pinned(id tool.ID) string { return os.Getenv(PinVar(id)) }

// PinVar is the variable `aims env` sets to pin a terminal, e.g. AIMS_CLAUDE_PROFILE.
func PinVar(id tool.ID) string { return "AIMS_" + strings.ToUpper(string(id)) + "_PROFILE" }

// Preferred is the profile a new session uses: explicit, then this terminal's
// pin, then the active profile, then the first one.
func (c *Config) Preferred(id tool.ID, explicit string) (string, error) {
	t := c.Tools[id]
	if explicit != "" {
		if t.Profiles[explicit] == nil {
			return "", fmt.Errorf("no %s profile %q (see: aims status)", id, explicit)
		}
		return explicit, nil
	}
	for _, n := range []string{Pinned(id), t.Active} {
		if n != "" && t.Profiles[n] != nil {
			return n, nil
		}
	}
	if len(t.Order) > 0 {
		return t.Order[0], nil
	}
	return "", nil
}

// Now is overridable in tests.
var Now = time.Now
