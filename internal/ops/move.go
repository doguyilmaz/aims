package ops

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/launch"
	"github.com/doguyilmaz/aims/internal/profiles"
	"github.com/doguyilmaz/aims/internal/tool"
	"github.com/doguyilmaz/aims/internal/tools"
)

// ErrNotMoving means the process is not inside a session aims moves
// between accounts.
var ErrNotMoving = errors.New("this session was not started with `aims claude` or `aims codex` (without @profile), so aims cannot move it; " +
	"aims use changes the account of new sessions")

// MoveResult says where `aims move` sends the conversation.
type MoveResult struct {
	Tool tool.ID `json:"tool"`
	From string  `json:"from"`
	To   string  `json:"to"`
}

// Move asks the session this process runs in to continue its conversation
// on profile to (the next usable one when ""): right away when the session
// is idle, after the current turn otherwise. The conversation then stays on
// to while it works.
func Move(ctx context.Context, to string) (MoveResult, error) {
	dir := os.Getenv(profiles.RunVar)
	id, from, ok := profiles.Session()
	if dir == "" || !ok || os.Getenv(profiles.NestedVar) != "" || fsx.Stat(dir) == nil || !strings.HasPrefix(fsx.Real(dir), fsx.Real(profiles.RunsDir())+string(filepath.Separator)) {
		return MoveResult{}, ErrNotMoving
	}
	a, known := tools.Parse(string(id))
	if !known {
		return MoveResult{}, ErrNotMoving
	}
	cfg, err := config.Load()
	if err != nil {
		return MoveResult{}, err
	}
	res := MoveResult{Tool: id, From: from, To: to}
	apart := profiles.Apart(cfg, id, from)
	if to == "" {
		pick := profiles.Choose(ctx, cfg, config.LoadState(), a, "", append(apart, from))
		if pick.Name == "" && profiles.AskFree(ctx, cfg, a, others(cfg, id, append(apart, from))) {
			pick = profiles.Choose(ctx, cfg, config.LoadState(), a, "", append(apart, from))
		}
		if pick.Name == "" {
			var why []string
			for _, s := range pick.Skipped {
				why = append(why, s.Name+": "+strings.Join(s.Reasons, ", "))
			}
			return res, fmt.Errorf("no other account can take this conversation (%s)", strings.Join(why, "; "))
		}
		res.To = pick.Name
	}
	switch {
	case cfg.Tools[id].Profiles[res.To] == nil:
		return res, fmt.Errorf("no %s profile %q (see: aims status)", id, res.To)
	case slices.Contains(apart, res.To):
		return res, fmt.Errorf("%s and %s do not fully share history; check sharing settings and run `aims doctor` before moving this conversation", from, res.To)
	}
	ev := profiles.Evaluate(ctx, cfg, config.LoadState(), a, res.To)
	if !ev.Usable && res.To != from && profiles.AskFree(ctx, cfg, a, []string{res.To}) {
		ev = profiles.Evaluate(ctx, cfg, config.LoadState(), a, res.To)
	}
	if !ev.Usable && res.To != from {
		return res, fmt.Errorf("%s cannot be used now: %s", res.To, strings.Join(ev.Reasons, ", "))
	}
	return res, fsx.WriteFile(filepath.Join(dir, launch.MoveFile), []byte(res.To+"\n"), 0o600)
}

// others is the tool's accounts in order, but those in exclude.
func others(cfg *config.Config, id tool.ID, exclude []string) []string {
	var out []string
	for _, n := range cfg.Tools[id].Order {
		if !slices.Contains(exclude, n) {
			out = append(out, n)
		}
	}
	return out
}
