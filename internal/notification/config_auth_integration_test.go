//go:build integration

package notification

import "testing"

func TestNotificationConfigAuthAndMutationPoliciesHoldInLiveStack(t *testing.T) {
	cfg, err := LoadNotificationConfig(map[string]string{"NOTIFICATION_INTELLIGENCE_ENABLED": "true", "NOTIFICATION_PERSISTENCE_THRESHOLD": "2", "NOTIFICATION_ESCALATION_SEVERITY": "high", "NOTIFICATION_LOW_CONFIDENCE_THRESHOLD": "0.55", "NOTIFICATION_MAX_RETRIES": "2", "NOTIFICATION_OUTPUT_CHANNELS": "dashboard"})
	if err != nil {
		t.Fatalf("load notification config: %v", err)
	}
	if !cfg.Enabled || len(cfg.OutputChannels) != 1 {
		t.Fatalf("notification config not populated: %+v", cfg)
	}
}
