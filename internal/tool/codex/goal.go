package codex

import (
	"context"
	"errors"
	"time"

	"github.com/tidwall/gjson"

	"github.com/doguyilmaz/aims/internal/tool"
)

var _ tool.Reviver = (*Adapter)(nil)

// Revive makes the conversation's goal active again when a usage limit
// stopped it: resumed, codex would open on "Resume paused goal?" and wait
// for an answer. A goal paused by the user or stalled stays as it is.
func (*Adapter) Revive(ctx context.Context, h tool.Home, id string) error {
	if h.Bin == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	res, err := rpc(ctx, h, []call{{"thread/goal/get", map[string]any{"threadId": id}}})
	got := res["thread/goal/get"]
	if got == nil {
		return err
	}
	// An error here is a codex without goals, or with them turned off.
	if got.Error != nil || gjson.GetBytes(got.Result, "goal.status").Str != "usageLimited" {
		return nil
	}
	res, err = rpc(ctx, h, []call{{"thread/goal/set", map[string]any{"threadId": id, "status": "active"}}})
	if set := res["thread/goal/set"]; set != nil && set.Error != nil {
		return errors.New(set.Error.Message)
	}
	return err
}
