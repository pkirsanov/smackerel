//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/notification"
)

func TestRepeatedRoutineNotificationsDoNotCreateRepeatedEscalations(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)
	prefix := notificationE2EPrefix()
	sourceID := prefix + "-routine-source"
	first := notificationManualIngest(t, cfg, map[string]any{"source_type": "manual_fixture", "source_instance_id": sourceID, "title": "backup complete", "body": "routine backup complete", "severity": "low", "subject": "backup", "service": "backup", "domain": "ops", "intent": "routine", "delivery_metadata": map[string]string{"actor": "e2e"}})
	second := notificationManualIngest(t, cfg, map[string]any{"source_type": "manual_fixture", "source_instance_id": sourceID, "title": "backup complete again", "body": "routine backup complete", "severity": "low", "subject": "backup", "service": "backup", "domain": "ops", "intent": "routine", "delivery_metadata": map[string]string{"actor": "e2e"}})
	if first.IncidentID != second.IncidentID {
		t.Fatalf("routine repeats should correlate into one auditable incident, first=%s second=%s", first.IncidentID, second.IncidentID)
	}
	incident := notificationIncidentDetail(t, cfg, first.IncidentID)
	if incident.PersistenceCount < 2 {
		t.Fatalf("routine repeated incident did not record persistence count: %+v", incident)
	}
	firstDetail := notificationEventDetail(t, cfg, first.NotificationID)
	secondDetail := notificationEventDetail(t, cfg, second.NotificationID)
	if firstDetail.Decision == nil || secondDetail.Decision == nil || firstDetail.Decision.DecisionType != "record_only" || secondDetail.Decision.DecisionType != "record_only" {
		t.Fatalf("routine notifications created actionable decisions: first=%+v second=%+v", firstDetail.Decision, secondDetail.Decision)
	}
	outputs := notificationOutputs(t, cfg)
	for _, output := range outputs {
		if output.IncidentID == first.IncidentID {
			t.Fatalf("routine incident produced user-facing escalation output: %+v", output)
		}
	}
}

func TestNotificationSnoozeAndQuietWindowsPersistThroughAPIs(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)
	prefix := notificationE2EPrefix()
	created := notificationManualIngest(t, cfg, map[string]any{"source_type": "manual_fixture", "source_instance_id": prefix + "-snooze-source", "title": "routine incident needs quiet handling", "body": "routine incident should be snoozed through durable suppression API", "severity": "low", "subject": prefix + "-snooze", "service": prefix + "-snooze", "domain": "ops", "intent": "routine", "delivery_metadata": map[string]string{"actor": "e2e"}})

	resp, err := apiPostJSON(cfg, "/api/notifications/incidents/"+created.IncidentID+"/snooze", map[string]any{"duration_minutes": 30, "reason": "operator snoozed noisy routine incident"})
	if err != nil {
		t.Fatalf("snooze request failed: %v", err)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read snooze body: %v", err)
	}
	if resp.StatusCode != 202 {
		t.Fatalf("snooze returned %d: %s", resp.StatusCode, string(body))
	}
	var snooze struct {
		Suppression struct {
			ID         string `json:"ID"`
			IncidentID string `json:"IncidentID"`
			Kind       string `json:"Kind"`
			Reason     string `json:"Reason"`
		} `json:"suppression"`
	}
	if err := json.Unmarshal(body, &snooze); err != nil {
		t.Fatalf("parse snooze response: %v; body=%s", err, string(body))
	}
	if snooze.Suppression.ID == "" || snooze.Suppression.IncidentID != created.IncidentID || snooze.Suppression.Kind != "user_preference" || !strings.Contains(snooze.Suppression.Reason, "operator snoozed") {
		t.Fatalf("snooze did not persist incident-scoped user-preference suppression: %+v", snooze.Suppression)
	}

	store, _, cleanup := notificationE2EStore(t)
	t.Cleanup(cleanup)
	now := time.Now().UTC()
	expiresAt := now.Add(15 * time.Minute)
	quietWindow, err := store.CreateSuppression(context.Background(), notification.Suppression{IncidentID: created.IncidentID, Kind: notification.SuppressionQuietWindow, Scope: map[string]any{"incident_id": created.IncidentID, "quiet_window": "e2e"}, Reason: "quiet window e2e audit", StartsAt: now, ExpiresAt: &expiresAt, CreatedAt: now})
	if err != nil {
		t.Fatalf("seed durable quiet-window suppression: %v", err)
	}

	quietResp, err := apiGet(cfg, "/api/notifications/quiet-windows")
	if err != nil {
		t.Fatalf("quiet windows request failed: %v", err)
	}
	quietBody, err := readBody(quietResp)
	if err != nil {
		t.Fatalf("read quiet windows body: %v", err)
	}
	if quietResp.StatusCode != 200 {
		t.Fatalf("quiet windows returned %d: %s", quietResp.StatusCode, string(quietBody))
	}
	if !strings.Contains(string(quietBody), quietWindow.ID) || !strings.Contains(string(quietBody), "quiet_window") || !strings.Contains(string(quietBody), created.IncidentID) {
		t.Fatalf("quiet window API did not return durable quiet-window suppression: %s", string(quietBody))
	}

	suppressionResp, err := apiGet(cfg, "/api/notifications/suppressions")
	if err != nil {
		t.Fatalf("suppressions request failed: %v", err)
	}
	suppressionBody, err := readBody(suppressionResp)
	if err != nil {
		t.Fatalf("read suppressions body: %v", err)
	}
	if suppressionResp.StatusCode != 200 {
		t.Fatalf("suppressions returned %d: %s", suppressionResp.StatusCode, string(suppressionBody))
	}
	if !strings.Contains(string(suppressionBody), snooze.Suppression.ID) || !strings.Contains(string(suppressionBody), quietWindow.ID) {
		t.Fatalf("suppression audit API did not expose snooze and quiet-window records: %s", string(suppressionBody))
	}
}

type notificationOutputResponse struct {
	DecisionID string `json:"DecisionID"`
	IncidentID string `json:"IncidentID"`
	Status     string `json:"Status"`
	Channel    string `json:"Channel"`
}

func notificationOutputs(t *testing.T, cfg e2eConfig) []notificationOutputResponse {
	t.Helper()
	resp, err := apiGet(cfg, "/api/notifications/outputs")
	if err != nil {
		t.Fatalf("notification outputs request failed: %v", err)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read notification outputs body: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("notification outputs returned %d: %s", resp.StatusCode, string(body))
	}
	var parsed struct {
		Outputs []notificationOutputResponse `json:"outputs"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("parse notification outputs: %v; body=%s", err, string(body))
	}
	return parsed.Outputs
}
