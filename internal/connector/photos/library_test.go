package photos

import (
	"encoding/json"
	"testing"
	"time"
)

func TestPhotoEventProviderNeutralShape(t *testing.T) {
	bytes := int64(7_654_321)
	lat := 38.7223
	lon := -9.1393
	event := PhotoEvent{
		ProviderRef:       "immich-asset-123",
		Operation:         PhotoOpUpsert,
		ProviderMediaKind: "IMAGE",
		MediaRole:         MediaRoleCameraOriginal,
		ContentHash:       "sha256:synthetic-photo-123",
		Bytes:             &bytes,
		MIMEType:          "image/heic",
		Filename:          "IMG_4821.HEIC",
		CapturedAt:        time.Date(2026, 3, 14, 12, 30, 0, 0, time.UTC),
		UploadedAt:        time.Date(2026, 3, 14, 12, 35, 0, 0, time.UTC),
		GeoLat:            &lat,
		GeoLon:            &lon,
		EXIF: map[string]any{
			"camera": "Synthetic Camera",
			"lens":   "35mm fixture",
		},
		Albums: []string{"Lisbon", "Menus"},
		Tags:   []string{"restaurant", "menu"},
		Faces: []FaceClusterRef{{
			Provider:   "immich",
			ClusterRef: "face-cluster-1",
			Name:       "Fixture Person",
		}},
		Sensitivity: ProviderSensitivity{Level: SensitivityNone, Source: "provider"},
		RawProvider: map[string]any{
			"provider": "immich",
			"assetId":  "immich-asset-123",
		},
	}

	if err := event.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if event.ProviderName() != "immich" {
		t.Fatalf("ProviderName() = %q, want immich", event.ProviderName())
	}

	artifact := event.RawArtifact("photos:immich")
	if artifact.SourceID != "photos:immich" {
		t.Errorf("SourceID = %q", artifact.SourceID)
	}
	if artifact.SourceRef != "immich-asset-123" {
		t.Errorf("SourceRef = %q", artifact.SourceRef)
	}
	if artifact.ContentType != "image/heic" {
		t.Errorf("ContentType = %q", artifact.ContentType)
	}
	if artifact.Title != "IMG_4821.HEIC" {
		t.Errorf("Title = %q", artifact.Title)
	}
	if artifact.Metadata["provider_ref"] != "immich-asset-123" {
		t.Errorf("provider_ref metadata = %#v", artifact.Metadata["provider_ref"])
	}
	if artifact.Metadata["media_role"] != string(MediaRoleCameraOriginal) {
		t.Errorf("media_role metadata = %#v", artifact.Metadata["media_role"])
	}
	if _, ok := artifact.Metadata["raw_provider"].(map[string]any); !ok {
		t.Fatalf("raw_provider metadata type = %T, want map[string]any", artifact.Metadata["raw_provider"])
	}

	encoded, err := json.Marshal(event.ProviderNeutralPayload())
	if err != nil {
		t.Fatalf("marshal provider-neutral payload: %v", err)
	}
	payload := string(encoded)
	for _, forbidden := range []string{"ImmichAsset", "GooglePhotos", "AmazonPhotos", "provider_specific"} {
		if contains(payload, forbidden) {
			t.Fatalf("provider-neutral payload leaked provider-specific marker %q: %s", forbidden, payload)
		}
	}
}

func TestPhotoEventRejectsProviderSpecificBranchingFields(t *testing.T) {
	event := SyntheticPhotoEvent()
	event.RawProvider = map[string]any{"provider_specific": true}
	if err := event.Validate(); err == nil {
		t.Fatal("expected provider_specific raw payload marker to be rejected")
	}
}

func contains(haystack string, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && index(haystack, needle) >= 0)
}

func index(haystack string, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
