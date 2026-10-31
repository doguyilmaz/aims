package meter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/proc"
	"github.com/doguyilmaz/aims/internal/tool"
)

// kiro has no command that prints usage, so it calls GetUsageLimits, the API
// behind kiro-cli's own /usage, with the token kiro-cli keeps. aims only
// reads that token; kiro-cli renews it.
type kiro struct{}

func (kiro) ID() string    { return "kiro" }
func (kiro) Title() string { return "Kiro" }
func (kiro) Bin() string   { return "kiro-cli" }

func kiroDB() string {
	if runtime.GOOS == "darwin" {
		return filepath.Join(fsx.Home(), "Library", "Application Support", "kiro-cli", "data.sqlite3")
	}
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, "kiro-cli", "data.sqlite3")
	}
	return filepath.Join(fsx.Home(), ".local", "share", "kiro-cli", "data.sqlite3")
}

func (k kiro) Read(ctx context.Context, bin string) Reading {
	r, expired := k.read(ctx)
	if expired {
		_, _, _ = proc.Output(bin, []string{"whoami"}, os.Environ(), 20*time.Second)
		r, _ = k.read(ctx)
	}
	return r
}

// read reports expired when kiro-cli's token ran out.
func (kiro) read(ctx context.Context) (Reading, bool) {
	q := func(sql string) (string, error) {
		out, code, err := proc.Output("sqlite3", []string{"-readonly", kiroDB(), sql}, os.Environ(), 5*time.Second)
		if err == nil && code != 0 {
			err = fmt.Errorf("sqlite3 exited %d", code)
		}
		return strings.TrimSpace(out), err
	}
	tokRaw, err := q("select value from auth_kv where key='kirocli:odic:token'")
	if err != nil || tokRaw == "" {
		return Reading{Error: "kiro-cli is not logged in"}, false
	}
	profRaw, _ := q("select value from state where key='api.codewhisperer.profile'")
	var tok struct {
		Access  string    `json:"access_token"`
		Expires time.Time `json:"expires_at"`
	}
	var prof struct {
		ARN string `json:"arn"`
	}
	if json.Unmarshal([]byte(tokRaw), &tok) != nil || json.Unmarshal([]byte(profRaw), &prof) != nil || prof.ARN == "" {
		return Reading{Error: "kiro-cli's saved login is not readable"}, false
	}
	if !tok.Expires.IsZero() && tok.Expires.Before(time.Now()) {
		return Reading{Error: "kiro-cli's login ran out; open kiro-cli to renew it"}, true
	}
	raw, err := kiroUsage(ctx, kiroEndpoint(prof.ARN), tok.Access, prof.ARN)
	if err != nil {
		return Reading{Error: "kiro: " + err.Error()}, false
	}
	return parseKiro(raw), false
}

var regionRe = regexp.MustCompile(`^[a-z]{2}(-[a-z]+)+-\d$`)

// kiroEndpoint is the service of the profile's region.
func kiroEndpoint(arn string) string {
	parts := strings.Split(arn, ":")
	if len(parts) < 4 || parts[3] == "us-east-1" || !regionRe.MatchString(parts[3]) {
		return "https://codewhisperer.us-east-1.amazonaws.com/"
	}
	return "https://q." + parts[3] + ".amazonaws.com/"
}

func kiroUsage(ctx context.Context, url, token, arn string) ([]byte, error) {
	body, _ := json.Marshal(map[string]string{"profileArn": arn})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-amz-json-1.0")
	req.Header.Set("X-Amz-Target", "AmazonCodeWhispererService.GetUsageLimits")
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("usage request failed (%s)", res.Status)
	}
	return b, err
}

func parseKiro(raw []byte) Reading {
	var doc struct {
		Sub struct {
			Title string `json:"subscriptionTitle"`
		} `json:"subscriptionInfo"`
		Usage []struct {
			Name   string  `json:"displayNamePlural"`
			Used   float64 `json:"currentUsageWithPrecision"`
			Over   float64 `json:"currentOveragesWithPrecision"`
			Limit  float64 `json:"usageLimitWithPrecision"`
			Resets float64 `json:"nextDateReset"`
		} `json:"usageBreakdownList"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil || len(doc.Usage) == 0 {
		return Reading{Error: "Kiro did not report its usage (" + firstLine(raw) + ")"}
	}
	r := Reading{Plan: doc.Sub.Title}
	for _, u := range doc.Usage {
		if u.Limit <= 0 {
			continue
		}
		plan := u.Used - u.Over
		w := tool.Window{Label: "month", Percent: pct(plan, u.Limit)}
		if u.Resets > 0 {
			w.ResetsAt = time.Unix(int64(u.Resets), 0).UTC()
		}
		name := strings.ToLower(u.Name)
		if name == "" {
			name = "credits"
		}
		detail := fmt.Sprintf("%g of %g %s", plan, u.Limit, name)
		if u.Over > 0 {
			detail += fmt.Sprintf(", %g over", u.Over)
		}
		r.Rows = append(r.Rows, Row{Name: name, Windows: []tool.Window{w}, Detail: detail})
	}
	return r
}
