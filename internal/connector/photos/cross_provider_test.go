package photos

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestCrossProviderDuplicateUsesProviderNeutralSignals proves the
// dedupe signal cares only about the photo's canonical content
// fingerprint. Renaming the provider, swapping the provider_ref, or
// dropping raw_provider MUST NOT change the duplicate decision.
//
// SCN-040-014: Cross-provider search and dedupe are provider-neutral.
func TestCrossProviderDuplicateUsesProviderNeutralSignals(t *testing.T) {
	bytes := int64(2_345_678)
	captured := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)

	immich := PhotoRecord{
		ID:          uuid.New(),
		Provider:    "immich",
		ProviderRef: "immich-vacation-001",
		ContentHash: "sha256:vacation-photo",
		Bytes:       &bytes,
		CapturedAt:  &captured,
	}
	photoprism := PhotoRecord{
		ID:          uuid.New(),
		Provider:    "photoprism",
		ProviderRef: "photoprism-12345",
		ContentHash: "sha256:vacation-photo",
		Bytes:       &bytes,
		CapturedAt:  &captured,
	}

	immichSignal := SignalForRecord(immich)
	photoprismSignal := SignalForRecord(photoprism)
	if immichSignal != photoprismSignal {
		t.Fatalf("provider-neutral signals diverged: immich=%+v photoprism=%+v", immichSignal, photoprismSignal)
	}
	if !SameCrossProviderDuplicate(immichSignal, photoprismSignal, time.Minute) {
		t.Fatalf("SameCrossProviderDuplicate returned false for matching content_hash across providers")
	}

	// Adversarial: two records with the SAME provider but different
	// content_hash MUST NOT be considered duplicates — proves the
	// signal does not collapse to the provider id.
	differentHash := immich
	differentHash.ContentHash = "sha256:different"
	if SameCrossProviderDuplicate(SignalForRecord(immich), SignalForRecord(differentHash), time.Minute) {
		t.Fatalf("expected non-duplicate when content_hash differs (same provider)")
	}

	// Adversarial: erasing content_hash should fall through to the
	// weak EXIF/bytes signal, but only when both bytes and
	// captured_at agree within the near-duplicate window.
	noHashImmich := immich
	noHashImmich.ContentHash = ""
	noHashPhotoprism := photoprism
	noHashPhotoprism.ContentHash = ""
	if !SameCrossProviderDuplicate(SignalForRecord(noHashImmich), SignalForRecord(noHashPhotoprism), time.Minute) {
		t.Fatalf("expected weak-signal duplicate when bytes+captured_at agree")
	}

	// Adversarial: drop bytes from one side — weak signal MUST NOT
	// fire (we refuse to cluster on captured_at alone).
	noBytes := noHashPhotoprism
	noBytes.Bytes = nil
	if SameCrossProviderDuplicate(SignalForRecord(noHashImmich), SignalForRecord(noBytes), time.Minute) {
		t.Fatalf("expected non-duplicate when only captured_at agrees and bytes is missing")
	}

	// Adversarial: same bytes but captured_at far apart — must not
	// fire; weak signal requires both bytes equality AND captured_at
	// proximity, never one alone.
	farAway := captured.Add(2 * time.Hour)
	noHashFarAway := noHashPhotoprism
	noHashFarAway.CapturedAt = &farAway
	if SameCrossProviderDuplicate(SignalForRecord(noHashImmich), SignalForRecord(noHashFarAway), time.Minute) {
		t.Fatalf("expected non-duplicate when captured_at exceeds near-duplicate window")
	}

	// Symmetry: function MUST be commutative — proves it isn't
	// branching on left/right argument order.
	if SameCrossProviderDuplicate(SignalForRecord(immich), SignalForRecord(photoprism), time.Minute) !=
		SameCrossProviderDuplicate(SignalForRecord(photoprism), SignalForRecord(immich), time.Minute) {
		t.Fatalf("SameCrossProviderDuplicate is not commutative — provider-order leak")
	}
}

// TestSignalForEventMatchesSignalForRecord guards against drift between
// the ingest-time signal and the persisted-row signal: identical input
// data MUST produce identical signals regardless of code path.
func TestSignalForEventMatchesSignalForRecord(t *testing.T) {
	captured := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	bytes := int64(2_345_678)

	event := SyntheticPhotoEvent()
	event.ContentHash = "sha256:event-vs-record"
	event.Bytes = &bytes
	event.CapturedAt = captured

	record := PhotoRecord{
		ContentHash: event.ContentHash,
		Bytes:       &bytes,
		CapturedAt:  &captured,
	}

	if SignalForEvent(event) != SignalForRecord(record) {
		t.Fatalf("SignalForEvent != SignalForRecord for identical input")
	}
}
