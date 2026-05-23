//go:build e2e

package e2e

import (
	"encoding/json"
	"testing"
	"time"
)

func TestApprovalRequestBlocksHighBlastActionUntilUserApproves(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)
	prefix := notificationE2EPrefix()
	created := notificationManualIngest(t, cfg, map[string]any{"source_type": "manual_fixture", "source_instance_id": prefix + "-approval", "title": "approval requested for shared-service mitigation", "body": "operator approval requested for high blast radius restart", "severity": "high", "subject": "shared-service", "service": "shared-service", "domain": "ops", "intent": "approval", "delivery_metadata": map[string]string{"actor": "e2e"}})
	if created.ApprovalID == "" {
		t.Fatalf("approval-request decision did not create durable approval request: %+v", created)
	}
	detail := notificationApprovalDetail(t, cfg, created.ApprovalID)
	if detail.Request.Status != "pending" || detail.Request.IncidentID != created.IncidentID || detail.Request.DecisionID != created.DecisionID {
		t.Fatalf("approval request did not block pending action with durable links: %+v", detail.Request)
	}
	updated := notificationRecordApprovalDecision(t, cfg, created.ApprovalID, "approve", "operator reviewed the redacted high-blast-radius action")
	if updated.Request.Status != "approved" || len(updated.Decisions) == 0 || updated.Decisions[0].Decision != "approve" {
		t.Fatalf("approval decision did not persist and update request status: %+v", updated)
	}
}

func TestDestructiveActionIsNeverExecutedAutomatically(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)
	prefix := notificationE2EPrefix()
	created := notificationManualIngest(t, cfg, map[string]any{"source_type": "manual_fixture", "source_instance_id": prefix + "-destructive", "title": "approval requested for destructive remediation", "body": "destructive wipe requested but automatic action must be refused", "severity": "critical", "subject": "shared-service", "service": "shared-service", "domain": "ops", "intent": "approval", "delivery_metadata": map[string]string{"actor": "e2e"}})
	detail := notificationEventDetail(t, cfg, created.NotificationID)
	if detail.Decision == nil || detail.Decision.DecisionType != "approval_request" || created.ApprovalID == "" {
		t.Fatalf("destructive/high-blast request was not held at approval boundary: decision=%+v approval=%s", detail.Decision, created.ApprovalID)
	}
	outputs := notificationOutputs(t, cfg)
	for _, output := range outputs {
		if output.DecisionID == created.DecisionID && output.Status == "sent" {
			t.Fatalf("destructive/high-blast approval request was executed as sent output before approval: %+v", output)
		}
	}
}

type notificationApprovalDetailResponse struct {
	Request struct {
		ID         string `json:"ID"`
		IncidentID string `json:"IncidentID"`
		DecisionID string `json:"DecisionID"`
		ActionKey  string `json:"ActionKey"`
		Status     string `json:"Status"`
	} `json:"Request"`
	Decisions []struct {
		Decision string `json:"Decision"`
		Reason   string `json:"Reason"`
	} `json:"Decisions"`
}

func notificationApprovalDetail(t *testing.T, cfg e2eConfig, id string) notificationApprovalDetailResponse {
	t.Helper()
	resp, err := apiGet(cfg, "/api/notifications/approvals/"+id)
	if err != nil {
		t.Fatalf("approval detail request failed: %v", err)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read approval detail body: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("approval detail returned %d: %s", resp.StatusCode, string(body))
	}
	var parsed notificationApprovalDetailResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("parse approval detail: %v; body=%s", err, string(body))
	}
	return parsed
}

func notificationRecordApprovalDecision(t *testing.T, cfg e2eConfig, id string, decision string, reason string) notificationApprovalDetailResponse {
	t.Helper()
	resp, err := apiPostJSON(cfg, "/api/notifications/approvals/"+id+"/decisions", map[string]any{"decision": decision, "reason": reason})
	if err != nil {
		t.Fatalf("approval decision request failed: %v", err)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read approval decision body: %v", err)
	}
	if resp.StatusCode != 202 {
		t.Fatalf("approval decision returned %d: %s", resp.StatusCode, string(body))
	}
	var parsed notificationApprovalDetailResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("parse approval decision detail: %v; body=%s", err, string(body))
	}
	return parsed
}
