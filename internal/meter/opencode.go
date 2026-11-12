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
			Go *struct {
				Rolling *openCodeMeter `json:"rolling"`
				Weekly  *openCodeMeter `json:"weekly"`
				Monthly *openCodeMeter `json:"monthly"`
			} `json:"go"`
			Budgets []struct {
				ID       string  `json:"id"`
				Limit    float64 `json:"limit"`
				Spent    float64 `json:"spent"`
				Per      string  `json:"per"`
				ResetsAt int64   `json:"resetsAt"`
				Models   []struct {
					ID       string   `json:"modelID"`
					Provider string   `json:"providerID"`
					Name     string   `json:"name"`
					Spent    *float64 `json:"spent"`
				} `json:"models"`
			} `json:"budgets"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil || (doc.Data.Budgets == nil && doc.Data.Go == nil) {
		return Reading{Error: "OpenCode did not report its budgets (" + firstLine(raw) + ")"}
	}
	var r Reading
	if g := doc.Data.Go; g != nil {
		row := Row{ID: "go", Name: doc.Data.Workspace.Name + " · Go"}
		for _, p := range []struct {
			label string
			m     *openCodeMeter
		}{{"rolling", g.Rolling}, {"7d", g.Weekly}, {"month", g.Monthly}} {
			if p.m == nil {
				continue
			}
			w := tool.Window{ID: p.label, Label: p.label, Percent: p.m.Percent, Limited: p.m.Limited}
			if p.m.ResetsAt > 0 {
				w.ResetsAt = time.UnixMilli(p.m.ResetsAt).UTC()
			}
			row.Windows = append(row.Windows, w)
		}
		r.Rows = append(r.Rows, row)
	}
	for i, b := range doc.Data.Budgets {
		if b.Limit < 0 {
			continue
		}
		w := tool.Window{Label: label(b.Per)}
		if b.Limit == 0 {
			w.PercentUnknown = true
			w.Limited = true
		} else {
			w.Percent = pct(b.Spent, b.Limit)
		}
		if w.Label == "set" || w.Label == "" {
			w.Label = "budget"
		}
		if b.ResetsAt > 0 {
			w.ResetsAt = time.UnixMilli(b.ResetsAt).UTC()
		}
		id := b.ID
		if id == "" {
			id = fmt.Sprintf("budget/%d", i)
		}
		if b.Per == "model" && len(b.Models) > 0 {
			for _, m := range b.Models {
				mw := w
				mw.PercentUnknown = m.Spent == nil || b.Limit == 0
				detail := fmt.Sprintf("$%g budget per model", b.Limit)
				if m.Spent != nil {
					detail = fmt.Sprintf("$%.2f of $%g per model", *m.Spent, b.Limit)
					if b.Limit > 0 {
						mw.Percent = pct(*m.Spent, b.Limit)
					}
				}
				r.Rows = append(r.Rows, Row{ID: id + "/" + m.Provider + "/" + m.ID, Name: doc.Data.Workspace.Name + " · " + m.Name, Windows: []tool.Window{mw}, Detail: detail})
			}
		} else {
			detail := fmt.Sprintf("$%.2f of $%g", b.Spent, b.Limit)
			if b.Limit == 0 {
				detail += " · $0 budget blocks spend"
			}
			r.Rows = append(r.Rows, Row{ID: id, Name: doc.Data.Workspace.Name, Windows: []tool.Window{w}, Detail: detail})
		}
	}
	if len(r.Rows) == 0 {
		r.Rows = []Row{{ID: "workspace", Name: doc.Data.Workspace.Name, Windows: []tool.Window{}, Detail: "No Console budgets or Go allowance reported"}}
	}
	return r
}

type openCodeMeter struct {
	Percent  float64 `json:"percent"`
	Limited  bool    `json:"limited"`
	ResetsAt int64   `json:"resetsAt"`
}
