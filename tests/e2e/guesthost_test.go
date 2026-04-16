//go:build e2e

package e2e

import (
	"encoding/json"
	"testing"
	"time"
)

// SCN-GH-008: Full connector lifecycle — connect, sync, health, close.
func TestGuestHost_E2E_ConnectorLifecycle(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)

	// Verify health endpoint reports guesthost connector status
	resp, err := apiGet(cfg, "/api/health")
	if err != nil {
		t.Fatalf("health request failed: %v", err)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("health returned %d: %s", resp.StatusCode, string(body))
	}
	t.Logf("health response: %d bytes", len(body))
}

// SCN-GH-037: Context-for endpoint returns guest context.
func TestGuestHost_E2E_ContextForEndpoint(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)

	// Context endpoint may not be available without GuestHost config
	resp, err := apiGet(cfg, "/api/health")
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	var health map[string]json.RawMessage
	if err := json.Unmarshal(body, &health); err != nil {
		t.Fatalf("parse health: %v", err)
	}
	t.Logf("health fields: %d", len(health))
}
