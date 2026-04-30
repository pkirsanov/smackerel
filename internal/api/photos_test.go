package api

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	photolib "github.com/smackerel/smackerel/internal/connector/photos"
)

func TestPhotoSearchResponse_UsesProviderNeutralDTO(t *testing.T) {
	confidence := 0.93
	classification := photolib.ClassificationDecision{
		Caption:         "whiteboard diagram from the March offsite",
		PrimaryCategory: "document/whiteboard",
		DocumentType:    "whiteboard",
		OCRText:         "Q2 OKR brainstorm north star activation retention",
		OCRSnippet:      "Q2 OKR brainstorm",
		Confidence:      confidence,
		Rationale:       "multimodal model saw a whiteboard and read the OCR text",
	}
	classificationBytes, err := json.Marshal(classification)
	if err != nil {
		t.Fatalf("marshal classification: %v", err)
	}
	capturedAt := time.Date(2026, 3, 14, 14, 8, 0, 0, time.UTC)
	record := photolib.PhotoRecord{
		ID:                       uuid.New(),
		ArtifactID:               "photo:immich:whiteboard-001",
		Provider:                 "immich",
		ProviderRef:              "whiteboard-001",
		Filename:                 "IMG_4821.HEIC",
		CapturedAt:               &capturedAt,
		Albums:                   []string{"Offsite - March 2026"},
		MediaRole:                photolib.MediaRoleCameraOriginal,
		Sensitivity:              photolib.SensitivityNone,
		LifecycleState:           "active",
		Classification:           classificationBytes,
		ClassificationConfidence: &confidence,
		RawProvider:              []byte(`{"provider":"immich","asset_id":"whiteboard-001"}`),
	}

	summary := photoSummaryFromRecord(record, 0.88)
	if summary.Provider != "immich" || summary.ProviderRef != "whiteboard-001" {
		t.Fatalf("provider shape mismatch: %+v", summary)
	}
	if summary.Classification.Caption != classification.Caption {
		t.Fatalf("caption = %q, want %q", summary.Classification.Caption, classification.Caption)
	}
	if summary.Classification.OCRSnippet != classification.OCRSnippet {
		t.Fatalf("OCRSnippet = %q, want %q", summary.Classification.OCRSnippet, classification.OCRSnippet)
	}
	if summary.MatchConfidence != 0.88 {
		t.Fatalf("MatchConfidence = %v, want 0.88", summary.MatchConfidence)
	}
	encoded, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal summary: %v", err)
	}
	for _, forbidden := range []string{"ImmichAsset", "provider_specific", "raw_provider"} {
		if containsString(encoded, forbidden) {
			t.Fatalf("provider-neutral search summary leaked %q: %s", forbidden, string(encoded))
		}
	}
}

func containsString(haystack []byte, needle string) bool {
	return strings.Contains(string(haystack), needle)
}
