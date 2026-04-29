package web

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/api"
	"github.com/smackerel/smackerel/internal/config"
	"github.com/smackerel/smackerel/internal/connector/bookmarks"
	"github.com/smackerel/smackerel/internal/graph"
	"github.com/smackerel/smackerel/internal/knowledge"
	smacknats "github.com/smackerel/smackerel/internal/nats"
	recprovider "github.com/smackerel/smackerel/internal/recommendation/provider"
	recstore "github.com/smackerel/smackerel/internal/recommendation/store"
)

// SyncTrigger triggers an immediate sync for a connector.
type SyncTrigger interface {
	TriggerSync(ctx context.Context, id string)
}

// Handler serves the web UI pages.
type Handler struct {
	Pool           *pgxpool.Pool
	NATS           *smacknats.Client
	Templates      *template.Template
	StartTime      time.Time
	SearchEngine   *api.SearchEngine
	Supervisor     SyncTrigger
	KnowledgeStore *knowledge.KnowledgeStore

	RecommendationsEnabled  bool
	RecommendationProviders RecommendationProviderLister
	RecommendationStore     *recstore.Store
	RecommendationRegistry  RecommendationRuntimeRegistry
	RecommendationConfig    config.RecommendationsConfig
}

// RecommendationProviderLister lists configured recommendation providers for operator status.
type RecommendationProviderLister interface {
	List() []recprovider.Provider
}

// RecommendationRuntimeRegistry is the provider registry used by the web
// request form to submit API-backed recommendation requests.
type RecommendationRuntimeRegistry interface {
	Len() int
	List() []recprovider.Provider
}

type recommendationProviderStatus struct {
	ProviderID    string
	DisplayName   string
	Status        string
	Reason        string
	CategoryLabel string
	Healthy       bool
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
		"timeAgo": func(t interface{}) string {
			var ts time.Time
			switch v := t.(type) {
			case time.Time:
				ts = v
			case *time.Time:
				if v == nil {
					return "never"
				}
				ts = *v
			default:
				return "unknown"
			}
			d := time.Since(ts)
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
		templateData := map[string]interface{}{
			"Results": nil,
			"Empty":   "No results found. Try a different query.",
		}
		if knowledgeMatch := h.searchKnowledgeMatch(r.Context(), query); knowledgeMatch != nil {
			templateData["KnowledgeMatch"] = knowledgeMatch
			delete(templateData, "Empty")
		}
		if err := h.Templates.ExecuteTemplate(w, "results-partial.html", templateData); err != nil {
			slog.Error("template error", "error", err)
			http.Error(w, "Internal error", 500)
		}
		return
	}

	templateData := map[string]interface{}{
		"Results": viewResults,
		"Query":   query,
	}
	if knowledgeMatch := h.searchKnowledgeMatch(r.Context(), query); knowledgeMatch != nil {
		templateData["KnowledgeMatch"] = knowledgeMatch
	}
	if err := h.Templates.ExecuteTemplate(w, "results-partial.html", templateData); err != nil {
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
	if err := json.Unmarshal(keyIdeas, &keyIdeasParsed); err != nil {
		slog.Debug("failed to unmarshal artifact key_ideas", "artifact_id", id, "error", err)
	}

	var topicsParsed []string
	if err := json.Unmarshal(topics, &topicsParsed); err != nil {
		slog.Debug("failed to unmarshal artifact topics", "artifact_id", id, "error", err)
	}

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
		Name        string
		Enabled     bool
		LastErr     string
		LastSync    string
		ItemsSynced int
	}
	var connectors []ConnectorStatus
	if h.Pool != nil {
		rows, err := h.Pool.Query(ctx, `SELECT source_id, enabled, COALESCE(last_error, ''), COALESCE(last_sync::text, ''), items_synced FROM sync_state ORDER BY source_id`)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var cs ConnectorStatus
				if err := rows.Scan(&cs.Name, &cs.Enabled, &cs.LastErr, &cs.LastSync, &cs.ItemsSynced); err == nil {
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
	var recommendationProviderStatuses []recommendationProviderStatus
	if h.RecommendationsEnabled {
		recommendationProviderStatuses = h.recommendationProviderStatuses(r.Context())
	}

	data := map[string]interface{}{
		"Title":                          "System Status",
		"ArtifactCount":                  artifactCount,
		"TopicCount":                     topicCount,
		"EdgeCount":                      edgeCount,
		"Uptime":                         fmt.Sprintf("%dh %dm", int(uptime.Hours()), int(uptime.Minutes())%60),
		"DBHealthy":                      h.Pool.Ping(r.Context()) == nil,
		"NATSHealthy":                    h.NATS != nil && h.NATS.Healthy(),
		"RecommendationsEnabled":         h.RecommendationsEnabled,
		"RecommendationProviderStatuses": recommendationProviderStatuses,
	}

	if h.KnowledgeStore != nil {
		stats, err := h.KnowledgeStore.GetStats(r.Context())
		if err != nil {
			slog.Warn("knowledge stats fetch failed", "error", err)
		} else {
			data["KnowledgeStats"] = stats
		}
	}

	h.Templates.ExecuteTemplate(w, "status.html", data)
}

func (h *Handler) recommendationProviderStatuses(ctx context.Context) []recommendationProviderStatus {
	if h.RecommendationProviders == nil {
		return nil
	}

	providerEntries := h.RecommendationProviders.List()
	statuses := make([]recommendationProviderStatus, 0, len(providerEntries))
	for _, providerEntry := range providerEntries {
		health := providerEntry.Health(ctx)
		providerID := health.ProviderID
		if providerID == "" {
			providerID = providerEntry.ID()
		}
		displayName := health.DisplayName
		if displayName == "" {
			displayName = providerEntry.DisplayName()
		}
		categories := health.CategoryList
		if len(categories) == 0 {
			categories = providerEntry.Categories()
		}
		categoryLabels := make([]string, 0, len(categories))
		for _, category := range categories {
			categoryLabels = append(categoryLabels, string(category))
		}

		statuses = append(statuses, recommendationProviderStatus{
			ProviderID:    providerID,
			DisplayName:   displayName,
			Status:        string(health.Status),
			Reason:        health.Reason,
			CategoryLabel: strings.Join(categoryLabels, ", "),
			Healthy:       health.Status == recprovider.StatusHealthy,
		})
	}

	return statuses
}

// SyncConnectorHandler handles POST /settings/connectors/{id}/sync — triggers immediate sync.
func (h *Handler) SyncConnectorHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}

	if h.Supervisor != nil {
		h.Supervisor.TriggerSync(r.Context(), id)
		slog.Info("manual sync triggered", "connector", id)
	} else {
		slog.Warn("sync trigger unavailable — no supervisor configured", "connector", id)
	}

	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

// BookmarkUploadHandler handles POST /settings/bookmarks/import — web UI bookmark file upload.
func (h *Handler) BookmarkUploadHandler(w http.ResponseWriter, r *http.Request) {
	const maxUploadSize = 10 << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)

	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		http.Error(w, "File too large or invalid form", http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Missing bookmark file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "Failed to read file", http.StatusBadRequest)
		return
	}

	if len(data) == 0 {
		http.Error(w, "File is empty", http.StatusBadRequest)
		return
	}

	// Try Chrome JSON first, then Netscape HTML.
	parsed, err := bookmarks.ParseChromeJSON(data)
	if err != nil || len(parsed) == 0 {
		parsed, _ = bookmarks.ParseNetscapeHTML(data)
	}

	count := len(parsed)
	if count == 0 {
		http.Error(w, "No bookmarks found — unsupported format", http.StatusBadRequest)
		return
	}

	slog.Info("web bookmark upload parsed", "count", count)

	// Render result page.
	h.Templates.ExecuteTemplate(w, "bookmark-import-result.html", map[string]interface{}{
		"Title":    "Bookmark Import",
		"Imported": count,
	})
}

// searchKnowledgeMatch searches the knowledge layer for a concept match.
// Returns nil if KnowledgeStore is not configured or no match is found.
func (h *Handler) searchKnowledgeMatch(ctx context.Context, query string) *struct {
	ConceptID     string
	Title         string
	Summary       string
	CitationCount int
	UpdatedAt     time.Time
} {
	if h.KnowledgeStore == nil {
		return nil
	}
	match, err := h.KnowledgeStore.SearchConcepts(ctx, query, 0.3)
	if err != nil {
		slog.Warn("web knowledge search failed", "error", err)
		return nil
	}
	if match == nil {
		return nil
	}
	return &struct {
		ConceptID     string
		Title         string
		Summary       string
		CitationCount int
		UpdatedAt     time.Time
	}{
		ConceptID:     match.ConceptID,
		Title:         match.Title,
		Summary:       match.Summary,
		CitationCount: match.CitationCount,
		UpdatedAt:     match.UpdatedAt,
	}
}

// KnowledgeDashboard handles GET /knowledge — knowledge layer dashboard.
func (h *Handler) KnowledgeDashboard(w http.ResponseWriter, r *http.Request) {
	if h.KnowledgeStore == nil {
		h.Templates.ExecuteTemplate(w, "knowledge-dashboard.html", map[string]interface{}{
			"Title": "Knowledge Layer",
			"Empty": "Knowledge layer is not enabled.",
		})
		return
	}

	stats, err := h.KnowledgeStore.GetStats(r.Context())
	if err != nil {
		slog.Error("knowledge stats failed", "error", err)
		h.Templates.ExecuteTemplate(w, "knowledge-dashboard.html", map[string]interface{}{
			"Title": "Knowledge Layer",
			"Empty": "Unable to load knowledge dashboard. Check system status.",
		})
		return
	}

	if stats.ConceptCount == 0 && stats.EntityCount == 0 {
		h.Templates.ExecuteTemplate(w, "knowledge-dashboard.html", map[string]interface{}{
			"Title": "Knowledge Layer",
			"Empty": "No knowledge synthesized yet. Connect sources and ingest content to start building your knowledge layer.",
		})
		return
	}

	// Fetch recent concepts for activity section
	recent, _, _ := h.KnowledgeStore.ListConceptsFiltered(r.Context(), "", "updated", 5, 0)

	h.Templates.ExecuteTemplate(w, "knowledge-dashboard.html", map[string]interface{}{
		"Title":          "Knowledge Layer",
		"Stats":          stats,
		"RecentConcepts": recent,
	})
}

// ConceptsList handles GET /knowledge/concepts — searchable concept list.
func (h *Handler) ConceptsList(w http.ResponseWriter, r *http.Request) {
	if h.KnowledgeStore == nil {
		h.Templates.ExecuteTemplate(w, "concepts-list.html", map[string]interface{}{
			"Title": "Concept Pages", "Total": 0, "Sort": "updated",
		})
		return
	}

	q := r.URL.Query().Get("q")
	if len(q) > 1000 {
		q = q[:1000]
	}
	sort := r.URL.Query().Get("sort")
	if sort == "" {
		sort = "updated"
	}

	concepts, total, err := h.KnowledgeStore.ListConceptsFiltered(r.Context(), q, sort, 20, 0)
	if err != nil {
		slog.Error("list concepts failed", "error", err)
	}

	h.Templates.ExecuteTemplate(w, "concepts-list.html", map[string]interface{}{
		"Title":    "Concept Pages",
		"Concepts": concepts,
		"Total":    total,
		"Sort":     sort,
		"Query":    q,
	})
}

// ConceptDetail handles GET /knowledge/concepts/{id} — concept page detail.
func (h *Handler) ConceptDetail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		http.Redirect(w, r, "/knowledge/concepts", http.StatusSeeOther)
		return
	}

	if h.KnowledgeStore == nil {
		http.Error(w, "Knowledge layer not available", http.StatusServiceUnavailable)
		return
	}

	concept, err := h.KnowledgeStore.GetConceptByID(r.Context(), id)
	if err != nil || concept == nil {
		http.Error(w, "Concept not found", http.StatusNotFound)
		return
	}

	// Parse claims from JSON
	var claims []knowledge.Claim
	if concept.Claims != nil {
		json.Unmarshal(concept.Claims, &claims)
	}

	// Fetch related concepts
	var relatedConcepts []*knowledge.ConceptPage
	for _, relID := range concept.RelatedConceptIDs {
		rel, err := h.KnowledgeStore.GetConceptByID(r.Context(), relID)
		if err == nil && rel != nil {
			relatedConcepts = append(relatedConcepts, rel)
		}
	}

	// Fetch connected entities — entities that reference this concept
	var entities []*knowledge.EntityProfile
	allEntities, _, _ := h.KnowledgeStore.ListEntitiesFiltered(r.Context(), "", "updated", 100, 0)
	for _, e := range allEntities {
		for _, cid := range e.RelatedConceptIDs {
			if cid == id {
				entities = append(entities, e)
				break
			}
		}
	}

	h.Templates.ExecuteTemplate(w, "concept-detail.html", map[string]interface{}{
		"Title":           concept.Title,
		"Concept":         concept,
		"Claims":          claims,
		"RelatedConcepts": relatedConcepts,
		"Entities":        entities,
	})
}

// EntitiesList handles GET /knowledge/entities — searchable entity list.
func (h *Handler) EntitiesList(w http.ResponseWriter, r *http.Request) {
	if h.KnowledgeStore == nil {
		h.Templates.ExecuteTemplate(w, "entities-list.html", map[string]interface{}{
			"Title": "Entity Profiles", "Total": 0, "Sort": "updated",
		})
		return
	}

	q := r.URL.Query().Get("q")
	if len(q) > 1000 {
		q = q[:1000]
	}
	sort := r.URL.Query().Get("sort")
	if sort == "" {
		sort = "updated"
	}

	entities, total, err := h.KnowledgeStore.ListEntitiesFiltered(r.Context(), q, sort, 20, 0)
	if err != nil {
		slog.Error("list entities failed", "error", err)
	}

	h.Templates.ExecuteTemplate(w, "entities-list.html", map[string]interface{}{
		"Title":    "Entity Profiles",
		"Entities": entities,
		"Total":    total,
		"Sort":     sort,
		"Query":    q,
	})
}

// EntityDetail handles GET /knowledge/entities/{id} — entity profile detail.
func (h *Handler) EntityDetail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		http.Redirect(w, r, "/knowledge/entities", http.StatusSeeOther)
		return
	}

	if h.KnowledgeStore == nil {
		http.Error(w, "Knowledge layer not available", http.StatusServiceUnavailable)
		return
	}

	entity, err := h.KnowledgeStore.GetEntityByID(r.Context(), id)
	if err != nil || entity == nil {
		http.Error(w, "Entity not found", http.StatusNotFound)
		return
	}

	// Parse mentions from JSON
	var mentions []knowledge.Mention
	if entity.Mentions != nil {
		json.Unmarshal(entity.Mentions, &mentions)
	}

	// Fetch related concepts
	var relatedConcepts []*knowledge.ConceptPage
	for _, cid := range entity.RelatedConceptIDs {
		c, err := h.KnowledgeStore.GetConceptByID(r.Context(), cid)
		if err == nil && c != nil {
			relatedConcepts = append(relatedConcepts, c)
		}
	}

	h.Templates.ExecuteTemplate(w, "entity-detail.html", map[string]interface{}{
		"Title":           entity.Name,
		"Entity":          entity,
		"Mentions":        mentions,
		"RelatedConcepts": relatedConcepts,
	})
}

// LintReport handles GET /knowledge/lint — lint findings report.
func (h *Handler) LintReport(w http.ResponseWriter, r *http.Request) {
	if h.KnowledgeStore == nil {
		h.Templates.ExecuteTemplate(w, "lint-report.html", map[string]interface{}{
			"Title": "Knowledge Lint Report",
		})
		return
	}

	report, err := h.KnowledgeStore.GetLatestLintReport(r.Context())
	if err != nil {
		slog.Warn("lint report fetch failed", "error", err)
		h.Templates.ExecuteTemplate(w, "lint-report.html", map[string]interface{}{
			"Title": "Knowledge Lint Report",
		})
		return
	}

	var findings []knowledge.LintFinding
	if report.Findings != nil {
		json.Unmarshal(report.Findings, &findings)
	}
	var summary knowledge.LintSummary
	if report.Summary != nil {
		json.Unmarshal(report.Summary, &summary)
	}

	h.Templates.ExecuteTemplate(w, "lint-report.html", map[string]interface{}{
		"Title":    "Knowledge Lint Report",
		"Report":   report,
		"Findings": findings,
		"Summary":  summary,
	})
}

// LintFindingDetail handles GET /knowledge/lint/{id} — individual lint finding detail.
func (h *Handler) LintFindingDetail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		http.Redirect(w, r, "/knowledge/lint", http.StatusSeeOther)
		return
	}

	if h.KnowledgeStore == nil {
		http.Error(w, "Knowledge layer not available", http.StatusServiceUnavailable)
		return
	}

	// Get the latest lint report and find the specific finding
	report, err := h.KnowledgeStore.GetLatestLintReport(r.Context())
	if err != nil {
		http.Error(w, "Lint report not available", http.StatusNotFound)
		return
	}

	var findings []knowledge.LintFinding
	if report.Findings != nil {
		json.Unmarshal(report.Findings, &findings)
	}

	var found *knowledge.LintFinding
	for i := range findings {
		if findings[i].TargetID == id {
			found = &findings[i]
			break
		}
	}

	if found == nil {
		http.Error(w, "Finding not found", http.StatusNotFound)
		return
	}

	// Try to load the associated concept
	var concept *knowledge.ConceptPage
	if found.TargetType == "concept" {
		concept, _ = h.KnowledgeStore.GetConceptByID(r.Context(), found.TargetID)
	}

	h.Templates.ExecuteTemplate(w, "lint-finding-detail.html", map[string]interface{}{
		"Title":   found.TargetTitle,
		"Finding": found,
		"Concept": concept,
	})
}
