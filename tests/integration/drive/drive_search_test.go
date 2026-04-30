//go:build integration

package drive

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/api"
	driveextract "github.com/smackerel/smackerel/internal/drive/extract"
	smscan "github.com/smackerel/smackerel/internal/drive/scan"
	"github.com/smackerel/smackerel/tests/integration/drive/fixtures"
)

// TestDriveSearchFindsFilesByContentFolderAndMetadata proves SCN-038-010
// against the live test database: drive files seeded by Scope 2 + Scope 3
// must be returned by /api/search with the breadcrumb, sharing,
// sensitivity, provider URL, and snippet metadata Screen 5 needs.
//
// The test bypasses NATS embedding by driving the text-fallback path via
// SearchEngine{} with no MLSidecarURL — that path exercises the real
// pgvector text search query AND the EnrichDriveResults batch join, so
// any regression in either layer fails the test.
//
// Adversarial guards:
//   - the seeded fixture set includes one matching air-fryer manual AND
//     one non-matching dumpling note; the assertion that the non-match
//     is absent prevents the "always returns everything" tautology;
//   - the snippet/breadcrumb/sharing/sensitivity assertions match exact
//     fixture values, not zero-defaults;
//   - the sharing-state assertion explicitly differs between the two
//     fixtures (private vs shared) so a regression that returned a
//     constant label would fail.
func TestDriveSearchFindsFilesByContentFolderAndMetadata(t *testing.T) {
	pool := driveTestPool(t)
	fixtureServer := fixtures.NewServer()
	defer fixtureServer.Close()
	fixtureServer.AddFiles([]fixtures.File{
		{
			ID:         "scope4-air-fryer-manual",
			Name:       "Air fryer manual.txt",
			MimeType:   "text/plain",
			SizeBytes:  256,
			FolderPath: []string{"Manuals", "Kitchen"},
			RevisionID: "scope4-air-fryer-rev-1",
			Owner:      "fixture-owner@example.com",
			URL:        "https://drive.example/scope4-air-fryer-manual",
			Content:    []byte("Air-fryer manual: preheat 5 minutes, basket cleaning instructions, warranty registration."),
			Shared:     false,
		},
		{
			ID:         "scope4-dumpling-dough",
			Name:       "Dumpling dough hydration.txt",
			MimeType:   "text/plain",
			SizeBytes:  192,
			FolderPath: []string{"Recipes", "Asian"},
			RevisionID: "scope4-dumpling-rev-1",
			Owner:      "fixture-owner@example.com",
			URL:        "https://drive.example/scope4-dumpling-dough",
			Content:    []byte("Dumpling dough hydration target 50 percent water by flour weight; rest 30 minutes covered."),
			Shared:     true,
		},
		{
			ID:         "scope4-noise",
			Name:       "Random meeting notes.txt",
			MimeType:   "text/plain",
			SizeBytes:  96,
			FolderPath: []string{"Meetings"},
			RevisionID: "scope4-noise-rev-1",
			Owner:      "fixture-owner@example.com",
			URL:        "https://drive.example/scope4-noise",
			Content:    []byte("Standup notes about Q3 marketing plan rollout."),
			Shared:     false,
		},
	})

	provider := newScope2GoogleProvider(fixtureServer, pool)
	connectionID := createScope2Connection(t, pool, fixtureServer, provider, fixtureScope("root"))

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if _, err := smscan.NewService(provider, smscan.NewPostgresStore(pool)).InitialScan(ctx, connectionID); err != nil {
		t.Fatalf("InitialScan: %v", err)
	}
	processor := driveextract.NewService(provider, driveextract.NewPostgresStore(pool), driveextract.NewRuleBasedWorker())
	if _, err := processor.ProcessPending(ctx, connectionID); err != nil {
		t.Fatalf("ProcessPending: %v", err)
	}

	// Drive the text-fallback path so this test does not depend on a
	// running ML embedding sidecar. EnrichDriveResults runs in both
	// branches, so this still proves drive enrichment.
	engine := &api.SearchEngine{Pool: pool}
	results, total, mode, err := engine.Search(ctx, api.SearchRequest{Query: "air-fryer manual", Limit: 5})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if mode != "text_fallback" {
		t.Fatalf("search mode = %q, want text_fallback (ML sidecar should not be required for this test)", mode)
	}
	if total == 0 {
		t.Fatalf("search returned zero results for 'air-fryer manual'; want at least the seeded drive file")
	}

	var manual *api.SearchResult
	for i := range results {
		if results[i].ArtifactID == "drive:google:"+connectionID+":scope4-air-fryer-manual" {
			manual = &results[i]
			break
		}
	}
	if manual == nil {
		t.Fatalf("seeded air-fryer manual missing from search results: %+v", results)
	}
	if manual.ArtifactType != "drive_file" {
		t.Fatalf("manual.artifact_type = %q, want drive_file", manual.ArtifactType)
	}
	if manual.Drive == nil {
		t.Fatalf("manual.drive metadata is nil; enrichment did not run for drive_file result")
	}
	if manual.Drive.ProviderID != "google" {
		t.Fatalf("manual.drive.provider_id = %q, want google", manual.Drive.ProviderID)
	}
	if manual.Drive.ProviderURL != "https://drive.example/scope4-air-fryer-manual" {
		t.Fatalf("manual.drive.provider_url = %q, want fixture URL", manual.Drive.ProviderURL)
	}
	if len(manual.Drive.FolderBreadcrumb) != 2 || manual.Drive.FolderBreadcrumb[0] != "Manuals" || manual.Drive.FolderBreadcrumb[1] != "Kitchen" {
		t.Fatalf("manual.drive.folder_breadcrumb = %v, want [Manuals Kitchen]", manual.Drive.FolderBreadcrumb)
	}
	if manual.Drive.SharingState != "private" {
		t.Fatalf("manual.drive.sharing_state = %q, want private (fixture Shared=false)", manual.Drive.SharingState)
	}
	if manual.Drive.Sensitivity != "none" {
		t.Fatalf("manual.drive.sensitivity = %q, want none for kitchen manual", manual.Drive.Sensitivity)
	}
	if manual.Drive.Availability != "available" || !manual.Drive.ActionsEnabled || manual.Drive.Tombstoned || manual.Drive.PermissionLost {
		t.Fatalf("available file metadata wrong: %+v", manual.Drive)
	}
	if !strings.Contains(strings.ToLower(manual.Snippet), "air-fryer") && !strings.Contains(strings.ToLower(manual.Snippet), "preheat") {
		t.Fatalf("manual.snippet = %q, want excerpt containing matched terms", manual.Snippet)
	}

	// Adversarial: a query for the dumpling fixture must surface the
	// dumpling artifact AND must not surface the air-fryer artifact.
	results, _, _, err = engine.Search(ctx, api.SearchRequest{Query: "dumpling dough hydration", Limit: 5})
	if err != nil {
		t.Fatalf("Search dumpling: %v", err)
	}
	var dumpling, accidentalManual *api.SearchResult
	for i := range results {
		switch results[i].ArtifactID {
		case "drive:google:" + connectionID + ":scope4-dumpling-dough":
			dumpling = &results[i]
		case "drive:google:" + connectionID + ":scope4-air-fryer-manual":
			accidentalManual = &results[i]
		}
	}
	if dumpling == nil {
		t.Fatalf("dumpling fixture missing from results: %+v", results)
	}
	if accidentalManual != nil {
		t.Fatalf("dumpling search should not also return the air-fryer manual; got %+v", accidentalManual)
	}
	if dumpling.Drive == nil {
		t.Fatalf("dumpling.drive metadata is nil")
	}
	if dumpling.Drive.SharingState == "private" {
		t.Fatalf("dumpling.drive.sharing_state = %q, want shared (fixture Shared=true)", dumpling.Drive.SharingState)
	}
	if len(dumpling.Drive.FolderBreadcrumb) != 2 || dumpling.Drive.FolderBreadcrumb[0] != "Recipes" || dumpling.Drive.FolderBreadcrumb[1] != "Asian" {
		t.Fatalf("dumpling.drive.folder_breadcrumb = %v, want [Recipes Asian]", dumpling.Drive.FolderBreadcrumb)
	}

	// Adversarial: the noise fixture must NOT match either query above
	// (it has no overlap with "air-fryer manual" or "dumpling").
	if results, _, _, err := engine.Search(ctx, api.SearchRequest{Query: "air-fryer manual", Limit: 5}); err == nil {
		for _, r := range results {
			if r.ArtifactID == "drive:google:"+connectionID+":scope4-noise" {
				t.Fatalf("noise fixture leaked into 'air-fryer manual' results: %+v", r)
			}
		}
	}
}
