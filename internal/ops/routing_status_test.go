package ops

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/testutil"
	"github.com/doguyilmaz/aims/internal/tool"
	"github.com/doguyilmaz/aims/internal/tools"
)

func TestStatusDoesNotShowAnOverriddenAutomaticCooldown(t *testing.T) {
	testutil.Sandbox(t)
	cfg, err := config.Update(func(c *config.Config) error {
		c.Tools["codex"].Profiles["work"] = &config.Profile{}
		c.Tools["codex"].Order = []string{"work"}
		c.Tools["codex"].Active = "work"
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	testutil.Write(t, filepath.Join(cfg.Dir("codex", "work"), "auth.json"), `{"tokens":{"refresh_token":"fixture"}}`)
	now := config.Now()
	err = config.UpdateStateFor(cfg, "codex", "work", func(ps *config.ProfileState) {
		ps.Until, ps.Reason, ps.Window = now.Add(time.Hour), "limit", "5h"
		ps.MarkedAt, ps.MarkedBy = now.Add(-time.Minute), "a session"
		ps.Usage = &config.Usage{At: now, Windows: []tool.Window{{Label: "5h", Percent: 41}}}
	})
	if err != nil {
		t.Fatal(err)
	}
	status := toolStatus(t.Context(), cfg, config.LoadStateFor(cfg), tools.Get("codex"))
	row := status.Profiles[0]
	if !row.Usable || !row.Until.IsZero() || row.Reason != "" || row.MarkedBy != "" {
		t.Fatalf("status disagrees with routing after reset: %+v", row)
	}
}
