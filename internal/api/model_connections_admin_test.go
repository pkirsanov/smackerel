// Spec 096 SCOPE-06 (SCN-096-W01/W02/W03/W04) — handler-level unit tests for
// the operator-gated /v1/admin/model-connections* surface. They drive the REAL
// ModelConnectionsAdminHandler through a chi router (so chi.URLParam resolves
// {id}) against an in-memory fake store, a SYNTHETIC vault, and an injected
// fake probe — proving the write-only-credential redaction, the truthful typed
// pass/fail, the 409 enable-guard, the 404 closed-set slot, and the per-kind
// secret fields in isolation. The live hosted-provider e2e legs (real
// reachability + real credentials) are DEFERRED to the home-lab bubbles.devops
// dispatch (C7).
//
// All secret values are SYNTHETIC; lines whose values resemble a key carry a
// gitleaks allow marker (repo convention) so the pre-commit scanner does not
// flag the fixtures.
package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/smackerel/smackerel/internal/assistant/openknowledge/connstore"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/connvault"
	"github.com/smackerel/smackerel/internal/config"
)

// --- fakes ------------------------------------------------------------------

type fakeConnStore struct {
	registry map[string]config.ModelConnection
	rows     map[string]connstore.Record
}

func newFakeConnStore(conns ...config.ModelConnection) *fakeConnStore {
	reg := make(map[string]config.ModelConnection, len(conns))
	for _, c := range conns {
		reg[c.ID] = c
	}
	return &fakeConnStore{registry: reg, rows: map[string]connstore.Record{}}
}

func (f *fakeConnStore) Connection(id string) (config.ModelConnection, bool) {
	c, ok := f.registry[id]
	return c, ok
}
func (f *fakeConnStore) ListDeclared() []config.ModelConnection {
	out := make([]config.ModelConnection, 0, len(f.registry))
	for _, c := range f.registry {
		out = append(out, c)
	}
	return out
}
func (f *fakeConnStore) Get(_ context.Context, id string) (connstore.Record, bool, error) {
	r, ok := f.rows[id]
	return r, ok, nil
}
func (f *fakeConnStore) List(_ context.Context) ([]connstore.Record, error) {
	out := make([]connstore.Record, 0, len(f.rows))
	for _, r := range f.rows {
		out = append(out, r)
	}
	return out, nil
}
func (f *fakeConnStore) UpsertCredential(_ context.Context, id, kind string, rec connvault.VaultRecord) error {
	r := f.rows[id]
	r.ConnectionID = id
	r.ProviderKind = kind
	cp := rec
	r.Secret = &cp
	// Storing a new credential resets the prior test (a rotated key invalidates it).
	r.LastTestOutcome = ""
	r.LastTestDetail = ""
	r.LastTestedAt = nil
	f.rows[id] = r
	return nil
}
func (f *fakeConnStore) RecordTest(_ context.Context, id, kind, outcome, detail string, at time.Time) error {
	r := f.rows[id]
	r.ConnectionID = id
	r.ProviderKind = kind
	r.LastTestOutcome = outcome
	r.LastTestDetail = detail
	t := at
	r.LastTestedAt = &t
	f.rows[id] = r
	return nil
}
func (f *fakeConnStore) SetEnabled(_ context.Context, id string, enabled bool) error {
	r, ok := f.rows[id]
	if !ok {
		return connstore.ErrNoRow
	}
	r.Enabled = enabled
	f.rows[id] = r
	return nil
}

type fakeProbe struct{ result ProbeResult }

func (p fakeProbe) Probe(_ context.Context, _ config.ModelConnection, _ map[string]string) ProbeResult {
	return p.result
}

// --- helpers ----------------------------------------------------------------

func syntheticVault(t *testing.T) *connvault.SecretVault {
	t.Helper()
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		t.Fatalf("synthetic master key: %v", err)
	}
	v, err := connvault.NewSecretVault(base64.StdEncoding.EncodeToString(raw), 1)
	if err != nil {
		t.Fatalf("build synthetic vault: %v", err)
	}
	return v
}

func dbConn(id, kind string) config.ModelConnection {
	return config.ModelConnection{
		ID:        id,
		Kind:      kind,
		SecretRef: config.ModelConnectionSecretRef{Mode: config.ModelConnectionSecretModeDB},
		Models:    config.ModelConnectionModels{Strategy: "curated", List: []config.ModelDescriptor{{ID: kind + "/m1"}}},
	}
}

func mountAdmin(h *ModelConnectionsAdminHandler) http.Handler {
	r := chi.NewRouter()
	r.Get("/v1/admin/model-connections", h.List)
	r.Get("/v1/admin/model-connections/{id}", h.GetOne)
	r.Put("/v1/admin/model-connections/{id}/credential", h.PutCredential)
	r.Post("/v1/admin/model-connections/{id}/test", h.Test)
	r.Post("/v1/admin/model-connections/{id}/enable", h.Enable)
	r.Post("/v1/admin/model-connections/{id}/disable", h.Disable)
	return r
}

func doAdmin(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	var rdr *strings.Reader
	if body == "" {
		rdr = strings.NewReader("")
	} else {
		rdr = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rdr)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decodeJSON(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, rec.Body.String())
	}
	return m
}

// --- tests ------------------------------------------------------------------

// TestAdminModelConnections_PutCredentialWriteOnly_RedactedView_Spec096
// (ADVERSARIAL) — PUT …/credential stores write-only and returns a redacted
// view; the cleartext secret is NEVER echoed in the response body. Fails if the
// secret is ever returned.
func TestAdminModelConnections_PutCredentialWriteOnly_RedactedView_Spec096(t *testing.T) {
	store := newFakeConnStore(dbConn("anthropic-primary", config.ModelConnectionKindAnthropic))
	h := mountAdmin(NewModelConnectionsAdminHandler(store, syntheticVault(t), fakeProbe{}))

	const apiKey = "sk-ant-synthetic-scope06-DEADbeef0000WXYZ" // gitleaks:allow
	rec := doAdmin(h, http.MethodPut, "/v1/admin/model-connections/anthropic-primary/credential",
		`{"secret_fields":{"api_key":"`+apiKey+`"}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT credential status=%d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	// ADVERSARIAL: the cleartext secret MUST NOT appear anywhere in the response.
	if strings.Contains(rec.Body.String(), apiKey) {
		t.Fatalf("write-only violation: the response echoed the cleartext credential; body=%s", rec.Body.String())
	}

	view := decodeJSON(t, rec)
	if view["secret_present"] != true {
		t.Fatalf("redacted view MUST report secret_present=true; got %v", view["secret_present"])
	}
	if view["secret_redaction"] != "…WXYZ" {
		t.Fatalf("redacted view secret_redaction=%v, want last-4 hint …WXYZ", view["secret_redaction"])
	}
	// And the stored record carries NO plaintext (the VaultRecord has no such field).
	if got := store.rows["anthropic-primary"]; got.Secret == nil || len(got.Secret.Ciphertext) == 0 {
		t.Fatal("PUT MUST persist an encrypted vault record")
	}

	// A read surface (GET) also never echoes the secret.
	getRec := doAdmin(h, http.MethodGet, "/v1/admin/model-connections/anthropic-primary", "")
	if strings.Contains(getRec.Body.String(), apiKey) {
		t.Fatalf("write-only violation: GET echoed the cleartext credential; body=%s", getRec.Body.String())
	}
}

// TestAdminModelConnections_TestConnection_TruthfulOutcome_Spec096 — POST …/test
// reports the TRUTHFUL probe outcome (ok on a reachable+authenticated probe);
// last_test_outcome is persisted.
func TestAdminModelConnections_TestConnection_TruthfulOutcome_Spec096(t *testing.T) {
	store := newFakeConnStore(dbConn("anthropic-primary", config.ModelConnectionKindAnthropic))
	probe := fakeProbe{result: ProbeResult{Outcome: connstore.TestOutcomeOK}}
	h := mountAdmin(NewModelConnectionsAdminHandler(store, syntheticVault(t), probe))

	// Store a credential first (test probes the stored credential).
	if rec := doAdmin(h, http.MethodPut, "/v1/admin/model-connections/anthropic-primary/credential",
		`{"secret_fields":{"api_key":"sk-ant-synthetic-ok-000000000000WXYZ"}}`); rec.Code != http.StatusOK { // gitleaks:allow
		t.Fatalf("seed credential: status=%d body=%s", rec.Code, rec.Body.String())
	}

	rec := doAdmin(h, http.MethodPost, "/v1/admin/model-connections/anthropic-primary/test", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("POST test status=%d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	res := decodeJSON(t, rec)
	if res["outcome"] != connstore.TestOutcomeOK {
		t.Fatalf("truthful outcome=%v, want ok", res["outcome"])
	}
	if got := store.rows["anthropic-primary"]; got.LastTestOutcome != connstore.TestOutcomeOK {
		t.Fatalf("last_test_outcome persisted=%q, want ok", got.LastTestOutcome)
	}
}

// TestAdminModelConnections_PerKindSecretFields_OpenAIFoundryGoogleBedrock_Spec096
// — each kind saves its provider-specific write-only secret fields (OpenAI /
// Azure-Foundry / Google / Bedrock) with non-secret params read-only from the
// SST registry; a missing required field is 422.
func TestAdminModelConnections_PerKindSecretFields_OpenAIFoundryGoogleBedrock_Spec096(t *testing.T) {
	store := newFakeConnStore(
		dbConn("openai-primary", config.ModelConnectionKindOpenAI),
		dbConn("foundry-primary", config.ModelConnectionKindAzureFoundry),
		dbConn("google-primary", config.ModelConnectionKindGoogle),
		dbConn("bedrock-primary", config.ModelConnectionKindBedrock),
	)
	h := mountAdmin(NewModelConnectionsAdminHandler(store, syntheticVault(t), fakeProbe{}))

	cases := []struct {
		id, body string
	}{
		{"openai-primary", `{"secret_fields":{"api_key":"sk-openai-synthetic-00000000000WXYZ"}}`},                                          // gitleaks:allow
		{"foundry-primary", `{"secret_fields":{"api_key":"azkey-synthetic-0000000000000WXYZ"}}`},                                           // gitleaks:allow
		{"google-primary", `{"secret_fields":{"service_account":"{\"type\":\"svc\",\"k\":\"vWXYZ\"}"}}`},                                   // gitleaks:allow
		{"bedrock-primary", `{"secret_fields":{"aws_access_key_id":"AKIASYNTH0000WXYZ","aws_secret_access_key":"awssecretSYNTH000WXYZ"}}`}, // gitleaks:allow
	}
	for _, tc := range cases {
		rec := doAdmin(h, http.MethodPut, "/v1/admin/model-connections/"+tc.id+"/credential", tc.body)
		if rec.Code != http.StatusOK {
			t.Fatalf("PUT %s status=%d, want 200; body=%s", tc.id, rec.Code, rec.Body.String())
		}
		if got := store.rows[tc.id]; got.Secret == nil || !got.HasCredential() {
			t.Fatalf("PUT %s MUST persist a credential", tc.id)
		}
		view := decodeJSON(t, rec)
		if view["secret_present"] != true {
			t.Fatalf("PUT %s view secret_present=%v, want true", tc.id, view["secret_present"])
		}
	}

	// Bedrock requires BOTH keys — a missing one is 422 (per-kind field contract).
	rec := doAdmin(h, http.MethodPut, "/v1/admin/model-connections/bedrock-primary/credential",
		`{"secret_fields":{"aws_access_key_id":"AKIASYNTH0000WXYZ"}}`) // gitleaks:allow
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bedrock missing secret key status=%d, want 422; body=%s", rec.Code, rec.Body.String())
	}
}

// TestAdminModelConnections_UnknownSlotRejected404_Spec096 (ADVERSARIAL) — an id
// not in the SST registry is 404 (closed-set fail-loud); fails if a UI-invented
// slot is ever accepted.
func TestAdminModelConnections_UnknownSlotRejected404_Spec096(t *testing.T) {
	store := newFakeConnStore(dbConn("anthropic-primary", config.ModelConnectionKindAnthropic))
	h := mountAdmin(NewModelConnectionsAdminHandler(store, syntheticVault(t), fakeProbe{}))

	for _, ep := range []struct{ method, path, body string }{
		{http.MethodGet, "/v1/admin/model-connections/grok-invented", ""},
		{http.MethodPut, "/v1/admin/model-connections/grok-invented/credential", `{"secret_fields":{"api_key":"x000WXYZ"}}`}, // gitleaks:allow
		{http.MethodPost, "/v1/admin/model-connections/grok-invented/test", ""},
		{http.MethodPost, "/v1/admin/model-connections/grok-invented/enable", ""},
		{http.MethodPost, "/v1/admin/model-connections/grok-invented/disable", ""},
	} {
		rec := doAdmin(h, ep.method, ep.path, ep.body)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s %s status=%d, want 404 (UI-invented slot accepted)", ep.method, ep.path, rec.Code)
		}
	}
	// And nothing was persisted for the invented slot.
	if _, ok := store.rows["grok-invented"]; ok {
		t.Fatal("a UI-invented slot must NEVER be persisted")
	}
}

// TestAdminModelConnections_EnableUntested_Blocked409_Spec096 (ADVERSARIAL) —
// enabling a slot with no credential or last_test_outcome != ok is 409; fails if
// an unverified connection is ever enabled into the catalog.
func TestAdminModelConnections_EnableUntested_Blocked409_Spec096(t *testing.T) {
	store := newFakeConnStore(dbConn("anthropic-primary", config.ModelConnectionKindAnthropic))
	h := mountAdmin(NewModelConnectionsAdminHandler(store, syntheticVault(t), fakeProbe{result: ProbeResult{Outcome: connstore.TestOutcomeOK}}))

	// No credential at all → 409.
	if rec := doAdmin(h, http.MethodPost, "/v1/admin/model-connections/anthropic-primary/enable", ""); rec.Code != http.StatusConflict {
		t.Fatalf("enable with no credential status=%d, want 409", rec.Code)
	}

	// Credential present but untested → still 409 (ADVERSARIAL: never enable unverified).
	if rec := doAdmin(h, http.MethodPut, "/v1/admin/model-connections/anthropic-primary/credential",
		`{"secret_fields":{"api_key":"sk-ant-synthetic-untested-0000WXYZ"}}`); rec.Code != http.StatusOK { // gitleaks:allow
		t.Fatalf("seed credential status=%d", rec.Code)
	}
	if rec := doAdmin(h, http.MethodPost, "/v1/admin/model-connections/anthropic-primary/enable", ""); rec.Code != http.StatusConflict {
		t.Fatalf("enable untested status=%d, want 409 (unverified connection enabled into the catalog)", rec.Code)
	}
	if store.rows["anthropic-primary"].Enabled {
		t.Fatal("an untested connection must NEVER be enabled")
	}

	// CONTROL: after a passing test, enable succeeds (the guard is not tautological).
	if rec := doAdmin(h, http.MethodPost, "/v1/admin/model-connections/anthropic-primary/test", ""); rec.Code != http.StatusOK {
		t.Fatalf("test status=%d", rec.Code)
	}
	rec := doAdmin(h, http.MethodPost, "/v1/admin/model-connections/anthropic-primary/enable", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("enable after passing test status=%d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if !store.rows["anthropic-primary"].Enabled {
		t.Fatal("a credentialed+tested connection MUST be enable-able")
	}

	// And disable removes it from the catalog (enabled=false).
	if rec := doAdmin(h, http.MethodPost, "/v1/admin/model-connections/anthropic-primary/disable", ""); rec.Code != http.StatusOK {
		t.Fatalf("disable status=%d", rec.Code)
	}
	if store.rows["anthropic-primary"].Enabled {
		t.Fatal("disable MUST set enabled=false")
	}
}

// TestAdminModelConnections_FailedTest_TypedError_NeverFalseSuccess_Spec096
// (ADVERSARIAL) — a failed probe yields outcome:failed with a typed detail and
// persists failed (never the secret); fails if a failed probe is ever reported
// ok or substitutes Ollama.
func TestAdminModelConnections_FailedTest_TypedError_NeverFalseSuccess_Spec096(t *testing.T) {
	store := newFakeConnStore(dbConn("anthropic-primary", config.ModelConnectionKindAnthropic))

	// Seed a credential so the probe runs.
	seed := func(h http.Handler) {
		if rec := doAdmin(h, http.MethodPut, "/v1/admin/model-connections/anthropic-primary/credential",
			`{"secret_fields":{"api_key":"sk-ant-synthetic-bad-00000000WXYZ"}}`); rec.Code != http.StatusOK { // gitleaks:allow
			t.Fatalf("seed credential status=%d", rec.Code)
		}
	}

	// A probe that returns a typed failure.
	hFail := mountAdmin(NewModelConnectionsAdminHandler(store, syntheticVault(t),
		fakeProbe{result: ProbeResult{Outcome: connstore.TestOutcomeFailed, Detail: ProbeDetailAuthFailed}}))
	seed(hFail)
	rec := doAdmin(hFail, http.MethodPost, "/v1/admin/model-connections/anthropic-primary/test", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("POST test status=%d, want 200 with a failed body; got %s", rec.Code, rec.Body.String())
	}
	res := decodeJSON(t, rec)
	if res["outcome"] != connstore.TestOutcomeFailed {
		t.Fatalf("ADVERSARIAL: a failed probe reported outcome=%v, want failed (false success)", res["outcome"])
	}
	if res["detail"] != ProbeDetailAuthFailed {
		t.Fatalf("failed probe detail=%v, want typed %q", res["detail"], ProbeDetailAuthFailed)
	}
	if got := store.rows["anthropic-primary"]; got.LastTestOutcome != connstore.TestOutcomeFailed {
		t.Fatalf("persisted last_test_outcome=%q, want failed", got.LastTestOutcome)
	}
	// A failed slot cannot be enabled (409) — the failure is load-bearing.
	if er := doAdmin(hFail, http.MethodPost, "/v1/admin/model-connections/anthropic-primary/enable", ""); er.Code != http.StatusConflict {
		t.Fatalf("enable after failed test status=%d, want 409", er.Code)
	}

	// ADVERSARIAL defense: a probe that returns a MALFORMED/empty outcome is
	// treated as a failure, NEVER silently passed as ok.
	store2 := newFakeConnStore(dbConn("anthropic-primary", config.ModelConnectionKindAnthropic))
	hMalformed := mountAdmin(NewModelConnectionsAdminHandler(store2, syntheticVault(t),
		fakeProbe{result: ProbeResult{Outcome: "garbage"}}))
	seed2 := func(h http.Handler) {
		_ = doAdmin(h, http.MethodPut, "/v1/admin/model-connections/anthropic-primary/credential",
			`{"secret_fields":{"api_key":"sk-ant-synthetic-bad2-0000000WXYZ"}}`) // gitleaks:allow
	}
	seed2(hMalformed)
	rec2 := doAdmin(hMalformed, http.MethodPost, "/v1/admin/model-connections/anthropic-primary/test", "")
	res2 := decodeJSON(t, rec2)
	if res2["outcome"] == connstore.TestOutcomeOK {
		t.Fatal("ADVERSARIAL: a malformed probe outcome was reported ok — a probe must NEVER yield a false success")
	}
	if store2.rows["anthropic-primary"].LastTestOutcome == connstore.TestOutcomeOK {
		t.Fatal("a malformed probe outcome must NEVER persist ok")
	}
}
