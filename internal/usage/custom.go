package usage

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/fsx"
)

// CustomReset is a user reminder, never evidence that a provider changed quota.
type CustomReset struct {
	ID       string    `json:"id"`
	Provider string    `json:"provider"`
	Row      string    `json:"row"`
	Label    string    `json:"label"`
	At       time.Time `json:"at"`
}

type customFile struct {
	SchemaVersion int           `json:"schemaVersion"`
	Resets        []CustomReset `json:"resets"`
}

func customPath() string { return filepath.Join(config.Home(), "usage-reminders.json") }

func readCustom() (customFile, error) {
	f := customFile{SchemaVersion: 1, Resets: []CustomReset{}}
	if _, err := fsx.ReadJSON(customPath(), &f); err != nil {
		return f, err
	}
	if f.SchemaVersion != 1 {
		return f, fmt.Errorf("unsupported usage-reminders schema %d; file was left unchanged", f.SchemaVersion)
	}
	return f, nil
}

func AddCustom(provider, row, label string, at time.Time) error {
	if strings.TrimSpace(label) == "" || len(label) > 120 || strings.ContainsAny(label, "\r\n\x1b") {
		return fmt.Errorf("give the reminder a single-line label of at most 120 bytes")
	}
	if at.IsZero() || !at.After(config.Now()) {
		return fmt.Errorf("custom reset time must be in the future")
	}
	s, err := Load(provider)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(s.Providers[0].Rows, func(r Row) bool { return r.ID == row }) {
		return fmt.Errorf("unknown %s account or group %q; use the row ID from aims usage --json", provider, row)
	}
	var id [8]byte
	if _, err := rand.Read(id[:]); err != nil {
		return err
	}
	return updateCustom(func(f *customFile) error {
		f.Resets = append(f.Resets, CustomReset{ID: hex.EncodeToString(id[:]), Provider: provider, Row: row, Label: label, At: at.UTC()})
		return nil
	})
}

func RemoveCustom(id string) error {
	return updateCustom(func(f *customFile) error {
		before := len(f.Resets)
		f.Resets = slices.DeleteFunc(f.Resets, func(r CustomReset) bool { return r.ID == id })
		if len(f.Resets) == before {
			return fmt.Errorf("custom reset %q does not exist", id)
		}
		return nil
	})
}

func updateCustom(change func(*customFile) error) error {
	if err := os.MkdirAll(config.Home(), 0o700); err != nil {
		return err
	}
	return fsx.WithLock(customPath(), func() error {
		f, err := readCustom()
		if err != nil {
			return err
		}
		if err = change(&f); err != nil {
			return err
		}
		return fsx.WriteJSON(customPath(), f, 0o600)
	})
}
