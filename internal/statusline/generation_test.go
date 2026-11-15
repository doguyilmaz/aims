package statusline

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/fsx"
	"github.com/doguyilmaz/aims/internal/testutil"
)

func TestStatuslineRejectsOldSessionAfterProfileRecreation(t *testing.T) {
	for _, token := range []string{"old", "", "current"} {
		t.Run("token-"+token, func(t *testing.T) {
			testutil.Sandbox(t)
			cfg, err := config.Update(func(c *config.Config) error {
				c.EnsureHub("claude")
				c.Tools["claude"].Profiles["work"] = &config.Profile{Generation: "current", Share: config.ShareNone}
				c.Tools["claude"].Order = []string{"work"}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Setenv("CLAUDE_CONFIG_DIR", cfg.Dir("claude", "work"))
			if token != "" {
				t.Setenv(config.GenerationVar("claude"), token)
			}
			path := filepath.Join(t.TempDir(), "input.json")
			testutil.Write(t, path, `{"rate_limits":{"five_hour":{"used_percentage":42.26}}}`)
			input, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer input.Close()
			previous := os.Stdin
			os.Stdin = input
			defer func() { os.Stdin = previous }()
			var output bytes.Buffer
			if err := Run(t.Context(), &output); err != nil {
				t.Fatal(err)
			}
			cached := config.LoadState().Get("claude", "work").Usage
			if token == "current" {
				if cached == nil {
					t.Fatal("current session lost usage")
				}
			} else if cached != nil || !strings.Contains(output.String(), "profile changed") {
				t.Fatalf("stale session was accepted: %+v %q", cached, output.String())
			}
		})
	}
}

func TestRunSelectsOnlyOriginatingHomeChain(t *testing.T) {
	testutil.Sandbox(t)
	cfg, err := config.Update(func(c *config.Config) error {
		c.EnsureHub("claude")
		for _, n := range []string{"personal", "work"} {
			c.Tools["claude"].Profiles[n] = &config.Profile{Share: config.ShareNone}
			c.Tools["claude"].Order = append(c.Tools["claude"].Order, n)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"personal", "work"} {
		testutil.Write(t, filepath.Join(cfg.Dir("claude", n), "placeholder"), "")
	}
	_, err = config.Update(func(c *config.Config) error {
		c.Status.Chain = "echo GLOBAL"
		c.Status.HomeChains = map[string]string{fsx.Real(c.Dir("claude", "personal")): "echo PERSONAL", fsx.Real(c.Dir("claude", "work")): "echo WORK"}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"personal", "work"} {
		t.Setenv("CLAUDE_CONFIG_DIR", cfg.Dir("claude", n))
		path := filepath.Join(t.TempDir(), "stdin.json")
		testutil.Write(t, path, "{}")
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		old := os.Stdin
		os.Stdin = f
		var out bytes.Buffer
		err = Run(t.Context(), &out)
		os.Stdin = old
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		want := strings.ToUpper(n)
		other := "PERSONAL"
		if n == "personal" {
			other = "WORK"
		}
		if !strings.Contains(out.String(), want) || strings.Contains(out.String(), other) || strings.Contains(out.String(), "GLOBAL") {
			t.Fatalf("cross-home chain selection: %q", out.String())
		}
	}
}

func TestRemovedRegistrationDoesNotRunSavedHomeCommand(t *testing.T) {
	testutil.Sandbox(t)
	cfg, err := config.Update(func(c *config.Config) error {
		c.EnsureHub("claude")
		c.Tools["claude"].Profiles["work"] = &config.Profile{Generation: "old", Share: config.ShareNone}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	home := cfg.Dir("claude", "work")
	testutil.Write(t, filepath.Join(home, "placeholder"), "")
	_, err = config.Update(func(c *config.Config) error {
		c.Status.HomeChains = map[string]string{fsx.Real(home): "echo STALE-SAVED-COMMAND"}
		delete(c.Tools["claude"].Profiles, "work")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAUDE_CONFIG_DIR", home)
	t.Setenv(config.GenerationVar("claude"), "old")
	path := filepath.Join(t.TempDir(), "input.json")
	testutil.Write(t, path, "{}")
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	previous := os.Stdin
	os.Stdin = f
	defer func() { os.Stdin = previous }()
	var out bytes.Buffer
	if err := Run(t.Context(), &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "STALE-SAVED-COMMAND") || !strings.Contains(out.String(), "profile changed") {
		t.Fatalf("removed profile still ran saved command: %q", out.String())
	}
}
