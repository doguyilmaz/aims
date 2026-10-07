// Package testutil keeps tests away from the state of the machine they run on.
package testutil

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/doguyilmaz/aims/internal/testutil/fake"
)

// cleared are variables that would make aims or a tool read real state.
var cleared = []string{
	"AIMS_HOME", "AIMS_CLAUDE_PROFILE", "AIMS_CODEX_PROFILE", "AIMS_SESSION_PROFILE",
	"CLAUDE_CONFIG_DIR", "CODEX_HOME",
	"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "CLAUDE_CODE_OAUTH_TOKEN", "CODEX_API_KEY", "OPENAI_API_KEY",
	"ZDOTDIR", "XDG_CONFIG_HOME", "XDG_CACHE_HOME", "SHELL", "NO_COLOR",
}

// Missing tool binaries: a test that forgets to install a fake finds no
// tool rather than the real one on PATH.
var missing = map[string]string{
	"AIMS_CLAUDE_BIN": "aims-test-no-claude",
	"AIMS_CODEX_BIN":  "aims-test-no-codex",
}

// Main runs a package's tests with HOME in a temporary directory, so a test
// that does not call Sandbox still cannot reach the real one. Started as
// claude or codex (see Fakes), the test binary is that fake instead.
func Main(m *testing.M) {
	switch strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe") {
	case "claude":
		os.Exit(fake.Claude())
	case "codex":
		os.Exit(fake.Codex())
	}
	dir, err := os.MkdirTemp("", "aims-test-")
	if err != nil {
		panic(err)
	}
	isolate(dir, os.Setenv, os.Unsetenv)
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// Fakes puts the fake claude and codex in place of the real ones for this
// test. The package's TestMain must call Main.
func Fakes(t testing.TB) {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for _, name := range []string{"claude", "codex"} {
		bin := filepath.Join(dir, name)
		if runtime.GOOS == "windows" {
			bin += ".exe"
			b, err := os.ReadFile(self)
			if err == nil {
				err = os.WriteFile(bin, b, 0o755)
			}
			if err != nil {
				t.Fatal(err)
			}
		} else if err := os.Symlink(self, bin); err != nil {
			t.Fatal(err)
		}
		t.Setenv("AIMS_"+strings.ToUpper(name)+"_BIN", bin)
	}
}

// Sandbox gives one test a fresh, empty HOME and returns it.
func Sandbox(t testing.TB) string {
	t.Helper()
	home := t.TempDir()
	set := func(k, v string) error { t.Setenv(k, v); return nil }
	unset := func(k string) error { t.Setenv(k, ""); return os.Unsetenv(k) }
	isolate(home, set, unset)
	return home
}

func isolate(home string, set func(k, v string) error, unset func(k string) error) {
	_ = set("HOME", home)
	_ = set("USERPROFILE", home)
	_ = set("AIMS_NO_UPDATE_CHECK", "1")
	for _, k := range cleared {
		_ = unset(k)
	}
	for k, v := range missing {
		_ = set(k, v)
	}
}

// Write creates a file and its parent directories, failing the test on error.
func Write(t testing.TB, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Read returns a file's content, failing the test on error.
func Read(t testing.TB, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
