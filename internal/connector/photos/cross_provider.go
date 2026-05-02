// Spec 040 Scope 5 — Cross-provider duplicate signals.
//
// The dedupe analyzer (Scope 3) accepts pre-computed cluster
// decisions, but the *signal* that decides "these two photos are
// duplicates" must be provider-neutral: content_hash, captured_at,
// EXIF, and bytes — never the provider id.
//
// SCN-040-014 (cross-provider search and dedupe). The unit test in
// `cross_provider_test.go` proves the signal stays stable when the
// provider strings change.
package photos

import (
	"strings"
	"time"
)

// CrossProviderSignal is the canonical cross-provider duplicate
// signal. Two photos collide on this signal IFF they have the same
// non-empty `ContentHash`. EXIF time + bytes are returned alongside
// so callers can compute weaker (near-duplicate) clusters without
// branching on provider strings.
type CrossProviderSignal struct {
	ContentHash    string
	CapturedAtUnix int64
	Bytes          int64
	HasBytes       bool
}

// SignalForRecord extracts the provider-neutral duplicate signal from
// a `PhotoRecord`. Provider, ProviderRef, and RawProvider are
// intentionally NOT included so the caller cannot accidentally branch
// on them.
func SignalForRecord(record PhotoRecord) CrossProviderSignal {
	signal := CrossProviderSignal{
		ContentHash: strings.TrimSpace(record.ContentHash),
	}
	if record.CapturedAt != nil && !record.CapturedAt.IsZero() {
		signal.CapturedAtUnix = record.CapturedAt.UTC().Unix()
	}
	if record.Bytes != nil {
		signal.Bytes = *record.Bytes
		signal.HasBytes = true
	}
	return signal
}

// SignalForEvent extracts the provider-neutral duplicate signal from a
// `PhotoEvent` (used during ingest before the row is persisted).
func SignalForEvent(event PhotoEvent) CrossProviderSignal {
	signal := CrossProviderSignal{
		ContentHash: strings.TrimSpace(event.ContentHash),
	}
	if !event.CapturedAt.IsZero() {
		signal.CapturedAtUnix = event.CapturedAt.UTC().Unix()
	}
	if event.Bytes != nil {
		signal.Bytes = *event.Bytes
		signal.HasBytes = true
	}
	return signal
}

// SameCrossProviderDuplicate reports whether two signals represent the
// same canonical photo across providers. The strong signal is a shared
// non-empty ContentHash; weak signals (within `nearDelta` of each
// other on captured_at + identical bytes) cluster under
// `cross_provider_hash` only when the strong signal is UNAVAILABLE on
// at least one side. When BOTH sides carry a non-empty content hash
// and the hashes disagree, the function MUST return false even if the
// weak signals coincide — otherwise a SHA-256 collision-free pair of
// edits could silently be merged.
func SameCrossProviderDuplicate(left CrossProviderSignal, right CrossProviderSignal, nearDelta time.Duration) bool {
	if left.ContentHash != "" && right.ContentHash != "" {
		return left.ContentHash == right.ContentHash
	}
	if !left.HasBytes || !right.HasBytes || left.Bytes != right.Bytes {
		return false
	}
	if left.CapturedAtUnix == 0 || right.CapturedAtUnix == 0 {
		return false
	}
	delta := time.Duration(left.CapturedAtUnix-right.CapturedAtUnix) * time.Second
	if delta < 0 {
		delta = -delta
	}
	return delta <= nearDelta
}
