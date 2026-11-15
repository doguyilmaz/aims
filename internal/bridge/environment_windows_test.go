package bridge

import (
	"testing"

	"github.com/doguyilmaz/aims/internal/proc"
	"github.com/doguyilmaz/aims/internal/tools"
)

func TestWindowsAgentDropsMixedCaseHostSessionMarkers(t *testing.T) {
	cfg := setup(t)
	env, _, err := AgentEnvironment(cfg, tools.Get("codex"), "other", []string{"aims_run=parent-run", "codex_thread_id=parent-thread", "Path=C:\\tools"})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"AIMS_RUN", "CODEX_THREAD_ID"} {
		if _, ok := proc.EnvValue(env, key); ok {
			t.Fatalf("inherited parent context marker %s", key)
		}
	}
}
