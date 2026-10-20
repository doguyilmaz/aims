package codex

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/doguyilmaz/aims/internal/testutil"
	"github.com/doguyilmaz/aims/internal/tool"
)

func TestSupervises(t *testing.T) {
	a := New()
	for _, args := range [][]string{nil, {"fix it"}, {"-m", "gpt-5", "resume", "--last"}, {"fork"}, {"-c", "exec=1"}} {
		if !a.Supervises(args) {
			t.Errorf("%q is a conversation", args)
		}
	}
	for _, args := range [][]string{{"exec", "hi"}, {"-m", "o5", "mcp", "list"}, {"login", "status"}, {"--version"}} {
		if a.Supervises(args) {
			t.Errorf("%q is not a conversation", args)
		}
	}
}

func TestResumeArgs(t *testing.T) {
	a := New()
	cases := []struct{ in, want []string }{
		{nil, []string{"resume", "S", "--", "continue"}},
		{[]string{"-m", "gpt-5", "--search", "fix it"}, []string{"-m", "gpt-5", "--search", "resume", "S", "--", "continue"}},
		{[]string{"resume", "--last", "-s", "read-only"}, []string{"-s", "read-only", "resume", "S", "--", "continue"}},
		{[]string{"-C", "dir", "resume", "old-id", "go on"}, []string{"-C", "dir", "resume", "S", "--", "continue"}},
		{[]string{"fork", "--all", "--", "-x"}, []string{"resume", "S", "--", "continue"}},
	}
	for _, c := range cases {
		if got, _ := a.ResumeArgs("", c.in, "S", "continue"); !slices.Equal(got, c.want) {
			t.Errorf("%q:\n got %q\nwant %q", c.in, got, c.want)
		}
	}
	if got, _ := a.ResumeArgs("", []string{"hi"}, "S", ""); !slices.Equal(got, []string{"resume", "S"}) {
		t.Errorf("no prompt: %q", got)
	}
	if got, _ := a.ResumeArgs("", []string{"-m", "o5", "resume", "--last"}, "", ""); !slices.Equal(got, []string{"-m", "o5"}) {
		t.Errorf("a new conversation: %q", got)
	}
}

func TestTurn(t *testing.T) {
	cases := []struct {
		line string
		ok   bool
		f    tool.Failure
	}{
		{`{"type":"event_msg","payload":{"type":"task_complete","last_agent_message":"done"}}`, true, tool.FailNone},
		{`{"type":"event_msg","payload":{"type":"task_complete","error":{"message":"You've hit your usage limit.","codex_error_info":"usage_limit_exceeded"}}}`, true, tool.FailLimit},
		{`{"type":"event_msg","payload":{"type":"task_complete","error":{"message":"x","codex_error_info":"unauthorized"}}}`, true, tool.FailLogin},
		{`{"type":"event_msg","payload":{"type":"task_complete","error":{"message":"Your refresh token was already used","codex_error_info":"other"}}}`, true, tool.FailLogin},
		{`{"type":"event_msg","payload":{"type":"task_complete","error":{"message":"overloaded","codex_error_info":"server_overloaded"}}}`, false, ""},
		{`{"type":"event_msg","payload":{"type":"task_complete","error":{"message":"down","codex_error_info":{"http_connection_failed":{"http_status_code":502}}}}}`, false, ""},
		{`{"type":"event_msg","payload":{"type":"task_aborted"}}`, false, ""},
		{`{"type":"response_item","payload":{}}`, false, ""},
	}
	for _, c := range cases {
		ev, _, ok := turn([]byte(c.line))
		if ok != c.ok || (ok && ev.Failure != c.f) {
			t.Errorf("%s: %v %+v", c.line, ok, ev)
		}
	}
	ev, at, ok := turn([]byte(`{"type":"event_msg","payload":{"type":"task_started","started_at":1792449061}}`))
	if !ok || !ev.Started || at.Unix() != 1792449061 {
		t.Errorf("started: %v %+v %v", ok, ev, at)
	}
}

// The follower finds the rollout of this run's conversation, not another
// directory's, and reports only what was written after the run started.
func TestFollow(t *testing.T) {
	home := t.TempDir()
	now := time.Now()
	day := filepath.Join(home, "sessions", now.Format("2006/01/02"))
	write := func(name, content string) string {
		p := filepath.Join(day, name)
		testutil.Write(t, p, content)
		return p
	}
	meta := func(id, cwd string, at time.Time) string {
		return `{"type":"session_meta","payload":{"id":"` + id + `","cwd":"` + cwd + `","originator":"codex-tui","thread_source":"user","timestamp":"` + at.UTC().Format(time.RFC3339Nano) + `"}}` + "\n"
	}
	limit := `{"type":"event_msg","payload":{"type":"task_complete","error":{"message":"limit","codex_error_info":"usage_limit_exceeded"}}}` + "\n"
	done := `{"type":"event_msg","payload":{"type":"task_complete"}}` + "\n"

	_, poll, _ := New().Watch(tool.Watch{Home: tool.Home{Dir: home}, Dir: "/work", Since: now})
	if evs := poll(); len(evs) != 0 {
		t.Fatalf("nothing yet: %+v", evs)
	}
	write("rollout-a-other.jsonl", meta("other", "/elsewhere", now)+limit)
	if evs := poll(); len(evs) != 0 {
		t.Fatalf("another directory's conversation: %+v", evs)
	}
	p := write("rollout-b-mine.jsonl", meta("mine", "/work", now)+limit)
	// Another directory's conversation, written later, does not hide it.
	later := write("rollout-c-later.jsonl", meta("later", "/elsewhere", now))
	future := now.Add(time.Second)
	_ = os.Chtimes(later, future, future)
	if evs := poll(); len(evs) != 1 || evs[0].Session != "mine" || evs[0].Failure != tool.FailLimit {
		t.Fatalf("mine: %+v", evs)
	}

	// A resumed conversation is read from where it was.
	args, poll, _ := New().Watch(tool.Watch{Args: []string{"resume", "mine"}, Home: tool.Home{Dir: home}, Dir: "/work", Since: now})
	if !slices.Equal(args[:2], noDaemon) {
		t.Fatalf("daemon left on: %q", args)
	}
	f, _ := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0o644)
	_, _ = f.WriteString(done)
	f.Close()
	if evs := poll(); len(evs) != 1 || evs[0].Session != "mine" || evs[0].Failure != tool.FailNone {
		t.Fatalf("resumed: %+v", evs)
	}

	// An older conversation in the same directory is not this run's.
	_, poll, _ = New().Watch(tool.Watch{Home: tool.Home{Dir: home}, Dir: "/work", Since: now.Add(time.Hour)})
	f, _ = os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0o644)
	_, _ = f.WriteString(limit)
	f.Close()
	_ = os.Chtimes(p, now.Add(2*time.Hour), now.Add(2*time.Hour))
	if evs := poll(); len(evs) != 0 {
		t.Fatalf("an older conversation: %+v", evs)
	}
}

// A fork's copy of its parent's turns is not read as new ones; exec runs and
// subagents in the same directory are not the run's conversation; and two
// conversations that both could be it leave the run unfollowed.
func TestFollowOnly(t *testing.T) {
	home := t.TempDir()
	now := time.Now().Truncate(time.Second)
	day := filepath.Join(home, "sessions", now.Format("2006/01/02"))
	meta := func(id, originator, source string, at time.Time) string {
		return `{"type":"session_meta","payload":{"id":"` + id + `","cwd":"/work","originator":"` + originator + `","thread_source":"` + source + `","timestamp":"` + at.UTC().Format(time.RFC3339Nano) + `"}}` + "\n"
	}
	done := func(at time.Time) string {
		return `{"type":"event_msg","payload":{"type":"task_complete","completed_at":` + strconv.FormatInt(at.Unix(), 10) + `}}` + "\n"
	}
	limit := func(at time.Time) string {
		return `{"type":"event_msg","payload":{"type":"task_complete","completed_at":` + strconv.FormatInt(at.Unix(), 10) + `,"error":{"message":"x","codex_error_info":"usage_limit_exceeded"}}}` + "\n"
	}
	watch := func(args ...string) func() []tool.SessionEvent {
		_, poll, _ := New().Watch(tool.Watch{Args: args, Home: tool.Home{Dir: home}, Dir: "/work", Since: now})
		return poll
	}

	poll := watch("fork", "S")
	testutil.Write(t, filepath.Join(day, "rollout-1-exec.jsonl"), meta("exec", "codex_exec", "user", now)+limit(now))
	testutil.Write(t, filepath.Join(day, "rollout-2-sub.jsonl"), meta("sub", "codex-tui", "subagent", now)+limit(now))
	testutil.Write(t, filepath.Join(day, "rollout-3-fork.jsonl"), meta("fork", "codex-tui", "user", now)+limit(now.Add(-time.Hour))+done(now.Add(time.Second)))
	if evs := poll(); len(evs) != 1 || evs[0].Failure != tool.FailNone || evs[0].Session != "fork" {
		t.Fatalf("fork: %+v", evs)
	}

	poll = watch()
	testutil.Write(t, filepath.Join(day, "rollout-4-other.jsonl"), meta("other", "codex-tui", "user", now)+limit(now))
	if evs := poll(); len(evs) != 0 {
		t.Fatalf("two candidates: %+v", evs)
	}
}

// resume --last finds an old conversation through a linked sessions folder.
func TestFollowLinkedSessions(t *testing.T) {
	hub, home := t.TempDir(), t.TempDir()
	now := time.Now()
	old := filepath.Join(hub, "sessions", "2020", "01", "02", "rollout-x-old.jsonl")
	testutil.Write(t, old, `{"type":"session_meta","payload":{"id":"old","cwd":"/work","originator":"codex-tui","thread_source":"user","timestamp":"2020-01-02T00:00:00Z"}}`+"\n")
	if err := os.Symlink(filepath.Join(hub, "sessions"), filepath.Join(home, "sessions")); err != nil {
		t.Skip(err)
	}
	_, poll, _ := New().Watch(tool.Watch{Args: []string{"resume", "--last"}, Home: tool.Home{Dir: home}, Dir: "/work", Since: now})
	appendLine := func(l string) {
		f, _ := os.OpenFile(old, os.O_APPEND|os.O_WRONLY, 0o644)
		_, _ = f.WriteString(l + "\n")
		f.Close()
	}
	// The run's first write (the prompt) shows which conversation it opened.
	appendLine(`{"type":"response_item","payload":{}}`)
	if evs := poll(); len(evs) != 0 {
		t.Fatalf("history replayed: %+v", evs)
	}
	appendLine(`{"type":"event_msg","payload":{"type":"task_complete","completed_at":` + strconv.FormatInt(now.Unix()+1, 10) + `}}`)
	if evs := poll(); len(evs) != 1 || evs[0].Session != "old" {
		t.Fatalf("linked: %+v", evs)
	}
}
