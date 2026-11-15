package usage

import (
	"os"
	"testing"
	"time"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/testutil"
)

func TestCustomRemindersAreSeparateAndDoNotOverwriteDamagedData(t *testing.T) {
	testutil.Sandbox(t)
	_, err := config.Update(func(c *config.Config) error {
		c.Tools["codex"].Profiles["personal"] = &config.Profile{}
		c.Tools["codex"].Order = []string{"personal"}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = AddCustom("codex", "personal", "Weekly check", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	s, err := Load("codex")
	if err != nil || len(s.CustomResets) != 1 {
		t.Fatalf("missing reminder: %+v %v", s, err)
	}
	if s.Providers[0].Rows[0].Resets != nil || len(s.Providers[0].Rows[0].Windows) != 0 {
		t.Fatal("reminder invented provider usage or a banked reset")
	}
	if err = RemoveCustom(s.CustomResets[0].ID); err != nil {
		t.Fatal(err)
	}
	broken := []byte(`{"schemaVersion":1,"resets":[`)
	if err = os.WriteFile(customPath(), broken, 0o600); err != nil {
		t.Fatal(err)
	}
	if err = AddCustom("codex", "personal", "Keep this", time.Now().Add(time.Hour)); err == nil {
		t.Fatal("damaged reminders were accepted")
	}
	got, _ := os.ReadFile(customPath())
	if string(got) != string(broken) {
		t.Fatal("damaged reminders were overwritten")
	}
}
