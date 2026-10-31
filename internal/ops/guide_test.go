package ops

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/testutil"
)

func TestOutlook(t *testing.T) {
	ready := func(n string) ProfileStatus { return ProfileStatus{Name: n, Usable: true} }
	out := func(n string, in time.Duration) ProfileStatus {
		return ProfileStatus{Name: n, ReadyAt: time.Now().Add(in)}
	}
	for _, c := range []struct {
		profiles []ProfileStatus
		auto     bool
		want     string
	}{
		{[]ProfileStatus{ready("personal"), ready("work")}, true, ""},
		{[]ProfileStatus{ready("personal"), out("work", 3*time.Hour)}, true, "a session at its limit has nowhere to move"},
		{[]ProfileStatus{out("personal", 2*time.Hour), ready("work")}, true, "new sessions start on work"},
		{[]ProfileStatus{out("personal", 2*time.Hour), ready("work")}, false, ""},
		{[]ProfileStatus{out("personal", 2*time.Hour+30*time.Second), out("work", 72*time.Hour)}, false, "no account works now; sessions wait 2h"},
	} {
		ts := ToolStatus{Active: "personal", Profiles: c.profiles}
		if got := ts.Outlook(c.auto); got != c.want {
			t.Errorf("%+v: got %q, want %q", c.profiles, got, c.want)
		}
	}
}

// A profile whose writer locks are still its own, and one with an unused
// index copy, each get a tip.
func TestSharingTips(t *testing.T) {
	testutil.Sandbox(t)
	cfg, err := config.Update(func(c *config.Config) error {
		for _, n := range []string{"a", "b"} {
			c.Tools["codex"].Profiles[n] = &config.Profile{}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	hub := cfg.EnsureHub("codex")
	testutil.Write(t, filepath.Join(hub, "thread_history_1.sqlite"), "shared")
	testutil.Write(t, filepath.Join(cfg.Dir("codex", "a"), "thread-writer-locks", "x.lock"), "")
	testutil.Write(t, filepath.Join(cfg.Dir("codex", "b"), "thread_history_1.sqlite"), strings.Repeat("x", 2<<20))
	tips := sharingTips(cfg)
	if len(tips) != 2 || !strings.Contains(tips[0].Text, "codex/a takes no part in moves") || !strings.Contains(tips[1].Text, "codex/b keeps 2 MB of unused database copies") {
		t.Fatalf("tips: %+v", tips)
	}
}
