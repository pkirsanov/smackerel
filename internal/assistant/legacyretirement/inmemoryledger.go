// inmemoryledger.go — spec 075 SCOPE-2 in-memory NoticeLedger used by
// unit tests and as a development fallback when no SQL backend is
// configured. Production deployments wire SQLNoticeLedger; this
// implementation is concurrency-safe so policy unit tests can
// exercise concurrent Handle() calls.
package legacyretirement

import (
	"context"
	"sync"
	"time"
)

type ledgerKey struct {
	userID   string
	command  string
	windowID string
}

// InMemoryNoticeLedger is a process-local NoticeLedger. Entries are
// lost on restart by construction — use SQLNoticeLedger for any
// non-test deployment.
type InMemoryNoticeLedger struct {
	mu      sync.Mutex
	entries map[ledgerKey]NoticeLedgerEntry
}

// NewInMemoryNoticeLedger returns a ready-to-use ledger.
func NewInMemoryNoticeLedger() *InMemoryNoticeLedger {
	return &InMemoryNoticeLedger{entries: make(map[ledgerKey]NoticeLedgerEntry)}
}

// HasNotified implements NoticeLedger.
func (l *InMemoryNoticeLedger) HasNotified(_ context.Context, userID, retiredCommand, windowID string) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, ok := l.entries[ledgerKey{userID, retiredCommand, windowID}]
	return ok, nil
}

// MarkShown implements NoticeLedger. Repeat calls bump notice_count
// and last_seen_at while preserving first_notified_at.
func (l *InMemoryNoticeLedger) MarkShown(_ context.Context, userID, retiredCommand, windowID string, shownAt time.Time) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	k := ledgerKey{userID, retiredCommand, windowID}
	entry, ok := l.entries[k]
	if !ok {
		entry = NoticeLedgerEntry{
			Command:         retiredCommand,
			FirstNotifiedAt: shownAt,
		}
	}
	entry.LastSeenAt = shownAt
	entry.NoticeCount++
	l.entries[k] = entry
	return nil
}

// Get implements NoticeLedger.
func (l *InMemoryNoticeLedger) Get(_ context.Context, userID, retiredCommand, windowID string) (NoticeLedgerEntry, bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry, ok := l.entries[ledgerKey{userID, retiredCommand, windowID}]
	return entry, ok, nil
}
