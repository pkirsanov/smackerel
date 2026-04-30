//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	photolib "github.com/smackerel/smackerel/internal/connector/photos"
)

func TestPhotosPrivacyBoundary_ProviderSpecificBranchingIsRejected(t *testing.T) {
	pool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	event := photolib.SyntheticPhotoEvent()
	event.ProviderRef = testID(t)
	event.RawProvider["provider_specific"] = "immich-only-branch"
	store := photolib.NewStore(pool)
	_, err := store.PublishPhotoEvent(ctx, "connector-040", "synthetic", event)
	if err == nil {
		t.Fatal("PublishPhotoEvent accepted provider_specific raw provider branch; want rejection")
	}
	if !strings.Contains(err.Error(), "provider_specific") {
		t.Fatalf("error = %v, want provider_specific", err)
	}

	artifactID := photolib.ArtifactID("synthetic", event.ProviderRef)
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM photos WHERE artifact_id=$1`, artifactID).Scan(&count); err != nil {
		t.Fatalf("count photos: %v", err)
	}
	if count != 0 {
		t.Fatalf("rejected provider-specific event persisted %d rows, want 0", count)
	}
}

func TestPhotosPrivacyBoundaryRejectsUserLibraryURLs(t *testing.T) {
	envPath := integrationEnvFilePath(t)
	keys := loadIntegrationEnvFileKeys(t, envPath)
	baseURL := keys["PHOTOS_PROVIDER_IMMICH_BASE_URL"]
	for _, forbidden := range []string{"file://", "/home/", "/Users/", "~/", "Pictures", "Photos Library"} {
		if strings.Contains(baseURL, forbidden) {
			t.Fatalf("test photo provider base URL points at a user-owned library path %q via %s", baseURL, envPath)
		}
	}

	pool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	event := photolib.SyntheticPhotoEvent()
	event.ProviderRef = testID(t)
	event.RawProvider["provider_specific"] = "file:///home/example/Pictures/user-library"
	store := photolib.NewStore(pool)
	if _, err := store.PublishPhotoEvent(ctx, "connector-040", "synthetic", event); err == nil {
		t.Fatal("PublishPhotoEvent accepted provider-specific user library URL marker; want rejection")
	}
}

func TestPhotosPrivacyBoundary_StableSignalsDoNotPersistLLMDecision(t *testing.T) {
	pool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	event := photolib.SyntheticPhotoEvent()
	event.ProviderRef = testID(t)
	event.ContentHash = "sha256:" + strings.ReplaceAll(event.ProviderRef, "/", "-")
	store := photolib.NewStore(pool)
	record, err := store.PublishPhotoEvent(ctx, "connector-040", "synthetic", event)
	if err != nil {
		t.Fatalf("PublishPhotoEvent: %v", err)
	}
	cleanupPhoto(t, record.ArtifactID)

	var classification string
	var confidence *float64
	var rationale *string
	if err := pool.QueryRow(ctx, `SELECT classification::text, classification_confidence, classification_rationale FROM photos WHERE id=$1`, record.ID).Scan(&classification, &confidence, &rationale); err != nil {
		t.Fatalf("read classification boundary: %v", err)
	}
	if classification != "{}" {
		t.Fatalf("classification = %s, want {} until LLM decision arrives", classification)
	}
	if confidence != nil || rationale != nil {
		t.Fatalf("stable ingest wrote LLM decision evidence: confidence=%v rationale=%v", confidence, rationale)
	}
}
