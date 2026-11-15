package claude

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/doguyilmaz/aims/internal/testutil"
	"github.com/doguyilmaz/aims/internal/tool"
)

type usageTransport func(*http.Request) (*http.Response, error)

func (f usageTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestUsageReadsOnlyTheQuotaEndpointAndRecognizesRealZero(t *testing.T) {
	client := usageClient()
	calls := 0
	client.Transport = usageTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != http.MethodGet || r.URL.String() != usageEndpoint || r.Body != nil || r.Header.Get("Authorization") != "Bearer fixture-secret" || r.Header.Get("anthropic-beta") != "oauth-2025-04-20" {
			t.Fatal("quota request changed endpoint, method or authentication")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"five_hour":{"utilization":0,"resets_at":"2026-11-15T14:00:00Z"},"seven_day":{"utilization":3.5,"resets_at":"2026-11-22T04:00:00Z"}}`)), Header: make(http.Header)}, nil
	})
	p := fetchUsage(t.Context(), client, "fixture-secret")
	if calls != 1 || p.Status != tool.ProbeOK || !p.UsageReported || len(p.Windows) != 2 || p.Windows[0].Percent != 0 || p.Windows[1].Percent != 3.5 || p.Windows[1].ResetsAt.IsZero() {
		t.Fatalf("unexpected quota observation: %+v", p)
	}
}

func TestUsageFailuresNeverPublishResponseTextOrFollowRedirects(t *testing.T) {
	for _, status := range []int{301, 401, 403, 429, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			client := usageClient()
			calls := 0
			client.Transport = usageTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: status, Header: http.Header{"Location": {"https://example.net/secret"}}, Body: io.NopCloser(strings.NewReader("fixture-secret provider body")), Request: r}, nil
			})
			p := fetchUsage(t.Context(), client, "fixture-secret")
			if calls != 1 || p.Status != tool.ProbeError || p.UsageReported || p.Windows != nil || p.UsageError == "" || strings.Contains(p.Detail+p.UsageError, "fixture-secret") {
				t.Fatalf("unsafe failure: %+v, requests=%d", p, calls)
			}
		})
	}
	client := usageClient()
	client.Transport = usageTransport(func(r *http.Request) (*http.Response, error) {
		return nil, errors.New("fixture-secret transport detail")
	})
	if p := fetchUsage(t.Context(), client, "fixture-secret"); strings.Contains(p.Detail, "fixture-secret") || p.UsageReported {
		t.Fatal("transport details leaked or a failed request became an observation")
	}
}

func TestUsageParsingDoesNotInventMissingOrInvalidPercentages(t *testing.T) {
	for _, body := range []string{`{}`, `null`, `[]`, `{"error":"denied"}`, `{"five_hour":{}}`, `{"five_hour":{"utilization":null}}`, `{"five_hour":{"utilization":"0"}}`, `{"five_hour":{"utilization":true}}`, `{"five_hour":{"utilization":-1}}`, `{"five_hour":{"utilization":20,"resets_at":"bad-date"}}`, `{"five_hour":{"utilization":20}} trailing`} {
		if windows, ok := parseUsage([]byte(body)); ok || windows != nil {
			t.Errorf("invalid response accepted: %s", body)
		}
	}
	if windows, ok := parseUsage([]byte(`{"five_hour":null,"seven_day":null}`)); !ok || len(windows) != 0 {
		t.Fatal("explicit absence should replace the old quota without inventing zero")
	}
	if windows, ok := parseUsage([]byte(`{"seven_day":null,"seven_day_oauth_apps":{"utilization":12},"seven_day_sonnet":{"utilization":7}}`)); !ok || len(windows) != 2 || windows[0].Label != "7d" || windows[1].Label != "Sonnet 7d" {
		t.Fatal("weekly fallback or model-specific quota lost")
	}
	client := usageClient()
	client.Transport = usageTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(strings.Repeat(" ", (64<<10)+1))), Header: make(http.Header)}, nil
	})
	if p := fetchUsage(t.Context(), client, "fixture"); p.Status != tool.ProbeError || p.UsageReported {
		t.Fatal("oversized response accepted")
	}
}

func TestUsageNeverRenewsOrBorrowsNativeCredentials(t *testing.T) {
	home := testutil.Sandbox(t)
	dir := filepath.Join(home, "work")
	path := filepath.Join(dir, ".credentials.json")
	credential := `{"claudeAiOauth":{"accessToken":"fixture-expired","refreshToken":"never-use","expiresAt":1}}`
	testutil.Write(t, path, credential)
	testutil.Write(t, filepath.Join(home, ".claude", ".credentials.json"), `{"claudeAiOauth":{"accessToken":"another-account"}}`)
	a := New()
	p := a.Usage(t.Context(), tool.Home{Dir: dir, EnvValue: dir})
	if p.Status != tool.ProbeError || !strings.Contains(p.UsageError, "expired") || p.UsageReported {
		t.Fatal("expired native login was not preserved")
	}
	if got := testutil.Read(t, path); got != credential {
		t.Fatal("usage changed native credentials")
	}
	if p := a.Usage(t.Context(), tool.Home{Dir: filepath.Join(home, "missing"), EnvValue: "custom"}); p.Status != tool.ProbeError || p.UsageReported {
		t.Fatal("missing custom profile borrowed another account's token")
	}
	for _, key := range []string{"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "CLAUDE_CODE_OAUTH_TOKEN"} {
		p := a.Usage(t.Context(), tool.Home{Dir: dir, ProfileEnv: map[string]string{key: "override"}})
		if p.Status != tool.ProbeError || !strings.Contains(p.Detail, "override") {
			t.Fatalf("%s override borrowed the native subscription", key)
		}
	}
	if info, err := os.Stat(path); err != nil || info.Size() != int64(len(credential)) {
		t.Fatal("native credential file lost")
	}
}

func TestUsageRequestHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	client := usageClient()
	client.Transport = usageTransport(func(r *http.Request) (*http.Response, error) {
		cancel()
		<-r.Context().Done()
		return nil, r.Context().Err()
	})
	defer cancel()
	if p := fetchUsage(ctx, client, "fixture"); p.Status != tool.ProbeError || p.UsageReported || client.Timeout > 10*time.Second {
		t.Fatal("cancelled quota request published a new measurement or lacks a bounded timeout")
	}
}
