package profiles

import (
	"testing"

	"github.com/doguyilmaz/aims/internal/proc"
)

func TestWindowsProfileEnvironmentOverridesMixedCaseNames(t *testing.T) {
	cfg, a := setup(t)
	p := cfg.Tools["codex"].Profiles["a"]
	p.Env = map[string]string{"Path": `C:\profile\tools`, "codex_sqlite_home": ""}
	env, _ := Env(cfg, a, "a", []string{`Path=C:\original\tools`, "openai_api_key=ambient-key"})
	if value, _ := proc.EnvValue(env, "PATH"); value != `C:\profile\tools` {
		t.Fatalf("profile PATH lost: %v", env)
	}
	if value, ok := proc.EnvValue(env, "OPENAI_API_KEY"); ok && value != "" {
		t.Fatal("mixed-case login override survived")
	}
	if StateShared(cfg, a, "a") {
		t.Fatal("mixed-case explicit database override ignored")
	}
	p.Env["codex_home"] = `C:\another-home`
	if value, ok := EnvOverride(p, "CODEX_HOME"); !ok || value != `C:\another-home` {
		t.Fatal("doctor cannot see mixed-case home override")
	}
}
