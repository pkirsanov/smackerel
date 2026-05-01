// Package watch implements the standing-watch evaluation pipeline for
// spec 039 Scope 4. Watches differ from reactive recommendations because
// they are scheduler-fired and audited as standing artifacts: each due
// watch is invoked as scenario `recommendation_watch_evaluate` (version
// `recommendation-watch-evaluate-v1`) and every decision (delivered,
// withheld, queue|summarize|drop) is persisted as a `recommendation_watch_run`.
package watch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/smackerel/smackerel/internal/metrics"
	"github.com/smackerel/smackerel/internal/recommendation"
	"github.com/smackerel/smackerel/internal/recommendation/location"
	"github.com/smackerel/smackerel/internal/recommendation/policy"
	recprovider "github.com/smackerel/smackerel/internal/recommendation/provider"
	recstore "github.com/smackerel/smackerel/internal/recommendation/store"
)

const (
	// ScenarioID matches the agent scenario identifier used by FireScenario.
	ScenarioID = "recommendation_watch_evaluate"
	// ScenarioVersion matches the contract version asset filename.
	ScenarioVersion = "recommendation-watch-evaluate-v1"
)

// ProviderRegistry is the provider lookup surface used by watch evaluation.
type ProviderRegistry interface {
	Len() int
	List() []recprovider.Provider
	Get(id string) (recprovider.Provider, bool)
}

// Options configures the watch evaluator.
type Options struct {
	Store    *recstore.Store
	Registry ProviderRegistry
	Clock    func() time.Time
}

// Outcome is the structured per-evaluation result returned to the scheduler
// bridge. It mirrors the run row that is persisted.
type Outcome struct {
	WatchID           string
	WatchRunID        string
	Status            string // delivered|withheld|no_match|rate_limited|quiet_hours|provider_degraded|failed
	DeliveryDecision  string // sent|queue|summarize|drop|none
	Delivered         int
	Withheld          int
	RawCandidates     int
	WithheldReasons   map[string]int
	TraceID           string
	RecommendationIDs []string
	NotifyEnvelopes   []NotifyEnvelope
}

// NotifyEnvelope is the renderer-safe payload the Telegram bridge uses to
// deliver a watch alert. Only emitted when DeliveryDecision == "sent".
type NotifyEnvelope struct {
	WatchID         string
	WatchName       string
	ActorUserID     string
	DeliveryChannel string
	Title           string
	Subtitle        string
	Provider        string
	Why             string
	Labels          []string
}

// Evaluator runs the watch decision pipeline.
type Evaluator struct {
	store    *recstore.Store
	registry ProviderRegistry
	clock    func() time.Time
}

// NewEvaluator returns a configured watch evaluator.
func NewEvaluator(opts Options) *Evaluator {
	clock := opts.Clock
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	return &Evaluator{store: opts.Store, registry: opts.Registry, clock: clock}
}

// EvaluateWatch runs the full watch evaluation for a single watch_id and
// persists the resulting watch run + recommendation rows.
func (e *Evaluator) EvaluateWatch(ctx context.Context, watchID string, trigger TriggerContext) (Outcome, error) {
	if e == nil || e.store == nil {
		return Outcome{}, fmt.Errorf("watch evaluator: store required")
	}
	startedAt := e.clock().UTC()
	watch, err := e.store.GetWatch(ctx, watchID)
	if err != nil {
		return Outcome{}, err
	}
	if !watch.Enabled {
		return e.persistEmptyRun(ctx, watch, trigger, startedAt, "withheld", "watch_disabled", "drop", nil, nil)
	}
	if watch.SilenceUntil != nil && watch.SilenceUntil.After(startedAt) {
		return e.persistEmptyRun(ctx, watch, trigger, startedAt, "withheld", "watch_silenced", "drop", nil, nil)
	}

	// Tool-call audit log accumulates across the pipeline so we can persist a
	// faithful agent_traces.tool_calls snapshot.
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

	parseResult := map[string]any{
		"watch_id": watch.ID,
		"kind":     watch.Kind,
		"category": stringFromAny(watch.Filters["category"]),
		"keywords": sliceFromAny(watch.Filters["keywords"]),
		"trigger":  trigger.Kind,
	}
	appendTool("recommendation_parse_intent", "read", map[string]any{"watch_id": watch.ID}, parseResult)

	// Quiet hours guard runs before any provider work to avoid unnecessary
	// provider load and to match the design contract that quiet hours always
	// audit a queue/summarize/drop decision.
	if isInQuietHours(watch.QuietHours, startedAt) {
		decision := watch.QueuePolicy
		if decision == "" {
			decision = "drop"
		}
		appendTool("recommendation_apply_quality_guard", "read", map[string]any{"guard": "quiet_hours"}, map[string]any{"decision": decision, "status": "withheld"})
		return e.persistEmptyRun(ctx, watch, trigger, startedAt, "quiet_hours", "withheld:quiet-hours", decision, toolCalls, nil)
	}

	// Reduce location for any geo-aware watch kind. Trip-context and
	// topic_keyword kinds may use city precision when no geometry is present.
	precision := watch.LocationPrecision
	geometry := location.ReducedGeometry{Precision: recommendation.PrecisionPolicy(precision)}
	if locationRef := stringFromAny(watch.Scope["anchor"]); locationRef != "" {
		reducer := location.NewReducer(location.Config{NeighborhoodCellSystem: "geohash", NeighborhoodCellLevel: 6})
		reduced, err := reducer.Reduce(ctx, location.RawLocationRef{Ref: locationRef}, recommendation.PrecisionPolicy(precision))
		if err == nil {
			geometry = reduced
		}
	}
	appendTool("recommendation_reduce_location", "read", map[string]any{"precision": precision, "anchor_present": watch.Scope["anchor"] != nil}, map[string]any{"precision": string(geometry.Precision), "cell_id": geometry.CellID, "label": geometry.Label})

	// Build the watch-kind candidate set. This either calls providers (for
	// location/topic_keyword/price_drop) or operates on trigger context (for
	// trip_context). Each branch returns provider facts, candidates, and
	// optional recommendation rows that the rate/freshness/cooldown guards
	// then filter.
	candidates, providerFacts, providerStatus, deliverableSeed, err := e.gatherCandidates(ctx, watch, trigger)
	if err != nil {
		appendTool("recommendation_fetch_candidates", "external", map[string]any{"watch_id": watch.ID}, map[string]any{"error_kind": "provider_failed"})
		return e.persistEmptyRun(ctx, watch, trigger, startedAt, "failed", "provider_error", "drop", toolCalls, providerStatus)
	}
	appendTool("recommendation_fetch_candidates", "external", map[string]any{"watch_id": watch.ID, "kind": watch.Kind}, map[string]any{"provider_status": providerStatus, "raw_candidate_count": len(candidates)})

	if len(candidates) == 0 {
		return e.persistEmptyRun(ctx, watch, trigger, startedAt, "no_match", "no-candidates", "none", toolCalls, providerStatus)
	}
	appendTool("recommendation_dedupe_candidates", "read", map[string]any{"raw_candidate_count": len(candidates)}, map[string]any{"candidate_count": len(candidates)})

	// SCN-039-039: scope guard withholds candidates that fall outside the
	// declared watch scope (e.g. unrelated appliances for an espresso watch).
	scopeFiltered, scopeWithheldKeys := filterScope(watch, candidates)
	for _, key := range scopeWithheldKeys {
		appendTool("recommendation_apply_policy", "read", map[string]any{"candidate_canonical_key": key}, map[string]any{"decision": "withheld:out-of-scope"})
	}

	// SCN-039-035: stale source data freshness guard. Watches reject facts
	// older than freshness_seconds, persisting them as withheld.
	freshFiltered, staleWithheldKeys := filterFreshness(watch, scopeFiltered, startedAt)
	for _, key := range staleWithheldKeys {
		appendTool("recommendation_apply_quality_guard", "read", map[string]any{"candidate_canonical_key": key, "guard": "freshness"}, map[string]any{"decision": "withheld:stale-source-data"})
	}

	// SCN-039-036: repeat-cooldown guard. Compute a material change hash and
	// look up prior seen-state. Anything still inside its cooldown_until is
	// withheld with reason `withheld:repeat-cooldown`.
	cooldownFiltered, cooldownWithheldKeys, hashes, err := e.filterRepeatCooldown(ctx, watch, freshFiltered, startedAt)
	if err != nil {
		return Outcome{}, err
	}
	for _, key := range cooldownWithheldKeys {
		appendTool("recommendation_apply_quality_guard", "read", map[string]any{"candidate_canonical_key": key, "guard": "repeat_cooldown"}, map[string]any{"decision": "withheld:repeat-cooldown"})
	}

	// Price-drop kind only delivers candidates that crossed the threshold.
	priceFiltered, priceSkippedKeys := filterPriceDrop(watch, cooldownFiltered, deliverableSeed)
	for _, key := range priceSkippedKeys {
		appendTool("recommendation_apply_quality_guard", "read", map[string]any{"candidate_canonical_key": key, "guard": "price_drop"}, map[string]any{"decision": "withheld:no-threshold-crossing"})
	}

	// Safety/restricted-category policy guard runs AFTER price-drop so a
	// recalled or restricted candidate is never delivered as an ordinary
	// alert (SCN-039-041 / SCN-039-042 / BS-025 / BS-026). Withheld
	// candidates are persisted with `withheld:safety-policy` or
	// `restricted:<category>` so audit + UI can surface the reason.
	safetyFiltered, safetyWithheldKeys, restrictedWithheldKeys := filterSafetyAndRestricted(priceFiltered, providerFacts, nil)
	for _, key := range safetyWithheldKeys {
		appendTool("recommendation_apply_policy", "read", map[string]any{"candidate_canonical_key": key, "guard": "safety"}, map[string]any{"decision": "withheld:safety-policy"})
	}
	for _, key := range restrictedWithheldKeys {
		appendTool("recommendation_apply_policy", "read", map[string]any{"candidate_canonical_key": key, "guard": "restricted_category"}, map[string]any{"decision": "withheld:restricted-category"})
	}

	// Rate-limit guard. SCN-039-030/031 require that the rate window is
	// enforced both within a single evaluation AND across consecutive
	// evaluations of the same watch — once max_alerts_per_window has been
	// delivered inside the active window, additional candidates are
	// persisted as withheld:rate-limit until the window rolls over.
	maxAlerts := watch.MaxAlertsPerWindow
	if maxAlerts < 1 {
		maxAlerts = 1
	}
	windowSeconds := watch.AlertWindowSeconds
	if windowSeconds <= 0 {
		windowSeconds = 3600
	}
	windowStart := startedAt.Add(-time.Duration(windowSeconds) * time.Second)
	priorDelivered, err := e.store.CountDeliveredInRateWindow(ctx, watch.ID, windowStart)
	if err != nil {
		return Outcome{}, err
	}
	remaining := maxAlerts - priorDelivered
	if remaining < 0 {
		remaining = 0
	}
	deliveredCount := len(safetyFiltered)
	if deliveredCount > remaining {
		deliveredCount = remaining
	}
	delivered := safetyFiltered[:deliveredCount]
	rateWithheld := safetyFiltered[deliveredCount:]
	for range rateWithheld {
		appendTool("recommendation_apply_quality_guard", "read", map[string]any{"guard": "rate_limit"}, map[string]any{"decision": "withheld:rate-limit"})
	}

	// Build the recommendation input rows. Delivered ones get rank positions
	// and a delivery channel; withheld ones carry their reason.
	recommendations := buildRecommendationInputs(watch, delivered, rateWithheld, scopeWithheldKeys, staleWithheldKeys, cooldownWithheldKeys, priceSkippedKeys, safetyWithheldKeys, restrictedWithheldKeys, candidates)
	totalWithheld := len(recommendations) - len(delivered)

	// PersistWatchRun first to obtain run_id; then PersistWatchOutcome to
	// link recommendations to that run.
	runStatus := "delivered"
	if len(delivered) == 0 && totalWithheld == 0 {
		runStatus = "no_match"
	} else if len(delivered) == 0 {
		runStatus = "withheld"
	}
	deliveryDecision := "sent"
	if len(delivered) == 0 {
		deliveryDecision = "none"
	}

	runInput := recstore.WatchRunInput{
		WatchID:           watch.ID,
		ScenarioID:        ScenarioID,
		TraceID:           "",
		TriggerKind:       trigger.Kind,
		TriggerContext:    trigger.Context,
		Status:            runStatus,
		ProviderStatus:    providerStatus,
		RawCandidateCount: len(candidates),
		DeliveredCount:    len(delivered),
		WithheldCount:     totalWithheld,
		DeliveryDecision:  deliveryDecision,
		StartedAt:         startedAt,
		CompletedAt:       e.clock().UTC(),
	}
	runID, err := e.store.PersistWatchRun(ctx, runInput)
	if err != nil {
		return Outcome{}, err
	}

	appendTool("recommendation_persist_outcome", "write", map[string]any{"watch_run_id": runID}, map[string]any{"delivered": len(delivered), "withheld": totalWithheld, "decision": deliveryDecision})

	persistedIDs, err := e.store.PersistWatchOutcome(ctx, recstore.PersistWatchOutcomeInput{
		WatchID:          watch.ID,
		WatchRunID:       runID,
		ActorUserID:      watch.ActorUserID,
		ScenarioID:       ScenarioID,
		ScenarioVersion:  ScenarioVersion,
		ScenarioHash:     ScenarioVersion,
		StartedAt:        startedAt,
		CompletedAt:      e.clock().UTC(),
		Source:           "scheduler",
		Status:           runStatus,
		ToolCalls:        toolCalls,
		ProviderFacts:    providerFacts,
		Candidates:       uniqueCandidates(candidates),
		Recommendations:  recommendations,
		TriggerContext:   trigger.Context,
		DeliveryDecision: deliveryDecision,
	})
	if err != nil {
		return Outcome{}, err
	}

	// SCN-039-036: write seen-state for delivered candidates so the next run
	// suppresses them inside the configured cooldown window.
	for _, candidate := range delivered {
		hash := hashes[candidate.LocalID]
		var cooldownUntil *time.Time
		if watch.CooldownSeconds > 0 {
			until := e.clock().UTC().Add(time.Duration(watch.CooldownSeconds) * time.Second)
			cooldownUntil = &until
		}
		err := e.store.UpsertSeenState(ctx, recstore.SeenStateInput{
			ActorUserID:        watch.ActorUserID,
			ContextKey:         "watch:" + watch.ID,
			Category:           candidate.Category,
			CanonicalKey:       candidate.CanonicalKey,
			MaterialChangeHash: hash,
			CooldownUntil:      cooldownUntil,
			Now:                e.clock().UTC(),
		})
		if err != nil {
			return Outcome{}, err
		}
	}

	withheldReasons := map[string]int{}
	if len(rateWithheld) > 0 {
		withheldReasons["withheld:rate-limit"] = len(rateWithheld)
	}
	if len(staleWithheldKeys) > 0 {
		withheldReasons["withheld:stale-source-data"] = len(staleWithheldKeys)
	}
	if len(cooldownWithheldKeys) > 0 {
		withheldReasons["withheld:repeat-cooldown"] = len(cooldownWithheldKeys)
	}
	if len(scopeWithheldKeys) > 0 {
		withheldReasons["withheld:out-of-scope"] = len(scopeWithheldKeys)
	}
	if len(priceSkippedKeys) > 0 {
		withheldReasons["withheld:no-threshold-crossing"] = len(priceSkippedKeys)
	}

	if len(safetyWithheldKeys) > 0 {
		withheldReasons["withheld:safety-policy"] = len(safetyWithheldKeys)
	}
	if len(restrictedWithheldKeys) > 0 {
		withheldReasons["withheld:restricted-category"] = len(restrictedWithheldKeys)
	}
	envelopes := buildNotifyEnvelopes(watch, delivered)

	// SCN-039-050 / SCN-039-051: emit the bounded watch_runs metric labeled
	// only by watch kind and outcome. Per-watch counts are computed via the
	// `recommendation_watch_runs` join in the operator audit view.
	metrics.RecommendationWatchRuns.WithLabelValues(watch.Kind, runStatus).Inc()
	// SCN-039-050: per-reason suppression metric for withheld watch outcomes.
	for reason, count := range withheldReasons {
		if count <= 0 || strings.TrimSpace(reason) == "" {
			continue
		}
		metrics.RecommendationSuppression.WithLabelValues(reason).Add(float64(count))
	}
	// SCN-039-050: delivery metric for the watch's configured channel. We
	// emit one observation per envelope sent and one drop observation when
	// the run produced no deliverable envelopes.
	channel := strings.TrimSpace(watch.DeliveryChannel)
	if channel == "" {
		channel = "telegram"
	}
	if len(envelopes) > 0 {
		metrics.RecommendationDelivery.WithLabelValues(channel, "sent").Add(float64(len(envelopes)))
	} else if runStatus != "no_match" {
		metrics.RecommendationDelivery.WithLabelValues(channel, "drop").Inc()
	}

	return Outcome{
		WatchID:           watch.ID,
		WatchRunID:        runID,
		Status:            runStatus,
		DeliveryDecision:  deliveryDecision,
		Delivered:         len(delivered),
		Withheld:          totalWithheld,
		RawCandidates:     len(candidates),
		WithheldReasons:   withheldReasons,
		RecommendationIDs: persistedIDs,
		NotifyEnvelopes:   envelopes,
	}, nil
}

// TriggerContext is the structured input the scheduler bridges into a watch
// evaluation. Kind classifies why the watch is firing (e.g. `dwell`,
// `schedule`, `price_check`, `trip_window`).
type TriggerContext struct {
	Kind    string
	Context map[string]any
}

func (e *Evaluator) persistEmptyRun(
	ctx context.Context,
	watch recstore.WatchRecord,
	trigger TriggerContext,
	startedAt time.Time,
	status, reason, decision string,
	toolCalls []recstore.ToolCallRecord,
	providerStatus []map[string]any,
) (Outcome, error) {
	completedAt := e.clock().UTC()
	runID, err := e.store.PersistWatchRun(ctx, recstore.WatchRunInput{
		WatchID:           watch.ID,
		ScenarioID:        ScenarioID,
		TriggerKind:       trigger.Kind,
		TriggerContext:    trigger.Context,
		Status:            status,
		ProviderStatus:    providerStatus,
		RawCandidateCount: 0,
		DeliveredCount:    0,
		WithheldCount:     0,
		DeliveryDecision:  decision,
		ErrorKind:         reason,
		StartedAt:         startedAt,
		CompletedAt:       completedAt,
	})
	if err != nil {
		return Outcome{}, err
	}
	if len(toolCalls) > 0 {
		_, err = e.store.PersistWatchOutcome(ctx, recstore.PersistWatchOutcomeInput{
			WatchID:          watch.ID,
			WatchRunID:       runID,
			ActorUserID:      watch.ActorUserID,
			ScenarioID:       ScenarioID,
			ScenarioVersion:  ScenarioVersion,
			ScenarioHash:     ScenarioVersion,
			StartedAt:        startedAt,
			CompletedAt:      completedAt,
			Source:           "scheduler",
			Status:           status,
			ToolCalls:        toolCalls,
			TriggerContext:   trigger.Context,
			DeliveryDecision: decision,
		})
		if err != nil {
			return Outcome{}, err
		}
	}
	// SCN-039-050: count empty/short-circuit watch runs against the same
	// bounded `kind`/`outcome` watch metric so dashboards reflect total
	// scheduler activity, not just runs that reached the gather phase.
	metrics.RecommendationWatchRuns.WithLabelValues(watch.Kind, status).Inc()
	if reason := strings.TrimSpace(reason); reason != "" {
		metrics.RecommendationSuppression.WithLabelValues(reason).Inc()
	}
	return Outcome{
		WatchID:          watch.ID,
		WatchRunID:       runID,
		Status:           status,
		DeliveryDecision: decision,
	}, nil
}

// gatherCandidates produces the kind-specific candidate seed list. For
// price_drop, the deliverableSeed map indicates which candidates crossed the
// threshold (seeded by the trigger context's `current_prices` payload).
func (e *Evaluator) gatherCandidates(ctx context.Context, watch recstore.WatchRecord, trigger TriggerContext) (
	[]recstore.CandidateInput,
	[]recstore.ProviderFactInput,
	[]map[string]any,
	map[string]bool,
	error,
) {
	switch watch.Kind {
	case "trip_context":
		return e.gatherTripContextCandidates(watch, trigger)
	case "price_drop":
		return e.gatherPriceDropCandidates(watch, trigger)
	default:
		return e.gatherProviderCandidates(ctx, watch, trigger)
	}
}

func (e *Evaluator) gatherProviderCandidates(ctx context.Context, watch recstore.WatchRecord, trigger TriggerContext) (
	[]recstore.CandidateInput,
	[]recstore.ProviderFactInput,
	[]map[string]any,
	map[string]bool,
	error,
) {
	if e.registry == nil || e.registry.Len() == 0 {
		return nil, nil, []map[string]any{{"status": "no_providers"}}, nil, nil
	}
	allowed := map[string]struct{}{}
	for _, source := range watch.AllowedSources {
		allowed[source] = struct{}{}
	}
	queryString := stringFromAny(watch.Filters["query"])
	if queryString == "" {
		queryString = stringFromAny(watch.Scope["query"])
	}
	if queryString == "" {
		queryString = stringFromAny(watch.Filters["category"])
	}
	if keywords := sliceFromAny(watch.Filters["keywords"]); len(keywords) > 0 && queryString == "" {
		queryString = strings.Join(keywords, " ")
	}
	category := recommendation.Category(stringFromAny(watch.Filters["category"]))
	if category == "" {
		category = recommendation.CategoryPlace
	}
	geometry := location.ReducedGeometry{Precision: recommendation.PrecisionPolicy(watch.LocationPrecision)}
	limit := watch.MaxAlertsPerWindow * 5
	if limit < 5 {
		limit = 5
	}
	providerQuery := recprovider.ReducedQuery{
		Category:        category,
		Query:           queryString,
		PrecisionPolicy: recommendation.PrecisionPolicy(watch.LocationPrecision),
		Geometry:        geometry,
		Limit:           limit,
	}
	statuses := []map[string]any{}
	facts := []recstore.ProviderFactInput{}
	for _, providerEntry := range e.registry.List() {
		if len(allowed) > 0 {
			if _, ok := allowed[providerEntry.ID()]; !ok {
				continue
			}
		}
		bundle, err := providerEntry.Fetch(ctx, providerQuery)
		status := map[string]any{"provider_id": providerEntry.ID()}
		if err != nil {
			status["status"] = "degraded"
			status["error_kind"] = "provider_fetch_failed"
			statuses = append(statuses, status)
			continue
		}
		status["status"] = "healthy"
		status["fact_count"] = len(bundle.Facts)
		statuses = append(statuses, status)
		for i, fact := range bundle.Facts {
			facts = append(facts, recstore.ProviderFactInput{
				LocalID:             fmt.Sprintf("watch_fact_%s_%d", providerEntry.ID(), i),
				ProviderID:          fact.ProviderID,
				ProviderCandidateID: fact.ProviderCandidateID,
				Category:            string(fact.Category),
				Title:               fact.Title,
				NormalizedFact:      cloneAnyMap(fact.NormalizedFact),
				RetrievedAt:         fact.RetrievedAt,
				SourceUpdatedAt:     fact.SourceUpdatedAt,
				Attribution:         cloneAnyMap(fact.Attribution),
				SponsoredState:      fact.SponsoredState,
				RestrictedFlags:     cloneAnyMap(fact.RestrictedFlags),
			})
		}
	}
	candidates := groupCandidates(facts)
	return candidates, facts, statuses, nil, nil
}

func (e *Evaluator) gatherPriceDropCandidates(watch recstore.WatchRecord, trigger TriggerContext) (
	[]recstore.CandidateInput,
	[]recstore.ProviderFactInput,
	[]map[string]any,
	map[string]bool,
	error,
) {
	// Trigger context for price-drop:
	// {"products": [{"canonical_key", "title", "provider_id", "provider_candidate_id",
	//                "baseline_price", "current_price", "currency"}, ...],
	//  "threshold_pct": 0.15}
	products, _ := trigger.Context["products"].([]any)
	thresholdPct := numericFromAny(trigger.Context["threshold_pct"])
	if thresholdPct == 0 {
		thresholdPct = numericFromAny(watch.Filters["threshold_pct"])
	}
	if thresholdPct == 0 {
		thresholdPct = 0.10
	}
	now := e.clock().UTC()
	deliverable := map[string]bool{}
	facts := []recstore.ProviderFactInput{}
	for i, raw := range products {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		title := stringFromAny(entry["title"])
		canonicalKey := stringFromAny(entry["canonical_key"])
		if canonicalKey == "" {
			canonicalKey = canonicalKeyForTitle(title)
		}
		providerID := stringFromAny(entry["provider_id"])
		if providerID == "" {
			providerID = "fixture_price_provider"
		}
		baseline := numericFromAny(entry["baseline_price"])
		current := numericFromAny(entry["current_price"])
		drop := 0.0
		if baseline > 0 {
			drop = (baseline - current) / baseline
		}
		crossed := baseline > 0 && current > 0 && current < baseline && drop >= thresholdPct
		facts = append(facts, recstore.ProviderFactInput{
			LocalID:             fmt.Sprintf("watch_price_fact_%d", i),
			ProviderID:          providerID,
			ProviderCandidateID: stringFromAny(entry["provider_candidate_id"]),
			Category:            string(recommendation.CategoryProduct),
			Title:               title,
			NormalizedFact: map[string]any{
				"title":             title,
				"canonical_key":     canonicalKey,
				"baseline_price":    baseline,
				"current_price":     current,
				"currency":          stringFromAny(entry["currency"]),
				"drop_pct":          drop,
				"threshold_pct":     thresholdPct,
				"threshold_crossed": crossed,
			},
			RetrievedAt:     now,
			SourceUpdatedAt: &now,
			Attribution:     map[string]any{"label": providerID},
			SponsoredState:  "none",
			RestrictedFlags: restrictedFlagsFromAny(entry["restricted_flags"]),
		})
		if crossed {
			deliverable[canonicalKey] = true
		}
	}
	candidates := groupCandidates(facts)
	statuses := []map[string]any{{"status": "trigger_payload", "fact_count": len(facts)}}
	return candidates, facts, statuses, deliverable, nil
}

func (e *Evaluator) gatherTripContextCandidates(watch recstore.WatchRecord, trigger TriggerContext) (
	[]recstore.CandidateInput,
	[]recstore.ProviderFactInput,
	[]map[string]any,
	map[string]bool,
	error,
) {
	// Trigger context: {"trip_id", "trip_start", "candidates":[{"canonical_key","title","provider_id","category","source_updated_at"}]}
	rawCandidates, _ := trigger.Context["candidates"].([]any)
	now := e.clock().UTC()
	facts := []recstore.ProviderFactInput{}
	for i, raw := range rawCandidates {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		title := stringFromAny(entry["title"])
		canonicalKey := stringFromAny(entry["canonical_key"])
		if canonicalKey == "" {
			canonicalKey = canonicalKeyForTitle(title)
		}
		category := stringFromAny(entry["category"])
		if category == "" {
			category = string(recommendation.CategoryPlace)
		}
		providerID := stringFromAny(entry["provider_id"])
		if providerID == "" {
			providerID = "trip_context_provider"
		}
		// Honor a caller-supplied source_updated_at so freshness guards (Scope 4
		// SCN-039-035) can reject stale provider facts deterministically.
		sourceUpdatedAt := now
		if parsed, ok := timeFromAny(entry["source_updated_at"]); ok {
			sourceUpdatedAt = parsed
		}
		normalized := map[string]any{
			"title":             title,
			"canonical_key":     canonicalKey,
			"trip_id":           stringFromAny(trigger.Context["trip_id"]),
			"trip_start":        stringFromAny(trigger.Context["trip_start"]),
			"source_updated_at": sourceUpdatedAt.Format(time.RFC3339),
		}
		facts = append(facts, recstore.ProviderFactInput{
			LocalID:             fmt.Sprintf("watch_trip_fact_%d", i),
			ProviderID:          providerID,
			ProviderCandidateID: fmt.Sprintf("%s-%d", canonicalKey, i),
			Category:            category,
			Title:               title,
			NormalizedFact:      normalized,
			RetrievedAt:         now,
			SourceUpdatedAt:     &sourceUpdatedAt,
			Attribution:         map[string]any{"label": providerID},
			SponsoredState:      "none",
			RestrictedFlags:     map[string]any{},
		})
	}
	candidates := groupCandidates(facts)
	statuses := []map[string]any{{"status": "trip_payload", "fact_count": len(facts)}}
	return candidates, facts, statuses, nil, nil
}

func (e *Evaluator) filterRepeatCooldown(ctx context.Context, watch recstore.WatchRecord, candidates []recstore.CandidateInput, now time.Time) ([]recstore.CandidateInput, []string, map[string]string, error) {
	hashes := map[string]string{}
	withheldKeys := []string{}
	kept := []recstore.CandidateInput{}
	for _, candidate := range candidates {
		hash := materialChangeHash(candidate)
		hashes[candidate.LocalID] = hash
		seen, ok, err := e.store.GetSeenState(ctx, watch.ActorUserID, "watch:"+watch.ID, candidate.Category, candidate.CanonicalKey)
		if err != nil {
			return nil, nil, nil, err
		}
		if ok && seen.MaterialChangeHash == hash && seen.CooldownUntil != nil && seen.CooldownUntil.After(now) {
			withheldKeys = append(withheldKeys, candidate.CanonicalKey)
			continue
		}
		kept = append(kept, candidate)
	}
	return kept, withheldKeys, hashes, nil
}

func filterScope(watch recstore.WatchRecord, candidates []recstore.CandidateInput) ([]recstore.CandidateInput, []string) {
	keywords := lowerStrings(sliceFromAny(watch.Filters["keywords"]))
	exclude := lowerStrings(sliceFromAny(watch.Filters["exclude_keywords"]))
	if len(keywords) == 0 && len(exclude) == 0 {
		return candidates, nil
	}
	kept := []recstore.CandidateInput{}
	withheld := []string{}
	for _, candidate := range candidates {
		title := strings.ToLower(candidate.Title)
		if len(keywords) > 0 && !anyContained(title, keywords) {
			withheld = append(withheld, candidate.CanonicalKey)
			continue
		}
		if len(exclude) > 0 && anyContained(title, exclude) {
			withheld = append(withheld, candidate.CanonicalKey)
			continue
		}
		kept = append(kept, candidate)
	}
	return kept, withheld
}

func filterFreshness(watch recstore.WatchRecord, candidates []recstore.CandidateInput, now time.Time) ([]recstore.CandidateInput, []string) {
	freshness := time.Duration(watch.FreshnessSeconds) * time.Second
	if freshness <= 0 {
		freshness = 24 * time.Hour
	}
	kept := []recstore.CandidateInput{}
	withheld := []string{}
	for _, candidate := range candidates {
		if retrievedAt, ok := timeFromAny(candidate.CanonicalFact["source_retrieved_at"]); ok {
			if now.Sub(retrievedAt) > freshness {
				withheld = append(withheld, candidate.CanonicalKey)
				continue
			}
		}
		if updated, ok := timeFromAny(candidate.CanonicalFact["source_updated_at"]); ok {
			if now.Sub(updated) > freshness {
				withheld = append(withheld, candidate.CanonicalKey)
				continue
			}
		}
		kept = append(kept, candidate)
	}
	return kept, withheld
}

func filterPriceDrop(watch recstore.WatchRecord, candidates []recstore.CandidateInput, deliverable map[string]bool) ([]recstore.CandidateInput, []string) {
	if watch.Kind != "price_drop" {
		return candidates, nil
	}
	kept := []recstore.CandidateInput{}
	skipped := []string{}
	for _, candidate := range candidates {
		if deliverable != nil && deliverable[candidate.CanonicalKey] {
			kept = append(kept, candidate)
			continue
		}
		skipped = append(skipped, candidate.CanonicalKey)
	}
	return kept, skipped
}

// filterSafetyAndRestricted withholds candidates whose underlying provider
// facts carry a safety advisory (recall/safety_advisory) or a restricted
// category that intersects the user's restricted-category list. Returns the
// surviving candidates plus the canonical keys of safety- and
// restricted-withheld candidates so the caller can audit each reason.
//
// The restrictedCategories argument MAY be nil — in that case only the
// safety guard runs. The watch evaluator passes nil today because watch
// flows do not yet wire the SST `recommendations.policy.restricted_categories`
// list; the safety guard alone covers SCN-039-042 / BS-026.
func filterSafetyAndRestricted(candidates []recstore.CandidateInput, providerFacts []recstore.ProviderFactInput, restrictedCategories []string) ([]recstore.CandidateInput, []string, []string) {
	factsByLocalID := map[string]recstore.ProviderFactInput{}
	for _, fact := range providerFacts {
		factsByLocalID[fact.LocalID] = fact
	}
	kept := []recstore.CandidateInput{}
	safetyKeys := []string{}
	restrictedKeys := []string{}
	for _, candidate := range candidates {
		fact, hasFact := lookupCandidateFact(candidate, factsByLocalID)
		var restrictedFlags map[string]any
		if hasFact {
			restrictedFlags = fact.RestrictedFlags
		}
		if safetyDecision := policy.EvaluateSafety(restrictedFlags); safetyDecision.Outcome == "withhold" {
			safetyKeys = append(safetyKeys, candidate.CanonicalKey)
			continue
		}
		if restrictedDecision := policy.EvaluateRestricted(restrictedFlags, restrictedCategories); restrictedDecision.Outcome == "withhold" {
			restrictedKeys = append(restrictedKeys, candidate.CanonicalKey)
			continue
		}
		kept = append(kept, candidate)
	}
	return kept, safetyKeys, restrictedKeys
}

// lookupCandidateFact returns the first provider fact referenced by the
// candidate; returns false when the candidate has no associated facts.
func lookupCandidateFact(candidate recstore.CandidateInput, byLocalID map[string]recstore.ProviderFactInput) (recstore.ProviderFactInput, bool) {
	for _, factID := range candidate.ProviderFactLocalIDs {
		if fact, ok := byLocalID[factID]; ok {
			return fact, true
		}
	}
	return recstore.ProviderFactInput{}, false
}

func buildRecommendationInputs(watch recstore.WatchRecord, delivered, rateWithheld []recstore.CandidateInput, scopeWithheld, staleWithheld, cooldownWithheld, priceSkipped, safetyWithheld, restrictedWithheld []string, allCandidates []recstore.CandidateInput) []recstore.RecommendationInput {
	keyToCandidate := map[string]recstore.CandidateInput{}
	for _, candidate := range allCandidates {
		keyToCandidate[candidate.CanonicalKey] = candidate
	}
	rationaleFor := func(candidate recstore.CandidateInput) []string {
		return []string{
			fmt.Sprintf("Watch %s matched %s", watch.Name, candidate.Title),
		}
	}
	out := []recstore.RecommendationInput{}
	for i, candidate := range delivered {
		out = append(out, recstore.RecommendationInput{
			CandidateLocalID: candidate.LocalID,
			RankPosition:     i + 1,
			Status:           "delivered",
			StatusReason:     "watch:eligible",
			ScoreBreakdown:   map[string]float64{"watch_match": 1.0},
			Rationale:        rationaleFor(candidate),
			GraphSignalRefs:  []string{},
			PolicyDecisions:  []map[string]any{{"kind": "consent", "outcome": "allow", "reason": "watch-consent-current"}},
			QualityDecisions: []map[string]any{{"kind": "rate_limit", "outcome": "allow", "reason": "within-window"}},
			DeliveryChannel:  watch.DeliveryChannel,
		})
	}
	for _, candidate := range rateWithheld {
		out = append(out, recstore.RecommendationInput{
			CandidateLocalID: candidate.LocalID,
			Status:           "withheld",
			StatusReason:     "withheld:rate-limit",
			ScoreBreakdown:   map[string]float64{"watch_match": 1.0},
			Rationale:        []string{"Withheld: rate limit reached for this window"},
			GraphSignalRefs:  []string{},
			PolicyDecisions:  []map[string]any{},
			QualityDecisions: []map[string]any{{"kind": "rate_limit", "outcome": "withheld", "reason": "rate-limit"}},
			DeliveryChannel:  "",
		})
	}
	addWithheldByKey := func(reason string, qualityKind string, keys []string) {
		for _, key := range keys {
			candidate, ok := keyToCandidate[key]
			if !ok {
				continue
			}
			out = append(out, recstore.RecommendationInput{
				CandidateLocalID: candidate.LocalID,
				Status:           "withheld",
				StatusReason:     reason,
				ScoreBreakdown:   map[string]float64{"watch_match": 0.0},
				Rationale:        []string{"Withheld: " + reason},
				GraphSignalRefs:  []string{},
				PolicyDecisions:  []map[string]any{},
				QualityDecisions: []map[string]any{{"kind": qualityKind, "outcome": "withheld", "reason": reason}},
				DeliveryChannel:  "",
			})
		}
	}
	addWithheldByKey("withheld:out-of-scope", "scope", scopeWithheld)
	addWithheldByKey("withheld:stale-source-data", "freshness", staleWithheld)
	addWithheldByKey("withheld:repeat-cooldown", "repeat_cooldown", cooldownWithheld)
	addWithheldByKey("withheld:no-threshold-crossing", "price_drop", priceSkipped)
	addWithheldByKey("withheld:safety-policy", "safety", safetyWithheld)
	addWithheldByKey("withheld:restricted-category", "restricted_category", restrictedWithheld)
	return out
}

func uniqueCandidates(candidates []recstore.CandidateInput) []recstore.CandidateInput {
	seen := map[string]struct{}{}
	out := make([]recstore.CandidateInput, 0, len(candidates))
	for _, candidate := range candidates {
		if _, ok := seen[candidate.LocalID]; ok {
			continue
		}
		seen[candidate.LocalID] = struct{}{}
		out = append(out, candidate)
	}
	return out
}

func buildNotifyEnvelopes(watch recstore.WatchRecord, delivered []recstore.CandidateInput) []NotifyEnvelope {
	if watch.DeliveryChannel != "telegram" {
		return nil
	}
	envelopes := make([]NotifyEnvelope, 0, len(delivered))
	for _, candidate := range delivered {
		providers := sliceFromAny(candidate.CanonicalFact["provider_ids"])
		envelopes = append(envelopes, NotifyEnvelope{
			WatchID:         watch.ID,
			WatchName:       watch.Name,
			ActorUserID:     watch.ActorUserID,
			DeliveryChannel: "telegram",
			Title:           candidate.Title,
			Subtitle:        stringFromAny(candidate.CanonicalFact["currency"]),
			Provider:        strings.Join(providers, " + "),
			Why:             fmt.Sprintf("Matches saved watch %s", watch.Name),
			Labels:          watchLabels(candidate),
		})
	}
	return envelopes
}

func watchLabels(candidate recstore.CandidateInput) []string {
	labels := []string{}
	if crossed, _ := candidate.CanonicalFact["threshold_crossed"].(bool); crossed {
		baseline := numericFromAny(candidate.CanonicalFact["baseline_price"])
		current := numericFromAny(candidate.CanonicalFact["current_price"])
		if baseline > 0 && current > 0 {
			labels = append(labels, fmt.Sprintf("Price drop %.0f → %.0f", baseline, current))
		} else {
			labels = append(labels, "Price drop")
		}
	}
	return labels
}

func materialChangeHash(candidate recstore.CandidateInput) string {
	payload := map[string]any{
		"canonical_key": candidate.CanonicalKey,
		"title":         candidate.Title,
		"baseline":      candidate.CanonicalFact["baseline_price"],
		"current":       candidate.CanonicalFact["current_price"],
		"crossed":       candidate.CanonicalFact["threshold_crossed"],
		"updated":       candidate.CanonicalFact["source_updated_at"],
		"providers":     candidate.CanonicalFact["provider_ids"],
	}
	data, _ := json.Marshal(payload)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func canonicalKeyForTitle(title string) string {
	value := strings.ToLower(strings.TrimSpace(title))
	value = strings.NewReplacer(" ", "-", "'", "", "&", "and").Replace(value)
	return value
}

func cloneAnyMap(values map[string]any) map[string]any {
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

func sliceFromAny(value any) []string {
	switch typed := value.(type) {
	case []string:
		out := append([]string(nil), typed...)
		sort.Strings(out)
		return out
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		sort.Strings(out)
		return out
	}
	return nil
}

func numericFromAny(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	}
	return 0
}

// restrictedFlagsFromAny normalises a trigger.Context entry's
// `restricted_flags` field into a `map[string]any` so the policy guard can
// inspect it without a type assertion at the call site. Returns an empty
// map when the value is missing or not a map.
func restrictedFlagsFromAny(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for k, v := range typed {
			out[k] = v
		}
		return out
	case map[string]string:
		out := make(map[string]any, len(typed))
		for k, v := range typed {
			out[k] = v
		}
		return out
	}
	return map[string]any{}
}

func timeFromAny(value any) (time.Time, bool) {
	switch typed := value.(type) {
	case time.Time:
		return typed.UTC(), true
	case *time.Time:
		if typed == nil {
			return time.Time{}, false
		}
		return typed.UTC(), true
	case string:
		if typed == "" {
			return time.Time{}, false
		}
		t, err := time.Parse(time.RFC3339, typed)
		if err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

func lowerStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func anyContained(haystack string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(haystack, needle) {
			return true
		}
	}
	return false
}

func isInQuietHours(quiet map[string]any, now time.Time) bool {
	if quiet == nil {
		return false
	}
	// Honor an explicit enabled=false override; otherwise the presence of a
	// start+end pair is sufficient. The watch input persists quiet hours as
	// {"start":"22:00","end":"07:00","timezone":"UTC"} without a separate
	// enabled flag.
	if raw, ok := quiet["enabled"]; ok {
		if enabled, ok := raw.(bool); ok && !enabled {
			return false
		}
	}
	start, _ := quiet["start"].(string)
	end, _ := quiet["end"].(string)
	if start == "" || end == "" {
		return false
	}
	startMinutes, ok := parseClock(start)
	if !ok {
		return false
	}
	endMinutes, ok := parseClock(end)
	if !ok {
		return false
	}
	loc := time.UTC
	if tz, ok := quiet["timezone"].(string); ok && tz != "" {
		if zone, err := time.LoadLocation(tz); err == nil {
			loc = zone
		}
	}
	local := now.In(loc)
	current := local.Hour()*60 + local.Minute()
	if startMinutes < endMinutes {
		return current >= startMinutes && current < endMinutes
	}
	// Overnight window (e.g. 22:00 → 07:00)
	return current >= startMinutes || current < endMinutes
}

func parseClock(value string) (int, bool) {
	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return 0, false
	}
	var hour, minute int
	_, err := fmt.Sscanf(parts[0], "%d", &hour)
	if err != nil {
		return 0, false
	}
	_, err = fmt.Sscanf(parts[1], "%d", &minute)
	if err != nil {
		return 0, false
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, false
	}
	return hour*60 + minute, true
}

// groupCandidates is identical in shape to the reactive engine's grouper but
// duplicated here so the watch package does not import the reactive engine.
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
			localID := "watch_cand_" + key
			groups[key] = &group{candidate: recstore.CandidateInput{
				LocalID:       localID,
				Category:      fact.Category,
				CanonicalKey:  key,
				Title:         fact.Title,
				CanonicalURL:  stringFromAny(fact.NormalizedFact["canonical_url"]),
				CanonicalFact: cloneAnyMap(fact.NormalizedFact),
				DedupeReason:  map[string]any{"strategy": "canonical_key"},
				MergeReason:   "watch-canonical-key",
			}}
			order = append(order, key)
		}
		groups[key].facts = append(groups[key].facts, fact)
		groups[key].candidate.ProviderFactLocalIDs = append(groups[key].candidate.ProviderFactLocalIDs, fact.LocalID)
		// Preserve provider_ids for rendering and threshold/material-hash tracking.
		providers, _ := groups[key].candidate.CanonicalFact["provider_ids"].([]string)
		providers = append(providers, fact.ProviderID)
		groups[key].candidate.CanonicalFact["provider_ids"] = providers
	}
	out := make([]recstore.CandidateInput, 0, len(order))
	for _, key := range order {
		out = append(out, groups[key].candidate)
	}
	return out
}

func canonicalKeyFromFact(fact recstore.ProviderFactInput) string {
	if key := stringFromAny(fact.NormalizedFact["canonical_key"]); key != "" {
		return key
	}
	return canonicalKeyForTitle(fact.Title)
}
