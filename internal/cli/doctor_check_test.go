package cli

import (
	"crypto/sha256"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/testutil"
)

func doctorTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			out[path], err = os.Readlink(path)
		} else if !d.IsDir() {
			b, e := os.ReadFile(path)
			if e != nil {
				return e
			}
			sum := sha256.Sum256(b)
			out[path] = string(sum[:])
		} else {
			out[path] = "directory"
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestDoctorCheckIsReadOnlyAndSecretFree(t *testing.T) {
	home := testutil.Sandbox(t)
	cfg, err := config.Update(func(c *config.Config) error {
		c.Tools["codex"].Profiles["work"] = &config.Profile{Env: map[string]string{"OPENAI_API_KEY": "DUMMY-PROFILE-SECRET"}}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	testutil.Write(t, filepath.Join(cfg.Dir("codex", "work"), "auth.json"), "DUMMY-LOGIN-SECRET")
	testutil.Write(t, filepath.Join(cfg.Dir("codex", "work"), "config.toml"), "independent settings")
	before := doctorTree(t, home)
	for _, args := range [][]string{{"doctor"}, {"doctor", "--check"}, {"doctor", "--check", "--json"}} {
		cmd := newRoot("v1.2.3")
		var out strings.Builder
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(args)
		if err := cmd.Execute(); err == nil {
			t.Fatal("missing sharing was reported healthy")
		}
		if strings.Contains(out.String(), "SECRET") {
			t.Fatalf("doctor exposed credentials: %s", out.String())
		}
		if args[len(args)-1] == "--json" {
			var r doctorReport
			if err := json.Unmarshal([]byte(out.String()), &r); err != nil || r.SchemaVersion != "1.0.0" || r.Mode != "check" || len(r.Repairs) != 0 {
				t.Fatalf("invalid report: %s (%v)", out.String(), err)
			}
		}
		if after := doctorTree(t, home); !reflect.DeepEqual(before, after) {
			t.Fatal("doctor check wrote to the filesystem")
		}
	}
}

func TestDoctorFixKeepsIndependentDataAndIsRepeatable(t *testing.T) {
	home := testutil.Sandbox(t)
	cfg, err := config.Update(func(c *config.Config) error {
		c.Tools["codex"].Profiles["work"] = &config.Profile{}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	dir, hub := cfg.Dir("codex", "work"), cfg.EnsureHub("codex")
	for _, base := range []string{dir, hub} {
		testutil.Write(t, filepath.Join(base, "config.toml"), base)
		testutil.Write(t, filepath.Join(base, "goals_1.sqlite"), base+" database")
	}
	testutil.Write(t, filepath.Join(dir, "auth.json"), "DUMMY-LOGIN-SECRET")
	var first map[string]string
	for i := 0; i < 2; i++ {
		cmd := newRoot("v1.2.3")
		var out strings.Builder
		cmd.SetOut(&out)
		cmd.SetArgs([]string{"doctor", "--fix", "--json"})
		if err := cmd.Execute(); err == nil {
			t.Fatal("unresolved independent files reported healthy")
		}
		var report doctorReport
		if err := json.Unmarshal([]byte(out.String()), &report); err != nil {
			t.Fatalf("invalid fix report: %s (%v)", out.String(), err)
		}
		if report.Mode != "fix" {
			t.Fatal("wrong mode")
		}
		for _, base := range []string{dir, hub} {
			if testutil.Read(t, filepath.Join(base, "config.toml")) != base || testutil.Read(t, filepath.Join(base, "goals_1.sqlite")) != base+" database" {
				t.Fatal("doctor reconciled independent data")
			}
		}
		if testutil.Read(t, filepath.Join(dir, "auth.json")) != "DUMMY-LOGIN-SECRET" {
			t.Fatal("login changed")
		}
		if i == 0 {
			first = doctorTree(t, home)
		} else if !reflect.DeepEqual(first, doctorTree(t, home)) {
			t.Fatal("second repair changed the filesystem")
		}
	}
}

func TestDoctorFlagConflicts(t *testing.T) {
	for _, args := range [][]string{{"--fix", "--check"}, {"--logins", "--json"}, {"--logins", "--fix"}, {"--logins", "--check"}} {
		cmd := newDoctorCmd("v1.2.3")
		cmd.SetArgs(args)
		if err := cmd.Execute(); err == nil {
			t.Fatalf("accepted conflicting flags: %v", args)
		}
	}
}
