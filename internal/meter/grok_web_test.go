package meter

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var grokTestNow = time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)

func grokTestPeriod() grokPeriod {
	return grokPeriod{Type: "USAGE_PERIOD_TYPE_WEEKLY", Start: grokTestNow.Add(-24 * time.Hour), End: grokTestNow.Add(6 * 24 * time.Hour)}
}

func grokTestVarint(number, value uint64) []byte {
	b := binary.AppendUvarint(nil, number<<3)
	return binary.AppendUvarint(b, value)
}

func grokTestBytes(number uint64, value []byte) []byte {
	b := binary.AppendUvarint(nil, number<<3|2)
	b = binary.AppendUvarint(b, uint64(len(value)))
	return append(b, value...)
}

func grokTestFloat(number uint64, value float32) []byte {
	b := binary.AppendUvarint(nil, number<<3|5)
	return binary.LittleEndian.AppendUint32(b, math.Float32bits(value))
}

func grokTestConfig() []byte {
	p := grokTestPeriod()
	period := grokTestVarint(1, 2)
	period = append(period, grokTestBytes(2, grokTestVarint(1, uint64(p.Start.Unix())))...)
	period = append(period, grokTestBytes(3, grokTestVarint(1, uint64(p.End.Unix())))...)
	config := grokTestBytes(8, period)
	return append(config, grokTestVarint(11, 1)...)
}

func grokTestFrame(flags byte, data []byte) []byte {
	b := []byte{flags}
	b = binary.BigEndian.AppendUint32(b, uint32(len(data)))
	return append(b, data...)
}

func grokTestResponse(config []byte) []byte {
	b := grokTestFrame(0, grokTestBytes(1, config))
	return append(b, grokTestFrame(128, []byte("grpc-status:0\r\n"))...)
}

func TestGrokWebRequiresCompleteMatchingActivePeriodForZero(t *testing.T) {
	for _, tc := range []struct {
		name     string
		raw      []byte
		want     float64
		implicit bool
	}{
		{"implicit zero", grokTestResponse(grokTestConfig()), 0, true},
		{"explicit zero", grokTestResponse(append(grokTestConfig(), grokTestFloat(1, 0)...)), 0, false},
		{"explicit fraction", grokTestResponse(append(grokTestConfig(), grokTestFloat(1, 0.5)...)), 0.5, false},
		{"explicit percent", grokTestResponse(append(grokTestConfig(), grokTestFloat(1, 73)...)), 73, false},
		{"unknown opaque extension", grokTestResponse(append(grokTestConfig(), grokTestBytes(99, []byte{0xff, 0, 0xfe})...)), 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseGrokWeb(tc.raw, "", grokTestPeriod(), grokTestNow)
			if err != nil || got.Percent != tc.want {
				t.Fatalf("got %v, %v; want %v", got, err, tc.want)
			}
			if got.ImplicitZero != tc.implicit {
				t.Fatal("wire percentage and active-period default were conflated")
			}
		})
	}
	valid := grokTestResponse(grokTestConfig())
	for _, raw := range [][]byte{
		valid[:len(valid)-1], append(bytes.Clone(valid), 0), grokTestFrame(1, grokTestBytes(1, grokTestConfig())),
		grokTestFrame(0, grokTestBytes(1, grokTestConfig())),
		append(grokTestFrame(0, grokTestBytes(1, grokTestConfig())), grokTestFrame(128, []byte("grpc-status:16\r\n"))...),
		grokTestResponse(nil), grokTestResponse(append(grokTestConfig(), grokTestFloat(1, float32(math.NaN()))...)),
		grokTestResponse(append(grokTestConfig(), grokTestFloat(1, 101)...)),
		grokTestResponse(append(grokTestConfig(), grokTestFloat(1, -1)...)),
		grokTestResponse(append(grokTestConfig(), grokTestVarint(1, 0)...)),
		grokTestResponse(append(append(grokTestConfig(), grokTestFloat(1, 2)...), grokTestFloat(1, 3)...)),
		grokTestResponse(append(grokTestConfig(), grokTestBytes(7, grokTestFloat(2, 19))...)),
		grokTestResponse(append(grokTestConfig(), grokTestBytes(4, []byte{0xff})...)),
		grokTestResponse(append(grokTestConfig(), []byte{0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0}...)),
		grokTestResponse(append(grokTestConfig(), 0)), bytes.Repeat([]byte{0}, 65537),
	} {
		if got, err := parseGrokWeb(raw, "", grokTestPeriod(), grokTestNow); err == nil {
			t.Fatalf("unverified response accepted as %v: %x", got, raw[:min(len(raw), 80)])
		}
	}
	for _, now := range []time.Time{grokTestPeriod().Start.Add(-time.Second), grokTestPeriod().End} {
		if _, err := parseGrokWeb(valid, "", grokTestPeriod(), now); err == nil {
			t.Fatal("inactive period accepted")
		}
	}
	other := grokTestPeriod()
	other.End = other.End.Add(time.Hour)
	if _, err := parseGrokWeb(valid, "", other, grokTestNow); err == nil {
		t.Fatal("different native/web periods combined")
	}
}

type grokTestTransport func(*http.Request) (*http.Response, error)

func (fn grokTestTransport) RoundTrip(req *http.Request) (*http.Response, error) { return fn(req) }

func TestGrokWebUsesOnlyFixedReadOnlyEndpointAndRejectsErrors(t *testing.T) {
	client := &http.Client{Transport: grokTestTransport(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != grokWebBillingURL || req.Method != http.MethodPost || req.Header.Get("Cookie") != "" ||
			req.Header.Get("Authorization") != "Bearer fixture-secret" {
			t.Fatalf("unexpected request destination or auth: %v", req.URL)
		}
		body, _ := io.ReadAll(req.Body)
		if !bytes.Equal(body, []byte{0, 0, 0, 0, 2, 8, 0}) {
			t.Fatal("request is not the read-only billing message")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/grpc-web+proto"}},
			Body: io.NopCloser(bytes.NewReader(grokTestResponse(grokTestConfig())))}, nil
	})}
	if value, err := grokWebPercent(context.Background(), client, "fixture-secret", grokTestPeriod(), grokTestNow); err != nil || value.Percent != 0 {
		t.Fatalf("valid read failed: %v %v", value, err)
	}
	client.Transport = grokTestTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 401, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("fixture-secret"))}, nil
	})
	_, err := grokWebPercent(context.Background(), client, "fixture-secret", grokTestPeriod(), grokTestNow)
	if err == nil || strings.Contains(err.Error(), "fixture-secret") {
		t.Fatal("rejection lost or response/credential leaked")
	}
}

func TestGrokLoginSnapshotIsReadOnlyUnambiguousAndAccountBound(t *testing.T) {
	t.Setenv("XAI_API_KEY", "")
	t.Setenv("GROK_API_KEY", "")
	path := filepath.Join(t.TempDir(), "auth.json")
	t.Setenv("GROK_AUTH_PATH", path)
	entry := map[string]any{"key": "fixture-secret", "oidc_issuer": "https://auth.x.ai", "auth_mode": "oidc", "expires_at": grokTestNow.Add(time.Hour)}
	write := func(entries map[string]any) []byte {
		t.Helper()
		raw, _ := json.Marshal(entries)
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		return raw
	}
	original := write(map[string]any{"https://auth.x.ai::fixture": entry})
	login := grokLoginSnapshot(grokTestNow)
	if login.token != "fixture-secret" || !login.unchanged() {
		t.Fatal("single native login not recognized")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(after, original) {
		t.Fatal("login reader changed credentials")
	}
	write(map[string]any{"https://auth.x.ai::fixture": entry, "https://auth.x.ai::other": entry})
	if login.unchanged() || grokLoginSnapshot(grokTestNow).token != "" {
		t.Fatal("changed or ambiguous account accepted")
	}
	entry["expires_at"] = grokTestNow
	write(map[string]any{"https://auth.x.ai::fixture": entry})
	if grokLoginSnapshot(grokTestNow).token != "" {
		t.Fatal("expired token accepted")
	}
	entry["expires_at"] = grokTestNow.Add(time.Hour)
	entry["oidc_issuer"] = "https://another.example"
	write(map[string]any{"https://auth.x.ai::fixture": entry})
	if grokLoginSnapshot(grokTestNow).token != "" {
		t.Fatal("foreign issuer accepted")
	}
	entry["oidc_issuer"] = "https://auth.x.ai"
	entry["principal_type"] = "team"
	write(map[string]any{"https://auth.x.ai::fixture": entry})
	if grokLoginSnapshot(grokTestNow).token != "" {
		t.Fatal("team login accepted for personal billing")
	}
}

func TestGrokWebDoesNotFollowRedirectsAndHonorsCancellation(t *testing.T) {
	client := grokWebClient()
	requests := 0
	client.Transport = grokTestTransport(func(*http.Request) (*http.Response, error) {
		requests++
		return &http.Response{StatusCode: 302, Header: http.Header{"Location": {"https://other.example/usage"}},
			Body: io.NopCloser(strings.NewReader(""))}, nil
	})
	if _, err := grokWebPercent(context.Background(), client, "fixture-secret", grokTestPeriod(), grokTestNow); err == nil || requests != 1 {
		t.Fatal("redirect not rejected before another request")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client.Transport = grokTestTransport(func(req *http.Request) (*http.Response, error) {
		return nil, req.Context().Err()
	})
	if _, err := grokWebPercent(ctx, client, "fixture-secret", grokTestPeriod(), grokTestNow); err == nil {
		t.Fatal("cancelled usage read succeeded")
	}
}

func FuzzGrokWebParser(f *testing.F) {
	f.Add(grokTestResponse(grokTestConfig()))
	f.Add([]byte{0, 0, 0, 0, 1, 0xff})
	f.Fuzz(func(t *testing.T, raw []byte) { _, _ = parseGrokWeb(raw, "", grokTestPeriod(), grokTestNow) })
}
