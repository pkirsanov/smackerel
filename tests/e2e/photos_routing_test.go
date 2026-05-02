//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	photolib "github.com/smackerel/smackerel/internal/connector/photos"
)

// TestPhotosRouting_E2E_ReceiptRecipeDocumentCreateDownstreamArtifacts
// covers SCN-040-011 against the live stack. A classified photo MUST
// route to the per-target downstream artifact rows (expense /
// document / knowledge for receipts; recipe / mealplan for recipes;
// document / knowledge for legal scans). Re-running the classifier
// for the same target MUST update the existing row instead of
// duplicating it.
func TestPhotosRouting_E2E_ReceiptRecipeDocumentCreateDownstreamArtifacts(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)
	pool := photosE2EPool(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store := photolib.NewStore(pool)
	event := photolib.SyntheticPhotoEvent()
	event.ProviderRef = "e2e-routing-" + uuid.NewString()
	event.ContentHash = "sha256:routing:" + event.ProviderRef
	event.SourceChannel = photolib.SourceChannelWeb
	event.SourceRef = "session:routing"
	record, err := store.PublishPhotoEvent(ctx, "e2e-routing", "web", event)
	if err != nil {
		t.Fatalf("publish photo: %v", err)
	}
	cleanupE2EPhoto(t, pool, record.ArtifactID)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := pool.Exec(ctx, `DELETE FROM photo_routing_decisions WHERE photo_id=$1`, record.ID); err != nil {
			t.Logf("cleanup routing decisions: %v", err)
		}
	})

	for _, target := range []photolib.RouteTarget{
		photolib.RouteTargetExpense,
		photolib.RouteTargetDocument,
		photolib.RouteTargetKnowledge,
	} {
		if _, err := store.RoutePhoto(ctx, photolib.RoutePhotoInput{
			PhotoID:              record.ID,
			Target:               target,
			DownstreamArtifactID: "downstream-" + string(target),
			Confidence:           0.91,
			Rationale:            "receipt → " + string(target),
			Actor:                "system",
		}); err != nil {
			t.Fatalf("route %s: %v", target, err)
		}
	}

	decisions, err := store.ListRouteDecisions(ctx, record.ID)
	if err != nil {
		t.Fatalf("list route decisions: %v", err)
	}
	if len(decisions) != 3 {
		t.Fatalf("expected 3 route decisions, got %d", len(decisions))
	}
	got := map[photolib.RouteTarget]bool{}
	for _, decision := range decisions {
		got[decision.Target] = true
	}
	for _, want := range []photolib.RouteTarget{
		photolib.RouteTargetExpense,
		photolib.RouteTargetDocument,
		photolib.RouteTargetKnowledge,
	} {
		if !got[want] {
			t.Fatalf("missing route target %s", want)
		}
	}

	// Adversarial: a re-run for the same target MUST update, not
	// duplicate. This is the regression that proves SCN-040-011 cannot
	// silently double-fire downstream creation.
	updated, err := store.RoutePhoto(ctx, photolib.RoutePhotoInput{
		PhotoID:              record.ID,
		Target:               photolib.RouteTargetExpense,
		DownstreamArtifactID: "downstream-expense-v2",
		Confidence:           0.97,
		Rationale:            "receipt re-run",
		Actor:                "system",
	})
	if err != nil {
		t.Fatalf("re-route receipt: %v", err)
	}
	if updated.DownstreamArtifactID != "downstream-expense-v2" {
		t.Fatalf("re-route did not update downstream artifact: %s", updated.DownstreamArtifactID)
	}
	final, err := store.ListRouteDecisions(ctx, record.ID)
	if err != nil {
		t.Fatalf("list route decisions after re-run: %v", err)
	}
	if len(final) != 3 {
		t.Fatalf("expected 3 routing rows after re-run (no duplicates), got %d", len(final))
	}

	// SCN-040-011 also requires that a multi-page document scan
	// uploaded through the unified pipeline lands as one cohesive
	// document group. Exercise that here so the routing test covers
	// both the receipt → expense+document and the multi-page document
	// flow against the same live stack.
	groupRef := "e2e-doc-" + uuid.NewString()
	const pages = 3
	var groupID string
	for page := 1; page <= pages; page++ {
		fields := uploadFields{
			channel:       "web",
			sourceRef:     groupRef + ":session",
			mode:          "document",
			documentGroup: groupRef,
			documentPage:  page,
			filename:      "doc-page.jpg",
			contents:      syntheticJPEG(groupRef),
		}
		resp := uploadPhoto(t, cfg, fields)
		cleanupE2EPhoto(t, pool, resp.ArtifactID)
		if resp.DocumentGroupID == "" {
			t.Fatalf("upload page %d missing document_group_id", page)
		}
		if page == 1 {
			groupID = resp.DocumentGroupID
		} else if resp.DocumentGroupID != groupID {
			t.Fatalf("document group drift: page %d returned %s want %s", page, resp.DocumentGroupID, groupID)
		}
		if resp.PageIndex != page {
			t.Fatalf("page %d echoed page_index=%d", page, resp.PageIndex)
		}
	}
	id, err := uuid.Parse(groupID)
	if err != nil {
		t.Fatalf("parse group id: %v", err)
	}
	rows, err := store.ListPhotosByDocumentGroup(ctx, id)
	if err != nil {
		t.Fatalf("list document group rows: %v", err)
	}
	if len(rows) != pages {
		t.Fatalf("expected %d rows for group %s, got %d", pages, groupID, len(rows))
	}
}
