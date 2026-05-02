//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
	photolib "github.com/smackerel/smackerel/internal/connector/photos"
	"github.com/smackerel/smackerel/internal/connector/photos/adapters/photoprism"
)

// TestPhotosCapability_UnsupportedOperationIs409AndNonMutating
// proves that invoking an UNSUPPORTED capability against a real
// PhotoPrism connector (a) returns the typed
// `*ProviderLimitationError`, (b) carries the canonical
// `LimitationCode` from the shared taxonomy registry, and (c) does
// NOT mutate any persistent photo store row. SCN-040-013 (capability
// matrix governance).
func TestPhotosCapability_UnsupportedOperationIs409AndNonMutating(t *testing.T) {
	pool := testPool(t)
	store := photolib.NewStore(pool)
	connectorID := "scope-040-photoprism-cap-" + testID(t)
	cleanupPhotosByConnector(t, pool, connectorID)
	t.Cleanup(func() { cleanupPhotosByConnector(t, pool, connectorID) })

	fixture := newIntegrationPhotoprismFixture(t, []photoprism.Photo{
		integrationPhotoprismPhoto("vacation-001", "Vacation", "fixturehash-vacation-001"),
	})
	client := photoprism.NewClient(fixture.Client())
	if err := client.Connect(context.Background(), connector.ConnectorConfig{
		AuthType:     "api_token",
		Credentials:  map[string]string{"api_token": fixture.APIToken()},
		SourceConfig: map[string]any{"base_url": fixture.URL()},
	}); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// Persist a baseline row so we can prove the writer call did NOT
	// touch it. The row is created via the store directly because we
	// need a deterministic photo_id to assert against.
	captured := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	bytes := int64(2_345_678)
	record, err := store.PublishPhotoEvent(context.Background(), connectorID, "photoprism", photolib.PhotoEvent{
		ProviderRef:       "vacation-001",
		Operation:         photolib.PhotoOpUpsert,
		ProviderMediaKind: "image",
		MediaRole:         photolib.MediaRoleCameraOriginal,
		ContentHash:       "sha1:fixturehash-vacation-001",
		Bytes:             &bytes,
		MIMEType:          "image/jpeg",
		Filename:          "vacation-001.jpg",
		CapturedAt:        captured,
		Sensitivity:       photolib.ProviderSensitivity{Level: photolib.SensitivityNone, Source: "photoprism:inferred-locally"},
		RawProvider:       map[string]any{"provider": "photoprism", "uid": "vacation-001"},
	})
	if err != nil {
		t.Fatalf("PublishPhotoEvent baseline: %v", err)
	}
	baselineJSON, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("marshal baseline: %v", err)
	}

	// Exercise faces_write via the writer — PhotoPrism marks it
	// UNSUPPORTED, so the typed error MUST carry the registered
	// limitation code.
	err = client.Writer().RenameFaceCluster(context.Background(), "subj-001", "Maria")
	if err == nil {
		t.Fatalf("RenameFaceCluster expected ProviderLimitationError, got nil")
	}
	var limit *photoprism.ProviderLimitationError
	if !errors.As(err, &limit) {
		t.Fatalf("RenameFaceCluster error type = %T, want *ProviderLimitationError", err)
	}
	if limit.LimitationCode != photolib.LimitationFacesWriteNotSupported {
		t.Fatalf("LimitationCode = %q, want %q", limit.LimitationCode, photolib.LimitationFacesWriteNotSupported)
	}

	// Adversarial: the persisted photo MUST NOT have been touched
	// by the failed writer call. Re-fetch and compare the full
	// JSON body byte-for-byte to detect ANY mutation.
	again, err := store.GetByID(context.Background(), record.ID)
	if err != nil {
		t.Fatalf("GetByID after writer attempt: %v", err)
	}
	againJSON, err := json.Marshal(again)
	if err != nil {
		t.Fatalf("marshal post-attempt: %v", err)
	}
	if string(baselineJSON) != string(againJSON) {
		t.Fatalf("photo record changed after refused write:\n  before=%s\n  after =%s", baselineJSON, againJSON)
	}
}
