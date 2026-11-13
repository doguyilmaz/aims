package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/spf13/cobra"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/testutil"
)

func TestDoctorFixKeepsPendingPurgeUnavailableWithoutRecreatingItsHome(t *testing.T) {
	testutil.Sandbox(t)
	cfg, err := config.Update(func(c *config.Config) error {
		c.EnsureHub("codex")
		c.Tools["codex"].Profiles["work"] = &config.Profile{
			Share: config.ShareNone, Purge: &config.PurgeState{LoggedOut: true},
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&output)
	if err := runDoctorCheck(cmd, "0.4.7-dev", true, true); err == nil {
		t.Fatal("pending purge was reported as healthy")
	}
	var report doctorReport
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, check := range report.Checks {
		if check.ID == "codex/work/purge" && check.Command == "aims rm codex work --purge" {
			found = true
		}
	}
	if !found {
		t.Fatal("doctor did not explain how to resume the explicit purge")
	}
	if _, err := os.Stat(cfg.Dir("codex", "work")); !os.IsNotExist(err) {
		t.Fatal("doctor recreated the pending profile home")
	}
}
