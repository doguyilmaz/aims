package ops

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/shell"
	"github.com/doguyilmaz/aims/internal/tools"
)

// Clean takes aims back out of the tools (skill, MCP server, status line,
// shell line) and forgets its settings and marks. Logins and account folders
// stay, so `aims init` brings everything back without logging in again.
func Clean(ctx context.Context) ([]Step, error) {
	steps, err := Uninstall(ctx, Integrations{Skill: true, MCP: true, StatusLine: true, Shell: shell.Detect()})
	if err != nil {
		return steps, err
	}
	for _, f := range []string{"config.json", "state.json"} {
		p := filepath.Join(config.Home(), f)
		switch err := os.Remove(p); {
		case err == nil:
			steps = append(steps, Step{What: "forgot", Target: fsx.Tildify(p)})
		case !errors.Is(err, os.ErrNotExist):
			steps = append(steps, Step{What: "forget", Target: fsx.Tildify(p), Err: err})
		}
	}
	return steps, nil
}

// WipePlan lists the account folders Wipe would delete and the custom
// folders it would only forget.
func WipePlan() (deleted, kept []string, err error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, err
	}
	for _, a := range tools.All() {
		for _, n := range cfg.Tools[a.ID()].Order {
			switch p := cfg.Tools[a.ID()].Profiles[n]; {
			case p.Existing:
			case p.Dir != "":
				kept = append(kept, fsx.Tildify(cfg.Dir(a.ID(), n)))
			default:
				deleted = append(deleted, fsx.Tildify(cfg.Dir(a.ID(), n)))
			}
		}
	}
	return deleted, kept, nil
}

// Wipe logs out and deletes the account folders aims created, then cleans.
// The tools' own folders, with every conversation, skill, setting and the
// main logins, stay. It stops before changing anything else when a folder
// holds something that is not shared, unless force.
func Wipe(ctx context.Context, force bool) ([]Step, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	var steps []Step
	for _, a := range tools.All() {
		for _, n := range slices.Clone(cfg.Tools[a.ID()].Order) {
			p := cfg.Tools[a.ID()].Profiles[n]
			if p.Existing {
				continue
			}
			purge := p.Dir == ""
			res, err := RemoveProfile(ctx, a, n, purge, force)
			what := n + " logged out and deleted"
			if !purge {
				what = n + " forgotten, its folder kept"
			}
			steps = append(steps, Step{Tool: a.ID(), What: what, Target: fsx.Tildify(res.Dir), Err: err})
			if err != nil {
				return steps, fmt.Errorf("wipe stopped, nothing else was changed: %w", err)
			}
		}
	}
	more, err := Clean(ctx)
	steps = append(steps, more...)
	if err != nil {
		return steps, err
	}
	for _, d := range []string{filepath.Join(config.ProfilesRoot(), "claude"), filepath.Join(config.ProfilesRoot(), "codex"), config.ProfilesRoot()} {
		_ = os.Remove(d) // only when empty
	}
	if entries, err := os.ReadDir(config.Home()); err == nil {
		for _, e := range entries {
			if filepath.Ext(e.Name()) == ".lock" {
				_ = os.Remove(filepath.Join(config.Home(), e.Name()))
			}
		}
		if os.Remove(config.Home()) == nil {
			steps = append(steps, Step{What: "removed", Target: fsx.Tildify(config.Home())})
		}
	}
	return steps, nil
}
