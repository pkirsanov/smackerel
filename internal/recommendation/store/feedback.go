package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// FeedbackInput records one explicit user feedback action.
type FeedbackInput struct {
	RecommendationID string
	ActorUserID      string
	FeedbackType     string
	SourceWatchID    string
	PreferenceKey    string
	CorrectionKind   string
	Payload          map[string]any
}

// FeedbackResult is returned by feedback API and web handlers.
type FeedbackResult struct {
	FeedbackID        string            `json:"feedback_id"`
	TraceID           string            `json:"trace_id"`
	SuppressionEffect SuppressionEffect `json:"suppression_effect"`
	PreferenceEffect  PreferenceEffect  `json:"preference_effect"`
	Acknowledgement   string            `json:"acknowledgement"`
}

// SuppressionEffect describes how a feedback action changed suppression state.
type SuppressionEffect struct {
	Applied       bool   `json:"applied"`
	SuppressionID string `json:"suppression_id,omitempty"`
	Reason        string `json:"reason,omitempty"`
	Scope         string `json:"scope,omitempty"`
}

// PreferenceEffect describes how a feedback action changed preference state.
type PreferenceEffect struct {
	Applied        bool   `json:"applied"`
	CorrectionID   string `json:"correction_id,omitempty"`
	PreferenceKey  string `json:"preference_key,omitempty"`
	CorrectionKind string `json:"correction_kind,omitempty"`
	Active         bool   `json:"active,omitempty"`
}

// PreferenceCorrectionRecord is an active or historical correction row.
type PreferenceCorrectionRecord struct {
	ID             string         `json:"id"`
	ActorUserID    string         `json:"actor_user_id"`
	PreferenceKey  string         `json:"preference_key"`
	CorrectionKind string         `json:"correction_kind"`
	Payload        map[string]any `json:"payload"`
	CreatedAt      time.Time      `json:"created_at"`
	RevokedAt      *time.Time     `json:"revoked_at,omitempty"`
}

// PreferencesView is the API/web read model for preference corrections.
type PreferencesView struct {
	ActorUserID       string                       `json:"actor_user_id"`
	ActiveCorrections []PreferenceCorrectionRecord `json:"active_corrections"`
	History           []PreferenceCorrectionRecord `json:"history"`
}

// CreatePreferenceCorrectionInput records a correction not tied to a specific recommendation.
type CreatePreferenceCorrectionInput struct {
	ActorUserID      string
	PreferenceKey    string
	CorrectionKind   string
	Payload          map[string]any
	SourceFeedbackID string
}

// SuppressionLookupInput asks for active suppression decisions for canonical candidates.
type SuppressionLookupInput struct {
	ActorUserID   string
	Category      string
	CanonicalKeys []string
	SourceWatchID string
}

// SuppressionDecision is the policy-facing active suppression decision.
type SuppressionDecision struct {
	SuppressionID   string
	CandidateID     string
	CanonicalKey    string
	SuppressionKind string
	SourceWatchID   string
	Reason          string
}

// RecordFeedback persists feedback plus any suppression/preference effect.
func (s *Store) RecordFeedback(ctx context.Context, input FeedbackInput) (FeedbackResult, error) {
	if s == nil || s.pool == nil {
		return FeedbackResult{}, fmt.Errorf("recommendation store: postgres pool is required")
	}
	if strings.TrimSpace(input.RecommendationID) == "" {
		return FeedbackResult{}, fmt.Errorf("recommendation_id is required")
	}
	if !validFeedbackType(input.FeedbackType) {
		return FeedbackResult{}, fmt.Errorf("invalid feedback_type %q", input.FeedbackType)
	}
	now := time.Now().UTC()

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return FeedbackResult{}, fmt.Errorf("begin feedback tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var recommendationActor, candidateID, recommendationWatchID string
	err = tx.QueryRow(ctx, `
SELECT actor_user_id, candidate_id, COALESCE(watch_id, '')
FROM recommendations
WHERE id = $1`, input.RecommendationID).Scan(&recommendationActor, &candidateID, &recommendationWatchID)
	if err != nil {
		return FeedbackResult{}, fmt.Errorf("load feedback recommendation %s: %w", input.RecommendationID, err)
	}
	actorUserID := strings.TrimSpace(input.ActorUserID)
	if actorUserID == "" {
		actorUserID = recommendationActor
	}
	if actorUserID != recommendationActor {
		return FeedbackResult{}, fmt.Errorf("feedback actor does not own recommendation")
	}

	feedbackID, err := newTextID("rec_feedback")
	if err != nil {
		return FeedbackResult{}, err
	}
	payload := copyPayload(input.Payload)
	if input.SourceWatchID != "" {
		payload["source_watch_id"] = input.SourceWatchID
	}
	if input.PreferenceKey != "" {
		payload["preference_key"] = input.PreferenceKey
	}
	feedbackPayload, err := marshalAny(payload)
	if err != nil {
		return FeedbackResult{}, fmt.Errorf("marshal feedback payload: %w", err)
	}
	_, err = tx.Exec(ctx, `
INSERT INTO recommendation_feedback (
    id, recommendation_id, candidate_id, actor_user_id, feedback_type,
    feedback_payload, graph_artifact_id, created_at
) VALUES ($1, $2, $3, $4, $5, $6, NULL, $7)`,
		feedbackID,
		input.RecommendationID,
		candidateID,
		actorUserID,
		input.FeedbackType,
		feedbackPayload,
		now,
	)
	if err != nil {
		return FeedbackResult{}, fmt.Errorf("insert recommendation feedback: %w", err)
	}

	result := FeedbackResult{FeedbackID: feedbackID, Acknowledgement: "Feedback recorded"}
	if effect, err := s.applySuppressionEffect(ctx, tx, applySuppressionInput{
		ActorUserID:   actorUserID,
		CandidateID:   candidateID,
		FeedbackType:  input.FeedbackType,
		SourceWatchID: chooseText(input.SourceWatchID, recommendationWatchID),
		CreatedAt:     now,
	}); err != nil {
		return FeedbackResult{}, err
	} else {
		result.SuppressionEffect = effect
	}
	if input.FeedbackType == "wrong_preference" {
		correction, err := s.insertPreferenceCorrection(ctx, tx, CreatePreferenceCorrectionInput{
			ActorUserID:      actorUserID,
			PreferenceKey:    input.PreferenceKey,
			CorrectionKind:   chooseText(input.CorrectionKind, "remove"),
			Payload:          payload,
			SourceFeedbackID: feedbackID,
		}, now)
		if err != nil {
			return FeedbackResult{}, err
		}
		result.PreferenceEffect = PreferenceEffect{Applied: true, CorrectionID: correction.ID, PreferenceKey: correction.PreferenceKey, CorrectionKind: correction.CorrectionKind, Active: true}
	}

	toolCall := ToolCallRecord{
		Name:            "recommendation_record_feedback",
		SideEffectClass: "write",
		Arguments: map[string]any{
			"recommendation_id": input.RecommendationID,
			"feedback_type":     input.FeedbackType,
		},
		Result: map[string]any{
			"feedback_id":        result.FeedbackID,
			"suppression_effect": result.SuppressionEffect,
			"preference_effect":  result.PreferenceEffect,
		},
		LatencyMillis: 1,
		StartedAt:     now,
	}
	traceID, err := insertScenarioTrace(ctx, tx, scenarioTraceInput{
		ScenarioID:      FeedbackScenarioID,
		ScenarioVersion: FeedbackScenarioVersion,
		Source:          "api",
		Outcome:         "recorded",
		InputEnvelope: map[string]any{
			"recommendation_id": input.RecommendationID,
			"feedback_type":     input.FeedbackType,
		},
		FinalOutput: map[string]any{"feedback_id": result.FeedbackID, "acknowledgement": result.Acknowledgement},
		OutcomeDetail: map[string]any{
			"suppression_applied": result.SuppressionEffect.Applied,
			"preference_applied":  result.PreferenceEffect.Applied,
		},
		ToolCalls: []ToolCallRecord{toolCall},
		StartedAt: now,
		EndedAt:   now,
	})
	if err != nil {
		return FeedbackResult{}, err
	}
	result.TraceID = traceID
	if err := tx.Commit(ctx); err != nil {
		return FeedbackResult{}, fmt.Errorf("commit feedback tx: %w", err)
	}
	return result, nil
}

func (s *Store) CreatePreferenceCorrection(ctx context.Context, input CreatePreferenceCorrectionInput) (PreferenceCorrectionRecord, error) {
	if s == nil || s.pool == nil {
		return PreferenceCorrectionRecord{}, fmt.Errorf("recommendation store: postgres pool is required")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return PreferenceCorrectionRecord{}, fmt.Errorf("begin preference correction tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	correction, err := s.insertPreferenceCorrection(ctx, tx, input, time.Now().UTC())
	if err != nil {
		return PreferenceCorrectionRecord{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PreferenceCorrectionRecord{}, fmt.Errorf("commit preference correction tx: %w", err)
	}
	return correction, nil
}

func (s *Store) RevokePreferenceCorrection(ctx context.Context, actorUserID, preferenceKey, correctionID string) error {
	if strings.TrimSpace(actorUserID) == "" || strings.TrimSpace(preferenceKey) == "" || strings.TrimSpace(correctionID) == "" {
		return fmt.Errorf("actor_user_id, preference_key, and correction_id are required")
	}
	commandTag, err := s.pool.Exec(ctx, `
UPDATE recommendation_preference_corrections
SET revoked_at = $1
WHERE id = $2 AND actor_user_id = $3 AND preference_key = $4 AND revoked_at IS NULL`, time.Now().UTC(), correctionID, actorUserID, preferenceKey)
	if err != nil {
		return fmt.Errorf("revoke preference correction: %w", err)
	}
	if commandTag.RowsAffected() != 1 {
		return fmt.Errorf("preference correction %s not found", correctionID)
	}
	return nil
}

func (s *Store) ListPreferences(ctx context.Context, actorUserID string) (PreferencesView, error) {
	if strings.TrimSpace(actorUserID) == "" {
		return PreferencesView{}, fmt.Errorf("actor_user_id is required")
	}
	rows, err := s.pool.Query(ctx, `
SELECT id, actor_user_id, preference_key, correction_kind, correction_payload, created_at, revoked_at
FROM recommendation_preference_corrections
WHERE actor_user_id = $1
ORDER BY created_at DESC`, actorUserID)
	if err != nil {
		return PreferencesView{}, fmt.Errorf("list preference corrections: %w", err)
	}
	defer rows.Close()
	view := PreferencesView{ActorUserID: actorUserID}
	for rows.Next() {
		correction, err := scanPreferenceCorrection(rows)
		if err != nil {
			return PreferencesView{}, err
		}
		view.History = append(view.History, correction)
		if correction.RevokedAt == nil {
			view.ActiveCorrections = append(view.ActiveCorrections, correction)
		}
	}
	if err := rows.Err(); err != nil {
		return PreferencesView{}, fmt.Errorf("iterate preference corrections: %w", err)
	}
	return view, nil
}

func (s *Store) ActivePreferenceCorrections(ctx context.Context, actorUserID string) ([]PreferenceCorrectionRecord, error) {
	view, err := s.ListPreferences(ctx, actorUserID)
	if err != nil {
		return nil, err
	}
	return view.ActiveCorrections, nil
}

func (s *Store) ActiveSuppressionDecisions(ctx context.Context, input SuppressionLookupInput) ([]SuppressionDecision, error) {
	if strings.TrimSpace(input.ActorUserID) == "" {
		return nil, fmt.Errorf("actor_user_id is required")
	}
	if strings.TrimSpace(input.Category) == "" {
		return nil, fmt.Errorf("category is required")
	}
	if len(input.CanonicalKeys) == 0 {
		return nil, nil
	}
	rows, err := s.pool.Query(ctx, `
SELECT s.id, s.candidate_id, c.canonical_key, s.suppression_kind, COALESCE(s.source_watch_id, '')
FROM recommendation_suppression_state s
JOIN recommendation_candidates c ON c.id = s.candidate_id
WHERE s.actor_user_id = $1
  AND c.category = $2
  AND c.canonical_key = ANY($3::text[])
  AND (s.expires_at IS NULL OR s.expires_at > $4)
  AND (
    s.suppression_kind = 'disliked'
    OR s.suppression_kind = 'snoozed'
    OR (s.suppression_kind = 'not_interested' AND (($5 <> '' AND s.source_watch_id = $5) OR ($5 = '' AND s.source_watch_id IS NULL)))
  )
ORDER BY s.created_at DESC`, input.ActorUserID, input.Category, input.CanonicalKeys, time.Now().UTC(), input.SourceWatchID)
	if err != nil {
		return nil, fmt.Errorf("load active suppressions: %w", err)
	}
	defer rows.Close()
	var decisions []SuppressionDecision
	for rows.Next() {
		var decision SuppressionDecision
		if err := rows.Scan(&decision.SuppressionID, &decision.CandidateID, &decision.CanonicalKey, &decision.SuppressionKind, &decision.SourceWatchID); err != nil {
			return nil, fmt.Errorf("scan active suppression: %w", err)
		}
		decision.Reason = suppressionReason(decision.SuppressionKind)
		decisions = append(decisions, decision)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active suppressions: %w", err)
	}
	return decisions, nil
}

type applySuppressionInput struct {
	ActorUserID   string
	CandidateID   string
	FeedbackType  string
	SourceWatchID string
	CreatedAt     time.Time
}

func (s *Store) applySuppressionEffect(ctx context.Context, tx pgx.Tx, input applySuppressionInput) (SuppressionEffect, error) {
	var kind, scope, reason string
	sourceWatchID := input.SourceWatchID
	switch input.FeedbackType {
	case "tried_disliked":
		kind = "disliked"
		scope = "all_surfaces"
		reason = "suppressed:user-disliked"
		sourceWatchID = ""
	case "not_interested":
		kind = "not_interested"
		reason = "suppressed:user-not-interested"
		if sourceWatchID != "" {
			scope = "watch"
		} else {
			scope = "request"
		}
	case "snooze":
		kind = "snoozed"
		scope = "all_surfaces"
		reason = "suppressed:user-snoozed"
	default:
		return SuppressionEffect{}, nil
	}
	suppressionID, err := newTextID("rec_suppress")
	if err != nil {
		return SuppressionEffect{}, err
	}
	appliesToScope, err := marshalAny(map[string]any{"scope": scope, "source_watch_id": sourceWatchID})
	if err != nil {
		return SuppressionEffect{}, fmt.Errorf("marshal suppression scope: %w", err)
	}
	var watchRef any
	if sourceWatchID != "" {
		watchRef = sourceWatchID
	}
	_, err = tx.Exec(ctx, `
INSERT INTO recommendation_suppression_state (
    id, actor_user_id, candidate_id, source_watch_id, suppression_kind,
    applies_to_scope, expires_at, created_at
) VALUES ($1, $2, $3, $4, $5, $6, NULL, $7)
ON CONFLICT (actor_user_id, candidate_id, source_watch_id, suppression_kind)
DO UPDATE SET applies_to_scope = EXCLUDED.applies_to_scope, expires_at = NULL, created_at = EXCLUDED.created_at`,
		suppressionID,
		input.ActorUserID,
		input.CandidateID,
		watchRef,
		kind,
		appliesToScope,
		input.CreatedAt,
	)
	if err != nil {
		return SuppressionEffect{}, fmt.Errorf("insert suppression state: %w", err)
	}
	return SuppressionEffect{Applied: true, SuppressionID: suppressionID, Reason: reason, Scope: scope}, nil
}

func (s *Store) insertPreferenceCorrection(ctx context.Context, tx pgx.Tx, input CreatePreferenceCorrectionInput, now time.Time) (PreferenceCorrectionRecord, error) {
	if strings.TrimSpace(input.ActorUserID) == "" {
		return PreferenceCorrectionRecord{}, fmt.Errorf("actor_user_id is required")
	}
	if strings.TrimSpace(input.PreferenceKey) == "" {
		return PreferenceCorrectionRecord{}, fmt.Errorf("preference_key is required")
	}
	correctionKind := chooseText(input.CorrectionKind, "remove")
	if !validCorrectionKind(correctionKind) {
		return PreferenceCorrectionRecord{}, fmt.Errorf("invalid correction_kind %q", correctionKind)
	}
	correctionID, err := newTextID("rec_pref")
	if err != nil {
		return PreferenceCorrectionRecord{}, err
	}
	payload := copyPayload(input.Payload)
	correctionPayload, err := marshalAny(payload)
	if err != nil {
		return PreferenceCorrectionRecord{}, fmt.Errorf("marshal preference correction payload: %w", err)
	}
	var sourceFeedbackID any
	if input.SourceFeedbackID != "" {
		sourceFeedbackID = input.SourceFeedbackID
	}
	_, err = tx.Exec(ctx, `
INSERT INTO recommendation_preference_corrections (
    id, actor_user_id, preference_key, correction_kind, correction_payload,
    source_feedback_id, created_at, revoked_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, NULL)`,
		correctionID,
		input.ActorUserID,
		input.PreferenceKey,
		correctionKind,
		correctionPayload,
		sourceFeedbackID,
		now,
	)
	if err != nil {
		return PreferenceCorrectionRecord{}, fmt.Errorf("insert preference correction: %w", err)
	}
	return PreferenceCorrectionRecord{ID: correctionID, ActorUserID: input.ActorUserID, PreferenceKey: input.PreferenceKey, CorrectionKind: correctionKind, Payload: payload, CreatedAt: now}, nil
}

func scanPreferenceCorrection(scanner interface{ Scan(dest ...any) error }) (PreferenceCorrectionRecord, error) {
	var correction PreferenceCorrectionRecord
	var payloadJSON []byte
	if err := scanner.Scan(&correction.ID, &correction.ActorUserID, &correction.PreferenceKey, &correction.CorrectionKind, &payloadJSON, &correction.CreatedAt, &correction.RevokedAt); err != nil {
		return PreferenceCorrectionRecord{}, fmt.Errorf("scan preference correction: %w", err)
	}
	if err := json.Unmarshal(payloadJSON, &correction.Payload); err != nil {
		return PreferenceCorrectionRecord{}, fmt.Errorf("decode preference correction %s: %w", correction.ID, err)
	}
	return correction, nil
}

func validFeedbackType(value string) bool {
	switch value {
	case "tried_liked", "tried_disliked", "not_interested", "snooze", "override_suppression", "wrong_preference", "wrong_category", "more_like_this":
		return true
	}
	return false
}

func validCorrectionKind(value string) bool {
	switch value {
	case "remove", "invert", "set_weight", "block_category", "allow_category":
		return true
	}
	return false
}

func suppressionReason(kind string) string {
	switch kind {
	case "disliked":
		return "suppressed:user-disliked"
	case "not_interested":
		return "suppressed:user-not-interested"
	case "snoozed":
		return "suppressed:user-snoozed"
	default:
		return "suppressed:" + kind
	}
}

func copyPayload(values map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range values {
		out[key] = value
	}
	return out
}

func chooseText(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
