package knowledge

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Spec 041 Scope 7 — personal-context read API host sensitivity query.
//
// The Scope 7 route handler hands the consent-token ceiling and the
// per-user privacy ceiling to this helper, which returns:
//
//   - items whose sensitivity_tier is at-or-below the EFFECTIVE ceiling
//     (the lesser of the two ceilings — SCN-SM-041-026), and
//   - the count of items that would have been returned at the consent
//     ceiling but were redacted by the per-user privacy ceiling
//     (so the handler can populate response.redaction_count and emit
//     outcome="degraded" when non-zero).
//
// The helper queries the canonical artifacts table; per-artifact
// metadata fields are stored under the JSONB metadata column
// (artifact.metadata->>'entity_ref' and
// artifact.metadata->>'sensitivity_tier'). The vocabulary is the
// documented Scope 7 vocabulary ("low" < "medium" < "high"); rows with
// missing or out-of-vocabulary metadata are conservatively skipped.

// PersonalContextItem is one row returned to QF in the response.items[]
// array. Field names match the JSON contract declared by SCN-SM-041-025.
type PersonalContextItem struct {
	ArtifactID      string    `json:"artifact_id"`
	Kind            string    `json:"kind"`
	SensitivityTier string    `json:"sensitivity_tier"`
	Summary         string    `json:"summary"`
	SourceRef       string    `json:"source_ref"`
	CapturedAt      time.Time `json:"captured_at"`
}

// PersonalContextSensitivityRanker is the per-tier ordering injected by
// the Scope 7 handler. The exported function lives in
// internal/connector/qfdecisions (PersonalContextTierLessOrEqual) so the
// knowledge package does not import qfdecisions and the ordering is
// owned by a single source.
type PersonalContextSensitivityRanker func(candidate, ceiling string) bool

// PersonalContextSensitivityQuerier reads sensitivity-filtered personal
// context items from the artifacts table.
type PersonalContextSensitivityQuerier struct {
	pool *pgxpool.Pool
}

// NewPersonalContextSensitivityQuerier returns a querier backed by pool.
func NewPersonalContextSensitivityQuerier(pool *pgxpool.Pool) *PersonalContextSensitivityQuerier {
	return &PersonalContextSensitivityQuerier{pool: pool}
}

// PersonalContextQueryRequest is the input to QueryByEntityRef.
type PersonalContextQueryRequest struct {
	EntityRef       string
	ConsentCeiling  string
	UserCeiling     string
	Limit           int
	TierLessOrEqual PersonalContextSensitivityRanker
}

// PersonalContextQueryResult is the output of QueryByEntityRef.
type PersonalContextQueryResult struct {
	Items          []PersonalContextItem
	RedactionCount int
	EffectiveTier  string
}

// QueryByEntityRef returns the items + redaction count for the given
// consent + user ceiling pair.
//
//   - Items returned satisfy sensitivity_tier <= min(consent, user).
//   - RedactionCount counts items that would have satisfied
//     sensitivity_tier <= consent BUT exceed user (so the per-user
//     privacy ceiling redacted them).
//
// Rows whose metadata->>'sensitivity_tier' is missing or out-of-vocabulary
// are conservatively skipped (not counted as either returned or
// redacted) so a future schema drift cannot accidentally widen access.
func (q *PersonalContextSensitivityQuerier) QueryByEntityRef(ctx context.Context, req PersonalContextQueryRequest) (PersonalContextQueryResult, error) {
	if q == nil || q.pool == nil {
		return PersonalContextQueryResult{}, errors.New("personal-context sensitivity querier is not configured")
	}
	entityRef := strings.TrimSpace(req.EntityRef)
	if entityRef == "" {
		return PersonalContextQueryResult{}, errors.New("entity_ref is required")
	}
	if req.TierLessOrEqual == nil {
		return PersonalContextQueryResult{}, errors.New("TierLessOrEqual ranker is required")
	}
	consentCeiling := strings.TrimSpace(req.ConsentCeiling)
	userCeiling := strings.TrimSpace(req.UserCeiling)
	if consentCeiling == "" || userCeiling == "" {
		return PersonalContextQueryResult{}, errors.New("consent and user ceilings are required")
	}

	limit := req.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	rows, err := q.pool.Query(ctx, `
		SELECT
			id,
			artifact_type,
			COALESCE(metadata->>'sensitivity_tier', '') AS sensitivity_tier,
			COALESCE(summary, '')                       AS summary,
			COALESCE(source_url, COALESCE(source_ref, '')) AS source_ref,
			created_at
		  FROM artifacts
		 WHERE metadata->>'entity_ref' = $1
		   AND metadata->>'sensitivity_tier' IS NOT NULL
		 ORDER BY created_at DESC
		 LIMIT $2
	`, entityRef, limit)
	if err != nil {
		return PersonalContextQueryResult{}, fmt.Errorf("query personal-context items: %w", err)
	}
	defer rows.Close()

	result := PersonalContextQueryResult{
		EffectiveTier: minTier(consentCeiling, userCeiling, req.TierLessOrEqual),
	}
	for rows.Next() {
		var (
			id        string
			kind      string
			tier      string
			summary   string
			sourceRef string
			createdAt time.Time
		)
		if err := rows.Scan(&id, &kind, &tier, &summary, &sourceRef, &createdAt); err != nil {
			return PersonalContextQueryResult{}, fmt.Errorf("scan personal-context row: %w", err)
		}
		tier = strings.TrimSpace(tier)
		if tier == "" {
			continue
		}
		// Skip rows whose tier is out-of-vocabulary (the ranker
		// returns false for both directions, so the row is neither
		// returned nor counted as redacted).
		if !req.TierLessOrEqual(tier, PersonalContextTierHighSentinel) &&
			!req.TierLessOrEqual(PersonalContextTierLowSentinel, tier) {
			continue
		}
		if !req.TierLessOrEqual(tier, consentCeiling) {
			// Above the consent ceiling — neither returned nor
			// counted as redacted. Only the user ceiling can
			// redact items the consent ceiling would otherwise
			// admit.
			continue
		}
		if !req.TierLessOrEqual(tier, userCeiling) {
			// Within consent but above user — redacted.
			result.RedactionCount++
			continue
		}
		result.Items = append(result.Items, PersonalContextItem{
			ArtifactID:      id,
			Kind:            kind,
			SensitivityTier: tier,
			Summary:         summary,
			SourceRef:       sourceRef,
			CapturedAt:      createdAt.UTC(),
		})
	}
	if err := rows.Err(); err != nil {
		return PersonalContextQueryResult{}, fmt.Errorf("iterate personal-context rows: %w", err)
	}
	return result, nil
}

// PersonalContextTierLowSentinel / HighSentinel are documented vocab
// constants used by the in-package vocabulary-membership guard. They
// MUST match the canonical vocabulary owned by
// internal/connector/qfdecisions (PersonalContextTierLow/High). The
// duplication is intentional: this package does not import qfdecisions
// to avoid an import cycle (qfdecisions imports knowledge transitively
// nowhere today, but the handler wires both, and keeping knowledge
// free of qfdecisions imports preserves that boundary).
const (
	PersonalContextTierLowSentinel  = "low"
	PersonalContextTierHighSentinel = "high"
)

// minTier returns the more-restrictive of a and b using the injected
// ranker. Both inputs MUST be from the documented vocabulary; an
// invalid input falls back to the most-restrictive sentinel.
func minTier(a, b string, lessOrEqual PersonalContextSensitivityRanker) string {
	if lessOrEqual(a, b) {
		return a
	}
	return b
}
