package codex

import (
	"encoding/json"
	"testing"
)

func TestMainWindowDoesNotInventAnEpochReset(t *testing.T) {
	acct := &reply{Result: json.RawMessage(`{"account":{"type":"chatgpt","planType":"plus"}}`)}
	for _, epoch := range []string{"0", "-1", "null"} {
		p := interpret(acct, &reply{Result: json.RawMessage(`{"rateLimits":{"primary":{"usedPercent":20,"resetsAt":` + epoch + `}}}`)})
		if len(p.Windows) != 1 || !p.Windows[0].ResetsAt.IsZero() {
			t.Fatalf("unavailable reset became an epoch date: %+v", p.Windows)
		}
	}
}
