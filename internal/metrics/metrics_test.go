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

	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	expected := map[string]bool{
		"smackerel_artifacts_ingested_total": false,
		"smackerel_capture_total":            false,
		"smackerel_search_latency_seconds":   false,
		"smackerel_domain_extraction_total":  false,
		"smackerel_connector_sync_total":     false,
		"smackerel_nats_deadletter_total":    false,
		"smackerel_db_connections_active":    false,
		"smackerel_digest_generation_total":  false,
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
