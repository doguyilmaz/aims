package meter

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/doguyilmaz/aims/internal/tool"
)

// opencode asks the OpenCode background server, which holds the login, for
// the budgets of the workspace it is signed in to.
type opencode struct{}

func (opencode) ID() string    { return "opencode" }
func (opencode) Title() string { return "OpenCode" }
func (opencode) Bin() string   { return "opencode" }

func (opencode) Read(ctx context.Context, bin string) Reading {
	out, err := run(ctx, bin, "api", "GET", "/api/experimental/usage/limits")
	if err != nil {
		return Reading{Error: "opencode: " + err.Error()}
	}
	return parseOpenCode(out)
}

func parseOpenCode(raw []byte) Reading {
	var doc struct {
		Data struct {
			Workspace struct {
				Name string `json:"name"`
			} `json:"workspace"`
			Budgets []struct {
				Limit    float64 `json:"limit"`
				Spent    float64 `json:"spent"`
				Per      string  `json:"per"`
				ResetsAt int64   `json:"resetsAt"`
			} `json:"budgets"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil || doc.Data.Budgets == nil {
		return Reading{Error: "OpenCode did not report its budgets (" + firstLine(raw) + ")"}
	}
	var r Reading
	for _, b := range doc.Data.Budgets {
		if b.Limit <= 0 {
			continue
		}
		w := tool.Window{Label: label(b.Per), Percent: pct(b.Spent, b.Limit)}
		if w.Label == "set" || w.Label == "" {
			w.Label = "budget"
		}
		if b.ResetsAt > 0 {
			w.ResetsAt = time.UnixMilli(b.ResetsAt).UTC()
		}
		r.Rows = append(r.Rows, Row{Name: doc.Data.Workspace.Name, Windows: []tool.Window{w}, Detail: fmt.Sprintf("$%.2f of $%g", b.Spent, b.Limit)})
	}
	return r
}
