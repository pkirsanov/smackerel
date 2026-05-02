//go:build integration

package integration

import (
	"github.com/go-chi/chi/v5"

	"github.com/smackerel/smackerel/internal/api"
)

// newCapabilityCanaryRouter wires the new ExerciseCapability handler
// behind a real chi router so `chi.URLParam(r, "capability")` resolves
// the way it does in production. Used by
// `TestPhotosCapabilityTaxonomyCanary_GoRegistryMatchesPWALimitationCodes`.
func newCapabilityCanaryRouter(handlers *api.PhotosHandlers) chi.Router {
	router := chi.NewRouter()
	router.Post("/v1/photos/connectors/capabilities/{capability}/exercise", handlers.ExerciseCapability)
	return router
}
