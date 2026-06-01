// Spec 074 SCOPE-2 — Postgres-backed CapturePolicyStore.
//
// Persists artifact_capture_policy rows for both explicit (spec 008)
// and fallback (spec 074) captures. SCOPE-2 wires the explicit-side
// recorder through the /api/capture success path
// (see internal/api/capture.go). SCOPE-3 will add per-user dedup
// lookups against the same table; SCOPE-4 will write fallback-side
// rows when the facade routes an unrouted/no-ground/abandoned-
// clarification turn through the capture path.

package capturefallback

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CapturePolicyStore is the persistence seam for artifact_capture_policy.
type CapturePolicyStore interface {
	Record(ctx context.Context, p CapturePayload) error
	CountByProvenance(ctx context.Context, userID string, provenance Provenance) (int, error)
	GetByArtifactID(ctx context.Context, artifactID string) (CapturePayload, error)
}

// ErrNotFound is returned by GetByArtifactID when no row exists.
var ErrNotFound = errors.New("capturefallback: capture policy row not found")

// PostgresStore is the pgxpool-backed CapturePolicyStore.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore constructs a CapturePolicyStore against the given
// pgxpool. A nil pool is a programming error.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	if pool == nil {
		panic("capturefallback.NewPostgresStore: pool is nil")
	}
	return &PostgresStore{pool: pool}
}

const insertSQL = `
INSERT INTO artifact_capture_policy (
    artifact_id, user_id, provenance, fallback_cause,
    normalized_text_hash, dedup_bucket_start, dedup_window_seconds,
    source_turn_id, intent_trace_id, abandoned_clarification,
    already_captured_source_id, schema_version, created_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
`

// Record implements CapturePolicyStore.Record.
func (s *PostgresStore) Record(ctx context.Context, p CapturePayload) error {
	if err := validatePayload(p); err != nil {
		return err
	}
	var dedupBucket, dedupWindow, fallbackCause, intentTrace, already any
	if !p.DedupBucketStart.IsZero() {
		dedupBucket = p.DedupBucketStart
	}
	if p.DedupWindowSeconds > 0 {
		dedupWindow = p.DedupWindowSeconds
	}
	if p.FallbackCause != "" {
		fallbackCause = string(p.FallbackCause)
	}
	if p.IntentTraceID != "" {
		intentTrace = p.IntentTraceID
	}
	if p.AlreadyCapturedSourceID != "" {
		already = p.AlreadyCapturedSourceID
	}
	if _, err := s.pool.Exec(ctx, insertSQL,
		p.ArtifactID, p.UserID, string(p.Provenance), fallbackCause,
		p.NormalizedTextHash, dedupBucket, dedupWindow,
		p.SourceTurnID, intentTrace, p.AbandonedClarification,
		already, p.SchemaVersion, p.CreatedAt,
	); err != nil {
		return fmt.Errorf("capturefallback: insert artifact_capture_policy: %w", err)
	}
	return nil
}

// CountByProvenance implements CapturePolicyStore.CountByProvenance.
func (s *PostgresStore) CountByProvenance(ctx context.Context, userID string, provenance Provenance) (int, error) {
	var n int
	if err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM artifact_capture_policy WHERE user_id=$1 AND provenance=$2`,
		userID, string(provenance),
	).Scan(&n); err != nil {
		return 0, fmt.Errorf("capturefallback: count by provenance: %w", err)
	}
	return n, nil
}

// GetByArtifactID implements CapturePolicyStore.GetByArtifactID.
func (s *PostgresStore) GetByArtifactID(ctx context.Context, artifactID string) (CapturePayload, error) {
	var (
		p             CapturePayload
		provenance    string
		fallbackCause *string
		dedupBucket   *time.Time
		dedupWindow   *int
		intentTrace   *string
		already       *string
	)
	err := s.pool.QueryRow(ctx, `
SELECT artifact_id, user_id, provenance, fallback_cause,
       normalized_text_hash, dedup_bucket_start, dedup_window_seconds,
       source_turn_id, intent_trace_id, abandoned_clarification,
       already_captured_source_id, schema_version, created_at
FROM artifact_capture_policy WHERE artifact_id = $1`, artifactID).Scan(
		&p.ArtifactID, &p.UserID, &provenance, &fallbackCause,
		&p.NormalizedTextHash, &dedupBucket, &dedupWindow,
		&p.SourceTurnID, &intentTrace, &p.AbandonedClarification,
		&already, &p.SchemaVersion, &p.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return CapturePayload{}, ErrNotFound
	}
	if err != nil {
		return CapturePayload{}, fmt.Errorf("capturefallback: get by artifact id: %w", err)
	}
	p.Provenance = Provenance(provenance)
	if fallbackCause != nil {
		p.FallbackCause = Cause(*fallbackCause)
	}
	if dedupBucket != nil {
		p.DedupBucketStart = *dedupBucket
	}
	if dedupWindow != nil {
		p.DedupWindowSeconds = *dedupWindow
	}
	if intentTrace != nil {
		p.IntentTraceID = *intentTrace
	}
	if already != nil {
		p.AlreadyCapturedSourceID = *already
	}
	return p, nil
}

// ExplicitCaptureInput is the closed input to BuildExplicitPayload.
type ExplicitCaptureInput struct {
	ArtifactID     string
	UserID         string
	NormalizedText string
	DedupHashKey   string
	SourceTurnID   string
	IntentTraceID  string
	CreatedAt      time.Time
}

// BuildExplicitPayload constructs a CapturePayload for a spec 008
// explicit capture. Explicit captures leave DedupBucketStart zero and
// DedupWindowSeconds 0 so the fallback-only unique index in the
// migration does NOT apply to them (SCN-074-A02 — explicit and
// fallback captures of the same normalized text must remain separate).
func BuildExplicitPayload(input ExplicitCaptureInput) CapturePayload {
	return CapturePayload{
		ArtifactID:             input.ArtifactID,
		UserID:                 input.UserID,
		Provenance:             ProvenanceExplicit,
		NormalizedText:         input.NormalizedText,
		NormalizedTextHash:     HashNormalized(input.NormalizedText, input.DedupHashKey),
		SourceTurnID:           input.SourceTurnID,
		IntentTraceID:          input.IntentTraceID,
		AbandonedClarification: false,
		SchemaVersion:          SchemaVersion,
		CreatedAt:              input.CreatedAt.UTC(),
	}
}

func validatePayload(p CapturePayload) error {
	if p.ArtifactID == "" {
		return errors.New("capturefallback: payload missing ArtifactID")
	}
	if p.UserID == "" {
		return errors.New("capturefallback: payload missing UserID")
	}
	if _, ok := validProvenances[p.Provenance]; !ok {
		return fmt.Errorf("capturefallback: payload provenance %q not in closed vocabulary", p.Provenance)
	}
	if p.Provenance == ProvenanceFallback {
		if _, ok := validCauses[p.FallbackCause]; !ok {
			return fmt.Errorf("capturefallback: fallback payload cause %q not in closed vocabulary", p.FallbackCause)
		}
	}
	if p.NormalizedTextHash == "" {
		return errors.New("capturefallback: payload missing NormalizedTextHash")
	}
	if p.SourceTurnID == "" {
		return errors.New("capturefallback: payload missing SourceTurnID")
	}
	if p.SchemaVersion <= 0 {
		return errors.New("capturefallback: payload missing SchemaVersion")
	}
	if p.CreatedAt.IsZero() {
		return errors.New("capturefallback: payload missing CreatedAt")
	}
	return nil
}

var validProvenances = func() map[Provenance]struct{} {
	m := make(map[Provenance]struct{}, len(AllProvenances))
	for _, pr := range AllProvenances {
		m[pr] = struct{}{}
	}
	return m
}()
