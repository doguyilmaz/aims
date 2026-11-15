package meter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"
)

const grokWebBillingURL = "https://grok.com/grok_api_v2.GrokBuildBilling/GetGrokCreditsConfig"

type grokWebReading struct {
	Percent      float64
	ImplicitZero bool
}

type grokPeriod struct {
	Type       string `json:"type"`
	Start, End time.Time
}

func enrichGrokWeb(ctx context.Context, raw []byte, reading Reading, login grokLogin, now time.Time) Reading {
	if reading.Error != "" || len(reading.Rows) != 1 || len(reading.Rows[0].Windows) != 1 ||
		!reading.Rows[0].Windows[0].PercentUnknown || !login.unchanged() {
		return reading
	}
	var doc struct {
		Config struct {
			Unified bool       `json:"isUnifiedBillingUser"`
			Period  grokPeriod `json:"currentPeriod"`
		}
	}
	if json.Unmarshal(raw, &doc) != nil || !doc.Config.Unified || doc.Config.Period.Type != "USAGE_PERIOD_TYPE_WEEKLY" ||
		doc.Config.Period.Start.IsZero() || doc.Config.Period.Start.After(now) || !doc.Config.Period.End.After(now) {
		return reading
	}
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	percent, err := grokWebPercent(ctx, grokWebClient(), login.token, doc.Config.Period, now)
	if err != nil || !login.unchanged() {
		return reading
	}
	reading.Rows[0].Windows[0].Percent = percent.Percent
	reading.Rows[0].Windows[0].PercentUnknown = false
	reading.Rows[0].Detail = "Usage verified with Grok web billing"
	if percent.ImplicitZero {
		reading.Rows[0].Detail = "No usage reported for the active period (Grok web default)"
	}
	return reading
}

func grokWebClient() *http.Client {
	return &http.Client{Timeout: 6 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
}

func grokWebPercent(ctx context.Context, client *http.Client, token string, period grokPeriod, now time.Time) (grokWebReading, error) {
	// Explicit false for exclude_legacy_monthly_usage; no purchase or reset action.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, grokWebBillingURL,
		bytes.NewReader([]byte{0, 0, 0, 0, 2, 8, 0}))
	if err != nil {
		return grokWebReading{}, errors.New("Grok web usage request unavailable")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/grpc-web+proto")
	req.Header.Set("X-Grpc-Web", "1")
	req.Header.Set("X-User-Agent", "connect-es/2.1.1")
	req.Header.Set("Origin", "https://grok.com")
	req.Header.Set("Referer", "https://grok.com/?_s=usage")
	res, err := client.Do(req)
	if err != nil {
		return grokWebReading{}, errors.New("Grok web usage unavailable")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "application/grpc-web+proto" {
		return grokWebReading{}, errors.New("Grok web usage rejected")
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 65537))
	if err != nil || len(body) > 65536 {
		return grokWebReading{}, errors.New("Grok web usage response unreadable")
	}
	return parseGrokWeb(body, res.Header.Get("Grpc-Status"), period, now)
}
