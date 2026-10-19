package launch

import (
	"context"
	"os"
	"slices"
	"time"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/proc"
	"github.com/doguyilmaz/aims/internal/profiles"
	"github.com/doguyilmaz/aims/internal/tool"
)

// RunEnv points the hooks of a supervised session at its run directory.
const RunEnv = "AIMS_RUN"

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
		env = append(env, RunEnv+"="+dir)
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
			evs := append(carry, poll()...)
			carry, next = nil, ""
			for _, ev := range evs {
				if ev.Session != "" {
					session = ev.Session
				}
				switch {
				case ev.Started:
					running, wake, next = true, nil, ""
				case ev.Failure != tool.FailNone:
					running = false
					next, prompt, why, wake = onFailure(ctx, o, name, ev)
				default:
					running, wake = false, nil
					next, why = onStop(ctx, o, name)
					prompt = ""
				}
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

// apart lists the profiles a conversation on name cannot move to: those
// that do not share history with it, so would not find the conversation.
func apart(cfg *config.Config, id tool.ID, name string) []string {
	t := cfg.Tools[id]
	shared := t.Profiles[name] != nil && t.Profiles[name].ShareMode() == config.ShareAll
	var out []string
	for _, n := range t.Order {
		if n != name && (!shared || t.Profiles[n].ShareMode() != config.ShareAll) {
			out = append(out, n)
		}
	}
	return out
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
		_ = profiles.MarkLimited(cfg, a.ID(), name, profiles.Limit{Reason: "busy", For: busyFor, By: by, Detail: ev.Text})
	}
	preferred, _ := cfg.Preferred(a.ID(), "")
	pick := profiles.Choose(ctx, cfg, config.LoadState(), a, preferred, apart(cfg, a.ID(), name))
	switch {
	case pick.Name != "" && pick.Name != name:
		return pick.Name, continuePrompt, name + " hit " + failureText(f) + "; continuing this conversation on " + pick.Name, nil
	case pick.Name == "" && pick.Soonest != "":
		return "", "", "", &pick
	}
	return "", "", "", nil
}

// onStop names the profile a conversation should move to after a turn that
// went well: the preferred one once it works again, or the next usable one
// when this one is about to run out.
func onStop(ctx context.Context, o Options, name string) (next, why string) {
	cfg, err := config.Load()
	if err != nil {
		return "", ""
	}
	preferred, _ := cfg.Preferred(o.Tool.ID(), "")
	pick := profiles.Choose(ctx, cfg, config.LoadState(), o.Tool, preferred, apart(cfg, o.Tool.ID(), name))
	switch {
	case pick.Name == "" || pick.Name == name:
		return "", ""
	case slices.ContainsFunc(pick.Skipped, func(e profiles.Evaluation) bool { return e.Name == name }):
		return pick.Name, name + " is about to run out; this conversation moves to " + pick.Name
	}
	return pick.Name, pick.Name + " works again; this conversation goes back to it"
}
