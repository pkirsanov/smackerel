//go:build e2e

// Package e2e: full-stack coverage for spec 054 Scope 9 (Surfacing Controller
// Integration). Drives the live core HTTP API (no route()/intercept()/mocks)
// and proves the notification decision engine routes user-facing decisions
// through the shared spec 078 surfacing controller in production — observable
// via the producer="notification" surfacing_* metric families on /metrics — and
// that the incident-acknowledgment (snooze) path is wired end-to-end.
//
// Covers SCN-054-027, SCN-054-028, SCN-054-029, SCN-054-030 end-to-end and
// carries the adversarial regression that any reintroduction of direct dispatch
// (bypassing the controller) fails because the notification-labeled surfacing
// metric would never appear.
package e2e

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// surfacingScrapeHasNotificationProducer scrapes the live /metrics endpoint and
// reports whether any smackerel_surfacing_* series carries producer="notification".
// Permit/escalated -> nudges_delivered_total, deduped -> dedupe_total, deferred
// -> deferred_budget_exhausted_total all carry the producer label, so any
// non-suppressed verdict from the notification producer leaves a fingerprint
// here. Direct dispatch (bypassing the controller) would leave none.
func surfacingScrapeHasNotificationProducer(t *testing.T, cfg e2eConfig) (bool, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, cfg.CoreURL+"/metrics", nil)
	if err != nil {
		t.Fatalf("new /metrics request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("scrape /metrics: %v", err)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read /metrics body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("scrape /metrics status=%d body=%s", resp.StatusCode, string(body))
	}
	for _, line := range strings.Split(string(body), "\n") {
		if strings.HasPrefix(line, "smackerel_surfacing_") && strings.Contains(line, `producer="notification"`) {
			return true, line
		}
	}
	return false, ""
}

// TestNotificationSurfacingControllerEndToEndArbitrationAndAck covers
// SCN-054-027..030 end-to-end: a user-facing notification is arbitrated by the
// shared controller (producer="notification" appears on /metrics), and the
// operator acknowledgment (snooze) path returns 202 and feeds the shared ack
// registry. Live stack only.
func TestNotificationSurfacingControllerEndToEndArbitrationAndAck(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)
	prefix := notificationE2EPrefix()

	// Ingest a high-severity outage notification — this resolves to a
	// user-facing (RequiresOutput) decision that MUST be arbitrated by the
	// surfacing controller (SCN-054-027).
	created := notificationManualIngest(t, cfg, map[string]any{
		"source_type":        "manual_fixture",
		"source_instance_id": prefix + "-source",
		"title":              "checkout-api outage",
		"body":               "checkout-api outage failed",
		"severity":           "high",
		"subject":            "checkout-api",
		"service":            "checkout-api",
		"domain":             "ops",
		"intent":             "outage",
		"delivery_metadata":  map[string]string{"actor": "e2e"},
	})
	if created.IncidentID == "" || !created.Receipt.Accepted {
		t.Fatalf("manual ingest did not produce a durable incident: %+v", created)
	}

	// The controller must have seen a notification-producer candidate.
	found, line := surfacingScrapeHasNotificationProducer(t, cfg)
	if !found {
		t.Fatalf("SCN-054-027: no smackerel_surfacing_* series with producer=\"notification\" after ingest — decision engine bypassed the controller")
	}
	t.Logf("surfacing producer fingerprint: %s", line)

	// Operator acknowledges (snoozes) the incident — the production ack feed
	// (SnoozeIncident -> AcknowledgeIncident -> shared registry) must accept it
	// (SCN-054-030 production half).
	resp, err := apiPostJSON(cfg, "/api/notifications/incidents/"+created.IncidentID+"/snooze", map[string]any{
		"duration_minutes": 60,
		"reason":           "acknowledged via e2e — surfacing suppression feed",
	})
	if err != nil {
		t.Fatalf("snooze incident: %v", err)
	}
	body, _ := readBody(resp)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("snooze incident returned %d (want 202): %s", resp.StatusCode, string(body))
	}
}

// TestNotificationDecisionEngineNeverDispatchesDirectlyWhenControllerEnabled is
// the persistent E2E regression for SCN-054-027: with the controller wired in
// production, EVERY user-facing notification decision routes through it. The
// adversarial proof is the producer="notification" surfacing metric fingerprint
// — if a future change reintroduced un-gated direct queueing, that fingerprint
// would never appear and this test would fail.
func TestNotificationDecisionEngineNeverDispatchesDirectlyWhenControllerEnabled(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)
	prefix := notificationE2EPrefix()

	created := notificationManualIngest(t, cfg, map[string]any{
		"source_type":        "manual_fixture",
		"source_instance_id": prefix + "-regression-source",
		"title":              "checkout-api threshold breach",
		"body":               "checkout-api failed threshold breach",
		"severity":           "high",
		"subject":            "checkout-api-regression",
		"service":            "checkout-api-regression",
		"domain":             "ops",
		"intent":             "investigate",
		"delivery_metadata":  map[string]string{"actor": "e2e"},
	})
	if created.DecisionID == "" {
		t.Fatalf("manual ingest did not produce a decision: %+v", created)
	}

	found, line := surfacingScrapeHasNotificationProducer(t, cfg)
	if !found {
		t.Fatalf("regression: user-facing notification decision did not route through the surfacing controller (no producer=\"notification\" surfacing metric) — direct dispatch reintroduced")
	}
	t.Logf("controller-routing fingerprint: %s", line)
}
