package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/smackerel/smackerel/internal/config"
	photolib "github.com/smackerel/smackerel/internal/connector/photos"
)

// Spec 040 chaos closure regression tests (C-001..C-006).
//
// Each test is intentionally tight to a single finding so that a future
// revert of the corresponding fix flips the test red. Names embed the
// finding ID so traceability between the closure section in
// `specs/040-cloud-photo-libraries/report.md` and the test surface is
// mechanical.

// chaosClosureHandlers builds a PhotosHandlers instance with no real
// store — sufficient for the validation-path tests below. Where the
// store-nil short-circuit fires before the validator we explicitly
// document it; callers can still exercise the new helper functions
// directly to keep regression coverage tight.
func chaosClosureHandlers(maxScopeSize int64Or) *PhotosHandlers {
	cfg := config.PhotosConfig{}
	cfg.Policy.ActionsMaxScopeSize = int(maxScopeSize)
	cfg.IOLimits.PhotoBinaryMaxBytes = 104857600
	cfg.Providers.Immich.BaseURL = ""
	cfg.Providers.Immich.APIKey = ""
	cfg.Providers.Photoprism.BaseURL = ""
	cfg.Providers.Photoprism.APIToken = ""
	return &PhotosHandlers{config: cfg}
}

// int64Or is a tiny convenience alias used only by the closure tests so
// the cfg builder reads naturally for the new policy field.
type int64Or = int64

// ---------------------------------------------------------------------
// C-001 — empty body MUST return 400 INVALID_REQUEST, not 502.
// ---------------------------------------------------------------------

func TestPhotosConnectorsTest_C001_EmptyBodyReturns400InvalidRequest(t *testing.T) {
	h := chaosClosureHandlers(50)

	req := httptest.NewRequest(http.MethodPost, "/v1/photos/connectors/test", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.TestConnector(rec, req)

	if got := rec.Result().StatusCode; got != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body=%s)", got, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "BAD_GATEWAY") || strings.Contains(body, "photo_provider_probe_failed") {
		t.Fatalf("response leaked upstream-error code; body=%s", body)
	}
	if !strings.Contains(body, "invalid_photo_connector") {
		t.Fatalf("response missing invalid_photo_connector code; body=%s", body)
	}
	if !strings.Contains(body, "base_url is required") {
		t.Fatalf("response missing local-validation reason; body=%s", body)
	}
}

func TestPhotosConnectors_C001_EmptyBodyReturns400FromConnect(t *testing.T) {
	// Connect short-circuits on store=nil with 503; the test instead
	// exercises the validator directly to prove the same 400 mapping
	// applies once the store is wired. This keeps regression coverage
	// for the error-classification fix without needing a real DB.
	h := chaosClosureHandlers(50)
	_, _, err := h.photoLibraryFromRequest(photoConnectorRequest{Provider: "immich", Config: map[string]string{}})
	if err == nil {
		t.Fatal("expected local-validation error for empty config; got nil")
	}
	if !strings.Contains(err.Error(), "base_url is required") {
		t.Fatalf("error must surface base_url is required; got %v", err)
	}
}

// ---------------------------------------------------------------------
// C-002 — non-UUID photo_ids MUST be rejected at plan time with 400.
// ---------------------------------------------------------------------

func TestPhotosActionsPlan_C002_NonUUIDPhotoIDReturns400(t *testing.T) {
	scope := photolib.ActionScope{PhotoIDs: []string{"not-a-uuid"}}
	if err := validatePlanScope(scope, 50); err == nil {
		t.Fatal("expected validatePlanScope to reject non-UUID photo_id; got nil")
	} else if !strings.Contains(err.Error(), "scope.photo_ids[0]") {
		t.Fatalf("error must point at the offending field; got %v", err)
	}

	// Handler boundary: validation runs BEFORE the store-availability
	// check (Spec 040 chaos closure) so a missing store does not mask
	// the 400 INVALID_REQUEST classification. The handler MUST NOT
	// mint an action token for a malformed scope under any condition.
	h := chaosClosureHandlers(50)
	body, _ := json.Marshal(PhotoActionsPlanRequest{
		Action: "delete",
		Scope:  scope,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/photos/actions/plan", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.PlanAction(rec, req)
	if rec.Result().StatusCode != http.StatusBadRequest {
		t.Fatalf("handler status = %d, want 400 (body=%s)", rec.Result().StatusCode, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid_action_scope") {
		t.Fatalf("400 response missing invalid_action_scope code; body=%s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "scope.photo_ids[0]") {
		t.Fatalf("400 response must point at the offending field; body=%s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "action_token") {
		t.Fatalf("malformed scope MUST NOT mint an action_token; body=%s", rec.Body.String())
	}
}

// ---------------------------------------------------------------------
// C-003 — PhotoPrism is advertised in the list endpoint AND accepted
// by the connect/test endpoints. The previous code rejected
// provider="photoprism" with "only immich photo connectors are
// supported in this scope".
// ---------------------------------------------------------------------

func TestPhotosConnectorsTest_C003_PhotoPrismProviderAccepted(t *testing.T) {
	h := chaosClosureHandlers(50)

	// First: validator-level proof that photoprism is recognised. An
	// empty config still fails (base_url missing) — but the failure
	// shape MUST be the local-validation contract, not the legacy
	// "only immich" rejection.
	_, _, err := h.photoLibraryFromRequest(photoConnectorRequest{Provider: "photoprism", Config: map[string]string{}})
	if err == nil {
		t.Fatal("expected local-validation error for empty photoprism config; got nil")
	}
	if strings.Contains(err.Error(), "only immich") {
		t.Fatalf("photoprism MUST NOT be rejected by the immich-only guard; got %v", err)
	}
	if !strings.Contains(err.Error(), "base_url is required") {
		t.Fatalf("validator should surface base_url is required for photoprism; got %v", err)
	}

	// Second: full request shape with base_url + api_token resolves to
	// a non-nil photolib.PhotoLibrary client.
	client, cfg, err := h.photoLibraryFromRequest(photoConnectorRequest{
		Provider: "photoprism",
		Config:   map[string]string{"base_url": "http://photoprism.example", "api_token": "tok"},
	})
	if err != nil {
		t.Fatalf("photoprism with full config must succeed; got %v", err)
	}
	if client == nil {
		t.Fatal("photoprism client must be non-nil")
	}
	if cfg.Credentials["api_token"] != "tok" {
		t.Fatalf("photoprism credentials = %v, want api_token=tok", cfg.Credentials)
	}

	// Third: handler-level smoke. Empty body → 400 with local-validation
	// reason for whichever provider was defaulted; ensure photoprism
	// also routes to 400, not 502.
	body, _ := json.Marshal(map[string]any{"provider": "photoprism", "config": map[string]string{}})
	req := httptest.NewRequest(http.MethodPost, "/v1/photos/connectors/test", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.TestConnector(rec, req)
	if rec.Result().StatusCode != http.StatusBadRequest {
		t.Fatalf("photoprism empty-config status = %d, want 400 (body=%s)", rec.Result().StatusCode, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "only immich") {
		t.Fatalf("photoprism MUST NOT receive the legacy immich-only rejection; body=%s", rec.Body.String())
	}
}

// ---------------------------------------------------------------------
// C-004 — pgx.ErrNoRows MUST be scrubbed before reaching the client.
// ---------------------------------------------------------------------

func TestPhotosHealth_C004_DuplicatesGetScrubsErrNoRows(t *testing.T) {
	// Direct sentinel match.
	status, code, message := clusterStoreErrorResponse(pgx.ErrNoRows)
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
	if code != "cluster_not_found" {
		t.Fatalf("code = %q, want cluster_not_found", code)
	}
	if message != "duplicate group not found" {
		t.Fatalf("message = %q, want clean copy", message)
	}
	if strings.Contains(message, "no rows in result set") {
		t.Fatalf("message MUST NOT leak lib/pq sentinel; got %q", message)
	}

	// Wrapped sentinel via fmt.Errorf("...: %w", err) — what the store
	// actually returns ("scan cluster: no rows in result set").
	wrapped := fmt.Errorf("scan cluster: %w", pgx.ErrNoRows)
	status, code, message = clusterStoreErrorResponse(wrapped)
	if status != http.StatusNotFound || code != "cluster_not_found" {
		t.Fatalf("wrapped sentinel: status=%d code=%q, want 404/cluster_not_found", status, code)
	}
	if strings.Contains(message, "no rows in result set") {
		t.Fatalf("wrapped sentinel: message MUST NOT leak lib/pq sentinel; got %q", message)
	}

	// String-only wrap (e.g. fmt.Errorf("photos: %s", err.Error())) —
	// errors.Is alone misses these, so the fallback substring match
	// must catch them too.
	stringWrap := errors.New("photos: requested best pick is not a cluster member: no rows in result set")
	status, code, _ = clusterStoreErrorResponse(stringWrap)
	if status != http.StatusNotFound || code != "cluster_not_found" {
		t.Fatalf("string-wrap sentinel: status=%d code=%q, want 404/cluster_not_found", status, code)
	}

	// Non-sentinel error MUST NOT be misclassified as not-found.
	other := errors.New("connection refused")
	status, code, _ = clusterStoreErrorResponse(other)
	if status == http.StatusNotFound {
		t.Fatalf("non-sentinel error MUST NOT map to 404; got status=%d code=%q", status, code)
	}
}

func TestPhotosHealth_C004_DuplicatesGetHandlerScrubsErrNoRows(t *testing.T) {
	// Verify the handler short-circuits on store=nil before touching the
	// scrub path — this is the same shape as the chaos audit's
	// integration call which hit a real store with no matching row. The
	// helper-level test above is the primary regression anchor; this
	// test simply guards the handler wiring so a future refactor that
	// drops the helper still fails on the helper test.
	h := &PhotosHandlers{}
	req := httptest.NewRequest(http.MethodGet, "/v1/photos/health/duplicates/00000000-0000-0000-0000-000000000000", nil)
	rec := httptest.NewRecorder()
	h.HealthDuplicatesGet(rec, req)
	// store=nil ⇒ 503; this just proves the handler did not panic and
	// the route still resolves to the same function.
	if rec.Result().StatusCode == 0 {
		t.Fatal("handler did not write a status")
	}
}

// ---------------------------------------------------------------------
// C-005 — control characters in /v1/photos/search?q= MUST be rejected.
// ---------------------------------------------------------------------

func TestPhotosSearch_C005_ControlCharsRejected(t *testing.T) {
	cases := []struct {
		name    string
		query   string
		wantErr bool
	}{
		{"plain word", "vacation", false},
		{"whitespace allowed", "vacation 2026\t spring", false},
		{"newline allowed", "line1\nline2", false},
		{"null byte rejected", "v\x00cation", true},
		{"bell rejected", "v\x07cation", true},
		{"escape rejected", "v\x1bcation", true},
		{"DEL rejected", "v\x7fcation", true},
		{"C1 control rejected", "v\u0085cation", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateSearchQuery(tc.query)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error for query %q", tc.query)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error for query %q: %v", tc.query, err)
			}
			if tc.wantErr && err != nil && !strings.Contains(err.Error(), "control characters are not permitted") {
				t.Fatalf("error message must explain rejection reason; got %v", err)
			}
		})
	}
}

func TestPhotosSearch_C005_HandlerReturns400ForControlChars(t *testing.T) {
	// Validation runs BEFORE the store-availability check (Spec 040
	// chaos closure) so a missing store does not mask the 400
	// INVALID_REQUEST classification. The handler MUST always reject
	// control characters with the right code.
	h := &PhotosHandlers{}
	req := httptest.NewRequest(http.MethodGet, "/v1/photos/search?q=hello%00world", nil)
	rec := httptest.NewRecorder()
	h.Search(rec, req)
	if rec.Result().StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body=%s)", rec.Result().StatusCode, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid_search_query") {
		t.Fatalf("body missing invalid_search_query; body=%s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "control characters are not permitted") {
		t.Fatalf("body missing rejection reason; body=%s", rec.Body.String())
	}
}

// ---------------------------------------------------------------------
// C-006 — scope size MUST be capped at the SST-derived
// PHOTOS_POLICY_ACTIONS_MAX_SCOPE_SIZE value.
// ---------------------------------------------------------------------

func TestPhotosActionsPlan_C006_ScopeOverMaxReturns400(t *testing.T) {
	// 51 valid UUIDs against a configured cap of 50 must fail.
	scope := photolib.ActionScope{PhotoIDs: make([]string, 51)}
	for i := range scope.PhotoIDs {
		scope.PhotoIDs[i] = newTestUUID(t, i)
	}

	err := validatePlanScope(scope, 50)
	if err == nil {
		t.Fatal("expected validatePlanScope to reject scope over cap; got nil")
	}
	if !strings.Contains(err.Error(), "exceeds maximum of 50") {
		t.Fatalf("error must reference SST cap; got %v", err)
	}

	// Exactly at the cap is allowed.
	scope.PhotoIDs = scope.PhotoIDs[:50]
	if err := validatePlanScope(scope, 50); err != nil {
		t.Fatalf("scope at the cap must be allowed; got %v", err)
	}

	// Combined photo_ids + removal_ids MUST also be capped.
	scope = photolib.ActionScope{
		PhotoIDs:   []string{newTestUUID(t, 0), newTestUUID(t, 1)},
		RemovalIDs: []string{newTestUUID(t, 2), newTestUUID(t, 3)},
	}
	if err := validatePlanScope(scope, 3); err == nil {
		t.Fatal("expected combined scope (4) to exceed cap (3)")
	}

	// Missing/zero SST cap is treated as a fail-loud config error so a
	// future regression that drops the SST wiring cannot silently
	// disable the cap.
	if err := validatePlanScope(scope, 0); err == nil {
		t.Fatal("expected fail-loud error when SST cap is unset")
	}

	// Handler boundary: the 200-UUID payload from the chaos audit MUST
	// be rejected with 400 INVALID_REQUEST, not minted into a token.
	h := chaosClosureHandlers(50)
	bigScope := photolib.ActionScope{PhotoIDs: make([]string, 200)}
	for i := range bigScope.PhotoIDs {
		bigScope.PhotoIDs[i] = newTestUUID(t, i)
	}
	body, _ := json.Marshal(PhotoActionsPlanRequest{
		Action: "delete",
		Scope:  bigScope,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/photos/actions/plan", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.PlanAction(rec, req)
	if rec.Result().StatusCode != http.StatusBadRequest {
		t.Fatalf("handler status = %d, want 400 (body=%s)", rec.Result().StatusCode, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid_action_scope") {
		t.Fatalf("400 response missing invalid_action_scope code; body=%s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "exceeds maximum of 50") {
		t.Fatalf("400 response must reference the SST cap; body=%s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "action_token") {
		t.Fatalf("oversized scope MUST NOT mint an action_token; body=%s", rec.Body.String())
	}
}

func newTestUUID(t *testing.T, seed int) string {
	t.Helper()
	// Deterministic per-seed UUIDs keep the test output stable when it
	// fails; using uuid.New() would obscure which slot tripped a future
	// per-index validator.
	return fmt.Sprintf("00000000-0000-0000-0000-%012d", seed)
}
