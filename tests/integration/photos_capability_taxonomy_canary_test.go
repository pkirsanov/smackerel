//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"testing"

	"github.com/smackerel/smackerel/internal/api"
	"github.com/smackerel/smackerel/internal/config"
	"github.com/smackerel/smackerel/internal/connector"
	photolib "github.com/smackerel/smackerel/internal/connector/photos"
	"github.com/smackerel/smackerel/internal/connector/photos/adapters/photoprism"
)

// TestPhotosCapabilityTaxonomyCanary_GoRegistryMatchesPWALimitationCodes
// proves the capability taxonomy SST is enforced across THREE
// surfaces:
//
//   • Go registry (`internal/connector/photos/capability_taxonomy.go`)
//   • API limitation envelopes (`/v1/photos/.../exercise` 409 body)
//   • PWA banner anchors (`web/pwa/photo-health.html` data-limitation-code)
//
// Drift on any side fails this test. SCN-040-013.
func TestPhotosCapabilityTaxonomyCanary_GoRegistryMatchesPWALimitationCodes(t *testing.T) {
	pool := testPool(t)
	store := photolib.NewStore(pool)
	connectorID := "scope-040-canary-" + testID(t)
	cleanupPhotosByConnector(t, pool, connectorID)
	t.Cleanup(func() { cleanupPhotosByConnector(t, pool, connectorID) })

	registered := map[string]bool{}
	descriptors := photolib.AllLimitationDescriptors()
	if len(descriptors) == 0 {
		t.Fatalf("Go limitation registry is empty — taxonomy SST is broken")
	}
	for _, descriptor := range descriptors {
		if descriptor.Code == "" {
			t.Fatalf("descriptor %+v has empty Code — taxonomy SST is broken", descriptor)
		}
		registered[string(descriptor.Code)] = true
	}

	// (1) PWA surface — every `data-limitation-code` value in
	// `photo-health.html` MUST exist in the Go registry. The test
	// inverts the check too: every UNSUPPORTED descriptor in the
	// registry MUST appear at least once in the PWA HTML.
	pwaPath := pwaHealthPath(t)
	pwaBytes, err := os.ReadFile(pwaPath)
	if err != nil {
		t.Fatalf("read %s: %v", pwaPath, err)
	}
	pattern := regexp.MustCompile(`data-limitation-code="([^"]+)"`)
	pwaCodes := map[string]bool{}
	for _, match := range pattern.FindAllSubmatch(pwaBytes, -1) {
		pwaCodes[string(match[1])] = true
	}
	if len(pwaCodes) == 0 {
		t.Fatalf("photo-health.html has no data-limitation-code anchors — canary surface is broken")
	}
	for code := range pwaCodes {
		if !registered[code] {
			t.Fatalf("PWA limitation_code %q has no Go registry entry — taxonomy drift", code)
		}
	}
	// Every descriptor surfaced via the API MUST also have a PWA anchor
	// so the user-visible banner can render.
	missing := []string{}
	for code := range registered {
		if !pwaCodes[code] {
			missing = append(missing, code)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Fatalf("Go registry codes missing from PWA anchors: %v", missing)
	}

	// (2) API surface — drive the `/exercise` endpoint with a
	// PhotoPrism connector. The response MUST be 409 with the
	// envelope's `limitation_code` matching a registered code.
	cfg := config.PhotosConfig{
		Enabled: true,
		Policy: config.PhotosPolicyConfig{
			LifecycleConfirmationThreshold: 0.65,
		},
	}
	handlers := api.NewPhotosHandlers(store, cfg)

	fixture := newIntegrationPhotoprismFixture(t, []photoprism.Photo{
		integrationPhotoprismPhoto("vacation-001", "Vacation", "vacation-content"),
	})
	server := httptest.NewServer(http.HandlerFunc(handlers.ExerciseCapability))
	t.Cleanup(server.Close)

	body, _ := json.Marshal(map[string]any{
		"provider":     "photoprism",
		"face_cluster": "subj-001",
		"face_name":    "Maria",
		"base_url":     fixture.URL(),
		"api_token":    fixture.APIToken(),
	})
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL+"/v1/photos/connectors/capabilities/faces_write/exercise", bytesReader(body))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Inject the chi URL parameter manually because we mounted the
	// handler on a bare ServeMux above. We do that by calling the
	// handler directly with a request whose context already has the
	// chi route param set via the api package's exposed helper. The
	// simpler path: rerun the request through a real chi router so
	// chi.URLParam(r, "capability") resolves.
	rec := httptest.NewRecorder()
	router := newCapabilityCanaryRouter(handlers)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("/exercise status = %d, want 409 PROVIDER_LIMITATION (body=%s)", rec.Code, rec.Body.String())
	}
	var envelope struct {
		Error struct {
			Code           string `json:"code"`
			LimitationCode string `json:"limitation_code"`
			Capability     string `json:"capability"`
			Provider       string `json:"provider"`
		} `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode 409 envelope: %v", err)
	}
	if envelope.Error.Code != "provider_limitation" {
		t.Fatalf("envelope.code = %q, want provider_limitation", envelope.Error.Code)
	}
	if !registered[envelope.Error.LimitationCode] {
		t.Fatalf("API limitation_code %q has no Go registry entry — taxonomy drift", envelope.Error.LimitationCode)
	}
	if envelope.Error.Capability != "faces_write" {
		t.Fatalf("envelope.capability = %q, want faces_write", envelope.Error.Capability)
	}
	if envelope.Error.Provider != "photoprism" {
		t.Fatalf("envelope.provider = %q, want photoprism", envelope.Error.Provider)
	}

	// (3) Direct typed error path — guards against the API translation
	// layer drifting independently of the writer.
	client := photoprism.NewClient(fixture.Client())
	if err := client.Connect(context.Background(), connector.ConnectorConfig{
		AuthType:     "api_token",
		Credentials:  map[string]string{"api_token": fixture.APIToken()},
		SourceConfig: map[string]any{"base_url": fixture.URL()},
	}); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	rawErr := client.Writer().RenameFaceCluster(context.Background(), "subj-001", "Maria")
	var typed *photoprism.ProviderLimitationError
	if !errors.As(rawErr, &typed) {
		t.Fatalf("typed error path broken: got %T", rawErr)
	}
	if string(typed.LimitationCode) != envelope.Error.LimitationCode {
		t.Fatalf("typed error code %q != API envelope code %q", typed.LimitationCode, envelope.Error.LimitationCode)
	}
}

func pwaHealthPath(t *testing.T) string {
	t.Helper()
	candidates := []string{
		"web/pwa/photo-health.html",
		"../web/pwa/photo-health.html",
		"../../web/pwa/photo-health.html",
		"/workspace/web/pwa/photo-health.html",
	}
	for _, candidate := range candidates {
		abs, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		if _, err := os.Stat(abs); err == nil {
			return abs
		}
	}
	t.Fatalf("web/pwa/photo-health.html not found in any of: %v", candidates)
	return ""
}

func bytesReader(body []byte) io.Reader { return &readerFromBytes{data: body} }

type readerFromBytes struct {
	data []byte
	pos  int
}

func (r *readerFromBytes) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}
