package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store owns PostgreSQL writes for recommendation foundation behavior.
type Store struct {
	pool *pgxpool.Pool
}

// New returns a recommendation store backed by pool.
func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// CreateRequestInput captures the no-provider vertical slice persisted by
// Scope 1. Later scopes extend persistence through additional methods.
type CreateRequestInput struct {
	ActorUserID                string
	Source                     string
	ScenarioID                 string
	RawInput                   string
	ParsedRequest              map[string]any
	LocationPrecisionRequested string
	LocationPrecisionSent      string
	Status                     string
}

// RequestRecord is the persisted request and trace identity returned to API
// callers.
type RequestRecord struct {
	ID      string
	TraceID string
	Status  string
}

// ToolCallRecord is the persisted tool-call snapshot stored in agent trace
// tables for recommendation scenarios.
type ToolCallRecord struct {
	Name            string         `json:"name"`
	SideEffectClass string         `json:"side_effect_class"`
	Arguments       map[string]any `json:"arguments"`
	Result          map[string]any `json:"result,omitempty"`
	RejectionReason string         `json:"rejection_reason,omitempty"`
	Error           string         `json:"error,omitempty"`
	LatencyMillis   int            `json:"latency_ms"`
	StartedAt       time.Time      `json:"started_at"`
}

// ProviderFactInput contains a normalized provider fact ready for persistence.
type ProviderFactInput struct {
	LocalID             string
	ProviderID          string
	ProviderCandidateID string
	Category            string
	Title               string
	NormalizedFact      map[string]any
	RetrievedAt         time.Time
	SourceUpdatedAt     *time.Time
	Attribution         map[string]any
	SponsoredState      string
	RestrictedFlags     map[string]any
}

// CandidateInput contains one canonical candidate and its provider-fact refs.
type CandidateInput struct {
	LocalID              string
	Category             string
	CanonicalKey         string
	Title                string
	CanonicalURL         string
	CanonicalFact        map[string]any
	DedupeReason         map[string]any
	ProviderFactLocalIDs []string
	MergeReason          string
}

// RecommendationInput contains a delivered or withheld recommendation row.
type RecommendationInput struct {
	CandidateLocalID string
	RankPosition     int
	Status           string
	StatusReason     string
	ScoreBreakdown   map[string]float64
	Rationale        []string
	GraphSignalRefs  []string
	PolicyDecisions  []map[string]any
	QualityDecisions []map[string]any
	DeliveryChannel  string
}

// ReactiveOutcomeInput is the durable result of one reactive recommendation run.
type ReactiveOutcomeInput struct {
	ActorUserID                string
	Source                     string
	ScenarioID                 string
	ScenarioVersion            string
	ScenarioHash               string
	RawInput                   string
	ParsedRequest              map[string]any
	LocationPrecisionRequested string
	LocationPrecisionSent      string
	Status                     string
	ToolCalls                  []ToolCallRecord
	ProviderFacts              []ProviderFactInput
	Candidates                 []CandidateInput
	Recommendations            []RecommendationInput
	Clarification              *Clarification
	StartedAt                  time.Time
	CompletedAt                time.Time
}

// ProviderBadge is the response-facing provider attribution summary.
type ProviderBadge struct {
	ProviderID string `json:"provider_id"`
	Label      string `json:"label"`
	URL        string `json:"url,omitempty"`
}

// RenderedRecommendation is the API/web read model for a delivered item.
type RenderedRecommendation struct {
	ID               string             `json:"id"`
	CandidateID      string             `json:"candidate_id"`
	Title            string             `json:"title"`
	Rank             int                `json:"rank"`
	ProviderBadges   []ProviderBadge    `json:"provider_badges"`
	Attribution      []ProviderBadge    `json:"attribution"`
	Rationale        []string           `json:"rationale"`
	GraphSignalRefs  []string           `json:"graph_signal_refs"`
	NoPersonalSignal bool               `json:"no_personal_signal"`
	SourceConflict   bool               `json:"source_conflict"`
	ScoreBreakdown   map[string]float64 `json:"score_breakdown"`
	PolicyDecisions  []map[string]any   `json:"policy_decisions"`
	QualityDecisions []map[string]any   `json:"quality_decisions"`
}

// Clarification is returned when the request is too ambiguous for provider calls.
type Clarification struct {
	Question string   `json:"question"`
	Choices  []string `json:"choices"`
}

// RenderedRequest is the response-facing recommendation request state.
type RenderedRequest struct {
	ID              string                   `json:"request_id"`
	TraceID         string                   `json:"trace_id"`
	Status          string                   `json:"status"`
	Recommendations []RenderedRecommendation `json:"recommendations"`
	Clarification   *Clarification           `json:"clarification,omitempty"`
	ToolCalls       []ToolCallRecord         `json:"-"`
}

// ToolCallNames returns the trace tool names in persisted order.
func (r RenderedRequest) ToolCallNames() []string {
	names := make([]string, 0, len(r.ToolCalls))
	for _, call := range r.ToolCalls {
		names = append(names, call.Name)
	}
	return names
}

// CreateNoProviderRequest writes one agent trace and one recommendation request
// in a single transaction when no providers are registered.
func (s *Store) CreateNoProviderRequest(ctx context.Context, input CreateRequestInput) (RequestRecord, error) {
	if s == nil || s.pool == nil {
		return RequestRecord{}, fmt.Errorf("recommendation store: postgres pool is required")
	}
	requestID, err := newTextID("rec_req")
	if err != nil {
		return RequestRecord{}, err
	}
	traceID, err := newTextID("rec_trace")
	if err != nil {
		return RequestRecord{}, err
	}
	now := time.Now().UTC()

	parsedJSON, err := marshalObject(input.ParsedRequest)
	if err != nil {
		return RequestRecord{}, fmt.Errorf("marshal parsed request: %w", err)
	}
	scenarioSnapshot, err := marshalObject(map[string]any{
		"id":    input.ScenarioID,
		"scope": "scope-01-foundation-schema",
	})
	if err != nil {
		return RequestRecord{}, fmt.Errorf("marshal scenario snapshot: %w", err)
	}
	inputEnvelope, err := marshalObject(map[string]any{
		"query":  input.RawInput,
		"source": input.Source,
	})
	if err != nil {
		return RequestRecord{}, fmt.Errorf("marshal input envelope: %w", err)
	}
	routing, err := marshalObject(map[string]any{
		"scenario_id": input.ScenarioID,
		"status":      input.Status,
	})
	if err != nil {
		return RequestRecord{}, fmt.Errorf("marshal routing: %w", err)
	}
	finalOutput, err := marshalObject(map[string]any{
		"request_id": requestID,
		"status":     input.Status,
	})
	if err != nil {
		return RequestRecord{}, fmt.Errorf("marshal final output: %w", err)
	}
	outcomeDetail, err := marshalObject(map[string]any{
		"reason": "no recommendation providers configured",
	})
	if err != nil {
		return RequestRecord{}, fmt.Errorf("marshal outcome detail: %w", err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return RequestRecord{}, fmt.Errorf("begin recommendation request tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx, `
INSERT INTO agent_traces (
    trace_id, scenario_id, scenario_version, scenario_hash, scenario_snapshot,
    source, input_envelope, routing, tool_calls, turn_log,
    final_output, outcome, outcome_detail,
    provider, model, tokens_prompt, tokens_completion,
    latency_ms, started_at, ended_at, created_at
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, $9, $10,
    $11, $12, $13,
    $14, $15, $16, $17,
    $18, $19, $20, $21
)`,
		traceID,
		input.ScenarioID,
		"scope-01",
		"scope-01-foundation-schema",
		scenarioSnapshot,
		input.Source,
		inputEnvelope,
		routing,
		[]byte("[]"),
		[]byte("[]"),
		finalOutput,
		input.Status,
		outcomeDetail,
		"",
		"",
		0,
		0,
		0,
		now,
		now,
		now,
	)
	if err != nil {
		return RequestRecord{}, fmt.Errorf("insert recommendation trace: %w", err)
	}

	_, err = tx.Exec(ctx, `
INSERT INTO recommendation_requests (
    id, actor_user_id, source, scenario_id, trace_id, raw_input,
    parsed_request, location_precision_requested, location_precision_sent,
    status, created_at, completed_at
) VALUES (
    $1, $2, $3, $4, $5, $6,
    $7, $8, $9,
    $10, $11, $12
)`,
		requestID,
		input.ActorUserID,
		input.Source,
		input.ScenarioID,
		traceID,
		input.RawInput,
		parsedJSON,
		input.LocationPrecisionRequested,
		input.LocationPrecisionSent,
		input.Status,
		now,
		now,
	)
	if err != nil {
		return RequestRecord{}, fmt.Errorf("insert recommendation request: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return RequestRecord{}, fmt.Errorf("commit recommendation request tx: %w", err)
	}

	return RequestRecord{ID: requestID, TraceID: traceID, Status: input.Status}, nil
}

// CreateReactiveRequest persists a complete reactive recommendation outcome.
func (s *Store) CreateReactiveRequest(ctx context.Context, input ReactiveOutcomeInput) (RenderedRequest, error) {
	if s == nil || s.pool == nil {
		return RenderedRequest{}, fmt.Errorf("recommendation store: postgres pool is required")
	}
	requestID, err := newTextID("rec_req")
	if err != nil {
		return RenderedRequest{}, err
	}
	traceID, err := newTextID("rec_trace")
	if err != nil {
		return RenderedRequest{}, err
	}
	now := input.CompletedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	startedAt := input.StartedAt
	if startedAt.IsZero() {
		startedAt = now
	}

	parsedJSON, err := marshalObject(input.ParsedRequest)
	if err != nil {
		return RenderedRequest{}, fmt.Errorf("marshal parsed request: %w", err)
	}
	scenarioSnapshot, err := marshalObject(map[string]any{
		"id":      input.ScenarioID,
		"version": input.ScenarioVersion,
		"scope":   "scope-02-reactive-place-recommendation",
	})
	if err != nil {
		return RenderedRequest{}, fmt.Errorf("marshal scenario snapshot: %w", err)
	}
	inputEnvelope, err := marshalObject(map[string]any{
		"query":  input.RawInput,
		"source": input.Source,
	})
	if err != nil {
		return RenderedRequest{}, fmt.Errorf("marshal input envelope: %w", err)
	}
	routing, err := marshalObject(map[string]any{
		"scenario_id": input.ScenarioID,
		"status":      input.Status,
	})
	if err != nil {
		return RenderedRequest{}, fmt.Errorf("marshal routing: %w", err)
	}
	toolCallsJSON, err := json.Marshal(input.ToolCalls)
	if err != nil {
		return RenderedRequest{}, fmt.Errorf("marshal tool calls: %w", err)
	}
	finalOutput, err := marshalAny(map[string]any{
		"request_id":        requestID,
		"status":            input.Status,
		"recommendations":   len(input.Recommendations),
		"clarification_set": input.Clarification != nil,
	})
	if err != nil {
		return RenderedRequest{}, fmt.Errorf("marshal final output: %w", err)
	}
	outcomeDetail, err := marshalAny(map[string]any{
		"provider_fact_count": len(input.ProviderFacts),
		"candidate_count":     len(input.Candidates),
	})
	if err != nil {
		return RenderedRequest{}, fmt.Errorf("marshal outcome detail: %w", err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return RenderedRequest{}, fmt.Errorf("begin recommendation reactive tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx, `
INSERT INTO agent_traces (
    trace_id, scenario_id, scenario_version, scenario_hash, scenario_snapshot,
    source, input_envelope, routing, tool_calls, turn_log,
    final_output, outcome, outcome_detail,
    provider, model, tokens_prompt, tokens_completion,
    latency_ms, started_at, ended_at, created_at
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, $9, $10,
    $11, $12, $13,
    $14, $15, $16, $17,
    $18, $19, $20, $21
)`,
		traceID,
		input.ScenarioID,
		input.ScenarioVersion,
		input.ScenarioHash,
		scenarioSnapshot,
		input.Source,
		inputEnvelope,
		routing,
		toolCallsJSON,
		[]byte("[]"),
		finalOutput,
		input.Status,
		outcomeDetail,
		"",
		"",
		0,
		0,
		int(now.Sub(startedAt).Milliseconds()),
		startedAt,
		now,
		now,
	)
	if err != nil {
		return RenderedRequest{}, fmt.Errorf("insert recommendation trace: %w", err)
	}

	for seq, call := range input.ToolCalls {
		arguments, err := marshalAny(call.Arguments)
		if err != nil {
			return RenderedRequest{}, fmt.Errorf("marshal tool arguments %s: %w", call.Name, err)
		}
		result, err := marshalAny(call.Result)
		if err != nil {
			return RenderedRequest{}, fmt.Errorf("marshal tool result %s: %w", call.Name, err)
		}
		started := call.StartedAt
		if started.IsZero() {
			started = startedAt
		}
		_, err = tx.Exec(ctx, `
INSERT INTO agent_tool_calls (
    trace_id, seq, tool_name, side_effect_class, arguments, result,
    rejection_reason, error, latency_ms, started_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			traceID,
			seq+1,
			call.Name,
			call.SideEffectClass,
			arguments,
			result,
			call.RejectionReason,
			call.Error,
			call.LatencyMillis,
			started,
		)
		if err != nil {
			return RenderedRequest{}, fmt.Errorf("insert recommendation tool call %s: %w", call.Name, err)
		}
	}

	_, err = tx.Exec(ctx, `
INSERT INTO recommendation_requests (
    id, actor_user_id, source, scenario_id, trace_id, raw_input,
    parsed_request, location_precision_requested, location_precision_sent,
    status, created_at, completed_at
) VALUES (
    $1, $2, $3, $4, $5, $6,
    $7, $8, $9,
    $10, $11, $12
)`,
		requestID,
		input.ActorUserID,
		input.Source,
		input.ScenarioID,
		traceID,
		input.RawInput,
		parsedJSON,
		input.LocationPrecisionRequested,
		input.LocationPrecisionSent,
		input.Status,
		now,
		now,
	)
	if err != nil {
		return RenderedRequest{}, fmt.Errorf("insert recommendation request: %w", err)
	}

	factIDs := make(map[string]string, len(input.ProviderFacts))
	for _, fact := range input.ProviderFacts {
		factID, err := newTextID("rec_fact")
		if err != nil {
			return RenderedRequest{}, err
		}
		normalized, err := marshalAny(fact.NormalizedFact)
		if err != nil {
			return RenderedRequest{}, fmt.Errorf("marshal provider fact %s: %w", fact.LocalID, err)
		}
		attribution, err := marshalAny(fact.Attribution)
		if err != nil {
			return RenderedRequest{}, fmt.Errorf("marshal attribution %s: %w", fact.LocalID, err)
		}
		restricted, err := marshalAny(fact.RestrictedFlags)
		if err != nil {
			return RenderedRequest{}, fmt.Errorf("marshal restricted flags %s: %w", fact.LocalID, err)
		}
		retrievedAt := fact.RetrievedAt
		if retrievedAt.IsZero() {
			retrievedAt = now
		}
		sponsoredState := fact.SponsoredState
		if sponsoredState == "" {
			sponsoredState = "none"
		}
		var persistedID string
		err = tx.QueryRow(ctx, `
INSERT INTO recommendation_provider_facts (
    id, request_id, watch_run_id, provider_id, provider_candidate_id,
    category, normalized_fact, source_retrieved_at, source_updated_at,
    source_payload_hash, raw_payload_expires_at, attribution,
    sponsored_state, restricted_flags, created_at
) VALUES (
    $1, $2, NULL, $3, $4,
    $5, $6, $7, $8,
    $9, $10, $11,
    $12, $13, $14
)
ON CONFLICT (provider_id, provider_candidate_id, source_retrieved_at)
DO UPDATE SET
    request_id = EXCLUDED.request_id,
    normalized_fact = EXCLUDED.normalized_fact,
    attribution = EXCLUDED.attribution,
    restricted_flags = EXCLUDED.restricted_flags
RETURNING id`,
			factID,
			requestID,
			fact.ProviderID,
			fact.ProviderCandidateID,
			fact.Category,
			normalized,
			retrievedAt,
			fact.SourceUpdatedAt,
			payloadHash(normalized),
			now.Add(24*time.Hour),
			attribution,
			sponsoredState,
			restricted,
			now,
		).Scan(&persistedID)
		if err != nil {
			return RenderedRequest{}, fmt.Errorf("insert provider fact %s: %w", fact.LocalID, err)
		}
		factIDs[fact.LocalID] = persistedID
	}

	candidateIDs := make(map[string]string, len(input.Candidates))
	for _, candidate := range input.Candidates {
		candidateID, err := newTextID("rec_cand")
		if err != nil {
			return RenderedRequest{}, err
		}
		canonicalFact, err := marshalAny(candidate.CanonicalFact)
		if err != nil {
			return RenderedRequest{}, fmt.Errorf("marshal candidate fact %s: %w", candidate.LocalID, err)
		}
		dedupeReason, err := marshalAny(candidate.DedupeReason)
		if err != nil {
			return RenderedRequest{}, fmt.Errorf("marshal candidate dedupe %s: %w", candidate.LocalID, err)
		}
		var persistedID string
		err = tx.QueryRow(ctx, `
INSERT INTO recommendation_candidates (
    id, category, canonical_key, title, canonical_url,
    canonical_fact, dedupe_reason, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (category, canonical_key)
DO UPDATE SET
    title = EXCLUDED.title,
    canonical_url = EXCLUDED.canonical_url,
    canonical_fact = EXCLUDED.canonical_fact,
    dedupe_reason = EXCLUDED.dedupe_reason,
    updated_at = EXCLUDED.updated_at
RETURNING id`,
			candidateID,
			candidate.Category,
			candidate.CanonicalKey,
			candidate.Title,
			nullableText(candidate.CanonicalURL),
			canonicalFact,
			dedupeReason,
			now,
			now,
		).Scan(&persistedID)
		if err != nil {
			return RenderedRequest{}, fmt.Errorf("insert candidate %s: %w", candidate.LocalID, err)
		}
		candidateIDs[candidate.LocalID] = persistedID

		mergeReason := candidate.MergeReason
		if mergeReason == "" {
			mergeReason = "provider-normalized"
		}
		for _, localFactID := range candidate.ProviderFactLocalIDs {
			providerFactID, ok := factIDs[localFactID]
			if !ok {
				return RenderedRequest{}, fmt.Errorf("candidate %s references unknown provider fact %s", candidate.LocalID, localFactID)
			}
			_, err = tx.Exec(ctx, `
INSERT INTO recommendation_candidate_provider_facts (candidate_id, provider_fact_id, merge_reason)
VALUES ($1, $2, $3)
ON CONFLICT (candidate_id, provider_fact_id) DO UPDATE SET merge_reason = EXCLUDED.merge_reason`,
				persistedID,
				providerFactID,
				mergeReason,
			)
			if err != nil {
				return RenderedRequest{}, fmt.Errorf("link candidate %s provider fact %s: %w", candidate.LocalID, localFactID, err)
			}
		}
	}

	renderedRecommendations := make([]RenderedRecommendation, 0, len(input.Recommendations))
	for _, rec := range input.Recommendations {
		candidateID, ok := candidateIDs[rec.CandidateLocalID]
		if !ok {
			return RenderedRequest{}, fmt.Errorf("recommendation references unknown candidate %s", rec.CandidateLocalID)
		}
		recommendationID, err := newTextID("rec")
		if err != nil {
			return RenderedRequest{}, err
		}
		scoreBreakdown, err := marshalAny(rec.ScoreBreakdown)
		if err != nil {
			return RenderedRequest{}, fmt.Errorf("marshal score breakdown: %w", err)
		}
		rationale, err := marshalAny(rec.Rationale)
		if err != nil {
			return RenderedRequest{}, fmt.Errorf("marshal rationale: %w", err)
		}
		graphRefs, err := marshalAny(rec.GraphSignalRefs)
		if err != nil {
			return RenderedRequest{}, fmt.Errorf("marshal graph refs: %w", err)
		}
		policyDecisions, err := marshalAny(rec.PolicyDecisions)
		if err != nil {
			return RenderedRequest{}, fmt.Errorf("marshal policy decisions: %w", err)
		}
		qualityDecisions, err := marshalAny(rec.QualityDecisions)
		if err != nil {
			return RenderedRequest{}, fmt.Errorf("marshal quality decisions: %w", err)
		}
		deliveryChannel := nullableText(rec.DeliveryChannel)
		var rankPosition any
		if rec.RankPosition > 0 {
			rankPosition = rec.RankPosition
		}
		_, err = tx.Exec(ctx, `
INSERT INTO recommendations (
    id, actor_user_id, request_id, watch_id, watch_run_id,
    candidate_id, artifact_id, trace_id, rank_position, status,
    status_reason, score_breakdown, rationale, graph_signal_refs,
    policy_decisions, quality_decisions, delivery_channel, delivered_at, created_at
) VALUES (
    $1, $2, $3, NULL, NULL,
    $4, NULL, $5, $6, $7,
    $8, $9, $10, $11,
    $12, $13, $14, $15, $16
)`,
			recommendationID,
			input.ActorUserID,
			requestID,
			candidateID,
			traceID,
			rankPosition,
			rec.Status,
			rec.StatusReason,
			scoreBreakdown,
			rationale,
			graphRefs,
			policyDecisions,
			qualityDecisions,
			deliveryChannel,
			now,
			now,
		)
		if err != nil {
			return RenderedRequest{}, fmt.Errorf("insert recommendation for candidate %s: %w", rec.CandidateLocalID, err)
		}
		if rec.Status == "delivered" && rec.DeliveryChannel != "" {
			deliveryID, err := newTextID("rec_delivery")
			if err != nil {
				return RenderedRequest{}, err
			}
			_, err = tx.Exec(ctx, `
INSERT INTO recommendation_delivery_attempts (
    id, recommendation_id, channel, destination_ref, outcome, error_kind, attempted_at
) VALUES ($1, $2, $3, $4, 'sent', NULL, $5)`,
				deliveryID,
				recommendationID,
				rec.DeliveryChannel,
				input.ActorUserID,
				now,
			)
			if err != nil {
				return RenderedRequest{}, fmt.Errorf("insert delivery attempt: %w", err)
			}
		}
		renderedRecommendations = append(renderedRecommendations, RenderedRecommendation{
			ID:              recommendationID,
			CandidateID:     candidateID,
			Rank:            rec.RankPosition,
			Rationale:       append([]string(nil), rec.Rationale...),
			GraphSignalRefs: append([]string(nil), rec.GraphSignalRefs...),
			ScoreBreakdown:  copyFloatMap(rec.ScoreBreakdown),
		})
	}

	if err := tx.Commit(ctx); err != nil {
		return RenderedRequest{}, fmt.Errorf("commit recommendation reactive tx: %w", err)
	}

	rendered, err := s.GetRequest(ctx, requestID)
	if err != nil {
		return RenderedRequest{}, err
	}
	rendered.ToolCalls = append([]ToolCallRecord(nil), input.ToolCalls...)
	return rendered, nil
}

// GetRequest loads a persisted recommendation request with rendered candidates.
func (s *Store) GetRequest(ctx context.Context, requestID string) (RenderedRequest, error) {
	var rendered RenderedRequest
	err := s.pool.QueryRow(ctx, `
SELECT id, trace_id, status
FROM recommendation_requests
WHERE id = $1`, requestID).Scan(&rendered.ID, &rendered.TraceID, &rendered.Status)
	if err != nil {
		if err == pgx.ErrNoRows {
			return RenderedRequest{}, fmt.Errorf("recommendation request %s not found", requestID)
		}
		return RenderedRequest{}, fmt.Errorf("load recommendation request %s: %w", requestID, err)
	}

	rows, err := s.pool.Query(ctx, `
SELECT r.id, r.candidate_id, c.title, COALESCE(r.rank_position, 0),
       r.score_breakdown, r.rationale, r.graph_signal_refs,
       r.policy_decisions, r.quality_decisions
FROM recommendations r
JOIN recommendation_candidates c ON c.id = r.candidate_id
WHERE r.request_id = $1 AND r.status = 'delivered'
ORDER BY r.rank_position ASC NULLS LAST, r.created_at ASC`, requestID)
	if err != nil {
		return RenderedRequest{}, fmt.Errorf("load recommendations for request %s: %w", requestID, err)
	}
	defer rows.Close()

	for rows.Next() {
		var rec RenderedRecommendation
		var scoreJSON, rationaleJSON, graphRefsJSON, policyJSON, qualityJSON []byte
		if err := rows.Scan(
			&rec.ID,
			&rec.CandidateID,
			&rec.Title,
			&rec.Rank,
			&scoreJSON,
			&rationaleJSON,
			&graphRefsJSON,
			&policyJSON,
			&qualityJSON,
		); err != nil {
			return RenderedRequest{}, fmt.Errorf("scan recommendation: %w", err)
		}
		if err := json.Unmarshal(scoreJSON, &rec.ScoreBreakdown); err != nil {
			return RenderedRequest{}, fmt.Errorf("decode score breakdown for %s: %w", rec.ID, err)
		}
		if err := json.Unmarshal(rationaleJSON, &rec.Rationale); err != nil {
			return RenderedRequest{}, fmt.Errorf("decode rationale for %s: %w", rec.ID, err)
		}
		if err := json.Unmarshal(graphRefsJSON, &rec.GraphSignalRefs); err != nil {
			return RenderedRequest{}, fmt.Errorf("decode graph refs for %s: %w", rec.ID, err)
		}
		if err := json.Unmarshal(policyJSON, &rec.PolicyDecisions); err != nil {
			return RenderedRequest{}, fmt.Errorf("decode policy decisions for %s: %w", rec.ID, err)
		}
		if err := json.Unmarshal(qualityJSON, &rec.QualityDecisions); err != nil {
			return RenderedRequest{}, fmt.Errorf("decode quality decisions for %s: %w", rec.ID, err)
		}
		rec.NoPersonalSignal = len(rec.GraphSignalRefs) == 0
		badges, sourceConflict, err := s.providerBadgesForCandidate(ctx, rec.CandidateID)
		if err != nil {
			return RenderedRequest{}, err
		}
		rec.ProviderBadges = badges
		rec.Attribution = badges
		rec.SourceConflict = sourceConflict
		rendered.Recommendations = append(rendered.Recommendations, rec)
	}
	if err := rows.Err(); err != nil {
		return RenderedRequest{}, fmt.Errorf("iterate recommendations: %w", err)
	}

	toolRows, err := s.pool.Query(ctx, `
SELECT tool_name, side_effect_class, arguments, result, rejection_reason, error, latency_ms, started_at
FROM agent_tool_calls
WHERE trace_id = $1
ORDER BY seq ASC`, rendered.TraceID)
	if err != nil {
		return RenderedRequest{}, fmt.Errorf("load tool calls for %s: %w", rendered.TraceID, err)
	}
	defer toolRows.Close()
	for toolRows.Next() {
		var call ToolCallRecord
		var argsJSON, resultJSON []byte
		if err := toolRows.Scan(&call.Name, &call.SideEffectClass, &argsJSON, &resultJSON, &call.RejectionReason, &call.Error, &call.LatencyMillis, &call.StartedAt); err != nil {
			return RenderedRequest{}, fmt.Errorf("scan tool call: %w", err)
		}
		if err := json.Unmarshal(argsJSON, &call.Arguments); err != nil {
			return RenderedRequest{}, fmt.Errorf("decode tool args for %s: %w", call.Name, err)
		}
		if len(resultJSON) > 0 {
			if err := json.Unmarshal(resultJSON, &call.Result); err != nil {
				return RenderedRequest{}, fmt.Errorf("decode tool result for %s: %w", call.Name, err)
			}
		}
		rendered.ToolCalls = append(rendered.ToolCalls, call)
	}
	if err := toolRows.Err(); err != nil {
		return RenderedRequest{}, fmt.Errorf("iterate tool calls: %w", err)
	}

	return rendered, nil
}

// GetRecommendation loads a single persisted recommendation.
func (s *Store) GetRecommendation(ctx context.Context, recommendationID string) (RenderedRecommendation, error) {
	var requestID string
	err := s.pool.QueryRow(ctx, `
SELECT request_id
FROM recommendations
WHERE id = $1`, recommendationID).Scan(&requestID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return RenderedRecommendation{}, fmt.Errorf("recommendation %s not found", recommendationID)
		}
		return RenderedRecommendation{}, fmt.Errorf("load recommendation %s: %w", recommendationID, err)
	}
	request, err := s.GetRequest(ctx, requestID)
	if err != nil {
		return RenderedRecommendation{}, err
	}
	for _, rec := range request.Recommendations {
		if rec.ID == recommendationID {
			return rec, nil
		}
	}
	return RenderedRecommendation{}, fmt.Errorf("recommendation %s not delivered", recommendationID)
}

// GraphSignalRefs returns bounded artifact refs relevant to a query.
func (s *Store) GraphSignalRefs(ctx context.Context, query string, limit int) ([]string, error) {
	if limit < 1 {
		limit = 3
	}
	rows, err := s.pool.Query(ctx, `
SELECT id
FROM artifacts
WHERE processing_status IN ('processed', 'completed')
  AND (
    LOWER(COALESCE(title, '') || ' ' || COALESCE(summary, '') || ' ' || COALESCE(content_raw, '')) LIKE '%' || LOWER($1) || '%'
    OR (LOWER($1) LIKE '%ramen%' AND LOWER(COALESCE(title, '') || ' ' || COALESCE(summary, '') || ' ' || COALESCE(content_raw, '')) LIKE '%ramen%')
  )
ORDER BY updated_at DESC
LIMIT $2`, query, limit)
	if err != nil {
		return nil, fmt.Errorf("load recommendation graph refs: %w", err)
	}
	defer rows.Close()
	var refs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan graph ref: %w", err)
		}
		refs = append(refs, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate graph refs: %w", err)
	}
	return refs, nil
}

func (s *Store) providerBadgesForCandidate(ctx context.Context, candidateID string) ([]ProviderBadge, bool, error) {
	rows, err := s.pool.Query(ctx, `
SELECT pf.provider_id, pf.attribution, pf.normalized_fact
FROM recommendation_candidate_provider_facts cpf
JOIN recommendation_provider_facts pf ON pf.id = cpf.provider_fact_id
WHERE cpf.candidate_id = $1
ORDER BY pf.provider_id ASC`, candidateID)
	if err != nil {
		return nil, false, fmt.Errorf("load provider badges for candidate %s: %w", candidateID, err)
	}
	defer rows.Close()
	seen := map[string]struct{}{}
	badges := []ProviderBadge{}
	sourceConflict := false
	for rows.Next() {
		var providerID string
		var attributionJSON, normalizedJSON []byte
		if err := rows.Scan(&providerID, &attributionJSON, &normalizedJSON); err != nil {
			return nil, false, fmt.Errorf("scan provider badge: %w", err)
		}
		var attribution map[string]any
		if err := json.Unmarshal(attributionJSON, &attribution); err != nil {
			return nil, false, fmt.Errorf("decode attribution for %s: %w", providerID, err)
		}
		var normalized map[string]any
		if err := json.Unmarshal(normalizedJSON, &normalized); err != nil {
			return nil, false, fmt.Errorf("decode normalized fact for %s: %w", providerID, err)
		}
		if conflict, _ := normalized["source_conflict"].(bool); conflict {
			sourceConflict = true
		}
		if _, ok := seen[providerID]; ok {
			continue
		}
		seen[providerID] = struct{}{}
		badge := ProviderBadge{ProviderID: providerID}
		if label, ok := attribution["label"].(string); ok {
			badge.Label = label
		}
		if badge.Label == "" {
			badge.Label = providerID
		}
		if url, ok := attribution["url"].(string); ok {
			badge.URL = url
		}
		badges = append(badges, badge)
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("iterate provider badges: %w", err)
	}
	return badges, sourceConflict, nil
}

func marshalObject(value map[string]any) ([]byte, error) {
	if value == nil {
		value = map[string]any{}
	}
	return json.Marshal(value)
}

func marshalAny(value any) ([]byte, error) {
	if value == nil {
		value = map[string]any{}
	}
	return json.Marshal(value)
}

func payloadHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func nullableText(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func copyFloatMap(values map[string]float64) map[string]float64 {
	if values == nil {
		return map[string]float64{}
	}
	out := make(map[string]float64, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func newTextID(prefix string) (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate %s id: %w", prefix, err)
	}
	return prefix + "_" + hex.EncodeToString(buf), nil
}
