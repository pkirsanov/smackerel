// Spec 074 SCOPE-3 — per-user dedup semantics.
//
// Two DedupStore implementations:
//
//   - MemDedupStore: in-memory, used by SCOPE-3 unit tests (TP-074-08,
//     TP-074-09) to prove same-user same-window dedup and
//     outside-window new-bucket semantics without a live DB.
//   - PostgresDedupStore: pgxpool-backed implementation against
//     artifact_capture_policy (migration 051), used by SCOPE-3
//     integration test TP-074-10 (cross-user separation) and wired
//     by SCOPE-4's facade.
//
// Dedup key — by spec — is (user_id, provenance='capture-as-fallback',
// normalized_text_hash, dedup_bucket_start). Provenance is fixed to
// fallback because explicit (spec 008) captures NEVER enter fallback
// dedup (SCN-074-A02 — enforced by the partial unique index in 051).

package capturefallback

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MemDedupStore is the in-memory DedupStore reference implementation.
// Safe for concurrent use.
type MemDedupStore struct {
	mu      sync.Mutex
	entries map[memDedupKey]string // → artifact id
}

type memDedupKey struct {
	userID     string
	provenance Provenance
	hash       string
	bucket     int64 // bucket-start UTC unix nanos
}

// NewMemDedupStore constructs an empty in-memory DedupStore.
func NewMemDedupStore() *MemDedupStore {
	return &MemDedupStore{entries: make(map[memDedupKey]string)}
}

func memKey(userID string, dec Decision) memDedupKey {
	return memDedupKey{
		userID:     userID,
		provenance: dec.Provenance,
		hash:       dec.NormalizedTextHash,
		bucket:     dec.DedupBucketStart.UTC().UnixNano(),
	}
}

// Lookup implements DedupStore.Lookup.
func (s *MemDedupStore) Lookup(_ context.Context, userID string, dec Decision) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.entries[memKey(userID, dec)]
	return id, ok, nil
}

// Record implements DedupStore.Record. Returns an error on a
// programming-mistake double-record for the same key (the policy's
// lookup-before-record discipline should prevent this; the explicit
// error keeps test failures loud).
func (s *MemDedupStore) Record(_ context.Context, userID, artifactID string, dec Decision) error {
	if artifactID == "" {
		return errors.New("capturefallback: MemDedupStore.Record: empty artifactID")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	k := memKey(userID, dec)
	if existing, ok := s.entries[k]; ok && existing != artifactID {
		return fmt.Errorf("capturefallback: MemDedupStore.Record: duplicate key for user %q (existing=%s, new=%s)", userID, existing, artifactID)
	}
	s.entries[k] = artifactID
	return nil
}

// PostgresDedupStore is the pgxpool-backed DedupStore against
// artifact_capture_policy. It reuses the same row that
// PostgresStore.Record persists so SCOPE-4's facade does NOT need to
// write the row twice — Policy.CaptureForUser calls Record exactly
// once after the IdeaWriter creates the artifact.
type PostgresDedupStore struct {
	pool *pgxpool.Pool
}

// NewPostgresDedupStore constructs the pgxpool-backed DedupStore.
// A nil pool is a programming error.
func NewPostgresDedupStore(pool *pgxpool.Pool) *PostgresDedupStore {
	if pool == nil {
		panic("capturefallback.NewPostgresDedupStore: pool is nil")
	}
	return &PostgresDedupStore{pool: pool}
}

const dedupLookupSQL = `
SELECT artifact_id FROM artifact_capture_policy
 WHERE user_id = $1
   AND provenance = 'capture-as-fallback'
   AND normalized_text_hash = $2
   AND dedup_bucket_start = $3
 LIMIT 1
`

// Lookup implements DedupStore.Lookup. Provenance is hard-coded to
// 'capture-as-fallback' in the SQL: explicit captures never enter
// fallback dedup (SCN-074-A02).
func (s *PostgresDedupStore) Lookup(ctx context.Context, userID string, dec Decision) (string, bool, error) {
	if userID == "" {
		return "", false, ErrMissingUser
	}
	if dec.NormalizedTextHash == "" {
		return "", false, errors.New("capturefallback: Lookup: decision missing NormalizedTextHash")
	}
	if dec.DedupBucketStart.IsZero() {
		return "", false, errors.New("capturefallback: Lookup: decision missing DedupBucketStart")
	}
	var id string
	err := s.pool.QueryRow(ctx, dedupLookupSQL, userID, dec.NormalizedTextHash, dec.DedupBucketStart.UTC()).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("capturefallback: dedup lookup: %w", err)
	}
	return id, true, nil
}

// Record implements DedupStore.Record by inserting the
// artifact_capture_policy row that future Lookups will see. SourceTurnID
// and other turn-context fields come from Decision (populated by Decide
// from the originating Request). The partial unique index on
// (user_id, provenance, normalized_text_hash, dedup_bucket_start)
// WHERE provenance='capture-as-fallback' is the second line of defense
// against a race between two concurrent in-window fallback writes for
// the same user/text.
func (s *PostgresDedupStore) Record(ctx context.Context, userID, artifactID string, dec Decision) error {
	if userID == "" {
		return ErrMissingUser
	}
	if artifactID == "" {
		return errors.New("capturefallback: Record: empty artifactID")
	}
	payload := CapturePayload{
		ArtifactID:             artifactID,
		UserID:                 userID,
		Provenance:             ProvenanceFallback,
		FallbackCause:          dec.Cause,
		NormalizedText:         dec.NormalizedText,
		NormalizedTextHash:     dec.NormalizedTextHash,
		DedupBucketStart:       dec.DedupBucketStart.UTC(),
		DedupWindowSeconds:     int(dec.DedupWindow / time.Second),
		SourceTurnID:           dec.SourceTurnID,
		IntentTraceID:          dec.IntentTraceID,
		AbandonedClarification: dec.AbandonedClarification,
		SchemaVersion:          dec.SchemaVersion,
		CreatedAt:              dec.OccurredAt.UTC(),
	}
	if err := validatePayload(payload); err != nil {
		return err
	}
	store := &PostgresStore{pool: s.pool}
	return store.Record(ctx, payload)
}
