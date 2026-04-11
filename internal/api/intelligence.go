package api

import (
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
		writeJSON(w, http.StatusOK, expertiseMap)
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
		writeJSON(w, http.StatusOK, paths)
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
		writeJSON(w, http.StatusOK, summary)
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
			writeJSON(w, http.StatusOK, map[string]string{
				"message": "No serendipity candidates available yet. Archive items need 6+ months of dormancy.",
			})
			return
		}
		writeJSON(w, http.StatusOK, pick)
	}
}

// ContentFuelHandler handles GET /api/content-fuel per R-503.
func ContentFuelHandler(engine *intelligence.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		angles, err := engine.GenerateContentFuel(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "content_fuel_error", "content fuel generation failed")
			return
		}
		if angles == nil {
			angles = []intelligence.ContentAngle{}
		}
		writeJSON(w, http.StatusOK, angles)
	}
}

// QuickReferencesHandler handles GET /api/quick-references per R-507.
func QuickReferencesHandler(engine *intelligence.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		refs, err := engine.GetQuickReferences(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "quick_references_error", "quick references query failed")
			return
		}
		if refs == nil {
			refs = []intelligence.QuickReference{}
		}
		writeJSON(w, http.StatusOK, refs)
	}
}

// MonthlyReportHandler handles GET /api/monthly-report per R-506.
func MonthlyReportHandler(engine *intelligence.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		report, err := engine.GenerateMonthlyReport(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "monthly_report_error", "monthly report generation failed")
			return
		}
		writeJSON(w, http.StatusOK, report)
	}
}

// SeasonalPatternsHandler handles GET /api/seasonal-patterns per R-508.
func SeasonalPatternsHandler(engine *intelligence.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		patterns, err := engine.DetectSeasonalPatterns(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "seasonal_error", "seasonal pattern detection failed")
			return
		}
		if patterns == nil {
			patterns = []intelligence.SeasonalPattern{}
		}
		writeJSON(w, http.StatusOK, patterns)
	}
}
