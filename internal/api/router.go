package api

import (
	"crypto/subtle"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/go-chi/httprate"

	"github.com/smackerel/smackerel/internal/metrics"
)

// NewRouter creates the Chi router with all routes and middleware.
func NewRouter(deps *Dependencies) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(structuredLogger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Heartbeat("/ping"))
	r.Use(securityHeadersMiddleware)

	// CORS — configured via SST (CORSAllowedOrigins from smackerel.yaml).
	// Default: no origins allowed (same-origin only). Set cors.allowed_origins
	// in smackerel.yaml to enable cross-origin access for web clients.
	if len(deps.CORSAllowedOrigins) > 0 {
		r.Use(cors.Handler(cors.Options{
			AllowedOrigins:   deps.CORSAllowedOrigins,
			AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
			AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
			ExposedHeaders:   []string{"Link"},
			AllowCredentials: true,
			MaxAge:           300,
		}))
	}

	// Prometheus metrics endpoint — unauthenticated (standard scrape pattern)
	r.Handle("/metrics", metrics.Handler())

	// Readiness probe — lightweight check for orchestrators (k8s, Docker HEALTHCHECK).
	// Only verifies DB connectivity; /api/health is the full liveness check.
	r.Get("/readyz", deps.ReadyzHandler)

	r.Route("/api", func(r chi.Router) {
		r.Use(middleware.Throttle(100))

		// Health endpoint — no auth required (monitoring)
		r.Get("/health", deps.HealthHandler)

		// Authenticated API routes
		r.Group(func(r chi.Router) {
			r.Use(deps.bearerAuthMiddleware)
			r.Post("/capture", deps.CaptureHandler)
			r.Post("/search", deps.SearchHandler)
			r.Get("/digest", deps.DigestHandler)
			r.Get("/recent", deps.RecentHandler)
			r.Get("/artifact/{id}", deps.ArtifactDetailHandler)
			r.Get("/artifacts/{id}/domain", deps.DomainDataHandler)
			r.Get("/export", deps.ExportHandler)

			// Context enrichment endpoint (GuestHost connector)
			if deps.ContextHandler != nil {
				r.Post("/context-for", deps.ContextHandler.HandleContextFor)
			}

			// Bookmark import endpoint (Phase 2)
			r.Post("/bookmarks/import", deps.BookmarkImportHandler)

			// Annotation endpoints (spec 027)
			if deps.AnnotationHandlers != nil {
				r.Route("/artifacts/{id}/annotations", func(r chi.Router) {
					r.Post("/", deps.AnnotationHandlers.CreateAnnotation)
					r.Get("/", deps.AnnotationHandlers.GetAnnotations)
					r.Get("/summary", deps.AnnotationHandlers.GetAnnotationSummary)
				})
				r.Delete("/artifacts/{id}/tags/{tag}", deps.AnnotationHandlers.DeleteTag)
				// Internal Telegram message-artifact mapping (spec 027, scope 5)
				r.Post("/internal/telegram-message-artifact", deps.AnnotationHandlers.RecordTelegramMessageArtifact)
				r.Get("/internal/telegram-message-artifact", deps.AnnotationHandlers.ResolveTelegramMessageArtifact)
			}

			// Knowledge layer endpoints (Scope 3)
			r.Route("/knowledge", func(r chi.Router) {
				r.Get("/concepts", deps.KnowledgeConceptsHandler)
				r.Get("/concepts/{id}", deps.KnowledgeConceptDetailHandler)
				r.Get("/entities", deps.KnowledgeEntitiesHandler)
				r.Get("/entities/{id}", deps.KnowledgeEntityDetailHandler)
				r.Get("/lint", deps.KnowledgeLintHandler)
				r.Get("/stats", deps.KnowledgeStatsHandler)
			})

			// Phase 5 intelligence endpoints (R-501..R-508)
			if deps.IntelligenceEngine != nil {
				r.Get("/expertise", ExpertiseHandler(deps.IntelligenceEngine))
				r.Get("/learning-paths", LearningPathsHandler(deps.IntelligenceEngine))
				r.Get("/subscriptions", SubscriptionsHandler(deps.IntelligenceEngine))
				r.Get("/serendipity", SerendipityHandler(deps.IntelligenceEngine))
				r.Get("/content-fuel", ContentFuelHandler(deps.IntelligenceEngine))
				r.Get("/quick-references", QuickReferencesHandler(deps.IntelligenceEngine))
				r.Get("/monthly-report", MonthlyReportHandler(deps.IntelligenceEngine))
				r.Get("/seasonal-patterns", SeasonalPatternsHandler(deps.IntelligenceEngine))
			}

			// Actionable list endpoints (spec 028)
			if deps.ListHandlers != nil {
				r.Route("/lists", func(r chi.Router) {
					r.Post("/", deps.ListHandlers.CreateListHandler)
					r.Get("/", deps.ListHandlers.ListListsHandler)
					r.Get("/{id}", deps.ListHandlers.GetListHandler)
					r.Patch("/{id}", deps.ListHandlers.UpdateListHandler)
					r.Delete("/{id}", deps.ListHandlers.ArchiveListHandler)
					r.Post("/{id}/items", deps.ListHandlers.AddItemHandler)
					r.Post("/{id}/items/{itemId}/check", deps.ListHandlers.CheckItemHandler)
					r.Delete("/{id}/items/{itemId}", deps.ListHandlers.RemoveItemHandler)
					r.Post("/{id}/complete", deps.ListHandlers.CompleteListHandler)
				})
			}

			// OAuth status requires authentication (token-bearing callers)
			if deps.OAuthHandler != nil {
				r.Get("/auth/status", deps.OAuthHandler.StatusHandler)
			}

			// Expense tracking endpoints (spec 034)
			if deps.ExpenseHandler != nil {
				deps.ExpenseHandler.RegisterRoutes(r)
			}

			// Meal planning endpoints (spec 036)
			if deps.MealPlanHandler != nil {
				deps.MealPlanHandler.RegisterRoutes(r)
			}
		})
	})

	// OAuth routes — no Bearer auth (browser redirect flow)
	// Both start and callback are rate-limited to prevent abuse (SEC-SWEEP-001).
	if deps.OAuthHandler != nil {
		r.Group(func(r chi.Router) {
			r.Use(httprate.LimitByIP(10, 1*time.Minute))
			r.Get("/auth/{provider}/start", deps.OAuthHandler.StartHandler)
			r.Get("/auth/{provider}/callback", deps.OAuthHandler.CallbackHandler)
		})
	}

	// Web UI routes (HTMX) - registered externally via RegisterWebRoutes
	if deps.WebHandler != nil {
		// Web UI group — auth required when AuthToken is configured.
		// When AuthToken is empty, webAuthMiddleware passes all requests (dev mode).
		r.Group(func(r chi.Router) {
			r.Use(deps.webAuthMiddleware)
			r.Get("/", deps.WebHandler.SearchPage)
			r.Post("/search", deps.WebHandler.SearchResults)
			r.Get("/artifact/{id}", deps.WebHandler.ArtifactDetail)
			r.Get("/digest", deps.WebHandler.DigestPage)
			r.Get("/topics", deps.WebHandler.TopicsPage)
			r.Get("/settings", deps.WebHandler.SettingsPage)
			r.Post("/settings/connectors/{id}/sync", deps.WebHandler.SyncConnectorHandler)
			r.Post("/settings/bookmarks/import", deps.WebHandler.BookmarkUploadHandler)
			r.Get("/status", deps.WebHandler.StatusPage)

			// Knowledge layer web routes
			r.Get("/knowledge", deps.WebHandler.KnowledgeDashboard)
			r.Get("/knowledge/concepts", deps.WebHandler.ConceptsList)
			r.Get("/knowledge/concepts/{id}", deps.WebHandler.ConceptDetail)
			r.Get("/knowledge/entities", deps.WebHandler.EntitiesList)
			r.Get("/knowledge/entities/{id}", deps.WebHandler.EntityDetail)
			r.Get("/knowledge/lint", deps.WebHandler.LintReport)
			r.Get("/knowledge/lint/{id}", deps.WebHandler.LintFindingDetail)
		})
	}

	// PWA routes (spec 033) — no auth required, PWA must be publicly installable
	r.Route("/pwa", func(r chi.Router) {
		r.Post("/share", deps.PWAShareHandler)
		r.Handle("/*", http.StripPrefix("/pwa", pwaFileServer()))
	})

	return r
}

// structuredLogger is a middleware that logs requests with slog.
// Health check and heartbeat endpoints are excluded to reduce log noise.
func structuredLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip logging for health check and heartbeat endpoints
		if r.URL.Path == "/api/health" || r.URL.Path == "/ping" {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		reqID := middleware.GetReqID(r.Context())
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
			"request_id", reqID,
		)
	})
}

// securityHeadersMiddleware sets security headers on all responses.
// Covers OWASP recommended headers: CSP, clickjacking, MIME sniffing, referrer leakage, cache control.
func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self' https://unpkg.com/htmx.org@1.9.12/ 'sha256-C7I7zL0TtdR86YSsw1T7pxobSVoQGAOH9Ua4apor8TI='; img-src 'self' data:; connect-src 'self'")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// extractBearerToken extracts the token from an "Authorization: Bearer <token>" header.
// Returns the token string, or empty string if the header is missing or malformed.
func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return ""
	}
	return parts[1]
}

// matchBearerToken returns true if the request carries a Bearer token that
// matches expected using constant-time comparison.
func matchBearerToken(r *http.Request, expected string) bool {
	token := extractBearerToken(r)
	if token == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(expected)) == 1
}

// webAuthMiddleware checks authentication for web UI routes.
// Accepts Bearer token in Authorization header or auth_token cookie.
// If no AuthToken is configured, all requests are allowed (dev mode).
func (d *Dependencies) webAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if d.AuthToken == "" {
			next.ServeHTTP(w, r)
			return
		}

		// Check Authorization header (Bearer token)
		if matchBearerToken(r, d.AuthToken) {
			next.ServeHTTP(w, r)
			return
		}

		// Check auth_token cookie (for browser sessions)
		if cookie, err := r.Cookie("auth_token"); err == nil {
			if subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(d.AuthToken)) == 1 {
				next.ServeHTTP(w, r)
				return
			}
		}

		slog.Warn("web auth failure", "path", r.URL.Path, "remote_addr", r.RemoteAddr)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	})
}

// bearerAuthMiddleware checks Bearer token authentication for API routes.
// If no AuthToken is configured, all requests are allowed (dev mode).
func (d *Dependencies) bearerAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if d.AuthToken == "" {
			next.ServeHTTP(w, r)
			return
		}

		token := extractBearerToken(r)
		if token == "" {
			reason := "missing header"
			if r.Header.Get("Authorization") != "" {
				reason = "invalid format"
			}
			slog.Warn("bearer auth failure", "path", r.URL.Path, "remote_addr", r.RemoteAddr, "reason", reason)
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Valid authentication required")
			return
		}

		if subtle.ConstantTimeCompare([]byte(token), []byte(d.AuthToken)) != 1 {
			slog.Warn("bearer auth failure", "path", r.URL.Path, "remote_addr", r.RemoteAddr, "reason", "invalid token")
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Valid authentication required")
			return
		}

		next.ServeHTTP(w, r)
	})
}
