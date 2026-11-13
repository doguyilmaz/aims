package statusline

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/doguyilmaz/aims/internal/config"
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
