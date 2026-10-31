package ops

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/meter"
	"github.com/doguyilmaz/aims/internal/profiles"
	"github.com/doguyilmaz/aims/internal/schedule"
	"github.com/doguyilmaz/aims/internal/tool"
	"github.com/doguyilmaz/aims/internal/tools"
)

// KeepAliveName names the scheduled `aims keepalive`.
const KeepAliveName = "keepalive"

// AppID is the bundle identifier of aims.app (packaging/Info.plist).
const AppID = "com.doguyilmaz.aims"

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
	// Run from aims.app itself, not a link to it, so Login Items shows the
	// app's name and icon.
	app := ""
	if real, err := filepath.EvalSymlinks(self); err == nil && strings.HasSuffix(filepath.ToSlash(real), "/aims.app/Contents/MacOS/aims") {
		self, app = real, AppID
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
	return schedule.Job{Name: KeepAliveName, Command: []string{self, "keepalive"}, Every: KeepAliveEvery, Env: env, Log: filepath.Join(config.Home(), "keepalive.log"), App: app}
}

// keepAliveWanted marks that the person turned the keepalive on. Homebrew
// removes the job on every upgrade of the cask, so this is how the next aims
// tells it was on.
func keepAliveWanted() string { return filepath.Join(config.Home(), "keepalive-wanted") }

// keepAliveFailure holds the last failure to set the keepalive up again.
func keepAliveFailure() string { return filepath.Join(config.Home(), "keepalive-failure") }

// InstallKeepAlive turns the keepalive on.
func InstallKeepAlive() (string, error) {
	path, err := schedule.Install(KeepAliveJob())
	if err == nil {
		err = fsx.WriteFile(keepAliveWanted(), nil, 0o600)
	}
	return path, err
}

// UninstallKeepAlive turns it off; found is false when it was not on.
func UninstallKeepAlive() (path string, found bool, err error) {
	if err := os.Remove(keepAliveWanted()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", false, err
	}
	return schedule.Uninstall(KeepAliveName)
}

// HealKeepAlive sets the keepalive up again when the person turned it on and
// it is gone or out of date (see schedule.Heal), which only the cask does,
// on macOS. It returns a failure only
// when it differs from the last one, so a lasting one is told once.
func HealKeepAlive() error {
	if runtime.GOOS != "darwin" {
		return nil
	}
	if fsx.Stat(keepAliveWanted()) == nil {
		// Turned on by an aims from before the marker.
		if _, on := schedule.Installed(KeepAliveName); !on {
			return nil
		}
		_ = fsx.WriteFile(keepAliveWanted(), nil, 0o600)
	}
	_, err := schedule.Heal(KeepAliveJob())
	if err == nil {
		_ = os.Remove(keepAliveFailure())
		return nil
	}
	if last, rerr := os.ReadFile(keepAliveFailure()); rerr == nil && string(last) == err.Error() {
		return nil
	}
	_ = fsx.WriteFile(keepAliveFailure(), []byte(err.Error()), 0o600)
	return err
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
	meter.Refresh(ctx)
	return out, nil
}
