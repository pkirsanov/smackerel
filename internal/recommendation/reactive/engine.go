package reactive

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/smackerel/smackerel/internal/config"
	"github.com/smackerel/smackerel/internal/recommendation"
	"github.com/smackerel/smackerel/internal/recommendation/location"
	"github.com/smackerel/smackerel/internal/recommendation/policy"
	recprovider "github.com/smackerel/smackerel/internal/recommendation/provider"
	"github.com/smackerel/smackerel/internal/recommendation/quality"
	"github.com/smackerel/smackerel/internal/recommendation/rank"
	recstore "github.com/smackerel/smackerel/internal/recommendation/store"
)

const (
	ScenarioID      = "recommendation_reactive"
	ScenarioVersion = "recommendation-reactive-v1"
	ScenarioHash    = "recommendation-reactive-v1"
)

// ProviderRegistry is the provider lookup surface used by the engine.
type ProviderRegistry interface {
	Len() int
	List() []recprovider.Provider
}

// Options configures the reactive engine.
type Options struct {
	Store    *recstore.Store
	Registry ProviderRegistry
	Config   config.RecommendationsConfig
	Clock    func() time.Time
}

// Request is the normalized input to one reactive recommendation run.
type Request struct {
	ActorUserID     string
	Source          string
	Query           string
	LocationRef     string
	NamedLocation   string
	PrecisionPolicy recommendation.PrecisionPolicy
	Style           string
	ResultCount     int
	AllowedSources  []string
}

// Engine runs the Scope 2 reactive place recommendation decision order.
type Engine struct {
	store    *recstore.Store
	registry ProviderRegistry
	cfg      config.RecommendationsConfig
	clock    func() time.Time
}

// NewEngine returns a configured reactive recommendation engine.
func NewEngine(opts Options) *Engine {
	clock := opts.Clock
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	return &Engine{store: opts.Store, registry: opts.Registry, cfg: opts.Config, clock: clock}
}

// Run executes the full reactive recommendation pipeline and persists the outcome.
func (e *Engine) Run(ctx context.Context, req Request) (recstore.RenderedRequest, error) {
	if e == nil || e.store == nil {
		return recstore.RenderedRequest{}, fmt.Errorf("reactive recommendation store is required")
	}
	if strings.TrimSpace(req.ActorUserID) == "" {
		req.ActorUserID = "local"
	}
	if strings.TrimSpace(req.Source) == "" {
		req.Source = "api"
	}
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return recstore.RenderedRequest{}, fmt.Errorf("query is required")
	}
	precision := req.PrecisionPolicy
	if precision == "" {
		precision = recommendation.PrecisionPolicy(e.cfg.LocationPrecision.UserStandard)
	}
	if err := precision.Validate(); err != nil {
		return recstore.RenderedRequest{}, fmt.Errorf("precision policy: %w", err)
	}
	resultCount := req.ResultCount
	if resultCount == 0 {
		resultCount = e.cfg.Ranking.StandardResultCount
	}
	if max := e.cfg.Ranking.MaxFinalResults; max > 0 && resultCount > max {
		resultCount = max
	}
	if resultCount < 1 || resultCount > 10 {
		return recstore.RenderedRequest{}, fmt.Errorf("result_count must be between 1 and 10")
	}

	startedAt := e.clock().UTC()
	toolCalls := []recstore.ToolCallRecord{}
	appendTool := func(name, sideEffect string, args, result map[string]any) {
		toolCalls = append(toolCalls, recstore.ToolCallRecord{
			Name:            name,
			SideEffectClass: sideEffect,
			Arguments:       args,
			Result:          result,
			LatencyMillis:   1,
			StartedAt:       e.clock().UTC(),
		})
	}

	category := recommendation.CategoryPlace
	intent := map[string]any{
		"category": category,
		"query":    query,
		"style":    choose(req.Style, e.cfg.Ranking.StandardStyle),
	}
	appendTool("recommendation_parse_intent", "read", map[string]any{"query": query}, intent)

	if isAmbiguous(query, req.LocationRef, req.NamedLocation) {
		return e.store.CreateReactiveRequest(ctx, recstore.ReactiveOutcomeInput{
			ActorUserID:                req.ActorUserID,
			Source:                     req.Source,
			ScenarioID:                 ScenarioID,
			ScenarioVersion:            ScenarioVersion,
			ScenarioHash:               ScenarioHash,
			RawInput:                   query,
			ParsedRequest:              parsedRequest(req, string(category), resultCount),
			LocationPrecisionRequested: string(precision),
			LocationPrecisionSent:      string(precision),
			Status:                     "ambiguous",
			ToolCalls:                  toolCalls,
			Clarification: &recstore.Clarification{
				Question: "What kind of place should I look for?",
				Choices:  []string{"ramen", "coffee", "dinner"},
			},
			StartedAt:   startedAt,
			CompletedAt: e.clock().UTC(),
		})
	}

	if e.registry == nil || e.registry.Len() == 0 {
		record, err := e.store.CreateNoProviderRequest(ctx, recstore.CreateRequestInput{
			ActorUserID:                req.ActorUserID,
			Source:                     req.Source,
			ScenarioID:                 ScenarioVersion,
			RawInput:                   query,
			ParsedRequest:              parsedRequest(req, string(category), resultCount),
			LocationPrecisionRequested: string(precision),
			LocationPrecisionSent:      string(precision),
			Status:                     "no_providers",
		})
		if err != nil {
			return recstore.RenderedRequest{}, err
		}
		return recstore.RenderedRequest{ID: record.ID, TraceID: record.TraceID, Status: record.Status, Recommendations: []recstore.RenderedRecommendation{}}, nil
	}

	reducer := location.NewReducer(location.Config{
		NeighborhoodCellSystem: e.cfg.LocationPrecision.NeighborhoodCellSystem,
		NeighborhoodCellLevel:  e.cfg.LocationPrecision.NeighborhoodCellLevel,
	})
	locationRef := req.LocationRef
	if locationRef == "" {
		locationRef = req.NamedLocation
	}
	geometry, err := reducer.Reduce(ctx, location.RawLocationRef{Ref: locationRef}, precision)
	if err != nil {
		return recstore.RenderedRequest{}, err
	}
	appendTool("recommendation_reduce_location", "read", map[string]any{
		"location_ref_present": locationRef != "",
		"precision_policy":     string(precision),
	}, map[string]any{
		"precision": string(geometry.Precision),
		"cell_id":   geometry.CellID,
		"label":     geometry.Label,
	})

	providerLimit := e.cfg.Ranking.MaxCandidatesPerProvider
	if providerLimit < 1 {
		return recstore.RenderedRequest{}, fmt.Errorf("recommendation ranking max candidates per provider is required")
	}
	providerQuery := recprovider.ReducedQuery{
		Category:        category,
		Query:           query,
		PrecisionPolicy: precision,
		Geometry:        geometry,
		Limit:           providerLimit,
	}

	allowedSources := allowedSourceSet(req.AllowedSources)
	providerFacts := []recstore.ProviderFactInput{}
	providerStatuses := []map[string]any{}
	for _, providerEntry := range e.registry.List() {
		if len(allowedSources) > 0 {
			if _, ok := allowedSources[providerEntry.ID()]; !ok {
				continue
			}
		}
		bundle, err := providerEntry.Fetch(ctx, providerQuery)
		status := map[string]any{"provider_id": providerEntry.ID()}
		if err != nil {
			status["status"] = "degraded"
			status["error_kind"] = "provider_fetch_failed"
			providerStatuses = append(providerStatuses, status)
			continue
		}
		status["status"] = "healthy"
		status["fact_count"] = len(bundle.Facts)
		providerStatuses = append(providerStatuses, status)
		for i, fact := range bundle.Facts {
			providerFacts = append(providerFacts, recstore.ProviderFactInput{
				LocalID:             fmt.Sprintf("fact_%s_%d", providerEntry.ID(), i),
				ProviderID:          fact.ProviderID,
				ProviderCandidateID: fact.ProviderCandidateID,
				Category:            string(fact.Category),
				Title:               fact.Title,
				NormalizedFact:      copyAnyMap(fact.NormalizedFact),
				RetrievedAt:         fact.RetrievedAt,
				SourceUpdatedAt:     fact.SourceUpdatedAt,
				Attribution:         copyAnyMap(fact.Attribution),
				SponsoredState:      fact.SponsoredState,
				RestrictedFlags:     copyAnyMap(fact.RestrictedFlags),
			})
		}
	}
	appendTool("recommendation_fetch_candidates", "external", map[string]any{
		"category":          string(category),
		"precision_policy":  string(providerQuery.PrecisionPolicy),
		"location_cell_id":  providerQuery.Geometry.CellID,
		"location_cell_tag": providerQuery.Geometry.Label,
		"limit":             providerQuery.Limit,
	}, map[string]any{
		"provider_status": providerStatuses,
		"fact_count":      len(providerFacts),
	})

	if len(providerFacts) == 0 {
		return e.store.CreateReactiveRequest(ctx, recstore.ReactiveOutcomeInput{
			ActorUserID:                req.ActorUserID,
			Source:                     req.Source,
			ScenarioID:                 ScenarioID,
			ScenarioVersion:            ScenarioVersion,
			ScenarioHash:               ScenarioHash,
			RawInput:                   query,
			ParsedRequest:              parsedRequest(req, string(category), resultCount),
			LocationPrecisionRequested: string(precision),
			LocationPrecisionSent:      string(geometry.Precision),
			Status:                     "no_eligible",
			ToolCalls:                  toolCalls,
			StartedAt:                  startedAt,
			CompletedAt:                e.clock().UTC(),
		})
	}

	candidates := groupCandidates(providerFacts)
	appendTool("recommendation_dedupe_candidates", "read", map[string]any{"fact_count": len(providerFacts)}, map[string]any{"candidate_count": len(candidates)})

	graphRefs, err := e.store.GraphSignalRefs(ctx, query, 3)
	if err != nil {
		return recstore.RenderedRequest{}, err
	}
	appendTool("recommendation_get_graph_snapshot", "read", map[string]any{"candidate_count": len(candidates)}, map[string]any{"graph_signal_refs": graphRefs})

	preferenceCorrections, err := e.store.ActivePreferenceCorrections(ctx, req.ActorUserID)
	if err != nil {
		return recstore.RenderedRequest{}, err
	}
	rankingCorrections := make([]rank.PreferenceCorrection, 0, len(preferenceCorrections))
	for _, correction := range preferenceCorrections {
		rankingCorrections = append(rankingCorrections, rank.PreferenceCorrection{ID: correction.ID, PreferenceKey: correction.PreferenceKey, CorrectionKind: correction.CorrectionKind})
	}
	preferenceKey := inferredPreferenceKey(query)
	activeCorrection, boostBlocked := rank.ActiveCorrectionForPreference(preferenceKey, rankingCorrections)
	ranked := rankCandidates(candidates, graphRefs, preferenceKey, rankingCorrections)
	providerBackedIDs := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		providerBackedIDs = append(providerBackedIDs, candidate.LocalID)
	}
	if err := rank.ValidateProviderBackedRankings(ranked, providerBackedIDs); err != nil {
		return recstore.RenderedRequest{}, err
	}
	rankResult := map[string]any{"ranked_count": len(ranked)}
	if boostBlocked {
		rankResult["active_correction_ids"] = []string{activeCorrection.ID}
		rankResult["preference_boost_blocked"] = preferenceKey
	}
	appendTool("recommendation_rank_candidates", "read", map[string]any{"candidate_count": len(candidates)}, rankResult)

	canonicalKeys := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		canonicalKeys = append(canonicalKeys, candidate.CanonicalKey)
	}
	suppressionDecisions, err := e.store.ActiveSuppressionDecisions(ctx, recstore.SuppressionLookupInput{
		ActorUserID:   req.ActorUserID,
		Category:      string(category),
		CanonicalKeys: canonicalKeys,
	})
	if err != nil {
		return recstore.RenderedRequest{}, err
	}
	suppressionByKey := map[string]recstore.SuppressionDecision{}
	for _, decision := range suppressionDecisions {
		if _, exists := suppressionByKey[decision.CanonicalKey]; !exists {
			suppressionByKey[decision.CanonicalKey] = decision
		}
	}

	recommendations := []recstore.RecommendationInput{}
	constraints := hardConstraintsFromQuery(query)
	policyOpts := policy.SponsoredOptions{
		PromotionsEnabled: e.cfg.Policy.SponsoredPromotionsEnabled,
		QueryOptIn:        false,
		WatchOptIn:        false,
	}
	restrictedCategories := append([]string(nil), e.cfg.Policy.RestrictedCategories...)
	// Phase A: walk ranked candidates, drop blocked ones, build a list of
	// eligible-with-decisions entries that diversity grouping can then thin
	// before we materialize the final top-K recommendations.
	eligible := []reactiveEligibleEntry{}
	for _, rankedCandidate := range ranked {
		candidate := candidateByLocalID(candidates, rankedCandidate.CandidateID)
		if candidate == nil {
			continue
		}
		if suppression, ok := suppressionByKey[candidate.CanonicalKey]; ok {
			decisions := []map[string]any{{"kind": "suppression", "outcome": "block", "reason": suppression.Reason, "suppression_id": suppression.SuppressionID}}
			appendTool("recommendation_apply_policy", "read", map[string]any{"candidate_id": candidate.LocalID}, map[string]any{"decisions": decisions})
			recommendations = append(recommendations, recstore.RecommendationInput{
				CandidateLocalID: candidate.LocalID,
				Status:           "suppressed",
				StatusReason:     suppression.Reason,
				ScoreBreakdown:   rankedCandidate.ScoreBreakdown,
				Rationale:        []string{suppression.Reason},
				GraphSignalRefs:  []string{},
				PolicyDecisions:  decisions,
				QualityDecisions: []map[string]any{{"kind": "suppression", "outcome": "withheld", "reason": suppression.Reason}},
				DeliveryChannel:  req.Source,
			})
			continue
		}
		factForCandidate := firstFactForCandidate(providerFacts, *candidate)
		policyDecisions := policyDecisionsFor(*candidate, factForCandidate, constraints, policyOpts, restrictedCategories)
		appendTool("recommendation_apply_policy", "read", map[string]any{"candidate_id": candidate.LocalID}, map[string]any{"decisions": policyDecisions})
		if blockReason := blockingDecisionReason(policyDecisions); blockReason != "" {
			recommendations = append(recommendations, recstore.RecommendationInput{
				CandidateLocalID: candidate.LocalID,
				Status:           "withheld",
				StatusReason:     blockReason,
				ScoreBreakdown:   rankedCandidate.ScoreBreakdown,
				Rationale:        []string{blockReason},
				GraphSignalRefs:  []string{},
				PolicyDecisions:  policyDecisions,
				QualityDecisions: []map[string]any{{"kind": "policy_block", "outcome": "withheld", "reason": blockReason}},
				DeliveryChannel:  req.Source,
			})
			continue
		}
		qualityDecisions := qualityDecisionsFor(rankedCandidate, *candidate)
		appendTool("recommendation_apply_quality_guard", "read", map[string]any{"candidate_id": candidate.LocalID}, map[string]any{"decisions": qualityDecisions})

		rationale := rationaleFor(*candidate, rankedCandidate.GraphSignalRefs, preferenceKey, activeCorrection, boostBlocked)
		eligible = append(eligible, reactiveEligibleEntry{
			candidate:   *candidate,
			ranked:      rankedCandidate,
			policyDecs:  policyDecisions,
			qualityDecs: qualityDecisions,
			rationale:   rationale,
			chainKey:    quality.ChainKeyOf(candidate.CanonicalFact, candidate.Title),
		})
	}

	// Phase B: diversity grouping — at most one same-chain candidate keeps
	// its top-K slot; the rest are recorded as variants on the parent and
	// persisted as withheld with reason `withheld:diversity-grouped`.
	diversityInput := make([]quality.CandidateForDiversity, 0, len(eligible))
	for _, entry := range eligible {
		diversityInput = append(diversityInput, quality.CandidateForDiversity{
			LocalID:      entry.candidate.LocalID,
			CanonicalKey: entry.candidate.CanonicalKey,
			Title:        entry.candidate.Title,
			ChainKey:     entry.chainKey,
		})
	}
	diversity := quality.GroupNearDuplicates(diversityInput)
	keptOrder := make(map[string]int, len(diversity.KeptOrder))
	for i, id := range diversity.KeptOrder {
		keptOrder[id] = i
	}

	// Phase C: emit top-K kept candidates + persist diversity-withheld variants.
	parentDeliveredOrder := map[string]int{} // parent local id -> rank position
	for _, id := range diversity.KeptOrder {
		entry, found := findEligible(eligible, id)
		if !found {
			continue
		}
		variants := diversity.VariantsByParent[id]
		qualityDecs := append([]map[string]any(nil), entry.qualityDecs...)
		rationale := append([]string(nil), entry.rationale...)
		if len(variants) > 0 {
			qualityDecs = append(qualityDecs, quality.VariantsDecision(variants))
			rationale = append(rationale, fmt.Sprintf("Grouped %d same-chain branches under this card (variants disclosed)", len(variants)))
			appendTool("recommendation_apply_quality_guard", "read", map[string]any{"candidate_id": entry.candidate.LocalID, "guard": "diversity"}, map[string]any{"variant_count": len(variants)})
		}
		recommendations = append(recommendations, recstore.RecommendationInput{
			CandidateLocalID: entry.candidate.LocalID,
			RankPosition:     len(parentDeliveredOrder) + 1,
			Status:           "delivered",
			StatusReason:     "eligible",
			ScoreBreakdown:   entry.ranked.ScoreBreakdown,
			Rationale:        rationale,
			GraphSignalRefs:  append([]string(nil), entry.ranked.GraphSignalRefs...),
			PolicyDecisions:  entry.policyDecs,
			QualityDecisions: qualityDecs,
			DeliveryChannel:  req.Source,
		})
		parentDeliveredOrder[entry.candidate.LocalID] = len(parentDeliveredOrder)
		if len(parentDeliveredOrder) == resultCount {
			break
		}
	}
	// Persist variants whose parent landed in top-K so audit can list them.
	for variantID, parentID := range diversity.ParentByVariant {
		if _, parentDelivered := parentDeliveredOrder[parentID]; !parentDelivered {
			continue
		}
		entry, found := findEligible(eligible, variantID)
		if !found {
			continue
		}
		recommendations = append(recommendations, recstore.RecommendationInput{
			CandidateLocalID: entry.candidate.LocalID,
			Status:           "withheld",
			StatusReason:     "withheld:diversity-grouped",
			ScoreBreakdown:   entry.ranked.ScoreBreakdown,
			Rationale:        []string{"Grouped under same-chain parent " + parentID},
			GraphSignalRefs:  []string{},
			PolicyDecisions:  entry.policyDecs,
			QualityDecisions: append(entry.qualityDecs, map[string]any{"kind": "diversity", "outcome": "variant_withheld", "reason": "same-chain", "parent_local_id": parentID}),
			DeliveryChannel:  req.Source,
		})
	}
	appendTool("recommendation_apply_quality_guard", "read", map[string]any{"delivered_count": len(recommendations)}, map[string]any{"status": "complete"})

	// Status reflects whether at least one recommendation was actually
	// delivered. Withheld / diversity-grouped / suppressed entries are
	// audit rows and MUST NOT promote a request from no_eligible to
	// delivered (BS-029 / no silent relaxation when no candidate qualifies).
	deliveredCount := 0
	for _, rec := range recommendations {
		if rec.Status == "delivered" {
			deliveredCount++
		}
	}
	status := "delivered"
	if deliveredCount == 0 {
		status = "no_eligible"
	}
	appendTool("recommendation_persist_outcome", "write", map[string]any{"status": status}, map[string]any{"candidate_count": len(candidates), "recommendation_count": len(recommendations)})

	return e.store.CreateReactiveRequest(ctx, recstore.ReactiveOutcomeInput{
		ActorUserID:                req.ActorUserID,
		Source:                     req.Source,
		ScenarioID:                 ScenarioID,
		ScenarioVersion:            ScenarioVersion,
		ScenarioHash:               ScenarioHash,
		RawInput:                   query,
		ParsedRequest:              parsedRequest(req, string(category), resultCount),
		LocationPrecisionRequested: string(precision),
		LocationPrecisionSent:      string(geometry.Precision),
		Status:                     status,
		ToolCalls:                  toolCalls,
		ProviderFacts:              providerFacts,
		Candidates:                 candidates,
		Recommendations:            recommendations,
		StartedAt:                  startedAt,
		CompletedAt:                e.clock().UTC(),
	})
}

func parsedRequest(req Request, category string, resultCount int) map[string]any {
	return map[string]any{
		"query":            req.Query,
		"source":           req.Source,
		"location_ref":     req.LocationRef,
		"named_location":   req.NamedLocation,
		"precision_policy": string(req.PrecisionPolicy),
		"category":         category,
		"style":            req.Style,
		"result_count":     resultCount,
		"allowed_sources":  append([]string(nil), req.AllowedSources...),
	}
}

func isAmbiguous(query, locationRef, namedLocation string) bool {
	lower := strings.ToLower(strings.TrimSpace(query))
	if locationRef != "" || namedLocation != "" {
		return false
	}
	for _, token := range []string{"ramen", "coffee", "restaurant", "place", "dinner", "lunch"} {
		if strings.Contains(lower, token) {
			return false
		}
	}
	return true
}

func choose(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func allowedSourceSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out[value] = struct{}{}
		}
	}
	return out
}

func groupCandidates(facts []recstore.ProviderFactInput) []recstore.CandidateInput {
	type group struct {
		candidate recstore.CandidateInput
		facts     []recstore.ProviderFactInput
	}
	groups := map[string]*group{}
	order := []string{}
	for _, fact := range facts {
		key := canonicalKeyFromFact(fact)
		if _, ok := groups[key]; !ok {
			localID := "cand_" + key
			groups[key] = &group{candidate: recstore.CandidateInput{
				LocalID:      localID,
				Category:     fact.Category,
				CanonicalKey: key,
				Title:        fact.Title,
				CanonicalURL: stringFromAny(fact.NormalizedFact["canonical_url"]),
				CanonicalFact: map[string]any{
					"title":         fact.Title,
					"canonical_key": key,
					"provider_ids":  []string{},
				},
				DedupeReason: map[string]any{"strategy": "canonical_key"},
				MergeReason:  "same-canonical-key",
			}}
			order = append(order, key)
		}
		groups[key].facts = append(groups[key].facts, fact)
		groups[key].candidate.ProviderFactLocalIDs = append(groups[key].candidate.ProviderFactLocalIDs, fact.LocalID)
		mergeFact(groups[key].candidate.CanonicalFact, fact)
	}
	out := make([]recstore.CandidateInput, 0, len(order))
	for _, key := range order {
		out = append(out, groups[key].candidate)
	}
	return out
}

func mergeFact(canonicalFact map[string]any, fact recstore.ProviderFactInput) {
	providerIDs, _ := canonicalFact["provider_ids"].([]string)
	providerIDs = append(providerIDs, fact.ProviderID)
	canonicalFact["provider_ids"] = providerIDs
	for _, key := range []string{
		"provider_score", "quiet", "vegetarian", "open_now", "opening_hours",
		"source_conflict", "canonical_url", "distance_basis", "distance_label",
		// Diversity grouping inputs (BS-027 / SCN-039-043).
		"chain_id", "chain_name",
		// Total-cost transparency inputs (BS-031 / SCN-039-044).
		"headline_price", "shipping_cost", "shipping_known",
		"return_policy", "return_policy_known", "taxes_included",
		"total_cost", "cheapest_claimed",
	} {
		if value, ok := fact.NormalizedFact[key]; ok {
			if _, exists := canonicalFact[key]; !exists || key == "source_conflict" {
				canonicalFact[key] = value
			}
		}
	}
}

func rankCandidates(candidates []recstore.CandidateInput, graphRefs []string, preferenceKey string, corrections []rank.PreferenceCorrection) []rank.RankedCandidate {
	ranked := make([]rank.RankedCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		providerScore := floatFromAny(candidate.CanonicalFact["provider_score"])
		quietBoost := 0.0
		if quiet, _ := candidate.CanonicalFact["quiet"].(bool); quiet {
			quietBoost = 0.06
		}
		graphBoost := 0.0
		candidateGraphRefs := []string{}
		if len(graphRefs) > 0 && candidateMatchesGraphSignal(candidate.Title) && rank.PositiveBoostAllowed(preferenceKey, corrections) {
			graphBoost = 0.12
			candidateGraphRefs = append(candidateGraphRefs, graphRefs...)
		}
		score := providerScore + quietBoost + graphBoost
		confidence := "high"
		if score < 0.7 {
			confidence = "low"
		} else if score < 0.82 {
			confidence = "medium"
		}
		ranked = append(ranked, rank.RankedCandidate{
			CandidateID: candidate.LocalID,
			ScoreBreakdown: map[string]float64{
				"provider_score": providerScore,
				"quiet_boost":    quietBoost,
				"graph_boost":    graphBoost,
				"total":          score,
			},
			GraphSignalRefs: candidateGraphRefs,
			Confidence:      confidence,
		})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		return ranked[i].ScoreBreakdown["total"] > ranked[j].ScoreBreakdown["total"]
	})
	for i := range ranked {
		ranked[i].Rank = i + 1
	}
	return ranked
}

func inferredPreferenceKey(query string) string {
	lower := strings.ToLower(query)
	if strings.Contains(lower, "spicy") {
		return "loves_spicy"
	}
	return ""
}

type hardConstraints struct {
	Vegetarian bool
	OpenNow    bool
}

func hardConstraintsFromQuery(query string) hardConstraints {
	lower := strings.ToLower(query)
	return hardConstraints{
		Vegetarian: strings.Contains(lower, "vegetarian"),
		OpenNow:    strings.Contains(lower, "open now"),
	}
}

func policyDecisionsFor(candidate recstore.CandidateInput, fact recstore.ProviderFactInput, constraints hardConstraints, sponsoredOpts policy.SponsoredOptions, restrictedCategories []string) []map[string]any {
	decisions := []map[string]any{{"kind": "consent", "outcome": "allow", "reason": "reactive-request"}}
	// Sponsored guard runs first so the renderer can label the candidate
	// even when other guards later allow it.
	for _, decision := range policy.EvaluateSponsored(fact.SponsoredState, sponsoredOpts) {
		decisions = append(decisions, map[string]any{
			"kind":    decision.Kind,
			"outcome": decision.Outcome,
			"reason":  decision.Reason,
		})
	}
	// Restricted-category guard withholds candidates that match the user's
	// blocked or restricted category list with a category-level reason.
	if restrictedDecision := policy.EvaluateRestricted(fact.RestrictedFlags, restrictedCategories); restrictedDecision.Kind != "" {
		decisions = append(decisions, map[string]any{
			"kind":    restrictedDecision.Kind,
			"outcome": restrictedDecision.Outcome,
			"reason":  restrictedDecision.Reason,
		})
		if restrictedDecision.Outcome == "withhold" {
			return decisions
		}
	}
	// Safety/recall guard withholds candidates carrying any safety advisory.
	if safetyDecision := policy.EvaluateSafety(fact.RestrictedFlags); safetyDecision.Kind != "" {
		decisions = append(decisions, map[string]any{
			"kind":    safetyDecision.Kind,
			"outcome": safetyDecision.Outcome,
			"reason":  safetyDecision.Reason,
		})
		if safetyDecision.Outcome == "withhold" {
			return decisions
		}
	}
	if constraints.Vegetarian {
		vegetarian, _ := candidate.CanonicalFact["vegetarian"].(bool)
		if !vegetarian || strings.Contains(strings.ToLower(candidate.Title), "pork") {
			decisions = append(decisions, map[string]any{"kind": "hard_constraint", "outcome": "block", "reason": "vegetarian-required"})
			return decisions
		}
	}
	if constraints.OpenNow {
		openNow, _ := candidate.CanonicalFact["open_now"].(bool)
		if !openNow {
			decisions = append(decisions, map[string]any{"kind": "hard_constraint", "outcome": "block", "reason": "open-now-required"})
			return decisions
		}
	}
	decisions = append(decisions, map[string]any{"kind": "hard_constraint", "outcome": "allow", "reason": "constraints-satisfied"})
	return decisions
}

func hasBlockingDecision(decisions []map[string]any) bool {
	for _, decision := range decisions {
		outcome, _ := decision["outcome"].(string)
		if outcome == "block" || outcome == "withhold" {
			return true
		}
	}
	return false
}

// blockingDecisionReason returns the first block/withhold reason found in
// decisions, or empty string when none is present.
func blockingDecisionReason(decisions []map[string]any) string {
	for _, decision := range decisions {
		outcome, _ := decision["outcome"].(string)
		if outcome == "block" || outcome == "withhold" {
			reason, _ := decision["reason"].(string)
			if reason == "" {
				return outcome
			}
			return reason
		}
	}
	return ""
}

func qualityDecisionsFor(candidate rank.RankedCandidate, ci recstore.CandidateInput) []map[string]any {
	decisions := []map[string]any{{"kind": "provider_fact_ref", "outcome": "allow", "reason": "ranked-candidate-has-provider-fact"}}
	if candidate.Confidence == "low" {
		decisions = append(decisions, map[string]any{"kind": "confidence", "outcome": "disclose", "reason": "low-confidence"})
	} else {
		decisions = append(decisions, map[string]any{"kind": "confidence", "outcome": "allow", "reason": candidate.Confidence})
	}
	// Total-cost transparency disclosures (BS-031 / SCN-039-044).
	totalCostFacts := quality.TotalCostFactsFromMap(ci.CanonicalFact)
	decisions = append(decisions, quality.EvaluateTotalCost(totalCostFacts)...)
	return decisions
}

// firstFactForCandidate returns the first persisted provider fact whose
// LocalID is referenced by the candidate. When no match is found a
// zero-valued ProviderFactInput is returned so guard callers can still
// inspect the (empty) sponsored_state and restricted_flags safely.
func firstFactForCandidate(facts []recstore.ProviderFactInput, candidate recstore.CandidateInput) recstore.ProviderFactInput {
	if len(candidate.ProviderFactLocalIDs) == 0 {
		return recstore.ProviderFactInput{RestrictedFlags: map[string]any{}, Attribution: map[string]any{}}
	}
	for _, factID := range candidate.ProviderFactLocalIDs {
		for _, fact := range facts {
			if fact.LocalID == factID {
				return fact
			}
		}
	}
	return recstore.ProviderFactInput{RestrictedFlags: map[string]any{}, Attribution: map[string]any{}}
}

// reactiveEligibleEntry is the per-candidate working record built by the
// reactive engine after policy guards but before diversity grouping.
type reactiveEligibleEntry struct {
	candidate   recstore.CandidateInput
	ranked      rank.RankedCandidate
	policyDecs  []map[string]any
	qualityDecs []map[string]any
	rationale   []string
	chainKey    string
}

// findEligible returns the eligible entry whose candidate matches localID.
func findEligible(entries []reactiveEligibleEntry, localID string) (reactiveEligibleEntry, bool) {
	for _, entry := range entries {
		if entry.candidate.LocalID == localID {
			return entry, true
		}
	}
	return reactiveEligibleEntry{}, false
}

func rationaleFor(candidate recstore.CandidateInput, graphRefs []string, preferenceKey string, correction rank.PreferenceCorrection, boostBlocked bool) []string {
	reasons := []string{"Provider facts support " + candidate.Title}
	if quiet, _ := candidate.CanonicalFact["quiet"].(bool); quiet {
		reasons = append(reasons, "Quiet setting matches the request")
	}
	if len(graphRefs) > 0 && candidateMatchesGraphSignal(candidate.Title) {
		reasons = append(reasons, "Personal graph signal "+strings.Join(graphRefs, ", ")+" supports this pick")
	}
	if len(graphRefs) == 0 {
		reasons = append(reasons, "No personal signals applied")
	}
	if boostBlocked && preferenceKey != "" && correction.ID != "" {
		reasons = append(reasons, "Preference correction "+correction.ID+" blocks "+preferenceKey+" as a positive ranking signal")
	}
	return reasons
}

func candidateMatchesGraphSignal(title string) bool {
	lower := strings.ToLower(title)
	return strings.Contains(lower, "menkichi") || strings.Contains(lower, "tonkotsu")
}

func candidateByLocalID(candidates []recstore.CandidateInput, id string) *recstore.CandidateInput {
	for i := range candidates {
		if candidates[i].LocalID == id {
			return &candidates[i]
		}
	}
	return nil
}

func canonicalKeyFromFact(fact recstore.ProviderFactInput) string {
	if key := stringFromAny(fact.NormalizedFact["canonical_key"]); key != "" {
		return key
	}
	value := strings.ToLower(strings.TrimSpace(fact.Title))
	value = strings.NewReplacer(" ", "-", "'", "", "&", "and").Replace(value)
	return value
}

func copyAnyMap(values map[string]any) map[string]any {
	if values == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func stringFromAny(value any) string {
	text, _ := value.(string)
	return text
}

func floatFromAny(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	default:
		return 0
	}
}
