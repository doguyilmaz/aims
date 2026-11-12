package cli

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/doguyilmaz/aims/internal/testutil"
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
