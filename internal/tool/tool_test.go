package tool

import (
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
		"stream error: 429 Too Many Requests":                                    FailLimit,
		"rate_limit_exceeded":                                                    FailLimit,
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
