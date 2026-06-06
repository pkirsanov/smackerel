//go:build integration

// Spec 058 BUG-058-DEDUP-KEY-OWNER-ISOLATION — live-Postgres proof that the
// owner-namespaced dedup key isolates tenants. Two DIFFERENT authenticated
// owners that emit an IDENTICAL (url, content_type, source_device_id, bucket)
// tuple MUST resolve to TWO distinct raw_ingest_dedup rows and TWO distinct
// artifact_ids — neither owner's publish is skipped, and neither owner
// receives the other's artifact_id.
//
// Adversarial cover: if owner_user_id is ever dropped from the
// ComputeDedupKey preimage, owner A and owner B produce the SAME dedup_key.
// Owner B's ResolveOrPublish then matches owner A's row and returns
// (art-A, deduped=true), and this test fails at the deduped / distinct-id /
// distinct-key assertions. This is the AC-3 (integration tier) gate of
// BUG-058-DEDUP-KEY-OWNER-ISOLATION. It runs in CI where DATABASE_URL points
// at the ephemeral integration Postgres; locally it skips cleanly when the
// live stack is not up.

package integration

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/connector/ingest"
)

// TestPostgresDedupStore_CrossOwnerIsolation pins the BUG-058 owner-isolation
// contract against the live stack using the REAL ComputeDedupKey keyer and the
// REAL PostgresDedupStore.ResolveOrPublish upsert path.
func TestPostgresDedupStore_CrossOwnerIsolation(t *testing.T) {
	pool := testPool(t)
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tid := testID(t)
	const (
		url    = "https://github.com"
		ct     = "bookmark"
		device = "laptop" // a natural operator-set device id both owners pick
		bucket = int64(0)
	)
	ownerA := "owner-a-" + tid
	ownerB := "owner-b-" + tid
	keyA := ingest.ComputeDedupKey(ownerA, url, ct, device, bucket)
	keyB := ingest.ComputeDedupKey(ownerB, url, ct, device, bucket)

	artA := "bug058-iso-" + tid + "-A"
	artB := "bug058-iso-" + tid + "-B"

	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer ccancel()
		_, _ = pool.Exec(cctx, `DELETE FROM raw_ingest_dedup WHERE dedup_key = $1`, keyA)
		_, _ = pool.Exec(cctx, `DELETE FROM raw_ingest_dedup WHERE dedup_key = $1`, keyB)
		_, _ = pool.Exec(cctx, `DELETE FROM artifacts WHERE id LIKE $1`, "bug058-iso-"+tid+"-%")
	})

	store := ingest.NewPostgresDedupStore(pool)

	// publishFor seeds a unique artifact row (so the dedup FK is satisfied)
	// and returns its id.
	publishFor := func(id string) ingest.PublishFunc {
		return func(ctx context.Context) (string, error) {
			if _, err := pool.Exec(ctx, `
				INSERT INTO artifacts (id, artifact_type, title, content_hash, source_id)
				VALUES ($1, 'bookmark', $2, $3, 'browser-extension')
			`, id, "iso-"+id, "h-"+id); err != nil {
				return "", fmt.Errorf("seed artifact %s: %w", id, err)
			}
			return id, nil
		}
	}

	rowFor := func(owner string, key []byte) ingest.DedupRow {
		return ingest.DedupRow{
			Key:            key,
			OwnerUserID:    owner,
			SourceID:       "browser-extension",
			ContentType:    ct,
			SourceDeviceID: device,
			CapturedAt:     time.Now().UTC().Truncate(time.Second),
		}
	}

	// Owner A publishes first — fresh row.
	gotA, dupA, err := store.ResolveOrPublish(ctx, rowFor(ownerA, keyA), publishFor(artA))
	if err != nil {
		t.Fatalf("owner A ResolveOrPublish: %v", err)
	}
	if dupA {
		t.Fatal("owner A first write must NOT be a dedup hit")
	}
	if gotA != artA {
		t.Fatalf("owner A: got artifact %q, want %q", gotA, artA)
	}

	// Owner B, SAME (url, content_type, device, bucket) but a DIFFERENT owner.
	// MUST be a fresh publish into its OWN separate row.
	gotB, dupB, err := store.ResolveOrPublish(ctx, rowFor(ownerB, keyB), publishFor(artB))
	if err != nil {
		t.Fatalf("owner B ResolveOrPublish: %v", err)
	}
	if dupB {
		t.Fatal("cross-tenant collapse: owner B was deduped onto owner A's row (publish skipped)")
	}
	if gotB != artB {
		t.Fatalf("owner B must receive its OWN artifact id; got %q (art-A would be a cross-tenant id leak)", gotB)
	}

	// The two owners' keys MUST differ.
	if bytes.Equal(keyA, keyB) {
		t.Fatal("owner A and owner B produced the SAME dedup_key — owner namespacing missing")
	}

	// Two distinct rows must exist, each bound to its own owner's artifact.
	var boundA, ownerColA string
	if err := pool.QueryRow(ctx, `SELECT artifact_id, owner_user_id FROM raw_ingest_dedup WHERE dedup_key = $1`, keyA).Scan(&boundA, &ownerColA); err != nil {
		t.Fatalf("read owner A row: %v", err)
	}
	var boundB, ownerColB string
	if err := pool.QueryRow(ctx, `SELECT artifact_id, owner_user_id FROM raw_ingest_dedup WHERE dedup_key = $1`, keyB).Scan(&boundB, &ownerColB); err != nil {
		t.Fatalf("read owner B row: %v", err)
	}
	if boundA != artA || ownerColA != ownerA {
		t.Fatalf("owner A row mismatch: artifact=%q owner=%q (want %q / %q)", boundA, ownerColA, artA, ownerA)
	}
	if boundB != artB || ownerColB != ownerB {
		t.Fatalf("owner B row mismatch: artifact=%q owner=%q (want %q / %q)", boundB, ownerColB, artB, ownerB)
	}
	if boundA == boundB {
		t.Fatalf("cross-tenant collapse: both owners bound to the same artifact %q", boundA)
	}
}
