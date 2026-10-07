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
	dim    = "\x1b[2m"
	green  = "\x1b[32m"
	yellow = "\x1b[33m"
	red    = "\x1b[31m"
	reset  = "\x1b[0m"
)

func segment(ctx context.Context, cfg *config.Config, name string, windows []tool.Window) string {
	color := os.Getenv("NO_COLOR") == ""
	paint := func(c, s string) string {
		if !color {
			return s
		}
		return c + s + reset
	}
	out := paint(dim, "aims ") + name
	for _, w := range windows {
		if w.Label != "5h" && w.Percent < 50 {
			continue // the weekly window matters only once it is filling up
		}
		c := green
		switch {
		case w.Percent >= cfg.Failover.Threshold:
			c = red
		case w.Percent >= 70:
			c = yellow
		}
		out += " " + paint(dim, w.Label) + " " + paint(c, fmt.Sprintf("%.0f%%", w.Percent))
	}
	for _, w := range windows {
		if w.Percent >= cfg.Failover.Threshold {
			if next := profiles.Choose(ctx, cfg, config.LoadState(), tools.Get(claude.ID), "", []string{name}); next.Name != "" {
				out += paint(yellow, " · aims failover claude ("+next.Name+")")
			}
			break
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
