// Package meter reads the plan limits of AI tools aims does not run sessions
// on yet (Antigravity, Grok, Kiro, OpenCode), through each tool's own CLI
// wherever it offers one.
package meter

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/tool"
)

// Meter reads one tool's limits.
type Meter interface {
	ID() string
	Title() string
	Bin() string
	Read(ctx context.Context, bin string) Reading
}

// Reading is what a tool reported. Rows are model groups or workspaces.
type Reading struct {
	Plan  string    `json:"plan,omitempty"`
	Rows  []Row     `json:"rows,omitempty"`
	Error string    `json:"error,omitempty"`
	At    time.Time `json:"at,omitzero"`
}

// Row is one set of windows: a model group, a workspace.
type Row struct {
	Name    string        `json:"name"`
	Windows []tool.Window `json:"windows"`
	// Detail is the amount behind the percentage, "$33.50 of $50".
	Detail string `json:"detail,omitempty"`
}

// Status is one installed tool and its last reading.
type Status struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Reading
}

// readTimeout bounds one tool's read.
const readTimeout = 45 * time.Second

var all = []Meter{agy{}, grok{}, kiro{}, opencode{}}

// binOf finds the tool's CLI; AIMS_<ID>_BIN names another one.
func binOf(m Meter) string {
	name := m.Bin()
	if v := os.Getenv("AIMS_" + strings.ToUpper(m.ID()) + "_BIN"); v != "" {
		name = v
	}
	bin, _ := exec.LookPath(name)
	return bin
}

func path() string { return filepath.Join(config.Home(), "meters.json") }

// Load returns the last reading of every installed tool.
func Load() []Status {
	var cache map[string]Reading
	fsx.ReadJSONLenient(path(), &cache)
	var out []Status
	for _, m := range all {
		if binOf(m) != "" {
			out = append(out, Status{ID: m.ID(), Title: m.Title(), Reading: cache[m.ID()]})
		}
	}
	return out
}

// Refresh asks every installed tool at once and saves what they report. A
// tool that fails keeps its last numbers, with the error beside them.
func Refresh(ctx context.Context) []Status {
	var mu sync.Mutex
	got := map[string]Reading{}
	var wg sync.WaitGroup
	for _, m := range all {
		bin := binOf(m)
		if bin == "" {
			continue
		}
		wg.Go(func() {
			c, cancel := context.WithTimeout(ctx, readTimeout)
			defer cancel()
			r := m.Read(c, bin)
			r.At = config.Now().UTC()
			mu.Lock()
			got[m.ID()] = r
			mu.Unlock()
		})
	}
	wg.Wait()
	_ = fsx.WithLock(path(), func() error {
		cache := map[string]Reading{}
		fsx.ReadJSONLenient(path(), &cache)
		for id, r := range got {
			if r.Error != "" && len(r.Rows) == 0 {
				r.Plan, r.Rows = cache[id].Plan, cache[id].Rows
			}
			cache[id] = r
		}
		return fsx.WriteJSON(path(), cache, 0o600)
	})
	return Load()
}

// pct is the percent of limit used, to one decimal, as tool.Window holds it.
func pct(used, limit float64) float64 {
	return float64(int(used/limit*1000+0.5)) / 10
}

// label names a window the way Claude Code and Codex rows do.
func label(window string) string {
	switch window {
	case "weekly", "week", "WEEKLY", "USAGE_PERIOD_TYPE_WEEKLY":
		return "7d"
	case "daily", "day", "DAILY", "USAGE_PERIOD_TYPE_DAILY":
		return "1d"
	case "monthly", "month", "MONTHLY", "USAGE_PERIOD_TYPE_MONTHLY":
		return "month"
	}
	return window
}

// workDir is an empty folder only this user can write: these CLIs load
// MCP servers, hooks and plugins from the folder they run in.
func workDir() (string, error) {
	d := filepath.Join(config.Home(), "meter")
	if err := os.MkdirAll(d, 0o700); err != nil {
		return "", err
	}
	return d, os.Chmod(d, 0o700)
}

// run runs a tool's CLI in workDir.
func run(ctx context.Context, bin string, args ...string) ([]byte, error) {
	dir, err := workDir()
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	cmd.WaitDelay = time.Second
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return nil, errors.New("no answer in time")
	}
	if len(out) > 0 {
		return out, nil
	}
	return nil, err
}

func firstLine(b []byte) string {
	for i, c := range b {
		if c == '\n' || i == 120 {
			return string(b[:i])
		}
	}
	return string(b)
}
