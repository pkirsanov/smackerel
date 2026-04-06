package api

import (
	"net/http"

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
		r.Get("/", wh.SearchPage)
		r.Post("/search", wh.SearchResults)
		r.Get("/artifact", wh.ArtifactDetail)
		r.Get("/ui/digest", wh.DigestPage)
		r.Get("/topics", wh.TopicsPage)
		r.Get("/settings", wh.SettingsPage)
		r.Get("/ui/status", wh.StatusPage)
	}

	return r
}
