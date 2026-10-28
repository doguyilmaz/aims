// Package launch runs a tool as a profile, choosing another profile when the
// preferred one is limited or logged out.
package launch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/links"
	"github.com/doguyilmaz/aims/internal/proc"
	"github.com/doguyilmaz/aims/internal/profiles"
	"github.com/doguyilmaz/aims/internal/tool"
)

// Options configures a launch.
type Options struct {
	Tool tool.Adapter
	// Profile pins the profile and turns automatic failover off.
	Profile string
	Args    []string
	// Headless forces captured mode (MCP runs).
	Headless bool
	// Stdin for headless runs; nil reads aims' own stdin when it is a pipe.
	Stdin   []byte
	NoStdin bool
	Stdout  io.Writer
	Stderr  io.Writer
	Dir     string
	Timeout time.Duration
	// StripEnv drops more variables from the child (MCP: the parent session's markers).
	StripEnv []string
	// ExtraEnv adds KEY=VALUE pairs to the child's environment.
	ExtraEnv []string
	// Notify receives progress messages ("skipping work: limited for 2h").
	Notify func(level Level, msg string)
}

// Level grades a Notify message.
type Level int

const (
	Info Level = iota
	Warn
	// Hint is a follow-up line: the command that undoes or fixes something.
	Hint
)

// Result describes a finished launch.
type Result struct {
	Code    int
	Profile string
	Failure tool.Failure
	// FinalText and ErrorText come from the analyzer of headless runs.
	FinalText string
	ErrorText string
}

func (o *Options) say(l Level, format string, args ...any) {
	if o.Notify != nil {
		o.Notify(l, fmt.Sprintf(format, args...))
	}
}

// tell is say for a supervised session, whose tool may cover what aims
// prints: the terminal shows it as a desktop notification too.
func (o *Options) tell(l Level, format string, args ...any) {
	o.say(l, format, args...)
	proc.Notify(fmt.Sprintf(format, args...))
}

// startPause is how long a session started on another account than the
// preferred one waits, so what aims said about it can be read before a tool
// without a status line (tool.StatusLiner) covers the screen.
const startPause = 2 * time.Second

func (o *Options) pause() {
	if _, line := o.Tool.(tool.StatusLiner); line {
		return
	}
	if f, ok := o.Stderr.(*os.File); !ok || !term.IsTerminal(int(f.Fd())) {
		return
	}
	o.say(Hint, "%s starts in %ds", o.Tool.Layout().Bin, int(startPause.Seconds()))
	time.Sleep(startPause)
}

// busyFor is how long a profile rests after a short-lived rate limit.
const busyFor = 2 * time.Minute

// ErrNotInstalled is returned when the tool's binary is not on PATH.
var ErrNotInstalled = errors.New("not installed")

// Run launches the tool.
func Run(ctx context.Context, o Options) (Result, error) {
	a := o.Tool
	if o.Stdout == nil {
		o.Stdout = os.Stdout
	}
	if o.Stderr == nil {
		o.Stderr = os.Stderr
	}
	bin := profiles.Bin(a)
	if bin == "" {
		return Result{Code: 127}, fmt.Errorf("%s is %w (or set %s)", a.Layout().Bin, ErrNotInstalled, a.Layout().BinEnv)
	}
	cfg, err := config.LoadIn(o.Dir)
	if err != nil {
		return Result{Code: 1}, err
	}
	t := cfg.Tools[a.ID()]
	headless := o.Headless || a.IsHeadless(o.Args)

	if len(t.Profiles) == 0 {
		if o.Profile != "" {
			return Result{Code: 2}, fmt.Errorf("no %s profile %q (see: aims status)", a.ID(), o.Profile)
		}
		// Nothing configured: behave exactly like the plain tool.
		env := append(strip(os.Environ(), o.StripEnv), o.ExtraEnv...)
		if !headless {
			code, err := proc.RunAttached(bin, o.Args, env, o.Dir)
			return Result{Code: code}, err
		}
		return runOnce(o, bin, env, readInput(o))
	}

	preferred, err := cfg.Preferred(a.ID(), o.Profile)
	if err != nil {
		return Result{Code: 2}, err
	}
	auto := cfg.Failover.Auto && o.Profile == ""
	name := preferred
	if auto {
		name = choose(ctx, o, cfg, preferred)
	} else if ev := profiles.Evaluate(ctx, cfg, config.LoadState(), a, name); !ev.Usable {
		o.say(Warn, "%s profile %q may not work: %s", a.ID(), name, strings.Join(ev.Reasons, ", "))
	}

	var input []byte
	if headless {
		input = readInput(o)
	}
	var tried []string
	for {
		tried = append(tried, name)
		if !headless {
			// A session, not `claude mcp list` or `codex login`: only a
			// session is explained and listed.
			sv, ok := a.(tool.Supervisor)
			session := ok && sv.Supervises(o.Args)
			mode := profiles.Unwatched
			switch {
			case o.Profile != "":
				mode = profiles.Pinned
			case session && auto:
				mode = profiles.Watched
			}
			o.ExtraEnv = append(o.ExtraEnv, profiles.ModeVar+"="+string(mode))
			if session {
				explain(o, cfg, preferred, name, mode)
				defer profiles.Untrack()
				if name != preferred {
					o.pause()
				}
			}
			if mode == profiles.Watched {
				return supervise(ctx, o, sv, bin, name)
			}
			if session {
				profiles.Track(profiles.OpenSession{Tool: a.ID(), Profile: name, Mode: mode, Dir: workDir(o), Started: time.Now()})
			}
			code, err := proc.RunAttached(bin, o.Args, sessionEnv(o, cfg, name), o.Dir)
			return Result{Code: code, Profile: name}, err
		}
		env := sessionEnv(o, cfg, name)

		res, retry, err := attempt(ctx, o, cfg, bin, env, input, name, auto, tried)
		if err != nil || retry.Name == "" {
			return res, err
		}
		o.say(Info, "%s/%s hit %s before doing anything, retrying with %q", a.ID(), name, failureText(res.Failure), retry.Name)
		name = retry.Name
	}
}

// attempt runs one headless try. retry is the profile to try next; its Name
// is "" when there is none.
func attempt(ctx context.Context, o Options, cfg *config.Config, bin string, env []string, input []byte, name string, auto bool, tried []string) (Result, profiles.Pick, error) {
	a := o.Tool
	an := a.NewAnalyzer(o.Args)
	cap, err := proc.RunCaptured(bin, o.Args, proc.CaptureOptions{
		Env: env, Dir: o.Dir, Stdin: input, Stdout: o.Stdout, Stderr: o.Stderr, Timeout: o.Timeout,
		OnLine: an.Line,
	})
	res := Result{Code: 127, Profile: name}
	if err != nil {
		return res, profiles.Pick{}, err
	}
	res.Code, res.FinalText, res.ErrorText = cap.Code, an.FinalText(), an.ErrorText()
	if cap.Code == 0 && !an.ResultFailed() {
		cap.Flush()
		// It just worked, so any limit or login mark on it is out of date.
		if ps := config.LoadState().Get(a.ID(), name); ps.NeedsLogin || ps.Until.After(config.Now()) {
			_ = profiles.Clear(a.ID(), name)
		}
		return res, profiles.Pick{}, nil
	}
	res.Failure = tool.Classify(res.ErrorText)
	const by = "a failed run"
	switch res.Failure {
	case tool.FailLogin:
		_ = profiles.MarkNeedsLogin(a.ID(), name, by, res.ErrorText)
	case tool.FailLimit:
		var windows []tool.Window
		if u := config.LoadState().Get(a.ID(), name).Usage; u != nil {
			windows = u.Windows
		}
		_ = profiles.MarkLimited(cfg, a.ID(), name, profiles.Limit{
			ResetsAt: tool.ResetTime(res.ErrorText, config.Now()), Window: profiles.LimitWindow(cfg, res.ErrorText, windows),
			By: by, Detail: res.ErrorText,
		})
	case tool.FailBusy:
		// A rate limit passes in moments; another account is worth a try, a
		// long mark is not.
		_ = profiles.MarkLimited(cfg, a.ID(), name, profiles.Limit{Reason: "busy", For: busyFor, By: by, Detail: res.ErrorText})
	}
	// Retry only when the failed run provably changed nothing: structured
	// output that shows no tool call. Plain output hides tool calls, and even a
	// login error can come mid-run (a refresh token rotated by another session).
	safe := an.Activity() == tool.ActivityNone
	if res.Failure != tool.FailNone && auto && !cap.Committed && safe {
		if next := profiles.Choose(ctx, cfg, config.LoadState(), a, "", tried); next.Name != "" {
			return res, next, nil
		}
	}
	cap.Flush()
	if res.Failure != tool.FailNone {
		why := ""
		switch {
		case !auto:
		case cap.Committed || !safe:
			why = " (not retried: the run may already have changed things)"
		default:
			why = " (no other usable profile)"
		}
		o.say(Warn, "%s/%s hit %s; new runs will use another profile%s", a.ID(), name, failureText(res.Failure), why)
	}
	return res, profiles.Pick{}, nil
}

// sessionEnv prepares a run as profile name: its links, its environment, and
// the record that it was used last.
func sessionEnv(o Options, cfg *config.Config, name string) []string {
	a := o.Tool
	for _, r := range profiles.Sync(cfg, a, name, false) {
		if r.Action == links.Conflict && !r.Quiet {
			o.say(Warn, "%s/%s: shared %q: %s (aims doctor)", a.ID(), name, r.Name, r.Detail)
		}
	}
	env, dropped := profiles.Env(cfg, a, name, os.Environ())
	env = append(strip(env, o.StripEnv), o.ExtraEnv...)
	if len(dropped) > 0 {
		o.say(Info, "ignoring %s from your shell so the %q login is used", strings.Join(dropped, ", "), name)
	}
	usedAt := config.Now().UTC()
	_ = config.UpdateState(a.ID(), name, func(ps *config.ProfileState) { ps.LastUsedAt = usedAt })
	return env
}

// choose picks the profile a session starts on and says why when it is not
// the preferred one: the first usable profile in order, or, when every one is
// limited, the one that works again first.
func choose(ctx context.Context, o Options, cfg *config.Config, preferred string) string {
	a := o.Tool
	pick := profiles.Choose(ctx, cfg, config.LoadState(), a, preferred, nil)
	if pick.Name == "" && profiles.AskFree(ctx, cfg, a, cfg.Tools[a.ID()].Order) {
		pick = profiles.Choose(ctx, cfg, config.LoadState(), a, preferred, nil)
	}
	switch {
	case pick.Name == preferred:
		return preferred
	case pick.Name != "":
		o.say(Info, "%s: using %s while %s", a.ID(), pick.Name, describe(pick.Skipped))
	case pick.Soonest != "":
		o.say(Warn, "%s: every account is limited; using %s, the first back (in %s)", a.ID(), pick.Soonest, profiles.Duration(time.Until(pick.ReadyAt)))
		return pick.Soonest
	default:
		o.say(Warn, "%s: no account looks usable (%s); trying %s anyway", a.ID(), describe(pick.Skipped), preferred)
		return preferred
	}
	hints(o, pick.Skipped)
	return pick.Name
}

// hints says how to bring back accounts that were passed over.
func hints(o Options, skipped []profiles.Evaluation) {
	for _, e := range skipped {
		switch {
		case e.State == nil || (e.Account.LoggedIn != nil && !*e.Account.LoggedIn):
		case e.State.NeedsLogin:
			o.say(Hint, "log %s in again: aims login %s %s", e.Name, o.Tool.ID(), e.Name)
		case e.State.Until.After(config.Now()):
			o.say(Hint, "%s works again? aims clear %s %s", e.Name, o.Tool.ID(), e.Name)
		}
	}
}

// explain says, as a session starts, its account and what happens at a
// limit, in the status line's words: "claude: personal (auto → work)".
func explain(o Options, cfg *config.Config, preferred, name string, mode profiles.Mode) {
	var others []string
	for _, n := range cfg.Tools[o.Tool.ID()].Order {
		if n != name && n != preferred {
			others = append(others, n)
		}
	}
	tag := "auto"
	switch {
	case mode == profiles.Pinned:
		tag = "pinned"
	case mode == profiles.Unwatched:
		tag = "auto off"
	case name != preferred:
		tag += ", back to " + preferred
	case len(others) > 0:
		tag += " → " + profiles.Names(others, ", ")
	}
	o.say(Info, "%s: %s (%s)", o.Tool.ID(), name, tag)
	// The first session after an upgrade says once what the tag means.
	if config.Guided() || mode == profiles.Unwatched {
		return
	}
	config.MarkGuided()
	bin := o.Tool.Layout().Bin
	switch {
	case mode == profiles.Pinned:
		o.say(Hint, "this session stays on %s; plain aims %s moves on at a limit. More: aims status", name, bin)
	case name != preferred:
		o.say(Hint, "%s is out, so this session runs on %s and goes back once %s works. More: aims status", preferred, name, preferred)
	case len(others) > 0:
		o.say(Hint, "at a limit this conversation continues on %s and comes back to %s. More: aims status", others[0], name)
	}
}

// workDir is where the session runs.
func workDir(o Options) string {
	if o.Dir != "" {
		return o.Dir
	}
	dir, _ := os.Getwd()
	return dir
}

// failureText names a failure in a sentence: "hit its usage limit".
func failureText(f tool.Failure) string {
	switch f {
	case tool.FailLogin:
		return "a login error"
	case tool.FailBusy:
		return "a rate limit"
	}
	return "its usage limit"
}

func runOnce(o Options, bin string, env []string, input []byte) (Result, error) {
	an := o.Tool.NewAnalyzer(o.Args)
	cap, err := proc.RunCaptured(bin, o.Args, proc.CaptureOptions{
		Env: env, Dir: o.Dir, Stdin: input, Stdout: o.Stdout, Stderr: o.Stderr, Timeout: o.Timeout, OnLine: an.Line,
	})
	if err != nil {
		return Result{Code: 127}, err
	}
	cap.Flush()
	return Result{Code: cap.Code, FinalText: an.FinalText(), ErrorText: an.ErrorText()}, nil
}

func readInput(o Options) []byte {
	if o.Stdin != nil || o.NoStdin {
		return o.Stdin
	}
	b, timedOut := ReadStdin(3*time.Second, true)
	if timedOut {
		// The same rule as `claude -p`.
		o.say(Warn, "no stdin data received in 3s, proceeding without it (use < /dev/null to skip the wait)")
	}
	return b
}

func strip(env, keys []string) []string {
	if len(keys) == 0 {
		return env
	}
	out := env[:0:0]
	for _, kv := range env {
		k, _, _ := strings.Cut(kv, "=")
		drop := false
		for _, s := range keys {
			if k == s {
				drop = true
				break
			}
		}
		if !drop {
			out = append(out, kv)
		}
	}
	return out
}

// describe says why profiles were passed over: "work is limited (5h at
// 100% for 2h10m)".
func describe(evs []profiles.Evaluation) string {
	parts := make([]string, len(evs))
	for i, e := range evs {
		why := strings.Join(e.Reasons, ", ")
		if e.State != nil && e.State.MarkedBy != "" {
			why += ", marked by " + e.State.MarkedBy
		}
		parts[i] = fmt.Sprintf("%s is out (%s)", e.Name, why)
	}
	return strings.Join(parts, ", ")
}
