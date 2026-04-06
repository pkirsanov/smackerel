package web

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	smacknats "github.com/smackerel/smackerel/internal/nats"
)

// Handler serves the web UI pages.
type Handler struct {
	Pool      *pgxpool.Pool
	NATS      *smacknats.Client
	Templates *template.Template
	StartTime time.Time
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
		"json": func(v interface{}) string {
			b, _ := json.Marshal(v)
			return string(b)
		},
	}).Parse(allTemplates))

	return &Handler{
		Pool:      pool,
		NATS:      nc,
		Templates: tmpl,
		StartTime: startTime,
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
		h.Templates.ExecuteTemplate(w, "results-partial.html", map[string]interface{}{
			"Results": nil,
			"Empty":   "Type a query to search your knowledge",
		})
		return
	}

	// Query artifacts with text search fallback
	rows, err := h.Pool.Query(r.Context(), `
		SELECT id, title, artifact_type, COALESCE(summary, ''), COALESCE(source_url, ''),
		       created_at
		FROM artifacts
		WHERE title ILIKE '%' || $1 || '%'
		   OR summary ILIKE '%' || $1 || '%'
		   OR content_raw ILIKE '%' || $1 || '%'
		ORDER BY created_at DESC LIMIT 20
	`, query)
	if err != nil {
		slog.Error("web search query failed", "error", err)
		h.Templates.ExecuteTemplate(w, "results-partial.html", map[string]interface{}{
			"Results": nil,
			"Error":   "Search failed. Try again.",
		})
		return
	}
	defer rows.Close()

	type Result struct {
		ID        string
		Title     string
		Type      string
		Summary   string
		SourceURL string
		CreatedAt time.Time
	}

	var results []Result
	for rows.Next() {
		var r Result
		if err := rows.Scan(&r.ID, &r.Title, &r.Type, &r.Summary, &r.SourceURL, &r.CreatedAt); err != nil {
			continue
		}
		results = append(results, r)
	}

	if len(results) == 0 {
		h.Templates.ExecuteTemplate(w, "results-partial.html", map[string]interface{}{
			"Results": nil,
			"Empty":   "No results found. Try a different query.",
		})
		return
	}

	h.Templates.ExecuteTemplate(w, "results-partial.html", map[string]interface{}{
		"Results": results,
		"Query":   query,
	})
}

// ArtifactDetail handles GET /artifact/{id}.
func (h *Handler) ArtifactDetail(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	var title, artType, summary, sourceURL string
	var keyIdeas, entities, topics []byte
	var createdAt time.Time
	var connections int

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

	h.Pool.QueryRow(r.Context(), `
		SELECT COUNT(*) FROM edges
		WHERE (src_type = 'artifact' AND src_id = $1) OR (dst_type = 'artifact' AND dst_id = $1)
	`, id).Scan(&connections)

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

	h.Templates.ExecuteTemplate(w, "digest.html", map[string]interface{}{
		"Title":      "Daily Digest",
		"DigestText": digestText,
		"DigestDate": digestDate,
		"IsQuiet":    isQuiet,
	})
}

// TopicsPage handles GET /topics.
func (h *Handler) TopicsPage(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Pool.Query(r.Context(), `
		SELECT id, name, state, capture_count_total, last_active
		FROM topics ORDER BY momentum_score DESC, capture_count_total DESC
	`)
	if err != nil {
		slog.Error("topics query failed", "error", err)
		h.Templates.ExecuteTemplate(w, "topics.html", map[string]interface{}{
			"Title": "Topics", "Topics": nil,
		})
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

	h.Templates.ExecuteTemplate(w, "topics.html", map[string]interface{}{
		"Title":  "Topics",
		"Topics": topics,
	})
}

// SettingsPage handles GET /settings.
func (h *Handler) SettingsPage(w http.ResponseWriter, r *http.Request) {
	h.Templates.ExecuteTemplate(w, "settings.html", map[string]interface{}{
		"Title": "Settings",
	})
}

// StatusPage handles GET /status.
func (h *Handler) StatusPage(w http.ResponseWriter, r *http.Request) {
	var artifactCount, topicCount, edgeCount int
	h.Pool.QueryRow(r.Context(), "SELECT COUNT(*) FROM artifacts").Scan(&artifactCount)
	h.Pool.QueryRow(r.Context(), "SELECT COUNT(*) FROM topics").Scan(&topicCount)
	h.Pool.QueryRow(r.Context(), "SELECT COUNT(*) FROM edges").Scan(&edgeCount)

	uptime := time.Since(h.StartTime)

	h.Templates.ExecuteTemplate(w, "status.html", map[string]interface{}{
		"Title":         "System Status",
		"ArtifactCount": artifactCount,
		"TopicCount":    topicCount,
		"EdgeCount":     edgeCount,
		"Uptime":        fmt.Sprintf("%dh %dm", int(uptime.Hours()), int(uptime.Minutes())%60),
		"DBHealthy":     true,
		"NATSHealthy":   h.NATS != nil && h.NATS.Healthy(),
	})
}
