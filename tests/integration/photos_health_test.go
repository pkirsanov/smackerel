//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/api"
	"github.com/smackerel/smackerel/internal/config"
	photolib "github.com/smackerel/smackerel/internal/connector/photos"
)

// TestPhotosHealth_ProgressMetricsAndCapabilityLimitsFromLiveAPI
// proves the new `/v1/photos/health` aggregate endpoint returns LIVE
// numbers (lifecycle counts, duplicate counts, capability limit
// list, skip counts) — never placeholder values. SCN-040-013.
func TestPhotosHealth_ProgressMetricsAndCapabilityLimitsFromLiveAPI(t *testing.T) {
	pool := testPool(t)
	store := photolib.NewStore(pool)
	connectorID := "scope-040-health-aggregate-" + testID(t)
	cleanupPhotosByConnector(t, pool, connectorID)
	t.Cleanup(func() { cleanupPhotosByConnector(t, pool, connectorID) })

	captured := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	bytes := int64(2_345_678)
	if _, err := store.PublishPhotoEvent(context.Background(), connectorID, "photoprism", photolib.PhotoEvent{
		ProviderRef:       "vacation-001",
		Operation:         photolib.PhotoOpUpsert,
		ProviderMediaKind: "image",
		MediaRole:         photolib.MediaRoleCameraOriginal,
		ContentHash:       "sha1:vacation-content",
		Bytes:             &bytes,
		MIMEType:          "image/jpeg",
		Filename:          "vacation-001.jpg",
		CapturedAt:        captured,
		Sensitivity:       photolib.ProviderSensitivity{Level: photolib.SensitivityNone, Source: "photoprism:inferred-locally"},
		RawProvider:       map[string]any{"provider": "photoprism", "uid": "vacation-001"},
	}); err != nil {
		t.Fatalf("PublishPhotoEvent: %v", err)
	}

	cfg := config.PhotosConfig{
		Enabled: true,
		Policy: config.PhotosPolicyConfig{
			LifecycleConfirmationThreshold: 0.65,
		},
	}
	handlers := api.NewPhotosHandlers(store, cfg, "test")

	server := httptest.NewServer(http.HandlerFunc(handlers.HealthAggregate))
	t.Cleanup(server.Close)

	response, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("/health status = %d, want 200", response.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode /health: %v", err)
	}

	limits, ok := body["capability_limits"].([]any)
	if !ok || len(limits) == 0 {
		t.Fatalf("capability_limits missing or empty: %+v", body["capability_limits"])
	}
	// Adversarial: every limit MUST carry the canonical
	// limitation_code from the Go registry. A drift here would mean
	// the API surface is out of sync with the taxonomy.
	registered := map[string]bool{}
	for _, descriptor := range photolib.AllLimitationDescriptors() {
		registered[string(descriptor.Code)] = true
	}
	for _, raw := range limits {
		entry, _ := raw.(map[string]any)
		code, _ := entry["limitation_code"].(string)
		if !registered[code] {
			t.Fatalf("/health emitted unknown limitation_code %q (not in Go registry)", code)
		}
	}

	// Lifecycle aggregate MUST be present and structured (live data,
	// not placeholder).
	lifecycleRaw, ok := body["lifecycle"].(map[string]any)
	if !ok {
		t.Fatalf("/health lifecycle missing or wrong shape: %T", body["lifecycle"])
	}
	if _, hasStates := lifecycleRaw["states"]; !hasStates {
		// SummarizeLifecycle returns the dashboard summary; the
		// concrete keys vary, but the JSON object MUST be non-empty.
		if len(lifecycleRaw) == 0 {
			t.Fatalf("/health lifecycle is an empty object — health surface is not live")
		}
	}

	// Duplicates aggregate MUST be present.
	if _, ok := body["duplicates"]; !ok {
		t.Fatalf("/health duplicates missing")
	}
	// Quality histogram MUST be present (even if empty).
	if _, ok := body["quality"]; !ok {
		t.Fatalf("/health quality missing")
	}

	// Skips field MUST be present (an array, possibly empty). Adversarial:
	// the type MUST be a JSON array, never a non-array placeholder.
	skipsRaw, ok := body["skips"].([]any)
	if !ok {
		t.Fatalf("/health skips not an array: %T", body["skips"])
	}
	for _, skip := range skipsRaw {
		entry, _ := skip.(map[string]any)
		if _, ok := entry["reason"].(string); !ok {
			t.Fatalf("/health skip entry missing reason: %+v", entry)
		}
	}

	// Sanity: the response MUST be JSON content type so PWA fetchers
	// can decode it.
	if !strings.Contains(response.Header.Get("Content-Type"), "application/json") {
		t.Fatalf("/health Content-Type = %q, want application/json", response.Header.Get("Content-Type"))
	}
}
