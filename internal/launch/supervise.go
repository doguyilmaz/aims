package launch

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/fsx"
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
	// settle is how long a session must sit idle before a move aims chose
	// restarts it, when no hook can hold the user's next prompt instead.
	settle = 2 * time.Minute
)

var (
	// recheck is how often an idle session, or one waiting for a reset,
	// looks whether it should move.
	recheck = 30 * time.Second
	// waitProbe is how often a session waiting for a reset asks a provider
	// that answers for free (tool.FreeProber) whether an account works.
	waitProbe = 5 * time.Minute
)

// supervise runs an interactive session that moves between profiles: to the
// next usable one when its profile hits a limit or loses its login, back to
// the preferred one once a turn ends and that works again, and, when every
// profile is limited, on to whichever resets first once it does.
func supervise(ctx context.Context, o Options, sv tool.Supervisor, bin, name string) (Result, error) {
	a := o.Tool
	runs := profiles.RunsDir()
	if err := os.MkdirAll(runs, 0o700); err != nil {
		return Result{Code: 1, Profile: name}, err
	}
	sweep(runs)
	root, err := os.MkdirTemp(runs, "run-")
	if err != nil {
		return Result{Code: 1, Profile: name}, err
	}
	defer os.RemoveAll(root)
	self, err := selfPath()
	if err != nil {
		return Result{Code: 1, Profile: name}, err
	}

	started := time.Now()
	args := o.Args
	// asked is the profile `aims move` asked for; held is the one it moved
	// to, which the conversation stays on while it works.
	var asked, held string
	// probed is when a session waiting for a reset last asked the provider.
	// Each time a conversation sent on sooner hits the limit again (early),
	// it asks half as often.
	var probed time.Time
	probeEvery, early := waitProbe, false
	for launch := 1; ; launch++ {
		cfg, err := config.LoadIn(o.Dir)
		if err != nil {
			return Result{Code: 1, Profile: name}, err
		}
		// Each start of the tool has its own directory, so a hook of the
		// one stopped cannot speak for the next.
		dir := filepath.Join(root, strconv.Itoa(launch))
		if err := os.Mkdir(dir, 0o700); err != nil {
			return Result{Code: 1, Profile: name}, err
		}
		var child *proc.Attached
		pid := func() int {
			if child == nil {
				return 0
			}
			return child.Pid()
		}
		watched, poll, err := sv.Watch(tool.Watch{
			Args: args, Self: self, RunDir: dir, Home: profiles.Home(cfg, a, name), Dir: o.Dir, Since: time.Now(), PID: pid,
			Env: map[string]string{profiles.RunVar: dir, profiles.SessionVar: string(a.ID()) + ":" + name},
		})
		if err != nil {
			return Result{Code: 2, Profile: name}, err
		}
		profiles.Track(profiles.OpenSession{Tool: a.ID(), Profile: name, Mode: profiles.Watched, Dir: workDir(o), Started: started})
		env := sessionEnv(o, cfg, name)
		env = append(env, profiles.RunVar+"="+dir)
		child, err = proc.StartAttached(bin, watched, env, o.Dir)
		if err != nil {
			return Result{Code: 127, Profile: name}, err
		}

		var session, transcript, mode string
		var wake *profiles.Pick
		var carry []tool.SessionEvent
		running := false // a turn is under way: no move that is not forced
		lastEvent := time.Now()
		// chosen is where aims would like the conversation to go without
		// being forced (back to the first account, or on before a limit):
		// it waits for a moment that loses nothing.
		var chosen, chosenWhy string
		var checked time.Time
		_, gated := sv.(tool.Hooker)
		ticker := time.NewTicker(pollEvery)
		var next, prompt, why, revive string
		var resume []string
	watch:
		for {
			select {
			case <-child.Done():
				ticker.Stop()
				return Result{Code: child.Code(), Profile: name}, nil
			case <-ticker.C:
			}
			if to := takeMove(dir); to != "" && validMove(a.ID(), name, to) {
				asked = to
			}
			evs := append(carry, poll()...)
			carry, next, prompt, why = nil, "", "", ""
			var heldFor, heldPrompt string
			for _, ev := range evs {
				lastEvent = time.Now()
				// Another conversation was opened in the tool: a continue
				// waiting for a reset was the last one's.
				if ev.Opened && ev.Session != "" && ev.Session != session {
					running, wake = false, nil
				}
				if ev.Session != "" {
					session = ev.Session
				}
				if ev.Transcript != "" {
					transcript = ev.Transcript
				}
				if ev.Mode != "" {
					mode = ev.Mode
				}
				switch {
				case ev.Lost:
					o.tell(Warn, "%s: another conversation in this folder could be this session's; it does not move at a limit", a.ID())
				case ev.Opened:
				case ev.Started && ev.HeldFor != "":
					heldFor, heldPrompt = ev.HeldFor, ev.Prompt
				case ev.Started:
					running, wake = true, nil
				case ev.Failure != tool.FailNone:
					running, held, chosen = false, "", ""
					next, prompt, why, wake = onFailure(ctx, o, name, ev)
					if early {
						probeEvery = min(2*probeEvery, time.Hour)
					}
					// A mark can be stale (a limit lifted early): the wait
					// below looks again right away.
					early, checked = false, time.Time{}
				default:
					running, wake = false, nil
					chosen, chosenWhy = onStop(ctx, o, name, held)
					checked = time.Now()
					probeEvery, early = waitProbe, false
				}
			}
			// While the session sits idle, accounts can come back (a reset,
			// aims clear): look again now and then.
			if !running && session != "" && wake == nil && next == "" && time.Since(checked) >= recheck {
				chosen, chosenWhy = onStop(ctx, o, name, held)
				checked = time.Now()
			}
			// Waiting for a reset, an account can come back sooner (aims clear,
			// a limit lifted early): look again now and then.
			sooner, told := false, why == ""
			if wake != nil && next == "" && time.Since(checked) >= recheck {
				live := time.Since(probed) >= probeEvery
				if live {
					probed = time.Now()
				}
				switch pick := onWait(ctx, o, name, live); {
				case pick.Name != "":
					if why == "" {
						why = pick.Name + " works again"
					}
					next, prompt, sooner = pick.Name, continuePrompt, true
					why += "; continuing this conversation on " + next
				case pick.Soonest != "" && (pick.Soonest != wake.Soonest || waitAt(pick) != waitAt(*wake)):
					wake, told = &pick, false
				}
				checked = time.Now()
			}
			if !told && next == "" && wake != nil {
				tellWait(o, *wake)
			}
			switch {
			case next != "":
				// A turn cut short goes on elsewhere right away.
			case heldFor != "":
				// The hook kept the user's prompt from being sent: it goes
				// with the conversation, or stays here if that move is off.
				next, prompt, why = heldFor, heldPrompt, chosenWhy
				if !validMove(a.ID(), name, heldFor) {
					next, why = name, "sending your message"
				}
				if why == "" {
					why = "this conversation moves to " + heldFor
				}
			case asked != "" && !running && session != "":
				if asked != name {
					next, why = asked, "moving this conversation to "+asked+", as asked"
				}
				held, asked, chosen = asked, "", ""
			case wake != nil && !config.Now().Before(wake.ReadyAt.Add(wakeAfter)):
				next, prompt = wake.Soonest, continuePrompt
				why = wake.Soonest + "'s limit has reset; continuing this conversation"
				if next != name {
					why += " on it"
				}
			case chosen != "" && !running && !gated && time.Since(lastEvent) >= settle:
				next, why = chosen, chosenWhy
			}
			// With hooks, a move aims chose waits for the user's next
			// prompt, which the hook holds and the move takes along.
			hold(dir, gated && chosen != "" && !running && next == "", chosen)
			if next == "" || session == "" || (next == name && prompt == "") {
				continue
			}
			if prompt == "" && heldFor == "" {
				// The user may have sent the next prompt meanwhile.
				if carry = poll(); len(carry) > 0 {
					continue
				}
			}
			if !tool.ValidSessionID(session) {
				continue // not one aims would put in a command line
			}
			// A conversation with nothing saved yet cannot be resumed: start
			// a new one instead.
			id := session
			if transcript != "" && fsx.Stat(transcript) == nil {
				id = ""
				if heldFor == "" {
					prompt = ""
				}
			}
			var err error
			if resume, err = sv.ResumeArgs(bin, o.Args, tool.Resume{ID: id, Prompt: prompt, Transcript: transcript, Mode: mode}); err != nil {
				wake = nil
				continue // stay: a resume that cannot be built would lose the conversation
			}
			// A pending move can outlive a change to sharing or a tool replacing
			// a link. Recheck before stopping the process that owns the conversation.
			if next != name && !validMove(a.ID(), name, next) {
				o.tell(Warn, "%s: history is no longer shared with %s; staying on %s (aims doctor)", a.ID(), next, name)
				wake, chosen = nil, ""
				continue
			}
			select {
			case <-child.Done():
				ticker.Stop()
				return Result{Code: child.Code(), Profile: name}, nil
			default:
			}
			// A limit stopped the conversation: what it was set to do goes on too.
			if prompt == continuePrompt {
				revive = id
			}
			early = sooner
			break watch
		}
		ticker.Stop()
		child.Stop(stopGrace)
		o.tell(Info, "%s: %s", a.ID(), why)
		if r, ok := sv.(tool.Reviver); ok && revive != "" {
			if err := r.Revive(ctx, profiles.Home(cfg, a, next), revive); err != nil {
				o.tell(Warn, "%s: this conversation's goal stays paused (%v); /goal resume picks it up", a.ID(), err)
			}
		}
		args, name = resume, next
	}
}

// onFailure handles a turn that ended on an error: it marks the profile and
// names the profile to continue on, or, when every one is limited, what to
// wait for and why.
func onFailure(ctx context.Context, o Options, name string, ev tool.SessionEvent) (next, prompt, why string, wake *profiles.Pick) {
	a := o.Tool
	cfg, err := config.LoadIn(o.Dir)
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
	apart := profiles.Apart(cfg, a.ID(), name)
	pick := profiles.Choose(ctx, cfg, config.LoadState(), a, preferred, apart)
	if pick.Name == "" && len(apart) > 0 {
		o.tell(Warn, "%s: %s cannot receive this conversation because history is not fully shared (aims doctor)", a.ID(), profiles.Names(apart, ", "))
	}
	switch {
	case pick.Name != "" && pick.Name != name:
		return pick.Name, continuePrompt, name + " hit " + failureText(f) + "; continuing this conversation on " + pick.Name, nil
	case pick.Name == "" && pick.Soonest != "":
		return "", "", name + " hit " + failureText(f), &pick
	}
	return "", "", "", nil
}

// onWait looks, while every account is limited, for one that works before
// the soonest reset: its mark cleared (aims clear, a live check elsewhere)
// or, with live, the provider saying so now.
func onWait(ctx context.Context, o Options, name string, live bool) profiles.Pick {
	a := o.Tool
	cfg, err := config.LoadIn(o.Dir)
	if err != nil {
		return profiles.Pick{}
	}
	apart := profiles.Apart(cfg, a.ID(), name)
	if live {
		liveCheck(ctx, cfg, a, apart)
	}
	preferred, _ := cfg.Preferred(a.ID(), "")
	return profiles.Choose(ctx, cfg, config.LoadState(), a, preferred, apart)
}

// liveCheck asks the provider about every account but those in exclude, and
// records what it says, when asking costs nothing (tool.FreeProber). It
// reports whether it asked.
func liveCheck(ctx context.Context, cfg *config.Config, a tool.Adapter, exclude []string) bool {
	if _, free := a.(tool.FreeProber); !free {
		return false
	}
	var wg sync.WaitGroup
	for _, n := range cfg.Tools[a.ID()].Order {
		if !slices.Contains(exclude, n) {
			wg.Go(func() { profiles.Live(ctx, cfg, a, n) })
		}
	}
	wg.Wait()
	return true
}

func tellWait(o Options, pick profiles.Pick) {
	o.tell(Warn, "%s: every account is limited; this conversation continues on %s at %s", o.Tool.ID(), pick.Soonest, waitAt(pick))
}

// waitAt is when a conversation waiting for pick goes on: the time, with the
// date when that is not today.
func waitAt(pick profiles.Pick) string {
	t, now := pick.ReadyAt.Add(wakeAfter).Local(), config.Now().Local()
	if t.YearDay() == now.YearDay() && t.Year() == now.Year() {
		return t.Format("15:04")
	}
	return t.Format("Jan 2 15:04")
}

// onStop names the profile a conversation should move to after a turn that
// went well: the preferred one (or the one held, which `aims move` chose)
// once it works again, or the next usable one when this one is about to
// run out.
func onStop(ctx context.Context, o Options, name, held string) (next, why string) {
	cfg, err := config.LoadIn(o.Dir)
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

// selfPath is how the session's hooks start aims: through PATH's link when
// that is this aims, so an upgrade that removes this version's folder does
// not break the hooks of a running session.
func selfPath() (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", err
	}
	if p, err := exec.LookPath("aims"); err == nil && filepath.IsAbs(p) && fsx.Real(p) == fsx.Real(self) {
		return p, nil
	}
	return self, nil
}

// validMove reports whether the conversation on name can move to to: a
// profile that exists and shares its history. The request comes from a
// file, so it is checked again here.
func validMove(id tool.ID, name, to string) bool {
	cfg, err := config.Load()
	return err == nil && config.ValidName(to) && cfg.Tools[id].Profiles[to] != nil && !slices.Contains(profiles.Apart(cfg, id, name), to)
}

// sweep removes the directories of runs that ended without cleaning up
// (aims was killed) long ago.
func sweep(runs string) {
	entries, _ := os.ReadDir(runs)
	for _, e := range entries {
		if info, err := e.Info(); err == nil && time.Since(info.ModTime()) > 30*24*time.Hour {
			_ = os.RemoveAll(filepath.Join(runs, e.Name()))
		}
	}
}

// hold leaves (on) or removes the request that makes the session's hook
// keep the user's next prompt for a move to profile.
func hold(dir string, on bool, profile string) {
	path := filepath.Join(dir, tool.HoldFile)
	if on {
		_ = fsx.WriteFile(path, []byte(profile+"\n"), 0o600)
		return
	}
	_ = os.Remove(path)
}
