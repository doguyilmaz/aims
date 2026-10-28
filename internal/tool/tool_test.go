package tool

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestClassify(t *testing.T) {
	cases := map[string]Failure{
		"":                                    FailNone,
		"Invalid API key · Please run /login": FailLogin,
		"OAuth token has expired. Please obtain a new token or refresh your existing token.": FailLogin,
		"401 Unauthorized": FailLogin,
		"Your refresh token was already used. Please log out and sign in again.": FailLogin,
		"You've hit your session limit · resets 5pm":                             FailLimit,
		"You’ve hit your weekly limit":                                           FailLimit,
		"stream error: 429 Too Many Requests":                                    FailBusy,
		"rate_limit_exceeded":                                                    FailBusy,
		"API Error: 429 {\"type\":\"rate_limit_error\"}":                         FailBusy,
		"5-hour limit reached ∙ resets 3pm":                                      FailLimit,
		"Claude AI usage limit reached|1767225600":                               FailLimit,
		"Error: ENOENT: no such file or directory":                               FailNone,
		"connection reset by peer":                                               FailNone,
		"port 14290 in use":                                                      FailNone,
	}
	for in, want := range cases {
		if got := Classify(in); got != want {
			t.Errorf("Classify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWindows(t *testing.T) {
	for label, short := range map[string]bool{"5h": true, "300m": true, "7d": false, "24h": false, "": false, "opus": false} {
		if got := (Window{Label: label}).Short(); got != short {
			t.Errorf("Short(%q) = %v", label, got)
		}
	}
	cases := map[string]string{
		"You've hit your session limit · resets 1:10am (Europe/Istanbul)": "5h",
		"5-hour limit reached ∙ resets 3pm":                               "5h",
		"You’ve hit your weekly limit":                                    "7d",
		"Opus weekly limit reached ∙ resets Oct 9, 11am":                  "7d",
		"Claude AI usage limit reached|1767225600":                        "",
		"You've hit your usage limit. Try again in 2 days 3 hours":        "",
		// Only the first line that reports a limit counts.
		"Approaching your weekly allowance\nYou've hit your session limit · resets 5pm": "5h",
		"You've hit your session limit · resets 5pm\nApproaching weekly limit":          "5h",
		"Claude AI usage limit reached|1767225600\nApproaching weekly limit":            "",
	}
	for in, want := range cases {
		if got := LimitWindow(in); got != want {
			t.Errorf("LimitWindow(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFlags(t *testing.T) {
	args := []string{"-p", "--output-format=json", "--model", "opus", "--", "--model", "x"}
	if v := OptionValue(args, "--output-format", "text"); v != "json" {
		t.Errorf("--output-format = %q", v)
	}
	if v := OptionValue(args, "--model", ""); v != "opus" {
		t.Errorf("--model = %q", v)
	}
	if v := OptionValue([]string{"--", "--model", "x"}, "--model", "def"); v != "def" {
		t.Errorf("value after -- was read: %q", v)
	}
	if !HasFlag(args, "-p") || HasFlag([]string{"--", "-p"}, "-p") {
		t.Error("HasFlag must stop at --")
	}
}

func TestParseTime(t *testing.T) {
	want := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	for _, v := range []any{float64(want.Unix()), float64(want.UnixMilli()), "2026-05-01T12:00:00Z", want.Unix()} {
		if got := ParseTime(v); !got.Equal(want) {
			t.Errorf("ParseTime(%v) = %v", v, got)
		}
	}
	for _, v := range []any{nil, float64(0), "soon", float64(-5)} {
		if got := ParseTime(v); !got.IsZero() {
			t.Errorf("ParseTime(%v) = %v, want zero", v, got)
		}
	}
}

func TestTail(t *testing.T) {
	tl := Tail{Max: 2}
	for _, l := range []string{"a", " ", "b", "c"} {
		tl.Add(l)
	}
	if tl.String() != "b\nc" {
		t.Errorf("tail = %q", tl.String())
	}
}

func TestCommandError(t *testing.T) {
	if e := (&CommandError{Output: "noise\nreal problem\n", Code: 1}); e.Error() != "real problem" {
		t.Errorf("Error() = %q", e.Error())
	}
	if e := (&CommandError{Code: 3}); e.Error() != "exit status 3" {
		t.Errorf("Error() = %q", e.Error())
	}
}

func TestLayoutEntries(t *testing.T) {
	l := Layout{Shared: Names("projects"), History: Names("projects")}
	if !l.IsShared("projects") || l.IsShared(".credentials.json") || !l.IsHistory("projects") {
		t.Error("Layout matching is wrong")
	}
}

func TestResetTime(t *testing.T) {
	loc := time.FixedZone("TRT", 3*3600)
	now := time.Date(2026, 6, 1, 10, 30, 0, 0, loc)
	old := time.Local
	time.Local = loc
	defer func() { time.Local = old }()
	cases := map[string]time.Time{
		"Claude AI usage limit reached|1780312500":                       time.Unix(1780312500, 0),
		"You've hit your limit · resets 5pm (Europe/Istanbul)":           time.Date(2026, 6, 1, 17, 0, 0, 0, loc),
		"5-hour limit reached ∙ resets 9:15am":                           time.Date(2026, 6, 2, 9, 15, 0, 0, loc),
		"You've hit your usage limit. Try again at 3:14 PM.":             time.Date(2026, 6, 1, 15, 14, 0, 0, loc),
		"You've hit your usage limit. Try again in 2 days 3 hours 5 min": now.Add(51*time.Hour + 5*time.Minute),
		"You’ve hit your usage limit. Upgrade to Pro (https://chatgpt.com/explore/pro), visit https://chatgpt.com/settings/usage to purchase more credits or try again at Nov 2nd, 2026 3:14 PM.": time.Date(2026, 11, 2, 15, 14, 0, 0, loc),
		"Opus weekly limit reached ∙ resets Oct 9, 11am":                   time.Date(2026, 10, 9, 11, 0, 0, 0, loc),
		"Weekly limit reached ∙ resets Jan 3, 9:30am (Europe/Istanbul)":    time.Date(2027, 1, 3, 9, 30, 0, 0, loc),
		"You've hit your usage limit. Try again at May 2nd, 2026 3:14 PM.": {}, // in the past
		"You've hit your usage limit.":                                     {},
		"Claude AI usage limit reached|1000000000":                         {}, // in the past
	}
	for in, want := range cases {
		if got := ResetTime(in, now); !got.Equal(want) {
			t.Errorf("ResetTime(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestFollowEvents(t *testing.T) {
	dir := t.TempDir()
	_ = RecordEvent(dir, SessionEvent{Session: "stale"})
	poll := FollowEvents(dir)
	if evs := poll(); len(evs) != 0 {
		t.Fatalf("a new run starts empty: %+v", evs)
	}
	_ = RecordEvent(dir, SessionEvent{Session: "one"})
	_ = RecordEvent(dir, SessionEvent{Session: "two", Failure: FailLimit})
	if evs := poll(); len(evs) != 2 || evs[1].Failure != FailLimit {
		t.Fatalf("events: %+v", evs)
	}
	if evs := poll(); len(evs) != 0 {
		t.Fatalf("read twice: %+v", evs)
	}
}

// TailLines keeps a line the window starts exactly on, and drops one it
// starts inside.
func TestTailLines(t *testing.T) {
	p := filepath.Join(t.TempDir(), "t.jsonl")
	if err := os.WriteFile(p, []byte("aaaa\nbbbb\ncccc\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := TailLines(p, 10); len(got) != 2 || string(got[0]) != "bbbb" {
		t.Errorf("on a line start: %q", got)
	}
	if got := TailLines(p, 8); len(got) != 1 || string(got[0]) != "cccc" {
		t.Errorf("inside a line: %q", got)
	}
}
