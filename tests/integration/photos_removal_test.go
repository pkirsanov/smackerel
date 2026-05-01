//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	photolib "github.com/smackerel/smackerel/internal/connector/photos"
)

// TestPhotosRemovalCandidates_RequireRationaleAndNoMutationBeforeConfirm
// proves SCN-040-009: removal candidates must enforce rationale, must
// stay in pending_review until an action-token confirmation flips them,
// and must not perform any provider mutation at analyzer time.
func TestPhotosRemovalCandidates_RequireRationaleAndNoMutationBeforeConfirm(t *testing.T) {
	pool := testPool(t)
	store := photolib.NewStore(pool)
	analyzer := photolib.NewRemovalAnalyzer(store)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	connectorID := "connector-040-scope3-removal"
	t.Cleanup(func() { cleanupPhotosByConnector(t, pool, connectorID) })

	event := photolib.SyntheticPhotoEvent()
	event.ProviderRef = testID(t) + "-removal"
	event.ContentHash = "sha256:removal-" + strings.ReplaceAll(event.ProviderRef, "/", "-")
	photo, err := store.PublishPhotoEvent(ctx, connectorID, "synthetic", event)
	if err != nil {
		t.Fatalf("publish photo: %v", err)
	}
	cleanupPhoto(t, photo.ArtifactID)

	// Adversarial: rationale missing must be rejected.
	if _, err := analyzer.Apply(ctx, photolib.RemovalDecisionInput{
		PhotoID:    photo.ID,
		Reason:     photolib.RemovalBurstNonBest,
		Confidence: 0.91,
	}); err == nil {
		t.Fatalf("expected removal apply with empty rationale to fail")
	}

	// Adversarial: invalid reason must be rejected.
	if _, err := analyzer.Apply(ctx, photolib.RemovalDecisionInput{
		PhotoID:    photo.ID,
		Reason:     photolib.RemovalReason("invalid_reason"),
		Confidence: 0.91,
		Rationale:  "ok",
	}); err == nil {
		t.Fatalf("expected removal apply with invalid reason to fail")
	}

	// Adversarial: invalid method must be rejected.
	if _, err := analyzer.Apply(ctx, photolib.RemovalDecisionInput{
		PhotoID:    photo.ID,
		Reason:     photolib.RemovalBurstNonBest,
		Confidence: 0.91,
		Rationale:  "ok",
		Method:     "spreadsheet",
	}); err == nil {
		t.Fatalf("expected removal apply with invalid method to fail")
	}

	// Happy path: candidate persists in pending_review and audit row exists.
	candidate, err := analyzer.Apply(ctx, photolib.RemovalDecisionInput{
		PhotoID:    photo.ID,
		Reason:     photolib.RemovalBurstNonBest,
		Confidence: 0.92,
		Rationale:  "non-best burst frame",
		Method:     "stable_signal",
	})
	if err != nil {
		t.Fatalf("apply removal candidate: %v", err)
	}
	if candidate.ActionStatus != "pending_review" {
		t.Fatalf("action_status=%q, want pending_review", candidate.ActionStatus)
	}

	// Anti-mutation guarantee: photo lifecycle_state must remain unchanged.
	var lifecycleState string
	if err := pool.QueryRow(ctx, `SELECT lifecycle_state::text FROM photos WHERE id=$1`, photo.ID).Scan(&lifecycleState); err != nil {
		t.Fatalf("check photo lifecycle_state: %v", err)
	}
	if lifecycleState == "deleted" || lifecycleState == "archived" {
		t.Fatalf("photo unexpectedly mutated to %s before confirm", lifecycleState)
	}

	// Decision endpoint MUST flip status only when the action token is set.
	tokenID := uuid.New()
	updated, err := store.MarkRemovalDecision(ctx, candidate.ID, "archived", "tester", tokenID)
	if err != nil {
		t.Fatalf("mark removal decision: %v", err)
	}
	if updated.ActionStatus != "archived" {
		t.Fatalf("action_status=%q after MarkRemovalDecision, want archived", updated.ActionStatus)
	}
	if updated.ActionTokenID == nil || *updated.ActionTokenID != tokenID {
		t.Fatalf("action_token_id missing on decided candidate: %v", updated.ActionTokenID)
	}
	if updated.DecidedAt == nil {
		t.Fatalf("decided_at not set on decided candidate")
	}

	// Decision endpoint must reject unsupported decisions.
	if _, err := store.MarkRemovalDecision(ctx, candidate.ID, "destroyed", "tester", tokenID); err == nil {
		t.Fatalf("expected MarkRemovalDecision to reject unsupported decision")
	}

	events, err := store.ListAuditEvents(ctx, "removal_", 10)
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	if len(events) == 0 {
		t.Fatalf("expected removal audit events to be written")
	}

	t.Cleanup(func() {
		cleanCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = pool.Exec(cleanCtx, `DELETE FROM photo_removal_candidates WHERE photo_id=$1`, photo.ID)
		_, _ = pool.Exec(cleanCtx, `DELETE FROM photo_audit_events WHERE photo_id=$1`, photo.ID)
	})
}
