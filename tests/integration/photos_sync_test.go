//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	photolib "github.com/smackerel/smackerel/internal/connector/photos"
	"github.com/smackerel/smackerel/internal/connector/photos/adapters/immich"
)

func TestPhotosImmich_IncrementalChangesUpdateState(t *testing.T) {
	pool := testPool(t)
	store := photolib.NewStore(pool)
	connectorID := "scope-040-immich-sync-" + testID(t)
	cleanupPhotosByConnector(t, pool, connectorID)

	initial := immichFixtureAsset("sync-move-001", "album-unsorted", "Unsorted", "whiteboard diagram already classified")
	fixture := newImmichFixtureServer(t, immichFixtureData{Assets: []immich.Asset{initial}})
	client := immich.NewClient(fixture.Client())
	connectImmichFixture(t, client, fixture)
	scanner := photolib.NewScanner(store, photolib.ScannerConfig{MaxFileSizeBytes: 200_000_000})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := scanner.Scan(ctx, client, connectorID, photolib.Scope{IncludedAlbums: []string{"album-unsorted"}}); err != nil {
		t.Fatalf("initial Scan: %v", err)
	}

	moved := initial
	moved.Albums = []immich.AlbumRef{{ID: "album-lisbon", Name: "Lisbon 2026"}}
	moved.Classification = nil
	deleted := immich.Asset{ID: "sync-delete-001", Deleted: true}
	newUpload := immichFixtureAsset("sync-new-upload-001", "album-lisbon", "Lisbon 2026", "new upload serial number on dishwasher")
	fixture.SetChanges("cursor-1", []immich.Asset{moved, deleted, newUpload})

	result, err := scanner.Monitor(ctx, client, connectorID, "cursor-1")
	if err != nil {
		t.Fatalf("Monitor: %v", err)
	}
	if result.ReusedClassificationCount != 1 {
		t.Fatalf("ReusedClassificationCount = %d, want 1", result.ReusedClassificationCount)
	}
	if result.ClassifiedCount != 1 {
		t.Fatalf("ClassifiedCount = %d, want 1 for the new upload", result.ClassifiedCount)
	}
	if result.TombstonedCount != 1 {
		t.Fatalf("TombstonedCount = %d, want 1", result.TombstonedCount)
	}

	movedRecord, err := store.GetByProviderRef(context.Background(), "immich", "sync-move-001")
	if err != nil {
		t.Fatalf("GetByProviderRef moved: %v", err)
	}
	if !containsStringValue(movedRecord.Albums, "Lisbon 2026") {
		t.Fatalf("moved albums = %#v, want Lisbon 2026", movedRecord.Albums)
	}
	if !strings.Contains(string(movedRecord.Classification), "already classified") {
		t.Fatalf("classification was rerun or dropped after album move: %s", string(movedRecord.Classification))
	}

	deletedRecord, err := store.GetByProviderRef(context.Background(), "immich", "sync-delete-001")
	if err != nil {
		t.Fatalf("GetByProviderRef deleted: %v", err)
	}
	if deletedRecord.LifecycleState != "deleted" {
		t.Fatalf("deleted lifecycle_state = %q, want deleted", deletedRecord.LifecycleState)
	}
}

func containsStringValue(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}