package notification

import (
	"strings"
	"testing"
)

func TestNotificationConfigFailsLoudWithoutRequiredValues(t *testing.T) {
	_, err := LoadNotificationConfig(map[string]string{
		"NOTIFICATION_INTELLIGENCE_ENABLED": "true",
	})
	if err == nil {
		t.Fatal("expected fail-loud config validation error")
	}
	for _, key := range []string{"NOTIFICATION_PERSISTENCE_THRESHOLD", "NOTIFICATION_ESCALATION_SEVERITY", "NOTIFICATION_LOW_CONFIDENCE_THRESHOLD", "NOTIFICATION_MAX_RETRIES", "NOTIFICATION_OUTPUT_CHANNELS"} {
		if !stringsContains(err.Error(), key) {
			t.Fatalf("error should name missing %s: %v", key, err)
		}
	}
}

func stringsContains(haystack string, needle string) bool {
	return strings.Contains(haystack, needle)
}
