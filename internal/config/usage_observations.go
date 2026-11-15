package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/tool"
)

const usageObservationSchema = "1.0.0"
const maxUsageObservationBytes = 2 << 20

// No routing/login marks belong in this file. Old aims versions know only
// state.json and therefore cannot erase passive API observations or reset data.
type usageObservations struct {
	SchemaVersion string                                   `json:"schemaVersion"`
	Profiles      map[tool.ID]map[string]*usageObservation `json:"profiles"`
}

type usageObservation struct {
	Generation      string    `json:"generation,omitempty"`
	CreatedAt       string    `json:"createdAt,omitempty"`
	Account         *Account  `json:"account,omitempty"`
	BindingSet      bool      `json:"bindingSet,omitempty"`
	Binding         *Account  `json:"binding,omitempty"`
	BindingRevision string    `json:"bindingRevision,omitempty"`
	Usage           *Usage    `json:"usage,omitempty"`
	Error           string    `json:"error,omitempty"`
	AttemptedAt     time.Time `json:"attemptedAt"`
}

// CaptureUsageAccount captures both negative identity fences from one raw state
// read. Legacy state can lack registration metadata; its account is used only to
// reject changed identities, never as authentication or a display label.
func CaptureUsageAccount(id tool.ID, name string) (*Account, string) {
	ps := LoadState().Get(id, name)
	if a := ps.Account; a != nil {
		copy := *a
		return &copy, ps.AccountRevision
	}
	return nil, ps.AccountRevision
}

// Known fields in current must agree with a prior observation. Losing optional
// metadata in an old writer is not evidence of a new account; learning a new
// identity requires a new observation rather than borrowing unknown metadata.
func matchesUsageAccount(prior, current *Account) bool {
	if current == nil || prior == nil {
		return current == nil && prior == nil
	}
	return (current.Email == "" || strings.EqualFold(prior.Email, current.Email)) &&
		(current.Tenant == "" || prior.Tenant == current.Tenant) &&
		(current.Plan == "" || strings.EqualFold(current.Plan, "apiKey") == strings.EqualFold(prior.Plan, "apiKey"))
}

func usageObservationsPath() string { return filepath.Join(Home(), "usage-observations.json") }

func readUsageObservations() (usageObservations, error) {
	c := usageObservations{SchemaVersion: usageObservationSchema, Profiles: map[tool.ID]map[string]*usageObservation{}}
	f, err := os.Open(usageObservationsPath())
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxUsageObservationBytes+1))
	if err != nil {
		return c, err
	}
	if len(b) > maxUsageObservationBytes {
		return c, fmt.Errorf("usage observation cache is too large; file kept")
	}
	var decoded usageObservations
	if err := json.Unmarshal(b, &decoded); err != nil {
		return c, fmt.Errorf("usage observation cache is damaged; file kept: %w", err)
	}
	if decoded.SchemaVersion != usageObservationSchema || decoded.Profiles == nil {
		return c, fmt.Errorf("unsupported usage observation cache; file kept")
	}
	return decoded, nil
}

func writeUsageObservations(cache usageObservations) error {
	b, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}
	if len(b)+1 > maxUsageObservationBytes {
		return fmt.Errorf("usage observation cache would exceed its size limit; previous cache kept")
	}
	return fsx.WriteFile(usageObservationsPath(), append(b, '\n'), 0o600)
}

// UpdateUsageObservationFor commits only observations, under the same profile
// identity fence as UpdateStateFor. It never writes state.json or config.json.
func UpdateUsageObservationFor(before *Config, id tool.ID, name string, fn func(*ProfileState)) error {
	account, revision := CaptureUsageAccount(id, name)
	return CommitUsageObservation(before, id, name, account, revision, fn)
}

// CommitUsageObservation also fences native identity corrections that happened
// while the provider request was in flight, including a switch back to a prior
// e-mail/workspace value. Both fences are captured before asking the provider.
func CommitUsageObservation(before *Config, id tool.ID, name string, account *Account, revision string, fn func(*ProfileState)) error {
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
		if err := ensureHome(); err != nil {
			return err
		}
		return fsx.WithLock(usageObservationsPath(), func() error {
			cache, err := readUsageObservations()
			if err != nil {
				return err // never overwrite damaged or newer-format data
			}
			binding, bindingRevision := CaptureUsageAccount(id, name)
			if revision != bindingRevision || !matchesUsageAccount(account, binding) {
				return fmt.Errorf("account identity changed; ignored stale usage observation")
			}
			state := *LoadStateFor(current).Get(id, name)
			fn(&state)
			if cache.Profiles[id] == nil {
				cache.Profiles[id] = map[string]*usageObservation{}
			}
			cache.Profiles[id][name] = &usageObservation{Generation: now.Generation, CreatedAt: now.CreatedAt, Account: state.Account, BindingSet: true, Binding: binding, BindingRevision: bindingRevision, Usage: state.Usage, Error: state.UsageError, AttemptedAt: state.UsageAttemptedAt}
			for toolID, entries := range cache.Profiles {
				for label, entry := range entries {
					var p *Profile
					if current.Tools[toolID] != nil {
						p = current.Tools[toolID].Profiles[label]
					}
					if p == nil || entry == nil || entry.Generation != p.Generation || entry.CreatedAt != p.CreatedAt {
						delete(entries, label)
					}
				}
			}
			return writeUsageObservations(cache)
		})
	})
}

func overlayUsageObservations(cfg *Config, state State) error {
	cache, err := readUsageObservations()
	if err != nil {
		return err
	}
	raw := LoadState()
	for id, entries := range cache.Profiles {
		for name, entry := range entries {
			if entry == nil || cfg.Tools[id] == nil {
				continue
			}
			p := cfg.Tools[id].Profiles[name]
			if p == nil || p.Purge != nil || entry.Generation != p.Generation || entry.CreatedAt != p.CreatedAt {
				continue
			}
			currentAccount := raw.Get(id, name).Account
			if !entry.BindingSet || entry.BindingRevision != raw.Get(id, name).AccountRevision || !matchesUsageAccount(entry.Binding, currentAccount) && !matchesUsageAccount(entry.Account, currentAccount) {
				continue
			}
			if state[id] == nil {
				state[id] = map[string]*ProfileState{}
			}
			ps := state.Get(id, name)
			ps.Generation = p.Generation
			state[id][name] = ps
			mainAt := ps.UsageAttemptedAt
			if ps.Usage != nil && ps.Usage.At.After(mainAt) {
				mainAt = ps.Usage.At
			}
			mainIsQuotaSuccess := ps.UsageError == "" && ps.Usage != nil && ps.Usage.Source == "live" && !ps.Usage.At.Before(ps.UsageAttemptedAt)
			if entry.Usage != nil && (ps.Usage == nil || !entry.Usage.At.Before(ps.Usage.At)) {
				ps.Usage = entry.Usage
			} else if ps.Usage != nil && entry.Usage != nil {
				// Native status lines can update a window without knowing model
				// groups/reset inventories. Retain those API-only details and use
				// the oldest contributing timestamp; never falsely freshen them.
				u := *ps.Usage
				retained := false
				if u.Groups == nil && entry.Usage.Groups != nil {
					u.Groups, retained = entry.Usage.Groups, true
				}
				if u.Resets == nil && entry.Usage.Resets != nil {
					u.Resets, retained = entry.Usage.Resets, true
				}
				if retained {
					if entry.Usage.At.Before(u.At) {
						u.At = entry.Usage.At
					}
					u.Source = "mixed-cache"
					ps.Usage = &u
				}
			}
			if entry.Account != nil && !entry.AttemptedAt.Before(mainAt) {
				ps.Account = entry.Account
			}
			if entry.Usage == nil && entry.Account != nil && !matchesUsageAccount(currentAccount, entry.Account) && !entry.AttemptedAt.Before(mainAt) {
				ps.Usage = nil // a newer API observed a different login, without quotas
			}
			// Preserve a newer native failure. A newer status-line update may
			// retain an older API error, but a real successful quota read can
			// settle it. Never rewind an error to an older successful attempt.
			if entry.AttemptedAt.After(mainAt) || ps.UsageError == "" && (!entry.AttemptedAt.Before(mainAt) || entry.Error != "" && !mainIsQuotaSuccess) {
				ps.UsageError, ps.UsageAttemptedAt = entry.Error, entry.AttemptedAt
			}
		}
	}
	return nil
}

// RemoveUsageObservation forgets only this label. Unreadable/newer formats are
// kept and reported by LoadStateView/doctor. An optional cache must not prevent
// profile creation, logout or removal; registration fences invalidate its rows.
func RemoveUsageObservation(id tool.ID, name string) error {
	if _, err := os.Stat(usageObservationsPath()); os.IsNotExist(err) {
		return nil
	}
	unlock, ok, err := fsx.Lock(usageObservationsPath(), 0)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("usage cache is busy; deferred optional cleanup")
	}
	defer unlock()
	return func() error {
		cache, err := readUsageObservations()
		if err != nil {
			return nil
		}
		if cache.Profiles[id][name] == nil {
			return nil
		}
		delete(cache.Profiles[id], name)
		return writeUsageObservations(cache)
	}()
}

// UsageObservationNotes reports ignored old data without turning optional
// cleanup failures into login/logout/purge failures or rewriting unknown data.
func UsageObservationNotes(cfg *Config) []string {
	cache, err := readUsageObservations()
	if err != nil {
		return []string{"usage-observations.json could not be read and was kept: " + err.Error()}
	}
	raw := LoadState()
	stale, unbound, identityMismatch := 0, 0, 0
	for id, entries := range cache.Profiles {
		for name, entry := range entries {
			var p *Profile
			if cfg.Tools[id] != nil {
				p = cfg.Tools[id].Profiles[name]
			}
			if entry == nil || p == nil || p.Purge != nil || entry.Generation != p.Generation || entry.CreatedAt != p.CreatedAt || entry.BindingRevision != raw.Get(id, name).AccountRevision {
				stale++
			} else if !entry.BindingSet {
				unbound++
			} else if account := raw.Get(id, name).Account; !matchesUsageAccount(entry.Binding, account) && !matchesUsageAccount(entry.Account, account) {
				identityMismatch++
			}
		}
	}
	var notes []string
	if stale > 0 {
		notes = append(notes, fmt.Sprintf("%d stale usage observations are ignored; a later successful refresh prunes removed registrations", stale))
	}
	if unbound > 0 {
		notes = append(notes, fmt.Sprintf("%d earlier usage observations need a fresh identity binding; run aims usage --refresh", unbound))
	}
	if identityMismatch > 0 {
		notes = append(notes, fmt.Sprintf("%d usage observations no longer match the native account; run aims usage --refresh", identityMismatch))
	}
	return notes
}
