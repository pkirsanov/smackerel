package photos

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// RouteTarget enumerates the cross-feature destinations a photo
// classification can route to (FR-007). Each value MUST appear in the
// `photo_routing_decisions_target_chk` constraint defined in migration
// 031; the integration canary tests assert the two stay in sync.
type RouteTarget string

const (
	RouteTargetExpense      RouteTarget = "expense"
	RouteTargetRecipe       RouteTarget = "recipe"
	RouteTargetDocument     RouteTarget = "document"
	RouteTargetKnowledge    RouteTarget = "knowledge"
	RouteTargetAnnotation   RouteTarget = "annotation"
	RouteTargetList         RouteTarget = "list"
	RouteTargetMealplan     RouteTarget = "mealplan"
	RouteTargetIntelligence RouteTarget = "intelligence"
)

func (target RouteTarget) Valid() bool {
	switch target {
	case RouteTargetExpense, RouteTargetRecipe, RouteTargetDocument,
		RouteTargetKnowledge, RouteTargetAnnotation, RouteTargetList,
		RouteTargetMealplan, RouteTargetIntelligence:
		return true
	}
	return false
}

// AllRouteTargets returns the canonical, alphabetised slice used by the
// capability/taxonomy canary tests so a new target cannot land without
// updating every coordinated surface.
func AllRouteTargets() []RouteTarget {
	return []RouteTarget{
		RouteTargetAnnotation,
		RouteTargetDocument,
		RouteTargetExpense,
		RouteTargetIntelligence,
		RouteTargetKnowledge,
		RouteTargetList,
		RouteTargetMealplan,
		RouteTargetRecipe,
	}
}

// RoutingPlan is the in-memory result of EvaluateRouting. Callers
// persist accepted plans through Store.RoutePhoto, which writes the
// authoritative `photo_routing_decisions` row and an audit event so the
// validate phase can replay every cross-feature route taken from a
// photo.
type RoutingPlan struct {
	Target              RouteTarget `json:"target"`
	Reason              string      `json:"reason"`
	Confidence          float64     `json:"confidence"`
	SensitivityBlocked  bool        `json:"sensitivity_blocked"`
	BlockingSensitivity string      `json:"blocking_sensitivity,omitempty"`
}

// EvaluateRouting derives the cross-feature targets implied by a
// classification result. The decision MUST satisfy three guards before a
// target is emitted:
//
//  1. the LLM-owned ClassificationDecision is itself valid (caption,
//     primary_category, confidence > 0, rationale present),
//  2. the confidence meets or exceeds the configured routing threshold
//     (`photos.policy.routing_confidence_threshold`),
//  3. the photo is not gated for sensitivity (`hidden` blocks every
//     target; `sensitive` blocks identity/medical/financial/intimate
//     routes that would surface preview bytes downstream).
//
// EvaluateRouting is pure: it never persists anything. Callers wrap the
// plan in a confirmation flow (Telegram ack, mobile UI banner, etc.)
// before invoking Store.RoutePhoto.
func EvaluateRouting(decision ClassificationDecision, sensitivity SensitivityLevel, sensitivityLabels []string, threshold float64) ([]RoutingPlan, error) {
	if _, err := decision.Validate(); err != nil {
		return nil, err
	}
	if threshold <= 0 || threshold > 1 {
		return nil, fmt.Errorf("photos: routing threshold must be in (0,1]; got %v", threshold)
	}
	if decision.Confidence < threshold {
		return nil, nil
	}
	category := strings.ToLower(strings.TrimSpace(decision.PrimaryCategory))
	docType := strings.ToLower(strings.TrimSpace(decision.DocumentType))
	if category == "" {
		return nil, nil
	}
	candidates := candidateTargetsFor(category, docType)
	if len(candidates) == 0 {
		return nil, nil
	}
	labelSet := make(map[string]struct{}, len(sensitivityLabels))
	for _, label := range sensitivityLabels {
		labelSet[strings.ToLower(strings.TrimSpace(label))] = struct{}{}
	}
	var plans []RoutingPlan
	for _, target := range candidates {
		blocked, reason := isSensitivityBlocked(sensitivity, labelSet, target)
		plans = append(plans, RoutingPlan{
			Target:              target,
			Reason:              fmt.Sprintf("classification=%s confidence=%.2f", category, decision.Confidence),
			Confidence:          decision.Confidence,
			SensitivityBlocked:  blocked,
			BlockingSensitivity: reason,
		})
	}
	return plans, nil
}

func candidateTargetsFor(category, docType string) []RouteTarget {
	switch {
	case category == "receipt" || category == "invoice" || docType == "receipt":
		return []RouteTarget{RouteTargetExpense, RouteTargetDocument, RouteTargetKnowledge}
	case category == "recipe_card" || category == "menu" || docType == "recipe":
		return []RouteTarget{RouteTargetRecipe, RouteTargetMealplan}
	case category == "food_dish":
		return []RouteTarget{RouteTargetRecipe, RouteTargetIntelligence}
	case category == "legal_document" || category == "identity_document" || docType == "legal":
		return []RouteTarget{RouteTargetDocument, RouteTargetKnowledge}
	case category == "product_screenshot" || category == "product":
		return []RouteTarget{RouteTargetList, RouteTargetAnnotation}
	case category == "place" || category == "place_context":
		return []RouteTarget{RouteTargetKnowledge, RouteTargetIntelligence}
	case strings.HasPrefix(category, "document/") || category == "document" || category == "whiteboard":
		return []RouteTarget{RouteTargetDocument, RouteTargetKnowledge}
	}
	return nil
}

func isSensitivityBlocked(level SensitivityLevel, labels map[string]struct{}, target RouteTarget) (bool, string) {
	if level == SensitivityHidden {
		return true, "hidden"
	}
	if level != SensitivitySensitive {
		return false, ""
	}
	// Sensitive (not hidden) photos block downstream routes that would
	// expose preview bytes outside the secured retrieval flow.
	switch target {
	case RouteTargetExpense, RouteTargetIntelligence, RouteTargetList,
		RouteTargetAnnotation, RouteTargetMealplan, RouteTargetRecipe:
		return true, "sensitive"
	}
	if _, ok := labels["identity_document"]; ok {
		return true, "identity_document"
	}
	if _, ok := labels["medical"]; ok {
		return true, "medical"
	}
	if _, ok := labels["financial"]; ok && target != RouteTargetDocument {
		return true, "financial"
	}
	if _, ok := labels["children"]; ok {
		return true, "children"
	}
	if _, ok := labels["intimate"]; ok {
		return true, "intimate"
	}
	return false, ""
}

// RoutingRecord mirrors a row in `photo_routing_decisions`.
type RoutingRecord struct {
	ID                   uuid.UUID
	PhotoID              uuid.UUID
	Target               RouteTarget
	DownstreamArtifactID string
	Confidence           float64
	Rationale            string
	SensitivityBlocked   bool
	Actor                string
	CreatedAt            time.Time
}

// RoutePhotoInput captures everything needed to persist a routing
// decision. Callers MUST supply a non-empty actor so audit replays can
// attribute the decision; integration tests pass `system` for
// classifier-driven routes.
type RoutePhotoInput struct {
	PhotoID              uuid.UUID
	Target               RouteTarget
	DownstreamArtifactID string
	Confidence           float64
	Rationale            string
	SensitivityBlocked   bool
	Actor                string
}

// RoutePhoto records an accepted routing decision. The database
// `(photo_id, target)` UNIQUE constraint enforces that re-running the
// classifier for the same target updates the existing row instead of
// duplicating it; the returned RoutingRecord reflects the persisted
// state (including the original CreatedAt for re-routes).
func (store *Store) RoutePhoto(ctx context.Context, input RoutePhotoInput) (*RoutingRecord, error) {
	if store == nil || store.pool == nil {
		return nil, fmt.Errorf("photos: store pool is nil")
	}
	if !input.Target.Valid() {
		return nil, fmt.Errorf("photos: invalid route target %q", input.Target)
	}
	actor := strings.TrimSpace(input.Actor)
	if actor == "" {
		return nil, fmt.Errorf("photos: route_photo requires a non-empty actor")
	}
	id := uuid.New()
	now := time.Now().UTC()
	var record RoutingRecord
	row := store.pool.QueryRow(ctx, `
		INSERT INTO photo_routing_decisions (
			id, photo_id, target, downstream_artifact_id,
			confidence, rationale, sensitivity_blocked, actor, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (photo_id, target) DO UPDATE SET
			downstream_artifact_id = EXCLUDED.downstream_artifact_id,
			confidence             = EXCLUDED.confidence,
			rationale              = EXCLUDED.rationale,
			sensitivity_blocked    = EXCLUDED.sensitivity_blocked,
			actor                  = EXCLUDED.actor
		RETURNING id, photo_id, target, downstream_artifact_id,
		          confidence, rationale, sensitivity_blocked, actor, created_at
	`, id, input.PhotoID, string(input.Target), input.DownstreamArtifactID,
		input.Confidence, input.Rationale, input.SensitivityBlocked, actor, now)
	var target string
	if err := row.Scan(&record.ID, &record.PhotoID, &target, &record.DownstreamArtifactID,
		&record.Confidence, &record.Rationale, &record.SensitivityBlocked, &record.Actor, &record.CreatedAt); err != nil {
		return nil, fmt.Errorf("persist photo routing decision: %w", err)
	}
	record.Target = RouteTarget(target)
	return &record, nil
}

// ListRouteDecisions returns every persisted route decision for a photo,
// ordered newest first. Used by the validate phase and the photo detail
// API to expose the routing history.
func (store *Store) ListRouteDecisions(ctx context.Context, photoID uuid.UUID) ([]RoutingRecord, error) {
	if store == nil || store.pool == nil {
		return nil, fmt.Errorf("photos: store pool is nil")
	}
	rows, err := store.pool.Query(ctx, `
		SELECT id, photo_id, target, downstream_artifact_id,
		       confidence, rationale, sensitivity_blocked, actor, created_at
		  FROM photo_routing_decisions
		 WHERE photo_id=$1
		 ORDER BY created_at DESC`, photoID)
	if err != nil {
		return nil, fmt.Errorf("list route decisions: %w", err)
	}
	defer rows.Close()
	var records []RoutingRecord
	for rows.Next() {
		var record RoutingRecord
		var target string
		if err := rows.Scan(&record.ID, &record.PhotoID, &target, &record.DownstreamArtifactID,
			&record.Confidence, &record.Rationale, &record.SensitivityBlocked, &record.Actor, &record.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan route decision: %w", err)
		}
		record.Target = RouteTarget(target)
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate route decisions: %w", err)
	}
	return records, nil
}

// ListPhotosBySource returns every photo persisted under the specified
// upload-channel reference. Used by integration tests to prove that
// uploads from Telegram, mobile, and the web all enter the same store
// via the unified pipeline (SCN-040-010).
func (store *Store) ListPhotosBySource(ctx context.Context, channel SourceChannel, sourceRef string) ([]PhotoRecord, error) {
	if store == nil || store.pool == nil {
		return nil, fmt.Errorf("photos: store pool is nil")
	}
	rows, err := store.pool.Query(ctx, `SELECT `+photoRecordColumnsSQL+`
		  FROM photos p
		 WHERE p.source_channel=$1 AND p.source_ref=$2
		 ORDER BY p.captured_at DESC NULLS LAST, p.updated_at DESC`,
		string(channel), sourceRef)
	if err != nil {
		return nil, fmt.Errorf("list photos by source: %w", err)
	}
	defer rows.Close()
	var records []PhotoRecord
	for rows.Next() {
		rec, err := scanPhotoRecordRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scan photo by source: %w", err)
		}
		records = append(records, *rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate photos by source: %w", err)
	}
	return records, nil
}

// ListPhotosByDocumentGroup returns every page belonging to the given
// document group, ordered by page index then captured_at. Used by the
// mobile document scan integration test (SCN-040-011) to prove a
// multi-page upload becomes one cohesive document artifact.
func (store *Store) ListPhotosByDocumentGroup(ctx context.Context, groupID uuid.UUID) ([]PhotoRecord, error) {
	if store == nil || store.pool == nil {
		return nil, fmt.Errorf("photos: store pool is nil")
	}
	rows, err := store.pool.Query(ctx, `SELECT `+photoRecordColumnsSQL+`
		  FROM photos p
		 WHERE p.document_group_id=$1
		 ORDER BY COALESCE(p.document_page_index, 0), p.captured_at`, groupID)
	if err != nil {
		return nil, fmt.Errorf("list document group: %w", err)
	}
	defer rows.Close()
	var records []PhotoRecord
	for rows.Next() {
		rec, err := scanPhotoRecordRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scan document group photo: %w", err)
		}
		records = append(records, *rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate document group: %w", err)
	}
	return records, nil
}

// upsertDocumentGroupTx ensures a row exists for the supplied group ref
// and returns its uuid. Page count is incremented every call so the
// mobile document scan integration test can assert page totals match
// the upload count.
func (store *Store) upsertDocumentGroupTx(ctx context.Context, tx pgx.Tx, groupRef string) (uuid.UUID, error) {
	groupRef = strings.TrimSpace(groupRef)
	if groupRef == "" {
		return uuid.Nil, fmt.Errorf("photos: document group ref is required")
	}
	id := uuid.New()
	var stored uuid.UUID
	err := tx.QueryRow(ctx, `
		INSERT INTO photo_document_groups (id, group_ref, page_count, updated_at)
		VALUES ($1, $2, 1, now())
		ON CONFLICT (group_ref) DO UPDATE SET
			page_count = photo_document_groups.page_count + 1,
			updated_at = now()
		RETURNING id`, id, groupRef).Scan(&stored)
	if err != nil {
		return uuid.Nil, fmt.Errorf("upsert document group: %w", err)
	}
	return stored, nil
}

// DocumentGroup is the read-only view returned by GetDocumentGroup.
type DocumentGroup struct {
	ID        uuid.UUID
	GroupRef  string
	PageCount int
	CreatedAt time.Time
	UpdatedAt time.Time
}

// GetDocumentGroup returns the stored document group metadata.
func (store *Store) GetDocumentGroup(ctx context.Context, id uuid.UUID) (*DocumentGroup, error) {
	if store == nil || store.pool == nil {
		return nil, fmt.Errorf("photos: store pool is nil")
	}
	var group DocumentGroup
	if err := store.pool.QueryRow(ctx, `
		SELECT id, group_ref, page_count, created_at, updated_at
		  FROM photo_document_groups
		 WHERE id=$1`, id).Scan(&group.ID, &group.GroupRef, &group.PageCount, &group.CreatedAt, &group.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrDocumentGroupNotFound
		}
		return nil, fmt.Errorf("load document group: %w", err)
	}
	return &group, nil
}

// ErrDocumentGroupNotFound is returned when GetDocumentGroup cannot
// resolve the supplied id.
var ErrDocumentGroupNotFound = errors.New("photos: document group not found")
