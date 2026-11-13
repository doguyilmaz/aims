package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/doguyilmaz/aims/internal/bridge"
	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/testutil"
)

func TestMain(m *testing.M) { testutil.Main(m) }

func TestBridgeCommandsAreReadOnlyAndDoNotHealSetup(t *testing.T) {
	home := testutil.Sandbox(t)
	for _, args := range [][]string{{"bridge", "--json"}, {"bridge", "commands", "--json"}, {"exec", "--dry-run", "--", "never-run-this"}} {
		cmd := newRoot("v1.2.3")
		var out strings.Builder
		cmd.SetOut(&out)
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if !json.Valid([]byte(out.String())) {
			t.Fatalf("not clean JSON: %q", out.String())
		}
		if _, err := os.Stat(filepath.Join(home, ".aims")); !os.IsNotExist(err) {
			t.Fatal("read-only command created aims state or setup")
		}
	}
}

func TestT3ExportRefusesLiveSettingsEvenWhenMissing(t *testing.T) {
	testutil.Sandbox(t)
	live := bridge.T3SettingsPath()
	for _, args := range [][]string{
		{"bridge", "t3", "--output", live},
		{"bridge", "t3", "--settings", live, "--output", live},
		{"bridge", "t3", "--settings", filepath.Join(t.TempDir(), "other.json"), "--output", live},
	} {
		cmd := newRoot("v1.2.3")
		cmd.SetArgs(args)
		if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "never live") {
			t.Fatalf("%v: %v", args, err)
		}
	}
	if _, err := os.Stat(live); !os.IsNotExist(err) {
		t.Fatal("created live T3 settings")
	}
}

func TestExecDryRunNeverPrintsEnvironmentOrCommandSecrets(t *testing.T) {
	testutil.Sandbox(t)
	_, err := config.Update(func(c *config.Config) error {
		c.Tools["claude"].Profiles["work"] = &config.Profile{Env: map[string]string{"ANTHROPIC_API_KEY": "PROFILE-SECRET"}}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	cmd := newRoot("v1.2.3")
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"exec", "--dry-run", "--", "never-run-this", "--token", "ARGUMENT-SECRET"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "SECRET") || !strings.Contains(out.String(), "work") {
		t.Fatalf("unsafe dry run output: %q", out.String())
	}
}
