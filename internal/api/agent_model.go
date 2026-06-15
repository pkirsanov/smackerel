// Spec 089 (Fork B/C) — claim-bound GET/PUT/DELETE /v1/agent/model.
//
// The per-user sticky open-knowledge /ask synthesis-model preference surface,
// mirroring the Telegram /model set/show/reset CRUD. It reads the SAME
// agenttool.ModelPref() store + agenttool.SwitchableModels() validator the /ask
// fast-path uses, so Telegram and HTTP render the SAME result from ONE store +
// ONE validator (SCN-089-A11 parity).
//
// CLAIM-BOUND (spec 044 / OWASP A01): the actor is ALWAYS the PASETO bearer
// subject (auth.UserIDFromContext); the route is mounted under
// bearerAuthMiddleware. The PUT body carries ONLY {model} and NEVER a user id —
// a user-id-shaped body field is structurally ignored by the decode, so a
// spoofed actor can never reach the store key.
package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/smackerel/smackerel/internal/assistant/openknowledge/agenttool"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/modelpref"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/modelswitch"
	"github.com/smackerel/smackerel/internal/auth"
)

// AgentModelHandler serves the /v1/agent/model surface. It holds no state — it
// reads the late-bound agenttool singletons at request time (installed by
// wireOpenKnowledge), exactly like the /agent/invoke fast-path.
type AgentModelHandler struct{}

// agentModelView is the GET / PUT / DELETE response envelope.
type agentModelView struct {
	EffectiveModel string   `json:"effective_model"`
	Source         string   `json:"source"` // "sticky" | "default"
	StickyModel    string   `json:"sticky_model,omitempty"`
	SystemDefault  string   `json:"system_default"`
	AllowedModels  []string `json:"allowed_models"`
}

// agentModelPutRequest is the PUT body. It carries ONLY the model id; a
// user-id-shaped field a caller adds is ignored by this decode (claim-binding
// is enforced by deriving the subject from the bearer token, not the body).
type agentModelPutRequest struct {
	Model string `json:"model"`
}

// Get handles GET /v1/agent/model — show the caller's effective model, its
// source, the system default, and the full switchable set.
func (h *AgentModelHandler) Get(w http.ResponseWriter, r *http.Request) {
	subject, allow, store, ok := h.resolve(w, r)
	if !ok {
		return
	}
	view := agentModelView{
		EffectiveModel: allow.DefaultModel(),
		Source:         "default",
		SystemDefault:  allow.DefaultModel(),
		AllowedModels:  allow.AllowedModels(),
	}
	if pref, found, err := store.Get(r.Context(), subject); err == nil && found && strings.TrimSpace(pref.SynthesisModel) != "" {
		view.EffectiveModel = pref.SynthesisModel
		view.StickyModel = pref.SynthesisModel
		view.Source = "sticky"
	}
	writeModelJSON(w, http.StatusOK, view)
}

// Put handles PUT /v1/agent/model {model} — set the caller's sticky model. An
// off-allowlist model ⇒ HTTP 400 rejection envelope (the SAME shape Telegram
// renders) and the existing preference is UNCHANGED (the failed set is a no-op).
func (h *AgentModelHandler) Put(w http.ResponseWriter, r *http.Request) {
	subject, allow, store, ok := h.resolve(w, r)
	if !ok {
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 8*1024))
	if err != nil {
		writeModelError(w, http.StatusBadRequest, "body_read_error")
		return
	}
	var req agentModelPutRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeModelError(w, http.StatusBadRequest, "body_invalid_json")
		return
	}
	resolved, rej := allow.Resolve(strings.TrimSpace(req.Model))
	if rej != nil {
		// Off-allowlist: render the shared rejection verbatim; NO store write,
		// so the caller's existing sticky preference is unchanged.
		writeOpenKnowledgeRejection(w, rej)
		return
	}
	if resolved.SynthesisModel == "" {
		writeModelError(w, http.StatusBadRequest, "model_required")
		return
	}
	if err := store.Set(r.Context(), subject, resolved.SynthesisModel); err != nil {
		slog.Warn("agent_model: set preference failed", "error", err)
		writeModelError(w, http.StatusInternalServerError, "preference_write_failed")
		return
	}
	writeModelJSON(w, http.StatusOK, agentModelView{
		EffectiveModel: resolved.SynthesisModel,
		Source:         "sticky",
		StickyModel:    resolved.SynthesisModel,
		SystemDefault:  allow.DefaultModel(),
		AllowedModels:  allow.AllowedModels(),
	})
}

// Delete handles DELETE /v1/agent/model — reset the caller's sticky model. The
// next /ask resolves the SST default. Idempotent.
func (h *AgentModelHandler) Delete(w http.ResponseWriter, r *http.Request) {
	subject, allow, store, ok := h.resolve(w, r)
	if !ok {
		return
	}
	if err := store.Clear(r.Context(), subject); err != nil {
		slog.Warn("agent_model: clear preference failed", "error", err)
		writeModelError(w, http.StatusInternalServerError, "preference_clear_failed")
		return
	}
	writeModelJSON(w, http.StatusOK, agentModelView{
		EffectiveModel: allow.DefaultModel(),
		Source:         "default",
		SystemDefault:  allow.DefaultModel(),
		AllowedModels:  allow.AllowedModels(),
	})
}

// resolve derives the claim-bound subject + the shared validator/store, writing
// the appropriate error and returning ok=false when any is missing.
func (h *AgentModelHandler) resolve(w http.ResponseWriter, r *http.Request) (subject string, allow *modelswitch.Allowlist, store modelpref.Store, ok bool) {
	subject = auth.UserIDFromContext(r.Context())
	if subject == "" {
		writeModelError(w, http.StatusForbidden, "authenticated_subject_required")
		return "", nil, nil, false
	}
	a := agenttool.SwitchableModels()
	s := agenttool.ModelPref()
	if a == nil || s == nil {
		writeModelError(w, http.StatusServiceUnavailable, "model_selection_not_enabled")
		return "", nil, nil, false
	}
	return subject, a, s, true
}

func writeModelJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil && !errors.Is(err, http.ErrBodyNotAllowed) {
		slog.Warn("agent_model: encode response failed", "error", err)
	}
}

func writeModelError(w http.ResponseWriter, status int, code string) {
	writeModelJSON(w, status, map[string]string{"status": "error", "error_code": code})
}
