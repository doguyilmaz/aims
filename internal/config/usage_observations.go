package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	Generation  string    `json:"generation,omitempty"`
	CreatedAt   string    `json:"createdAt,omitempty"`
	Account     *Account  `json:"account,omitempty"`
	Usage       *Usage    `json:"usage,omitempty"`
	Error       string    `json:"error,omitempty"`
	AttemptedAt time.Time `json:"attemptedAt"`
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
			state := *LoadStateFor(current).Get(id, name)
			fn(&state)
			if cache.Profiles[id] == nil {
				cache.Profiles[id] = map[string]*usageObservation{}
			}
			cache.Profiles[id][name] = &usageObservation{Generation: now.Generation, CreatedAt: now.CreatedAt, Account: state.Account, Usage: state.Usage, Error: state.UsageError, AttemptedAt: state.UsageAttemptedAt}
			return writeUsageObservations(cache)
		})
	})
}

func overlayUsageObservations(cfg *Config, state State) error {
	cache, err := readUsageObservations()
	if err != nil {
		return err
	}
	for id, entries := range cache.Profiles {
		for name, entry := range entries {
			if entry == nil || cfg.Tools[id] == nil {
				continue
			}
			p := cfg.Tools[id].Profiles[name]
			if p == nil || p.Purge != nil || entry.Generation != p.Generation || entry.CreatedAt != p.CreatedAt {
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
			if !entry.AttemptedAt.Before(mainAt) {
				ps.Usage = entry.Usage
				if entry.Account != nil {
					ps.Account = entry.Account
				}
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
			// A local/native cache update is not a successful retry of the
			// passive API request whose failure is reported here.
			ps.UsageError, ps.UsageAttemptedAt = entry.Error, entry.AttemptedAt
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
	return fsx.WithLock(usageObservationsPath(), func() error {
		cache, err := readUsageObservations()
		if err != nil {
			return nil
		}
		if cache.Profiles[id][name] == nil {
			return nil
		}
		delete(cache.Profiles[id], name)
		return writeUsageObservations(cache)
	})
}
