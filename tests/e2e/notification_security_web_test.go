//go:build e2e

package e2e

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestNotificationWebSurfacesAreRedactedAndAuthProtected(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)
	unauthenticated, err := http.Get(cfg.CoreURL + "/notifications")
	if err != nil {
		t.Fatalf("unauthenticated notification web request failed: %v", err)
	}
	unauthenticated.Body.Close()
	if unauthenticated.StatusCode != http.StatusUnauthorized {
		t.Fatalf("notification web surface did not require authentication, got %d", unauthenticated.StatusCode)
	}
	body := notificationWebPage(t, cfg, "/notifications")
	if !strings.Contains(body, "Notifications") || !strings.Contains(body, "Open Incidents") {
		t.Fatalf("authenticated notification web status did not render operator state: %s", body)
	}
	if strings.Contains(body, "secret-token") || strings.Contains(body, "password=hunter2") {
		t.Fatalf("authenticated notification web status leaked sensitive content: %s", body)
	}
}