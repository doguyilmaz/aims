package meter

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/doguyilmaz/aims/internal/tool"
)

// grok asks Grok Build's agent over its own ACP channel (`grok agent
// stdio`), which signs in with the CLI's cached login.
type grok struct{}

func (grok) ID() string    { return "grok" }
func (grok) Title() string { return "Grok" }
func (grok) Bin() string   { return "grok" }

func (grok) Read(ctx context.Context, bin string) Reading {
	raw, err := acpCall(ctx, bin, []string{"agent", "stdio"}, "_x.ai/billing")
	if err != nil {
		return Reading{Error: "grok: " + err.Error()}
	}
	return parseGrok(raw)
}

// acpCall starts an ACP agent, initializes it and returns the result of one
// method call.
func acpCall(ctx context.Context, bin string, args []string, method string) (json.RawMessage, error) {
	dir, err := workDir()
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = os.Environ()
	cmd.Dir = dir
	cmd.WaitDelay = time.Second
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	defer func() {
		in.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()
	send := func(id int, m string, params any) error {
		b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": m, "params": params})
		_, err := in.Write(append(b, '\n'))
		return err
	}
	if err := send(1, "initialize", map[string]any{"protocolVersion": 1, "clientCapabilities": map[string]any{}}); err != nil {
		return nil, err
	}
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 64<<10), 16<<20)
	for sc.Scan() {
		var msg struct {
			ID     *int            `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(sc.Bytes(), &msg) != nil || msg.ID == nil {
			continue
		}
		switch {
		case msg.Error != nil:
			return nil, errors.New(msg.Error.Message)
		case *msg.ID == 1:
			if err := send(2, method, map[string]any{}); err != nil {
				return nil, err
			}
		case *msg.ID == 2:
			return msg.Result, nil
		}
	}
	if ctx.Err() != nil {
		return nil, fmt.Errorf("no answer in time")
	}
	if err := sc.Err(); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return nil, fmt.Errorf("the agent closed without an answer")
}

func parseGrok(raw []byte) Reading {
	var doc struct {
		Config *struct {
			CreditUsagePercent *float64 `json:"creditUsagePercent"`
			CurrentPeriod      struct {
				Type string    `json:"type"`
				End  time.Time `json:"end"`
			} `json:"currentPeriod"`
		} `json:"config"`
		Tier string `json:"subscription_tier"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil || doc.Config == nil || doc.Config.CreditUsagePercent == nil {
		return Reading{Error: "grok did not report its credits (" + firstLine(raw) + ")"}
	}
	c := doc.Config
	return Reading{Plan: doc.Tier, Rows: []Row{{Name: "credits", Windows: []tool.Window{{Label: label(c.CurrentPeriod.Type), Percent: *c.CreditUsagePercent, ResetsAt: c.CurrentPeriod.End}}}}}
}
