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
	cfg, err := config.Load()
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
		pick := profiles.Choose(ctx, cfg, config.LoadState(), a, preferred, nil)
		switch {
		case pick.Name == "":
			o.say(Warn, "no %s profile looks usable (%s); trying %q anyway", a.ID(), describe(pick.Skipped), preferred)
		case pick.Name != preferred:
			o.say(Info, "%s: skipping %s, using %q", a.ID(), describe(pick.Skipped), pick.Name)
			name = pick.Name
		}
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

		if !headless {
			code, err := proc.RunAttached(bin, o.Args, env, o.Dir)
			return Result{Code: code, Profile: name}, err
		}

		res, retry, err := attempt(ctx, o, cfg, bin, env, input, name, auto, tried)
		if err != nil || retry == "" {
			return res, err
		}
		o.say(Info, "%s/%s hit a %s error before doing anything, retrying with %q", a.ID(), name, res.Failure, retry)
		name = retry
	}
}

// attempt runs one headless try. retry names the profile to try next, or "".
func attempt(ctx context.Context, o Options, cfg *config.Config, bin string, env []string, input []byte, name string, auto bool, tried []string) (Result, string, error) {
	a := o.Tool
	an := a.NewAnalyzer(o.Args)
	cap, err := proc.RunCaptured(bin, o.Args, proc.CaptureOptions{
		Env: env, Dir: o.Dir, Stdin: input, Stdout: o.Stdout, Stderr: o.Stderr, Timeout: o.Timeout,
		OnLine: an.Line,
	})
	res := Result{Code: 127, Profile: name}
	if err != nil {
		return res, "", err
	}
	res.Code, res.FinalText, res.ErrorText = cap.Code, an.FinalText(), an.ErrorText()
	if cap.Code == 0 && !an.ResultFailed() {
		cap.Flush()
		// It just worked, so any limit or login mark on it is out of date.
		if ps := config.LoadState().Get(a.ID(), name); ps.NeedsLogin || ps.Until.After(config.Now()) {
			_ = profiles.Clear(a.ID(), name)
		}
		return res, "", nil
	}
	res.Failure = tool.Classify(res.ErrorText)
	switch res.Failure {
	case tool.FailLogin:
		_ = profiles.MarkNeedsLogin(a.ID(), name)
	case tool.FailLimit:
		_ = profiles.MarkLimited(cfg, a.ID(), name, "limit", time.Time{}, 0)
	}
	// Retry only when the failed run provably changed nothing: structured
	// output that shows no tool call. Plain output hides tool calls, and even a
	// login error can come mid-run (a refresh token rotated by another session).
	safe := an.Activity() == tool.ActivityNone
	if res.Failure != tool.FailNone && auto && !cap.Committed && safe {
		if next := profiles.Choose(ctx, cfg, config.LoadState(), a, "", tried); next.Name != "" {
			return res, next.Name, nil
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
		o.say(Warn, "%s/%s hit a %s error; new runs will use another profile%s", a.ID(), name, res.Failure, why)
	}
	return res, "", nil
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

func describe(evs []profiles.Evaluation) string {
	parts := make([]string, len(evs))
	for i, e := range evs {
		parts[i] = fmt.Sprintf("%q (%s)", e.Name, strings.Join(e.Reasons, ", "))
	}
	return strings.Join(parts, ", ")
}
