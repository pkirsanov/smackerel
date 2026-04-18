package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/smackerel/smackerel/internal/recipe"
)

// DomainDataHandler handles GET /api/artifacts/{id}/domain?servings={N}.
// Returns the artifact's domain_data, optionally scaled if servings parameter is provided.
func (d *Dependencies) DomainDataHandler(w http.ResponseWriter, r *http.Request) {
	artifactID := chi.URLParam(r, "id")
	if artifactID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_INPUT", "Artifact ID is required")
		return
	}
	if len(artifactID) > maxArtifactIDLen {
		writeError(w, http.StatusBadRequest, "INVALID_INPUT", "Artifact ID exceeds maximum length")
		return
	}

	if d.ArtifactStore == nil {
		writeError(w, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "Service unavailable")
		return
	}

	a, err := d.ArtifactStore.GetArtifactWithDomain(r.Context(), artifactID)
	if err != nil {
		writeError(w, http.StatusNotFound, "ARTIFACT_NOT_FOUND", "Artifact not found")
		return
	}

	if len(a.DomainData) == 0 {
		writeError(w, http.StatusNotFound, "NO_DOMAIN_DATA", "Artifact has no domain data")
		return
	}

	// Check for servings query parameter
	servingsParam := r.URL.Query().Get("servings")
	if servingsParam == "" {
		// No scaling requested — return raw domain_data
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(a.DomainData) //nolint:errcheck
		return
	}

	// Parse and validate servings parameter
	targetServings, err := strconv.Atoi(servingsParam)
	if err != nil || targetServings <= 0 {
		writeError(w, http.StatusBadRequest, "INVALID_SERVINGS", "Servings must be a positive integer")
		return
	}

	// Unmarshal domain_data to check if it's a recipe
	var rd recipe.RecipeData
	if err := json.Unmarshal(a.DomainData, &rd); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "DOMAIN_NOT_SCALABLE", "Serving scaling only applies to recipes")
		return
	}

	if rd.Domain != "recipe" {
		writeError(w, http.StatusUnprocessableEntity, "DOMAIN_NOT_SCALABLE", "Serving scaling only applies to recipes")
		return
	}

	if rd.Servings == nil {
		writeError(w, http.StatusUnprocessableEntity, "NO_BASELINE_SERVINGS", "Recipe does not specify a base serving count")
		return
	}

	originalServings := *rd.Servings
	scaleFactor := float64(targetServings) / float64(originalServings)

	// Scale ingredients
	scaled := recipe.ScaleIngredients(rd.Ingredients, originalServings, targetServings)

	// Build scaled response
	type scaledIngredientResponse struct {
		Name            string `json:"name"`
		Quantity        string `json:"quantity"`
		DisplayQuantity string `json:"display_quantity"`
		Unit            string `json:"unit"`
		Scaled          bool   `json:"scaled"`
		Preparation     string `json:"preparation,omitempty"`
	}

	scaledIngredients := make([]scaledIngredientResponse, len(scaled))
	for i, si := range scaled {
		scaledIngredients[i] = scaledIngredientResponse{
			Name:            si.Name,
			Quantity:        si.DisplayQuantity,
			DisplayQuantity: si.DisplayQuantity,
			Unit:            si.Unit,
			Scaled:          si.Scaled,
			Preparation:     si.Preparation,
		}
	}

	response := map[string]interface{}{
		"domain":            rd.Domain,
		"title":             rd.Title,
		"servings":          targetServings,
		"original_servings": originalServings,
		"scale_factor":      scaleFactor,
		"timing":            rd.Timing,
		"cuisine":           rd.Cuisine,
		"difficulty":        rd.Difficulty,
		"dietary_tags":      rd.DietaryTags,
		"ingredients":       scaledIngredients,
		"steps":             rd.Steps,
	}

	writeJSON(w, http.StatusOK, response)
}
