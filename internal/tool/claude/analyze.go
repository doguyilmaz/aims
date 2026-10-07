package claude

import (
	"encoding/json"
	"strings"

	"github.com/doguyilmaz/aims/internal/tool"
)

func (*Adapter) NewAnalyzer(args []string) tool.Analyzer {
	return &analyzer{
		format:  tool.OptionValue(args, "--output-format", "text"),
		lastOut: tool.Tail{Max: 5},
		errs:    tool.Tail{Max: 20},
	}
}

// analyzer reads `claude -p` output in text, json or stream-json format.
type analyzer struct {
	format  string
	lastOut tool.Tail
	outN    int
	errs    tool.Tail
	result  *result
	toolUse bool
}

type result struct {
	IsError bool   `json:"is_error"`
	Result  string `json:"result"`
	Subtype string `json:"subtype"`
}

func (a *analyzer) Line(stderr bool, line string) {
	if stderr {
		a.errs.Add(line)
		return
	}
	if a.format == "text" {
		a.lastOut.Add(line)
		if strings.TrimSpace(line) != "" {
			a.outN++
		}
		return
	}
	if !strings.HasPrefix(line, "{") {
		return
	}
	var ev struct {
		Type    string `json:"type"`
		Message struct {
			Content []struct {
				Type string `json:"type"`
			} `json:"content"`
		} `json:"message"`
	}
	if json.Unmarshal([]byte(line), &ev) != nil {
		return
	}
	switch ev.Type {
	case "result":
		var r result
		if json.Unmarshal([]byte(line), &r) == nil {
			a.result = &r
		}
	case "assistant":
		for _, c := range ev.Message.Content {
			if c.Type == "tool_use" {
				a.toolUse = true
			}
		}
	}
}

func (a *analyzer) ResultFailed() bool { return a.result != nil && a.result.IsError }

func (a *analyzer) ErrorText() string {
	var parts []string
	switch {
	case a.result != nil && a.result.IsError:
		parts = append(parts, strings.TrimSpace(a.result.Result+" "+a.result.Subtype))
	case a.result == nil && a.outN <= 2:
		// Text mode prints only the final result, so a failed run's short
		// stdout is the error. Anything longer is an answer, never read here.
		parts = append(parts, a.lastOut.String())
	}
	parts = append(parts, a.errs.String())
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func (a *analyzer) Activity() tool.Activity {
	switch {
	case a.format != "stream-json":
		return tool.ActivityUnknown
	case a.toolUse:
		return tool.ActivitySome
	}
	return tool.ActivityNone
}

func (a *analyzer) FinalText() string {
	if a.result != nil && !a.result.IsError {
		return a.result.Result
	}
	return ""
}
