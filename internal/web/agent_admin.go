// Spec 037 Scope 8 — admin web routes for the operator UI.
//
// The handlers in this file mirror the `smackerel agent ...` CLI
// subcommands one-for-one. Both surfaces share the rendering layer in
// internal/agent/render so a change to required outcome fields shows
// up everywhere at once.
//
// Routes live under /admin/agent/... and are wired in
// internal/api/router.go via the AgentAdminUI interface (see health.go).

package web

import (
	"context"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/agent/render"
)

// AgentAdminHandler serves the /admin/agent/... screens.
type AgentAdminHandler struct {
	Pool      *pgxpool.Pool
	Templates *template.Template
	// LoadScenarios is the indirection seam that lets tests inject a
	// fixed scenario set instead of reading config/agent on disk.
	LoadScenarios func() (registered []*agent.Scenario, rejected []agent.LoadError, fatal error)
}

// NewAgentAdminHandler builds the admin handler with the standard
// scenario loader (calls agent.LoadConfig + DefaultLoader).
func NewAgentAdminHandler(pool *pgxpool.Pool) *AgentAdminHandler {
	t := template.Must(template.New("agent_admin").Funcs(template.FuncMap{
		"sevClass": severityClass,
	}).Parse(agentAdminTemplates))
	return &AgentAdminHandler{
		Pool:      pool,
		Templates: t,
		LoadScenarios: func() ([]*agent.Scenario, []agent.LoadError, error) {
			cfg, err := agent.LoadConfig()
			if err != nil {
				return nil, nil, err
			}
			r, rej, fatal := agent.DefaultLoader().Load(cfg.ScenarioDir, cfg.ScenarioGlob)
			return r, rej, fatal
		},
	}
}

func severityClass(s render.Severity) string {
	switch s {
	case render.SeverityError:
		return "outcome-error"
	case render.SeverityWarning:
		return "outcome-warning"
	default:
		return "outcome-info"
	}
}

// TracesIndex handles GET /admin/agent/traces.
func (h *AgentAdminHandler) TracesIndex(w http.ResponseWriter, r *http.Request) {
	if h.Pool == nil {
		http.Error(w, "agent admin: trace store not configured", http.StatusServiceUnavailable)
		return
	}
	outcome := r.URL.Query().Get("outcome")
	if outcome != "" && !render.IsValidOutcomeClass(outcome) {
		http.Error(w, "unknown outcome class", http.StatusBadRequest)
		return
	}
	limit := parsePositiveInt(r.URL.Query().Get("limit"), 50, 1000)
	offset := parseNonNegativeInt(r.URL.Query().Get("offset"), 0)

	ctx, cancel := context.WithTimeout(r.Context(), 10_000_000_000) // 10s
	defer cancel()

	rows, err := agent.ListTraces(ctx, h.Pool, agent.TraceListFilter{Outcome: outcome}, limit, offset)
	if err != nil {
		slog.Error("agent admin: list traces failed", "error", err)
		http.Error(w, "list traces failed", http.StatusInternalServerError)
		return
	}
	total, err := agent.CountTraces(ctx, h.Pool, agent.TraceListFilter{Outcome: outcome})
	if err != nil {
		slog.Warn("agent admin: count traces failed", "error", err)
		total = len(rows)
	}
	summaries := make([]render.TraceSummary, 0, len(rows))
	for i := range rows {
		summaries = append(summaries, render.BuildTraceSummary(&rows[i]))
	}

	data := map[string]any{
		"Title":          "Agent Traces",
		"Rows":           summaries,
		"Total":          total,
		"OutcomeFilter":  outcome,
		"OutcomeClasses": render.AllOutcomeClasses(),
		"Limit":          limit,
		"Offset":         offset,
		"NextOffset":     offset + limit,
		"PrevOffset":     maxInt(0, offset-limit),
	}
	h.render(w, "agent_traces_index.html", data)
}

// TracesShow handles GET /admin/agent/traces/{id}.
func (h *AgentAdminHandler) TracesShow(w http.ResponseWriter, r *http.Request) {
	if h.Pool == nil {
		http.Error(w, "agent admin: trace store not configured", http.StatusServiceUnavailable)
		return
	}
	id := chi.URLParam(r, "id")
	if id == "" {
		http.Error(w, "missing trace id", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10_000_000_000)
	defer cancel()
	tr, err := agent.LoadTrace(ctx, h.Pool, id)
	if err != nil {
		http.Error(w, "trace not found", http.StatusNotFound)
		return
	}
	det := render.BuildTraceDetail(tr)
	h.render(w, "agent_trace_show.html", map[string]any{
		"Title":  "Trace " + det.Summary.TraceID,
		"Detail": det,
	})
}

// ScenariosIndex handles GET /admin/agent/scenarios.
func (h *AgentAdminHandler) ScenariosIndex(w http.ResponseWriter, r *http.Request) {
	registered, rejected, fatal := h.LoadScenarios()
	registeredViews := make([]render.ScenarioSummary, 0, len(registered))
	for _, s := range registered {
		registeredViews = append(registeredViews, render.BuildScenarioSummary(s))
	}
	rejectedViews := make([]render.LoadRejectionView, 0, len(rejected))
	for _, e := range rejected {
		rejectedViews = append(rejectedViews, render.BuildLoadRejection(e))
	}
	data := map[string]any{
		"Title":      "Agent Scenarios",
		"Registered": registeredViews,
		"Rejected":   rejectedViews,
		"FatalErr":   fatalString(fatal),
	}
	h.render(w, "agent_scenarios_index.html", data)
}

func fatalString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// ScenariosShow handles GET /admin/agent/scenarios/{id}.
func (h *AgentAdminHandler) ScenariosShow(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		http.Error(w, "missing scenario id", http.StatusBadRequest)
		return
	}
	registered, _, fatal := h.LoadScenarios()
	if fatal != nil && len(registered) == 0 {
		http.Error(w, fatal.Error(), http.StatusInternalServerError)
		return
	}
	for _, s := range registered {
		if s.ID == id {
			det := render.BuildScenarioDetail(s)
			h.render(w, "agent_scenario_show.html", map[string]any{
				"Title":  "Scenario " + det.Summary.ID,
				"Detail": det,
			})
			return
		}
	}
	http.Error(w, "scenario not found", http.StatusNotFound)
}

// ToolsIndex handles GET /admin/agent/tools.
func (h *AgentAdminHandler) ToolsIndex(w http.ResponseWriter, r *http.Request) {
	tools := agent.All()
	scenarios, _, _ := h.LoadScenarios()
	views := make([]render.ToolSummary, 0, len(tools))
	for _, t := range tools {
		views = append(views, render.BuildToolSummary(t, render.AllowlistedBy(t.Name, scenarios)))
	}
	h.render(w, "agent_tools_index.html", map[string]any{
		"Title": "Agent Tools",
		"Rows":  views,
	})
}

// ToolsShow handles GET /admin/agent/tools/{name}.
func (h *AgentAdminHandler) ToolsShow(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		http.Error(w, "missing tool name", http.StatusBadRequest)
		return
	}
	tool, ok := agent.ByName(name)
	if !ok {
		http.Error(w, "tool not registered", http.StatusNotFound)
		return
	}
	scenarios, _, _ := h.LoadScenarios()
	det := render.BuildToolDetail(tool, render.AllowlistedBy(name, scenarios))
	h.render(w, "agent_tool_show.html", map[string]any{
		"Title":  "Tool " + det.Summary.Name,
		"Detail": det,
	})
}

func (h *AgentAdminHandler) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.Templates.ExecuteTemplate(w, name, data); err != nil {
		slog.Error("agent admin template failed", "template", name, "error", err)
		http.Error(w, "template render failed", http.StatusInternalServerError)
	}
}

func parsePositiveInt(s string, def, max int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return def
	}
	if n > max {
		return max
	}
	return n
}

func parseNonNegativeInt(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return def
	}
	return n
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ensure errors is referenced (kept for future error wrapping).
var _ = errors.New
