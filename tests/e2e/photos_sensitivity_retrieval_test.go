//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	photolib "github.com/smackerel/smackerel/internal/connector/photos"
)

// TestPhotosSensitivity_E2E_TelegramDoesNotAutoSendSensitivePhoto
// covers SCN-040-012 against the live stack. The contract is:
//
//   - GET /v1/photos/{id}/preview MUST refuse with HTTP 403 +
//     `sensitivity_requires_reveal` for sensitive photos when no
//     reveal token is presented (Telegram and the agent tools rely on
//     this gate).
//   - POST /v1/photos/{id}/reveal mints a single-use, actor-bound
//     token; presenting it once allows preview, but reusing it MUST
//     fail (the bot/agent must request a fresh token to see the
//     photo again).
//   - Search results for sensitive photos MUST set
//     `requires_reveal=true` and MUST NOT include a clickable preview
//     URL — Telegram and the agent rely on this to refuse auto-send
//     of sensitive bytes.
func TestPhotosSensitivity_E2E_TelegramDoesNotAutoSendSensitivePhoto(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)
	pool := photosE2EPool(t)

	// Persist the sensitive photo through the store directly so the
	// classifier is not part of the test surface; the API gate is the
	// behavior under test here.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	uniq := "e2e-sensitive-" + uuid.NewString()
	event := photolib.SyntheticPhotoEvent()
	event.ProviderRef = "web:sensitive:" + uniq
	event.ContentHash = "sha256:sensitive:" + uniq
	event.Filename = uniq + ".jpg"
	event.Tags = []string{"e2e-sensitivity", uniq}
	event.SourceChannel = photolib.SourceChannelWeb
	event.SourceRef = "session:" + uniq
	event.Sensitivity = photolib.ProviderSensitivity{
		Level:  photolib.SensitivitySensitive,
		Source: "test",
		Labels: []string{"financial"},
	}
	store := photolib.NewStore(pool)
	record, err := store.PublishPhotoEvent(ctx, "e2e-sensitivity", "web", event)
	if err != nil {
		t.Fatalf("publish sensitive photo: %v", err)
	}
	cleanupE2EPhoto(t, pool, record.ArtifactID)

	// Preview without a reveal token MUST be rejected.
	noTokenResp, err := apiGet(cfg, "/v1/photos/"+record.ID.String()+"/preview")
	if err != nil {
		t.Fatalf("preview no token: %v", err)
	}
	noTokenBody, err := readBody(noTokenResp)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if noTokenResp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 without reveal, got %d body=%s", noTokenResp.StatusCode, noTokenBody)
	}
	if !strings.Contains(string(noTokenBody), "sensitivity_requires_reveal") {
		t.Fatalf("preview rejection missing sensitivity_requires_reveal code: %s", noTokenBody)
	}

	// Mint a reveal token via the API.
	mintResp, err := apiPostJSON(cfg, "/v1/photos/"+record.ID.String()+"/reveal", map[string]any{"ttl_seconds": 30})
	if err != nil {
		t.Fatalf("mint reveal: %v", err)
	}
	mintBody, err := readBody(mintResp)
	if err != nil {
		t.Fatalf("read mint body: %v", err)
	}
	if mintResp.StatusCode != http.StatusCreated {
		t.Fatalf("mint reveal status=%d body=%s", mintResp.StatusCode, mintBody)
	}
	var minted struct {
		RevealToken string `json:"reveal_token"`
	}
	if err := json.Unmarshal(mintBody, &minted); err != nil {
		t.Fatalf("decode mint: %v body=%s", err, mintBody)
	}
	if minted.RevealToken == "" {
		t.Fatalf("mint response missing reveal_token: %s", mintBody)
	}

	// Preview with the reveal token MUST succeed.
	previewResp, err := apiGet(cfg, "/v1/photos/"+record.ID.String()+"/preview?reveal_token="+minted.RevealToken)
	if err != nil {
		t.Fatalf("preview with reveal: %v", err)
	}
	previewBody, err := readBody(previewResp)
	if err != nil {
		t.Fatalf("read preview body: %v", err)
	}
	if previewResp.StatusCode != http.StatusOK {
		t.Fatalf("preview with reveal status=%d body=%s", previewResp.StatusCode, previewBody)
	}

	// Re-using the consumed token MUST be rejected.
	repeatResp, err := apiGet(cfg, "/v1/photos/"+record.ID.String()+"/preview?reveal_token="+minted.RevealToken)
	if err != nil {
		t.Fatalf("repeat preview: %v", err)
	}
	repeatBody, _ := readBody(repeatResp)
	if repeatResp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 on token reuse, got %d body=%s", repeatResp.StatusCode, repeatBody)
	}

	// Adversarial: search results for sensitive photos MUST set
	// requires_reveal=true and MUST NOT include a clickable preview
	// URL. The Telegram bot reads this same shape from /find before
	// deciding whether to auto-send bytes.
	searchResp, err := apiGet(cfg, "/v1/photos/search?q="+uniq)
	if err != nil {
		t.Fatalf("photos search: %v", err)
	}
	searchBody, _ := readBody(searchResp)
	if searchResp.StatusCode != http.StatusOK {
		t.Fatalf("photos search status=%d body=%s", searchResp.StatusCode, searchBody)
	}
	if !strings.Contains(string(searchBody), `"requires_reveal":true`) {
		t.Fatalf("expected search result to include requires_reveal=true: %s", searchBody)
	}
	if strings.Contains(string(searchBody), `/v1/photos/`+record.ID.String()+`/preview?size=thumb`) {
		t.Fatalf("sensitive search result must not expose preview URL: %s", searchBody)
	}
}
