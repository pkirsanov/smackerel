// ledger.go — spec 075 SCOPE-1 notice-dedup ledger seam.
package legacyretirement

import (
	"context"
	"time"
)

// NoticeLedgerEntry is one row in the per-conversation JSONB ledger
// stored in assistant_conversations.legacy_retirement_notices.
type NoticeLedgerEntry struct {
	Command         string
	FirstNotifiedAt time.Time
	LastSeenAt      time.Time
	NoticeCount     int
}

// NoticeLedger is the durable, transport-independent dedup store.
// Concrete implementations live alongside the assistant conversation
// store and persist into the JSONB column added by migration 046.
// Scope 1 declares only the contract.
type NoticeLedger interface {
	// HasNotified returns true iff a notice has already been recorded
	// for (userID, retiredCommand, windowID). Cross-transport dedup
	// is satisfied by the fact that the key does not include
	// transport.
	HasNotified(ctx context.Context, userID, retiredCommand, windowID string) (bool, error)
	// MarkShown records that a notice was shown for
	// (userID, retiredCommand, windowID) at shownAt. Repeated calls
	// for the same key MUST be idempotent (notice_count is bumped,
	// FirstNotifiedAt is preserved) so the policy can call MarkShown
	// without first calling HasNotified.
	MarkShown(ctx context.Context, userID, retiredCommand, windowID string, shownAt time.Time) error
	// Get returns the entry for (userID, retiredCommand, windowID),
	// or ok=false if none exists. Intended for the observation
	// report and the cross-transport dedup integration test.
	Get(ctx context.Context, userID, retiredCommand, windowID string) (NoticeLedgerEntry, bool, error)
}
