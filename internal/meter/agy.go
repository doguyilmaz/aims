package meter

import (
	"context"
	"encoding/json"
	"time"

	"github.com/doguyilmaz/aims/internal/tool"
)

// agy is Google Antigravity's CLI. `/usage` in print mode answers from the
// CLI without a model turn.
type agy struct{}

func (agy) ID() string    { return "agy" }
func (agy) Title() string { return "Antigravity" }
func (agy) Bin() string   { return "agy" }

func (agy) Read(ctx context.Context, bin string) Reading {
	out, err := run(ctx, bin, "-p", "/usage", "--output-format", "json", "--print-timeout", "30s")
	if err != nil {
		return Reading{Error: "agy: " + err.Error()}
	}
	return parseAgy(out)
}

func parseAgy(raw []byte) Reading {
	var doc struct {
		Status  string `json:"status"`
		Command struct {
			Data struct {
				Groups []struct {
					Name    string `json:"name"`
					Buckets []struct {
						Window    string    `json:"window"`
						Remaining *float64  `json:"remaining_fraction"`
						ResetTime time.Time `json:"reset_time"`
						Disabled  bool      `json:"disabled"`
					} `json:"buckets"`
				} `json:"groups"`
			} `json:"data"`
		} `json:"command"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil || len(doc.Command.Data.Groups) == 0 {
		return Reading{Error: "agy did not report its quota (" + firstLine(raw) + ")"}
	}
	var r Reading
	for _, g := range doc.Command.Data.Groups {
		row := Row{Name: g.Name}
		for _, b := range g.Buckets {
			if b.Disabled || b.Remaining == nil {
				continue
			}
			row.Windows = append(row.Windows, tool.Window{Label: label(b.Window), Percent: pct(1-*b.Remaining, 1), ResetsAt: b.ResetTime})
		}
		if len(row.Windows) > 0 {
			r.Rows = append(r.Rows, row)
		}
	}
	return r
}
