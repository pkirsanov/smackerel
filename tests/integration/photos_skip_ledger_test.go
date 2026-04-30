//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	photolib "github.com/smackerel/smackerel/internal/connector/photos"
	"github.com/smackerel/smackerel/internal/connector/photos/adapters/immich"
)

func TestPhotosImmich_SkipLedgerVisibleAndRetryable(t *testing.T) {
	pool := testPool(t)
	store := photolib.NewStore(pool)
	connectorID := "scope-040-immich-skips-" + testID(t)
	cleanupPhotosByConnector(t, pool, connectorID)

	largeSize := int64(201_000_000)
	fixture := newImmichFixtureServer(t, immichFixtureData{Assets: []immich.Asset{
		{ID: "skip-too-large", Type: "IMAGE", OriginalFileName: "too-large.tif", MIMEType: "image/tiff", Checksum: "sha256:large", Size: &largeSize, FileCreatedAt: "2026-03-14T14:08:00Z"},
		{ID: "skip-unsupported", Type: "IMAGE", OriginalFileName: "unsupported.bin", MIMEType: "application/octet-stream", Checksum: "sha256:unsupported", FileCreatedAt: "2026-03-14T14:08:00Z"},
		immichSkipAsset("skip-permission", "permission_denied"),
		immichSkipAsset("skip-provider", "provider_error"),
		immichSkipAsset("skip-extraction", "extraction_failed"),
	}})
	client := immich.NewClient(fixture.Client())
	connectImmichFixture(t, client, fixture)
	scanner := photolib.NewScanner(store, photolib.ScannerConfig{MaxFileSizeBytes: 200_000_000})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := scanner.Scan(ctx, client, connectorID, photolib.Scope{})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	assertSkipReason(t, result.Skips, "too_large", "skip-too-large")
	assertSkipReason(t, result.Skips, "unsupported_format", "skip-unsupported")
	assertSkipReason(t, result.Skips, "permission_denied", "skip-permission")
	assertSkipReason(t, result.Skips, "provider_error", "skip-provider")
	assertSkipReason(t, result.Skips, "extraction_failed", "skip-extraction")

	state, err := store.GetConnectorState(context.Background(), connectorID)
	if err != nil {
		t.Fatalf("GetConnectorState: %v", err)
	}
	if len(state.Skips) != 5 {
		t.Fatalf("persisted skips = %d, want 5: %+v", len(state.Skips), state.Skips)
	}
	for _, entry := range state.Skips {
		if entry.RetryToken == "" {
			t.Fatalf("skip entry missing retry token: %+v", entry)
		}
		if len(entry.FileIdentities) == 0 {
			t.Fatalf("skip entry missing file identity: %+v", entry)
		}
	}
}

func immichSkipAsset(id string, reason string) immich.Asset {
	size := int64(128_000)
	return immich.Asset{
		ID:               id,
		Type:             "IMAGE",
		OriginalFileName: id + ".jpg",
		MIMEType:         "image/jpeg",
		Checksum:         "sha256:" + id,
		Size:             &size,
		FileCreatedAt:    "2026-03-14T14:08:00Z",
		SkipReason:       reason,
	}
}

func assertSkipReason(t *testing.T, skips []photolib.SkipEntry, reason string, fileID string) {
	t.Helper()
	for _, entry := range skips {
		if entry.Reason != reason {
			continue
		}
		if entry.Count != 1 {
			t.Fatalf("skip %s count = %d, want 1", reason, entry.Count)
		}
		if !containsStringValue(entry.FileIdentities, fileID) {
			t.Fatalf("skip %s identities = %#v, want %s", reason, entry.FileIdentities, fileID)
		}
		if entry.RetryToken == "" {
			t.Fatalf("skip %s missing retry token", reason)
		}
		return
	}
	t.Fatalf("missing skip reason %q in %+v", reason, skips)
}