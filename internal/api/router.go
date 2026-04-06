package api

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// NewRouter creates the Chi router with all routes and middleware.
func NewRouter(deps *Dependencies) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Heartbeat("/ping"))

	r.Route("/api", func(r chi.Router) {
		r.Get("/health", deps.HealthHandler)
		r.Post("/capture", deps.CaptureHandler)
		r.Post("/search", deps.SearchHandler)
		r.Get("/digest", deps.DigestHandler)
	})

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

		// Web UI group with optional auth (same as API)
		r.Group(func(r chi.Router) {
			r.Use(deps.webAuthMiddleware)
			r.Get("/", wh.SearchPage)
			r.Post("/search", wh.SearchResults)
			r.Get("/artifact", wh.ArtifactDetail)
			r.Get("/ui/digest", wh.DigestPage)
			r.Get("/topics", wh.TopicsPage)
			r.Get("/settings", wh.SettingsPage)
			r.Get("/ui/status", wh.StatusPage)
		})
	}

	return r
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
