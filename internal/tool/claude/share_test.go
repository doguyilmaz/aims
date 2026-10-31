package claude

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tidwall/gjson"

	"github.com/doguyilmaz/aims/internal/tool"
)

type shareFixture struct {
	t     *testing.T
	homes []tool.Home
	base  string
	clock time.Time
}

func newShare(t *testing.T, docs ...string) *shareFixture {
	f := &shareFixture{t: t, base: filepath.Join(t.TempDir(), "claude-settings.json"), clock: time.Now().Add(-time.Hour)}
	for _, d := range docs {
		h := tool.Home{EnvValue: t.TempDir()}
		f.homes = append(f.homes, h)
		f.write(len(f.homes)-1, d)
	}
	return f
}

// write saves home i's file, each one newer than the last.
func (f *shareFixture) write(i int, doc string) {
	f.t.Helper()
	p := claudeJSON(f.homes[i])
	if err := os.WriteFile(p, []byte(doc), 0o600); err != nil {
		f.t.Fatal(err)
	}
	f.clock = f.clock.Add(time.Minute)
	if err := os.Chtimes(p, f.clock, f.clock); err != nil {
		f.t.Fatal(err)
	}
}

func (f *shareFixture) read(i int) []byte {
	f.t.Helper()
	b, err := os.ReadFile(claudeJSON(f.homes[i]))
	if err != nil {
		f.t.Fatal(err)
	}
	return b
}

func (f *shareFixture) get(i int, path string) gjson.Result {
	return gjson.GetBytes(f.read(i), path)
}

func (f *shareFixture) share() []string {
	f.t.Helper()
	changed, err := (&Adapter{}).ShareSettings(f.homes, f.base)
	if err != nil {
		f.t.Fatal(err)
	}
	return changed
}

func TestShareSettingsBringsEachAccountsAdditionsToTheOthers(t *testing.T) {
	f := newShare(t,
		`{"oauthAccount":{"emailAddress":"me@home"},"mcpServers":{"coolify":{"type":"http","url":"https://c"}},
		  "projects":{"/code/scoly.app":{"hasTrustDialogAccepted":true,"allowedTools":["Bash(ls)"],"lastCost":1}}}`,
		`{"oauthAccount":{"emailAddress":"me@work"},"mcpServers":{"jira":{"command":"jira"}},
		  "projects":{"/code/scoly.app":{"hasTrustDialogAccepted":false,"lastCost":2}}}`)
	if len(f.share()) == 0 {
		t.Fatal("reported no change")
	}
	for i := range 2 {
		if !f.get(i, "mcpServers.coolify").Exists() || !f.get(i, "mcpServers.jira").Exists() {
			t.Errorf("home %d servers: %s", i, f.get(i, "mcpServers").Raw)
		}
	}
	work := `projects.\/code\/scoly\.app`
	if !f.get(1, work+".hasTrustDialogAccepted").Bool() || f.get(1, work+".allowedTools.0").String() != "Bash(ls)" {
		t.Errorf("trust or allowed tools did not reach work:\n%s", f.read(1))
	}
	if f.get(1, "oauthAccount.emailAddress").String() != "me@work" || f.get(1, work+".lastCost").Int() != 2 {
		t.Errorf("an account or private field changed:\n%s", f.read(1))
	}
	before := string(f.read(1))
	if changed := f.share(); len(changed) != 0 || string(f.read(1)) != before {
		t.Errorf("a second run changed %v", changed)
	}
}

func TestShareSettingsSpreadsARemovalButNeverTakesTrustBack(t *testing.T) {
	f := newShare(t,
		`{"mcpServers":{"coolify":{"url":"c"},"jira":{"url":"j"}},"projects":{"/p":{"hasTrustDialogAccepted":true,"allowedTools":["A","B"]}}}`,
		`{"mcpServers":{}}`)
	f.share()
	f.write(0, `{"mcpServers":{"jira":{"url":"j"}},"projects":{"/p":{"hasTrustDialogAccepted":false,"allowedTools":["B"]}}}`)
	f.share()
	for i := range 2 {
		if f.get(i, "mcpServers.coolify").Exists() || !f.get(i, "mcpServers.jira").Exists() {
			t.Errorf("home %d servers: %s", i, f.get(i, "mcpServers").Raw)
		}
		if got := f.get(i, `projects.\/p.allowedTools`).Raw; got != `["B"]` {
			t.Errorf("home %d allowed tools %s", i, got)
		}
		if !f.get(i, `projects.\/p.hasTrustDialogAccepted`).Bool() {
			t.Errorf("home %d lost trust", i)
		}
	}
}

func TestShareSettingsNewerEditWins(t *testing.T) {
	f := newShare(t, `{"mcpServers":{"a":{"url":"1"}}}`, `{}`)
	f.share()
	f.write(0, `{"mcpServers":{"a":{"url":"home"}}}`)
	f.write(1, `{"mcpServers":{"a":{"url":"work"}}}`)
	f.share()
	for i := range 2 {
		if got := f.get(i, "mcpServers.a.url").String(); got != "work" {
			t.Errorf("home %d has %q", i, got)
		}
	}
}

func TestShareSettingsWaitsOutClaudeCodesLock(t *testing.T) {
	defer func(w time.Duration) { configLockWait = w }(configLockWait)
	configLockWait = 50 * time.Millisecond
	f := newShare(t, `{"mcpServers":{"a":{"url":"a"}}}`, `{"mcpServers":{"b":{"url":"b"}}}`)
	lock := claudeJSON(f.homes[1]) + ".lock"
	if err := os.Mkdir(lock, 0o755); err != nil {
		t.Fatal(err)
	}
	f.share()
	if f.get(1, "mcpServers.a").Exists() {
		t.Fatal("wrote a file Claude Code holds locked")
	}
	if !f.get(0, "mcpServers.b").Exists() {
		t.Error("the locked home's server did not reach the other")
	}
	if _, err := os.Stat(lock); err != nil {
		t.Error("removed a lock aims did not take")
	}
	os.Remove(lock)
	f.share()
	for i := range 2 {
		if !f.get(i, "mcpServers.a").Exists() || !f.get(i, "mcpServers.b").Exists() {
			t.Errorf("home %d after the lock: %s", i, f.get(i, "mcpServers").Raw)
		}
	}
}

func TestShareSettingsSkipsAHomeWithoutAFile(t *testing.T) {
	f := newShare(t, `{"mcpServers":{"a":{}}}`)
	f.homes = append(f.homes, tool.Home{EnvValue: t.TempDir()})
	f.share()
	if _, err := os.Stat(claudeJSON(f.homes[1])); err == nil {
		t.Error("created a login file")
	}
}
