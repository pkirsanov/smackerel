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

// TestPhotosDedupe_BurstHDRPanoramaAndExactClusters proves SCN-040-008.
// Each cluster kind in the planning taxonomy must persist with a
// best-pick attribution, an audit event, and reject inputs that lack
// rationale/confidence or a valid best photo.
func TestPhotosDedupe_BurstHDRPanoramaAndExactClusters(t *testing.T) {
	pool := testPool(t)
	store := photolib.NewStore(pool)
	analyzer := photolib.NewDedupeAnalyzer(store)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	connectorID := "connector-040-scope3-dedupe"
	t.Cleanup(func() { cleanupPhotosByConnector(t, pool, connectorID) })

	createPhoto := func(label string) *photolib.PhotoRecord {
		t.Helper()
		event := photolib.SyntheticPhotoEvent()
		event.ProviderRef = testID(t) + "-" + label
		event.ContentHash = "sha256:dedupe-" + label + "-" + strings.ReplaceAll(event.ProviderRef, "/", "-")
		record, err := store.PublishPhotoEvent(ctx, connectorID, "synthetic", event)
		if err != nil {
			t.Fatalf("publish %s: %v", label, err)
		}
		cleanupPhoto(t, record.ArtifactID)
		return record
	}

	a := createPhoto("burst-1")
	b := createPhoto("burst-2")
	c := createPhoto("hdr-1")
	d := createPhoto("hdr-2")
	e := createPhoto("pano-1")
	f := createPhoto("pano-2")
	g := createPhoto("exact-1")
	h := createPhoto("exact-2")

	// Adversarial: rationale missing must be rejected.
	if _, err := analyzer.Apply(ctx, photolib.ClusterDecisionInput{
		Kind:        photolib.ClusterBurst,
		Provider:    "synthetic",
		PhotoIDs:    []uuid.UUID{a.ID, b.ID},
		BestPhotoID: a.ID,
		Confidence:  0.9,
	}); err == nil {
		t.Fatalf("expected dedupe.Apply with empty rationale to fail")
	}

	// Adversarial: best photo not in members must be rejected.
	if _, err := analyzer.Apply(ctx, photolib.ClusterDecisionInput{
		Kind:        photolib.ClusterBurst,
		Provider:    "synthetic",
		PhotoIDs:    []uuid.UUID{a.ID, b.ID},
		BestPhotoID: g.ID,
		Confidence:  0.9,
		Rationale:   "burst sequence with shared exposure",
	}); err == nil {
		t.Fatalf("expected dedupe.Apply with non-member best photo to fail")
	}

	// Adversarial: only one member must be rejected.
	if _, err := analyzer.Apply(ctx, photolib.ClusterDecisionInput{
		Kind:        photolib.ClusterBurst,
		Provider:    "synthetic",
		PhotoIDs:    []uuid.UUID{a.ID},
		BestPhotoID: a.ID,
		Confidence:  0.9,
		Rationale:   "burst sequence",
	}); err == nil {
		t.Fatalf("expected dedupe.Apply with single member to fail")
	}

	cases := []struct {
		name    string
		kind    photolib.ClusterKind
		members []uuid.UUID
		best    uuid.UUID
	}{
		{name: "burst", kind: photolib.ClusterBurst, members: []uuid.UUID{a.ID, b.ID}, best: a.ID},
		{name: "hdr", kind: photolib.ClusterHDR, members: []uuid.UUID{c.ID, d.ID}, best: d.ID},
		{name: "panorama", kind: photolib.ClusterPanoramaMember, members: []uuid.UUID{e.ID, f.ID}, best: e.ID},
		{name: "exact", kind: photolib.ClusterExactHash, members: []uuid.UUID{g.ID, h.ID}, best: g.ID},
	}
	createdIDs := make([]uuid.UUID, 0, len(cases))
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			cluster, err := analyzer.Apply(ctx, photolib.ClusterDecisionInput{
				Kind:         c.kind,
				Provider:     "synthetic",
				PhotoIDs:     c.members,
				BestPhotoID:  c.best,
				BestPickedBy: "llm",
				Confidence:   0.93,
				Rationale:    "synthetic " + c.name + " sequence",
				ModelVersion: "test-model-1",
			})
			if err != nil {
				t.Fatalf("apply %s cluster: %v", c.name, err)
			}
			if cluster.Kind != c.kind {
				t.Fatalf("kind=%q, want %q", cluster.Kind, c.kind)
			}
			if cluster.BestPhotoID == nil || *cluster.BestPhotoID != c.best {
				t.Fatalf("best_photo_id mismatch: got %v want %v", cluster.BestPhotoID, c.best)
			}
			if len(cluster.Members) != len(c.members) {
				t.Fatalf("members=%d, want %d", len(cluster.Members), len(c.members))
			}
			createdIDs = append(createdIDs, cluster.ID)
		})
	}

	// User override path: SetBestPick must move best role to the new
	// pick and write an audit event.
	override, err := store.SetBestPick(ctx, createdIDs[0], cases[0].members[1], "user", "tester")
	if err != nil {
		t.Fatalf("set best pick: %v", err)
	}
	if override.BestPickedBy != "user" {
		t.Fatalf("best_picked_by=%q, want user", override.BestPickedBy)
	}
	if override.BestPhotoID == nil || *override.BestPhotoID != cases[0].members[1] {
		t.Fatalf("override best_photo_id mismatch: got %v want %v", override.BestPhotoID, cases[0].members[1])
	}

	events, err := store.ListAuditEvents(ctx, "cluster_", 50)
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	if len(events) < 5 {
		t.Fatalf("expected at least 5 cluster audit events, got %d", len(events))
	}

	t.Cleanup(func() {
		cleanCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for _, id := range createdIDs {
			_, _ = pool.Exec(cleanCtx, `DELETE FROM photo_cluster_members WHERE cluster_id=$1`, id)
			_, _ = pool.Exec(cleanCtx, `DELETE FROM photo_clusters WHERE id=$1`, id)
		}
	})
}
