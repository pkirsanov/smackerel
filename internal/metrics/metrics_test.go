package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestMetricsRegistered(t *testing.T) {
	// Initialize vector metrics so they appear in Gather output.
	ArtifactsIngested.WithLabelValues("_test_reg", "article")
	CaptureTotal.WithLabelValues("_test_reg")
	SearchLatency.WithLabelValues("_test_reg")
	DomainExtraction.WithLabelValues("_test_reg", "ok")
	ConnectorSync.WithLabelValues("_test_reg", "success")
	NATSDeadLetter.WithLabelValues("_test_reg")
	DigestGeneration.WithLabelValues("_test_reg")
	ListsGenerated.WithLabelValues("_test_reg", "_test_reg")
	ListGenerationLatency.WithLabelValues("_test_reg")
	ListItemStatusChanges.WithLabelValues("_test_reg")
	ListsCompleted.WithLabelValues("_test_reg")
	// Spec 039 Scope 6 — recommendation observability metrics.
	RecommendationProviderRequests.WithLabelValues("_test_reg", "_test_reg", "_test_reg")
	RecommendationProviderLatency.WithLabelValues("_test_reg", "_test_reg")
	RecommendationCandidates.WithLabelValues("_test_reg", "_test_reg", "_test_reg")
	RecommendationWatchRuns.WithLabelValues("_test_reg", "_test_reg")
	RecommendationDelivery.WithLabelValues("_test_reg", "_test_reg")
	RecommendationSuppression.WithLabelValues("_test_reg")
	RecommendationRankingConfidence.WithLabelValues("_test_reg")
	RecommendationLocationPrecision.WithLabelValues("_test_reg", "_test_reg")
	QFPacketIngestTotal.WithLabelValues("_test_reg", "_test_reg", "_test_reg", "_test_reg")
	QFPacketValidationFailures.WithLabelValues("_test_reg")
	QFEvidenceExportAttempts.WithLabelValues("_test_reg", "_test_reg", "_test_reg")
	QFCursorLagSeconds.Set(0)
	QFActionBoundaryAttemptsTotal.WithLabelValues("_test_reg")
	QFCapabilityMismatch.WithLabelValues("_test_reg", "_test_reg")
	QFUnknownDecisionType.WithLabelValues("_test_reg")
	QFEngagementSignalAttemptsTotal.WithLabelValues("_test_reg", "_test_reg", "_test_reg")
	QFEvidenceRevokedTotal.WithLabelValues("_test_reg")
	QFCallbackAttemptsTotal.WithLabelValues("_test_reg", "_test_reg")
	QFDeepLinkRenderTotal.WithLabelValues("_test_reg", "_test_reg")
	QFTrustObjectRenderFailures.WithLabelValues("_test_reg")
	// Spec 021 Scope 4 — Unified Surfacing Controller metrics.
	SurfacingNudgesDelivered.WithLabelValues("_test_reg", "_test_reg")
	SurfacingActedOn.WithLabelValues("_test_reg")
	SurfacingFalsePositive.WithLabelValues("_test_reg")
	SurfacingDedupe.WithLabelValues("_test_reg")
	SurfacingSuppression.WithLabelValues("_test_reg")
	SurfacingBudgetOverrides.WithLabelValues("_test_reg")
	SurfacingBudgetRemaining.Set(0)
	SurfacingDeferredExhausted.WithLabelValues("_test_reg")

	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	expected := map[string]bool{
		"smackerel_artifacts_ingested_total":                  false,
		"smackerel_capture_total":                             false,
		"smackerel_search_latency_seconds":                    false,
		"smackerel_domain_extraction_total":                   false,
		"smackerel_connector_sync_total":                      false,
		"smackerel_nats_deadletter_total":                     false,
		"smackerel_db_connections_active":                     false,
		"smackerel_digest_generation_total":                   false,
		"smackerel_lists_generated_total":                     false,
		"smackerel_list_generation_latency_seconds":           false,
		"smackerel_list_item_status_changes_total":            false,
		"smackerel_lists_completed_total":                     false,
		"smackerel_recommendation_provider_requests_total":    false,
		"smackerel_recommendation_provider_latency_seconds":   false,
		"smackerel_recommendation_candidates_total":           false,
		"smackerel_recommendation_watch_runs_total":           false,
		"smackerel_recommendation_delivery_total":             false,
		"smackerel_recommendation_suppression_total":          false,
		"smackerel_recommendation_ranking_confidence_total":   false,
		"smackerel_recommendation_location_precision_total":   false,
		"smackerel_qf_packet_ingest_total":                    false,
		"smackerel_qf_packet_validation_failures_total":       false,
		"smackerel_qf_evidence_export_attempts_total":         false,
		"smackerel_qf_cursor_lag_seconds":                     false,
		"smackerel_qf_action_boundary_attempts_total":         false,
		"smackerel_qf_capability_mismatch_total":              false,
		"smackerel_qf_unknown_decision_type_total":            false,
		"smackerel_qf_engagement_signal_attempts_total":       false,
		"smackerel_qf_evidence_revoked_total":                 false,
		"smackerel_qf_callback_attempts_total":                false,
		"smackerel_qf_deep_link_render_total":                 false,
		"smackerel_qf_trust_object_render_failures_total":     false,
		"smackerel_surfacing_nudges_delivered_total":          false,
		"smackerel_surfacing_acted_on_total":                  false,
		"smackerel_surfacing_false_positive_total":            false,
		"smackerel_surfacing_dedupe_total":                    false,
		"smackerel_surfacing_suppression_total":               false,
		"smackerel_surfacing_budget_overrides_total":          false,
		"smackerel_surfacing_budget_remaining":                false,
		"smackerel_surfacing_deferred_budget_exhausted_total": false,
	}

	for _, fam := range families {
		if _, ok := expected[fam.GetName()]; ok {
			expected[fam.GetName()] = true
		}
	}

	for name, found := range expected {
		if !found {
			t.Errorf("metric %q not registered", name)
		}
	}
}

func TestQFCompanionMetricLabelParity(t *testing.T) {
	QFPacketIngestTotal.WithLabelValues("packet_created", "recommendation", "display_only", "dashboard").Inc()
	QFPacketValidationFailures.WithLabelValues("missing_packet_id").Inc()
	QFEvidenceExportAttempts.WithLabelValues("accepted", "packet_context", "personal").Inc()
	QFCursorLagSeconds.Set(5)
	QFActionBoundaryAttemptsTotal.WithLabelValues("approval").Inc()
	QFCapabilityMismatch.WithLabelValues("v1", "v0").Inc()
	QFUnknownDecisionType.WithLabelValues("future_decision").Inc()
	QFEngagementSignalAttemptsTotal.WithLabelValues("flush", "digest", "queued").Inc()
	QFEvidenceRevokedTotal.WithLabelValues("consent_revoked").Inc()
	QFCallbackAttemptsTotal.WithLabelValues("ack", "blocked").Inc()
	QFDeepLinkRenderTotal.WithLabelValues("web", "signed_used").Inc()
	QFTrustObjectRenderFailures.WithLabelValues("missing_required_field").Inc()

	expected := map[string][]string{
		"smackerel_qf_packet_ingest_total":                {"approval_state", "decision_type", "event_type", "source_surface"},
		"smackerel_qf_packet_validation_failures_total":   {"reason"},
		"smackerel_qf_evidence_export_attempts_total":     {"sensitivity_tier", "status", "target_context_type"},
		"smackerel_qf_cursor_lag_seconds":                 {},
		"smackerel_qf_action_boundary_attempts_total":     {"attempted_action_type"},
		"smackerel_qf_capability_mismatch_total":          {"actual", "required"},
		"smackerel_qf_unknown_decision_type_total":        {"value"},
		"smackerel_qf_engagement_signal_attempts_total":   {"event", "status", "surface"},
		"smackerel_qf_evidence_revoked_total":             {"reason"},
		"smackerel_qf_callback_attempts_total":            {"action", "status"},
		"smackerel_qf_deep_link_render_total":             {"status", "surface"},
		"smackerel_qf_trust_object_render_failures_total": {"reason"},
	}

	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}
	seen := map[string]bool{}
	for _, family := range families {
		wantLabels, ok := expected[family.GetName()]
		if !ok {
			continue
		}
		seen[family.GetName()] = true
		if len(wantLabels) == 0 {
			if len(family.GetMetric()) == 0 {
				t.Fatalf("metric %s has no samples", family.GetName())
			}
			if len(family.GetMetric()[0].GetLabel()) != 0 {
				t.Fatalf("metric %s labels = %v, want none", family.GetName(), family.GetMetric()[0].GetLabel())
			}
			continue
		}
		labels := map[string]bool{}
		for _, sample := range family.GetMetric() {
			for _, label := range sample.GetLabel() {
				labels[label.GetName()] = true
			}
		}
		for _, wantLabel := range wantLabels {
			if !labels[wantLabel] {
				t.Fatalf("metric %s missing label %q; saw %v", family.GetName(), wantLabel, labels)
			}
		}
		if len(labels) != len(wantLabels) {
			t.Fatalf("metric %s label count = %d labels=%v, want %d %v", family.GetName(), len(labels), labels, len(wantLabels), wantLabels)
		}
	}
	for metricName := range expected {
		if !seen[metricName] {
			t.Fatalf("metric %s was not gathered", metricName)
		}
	}
}

func TestHandler_ReturnsPrometheusFormat(t *testing.T) {
	// Increment a counter so there's output beyond just Go runtime metrics.
	ArtifactsIngested.WithLabelValues("test", "article").Inc()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	body, _ := io.ReadAll(rec.Body)
	bodyStr := string(body)

	if !strings.Contains(bodyStr, "smackerel_artifacts_ingested_total") {
		t.Error("response missing smackerel_artifacts_ingested_total")
	}

	if !strings.Contains(bodyStr, "go_goroutines") {
		t.Error("response missing Go runtime metrics (go_goroutines)")
	}

	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/plain") && !strings.Contains(ct, "text/openmetrics") {
		t.Errorf("unexpected content type: %s", ct)
	}
}

func TestCounterIncrement(t *testing.T) {
	CaptureTotal.WithLabelValues("api").Inc()
	CaptureTotal.WithLabelValues("api").Inc()

	// Verify via gather
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather failed: %v", err)
	}

	for _, fam := range families {
		if fam.GetName() == "smackerel_capture_total" {
			for _, m := range fam.GetMetric() {
				for _, l := range m.GetLabel() {
					if l.GetName() == "source" && l.GetValue() == "api" {
						val := m.GetCounter().GetValue()
						if val < 2 {
							t.Errorf("expected capture_total{source=api} >= 2, got %f", val)
						}
						return
					}
				}
			}
		}
	}

	t.Error("smackerel_capture_total{source=api} not found in gathered metrics")
}

func TestHistogramObserve(t *testing.T) {
	SearchLatency.WithLabelValues("vector").Observe(0.123)

	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather failed: %v", err)
	}

	for _, fam := range families {
		if fam.GetName() == "smackerel_search_latency_seconds" {
			for _, m := range fam.GetMetric() {
				if m.GetHistogram().GetSampleCount() > 0 {
					return // found observation
				}
			}
		}
	}

	t.Error("smackerel_search_latency_seconds histogram has no observations")
}

func TestGaugeSet(t *testing.T) {
	DBConnectionsActive.Set(5)

	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather failed: %v", err)
	}

	for _, fam := range families {
		if fam.GetName() == "smackerel_db_connections_active" {
			for _, m := range fam.GetMetric() {
				val := m.GetGauge().GetValue()
				if val != 5 {
					t.Errorf("expected db_connections_active = 5, got %f", val)
				}
				return
			}
		}
	}

	t.Error("smackerel_db_connections_active not found in gathered metrics")
}

func TestConnectorSyncCounter(t *testing.T) {
	ConnectorSync.WithLabelValues("bookmarks", "success").Inc()
	ConnectorSync.WithLabelValues("bookmarks", "error").Inc()

	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather failed: %v", err)
	}

	found := map[string]bool{"success": false, "error": false}
	for _, fam := range families {
		if fam.GetName() == "smackerel_connector_sync_total" {
			for _, m := range fam.GetMetric() {
				var connector, status string
				for _, l := range m.GetLabel() {
					switch l.GetName() {
					case "connector":
						connector = l.GetValue()
					case "status":
						status = l.GetValue()
					}
				}
				if connector == "bookmarks" {
					if m.GetCounter().GetValue() < 1 {
						t.Errorf("connector_sync{connector=bookmarks,status=%s} expected >= 1, got %f", status, m.GetCounter().GetValue())
					}
					found[status] = true
				}
			}
		}
	}

	for status, ok := range found {
		if !ok {
			t.Errorf("connector_sync{connector=bookmarks,status=%s} not found", status)
		}
	}
}

func TestDomainExtractionCounter(t *testing.T) {
	DomainExtraction.WithLabelValues("recipe", "published").Inc()
	DomainExtraction.WithLabelValues("recipe", "error").Inc()

	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather failed: %v", err)
	}

	found := map[string]bool{"published": false, "error": false}
	for _, fam := range families {
		if fam.GetName() == "smackerel_domain_extraction_total" {
			for _, m := range fam.GetMetric() {
				var schema, status string
				for _, l := range m.GetLabel() {
					switch l.GetName() {
					case "schema":
						schema = l.GetValue()
					case "status":
						status = l.GetValue()
					}
				}
				if schema == "recipe" {
					if m.GetCounter().GetValue() < 1 {
						t.Errorf("domain_extraction{schema=recipe,status=%s} expected >= 1, got %f", status, m.GetCounter().GetValue())
					}
					found[status] = true
				}
			}
		}
	}

	for status, ok := range found {
		if !ok {
			t.Errorf("domain_extraction{schema=recipe,status=%s} not found", status)
		}
	}
}

func TestDomainExtractionLatencyHistogram(t *testing.T) {
	DomainExtractionLatency.WithLabelValues("recipe-extraction-v1").Observe(2500)
	DomainExtractionLatency.WithLabelValues("recipe-extraction-v1").Observe(15000)

	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather failed: %v", err)
	}

	found := false
	for _, fam := range families {
		if fam.GetName() == "smackerel_domain_extraction_duration_ms" {
			for _, m := range fam.GetMetric() {
				for _, l := range m.GetLabel() {
					if l.GetName() == "contract" && l.GetValue() == "recipe-extraction-v1" {
						h := m.GetHistogram()
						if h.GetSampleCount() < 2 {
							t.Errorf("expected >= 2 samples, got %d", h.GetSampleCount())
						}
						found = true
					}
				}
			}
		}
	}
	if !found {
		t.Error("domain_extraction_duration_ms{contract=recipe-extraction-v1} not found")
	}
}

func TestNATSDeadLetterCounter(t *testing.T) {
	NATSDeadLetter.WithLabelValues("artifacts").Inc()

	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather failed: %v", err)
	}

	for _, fam := range families {
		if fam.GetName() == "smackerel_nats_deadletter_total" {
			for _, m := range fam.GetMetric() {
				for _, l := range m.GetLabel() {
					if l.GetName() == "stream" && l.GetValue() == "artifacts" {
						if m.GetCounter().GetValue() < 1 {
							t.Errorf("nats_deadletter{stream=artifacts} expected >= 1, got %f", m.GetCounter().GetValue())
						}
						return
					}
				}
			}
		}
	}

	t.Error("smackerel_nats_deadletter_total{stream=artifacts} not found")
}

func TestAlertDeliveryMetrics(t *testing.T) {
	AlertsDelivered.WithLabelValues("bill").Inc()
	AlertsDelivered.WithLabelValues("trip_prep").Inc()
	AlertDeliveryFailures.Inc()
	AlertsProduced.WithLabelValues("bill").Inc()
	AlertsProduced.WithLabelValues("return_window").Inc()

	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather failed: %v", err)
	}

	checks := map[string]bool{
		"smackerel_alerts_delivered_total":        false,
		"smackerel_alert_delivery_failures_total": false,
		"smackerel_alerts_produced_total":         false,
	}

	for _, fam := range families {
		name := fam.GetName()
		if _, ok := checks[name]; ok {
			for _, m := range fam.GetMetric() {
				if m.GetCounter().GetValue() >= 1 {
					checks[name] = true
					break
				}
			}
		}
	}

	for name, found := range checks {
		if !found {
			t.Errorf("metric %s not found or has zero value", name)
		}
	}
}

// TestAlertProducerFailuresMetric verifies the producer-side failure counter
// (BUG-021-003) exposes the smackerel_alert_producer_failures_total family
// with the expected `type` label vocabulary and accepts increments for
// every alert-producer type.
func TestAlertProducerFailuresMetric(t *testing.T) {
	AlertProducerFailures.WithLabelValues("bill").Inc()
	AlertProducerFailures.WithLabelValues("trip_prep").Inc()
	AlertProducerFailures.WithLabelValues("return_window").Inc()
	AlertProducerFailures.WithLabelValues("relationship_cooling").Inc()

	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather failed: %v", err)
	}

	typesSeen := map[string]bool{
		"bill":                 false,
		"trip_prep":            false,
		"return_window":        false,
		"relationship_cooling": false,
	}
	familyFound := false

	for _, fam := range families {
		if fam.GetName() != "smackerel_alert_producer_failures_total" {
			continue
		}
		familyFound = true
		for _, m := range fam.GetMetric() {
			if m.GetCounter().GetValue() < 1 {
				continue
			}
			for _, lbl := range m.GetLabel() {
				if lbl.GetName() == "type" {
					if _, want := typesSeen[lbl.GetValue()]; want {
						typesSeen[lbl.GetValue()] = true
					}
				}
			}
		}
	}

	if !familyFound {
		t.Fatal("metric family smackerel_alert_producer_failures_total not found in gather output")
	}
	for typ, seen := range typesSeen {
		if !seen {
			t.Errorf("type label %q not observed with counter value >= 1", typ)
		}
	}
}
