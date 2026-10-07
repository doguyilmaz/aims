package cli

import (
	"strings"
	"testing"
)

func TestRewrite(t *testing.T) {
	cases := map[string]string{
		"claude@work -p hi": "run claude@work -p hi",
		"codex:personal":    "run codex:personal",
		"status":            "status",
		"gemini@work":       "gemini@work",
		"claude@-x":         "claude@-x",
		"":                  "",
	}
	for in, want := range cases {
		if got := strings.Join(rewrite(strings.Fields(in)), " "); got != want {
			t.Errorf("rewrite(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestReleaseTag(t *testing.T) {
	for v, want := range map[string]bool{
		"v1.2.3": true, "v0.1.0-rc.1": true, "v1.2.3-beta": true,
		"v0.0.0-20260101000000-abcdef123456": false, "v1.2.3+dirty": false, "(devel)": false,
	} {
		if releaseTag.MatchString(v) != want {
			t.Errorf("releaseTag(%q) = %v", v, !want)
		}
	}
}

func TestHelpListsEveryCommand(t *testing.T) {
	root := newRoot("v1.0.0")
	var b strings.Builder
	renderHelp(&b, root)
	for _, c := range root.Commands() {
		if c.IsAvailableCommand() && !strings.Contains(b.String(), c.Name()) {
			t.Errorf("help is missing %s", c.Name())
		}
		if c.Short == "" {
			t.Errorf("%s has no description", c.Name())
		}
	}
	if strings.Contains(b.String(), "statusline") {
		t.Error("hidden command listed")
	}
}
