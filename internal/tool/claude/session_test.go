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
  --model <model>                   Model
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
		if got, err := a.ResumeArgs(bin, c.in, "S", "continue"); err != nil || !slices.Equal(got, c.want) {
			t.Errorf("%q:\n got %q %v\nwant %q", c.in, got, err, c.want)
		}
	}
	if got, _ := a.ResumeArgs(bin, []string{"hi"}, "S", ""); !slices.Equal(got, []string{"--resume", "S"}) {
		t.Errorf("no prompt: %q", got)
	}
	// Without the help, a prompt cannot be told from an option's value.
	if _, err := a.ResumeArgs(filepath.Join(dir, "missing"), []string{"--model", "opus"}, "S", ""); err == nil {
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
	got, err := withSettings([]string{"--model", "opus"}, shellQuote("/opt/it's/aims")+" hook claude")
	if err != nil || got[0] != "--settings" || got[2] != "--model" {
		t.Fatalf("prepended: %q %v", got, err)
	}
	if cmd := gjson.Get(got[1], "hooks.StopFailure.0.hooks.0.command").String(); cmd != `'/opt/it'\''s/aims' hook claude` {
		t.Fatalf("command: %s", cmd)
	}

	file := filepath.Join(t.TempDir(), "s.json")
	testutil.Write(t, file, `{"model":"haiku","hooks":{"Stop":[{"hooks":[{"type":"command","command":"mine"}]}]}}`)
	got, err = withSettings([]string{"--settings", file, "hi"}, "'aims' hook claude")
	if err != nil || len(got) != 3 {
		t.Fatalf("merged: %q %v", got, err)
	}
	j := gjson.Parse(got[1])
	if j.Get("model").String() != "haiku" || j.Get("hooks.Stop.0.hooks.0.command").String() != "mine" || j.Get("hooks.Stop.1.hooks.0.command").String() != "'aims' hook claude" {
		t.Fatalf("user settings lost: %s", got[1])
	}
	got, _ = withSettings([]string{`--settings={"x":1}`}, "'aims' hook claude")
	if j := gjson.Parse(got[0][len("--settings="):]); j.Get("x").Int() != 1 || !j.Get("hooks.Stop").Exists() {
		t.Fatalf("inline: %q", got)
	}
	if _, err := withSettings([]string{"--settings", "/missing.json"}, "aims"); err == nil {
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
