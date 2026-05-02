//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/smackerel/smackerel/internal/connector"
	photolib "github.com/smackerel/smackerel/internal/connector/photos"
	"github.com/smackerel/smackerel/internal/connector/photos/adapters/immich"
	"github.com/smackerel/smackerel/internal/connector/photos/adapters/photoprism"
)

// TestPhotosProviderNeutrality_SecondAdapterMatchesImmichShape proves
// that the PhotoEvent shape produced by the PhotoPrism adapter is
// IDENTICAL to the Immich shape on every provider-neutral field. The
// downstream pipeline (dedupe, sensitivity, lifecycle, search) MUST
// be able to consume both adapters without branching. SCN-040-014
// (cross-provider search and dedupe).
func TestPhotosProviderNeutrality_SecondAdapterMatchesImmichShape(t *testing.T) {
	pool := testPool(t)
	store := photolib.NewStore(pool)
	connectorImmich := "scope-040-immich-neutrality-" + testID(t)
	connectorPhotoprism := "scope-040-photoprism-neutrality-" + testID(t)
	cleanupPhotosByConnector(t, pool, connectorImmich)
	cleanupPhotosByConnector(t, pool, connectorPhotoprism)
	t.Cleanup(func() {
		cleanupPhotosByConnector(t, pool, connectorImmich)
		cleanupPhotosByConnector(t, pool, connectorPhotoprism)
	})

	immichFixture := newImmichFixtureServer(t, immichFixtureData{Assets: []immich.Asset{
		immichFixtureAsset("vacation-001", "album-vacation", "Vacation 2026", "vacation 2026 sunset"),
	}})
	photoprismFixture := newIntegrationPhotoprismFixture(t, []photoprism.Photo{
		integrationPhotoprismPhoto("vacation-001", "Vacation 2026", "vacation-2026-content"),
	})

	immichClient := immich.NewClient(immichFixture.Client())
	if err := immichClient.Connect(context.Background(), connector.ConnectorConfig{
		AuthType:     "api_key",
		Credentials:  map[string]string{"api_key": immichFixture.APIKey()},
		SourceConfig: map[string]any{"base_url": immichFixture.URL()},
	}); err != nil {
		t.Fatalf("Connect Immich: %v", err)
	}
	photoprismClient := photoprism.NewClient(photoprismFixture.Client())
	if err := photoprismClient.Connect(context.Background(), connector.ConnectorConfig{
		AuthType:     "api_token",
		Credentials:  map[string]string{"api_token": photoprismFixture.APIToken()},
		SourceConfig: map[string]any{"base_url": photoprismFixture.URL()},
	}); err != nil {
		t.Fatalf("Connect PhotoPrism: %v", err)
	}

	immichEvent := drainOnePhotoEvent(t, immichClient, photolib.Scope{})
	photoprismEvent := drainOnePhotoEvent(t, photoprismClient, photolib.Scope{})

	if err := immichEvent.Validate(); err != nil {
		t.Fatalf("immich event validate: %v", err)
	}
	if err := photoprismEvent.Validate(); err != nil {
		t.Fatalf("photoprism event validate: %v", err)
	}

	// Provider-neutrality contract: the same set of structural fields
	// MUST be populated on both events. We do not require values to be
	// equal (the providers carry different content hashes), but every
	// provider-neutral key path MUST be set on both sides.
	checkPair(t, "Operation", immichEvent.Operation != "", photoprismEvent.Operation != "")
	checkPair(t, "ProviderMediaKind", immichEvent.ProviderMediaKind != "", photoprismEvent.ProviderMediaKind != "")
	checkPair(t, "MediaRole", immichEvent.MediaRole != "", photoprismEvent.MediaRole != "")
	checkPair(t, "ContentHash", immichEvent.ContentHash != "", photoprismEvent.ContentHash != "")
	checkPair(t, "Bytes", immichEvent.Bytes != nil, photoprismEvent.Bytes != nil)
	checkPair(t, "MIMEType", immichEvent.MIMEType != "", photoprismEvent.MIMEType != "")
	checkPair(t, "Filename", immichEvent.Filename != "", photoprismEvent.Filename != "")
	checkPair(t, "CapturedAt", !immichEvent.CapturedAt.IsZero(), !photoprismEvent.CapturedAt.IsZero())
	checkPair(t, "Sensitivity.Source", immichEvent.Sensitivity.Source != "", photoprismEvent.Sensitivity.Source != "")
	checkPair(t, "RawProvider", len(immichEvent.RawProvider) > 0, len(photoprismEvent.RawProvider) > 0)

	// Persist both events and prove the resulting PhotoRecord shape
	// is provider-neutral at the row level too.
	immichRecord, err := store.PublishPhotoEvent(context.Background(), connectorImmich, "immich", immichEvent)
	if err != nil {
		t.Fatalf("publish immich event: %v", err)
	}
	photoprismRecord, err := store.PublishPhotoEvent(context.Background(), connectorPhotoprism, "photoprism", photoprismEvent)
	if err != nil {
		t.Fatalf("publish photoprism event: %v", err)
	}
	if immichRecord.MediaRole == "" || photoprismRecord.MediaRole == "" {
		t.Fatalf("MediaRole missing on persisted record: immich=%q photoprism=%q", immichRecord.MediaRole, photoprismRecord.MediaRole)
	}
	if immichRecord.ContentHash == "" || photoprismRecord.ContentHash == "" {
		t.Fatalf("ContentHash missing on persisted record")
	}
	if immichRecord.Provider != "immich" || photoprismRecord.Provider != "photoprism" {
		t.Fatalf("Provider field tampered: immich=%q photoprism=%q", immichRecord.Provider, photoprismRecord.Provider)
	}
}

func drainOnePhotoEvent(t *testing.T, client photolib.PhotoLibrary, scope photolib.Scope) photolib.PhotoEvent {
	t.Helper()
	events, errs := client.EnumerateScope(context.Background(), scope)
	var collected []photolib.PhotoEvent
	for event := range events {
		collected = append(collected, event)
	}
	for err := range errs {
		if err != nil {
			t.Fatalf("EnumerateScope error: %v", err)
		}
	}
	if len(collected) != 1 {
		t.Fatalf("expected exactly one event, got %d: %+v", len(collected), collected)
	}
	return collected[0]
}

func checkPair(t *testing.T, field string, leftPresent bool, rightPresent bool) {
	t.Helper()
	if leftPresent != rightPresent {
		t.Fatalf("provider-neutrality drift on field %q: immich=%v photoprism=%v", field, leftPresent, rightPresent)
	}
	if !leftPresent {
		t.Fatalf("provider-neutral field %q missing on both adapters", field)
	}
}
