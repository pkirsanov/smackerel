//go:build integration

// Spec 076 SCOPE-5 — TP-076-05-04 / SCN-074-A05.
//
// Live-Postgres adversarial regression that two distinct users
// sending the SAME normalized text in the SAME dedup bucket MUST
// each receive their own Idea artifact. Cross-user isolation is
// enforced structurally because user_id participates in the partial
// unique index on artifact_capture_policy
// (user_id, provenance, normalized_text_hash, dedup_bucket_start)
// WHERE provenance='capture-as-fallback'.
//
// Adversarial probe: the test then performs a SECOND same-bucket
// capture for user A and asserts it DOES dedup back to user A's
// first artifact. Without this second probe, the cross-user check
// would pass vacuously if (for example) the dedup key were
// accidentally salted with a per-test random value and no dedup ever
// fired. The probe proves the dedup key composition is exercised.

package capture_integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/assistant/capturefallback"
)

// TestCaptureDedup_CrossUserNeverDedupes_Adversarial — TP-076-05-04 / SCN-074-A05.
func TestCaptureDedup_CrossUserNeverDedupes_Adversarial(t *testing.T) {
	pool := openScope5Pool(t)
	store := capturefallback.NewPostgresStore(pool)
	policy, _ := newScope5DedupPolicy(t, pool, "spec076-scope5-xuser-art")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	const text = "cross-user dedup-leak regression candidate for spec 076"
	now := time.Now().UTC()
	stamp := now.UnixNano()
	userA := fmt.Sprintf("spec076-scope5-xuser-A-%d", stamp)
	userB := fmt.Sprintf("spec076-scope5-xuser-B-%d", stamp)

	mkReq := func(user, msg string) capturefallback.Request {
		return capturefallback.Request{
			UserID:             user,
			Transport:          "telegram",
			TransportMessageID: msg,
			OriginalText:       text,
			Cause:              capturefallback.CauseUnrouted,
			IntentTraceID:      "intent-xuser-" + msg,
			OccurredAt:         now,
		}
	}

	// userA — first capture.
	decA1, err := policy.Decide(ctx, mkReq(userA, "tg:A1"))
	if err != nil {
		t.Fatalf("Decide userA: %v", err)
	}
	resA1, err := policy.CaptureForUser(ctx, userA, decA1)
	if err != nil {
		t.Fatalf("CaptureForUser userA: %v", err)
	}
	if resA1.AlreadyCaptured {
		t.Fatal("userA first capture wrongly reported AlreadyCaptured")
	}

	// userB — same text, same bucket, distinct artifact.
	decB1, err := policy.Decide(ctx, mkReq(userB, "tg:B1"))
	if err != nil {
		t.Fatalf("Decide userB: %v", err)
	}
	if !decB1.DedupBucketStart.Equal(decA1.DedupBucketStart) {
		t.Fatalf("test-setup error: userA/userB buckets diverged — A=%s B=%s", decA1.DedupBucketStart, decB1.DedupBucketStart)
	}
	resB1, err := policy.CaptureForUser(ctx, userB, decB1)
	if err != nil {
		t.Fatalf("CaptureForUser userB: %v", err)
	}
	if resB1.AlreadyCaptured {
		t.Fatalf("cross-user dedup leak — userB first capture reported AlreadyCaptured (source=%q); SCN-074-A05 regression", resB1.AlreadyCapturedSourceID)
	}
	if resB1.IdeaArtifactID == resA1.IdeaArtifactID {
		t.Fatalf("cross-user dedup leak — userB got userA's artifact id %q", resA1.IdeaArtifactID)
	}

	// Adversarial in-window probe — userA again MUST dedup. If the
	// dedup key were broken (e.g., included a random per-request
	// salt), this assertion would fail and the cross-user check
	// above would no longer be meaningful.
	decA2, err := policy.Decide(ctx, mkReq(userA, "tg:A2"))
	if err != nil {
		t.Fatalf("Decide userA second: %v", err)
	}
	resA2, err := policy.CaptureForUser(ctx, userA, decA2)
	if err != nil {
		t.Fatalf("CaptureForUser userA second: %v", err)
	}
	if !resA2.AlreadyCaptured {
		t.Fatalf("userA second in-window capture did NOT dedup; got %q — cross-user assertion would be vacuous", resA2.IdeaArtifactID)
	}
	if resA2.AlreadyCapturedSourceID != resA1.IdeaArtifactID {
		t.Errorf("userA dedup source = %q, want %q", resA2.AlreadyCapturedSourceID, resA1.IdeaArtifactID)
	}

	// Final structural check — strict counts per user.
	for _, u := range []string{userA, userB} {
		got, err := store.CountByProvenance(ctx, u, capturefallback.ProvenanceFallback)
		if err != nil {
			t.Fatalf("CountByProvenance(%s): %v", u, err)
		}
		if got != 1 {
			t.Errorf("user %s fallback row count = %d, want 1", u, got)
		}
	}
}
