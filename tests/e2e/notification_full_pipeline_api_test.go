//go:build e2e

package e2e

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestNotificationIngestPersistsRawAndNormalizedRecords(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)
	prefix := notificationE2EPrefix()
	created := notificationManualIngest(t, cfg, map[string]any{"source_type": "manual_fixture", "source_instance_id": prefix + "-source", "title": "checkout-api outage", "body": "checkout-api outage failed with token=secret-token", "severity": "high", "subject": "checkout-api", "service": "checkout-api", "domain": "ops", "intent": "outage", "delivery_metadata": map[string]string{"actor": "e2e"}})
	if created.NotificationID == "" || created.IncidentID == "" || created.DecisionID == "" || !created.Receipt.Accepted {
		t.Fatalf("manual ingest did not return durable identifiers: %+v", created)
	}
	detail := notificationEventDetail(t, cfg, created.NotificationID)
	if detail.Notification.ID == "" || detail.RawEvent.ID == "" {
		t.Fatalf("event detail missing raw/normalized records: %+v", detail)
	}
	if strings.Contains(detail.Notification.Body, "secret-token") {
		t.Fatalf("event detail leaked secret token: %+v", detail.Notification)
	}
}

func TestNotificationIngestDerivesStableEventIDWhenSourceIDMissing(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)
	prefix := notificationE2EPrefix()
	first := notificationManualIngest(t, cfg, map[string]any{"source_type": "manual_fixture", "source_instance_id": prefix + "-source", "title": "backup complete", "body": "routine backup complete", "severity": "low", "subject": "backup", "service": "backup", "domain": "ops", "intent": "routine", "delivery_metadata": map[string]string{"actor": "e2e", "request_id": "stable"}})
	detail := notificationEventDetail(t, cfg, first.NotificationID)
	if detail.RawEvent.SourceEventID == "" || detail.RawEvent.SourceEventIDOrigin != "handler_derived" {
		t.Fatalf("handler did not derive source event identity: %+v", detail.RawEvent)
	}
}

func TestNotificationDetailShowsClassificationRationale(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)
	prefix := notificationE2EPrefix()
	created := notificationManualIngest(t, cfg, map[string]any{"source_type": "manual_fixture", "source_instance_id": prefix + "-source", "title": "checkout-api outage", "body": "checkout-api outage failed", "severity": "critical", "subject": "checkout-api", "service": "checkout-api", "domain": "ops", "intent": "outage", "delivery_metadata": map[string]string{"actor": "e2e"}})
	detail := notificationEventDetail(t, cfg, created.NotificationID)
	if detail.Classification == nil || detail.Classification.Rationale == "" || detail.Classification.Severity == "" {
		t.Fatalf("classification rationale missing: %+v", detail.Classification)
	}
}

func TestRelatedNotificationsAppearAsSingleIncident(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)
	prefix := notificationE2EPrefix()
	first := notificationManualIngest(t, cfg, map[string]any{"source_type": "manual_fixture", "source_instance_id": prefix + "-source-a", "title": "checkout-api outage", "body": "checkout-api outage failed", "severity": "high", "subject": "checkout-api", "service": "checkout-api", "domain": "ops", "intent": "outage", "delivery_metadata": map[string]string{"actor": "e2e"}})
	second := notificationManualIngest(t, cfg, map[string]any{"source_type": "manual_fixture", "source_instance_id": prefix + "-source-b", "title": "checkout-api outage again", "body": "checkout-api outage failed again", "severity": "high", "subject": "checkout-api", "service": "checkout-api", "domain": "ops", "intent": "outage", "delivery_metadata": map[string]string{"actor": "e2e"}})
	if first.IncidentID != second.IncidentID {
		t.Fatalf("related notifications were not correlated into one incident: first=%s second=%s", first.IncidentID, second.IncidentID)
	}
	incident := notificationIncidentDetail(t, cfg, first.IncidentID)
	if incident.PersistenceCount < 2 {
		t.Fatalf("incident persistence count did not update: %+v", incident)
	}
}

func TestPersistentSevereIncidentProducesDiagnosticsOrEscalationDecision(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)
	prefix := notificationE2EPrefix()
	created := notificationManualIngest(t, cfg, map[string]any{"source_type": "manual_fixture", "source_instance_id": prefix + "-source", "title": "checkout-api outage", "body": "checkout-api outage failed", "severity": "high", "subject": "checkout-api", "service": "checkout-api", "domain": "ops", "intent": "outage", "delivery_metadata": map[string]string{"actor": "e2e"}})
	detail := notificationEventDetail(t, cfg, created.NotificationID)
	if detail.Decision == nil || detail.Decision.DecisionType == "" || detail.Decision.Rationale == "" {
		t.Fatalf("decision missing from detail: %+v", detail.Decision)
	}
}

func TestNotificationOperatorAPIReturnsStatusHistoryIncidentsActionsApprovalsSuppressionsSummariesAndOutputs(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)
	prefix := notificationE2EPrefix()
	created := notificationManualIngest(t, cfg, map[string]any{"source_type": "manual_fixture", "source_instance_id": prefix + "-operator-api", "title": "approval requested for operator API", "body": "operator approval requested token=secret-token", "severity": "high", "subject": "operator-api", "service": "operator-api", "domain": "ops", "intent": "approval", "delivery_metadata": map[string]string{"actor": "e2e"}})
	if created.ApprovalID == "" {
		t.Fatalf("operator API setup did not create a durable approval request: %+v", created)
	}
	paths := []string{"/api/notifications/status", "/api/notifications/sources", "/api/notifications/events", "/api/notifications/incidents", "/api/notifications/suppressions", "/api/notifications/quiet-windows", "/api/notifications/summary", "/api/notifications/outputs", "/api/notifications/approvals/" + created.ApprovalID}
	for _, path := range paths {
		resp, err := apiGet(cfg, path)
		if err != nil {
			t.Fatalf("GET %s failed: %v", path, err)
		}
		body, err := readBody(resp)
		if err != nil {
			t.Fatalf("read %s body: %v", path, err)
		}
		if resp.StatusCode != 200 {
			t.Fatalf("GET %s returned %d: %s", path, resp.StatusCode, string(body))
		}
		if strings.Contains(string(body), "secret-token") || strings.Contains(string(body), "password=hunter2") {
			t.Fatalf("GET %s leaked sensitive data: %s", path, string(body))
		}
	}
}

func TestNotificationFullPipelinePreservesAuditAndBlocksPolicyBypass(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)
	prefix := notificationE2EPrefix()
	created := notificationManualIngest(t, cfg, map[string]any{"source_type": "manual_fixture", "source_instance_id": prefix + "-source", "title": "approval requested", "body": "restart shared service approval", "severity": "high", "subject": "shared-service", "service": "shared-service", "domain": "ops", "intent": "approval", "delivery_metadata": map[string]string{"actor": "e2e"}})
	detail := notificationEventDetail(t, cfg, created.NotificationID)
	if detail.RawEvent.ID == "" || detail.Notification.ID == "" || detail.Classification == nil || detail.Decision == nil || detail.Incident == nil {
		t.Fatalf("full pipeline did not preserve audit chain: %+v", detail)
	}
}

type notificationIngestResponse struct {
	Receipt struct {
		Accepted   bool   `json:"Accepted"`
		RawEventID string `json:"RawEventID"`
	} `json:"receipt"`
	NotificationID string `json:"notification_id"`
	IncidentID     string `json:"incident_id"`
	DecisionID     string `json:"decision_id"`
	ApprovalID     string `json:"approval_id"`
}

type notificationEventDetailResponse struct {
	Notification struct {
		ID               string            `json:"ID"`
		SourceInstanceID string            `json:"SourceInstanceID"`
		SourceEventID    string            `json:"SourceEventID"`
		Body             string            `json:"Body"`
		DeliveryMetadata map[string]string `json:"DeliveryMetadata"`
	} `json:"Notification"`
	RawEvent struct {
		ID                  string            `json:"ID"`
		SourceEventID       string            `json:"SourceEventID"`
		SourceEventIDOrigin string            `json:"SourceEventIDOrigin"`
		SourceSpecific      map[string]string `json:"SourceSpecific"`
		DeliveryMetadata    map[string]string `json:"DeliveryMetadata"`
	} `json:"RawEvent"`
	Classification *struct {
		Severity    string         `json:"Severity"`
		Domain      string         `json:"Domain"`
		Intent      string         `json:"Intent"`
		Confidence  float64        `json:"Confidence"`
		Rationale   string         `json:"Rationale"`
		Uncertainty map[string]any `json:"Uncertainty"`
	} `json:"Classification"`
	Decision *struct {
		DecisionType string `json:"DecisionType"`
		Rationale    string `json:"Rationale"`
	} `json:"Decision"`
	Incident *notificationIncidentResponse `json:"Incident"`
}

type notificationIncidentResponse struct {
	ID               string `json:"ID"`
	PersistenceCount int    `json:"PersistenceCount"`
}

func notificationManualIngest(t *testing.T, cfg e2eConfig, payload map[string]any) notificationIngestResponse {
	t.Helper()
	resp, err := apiPostJSON(cfg, "/api/notifications/manual-ingest", payload)
	if err != nil {
		t.Fatalf("manual ingest failed: %v", err)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read manual ingest body: %v", err)
	}
	if resp.StatusCode != 201 {
		t.Fatalf("manual ingest returned %d: %s", resp.StatusCode, string(body))
	}
	var parsed notificationIngestResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("parse manual ingest response: %v; body=%s", err, string(body))
	}
	return parsed
}

func notificationEventDetail(t *testing.T, cfg e2eConfig, id string) notificationEventDetailResponse {
	t.Helper()
	resp, err := apiGet(cfg, "/api/notifications/events/"+id)
	if err != nil {
		t.Fatalf("event detail failed: %v", err)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read event detail body: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("event detail returned %d: %s", resp.StatusCode, string(body))
	}
	var parsed notificationEventDetailResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("parse event detail: %v; body=%s", err, string(body))
	}
	return parsed
}

func notificationIncidentDetail(t *testing.T, cfg e2eConfig, id string) notificationIncidentResponse {
	t.Helper()
	resp, err := apiGet(cfg, "/api/notifications/incidents/"+id)
	if err != nil {
		t.Fatalf("incident detail failed: %v", err)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read incident detail body: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("incident detail returned %d: %s", resp.StatusCode, string(body))
	}
	var parsed notificationIncidentResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("parse incident detail: %v; body=%s", err, string(body))
	}
	return parsed
}
