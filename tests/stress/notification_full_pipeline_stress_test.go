//go:build stress

package stress

import (
	"strings"
	"testing"
	"time"
)

func TestNotificationPipelineHandlesBurstWithBoundedRetriesAndRedactedTelemetry(t *testing.T) {
	cfg := loadStressConfig(t)
	stressWaitForHealth(t, cfg, 120*time.Second)
	prefix := notificationStressPrefix()
	routine := notificationStressIngest(t, cfg, notificationStressPayload(prefix+"-routine", prefix+"-routine-source", 0, "low", "routine"))
	approval := notificationStressIngest(t, cfg, map[string]any{"source_type": "manual_fixture", "source_instance_id": prefix + "-approval-source", "title": "approval requested full pipeline", "body": "approval requested token=secret-token", "severity": "high", "subject": prefix + "-approval", "service": prefix + "-approval", "domain": "ops", "intent": "approval", "delivery_metadata": map[string]string{"actor": "stress"}})
	uncertain := notificationStressIngest(t, cfg, map[string]any{"source_type": "manual_fixture", "source_instance_id": prefix + "-uncertain-source", "title": "unknown failed threshold", "body": "failed threshold without service metadata", "severity": "high", "subject": prefix + "-uncertain", "domain": "unknown", "intent": "investigate", "delivery_metadata": map[string]string{"actor": "stress"}})

	if routine.DecisionID == "" || approval.ApprovalID == "" || uncertain.DecisionID == "" {
		t.Fatalf("full pipeline burst did not return durable decision/approval identifiers: routine=%+v approval=%+v uncertain=%+v", routine, approval, uncertain)
	}
	status, body, err := stressAPIGet(cfg, "/api/notifications/status")
	if err != nil {
		t.Fatalf("notification status failed after burst: %v", err)
	}
	if status != 200 {
		t.Fatalf("notification status returned %d: %s", status, string(body))
	}
	if strings.Contains(string(body), "secret-token") {
		t.Fatalf("notification status leaked secret token after burst: %s", string(body))
	}
	outputs := notificationStressOutputs(t, cfg)
	foundApprovalOutput := false
	for _, output := range outputs {
		if output.DecisionID == approval.DecisionID {
			foundApprovalOutput = true
			if output.Channel != "dashboard" || output.Status != "queued" {
				t.Fatalf("approval output was not bounded to queued dashboard delivery: %+v", output)
			}
		}
	}
	if !foundApprovalOutput {
		t.Fatalf("full pipeline burst did not persist approval output attempt for decision %s", approval.DecisionID)
	}
}
