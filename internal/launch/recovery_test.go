package launch

import (
	"strings"
	"testing"
	"time"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/profiles"
	"github.com/doguyilmaz/aims/internal/testutil"
	"github.com/doguyilmaz/aims/internal/tool"
)

func TestSuccessfulSupervisedTurnClearsPreexistingLoginMark(t *testing.T) {
	o := setup(t)
	testutil.Fakes(t)
	t.Setenv("FAKE_LINGER", "3s")
	cfg, _ := config.Load()
	for _, n := range cfg.Tools[o.Tool.ID()].Order {
		profiles.Sync(cfg, o.Tool, n, false)
	}
	if err := profiles.MarkNeedsLogin(o.Tool.ID(), "a", "a session", "old login error", cfg); err != nil {
		t.Fatal(err)
	}
	o.Dir = t.TempDir()
	res, err := supervise(t.Context(), o, o.Tool.(tool.Supervisor), profiles.Bin(o.Tool), "a")
	if err != nil || res.Code != 0 {
		t.Fatalf("successful fixture failed: %+v %v", res, err)
	}
	if ps := config.LoadState().Get(o.Tool.ID(), "a"); ps.NeedsLogin {
		t.Fatalf("successful turn left login mark: %+v", ps)
	}
}

type paidProbeAdapter struct{ tool.Adapter }

func TestChooseExplainsMixedLoginAndQuotaFailures(t *testing.T) {
	o := setup(t)
	o.Tool = paidProbeAdapter{o.Tool}
	cfg, _ := config.Load()
	_ = profiles.MarkNeedsLogin(o.Tool.ID(), "a", "a session", "old error", cfg)
	limit(t, "b", time.Hour)
	limit(t, "c", 2*time.Hour)
	var said []string
	o.Notify = func(_ Level, text string) { said = append(said, text) }
	if got := choose(t.Context(), o, cfg, "a"); got != "b" {
		t.Fatalf("expected earliest reset fallback, got %s", got)
	}
	text := strings.Join(said, "\n")
	if strings.Contains(text, "every account is limited") || !strings.Contains(text, "login expired") || !strings.Contains(text, "aims login codex a") {
		t.Fatalf("mixed causes were hidden: %s", text)
	}
}
