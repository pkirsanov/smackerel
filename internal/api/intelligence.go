package api

import (
	"net/http"
	"time"

	"github.com/smackerel/smackerel/internal/intelligence"
	"github.com/smackerel/smackerel/internal/metrics"
)

// ExpertiseHandler handles GET /api/expertise per R-501.
func ExpertiseHandler(engine *intelligence.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		expertiseMap, err := engine.GenerateExpertiseMap(r.Context())
		if err != nil {
			metrics.IntelligenceErrors.WithLabelValues("expertise").Inc()
			writeError(w, http.StatusInternalServerError, "expertise_error", "expertise map generation failed")
			return
		}
		metrics.IntelligenceLatency.WithLabelValues("expertise").Observe(time.Since(start).Seconds())
		writeJSON(w, http.StatusOK, expertiseMap)
	}
}

// LearningPathsHandler handles GET /api/learning-paths per R-502.
func LearningPathsHandler(engine *intelligence.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		paths, err := engine.GetLearningPaths(r.Context())
		if err != nil {
			metrics.IntelligenceErrors.WithLabelValues("learning_paths").Inc()
			writeError(w, http.StatusInternalServerError, "learning_error", "learning paths query failed")
			return
		}
		metrics.IntelligenceLatency.WithLabelValues("learning_paths").Observe(time.Since(start).Seconds())
		writeJSON(w, http.StatusOK, paths)
	}
}

// SubscriptionsHandler handles GET /api/subscriptions per R-504.
func SubscriptionsHandler(engine *intelligence.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		summary, err := engine.GetSubscriptionSummary(r.Context())
		if err != nil {
			metrics.IntelligenceErrors.WithLabelValues("subscriptions").Inc()
			writeError(w, http.StatusInternalServerError, "subscription_error", "subscription summary failed")
			return
		}
		metrics.IntelligenceLatency.WithLabelValues("subscriptions").Observe(time.Since(start).Seconds())
		writeJSON(w, http.StatusOK, summary)
	}
}

// SerendipityHandler handles GET /api/serendipity per R-505.
func SerendipityHandler(engine *intelligence.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		pick, err := engine.SerendipityPick(r.Context())
		if err != nil {
			metrics.IntelligenceErrors.WithLabelValues("serendipity").Inc()
			writeError(w, http.StatusInternalServerError, "serendipity_error", "serendipity pick failed")
			return
		}
		metrics.IntelligenceLatency.WithLabelValues("serendipity").Observe(time.Since(start).Seconds())
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
		start := time.Now()
		angles, err := engine.GenerateContentFuel(r.Context())
		if err != nil {
			metrics.IntelligenceErrors.WithLabelValues("content_fuel").Inc()
			writeError(w, http.StatusInternalServerError, "content_fuel_error", "content fuel generation failed")
			return
		}
		metrics.IntelligenceLatency.WithLabelValues("content_fuel").Observe(time.Since(start).Seconds())
		if angles == nil {
			angles = []intelligence.ContentAngle{}
		}
		writeJSON(w, http.StatusOK, angles)
	}
}

// QuickReferencesHandler handles GET /api/quick-references per R-507.
func QuickReferencesHandler(engine *intelligence.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		refs, err := engine.GetQuickReferences(r.Context())
		if err != nil {
			metrics.IntelligenceErrors.WithLabelValues("quick_references").Inc()
			writeError(w, http.StatusInternalServerError, "quick_references_error", "quick references query failed")
			return
		}
		metrics.IntelligenceLatency.WithLabelValues("quick_references").Observe(time.Since(start).Seconds())
		if refs == nil {
			refs = []intelligence.QuickReference{}
		}
		writeJSON(w, http.StatusOK, refs)
	}
}

// MonthlyReportHandler handles GET /api/monthly-report per R-506.
func MonthlyReportHandler(engine *intelligence.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		report, err := engine.GenerateMonthlyReport(r.Context())
		if err != nil {
			metrics.IntelligenceErrors.WithLabelValues("monthly_report").Inc()
			writeError(w, http.StatusInternalServerError, "monthly_report_error", "monthly report generation failed")
			return
		}
		metrics.IntelligenceLatency.WithLabelValues("monthly_report").Observe(time.Since(start).Seconds())
		writeJSON(w, http.StatusOK, report)
	}
}

// SeasonalPatternsHandler handles GET /api/seasonal-patterns per R-508.
func SeasonalPatternsHandler(engine *intelligence.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		patterns, err := engine.DetectSeasonalPatterns(r.Context())
		if err != nil {
			metrics.IntelligenceErrors.WithLabelValues("seasonal_patterns").Inc()
			writeError(w, http.StatusInternalServerError, "seasonal_error", "seasonal pattern detection failed")
			return
		}
		metrics.IntelligenceLatency.WithLabelValues("seasonal_patterns").Observe(time.Since(start).Seconds())
		if patterns == nil {
			patterns = []intelligence.SeasonalPattern{}
		}
		writeJSON(w, http.StatusOK, patterns)
	}
}
