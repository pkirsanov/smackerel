//go:build integration

package drive

import (
	"context"
	"testing"
	"time"

	smdrive "github.com/smackerel/smackerel/internal/drive"
	smscan "github.com/smackerel/smackerel/internal/drive/scan"
	"github.com/smackerel/smackerel/tests/integration/drive/fixtures"
)

func TestDriveScanFixturePreservesHierarchyAndMetadata(t *testing.T) {
	pool := driveTestPool(t)
	fixtureServer := fixtures.NewServer()
	defer fixtureServer.Close()
	fixtureServer.AddFiles(generateBulkDriveFiles(1200, 80))

	provider := newScope2GoogleProvider(fixtureServer, pool)
	connectionID := createScope2Connection(t, pool, fixtureServer, provider, fixtureScope("root"))

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	service := smscan.NewService(provider, smscan.NewPostgresStore(pool))
	result, err := service.InitialScan(ctx, connectionID)
	if err != nil {
		t.Fatalf("InitialScan: %v", err)
	}
	if result.IndexedCount != 1200 {
		t.Fatalf("IndexedCount = %d, want 1200", result.IndexedCount)
	}

	var driveFileCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM drive_files WHERE connection_id=$1`, connectionID).Scan(&driveFileCount); err != nil {
		t.Fatalf("count drive_files: %v", err)
	}
	if driveFileCount != 1200 {
		t.Fatalf("drive_files count = %d, want 1200", driveFileCount)
	}
	var linkedArtifacts int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM artifacts a JOIN drive_files f ON f.artifact_id=a.id WHERE f.connection_id=$1`, connectionID).Scan(&linkedArtifacts); err != nil {
		t.Fatalf("count linked artifacts: %v", err)
	}
	if linkedArtifacts != 1200 {
		t.Fatalf("linked artifacts = %d, want 1200", linkedArtifacts)
	}
	var distinctFolders int
	if err := pool.QueryRow(ctx, `SELECT COUNT(DISTINCT folder_path) FROM drive_files WHERE connection_id=$1`, connectionID).Scan(&distinctFolders); err != nil {
		t.Fatalf("count distinct folders: %v", err)
	}
	if distinctFolders != 80 {
		t.Fatalf("distinct folder paths = %d, want 80", distinctFolders)
	}

	var missingMetadata int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM drive_files
		WHERE connection_id=$1
		  AND (provider_url='' OR owner_label='' OR mime_type='' OR size_bytes <= 0 OR provider_revision_id='' OR jsonb_array_length(version_chain)=0)`, connectionID).Scan(&missingMetadata); err != nil {
		t.Fatalf("count missing metadata: %v", err)
	}
	if missingMetadata != 0 {
		t.Fatalf("files missing provider metadata = %d, want 0", missingMetadata)
	}

	var status string
	var indexedCount int64
	var skippedCount int64
	if err := pool.QueryRow(ctx, `
		SELECT status, indexed_count, skipped_count
		FROM drive_scan_jobs
		WHERE connection_id=$1 AND phase='scan'
		ORDER BY updated_at DESC LIMIT 1`, connectionID).Scan(&status, &indexedCount, &skippedCount); err != nil {
		t.Fatalf("scan progress row missing: %v", err)
	}
	if status != "complete" || indexedCount != 1200 || skippedCount != 0 {
		t.Fatalf("progress = status=%s indexed=%d skipped=%d, want complete/1200/0", status, indexedCount, skippedCount)
	}

	var extractionStates int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM drive_files WHERE connection_id=$1 AND extraction_state <> 'pending'`, connectionID).Scan(&extractionStates); err != nil {
		t.Fatalf("count extraction states: %v", err)
	}
	if extractionStates != 0 {
		t.Fatalf("Scope 2 must not start extraction/classification; non-pending extraction rows=%d", extractionStates)
	}
	var connectionStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM drive_connections WHERE id=$1`, connectionID).Scan(&connectionStatus); err != nil {
		t.Fatalf("read connection status: %v", err)
	}
	if connectionStatus != string(smdrive.HealthHealthy) {
		t.Fatalf("connection status = %s, want healthy", connectionStatus)
	}
}

func fixtureScope(folderID string) smdrive.Scope {
	return smdrive.Scope{FolderIDs: []string{folderID}, IncludeShared: false}
}
