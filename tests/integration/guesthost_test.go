//go:build integration

package integration

import (
	"os"
	"testing"
)

// SCN-GH-008: GuestHost connector full sync against live stack.
// STUB: requires live GuestHost API + database — no assertions yet.
func TestGuestHost_Integration_SyncLifecycle(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("STUB: requires DATABASE_URL — integration test body not yet implemented")
	}
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		t.Skip("STUB: requires NATS_URL — integration test body not yet implemented")
	}
	t.Skip("STUB: test body not yet implemented — needs seeded GH mock + DB assertions")
}

// SCN-GH-001: API client authenticates with GuestHost tenant API.
// STUB: requires live GuestHost API credentials — no assertions yet.
func TestGuestHost_Integration_ClientAuth(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("STUB: requires DATABASE_URL — integration test body not yet implemented")
	}
	t.Skip("STUB: test body not yet implemented — needs GuestHost API credentials + auth validation")
}
