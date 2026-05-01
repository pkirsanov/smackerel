// Spec 039 Scope 6 recommendation metrics.
//
// All eight smackerel_recommendation_* metrics required by spec 039
// design.md "Observability and Failure Handling" → "Metrics" live here.
// Every metric uses bounded labels only — no watch_id, no recommendation_id,
// no request_id, no user id. Per-watch operational visibility is satisfied
// by joining the bounded `kind`/`outcome` watch metric with the persisted
// `recommendation_watch_runs` table on `watch_id` (see SCN-039-051 audit
// view).
package metrics

import "github.com/prometheus/client_golang/prometheus"

// RecommendationProviderRequests counts read-only recommendation provider
// calls by provider id, category, and outcome (success|degraded|error|
// quota|timeout). Provider id and category are bounded by config; outcome
// is a closed enum.
var RecommendationProviderRequests = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_recommendation_provider_requests_total",
		Help: "Recommendation provider call count by provider, category, and outcome (spec 039 Scope 6)",
	},
	[]string{"provider", "category", "outcome"},
)

// RecommendationProviderLatency records read-only recommendation provider
// call latency in seconds by provider id and category. Buckets cover the
// reactive p95 NFR (≤2s warm) and slow-path provider behavior up to 30s.
var RecommendationProviderLatency = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "smackerel_recommendation_provider_latency_seconds",
		Help:    "Recommendation provider call latency by provider and category (spec 039 Scope 6)",
		Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30},
	},
	[]string{"provider", "category"},
)

// RecommendationCandidates counts candidate volume by category, stage
// (raw|deduped|ranked|delivered|withheld|suppressed), and outcome
// (count|drop). Used for funnel analysis without exposing per-request ids.
var RecommendationCandidates = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_recommendation_candidates_total",
		Help: "Recommendation candidate funnel counts by category, stage, and outcome (spec 039 Scope 6)",
	},
	[]string{"category", "stage", "outcome"},
)

// RecommendationWatchRuns counts watch evaluator runs by watch kind and
// outcome (delivered|withheld|no_match|rate_limited|quiet_hours|
// provider_degraded|failed). Per-watch counts are obtained by joining
// `recommendation_watch_runs` on `watch_id` — never as a label.
var RecommendationWatchRuns = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_recommendation_watch_runs_total",
		Help: "Recommendation watch evaluator runs by kind and outcome (spec 039 Scope 6)",
	},
	[]string{"kind", "outcome"},
)

// RecommendationDelivery counts delivered/queued/dropped recommendation
// envelopes by delivery channel and outcome. `channel` is a closed enum
// (telegram|web|api|digest|trip_dossier); `outcome` is sent|queued|drop|
// summarized|failed.
var RecommendationDelivery = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_recommendation_delivery_total",
		Help: "Recommendation delivery outcomes by channel and outcome (spec 039 Scope 6)",
	},
	[]string{"channel", "outcome"},
)

// RecommendationSuppression counts withheld/suppressed candidates by
// bounded reason label (e.g. withheld:rate-limit, withheld:quiet-hours,
// withheld:safety-policy, withheld:repeat-cooldown, suppressed:disliked).
// New reasons must remain a closed enum maintained alongside the policy
// and quality guards.
var RecommendationSuppression = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_recommendation_suppression_total",
		Help: "Recommendation suppression and withholding counts by bounded reason (spec 039 Scope 6)",
	},
	[]string{"reason"},
)

// RecommendationRankingConfidence counts ranked recommendations by
// confidence band (high|medium|low) — the closed enum from the ranker.
// Used to detect drift toward low-confidence outcomes that the UI must
// label.
var RecommendationRankingConfidence = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_recommendation_ranking_confidence_total",
		Help: "Recommendation ranking confidence band distribution (spec 039 Scope 6)",
	},
	[]string{"confidence"},
)

// RecommendationLocationPrecision counts location precision reductions by
// requested precision (the policy a caller asked for) and sent precision
// (the precision actually transmitted to the provider). Used to audit
// whether precision policy is being honored without recording any actual
// coordinates.
var RecommendationLocationPrecision = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_recommendation_location_precision_total",
		Help: "Recommendation location precision audit (requested vs. sent) (spec 039 Scope 6)",
	},
	[]string{"requested", "sent"},
)
