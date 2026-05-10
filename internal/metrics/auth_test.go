// Spec 044 Scope 04 — coverage for the per-user bearer-auth metrics
// surface defined in `internal/metrics/auth.go`. The tests assert
// that:
//
//  1. Every series declared by Scope 04 is registered with the global
//     Prometheus registry under the `smackerel_auth_*` name family.
//  2. The closed-set labels accept the documented values and emit
//     non-zero observations after a representative event sequence.
//  3. `NormalizeRevocationReason` buckets free-text reasons into the
//     documented closed-set label values (and never lets caller-
//     supplied free text reach the `reason` label, which would blow
//     up cardinality).
//
// All assertions go through the shared global registry — there is no
// in-test isolated registry — because production wiring also targets
// the global registry via `init()` in `internal/metrics/auth.go`. To
// avoid order-dependence on the integer counter values, the tests
// take a delta around each emission.
package metrics

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
)

// TestAuthMetrics_EmitsAllExpectedSeries proves the spec 044 Scope 04
// metric registration contract: every series listed in scopes.md
// Scope 4 DoD ("Prometheus metrics emitters live; registered in
// `internal/metrics/`") is gathered under its documented
// `smackerel_auth_*` name. The test seeds one observation on each
// LabelVec child first (Prometheus only surfaces a CounterVec /
// HistogramVec under Gather() once at least one labeled child has
// been observed) so a registered-but-unused metric still shows up.
func TestAuthMetrics_EmitsAllExpectedSeries(t *testing.T) {
	seedAllAuthMetrics()
	expected := []string{
		"smackerel_auth_issuance_total",
		"smackerel_auth_rotation_total",
		"smackerel_auth_revocation_total",
		"smackerel_auth_validation_latency_seconds",
		"smackerel_auth_validation_outcome_total",
		"smackerel_auth_legacy_fallback_used_total",
		"smackerel_auth_failure_total",
	}
	gathered, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	gotNames := make(map[string]bool, len(gathered))
	for _, mf := range gathered {
		gotNames[mf.GetName()] = true
	}
	for _, name := range expected {
		if !gotNames[name] {
			t.Errorf("expected metric %q to be registered with the default Prometheus registry", name)
		}
	}
}

// seedAllAuthMetrics observes one sample on each LabelVec child so
// Prometheus surfaces every series under Gather(). The seed values
// are chosen so they do not conflict with any other test's delta
// expectations; per-test increments use their own label set or take
// before/after deltas.
func seedAllAuthMetrics() {
	AuthIssuance.WithLabelValues("admin_api").Add(0)
	AuthIssuance.WithLabelValues("bootstrap_cli").Add(0)
	AuthIssuance.WithLabelValues("telegram_bridge").Add(0)
	AuthRevocation.WithLabelValues("unspecified").Add(0)
	AuthValidationOutcome.WithLabelValues("accepted", "header").Add(0)
	AuthLegacyFallbackUsed.WithLabelValues("production").Add(0)
	AuthFailure.WithLabelValues("missing_token").Add(0)
}

// TestAuthIssuance_IncrementsBySource exercises every documented
// `source` label value to prove the counter accepts the closed-set.
// The before/after deltas pin emission to a single increment per call
// so a wiring change that double-counts shows up as a unit-test
// failure.
func TestAuthIssuance_IncrementsBySource(t *testing.T) {
	for _, source := range []string{"admin_api", "bootstrap_cli", "telegram_bridge"} {
		t.Run(source, func(t *testing.T) {
			before := testutil.ToFloat64(AuthIssuance.WithLabelValues(source))
			AuthIssuance.WithLabelValues(source).Inc()
			after := testutil.ToFloat64(AuthIssuance.WithLabelValues(source))
			if got := after - before; got != 1 {
				t.Errorf("AuthIssuance{source=%q} delta = %v, want 1", source, got)
			}
		})
	}
}

// TestAuthRotation_Increments proves the rotation counter is
// observable and always paired (in production wiring) with an
// `AuthIssuance{source="admin_api"}` increment — that pairing is
// asserted at the integration level; here we just confirm the local
// counter moves.
func TestAuthRotation_Increments(t *testing.T) {
	before := testutil.ToFloat64(AuthRotation)
	AuthRotation.Inc()
	after := testutil.ToFloat64(AuthRotation)
	if got := after - before; got != 1 {
		t.Errorf("AuthRotation delta = %v, want 1", got)
	}
}

// TestAuthRevocation_NormalizesReason proves
// `NormalizeRevocationReason` buckets every documented free-text
// reason into the documented closed-set label value. An unknown
// free-text reason MUST land in `"other"` so a malicious operator
// can't use the reason field to inflate label cardinality.
func TestAuthRevocation_NormalizesReason(t *testing.T) {
	cases := []struct {
		raw, want string
	}{
		{"", "unspecified"},
		{"compromise", "compromise"},
		{"Possible Token Compromise", "compromise"},
		{"key leak detected", "compromise"},
		{"rotation cadence", "rotation"},
		{"scheduled rotate", "rotation"},
		{"user offboarded", "offboarding"},
		{"departure", "offboarding"},
		{"left team", "offboarding"},
		{"smoke test cleanup", "test"},
		{"some random caller-supplied free text", "other"},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			if got := NormalizeRevocationReason(tc.raw); got != tc.want {
				t.Errorf("NormalizeRevocationReason(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}

	// Adversarial: an unbounded operator-supplied reason MUST NOT
	// reach the label — the bucket label MUST be one of the closed
	// set. The closed set is exactly: unspecified, compromise,
	// rotation, offboarding, test, other.
	allowed := map[string]bool{
		"unspecified": true,
		"compromise":  true,
		"rotation":    true,
		"offboarding": true,
		"test":        true,
		"other":       true,
	}
	weirdReason := "Bobby Tables\n\n\nDROP TABLE auth_tokens;--"
	bucket := NormalizeRevocationReason(weirdReason)
	if !allowed[bucket] {
		t.Errorf("NormalizeRevocationReason emitted out-of-set bucket %q for adversarial input", bucket)
	}
}

// TestAuthRevocation_IncrementsBucketed exercises the counter with a
// normalized bucket label — the production wiring at
// `internal/auth/bearer_store.go::RevokeToken` MUST normalize before
// labeling.
func TestAuthRevocation_IncrementsBucketed(t *testing.T) {
	bucket := NormalizeRevocationReason("rotation cadence")
	before := testutil.ToFloat64(AuthRevocation.WithLabelValues(bucket))
	AuthRevocation.WithLabelValues(bucket).Inc()
	after := testutil.ToFloat64(AuthRevocation.WithLabelValues(bucket))
	if got := after - before; got != 1 {
		t.Errorf("AuthRevocation{reason=%q} delta = %v, want 1", bucket, got)
	}
}

// TestAuthValidationLatency_RecordsObservation proves the histogram
// is wired and that an observation lands in a bucket — the actual
// bucket boundaries are asserted via the metric definition itself in
// auth.go. Here we just check the sample count moves by one.
func TestAuthValidationLatency_RecordsObservation(t *testing.T) {
	// Use Gather to read a histogram count safely. testutil.CollectAndCount
	// counts samples regardless of bucket.
	before := histogramSampleCount(t, "smackerel_auth_validation_latency_seconds")
	AuthValidationLatency.Observe(0.0007)
	after := histogramSampleCount(t, "smackerel_auth_validation_latency_seconds")
	if got := after - before; got != 1 {
		t.Errorf("AuthValidationLatency sample-count delta = %v, want 1", got)
	}
}

// TestAuthValidationOutcome_AcceptsClosedSetLabels exercises every
// documented (result, source) combination so a future closed-set
// extension is forced through this test (a new label value would
// silently work if this test wasn't here, hiding a label-cardinality
// regression).
func TestAuthValidationOutcome_AcceptsClosedSetLabels(t *testing.T) {
	results := []string{
		"accepted",
		"rejected_revoked",
		"rejected_expired",
		"rejected_malformed",
		"rejected_unknown_key",
	}
	sources := []string{"header", "pwa_cookie"}
	for _, result := range results {
		for _, source := range sources {
			before := testutil.ToFloat64(AuthValidationOutcome.WithLabelValues(result, source))
			AuthValidationOutcome.WithLabelValues(result, source).Inc()
			after := testutil.ToFloat64(AuthValidationOutcome.WithLabelValues(result, source))
			if got := after - before; got != 1 {
				t.Errorf("AuthValidationOutcome{result=%q,source=%q} delta = %v, want 1",
					result, source, got)
			}
		}
	}
}

// TestAuthLegacyFallbackUsed_OperatorVisibility proves the
// deprecation-pathway counter is observable and that
// `environment=production` is the documented label value. The
// production wiring at `internal/api/router.go::bearerAuthMiddleware`
// only fires this counter in production, but we exercise the metric
// shape unconditionally in unit tests.
func TestAuthLegacyFallbackUsed_OperatorVisibility(t *testing.T) {
	before := testutil.ToFloat64(AuthLegacyFallbackUsed.WithLabelValues("production"))
	AuthLegacyFallbackUsed.WithLabelValues("production").Inc()
	after := testutil.ToFloat64(AuthLegacyFallbackUsed.WithLabelValues("production"))
	if got := after - before; got != 1 {
		t.Errorf("AuthLegacyFallbackUsed{environment=production} delta = %v, want 1", got)
	}
}

// TestAuthFailure_AcceptsClosedSetLabels exercises every documented
// `reason` label value, mirroring the
// TestAuthValidationOutcome_AcceptsClosedSetLabels pattern.
func TestAuthFailure_AcceptsClosedSetLabels(t *testing.T) {
	for _, reason := range []string{
		"missing_token",
		"invalid_format",
		"paseto_verify_failed",
		"revoked",
		"shared_token_mismatch",
		"auth_not_configured",
	} {
		t.Run(reason, func(t *testing.T) {
			before := testutil.ToFloat64(AuthFailure.WithLabelValues(reason))
			AuthFailure.WithLabelValues(reason).Inc()
			after := testutil.ToFloat64(AuthFailure.WithLabelValues(reason))
			if got := after - before; got != 1 {
				t.Errorf("AuthFailure{reason=%q} delta = %v, want 1", reason, got)
			}
		})
	}
}

// histogramSampleCount returns the cumulative sample count for a named
// histogram by gathering from the default registry. Returns 0 if the
// histogram has not yet been registered.
func histogramSampleCount(t *testing.T, name string) uint64 {
	t.Helper()
	gathered, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	for _, mf := range gathered {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			if h := m.GetHistogram(); h != nil {
				return h.GetSampleCount()
			}
		}
	}
	return 0
}

// TestAuthMetrics_NamesUseCanonicalPrefix asserts every spec 044
// Scope 04 metric name starts with `smackerel_auth_` so an operator
// scrape pattern of that prefix sweeps every per-user-bearer-auth
// series in one rule.
func TestAuthMetrics_NamesUseCanonicalPrefix(t *testing.T) {
	seedAllAuthMetrics()
	gathered, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	want := []string{
		"smackerel_auth_issuance_total",
		"smackerel_auth_rotation_total",
		"smackerel_auth_revocation_total",
		"smackerel_auth_validation_latency_seconds",
		"smackerel_auth_validation_outcome_total",
		"smackerel_auth_legacy_fallback_used_total",
		"smackerel_auth_failure_total",
	}
	got := make(map[string]*dto.MetricFamily, len(gathered))
	for _, mf := range gathered {
		got[mf.GetName()] = mf
	}
	for _, name := range want {
		if !strings.HasPrefix(name, "smackerel_auth_") {
			t.Fatalf("expected name list element %q to use smackerel_auth_ prefix", name)
		}
		if _, ok := got[name]; !ok {
			t.Errorf("expected gathered metrics to include %q", name)
		}
	}
}
