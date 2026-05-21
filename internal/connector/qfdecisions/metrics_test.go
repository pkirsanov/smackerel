package qfdecisions

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/smackerel/smackerel/internal/metrics"
)

// TestQFSymmetricMetricSetRegistersAllTwelveMetricsWithQFLabelParity is the
// SCN-SM-041-020 metric-parity assertion. It enumerates every QF
// observability metric the connector contracts to expose, drives each
// vector through its documented label set, and confirms (a) the metric
// is registered with the global gatherer and (b) the resulting sample
// carries the documented label keys verbatim.
//
// The 12 QF-specific metrics under spec 041 Scope 5 are:
//
//  1. smackerel_qf_packet_ingest_total              {event_type, decision_type, approval_state, source_surface}
//  2. smackerel_qf_capability_mismatch_total        {required, actual}
//  3. smackerel_qf_unknown_decision_type_total      {value}
//  4. smackerel_qf_cursor_lag_seconds               {} (gauge)
//  5. smackerel_qf_cursor_fast_forward_events_skipped_total {} (counter)
//  6. smackerel_qf_action_boundary_attempts_total   {attempted_action_type}
//  7. smackerel_qf_packet_validation_failures_total {reason}
//  8. smackerel_qf_freshness_p95_seconds            {stage}
//  9. smackerel_qf_trust_object_render_failures_total {reason}
// 10. smackerel_qf_deep_link_render_total           {surface, status}
// 11. smackerel_qf_evidence_export_attempts_total   {status, target_context_type, sensitivity_tier}
// 12. smackerel_qf_evidence_revoked_total           {reason}
//
// Two pre-MVP transport-handoff metrics:
//
//  13. smackerel_qf_engagement_signal_attempts_total {event, surface, status}
//  14. smackerel_qf_callback_attempts_total          {action, status}
//
// SCN-SM-041-020 (Scope 5 V3 + observability DoD).
func TestQFSymmetricMetricSetRegistersAllTwelveMetricsWithQFLabelParity(t *testing.T) {
	cases := []struct {
		name           string
		metricName     string
		expectedLabels []string
		// drive emits at least one sample via the documented helper or
		// vector accessor. labelExpect is the bounded label assignment
		// the test asserts in the resulting sample.
		drive       func()
		labelExpect map[string]string
		// noLabels is true for unlabeled gauges/counters.
		noLabels bool
	}{
		{
			name:           "packet_ingest_total",
			metricName:     "smackerel_qf_packet_ingest_total",
			expectedLabels: []string{"event_type", "decision_type", "approval_state", "source_surface"},
			drive: func() {
				RecordQFPacketIngest(QFDecisionEvent{
					EventType:     "packet_created",
					DecisionType:  "recommendation",
					ApprovalState: "approved",
					SourceSurface: SurfaceDigest,
				})
			},
			labelExpect: map[string]string{
				"event_type":     "packet_created",
				"decision_type":  "recommendation",
				"approval_state": "approved",
				"source_surface": SurfaceDigest,
			},
		},
		{
			name:           "capability_mismatch_total",
			metricName:     "smackerel_qf_capability_mismatch_total",
			expectedLabels: []string{"required", "actual"},
			drive: func() {
				metrics.QFCapabilityMismatch.WithLabelValues("v1", "v2").Inc()
			},
			labelExpect: map[string]string{"required": "v1", "actual": "v2"},
		},
		{
			name:           "unknown_decision_type_total",
			metricName:     "smackerel_qf_unknown_decision_type_total",
			expectedLabels: []string{"value"},
			drive: func() {
				metrics.QFUnknownDecisionType.WithLabelValues("frontier_decision").Inc()
			},
			labelExpect: map[string]string{"value": "frontier_decision"},
		},
		{
			name:           "cursor_lag_seconds",
			metricName:     "smackerel_qf_cursor_lag_seconds",
			expectedLabels: nil,
			noLabels:       true,
			drive: func() {
				metrics.QFCursorLagSeconds.Set(12.5)
			},
		},
		{
			name:           "cursor_fast_forward_events_skipped_total",
			metricName:     "smackerel_qf_cursor_fast_forward_events_skipped_total",
			expectedLabels: nil,
			noLabels:       true,
			drive: func() {
				metrics.QFCursorFastForwardEventsSkipped.Add(3)
			},
		},
		{
			name:           "action_boundary_attempts_total",
			metricName:     "smackerel_qf_action_boundary_attempts_total",
			expectedLabels: []string{"attempted_action_type"},
			drive: func() {
				RecordQFActionBoundaryAttempt(ActionTypeApproval)
			},
			labelExpect: map[string]string{"attempted_action_type": ActionTypeApproval},
		},
		{
			name:           "packet_validation_failures_total",
			metricName:     "smackerel_qf_packet_validation_failures_total",
			expectedLabels: []string{"reason"},
			drive: func() {
				metrics.QFPacketValidationFailures.WithLabelValues("page_size_out_of_range").Inc()
			},
			labelExpect: map[string]string{"reason": "page_size_out_of_range"},
		},
		{
			name:           "freshness_p95_seconds",
			metricName:     "smackerel_qf_freshness_p95_seconds",
			expectedLabels: []string{"stage"},
			drive: func() {
				metrics.QFFreshnessP95Seconds.WithLabelValues(FreshnessStageRender).Set(22.5)
			},
			labelExpect: map[string]string{"stage": FreshnessStageRender},
		},
		{
			name:           "trust_object_render_failures_total",
			metricName:     "smackerel_qf_trust_object_render_failures_total",
			expectedLabels: []string{"reason"},
			drive: func() {
				metrics.QFTrustObjectRenderFailures.WithLabelValues("missing_required_field").Inc()
			},
			labelExpect: map[string]string{"reason": "missing_required_field"},
		},
		{
			name:           "deep_link_render_total",
			metricName:     "smackerel_qf_deep_link_render_total",
			expectedLabels: []string{"surface", "status"},
			drive: func() {
				metrics.QFDeepLinkRenderTotal.WithLabelValues(SurfaceDigest, "signed_used").Inc()
			},
			labelExpect: map[string]string{"surface": SurfaceDigest, "status": "signed_used"},
		},
		{
			name:           "evidence_export_attempts_total",
			metricName:     "smackerel_qf_evidence_export_attempts_total",
			expectedLabels: []string{"status", "target_context_type", "sensitivity_tier"},
			drive: func() {
				RecordQFEvidenceExportAttempt("ok", TargetContextPacketContext, "personal")
			},
			labelExpect: map[string]string{
				"status":              "ok",
				"target_context_type": TargetContextPacketContext,
				"sensitivity_tier":    "personal",
			},
		},
		{
			name:           "evidence_revoked_total",
			metricName:     "smackerel_qf_evidence_revoked_total",
			expectedLabels: []string{"reason"},
			drive: func() {
				metrics.QFEvidenceRevokedTotal.WithLabelValues("consent_revoked").Inc()
			},
			labelExpect: map[string]string{"reason": "consent_revoked"},
		},
		{
			name:           "engagement_signal_attempts_total",
			metricName:     "smackerel_qf_engagement_signal_attempts_total",
			expectedLabels: []string{"event", "surface", "status"},
			drive: func() {
				RecordQFEngagementSignalAttempt("packet_marked_seen", SurfaceDigest, "ok")
			},
			labelExpect: map[string]string{
				"event":   "packet_marked_seen",
				"surface": SurfaceDigest,
				"status":  "ok",
			},
		},
		{
			name:           "callback_attempts_total",
			metricName:     "smackerel_qf_callback_attempts_total",
			expectedLabels: []string{"action", "status"},
			drive: func() {
				RecordQFCallbackAttempt("surface_dismiss", "ok")
			},
			labelExpect: map[string]string{"action": "surface_dismiss", "status": "ok"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			tc.drive()
			labels, found := sampleLabelKeysFor(t, tc.metricName, tc.labelExpect)
			if !found {
				t.Fatalf("metric %q sample not found after drive (expected labels %v)", tc.metricName, tc.labelExpect)
			}
			if tc.noLabels {
				if len(labels) != 0 {
					t.Fatalf("metric %q expected no labels, got %v", tc.metricName, labels)
				}
				return
			}
			if !slicesEqualUnordered(labels, tc.expectedLabels) {
				t.Fatalf("metric %q sample label keys = %v, want %v", tc.metricName, labels, tc.expectedLabels)
			}
		})
	}
}

// TestQFSymmetricMetricSetHasAllFourteenQFPrefixedRegistrations asserts the
// fourteen QF-specific metrics (the smackerel_qf_-prefixed vectors Scope 5
// V3 enumerates) are each registered exactly once with the global gatherer.
// This is the registration-half complement to the per-vector label-parity
// assertion and guards against accidental deregistration during future
// refactors.
func TestQFSymmetricMetricSetHasAllFourteenQFPrefixedRegistrations(t *testing.T) {
	wantQFPrefixedMetrics := []string{
		"smackerel_qf_packet_ingest_total",
		"smackerel_qf_capability_mismatch_total",
		"smackerel_qf_unknown_decision_type_total",
		"smackerel_qf_cursor_lag_seconds",
		"smackerel_qf_cursor_fast_forward_events_skipped_total",
		"smackerel_qf_action_boundary_attempts_total",
		"smackerel_qf_packet_validation_failures_total",
		"smackerel_qf_freshness_p95_seconds",
		"smackerel_qf_trust_object_render_failures_total",
		"smackerel_qf_deep_link_render_total",
		"smackerel_qf_evidence_export_attempts_total",
		"smackerel_qf_evidence_revoked_total",
		"smackerel_qf_engagement_signal_attempts_total",
		"smackerel_qf_callback_attempts_total",
	}

	mfs, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("prometheus.DefaultGatherer.Gather: %v", err)
	}
	registered := make(map[string]int, len(mfs))
	for _, mf := range mfs {
		registered[mf.GetName()]++
	}
	for _, name := range wantQFPrefixedMetrics {
		if registered[name] == 0 {
			t.Fatalf("metric %q is not registered with the global prometheus.DefaultGatherer", name)
		}
		if registered[name] > 1 {
			t.Fatalf("metric %q is registered %d times, want exactly 1", name, registered[name])
		}
	}
}

// TestMetricLabelDefaultsBlankToUnknownAndTrimsWhitespace pins the bounded-label
// guard the connector relies on so the QF design 063 contract cannot leak
// unbounded label cardinality through whitespace-only inputs.
func TestMetricLabelDefaultsBlankToUnknownAndTrimsWhitespace(t *testing.T) {
	if got := metricLabel(""); got != metricUnknown {
		t.Fatalf("metricLabel(\"\") = %q, want %q", got, metricUnknown)
	}
	if got := metricLabel("  "); got != metricUnknown {
		t.Fatalf("metricLabel(\"  \") = %q, want %q", got, metricUnknown)
	}
	if got := metricLabel("  packet_created  "); got != "packet_created" {
		t.Fatalf("metricLabel whitespace-trim = %q, want %q", got, "packet_created")
	}
}

// TestQFFreshnessRollingP95UsesPerStageGaugeAfterRecord pins SCN-SM-041-020
// freshness emission for the render stage specifically. The bare assertion
// confirms calling QFFreshnessP95Seconds.Set on stage="render" produces a
// non-zero gauge sample with the expected label.
func TestQFFreshnessRollingP95UsesPerStageGaugeAfterRecord(t *testing.T) {
	metrics.QFFreshnessP95Seconds.WithLabelValues(FreshnessStageRender).Set(0)
	metrics.QFFreshnessP95Seconds.WithLabelValues(FreshnessStageRender).Set(15.0)
	got := testutil.ToFloat64(metrics.QFFreshnessP95Seconds.WithLabelValues(FreshnessStageRender))
	if got != 15.0 {
		t.Fatalf("freshness p95 stage=render gauge = %v, want 15.0", got)
	}
}

// sampleLabelKeysFor finds a metric sample matching the provided label values
// and returns the label keys present on that sample. The caller passes
// `labelExpect` to disambiguate when multiple samples are present for the
// same vector. For unlabeled metric families (no labels at all) it returns
// nil with found=true.
func sampleLabelKeysFor(t *testing.T, fqName string, labelExpect map[string]string) ([]string, bool) {
	t.Helper()
	mfs, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("prometheus.DefaultGatherer.Gather: %v", err)
	}
	for _, mf := range mfs {
		if mf.GetName() != fqName {
			continue
		}
		// Unlabeled metric family: zero label pairs on every sample.
		if len(labelExpect) == 0 {
			if len(mf.GetMetric()) == 0 {
				return nil, false
			}
			return nil, true
		}
		for _, m := range mf.GetMetric() {
			labelsOnSample := make(map[string]string, len(m.GetLabel()))
			for _, lp := range m.GetLabel() {
				labelsOnSample[lp.GetName()] = lp.GetValue()
			}
			match := true
			for k, want := range labelExpect {
				if got, ok := labelsOnSample[k]; !ok || got != want {
					match = false
					break
				}
			}
			if !match {
				continue
			}
			keys := make([]string, 0, len(labelsOnSample))
			for k := range labelsOnSample {
				keys = append(keys, k)
			}
			return keys, true
		}
	}
	return nil, false
}

func slicesEqualUnordered(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	sa := make(map[string]int, len(a))
	for _, v := range a {
		sa[v]++
	}
	for _, v := range b {
		sa[v]--
		if sa[v] < 0 {
			return false
		}
	}
	return true
}
