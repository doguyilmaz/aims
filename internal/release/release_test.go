package release

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"v1.2.3", "1.2.3", 0},
		{"v1.10.0", "v1.9.9", 1},
		{"v1.2", "v1.2.1", -1},
		{"v2.0.0-rc.1", "v2.0.0", -1},
		{"v2.0.0-rc.2", "v2.0.0-rc.1", 1},
		{"v1.0.0+build", "v1.0.0", 0},
	}
	for _, c := range cases {
		if got := Compare(c.a, c.b); got != c.want {
			t.Errorf("Compare(%s, %s) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestDetect(t *testing.T) {
	cases := map[string]Method{
		"/opt/homebrew/Caskroom/aims/1.0.0/aims":                    Homebrew,
		"/home/linuxbrew/.linuxbrew/bin/aims":                       Homebrew,
		"/Applications/aims.app/Contents/MacOS/aims":                Homebrew,
		"/usr/lib/node_modules/@doguyilmaz/aims/bin/linux-x64/aims": Npm,
		"/home/me/go/bin/aims":                                      GoInstall,
		"/home/me/.local/bin/aims":                                  Script,
		"/usr/local/bin/aims":                                       Manual,
		"/opt/homebrew-like/aims":                                   Manual,
		`C:/Users/me/scoop/apps/aims/current/aims.exe`:              Scoop,
	}
	for exe, want := range cases {
		if got := Detect(exe, "/home/me/go/bin", "/home/me/.local/bin"); got != want {
			t.Errorf("Detect(%s) = %v, want %v", exe, got, want)
		}
	}
	if UpgradeCommand(Manual, "linux", "/x") != nil || len(UpgradeCommand(Homebrew, "darwin", "/x")) != 2 {
		t.Error("upgrade commands")
	}
	if got := UpgradeCommand(Homebrew, "darwin", "")[1]; got[len(got)-1] != "doguyilmaz/tap/aims" {
		t.Errorf("the cask is named in full: %q", got)
	}
	got := UpgradeCommand(Script, "linux", "/home/it's/bin")[0]
	if got[0] != "sh" || !strings.HasSuffix(got[2], `| AIMS_BIN_DIR='/home/it'\''s/bin' sh`) {
		t.Errorf("script: %q", got)
	}
	if got := UpgradeCommand(Script, "windows", `C:\a`)[0]; got[0] != "powershell" || !strings.HasPrefix(got[3], `$env:AIMS_BIN_DIR='C:\a'; irm `) {
		t.Errorf("script on windows: %q", got)
	}
}

func TestCheckerCachesADay(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	calls := 0
	reply, fail := tagPrefix+"v1.4.0", error(nil)
	c := &Checker{
		CachePath: filepath.Join(t.TempDir(), "update.json"),
		TTL:       24 * time.Hour,
		Now:       func() time.Time { return now },
		Fetch: func(context.Context) (string, error) {
			calls++
			return reply, fail
		},
	}
	if v, err := c.Latest(t.Context(), false); v != "v1.4.0" || err != nil {
		t.Fatalf("first: %s %v", v, err)
	}
	now = now.Add(time.Hour)
	if v, _ := c.Latest(t.Context(), false); v != "v1.4.0" || calls != 1 {
		t.Fatalf("cached: %s after %d calls", v, calls)
	}
	// Offline: the old answer stays and the next try waits another day.
	now = now.Add(25 * time.Hour)
	fail = errors.New("offline")
	if v, err := c.Latest(t.Context(), false); v != "v1.4.0" || err == nil {
		t.Fatalf("offline: %s %v", v, err)
	}
	if _, err := c.Latest(t.Context(), false); err != nil || calls != 2 {
		t.Fatalf("retried within a day: %d calls", calls)
	}
	// A redirect somewhere unexpected is not trusted.
	fail, reply = nil, "https://example.com/evil"
	if _, err := c.Latest(t.Context(), true); err == nil {
		t.Fatal("foreign location accepted")
	}
	if c.Cached() != "v1.4.0" {
		t.Fatal("cache overwritten by a bad answer")
	}
}
