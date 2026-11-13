package usage

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/testutil"
	"github.com/doguyilmaz/aims/internal/tool"
)

func TestMain(m *testing.M) { testutil.Main(m) }

func TestSnapshotOnlyExportsUsageAndKeepsProfileIdentity(t *testing.T) {
	testutil.Sandbox(t)
	_, err := config.Update(func(c *config.Config) error {
		for _, n := range []string{"personal", "work"} {
			c.Tools["codex"].Profiles[n] = &config.Profile{Env: map[string]string{"OPENAI_API_KEY": "fixture-secret"}}
			c.Tools["codex"].Order = append(c.Tools["codex"].Order, n)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	err = config.UpdateState("codex", "personal", func(ps *config.ProfileState) {
		ps.Account = &config.Account{Email: "private@example.test", Plan: "plus", Tenant: "private-tenant"}
		ps.Usage = &config.Usage{Windows: []tool.Window{{Label: "5h", Percent: 80}}, At: time.Now(), Resets: &tool.ResetBalance{Available: 2}}
	})
	if err != nil {
		t.Fatal(err)
	}
	s, err := Load("codex")
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Providers) != 1 || len(s.Providers[0].Rows) != 2 || s.Providers[0].Rows[0].Name != "personal" || s.Providers[0].Rows[1].Name != "work" {
		t.Fatalf("profile identity lost: %+v", s)
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"fixture-secret", "private@example.test", "private-tenant", "auth.json", "apiKey"} {
		if strings.Contains(string(b), secret) {
			t.Fatalf("usage exports account data: %s", secret)
		}
	}
	if s.Providers[0].Rows[1].Resets != nil {
		t.Fatal("unknown reset balance became an empty balance")
	}
}
