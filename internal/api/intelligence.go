package api

import (
	"encoding/json"
	"net/http"

	"github.com/smackerel/smackerel/internal/intelligence"
)

// ExpertiseHandler handles GET /api/expertise per R-501.
func ExpertiseHandler(engine *intelligence.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		expertiseMap, err := engine.GenerateExpertiseMap(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "expertise_error", "expertise map generation failed")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(expertiseMap)
	}
}

// LearningPathsHandler handles GET /api/learning-paths per R-502.
func LearningPathsHandler(engine *intelligence.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		paths, err := engine.GetLearningPaths(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "learning_error", "learning paths query failed")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(paths)
	}
}

// SubscriptionsHandler handles GET /api/subscriptions per R-504.
func SubscriptionsHandler(engine *intelligence.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		summary, err := engine.GetSubscriptionSummary(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "subscription_error", "subscription summary failed")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(summary)
	}
}

// SerendipityHandler handles GET /api/serendipity per R-505.
func SerendipityHandler(engine *intelligence.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pick, err := engine.SerendipityPick(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "serendipity_error", "serendipity pick failed")
			return
		}
		if pick == nil {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"message":"No serendipity candidates available yet. Archive items need 6+ months of dormancy."}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(pick)
	}
}
