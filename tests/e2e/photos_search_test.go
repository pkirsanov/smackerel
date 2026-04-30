//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	photolib "github.com/smackerel/smackerel/internal/connector/photos"
)

func TestPhotosSearch_E2E_ImmichWhiteboardOCRResult(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)
	pool := photosE2EPool(t)
	store := photolib.NewStore(pool)
	connectorID := "e2e-immich-search-" + strings.ReplaceAll(t.Name(), "/", "-")
	record := seedE2EImmichPhoto(t, store, connectorID, "e2e-whiteboard-001", "whiteboard diagram from March offsite Q2 OKR brainstorm")
	cleanupE2EPhoto(t, pool, record.ArtifactID)

	resp, err := apiGet(cfg, "/v1/photos/search?q="+url.QueryEscape("whiteboard diagram March offsite Q2 OKR"))
	if err != nil {
		t.Fatalf("GET /v1/photos/search: %v", err)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read search body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("search status=%d body=%s", resp.StatusCode, string(body))
	}
	var parsed struct {
		Results []struct {
			PhotoID         string  `json:"photo_id"`
			Provider        string  `json:"provider"`
			ProviderRef     string  `json:"provider_ref"`
			MatchConfidence float64 `json:"match_confidence"`
			Classification  struct {
				OCRSnippet string `json:"ocr_snippet"`
			} `json:"classification"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("decode search body: %v body=%s", err, string(body))
	}
	if len(parsed.Results) != 1 {
		t.Fatalf("search returned %d results, want 1: %s", len(parsed.Results), string(body))
	}
	if parsed.Results[0].Provider != "immich" || parsed.Results[0].ProviderRef != "e2e-whiteboard-001" {
		t.Fatalf("provider result mismatch: %+v", parsed.Results[0])
	}
	if parsed.Results[0].MatchConfidence <= 0 {
		t.Fatalf("match confidence not surfaced: %+v", parsed.Results[0])
	}
	if !strings.Contains(parsed.Results[0].Classification.OCRSnippet, "Q2 OKR") {
		t.Fatalf("OCR snippet missing Q2 OKR: %+v", parsed.Results[0].Classification)
	}
}

func seedE2EImmichPhoto(t *testing.T, store *photolib.Store, connectorID string, providerRef string, text string) *photolib.PhotoRecord {
	t.Helper()
	event := photolib.SyntheticPhotoEvent()
	event.ProviderRef = providerRef
	event.ContentHash = "sha256:" + providerRef
	event.RawProvider = map[string]any{"provider": "immich", "asset_id": providerRef}
	event.Albums = []string{"Offsite - March 2026"}
	event.Tags = []string{"whiteboard", "offsite"}
	event.Filename = providerRef + ".jpg"
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	record, err := store.PublishPhotoEvent(ctx, connectorID, "immich", event)
	if err != nil {
		t.Fatalf("PublishPhotoEvent: %v", err)
	}
	decision := photolib.ClassificationDecision{
		Caption:         text,
		PrimaryCategory: "document/whiteboard",
		DocumentType:    "whiteboard",
		OCRText:         text,
		OCRSnippet:      text,
		Confidence:      0.93,
		Rationale:       "fixture multimodal model identified whiteboard content",
	}
	if err := store.UpdateClassification(ctx, record.ID, decision); err != nil {
		t.Fatalf("UpdateClassification: %v", err)
	}
	updated, err := store.GetByID(ctx, record.ID)
	if err != nil {
		t.Fatalf("GetByID after classification: %v", err)
	}
	return updated
}
