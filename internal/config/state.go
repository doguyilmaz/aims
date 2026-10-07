package config

import (
	"time"

	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/tool"
)

// Usage is the last usage report for a profile.
type Usage struct {
	Windows []tool.Window `json:"windows"`
	Source  string        `json:"source,omitempty"`
	At      time.Time     `json:"at,omitzero"`
}

// Account is who a profile is logged in as, as last seen.
type Account struct {
	Email string `json:"email,omitempty"`
	Plan  string `json:"plan,omitempty"`
}

// ProfileState is what aims learned about one profile.
type ProfileState struct {
	// Until is when a limited profile becomes usable again.
	Until      time.Time `json:"until,omitzero"`
	Reason     string    `json:"reason,omitempty"`
	NeedsLogin bool      `json:"needsLogin,omitempty"`
	// MarkedBy and Detail say who set the limit or login mark and the
	// provider's message, so a surprising mark can be explained.
	MarkedBy   string    `json:"markedBy,omitempty"`
	Detail     string    `json:"detail,omitempty"`
	MarkedAt   time.Time `json:"markedAt,omitzero"`
	Usage      *Usage    `json:"usage,omitempty"`
	Account    *Account  `json:"account,omitempty"`
	LastUsedAt time.Time `json:"lastUsedAt,omitzero"`
}

// State is ~/.aims/state.json.
type State map[tool.ID]map[string]*ProfileState

// LoadState reads the state. It is a cache: a damaged file reads as empty.
func LoadState() State {
	s := State{}
	fsx.ReadJSONLenient(statePath(), &s)
	return s
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
	if err := ensureHome(); err != nil {
		return err
	}
	return fsx.WithLock(statePath(), func() error {
		s := LoadState()
		if s[id] == nil {
			s[id] = map[string]*ProfileState{}
		}
		ps := s[id][name]
		if ps == nil {
			ps = &ProfileState{}
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
