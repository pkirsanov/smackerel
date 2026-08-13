// Spec 044 Scope 04 — per-user bearer auth metrics surface.
//
// Metric names follow the `smackerel_auth_*` family declared in
// `auth.telemetry_metric_prefix` (config/smackerel.yaml). Label
// cardinality is bounded — `source`, `result`, `actor_kind`,
// `environment`, and `reason` each take values from a closed set so
// scrape volume is predictable.
//
// Emission sites are the auth lifecycle pivots:
//   - `AuthIssuance`  — incremented from `internal/auth/issue.go`
//     and `internal/telegram/per_user_token.go`
//     (the Telegram per-user PASETO bridge)
//   - `AuthRotation` — incremented from
//     `internal/auth/bearer_store.go::MarkTokenRotated`
//   - `AuthRevocation` — incremented from
//     `internal/auth/bearer_store.go::RevokeToken`
//   - `AuthValidationLatency` + `AuthValidationOutcome` — recorded
//     from `internal/api/router.go::bearerAuthMiddleware`
//     around the `auth.VerifyAndParse` + revocation
//     check pair
//   - `AuthLegacyFallbackUsed` — incremented from
//     `internal/api/router.go::bearerAuthMiddleware`
//     Branch 2 (production opt-in shared-token
//     fallback) so operators can monitor the
//     deprecation pathway
//   - `AuthFailure` — incremented alongside every middleware 401
//     response
//   - `AuthCorpusGrantWouldDeny` + `AuthCorpusGrantAllowed` —
//     recorded from `internal/api::CorpusGrantGate.Observe`
//     (spec 108 Scope 02) on every corpus route group
//   - `AuthCorpusGrantEnforcementMode` — set once from the
//     `cmd/core` startup resolution point
package metrics

import (
	"errors"
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
)

// AuthIssuance counts spec 044 token mints by source. The label is
// closed-set so cardinality is bounded; new sources MUST extend the
// allowed values list documented inline below.
//
// Allowed `source` values (closed set):
//   - "admin_api"        — POST /v1/auth/users (enrollment) +
//     POST /v1/auth/users/{id}/rotate (rotation)
//   - "bootstrap_cli"    — `./smackerel.sh auth bootstrap`
//   - "telegram_bridge"  — `internal/telegram/per_user_token.go`
//     mint per inbound Telegram message
var AuthIssuance = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_auth_issuance_total",
		Help: "Per-user bearer-auth token mints by issuance source",
	},
	[]string{"source"},
)

// AuthRotation counts the number of successful prior-token rotations.
// A rotation is the same operation as a mint plus a flip of the
// previous token to status=`rotated`; every increment here is paired
// with an `AuthIssuance{source="admin_api"}` increment.
var AuthRotation = prometheus.NewCounter(
	prometheus.CounterOpts{
		Name: "smackerel_auth_rotation_total",
		Help: "Per-user bearer-auth token rotations (prior token marked rotated)",
	},
)

// AuthRevocation counts the number of successful revocations by
// reason. The `reason` label is the operator-supplied free-text
// reason, normalized to a closed-set bucket via NormalizeRevocationReason
// to keep label cardinality bounded.
var AuthRevocation = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_auth_revocation_total",
		Help: "Per-user bearer-auth token revocations by normalized reason",
	},
	[]string{"reason"},
)

// AuthValidationLatency records the wall-clock time spent in the
// hot-path verifier (PASETO signature verify + claim parse +
// revocation cache lookup) for the per-user bearer-auth path. It does
// NOT include network or chi-router overhead; the histogram measures
// pure middleware work so dashboards can size NFR-AUTH-001 (≤5ms p99)
// directly from this series.
var AuthValidationLatency = prometheus.NewHistogram(
	prometheus.HistogramOpts{
		Name:    "smackerel_auth_validation_latency_seconds",
		Help:    "Per-user bearer-auth validation hot-path latency (PASETO verify + revocation cache lookup)",
		Buckets: []float64{0.0001, 0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1},
	},
)

// AuthValidationOutcome counts hot-path validation outcomes by result
// and source. The label combinations are bounded; both labels take
// values from a closed set documented below.
//
// Allowed `result` values (closed set):
//   - "accepted"             — token verified + not revoked
//   - "rejected_revoked"     — token verified + present in revocation cache
//   - "rejected_expired"     — token expired (or not-yet-valid)
//   - "rejected_malformed"   — wire token failed signature/parse
//   - "rejected_unknown_key" — kid not in active or prior rotation
//
// Allowed `source` values (closed set):
//   - "header"        — Authorization: Bearer <token>
//   - "pwa_cookie"    — auth_token cookie fallback
var AuthValidationOutcome = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_auth_validation_outcome_total",
		Help: "Per-user bearer-auth validation outcomes by result and bearer source",
	},
	[]string{"result", "source"},
)

// AuthLegacyFallbackUsed counts the number of times the production
// shared-token fallback (`auth.production_shared_token_fallback_enabled`)
// admitted a request. Operators monitor this counter during the
// migration window — the goal is to reach zero before flipping the
// flag to `false`. The `environment` label is always `"production"`
// at the emission site (the fallback only fires in production), but
// is kept as a label so dashboards can dedupe across multiple
// deployments scraped into a single Prometheus instance.
var AuthLegacyFallbackUsed = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_auth_legacy_fallback_used_total",
		Help: "Production shared-token fallback admissions (deprecation-pathway monitor)",
	},
	[]string{"environment"},
)

// AuthFailure counts every 401 response emitted by
// `bearerAuthMiddleware`. The `reason` label takes values from a
// closed set so dashboards can group failures by class without
// cardinality blow-up.
//
// Allowed `reason` values (closed set):
//   - "missing_token"          — no Authorization header AND no cookie
//   - "invalid_format"         — header present but not "Bearer <token>"
//   - "paseto_verify_failed"   — production per-user verify failed
//   - "revoked"                — token verified but revoked
//   - "shared_token_mismatch"  — dev/test or fallback compare failed
//   - "auth_not_configured"    — production with no auth configured
var AuthFailure = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_auth_failure_total",
		Help: "Per-user bearer-auth 401 emissions by failure reason",
	},
	[]string{"reason"},
)

// AuthScopeRejected counts 403 emissions from `auth.RequireScope`
// middleware (spec 060). Labels:
//
//   - `required_scope` — the FIRST missing required scope (the same
//     value echoed in the 403 response body's `required` field).
//   - `user_id`        — the session's UserID; empty for non-per-user
//     sources (which should never reach the reject branch because
//     shared-token / bootstrap bypass the middleware).
//
// Label cardinality is bounded by the operator-controlled scope
// registry and the user count; spec 060 BS-002 + BS-003 cover the
// rejection paths.
var AuthScopeRejected = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_auth_scope_rejected_total",
		Help: "auth.RequireScope rejections by first-missing required scope and user_id",
	},
	[]string{"required_scope", "user_id"},
)

// AuthScopeCheckBypassed counts `auth.RequireScope` pass-throughs
// for non-per-user session sources (spec 060). Allowed `source`
// values (closed set):
//
//   - "shared_token" — SessionSourceSharedToken (dev/test ergonomic
//     and the production opt-in fallback)
//   - "bootstrap"    — SessionSourceBootstrap (one-shot first-user
//     enrollment)
//
// Operators monitor this counter to confirm that production traffic
// does NOT exercise the bypass paths once a deployment has fully
// migrated off legacy sources.
var AuthScopeCheckBypassed = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_auth_scope_check_bypassed_total",
		Help: "auth.RequireScope pass-throughs for non-per-user session sources",
	},
	[]string{"source"},
)

// ── Spec 108 Scope 02 — corpus-grant observe-stage telemetry ────────────────
//
// These three series extend the `smackerel_auth_*` family declared above; they
// are deliberately NOT a parallel family and they deliberately do NOT reuse
// `AuthScopeRejected` for the observe signal (R-108-O2). `AuthScopeRejected`
// keeps its spec-060 meaning — a real 403 emitted under ENFORCE — so an
// operator can tell an actual denial apart from a counterfactual one.

// CorpusRouteGroup is the closed-set `route_group` label type for the
// corpus-grant series. It is a named type so a raw request path cannot reach a
// label by accident: every emitter takes a CorpusRouteGroup, and the recorders
// below re-validate it against the canonical set because a Go string
// conversion can still forge one.
type CorpusRouteGroup string

// The closed SIXTEEN-value `route_group` set (spec.md §4.2). Tier A (1–8) is
// raw corpus retrieval; Tier B (9–16) is corpus-derived Phase-5 intelligence,
// brought in scope by spec.md §18 decision 5. The tier split is documentation,
// not a difference in authority — both carry `corpus:read`.
//
// Raw request paths are NEVER label values (R-108-O3/O4): `/api/artifact/{id}`
// alone would be per-artifact and therefore unbounded.
const (
	// Tier A — raw corpus retrieval.
	CorpusRouteGroupSearch         CorpusRouteGroup = "search"
	CorpusRouteGroupDigest         CorpusRouteGroup = "digest"
	CorpusRouteGroupRecent         CorpusRouteGroup = "recent"
	CorpusRouteGroupArtifactDetail CorpusRouteGroup = "artifact_detail"
	CorpusRouteGroupArtifactDomain CorpusRouteGroup = "artifact_domain"
	CorpusRouteGroupExport         CorpusRouteGroup = "export"
	CorpusRouteGroupContextFor     CorpusRouteGroup = "context_for"
	CorpusRouteGroupKnowledge      CorpusRouteGroup = "knowledge"

	// Tier B — corpus-derived Phase-5 intelligence.
	CorpusRouteGroupExpertise        CorpusRouteGroup = "expertise"
	CorpusRouteGroupLearningPaths    CorpusRouteGroup = "learning_paths"
	CorpusRouteGroupSubscriptions    CorpusRouteGroup = "subscriptions"
	CorpusRouteGroupSerendipity      CorpusRouteGroup = "serendipity"
	CorpusRouteGroupContentFuel      CorpusRouteGroup = "content_fuel"
	CorpusRouteGroupQuickReferences  CorpusRouteGroup = "quick_references"
	CorpusRouteGroupMonthlyReport    CorpusRouteGroup = "monthly_report"
	CorpusRouteGroupSeasonalPatterns CorpusRouteGroup = "seasonal_patterns"
)

// corpusRouteGroups is the canonical ordered set, Tier A then Tier B.
var corpusRouteGroups = []CorpusRouteGroup{
	CorpusRouteGroupSearch,
	CorpusRouteGroupDigest,
	CorpusRouteGroupRecent,
	CorpusRouteGroupArtifactDetail,
	CorpusRouteGroupArtifactDomain,
	CorpusRouteGroupExport,
	CorpusRouteGroupContextFor,
	CorpusRouteGroupKnowledge,
	CorpusRouteGroupExpertise,
	CorpusRouteGroupLearningPaths,
	CorpusRouteGroupSubscriptions,
	CorpusRouteGroupSerendipity,
	CorpusRouteGroupContentFuel,
	CorpusRouteGroupQuickReferences,
	CorpusRouteGroupMonthlyReport,
	CorpusRouteGroupSeasonalPatterns,
}

// corpusRouteGroupSet is the membership index for ValidateCorpusRouteGroup.
var corpusRouteGroupSet = func() map[CorpusRouteGroup]struct{} {
	set := make(map[CorpusRouteGroup]struct{}, len(corpusRouteGroups))
	for _, g := range corpusRouteGroups {
		set[g] = struct{}{}
	}
	return set
}()

// ErrUnknownCorpusRouteGroup is returned when a value outside the closed
// sixteen-value set reaches a corpus-grant emitter. Callers MUST treat it as a
// wiring defect: nothing is emitted, so an unbounded label (a raw path, a
// caller-supplied string) can never become a series.
var ErrUnknownCorpusRouteGroup = errors.New("metrics: route_group is not one of the sixteen closed-set corpus route groups")

// CorpusRouteGroups returns a copy of the canonical closed set in Tier A then
// Tier B order. Callers that mount the gate iterate this rather than
// re-declaring the list, so a seventeenth group cannot be added in one place
// and forgotten in another.
func CorpusRouteGroups() []CorpusRouteGroup {
	out := make([]CorpusRouteGroup, len(corpusRouteGroups))
	copy(out, corpusRouteGroups)
	return out
}

// ValidateCorpusRouteGroup reports whether group is inside the closed set.
// This is the single membership authority; no emitter may bypass it.
func ValidateCorpusRouteGroup(group CorpusRouteGroup) error {
	if _, ok := corpusRouteGroupSet[group]; !ok {
		return fmt.Errorf("%w: %q", ErrUnknownCorpusRouteGroup, string(group))
	}
	return nil
}

// AuthCorpusGrantWouldDeny counts requests that WOULD have been denied under
// ENFORCE but were allowed under OBSERVE (design.md §4). The `user_id` label
// follows the existing `AuthScopeRejected` precedent above and is what makes
// UC-108-001 answerable:
//
//	sum by (user_id, route_group) (increase(smackerel_auth_corpus_grant_would_deny_total[7d]))
//
// Cardinality: `route_group` is the closed sixteen-value set, `user_id` is
// bounded by the operator-controlled principal count, and `session_source` is
// the existing closed session-source enum.
var AuthCorpusGrantWouldDeny = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_auth_corpus_grant_would_deny_total",
		Help: "Corpus reads that would be denied under ENFORCE but were allowed under OBSERVE, by route group, principal, and session source",
	},
	[]string{"route_group", "user_id", "session_source"},
)

// AuthCorpusGrantAllowed counts requests that carried `corpus:read`. It is the
// denominator for the would-deny numerator, and it carries `user_id` so that a
// GRANTED principal's traffic is visible too.
//
// Without `user_id` here, only denied principals were observable, so a
// principal that holds the grant and uses it looked identical to one that never
// called at all — which is precisely why the §18 decision 1(b) coverage bar was
// not computable and fell back to per-cell operator attestation
// (F-108-COVERAGE-LABEL-01). With this label, a cell is closed by observed
// traffic of EITHER outcome:
//
//	sum by (user_id, route_group) (
//	  increase(smackerel_auth_corpus_grant_allowed_total[14d])
//	  or increase(smackerel_auth_corpus_grant_would_deny_total[14d])
//	)
//
// Cardinality is unchanged in kind: `route_group` is the closed sixteen-value
// set and `user_id` follows the `AuthScopeRejected` precedent above.
var AuthCorpusGrantAllowed = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_auth_corpus_grant_allowed_total",
		Help: "Corpus reads that carried the corpus:read grant, by route group, principal, and session source",
	},
	[]string{"route_group", "user_id", "session_source"},
)

// AuthCorpusGrantEnforcementMode reports the resolved stage: 0 = OBSERVE,
// 1 = ENFORCE. A dashboard reads the stage from here instead of reading config.
//
// An unset gauge reads 0, which is indistinguishable from OBSERVE. That is why
// SetCorpusGrantEnforcementMode is called from the startup resolution point in
// `cmd/core`, immediately after the fail-loud resolve and before any listener
// binds: a process that could not resolve the stage never starts, so a
// serving process has always set this explicitly.
var AuthCorpusGrantEnforcementMode = prometheus.NewGauge(
	prometheus.GaugeOpts{
		Name: "smackerel_auth_corpus_grant_enforcement_mode",
		Help: "Resolved corpus-grant enforcement stage (0 = OBSERVE, 1 = ENFORCE)",
	},
)

// RecordCorpusGrantWouldDeny increments the would-deny counter. It emits
// nothing and returns ErrUnknownCorpusRouteGroup when group is outside the
// closed set, so an unbounded label value cannot create a series.
func RecordCorpusGrantWouldDeny(group CorpusRouteGroup, userID, sessionSource string) error {
	if err := ValidateCorpusRouteGroup(group); err != nil {
		return err
	}
	AuthCorpusGrantWouldDeny.WithLabelValues(string(group), userID, sessionSource).Inc()
	return nil
}

// RecordCorpusGrantAllowed increments the allowed counter under the same
// closed-set guarantee as RecordCorpusGrantWouldDeny.
func RecordCorpusGrantAllowed(group CorpusRouteGroup, userID, sessionSource string) error {
	if err := ValidateCorpusRouteGroup(group); err != nil {
		return err
	}
	AuthCorpusGrantAllowed.WithLabelValues(string(group), userID, sessionSource).Inc()
	return nil
}

// SetCorpusGrantEnforcementMode publishes the resolved stage gauge.
func SetCorpusGrantEnforcementMode(enforce bool) {
	if enforce {
		AuthCorpusGrantEnforcementMode.Set(1)
		return
	}
	AuthCorpusGrantEnforcementMode.Set(0)
}

// NormalizeRevocationReason buckets free-text revocation reasons into
// a closed-set label value so `AuthRevocation` stays bounded. Unknown
// reasons land in `"other"`. Empty reasons land in `"unspecified"`.
func NormalizeRevocationReason(raw string) string {
	switch {
	case raw == "":
		return "unspecified"
	case containsFold(raw, "compromise"), containsFold(raw, "leak"):
		return "compromise"
	case containsFold(raw, "rotation"), containsFold(raw, "rotate"):
		return "rotation"
	case containsFold(raw, "offboard"), containsFold(raw, "depart"), containsFold(raw, "leave"), containsFold(raw, "left team"):
		return "offboarding"
	case containsFold(raw, "test"):
		return "test"
	default:
		return "other"
	}
}

// containsFold is a tiny case-insensitive substring check; kept inline
// to avoid pulling `strings` into the hot path imports.
func containsFold(haystack, needle string) bool {
	if len(needle) == 0 {
		return true
	}
	if len(haystack) < len(needle) {
		return false
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		match := true
		for j := 0; j < len(needle); j++ {
			a := haystack[i+j]
			b := needle[j]
			if a >= 'A' && a <= 'Z' {
				a += 'a' - 'A'
			}
			if b >= 'A' && b <= 'Z' {
				b += 'a' - 'A'
			}
			if a != b {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func init() {
	prometheus.MustRegister(
		AuthIssuance,
		AuthRotation,
		AuthRevocation,
		AuthValidationLatency,
		AuthValidationOutcome,
		AuthLegacyFallbackUsed,
		AuthFailure,
		AuthScopeRejected,
		AuthScopeCheckBypassed,
		AuthCorpusGrantWouldDeny,
		AuthCorpusGrantAllowed,
		AuthCorpusGrantEnforcementMode,
	)
}
