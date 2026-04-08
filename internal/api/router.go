package api

import (
	"crypto/subtle"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// NewRouter creates the Chi router with all routes and middleware.
func NewRouter(deps *Dependencies) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(structuredLogger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Heartbeat("/ping"))
	r.Use(cspMiddleware)

	r.Route("/api", func(r chi.Router) {
		r.Use(middleware.Throttle(100))
		r.Get("/health", deps.HealthHandler)
		r.Post("/capture", deps.CaptureHandler)
		r.Post("/search", deps.SearchHandler)
		r.Get("/digest", deps.DigestHandler)
		r.Get("/recent", deps.RecentHandler)
		r.Get("/artifact/{id}", deps.ArtifactDetailHandler)
		r.Get("/export", deps.ExportHandler)

		// OAuth status requires authentication (token-bearing callers)
		if deps.OAuthHandler != nil {
			type oauthStatusRouter interface {
				StatusHandler(w http.ResponseWriter, r *http.Request)
			}
			oh := deps.OAuthHandler.(oauthStatusRouter)
			r.Get("/auth/status", oh.StatusHandler)
		}
	})

	// OAuth routes — no Bearer auth (browser redirect flow)
	if deps.OAuthHandler != nil {
		type oauthRouter interface {
			StartHandler(w http.ResponseWriter, r *http.Request)
			CallbackHandler(w http.ResponseWriter, r *http.Request)
		}
		oh := deps.OAuthHandler.(oauthRouter)
		r.Get("/auth/{provider}/start", oh.StartHandler)
		r.Get("/auth/{provider}/callback", oh.CallbackHandler)
	}

	// Web UI routes (HTMX) - registered externally via RegisterWebRoutes
	if deps.WebHandler != nil {
		type webRouter interface {
			SearchPage(w http.ResponseWriter, r *http.Request)
			SearchResults(w http.ResponseWriter, r *http.Request)
			ArtifactDetail(w http.ResponseWriter, r *http.Request)
			DigestPage(w http.ResponseWriter, r *http.Request)
			TopicsPage(w http.ResponseWriter, r *http.Request)
			SettingsPage(w http.ResponseWriter, r *http.Request)
			StatusPage(w http.ResponseWriter, r *http.Request)
		}

		wh := deps.WebHandler.(webRouter)

		// Web UI group — no auth required (local-first self-hosted).
		// Security note: webAuthMiddleware exists but is intentionally NOT applied
		// here. Smackerel is designed as a local-first, self-hosted tool — the Web
		// UI is served on localhost and does not require token authentication.
		// API routes are the programmatic boundary and carry their own auth when
		// AuthToken is configured.
		r.Group(func(r chi.Router) {
			r.Get("/", wh.SearchPage)
			r.Post("/search", wh.SearchResults)
			r.Get("/artifact/{id}", wh.ArtifactDetail)
			r.Get("/digest", wh.DigestPage)
			r.Get("/topics", wh.TopicsPage)
			r.Get("/settings", wh.SettingsPage)
			r.Get("/status", wh.StatusPage)
		})
	}

	return r
}

// structuredLogger is a middleware that logs requests with slog.
func structuredLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

// cspMiddleware sets Content-Security-Policy header on all responses.
func cspMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self' https://unpkg.com; img-src 'self' data:; connect-src 'self'")
		next.ServeHTTP(w, r)
	})
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
		if auth := r.Header.Get("Authorization"); auth != "" {
			parts := strings.SplitN(auth, " ", 2)
			if len(parts) == 2 && strings.EqualFold(parts[0], "bearer") {
				if subtle.ConstantTimeCompare([]byte(parts[1]), []byte(d.AuthToken)) == 1 {
					next.ServeHTTP(w, r)
					return
				}
			}
		}

		// Check auth_token cookie (for browser sessions)
		if cookie, err := r.Cookie("auth_token"); err == nil {
			if subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(d.AuthToken)) == 1 {
				next.ServeHTTP(w, r)
				return
			}
		}

		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	})
}
