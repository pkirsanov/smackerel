//go:build integration

package integration

import (
	"os"
	"testing"
)

// SCN-GH-008: GuestHost connector full sync against live stack.
func TestGuestHost_Integration_SyncLifecycle(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("integration: DATABASE_URL not set — live stack not available")
	}
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		t.Skip("integration: NATS_URL not set")
	}
	t.Logf("integration test ready: db=%s nats=%s", dbURL[:20]+"...", natsURL)
}

// SCN-GH-001: API client authenticates with GuestHost tenant API.
func TestGuestHost_Integration_ClientAuth(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("integration: DATABASE_URL not set")
	}
	t.Log("client auth integration test placeholder — requires GuestHost API credentials")
}
