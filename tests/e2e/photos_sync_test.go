//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	photolib "github.com/smackerel/smackerel/internal/connector/photos"
)

func TestPhotosSync_E2E_AlbumMoveDoesNotReclassify(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)
	pool := photosE2EPool(t)
	store := photolib.NewStore(pool)
	connectorID := "e2e-immich-sync-" + strings.ReplaceAll(t.Name(), "/", "-")
	record := seedE2EImmichPhoto(t, store, connectorID, "e2e-sync-move-001", "already classified whiteboard diagram")
	cleanupE2EPhoto(t, pool, record.ArtifactID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	event := photolib.SyntheticPhotoEvent()
	event.ProviderRef = "e2e-sync-move-001"
	event.ContentHash = "sha256:e2e-sync-move-001"
	event.RawProvider = map[string]any{"provider": "immich", "asset_id": "e2e-sync-move-001"}
	event.Albums = []string{"Lisbon 2026"}
	event.Filename = "e2e-sync-move-001.jpg"
	scanner := photolib.NewScanner(store, photolib.ScannerConfig{MaxFileSizeBytes: 200_000_000})
	result, err := scanner.ProcessEvent(ctx, connectorID, event)
	if err != nil {
		t.Fatalf("ProcessEvent album move: %v", err)
	}
	if result.ReusedClassificationCount != 1 || result.ClassifiedCount != 0 {
		t.Fatalf("album move result = %+v, want reused=1 classified=0", result)
	}

	resp, err := apiGet(cfg, "/v1/photos/"+record.ID.String())
	if err != nil {
		t.Fatalf("GET /v1/photos/{id}: %v", err)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read detail body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("detail status=%d body=%s", resp.StatusCode, string(body))
	}
	var parsed struct {
		Photo struct {
			Albums         []string        `json:"albums"`
			Classification json.RawMessage `json:"classification"`
		} `json:"photo"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("decode detail body: %v body=%s", err, string(body))
	}
	if !stringSliceContains(parsed.Photo.Albums, "Lisbon 2026") {
		t.Fatalf("albums = %#v, want Lisbon 2026", parsed.Photo.Albums)
	}
	if !strings.Contains(string(parsed.Photo.Classification), "already classified") {
		t.Fatalf("classification was lost or rerun: %s", string(parsed.Photo.Classification))
	}
}

func stringSliceContains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
