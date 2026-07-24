package web

import (
	"bytes"
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
	"unicode"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/api"
	"github.com/smackerel/smackerel/internal/config"
	"github.com/smackerel/smackerel/internal/connector"
	"github.com/smackerel/smackerel/internal/connector/bookmarks"
	"github.com/smackerel/smackerel/internal/connector/qfdecisions"
	"github.com/smackerel/smackerel/internal/graph"
	"github.com/smackerel/smackerel/internal/knowledge"
	smacknats "github.com/smackerel/smackerel/internal/nats"
	"github.com/smackerel/smackerel/internal/notification"
	ntfysource "github.com/smackerel/smackerel/internal/notification/source/ntfy"
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

	// DigestReader is the canonical typed digest read seam (BUG-002-007). The
	// server-rendered /digest page consumes it instead of owning duplicate SQL;
	// only a wrapped pgx.ErrNoRows is empty and every other fault is an honest
	// read error. Production injects *digest.Generator (cmd/core wiring); a nil
	// reader renders an honest read_error, never a false-empty.
	DigestReader DigestReader
	// DigestStaleAfter is the operator-owned freshness threshold. Zero means the
	// DIGEST_STALE_AFTER_HOURS SST contract (BUG-002-007 Scope 01) is not yet
	// wired, which keeps stale determination inert (a non-quiet row renders
	// current, never arbitrarily stale) until the concurrent config work lands.
	DigestStaleAfter time.Duration
	// ClockOverride, when non-nil, replaces time.Now for deterministic stale-age
	// tests. Production leaves it nil. It is an observation seam only.
	ClockOverride func() time.Time

	// SearchExecutorOverride, when non-nil, replaces the real *api.SearchEngine
	// as the SearchResults domain-dispatch seam. Production leaves it nil;
	// focused unit tests inject a counting fake to prove zero SearchEngine
	// dispatch for blank/control/whitespace-only input (BUG-002-006 SEARCH-004).
	// It is an observation seam only — no runtime input selects it.
	SearchExecutorOverride SearchExecutor

	RecommendationsEnabled  bool
	RecommendationProviders RecommendationProviderLister
	RecommendationStore     *recstore.Store
	RecommendationRegistry  RecommendationRuntimeRegistry
	RecommendationConfig    config.RecommendationsConfig
	NotificationStore       *notification.Store
	NtfyStore               *ntfysource.Store
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
	// Spec 100 SCOPE-01 — parse the single-source cross-surface app-shell nav
	// partial into the knowledge-base set so every page that uses the shared
	// "head" cross-links the assistant, cards, knowledge, notifications, and
	// settings surfaces.
	template.Must(tmpl.Parse(appShellNav))

	return &Handler{
		Pool:         pool,
		NATS:         nc,
		Templates:    tmpl,
		StartTime:    startTime,
		SearchEngine: &api.SearchEngine{Pool: pool, NATS: nc},
	}
}

// executor returns the domain-dispatch seam used by SearchResults. Production
// uses the real *api.SearchEngine; focused tests inject a counting fake via
// SearchExecutorOverride to prove zero SearchEngine dispatch for blank input.
func (h *Handler) executor() SearchExecutor {
	if h.SearchExecutorOverride != nil {
		return h.SearchExecutorOverride
	}
	return h.SearchEngine
}

// trimSearchQuery edge-trims leading/trailing Unicode whitespace and control
// code points without deleting interior content (BUG-002-006 design
// "Pre-Dispatch Input Gate").
func trimSearchQuery(raw string) string {
	return strings.TrimFunc(raw, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r)
	})
}

// isBlankQuery reports whether the edge-trimmed query is empty. Empty,
// whitespace-only, control-only, and mixed whitespace/control input all trim to
// "" and MUST NOT dispatch any SearchEngine or downstream search-domain work
// (SEARCH-004).
func isBlankQuery(trimmed string) bool { return trimmed == "" }

// renderSearch writes one Search response from the typed model. A baseline
// (non-HTMX) request receives a complete page; an HX-Request receives the
// outcome fragment. Both use the same real HTTP status. Rendering happens into a
// buffer first so a template failure cannot emit a partially-written body under
// a wrong status.
func (h *Handler) renderSearch(w http.ResponseWriter, htmx bool, model SearchPageModel, status int) {
	tmplName := "search.html"
	if htmx {
		tmplName = "results-partial.html"
	}
	var buf bytes.Buffer
	if err := h.Templates.ExecuteTemplate(&buf, tmplName, model); err != nil {
		slog.Error("template render failed", "template", tmplName, "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

// SearchPage handles GET / — the complete server-rendered Search page in the
// initial ready state. It renders a semantic form that works with JavaScript
// disabled or HTMX blocked (progressive enhancement baseline).
func (h *Handler) SearchPage(w http.ResponseWriter, r *http.Request) {
	h.renderSearch(w, false, SearchPageModel{Title: "Smackerel", State: SearchStateReady}, http.StatusOK)
}

// SearchResults handles POST /search. It normalizes and classifies the query
// once before any domain dispatch: blank/control/whitespace-only input returns
// HTTP 422 validation with ZERO SearchEngine or downstream search-domain work
// (SEARCH-004); otherwise the injected SearchExecutor is called exactly once and
// one closed terminal state is projected. A baseline (non-HTMX) request receives
// a complete page; an HX-Request receives the outcome fragment with the same
// real HTTP status.
func (h *Handler) SearchResults(w http.ResponseWriter, r *http.Request) {
	htmx := r.Header.Get("HX-Request") == "true"
	trimmed := trimSearchQuery(r.FormValue("query"))

	// Pre-dispatch gate: blank input is validated without touching SearchEngine
	// or any downstream search-domain dependency.
	if isBlankQuery(trimmed) {
		h.renderSearch(w, htmx, SearchPageModel{
			Title:             "Smackerel",
			State:             SearchStateValidation,
			ValidationMessage: "Enter a search query",
		}, http.StatusUnprocessableEntity)
		return
	}

	// Exactly one domain dispatch for a searchable query.
	results, _, _, err := h.executor().Search(r.Context(), api.SearchRequest{
		Query: trimmed,
		Limit: 20,
	})
	if err != nil {
		slog.Error("web search failed", "error", err)
		h.renderSearch(w, htmx, SearchPageModel{
			Title:        "Smackerel",
			Query:        trimmed,
			State:        SearchStateServerError,
			ErrorMessage: "Search could not complete. Try again.",
		}, http.StatusInternalServerError)
		return
	}

	views := make([]SearchResultView, 0, len(results))
	for _, sr := range results {
		views = append(views, SearchResultView{
			ID:        sr.ArtifactID,
			Title:     sr.Title,
			Type:      sr.ArtifactType,
			Summary:   sr.Summary,
			SourceURL: sr.SourceURL,
			QFCard:    sr.QFCard,
		})
	}

	km := h.searchKnowledgeMatch(r.Context(), trimmed)

	if len(views) == 0 {
		h.renderSearch(w, htmx, SearchPageModel{
			Title:          "Smackerel",
			Query:          trimmed,
			State:          SearchStateEmpty,
			KnowledgeMatch: km,
		}, http.StatusOK)
		return
	}

	h.renderSearch(w, htmx, SearchPageModel{
		Title:          "Smackerel",
		Query:          trimmed,
		State:          SearchStateResults,
		Results:        views,
		ResultCount:    len(views),
		KnowledgeMatch: km,
	}, http.StatusOK)
}

// ArtifactDetail handles GET /artifact/{id}.
func (h *Handler) ArtifactDetail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	var title, artType, summary, sourceURL string
	var keyIdeas, entities, topics, metadataJSON []byte
	var createdAt time.Time

	err := h.Pool.QueryRow(r.Context(), `
		SELECT title, artifact_type, COALESCE(summary, ''), COALESCE(source_url, ''),
		       COALESCE(key_ideas::text, '[]')::bytea, COALESCE(entities::text, '{}')::bytea,
		       COALESCE(topics::text, '[]')::bytea, COALESCE(metadata::text, '{}')::bytea, created_at
		FROM artifacts WHERE id = $1
	`, id).Scan(&title, &artType, &summary, &sourceURL, &keyIdeas, &entities, &topics, &metadataJSON, &createdAt)
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

	var qfCard *qfdecisions.PacketCard
	if strings.HasPrefix(artType, "qf/") {
		metadata := map[string]any{}
		if err := json.Unmarshal(metadataJSON, &metadata); err != nil {
			slog.Debug("failed to unmarshal QF artifact metadata", "artifact_id", id, "error", err)
		} else {
			card, err := qfdecisions.RenderPacketCard(r.Context(), connector.RawArtifact{
				SourceRef:   id,
				ContentType: artType,
				Title:       title,
				RawContent:  summary,
				URL:         sourceURL,
				Metadata:    metadata,
			}, qfdecisions.RenderOptions{
				Surface:                       qfdecisions.SurfaceWeb,
				DeepLinkSigningSupported:      strings.TrimSpace(webStringFromAny(metadata["packet_url_signed"])) != "",
				PreferredSurfaceHintSupported: true,
			})
			if err == nil {
				qfCard = &card
				// Scope 6: capture an `opened` engagement signal on
				// the WEB surface immediately after the packet card
				// renders. The capture is one-way Smackerel→QF
				// observability — it MUST NOT influence subsequent
				// local rendering, ranking, or trust metadata.
				// SCN-SM-041-022.
				qfdecisions.CaptureEngagementOpened(r.Context(), qfdecisions.SurfaceWeb, card.PacketID, card.TraceID, "")
			}
		}
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
		"QFCard":      qfCard,
	})
}

// EvidenceBundleBuilderPage handles GET /evidence-bundles/new.
func (h *Handler) EvidenceBundleBuilderPage(w http.ResponseWriter, r *http.Request) {
	if err := h.Templates.ExecuteTemplate(w, "evidence-builder.html", map[string]interface{}{
		"Title":        "Personal Evidence Bundle",
		"QFArtifactID": r.URL.Query().Get("qf_artifact_id"),
		"PacketID":     r.URL.Query().Get("packet_id"),
	}); err != nil {
		slog.Error("template render failed", "template", "evidence-builder.html", "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
	}
}

func webStringFromAny(value any) string {
	stringValue, ok := value.(string)
	if !ok {
		return ""
	}
	return stringValue
}

// DigestPage handles GET /digest. It consumes the canonical typed digest reader
// and renders exactly one truthful state; only a wrapped pgx.ErrNoRows is empty
// (BUG-002-007). A read fault is an honest read_error at HTTP 500 with no
// digest-derived fields and no today's-date substitution.
func (h *Handler) DigestPage(w http.ResponseWriter, r *http.Request) {
	// Honor the reader's existing exact-date capability (mirrors GET /api/digest).
	// An invalid date falls back to the latest digest and never selects a test path.
	selectedDate := r.URL.Query().Get("date")
	if selectedDate != "" {
		if _, perr := time.Parse("2006-01-02", selectedDate); perr != nil {
			selectedDate = ""
		}
	}

	var model DigestPageModel
	if h.DigestReader == nil {
		model = classifyDigest(nil, errDigestReaderUnavailable, selectedDate, h.now(), h.DigestStaleAfter)
	} else {
		d, err := h.DigestReader.GetLatest(r.Context(), selectedDate)
		model = classifyDigest(d, err, selectedDate, h.now(), h.DigestStaleAfter)
	}

	status := http.StatusOK
	if model.State == DigestReadError {
		status = http.StatusInternalServerError
		slog.Error("digest read failed", "kind", model.ErrorKind, "date_selected", selectedDate != "")
	}

	var buf bytes.Buffer
	if terr := h.Templates.ExecuteTemplate(&buf, "digest.html", model); terr != nil {
		slog.Error("template error", "error", terr)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
}

// now returns the injected clock or time.Now (BUG-002-007 stale-age seam).
func (h *Handler) now() time.Time {
	if h.ClockOverride != nil {
		return h.ClockOverride()
	}
	return time.Now()
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

func (h *Handler) NotificationDashboard(w http.ResponseWriter, r *http.Request) {
	store, ok := h.requireNotificationStore(w)
	if !ok {
		return
	}
	summary, err := store.StatusSummary(r.Context())
	if err != nil {
		h.renderNotificationError(w, "Notification Status", "notification status is unavailable")
		return
	}
	h.Templates.ExecuteTemplate(w, "notifications-status.html", map[string]interface{}{"Title": "Notifications", "Summary": summary})
}

func (h *Handler) NotificationSourcesPage(w http.ResponseWriter, r *http.Request) {
	store, ok := h.requireNotificationStore(w)
	if !ok {
		return
	}
	sources, err := store.ListSourceStatuses(r.Context())
	if err != nil {
		h.renderNotificationError(w, "Notification Sources", "notification sources are unavailable")
		return
	}
	h.Templates.ExecuteTemplate(w, "notifications-sources.html", map[string]interface{}{"Title": "Notification Sources", "Sources": sources})
}

func (h *Handler) NotificationNtfySourcePage(w http.ResponseWriter, r *http.Request) {
	store, ok := h.requireNotificationStore(w)
	if !ok {
		return
	}
	instanceID := chi.URLParam(r, "source_instance_id")
	statuses, err := store.ListSourceStatuses(r.Context())
	if err != nil {
		h.renderNotificationError(w, "ntfy Source", "notification sources are unavailable")
		return
	}
	var source *notification.SourceStatus
	for _, status := range statuses {
		if status.Config.SourceInstanceID == instanceID && status.Config.SourceType == ntfysource.SourceType {
			copy := status
			source = &copy
			break
		}
	}
	if source == nil {
		h.renderNotificationError(w, "ntfy Source", "ntfy source is unavailable")
		return
	}
	topics := []ntfysource.SubscriptionState{}
	deadLetters := []ntfysource.DeadLetterRecord{}
	if h.NtfyStore != nil {
		if loadedTopics, err := h.NtfyStore.ListSubscriptionStates(r.Context(), instanceID); err == nil {
			topics = loadedTopics
		}
		if page, err := h.NtfyStore.ListDeadLetters(r.Context(), instanceID, 5, ""); err == nil {
			deadLetters = page.Records
		}
	}
	events, _ := store.ListNotifications(r.Context(), 25)
	var lastEvent *notification.NormalizedNotification
	for _, event := range events {
		if event.SourceInstanceID == instanceID {
			copy := event
			lastEvent = &copy
			break
		}
	}
	h.Templates.ExecuteTemplate(w, "notifications-ntfy-source.html", map[string]interface{}{"Title": "ntfy Source", "Source": source, "Topics": topics, "DeadLetters": deadLetters, "LastEvent": lastEvent})
}

func (h *Handler) NotificationNtfyDeadLettersPage(w http.ResponseWriter, r *http.Request) {
	store, ok := h.requireNotificationStore(w)
	if !ok {
		return
	}
	instanceID := chi.URLParam(r, "source_instance_id")
	statuses, err := store.ListSourceStatuses(r.Context())
	if err != nil {
		h.renderNotificationError(w, "ntfy Dead Letters", "notification sources are unavailable")
		return
	}
	found := false
	for _, status := range statuses {
		if status.Config.SourceInstanceID == instanceID && status.Config.SourceType == ntfysource.SourceType {
			found = true
			break
		}
	}
	if !found || h.NtfyStore == nil {
		h.renderNotificationError(w, "ntfy Dead Letters", "ntfy dead-letter records are unavailable")
		return
	}
	page, err := h.NtfyStore.ListDeadLetters(r.Context(), instanceID, 50, "")
	if err != nil {
		h.renderNotificationError(w, "ntfy Dead Letters", "ntfy dead-letter records are unavailable")
		return
	}
	h.Templates.ExecuteTemplate(w, "notifications-ntfy-dead-letters.html", map[string]interface{}{"Title": "ntfy Dead Letters", "SourceInstanceID": instanceID, "DeadLetters": page.Records, "NextCursor": page.NextCursor})
}

func (h *Handler) NotificationNtfyDeadLetterDetailPage(w http.ResponseWriter, r *http.Request) {
	store, ok := h.requireNotificationStore(w)
	if !ok {
		return
	}
	instanceID := chi.URLParam(r, "source_instance_id")
	deadLetterID := chi.URLParam(r, "dead_letter_id")
	statuses, err := store.ListSourceStatuses(r.Context())
	if err != nil {
		h.renderNotificationError(w, "ntfy Dead Letter", "notification sources are unavailable")
		return
	}
	found := false
	for _, status := range statuses {
		if status.Config.SourceInstanceID == instanceID && status.Config.SourceType == ntfysource.SourceType {
			found = true
			break
		}
	}
	if !found || h.NtfyStore == nil {
		h.renderNotificationError(w, "ntfy Dead Letter", "ntfy dead-letter record is unavailable")
		return
	}
	record, err := h.NtfyStore.GetDeadLetter(r.Context(), instanceID, deadLetterID)
	if err != nil {
		h.renderNotificationError(w, "ntfy Dead Letter", "ntfy dead-letter record is unavailable")
		return
	}
	h.Templates.ExecuteTemplate(w, "notifications-ntfy-dead-letter-detail.html", map[string]interface{}{"Title": "ntfy Dead Letter", "SourceInstanceID": instanceID, "Record": record})
}

func (h *Handler) NotificationEventsPage(w http.ResponseWriter, r *http.Request) {
	store, ok := h.requireNotificationStore(w)
	if !ok {
		return
	}
	events, err := store.ListNotifications(r.Context(), 100)
	if err != nil {
		h.renderNotificationError(w, "Notification Events", "notification events are unavailable")
		return
	}
	h.Templates.ExecuteTemplate(w, "notifications-events.html", map[string]interface{}{"Title": "Notification Events", "Events": events})
}

func (h *Handler) NotificationIncidentsPage(w http.ResponseWriter, r *http.Request) {
	store, ok := h.requireNotificationStore(w)
	if !ok {
		return
	}
	incidents, err := store.ListIncidents(r.Context(), 100)
	if err != nil {
		h.renderNotificationError(w, "Notification Incidents", "notification incidents are unavailable")
		return
	}
	h.Templates.ExecuteTemplate(w, "notifications-incidents.html", map[string]interface{}{"Title": "Notification Incidents", "Incidents": incidents})
}

func (h *Handler) NotificationIncidentDetailPage(w http.ResponseWriter, r *http.Request) {
	store, ok := h.requireNotificationStore(w)
	if !ok {
		return
	}
	incident, err := store.GetIncident(r.Context(), chi.URLParam(r, "incident_id"))
	if err != nil {
		h.renderNotificationError(w, "Notification Incident", "notification incident is unavailable")
		return
	}
	h.Templates.ExecuteTemplate(w, "notifications-incident-detail.html", map[string]interface{}{"Title": "Notification Incident", "Incident": incident})
}

func (h *Handler) NotificationApprovalsPage(w http.ResponseWriter, r *http.Request) {
	store, ok := h.requireNotificationStore(w)
	if !ok {
		return
	}
	approvals, err := store.ListApprovalRequests(r.Context(), 100)
	if err != nil {
		h.renderNotificationError(w, "Notification Approvals", "notification approvals are unavailable")
		return
	}
	h.Templates.ExecuteTemplate(w, "notifications-approvals.html", map[string]interface{}{"Title": "Notification Approvals", "Approvals": approvals})
}

func (h *Handler) NotificationApprovalDetailPage(w http.ResponseWriter, r *http.Request) {
	store, ok := h.requireNotificationStore(w)
	if !ok {
		return
	}
	detail, err := store.GetApprovalDetail(r.Context(), chi.URLParam(r, "approval_id"))
	if err != nil {
		h.renderNotificationError(w, "Notification Approval", "notification approval is unavailable")
		return
	}
	h.Templates.ExecuteTemplate(w, "notifications-approval-detail.html", map[string]interface{}{"Title": "Notification Approval", "Approval": detail.Request, "Decisions": detail.Decisions})
}

func (h *Handler) NotificationSuppressionsPage(w http.ResponseWriter, r *http.Request) {
	store, ok := h.requireNotificationStore(w)
	if !ok {
		return
	}
	suppressions, err := store.ListSuppressions(r.Context(), 100)
	if err != nil {
		h.renderNotificationError(w, "Notification Suppressions", "notification suppressions are unavailable")
		return
	}
	quietWindows, err := store.ListQuietWindows(r.Context(), 100)
	if err != nil {
		h.renderNotificationError(w, "Notification Suppressions", "notification quiet windows are unavailable")
		return
	}
	h.Templates.ExecuteTemplate(w, "notifications-suppressions.html", map[string]interface{}{"Title": "Notification Suppressions", "Suppressions": suppressions, "QuietWindows": quietWindows})
}

func (h *Handler) NotificationSummaryPage(w http.ResponseWriter, r *http.Request) {
	store, ok := h.requireNotificationStore(w)
	if !ok {
		return
	}
	summary, err := store.StatusSummary(r.Context())
	if err != nil {
		h.renderNotificationError(w, "Notification Summary", "notification summary is unavailable")
		return
	}
	h.Templates.ExecuteTemplate(w, "notifications-summary.html", map[string]interface{}{"Title": "Notification Summary", "Summary": summary})
}

func (h *Handler) NotificationOutputsPage(w http.ResponseWriter, r *http.Request) {
	store, ok := h.requireNotificationStore(w)
	if !ok {
		return
	}
	outputs, err := store.ListDeliveries(r.Context(), 100)
	if err != nil {
		h.renderNotificationError(w, "Notification Outputs", "notification outputs are unavailable")
		return
	}
	h.Templates.ExecuteTemplate(w, "notifications-outputs.html", map[string]interface{}{"Title": "Notification Outputs", "Outputs": outputs})
}

func (h *Handler) requireNotificationStore(w http.ResponseWriter) (*notification.Store, bool) {
	if h.NotificationStore != nil {
		return h.NotificationStore, true
	}
	if h.Pool == nil {
		h.renderNotificationError(w, "Notifications", "notification store is unavailable")
		return nil, false
	}
	return notification.NewStore(h.Pool), true
}

func (h *Handler) renderNotificationError(w http.ResponseWriter, title string, message string) {
	w.WriteHeader(http.StatusInternalServerError)
	h.Templates.ExecuteTemplate(w, "notification-error.html", map[string]interface{}{"Title": title, "Error": message})
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
		h.Supervisor.TriggerSync(context.WithoutCancel(r.Context()), id)
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
func (h *Handler) searchKnowledgeMatch(ctx context.Context, query string) *KnowledgeMatchView {
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
	return &KnowledgeMatchView{
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
