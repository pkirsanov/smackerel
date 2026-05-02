//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	photolib "github.com/smackerel/smackerel/internal/connector/photos"
)

// TestPhotosSensitivity_ServerSidePreviewRevealAndAudit covers
// SCN-040-012: sensitive photos require a reveal token; non-sensitive
// photos are auto-served. Hidden photos block all routing targets and
// can only be retrieved with a reveal token bound to the requesting
// actor; a wrong actor or expired token is rejected. The integration
// surface verifies the persistence layer that backs the API gate.
func TestPhotosSensitivity_ServerSidePreviewRevealAndAudit(t *testing.T) {
	pool := testPool(t)
	store := photolib.NewStore(pool)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	uniq := testID(t)
	sensitive := photolib.SyntheticPhotoEvent()
	sensitive.ProviderRef = "web:sensitive:" + uniq
	sensitive.ContentHash = "sha256:sensitive:" + uniq
	sensitive.SourceChannel = photolib.SourceChannelWeb
	sensitive.SourceRef = "session:" + uniq
	sensitive.Sensitivity = photolib.ProviderSensitivity{
		Level:  photolib.SensitivitySensitive,
		Source: "test",
		Labels: []string{"financial"},
	}
	record, err := store.PublishPhotoEvent(ctx, "test-sensitivity", "web", sensitive)
	if err != nil {
		t.Fatalf("publish sensitive photo: %v", err)
	}
	cleanupPhoto(t, record.ArtifactID)

	noTokenDecision := photolib.EvaluateRetrieval(*record, false)
	if noTokenDecision.Allowed {
		t.Fatalf("sensitive photo retrieval allowed without token: %+v", noTokenDecision)
	}
	if noTokenDecision.Reason != photolib.RetrievalBlockedSensitive {
		t.Fatalf("expected sensitive block reason, got %v", noTokenDecision.Reason)
	}

	now := time.Now().UTC()
	mintInput := photolib.MintRevealTokenInput{
		PhotoID: record.ID,
		ActorID: "alice",
		TTL:     30 * time.Second,
	}
	token, err := store.MintRevealToken(ctx, mintInput, now)
	if err != nil {
		t.Fatalf("mint reveal token: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := pool.Exec(ctx, `DELETE FROM photo_reveal_tokens WHERE photo_id=$1`, record.ID); err != nil {
			t.Logf("cleanup reveal tokens: %v", err)
		}
	})

	// Wrong actor must be rejected (adversarial — a reused token cannot
	// be replayed by another user).
	if _, err := store.ConsumeRevealToken(ctx, record.ID, "mallory", token.Plaintext, now); err == nil {
		t.Fatalf("expected reveal consume to reject wrong actor")
	}
	if err := store.CheckRevealToken(ctx, record.ID, "alice", token.Plaintext, now); err != nil {
		t.Fatalf("check reveal token: %v", err)
	}
	consumed, err := store.ConsumeRevealToken(ctx, record.ID, "alice", token.Plaintext, now.Add(time.Second))
	if err != nil {
		t.Fatalf("consume reveal: %v", err)
	}
	if consumed.ConsumedAt == nil {
		t.Fatalf("consumed token must record consumed_at")
	}
	withTokenDecision := photolib.EvaluateRetrieval(*record, true)
	if !withTokenDecision.Allowed {
		t.Fatalf("sensitive photo with valid reveal must be allowed: %+v", withTokenDecision)
	}

	// A second consume MUST fail — tokens are single-use.
	if _, err := store.ConsumeRevealToken(ctx, record.ID, "alice", token.Plaintext, now.Add(2*time.Second)); err == nil {
		t.Fatalf("expected second consume to fail (single-use token)")
	}

	// An expired token must be rejected even with the correct actor.
	freshToken, err := store.MintRevealToken(ctx, photolib.MintRevealTokenInput{
		PhotoID: record.ID,
		ActorID: "alice",
		TTL:     time.Second,
	}, now)
	if err != nil {
		t.Fatalf("mint expiring reveal token: %v", err)
	}
	if _, err := store.ConsumeRevealToken(ctx, record.ID, "alice", freshToken.Plaintext, now.Add(2*time.Hour)); err == nil {
		t.Fatalf("expected expired reveal token to be rejected")
	}

	// Hidden photos block all routing targets and require reveal.
	hidden := photolib.SyntheticPhotoEvent()
	hidden.ProviderRef = "web:hidden:" + uniq
	hidden.ContentHash = "sha256:hidden:" + uniq
	hidden.SourceChannel = photolib.SourceChannelWeb
	hidden.SourceRef = "session:" + uniq
	hidden.Sensitivity = photolib.ProviderSensitivity{
		Level:  photolib.SensitivityHidden,
		Source: "test",
		Labels: []string{"identity_document"},
	}
	hiddenRecord, err := store.PublishPhotoEvent(ctx, "test-sensitivity-hidden", "web", hidden)
	if err != nil {
		t.Fatalf("publish hidden photo: %v", err)
	}
	cleanupPhoto(t, hiddenRecord.ArtifactID)
	hiddenDecision := photolib.EvaluateRetrieval(*hiddenRecord, false)
	if hiddenDecision.Allowed {
		t.Fatalf("hidden photo allowed without reveal: %+v", hiddenDecision)
	}
	if hiddenDecision.Reason != photolib.RetrievalBlockedHidden {
		t.Fatalf("hidden block reason mismatch: %v", hiddenDecision.Reason)
	}

	// Routing for a hidden, identity-document sensitive plan must mark
	// every plan blocked (FR-007).
	plans, err := photolib.EvaluateRouting(photolib.ClassificationDecision{
		Caption:         "Passport page",
		PrimaryCategory: "identity_document",
		Confidence:      0.95,
		Rationale:       "MRZ pattern + face crop",
	}, photolib.SensitivityHidden, []string{"identity_document"}, 0.75)
	if err != nil {
		t.Fatalf("evaluate routing under hidden: %v", err)
	}
	if len(plans) == 0 {
		t.Fatalf("expected hidden routing plans for audit, got none")
	}
	for _, plan := range plans {
		if !plan.SensitivityBlocked {
			t.Fatalf("hidden routing plan %s not blocked", plan.Target)
		}
	}
}
