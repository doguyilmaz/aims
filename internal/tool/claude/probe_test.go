package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/tidwall/gjson"

	"github.com/doguyilmaz/aims/internal/testutil"
	"github.com/doguyilmaz/aims/internal/tool"
)

func TestProbeKeepsCheapControlsLocalToItsChild(t *testing.T) {
	for _, kind := range []string{"profile-environment", "inherited-environment", "relative-binary"} {
		t.Run(kind, func(t *testing.T) {
			home := testutil.Sandbox(t)
			testutil.Fakes(t)
			dir := filepath.Join(home, ".claude")
			settings := filepath.Join(dir, "settings.json")
			original := `{"model":"opus","effortLevel":"max"}`
			testutil.Write(t, settings, original)
			testutil.Write(t, filepath.Join(dir, ".credentials.json"), `{"claudeAiOauth":{"accessToken":"fake"}}`)
			capture := filepath.Join(home, "probe.json")
			parent := map[string]string{
				"CLAUDE_CODE_EFFORT_LEVEL": "max", "MAX_THINKING_TOKENS": "32000",
				"CLAUDE_CODE_MAX_OUTPUT_TOKENS": "32000", "CLAUDE_CODE_MAX_RETRIES": "10",
				"CLAUDE_CODE_RETRY_WATCHDOG": "1", "CLAUDE_CONFIG_DIR": dir,
				"FAKE_CLAUDE_CAPTURE": capture,
			}
			for k, v := range parent {
				t.Setenv(k, v)
			}
			var env []string
			if kind != "inherited-environment" {
				env = os.Environ()
			}
			before := slices.Clone(env)
			cwd, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			h := tool.Home{Dir: dir, Bin: os.Getenv("AIMS_CLAUDE_BIN"), Env: env}
			if kind == "relative-binary" {
				h.Bin, err = filepath.Rel(cwd, h.Bin)
				if err != nil {
					t.Skipf("relative executable crosses volumes: %v", err)
				}
			}
			if p := New().Probe(t.Context(), h); p.Status != tool.ProbeOK {
				t.Fatalf("fake login check failed: %+v", p)
			}
			var child struct {
				Args []string          `json:"args"`
				Env  map[string]string `json:"env"`
				Dir  string            `json:"dir"`
			}
			data, err := os.ReadFile(capture)
			if err != nil || json.Unmarshal(data, &child) != nil {
				t.Fatalf("fake child did not record its launch: %v", err)
			}
			for key, want := range map[string]string{
				"CLAUDE_CODE_EFFORT_LEVEL": "low", "MAX_THINKING_TOKENS": "0",
				"CLAUDE_CODE_MAX_OUTPUT_TOKENS": "64", "CLAUDE_CODE_MAX_RETRIES": "0",
				"CLAUDE_CODE_RETRY_WATCHDOG": "", "CLAUDE_CONFIG_DIR": dir,
			} {
				if child.Env[key] != want {
					t.Errorf("child %s = %q, want %q", key, child.Env[key], want)
				}
			}
			for flag, want := range map[string]string{"--model": "haiku", "--max-turns": "1", "--tools": "", "--output-format": "json"} {
				if i := slices.Index(child.Args, flag); i < 0 || i+1 >= len(child.Args) || child.Args[i+1] != want {
					t.Errorf("wrong %s in %q", flag, child.Args)
				}
			}
			for _, flag := range []string{"--no-session-persistence", "--strict-mcp-config"} {
				if !slices.Contains(child.Args, flag) {
					t.Errorf("missing %s", flag)
				}
			}
			raw := tool.OptionValue(child.Args, "--settings", "")
			if !gjson.Get(raw, "disableAllHooks").Bool() {
				t.Fatal("probe can run user hooks")
			}
			if gjson.Get(raw, "effortLevel").String() != "low" || gjson.Get(raw, "env.CLAUDE_CODE_EFFORT_LEVEL").String() != "low" ||
				gjson.Get(raw, "env.CLAUDE_CODE_MAX_OUTPUT_TOKENS").String() != "64" {
				t.Fatal("saved settings can raise probe effort or output limits")
			}
			if len(child.Args) < 2 || !slices.Equal(child.Args[len(child.Args)-2:], []string{"--", "Reply with just: OK"}) {
				t.Fatalf("probe resumed or sent a different prompt: %q", child.Args)
			}
			if child.Dir == "" || child.Dir == cwd {
				t.Fatal("probe inherited the caller's working directory")
			}
			if _, err := os.Stat(child.Dir); !os.IsNotExist(err) {
				t.Fatalf("probe temporary working directory remained: %v", err)
			}
			if after, _ := os.Getwd(); after != cwd || !slices.Equal(h.Env, before) {
				t.Fatal("probe changed the parent's directory or profile environment")
			}
			for key, want := range parent {
				if os.Getenv(key) != want {
					t.Errorf("probe changed parent %s", key)
				}
			}
			data, err = os.ReadFile(settings)
			if err != nil || string(data) != original {
				t.Fatal("probe changed saved conversation model/effort settings")
			}
		})
	}
}
