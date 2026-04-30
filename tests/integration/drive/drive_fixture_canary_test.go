//go:build integration

package drive

import (
	"context"
	"io"
	"testing"
	"time"

	smscan "github.com/smackerel/smackerel/internal/drive/scan"
	"github.com/smackerel/smackerel/tests/integration/drive/fixtures"
)

func TestDriveFixtureCanary_ProductionProviderPathConsumesFixtureServer(t *testing.T) {
	pool := driveTestPool(t)
	fixtureServer := fixtures.NewServer()
	defer fixtureServer.Close()
	fixtureServer.AddFile(fixtures.File{
		ID:         "canary-file-001",
		Name:       "Fixture canary.txt",
		MimeType:   "text/plain",
		SizeBytes:  33,
		FolderPath: []string{"Canary"},
		RevisionID: "canary-rev-001",
		Owner:      "fixture-owner@example.com",
		URL:        "https://drive.example/canary-file-001",
		Content:    []byte("production provider path consumed me"),
	})

	provider := newScope2GoogleProvider(fixtureServer, pool)
	connectionID := createScope2Connection(t, pool, fixtureServer, provider, fixtureScope("root"))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	items, nextPageToken, err := provider.ListFolder(ctx, connectionID, "root", "")
	if err != nil {
		t.Fatalf("ListFolder through production provider: %v", err)
	}
	if nextPageToken != "" {
		t.Fatalf("nextPageToken = %q, want empty", nextPageToken)
	}
	if len(items) != 1 || items[0].ProviderFileID != "canary-file-001" {
		t.Fatalf("fixture items = %+v, want canary-file-001", items)
	}
	bytesResult, err := provider.GetFile(ctx, connectionID, "canary-file-001")
	if err != nil {
		t.Fatalf("GetFile through production provider: %v", err)
	}
	defer bytesResult.Reader.Close()
	body, err := io.ReadAll(bytesResult.Reader)
	if err != nil {
		t.Fatalf("read fixture bytes: %v", err)
	}
	if string(body) != "production provider path consumed me" {
		t.Fatalf("GetFile body = %q", string(body))
	}

	store := smscan.NewPostgresStore(pool)
	service := smscan.NewService(provider, store)
	if _, err := service.InitialScan(ctx, connectionID); err != nil {
		t.Fatalf("InitialScan through production provider: %v", err)
	}
	if fixtureServer.RequestCount("/drive/v3/files") == 0 {
		t.Fatalf("fixture /drive/v3/files request count = 0; provider did not use fixture boundary")
	}
}
