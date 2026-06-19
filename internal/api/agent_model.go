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
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/smackerel/smackerel/internal/assistant/openknowledge/agenttool"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/catalog"
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
	// Spec 096 SCOPE-07 (R2) — ADDITIVE catalog enrichment. allowed_models[]
	// above is preserved BYTE-FOR-BYTE for existing 089 clients (same ordering,
	// same provider-qualified strings); the fields below are omitempty siblings
	// populated ONLY when the combined-catalog source is wired, so an 089-era
	// consumer that reads only allowed_models / effective_model / sticky_model /
	// source sees no change (proven by
	// TestAgentModel_GetEnrichedCatalogAdditive_AllowedModelsPreserved_Spec096).
	Catalog          []agentModelCatalogEntry   `json:"catalog,omitempty"`
	ProviderStatuses []agentModelProviderStatus `json:"provider_statuses,omitempty"`
	Budget           *agentModelBudgetView      `json:"budget,omitempty"`
}

// agentModelCatalogEntry is one REACHABLE provider-qualified model in the
// combined catalog, carrying the SCOPE-07 additive enrichment (capabilities[] +
// cost_class). Only reachable models appear here (so a selectable entry always
// maps to a dispatchable model); unreachable providers are surfaced via
// ProviderStatuses (shown-but-disabled), never silently dropped.
type agentModelCatalogEntry struct {
	ID            string   `json:"id"`
	Kind          string   `json:"kind"`
	Capabilities  []string `json:"capabilities,omitempty"`
	CostClass     string   `json:"cost_class"` // "free" (ollama/local) | "paid" (hosted)
	ContextWindow int      `json:"context_window,omitempty"`
}

// agentModelProviderStatus is the typed per-connection discovery status, ALWAYS
// emitted for every effective-enabled connection (reachable or not) so the web
// picker can render an unreachable provider shown-but-disabled with its typed
// state — never silently dropped (graceful degradation, Principle 8).
type agentModelProviderStatus struct {
	ConnectionID string `json:"connection_id"`
	Kind         string `json:"kind"`
	State        string `json:"state"` // "ok" | "unreachable" | "timeout" | "auth_failed" | "disabled"
	ModelCount   int    `json:"model_count"`
	Reachable    bool   `json:"reachable"`
}

// agentModelBudgetView is the optional month-to-date USD spend enrichment (from
// the SCOPE-05 SpendLedger), present only when a budget source is wired AND a
// paid model is in the catalog (Principle 6 pull-not-push).
type agentModelBudgetView struct {
	MonthToDateUSD       float64 `json:"month_to_date_usd"`
	MonthToDateGlobalUSD float64 `json:"month_to_date_global_usd"`
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
	// Spec 096 SCOPE-07 (R2) — additive combined-catalog + budget enrichment.
	// allowed_models[] is left untouched (byte-for-byte 089); the enrichment is
	// sibling omitempty fields populated only when a catalog source is wired.
	enrichAgentModelView(r.Context(), &view)
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

// enrichAgentModelView adds the spec-096 SCOPE-07 additive catalog + budget
// enrichment to a GET view. It is a no-op (leaving the byte-for-byte 089 shape)
// when no combined-catalog source is wired (the deferred live-aggregator
// activation state). allowed_models[] is NEVER touched here — the enrichment is
// purely additive sibling fields, so an 089 client is unaffected (R2).
func enrichAgentModelView(ctx context.Context, view *agentModelView) {
	source := agenttool.ModelCatalogProvider()
	if source == nil {
		return
	}
	cat, statuses := source.GetCatalog(ctx)

	entries := make([]agentModelCatalogEntry, 0, len(cat.Models))
	hasPaid := false
	for _, m := range cat.Models {
		entries = append(entries, agentModelCatalogEntry{
			ID:            m.ID,
			Kind:          m.Kind,
			Capabilities:  modelCapabilities(m),
			CostClass:     catalogCostClass(m.Kind),
			ContextWindow: m.ContextWindow,
		})
		if m.Kind != catalogKindOllama {
			hasPaid = true
		}
	}
	if len(entries) > 0 {
		view.Catalog = entries
	}

	if len(statuses) > 0 {
		ps := make([]agentModelProviderStatus, 0, len(statuses))
		for _, st := range statuses {
			ps = append(ps, agentModelProviderStatus{
				ConnectionID: st.ConnectionID,
				Kind:         st.Kind,
				State:        string(st.State),
				ModelCount:   st.ModelCount,
				Reachable:    st.State == catalog.StateOK,
			})
		}
		view.ProviderStatuses = ps
	}

	// Budget enrichment (optional) — only when a paid model is in the catalog
	// AND a budget source is wired (Principle 6 pull-not-push).
	if hasPaid {
		if budget := agenttool.CurrentBudgetProvider(); budget != nil {
			if perUser, global, err := budget.MonthToDateSpend(ctx); err == nil {
				view.Budget = &agentModelBudgetView{
					MonthToDateUSD:       perUser,
					MonthToDateGlobalUSD: global,
				}
			}
		}
	}
}

// catalogKindOllama is the local-inference kind that anchors the free cost
// class. Mirrors config.ModelConnectionKindOllama; a local const keeps this file
// free of a config import (same pattern as catalog.go).
const catalogKindOllama = "ollama"

// catalogCostClass is the machine cost hint: ollama/local is "free", every
// hosted provider is "paid" (the per-model rate lives in SCOPE-05 model_costs).
func catalogCostClass(kind string) string {
	if kind == catalogKindOllama {
		return "free"
	}
	return "paid"
}

// modelCapabilities flattens a descriptor's capability flags into the additive
// capabilities[] enrichment (omitempty when none).
func modelCapabilities(m catalog.ModelDescriptor) []string {
	var caps []string
	if m.ToolCapable {
		caps = append(caps, "tool_capable")
	}
	if m.Vision {
		caps = append(caps, "vision")
	}
	return caps
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
