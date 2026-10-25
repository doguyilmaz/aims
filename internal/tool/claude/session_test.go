package claude

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/tidwall/gjson"

	"github.com/doguyilmaz/aims/internal/testutil"
	"github.com/doguyilmaz/aims/internal/tool"
)

const help = `Usage: claude [options] [command] [prompt]

Options:
  --add-dir <directories...>        Additional directories
  --allowedTools, --allowed-tools <tools...>  Tools
  -c, --continue                    Continue the most recent conversation
  -d, --debug [filter]              Enable debug mode
  --dangerously-skip-permissions    Bypass permissions
  --effort <level>                  Effort
  --model <model>                   Model
  --permission-mode <mode>          Permission mode (choices: "acceptEdits", "auto", "plan")
  -n, --name <name>                 Name
  -r, --resume [value]              Resume a conversation
  -w, --worktree [name]             Worktree
`

func TestResumeArgs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stands in for claude with a shell script")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "claude")
	testutil.Write(t, filepath.Join(dir, "help"), help)
	testutil.Write(t, bin, "#!/bin/sh\ncat "+filepath.Join(dir, "help")+"\n")
	_ = os.Chmod(bin, 0o755)

	a := New()
	cases := []struct{ in, want []string }{
		{nil, []string{"--resume", "S", "--", "continue"}},
		{[]string{"fix the bug"}, []string{"--resume", "S", "--", "continue"}},
		{[]string{"--model", "opus", "fix", "--dangerously-skip-permissions"}, []string{"--model", "opus", "--dangerously-skip-permissions", "--resume", "S", "--", "continue"}},
		{[]string{"--add-dir", "a", "b", "--", "-p looks like a flag"}, []string{"--add-dir", "a", "b", "--resume", "S", "--", "continue"}},
		{[]string{"-r", "old", "-c", "--model=haiku", "-d", "api", "-n", "x"}, []string{"--model=haiku", "-d", "api", "--resume", "S", "--", "continue"}},
		{[]string{"--resume", "--allowed-tools", "Bash", "Edit", "-d"}, []string{"--allowed-tools", "Bash", "Edit", "-d", "--resume", "S", "--", "continue"}},
	}
	for _, c := range cases {
		if got, err := a.ResumeArgs(bin, c.in, tool.Resume{ID: "S", Prompt: "continue"}); err != nil || !slices.Equal(got, c.want) {
			t.Errorf("%q:\n got %q %v\nwant %q", c.in, got, err, c.want)
		}
	}
	if got, _ := a.ResumeArgs(bin, []string{"hi"}, tool.Resume{ID: "S"}); !slices.Equal(got, []string{"--resume", "S"}) {
		t.Errorf("no prompt: %q", got)
	}
	if got, _ := a.ResumeArgs(bin, []string{"--model", "opus", "hi"}, tool.Resume{}); !slices.Equal(got, []string{"--model", "opus"}) {
		t.Errorf("a new conversation: %q", got)
	}
	// A resumed conversation goes on as it was at its last turn: the model
	// and effort of its last answer, the mode it was in, not the ones it
	// started with. Bypassing permissions stays with the start's flags.
	transcript := filepath.Join(dir, "t.jsonl")
	testutil.Write(t, transcript, `{"type":"assistant","requestedModel":"claude-opus-5-5","effort":"low"}
{"type":"assistant","isSidechain":true,"requestedModel":"claude-haiku-5-5","effort":"max"}
{"type":"user"}
{"type":"assistant","isApiErrorMessage":true,"message":{"model":"<synthetic>"}}
`)
	start := []string{"hi", "--model", "sonnet", "--effort=high", "--permission-mode", "plan", "--add-dir", "a"}
	got, _ := a.ResumeArgs(bin, start, tool.Resume{ID: "S", Transcript: transcript, Mode: "auto"})
	if want := []string{"--add-dir", "a", "--model", "claude-opus-5-5", "--effort", "low", "--permission-mode", "auto", "--resume", "S"}; !slices.Equal(got, want) {
		t.Errorf("as at the last turn:\n got %q\nwant %q", got, want)
	}
	// A mode carried over turns off bypassing permissions from the start.
	got, _ = a.ResumeArgs(bin, []string{"--dangerously-skip-permissions"}, tool.Resume{ID: "S", Mode: "plan"})
	if want := []string{"--permission-mode", "plan", "--resume", "S"}; !slices.Equal(got, want) {
		t.Errorf("bypass turned off:\n got %q\nwant %q", got, want)
	}
	// A mode this claude does not list stays with the start's flags.
	got, _ = a.ResumeArgs(bin, start, tool.Resume{ID: "S", Mode: "default"})
	if want := []string{"--model", "sonnet", "--effort=high", "--permission-mode", "plan", "--add-dir", "a", "--resume", "S"}; !slices.Equal(got, want) {
		t.Errorf("unknown mode:\n got %q\nwant %q", got, want)
	}
	got, _ = a.ResumeArgs(bin, start, tool.Resume{ID: "S", Mode: "bypassPermissions"})
	if want := []string{"--model", "sonnet", "--effort=high", "--permission-mode", "plan", "--add-dir", "a", "--resume", "S"}; !slices.Equal(got, want) {
		t.Errorf("bypass, no transcript:\n got %q\nwant %q", got, want)
	}
	// Without the help, a prompt cannot be told from an option's value.
	if _, err := a.ResumeArgs(filepath.Join(dir, "missing"), []string{"--model", "opus"}, tool.Resume{ID: "S"}); err == nil {
		t.Error("a missing claude must fail")
	}
}

func TestSupervises(t *testing.T) {
	a := New()
	for _, args := range [][]string{nil, {"fix it"}, {"--model", "opus"}, {"-c"}} {
		if !a.Supervises(args) {
			t.Errorf("%q is a conversation", args)
		}
	}
	for _, args := range [][]string{{"-p", "hi"}, {"mcp", "list"}, {"auth", "status"}, {"-w"}, {"--worktree", "x"}, {"--version"}, {"--bg"}} {
		if a.Supervises(args) {
			t.Errorf("%q is not followed", args)
		}
	}
}

func TestWithSettings(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "merged.json")
	read := func() gjson.Result {
		b, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		return gjson.ParseBytes(b)
	}
	got, err := withSettings([]string{"--model", "opus"}, shellQuote("/opt/it's/aims")+" hook claude", out)
	if err != nil || !slices.Equal(got, []string{"--settings", out, "--model", "opus"}) {
		t.Fatalf("prepended: %q %v", got, err)
	}
	want := `'/opt/it'\''s/aims' hook claude`
	if runtime.GOOS == "windows" {
		want = `"/opt/it's/aims" hook claude`
	}
	if cmd := read().Get("hooks.StopFailure.0.hooks.0.command").String(); cmd != want {
		t.Fatalf("command: %s", cmd)
	}
	if fi, _ := os.Stat(out); runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Fatalf("settings readable by others: %v", fi.Mode())
	}

	user := filepath.Join(dir, "s.json")
	testutil.Write(t, user, `{"model":"haiku","hooks":{"Stop":[{"hooks":[{"type":"command","command":"mine"}]}]}}`)
	got, err = withSettings([]string{"--settings", user, "hi"}, "'aims' hook claude", out)
	if err != nil || !slices.Equal(got, []string{"--settings", out, "hi"}) {
		t.Fatalf("merged: %q %v", got, err)
	}
	if j := read(); j.Get("model").String() != "haiku" || j.Get("hooks.Stop.0.hooks.0.command").String() != "mine" || j.Get("hooks.Stop.1.hooks.0.command").String() != "'aims' hook claude" {
		t.Fatalf("user settings lost: %s", j.Raw)
	}
	got, _ = withSettings([]string{`--settings={"x":1}`}, "'aims' hook claude", out)
	if j := read(); got[0] != "--settings="+out || j.Get("x").Int() != 1 || !j.Get("hooks.Stop").Exists() {
		t.Fatalf("inline: %q %s", got, j.Raw)
	}
	if _, err := withSettings([]string{"--settings", "/missing.json"}, "aims", out); err == nil {
		t.Fatal("a missing settings file must fail")
	}
}

func TestHookEvent(t *testing.T) {
	a := New()
	ev, ok := a.HookEvent([]byte(`{"hook_event_name":"StopFailure","session_id":"S","error":"rate_limit","last_assistant_message":"You've hit your limit","error_details":"429"}`))
	if !ok || ev.Failure != tool.FailLimit || ev.Session != "S" || ev.Text != "You've hit your limit\n429" {
		t.Fatalf("limit: %+v", ev)
	}
	if ev, _ := a.HookEvent([]byte(`{"hook_event_name":"StopFailure","session_id":"S","error":"rate_limit","error_details":"429 Too Many Requests"}`)); ev.Failure != tool.FailBusy {
		t.Fatalf("a bare rate limit is short: %+v", ev)
	}
	if ev, _ := a.HookEvent([]byte(`{"hook_event_name":"StopFailure","session_id":"S","error":"authentication_failed"}`)); ev.Failure != tool.FailLogin {
		t.Fatalf("login: %+v", ev)
	}
	if _, ok := a.HookEvent([]byte(`{"hook_event_name":"StopFailure","session_id":"S","error":"model_not_found","last_assistant_message":"no such model"}`)); ok {
		t.Fatal("an error another account cannot fix is no event")
	}
	ev, ok = a.HookEvent([]byte(`{"hook_event_name":"Stop","session_id":"S","last_assistant_message":"401 unauthorized is a status code"}`))
	if !ok || ev.Failure != tool.FailNone || ev.Text != "" {
		t.Fatalf("a turn that went well is not read for errors: %+v", ev)
	}
	if ev, ok := a.HookEvent([]byte(`{"hook_event_name":"SessionStart","session_id":"S","transcript_path":"/t/S.jsonl"}`)); !ok || !ev.Opened || ev.Transcript != "/t/S.jsonl" {
		t.Fatalf("opened: %+v", ev)
	}
	if ev, ok := a.HookEvent([]byte(`{"hook_event_name":"UserPromptSubmit","session_id":"S","prompt":"hi"}`)); !ok || !ev.Started {
		t.Fatalf("a prompt starts a turn: %+v", ev)
	}
	if _, ok := a.HookEvent([]byte(`{"hook_event_name":"PreToolUse","session_id":"S"}`)); ok {
		t.Fatal("other hooks are not turns")
	}
	if _, ok := a.HookEvent([]byte(`not json`)); ok {
		t.Fatal("garbage")
	}
}

func TestDiagnose(t *testing.T) {
	dir := t.TempDir()
	if got := New().Diagnose(tool.Home{Dir: dir}); len(got) != 0 {
		t.Fatalf("no settings: %q", got)
	}
	testutil.Write(t, filepath.Join(dir, "settings.json"), `{"disableAllHooks": true}`)
	if got := New().Diagnose(tool.Home{Dir: dir}); len(got) != 1 {
		t.Fatalf("hooks off: %q", got)
	}
}
