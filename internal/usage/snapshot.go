// Package usage provides a credential-free, cached view for usage displays.
package usage

import (
	"fmt"
	"time"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/meter"
	"github.com/doguyilmaz/aims/internal/profiles"
	"github.com/doguyilmaz/aims/internal/tool"
	"github.com/doguyilmaz/aims/internal/tools"
)

type Snapshot struct {
	SchemaVersion     int           `json:"schemaVersion"`
	GeneratedAt       time.Time     `json:"generatedAt"`
	Providers         []Provider    `json:"providers"`
	CustomResets      []CustomReset `json:"customResets"`
	CustomResetsError string        `json:"customResetsError,omitempty"`
}

type Provider struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Installed   bool      `json:"installed"`
	Rows        []Row     `json:"rows"`
	Error       string    `json:"error,omitempty"`
	AttemptedAt time.Time `json:"attemptedAt,omitzero"`
}

type Row struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Plan        string             `json:"plan,omitempty"`
	Detail      string             `json:"detail,omitempty"`
	Windows     []tool.Window      `json:"windows"`
	Resets      *tool.ResetBalance `json:"bankedResets"`
	ObservedAt  time.Time          `json:"observedAt,omitzero"`
	Source      string             `json:"source,omitempty"`
	Error       string             `json:"error,omitempty"`
	AttemptedAt time.Time          `json:"attemptedAt,omitzero"`
}

// Load reads aims caches only: no provider, keychain, account inspection,
// directory adoption, sharing reconciliation, or model request is performed.
func Load(filter string) (Snapshot, error) {
	cfg, err := config.Load()
	if err != nil {
		return Snapshot{}, err
	}
	state := config.LoadState()
	s := Snapshot{SchemaVersion: 1, GeneratedAt: config.Now().UTC(), Providers: []Provider{}}
	custom, err := readCustom()
	if err != nil {
		s.CustomResetsError = "Custom reminders could not be read; usage-reminders.json was left unchanged."
		s.CustomResets = []CustomReset{}
	} else {
		s.CustomResets = custom.Resets
	}
	for _, id := range tools.IDs() {
		a := tools.Get(id)
		p := Provider{ID: string(id), Name: a.Title(), Installed: profiles.Bin(a) != "", Rows: []Row{}}
		for _, n := range cfg.Tools[id].Order {
			cached := state.Get(id, n)
			r := Row{ID: n, Name: n, Windows: []tool.Window{}, Error: cached.UsageError, AttemptedAt: cached.UsageAttemptedAt}
			if cached.Account != nil {
				r.Plan = cached.Account.Plan
			}
			if u := cached.Usage; u != nil {
				r.Windows, r.Resets, r.ObservedAt, r.Source = u.Windows, u.Resets, u.At, u.Source
				if r.Windows == nil {
					r.Windows = []tool.Window{}
				}
				p.Rows = append(p.Rows, r)
				for _, g := range u.Groups {
					p.Rows = append(p.Rows, Row{ID: n + "/" + g.ID, Name: n + " · " + g.Name, Windows: g.Windows, ObservedAt: u.At, Source: u.Source, Error: cached.UsageError, AttemptedAt: cached.UsageAttemptedAt})
				}
			} else {
				p.Rows = append(p.Rows, r)
			}
		}
		s.Providers = append(s.Providers, p)
	}
	meters := map[string]meter.Status{}
	for _, m := range meter.Cached() {
		meters[m.ID] = m
	}
	for _, spec := range []struct{ id, name string }{{"kiro", "Kiro"}, {"grok", "Grok"}, {"agy", "Antigravity"}, {"opencode", "OpenCode"}} {
		m := meters[spec.id]
		p := Provider{ID: spec.id, Name: spec.name, Installed: m.Installed, Rows: []Row{}, Error: m.Error, AttemptedAt: m.AttemptedAt}
		if p.Error != "" {
			p.Error = "Usage refresh failed; the previous reading is retained. Check the provider CLI."
		}
		for i, r := range m.Rows {
			id := r.ID
			if id == "" {
				id = fmt.Sprintf("%s/%d", r.Name, i)
			}
			p.Rows = append(p.Rows, Row{ID: id, Name: r.Name, Plan: m.Plan, Detail: r.Detail, Windows: r.Windows, ObservedAt: m.At, Source: "native-usage"})
		}
		s.Providers = append(s.Providers, p)
	}
	if filter != "" {
		filtered := []CustomReset{}
		for _, r := range s.CustomResets {
			if r.Provider == filter {
				filtered = append(filtered, r)
			}
		}
		s.CustomResets = filtered
		for _, p := range s.Providers {
			if p.ID == filter {
				s.Providers = []Provider{p}
				return s, nil
			}
		}
		return Snapshot{}, fmt.Errorf("unknown usage provider %q (claude, codex, kiro, grok, agy, opencode)", filter)
	}
	return s, nil
}
