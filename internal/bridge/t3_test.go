package bridge

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/testutil"
	"github.com/doguyilmaz/aims/internal/tool"
)

func TestT3ExportPreservesSettingsAndNeverCopiesLogins(t *testing.T) {
	cfg := setup(t)
	settings := filepath.Join(t.TempDir(), "settings.json")
	const original = `{"unknownFutureSetting":{"keep":[1,true,"yes"]},"defaultModelSelection":{"instanceId":"managed"},"providerInstances":{"managed":{"driver":"codex","displayName":"My T3 account","config":{"setupMode":"managed"},"secretRef":"KEEP-PRIVATE-REFERENCE","environment":[{"name":"OPENAI_API_KEY","value":"EXISTING-INLINE-SECRET"}]}}}`
	testutil.Write(t, settings, original)
	for _, a := range []string{"claude", "codex"} {
		testutil.Write(t, filepath.Join(cfg.Dir(tool.ID(a), "other"), "auth.json"), "NEVER-COPY-A-LOGIN")
	}
	before := snapshot(t, config.Home())
	p, err := PlanT3(cfg, settings)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Add) != 4 || len(p.Reuse) != 0 {
		t.Fatalf("managed account incorrectly reused: %+v", p)
	}
	public, _ := json.Marshal(p)
	if strings.Contains(string(public), "KEEP-PRIVATE-REFERENCE") || strings.Contains(string(public), "NEVER-COPY-A-LOGIN") || strings.Contains(string(public), "EXISTING-INLINE-SECRET") {
		t.Fatal("inspection disclosed credentials or existing settings")
	}
	b, err := p.Merged()
	if err != nil {
		t.Fatal(err)
	}
	var doc, orig map[string]any
	_ = json.Unmarshal(b, &doc)
	_ = json.Unmarshal([]byte(original), &orig)
	for _, k := range []string{"unknownFutureSetting", "defaultModelSelection"} {
		if !reflect.DeepEqual(doc[k], orig[k]) {
			t.Fatalf("changed %s", k)
		}
	}
	if !reflect.DeepEqual(doc["providerInstances"].(map[string]any)["managed"], orig["providerInstances"].(map[string]any)["managed"]) {
		t.Fatal("modified existing managed account")
	}
	if testutil.Read(t, settings) != original || !reflect.DeepEqual(before, snapshot(t, config.Home())) {
		t.Fatal("planning modified settings or profiles")
	}
	// Replanning the exported document is idempotent, including absent CLIs.
	file := filepath.Join(t.TempDir(), "export.json")
	if err := WriteNew(file, b); err != nil {
		t.Fatal(err)
	}
	p2, err := PlanT3(cfg, file)
	if err != nil || len(p2.Add) != 0 || len(p2.Reuse) != 4 {
		t.Fatalf("second export: %v %+v", err, p2)
	}
}

func TestT3EnvironmentPreservesDefaultKeychainAndPrivateHistory(t *testing.T) {
	cfg := setup(t)
	cfg.Tools["codex"].Profiles["other"].Share = config.ShareNone
	p, err := PlanT3(cfg, filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range p.Instances {
		m := map[string]string{}
		for _, v := range i.Environment {
			m[v.Name] = v.Value
		}
		if i.Driver == "claudeAgent" && strings.HasSuffix(i.DisplayName, "/ default") {
			if i.Config["homePath"] != "" || m["CLAUDE_CONFIG_DIR"] != "" {
				t.Fatal("export created a different default keychain identity")
			}
			if _, ok := m["CLAUDE_CONFIG_DIR"]; !ok {
				t.Fatal("export did not clear an inherited Claude home")
			}
		}
		if i.Driver == "codex" && strings.HasSuffix(i.DisplayName, "/ other") {
			if m["CODEX_SQLITE_HOME"] != "" || i.Config["shadowHomePath"] != "" || i.Config["setupMode"] != "existing" {
				t.Fatal("private profile exported into shared history or managed auth")
			}
		}
	}
}

func TestT3ReusesOnlyExactDirectLogin(t *testing.T) {
	cfg := setup(t)
	for _, tc := range []struct {
		name, driver, home, mode string
		reuse                    bool
	}{
		{"same", "claudeAgent", cfg.Dir("claude", "other"), "", true},
		{"managed", "codex", cfg.Dir("codex", "other"), "managed", false},
		{"explicit-default-is-another-keychain", "claudeAgent", cfg.EnsureHub("claude"), "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, _ := json.Marshal(map[string]any{"providerInstances": map[string]any{"mine": map[string]any{
				"driver": tc.driver, "config": map[string]string{"homePath": tc.home, "setupMode": tc.mode},
			}}})
			file := filepath.Join(t.TempDir(), "settings.json")
			testutil.Write(t, file, string(b))
			p, err := PlanT3(cfg, file)
			if err != nil {
				t.Fatal(err)
			}
			if slices.Contains(p.Reuse, "mine") != tc.reuse {
				t.Fatalf("reuse: %v", p.Reuse)
			}
		})
	}
}

func TestT3RefusesMalformedInputAndIDCollisions(t *testing.T) {
	cfg := setup(t)
	for _, doc := range []string{"null", "[]", "{} trailing", `{"providerInstances":null}`, `{"providerInstances":[]}`} {
		file := filepath.Join(t.TempDir(), "settings.json")
		testutil.Write(t, file, doc)
		if _, err := PlanT3(cfg, file); err == nil {
			t.Fatalf("accepted %q", doc)
		}
		if testutil.Read(t, file) != doc {
			t.Fatal("malformed settings rewritten")
		}
	}
	p, err := PlanT3(cfg, filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "settings.json")
	b, _ := json.Marshal(map[string]any{"providerInstances": map[string]any{p.Add[0]: map[string]string{"driver": "another"}}})
	testutil.Write(t, file, string(b))
	if _, err := PlanT3(cfg, file); err == nil || !strings.Contains(err.Error(), "different settings") {
		t.Fatalf("reserved instance overwritten: %v", err)
	}
}

func TestT3RefusesCustomEnvironmentWithoutLeakingIt(t *testing.T) {
	cfg := setup(t)
	cfg.Tools["codex"].Profiles["other"].Env = map[string]string{"CODEX_API_KEY": "PRIVATE-TOKEN"}
	_, err := PlanT3(cfg, filepath.Join(t.TempDir(), "missing.json"))
	if err == nil || strings.Contains(err.Error(), "PRIVATE-TOKEN") {
		t.Fatalf("unsafe export: %v", err)
	}
	cfg.Tools["codex"].Profiles["other"].Env = nil
	cfg.Tools["claude"].HubEnv = "~/some-home"
	if _, err := PlanT3(cfg, filepath.Join(t.TempDir(), "missing.json")); err == nil || !strings.Contains(err.Error(), "keychain") {
		t.Fatalf("changed exact keychain identity: %v", err)
	}
}

func TestExportNeverClobbersAndConcurrentExportsHaveOneWinner(t *testing.T) {
	testutil.Sandbox(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "export.json")
	var successes atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if WriteNew(file, []byte("complete export")) == nil {
				successes.Add(1)
			}
		})
	}
	wg.Wait()
	if successes.Load() != 1 || testutil.Read(t, file) != "complete export" {
		t.Fatal("concurrent exports clobbered one another")
	}
	fi, _ := os.Stat(file)
	if os.PathSeparator == '/' && fi.Mode().Perm() != 0o600 {
		t.Fatal("export is not private")
	}
	if os.PathSeparator != '/' {
		return
	}
	for _, target := range []string{file, filepath.Join(dir, "missing")} {
		link := filepath.Join(t.TempDir(), "link")
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		if err := WriteNew(link, []byte("bad")); err == nil {
			t.Fatal("export followed a symlink")
		}
	}
	if testutil.Read(t, file) != "complete export" {
		t.Fatal("symlink target changed")
	}
}
