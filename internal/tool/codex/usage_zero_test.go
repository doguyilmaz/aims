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

func TestMissingRatioNeverBecomesAResetZero(t *testing.T) {
	acct := &reply{Result: json.RawMessage(`{"account":{"type":"chatgpt","planType":"business"}}`)}
	for _, ratio := range []string{"", `"usedPercent":null,`, `"renamedUsedPercent":0,`, `"usedPercent":0,`} {
		unknown := ratio != `"usedPercent":0,`
		raw := `{"rateLimits":{"primary":{` + ratio + `"windowDurationMins":10080}},"rateLimitsByLimitId":{"feature":{"primary":{` + ratio + `"windowDurationMins":10080}}}}`
		p := interpret(acct, &reply{Result: json.RawMessage(raw)})
		if len(p.Windows) != 1 || p.Windows[0].PercentUnknown != unknown {
			t.Fatalf("main ratio %q: %+v", ratio, p.Windows)
		}
		if len(p.Groups) != 1 || p.Groups[0].Windows[0].PercentUnknown != unknown {
			t.Fatalf("group ratio %q: %+v", ratio, p.Groups)
		}
		if !unknown && p.Windows[0].Percent != 0 {
			t.Fatal("lost real zero")
		}
	}
}
