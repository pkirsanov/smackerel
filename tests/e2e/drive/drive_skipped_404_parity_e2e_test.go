//go:build e2e

// 404-parity adversarial e2e test for chaos finding C-001 (round 3 harden,
// optionalHardeningPackets HARDEN-C001-test-parity). Before the fix,
// GET /v1/connectors/drive/connection/{valid-uuid-that-doesnt-exist}/skipped
// returned 200 with an empty payload while the parent endpoint
// GET /v1/connectors/drive/connection/{...} returned 404. Both endpoints
// MUST agree on the 404 contract.
//
// This file lives under the e2e build tag because it requires a real
// Postgres test DB (the validation happens AFTER the UUID check
// at the handler boundary). Skipping it as a unit test was the right
// call earlier — the proper home is e2e, where the live test stack
// gives us a real drive_connections table to probe against.
//
// Owner: spec 038 round 3 harden HARDEN-C001-test-parity (optional packet,
// low priority; locked in as e2e to prevent regression).

package drive

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestDriveSkippedEndpointReturns404ForNonExistentConnection(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)

	// A syntactically valid UUID that does not exist in drive_connections.
	// Generated fresh per test run so we don't depend on table state.
	nonExistentID := uuid.New().String()

	url := cfg.CoreURL + "/v1/connectors/drive/connection/" + nonExistentID + "/skipped"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("build GET %s: %v", url, err)
	}
	req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	// Adversarial assertion 1: status MUST be 404, not 200 (regression guard
	// for chaos finding C-001 — parent endpoint already returned 404,
	// /skipped returned 200 empty before the fix).
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("ADVERSARIAL FAILURE: /skipped returned status=%d for non-existent connection id %s (chaos finding C-001 regressed: /skipped MUST return 404 to match parent endpoint contract); body=%s",
			resp.StatusCode, nonExistentID, string(body))
	}

	// Adversarial assertion 2: response body MUST carry the stable error
	// code CONNECTION_NOT_FOUND (matches GetConnection's contract).
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "CONNECTION_NOT_FOUND") {
		t.Fatalf("body missing CONNECTION_NOT_FOUND error code; body=%s", bodyStr)
	}

	// Adversarial assertion 3: response body MUST NOT leak a raw DB error
	// (the handler does the SELECT EXISTS check BEFORE the
	// driveextract.NewPostgresStore call; if the contract regressed to
	// "ignore exists check, run the query, swallow error" the body could
	// leak SQLSTATE or "no rows" — neither should appear).
	for _, forbidden := range []string{"SQLSTATE", "no rows", "sql:", "pgx:"} {
		if strings.Contains(bodyStr, forbidden) {
			t.Fatalf("ADVERSARIAL FAILURE: /skipped response body leaks raw DB error substring %q; body=%s", forbidden, bodyStr)
		}
	}
}
