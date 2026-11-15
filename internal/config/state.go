package config

import (
	"errors"
	"time"

	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/tool"
)

// Usage is the last usage report for a profile.
type Usage struct {
	Windows []tool.Window      `json:"windows"`
	Groups  []tool.QuotaGroup  `json:"groups,omitempty"`
	Resets  *tool.ResetBalance `json:"resets,omitempty"`
	Source  string             `json:"source,omitempty"`
	At      time.Time          `json:"at,omitzero"`
}

// Account is who a profile is logged in as, as last seen.
type Account struct {
	Email string `json:"email,omitempty"`
	Plan  string `json:"plan,omitempty"`
	// Tenant is the organization or workspace of the login, as
	// tool.Account.Tenant.
	Tenant string `json:"tenant,omitempty"`
}

// ProfileState is what aims learned about one profile.
type ProfileState struct {
	Generation string `json:"generation,omitempty"`
	// Until is when a limited profile becomes usable again.
	Until  time.Time `json:"until,omitzero"`
	Reason string    `json:"reason,omitempty"`
	// Window is the usage window the limit is for ("5h", "7d"), when the
	// provider's message or the usage report tells.
	Window     string `json:"window,omitempty"`
	NeedsLogin bool   `json:"needsLogin,omitempty"`
	// MarkedBy and Detail say who set the limit or login mark and the
	// provider's message, so a surprising mark can be explained.
	MarkedBy         string    `json:"markedBy,omitempty"`
	Detail           string    `json:"detail,omitempty"`
	MarkedAt         time.Time `json:"markedAt,omitzero"`
	Usage            *Usage    `json:"usage,omitempty"`
	UsageError       string    `json:"usageError,omitempty"`
	UsageAttemptedAt time.Time `json:"usageAttemptedAt,omitzero"`
	Account          *Account  `json:"account,omitempty"`
	LastUsedAt       time.Time `json:"lastUsedAt,omitzero"`
}

// State is ~/.aims/state.json.
type State map[tool.ID]map[string]*ProfileState

// LoadState reads the state. It is a cache: a damaged file reads as empty.
func LoadState() State {
	s := State{}
	fsx.ReadJSONLenient(statePath(), &s)
	return s
}

// LoadStateFor ignores observations from a previous registration of the label.
// Older writers that do not retain generation metadata cannot relabel a new
// profile with their old account, usage or routing marks.
func LoadStateFor(cfg *Config) State {
	s := LoadState()
	if cfg == nil {
		return State{}
	}
	for id, profiles := range s {
		for name, state := range profiles {
			var p *Profile
			if t := cfg.Tools[id]; t != nil {
				p = t.Profiles[name]
			}
			if p == nil || p.Purge != nil || state == nil || state.Generation != p.Generation {
				delete(profiles, name)
			}
		}
	}
	return s
}

var ErrProfileChanged = errors.New("profile registration changed or removal is pending; ignored stale result")

// UpdateStateFor commits a result only to the registration that requested it.
// Lock ordering is config then state, as for profile creation/removal.
func UpdateStateFor(before *Config, id tool.ID, name string, fn func(*ProfileState)) error {
	return fsx.WithLock(configPath(), func() error {
		current, err := load()
		if err != nil {
			return err
		}
		if before == nil || before.Tools[id] == nil || current.Tools[id] == nil {
			return ErrProfileChanged
		}
		want, now := before.Tools[id].Profiles[name], current.Tools[id].Profiles[name]
		if want == nil || now == nil || want.Purge != nil || now.Purge != nil || want.Generation != now.Generation || want.CreatedAt != now.CreatedAt {
			return ErrProfileChanged
		}
		return updateState(id, name, now.Generation, fn)
	})
}

// Get returns a profile's state, never nil.
func (s State) Get(id tool.ID, name string) *ProfileState {
	if ps := s[id][name]; ps != nil {
		return ps
	}
	return &ProfileState{}
}

// UpdateState applies fn to one profile's state under the state lock.
func UpdateState(id tool.ID, name string, fn func(*ProfileState)) error {
	cfg, err := load()
	if err != nil {
		return err
	}
	generation := ""
	if t := cfg.Tools[id]; t != nil {
		if p := t.Profiles[name]; p != nil {
			generation = p.Generation
		}
	}
	return updateState(id, name, generation, fn)
}

func updateState(id tool.ID, name, generation string, fn func(*ProfileState)) error {
	if err := ensureHome(); err != nil {
		return err
	}
	return fsx.WithLock(statePath(), func() error {
		s := LoadState()
		if s[id] == nil {
			s[id] = map[string]*ProfileState{}
		}
		ps := s[id][name]
		if ps == nil || ps.Generation != generation {
			ps = &ProfileState{Generation: generation}
			s[id][name] = ps
		}
		fn(ps)
		return fsx.WriteJSON(statePath(), s, 0o600)
	})
}

// RemoveState forgets a profile.
func RemoveState(id tool.ID, name string) error {
	return fsx.WithLock(statePath(), func() error {
		s := LoadState()
		if s[id] == nil || s[id][name] == nil {
			return nil
		}
		delete(s[id], name)
		return fsx.WriteJSON(statePath(), s, 0o600)
	})
}
