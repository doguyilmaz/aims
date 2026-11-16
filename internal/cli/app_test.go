package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/doguyilmaz/aims/internal/desktop"
)

func TestAppStatusDoesNotCreateConfiguration(t *testing.T) {
	if !desktop.Supported() {
		t.Skip("macOS command")
	}
	home := filepath.Join(t.TempDir(), "untouched")
	t.Setenv("AIMS_HOME", home)
	root := newRoot("0.4.7-dev")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"app", "status", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var status desktop.Status
	if err := json.Unmarshal(out.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.SchemaVersion != "1.0.0" {
		t.Fatal("missing status schema")
	}
	if _, err := os.Lstat(home); !os.IsNotExist(err) {
		t.Fatal("optional status changed configuration")
	}
}

func TestAppCommandsAndFlagsOnlyOnMacOS(t *testing.T) {
	root := newRoot("0.4.7-dev")
	found := false
	for _, cmd := range root.Commands() {
		if cmd.Name() == "app" {
			found = true
		}
	}
	if found != desktop.Supported() {
		t.Fatalf("app command visible=%v on unsupported platform", found)
	}
	for _, name := range []string{"init", "setup"} {
		cmd, _, err := root.Find([]string{name})
		if err != nil {
			t.Fatal(err)
		}
		if (cmd.Flags().Lookup("app") != nil) != desktop.Supported() {
			t.Fatalf("%s --app platform mismatch", name)
		}
	}
}

func TestAbsentAppIsInformational(t *testing.T) {
	check := desktopDoctorCheck(desktop.Status{})
	if check.Status != "info" || check.Command != "aims app install" {
		t.Fatalf("optional app became a problem: %+v", check)
	}
}
