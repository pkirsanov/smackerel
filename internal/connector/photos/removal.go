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

// RemovalReason mirrors the photo_removal_reason enum. Values prefixed
// with the planning taxonomy from design.md (unprocessed_raw,
// burst_non_best, blurry, screenshot_transient, cross_provider_duplicate,
// user_marked) are accepted plus the legacy Scope-1 values (duplicate,
// low_quality, blurred, screenshot, other).
type RemovalReason string

const (
	RemovalUnprocessedRAW         RemovalReason = "unprocessed_raw"
	RemovalBurstNonBest           RemovalReason = "burst_non_best"
	RemovalBlurry                 RemovalReason = "blurry"
	RemovalScreenshotTransient    RemovalReason = "screenshot_transient"
	RemovalCrossProviderDuplicate RemovalReason = "cross_provider_duplicate"
	RemovalUserMarked             RemovalReason = "user_marked"

	// Legacy reasons retained from the Scope-1 enum.
	RemovalLegacyDuplicate  RemovalReason = "duplicate"
	RemovalLegacyLowQuality RemovalReason = "low_quality"
	RemovalLegacyBlurred    RemovalReason = "blurred"
	RemovalLegacyScreenshot RemovalReason = "screenshot"
	RemovalLegacyOther      RemovalReason = "other"
)

func (reason RemovalReason) Valid() bool {
	switch reason {
	case RemovalUnprocessedRAW, RemovalBurstNonBest, RemovalBlurry, RemovalScreenshotTransient,
		RemovalCrossProviderDuplicate, RemovalUserMarked,
		RemovalLegacyDuplicate, RemovalLegacyLowQuality, RemovalLegacyBlurred, RemovalLegacyScreenshot, RemovalLegacyOther:
		return true
	}
	return false
}

// SupportedRemovalReasons returns the planning taxonomy used by the
// validate suite to assert UI/API/agent strings stay in lockstep.
func SupportedRemovalReasons() []RemovalReason {
	return []RemovalReason{RemovalUnprocessedRAW, RemovalBurstNonBest, RemovalBlurry, RemovalScreenshotTransient, RemovalCrossProviderDuplicate, RemovalUserMarked}
}

// RemovalCandidate is the persisted row used by the removal review
// screen and the action-token planner.
type RemovalCandidate struct {
	ID            uuid.UUID
	PhotoID       uuid.UUID
	Reason        RemovalReason
	Confidence    float64
	Rationale     string
	Method        string
	ActionStatus  string
	ActionTokenID *uuid.UUID
	DecidedAt     *time.Time
	DecidedBy     string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// RemovalDecisionInput is the payload returned by photos.removal.result.
type RemovalDecisionInput struct {
	PhotoID    uuid.UUID
	Reason     RemovalReason
	Confidence float64
	Rationale  string
	Method     string
}

// RemovalAnalyzer turns ML / stable-signal removal evaluations into
// removal-candidate rows. The analyzer NEVER mutates the underlying
// provider — that path lives in the action-token confirmation layer.
type RemovalAnalyzer struct {
	store *Store
	now   func() time.Time
}

func NewRemovalAnalyzer(store *Store) *RemovalAnalyzer {
	return &RemovalAnalyzer{store: store, now: time.Now}
}

func (analyzer *RemovalAnalyzer) Apply(ctx context.Context, input RemovalDecisionInput) (*RemovalCandidate, error) {
	if analyzer == nil || analyzer.store == nil || analyzer.store.pool == nil {
		return nil, fmt.Errorf("photos: removal analyzer store is required")
	}
	if input.PhotoID == uuid.Nil {
		return nil, fmt.Errorf("photos: removal candidate requires photo_id")
	}
	if !input.Reason.Valid() {
		return nil, fmt.Errorf("photos: unsupported removal reason %q", input.Reason)
	}
	conf := input.Confidence
	if _, err := ValidateLLMDecision(LLMDecision{Kind: DecisionRemoval, Confidence: &conf, Rationale: input.Rationale}); err != nil {
		return nil, err
	}
	method := strings.TrimSpace(input.Method)
	if method == "" {
		method = "llm"
	}
	if method != "stable_signal" && method != "llm" {
		return nil, fmt.Errorf("photos: removal method must be stable_signal or llm")
	}
	id := uuid.New()
	now := analyzer.now().UTC()
	if _, err := analyzer.store.pool.Exec(ctx, `
		INSERT INTO photo_removal_candidates (
			id, photo_id, reason, confidence, rationale, method,
			action_status, created_at, updated_at
		) VALUES ($1, $2, $3::photo_removal_reason, $4, $5, $6, 'pending_review', $7, $7)
		ON CONFLICT (photo_id, reason) DO UPDATE SET
			confidence=EXCLUDED.confidence,
			rationale=EXCLUDED.rationale,
			method=EXCLUDED.method,
			action_status=CASE WHEN photo_removal_candidates.action_status IN ('archived', 'deleted') THEN photo_removal_candidates.action_status ELSE 'pending_review' END,
			updated_at=EXCLUDED.updated_at
	`, id, input.PhotoID, string(input.Reason), conf, strings.TrimSpace(input.Rationale), method, now); err != nil {
		return nil, fmt.Errorf("upsert removal candidate: %w", err)
	}
	if err := analyzer.store.WriteAuditEvent(ctx, AuditEvent{
		Action:    "removal_candidate",
		PhotoID:   &input.PhotoID,
		Outcome:   "pending_review",
		Reason:    string(input.Reason),
		Metadata:  map[string]any{"method": method, "confidence": conf},
		CreatedAt: now,
	}); err != nil {
		return nil, err
	}
	return analyzer.store.GetRemovalCandidateByPhotoReason(ctx, input.PhotoID, input.Reason)
}

// MarkRemovalDecision records the outcome of a confirmation flow on a
// removal candidate. Called by the action-token confirm path AFTER the
// provider mutation succeeds. Decision values mirror the photo_removal
// status check constraint.
func (store *Store) MarkRemovalDecision(ctx context.Context, candidateID uuid.UUID, decision string, actor string, tokenID uuid.UUID) (*RemovalCandidate, error) {
	if store == nil || store.pool == nil {
		return nil, fmt.Errorf("photos: store pool is nil")
	}
	switch decision {
	case "kept", "archived", "deleted", "exempted":
	default:
		return nil, fmt.Errorf("photos: unsupported removal decision %q", decision)
	}
	now := time.Now().UTC()
	if _, err := store.pool.Exec(ctx, `
		UPDATE photo_removal_candidates
		   SET action_status=$2,
		       action_token_id=$3,
		       decided_at=$4,
		       decided_by=$5,
		       updated_at=$4
		 WHERE id=$1`,
		candidateID, decision, tokenID, now, strings.TrimSpace(actor)); err != nil {
		return nil, fmt.Errorf("mark removal decision: %w", err)
	}
	return store.GetRemovalCandidate(ctx, candidateID)
}

// GetRemovalCandidate fetches a single candidate row.
func (store *Store) GetRemovalCandidate(ctx context.Context, id uuid.UUID) (*RemovalCandidate, error) {
	if store == nil || store.pool == nil {
		return nil, fmt.Errorf("photos: store pool is nil")
	}
	row := store.pool.QueryRow(ctx, `
		SELECT id, photo_id, reason::text, confidence, rationale, method,
		       action_status, action_token_id, decided_at, decided_by,
		       created_at, updated_at
		  FROM photo_removal_candidates
		 WHERE id=$1`, id)
	return scanRemovalCandidate(row)
}

// GetRemovalCandidateByPhotoReason returns the most recent candidate for
// a (photo_id, reason) pair. The (photo_id, reason) tuple is unique in
// the schema so this is one row at most.
func (store *Store) GetRemovalCandidateByPhotoReason(ctx context.Context, photoID uuid.UUID, reason RemovalReason) (*RemovalCandidate, error) {
	row := store.pool.QueryRow(ctx, `
		SELECT id, photo_id, reason::text, confidence, rationale, method,
		       action_status, action_token_id, decided_at, decided_by,
		       created_at, updated_at
		  FROM photo_removal_candidates
		 WHERE photo_id=$1 AND reason=$2::photo_removal_reason`, photoID, string(reason))
	return scanRemovalCandidate(row)
}

// ListRemovalCandidates returns every removal candidate filtered by
// status (empty = all). Used by the photo-health removal screen and the
// action-planner DTO.
func (store *Store) ListRemovalCandidates(ctx context.Context, status string, limit int) ([]RemovalCandidate, error) {
	if store == nil || store.pool == nil {
		return nil, fmt.Errorf("photos: store pool is nil")
	}
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	query := `
		SELECT id, photo_id, reason::text, confidence, rationale, method,
		       action_status, action_token_id, decided_at, decided_by,
		       created_at, updated_at
		  FROM photo_removal_candidates`
	args := []any{}
	if strings.TrimSpace(status) != "" {
		query += ` WHERE action_status=$1`
		args = append(args, status)
	}
	query += ` ORDER BY created_at DESC LIMIT ` + fmt.Sprintf("%d", limit)
	rows, err := store.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query removal candidates: %w", err)
	}
	defer rows.Close()
	var candidates []RemovalCandidate
	for rows.Next() {
		c, err := scanRemovalCandidateRow(rows)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, *c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate removal candidates: %w", err)
	}
	return candidates, nil
}

// ListRemovalCandidatesByIDs fetches the removal candidates referenced by
// an action token's scope.RemovalIDs slice. Used by the confirm path to
// resolve the actual photos to mutate without re-querying.
func (store *Store) ListRemovalCandidatesByIDs(ctx context.Context, ids []uuid.UUID) ([]RemovalCandidate, error) {
	if store == nil || store.pool == nil {
		return nil, fmt.Errorf("photos: store pool is nil")
	}
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := store.pool.Query(ctx, `
		SELECT id, photo_id, reason::text, confidence, rationale, method,
		       action_status, action_token_id, decided_at, decided_by,
		       created_at, updated_at
		  FROM photo_removal_candidates
		 WHERE id = ANY($1)`, ids)
	if err != nil {
		return nil, fmt.Errorf("query removal candidates by ids: %w", err)
	}
	defer rows.Close()
	var candidates []RemovalCandidate
	for rows.Next() {
		c, err := scanRemovalCandidateRow(rows)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, *c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate removal candidates by ids: %w", err)
	}
	return candidates, nil
}

func scanRemovalCandidate(row pgx.Row) (*RemovalCandidate, error) {
	c, err := scanRemovalCandidateScanner(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRemovalCandidateNotFound
		}
		return nil, err
	}
	return c, nil
}

func scanRemovalCandidateRow(rows pgx.Rows) (*RemovalCandidate, error) {
	return scanRemovalCandidateScanner(rows)
}

// scanRemovalCandidateScanner accepts both pgx.Row and pgx.Rows so the
// candidate field list lives in exactly one place.
func scanRemovalCandidateScanner(scanner interface{ Scan(...any) error }) (*RemovalCandidate, error) {
	var c RemovalCandidate
	var reason string
	var actionTokenID *uuid.UUID
	var decidedBy *string
	if err := scanner.Scan(&c.ID, &c.PhotoID, &reason, &c.Confidence, &c.Rationale, &c.Method,
		&c.ActionStatus, &actionTokenID, &c.DecidedAt, &decidedBy, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return nil, fmt.Errorf("scan removal candidate: %w", err)
	}
	c.Reason = RemovalReason(reason)
	c.ActionTokenID = actionTokenID
	if decidedBy != nil {
		c.DecidedBy = *decidedBy
	}
	return &c, nil
}

// ErrRemovalCandidateNotFound is returned when GetRemovalCandidate fails
// to locate the requested row.
var ErrRemovalCandidateNotFound = errors.New("photos: removal candidate not found")
