//go:build stress

package stress

import (
	"testing"
	"time"
)

func TestLoopGuardPreventsRepeatedActionableReentryUnderBurst(t *testing.T) {
	cfg := loadStressConfig(t)
	stressWaitForHealth(t, cfg, 120*time.Second)
	prefix := notificationStressPrefix()
	created := notificationStressIngest(t, cfg, map[string]any{"source_type": "manual_fixture", "source_instance_id": prefix + "-loop-origin", "title": "approval requested loop source", "body": "approval requested for loop guard", "severity": "high", "subject": prefix, "service": prefix, "domain": "ops", "intent": "approval", "delivery_metadata": map[string]string{"actor": "stress"}})
	var loopKey string
	for _, output := range notificationStressOutputs(t, cfg) {
		if output.DecisionID == created.DecisionID {
			if value, ok := output.RedactionState["loop_guard_key"].(string); ok {
				loopKey = value
			}
		}
	}
	if loopKey == "" {
		t.Fatalf("approval output did not expose a redacted loop guard key for reentry suppression")
	}
	for index := 0; index < 6; index++ {
		reentry := notificationStressIngest(t, cfg, map[string]any{"source_type": "manual_fixture", "source_instance_id": prefix + "-loop-reentry", "title": "handler output reentered", "body": "handler output reentered as source event", "severity": "high", "subject": prefix, "service": prefix, "domain": "ops", "intent": "approval", "delivery_metadata": map[string]string{"actor": "stress", "loop_guard_key": loopKey}, "loop_metadata": map[string]string{"loop_guard_key": loopKey}})
		detail := notificationStressEventDetail(t, cfg, reentry.NotificationID)
		if detail.Decision == nil || detail.Decision.DecisionType != "no_action" {
			t.Fatalf("loop reentry remained actionable on iteration %d: %+v", index, detail.Decision)
		}
	}
	pool := notificationStressPool(t)
	reactionSuppressions := notificationStressCount(t, pool, "SELECT COUNT(*) FROM notification_suppressions WHERE source_instance_id = $1 AND suppression_kind = 'reaction_loop'", prefix+"-loop-reentry")
	if reactionSuppressions < 6 {
		t.Fatalf("loop guard did not persist every reentry suppression: got=%d want-at-least=6", reactionSuppressions)
	}
}
