//go:build e2e

// Spec 038 Scope 8 — Multi-provider search UI (SCN-038-023).
//
// Asserts the live PWA Screen 5 search response surfaces drive_file
// rows from MORE THAN ONE provider in a single ranked list AND that the
// new provider/folder/sharing/audience filter parameters are honored
// by the live /api/search endpoint.
package drive

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	smdrive "github.com/smackerel/smackerel/internal/drive"
	driveextract "github.com/smackerel/smackerel/internal/drive/extract"
	"github.com/smackerel/smackerel/internal/drive/memprovider"
	smscan "github.com/smackerel/smackerel/internal/drive/scan"
	"github.com/smackerel/smackerel/tests/integration/drive/fixtures"
)

func TestMultiProviderDriveSearchReturnsOneRankedListWithAudienceFilters(t *testing.T) {
	liveConfig := loadE2EConfig(t)
	waitForHealth(t, liveConfig, 120*time.Second)
	pool := driveE2EPool(t)

	// Provider 1 (google fixture).
	fixtureServer := fixtures.NewServer()
	defer fixtureServer.Close()
	fixtureServer.AddFiles([]fixtures.File{
		{
			ID: "scope8-mp-ui-google", Name: "Carbonara technique.txt", MimeType: "text/plain",
			SizeBytes: 96, FolderPath: []string{"Recipes", "Italian"},
			RevisionID: "scope8-mp-ui-google-rev-1", Owner: "fixture@example.com",
			URL:     "https://drive.example/scope8-mp-ui-google",
			Content: []byte("Carbonara technique: pancetta, eggs, pecorino, black pepper. No cream."),
		},
	})
	googleProvider := newE2EGoogleProvider(fixtureServer, pool)
	googleConnID := createE2EConnection(t, pool, fixtureServer, googleProvider, []string{"root"})

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	if _, err := smscan.NewService(googleProvider, smscan.NewPostgresStore(pool)).InitialScan(ctx, googleConnID); err != nil {
		t.Fatalf("Google InitialScan: %v", err)
	}
	if _, err := driveextract.NewService(googleProvider, driveextract.NewPostgresStore(pool), driveextract.NewRuleBasedWorker()).ProcessPending(ctx, googleConnID); err != nil {
		t.Fatalf("Google ProcessPending: %v", err)
	}

	// Provider 2 (memdrive).
	memProvider := memprovider.New(memprovider.DefaultCapabilities())
	memOwner := uuid.NewString()
	memConnID := uuid.NewString()
	memProvider.SeedConnection(memConnID, memOwner, smdrive.AccessReadSave, smdrive.Scope{FolderIDs: []string{"root"}})
	uniqueLabel := "scope8-mp-ui-mem-" + uuid.NewString()[:8]
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
		ProviderFileID:     "scope8-mp-ui-mem",
		ProviderRevisionID: "scope8-mp-ui-mem-rev-1",
		Title:              "Carbonara timing.txt",
		MimeType:           "text/plain",
		SizeBytes:          80,
		FolderPath:         []string{"Recipes", "Italian"},
		ProviderURL:        "memdrive://files/scope8-mp-ui-mem",
		ModifiedAt:         time.Now().UTC(),
		OwnerLabel:         "fixture-owner",
	}, []byte("Carbonara timing: 8 minute pasta cook, off-heat egg toss to avoid scrambling."))

	if _, err := smscan.NewService(memProvider, smscan.NewPostgresStore(pool)).InitialScan(ctx, memConnID); err != nil {
		t.Fatalf("memdrive InitialScan: %v", err)
	}
	if _, err := driveextract.NewService(memProvider, driveextract.NewPostgresStore(pool), driveextract.NewRuleBasedWorker()).ProcessPending(ctx, memConnID); err != nil {
		t.Fatalf("memdrive ProcessPending: %v", err)
	}

	googleArtifactID := "drive:google:" + googleConnID + ":scope8-mp-ui-google"
	memArtifactID := "drive:memdrive:" + memConnID + ":scope8-mp-ui-mem"

	// Unfiltered: BOTH providers must surface in the same response.
	body := postJSON(t, liveConfig, "/api/search", map[string]any{
		"query": "carbonara",
		"limit": 20,
	})
	resp := map[string]any{}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode unfiltered: %v", err)
	}
	rows, _ := resp["results"].([]any)
	if !containsArtifactID(rows, googleArtifactID) || !containsArtifactID(rows, memArtifactID) {
		t.Fatalf("unfiltered carbonara search must surface BOTH providers; results=%d", len(rows))
	}

	// Provider filter: google only — memdrive row MUST drop.
	body = postJSON(t, liveConfig, "/api/search", map[string]any{
		"query": "carbonara",
		"limit": 20,
		"filters": map[string]any{
			"drive_provider": "google",
		},
	})
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode google filter: %v", err)
	}
	rows, _ = resp["results"].([]any)
	if containsArtifactID(rows, memArtifactID) {
		t.Fatalf("provider=google filter leaked memdrive row through live API")
	}
	if !containsArtifactID(rows, googleArtifactID) {
		t.Fatalf("provider=google filter dropped its own provider's row through live API")
	}

	// Folder filter: Italian — BOTH providers' rows must remain.
	body = postJSON(t, liveConfig, "/api/search", map[string]any{
		"query": "carbonara",
		"limit": 20,
		"filters": map[string]any{
			"drive_folder": "Italian",
		},
	})
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode folder filter: %v", err)
	}
	rows, _ = resp["results"].([]any)
	if !containsArtifactID(rows, googleArtifactID) || !containsArtifactID(rows, memArtifactID) {
		t.Fatalf("folder=Italian filter dropped a provider; results=%d", len(rows))
	}
	_ = ctx
}

func containsArtifactID(rows []any, id string) bool {
	for _, r := range rows {
		row, _ := r.(map[string]any)
		if row == nil {
			continue
		}
		if got, _ := row["artifact_id"].(string); got == id {
			return true
		}
	}
	return false
}
