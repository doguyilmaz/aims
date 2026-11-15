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
	var steps []Step
	err := profileOperation(func() error { var err error; steps, err = clean(ctx); return err })
	return steps, err
}

func clean(ctx context.Context) ([]Step, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	for id, tool := range cfg.Tools {
		for name, p := range tool.Profiles {
			if p.Purge != nil {
				return nil, fmt.Errorf("%s/%s has a pending purge; finish it or forget only its label before clean", id, name)
			}
		}
	}
	steps, err := uninstall(ctx, Integrations{Skill: true, MCP: true, StatusLine: true, Shell: shell.Detect(), KeepAlive: true})
	if err != nil {
		return steps, err
	}
	for _, step := range steps {
		if step.Err != nil {
			return steps, fmt.Errorf("some integrations could not be removed; aims configuration and marks were kept for retry")
		}
	}
	err = fsx.WithLock(filepath.Join(config.Home(), "config.json"), func() error {
		return fsx.WithLock(filepath.Join(config.Home(), "state.json"), func() error {
			for _, f := range []string{"config.json", "state.json"} {
				p := filepath.Join(config.Home(), f)
				switch err := os.Remove(p); {
				case err == nil:
					steps = append(steps, Step{What: "forgot", Target: fsx.Tildify(p)})
				case !errors.Is(err, os.ErrNotExist):
					steps = append(steps, Step{What: "forget", Target: fsx.Tildify(p), Err: err})
					return err
				}
			}
			return nil
		})
	})
	if err != nil {
		return steps, err
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
	var steps []Step
	err := profileOperation(func() error { var err error; steps, err = wipe(ctx, force); return err })
	if err == nil && os.Remove(config.Home()) == nil {
		steps = append(steps, Step{What: "removed", Target: fsx.Tildify(config.Home())})
	}
	return steps, err
}

func wipe(ctx context.Context, force bool) ([]Step, error) {
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
			res, err := removeProfile(ctx, a, n, purge, force)
			what := n + " logged out and deleted"
			if !purge {
				what = n + " forgotten, its folder kept"
			}
			if err != nil {
				what = n + " removal did not complete"
			}
			steps = append(steps, Step{Tool: a.ID(), What: what, Target: fsx.Tildify(res.Dir), Err: err})
			if err != nil {
				return steps, fmt.Errorf("wipe stopped, nothing else was changed: %w", err)
			}
		}
	}
	more, err := clean(ctx)
	steps = append(steps, more...)
	if err != nil {
		return steps, err
	}
	for _, d := range []string{filepath.Join(config.ProfilesRoot(), "claude"), filepath.Join(config.ProfilesRoot(), "codex"), config.ProfilesRoot()} {
		_ = os.Remove(d) // only when empty
	}

	return steps, nil
}
