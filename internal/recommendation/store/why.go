package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// ProviderFactSummary is safe recommendation provenance for why responses.
type ProviderFactSummary struct {
	ProviderID          string         `json:"provider_id"`
	ProviderCandidateID string         `json:"provider_candidate_id"`
	Category            string         `json:"category"`
	Title               string         `json:"title"`
	Attribution         map[string]any `json:"attribution"`
}

// WhyExplanation is the API/web model for an existing recommendation explanation.
type WhyExplanation struct {
	RecommendationID    string                `json:"recommendation_id"`
	TraceID             string                `json:"trace_id"`
	OriginTraceID       string                `json:"origin_trace_id"`
	ProviderCallsIssued bool                  `json:"provider_calls_issued"`
	Explanation         []string              `json:"explanation"`
	ProviderFacts       []ProviderFactSummary `json:"provider_facts"`
	PersonalSignals     []string              `json:"personal_signals"`
	PolicyDecisions     []map[string]any      `json:"policy_decisions"`
	QualityDecisions    []map[string]any      `json:"quality_decisions"`
}

// ExplainRecommendation creates a recommendation-why-v1 trace and explains
// one recommendation from persisted refs only.
func (s *Store) ExplainRecommendation(ctx context.Context, recommendationID string) (WhyExplanation, error) {
	if s == nil || s.pool == nil {
		return WhyExplanation{}, fmt.Errorf("recommendation store: postgres pool is required")
	}
	if strings.TrimSpace(recommendationID) == "" {
		return WhyExplanation{}, fmt.Errorf("recommendation_id is required")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return WhyExplanation{}, fmt.Errorf("begin why tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var requestID, candidateID, title, originTraceID string
	var rationaleJSON, graphRefsJSON, policyJSON, qualityJSON []byte
	err = tx.QueryRow(ctx, `
SELECT COALESCE(r.request_id, ''), r.candidate_id, c.title, COALESCE(r.trace_id, ''),
       r.rationale, r.graph_signal_refs, r.policy_decisions, r.quality_decisions
FROM recommendations r
JOIN recommendation_candidates c ON c.id = r.candidate_id
WHERE r.id = $1`, recommendationID).Scan(&requestID, &candidateID, &title, &originTraceID, &rationaleJSON, &graphRefsJSON, &policyJSON, &qualityJSON)
	if err != nil {
		return WhyExplanation{}, fmt.Errorf("load recommendation why target %s: %w", recommendationID, err)
	}

	var rationale []string
	if err := json.Unmarshal(rationaleJSON, &rationale); err != nil {
		return WhyExplanation{}, fmt.Errorf("decode why rationale: %w", err)
	}
	var graphRefs []string
	if err := json.Unmarshal(graphRefsJSON, &graphRefs); err != nil {
		return WhyExplanation{}, fmt.Errorf("decode why graph refs: %w", err)
	}
	var policyDecisions []map[string]any
	if err := json.Unmarshal(policyJSON, &policyDecisions); err != nil {
		return WhyExplanation{}, fmt.Errorf("decode why policy decisions: %w", err)
	}
	var qualityDecisions []map[string]any
	if err := json.Unmarshal(qualityJSON, &qualityDecisions); err != nil {
		return WhyExplanation{}, fmt.Errorf("decode why quality decisions: %w", err)
	}

	facts, err := loadProviderFactSummaries(ctx, tx, candidateID, requestID)
	if err != nil {
		return WhyExplanation{}, err
	}
	explanation := []string{
		fmt.Sprintf("Provider facts support %s from %s", title, providerFactLabels(facts)),
		fmt.Sprintf("Personal signals: %s", strings.Join(graphRefs, ", ")),
		fmt.Sprintf("Policy decisions: %s", decisionSummary(policyDecisions)),
		fmt.Sprintf("Quality decisions: %s", decisionSummary(qualityDecisions)),
	}
	if len(graphRefs) == 0 {
		explanation[1] = "Personal signals: none"
	}
	why := WhyExplanation{
		RecommendationID:    recommendationID,
		OriginTraceID:       originTraceID,
		ProviderCallsIssued: false,
		Explanation:         explanation,
		ProviderFacts:       facts,
		PersonalSignals:     graphRefs,
		PolicyDecisions:     policyDecisions,
		QualityDecisions:    qualityDecisions,
	}
	now := time.Now().UTC()
	toolCall := ToolCallRecord{
		Name:            "recommendation_explain_from_trace",
		SideEffectClass: "read",
		Arguments: map[string]any{
			"recommendation_id": recommendationID,
			"origin_trace_id":   originTraceID,
		},
		Result: map[string]any{
			"provider_calls_issued": false,
			"provider_fact_count":   len(facts),
			"personal_signal_count": len(graphRefs),
		},
		LatencyMillis: 1,
		StartedAt:     now,
	}
	traceID, err := insertScenarioTrace(ctx, tx, scenarioTraceInput{
		ScenarioID:      WhyScenarioID,
		ScenarioVersion: WhyScenarioVersion,
		Source:          "api",
		Outcome:         "explained",
		InputEnvelope:   map[string]any{"recommendation_id": recommendationID, "origin_trace_id": originTraceID},
		FinalOutput:     map[string]any{"recommendation_id": recommendationID, "provider_calls_issued": false},
		OutcomeDetail:   map[string]any{"provider_fact_count": len(facts), "personal_signal_count": len(graphRefs)},
		ToolCalls:       []ToolCallRecord{toolCall},
		StartedAt:       now,
		EndedAt:         now,
	})
	if err != nil {
		return WhyExplanation{}, err
	}
	why.TraceID = traceID
	if err := tx.Commit(ctx); err != nil {
		return WhyExplanation{}, fmt.Errorf("commit why tx: %w", err)
	}
	return why, nil
}

func loadProviderFactSummaries(ctx context.Context, tx pgx.Tx, candidateID, requestID string) ([]ProviderFactSummary, error) {
	rows, err := tx.Query(ctx, `
SELECT pf.provider_id, pf.provider_candidate_id, pf.category, COALESCE(pf.normalized_fact->>'title', ''), pf.attribution
FROM recommendation_candidate_provider_facts cpf
JOIN recommendation_provider_facts pf ON pf.id = cpf.provider_fact_id
WHERE cpf.candidate_id = $1
  AND ($2 = '' OR pf.request_id = $2)
ORDER BY pf.provider_id ASC`, candidateID, requestID)
	if err != nil {
		return nil, fmt.Errorf("load why provider facts: %w", err)
	}
	defer rows.Close()
	var facts []ProviderFactSummary
	for rows.Next() {
		var fact ProviderFactSummary
		var attributionJSON []byte
		if err := rows.Scan(&fact.ProviderID, &fact.ProviderCandidateID, &fact.Category, &fact.Title, &attributionJSON); err != nil {
			return nil, fmt.Errorf("scan why provider fact: %w", err)
		}
		if err := json.Unmarshal(attributionJSON, &fact.Attribution); err != nil {
			return nil, fmt.Errorf("decode why attribution: %w", err)
		}
		facts = append(facts, fact)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate why provider facts: %w", err)
	}
	return facts, nil
}

func providerFactLabels(facts []ProviderFactSummary) string {
	if len(facts) == 0 {
		return "no provider facts"
	}
	labels := make([]string, 0, len(facts))
	for _, fact := range facts {
		if label, ok := fact.Attribution["label"].(string); ok && label != "" {
			labels = append(labels, label)
			continue
		}
		labels = append(labels, fact.ProviderID)
	}
	return strings.Join(labels, ", ")
}

func decisionSummary(decisions []map[string]any) string {
	if len(decisions) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(decisions))
	for _, decision := range decisions {
		kind, _ := decision["kind"].(string)
		outcome, _ := decision["outcome"].(string)
		if kind == "" {
			kind = "decision"
		}
		if outcome == "" {
			outcome = "recorded"
		}
		parts = append(parts, kind+"="+outcome)
	}
	return strings.Join(parts, "; ")
}
