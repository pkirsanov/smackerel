//go:build integration

package drive

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/api"
	smdrive "github.com/smackerel/smackerel/internal/drive"
	driveextract "github.com/smackerel/smackerel/internal/drive/extract"
	smscan "github.com/smackerel/smackerel/internal/drive/scan"
	"github.com/smackerel/smackerel/tests/integration/drive/fixtures"
)

// TestTombstoneAndPermissionLossRemainQueryableWithoutBytes proves
// SCN-038-012 (design.md §11): a drive file that the provider trashes or
// revokes access for MUST stay queryable through search and detail
// endpoints, but byte-delivery actions MUST be disabled. The retained
// extracted knowledge is the user's recovery story; silently dropping
// the artifact would be a knowledge regression.
//
// Adversarial guards:
//   - the test seeds two distinct removal kinds (trash + permission loss)
//     and asserts each maps to its own availability label so a regression
//     that collapsed them to a single "unavailable" state would fail;
//   - banner text is asserted to be non-empty AND to mention what the
//     user has lost (bytes / access) so a fix that returned a generic
//     "unavailable" string would fail; tests would not be tautological
//     against an empty default;
//   - the detail handler MUST suppress extracted_text when bytes are
//     unavailable; the assertion that extracted_text == "" enforces this
//     and the assertion that summary remains non-empty proves we did not
//     accidentally erase the queryable knowledge surface.
func TestTombstoneAndPermissionLossRemainQueryableWithoutBytes(t *testing.T) {
	pool := driveTestPool(t)
	fixtureServer := fixtures.NewServer()
	defer fixtureServer.Close()
	fixtureServer.AddFiles([]fixtures.File{
		{
			ID:         "scope4-trashed",
			Name:       "Trashed receipt.txt",
			MimeType:   "text/plain",
			SizeBytes:  128,
			FolderPath: []string{"Receipts", "Travel"},
			RevisionID: "scope4-trashed-rev-1",
			Owner:      "fixture-owner@example.com",
			URL:        "https://drive.example/scope4-trashed",
			Content:    []byte("Receipt total 42.13 for airport lunch paid with card."),
			Shared:     false,
		},
		{
			ID:         "scope4-permission-lost",
			Name:       "Internal Q3 brief.txt",
			MimeType:   "text/plain",
			SizeBytes:  192,
			FolderPath: []string{"Strategy"},
			RevisionID: "scope4-permission-lost-rev-1",
			Owner:      "fixture-owner@example.com",
			URL:        "https://drive.example/scope4-permission-lost",
			Content:    []byte("Internal Q3 launch brief: rollout schedule, owner assignments, success metrics."),
			Shared:     true,
		},
	})

	provider := newScope2GoogleProvider(fixtureServer, pool)
	connectionID := createScope2Connection(t, pool, fixtureServer, provider, fixtureScope("root"))

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	store := smscan.NewPostgresStore(pool)
	if _, err := smscan.NewService(provider, store).InitialScan(ctx, connectionID); err != nil {
		t.Fatalf("InitialScan: %v", err)
	}
	processor := driveextract.NewService(provider, driveextract.NewPostgresStore(pool), driveextract.NewRuleBasedWorker())
	if _, err := processor.ProcessPending(ctx, connectionID); err != nil {
		t.Fatalf("ProcessPending: %v", err)
	}

	// Mark the trashed file tombstoned and the brief as permission-lost
	// using the canonical scan store API.
	if err := store.MarkRemoved(ctx, connectionID, "scope4-trashed", smdrive.ChangeTrash); err != nil {
		t.Fatalf("MarkRemoved trash: %v", err)
	}
	if err := store.MarkRemoved(ctx, connectionID, "scope4-permission-lost", smdrive.ChangePermLost); err != nil {
		t.Fatalf("MarkRemoved permission_lost: %v", err)
	}

	// Search MUST still return both artifacts so the retained knowledge
	// remains queryable. Drive a text-fallback search via SearchEngine
	// directly so this test does not depend on an embedding sidecar.
	// Use targeted single-fixture queries because websearch_to_tsquery
	// ANDs query tokens; the integration test must not rely on every
	// fixture sharing every term.
	engine := &api.SearchEngine{Pool: pool}
	trashedResults, _, _, err := engine.Search(ctx, api.SearchRequest{Query: "airport lunch", Limit: 10})
	if err != nil {
		t.Fatalf("Search trashed: %v", err)
	}
	permLostResults, _, _, err := engine.Search(ctx, api.SearchRequest{Query: "rollout brief", Limit: 10})
	if err != nil {
		t.Fatalf("Search permission_lost: %v", err)
	}
	var trashed, permissionLost *api.SearchResult
	for i := range trashedResults {
		if trashedResults[i].ArtifactID == "drive:google:"+connectionID+":scope4-trashed" {
			trashed = &trashedResults[i]
		}
	}
	for i := range permLostResults {
		if permLostResults[i].ArtifactID == "drive:google:"+connectionID+":scope4-permission-lost" {
			permissionLost = &permLostResults[i]
		}
	}
	if trashed == nil {
		t.Fatalf("tombstoned artifact dropped from search: %+v", trashedResults)
	}
	if permissionLost == nil {
		t.Fatalf("permission-lost artifact dropped from search: %+v", permLostResults)
	}
	if trashed.Drive == nil || trashed.Drive.Availability != "tombstoned" || !trashed.Drive.Tombstoned || trashed.Drive.ActionsEnabled {
		t.Fatalf("tombstoned search metadata wrong: %+v", trashed.Drive)
	}
	if permissionLost.Drive == nil || permissionLost.Drive.Availability != "permission_lost" || !permissionLost.Drive.PermissionLost || permissionLost.Drive.ActionsEnabled {
		t.Fatalf("permission-lost search metadata wrong: %+v", permissionLost.Drive)
	}

	// Detail endpoint MUST surface the banner, suppress extracted_text,
	// and keep the summary populated so the queryable knowledge remains
	// visible in Screen 6.
	trashedDetail, err := api.LoadDriveArtifactDetail(ctx, pool, "drive:google:"+connectionID+":scope4-trashed")
	if err != nil {
		t.Fatalf("LoadDriveArtifactDetail trashed: %v", err)
	}
	if trashedDetail.Drive.Availability != "tombstoned" || trashedDetail.BannerSeverity != "warning" {
		t.Fatalf("trashed detail availability/banner wrong: %+v", trashedDetail)
	}
	if trashedDetail.BannerMessage == "" || !strings.Contains(strings.ToLower(trashedDetail.BannerMessage), "trashed") {
		t.Fatalf("trashed banner message wrong: %q", trashedDetail.BannerMessage)
	}
	if trashedDetail.ExtractedText != "" {
		t.Fatalf("trashed detail still served extracted bytes (extracted_text non-empty): %q", trashedDetail.ExtractedText)
	}
	if trashedDetail.Summary == "" {
		t.Fatalf("trashed detail dropped queryable summary; retained knowledge MUST remain visible")
	}

	permLostDetail, err := api.LoadDriveArtifactDetail(ctx, pool, "drive:google:"+connectionID+":scope4-permission-lost")
	if err != nil {
		t.Fatalf("LoadDriveArtifactDetail permission_lost: %v", err)
	}
	if permLostDetail.Drive.Availability != "permission_lost" || permLostDetail.BannerSeverity != "warning" {
		t.Fatalf("permission_lost detail availability/banner wrong: %+v", permLostDetail)
	}
	if permLostDetail.BannerMessage == "" || !strings.Contains(strings.ToLower(permLostDetail.BannerMessage), "permission") {
		t.Fatalf("permission_lost banner message wrong: %q", permLostDetail.BannerMessage)
	}
	if permLostDetail.ExtractedText != "" {
		t.Fatalf("permission_lost detail still served extracted bytes: %q", permLostDetail.ExtractedText)
	}
	if permLostDetail.Summary == "" {
		t.Fatalf("permission_lost detail dropped queryable summary; retained knowledge MUST remain visible")
	}
}
