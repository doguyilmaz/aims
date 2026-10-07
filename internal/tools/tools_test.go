package tools

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/doguyilmaz/aims/internal/tool"
)

// TestAdapters holds every adapter to the rules the rest of aims relies on.
// A new adapter that passes this is most of the way there.
func TestAdapters(t *testing.T) {
	idRe := regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	seen := map[string]string{}
	unique := func(kind, v string, id tool.ID) {
		if prev, ok := seen[kind+v]; ok {
			t.Errorf("%s and %s share %s %q", prev, id, kind, v)
		}
		seen[kind+v] = string(id)
	}
	for _, a := range All() {
		id := a.ID()
		l := a.Layout()
		if !idRe.MatchString(string(id)) {
			t.Errorf("%q: ids are lowercase words (they appear in tool@profile)", id)
		}
		if a.Title() == "" || l.Bin == "" || l.HomeEnv == "" || l.DefaultHome == "" || len(l.Markers) == 0 {
			t.Errorf("%s: Title, Bin, HomeEnv, DefaultHome and Markers are required", id)
		}
		if want := "AIMS_" + strings.ToUpper(string(id)) + "_BIN"; l.BinEnv != want {
			t.Errorf("%s: BinEnv is %q, want %q", id, l.BinEnv, want)
		}
		unique("id", string(id), id)
		unique("home variable", l.HomeEnv, id)
		unique("home folder", l.DefaultHome, id)

		// History must be shared to be detachable, and shared dirs must be shared.
		for _, h := range l.History {
			if h.Pattern == nil && !l.IsShared(h.Name) {
				t.Errorf("%s: history entry %q is not shared", id, h.Name)
			}
		}
		for _, d := range l.SharedDirs {
			if !l.IsShared(d) {
				t.Errorf("%s: shared dir %q is not in Shared", id, d)
			}
		}
		// Logins stay private: nothing that looks like a credential is shared.
		for _, n := range []string{"auth.json", ".credentials.json", ".claude.json", "credentials", "token.json"} {
			if l.IsShared(n) {
				t.Errorf("%s: %s must never be shared", id, n)
			}
		}

		c := a.Commands()
		if len(c.Login) == 0 || len(c.Logout) == 0 || len(c.Resume) == 0 || c.LoginTip == "" {
			t.Errorf("%s: every command is required", id)
		}
		args := a.HeadlessArgs(tool.HeadlessOptions{Prompt: "--help", Model: "m"})
		if !a.IsHeadless(args) {
			t.Errorf("%s: HeadlessArgs %q is not seen as headless", id, args)
		}
		if i := slices.Index(args, "--"); i < 0 || args[len(args)-1] != "--help" || i != len(args)-2 {
			t.Errorf("%s: the prompt must be the last argument, after --: %q", id, args)
		}
		if a.IsHeadless(nil) {
			t.Errorf("%s: no arguments is an interactive session", id)
		}
		if got, ok := Parse(strings.ToUpper(string(id))); !ok || got != a {
			t.Errorf("%s: Parse is not case-insensitive", id)
		}
	}
}
