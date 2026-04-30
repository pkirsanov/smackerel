//go:build e2e

package drive

import (
	"context"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/drive/monitor"
	"github.com/smackerel/smackerel/internal/drive/scan"
	"github.com/smackerel/smackerel/tests/integration/drive/fixtures"
)

func TestDriveScanE2E_EmptyDriveCreatesNoArtifacts(t *testing.T) {
	liveConfig := loadE2EConfig(t)
	waitForHealth(t, liveConfig, 120*time.Second)
	pool := driveE2EPool(t)
	fixtureServer := fixtures.NewServer()
	defer fixtureServer.Close()
	provider := newE2EGoogleProvider(fixtureServer, pool)
	connectionID := createE2EConnection(t, pool, fixtureServer, provider, []string{"root"})
	store := scan.NewPostgresStore(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if _, err := scan.NewService(provider, store).InitialScan(ctx, connectionID); err != nil {
		t.Fatalf("InitialScan empty drive: %v", err)
	}
	view := getDriveConnectionView(t, liveConfig, connectionID)
	if int(view["indexed_count"].(float64)) != 0 || view["empty_drive"].(bool) != true || view["status"] != "healthy" {
		t.Fatalf("empty drive view = %+v, want healthy indexed_count=0 empty_drive=true", view)
	}

	fixtureServer.AddFile(fixtures.File{
		ID:         "empty-e2e-upload-001",
		Name:       "Empty drive later upload.txt",
		MimeType:   "text/plain",
		SizeBytes:  64,
		FolderPath: []string{"Inbox"},
		RevisionID: "empty-e2e-rev-001",
		Owner:      "fixture-owner@example.com",
		URL:        "https://drive.example/empty-e2e-upload-001",
		Content:    []byte("later upload visible through monitor"),
	})
	fixtureServer.AddChange(fixtures.Change{Kind: "upsert", FileID: "empty-e2e-upload-001"})
	if _, err := monitor.NewService(provider, store).RunOnce(ctx, connectionID); err != nil {
		t.Fatalf("monitor after upload: %v", err)
	}
	view = getDriveConnectionView(t, liveConfig, connectionID)
	if int(view["indexed_count"].(float64)) != 1 || view["empty_drive"].(bool) != false {
		t.Fatalf("post-monitor view = %+v, want indexed_count=1 empty_drive=false", view)
	}
}
