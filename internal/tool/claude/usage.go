package claude

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"time"

	"github.com/doguyilmaz/aims/internal/tool"
)

var _ tool.UsageReader = (*Adapter)(nil)

const usageEndpoint = "https://api.anthropic.com/api/oauth/usage"

// Usage asks only for subscription quotas. Native Claude Code remains the sole
// owner of token renewal: no refresh-token request, credential write or model
// invocation is performed, including on authentication failure.
func (*Adapter) Usage(ctx context.Context, h tool.Home) tool.Probe {
	for _, key := range []string{"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "CLAUDE_CODE_OAUTH_TOKEN"} {
		if h.ProfileEnv[key] != "" {
			return usageFailure("Claude subscription usage is unavailable for a token override; use Claude Code's /usage for this account.")
		}
	}
	_, credential := readOAuth(ctx, h)
	if err := ctx.Err(); err != nil {
		return usageFailure("Claude usage read cancelled.")
	}
	if credential == nil || credential.AccessToken == "" {
		return usageFailure("Claude usage access is unavailable; open this account in Claude Code and retry Refresh.")
	}
	if credential.ExpiresAt > 0 && !time.UnixMilli(credential.ExpiresAt).After(time.Now()) {
		return usageFailure("Claude usage access has expired; use this account in Claude Code, then retry Refresh. Aims did not renew the login.")
	}
	return fetchUsage(ctx, usageClient(), credential.AccessToken)
}

func usageClient() *http.Client {
	return &http.Client{
		Timeout: 8 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errors.New("Claude usage redirects are refused")
		},
	}
}

func usageFailure(message string) tool.Probe {
	return tool.Probe{Status: tool.ProbeError, Detail: message, UsageError: message}
}

func fetchUsage(ctx context.Context, client *http.Client, token string) tool.Probe {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, usageEndpoint, nil)
	if err != nil {
		return usageFailure("Claude usage request could not be prepared.")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")
	req.Header.Set("Accept", "application/json")
	response, err := client.Do(req)
	if err != nil {
		return usageFailure("Claude usage could not be refreshed; the previous measurement is retained. Retry Refresh later.")
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized, http.StatusForbidden:
		return usageFailure("Claude usage access was rejected; open this account's /usage in Claude Code, then retry Refresh. Aims did not change the login.")
	case http.StatusTooManyRequests:
		return usageFailure("Claude usage is temporarily rate limited; the previous measurement is retained. Retry Refresh later.")
	default:
		return usageFailure("Claude usage is temporarily unavailable; the previous measurement is retained.")
	}
	const limit = 64 << 10
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil || len(body) > limit {
		return usageFailure("Claude returned an unreadable usage response; the previous measurement is retained.")
	}
	windows, ok := parseUsage(body)
	if !ok {
		return usageFailure("Claude did not report recognized quota data; the previous measurement is retained.")
	}
	return tool.Probe{Status: tool.ProbeOK, Windows: windows, UsageReported: true}
}

func parseUsage(body []byte) ([]tool.Window, bool) {
	var document map[string]json.RawMessage
	if json.Unmarshal(body, &document) != nil || document == nil {
		return nil, false
	}
	var windows []tool.Window
	recognized := false
	for _, spec := range []struct{ key, label string }{
		{"five_hour", "5h"}, {"seven_day", "7d"},
		{"seven_day_sonnet", "Sonnet 7d"}, {"seven_day_opus", "Opus 7d"},
	} {
		raw, exists := document[spec.key]
		if spec.key == "seven_day" && (!exists || string(raw) == "null") {
			if fallback, found := document["seven_day_oauth_apps"]; found {
				raw, exists = fallback, true
			}
		}
		if !exists {
			continue
		}
		recognized = true
		if string(raw) == "null" {
			continue // Explicit absence is not zero usage.
		}
		var window struct {
			Utilization *float64 `json:"utilization"`
			ResetsAt    any      `json:"resets_at"`
		}
		if json.Unmarshal(raw, &window) != nil || window.Utilization == nil || math.IsNaN(*window.Utilization) || math.IsInf(*window.Utilization, 0) || *window.Utilization < 0 {
			return nil, false
		}
		reset := tool.ParseTime(window.ResetsAt)
		if window.ResetsAt != nil && reset.IsZero() {
			return nil, false
		}
		windows = append(windows, tool.Window{Label: spec.label, Percent: *window.Utilization, ResetsAt: reset})
	}
	return windows, recognized
}
