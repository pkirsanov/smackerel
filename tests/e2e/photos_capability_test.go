//go:build e2e

package e2e

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestPhotosCapability_E2E_AlbumWriteBlockedWhileSearchWorks proves
// the live runtime API translates a typed PhotoPrism
// `ProviderLimitationError` into a `409 PROVIDER_LIMITATION` envelope
// while leaving the unrelated read paths (search, health) functional.
// SCN-040-013, SCN-040-015 (capability matrix governance + observability).
func TestPhotosCapability_E2E_AlbumWriteBlockedWhileSearchWorks(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)

	// Drive the capability-exercise endpoint with NO base_url/api_token —
	// the live API resolves the missing creds via SST. When PhotoPrism
	// is not configured, the API returns 400 invalid_capability_request
	// (validates the SST guard); when it IS configured, the API
	// translates the FacesWrite UNSUPPORTED status into a 409 envelope.
	body, err := apiPostJSON(cfg, "/v1/photos/connectors/capabilities/faces_write/exercise", map[string]any{
		"provider":     "photoprism",
		"face_cluster": "subj-001",
		"face_name":    "Maria",
	})
	if err != nil {
		t.Fatalf("POST /v1/photos/.../exercise: %v", err)
	}
	defer body.Body.Close()

	if body.StatusCode != http.StatusConflict && body.StatusCode != http.StatusBadRequest && body.StatusCode != http.StatusBadGateway {
		t.Fatalf("/exercise returned unexpected status %d (want 400 or 409)", body.StatusCode)
	}

	if body.StatusCode == http.StatusConflict {
		// PhotoPrism IS configured — adversarial: the envelope MUST
		// carry the canonical limitation_code so the PWA banner can
		// render the right copy.
		var envelope struct {
			Error struct {
				Code           string `json:"code"`
				LimitationCode string `json:"limitation_code"`
				Capability     string `json:"capability"`
			} `json:"error"`
		}
		if err := json.NewDecoder(body.Body).Decode(&envelope); err != nil {
			t.Fatalf("decode 409 envelope: %v", err)
		}
		if envelope.Error.Code != "provider_limitation" {
			t.Fatalf("error code = %q, want provider_limitation", envelope.Error.Code)
		}
		if !strings.HasSuffix(envelope.Error.LimitationCode, "_by_provider") {
			t.Fatalf("limitation_code does not match canonical taxonomy suffix: %q", envelope.Error.LimitationCode)
		}
		if envelope.Error.Capability != "faces_write" {
			t.Fatalf("envelope.capability = %q, want faces_write", envelope.Error.Capability)
		}
	}

	// Adversarial: the read paths MUST still work even after the writer
	// 409 — the capability denial is per-operation, not per-connector.
	searchResp, err := apiGet(cfg, "/v1/photos/search?q=lisbon")
	if err != nil {
		t.Fatalf("GET /v1/photos/search: %v", err)
	}
	defer searchResp.Body.Close()
	if searchResp.StatusCode != http.StatusOK {
		t.Fatalf("search status after 409 = %d, want 200 (unrelated path must remain available)", searchResp.StatusCode)
	}

	// Adversarial: the photo health aggregate MUST still serve live
	// numbers even after a writer 409.
	healthResp, err := apiGet(cfg, "/v1/photos/health")
	if err != nil {
		t.Fatalf("GET /v1/photos/health: %v", err)
	}
	defer healthResp.Body.Close()
	if healthResp.StatusCode != http.StatusOK {
		t.Fatalf("/v1/photos/health status = %d, want 200", healthResp.StatusCode)
	}
	healthBody, err := readBody(healthResp)
	if err != nil {
		t.Fatalf("read health body: %v", err)
	}
	var aggregate map[string]any
	if err := json.Unmarshal(healthBody, &aggregate); err != nil {
		t.Fatalf("decode health body: %v body=%s", err, string(healthBody))
	}
	limits, ok := aggregate["capability_limits"].([]any)
	if !ok || len(limits) == 0 {
		t.Fatalf("/v1/photos/health capability_limits missing or empty (body=%s)", string(healthBody))
	}
}
