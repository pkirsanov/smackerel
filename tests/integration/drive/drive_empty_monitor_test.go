//go:build integration

package drive

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/drive/monitor"
	"github.com/smackerel/smackerel/internal/drive/scan"
	"github.com/smackerel/smackerel/tests/integration/drive/fixtures"
)

func TestEmptyDriveStaysHealthyAndDetectsLaterUpload(t *testing.T) {
	pool := driveTestPool(t)
	fixtureServer := fixtures.NewServer()
	defer fixtureServer.Close()
	provider := newScope2GoogleProvider(fixtureServer, pool)
	connectionID := createScope2Connection(t, pool, fixtureServer, provider, fixtureScope("root"))
	store := scan.NewPostgresStore(pool)
	scanner := scan.NewService(provider, store)
	monitorService := monitor.NewService(provider, store)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if _, err := scanner.InitialScan(ctx, connectionID); err != nil {
		t.Fatalf("InitialScan empty drive: %v", err)
	}
	if _, err := monitorService.RunOnce(ctx, connectionID); err != nil {
		t.Fatalf("first monitor cycle empty drive: %v", err)
	}
	assertDriveFileCount(t, pool, ctx, connectionID, 0)
	assertConnectionStatus(t, pool, ctx, connectionID, "healthy")

	fixtureServer.AddFile(fixtures.File{
		ID:         "later-upload-001",
		Name:       "Later upload.txt",
		MimeType:   "text/plain",
		SizeBytes:  42,
		FolderPath: []string{"Inbox"},
		RevisionID: "later-rev-001",
		Owner:      "fixture-owner@example.com",
		URL:        "https://drive.example/later-upload-001",
		Content:    []byte("later upload detected by monitor"),
	})
	fixtureServer.AddChange(fixtures.Change{Kind: "upsert", FileID: "later-upload-001"})

	result, err := monitorService.RunOnce(ctx, connectionID)
	if err != nil {
		t.Fatalf("monitor after later upload: %v", err)
	}
	if result.UpsertedCount != 1 {
		t.Fatalf("UpsertedCount = %d, want 1", result.UpsertedCount)
	}
	assertDriveFileCount(t, pool, ctx, connectionID, 1)
	assertConnectionStatus(t, pool, ctx, connectionID, "healthy")
}

func assertDriveFileCount(t *testing.T, pool *pgxpool.Pool, ctx context.Context, connectionID string, want int) {
	t.Helper()
	var got int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM drive_files WHERE connection_id=$1`, connectionID).Scan(&got); err != nil {
		t.Fatalf("count drive_files: %v", err)
	}
	if got != want {
		t.Fatalf("drive_files count = %d, want %d", got, want)
	}
}

func assertConnectionStatus(t *testing.T, pool *pgxpool.Pool, ctx context.Context, connectionID string, want string) {
	t.Helper()
	var got string
	if err := pool.QueryRow(ctx, `SELECT status FROM drive_connections WHERE id=$1`, connectionID).Scan(&got); err != nil {
		t.Fatalf("read connection status: %v", err)
	}
	if got != want {
		t.Fatalf("connection status = %s, want %s", got, want)
	}
}
