package ops

import (
	"slices"
	"testing"

	"github.com/doguyilmaz/aims/internal/config"
)

func TestExplain(t *testing.T) {
	cfg := &config.Config{Failover: config.Failover{Auto: true}}
	ready := func(n string) ProfileStatus { return ProfileStatus{Name: n, Usable: true} }
	out := func(n, why string) ProfileStatus { return ProfileStatus{Name: n, Reasons: []string{why}} }
	report := []ToolStatus{
		{ID: "claude", Installed: true, Active: "personal", Profiles: []ProfileStatus{ready("personal"), out("work", "7d at 98% for 3d")}},
		{ID: "codex", Installed: true, Active: "personal", Profiles: []ProfileStatus{ready("personal"), ready("work")}},
	}
	want := []string{
		"claude and codex start on personal. At a limit or a dead login the conversation continues on work, and goes back to personal once it works again.",
		"claude/work cannot be used now (7d at 98% for 3d); a claude session at its limit has nowhere to move.",
		"aims claude@work keeps a session on work and never moves it.",
	}
	if got := Explain(cfg, report); !slices.Equal(got, want) {
		t.Fatalf("got %q", got)
	}

	report[0].Profiles = []ProfileStatus{out("personal", "5h at 100% for 2h"), ready("work")}
	report[1].Active, report[1].Profiles = "work", []ProfileStatus{ready("work")}
	cfg.Failover.Auto = false
	want = []string{
		"claude starts on personal and stays there; auto moves are off.",
		"codex starts on work, the only account; at a limit a session waits for the reset.",
		"claude/personal cannot be used now (5h at 100% for 2h).",
		"aims claude@work keeps a session on work and never moves it.",
	}
	if got := Explain(cfg, report); !slices.Equal(got, want) {
		t.Fatalf("got %q", got)
	}
}
