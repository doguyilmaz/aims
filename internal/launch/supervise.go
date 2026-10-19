package launch

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/proc"
	"github.com/doguyilmaz/aims/internal/profiles"
	"github.com/doguyilmaz/aims/internal/tool"
)

// continuePrompt is sent when a conversation moves on after a limit cut a
// turn short.
const continuePrompt = "continue"

const (
	pollEvery = time.Second
	stopGrace = 5 * time.Second
	// wakeAfter is how long after a limit's reset a waiting conversation is
	// continued: a little later than the tool would itself, so its own
	// automatic continue wins.
	wakeAfter = time.Minute
	// busyAgain is how long an account rests after a second rate limit
	// within that time.
	busyAgain = 15 * time.Minute
)

// supervise runs an interactive session that moves between profiles: to the
// next usable one when its profile hits a limit or loses its login, back to
// the preferred one once a turn ends and that works again, and, when every
// profile is limited, on to whichever resets first once it does.
func supervise(ctx context.Context, o Options, sv tool.Supervisor, bin, name string) (Result, error) {
	a := o.Tool
	dir, err := os.MkdirTemp("", "aims-run-")
	if err != nil {
		return Result{Code: 1, Profile: name}, err
	}
	defer os.RemoveAll(dir)
	self, err := os.Executable()
	if err != nil {
		return Result{Code: 1, Profile: name}, err
	}

	args := o.Args
	// asked is the profile `aims move` asked for; held is the one it moved
	// to, which the conversation stays on while it works.
	var asked, held string
	for {
		cfg, err := config.Load()
		if err != nil {
			return Result{Code: 1, Profile: name}, err
		}
		watched, poll, err := sv.Watch(tool.Watch{Args: args, Self: self, RunDir: dir, Home: profiles.Home(cfg, a, name), Dir: o.Dir, Since: time.Now()})
		if err != nil {
			return Result{Code: 2, Profile: name}, err
		}
		env := sessionEnv(o, cfg, name)
		env = append(env, profiles.RunVar+"="+dir)
		child, err := proc.StartAttached(bin, watched, env, o.Dir)
		if err != nil {
			return Result{Code: 127, Profile: name}, err
		}

		var session string
		var wake *profiles.Pick
		var carry []tool.SessionEvent
		running := false // a turn is under way: no move that is not forced
		ticker := time.NewTicker(pollEvery)
		var next, prompt, why string
		var resume []string
	watch:
		for {
			select {
			case <-child.Done():
				ticker.Stop()
				return Result{Code: child.Code(), Profile: name}, nil
			case <-ticker.C:
			}
			if to := takeMove(dir); to != "" {
				asked = to
			}
			evs := append(carry, poll()...)
			carry, next, prompt = nil, "", ""
			for _, ev := range evs {
				if ev.Session != "" {
					session = ev.Session
				}
				switch {
				case ev.Opened:
				case ev.Started:
					running, wake, next = true, nil, ""
				case ev.Failure != tool.FailNone:
					running, held = false, ""
					next, prompt, why, wake = onFailure(ctx, o, name, ev)
				default:
					running, wake = false, nil
					next, why = onStop(ctx, o, name, held)
					prompt = ""
				}
			}
			// A move asked for wins over one aims chose, but not over a turn
			// cut short, which must go on.
			if asked != "" && !running && session != "" && prompt == "" {
				next, why = "", ""
				if asked != name {
					next, why = asked, "moving this conversation to "+asked+", as asked"
				}
				held, asked = asked, ""
			}
			if wake != nil && !config.Now().Before(wake.ReadyAt.Add(wakeAfter)) {
				next, prompt = wake.Soonest, continuePrompt
				why = wake.Soonest + "'s limit has reset; continuing this conversation"
				if next != name {
					why += " on it"
				}
			}
			if next == "" || session == "" || (running && prompt == "") {
				continue
			}
			if prompt == "" {
				// The user may have sent the next prompt since the turn ended.
				if carry = poll(); len(carry) > 0 {
					continue
				}
			}
			var err error
			if resume, err = sv.ResumeArgs(bin, o.Args, session, prompt); err != nil {
				wake = nil
				continue // stay: a resume that cannot be built would lose the conversation
			}
			select {
			case <-child.Done():
				ticker.Stop()
				return Result{Code: child.Code(), Profile: name}, nil
			default:
			}
			break watch
		}
		ticker.Stop()
		child.Stop(stopGrace)
		o.say(Info, "%s: %s", a.ID(), why)
		args, name = resume, next
	}
}

// onFailure handles a turn that ended on an error: it marks the profile and
// names the profile to continue on, or what to wait for when every one is
// limited.
func onFailure(ctx context.Context, o Options, name string, ev tool.SessionEvent) (next, prompt, why string, wake *profiles.Pick) {
	a := o.Tool
	cfg, err := config.Load()
	if err != nil {
		return "", "", "", nil
	}
	f := ev.Failure
	const by = "a session"
	switch f {
	case tool.FailLogin:
		_ = profiles.MarkNeedsLogin(a.ID(), name, by, ev.Text)
	case tool.FailLimit:
		var windows []tool.Window
		if u := config.LoadState().Get(a.ID(), name).Usage; u != nil {
			windows = u.Windows
		}
		_ = profiles.MarkLimited(cfg, a.ID(), name, profiles.Limit{
			ResetsAt: tool.ResetTime(ev.Text, config.Now()), Window: profiles.LimitWindow(cfg, ev.Text, windows),
			By: by, Detail: ev.Text,
		})
	case tool.FailBusy:
		// A rate limit that comes back soon after the last is not passing:
		// rest longer, so the conversation does not bounce between accounts.
		rest := busyFor
		if ps := config.LoadState().Get(a.ID(), name); ps.Reason == "busy" && config.Now().Sub(ps.MarkedAt) < busyAgain {
			rest = busyAgain
		}
		_ = profiles.MarkLimited(cfg, a.ID(), name, profiles.Limit{Reason: "busy", For: rest, By: by, Detail: ev.Text})
	}
	preferred, _ := cfg.Preferred(a.ID(), "")
	pick := profiles.Choose(ctx, cfg, config.LoadState(), a, preferred, profiles.Apart(cfg, a.ID(), name))
	switch {
	case pick.Name != "" && pick.Name != name:
		return pick.Name, continuePrompt, name + " hit " + failureText(f) + "; continuing this conversation on " + pick.Name, nil
	case pick.Name == "" && pick.Soonest != "":
		return "", "", "", &pick
	}
	return "", "", "", nil
}

// onStop names the profile a conversation should move to after a turn that
// went well: the preferred one (or the one held, which `aims move` chose)
// once it works again, or the next usable one when this one is about to
// run out.
func onStop(ctx context.Context, o Options, name, held string) (next, why string) {
	cfg, err := config.Load()
	if err != nil {
		return "", ""
	}
	preferred, _ := cfg.Preferred(o.Tool.ID(), "")
	if held != "" {
		preferred = held
	}
	pick := profiles.Choose(ctx, cfg, config.LoadState(), o.Tool, preferred, profiles.Apart(cfg, o.Tool.ID(), name))
	switch {
	case pick.Name == "" || pick.Name == name:
		return "", ""
	case slices.ContainsFunc(pick.Skipped, func(e profiles.Evaluation) bool { return e.Name == name }):
		return pick.Name, name + " is about to run out; this conversation moves to " + pick.Name
	}
	return pick.Name, pick.Name + " works again; this conversation goes back to it"
}

// MoveFile is the request `aims move` leaves in a run's directory.
const MoveFile = "move"

// takeMove returns and removes the profile `aims move` asked for.
func takeMove(dir string) string {
	path := filepath.Join(dir, MoveFile)
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	_ = os.Remove(path)
	return strings.TrimSpace(string(b))
}
