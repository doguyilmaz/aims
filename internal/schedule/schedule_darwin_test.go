package schedule

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestPlist(t *testing.T) {
	j := Job{Name: "keepalive", Command: []string{"/Applications/aims.app/Contents/MacOS/aims", "keepalive"}, Every: time.Hour, Env: map[string]string{"PATH": "/a:/b"}}
	if strings.Contains(plist(j), "AssociatedBundleIdentifiers") {
		t.Fatal("a job outside an app names none")
	}
	j.App = "com.doguyilmaz.aims"
	if !strings.Contains(plist(j), "<key>AssociatedBundleIdentifiers</key>\n\t<array>\n\t\t<string>com.doguyilmaz.aims</string>") {
		t.Fatalf("app: %s", plist(j))
	}
	// Another shell's PATH is no reason to set the job up again.
	other := j
	other.Env = map[string]string{"PATH": "/c"}
	if envBlock.ReplaceAllString(plist(j), "") != envBlock.ReplaceAllString(plist(other), "") {
		t.Fatal("the environment counts as a change")
	}
	other.Command = []string{"/opt/homebrew/bin/aims", "keepalive"}
	if envBlock.ReplaceAllString(plist(j), "") == envBlock.ReplaceAllString(plist(other), "") {
		t.Fatal("a new program must count as a change")
	}
}

func TestHeal(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	j := Job{Name: "keepalive", Command: []string{"/Applications/aims.app/Contents/MacOS/aims", "keepalive"}, Every: time.Hour, Env: map[string]string{"PATH": "/a"}}
	if healed, err := Heal(j); !healed || err != nil {
		t.Fatalf("a job Homebrew removed comes back: %v %v", healed, err)
	}
	j.Env = map[string]string{"PATH": "/b"}
	if healed, _ := Heal(j); healed {
		t.Fatal("another PATH is no reason to set it up again")
	}
	j.App = "com.doguyilmaz.aims"
	if healed, _ := Heal(j); !healed {
		t.Fatal("a job that moved into the app is set up again")
	}
	if b, _ := os.ReadFile(plistPath(j)); !strings.Contains(string(b), "AssociatedBundleIdentifiers") {
		t.Fatalf("plist: %s", b)
	}
}
