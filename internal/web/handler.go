package web

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/api"
	"github.com/smackerel/smackerel/internal/graph"
	smacknats "github.com/smackerel/smackerel/internal/nats"
)

// Handler serves the web UI pages.
type Handler struct {
	Pool         *pgxpool.Pool
	NATS         *smacknats.Client
	Templates    *template.Template
	StartTime    time.Time
	SearchEngine *api.SearchEngine
}

// NewHandler creates a web UI handler with embedded templates.
func NewHandler(pool *pgxpool.Pool, nc *smacknats.Client, startTime time.Time) *Handler {
	tmpl := template.Must(template.New("").Funcs(template.FuncMap{
		"truncate": func(s string, n int) string {
			if len(s) <= n {
				return s
			}
			return s[:n] + "..."
		},
		"timeAgo": func(t time.Time) string {
			d := time.Since(t)
			switch {
			case d < time.Minute:
				return "just now"
			case d < time.Hour:
				return fmt.Sprintf("%dm ago", int(d.Minutes()))
			case d < 24*time.Hour:
				return fmt.Sprintf("%dh ago", int(d.Hours()))
			default:
				return fmt.Sprintf("%dd ago", int(d.Hours()/24))
			}
		},
		"safeURL": func(s string) string {
			if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
				return s
			}
			return ""
		},
	}).Parse(allTemplates))

	return &Handler{
		Pool:         pool,
		NATS:         nc,
		Templates:    tmpl,
		StartTime:    startTime,
		SearchEngine: &api.SearchEngine{Pool: pool, NATS: nc},
	}
}

// SearchPage handles GET / — the main search page.
func (h *Handler) SearchPage(w http.ResponseWriter, r *http.Request) {
	if err := h.Templates.ExecuteTemplate(w, "search.html", map[string]interface{}{
		"Title": "Smackerel",
	}); err != nil {
		slog.Error("template render failed", "template", "search.html", "error", err)
	}
}

// SearchResults handles POST /search — HTMX partial for search results.
func (h *Handler) SearchResults(w http.ResponseWriter, r *http.Request) {
	query := r.FormValue("query")
	if query == "" {
		if err := h.Templates.ExecuteTemplate(w, "results-partial.html", map[string]interface{}{
			"Results": nil,
			"Empty":   "Type a query to search your knowledge",
		}); err != nil {
			slog.Error("template error", "error", err)
			http.Error(w, "Internal error", 500)
		}
		return
	}

	// Use the semantic search engine (vector + text fallback) instead of raw ILIKE
	results, _, _, err := h.SearchEngine.Search(r.Context(), api.SearchRequest{
		Query: query,
		Limit: 20,
	})
	if err != nil {
		slog.Error("web search failed", "error", err)
		if err := h.Templates.ExecuteTemplate(w, "results-partial.html", map[string]interface{}{
			"Results": nil,
			"Error":   "Search failed. Try again.",
		}); err != nil {
			slog.Error("template error", "error", err)
			http.Error(w, "Internal error", 500)
		}
		return
	}

	type Result struct {
		ID        string
		Title     string
		Type      string
		Summary   string
		SourceURL string
		CreatedAt time.Time
	}

	var viewResults []Result
	for _, sr := range results {
		var createdAt time.Time
		if t, err := time.Parse(time.RFC3339, sr.CreatedAt); err == nil {
			createdAt = t
		}
		viewResults = append(viewResults, Result{
			ID:        sr.ArtifactID,
			Title:     sr.Title,
			Type:      sr.ArtifactType,
			Summary:   sr.Summary,
			SourceURL: sr.SourceURL,
			CreatedAt: createdAt,
		})
	}

	if len(viewResults) == 0 {
		if err := h.Templates.ExecuteTemplate(w, "results-partial.html", map[string]interface{}{
			"Results": nil,
			"Empty":   "No results found. Try a different query.",
		}); err != nil {
			slog.Error("template error", "error", err)
			http.Error(w, "Internal error", 500)
		}
		return
	}

	if err := h.Templates.ExecuteTemplate(w, "results-partial.html", map[string]interface{}{
		"Results": viewResults,
		"Query":   query,
	}); err != nil {
		slog.Error("template error", "error", err)
		http.Error(w, "Internal error", 500)
	}
}

// ArtifactDetail handles GET /artifact/{id}.
func (h *Handler) ArtifactDetail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	var title, artType, summary, sourceURL string
	var keyIdeas, entities, topics []byte
	var createdAt time.Time

	err := h.Pool.QueryRow(r.Context(), `
		SELECT title, artifact_type, COALESCE(summary, ''), COALESCE(source_url, ''),
		       COALESCE(key_ideas::text, '[]')::bytea, COALESCE(entities::text, '{}')::bytea,
		       COALESCE(topics::text, '[]')::bytea, created_at
		FROM artifacts WHERE id = $1
	`, id).Scan(&title, &artType, &summary, &sourceURL, &keyIdeas, &entities, &topics, &createdAt)
	if err != nil {
		http.Error(w, "Artifact not found", http.StatusNotFound)
		return
	}

	connections := graph.ConnectionCount(r.Context(), h.Pool, id)

	var keyIdeasParsed []string
	json.Unmarshal(keyIdeas, &keyIdeasParsed)

	var topicsParsed []string
	json.Unmarshal(topics, &topicsParsed)

	h.Templates.ExecuteTemplate(w, "detail.html", map[string]interface{}{
		"Title":       title,
		"Type":        artType,
		"Summary":     summary,
		"SourceURL":   sourceURL,
		"KeyIdeas":    keyIdeasParsed,
		"Topics":      topicsParsed,
		"Connections": connections,
		"CreatedAt":   createdAt,
		"ID":          id,
	})
}

// DigestPage handles GET /digest.
func (h *Handler) DigestPage(w http.ResponseWriter, r *http.Request) {
	var digestText, digestDate string
	var isQuiet bool

	err := h.Pool.QueryRow(r.Context(), `
		SELECT digest_text, digest_date, is_quiet FROM digests
		ORDER BY digest_date DESC LIMIT 1
	`).Scan(&digestText, &digestDate, &isQuiet)

	if err != nil {
		digestText = "No digest generated yet."
		digestDate = time.Now().Format("2006-01-02")
	}

	if err := h.Templates.ExecuteTemplate(w, "digest.html", map[string]interface{}{
		"Title":      "Daily Digest",
		"DigestText": digestText,
		"DigestDate": digestDate,
		"IsQuiet":    isQuiet,
	}); err != nil {
		slog.Error("template error", "error", err)
		http.Error(w, "Internal error", 500)
	}
}

// TopicsPage handles GET /topics.
func (h *Handler) TopicsPage(w http.ResponseWriter, r *http.Request) {
	// Parse pagination
	page := 1
	if p := r.URL.Query().Get("page"); p != "" {
		if n, err := fmt.Sscanf(p, "%d", &page); n != 1 || err != nil || page < 1 {
			page = 1
		}
	}
	const perPage = 20
	offset := (page - 1) * perPage

	rows, err := h.Pool.Query(r.Context(), `
		SELECT id, name, state, capture_count_total, last_active
		FROM topics ORDER BY momentum_score DESC, capture_count_total DESC
		LIMIT $1 OFFSET $2
	`, perPage+1, offset)
	if err != nil {
		slog.Error("topics query failed", "error", err)
		if err := h.Templates.ExecuteTemplate(w, "topics.html", map[string]interface{}{
			"Title": "Topics", "Topics": nil,
		}); err != nil {
			slog.Error("template error", "error", err)
			http.Error(w, "Internal error", 500)
		}
		return
	}
	defer rows.Close()

	type Topic struct {
		ID         string
		Name       string
		State      string
		Count      int
		LastActive *time.Time
	}

	var topics []Topic
	for rows.Next() {
		var t Topic
		if err := rows.Scan(&t.ID, &t.Name, &t.State, &t.Count, &t.LastActive); err != nil {
			continue
		}
		topics = append(topics, t)
	}

	if err := rows.Err(); err != nil {
		slog.Error("topics row iteration error", "error", err)
	}

	// Determine if there is a next page (we fetched perPage+1 rows)
	hasNext := len(topics) > perPage
	if hasNext {
		topics = topics[:perPage]
	}

	if err := h.Templates.ExecuteTemplate(w, "topics.html", map[string]interface{}{
		"Title":    "Topics",
		"Topics":   topics,
		"Page":     page,
		"PrevPage": page - 1,
		"NextPage": page + 1,
		"HasPrev":  page > 1,
		"HasNext":  hasNext,
	}); err != nil {
		slog.Error("template error", "error", err)
		http.Error(w, "Internal error", 500)
	}
}

// SettingsPage handles GET /settings.
func (h *Handler) SettingsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Query LLM provider from environment
	llmProvider := "unknown"
	if p := os.Getenv("LLM_PROVIDER"); p != "" {
		llmProvider = p
	}
	llmModel := "unknown"
	if m := os.Getenv("LLM_MODEL"); m != "" {
		llmModel = m
	}

	// Digest cron schedule
	digestCron := "not configured"
	if c := os.Getenv("DIGEST_CRON"); c != "" {
		digestCron = c
	}

	// Connector status from sync_state
	type ConnectorStatus struct {
		Name    string
		Enabled bool
		LastErr string
	}
	var connectors []ConnectorStatus
	if h.Pool != nil {
		rows, err := h.Pool.Query(ctx, `SELECT source_id, enabled, COALESCE(last_error, '') FROM sync_state ORDER BY source_id`)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var cs ConnectorStatus
				if err := rows.Scan(&cs.Name, &cs.Enabled, &cs.LastErr); err == nil {
					connectors = append(connectors, cs)
				}
			}
		}
	}

	// OAuth connection status
	type OAuthStatus struct {
		Provider  string
		Connected bool
	}
	var oauthStatuses []OAuthStatus
	if h.Pool != nil {
		oauthRows, err := h.Pool.Query(ctx, `SELECT provider, expires_at > NOW() AS connected FROM oauth_tokens ORDER BY provider`)
		if err == nil {
			defer oauthRows.Close()
			for oauthRows.Next() {
				var os OAuthStatus
				if err := oauthRows.Scan(&os.Provider, &os.Connected); err == nil {
					oauthStatuses = append(oauthStatuses, os)
				}
			}
		}
	}

	h.Templates.ExecuteTemplate(w, "settings.html", map[string]interface{}{
		"Title":       "Settings",
		"LLMProvider": llmProvider,
		"LLMModel":    llmModel,
		"DigestCron":  digestCron,
		"Connectors":  connectors,
		"OAuth":       oauthStatuses,
	})
}

// StatusPage handles GET /status.
func (h *Handler) StatusPage(w http.ResponseWriter, r *http.Request) {
	var artifactCount, topicCount, edgeCount int
	h.Pool.QueryRow(r.Context(), `
		SELECT
			(SELECT COUNT(*) FROM artifacts WHERE processing_status = 'processed') AS artifacts,
			(SELECT COUNT(*) FROM topics) AS topics,
			(SELECT COUNT(*) FROM edges) AS edges
	`).Scan(&artifactCount, &topicCount, &edgeCount)

	uptime := time.Since(h.StartTime)

	h.Templates.ExecuteTemplate(w, "status.html", map[string]interface{}{
		"Title":         "System Status",
		"ArtifactCount": artifactCount,
		"TopicCount":    topicCount,
		"EdgeCount":     edgeCount,
		"Uptime":        fmt.Sprintf("%dh %dm", int(uptime.Hours()), int(uptime.Minutes())%60),
		"DBHealthy":     h.Pool.Ping(r.Context()) == nil,
		"NATSHealthy":   h.NATS != nil && h.NATS.Healthy(),
	})
}
