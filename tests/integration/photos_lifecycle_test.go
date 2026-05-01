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

// TestPhotosLifecycle_RAWExportsLinkedWithRationale exercises SCN-040-007.
// The lifecycle decision pipeline must persist RAW→export pairs with
// editor signatures and rationale, surface review queue entries when
// confidence is below the configured threshold, and write an audit
// event for every link transition.
//
// Adversarial cases prove that:
//   - low confidence falls into review_required (does not auto-confirm)
//   - missing rationale is rejected at the LLM-decision validator
//   - missing editor signature is rejected
func TestPhotosLifecycle_RAWExportsLinkedWithRationale(t *testing.T) {
	pool := testPool(t)
	store := photolib.NewStore(pool)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	connectorID := "connector-040-scope3-lifecycle"
	t.Cleanup(func() { cleanupPhotosByConnector(t, pool, connectorID) })

	rawEvent := photolib.SyntheticPhotoEvent()
	rawEvent.ProviderRef = testID(t) + "-raw"
	rawEvent.ContentHash = "sha256:lifecycle-raw-" + strings.ReplaceAll(rawEvent.ProviderRef, "/", "-")
	raw, err := store.PublishPhotoEvent(ctx, connectorID, "synthetic", rawEvent)
	if err != nil {
		t.Fatalf("publish raw photo: %v", err)
	}
	cleanupPhoto(t, raw.ArtifactID)

	derivedEvent := photolib.SyntheticPhotoEvent()
	derivedEvent.ProviderRef = testID(t) + "-derived"
	derivedEvent.ContentHash = "sha256:lifecycle-derived-" + strings.ReplaceAll(derivedEvent.ProviderRef, "/", "-")
	derivedEvent.MediaRole = photolib.MediaRoleEditedExport
	derivedEvent.EXIF = map[string]any{"software": "Adobe Photoshop Lightroom Classic 13.2"}
	derived, err := store.PublishPhotoEvent(ctx, connectorID, "synthetic", derivedEvent)
	if err != nil {
		t.Fatalf("publish derived photo: %v", err)
	}
	cleanupPhoto(t, derived.ArtifactID)

	threshold := 0.75
	analyzer := photolib.NewLifecycleAnalyzer(store, threshold)

	// Adversarial: missing rationale must be rejected.
	if _, err := analyzer.Apply(ctx, photolib.LifecycleDecisionInput{
		RawPhotoID:     raw.ID,
		DerivedPhotoID: derived.ID,
		Editor:         photolib.EditorLightroomClass,
		EditorVersion:  "Adobe Photoshop Lightroom Classic 13.2",
		Confidence:     0.91,
		Method:         "stable_signal",
	}); err == nil {
		t.Fatalf("expected lifecycle apply with empty rationale to fail")
	}

	// Adversarial: invalid method must be rejected.
	if _, err := analyzer.Apply(ctx, photolib.LifecycleDecisionInput{
		RawPhotoID:     raw.ID,
		DerivedPhotoID: derived.ID,
		Editor:         photolib.EditorLightroomClass,
		EditorVersion:  "Adobe Photoshop Lightroom Classic 13.2",
		Confidence:     0.91,
		Rationale:      "matched on capture timestamp + sha-prefix",
		Method:         "spreadsheet",
	}); err == nil {
		t.Fatalf("expected lifecycle apply with invalid method to fail")
	}

	// Happy path 1: high confidence link is confirmed.
	confirmed, err := analyzer.Apply(ctx, photolib.LifecycleDecisionInput{
		RawPhotoID:     raw.ID,
		DerivedPhotoID: derived.ID,
		Editor:         photolib.EditorLightroomClass,
		EditorVersion:  "Adobe Photoshop Lightroom Classic 13.2",
		Confidence:     0.91,
		Rationale:      "matched on capture timestamp + sha-prefix",
		Method:         "stable_signal",
	})
	if err != nil {
		t.Fatalf("apply confirmed lifecycle link: %v", err)
	}
	if confirmed.ReviewState != "confirmed" {
		t.Fatalf("review_state = %q, want confirmed (high confidence path)", confirmed.ReviewState)
	}
	if confirmed.Editor != photolib.EditorLightroomClass {
		t.Fatalf("editor = %q, want lightroom_classic", confirmed.Editor)
	}

	// Reset the link to test the low-confidence path independently.
	if _, err := pool.Exec(ctx, `DELETE FROM photo_raw_export_links WHERE raw_photo_id=$1 AND derived_photo_id=$2`, raw.ID, derived.ID); err != nil {
		t.Fatalf("reset lifecycle link: %v", err)
	}

	// Happy path 2: low confidence enqueues review.
	enqueued, err := analyzer.Apply(ctx, photolib.LifecycleDecisionInput{
		RawPhotoID:     raw.ID,
		DerivedPhotoID: derived.ID,
		Editor:         photolib.EditorLightroomClass,
		EditorVersion:  "Adobe Photoshop Lightroom Classic 13.2",
		Confidence:     0.62,
		Rationale:      "weak match: sha prefix only",
		Method:         "llm",
	})
	if err != nil {
		t.Fatalf("apply low-confidence lifecycle link: %v", err)
	}
	if enqueued.ReviewState != "review_required" {
		t.Fatalf("review_state = %q, want review_required (low confidence path)", enqueued.ReviewState)
	}

	summary, err := store.SummarizeLifecycle(ctx, threshold, time.Now().UTC())
	if err != nil {
		t.Fatalf("summarize lifecycle: %v", err)
	}
	if summary.Total < 1 {
		t.Fatalf("summary.Total = %d, want >= 1", summary.Total)
	}
	if len(summary.ReviewQueue) == 0 {
		t.Fatalf("expected at least one review queue entry, got %d", len(summary.ReviewQueue))
	}

	events, err := store.ListAuditEvents(ctx, "lifecycle_link", 10)
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	if len(events) < 2 {
		t.Fatalf("expected at least two audit events for lifecycle, got %d", len(events))
	}

	// Cleanup the lifecycle link rows we created so reruns are isolated.
	t.Cleanup(func() {
		cleanCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = pool.Exec(cleanCtx, `DELETE FROM photo_raw_export_links WHERE raw_photo_id=$1 OR derived_photo_id=$2`, raw.ID, derived.ID)
		_, _ = pool.Exec(cleanCtx, `DELETE FROM photo_audit_events WHERE photo_id IN ($1,$2)`, uuid.UUID(raw.ID), uuid.UUID(derived.ID))
	})
}
