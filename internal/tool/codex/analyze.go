package codex

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/doguyilmaz/aims/internal/tool"
)

func (*Adapter) NewAnalyzer(args []string) tool.Analyzer {
	return &analyzer{json: tool.HasFlag(args, "--json"), errs: tool.Tail{Max: 20}}
}

var toolItems = map[string]bool{"command_execution": true, "file_change": true, "mcp_tool_call": true, "web_search": true}

// errorLine matches `ERROR: ...`, `2026-...Z ERROR module: ...` and
// `warning: ...`, never the prompt codex echoes back after "user".
var errorLine = regexp.MustCompile(`^(?:ERROR:|warning:|\d{4}-\d\d-\d\dT\S+\s+ERROR\s)`)

// analyzer reads `codex exec` output: human-readable, or --json events.
type analyzer struct {
	json    bool
	errs    tool.Tail
	tool    bool
	failed  bool
	message string
}

func (a *analyzer) Line(stderr bool, line string) {
	if stderr {
		if errorLine.MatchString(line) {
			a.errs.Add(line)
		}
		return
	}
	if !a.json || !strings.HasPrefix(line, "{") {
		return
	}
	var ev struct {
		Type    string `json:"type"`
		Message string `json:"message"`
		Error   struct {
			Message string `json:"message"`
		} `json:"error"`
		Item struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"item"`
	}
	if json.Unmarshal([]byte(line), &ev) != nil {
		return
	}
	switch ev.Type {
	case "error":
		a.errs.Add(ev.Message)
	case "turn.failed":
		a.failed = true
		msg := ev.Error.Message
		if msg == "" {
			msg = "turn failed"
		}
		a.errs.Add(msg)
	case "item.started", "item.completed":
		if toolItems[ev.Item.Type] {
			a.tool = true
		}
		if ev.Type == "item.completed" && ev.Item.Type == "agent_message" {
			a.message = ev.Item.Text
		}
	}
}

func (a *analyzer) ResultFailed() bool { return a.failed }
func (a *analyzer) ErrorText() string  { return a.errs.String() }
func (a *analyzer) FinalText() string  { return a.message }

func (a *analyzer) Activity() tool.Activity {
	switch {
	case !a.json:
		return tool.ActivityUnknown
	case a.tool:
		return tool.ActivitySome
	}
	return tool.ActivityNone
}
