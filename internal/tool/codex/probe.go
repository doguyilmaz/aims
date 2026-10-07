package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/doguyilmaz/aims/internal/tool"
)

// Probe asks `codex app-server` (JSON-RPC over stdio) for the account and its
// rate limits. Neither call spends tokens.
func (*Adapter) Probe(ctx context.Context, h tool.Home) tool.Probe {
	if h.Bin == "" {
		return tool.Probe{Status: tool.ProbeError, Detail: "codex is not installed"}
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	res, err := rpc(ctx, h, []call{
		{"account/read", map[string]any{}},
		{"account/rateLimits/read", map[string]any{"excludeResetCreditDetails": true}},
	})
	if err != nil && res["account/read"] == nil {
		return tool.Probe{Status: tool.ProbeError, Detail: err.Error()}
	}
	return interpret(res["account/read"], res["account/rateLimits/read"])
}

type call struct {
	method string
	params any
}

type reply struct {
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// rpc runs one app-server session: initialize, then each call in order.
func rpc(ctx context.Context, h tool.Home, calls []call) (map[string]*reply, error) {
	cmd := exec.CommandContext(ctx, h.Bin, "app-server")
	cmd.Env = h.Env
	cmd.WaitDelay = time.Second
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	defer func() {
		stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	send := func(v map[string]any) error {
		v["jsonrpc"] = "2.0"
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		_, err = stdin.Write(append(b, '\n'))
		return err
	}
	if err := send(map[string]any{"id": 0, "method": "initialize", "params": map[string]any{"clientInfo": map[string]any{"name": "aims", "title": "aims", "version": "1"}}}); err != nil {
		return nil, err
	}

	byID := map[int]string{}
	out := map[string]*reply{}
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 64<<10), 8<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var msg struct {
			ID *int `json:"id"`
			reply
		}
		if json.Unmarshal([]byte(line), &msg) != nil || msg.ID == nil {
			continue // notifications
		}
		if *msg.ID == 0 {
			if err := send(map[string]any{"method": "initialized"}); err != nil {
				return out, err
			}
			for i, c := range calls {
				byID[i+1] = c.method
				if err := send(map[string]any{"id": i + 1, "method": c.method, "params": c.params}); err != nil {
					return out, err
				}
			}
			continue
		}
		if m, ok := byID[*msg.ID]; ok {
			r := msg.reply
			out[m] = &r
			if len(out) == len(calls) {
				return out, nil
			}
		}
	}
	if ctx.Err() != nil {
		return out, fmt.Errorf("codex app-server did not answer in time")
	}
	return out, fmt.Errorf("codex app-server closed the connection")
}

type window struct {
	UsedPercent        float64 `json:"usedPercent"`
	ResetsAt           *int64  `json:"resetsAt"`
	WindowDurationMins *int64  `json:"windowDurationMins"`
}

func interpret(acct, limits *reply) tool.Probe {
	if acct == nil {
		return tool.Probe{Status: tool.ProbeError, Detail: "no answer from codex app-server"}
	}
	if acct.Error != nil {
		return failure(acct.Error.Message)
	}
	var a struct {
		Account *struct {
			Type     string `json:"type"`
			Email    string `json:"email"`
			PlanType string `json:"planType"`
		} `json:"account"`
		RequiresOpenaiAuth bool `json:"requiresOpenaiAuth"`
	}
	if err := json.Unmarshal(acct.Result, &a); err != nil {
		return tool.Probe{Status: tool.ProbeError, Detail: "unexpected account answer"}
	}
	if a.Account == nil {
		if a.RequiresOpenaiAuth {
			return tool.Probe{Status: tool.ProbeLogin, Detail: "not logged in"}
		}
		return tool.Probe{Status: tool.ProbeOK}
	}
	p := tool.Probe{Status: tool.ProbeOK, Email: a.Account.Email, Plan: a.Account.PlanType}
	if p.Plan == "" {
		p.Plan = a.Account.Type
	}
	if limits == nil {
		return p
	}
	if limits.Error != nil {
		f := failure(limits.Error.Message)
		if f.Status == tool.ProbeError {
			f.Status = tool.ProbeOK // rate limits are optional (API key accounts)
		}
		f.Email, f.Plan = p.Email, p.Plan
		return f
	}
	var l struct {
		OrdinaryUsageAllowed *bool `json:"ordinaryUsageAllowed"`
		RateLimits           struct {
			Primary              *window `json:"primary"`
			Secondary            *window `json:"secondary"`
			RateLimitReachedType *string `json:"rateLimitReachedType"`
		} `json:"rateLimits"`
	}
	if json.Unmarshal(limits.Result, &l) != nil {
		return p
	}
	for _, w := range []struct {
		w        *window
		fallback string
	}{{l.RateLimits.Primary, "5h"}, {l.RateLimits.Secondary, "7d"}} {
		if w.w == nil {
			continue
		}
		win := tool.Window{Label: label(w.w.WindowDurationMins, w.fallback), Percent: w.w.UsedPercent}
		if w.w.ResetsAt != nil {
			win.ResetsAt = time.Unix(*w.w.ResetsAt, 0).UTC()
		}
		p.Windows = append(p.Windows, win)
	}
	if l.RateLimits.RateLimitReachedType != nil || (l.OrdinaryUsageAllowed != nil && !*l.OrdinaryUsageAllowed) {
		p.Status = tool.ProbeLimit
		if l.RateLimits.RateLimitReachedType != nil {
			p.Detail = strings.ReplaceAll(*l.RateLimits.RateLimitReachedType, "_", " ")
		} else {
			p.Detail = "usage not allowed"
		}
	}
	return p
}

func failure(msg string) tool.Probe {
	switch tool.Classify(msg) {
	case tool.FailLogin:
		return tool.Probe{Status: tool.ProbeLogin, Detail: msg}
	case tool.FailLimit:
		return tool.Probe{Status: tool.ProbeLimit, Detail: msg}
	}
	return tool.Probe{Status: tool.ProbeError, Detail: msg}
}

func label(mins *int64, fallback string) string {
	if mins == nil || *mins <= 0 {
		return fallback
	}
	m := *mins
	switch {
	case m%1440 == 0:
		return fmt.Sprintf("%dd", m/1440)
	case m%60 == 0:
		return fmt.Sprintf("%dh", m/60)
	}
	return fmt.Sprintf("%dm", m)
}
