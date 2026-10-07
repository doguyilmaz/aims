package tool

import (
	"regexp"
	"strings"
)

// Failure is why a run failed, when it matters to aims.
type Failure string

const (
	FailNone  Failure = ""
	FailLogin Failure = "login"
	FailLimit Failure = "limit"
)

// These are only ever applied to error output an Analyzer extracted, never to
// prompts or model text, so plain status codes are safe to match.
var (
	loginRe = regexp.MustCompile(`(?i)please run /login|oauth token (?:has expired|revoked)|invalid api key|not logged in|could not be refreshed|refresh token (?:was|has been) (?:already used|revoked)|sign in again|signing in again|log out and sign in|authentication required|run codex login|unauthori[sz]ed|\b401\b|token_expired`)
	limitRe = regexp.MustCompile(`(?i)you(?:'|’)ve hit your (?:usage |session |weekly )?limit|hit your usage limit|usage limit reached|usage limit for|limit reached|rate[ _-]?limit(?:ed| exceeded)|quota exceeded|spend cap|too many requests|\b429\b`)
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
	}
	return FailNone
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
