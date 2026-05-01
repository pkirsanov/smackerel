package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/drive"
	"github.com/smackerel/smackerel/internal/drive/rules"
	"github.com/smackerel/smackerel/internal/drive/save"
)

// DriveRulesHandlers exposes Spec 038 Scope 5 Save Rules CRUD + audit + dry-run.
type DriveRulesHandlers struct {
	repo     *rules.Repository
	engine   *rules.Engine
	pool     *pgxpool.Pool
	registry save.ProviderResolver
}

// NewDriveRulesHandlers constructs the handler set. pool is required for
// dry-run artifact lookup; registry is used to validate the provider id.
func NewDriveRulesHandlers(pool *pgxpool.Pool, registry save.ProviderResolver) *DriveRulesHandlers {
	if pool == nil {
		return nil
	}
	return &DriveRulesHandlers{
		repo:     rules.NewRepository(pool),
		engine:   rules.NewEngine(time.Now),
		pool:     pool,
		registry: registry,
	}
}

// DriveRuleView is the wire shape for a Save Rule.
type DriveRuleView struct {
	ID                   string              `json:"id"`
	Name                 string              `json:"name"`
	Enabled              bool                `json:"enabled"`
	SourceKinds          []string            `json:"source_kinds"`
	Classification       string              `json:"classification"`
	SensitivityIn        []string            `json:"sensitivity_in"`
	ConfidenceMin        float64             `json:"confidence_min"`
	ProviderID           string              `json:"provider_id"`
	TargetFolderTemplate string              `json:"target_folder_template"`
	OnMissingFolder      string              `json:"on_missing_folder"`
	OnExistingFile       string              `json:"on_existing_file"`
	Guardrails           DriveRuleGuardrails `json:"guardrails"`
	CreatedAt            string              `json:"created_at"`
	UpdatedAt            string              `json:"updated_at"`
}

// DriveRuleGuardrails mirrors rules.Guardrails on the wire.
type DriveRuleGuardrails struct {
	NeverLinkShare      bool    `json:"never_link_share"`
	RequireConfirmBelow float64 `json:"require_confirm_below"`
}

// DriveRulesListResponse is the response for GET /v1/drive/rules.
type DriveRulesListResponse struct {
	Rules []DriveRuleView `json:"rules"`
}

// DriveRulesAuditView is one row from /v1/drive/rules/{id}/audit.
type DriveRulesAuditView struct {
	ID               int64  `json:"id"`
	RuleID           string `json:"rule_id"`
	SourceArtifactID string `json:"source_artifact_id"`
	Outcome          string `json:"outcome"`
	Reason           string `json:"reason"`
	CreatedAt        string `json:"created_at"`
}

// DriveRulesAuditResponse is the response for /v1/drive/rules/audit.
type DriveRulesAuditResponse struct {
	Rows []DriveRulesAuditView `json:"rows"`
}

// DriveRuleTestRequest is the body for POST /v1/drive/rules/{id}/test.
type DriveRuleTestRequest struct {
	SourceArtifactID string            `json:"source_artifact_id"`
	SourceKind       string            `json:"source_kind"`
	Classification   string            `json:"classification"`
	Sensitivity      string            `json:"sensitivity"`
	Confidence       float64           `json:"confidence"`
	Tokens           map[string]string `json:"tokens"`
	CapturedAt       string            `json:"captured_at"`
}

// DriveRuleTestResponse describes the dry-run outcome of the rule.
type DriveRuleTestResponse struct {
	Matched         bool   `json:"matched"`
	Reason          string `json:"reason"`
	RenderedPath    string `json:"rendered_path"`
	RenderError     string `json:"render_error"`
	ConfirmRequired bool   `json:"confirm_required"`
}

// List handles GET /v1/drive/rules.
func (h *DriveRulesHandlers) List(w http.ResponseWriter, r *http.Request) {
	rs, err := h.repo.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}
	out := make([]DriveRuleView, 0, len(rs))
	for _, rule := range rs {
		out = append(out, ruleToView(rule))
	}
	writeJSON(w, http.StatusOK, DriveRulesListResponse{Rules: out})
}

// Get handles GET /v1/drive/rules/{id}.
func (h *DriveRulesHandlers) Get(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "missing rule id")
		return
	}
	rule, err := h.repo.Get(r.Context(), id)
	if errors.Is(err, rules.ErrRuleNotFound) {
		writeError(w, http.StatusNotFound, "RULE_NOT_FOUND", "no save rule with id "+id)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ruleToView(rule))
}

// Create handles POST /v1/drive/rules.
func (h *DriveRulesHandlers) Create(w http.ResponseWriter, r *http.Request) {
	var view DriveRuleView
	if !decodeJSONBody(w, r, &view, "INVALID_REQUEST", "invalid JSON body") {
		return
	}
	rule := viewToRule(view)
	if h.registry != nil {
		if _, ok := h.registry.Get(rule.ProviderID); !ok {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "unknown provider_id: "+rule.ProviderID)
			return
		}
	}
	created, err := h.repo.Create(r.Context(), rule)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_RULE", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, ruleToView(created))
}

// Update handles PUT /v1/drive/rules/{id}.
func (h *DriveRulesHandlers) Update(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "missing rule id")
		return
	}
	var view DriveRuleView
	if !decodeJSONBody(w, r, &view, "INVALID_REQUEST", "invalid JSON body") {
		return
	}
	rule := viewToRule(view)
	rule.ID = id
	updated, err := h.repo.Update(r.Context(), rule)
	if errors.Is(err, rules.ErrRuleNotFound) {
		writeError(w, http.StatusNotFound, "RULE_NOT_FOUND", "no save rule with id "+id)
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_RULE", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ruleToView(updated))
}

// Delete handles DELETE /v1/drive/rules/{id}.
func (h *DriveRulesHandlers) Delete(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "missing rule id")
		return
	}
	if err := h.repo.Delete(r.Context(), id); errors.Is(err, rules.ErrRuleNotFound) {
		writeError(w, http.StatusNotFound, "RULE_NOT_FOUND", "no save rule with id "+id)
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Test handles POST /v1/drive/rules/{id}/test (Screen 8 dry-run).
func (h *DriveRulesHandlers) Test(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "missing rule id")
		return
	}
	var req DriveRuleTestRequest
	if !decodeJSONBody(w, r, &req, "INVALID_REQUEST", "invalid JSON body") {
		return
	}
	rule, err := h.repo.Get(r.Context(), id)
	if errors.Is(err, rules.ErrRuleNotFound) {
		writeError(w, http.StatusNotFound, "RULE_NOT_FOUND", "no save rule with id "+id)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	artifact := rules.Artifact{
		ID:             req.SourceArtifactID,
		SourceKind:     req.SourceKind,
		Classification: req.Classification,
		Sensitivity:    req.Sensitivity,
		Confidence:     req.Confidence,
		Tokens:         req.Tokens,
	}
	if req.CapturedAt != "" {
		if t, err := time.Parse(time.RFC3339, req.CapturedAt); err == nil {
			artifact.CapturedAt = t
		}
	}
	decision := h.engine.Evaluate(r.Context(), artifact, []rules.Rule{rule})
	resp := DriveRuleTestResponse{}
	if decision.Selected != nil {
		resp.Matched = true
		resp.Reason = decision.Selected.Reason
		resp.RenderedPath = decision.Selected.RenderedPath
		if decision.Selected.RenderError != nil {
			resp.RenderError = decision.Selected.RenderError.Error()
		}
		resp.ConfirmRequired = decision.Selected.ConfirmRequired
	} else if len(decision.Outcomes) == 1 {
		resp.Reason = decision.Outcomes[0].Reason
	} else {
		resp.Reason = "no_match"
	}
	writeJSON(w, http.StatusOK, resp)
}

// Audit handles GET /v1/drive/rules/audit (Screen 7 audit feed).
// Optional ?rule_id={id} narrows the feed; ?limit=N (default 50) controls
// the page size.
func (h *DriveRulesHandlers) Audit(w http.ResponseWriter, r *http.Request) {
	ruleID := strings.TrimSpace(r.URL.Query().Get("rule_id"))
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := parseLimit(raw); err == nil {
			limit = parsed
		}
	}
	rows, err := h.repo.ListAudit(r.Context(), ruleID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}
	out := make([]DriveRulesAuditView, 0, len(rows))
	for _, row := range rows {
		out = append(out, DriveRulesAuditView{
			ID:               row.ID,
			RuleID:           row.RuleID,
			SourceArtifactID: row.SourceArtifactID,
			Outcome:          string(row.Outcome),
			Reason:           row.Reason,
			CreatedAt:        row.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	writeJSON(w, http.StatusOK, DriveRulesAuditResponse{Rows: out})
}

func ruleToView(r rules.Rule) DriveRuleView {
	view := DriveRuleView{
		ID:                   r.ID,
		Name:                 r.Name,
		Enabled:              r.Enabled,
		SourceKinds:          r.SourceKinds,
		Classification:       r.Classification,
		SensitivityIn:        r.SensitivityIn,
		ConfidenceMin:        r.ConfidenceMin,
		ProviderID:           r.ProviderID,
		TargetFolderTemplate: r.TargetFolderTemplate,
		OnMissingFolder:      string(r.OnMissingFolder),
		OnExistingFile:       string(r.OnExistingFile),
		Guardrails: DriveRuleGuardrails{
			NeverLinkShare:      r.Guardrails.NeverLinkShare,
			RequireConfirmBelow: r.Guardrails.RequireConfirmBelow,
		},
		CreatedAt: r.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: r.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if view.SourceKinds == nil {
		view.SourceKinds = []string{}
	}
	if view.SensitivityIn == nil {
		view.SensitivityIn = []string{}
	}
	return view
}

func viewToRule(v DriveRuleView) rules.Rule {
	return rules.Rule{
		ID:                   v.ID,
		Name:                 v.Name,
		Enabled:              v.Enabled,
		SourceKinds:          v.SourceKinds,
		Classification:       v.Classification,
		SensitivityIn:        v.SensitivityIn,
		ConfidenceMin:        v.ConfidenceMin,
		ProviderID:           v.ProviderID,
		TargetFolderTemplate: v.TargetFolderTemplate,
		OnMissingFolder:      rules.OnMissingFolder(v.OnMissingFolder),
		OnExistingFile:       rules.OnExistingFile(v.OnExistingFile),
		Guardrails: rules.Guardrails{
			NeverLinkShare:      v.Guardrails.NeverLinkShare,
			RequireConfirmBelow: v.Guardrails.RequireConfirmBelow,
		},
	}
}

func parseLimit(s string) (int, error) {
	var n int
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		return 0, err
	}
	if n <= 0 || n > 1000 {
		return 0, fmt.Errorf("limit out of range: %d", n)
	}
	return n, nil
}

// adapterProviderResolver wraps drive.DefaultRegistry into the
// save.ProviderResolver interface; the interface intentionally exposes
// only Get so the handlers cannot iterate or mutate the registry.
type adapterProviderResolver struct {
	registry interface {
		Get(id string) (drive.Provider, bool)
	}
}

// NewProviderResolverAdapter wraps a drive registry-like object so it
// satisfies save.ProviderResolver (and DriveRulesHandlers' provider check).
func NewProviderResolverAdapter(registry DriveProviderRegistry) save.ProviderResolver {
	return &adapterProviderResolver{registry: registry}
}

func (a *adapterProviderResolver) Get(id string) (drive.Provider, bool) {
	if a == nil || a.registry == nil {
		return nil, false
	}
	return a.registry.Get(id)
}

var _ = json.Marshal // keep encoding/json import even when none of the
//                     handler methods explicitly call it directly through
//                     this file (writeJSON encapsulates marshalling).
