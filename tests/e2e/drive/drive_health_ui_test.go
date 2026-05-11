//go:build e2e

package drive

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	drivehealth "github.com/smackerel/smackerel/internal/drive/health"
	"github.com/smackerel/smackerel/tests/integration/drive/fixtures"
)

func TestDriveConnectorDetailSurfacesProviderOutageAndRetryState(t *testing.T) {
	liveConfig := loadE2EConfig(t)
	waitForHealth(t, liveConfig, 120*time.Second)
	pool := driveE2EPool(t)
	fixtureServer := fixtures.NewServer()
	defer fixtureServer.Close()
	provider := newE2EGoogleProvider(fixtureServer, pool)
	connectionID := createE2EConnection(t, pool, fixtureServer, provider, []string{"root"})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	recorder := drivehealth.NewPostgresRecorder(pool, drivehealth.Policy{DegradedAfter: 1, FailingAfter: 3})
	providerError := errors.New("fixture provider returned 503 outage")
	for _, workType := range []string{"scan", "monitor", "retrieve"} {
		if _, err := recorder.RecordProviderError(ctx, connectionID, workType, providerError); err != nil {
			t.Fatalf("RecordProviderError(%s): %v", workType, err)
		}
	}

	view := getDriveConnectionView(t, liveConfig, connectionID)
	if view["status"] != "failing" {
		t.Fatalf("status = %v, want failing; view=%+v", view["status"], view)
	}
	if int(view["retryable_work_count"].(float64)) != 3 {
		t.Fatalf("retryable_work_count = %v, want 3", view["retryable_work_count"])
	}
	if !strings.Contains(view["health_reason"].(string), "503 outage") {
		t.Fatalf("health_reason = %q, want provider error", view["health_reason"])
	}

	detailJS := getText(t, liveConfig, liveConfig.CoreURL+"/pwa/connector-detail.js")
	for _, expected := range []string{"retryable_work_count", "health_reason", "Provider work is queued"} {
		if !strings.Contains(detailJS, expected) {
			t.Fatalf("connector-detail.js missing %q", expected)
		}
	}
}
