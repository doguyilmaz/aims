package claude

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tidwall/gjson"

	"github.com/doguyilmaz/aims/internal/testutil"
	"github.com/doguyilmaz/aims/internal/tool"
)

func TestMain(m *testing.M) { testutil.Main(m) }

func feed(a tool.Analyzer, lines ...string) {
	for _, l := range lines {
		stderr := strings.HasPrefix(l, "!")
		a.Line(stderr, strings.TrimPrefix(l, "!"))
	}
}

func TestAnalyzerStreamJSON(t *testing.T) {
	ad := New()
	args := ad.HeadlessArgs(tool.HeadlessOptions{Prompt: "-x"})
	if args[len(args)-2] != "--" || args[len(args)-1] != "-x" {
		t.Fatalf("prompt must follow --: %q", args)
	}

	an := ad.NewAnalyzer(args)
	feed(an,
		`{"type":"system","subtype":"init"}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"You've hit your limit"}]}}`,
		`{"type":"result","subtype":"success","is_error":true,"result":"Claude AI usage limit reached"}`,
	)
	if an.Activity() != tool.ActivityNone || !an.ResultFailed() || tool.Classify(an.ErrorText()) != tool.FailLimit {
		t.Fatalf("activity=%v failed=%v err=%q", an.Activity(), an.ResultFailed(), an.ErrorText())
	}

	an = ad.NewAnalyzer(args)
	feed(an, `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash"}]}}`)
	if an.Activity() != tool.ActivitySome {
		t.Fatal("tool_use not seen")
	}

	// Model text is never read as an error.
	an = ad.NewAnalyzer(args)
	feed(an, `{"type":"result","subtype":"success","is_error":false,"result":"401 rate limit exceeded is a fine answer"}`)
	if an.ResultFailed() || an.ErrorText() != "" || an.FinalText() == "" {
		t.Fatalf("successful result misread: %q", an.ErrorText())
	}
}

func TestAnalyzerText(t *testing.T) {
	an := New().NewAnalyzer([]string{"-p", "hi"})
	feed(an, "You've hit your session limit · resets 5pm", "!some warning")
	if an.Activity() != tool.ActivityUnknown {
		t.Fatal("text output cannot show tool calls")
	}
	if !strings.Contains(an.ErrorText(), "hit your session limit") || !strings.Contains(an.ErrorText(), "some warning") {
		t.Fatalf("ErrorText = %q", an.ErrorText())
	}
}

func TestIsHeadless(t *testing.T) {
	a := New()
	for args, want := range map[string]bool{"-p hi": true, "--print": true, "--model opus": false, "-- -p": false} {
		if got := a.IsHeadless(strings.Fields(args)); got != want {
			t.Errorf("IsHeadless(%q) = %v", args, got)
		}
	}
}

func TestKeychainService(t *testing.T) {
	if KeychainService("") != "Claude Code-credentials" {
		t.Fatal("default entry")
	}
	sum := sha256.Sum256([]byte("/Users/me/.aims/profiles/claude/work"))
	if got, want := KeychainService("/Users/me/.aims/profiles/claude/work"), "Claude Code-credentials-"+hex.EncodeToString(sum[:])[:8]; got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
	// Decomposed and composed spellings of a path hash the same.
	if KeychainService("/Users/Jose\u0301") != KeychainService("/Users/Jos\u00e9") {
		t.Fatal("path is not NFC-normalized")
	}
}

func TestSeedJSON(t *testing.T) {
	src := `{"oauthAccount":{"emailAddress":"me@x"},"userID":"secret","theme":"dark",
	  "mcpServers":{"gh":{"command":"gh-mcp"}},
	  "projects":{"/work/a.b":{"allowedTools":["Bash(ls)"],"hasTrustDialogAccepted":true,"lastCost":3,"history":["secret prompt"]}}}`
	out, err := seedJSON([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, leak := range []string{"oauthAccount", "userID", "secret", "lastCost"} {
		if strings.Contains(s, leak) {
			t.Errorf("seed copied %s:\n%s", leak, s)
		}
	}
	if gjson.Get(s, "theme").String() != "dark" || !gjson.Get(s, `projects./work/a\.b.hasTrustDialogAccepted`).Bool() || !gjson.Get(s, "mcpServers.gh").Exists() {
		t.Errorf("seed lost settings:\n%s", s)
	}
	if out, _ := seedJSON([]byte(`{"oauthAccount":{}}`)); out != nil {
		t.Errorf("nothing to seed, got %s", out)
	}
}

func TestMergeMCPServers(t *testing.T) {
	src := `{"mcpServers":{"a":{"command":"a"},"b.c":{"command":"bc"}}}`
	dst := `{"z":1,"mcpServers":{"a":{"command":"mine"}}}`
	out, added, err := mergeMCPServers([]byte(src), []byte(dst))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(added, ",") != "b.c" {
		t.Fatalf("added %v", added)
	}
	if gjson.GetBytes(out, "mcpServers.a.command").String() != "mine" || !gjson.GetBytes(out, `mcpServers.b\.c`).Exists() {
		t.Fatalf("merge:\n%s", out)
	}
	if strings.Index(string(out), `"z"`) > strings.Index(string(out), `"mcpServers"`) {
		t.Error("key order changed")
	}
}

func TestStatusLine(t *testing.T) {
	cmd := "/opt/aims statusline"
	in := `{"model":"opus","statusLine":{"type":"command","command":"~/line.sh","padding":2}}`
	out, prev, err := setStatusLine([]byte(in), cmd)
	if err != nil || prev != "~/line.sh" {
		t.Fatalf("prev=%q err=%v", prev, err)
	}
	if gjson.GetBytes(out, "statusLine.command").String() != cmd || gjson.GetBytes(out, "statusLine.padding").Int() != 2 || gjson.GetBytes(out, "model").String() != "opus" {
		t.Fatalf("set:\n%s", out)
	}
	// Installing again does not chain aims to itself.
	if _, prev, _ := setStatusLine(out, cmd); prev != "" {
		t.Fatalf("aims chained to itself: %q", prev)
	}
	back, changed, err := unsetStatusLine(out, cmd, prev)
	if err != nil || !changed || gjson.GetBytes(back, "statusLine.command").String() != "~/line.sh" {
		t.Fatalf("unset: %s %v", back, err)
	}
	// Someone else's status line is left alone.
	if _, changed, _ := unsetStatusLine([]byte(in), cmd, ""); changed {
		t.Fatal("removed a status line aims did not set")
	}
	// No previous line: the key goes away.
	out, _, _ = setStatusLine(nil, `C:\Program Files\aims.exe statusline`)
	if !strings.Contains(gjson.GetBytes(out, "statusLine.command").String(), `C:\Program Files`) {
		t.Fatalf("Windows path mangled: %s", out)
	}
	back, _, _ = unsetStatusLine(out, `C:\Program Files\aims.exe statusline`, "")
	if gjson.GetBytes(back, "statusLine").Exists() {
		t.Fatalf("statusLine left behind: %s", back)
	}
	if _, _, err := setStatusLine([]byte("{nope"), cmd); err == nil {
		t.Fatal("invalid settings accepted")
	}
}

func TestParseStatusLine(t *testing.T) {
	ws := ParseStatusLine([]byte(`{"rate_limits":{"five_hour":{"used_percentage":42.26,"resets_at":1767225600},"seven_day":{"remaining":25,"limit":100}}}`))
	if len(ws) != 2 || ws[0].Label != "5h" || ws[0].Percent != 42.3 || ws[0].ResetsAt.IsZero() || ws[1].Label != "7d" || ws[1].Percent != 75 {
		t.Fatalf("windows: %+v", ws)
	}
	if ParseStatusLine([]byte(`{"model":{}}`)) != nil || ParseStatusLine([]byte("garbage")) != nil {
		t.Fatal("no rate limits should give no windows")
	}
}

func TestAccount(t *testing.T) {
	home := testutil.Sandbox(t)
	a := New()
	dir := filepath.Join(home, "p")
	h := tool.Home{Dir: dir, EnvValue: dir}
	if acct := a.Account(t.Context(), h); acct.LoggedIn == nil || *acct.LoggedIn {
		t.Fatalf("empty dir: %+v", acct)
	}
	testutil.Write(t, filepath.Join(dir, ".credentials.json"), `{"claudeAiOauth":{"subscriptionType":"max"}}`)
	testutil.Write(t, filepath.Join(dir, ".claude.json"), `{"oauthAccount":{"emailAddress":"w@x.com","organizationName":"X"}}`)
	acct := a.Account(t.Context(), h)
	if acct.LoggedIn == nil || !*acct.LoggedIn || acct.Email != "w@x.com" || acct.Plan != "max" || acct.Org != "X" {
		t.Fatalf("logged in: %+v", acct)
	}
	// The default login keeps .claude.json next to the folder, in HOME.
	testutil.Write(t, filepath.Join(home, ".claude.json"), `{"oauthAccount":{"emailAddress":"me@x.com"}}`)
	if acct := a.Account(t.Context(), tool.Home{Dir: filepath.Join(home, ".claude")}); acct.Email != "me@x.com" {
		t.Fatalf("default login: %+v", acct)
	}
	if acct := a.Account(t.Context(), tool.Home{Dir: dir, ProfileEnv: map[string]string{"ANTHROPIC_API_KEY": "k"}}); acct.Method != "token" {
		t.Fatalf("API key profile: %+v", acct)
	}
}

// A text-mode answer that talks about limits is not an error message.
func TestTextAnswerIsNotAnError(t *testing.T) {
	an := New().NewAnalyzer([]string{"-p", "explain 429s"})
	feed(an, "A 429 means you hit a rate limit.", "Back off and retry.", "Or ask for a higher quota exceeded threshold.")
	if an.ErrorText() != "" {
		t.Fatalf("answer read as an error: %q", an.ErrorText())
	}
}
