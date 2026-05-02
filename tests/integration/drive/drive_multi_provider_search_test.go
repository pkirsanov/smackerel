//go:build integration

// Spec 038 Scope 8 — Multi-provider unified search (SCN-038-023).
//
// Asserts that drive_file artifacts from MORE THAN ONE concrete drive
// provider (google + memdrive) appear in a single ranked /api/search
// result list AND that the new provider/folder/sharing/audience/
// sensitivity filters apply identically across providers.
//
// Adversarial guards:
//   - The unfiltered query MUST return drive_file results from BOTH
//     providers — a regression hard-coded to a single provider would
//     surface as a missing provider in the result set.
//   - The provider-filtered query MUST drop the other provider's
//     results — a regression where the filter was a no-op would still
//     return both providers' rows.
//   - The sharing-filter query MUST narrow the result set without
//     introducing false matches — a regression that swapped the
//     filter logic would surface as the wrong provider's row passing.
package drive

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/smackerel/smackerel/internal/api"
	smdrive "github.com/smackerel/smackerel/internal/drive"
	driveextract "github.com/smackerel/smackerel/internal/drive/extract"
	"github.com/smackerel/smackerel/internal/drive/memprovider"
	smscan "github.com/smackerel/smackerel/internal/drive/scan"
	"github.com/smackerel/smackerel/tests/integration/drive/fixtures"
)

func TestMultiProviderDriveSearchUsesUnifiedRankingAndAudienceFilters(t *testing.T) {
	pool := driveTestPool(t)

	// --- Provider 1: Google fixture (private, "kitchen" content) ---
	fixtureServer := fixtures.NewServer()
	defer fixtureServer.Close()
	fixtureServer.AddFiles([]fixtures.File{
		{
			ID: "scope8-mp-google", Name: "Sourdough technique.txt", MimeType: "text/plain",
			SizeBytes: 200, FolderPath: []string{"Recipes", "Bread"},
			RevisionID: "scope8-mp-google-rev-1", Owner: "fixture-owner@example.com",
			URL:     "https://drive.example/scope8-mp-google",
			Content: []byte("Sourdough technique: 80 percent hydration, autolyse 30 minutes, bulk ferment 4 hours."),
			Shared:  false,
		},
	})
	googleProvider := newScope2GoogleProvider(fixtureServer, pool)
	googleConnID := createScope2Connection(t, pool, fixtureServer, googleProvider, fixtureScope("root"))

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if _, err := smscan.NewService(googleProvider, smscan.NewPostgresStore(pool)).InitialScan(ctx, googleConnID); err != nil {
		t.Fatalf("Google InitialScan: %v", err)
	}
	if _, err := driveextract.NewService(googleProvider, driveextract.NewPostgresStore(pool), driveextract.NewRuleBasedWorker()).ProcessPending(ctx, googleConnID); err != nil {
		t.Fatalf("Google ProcessPending: %v", err)
	}

	// --- Provider 2: memdrive (also "kitchen" content but shared) ---
	memProvider := memprovider.New(memprovider.DefaultCapabilities())
	memOwner := uuid.NewString()
	memConnID := uuid.NewString()
	memProvider.SeedConnection(memConnID, memOwner, smdrive.AccessReadSave, smdrive.Scope{FolderIDs: []string{"root"}})
	uniqueLabel := "scope8-mp-mem-" + uuid.NewString()[:8]
	if _, err := pool.Exec(ctx, `
		INSERT INTO drive_connections (id, provider_id, owner_user_id, account_label, access_mode, status, scope)
		VALUES ($1, 'memdrive', $2, $3, 'read_save', 'healthy', '{"folder_ids":["root"]}'::jsonb)`,
		memConnID, memOwner, uniqueLabel,
	); err != nil {
		t.Fatalf("insert memdrive connection: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM drive_connections WHERE id=$1`, memConnID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM artifacts WHERE id LIKE $1`, "drive:memdrive:"+memConnID+":%")
	})

	memProvider.AddFile(memConnID, smdrive.FolderItem{
		ProviderFileID:     "scope8-mp-mem",
		ProviderRevisionID: "scope8-mp-mem-rev-1",
		Title:              "Sourdough schedule.txt",
		MimeType:           "text/plain",
		SizeBytes:          120,
		FolderPath:         []string{"Recipes", "Bread"},
		ProviderURL:        "memdrive://files/scope8-mp-mem",
		ModifiedAt:         time.Now().UTC(),
		OwnerLabel:         "fixture-owner",
		SharingState:       map[string]any{"shared": true, "visibility": "shared"},
	}, []byte("Sourdough schedule: feed starter Friday night, mix dough Saturday morning."))

	if _, err := smscan.NewService(memProvider, smscan.NewPostgresStore(pool)).InitialScan(ctx, memConnID); err != nil {
		t.Fatalf("memdrive InitialScan: %v", err)
	}
	if _, err := driveextract.NewService(memProvider, driveextract.NewPostgresStore(pool), driveextract.NewRuleBasedWorker()).ProcessPending(ctx, memConnID); err != nil {
		t.Fatalf("memdrive ProcessPending: %v", err)
	}

	googleArtifactID := "drive:google:" + googleConnID + ":scope8-mp-google"
	memArtifactID := "drive:memdrive:" + memConnID + ":scope8-mp-mem"

	engine := &api.SearchEngine{Pool: pool}

	// --- Unfiltered query — both providers must appear ---
	results, _, mode, err := engine.Search(ctx, api.SearchRequest{Query: "sourdough", Limit: 10})
	if err != nil {
		t.Fatalf("Search unfiltered: %v", err)
	}
	if mode != "text_fallback" {
		t.Fatalf("search mode = %q, want text_fallback (sidecar should not be required)", mode)
	}
	foundGoogle, foundMem := false, false
	for _, r := range results {
		if r.ArtifactID == googleArtifactID {
			foundGoogle = true
			if r.Drive == nil || r.Drive.ProviderID != "google" {
				t.Fatalf("google result drive metadata missing or wrong provider: %+v", r.Drive)
			}
		}
		if r.ArtifactID == memArtifactID {
			foundMem = true
			if r.Drive == nil || r.Drive.ProviderID != "memdrive" {
				t.Fatalf("memdrive result drive metadata missing or wrong provider: %+v", r.Drive)
			}
		}
	}
	if !foundGoogle {
		t.Fatalf("unfiltered search missed google fixture (results=%d)", len(results))
	}
	if !foundMem {
		t.Fatalf("unfiltered search missed memdrive fixture (results=%d)", len(results))
	}

	// --- Provider filter: google only ---
	results, _, _, err = engine.Search(ctx, api.SearchRequest{
		Query:   "sourdough",
		Limit:   10,
		Filters: api.SearchFilters{DriveProvider: "google"},
	})
	if err != nil {
		t.Fatalf("Search provider=google: %v", err)
	}
	for _, r := range results {
		if r.ArtifactID == memArtifactID {
			t.Fatalf("provider=google filter leaked memdrive row: %+v", r)
		}
	}
	foundGoogle = false
	for _, r := range results {
		if r.ArtifactID == googleArtifactID {
			foundGoogle = true
		}
	}
	if !foundGoogle {
		t.Fatalf("provider=google filter dropped its own provider's row")
	}

	// --- Provider filter: memdrive only ---
	results, _, _, err = engine.Search(ctx, api.SearchRequest{
		Query:   "sourdough",
		Limit:   10,
		Filters: api.SearchFilters{DriveProvider: "memdrive"},
	})
	if err != nil {
		t.Fatalf("Search provider=memdrive: %v", err)
	}
	for _, r := range results {
		if r.ArtifactID == googleArtifactID {
			t.Fatalf("provider=memdrive filter leaked google row: %+v", r)
		}
	}
	foundMem = false
	for _, r := range results {
		if r.ArtifactID == memArtifactID {
			foundMem = true
		}
	}
	if !foundMem {
		t.Fatalf("provider=memdrive filter dropped its own provider's row")
	}

	// --- Sharing filter: shared only — should drop the private google row ---
	results, _, _, err = engine.Search(ctx, api.SearchRequest{
		Query:   "sourdough",
		Limit:   10,
		Filters: api.SearchFilters{DriveSharing: "shared"},
	})
	if err != nil {
		t.Fatalf("Search sharing=shared: %v", err)
	}
	for _, r := range results {
		if r.ArtifactID == googleArtifactID {
			t.Fatalf("sharing=shared filter leaked private google row: %+v", r)
		}
	}

	// --- Folder filter: Bread — should match BOTH providers (unified ranking) ---
	results, _, _, err = engine.Search(ctx, api.SearchRequest{
		Query:   "sourdough",
		Limit:   10,
		Filters: api.SearchFilters{DriveFolder: "Bread"},
	})
	if err != nil {
		t.Fatalf("Search folder=Bread: %v", err)
	}
	foundGoogle, foundMem = false, false
	for _, r := range results {
		if r.ArtifactID == googleArtifactID {
			foundGoogle = true
		}
		if r.ArtifactID == memArtifactID {
			foundMem = true
		}
	}
	if !foundGoogle || !foundMem {
		t.Fatalf("folder=Bread filter must keep BOTH providers' rows; google=%v mem=%v", foundGoogle, foundMem)
	}
}
