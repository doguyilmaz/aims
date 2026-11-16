package profiles

import (
	"time"

	"github.com/doguyilmaz/aims/internal/config"
	"github.com/doguyilmaz/aims/internal/tool"
)

// activeLimitMark keeps explicit holds authoritative. A newer successful quota
// reading can contradict only an automatic plan-limit mark for that exact window.
// It says nothing about login validity or a short-lived request-rate limit.
func activeLimitMark(cfg *config.Config, ps *config.ProfileState, now time.Time) bool {
	if !ps.Until.After(now) {
		return false
	}
	if ps.Reason != "limit" || ps.Window == "" || ps.MarkedAt.IsZero() || ps.Usage == nil ||
		ps.UsageError != "" || !ps.Usage.At.After(ps.MarkedAt) || ps.UsageAttemptedAt.After(ps.Usage.At) {
		return true
	}
	switch ps.MarkedBy {
	case "a session", "a failed run", "a live check":
	default:
		return true
	}
	for _, w := range ps.Usage.Windows {
		if w.Label == ps.Window && !w.PercentUnknown && !windowLimited(cfg, w) &&
			(w.ResetsAt.IsZero() || w.ResetsAt.After(now)) {
			return false
		}
	}
	return true
}

// ClearAfterSuccess invalidates marks seen before this worker started. A success
// from an old worker must not clear a newer failure, manual hold or login change.
// Usage observations and native credentials are kept.
func ClearAfterSuccess(cfg *config.Config, id tool.ID, name string, before *config.ProfileState) error {
	if before == nil || !before.NeedsLogin && before.Until.IsZero() {
		return nil
	}
	return config.UpdateStateFor(cfg, id, name, func(ps *config.ProfileState) {
		if ps.Generation != before.Generation || ps.AccountRevision != before.AccountRevision || accountChanged(before.Account, ps.Account) ||
			!ps.MarkedAt.Equal(before.MarkedAt) || !ps.Until.Equal(before.Until) ||
			ps.Reason != before.Reason || ps.Window != before.Window || ps.NeedsLogin != before.NeedsLogin ||
			ps.MarkedBy != before.MarkedBy || ps.Detail != before.Detail {
			return
		}
		ps.Until, ps.Reason, ps.Window, ps.NeedsLogin = time.Time{}, "", "", false
		ps.MarkedBy, ps.Detail, ps.MarkedAt = "", "", time.Time{}
	})
}
