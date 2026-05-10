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
package metrics

import "github.com/prometheus/client_golang/prometheus"

// AuthIssuance counts spec 044 token mints by source. The label is
// closed-set so cardinality is bounded; new sources MUST extend the
// allowed values list documented inline below.
//
// Allowed `source` values (closed set):
//   - "admin_api"        — POST /v1/auth/users (enrollment) +
//                          POST /v1/auth/users/{id}/rotate (rotation)
//   - "bootstrap_cli"    — `./smackerel.sh auth bootstrap`
//   - "telegram_bridge"  — `internal/telegram/per_user_token.go`
//                          mint per inbound Telegram message
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
	)
}
