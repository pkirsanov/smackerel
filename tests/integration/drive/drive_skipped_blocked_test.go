//go:build integration

package drive

import (
	"context"
	"testing"
	"time"

	driveextract "github.com/smackerel/smackerel/internal/drive/extract"
	smscan "github.com/smackerel/smackerel/internal/drive/scan"
	"github.com/smackerel/smackerel/tests/integration/drive/fixtures"
)

func TestSkippedAndBlockedFilesPersistReasonAndAction(t *testing.T) {
	pool := driveTestPool(t)
	fixtureServer := fixtures.NewServer()
	defer fixtureServer.Close()
	fixtureServer.AddFiles([]fixtures.File{
		{
			ID:         "scope3-too-large",
			Name:       "Oversized contract.pdf",
			MimeType:   "application/pdf",
			SizeBytes:  4096,
			FolderPath: []string{"Contracts"},
			RevisionID: "scope3-too-large-rev-1",
			Owner:      "fixture-owner@example.com",
			URL:        "https://drive.example/scope3-too-large",
			Content:    []byte("large contract bytes"),
		},
		{
			ID:         "scope3-encrypted",
			Name:       "Encrypted archive.zip",
			MimeType:   "application/zip",
			SizeBytes:  64,
			FolderPath: []string{"Sensitive"},
			RevisionID: "scope3-encrypted-rev-1",
			Owner:      "fixture-owner@example.com",
			URL:        "https://drive.example/scope3-encrypted",
			Content:    []byte("PK encrypted"),
		},
	})

	provider := newScope2GoogleProvider(fixtureServer, pool)
	connectionID := createScope2Connection(t, pool, fixtureServer, provider, fixtureScope("root"))
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	store := smscan.NewPostgresStore(pool)
	if _, err := smscan.NewService(provider, store).InitialScan(ctx, connectionID); err != nil {
		t.Fatalf("InitialScan: %v", err)
	}
	processor := driveextract.NewService(provider, driveextract.NewPostgresStore(pool), driveextract.NewRuleBasedWorker(), driveextract.WithMaxFileSizeBytes(1024))
	result, err := processor.ProcessPending(ctx, connectionID)
	if err != nil {
		t.Fatalf("ProcessPending: %v", err)
	}
	if result.SkippedCount != 1 || result.BlockedCount != 1 {
		t.Fatalf("ProcessPending result = %+v, want one skipped and one blocked", result)
	}

	items, err := driveextract.NewPostgresStore(pool).ListSkippedBlocked(ctx, connectionID)
	if err != nil {
		t.Fatalf("ListSkippedBlocked: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("skipped/blocked item count = %d, want 2", len(items))
	}
	states := map[string]driveextract.SkippedBlockedItem{}
	for _, item := range items {
		states[item.ProviderFileID] = item
		if item.SkipReason == "" || item.RecommendedAction == "" || item.ProviderURL == "" {
			t.Fatalf("item missing reason/action/url: %+v", item)
		}
	}
	if states["scope3-too-large"].ExtractionState != "skipped" || states["scope3-too-large"].SkipReason != "file_too_large" {
		t.Fatalf("too-large state = %+v", states["scope3-too-large"])
	}
	if states["scope3-encrypted"].ExtractionState != "blocked" || states["scope3-encrypted"].SkipReason != "unsupported_binary" {
		t.Fatalf("encrypted state = %+v", states["scope3-encrypted"])
	}
}