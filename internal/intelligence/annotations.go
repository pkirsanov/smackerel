package intelligence

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"

	"github.com/nats-io/nats.go"

	"github.com/smackerel/smackerel/internal/annotation"
	smacknats "github.com/smackerel/smackerel/internal/nats"
)

// SubscribeAnnotations subscribes to annotation.created NATS events
// and triggers relevance score updates for affected artifacts.
func (e *Engine) SubscribeAnnotations(ctx context.Context) error {
	if e.NATS == nil || e.NATS.Conn == nil {
		return fmt.Errorf("NATS connection not available")
	}

	_, err := e.NATS.Conn.Subscribe(smacknats.SubjectAnnotationsCreated, func(msg *nats.Msg) {
		var ann annotation.Annotation
		if err := json.Unmarshal(msg.Data, &ann); err != nil {
			slog.Warn("failed to unmarshal annotation event", "error", err)
			return
		}

		if err := e.updateRelevanceFromAnnotation(ctx, &ann); err != nil {
			slog.Warn("failed to update relevance from annotation",
				"error", err,
				"artifact_id", ann.ArtifactID,
				"annotation_type", ann.AnnotationType,
			)
		}
	})
	if err != nil {
		return fmt.Errorf("subscribe to %s: %w", smacknats.SubjectAnnotationsCreated, err)
	}

	slog.Info("intelligence engine subscribed to annotation events")
	return nil
}

// updateRelevanceFromAnnotation atomically applies the per-annotation
// relevance delta to the artifact row.
//
// BUG-027-002 reliability fix: the prior implementation read the
// current `relevance_score` with `SELECT` and then wrote the new
// value with `UPDATE` in two separate round-trips. Annotation events
// arrive concurrently through the NATS subscriber callback (each
// callback runs in its own goroutine), so two events for the same
// artifact in burst would race read→write and silently lose one
// delta (classic last-write-wins). The fix collapses the read and
// the write into a single atomic statement: PostgreSQL serializes
// concurrent UPDATEs against the same row via the row-level write
// lock, and the in-statement arithmetic ensures every delta is
// applied exactly once. The bounded-range clamp [0, 1] now lives
// inside the SQL with GREATEST/LEAST so the invariant is enforced
// at the storage layer.
func (e *Engine) updateRelevanceFromAnnotation(ctx context.Context, ann *annotation.Annotation) error {
	delta := annotationRelevanceDelta(ann)
	if delta == 0 {
		return nil
	}

	// Atomically apply the delta and return the post-update score so
	// the structured-log line records the actual persisted value
	// after the clamp. RETURNING runs after the write commits the
	// row-level lock, so the returned `newScore` is consistent with
	// the persisted row even under concurrent subscribers.
	var newScore float64
	err := e.Pool.QueryRow(ctx, `
		UPDATE artifacts
		SET relevance_score = LEAST(GREATEST(COALESCE(relevance_score, 0.5) + $1, 0), 1)
		WHERE id = $2
		RETURNING relevance_score
	`, delta, ann.ArtifactID).Scan(&newScore)
	if err != nil {
		return fmt.Errorf("atomically update relevance score: %w", err)
	}

	slog.Debug("relevance score updated",
		"artifact_id", ann.ArtifactID,
		"delta", delta,
		"new_score", newScore,
	)
	return nil
}

// ApplyAnnotationRelevanceForTest is a test-only exported wrapper
// around the unexported updateRelevanceFromAnnotation. It exists so
// the BUG-027-002 race regression integration tests can drive the
// same SQL path without reaching through NATS (the production caller
// is the SubscribeAnnotations subscriber callback).
//
// This wrapper is NOT for production code. The exported name is
// suffixed with `ForTest` and this godoc explicitly forbids non-test
// use, mirroring the Go stdlib's convention for runtime/testing
// hooks.
func (e *Engine) ApplyAnnotationRelevanceForTest(ctx context.Context, ann *annotation.Annotation) error {
	return e.updateRelevanceFromAnnotation(ctx, ann)
}

// annotationRelevanceDelta calculates the relevance score adjustment for an annotation.
// Rating: centered at 2.5, multiplied by 0.06 → range [-0.09, +0.15]
// Interaction: +0.10
// Tag add: +0.02
// Note: +0.03
// Other types: 0
func annotationRelevanceDelta(ann *annotation.Annotation) float64 {
	switch ann.AnnotationType {
	case annotation.TypeRating:
		if ann.Rating == nil {
			return 0
		}
		// Center at 2.5: rating 5 → +0.15, rating 3 → +0.03, rating 1 → -0.09
		return (float64(*ann.Rating) - 2.5) * 0.06
	case annotation.TypeInteraction:
		return 0.10
	case annotation.TypeTagAdd:
		return 0.02
	case annotation.TypeNote:
		return 0.03
	default:
		return 0
	}
}

// ResurfacingCandidates returns artifacts older than thresholdDays with no annotations.
func (e *Engine) ResurfacingCandidates(ctx context.Context, thresholdDays int, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 10
	}

	rows, err := e.Pool.Query(ctx, `
		SELECT a.id FROM artifacts a
		LEFT JOIN artifact_annotation_summary aas ON aas.artifact_id = a.id
		WHERE a.created_at < NOW() - ($1 || ' days')::interval
		  AND aas.artifact_id IS NULL
		ORDER BY a.created_at ASC
		LIMIT $2
	`, fmt.Sprintf("%d", thresholdDays), limit)
	if err != nil {
		return nil, fmt.Errorf("resurfacing query: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// clampFloat64 clamps a value to the range [min, max].
func clampFloat64(v, min, max float64) float64 {
	return math.Max(min, math.Min(max, v))
}
