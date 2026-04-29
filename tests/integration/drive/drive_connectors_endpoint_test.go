//go:build integration

// Spec 038 Scope 1 — DoD item 6 / SCN-038-003 live integration coverage.
//
// This test exercises GET /v1/connectors/drive against the real running
// test stack (smackerel-core container brought up by ./smackerel.sh test
// integration). It proves that the connectors-list endpoint:
//
//  1. Is mounted on the live HTTP server.
//  2. Returns the production drive.DefaultRegistry contents — i.e. the
//     Google provider registered via init() in internal/drive/google.
//  3. Emits the provider-neutral wire shape declared in
//     api.DriveConnectorsResponse exactly (provider id + display name +
//     full Capabilities round-trip), so downstream consumers (PWA Screen 1)
//     never need provider-specific branching.
//
// Adversarial coverage: the test ALSO asserts that the response shape is
// {"providers":[…]} (NOT a top-level array) so a downstream consumer
// caller cannot regress to the wrong shape, and that every required
// capability key is present and non-default for the Google provider.
package drive

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// TestDriveConnectorsEndpoint_LiveStackReturnsNeutralProviderList runs
// against the live test stack. It is the live counterpart to the
// internal/api unit test of the same name and proves the endpoint is
// actually reachable on the real Docker test stack — not just in a
// httptest.NewRecorder fake.
func TestDriveConnectorsEndpoint_LiveStackReturnsNeutralProviderList(t *testing.T) {
	baseURL := liveStackHTTPBase(t)
	authToken := os.Getenv("SMACKEREL_AUTH_TOKEN")

	url := strings.TrimRight(baseURL, "/") + "/v1/connectors/drive"

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Accept", "application/json")
	if authToken != "" {
		// The endpoint is intentionally unauthenticated — but if a
		// future change tightens it, the test should still pass with a
		// valid token rather than fail with 401. Sending an Authorization
		// header is harmless for the unauthenticated case.
		req.Header.Set("Authorization", "Bearer "+authToken)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v (live test stack must be up via ./smackerel.sh test integration)", url, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d (want 200); body=%s", resp.StatusCode, string(body))
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json prefix", ct)
	}

	// Adversarial: the wire shape MUST be {"providers":[…]}, not a
	// top-level array and not {"providers":null}. We assert both via the
	// raw body and via the typed decode.
	if !strings.HasPrefix(strings.TrimSpace(string(body)), `{"providers":`) {
		t.Fatalf("response body does not start with {\"providers\":; body=%s", string(body))
	}

	type capView struct {
		SupportsVersions      bool     `json:"supports_versions"`
		SupportsSharing       bool     `json:"supports_sharing"`
		SupportsChangeHistory bool     `json:"supports_change_history"`
		MaxFileSizeBytes      int64    `json:"max_file_size_bytes"`
		SupportedMimeFilter   []string `json:"supported_mime_filter"`
	}
	type providerView struct {
		ID           string  `json:"id"`
		DisplayName  string  `json:"display_name"`
		Capabilities capView `json:"capabilities"`
	}
	type responseShape struct {
		Providers []providerView `json:"providers"`
	}

	var decoded responseShape
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, string(body))
	}
	if decoded.Providers == nil {
		t.Fatalf(`providers is null; the endpoint must always emit a non-null array; body=%s`, string(body))
	}

	// The production registry must contain at least the Google provider
	// (registered in internal/drive/google init()). The endpoint is
	// provider-neutral so we don't hard-code the count: we just assert
	// Google is present and that EVERY listed provider exposes the full
	// neutral shape. Adding a second provider in the future requires
	// zero changes here.
	var gotGoogle *providerView
	for i := range decoded.Providers {
		p := &decoded.Providers[i]
		if p.ID == "" {
			t.Errorf("provider at index %d has empty id; full=%+v", i, p)
		}
		if p.DisplayName == "" {
			t.Errorf("provider %q has empty display_name", p.ID)
		}
		if p.Capabilities.MaxFileSizeBytes <= 0 {
			t.Errorf("provider %q has non-positive MaxFileSizeBytes %d (must be SST-injected, never zero)",
				p.ID, p.Capabilities.MaxFileSizeBytes)
		}
		if p.ID == "google" {
			gotGoogle = p
		}
	}
	if gotGoogle == nil {
		ids := make([]string, 0, len(decoded.Providers))
		for _, p := range decoded.Providers {
			ids = append(ids, p.ID)
		}
		t.Fatalf(`google provider missing from response; got ids=%v (init() in internal/drive/google must register)`, ids)
	}
	if gotGoogle.DisplayName != "Google Drive" {
		t.Errorf("google DisplayName = %q, want %q", gotGoogle.DisplayName, "Google Drive")
	}
	// Google supports versions, sharing, and change history — these are
	// declared in DefaultCapabilities() and config.Configure() preserves
	// them. If a future capability flip makes one false, this test will
	// flag the regression.
	if !gotGoogle.Capabilities.SupportsVersions {
		t.Errorf("google SupportsVersions = false, want true")
	}
	if !gotGoogle.Capabilities.SupportsSharing {
		t.Errorf("google SupportsSharing = false, want true")
	}
	if !gotGoogle.Capabilities.SupportsChangeHistory {
		t.Errorf("google SupportsChangeHistory = false, want true")
	}
	// MaxFileSizeBytes must be the SST-resolved drive.limits.max_file_size_bytes
	// value (104857600 in dev/test) — NOT the 5 TiB Google API hard ceiling.
	// If wiring forgets to call Configure, this will catch it.
	if gotGoogle.Capabilities.MaxFileSizeBytes >= 5*1024*1024*1024*1024 {
		t.Errorf("google MaxFileSizeBytes = %d, looks like the 5 TiB hard ceiling — wiring forgot to call Configure with SST drive.limits.max_file_size_bytes",
			gotGoogle.Capabilities.MaxFileSizeBytes)
	}
}

// liveStackHTTPBase resolves the http://127.0.0.1:<port> base URL of the
// live test stack's smackerel-core container. The integration runner uses
// `--network host` and exposes the core API on the host port declared by
// CORE_HOST_PORT in config/generated/test.env. Reading the env file
// directly (rather than depending on a CORE_EXTERNAL_URL env var being
// forwarded into the container) keeps this resolution self-contained
// inside the drive integration test package, matching the pattern used
// by drive_foundation_canary_test.go for envFilePath/loadEnvFileKeys.
func liveStackHTTPBase(t *testing.T) string {
	t.Helper()
	envPath := envFilePath(t)
	keys := loadEnvFileKeys(t, envPath)
	port := keys["CORE_HOST_PORT"]
	if port == "" {
		t.Skipf("integration: CORE_HOST_PORT not present in %s — cannot reach live test stack", envPath)
	}
	return "http://127.0.0.1:" + port
}
