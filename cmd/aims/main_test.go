package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"

	"github.com/doguyilmaz/aims/internal/cli"
	"github.com/doguyilmaz/aims/internal/testutil/fake"
)

// The scripts run aims as a subprocess next to stand-ins for claude and codex
// (see internal/testutil/fake), each in its own empty HOME.
func TestMain(m *testing.M) {
	testscript.Main(m, map[string]func(){
		"aims":   func() { os.Exit(cli.Execute("v1.2.3")) },
		"claude": func() { os.Exit(fake.Claude()) },
		"codex":  func() { os.Exit(fake.Codex()) },
	})
}

func TestScripts(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir:                 "testdata/script",
		RequireExplicitExec: true,
		UpdateScripts:       os.Getenv("UPDATE_SCRIPTS") != "",
		Setup: func(e *testscript.Env) error {
			home := filepath.Join(e.WorkDir, "home")
			e.Setenv("HOME", home)
			e.Setenv("USERPROFILE", home)
			e.Setenv("USER", "tester")
			e.Setenv("SHELL", "/bin/zsh")
			e.Setenv("AIMS_NO_UPDATE_CHECK", "1")
			e.Setenv("FAKE_SIDE_EFFECTS", filepath.Join(e.WorkDir, "side-effects"))
			return os.MkdirAll(home, 0o755)
		},
	})
}
