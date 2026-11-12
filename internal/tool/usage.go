package tool

import "time"

// QuotaGroup is an independently metered model or feature allowance. These
// additional groups are for display; they do not drive the main-account failover.
type QuotaGroup struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Windows []Window `json:"windows"`
}

// ResetBalance is a provider-reported inventory, not a usage allowance.
// A nil balance means unavailable; Available == 0 means a reported empty balance.
type ResetBalance struct {
	Available int64         `json:"available"`
	Credits   []ResetCredit `json:"credits"`
}

// ResetCredit describes a banked reset. aims never redeems or purchases one.
type ResetCredit struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Status    string    `json:"status"`
	Title     string    `json:"title,omitempty"`
	GrantedAt time.Time `json:"grantedAt,omitzero"`
	ExpiresAt time.Time `json:"expiresAt,omitzero"`
}
