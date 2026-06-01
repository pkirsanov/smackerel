// telemetry.go — spec 075 SCOPE-1 residual telemetry and HMAC user
// bucket helper. This file owns the privacy-preserving primitives
// the rest of the package and the monitoring stack consume.
//
// Privacy invariants (design.md §Security/Compliance, SCN-075-A11):
//
//   - Telemetry labels include retired command tokens but NEVER raw
//     user identifiers and NEVER raw user turn text.
//   - User identity is observable ONLY as the HMAC-SHA256 hex digest
//     produced by UserBucket(). Direct emission of the raw user id
//     as a Prometheus label is structurally impossible because the
//     metric definitions declare the closed label set
//     {"command", "user_bucket"} — adding a "user_id" label would
//     require editing this file (which the privacy test guards).
//
// Scope split: Scope 1 owns metric registration and the bucket
// helper. Scope 3 wires the residual increment from the policy
// decision path.
package legacyretirement

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
)

// MetricNameResidualTotal is the canonical Prometheus name for the
// residual-usage counter. Exported so the privacy test and the
// monitoring dashboard query can reference it without string drift.
const MetricNameResidualTotal = "smackerel_legacy_command_residual_total"

// MetricNameNoticeTotal is the canonical name for the notice-outcome
// counter (Scope 3 increments it; Scope 1 only registers it).
const MetricNameNoticeTotal = "smackerel_legacy_retirement_notice_total"

// MetricNameWindowState is the canonical name for the effective-state
// gauge.
const MetricNameWindowState = "smackerel_legacy_retirement_window_state"

// MetricNameRetiredHandlerInvocation is the canonical name for the
// closed-state safety counter (Scope 5 increments; Scope 1 registers).
const MetricNameRetiredHandlerInvocation = "smackerel_legacy_retired_handler_invocation_total"

// MetricNameThresholdOver is the canonical name for the per-day
// threshold-breach counter (Scope 4 increments; Scope 1 registers).
const MetricNameThresholdOver = "smackerel_legacy_retirement_threshold_over_total"

// LabelUserBucket is the canonical label name for the HMAC user
// bucket. This constant is referenced by the privacy guard test —
// any future regression that swaps this for a raw-id label name
// must update the constant and trip the test.
const LabelUserBucket = "user_bucket"

// LabelCommand is the canonical retired-command-token label name.
const LabelCommand = "command"

// LabelOutcome is the closed-set notice outcome label.
const LabelOutcome = "outcome"

// LabelState is the effective-state gauge label.
const LabelState = "state"

// ResidualUsageCounter is the prom metric the Scope 3 policy
// increments on every retired-command invocation during an open or
// paused window. Label set is intentionally restricted to {command,
// user_bucket}; user_bucket MUST be the HMAC bucket from
// UserBucket(), never the raw user id.
var ResidualUsageCounter = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: MetricNameResidualTotal,
		Help: "Spec 075 — retired-command invocations during the deprecation window, labeled by command and HMAC user bucket. user_bucket is a privacy-preserving HMAC-SHA256 hex digest; raw user ids and raw turn text MUST NEVER appear as a label value.",
	},
	[]string{LabelCommand, LabelUserBucket},
)

// NoticeOutcomeCounter is the notice-outcome counter (Scope 3 wires).
var NoticeOutcomeCounter = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: MetricNameNoticeTotal,
		Help: "Spec 075 — retirement notice outcomes (shown, dedup_suppressed, paused_suppressed).",
	},
	[]string{LabelCommand, LabelOutcome},
)

// WindowStateGauge exposes the effective-state for dashboard
// rendering. Value semantics: 1 for the active state, 0 otherwise.
var WindowStateGauge = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: MetricNameWindowState,
		Help: "Spec 075 — effective deprecation-window state gauge; one of {open, paused, closed} is set to 1 at a time.",
	},
	[]string{LabelState},
)

// RetiredHandlerInvocationCounter counts any retired-handler invocation
// after the window closes (Scope 5 safety guard).
var RetiredHandlerInvocationCounter = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: MetricNameRetiredHandlerInvocation,
		Help: "Spec 075 — retired-handler invocations after window_state=closed. MUST stay at zero before final deletion proceeds.",
	},
	[]string{LabelCommand},
)

// ThresholdOverCounter counts per-day threshold breaches that drive
// the automatic pause transition (Scope 4 wires).
var ThresholdOverCounter = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: MetricNameThresholdOver,
		Help: "Spec 075 — daily threshold-breach count per retired command, used by the threshold evaluator.",
	},
	[]string{LabelCommand},
)

func init() {
	prometheus.MustRegister(
		ResidualUsageCounter,
		NoticeOutcomeCounter,
		WindowStateGauge,
		RetiredHandlerInvocationCounter,
		ThresholdOverCounter,
	)
}

// ErrEmptyHMACKey is returned by NewUserBucketHasher when the SST
// secret is empty. The config validator catches this at startup;
// the constructor check is the defense-in-depth so a caller cannot
// silently construct a hasher with an empty key.
var ErrEmptyHMACKey = errors.New("legacyretirement: user_bucket_hmac_key is empty; refusing to construct a non-keyed bucket hasher")

// UserBucketHasher computes the privacy-preserving HMAC-SHA256
// bucket label for a user id. Single instance per process; the key
// comes from legacy_retirement.user_bucket_hmac_key (SST).
type UserBucketHasher struct {
	key []byte
}

// NewUserBucketHasher constructs a UserBucketHasher from the SST key.
// Empty key returns ErrEmptyHMACKey.
func NewUserBucketHasher(hmacKey string) (*UserBucketHasher, error) {
	if strings.TrimSpace(hmacKey) == "" {
		return nil, ErrEmptyHMACKey
	}
	// Copy the key into a private slice so a caller cannot mutate
	// it after construction.
	k := make([]byte, len(hmacKey))
	copy(k, hmacKey)
	return &UserBucketHasher{key: k}, nil
}

// UserBucket returns the HMAC-SHA256 hex digest of userID. Empty
// userID returns the empty string (the caller decides whether to
// drop the observation or emit it with bucket="unknown"); the
// hasher never returns the raw userID.
func (h *UserBucketHasher) UserBucket(userID string) string {
	if userID == "" {
		return ""
	}
	mac := hmac.New(sha256.New, h.key)
	mac.Write([]byte(userID))
	return hex.EncodeToString(mac.Sum(nil))
}

// ResidualTelemetry is the seam Scope 3 implements to drive the
// residual counter from the policy decision path. Declaring it as
// an interface in Scope 1 means Scope 2's policy can depend on the
// contract without depending on the (later) wiring.
type ResidualTelemetry interface {
	// Record bumps the residual counter for a single retired-command
	// invocation. The implementation MUST translate userID through
	// UserBucketHasher.UserBucket before emitting the metric;
	// passing the raw userID as a label value is a policy violation
	// guarded by the privacy test.
	Record(command, userBucket string, outcome RetirementOutcome)
}
