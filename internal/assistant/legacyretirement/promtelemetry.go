// promtelemetry.go — spec 075 SCOPE-3 concrete ResidualTelemetry
// implementations that drive the Prometheus counters declared in
// telemetry.go and fan out to optional sinks (e.g. the SQL residual
// store used by the rolling 7-day report).
//
// Privacy invariants (SCN-075-A11):
//   - PrometheusResidualTelemetry.Record refuses to emit a sample
//     whose user_bucket value is anything other than the 64-char
//     hex HMAC produced by UserBucketHasher.UserBucket. An empty
//     bucket (anonymous turn) is collapsed to the literal
//     "anonymous" label value so the counter never carries a raw
//     id-shaped value.
package legacyretirement

import (
	"regexp"
)

// hmacHexRE matches the 64-char lowercase hex digest produced by
// UserBucketHasher.UserBucket. Any other bucket-shaped value is
// rejected on the Record path so a regression in a caller cannot
// smuggle a raw id into the metric label.
var hmacHexRE = regexp.MustCompile(`^[0-9a-f]{64}$`)

// AnonymousBucketLabel is the closed-set sentinel emitted when a
// turn has no user identity (UserBucket returned ""). Keeping the
// sentinel as a fixed string preserves the metric's bounded label
// cardinality.
const AnonymousBucketLabel = "anonymous"

// PrometheusResidualTelemetry drives the spec 075 residual-usage and
// notice-outcome counters. It is the canonical ResidualTelemetry
// implementation wired in production assistant startup.
type PrometheusResidualTelemetry struct{}

// NewPrometheusResidualTelemetry returns the prometheus-backed
// telemetry sink. The counters are package-level singletons
// registered in telemetry.go init(), so this constructor is
// dependency-free.
func NewPrometheusResidualTelemetry() *PrometheusResidualTelemetry {
	return &PrometheusResidualTelemetry{}
}

// Record implements ResidualTelemetry by incrementing both the
// residual-usage counter (labelled {command, user_bucket}) and the
// notice-outcome counter (labelled {command, outcome}).
//
// An empty command is dropped (no observation rather than a sample
// with an empty label, which would silently break dashboards).
func (p *PrometheusResidualTelemetry) Record(command, userBucket string, outcome RetirementOutcome) {
	if command == "" {
		return
	}
	bucket := normaliseBucketLabel(userBucket)
	ResidualUsageCounter.WithLabelValues(command, bucket).Inc()
	if outcome != "" {
		NoticeOutcomeCounter.WithLabelValues(command, string(outcome)).Inc()
	}
}

// normaliseBucketLabel enforces the privacy invariant on the label
// value: only an HMAC-shaped digest or the anonymous sentinel may
// reach the Prometheus client. A non-empty, non-HMAC value is a
// caller bug and is collapsed to "anonymous" so the counter never
// leaks a raw id; a regression test pins this behaviour.
func normaliseBucketLabel(userBucket string) string {
	if userBucket == "" {
		return AnonymousBucketLabel
	}
	if !hmacHexRE.MatchString(userBucket) {
		return AnonymousBucketLabel
	}
	return userBucket
}

// MultiResidualTelemetry fans Record() out to N sinks. The order is
// the order sinks were supplied; sinks must not panic. Used to pair
// the Prometheus sink with the SQL residual store so the rolling
// 7-day report and the live /metrics scrape agree on counts.
type MultiResidualTelemetry struct {
	sinks []ResidualTelemetry
}

// NewMultiResidualTelemetry constructs a fan-out telemetry. Nil
// sinks are dropped silently so callers can wire optional sinks
// (e.g. SQL store only present in production) without conditional
// construction code.
func NewMultiResidualTelemetry(sinks ...ResidualTelemetry) *MultiResidualTelemetry {
	out := make([]ResidualTelemetry, 0, len(sinks))
	for _, s := range sinks {
		if s == nil {
			continue
		}
		out = append(out, s)
	}
	return &MultiResidualTelemetry{sinks: out}
}

// Record implements ResidualTelemetry.
func (m *MultiResidualTelemetry) Record(command, userBucket string, outcome RetirementOutcome) {
	for _, s := range m.sinks {
		s.Record(command, userBucket, outcome)
	}
}
