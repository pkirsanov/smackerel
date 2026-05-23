//go:build e2e

package e2e

import (
	"strings"
	"testing"
	"time"
)

func TestNotificationOperatorPagesShowRedactedStatusAndIncidentTimeline(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)
	prefix := notificationE2EPrefix()
	created := notificationManualIngest(t, cfg, map[string]any{"source_type": "manual_fixture", "source_instance_id": prefix + "-web", "title": "approval requested for web timeline", "body": "approval token=secret-token should be redacted", "severity": "high", "subject": "web-service", "service": "web-service", "domain": "ops", "intent": "approval", "delivery_metadata": map[string]string{"actor": "e2e"}})
	paths := map[string]string{
		"/notifications":                                 "Pending Approvals",
		"/notifications/events":                          "Notification Events",
		"/notifications/incidents":                       "Notification Incidents",
		"/notifications/incidents/" + created.IncidentID: "Incident Timeline",
		"/notifications/approvals":                       "Notification Approvals",
		"/notifications/approvals/" + created.ApprovalID: "high-blast-radius",
		"/notifications/suppressions":                    "Quiet Windows",
		"/notifications/summary":                         "Handled Noise And Open Work",
		"/notifications/outputs":                         "Notification Outputs",
	}
	for path, marker := range paths {
		body := notificationWebPage(t, cfg, path)
		if !strings.Contains(body, marker) {
			t.Fatalf("web page %s missing marker %q: %s", path, marker, body)
		}
		if strings.Contains(body, "secret-token") || strings.Contains(body, "token=secret-token") {
			t.Fatalf("web page %s leaked sensitive notification payload: %s", path, body)
		}
	}
}

func TestNotificationOutputPageDoesNotExposeSecretsOrHardcodeTelegram(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)
	body := notificationWebPage(t, cfg, "/notifications/outputs")
	if strings.Contains(body, "secret-token") || strings.Contains(body, "password=hunter2") {
		t.Fatalf("notification output page leaked sensitive content: %s", body)
	}
	if strings.Contains(body, "Telegram") || strings.Contains(body, "telegram") {
		t.Fatalf("notification output page hardcoded Telegram instead of channel abstraction: %s", body)
	}
}

func notificationWebPage(t *testing.T, cfg e2eConfig, path string) string {
	t.Helper()
	resp, err := apiGet(cfg, path)
	if err != nil {
		t.Fatalf("web GET %s failed: %v", path, err)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read web body %s: %v", path, err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("web GET %s returned %d: %s", path, resp.StatusCode, string(body))
	}
	return string(body)
}
