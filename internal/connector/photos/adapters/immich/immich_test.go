package immich

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
	photolib "github.com/smackerel/smackerel/internal/connector/photos"
)

func TestImmichAdapter_MapsProviderMediaToPhotoEvent(t *testing.T) {
	size := int64(1_245_000)
	asset := Asset{
		ID:               "asset-whiteboard-001",
		Type:             "IMAGE",
		OriginalFileName: "IMG_4821.HEIC",
		MIMEType:         "image/heic",
		Checksum:         "sha256:whiteboard-001",
		FileCreatedAt:    "2026-03-14T14:08:00Z",
		FileModifiedAt:   "2026-03-14T14:09:00Z",
		Size:             &size,
		EXIFInfo: map[string]any{
			"camera": "Sony A7 IV",
			"lens":   "35mm",
		},
		Albums: []AlbumRef{{ID: "album-offsite", Name: "Offsite - March 2026"}},
		Tags:   []string{"whiteboard", "offsite"},
		People: []PersonRef{{ID: "person-1", Name: "Maria"}},
	}

	event, skip, err := MapAsset(asset)
	if err != nil {
		t.Fatalf("MapAsset: %v", err)
	}
	if skip != nil {
		t.Fatalf("skip = %+v, want nil", skip)
	}
	if event.ProviderRef != asset.ID {
		t.Fatalf("ProviderRef = %q, want %q", event.ProviderRef, asset.ID)
	}
	if event.Operation != photolib.PhotoOpUpsert {
		t.Fatalf("Operation = %q, want upsert", event.Operation)
	}
	if event.MediaRole != photolib.MediaRoleCameraOriginal {
		t.Fatalf("MediaRole = %q, want camera_original", event.MediaRole)
	}
	if event.MIMEType != "image/heic" {
		t.Fatalf("MIMEType = %q, want image/heic", event.MIMEType)
	}
	if len(event.Albums) != 1 || event.Albums[0] != "Offsite - March 2026" {
		t.Fatalf("Albums = %#v", event.Albums)
	}
	if len(event.Faces) != 1 || event.Faces[0].ClusterRef != "person-1" || event.Faces[0].Provider != "immich" {
		t.Fatalf("Faces = %#v", event.Faces)
	}
	if event.CapturedAt.IsZero() || !event.CapturedAt.Equal(time.Date(2026, 3, 14, 14, 8, 0, 0, time.UTC)) {
		t.Fatalf("CapturedAt = %s", event.CapturedAt)
	}
	if _, forbidden := event.RawProvider["provider_specific"]; forbidden {
		t.Fatalf("raw_provider leaked provider_specific marker: %#v", event.RawProvider)
	}
	if err := event.Validate(); err != nil {
		t.Fatalf("event Validate: %v", err)
	}
}

func TestImmichAdapter_EnumerateScopeExcludesAlbums(t *testing.T) {
	server := NewFixtureServer(t, FixtureData{
		Assets: []Asset{
			fixtureAsset("included-1", "Included", "whiteboard diagram from March offsite"),
			fixtureAsset("excluded-1", "Private", "private excluded note"),
		},
	})

	client := NewClient(server.Client())
	config := connector.ConnectorConfig{
		AuthType:    "api_key",
		Credentials: map[string]string{"api_key": server.APIKey()},
		SourceConfig: map[string]any{
			"base_url": server.URL(),
		},
	}
	if err := client.Connect(context.Background(), config); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	events, errs := client.EnumerateScope(context.Background(), photolib.Scope{
		IncludedAlbums: []string{"Included"},
		ExcludedAlbums: []string{"Private"},
	})
	var got []photolib.PhotoEvent
	for event := range events {
		got = append(got, event)
	}
	for err := range errs {
		if err != nil {
			t.Fatalf("EnumerateScope error: %v", err)
		}
	}
	if len(got) != 1 {
		t.Fatalf("got %d events, want 1", len(got))
	}
	if got[0].ProviderRef != "included-1" {
		t.Fatalf("ProviderRef = %q, want included-1", got[0].ProviderRef)
	}
}

func fixtureAsset(id string, album string, caption string) Asset {
	size := int64(512_000)
	return Asset{
		ID:               id,
		Type:             "IMAGE",
		OriginalFileName: id + ".jpg",
		MIMEType:         "image/jpeg",
		Checksum:         "sha256:" + id,
		FileCreatedAt:    "2026-03-14T14:08:00Z",
		Size:             &size,
		Albums:           []AlbumRef{{ID: album, Name: album}},
		Classification: &photolib.ClassificationDecision{
			Caption:         caption,
			PrimaryCategory: "document/whiteboard",
			OCRText:         caption,
			OCRSnippet:      caption,
			Confidence:      0.93,
			Rationale:       "fixture multimodal model classified the image content",
		},
	}
}

// TestImmichUpload_LimitReaderTruncatesOversizedSource is the
// adversarial regression for MIT-040-S-006 on the immich Upload path.
//
// The test sets `uploadMaxBytes = 1024` and feeds the writer a 16 KiB
// `io.Reader`. With the LimitReader wrap in place, the JSON body sent
// to the server contains exactly 1024 bytes in the `bytes` field
// (LimitReader silently truncates after `max` bytes). Without the
// wrap, the body would contain all 16 KiB.
//
// If the LimitReader call inside (writer).Upload is removed, this
// test fails with `body bytes = 16384, want 1024`.
func TestImmichUpload_LimitReaderTruncatesOversizedSource(t *testing.T) {
	const cap = int64(1024)
	const sourceLen = 16 * 1024

	var capturedBytes []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/server/version":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"version": "v1.120.0", "major": 1})
			return
		case "/api/assets":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read upload body: %v", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			var parsed struct {
				Bytes []byte `json:"bytes"`
			}
			if err := json.Unmarshal(body, &parsed); err != nil {
				t.Errorf("decode upload body: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			capturedBytes = parsed.Bytes
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "asset-test"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	client := NewClient(server.Client())
	client.SetUploadMaxBytes(cap)
	if err := client.Connect(context.Background(), connector.ConnectorConfig{
		AuthType:     "api_key",
		Credentials:  map[string]string{"api_key": "test"},
		SourceConfig: map[string]any{"base_url": server.URL},
	}); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	src := bytes.NewReader(make([]byte, sourceLen))
	if _, err := client.Writer().Upload(context.Background(), src, photolib.UploadMeta{
		Filename: "test.jpg",
		MIMEType: "image/jpeg",
	}); err != nil {
		t.Fatalf("Upload: %v", err)
	}

	if int64(len(capturedBytes)) != cap {
		t.Fatalf("body bytes = %d, want %d (LimitReader truncation not honored)", len(capturedBytes), cap)
	}
}
