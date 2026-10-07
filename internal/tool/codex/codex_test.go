package codex

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/doguyilmaz/aims/internal/testutil"
	"github.com/doguyilmaz/aims/internal/tool"
)

func TestMain(m *testing.M) { testutil.Main(m) }

func TestAnalyzer(t *testing.T) {
	a := New()
	args := a.HeadlessArgs(tool.HeadlessOptions{Prompt: "go", Continue: true})
	if strings.Join(args[:3], " ") != "exec resume --last" || !a.IsHeadless(args) {
		t.Fatalf("args %q", args)
	}

	an := a.NewAnalyzer(args)
	an.Line(false, `{"type":"thread.started"}`)
	an.Line(false, `{"type":"item.completed","item":{"type":"agent_message","text":"You've hit your usage limit, it says"}}`)
	an.Line(false, `{"type":"turn.failed","error":{"message":"You've hit your usage limit."}}`)
	if an.Activity() != tool.ActivityNone || !an.ResultFailed() || tool.Classify(an.ErrorText()) != tool.FailLimit {
		t.Fatalf("activity=%v failed=%v err=%q", an.Activity(), an.ResultFailed(), an.ErrorText())
	}

	an = a.NewAnalyzer(args)
	an.Line(false, `{"type":"item.started","item":{"type":"command_execution"}}`)
	if an.Activity() != tool.ActivitySome {
		t.Fatal("command not seen")
	}

	// Plain output: only error lines on stderr count, never the echoed prompt.
	an = a.NewAnalyzer([]string{"exec", "--", "fix the 401 handling"})
	an.Line(true, "user")
	an.Line(true, "fix the 401 handling")
	if an.ErrorText() != "" {
		t.Fatalf("prompt read as an error: %q", an.ErrorText())
	}
	an.Line(true, "ERROR: unexpected status 401 Unauthorized")
	if an.Activity() != tool.ActivityUnknown || tool.Classify(an.ErrorText()) != tool.FailLogin {
		t.Fatalf("err=%q", an.ErrorText())
	}
}

func TestIsHeadless(t *testing.T) {
	a := New()
	for args, want := range map[string]bool{"exec hi": true, "e hi": true, "-m o3 exec x": true, "-c model=o3 --search e x": true, "--model=o3 exec": true, "-m exec": false, "resume --last": false, "": false, "-- exec": false} {
		if got := a.IsHeadless(strings.Fields(args)); got != want {
			t.Errorf("IsHeadless(%q) = %v", args, got)
		}
	}
}

func jwt(claims map[string]any) string {
	b, _ := json.Marshal(claims)
	return "x." + base64.RawURLEncoding.EncodeToString(b) + ".sig"
}

func TestAccount(t *testing.T) {
	home := testutil.Sandbox(t)
	a := New()
	dir := filepath.Join(home, "p")
	h := tool.Home{Dir: dir}
	// No auth.json and no binary: unknown.
	if acct := a.Account(t.Context(), h); acct.LoggedIn != nil {
		t.Fatalf("unknown expected: %+v", acct)
	}
	tok := jwt(map[string]any{"email": "w@x.com", "https://api.openai.com/auth": map[string]any{"chatgpt_plan_type": "pro"}})
	testutil.Write(t, filepath.Join(dir, "auth.json"), `{"tokens":{"id_token":"`+tok+`","refresh_token":"r"}}`)
	acct := a.Account(t.Context(), h)
	if acct.LoggedIn == nil || !*acct.LoggedIn || acct.Email != "w@x.com" || acct.Plan != "pro" {
		t.Fatalf("chatgpt login: %+v", acct)
	}
	testutil.Write(t, filepath.Join(dir, "auth.json"), `{"OPENAI_API_KEY":"sk-1"}`)
	if acct := a.Account(t.Context(), h); acct.Method != "api-key" {
		t.Fatalf("api key: %+v", acct)
	}
	testutil.Write(t, filepath.Join(dir, "auth.json"), `{"OPENAI_API_KEY":null,"tokens":null}`)
	if acct := a.Account(t.Context(), h); acct.LoggedIn == nil || *acct.LoggedIn {
		t.Fatalf("logged out: %+v", acct)
	}
	if e, p := claims("not-a-jwt"); e != "" || p != "" {
		t.Fatal("garbage token")
	}
}

func TestInterpret(t *testing.T) {
	raw := func(s string) *reply { return &reply{Result: json.RawMessage(s)} }
	p := interpret(raw(`{"account":{"type":"chatgpt","email":"w@x.com","planType":"plus"}}`),
		raw(`{"rateLimits":{"primary":{"usedPercent":40,"windowDurationMins":300,"resetsAt":1767225600},"secondary":{"usedPercent":10,"windowDurationMins":10080}}}`))
	if p.Status != tool.ProbeOK || p.Email != "w@x.com" || len(p.Windows) != 2 || p.Windows[0].Label != "5h" || p.Windows[1].Label != "7d" || p.Windows[0].ResetsAt.IsZero() {
		t.Fatalf("probe: %+v", p)
	}
	p = interpret(raw(`{"account":{"type":"chatgpt"}}`), raw(`{"rateLimits":{"primary":{"usedPercent":100},"rateLimitReachedType":"usage_limit_reached"}}`))
	if p.Status != tool.ProbeLimit || p.Detail != "usage limit reached" || p.Plan != "chatgpt" {
		t.Fatalf("limit: %+v", p)
	}
	if p := interpret(raw(`{"account":null,"requiresOpenaiAuth":true}`), nil); p.Status != tool.ProbeLogin {
		t.Fatalf("login: %+v", p)
	}
	if p := interpret(&reply{Error: &struct {
		Message string `json:"message"`
	}{"token_expired"}}, nil); p.Status != tool.ProbeLogin {
		t.Fatalf("rpc error: %+v", p)
	}
	// Rate limits are optional (API key accounts).
	p = interpret(raw(`{"account":{"type":"apiKey"}}`), &reply{Error: &struct {
		Message string `json:"message"`
	}{"not available"}})
	if p.Status != tool.ProbeOK {
		t.Fatalf("api key: %+v", p)
	}
	if p := interpret(nil, nil); p.Status != tool.ProbeError {
		t.Fatalf("no answer: %+v", p)
	}
}

func TestProbeWithFake(t *testing.T) {
	home := testutil.Sandbox(t)
	testutil.Fakes(t)
	dir := filepath.Join(home, ".codex")
	testutil.Write(t, filepath.Join(dir, "auth.json"), `{"tokens":{"access_token":"a"}}`)
	h := tool.Home{Dir: dir, Env: []string{"HOME=" + home}, Bin: os.Getenv("AIMS_CODEX_BIN")}
	p := New().Probe(t.Context(), h)
	if p.Status != tool.ProbeOK || p.Email != "default@example.com" || len(p.Windows) != 1 || p.Windows[0].Percent != 12 {
		t.Fatalf("probe: %+v", p)
	}
}

// A pasted error in the prompt must not decide how a run failed.
func TestPlainOutputUsesTheLastErrorOnly(t *testing.T) {
	an := New().NewAnalyzer([]string{"exec", "--", "why does this fail?"})
	an.Line(true, "user")
	an.Line(true, "ERROR: upstream 401 Unauthorized")
	an.Line(true, "ERROR: stream ended unexpectedly")
	if f := tool.Classify(an.ErrorText()); f != tool.FailNone {
		t.Fatalf("classified as %q from %q", f, an.ErrorText())
	}
}
