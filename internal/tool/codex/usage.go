package codex

import (
	"bytes"
	"encoding/json"
	"sort"
	"time"

	"github.com/doguyilmaz/aims/internal/tool"
)

// additionalUsage reads the documented app-server usage snapshot. The main
// rateLimits view remains the sole input to account failover.
func additionalUsage(raw []byte, p *tool.Probe) {
	var doc struct {
		Main struct {
			ID string `json:"limitId"`
		} `json:"rateLimits"`
		Groups map[string]struct {
			ID        string  `json:"limitId"`
			Name      string  `json:"limitName"`
			Primary   *window `json:"primary"`
			Secondary *window `json:"secondary"`
		} `json:"rateLimitsByLimitId"`
		Resets *struct {
			Available *int64 `json:"availableCount"`
			Credits   []struct {
				ID        string          `json:"id"`
				Kind      string          `json:"resetType"`
				Status    string          `json:"status"`
				Title     string          `json:"title"`
				GrantedAt int64           `json:"grantedAt"`
				ExpiresAt json.RawMessage `json:"expiresAt"`
			} `json:"credits"`
		} `json:"rateLimitResetCredits"`
	}
	if json.Unmarshal(raw, &doc) != nil {
		return
	}
	keys := make([]string, 0, len(doc.Groups))
	for key := range doc.Groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		g := doc.Groups[key]
		if key == "codex" || g.ID == "codex" || (doc.Main.ID != "" && (key == doc.Main.ID || g.ID == doc.Main.ID)) {
			continue
		} // historical main view
		name := g.Name
		if name == "" {
			name = key
		}
		group := tool.QuotaGroup{ID: key, Name: name}
		for i, w := range []*window{g.Primary, g.Secondary} {
			if w == nil {
				continue
			}
			fallback := []string{"5h", "7d"}[i]
			win := tool.Window{ID: []string{"primary", "secondary"}[i], Label: label(w.WindowDurationMins, fallback), Percent: w.UsedPercent}
			if w.ResetsAt != nil && *w.ResetsAt > 0 {
				win.ResetsAt = time.Unix(*w.ResetsAt, 0).UTC()
			}
			group.Windows = append(group.Windows, win)
		}
		if len(group.Windows) > 0 {
			p.Groups = append(p.Groups, group)
		}
	}
	if doc.Resets == nil || doc.Resets.Available == nil || *doc.Resets.Available < 0 {
		return
	}
	b := &tool.ResetBalance{Available: *doc.Resets.Available}
	if doc.Resets.Credits != nil {
		b.Credits = make([]tool.ResetCredit, 0, len(doc.Resets.Credits))
	}
	for _, c := range doc.Resets.Credits {
		credit := tool.ResetCredit{ID: c.ID, Kind: c.Kind, Status: c.Status, Title: c.Title}
		if c.GrantedAt > 0 {
			credit.GrantedAt = time.Unix(c.GrantedAt, 0).UTC()
		}
		if bytes.Equal(bytes.TrimSpace(c.ExpiresAt), []byte("null")) {
			credit.NoExpiry = true
		} else {
			var epoch int64
			if json.Unmarshal(c.ExpiresAt, &epoch) == nil && epoch > 0 {
				credit.ExpiresAt = time.Unix(epoch, 0).UTC()
			}
		}
		b.Credits = append(b.Credits, credit)
	}
	p.Resets = b
}
