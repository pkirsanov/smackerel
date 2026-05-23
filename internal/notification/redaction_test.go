package notification

import (
	"strings"
	"testing"
)

func TestNotificationRedactorRemovesSecretsFromLogsAPIAndDeliveryPayloads(t *testing.T) {
	redacted, state := RedactText("call https://example.test?token=abc123 with password=hunter2 and Authorization: Bearer secret-token")
	for _, forbidden := range []string{"abc123", "hunter2", "secret-token"} {
		if strings.Contains(redacted, forbidden) {
			t.Fatalf("redacted text leaked %q: %s", forbidden, redacted)
		}
	}
	if len(state.Categories) == 0 {
		t.Fatalf("redaction state did not record categories: %+v", state)
	}
}
