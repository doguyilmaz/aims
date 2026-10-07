// Package statusline implements `aims statusline`, Claude Code's status line
// command: it shows the account a session runs on and records the plan usage
// Claude Code reports, so aims can switch accounts before a limit is hit.
package statusline

import (
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/launch"
	"github.com/doguyilmaz/aims/internal/profiles"
	"github.com/doguyilmaz/aims/internal/tool"
	"github.com/doguyilmaz/aims/internal/tool/claude"
	"github.com/doguyilmaz/aims/internal/tools"
)

// Run reads Claude Code's JSON on stdin and writes one status line to w.
func Run(ctx context.Context, w io.Writer) error {
	raw, _ := launch.ReadStdin(300*time.Millisecond, false)
	cfg, err := config.Load()
	if err != nil {
		// A broken config must not break every Claude Code session's status line.
		fmt.Fprintln(w, "aims: config error")
		return nil
	}
	name := cfg.ProfileForHomeEnv(claude.ID, os.Getenv("CLAUDE_CONFIG_DIR"))
	windows := claude.ParseStatusLine(raw)
	if name != "" && len(windows) > 0 && changed(config.LoadState().Get(claude.ID, name).Usage, windows) {
		_ = profiles.RecordUsage(claude.ID, name, windows, "statusline")
	}

	var parts []string
	if chain := cfg.Status.Chain; chain != "" && !claude.IsAimsStatusLine(chain) {
		if out := runChain(ctx, chain, raw); out != "" {
			parts = append(parts, out)
		}
	}
	if name != "" {
		parts = append(parts, segment(ctx, cfg, name, windows))
	}
	fmt.Fprintln(w, strings.Join(parts, "  "))
	return nil
}

// changed limits writes: the status line refreshes every few seconds.
func changed(prev *config.Usage, next []tool.Window) bool {
	if prev == nil || time.Since(prev.At) > time.Minute {
		return true
	}
	for _, w := range next {
		found := false
		for _, p := range prev.Windows {
			if p.Label == w.Label {
				found = true
				if math.Abs(p.Percent-w.Percent) >= 1 || !p.ResetsAt.Equal(w.ResetsAt) {
					return true
				}
			}
		}
		if !found {
			return true
		}
	}
	return false
}

const (
	bold   = "\x1b[1m"
	dim    = "\x1b[2m"
	green  = "\x1b[32m"
	yellow = "\x1b[33m"
	red    = "\x1b[31m"
	reset  = "\x1b[0m"
)

// segment is aims' part of the line, as short as it can be while still
// answering "which account, how much is left, what next":
//
//	work ▰▱▱▱▱  11% · 7d 47%
//	work ▰▰▰▰▱  82% ↻1h20m · 7d 61%
//	work ▰▰▰▰▰  97% ↻54m · 7d 61% → personal
func segment(ctx context.Context, cfg *config.Config, name string, windows []tool.Window) string {
	return render(cfg, name, windows, config.LoadState().Get(claude.ID, name), func() string {
		return profiles.Choose(ctx, cfg, config.LoadState(), tools.Get(claude.ID), "", []string{name}).Name
	}, config.Now(), os.Getenv("NO_COLOR") == "")
}

func render(cfg *config.Config, name string, windows []tool.Window, ps *config.ProfileState, next func() string, now time.Time, color bool) string {
	paint := func(c, s string) string {
		if !color {
			return s
		}
		return c + s + reset
	}
	level := func(pct float64) string {
		switch {
		case pct >= cfg.Failover.Threshold:
			return red
		case pct >= 70:
			return yellow
		}
		return green
	}
	parts := []string{paint(bold, name)}
	limited := false
	if ps != nil && ps.Until.After(now) {
		parts = append(parts, paint(red, "limited")+paint(dim, " ↻"+profiles.Duration(ps.Until.Sub(now))))
		limited = true
	}
	var tail []string
	for _, w := range windows {
		c := level(w.Percent)
		var p string
		if w.Label == "5h" {
			cells := int(w.Percent/20 + 0.5)
			cells = max(0, min(cells, 5))
			p = paint(c, strings.Repeat("▰", cells)) + paint(dim, strings.Repeat("▱", 5-cells)) + "  " + paint(c, fmt.Sprintf("%.0f%%", w.Percent))
		} else {
			p = paint(dim, w.Label+" ") + paint(c, fmt.Sprintf("%.0f%%", w.Percent))
		}
		if w.Percent >= 70 && w.ResetsAt.After(now) {
			p += paint(dim, " ↻"+profiles.Duration(w.ResetsAt.Sub(now)))
		}
		if w.Percent >= cfg.Failover.Threshold {
			limited = true
		}
		if w.Label == "5h" {
			parts = append(parts, p)
		} else {
			tail = append(tail, p)
		}
	}
	out := strings.Join(parts, " ")
	if len(tail) > 0 {
		out += paint(dim, " · ") + strings.Join(tail, paint(dim, " · "))
	}
	if limited {
		if n := next(); n != "" {
			out += paint(yellow, " → "+n)
		}
	}
	return out
}

// runChain runs the user's previous status line command with the same input.
func runChain(ctx context.Context, chain string, input []byte) string {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/d", "/s", "/c", chain)
	} else {
		cmd = exec.CommandContext(ctx, "/bin/sh", "-c", chain)
	}
	cmd.Stdin = strings.NewReader(string(input))
	cmd.WaitDelay = time.Second
	out, _ := cmd.Output()
	return strings.TrimRight(string(out), " \t\r\n")
}
