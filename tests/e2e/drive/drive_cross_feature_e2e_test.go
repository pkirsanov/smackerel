//go:build e2e

// Spec 038 Scope 8 — Cross-Feature & Scale Convergence E2E
// (SCN-038-021, SCN-038-022, SCN-038-024).
//
// Live-stack proof that drive_file artifacts produced by ANY drive
// provider (google + memdrive) flow through the canonical artifact
// store and surface to downstream features (search/digest) without
// any consumer needing provider-specific code paths.
//
// The test runs against the real running stack — it boots fixtures,
// drives the live API, and asserts the live API responses use only
// provider-neutral fields.
package drive

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	smdrive "github.com/smackerel/smackerel/internal/drive"
	"github.com/smackerel/smackerel/internal/drive/consumers"
	driveextract "github.com/smackerel/smackerel/internal/drive/extract"
	"github.com/smackerel/smackerel/internal/drive/memprovider"
	smscan "github.com/smackerel/smackerel/internal/drive/scan"
	"github.com/smackerel/smackerel/tests/integration/drive/fixtures"
)

func TestDriveCrossFeatureE2E_ProviderNeutralConsumersAndProducers(t *testing.T) {
	liveConfig := loadE2EConfig(t)
	waitForHealth(t, liveConfig, 120*time.Second)
	pool := driveE2EPool(t)
	searchTerm := "drivecrossprovider" + strings.ReplaceAll(uuid.NewString(), "-", "")

	// Provider 1 (google fixture).
	fixtureServer := fixtures.NewServer()
	defer fixtureServer.Close()
	fixtureServer.AddFiles([]fixtures.File{
		{
			ID: "scope8-e2e-google", Name: searchTerm + " Google recipe.txt", MimeType: "text/plain",
			SizeBytes: 96, FolderPath: []string{"Recipes", "Salads"},
			RevisionID: "scope8-e2e-google-rev-1", Owner: "fixture-owner@example.com",
			URL:     "https://drive.example/scope8-e2e-google",
			Content: []byte(searchTerm + ": tomatoes, basil, olive oil. Action: buy basil."),
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
	uniqueLabel := "scope8-e2e-mem-" + uuid.NewString()[:8]
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
		ProviderFileID:     "scope8-e2e-mem",
		ProviderRevisionID: "scope8-e2e-mem-rev-1",
		Title:              searchTerm + " memdrive recipe.txt",
		MimeType:           "text/plain",
		SizeBytes:          96,
		FolderPath:         []string{"Recipes", "Salads"},
		ProviderURL:        "memdrive://files/scope8-e2e-mem",
		ModifiedAt:         time.Now().UTC(),
		OwnerLabel:         "fixture-owner",
	}, []byte(searchTerm+": tomatoes, mozzarella. Action: buy mozzarella."))

	if _, err := smscan.NewService(memProvider, smscan.NewPostgresStore(pool)).InitialScan(ctx, memConnID); err != nil {
		t.Fatalf("memdrive InitialScan: %v", err)
	}
	if _, err := driveextract.NewService(memProvider, driveextract.NewPostgresStore(pool), driveextract.NewRuleBasedWorker()).ProcessPending(ctx, memConnID); err != nil {
		t.Fatalf("memdrive ProcessPending: %v", err)
	}

	googleArtifactID := "drive:google:" + googleConnID + ":scope8-e2e-google"
	memArtifactID := "drive:memdrive:" + memConnID + ":scope8-e2e-mem"
	contaminantPrefix := "drive-search-contaminant-" + uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO artifacts
		 (id, artifact_type, title, summary, content_raw, content_hash, source_id,
		  source_ref, source_quality, processing_status, created_at, updated_at)
		SELECT $1 || '-' || sequence_number::text,
		       'note',
		       'Tomato salad',
		       'Earlier package search contender',
		       'Bounded search contamination fixture',
		       'hash-' || $1 || '-' || sequence_number::text,
		       'e2e-adversarial',
		       $1 || '-' || sequence_number::text,
		       'primary',
		       'completed',
		       now() + sequence_number * interval '1 millisecond',
		       now() + sequence_number * interval '1 millisecond'
		  FROM generate_series(1, 20) AS sequence_number`, contaminantPrefix,
	); err != nil {
		t.Fatalf("insert prior-package search contaminants: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM artifacts WHERE id LIKE $1`, contaminantPrefix+"%")
	})

	// Provider-neutral consumer surface: both providers must load identically.
	for _, artifactID := range []string{googleArtifactID, memArtifactID} {
		summary, err := consumers.LoadDriveArtifact(ctx, pool, artifactID)
		if err != nil {
			t.Fatalf("LoadDriveArtifact(%s): %v", artifactID, err)
		}
		if summary.ArtifactID != artifactID {
			t.Fatalf("summary.ArtifactID = %q, want %q", summary.ArtifactID, artifactID)
		}
		if !summary.IsAvailable {
			t.Fatalf("artifact %s should be available", artifactID)
		}
	}

	// Live API search — both providers' rows surface in /api/search response.
	body := postJSON(t, liveConfig, "/api/search", map[string]any{
		"query": searchTerm,
		"limit": 20,
	})
	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode search response: %v", err)
	}
	results, _ := resp["results"].([]any)
	foundGoogle, foundMem := false, false
	for _, r := range results {
		row, _ := r.(map[string]any)
		id, _ := row["artifact_id"].(string)
		if id == googleArtifactID {
			foundGoogle = true
			drive, _ := row["drive"].(map[string]any)
			if drive == nil || drive["provider_id"] != "google" {
				t.Fatalf("google result missing drive metadata or wrong provider: %+v", row)
			}
		}
		if id == memArtifactID {
			foundMem = true
			drive, _ := row["drive"].(map[string]any)
			if drive == nil || drive["provider_id"] != "memdrive" {
				t.Fatalf("memdrive result missing drive metadata or wrong provider: %+v", row)
			}
		}
	}
	if !foundGoogle || !foundMem {
		t.Fatalf("/api/search must return BOTH provider rows; google=%v mem=%v", foundGoogle, foundMem)
	}
}

func postJSON(t *testing.T, cfg e2eConfig, path string, payload map[string]any) []byte {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req, err := http.NewRequest("POST", cfg.CoreURL+path, strings.NewReader(string(body)))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	// Spec 044 Scope 02 — /api/* and /v1/* are behind
	// bearerAuthMiddleware; loadE2EConfig fails loud if
	// SMACKEREL_AUTH_TOKEN is unset, so the header is always populated.
	req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	respBody, err := readBody(resp)
	if err != nil {
		t.Fatalf("read POST %s: %v", path, err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST %s status=%d body=%s", path, resp.StatusCode, string(respBody))
	}
	return respBody
}
