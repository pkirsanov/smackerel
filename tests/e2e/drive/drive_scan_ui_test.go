//go:build e2e

package drive

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	smscan "github.com/smackerel/smackerel/internal/drive/scan"
	"github.com/smackerel/smackerel/tests/integration/drive/fixtures"
)

func TestDriveConnectorDetailShowsLiveScanProgressAndFinalCounts(t *testing.T) {
	liveConfig := loadE2EConfig(t)
	waitForHealth(t, liveConfig, 120*time.Second)
	pool := driveE2EPool(t)
	fixtureServer := fixtures.NewServer()
	defer fixtureServer.Close()
	fixtureServer.AddFiles(generateE2EBulkDriveFiles(24, 6))
	provider := newE2EGoogleProvider(fixtureServer, pool)
	connectionID := createE2EConnection(t, pool, fixtureServer, provider, []string{"root"})

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	result, err := smscan.NewService(provider, smscan.NewPostgresStore(pool)).InitialScan(ctx, connectionID)
	if err != nil {
		t.Fatalf("InitialScan: %v", err)
	}
	if result.IndexedCount != 24 {
		t.Fatalf("IndexedCount = %d, want 24", result.IndexedCount)
	}

	response := getDriveConnectionView(t, liveConfig, connectionID)
	progress := response["progress"].(map[string]any)
	if progress["status"] != "complete" {
		t.Fatalf("progress.status = %v, want complete; body=%+v", progress["status"], response)
	}
	if int(progress["indexed_count"].(float64)) != 24 {
		t.Fatalf("progress.indexed_count = %v, want 24", progress["indexed_count"])
	}
	if int(response["indexed_count"].(float64)) != 24 {
		t.Fatalf("indexed_count = %v, want 24", response["indexed_count"])
	}

	detailHTML := getText(t, liveConfig, liveConfig.CoreURL+"/pwa/connector-detail.html")
	for _, expected := range []string{"id=\"scan-progress\"", "id=\"recent-activity\"", "Indexed files", "Skipped files"} {
		if !strings.Contains(detailHTML, expected) {
			t.Fatalf("connector-detail.html missing %q", expected)
		}
	}
	detailJS := getText(t, liveConfig, liveConfig.CoreURL+"/pwa/connector-detail.js")
	for _, expected := range []string{"view.progress", "recent_activity", "scan-progress"} {
		if !strings.Contains(detailJS, expected) {
			t.Fatalf("connector-detail.js missing %q", expected)
		}
	}
}

func getDriveConnectionView(t *testing.T, liveConfig e2eConfig, connectionID string) map[string]any {
	t.Helper()
	responseText := getText(t, liveConfig, liveConfig.CoreURL+"/v1/connectors/drive/connection/"+connectionID)
	var decoded map[string]any
	if err := json.Unmarshal([]byte(responseText), &decoded); err != nil {
		t.Fatalf("decode connection view: %v body=%s", err, responseText)
	}
	return decoded
}
