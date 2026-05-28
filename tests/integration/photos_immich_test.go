//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
	photolib "github.com/smackerel/smackerel/internal/connector/photos"
	"github.com/smackerel/smackerel/internal/connector/photos/adapters/immich"
)

func TestPhotosImmich_ConnectScopeAndScanLiveProvider(t *testing.T) {
	pool := testPool(t)
	store := photolib.NewStore(pool)
	connectorID := "scope-040-immich-scan-" + testID(t)
	cleanupPhotosByConnector(t, pool, connectorID)

	fixture := newImmichFixtureServer(t, immichFixtureData{Assets: []immich.Asset{
		immichFixtureAsset("whiteboard-001", "album-offsite", "Offsite - March 2026", "whiteboard diagram from March offsite Q2 OKR brainstorm"),
		immichFixtureAsset("private-001", "album-private", "Private", "private excluded handwritten note"),
	}})
	client := immich.NewClient(fixture.Client())
	connectImmichFixture(t, client, fixture)

	scanner := photolib.NewScanner(store, photolib.ScannerConfig{MaxFileSizeBytes: 200_000_000})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := scanner.Scan(ctx, client, connectorID, photolib.Scope{
		IncludedAlbums: []string{"album-offsite"},
		ExcludedAlbums: []string{"album-private"},
		UseFaces:       true,
	})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if result.Progress.Metadata.Done != 1 || result.Progress.Classify.Done != 1 || result.Progress.Embeddings.Done != 1 {
		t.Fatalf("progress = %+v, want one metadata/classify/embed", result.Progress)
	}
	if result.ClassifiedCount != 1 {
		t.Fatalf("ClassifiedCount = %d, want 1", result.ClassifiedCount)
	}

	results, err := store.Search(context.Background(), "whiteboard March offsite Q2 OKR", 10)
	if err != nil {
		t.Fatalf("Search whiteboard: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("whiteboard search returned %d results, want 1: %+v", len(results), results)
	}
	if results[0].ProviderRef != "whiteboard-001" {
		t.Fatalf("ProviderRef = %q, want whiteboard-001", results[0].ProviderRef)
	}
	if results[0].Classification.OCRSnippet == "" {
		t.Fatalf("search result omitted OCR snippet: %+v", results[0])
	}

	excluded, err := store.Search(context.Background(), "private excluded handwritten", 10)
	if err != nil {
		t.Fatalf("Search excluded: %v", err)
	}
	if len(excluded) != 0 {
		t.Fatalf("excluded album produced search results: %+v", excluded)
	}
}

func connectImmichFixture(t *testing.T, client *immich.Client, fixture *immichFixtureServer) {
	t.Helper()
	config := connector.ConnectorConfig{
		AuthType:    "api_key",
		Credentials: map[string]string{"api_key": fixture.APIKey()},
		SourceConfig: map[string]any{
			"base_url": fixture.URL(),
		},
	}
	if err := client.Connect(context.Background(), config); err != nil {
		t.Fatalf("Connect Immich fixture: %v", err)
	}
}

func immichFixtureAsset(id string, albumID string, albumName string, text string) immich.Asset {
	size := int64(1_048_576)
	return immich.Asset{
		ID:               id,
		Type:             "IMAGE",
		OriginalFileName: id + ".jpg",
		MIMEType:         "image/jpeg",
		Checksum:         "sha256:" + id,
		FileCreatedAt:    "2026-03-14T14:08:00Z",
		FileModifiedAt:   "2026-03-14T14:09:00Z",
		Size:             &size,
		Albums:           []immich.AlbumRef{{ID: albumID, Name: albumName}},
		EXIFInfo: map[string]any{
			"camera": "Synthetic Camera",
			"gps":    "Lisbon",
		},
		People: []immich.PersonRef{{ID: "face-maria", Name: "Maria"}},
		Classification: &photolib.ClassificationDecision{
			Caption:         text,
			PrimaryCategory: "document/whiteboard",
			DocumentType:    "whiteboard",
			OCRText:         text,
			OCRSnippet:      text,
			Confidence:      0.93,
			Rationale:       "fixture multimodal model identified whiteboard content",
		},
	}
}
