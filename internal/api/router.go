package api

import (
	"crypto/subtle"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"
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
			r.Get("/export", deps.ExportHandler)

			// Context enrichment endpoint (GuestHost connector)
			if deps.ContextHandler != nil {
				r.Post("/context-for", deps.ContextHandler.HandleContextFor)
			}

			// Bookmark import endpoint (Phase 2)
			r.Post("/bookmarks/import", deps.BookmarkImportHandler)

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

			// OAuth status requires authentication (token-bearing callers)
			if deps.OAuthHandler != nil {
				r.Get("/auth/status", deps.OAuthHandler.StatusHandler)
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
		})
	}

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
// Covers OWASP recommended headers: CSP, clickjacking, MIME sniffing, referrer leakage.
func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self' https://unpkg.com; img-src 'self' data:; connect-src 'self'")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
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
