//go:build integration

// Spec 038 Scope 8 — Consumer Canary (SCN-038-021).
//
// Asserts the smallest possible end-to-end path: one drive artifact
// loaded from the canonical artifact store via the provider-neutral
// consumer adapter, then surfaced through the same SQL the digest
// generator uses (artifacts table, processing_status='completed',
// recent created_at). Failure here means downstream features cannot
// trust the consumer adapter, regardless of what the larger
// cross-feature test reports.
package drive

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/drive/consumers"
	driveextract "github.com/smackerel/smackerel/internal/drive/extract"
	smscan "github.com/smackerel/smackerel/internal/drive/scan"
	"github.com/smackerel/smackerel/tests/integration/drive/fixtures"
)

func TestDriveConsumerCanary_OneArtifactFlowsThroughArtifactStoreToDigest(t *testing.T) {
	pool := driveTestPool(t)
	fixtureServer := fixtures.NewServer()
	defer fixtureServer.Close()
	fixtureServer.AddFiles([]fixtures.File{
		{
			ID: "scope8-canary", Name: "Canary recipe.txt", MimeType: "text/plain",
			SizeBytes: 96, FolderPath: []string{"Canary", "Folder"},
			RevisionID: "scope8-canary-rev-1", Owner: "fixture-owner@example.com",
			URL:     "https://drive.example/scope8-canary",
			Content: []byte("Canary recipe: olive oil, basil. Action: buy basil."),
		},
	})
	provider := newScope2GoogleProvider(fixtureServer, pool)
	connectionID := createScope2Connection(t, pool, fixtureServer, provider, fixtureScope("root"))

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if _, err := smscan.NewService(provider, smscan.NewPostgresStore(pool)).InitialScan(ctx, connectionID); err != nil {
		t.Fatalf("InitialScan: %v", err)
	}
	if _, err := driveextract.NewService(provider, driveextract.NewPostgresStore(pool), driveextract.NewRuleBasedWorker()).ProcessPending(ctx, connectionID); err != nil {
		t.Fatalf("ProcessPending: %v", err)
	}

	artifactID := "drive:google:" + connectionID + ":scope8-canary"

	// Provider-neutral adapter — single entry point for downstream features.
	summary, err := consumers.LoadDriveArtifact(ctx, pool, artifactID)
	if err != nil {
		t.Fatalf("LoadDriveArtifact: %v", err)
	}
	if summary.ProviderID != "google" {
		t.Fatalf("summary.ProviderID = %q, want google", summary.ProviderID)
	}
	if !strings.Contains(strings.ToLower(summary.ExtractedText), "olive") {
		t.Fatalf("summary.ExtractedText = %q, want fixture content", summary.ExtractedText)
	}
	if !summary.IsAvailable || summary.Tombstoned || summary.PermissionLost {
		t.Fatalf("availability state wrong: %+v", summary)
	}

	// Digest-shaped query — same shape internal/digest/generator.go uses
	// (provider-neutral SELECT against artifacts table).
	var artifactType, processingStatus string
	if err := pool.QueryRow(ctx, `
		SELECT artifact_type, processing_status FROM artifacts WHERE id=$1`, artifactID,
	).Scan(&artifactType, &processingStatus); err != nil {
		t.Fatalf("digest-shaped lookup: %v", err)
	}
	if artifactType != "drive_file" || processingStatus != "completed" {
		t.Fatalf("digest-shaped lookup: type=%q status=%q, want drive_file/completed", artifactType, processingStatus)
	}

	// Adversarial: a non-drive artifact ID must surface ErrNotDriveArtifact.
	if _, err := consumers.LoadDriveArtifact(ctx, pool, "telegram:nonsense:123"); err == nil {
		t.Fatalf("LoadDriveArtifact for non-drive ID returned no error; consumer adapter must fail loud")
	}
}
