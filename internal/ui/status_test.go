package ui

import "testing"

func TestShortPlan(t *testing.T) {
	for in, want := range map[string]string{
		"self_serve_business_prolite": "business", "team": "team", "pro": "pro", "max": "max",
		"plus": "plus", "enterprise": "enterprise", "": "", "something_very_long": "something…",
	} {
		if got := ShortPlan(in); got != want {
			t.Errorf("ShortPlan(%q) = %q, want %q", in, got, want)
		}
	}
}
