// Spec 089 SCOPE-04 — HTTP /v1/agent/model CRUD tests. They drive the real
// AgentModelHandler against the agenttool singletons (a real modelswitch
// allowlist + a fake claim-bound store) with an authenticated request context,
// proving: GET shows effective/allowed/default; PUT sets (claim-bound to the
// PASETO subject, a spoofed body user id is ignored); an off-allowlist PUT is a
// 400 no-op; DELETE resets; and the HTTP rejection is byte-identical to the
// shared validator (the SAME sentence the Telegram /model surface renders).
// SCN-089-A03/A04/A08/A11.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/smackerel/smackerel/internal/assistant/openknowledge/agenttool"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/modelpref"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/modelswitch"
	"github.com/smackerel/smackerel/internal/auth"
)

type fakeModelPrefStore struct {
	m map[string]modelpref.Preference
}

func (f *fakeModelPrefStore) Get(_ context.Context, userID string) (modelpref.Preference, bool, error) {
	p, ok := f.m[userID]
	return p, ok, nil
}
func (f *fakeModelPrefStore) Set(_ context.Context, userID, synthesisModel string) error {
	if f.m == nil {
		f.m = map[string]modelpref.Preference{}
	}
	f.m[userID] = modelpref.Preference{SynthesisModel: synthesisModel}
	return nil
}
func (f *fakeModelPrefStore) Clear(_ context.Context, userID string) error {
	delete(f.m, userID)
	return nil
}

// spec089WireModel installs a self-hosted-shaped allowlist + the fake store into
// the agenttool singletons and returns the store + a cleanup.
func spec089WireModel(t *testing.T) (*fakeModelPrefStore, func()) {
	t.Helper()
	allow, err := modelswitch.NewAllowlist(
		[]string{"deepseek-r1:32b", "deepseek-r1:7b", "gemma4:26b"},
		map[string]int{"gemma4:26b": 18432, "deepseek-r1:7b": 4864, "deepseek-r1:32b": 22528, "llama3.1:8b": 6144},
		0,
		"gemma4:26b",
		"deepseek-r1:32b",
		[]string{"gemma4:26b", "llama3.1:8b"},
	)
	if err != nil {
		t.Fatalf("NewAllowlist: %v", err)
	}
	store := &fakeModelPrefStore{}
	agenttool.SetSwitchableModels(allow)
	agenttool.SetModelPref(store)
	return store, func() {
		agenttool.SetSwitchableModels(nil)
		agenttool.SetModelPref(nil)
	}
}

// modelReq drives the AgentModelHandler with an authenticated subject (the
// PASETO subject the bearer middleware would attach).
func modelReq(method, body, subject string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/v1/agent/model", bytes.NewReader([]byte(body)))
	if subject != "" {
		req = req.WithContext(auth.WithSession(req.Context(), auth.Session{UserID: subject, Source: auth.SessionSourcePerUserToken}))
	}
	rec := httptest.NewRecorder()
	h := &AgentModelHandler{}
	switch method {
	case http.MethodGet:
		h.Get(rec, req)
	case http.MethodPut:
		h.Put(rec, req)
	case http.MethodDelete:
		h.Delete(rec, req)
	}
	return rec
}

func decodeModelEnv(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var env map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v; body=%s", err, rec.Body.String())
	}
	return env
}

// TestAgentModel_GetShowsEffective_DeleteResets_Spec089 — GET shows the
// effective model + source + allowed set; PUT sets sticky; GET reflects it;
// DELETE resets to the SST default.
func TestAgentModel_GetShowsEffective_DeleteResets_Spec089(t *testing.T) {
	_, cleanup := spec089WireModel(t)
	defer cleanup()

	// GET inherited.
	env := decodeModelEnv(t, modelReq(http.MethodGet, "", "user-A"))
	if env["effective_model"] != "deepseek-r1:32b" || env["source"] != "default" {
		t.Fatalf("GET inherited: effective=%v source=%v want deepseek-r1:32b/default", env["effective_model"], env["source"])
	}
	if env["system_default"] != "deepseek-r1:32b" {
		t.Fatalf("GET system_default = %v, want deepseek-r1:32b", env["system_default"])
	}
	if _, ok := env["allowed_models"].([]any); !ok {
		t.Fatalf("GET MUST carry allowed_models, got %v", env)
	}

	// PUT set.
	putRec := modelReq(http.MethodPut, `{"model":"deepseek-r1:7b"}`, "user-A")
	if putRec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, want 200; body=%s", putRec.Code, putRec.Body.String())
	}
	putEnv := decodeModelEnv(t, putRec)
	if putEnv["effective_model"] != "deepseek-r1:7b" || putEnv["source"] != "sticky" {
		t.Fatalf("PUT envelope: effective=%v source=%v want deepseek-r1:7b/sticky", putEnv["effective_model"], putEnv["source"])
	}

	// GET reflects the sticky.
	getEnv := decodeModelEnv(t, modelReq(http.MethodGet, "", "user-A"))
	if getEnv["effective_model"] != "deepseek-r1:7b" || getEnv["source"] != "sticky" {
		t.Fatalf("GET after PUT: effective=%v source=%v want deepseek-r1:7b/sticky", getEnv["effective_model"], getEnv["source"])
	}

	// DELETE resets.
	delRec := modelReq(http.MethodDelete, "", "user-A")
	if delRec.Code != http.StatusOK {
		t.Fatalf("DELETE status = %d, want 200", delRec.Code)
	}
	delEnv := decodeModelEnv(t, delRec)
	if delEnv["effective_model"] != "deepseek-r1:32b" || delEnv["source"] != "default" {
		t.Fatalf("DELETE envelope: effective=%v source=%v want deepseek-r1:32b/default", delEnv["effective_model"], delEnv["source"])
	}
}

// TestAgentModel_Put_BodyUserIdIgnored_ClaimBoundToSubject_Spec089 — ADVERSARIAL.
// A PUT whose body attempts a different user id sets ONLY the PASETO subject's
// preference; the body id is ignored (OWASP A01). Fails if a spoofed body id
// reaches the store key.
func TestAgentModel_Put_BodyUserIdIgnored_ClaimBoundToSubject_Spec089(t *testing.T) {
	store, cleanup := spec089WireModel(t)
	defer cleanup()

	rec := modelReq(http.MethodPut, `{"model":"deepseek-r1:7b","user_id":"victim","owner":"victim"}`, "subject-A")
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	ctx := context.Background()
	if pref, ok, _ := store.Get(ctx, "subject-A"); !ok || pref.SynthesisModel != "deepseek-r1:7b" {
		t.Fatalf("PUT MUST set the PASETO subject's preference; got ok=%v pref=%+v", ok, pref)
	}
	if _, ok, _ := store.Get(ctx, "victim"); ok {
		t.Fatalf("CLAIM-BINDING BREACH: a spoofed body user id reached the store key")
	}
}

// TestAgentModel_Put_OffAllowlist_400_PreferenceUnchanged_Spec089 — ADVERSARIAL.
// An off-allowlist PUT ⇒ HTTP 400 rejection and the caller's existing sticky
// preference is UNCHANGED (the failed set is a no-op).
func TestAgentModel_Put_OffAllowlist_400_PreferenceUnchanged_Spec089(t *testing.T) {
	store, cleanup := spec089WireModel(t)
	defer cleanup()
	ctx := context.Background()
	if err := store.Set(ctx, "subject-A", "deepseek-r1:7b"); err != nil {
		t.Fatalf("seed Set: %v", err)
	}

	rec := modelReq(http.MethodPut, `{"model":"gpt-4o"}`, "subject-A")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("off-allowlist PUT status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	env := decodeModelEnv(t, rec)
	if env["error_code"] != modelswitch.ReasonNotAllowlisted {
		t.Fatalf("error_code = %v, want %q", env["error_code"], modelswitch.ReasonNotAllowlisted)
	}
	if pref, ok, _ := store.Get(ctx, "subject-A"); !ok || pref.SynthesisModel != "deepseek-r1:7b" {
		t.Fatalf("an off-allowlist set MUST be a no-op; preference changed to %+v", pref)
	}
}

// TestParity_SameStickyAndOffAllowlist_IdenticalAcrossSurfaces_Spec089 —
// ADVERSARIAL. The HTTP surface renders the SAME shared modelswitch validator
// output the Telegram /model surface renders verbatim, and reads/writes the
// SAME store. The HTTP 400 rejection message is byte-identical to
// allow.Resolve(offList).Message — which is EXACTLY what Telegram's
// modelCommandReply returns for an off-allowlist set (see
// internal/telegram/model_command.go + model_command_test.go). Fails if either
// surface reformats the validator sentence or diverges on the store.
func TestParity_SameStickyAndOffAllowlist_IdenticalAcrossSurfaces_Spec089(t *testing.T) {
	_, cleanup := spec089WireModel(t)
	defer cleanup()
	allow := agenttool.SwitchableModels()

	// Off-allowlist: the HTTP message == the shared validator's Resolve message.
	_, wantRej := allow.Resolve("gpt-4o")
	if wantRej == nil {
		t.Fatalf("expected a rejection for the off-allowlist model gpt-4o")
	}
	rec := modelReq(http.MethodPut, `{"model":"gpt-4o"}`, "subject-A")
	env := decodeModelEnv(t, rec)
	if env["message"] != wantRej.Message {
		t.Fatalf("HTTP rejection message MUST be byte-identical to the shared validator (the SAME sentence Telegram renders):\n got: %v\nwant: %q", env["message"], wantRej.Message)
	}

	// Allowlisted sticky: a PUT then a GET reflects the SAME shared store.
	if modelReq(http.MethodPut, `{"model":"deepseek-r1:7b"}`, "subject-A").Code != http.StatusOK {
		t.Fatalf("valid PUT must succeed")
	}
	getEnv := decodeModelEnv(t, modelReq(http.MethodGet, "", "subject-A"))
	if getEnv["effective_model"] != "deepseek-r1:7b" || getEnv["source"] != "sticky" {
		t.Fatalf("GET after PUT MUST reflect the shared store, got %v", getEnv)
	}
}

// TestAgentModel_NoSubject_Forbidden_Spec089 — an unauthenticated request (no
// PASETO subject) is refused; the body id can never substitute for the claim.
func TestAgentModel_NoSubject_Forbidden_Spec089(t *testing.T) {
	_, cleanup := spec089WireModel(t)
	defer cleanup()
	rec := modelReq(http.MethodGet, "", "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("unauthenticated GET status = %d, want 403", rec.Code)
	}
}
