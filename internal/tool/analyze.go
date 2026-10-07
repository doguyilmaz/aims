package tool

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Failure is why a run failed, when it matters to aims.
type Failure string

const (
	FailNone  Failure = ""
	FailLogin Failure = "login"
	// FailLimit: the plan's usage limit; it lasts until the window resets.
	FailLimit Failure = "limit"
	// FailBusy: a short-lived rate limit (429, too many requests), which on
	// a team plan can be the organization's and says nothing about the plan.
	FailBusy Failure = "busy"
)

// These are only ever applied to error output an Analyzer extracted, never to
// prompts or model text, so plain status codes are safe to match.
var (
	loginRe = regexp.MustCompile(`(?i)please run /login|oauth token (?:has expired|revoked)|invalid api key|not logged in|could not be refreshed|refresh token (?:was|has been) (?:already used|revoked)|sign in again|signing in again|log out and sign in|authentication required|run codex login|unauthori[sz]ed|\b401\b|token_expired`)
	limitRe = regexp.MustCompile(`(?i)you(?:'|’)ve hit your (?:usage |session |weekly |5-hour |opus )?limit|hit your usage limit|usage limit reached|usage limit for|(?:5-hour|weekly|session|opus) limit|limit reached|quota exceeded|spend cap|credit balance is too low`)
	busyRe  = regexp.MustCompile(`(?i)rate[ _-]?limit(?:ed|[ _-]exceeded|_error)?|too many requests|\b429\b`)
)

// Classify maps provider error text to a Failure.
func Classify(text string) Failure {
	switch {
	case strings.TrimSpace(text) == "":
		return FailNone
	case loginRe.MatchString(text):
		return FailLogin
	case limitRe.MatchString(text):
		return FailLimit
	case busyRe.MatchString(text):
		return FailBusy
	}
	return FailNone
}

var (
	epochRe   = regexp.MustCompile(`\|(\d{10})\b`)
	clockRe   = regexp.MustCompile(`(?i)(?:resets?|try again)(?: at)? (\d{1,2})(?::(\d{2}))?\s*([ap]m)\b`)
	durRe     = regexp.MustCompile(`(?i)try again in ((?:\d+\s*(?:days?|d|hours?|h|minutes?|mins?|m)\b[\s,]*(?:and\s+)?)+)`)
	durPartRe = regexp.MustCompile(`(?i)(\d+)\s*(days?|d|hours?|h|minutes?|mins?|m)\b`)
)

// ResetTime reads when a limit lifts from a provider message: Claude Code's
// "usage limit reached|<epoch>" and "resets 5pm", Codex's "try again at
// 3:14 PM" and "try again in 2 days 3 hours". Zero when it says nothing.
func ResetTime(text string, now time.Time) time.Time {
	if m := epochRe.FindStringSubmatch(text); m != nil {
		if sec, err := strconv.ParseInt(m[1], 10, 64); err == nil {
			if t := time.Unix(sec, 0); t.After(now) {
				return t
			}
		}
	}
	if m := clockRe.FindStringSubmatch(text); m != nil {
		h, _ := strconv.Atoi(m[1])
		mins, _ := strconv.Atoi(m[2])
		if h >= 1 && h <= 12 && mins < 60 {
			h %= 12
			if strings.EqualFold(m[3], "pm") {
				h += 12
			}
			local := now.Local()
			t := time.Date(local.Year(), local.Month(), local.Day(), h, mins, 0, 0, local.Location())
			if !t.After(now) {
				t = t.AddDate(0, 0, 1)
			}
			return t
		}
	}
	if m := durRe.FindStringSubmatch(text); m != nil {
		var d time.Duration
		for _, p := range durPartRe.FindAllStringSubmatch(m[1], -1) {
			n, _ := strconv.Atoi(p[1])
			switch u := strings.ToLower(p[2]); {
			case strings.HasPrefix(u, "d"):
				d += time.Duration(n) * 24 * time.Hour
			case strings.HasPrefix(u, "h"):
				d += time.Duration(n) * time.Hour
			default:
				d += time.Duration(n) * time.Minute
			}
		}
		if d > 0 {
			return now.Add(d)
		}
	}
	return time.Time{}
}

// Activity says whether a run demonstrably did something with side effects.
type Activity int

const (
	// ActivityUnknown: the output format hides tool calls.
	ActivityUnknown Activity = iota
	ActivityNone
	ActivitySome
)

// Analyzer watches a headless run line by line and reports the provider's
// error and whether a tool ran, without reading the prompt or the model's text.
type Analyzer interface {
	Line(stderr bool, line string)
	// ResultFailed: a structured final result reported failure, whatever the exit code.
	ResultFailed() bool
	ErrorText() string
	Activity() Activity
	// FinalText is the answer of a structured run, or "".
	FinalText() string
}

// OptionValue returns the value of a --flag in args (before any "--").
func OptionValue(args []string, name, def string) string {
	for i, a := range args {
		if a == "--" {
			break
		}
		if a == name && i+1 < len(args) {
			return args[i+1]
		}
		if v, ok := strings.CutPrefix(a, name+"="); ok {
			return v
		}
	}
	return def
}

// HasFlag reports whether args contain flag before any "--".
func HasFlag(args []string, flag string) bool {
	for _, a := range args {
		if a == "--" {
			return false
		}
		if a == flag {
			return true
		}
	}
	return false
}

// Tail keeps the last Max non-empty lines.
type Tail struct {
	Max   int
	lines []string
}

// Add appends a line, dropping the oldest beyond Max.
func (t *Tail) Add(line string) {
	if strings.TrimSpace(line) == "" {
		return
	}
	t.lines = append(t.lines, line)
	if t.Max > 0 && len(t.lines) > t.Max {
		t.lines = t.lines[len(t.lines)-t.Max:]
	}
}

func (t *Tail) String() string { return strings.Join(t.lines, "\n") }
