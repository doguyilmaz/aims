package profiles

import (
	"context"
	"testing"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/tool"
)

type passiveUsageReader struct {
	tool.Adapter
	probeCalls, usageCalls int
	result                 tool.Probe
}

func (a *passiveUsageReader) Probe(context.Context, tool.Home) tool.Probe {
	a.probeCalls++
	return tool.Probe{Status: tool.ProbeOK}
}

func (a *passiveUsageReader) Usage(context.Context, tool.Home) tool.Probe {
	a.usageCalls++
	return a.result
}

func TestPassiveUsageReaderPreservesMarksAndFailedObservationAge(t *testing.T) {
	cfg, adapter := setup(t)
	a := &passiveUsageReader{Adapter: adapter, result: tool.Probe{Status: tool.ProbeError, UsageError: "Quota access expired; use the native CLI."}}
	until := now.Add(RefreshDue)
	_ = config.UpdateState(a.ID(), "a", func(ps *config.ProfileState) {
		ps.Until, ps.Reason, ps.MarkedBy = until, "busy", "owner"
		ps.Usage = &config.Usage{At: now, Windows: []tool.Window{{Label: "7d", Percent: 97}}}
	})
	ReadUsage(t.Context(), cfg, a, "a")
	ps := config.LoadStateFor(cfg).Get(a.ID(), "a")
	if a.probeCalls != 0 || a.usageCalls != 1 || ps.UsageError != a.result.UsageError || ps.Usage == nil || !ps.Usage.At.Equal(now) || ps.Usage.Windows[0].Percent != 97 || !ps.Until.Equal(until) || ps.MarkedBy != "owner" || ps.NeedsLogin {
		t.Fatalf("passive refresh changed routing, credential state or old measurement: %+v", ps)
	}
	a.result = tool.Probe{Status: tool.ProbeOK, UsageReported: true, Windows: []tool.Window{{Label: "7d", Percent: 2}}}
	ReadUsage(t.Context(), cfg, a, "a")
	ps = config.LoadStateFor(cfg).Get(a.ID(), "a")
	if a.probeCalls != 0 || a.usageCalls != 2 || ps.Usage.Windows[0].Percent != 2 || ps.UsageError != "" || !ps.Until.Equal(until) {
		t.Fatalf("successful passive observation was not isolated: %+v", ps)
	}
	Live(t.Context(), cfg, a, "a")
	if a.probeCalls != 1 || a.usageCalls != 2 {
		t.Fatal("explicit login probe unexpectedly used the passive quota reader")
	}
}
