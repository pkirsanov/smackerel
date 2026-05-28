// Package ingest implements the server-authoritative dedup contract
// shared by live ingestion paths. Spec 058 Scope 2 is the first
// consumer (POST /v1/connectors/extension/ingest); the same keyer +
// store is reusable by any future ingestion surface that needs the
// "url + content_type + device + time-bucket" collapse semantics.
package ingest

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// dedupKeySeparator is an explicit null byte between dedup-tuple
// fields. Without it, ("ab", "c") and ("a", "bc") would collide on
// concatenation; the separator removes that boundary ambiguity. The
// boundary-collision regression test pins this guarantee.
const dedupKeySeparator = "\x00"

// ComputeDedupKey returns the SHA-256 digest of the canonical dedup
// tuple. The tuple is (url, contentType, deviceID, bucket) joined by
// explicit null bytes. Spec 058 design §2.3.
//
// Bucket semantics (the caller computes the bucket):
//   - bookmarks: 0 always (window is bypassed; one row per
//     url+content_type+device for the lifetime of the bookmark).
//   - history (and other time-bucketed types): floor(captured_at_unix /
//     window_seconds), where window_seconds is clamped to [60, 86400]
//     by the caller before being passed in here.
func ComputeDedupKey(url, contentType, deviceID string, bucket int64) []byte {
	h := sha256.New()
	h.Write([]byte(url))
	h.Write([]byte(dedupKeySeparator))
	h.Write([]byte(contentType))
	h.Write([]byte(dedupKeySeparator))
	h.Write([]byte(deviceID))
	h.Write([]byte(dedupKeySeparator))
	h.Write([]byte(strconv.FormatInt(bucket, 10)))
	return h.Sum(nil)
}

// DedupRow is the SST shape for a dedup record. Callers populate
// every field except Key (computed via ComputeDedupKey).
type DedupRow struct {
	Key            []byte
	OwnerUserID    string
	SourceID       string
	ContentType    string
	SourceDeviceID string
	CapturedAt     time.Time
}

// PublishFunc is the callback that the dedup store invokes ONLY when
// no existing row matches the key. It MUST publish the artifact
// downstream (Postgres + NATS via pipeline.ArtifactPublisher) and
// return the canonical artifact id. The dedup store then binds the
// returned id to the dedup key so subsequent collisions resolve to
// the same artifact.
type PublishFunc func(ctx context.Context) (artifactID string, err error)

// DedupStore resolves a dedup key against the persistent dedup table.
// Implementations MUST be safe for concurrent use.
type DedupStore interface {
	// ResolveOrPublish atomically resolves the key. On collision it
	// increments visit_count, bumps last_seen_at, and returns the
	// existing artifact_id with deduped=true. On a fresh key it
	// invokes publish(ctx), persists the dedup row bound to the
	// returned artifact_id, and returns (artifact_id, false, nil).
	//
	// A publish error is returned verbatim; no dedup row is written
	// in that case so a retry can attempt the publish again.
	ResolveOrPublish(ctx context.Context, row DedupRow, publish PublishFunc) (artifactID string, deduped bool, err error)
}

// NoOpDedupStore is the Scope-1-stage wiring helper. It performs no
// dedup lookup: every call invokes publish(ctx) and returns the
// resulting id with deduped=false. Scope 2 swaps in
// NewPostgresDedupStore for the production path.
type NoOpDedupStore struct{}

// ResolveOrPublish satisfies DedupStore by always invoking publish.
func (NoOpDedupStore) ResolveOrPublish(ctx context.Context, _ DedupRow, publish PublishFunc) (string, bool, error) {
	if publish == nil {
		return "", false, errors.New("ingest: publish callback required")
	}
	id, err := publish(ctx)
	if err != nil {
		return "", false, err
	}
	return id, false, nil
}

// PostgresDedupStore persists dedup rows in raw_ingest_dedup.
type PostgresDedupStore struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

// NewPostgresDedupStore returns a DedupStore backed by the supplied
// pgxpool.Pool. The optional now function override is used by tests
// to pin last_seen_at values; production callers pass nil.
func NewPostgresDedupStore(pool *pgxpool.Pool) *PostgresDedupStore {
	if pool == nil {
		panic("ingest: NewPostgresDedupStore requires a non-nil pool")
	}
	return &PostgresDedupStore{pool: pool, now: time.Now}
}

// WithNow returns a copy of the store with the supplied clock function
// — primarily for tests that need deterministic last_seen_at values.
func (s *PostgresDedupStore) WithNow(now func() time.Time) *PostgresDedupStore {
	cp := *s
	cp.now = now
	return &cp
}

// ResolveOrPublish implements the spec 058 §2.3 contract.
func (s *PostgresDedupStore) ResolveOrPublish(ctx context.Context, row DedupRow, publish PublishFunc) (string, bool, error) {
	if publish == nil {
		return "", false, errors.New("ingest: publish callback required")
	}
	if len(row.Key) == 0 {
		return "", false, errors.New("ingest: dedup row missing Key")
	}

	now := s.now()

	// Fast path: collision. Increment visit_count + bump last_seen_at
	// in one statement and return the existing artifact_id. A 0-row
	// update means the key is new; fall through to the publish path.
	var existingID string
	err := s.pool.QueryRow(ctx, `
		UPDATE raw_ingest_dedup
		SET visit_count = visit_count + 1,
		    last_seen_at = $2
		WHERE dedup_key = $1
		RETURNING artifact_id
	`, row.Key, now).Scan(&existingID)
	if err == nil {
		return existingID, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", false, fmt.Errorf("ingest: dedup update: %w", err)
	}

	// Fresh key: publish first, then bind. ON CONFLICT covers the
	// concurrent-insert race: if another worker inserted between the
	// UPDATE above and this INSERT, the existing row's artifact_id
	// is returned and we treat this call as a dedup hit (the just-
	// published artifact in that race remains in the artifacts table
	// but no future request will resolve to it via this key).
	artifactID, err := publish(ctx)
	if err != nil {
		return "", false, err
	}

	capturedAt := row.CapturedAt
	if capturedAt.IsZero() {
		capturedAt = now
	}

	var boundID string
	var inserted bool
	err = s.pool.QueryRow(ctx, `
		INSERT INTO raw_ingest_dedup
			(dedup_key, owner_user_id, source_id, content_type,
			 source_device_id, artifact_id, first_seen_at, last_seen_at, visit_count)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $7, 1)
		ON CONFLICT (dedup_key) DO UPDATE
			SET visit_count = raw_ingest_dedup.visit_count + 1,
			    last_seen_at = EXCLUDED.last_seen_at
		RETURNING artifact_id, (xmax = 0) AS inserted
	`,
		row.Key,
		row.OwnerUserID,
		row.SourceID,
		row.ContentType,
		row.SourceDeviceID,
		artifactID,
		capturedAt,
	).Scan(&boundID, &inserted)
	if err != nil {
		return "", false, fmt.Errorf("ingest: dedup insert: %w", err)
	}
	if !inserted {
		// Lost the race; the row already existed and visit_count was
		// just incremented. The caller's perspective: this is a
		// dedup hit, NOT a fresh insert.
		return boundID, true, nil
	}
	return artifactID, false, nil
}

// Compile-time assertions that both stores satisfy DedupStore.
var (
	_ DedupStore = NoOpDedupStore{}
	_ DedupStore = (*PostgresDedupStore)(nil)
)
