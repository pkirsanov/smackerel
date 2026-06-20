// Spec 074 SCOPE-3 — dedup semantics unit tests.
//
// TP-074-08 / SCN-074-A03: same user, same normalized text, second
// turn within capture_as_fallback.dedup_window → exactly one Idea +
// dedup-hit acknowledgement.
//
// TP-074-09 / SCN-074-A04: same user, same normalized text, second
// turn after the dedup window has elapsed → two distinct Ideas
// (different dedup buckets, no dedup hit).
//
// Adversarial coverage:
//   - TP-074-08 captures the first turn, then asserts a dedup hit
//     on the second; the stub IdeaWriter records call count so a
//     regression that produced a second Idea instead of dedup would
//     trip writerCalls == 2.
//   - TP-074-09 advances the clock past the window and asserts the
//     second turn lands in a different bucket and produces a new
//     artifact id; a regression that kept bucket alignment would
//     show writerCalls == 1 (dedup hit) and fail.
//   - Cross-user isolation (SCN-074-A05 / TP-074-10) is also
//     unit-asserted here as a fast-feedback adversarial guard,
//     in addition to the live integration row in
//     tests/integration/assistant/capture_fallback_policy_test.go.

package capturefallback

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

const (
	scope3HashKey     = "tp-074-08-hmac-key"
	scope3DedupWindow = 5 * time.Minute
)

// stubWriter is a deterministic IdeaWriter that returns a fresh
// artifact id per call and counts invocations.
type stubWriter struct {
	calls atomic.Int64
}

func (w *stubWriter) WriteIdea(_ context.Context, _ string, _ string, _ Decision) (string, error) {
	n := w.calls.Add(1)
	return fmt.Sprintf("idea-%d", n), nil
}

func newScope3Policy(t *testing.T, store DedupStore, writer IdeaWriter, now func() time.Time) Policy {
	t.Helper()
	p, err := New(Config{
		DedupWindow:         scope3DedupWindow,
		NormalizationPolicy: NormalizationPolicyV1,
		DedupHashKey:        scope3HashKey,
	}, store, writer)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Replace the policy clock for deterministic bucket arithmetic.
	p.(*defaultPolicy).now = now
	return p
}

func mustDecide(t *testing.T, p Policy, req Request) Decision {
	t.Helper()
	dec, err := p.Decide(context.Background(), req)
	if err != nil {
		t.Fatalf("Decide(%+v): %v", req, err)
	}
	return dec
}

// TestDedup_TP_074_08_SameUserSameTextWithinWindowDedupes — SCN-074-A03.
func TestDedup_TP_074_08_SameUserSameTextWithinWindowDedupes(t *testing.T) {
	store := NewMemDedupStore()
	writer := &stubWriter{}
	clockNow := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	p := newScope3Policy(t, store, writer, func() time.Time { return clockNow })

	req := Request{
		UserID:             "user-A",
		Transport:          "telegram",
		TransportMessageID: "msg-1",
		OriginalText:       "Try the new bakery on 4th",
		Cause:              CauseUnrouted,
	}

	dec1 := mustDecide(t, p, req)
	res1, err := p.CaptureForUser(context.Background(), req.UserID, dec1)
	if err != nil {
		t.Fatalf("CaptureForUser (first): %v", err)
	}
	if res1.AlreadyCaptured {
		t.Fatalf("first capture must not be a dedup hit")
	}
	if res1.IdeaArtifactID == "" {
		t.Fatal("first capture missing IdeaArtifactID")
	}

	// Second turn 2 minutes later — still inside the 5-minute window.
	clockNow = clockNow.Add(2 * time.Minute)
	req2 := req
	req2.TransportMessageID = "msg-2"
	dec2 := mustDecide(t, p, req2)
	if !dec2.DedupBucketStart.Equal(dec1.DedupBucketStart) {
		t.Fatalf("buckets diverged within window: dec1=%s dec2=%s", dec1.DedupBucketStart, dec2.DedupBucketStart)
	}
	res2, err := p.CaptureForUser(context.Background(), req2.UserID, dec2)
	if err != nil {
		t.Fatalf("CaptureForUser (second): %v", err)
	}
	if !res2.AlreadyCaptured {
		t.Fatalf("second in-window capture must be a dedup hit; got new artifact %s", res2.IdeaArtifactID)
	}
	if res2.AlreadyCapturedSourceID != res1.IdeaArtifactID {
		t.Errorf("dedup-hit source id = %q, want %q", res2.AlreadyCapturedSourceID, res1.IdeaArtifactID)
	}
	if got := writer.calls.Load(); got != 1 {
		t.Errorf("writer invocations = %d, want 1 (SCN-074-A03 dedup hit must not call WriteIdea)", got)
	}
}

// TestDedup_TP_074_09_SameUserSameTextOutsideWindowCreatesNewBucket — SCN-074-A04.
func TestDedup_TP_074_09_SameUserSameTextOutsideWindowCreatesNewBucket(t *testing.T) {
	store := NewMemDedupStore()
	writer := &stubWriter{}
	// Anchor at a bucket boundary so a step strictly past the window
	// guarantees a bucket change.
	clockNow := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	p := newScope3Policy(t, store, writer, func() time.Time { return clockNow })

	req := Request{
		UserID:             "user-A",
		Transport:          "http",
		TransportMessageID: "req-1",
		OriginalText:       "Reserve a room for the offsite",
		Cause:              CauseOpenKnowledgeNoGround,
	}

	dec1 := mustDecide(t, p, req)
	res1, err := p.CaptureForUser(context.Background(), req.UserID, dec1)
	if err != nil {
		t.Fatalf("CaptureForUser (first): %v", err)
	}
	if res1.AlreadyCaptured {
		t.Fatal("first capture must not be a dedup hit")
	}

	// Advance past the dedup window so the second turn lands in a
	// new bucket.
	clockNow = clockNow.Add(scope3DedupWindow + time.Minute)
	req2 := req
	req2.TransportMessageID = "req-2"
	dec2 := mustDecide(t, p, req2)
	if dec2.DedupBucketStart.Equal(dec1.DedupBucketStart) {
		t.Fatalf("bucket did not advance after %s: dec1=%s dec2=%s",
			scope3DedupWindow+time.Minute, dec1.DedupBucketStart, dec2.DedupBucketStart)
	}
	res2, err := p.CaptureForUser(context.Background(), req2.UserID, dec2)
	if err != nil {
		t.Fatalf("CaptureForUser (second): %v", err)
	}
	if res2.AlreadyCaptured {
		t.Fatalf("second out-of-window capture must NOT dedup; got AlreadyCaptured=true source=%q", res2.AlreadyCapturedSourceID)
	}
	if res2.IdeaArtifactID == res1.IdeaArtifactID {
		t.Errorf("out-of-window capture reused artifact id %q (regression: dedup leaked across buckets)", res1.IdeaArtifactID)
	}
	if got := writer.calls.Load(); got != 2 {
		t.Errorf("writer invocations = %d, want 2 (SCN-074-A04 must create a new Idea outside the window)", got)
	}
}

// TestDedup_CrossUserSameTextIsolated — adversarial unit-level mirror
// of TP-074-10 / SCN-074-A05. The live integration row in
// tests/integration/assistant/capture_fallback_policy_test.go covers
// the Postgres path; this guard catches regressions in the dedup-key
// composition without requiring a live DB.
func TestDedup_CrossUserSameTextIsolated(t *testing.T) {
	store := NewMemDedupStore()
	writer := &stubWriter{}
	clockNow := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	p := newScope3Policy(t, store, writer, func() time.Time { return clockNow })

	mk := func(user, msg string) Request {
		return Request{
			UserID:             user,
			Transport:          "telegram",
			TransportMessageID: msg,
			OriginalText:       "Cross-user-isolation regression candidate",
			Cause:              CauseUnrouted,
		}
	}
	resA, err := p.CaptureForUser(context.Background(), "user-A", mustDecide(t, p, mk("user-A", "a-1")))
	if err != nil {
		t.Fatalf("user-A capture: %v", err)
	}
	resB, err := p.CaptureForUser(context.Background(), "user-B", mustDecide(t, p, mk("user-B", "b-1")))
	if err != nil {
		t.Fatalf("user-B capture: %v", err)
	}
	if resB.AlreadyCaptured {
		t.Fatalf("cross-user dedup leaked: user-B's first capture was reported as dedup hit (source=%q)", resB.AlreadyCapturedSourceID)
	}
	if resA.IdeaArtifactID == resB.IdeaArtifactID {
		t.Errorf("user-A and user-B got same artifact id %q (SCN-074-A05 regression)", resA.IdeaArtifactID)
	}
	if got := writer.calls.Load(); got != 2 {
		t.Errorf("writer invocations = %d, want 2 (each user must produce their own Idea)", got)
	}
}

// TestDedup_CaptureWithoutUserFails — SCN-074-A05 negative guard: the
// userless Capture path must return ErrMissingUser, never silently
// dedup or write an artifact.
func TestDedup_CaptureWithoutUserFails(t *testing.T) {
	p := newScope3Policy(t, NewMemDedupStore(), &stubWriter{}, func() time.Time {
		return time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	})
	_, err := p.Capture(context.Background(), Decision{
		Cause:              CauseUnrouted,
		Provenance:         ProvenanceFallback,
		NormalizedTextHash: "x",
		DedupBucketStart:   time.Now().UTC(),
		SchemaVersion:      SchemaVersion,
	})
	if err == nil {
		t.Fatal("expected Capture(no user) to return ErrMissingUser")
	}
}

// failingStore is a DedupStore whose Record method always fails,
// used to test orphan cleanup behavior.
type failingStore struct {
	*MemDedupStore
	recordErr error
}

func (s *failingStore) Record(_ context.Context, _ string, _ string, _ Decision) error {
	return s.recordErr
}

// cleanableWriter is an IdeaWriter that implements IdeaCleaner for
// testing orphan cleanup.
type cleanableWriter struct {
	stubWriter
	deletedIDs []string
	deleteErr  error
}

func (w *cleanableWriter) DeleteIdea(_ context.Context, artifactID string) error {
	w.deletedIDs = append(w.deletedIDs, artifactID)
	return w.deleteErr
}

// TestCaptureForUser_RecordFailure_CleansUpOrphan verifies that when
// Record fails after WriteIdea succeeds, the orphaned artifact is
// cleaned up if the writer implements IdeaCleaner. This tests the
// compensating transaction pattern added to prevent orphan Ideas
// without dedup metadata.
func TestCaptureForUser_RecordFailure_CleansUpOrphan(t *testing.T) {
	store := &failingStore{
		MemDedupStore: NewMemDedupStore(),
		recordErr:     fmt.Errorf("simulated DB constraint violation"),
	}
	writer := &cleanableWriter{}
	clockNow := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	p := newScope3Policy(t, store, writer, func() time.Time { return clockNow })

	req := Request{
		UserID:             "user-orphan-test",
		Transport:          "http",
		TransportMessageID: "msg-orphan-1",
		OriginalText:       "test orphan cleanup",
		Cause:              CauseUnrouted,
	}

	dec := mustDecide(t, p, req)
	_, err := p.CaptureForUser(context.Background(), req.UserID, dec)
	if err == nil {
		t.Fatal("expected CaptureForUser to fail when Record fails")
	}

	// Verify WriteIdea was called (artifact created)
	if got := writer.calls.Load(); got != 1 {
		t.Errorf("WriteIdea calls = %d, want 1", got)
	}

	// Verify DeleteIdea was called (orphan cleanup attempted)
	if len(writer.deletedIDs) != 1 {
		t.Fatalf("DeleteIdea calls = %d, want 1 (orphan cleanup must be attempted)", len(writer.deletedIDs))
	}
	if writer.deletedIDs[0] != "idea-1" {
		t.Errorf("DeleteIdea called with %q, want %q", writer.deletedIDs[0], "idea-1")
	}
}

// TestCaptureForUser_RecordFailure_NoCleanerInterface verifies that
// when Record fails and the writer does NOT implement IdeaCleaner,
// the error is still returned (orphan is logged but not cleaned).
// This ensures backward compatibility with writers that don't support
// cleanup.
func TestCaptureForUser_RecordFailure_NoCleanerInterface(t *testing.T) {
	store := &failingStore{
		MemDedupStore: NewMemDedupStore(),
		recordErr:     fmt.Errorf("simulated network error"),
	}
	// Use stubWriter which does NOT implement IdeaCleaner
	writer := &stubWriter{}
	clockNow := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	p := newScope3Policy(t, store, writer, func() time.Time { return clockNow })

	req := Request{
		UserID:             "user-no-cleaner-test",
		Transport:          "telegram",
		TransportMessageID: "msg-no-cleaner-1",
		OriginalText:       "test no cleaner interface",
		Cause:              CauseOpenKnowledgeNoGround,
	}

	dec := mustDecide(t, p, req)
	_, err := p.CaptureForUser(context.Background(), req.UserID, dec)
	if err == nil {
		t.Fatal("expected CaptureForUser to fail when Record fails")
	}

	// Verify WriteIdea was called (artifact created, now orphaned)
	if got := writer.calls.Load(); got != 1 {
		t.Errorf("WriteIdea calls = %d, want 1", got)
	}

	// Writer doesn't implement IdeaCleaner, so no cleanup is possible.
	// The orphan is logged but the error is still returned correctly.
	// This test ensures the function returns the correct error even
	// when cleanup isn't available.
	if err.Error() != "capturefallback: record dedup: simulated network error" {
		t.Errorf("unexpected error: %v", err)
	}
}
