package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/testutil"
	"github.com/doguyilmaz/aims/internal/tool"
	"github.com/doguyilmaz/aims/internal/usage"
)

func TestCachedUsageJSONCanReuseTheRootContext(t *testing.T) {
	testutil.Sandbox(t)
	root := newRoot("v1.0.0")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	for range 2 {
		out.Reset()
		root.SetArgs([]string{"usage", "codex", "--json"})
		if err := root.ExecuteContext(t.Context()); err != nil {
			t.Fatal(err)
		}
		var snapshot usage.Snapshot
		if err := json.Unmarshal(out.Bytes(), &snapshot); err != nil {
			t.Fatal(err)
		}
		if snapshot.SchemaVersion != 1 || len(snapshot.Providers) != 1 || snapshot.Providers[0].ID != "codex" {
			t.Fatalf("unexpected cached snapshot: %+v", snapshot)
		}
	}
}

func TestClaudeRefreshUsesThePassiveReaderAndKeepsTheLastMeasurement(t *testing.T) {
	testutil.Sandbox(t)
	bin, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("AIMS_CLAUDE_BIN", bin)
	if _, err := config.Update(func(c *config.Config) error {
		c.Tools["claude"].Profiles["work"] = &config.Profile{}
		c.Tools["claude"].Order = []string{"work"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := config.UpdateState("claude", "work", func(ps *config.ProfileState) {
		ps.Usage = &config.Usage{At: config.Now(), Windows: []tool.Window{{Label: "7d", Percent: 97}}}
	}); err != nil {
		t.Fatal(err)
	}
	root := newRoot("v1.0.0")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"usage", "claude", "--refresh", "--json"})
	if err := root.ExecuteContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	var snapshot usage.Snapshot
	if err := json.Unmarshal(out.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	row := snapshot.Providers[0].Rows[0]
	if row.Error != "Claude usage access is unavailable; open this account in Claude Code and retry Refresh." || len(row.Windows) != 1 || row.Windows[0].Percent != 97 || row.AttemptedAt.IsZero() {
		t.Fatalf("Claude refresh skipped the passive reader or lost the old measurement: %+v", row)
	}
}
