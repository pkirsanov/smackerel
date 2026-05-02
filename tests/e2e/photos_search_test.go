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

// TestPhotosSearch_E2E_CrossProviderUnifiedRanking proves the
// cross-provider rerank merges two photos that share the same
// content_hash across Immich + PhotoPrism into a single result row
// with both provider links surfaced. SCN-040-014 (cross-provider
// search and dedupe).
func TestPhotosSearch_E2E_CrossProviderUnifiedRanking(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)
	pool := photosE2EPool(t)
	store := photolib.NewStore(pool)
	connectorImmich := "e2e-immich-cross-" + strings.ReplaceAll(t.Name(), "/", "-")
	connectorPhotoprism := "e2e-photoprism-cross-" + strings.ReplaceAll(t.Name(), "/", "-")

	immichRecord := seedE2EImmichPhoto(t, store, connectorImmich, "e2e-cross-shared-001", "lisbon sunset cross provider canary photo")
	cleanupE2EPhoto(t, pool, immichRecord.ArtifactID)

	photoprismRecord := seedE2EPhotoprismPhoto(t, store, connectorPhotoprism, "e2e-cross-shared-001", "lisbon sunset cross provider canary photo")
	cleanupE2EPhoto(t, pool, photoprismRecord.ArtifactID)

	resp, err := apiGet(cfg, "/v1/photos/search?q="+url.QueryEscape("lisbon sunset cross provider canary"))
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
			ProviderRef string `json:"provider_ref"`
			Provider    string `json:"provider"`
			Preview     struct {
				Providers []string `json:"providers"`
			} `json:"preview"`
		} `json:"results"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("decode search body: %v body=%s", err, string(body))
	}
	matched := 0
	for _, result := range parsed.Results {
		if result.ProviderRef != "e2e-cross-shared-001" {
			continue
		}
		matched++
		// Adversarial: the merged result MUST list BOTH providers in
		// preview.providers — proving the rerank actually collapsed
		// the two rows.
		if len(result.Preview.Providers) < 2 {
			t.Fatalf("merged result missing both providers: %+v", result)
		}
		seen := map[string]bool{}
		for _, provider := range result.Preview.Providers {
			seen[provider] = true
		}
		if !seen["immich"] || !seen["photoprism"] {
			t.Fatalf("merged result missing immich or photoprism: %+v", result.Preview.Providers)
		}
	}
	if matched != 1 {
		t.Fatalf("expected exactly one merged cross-provider row for shared content_hash, got %d (body=%s)", matched, string(body))
	}
}

// seedE2EPhotoprismPhoto persists a synthetic PhotoPrism row with the
// SAME content_hash as the matching Immich row so the cross-provider
// rerank can merge them.
func seedE2EPhotoprismPhoto(t *testing.T, store *photolib.Store, connectorID string, providerRef string, text string) *photolib.PhotoRecord {
	t.Helper()
	event := photolib.SyntheticPhotoEvent()
	event.ProviderRef = providerRef
	event.ContentHash = "sha256:" + providerRef
	event.RawProvider = map[string]any{"provider": "photoprism", "uid": providerRef}
	event.Albums = []string{"Lisbon - Spring 2026"}
	event.Tags = []string{"travel", "sunset"}
	event.Filename = providerRef + ".jpg"
	event.Sensitivity = photolib.ProviderSensitivity{Level: photolib.SensitivityNone, Source: "photoprism:inferred-locally"}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	record, err := store.PublishPhotoEvent(ctx, connectorID, "photoprism", event)
	if err != nil {
		t.Fatalf("PublishPhotoEvent (photoprism): %v", err)
	}
	decision := photolib.ClassificationDecision{
		Caption:         text,
		PrimaryCategory: "lifestyle/travel",
		Confidence:      0.91,
		Rationale:       "fixture multimodal model identified travel photo",
	}
	if err := store.UpdateClassification(ctx, record.ID, decision); err != nil {
		t.Fatalf("UpdateClassification (photoprism): %v", err)
	}
	updated, err := store.GetByID(ctx, record.ID)
	if err != nil {
		t.Fatalf("GetByID after classification (photoprism): %v", err)
	}
	return updated
}
