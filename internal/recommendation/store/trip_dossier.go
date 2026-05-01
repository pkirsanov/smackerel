package store

import (
	"context"
	"encoding/json"
	"fmt"
)

// TripDossierGroup is one category-grouped block in the trip dossier render
// model. The Web handler renders Groups in iteration order; tests assert the
// shape and the count of variants per parent recommendation.
type TripDossierGroup struct {
	Category        string                      `json:"category"`
	Recommendations []TripDossierRecommendation `json:"recommendations"`
}

// TripDossierRecommendation is one delivered recommendation enriched with
// its diversity-grouped variants. The variants are rebuilt from the parent
// recommendation's quality_decisions[].variant_keys + variant_titles entries
// so the render does not need a separate variant table.
type TripDossierRecommendation struct {
	RenderedRecommendation
	Variants []TripDossierVariant `json:"variants"`
}

// TripDossierVariant is one near-duplicate variant collapsed under its
// parent recommendation by the diversity guard (BS-027 / SCN-039-043).
type TripDossierVariant struct {
	CanonicalKey string `json:"canonical_key"`
	Title        string `json:"title"`
}

// ListRecommendationsForTrip returns the trip dossier render model for the
// supplied trip ID. A trip ID matches when the candidate's canonical_fact
// JSONB contains a `trip_id` field equal to the supplied value AND the
// recommendation status is `delivered`. Recommendations are returned grouped
// by category in stable category-name order, then by rank_position within
// each category.
//
// The dossier render block (per design Component Tree:
// `TripDossier -> RecommendationGroupByCategory -> DossierRecommendationRow ->
// VariantGroup`) is built here so the web handler stays presentation-only.
func (s *Store) ListRecommendationsForTrip(ctx context.Context, tripID string) ([]TripDossierGroup, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("recommendation store: pool not configured")
	}
	if tripID == "" {
		return nil, fmt.Errorf("recommendation store: trip id is required")
	}
	const query = `
SELECT r.id, r.candidate_id, c.title, COALESCE(r.rank_position, 0), c.canonical_fact,
       c.category,
       r.score_breakdown, r.rationale, r.graph_signal_refs,
       r.policy_decisions, r.quality_decisions
FROM recommendations r
JOIN recommendation_candidates c ON c.id = r.candidate_id
WHERE c.canonical_fact->>'trip_id' = $1
  AND r.status = 'delivered'
ORDER BY c.category ASC, r.rank_position ASC NULLS LAST, r.created_at ASC`
	rows, err := s.pool.Query(ctx, query, tripID)
	if err != nil {
		return nil, fmt.Errorf("query trip dossier recommendations: %w", err)
	}
	defer rows.Close()

	groupsByCategory := map[string]*TripDossierGroup{}
	categoryOrder := []string{}
	for rows.Next() {
		var rec RenderedRecommendation
		var category string
		var canonicalJSON, scoreJSON, rationaleJSON, graphRefsJSON, policyJSON, qualityJSON []byte
		if err := rows.Scan(
			&rec.ID,
			&rec.CandidateID,
			&rec.Title,
			&rec.Rank,
			&canonicalJSON,
			&category,
			&scoreJSON,
			&rationaleJSON,
			&graphRefsJSON,
			&policyJSON,
			&qualityJSON,
		); err != nil {
			return nil, fmt.Errorf("scan trip dossier row: %w", err)
		}
		if err := json.Unmarshal(scoreJSON, &rec.ScoreBreakdown); err != nil {
			return nil, fmt.Errorf("decode score breakdown for %s: %w", rec.ID, err)
		}
		if err := json.Unmarshal(rationaleJSON, &rec.Rationale); err != nil {
			return nil, fmt.Errorf("decode rationale for %s: %w", rec.ID, err)
		}
		if err := json.Unmarshal(graphRefsJSON, &rec.GraphSignalRefs); err != nil {
			return nil, fmt.Errorf("decode graph refs for %s: %w", rec.ID, err)
		}
		if err := json.Unmarshal(policyJSON, &rec.PolicyDecisions); err != nil {
			return nil, fmt.Errorf("decode policy decisions for %s: %w", rec.ID, err)
		}
		if err := json.Unmarshal(qualityJSON, &rec.QualityDecisions); err != nil {
			return nil, fmt.Errorf("decode quality decisions for %s: %w", rec.ID, err)
		}
		var canonical map[string]any
		if err := json.Unmarshal(canonicalJSON, &canonical); err != nil {
			return nil, fmt.Errorf("decode canonical fact for %s: %w", rec.ID, err)
		}
		rec.PersonalSignalsApplied = len(rec.GraphSignalRefs) > 0
		rec.NoPersonalSignal = !rec.PersonalSignalsApplied
		rec.LowConfidence = qualityDecisionMatches(rec.QualityDecisions, "confidence", "disclose")
		rec.DistanceBasis = stringFromAny(canonical["distance_basis"])
		rec.DistanceLabel = stringFromAny(canonical["distance_label"])
		badges, sourceConflict, err := s.providerBadgesForCandidate(ctx, rec.CandidateID, rec.ID)
		if err != nil {
			return nil, err
		}
		rec.ProviderBadges = badges
		rec.Attribution = badges
		canonicalConflict, _ := canonical["source_conflict"].(bool)
		rec.SourceConflict = sourceConflict || canonicalConflict

		row := TripDossierRecommendation{
			RenderedRecommendation: rec,
			Variants:               extractDiversityVariants(rec.QualityDecisions),
		}
		group, exists := groupsByCategory[category]
		if !exists {
			group = &TripDossierGroup{Category: category}
			groupsByCategory[category] = group
			categoryOrder = append(categoryOrder, category)
		}
		group.Recommendations = append(group.Recommendations, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate trip dossier rows: %w", err)
	}
	out := make([]TripDossierGroup, 0, len(categoryOrder))
	for _, cat := range categoryOrder {
		out = append(out, *groupsByCategory[cat])
	}
	return out, nil
}

// extractDiversityVariants walks the parent recommendation's quality_decisions
// list and pulls each `diversity` decision's variant_keys + variant_titles into
// an ordered TripDossierVariant slice. The slice is empty when no diversity
// guard fired for this parent.
func extractDiversityVariants(decisions []map[string]any) []TripDossierVariant {
	out := []TripDossierVariant{}
	for _, decision := range decisions {
		kind, _ := decision["kind"].(string)
		if kind != "diversity" {
			continue
		}
		keys := stringSliceFromAny(decision["variant_keys"])
		titles := stringSliceFromAny(decision["variant_titles"])
		for i, key := range keys {
			title := ""
			if i < len(titles) {
				title = titles[i]
			}
			out = append(out, TripDossierVariant{CanonicalKey: key, Title: title})
		}
	}
	return out
}

// stringSliceFromAny converts a JSON-decoded `[]any` of strings (possibly
// nil) into a clean `[]string`. Non-string entries are skipped.
func stringSliceFromAny(value any) []string {
	raw, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, entry := range raw {
		if s, ok := entry.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
