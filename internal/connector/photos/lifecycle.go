package photos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// LifecycleLink is the persisted RAW-to-derived edge surfaced by the
// lifecycle review screen and the photo detail siblings panel.
type LifecycleLink struct {
	ID             uuid.UUID
	RawPhotoID     uuid.UUID
	DerivedPhotoID uuid.UUID
	Editor         EditorSignature
	EditorVersion  string
	Confidence     float64
	Rationale      string
	Method         string
	ReviewState    string
	DecidedAt      *time.Time
	DecidedBy      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// LifecycleDecisionInput is the shape returned by the ML
// photos.lifecycle.result handler. Both confidence and rationale MUST be
// present (see ml/app/photos.py); the analyzer rejects any input that
// fails this contract before it touches the database.
type LifecycleDecisionInput struct {
	RawPhotoID     uuid.UUID
	DerivedPhotoID uuid.UUID
	Editor         EditorSignature
	EditorVersion  string
	Confidence     float64
	Rationale      string
	Method         string
}

// LifecycleAnalyzer turns ML / stable-signal lifecycle decisions into
// rows in photo_raw_export_links. Low-confidence decisions are persisted
// in review_required state so the UI can keep them out of automated
// removal flows.
type LifecycleAnalyzer struct {
	store     *Store
	threshold float64
	now       func() time.Time
}

func NewLifecycleAnalyzer(store *Store, lifecycleConfirmationThreshold float64) *LifecycleAnalyzer {
	if lifecycleConfirmationThreshold <= 0 {
		lifecycleConfirmationThreshold = 0.75
	}
	return &LifecycleAnalyzer{store: store, threshold: lifecycleConfirmationThreshold, now: time.Now}
}

func (analyzer *LifecycleAnalyzer) Apply(ctx context.Context, input LifecycleDecisionInput) (*LifecycleLink, error) {
	if analyzer == nil || analyzer.store == nil || analyzer.store.pool == nil {
		return nil, fmt.Errorf("photos: lifecycle analyzer store is required")
	}
	if input.RawPhotoID == uuid.Nil || input.DerivedPhotoID == uuid.Nil {
		return nil, fmt.Errorf("photos: lifecycle analyzer requires raw_photo_id and derived_photo_id")
	}
	if input.RawPhotoID == input.DerivedPhotoID {
		return nil, fmt.Errorf("photos: lifecycle link must connect distinct photos")
	}
	confidence := input.Confidence
	conf := &confidence
	if _, err := ValidateLLMDecision(LLMDecision{Kind: DecisionLifecycle, Confidence: conf, Rationale: input.Rationale}); err != nil {
		return nil, err
	}
	method := strings.TrimSpace(input.Method)
	if method == "" {
		method = "llm"
	}
	if method != "stable_signal" && method != "llm" {
		return nil, fmt.Errorf("photos: lifecycle method must be stable_signal or llm")
	}
	editor := input.Editor
	if strings.TrimSpace(string(editor)) == "" {
		editor = EditorUnknown
	}
	reviewState := "confirmed"
	if confidence < analyzer.threshold {
		reviewState = "review_required"
	}
	id := uuid.New()
	now := analyzer.now().UTC()
	if _, err := analyzer.store.pool.Exec(ctx, `
		INSERT INTO photo_raw_export_links (
			id, raw_photo_id, derived_photo_id, editor, editor_version,
			confidence, rationale, method, review_state, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $10)
		ON CONFLICT (raw_photo_id, derived_photo_id) DO UPDATE SET
			editor=EXCLUDED.editor,
			editor_version=EXCLUDED.editor_version,
			confidence=EXCLUDED.confidence,
			rationale=EXCLUDED.rationale,
			method=EXCLUDED.method,
			review_state=EXCLUDED.review_state,
			updated_at=EXCLUDED.updated_at
	`, id, input.RawPhotoID, input.DerivedPhotoID, string(editor), strings.TrimSpace(input.EditorVersion),
		confidence, strings.TrimSpace(input.Rationale), method, reviewState, now); err != nil {
		return nil, fmt.Errorf("persist lifecycle link: %w", err)
	}
	if err := analyzer.store.WriteAuditEvent(ctx, AuditEvent{
		Action:    "lifecycle_link",
		PhotoID:   &input.DerivedPhotoID,
		Outcome:   reviewState,
		Reason:    string(editor),
		Metadata:  map[string]any{"raw_photo_id": input.RawPhotoID.String(), "method": method, "confidence": confidence},
		CreatedAt: now,
	}); err != nil {
		return nil, err
	}
	link, err := analyzer.store.GetLifecycleLink(ctx, input.RawPhotoID, input.DerivedPhotoID)
	if err != nil {
		return nil, err
	}
	return link, nil
}

// GetLifecycleLink fetches a single RAW-to-derived edge for inspection
// (lifecycle review screen, photo detail siblings).
func (store *Store) GetLifecycleLink(ctx context.Context, rawID uuid.UUID, derivedID uuid.UUID) (*LifecycleLink, error) {
	if store == nil || store.pool == nil {
		return nil, fmt.Errorf("photos: store pool is nil")
	}
	row := store.pool.QueryRow(ctx, `
		SELECT id, raw_photo_id, derived_photo_id, editor, editor_version,
		       confidence, rationale, method, review_state, decided_at, decided_by,
		       created_at, updated_at
		  FROM photo_raw_export_links
		 WHERE raw_photo_id=$1 AND derived_photo_id=$2`, rawID, derivedID)
	var link LifecycleLink
	var editor string
	var decidedBy *string
	if err := row.Scan(&link.ID, &link.RawPhotoID, &link.DerivedPhotoID, &editor, &link.EditorVersion,
		&link.Confidence, &link.Rationale, &link.Method, &link.ReviewState, &link.DecidedAt, &decidedBy,
		&link.CreatedAt, &link.UpdatedAt); err != nil {
		return nil, fmt.Errorf("scan lifecycle link: %w", err)
	}
	link.Editor = EditorSignature(editor)
	if decidedBy != nil {
		link.DecidedBy = *decidedBy
	}
	return &link, nil
}

// ListLifecycleLinksByDerived returns every link (RAW→derived) where the
// supplied derived photo is on the right-hand side. Used by photo detail
// to show "originals" and by the lifecycle review screen.
func (store *Store) ListLifecycleLinksByDerived(ctx context.Context, derivedID uuid.UUID) ([]LifecycleLink, error) {
	if store == nil || store.pool == nil {
		return nil, fmt.Errorf("photos: store pool is nil")
	}
	rows, err := store.pool.Query(ctx, `
		SELECT id, raw_photo_id, derived_photo_id, editor, editor_version,
		       confidence, rationale, method, review_state, decided_at, decided_by,
		       created_at, updated_at
		  FROM photo_raw_export_links
		 WHERE derived_photo_id=$1
		 ORDER BY created_at DESC`, derivedID)
	if err != nil {
		return nil, fmt.Errorf("query lifecycle links: %w", err)
	}
	defer rows.Close()
	return scanLifecycleLinks(rows)
}

// LifecycleSummary feeds the Photo Health Lifecycle dashboard.
type LifecycleSummary struct {
	Total              int                            `json:"total"`
	ByEditor           map[EditorSignature]int        `json:"by_editor"`
	ReviewQueue        []LifecycleLink                `json:"review_queue"`
	StatusCounts       map[string]int                 `json:"status_counts"`
	ConfirmationFloor  float64                        `json:"confirmation_threshold"`
	GeneratedAt        time.Time                      `json:"generated_at"`
	ByEditorWithCounts []LifecycleEditorSummary       `json:"by_editor_breakdown"`
	ReviewByMethod     map[string]int                 `json:"review_by_method"`
	GroupedByEditorVer map[string]LifecycleEditorList `json:"grouped_by_editor_version"`
}

type LifecycleEditorSummary struct {
	Editor EditorSignature `json:"editor"`
	Count  int             `json:"count"`
}

type LifecycleEditorList struct {
	Editor EditorSignature `json:"editor"`
	IDs    []uuid.UUID     `json:"ids"`
}

// SummarizeLifecycle returns the dashboard shape used by
// /v1/photos/health/lifecycle. The query is scope-wide today; multi-user
// scoping is owned by Scope 4.
func (store *Store) SummarizeLifecycle(ctx context.Context, threshold float64, now time.Time) (*LifecycleSummary, error) {
	if store == nil || store.pool == nil {
		return nil, fmt.Errorf("photos: store pool is nil")
	}
	rows, err := store.pool.Query(ctx, `
		SELECT id, raw_photo_id, derived_photo_id, editor, editor_version,
		       confidence, rationale, method, review_state, decided_at, decided_by,
		       created_at, updated_at
		  FROM photo_raw_export_links
		 ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("query lifecycle summary: %w", err)
	}
	defer rows.Close()
	links, err := scanLifecycleLinks(rows)
	if err != nil {
		return nil, err
	}
	summary := &LifecycleSummary{
		Total:              len(links),
		ByEditor:           map[EditorSignature]int{},
		StatusCounts:       map[string]int{},
		ReviewByMethod:     map[string]int{},
		GroupedByEditorVer: map[string]LifecycleEditorList{},
		ConfirmationFloor:  threshold,
		GeneratedAt:        now.UTC(),
	}
	for _, link := range links {
		summary.ByEditor[link.Editor]++
		summary.StatusCounts[link.ReviewState]++
		if link.ReviewState == "review_required" {
			summary.ReviewQueue = append(summary.ReviewQueue, link)
			summary.ReviewByMethod[link.Method]++
		}
		key := string(link.Editor)
		if link.EditorVersion != "" {
			key = key + "@" + link.EditorVersion
		}
		entry := summary.GroupedByEditorVer[key]
		entry.Editor = link.Editor
		entry.IDs = append(entry.IDs, link.ID)
		summary.GroupedByEditorVer[key] = entry
	}
	for editor, count := range summary.ByEditor {
		summary.ByEditorWithCounts = append(summary.ByEditorWithCounts, LifecycleEditorSummary{Editor: editor, Count: count})
	}
	return summary, nil
}

func scanLifecycleLinks(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}) ([]LifecycleLink, error) {
	var links []LifecycleLink
	for rows.Next() {
		var link LifecycleLink
		var editor string
		var decidedBy *string
		if err := rows.Scan(&link.ID, &link.RawPhotoID, &link.DerivedPhotoID, &editor, &link.EditorVersion,
			&link.Confidence, &link.Rationale, &link.Method, &link.ReviewState, &link.DecidedAt, &decidedBy,
			&link.CreatedAt, &link.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan lifecycle link row: %w", err)
		}
		link.Editor = EditorSignature(editor)
		if decidedBy != nil {
			link.DecidedBy = *decidedBy
		}
		links = append(links, link)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate lifecycle link rows: %w", err)
	}
	return links, nil
}

// AuditEvent is the row shape persisted into photo_audit_events from
// every lifecycle / dedupe / removal / action-token boundary so the
// validate phase can replay decisions.
type AuditEvent struct {
	Action    string
	PhotoID   *uuid.UUID
	Connector string
	Provider  string
	Outcome   string
	Reason    string
	Metadata  map[string]any
	Actor     string
	CreatedAt time.Time
}

// WriteAuditEvent persists a row into photo_audit_events. Failures are
// returned to the caller so we surface storage outages instead of
// silently dropping audit history.
func (store *Store) WriteAuditEvent(ctx context.Context, event AuditEvent) error {
	if store == nil || store.pool == nil {
		return fmt.Errorf("photos: store pool is nil")
	}
	if strings.TrimSpace(event.Action) == "" {
		return fmt.Errorf("photos: audit event missing action")
	}
	actor := strings.TrimSpace(event.Actor)
	if actor == "" {
		actor = "system"
	}
	created := event.CreatedAt
	if created.IsZero() {
		created = time.Now().UTC()
	}
	metadata := event.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}
	if event.Reason != "" {
		metadata["reason_code"] = event.Reason
	}
	if event.Provider != "" {
		metadata["provider"] = event.Provider
	}
	if event.Connector != "" {
		metadata["connector_id"] = event.Connector
	}
	if event.Outcome != "" {
		metadata["outcome"] = event.Outcome
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("encode audit metadata: %w", err)
	}
	if _, err := store.pool.Exec(ctx, `
		INSERT INTO photo_audit_events (
			id, photo_id, connector_id, event_type, actor, payload, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, uuid.New(), event.PhotoID, nullableString(event.Connector), event.Action, actor, encoded, created); err != nil {
		return fmt.Errorf("persist photo audit event: %w", err)
	}
	return nil
}

// ListAuditEvents returns recent audit rows scoped by action prefix. Used
// by the validate suite and the photo health screen to prove that no
// provider mutation happened before confirmation.
func (store *Store) ListAuditEvents(ctx context.Context, actionPrefix string, limit int) ([]AuditEvent, error) {
	if store == nil || store.pool == nil {
		return nil, fmt.Errorf("photos: store pool is nil")
	}
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	rows, err := store.pool.Query(ctx, `
		SELECT event_type, photo_id, connector_id, actor, payload, created_at
		  FROM photo_audit_events
		 WHERE event_type LIKE $1
		 ORDER BY created_at DESC
		 LIMIT $2`, actionPrefix+"%", limit)
	if err != nil {
		return nil, fmt.Errorf("query audit events: %w", err)
	}
	defer rows.Close()
	var events []AuditEvent
	for rows.Next() {
		var event AuditEvent
		var payload []byte
		var connector *string
		if err := rows.Scan(&event.Action, &event.PhotoID, &connector, &event.Actor, &payload, &event.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan audit event: %w", err)
		}
		if connector != nil {
			event.Connector = *connector
		}
		_ = json.Unmarshal(payload, &event.Metadata)
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate audit events: %w", err)
	}
	return events, nil
}

// ErrLifecycleLinkNotFound is returned when GetLifecycleLink cannot find
// the requested edge. Tests rely on the sentinel to assert the absence
// of links before confirmation.
var ErrLifecycleLinkNotFound = errors.New("photos: lifecycle link not found")

func nullableString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}
