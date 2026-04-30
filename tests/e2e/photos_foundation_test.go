//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	photolib "github.com/smackerel/smackerel/internal/connector/photos"
)

func TestPhotosFoundation_E2E_SyntheticPhotoDetailFromLiveAPI(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)

	pool := photosE2EPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	event := photolib.SyntheticPhotoEvent()
	event.ProviderRef = "e2e-" + strings.ReplaceAll(t.Name(), "/", "-") + "-040"
	event.ContentHash = "sha256:" + event.ProviderRef
	store := photolib.NewStore(pool)
	record, err := store.PublishPhotoEvent(ctx, "e2e-connector-040", "synthetic", event)
	if err != nil {
		t.Fatalf("PublishPhotoEvent: %v", err)
	}
	cleanupE2EPhoto(t, pool, record.ArtifactID)

	connectorsResp, err := apiGet(cfg, "/v1/photos/connectors")
	if err != nil {
		t.Fatalf("GET /v1/photos/connectors: %v", err)
	}
	connectorsBody, err := readBody(connectorsResp)
	if err != nil {
		t.Fatalf("read connectors body: %v", err)
	}
	if connectorsResp.StatusCode != http.StatusOK {
		t.Fatalf("connectors status=%d body=%s", connectorsResp.StatusCode, string(connectorsBody))
	}
	var connectors struct {
		Connectors []struct {
			Provider     string   `json:"provider"`
			Enabled      bool     `json:"enabled"`
			Capabilities []string `json:"capabilities"`
		} `json:"connectors"`
	}
	if err := json.Unmarshal(connectorsBody, &connectors); err != nil {
		t.Fatalf("decode connectors body: %v body=%s", err, string(connectorsBody))
	}
	if len(connectors.Connectors) == 0 {
		t.Fatalf("connectors response has no providers: %s", string(connectorsBody))
	}
	if connectors.Connectors[0].Provider != "immich" {
		t.Fatalf("provider = %q, want immich; body=%s", connectors.Connectors[0].Provider, string(connectorsBody))
	}

	photoResp, err := apiGet(cfg, "/v1/photos/"+record.ID.String())
	if err != nil {
		t.Fatalf("GET /v1/photos/{id}: %v", err)
	}
	photoBody, err := readBody(photoResp)
	if err != nil {
		t.Fatalf("read photo body: %v", err)
	}
	if photoResp.StatusCode != http.StatusOK {
		t.Fatalf("photo status=%d body=%s", photoResp.StatusCode, string(photoBody))
	}
	var parsed struct {
		Photo struct {
			ID          string `json:"photo_id"`
			ArtifactID  string `json:"artifact_id"`
			Provider    string `json:"provider"`
			ProviderRef string `json:"provider_ref"`
			MediaRole   string `json:"media_role"`
			RawProvider json.RawMessage `json:"raw_provider"`
		} `json:"photo"`
	}
	if err := json.Unmarshal(photoBody, &parsed); err != nil {
		t.Fatalf("decode photo body: %v body=%s", err, string(photoBody))
	}
	if parsed.Photo.ID != record.ID.String() || parsed.Photo.ArtifactID != record.ArtifactID {
		t.Fatalf("identity mismatch: got id=%q artifact=%q want id=%q artifact=%q body=%s", parsed.Photo.ID, parsed.Photo.ArtifactID, record.ID.String(), record.ArtifactID, string(photoBody))
	}
	if parsed.Photo.Provider != "synthetic" || parsed.Photo.ProviderRef != event.ProviderRef {
		t.Fatalf("provider shape mismatch: provider=%q ref=%q body=%s", parsed.Photo.Provider, parsed.Photo.ProviderRef, string(photoBody))
	}
	if parsed.Photo.MediaRole != string(photolib.MediaRoleCameraOriginal) {
		t.Fatalf("media_role=%q, want %q", parsed.Photo.MediaRole, photolib.MediaRoleCameraOriginal)
	}
	if strings.Contains(string(parsed.Photo.RawProvider), "provider_specific") {
		t.Fatalf("raw_provider leaked provider_specific marker: %s", string(parsed.Photo.RawProvider))
	}
}

func photosE2EPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("e2e: DATABASE_URL not set — live stack DB not available")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect e2e database: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

func cleanupE2EPhoto(t *testing.T, pool *pgxpool.Pool, artifactID string) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := pool.Exec(ctx, `DELETE FROM artifacts WHERE id=$1`, artifactID); err != nil {
			t.Logf("cleanup e2e photo artifact %s failed: %v", artifactID, err)
		}
	})
}
