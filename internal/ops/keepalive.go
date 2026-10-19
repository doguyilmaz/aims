package ops

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/profiles"
	"github.com/doguyilmaz/aims/internal/schedule"
	"github.com/doguyilmaz/aims/internal/tool"
	"github.com/doguyilmaz/aims/internal/tools"
)

// KeepAliveName names the scheduled `aims keepalive`.
const KeepAliveName = "keepalive"

// KeepAliveEvery is how often the scheduled run checks the logins.
const KeepAliveEvery = time.Hour

// KeepAliveJob is `aims keepalive` for the system scheduler. The scheduler
// starts it with a bare environment, so it carries the full path of aims and
// the PATH that finds claude and codex.
func KeepAliveJob() schedule.Job {
	self, err := exec.LookPath("aims")
	if err != nil {
		if self, err = os.Executable(); err == nil {
			self, _ = filepath.EvalSymlinks(self)
		}
	}
	if !filepath.IsAbs(self) {
		self, _ = filepath.Abs(self)
	}
	env := map[string]string{}
	keys := []string{"PATH", config.HomeVar}
	for _, a := range tools.All() {
		keys = append(keys, a.Layout().BinEnv)
	}
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			env[k] = v
		}
	}
	return schedule.Job{Name: KeepAliveName, Command: []string{self, "keepalive"}, Every: KeepAliveEvery, Env: env, Log: filepath.Join(config.Home(), "keepalive.log")}
}

// KeepAliveResult is what `aims keepalive` did for one profile.
type KeepAliveResult struct {
	Tool    tool.ID
	Profile string
	// Renewed: the access token had run out and the tool renewed it.
	Renewed bool
	Probe   tool.Probe
	// Expires is when the access token runs out (zero when unknown or when
	// the profile is not logged in).
	Expires time.Time
	// LoggedIn is nil when it cannot be told offline.
	LoggedIn *bool
}

// KeepAlive renews every login whose access token has run out or is about
// to: see profiles.KeepAlive.
func KeepAlive(ctx context.Context) ([]KeepAliveResult, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	var out []KeepAliveResult
	for _, a := range tools.All() {
		if profiles.Bin(a) == "" {
			continue
		}
		for _, n := range cfg.Tools[a.ID()].Order {
			p, acct, renewed := profiles.KeepAlive(ctx, cfg, a, n)
			out = append(out, KeepAliveResult{Tool: a.ID(), Profile: n, Renewed: renewed, Probe: p, Expires: acct.Expires, LoggedIn: acct.LoggedIn})
		}
	}
	return out, nil
}
