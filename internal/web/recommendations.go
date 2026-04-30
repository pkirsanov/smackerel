package web

import (
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/smackerel/smackerel/internal/recommendation"
	"github.com/smackerel/smackerel/internal/recommendation/reactive"
	recstore "github.com/smackerel/smackerel/internal/recommendation/store"
)

// RecommendationsPage renders the recommendation request shell and optional persisted result.
func (h *Handler) RecommendationsPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprint(w, `<div class="container"><h1>Recommendations</h1><form hx-post="/recommendations/results" hx-target="#recommendation-results" method="post"><input name="query" placeholder="quiet ramen near Mission" required><input name="location_ref" placeholder="neighborhood"><select name="precision_policy"><option value="neighborhood">Neighborhood</option><option value="city">City</option></select><button type="submit">Get recommendations</button></form><div id="recommendation-results">`)
	requestID := strings.TrimSpace(r.URL.Query().Get("request_id"))
	if requestID != "" && h.RecommendationStore != nil {
		outcome, err := h.RecommendationStore.GetRequest(r.Context(), requestID)
		if err == nil {
			h.renderRecommendationCards(w, outcome)
		}
	}
	_, _ = fmt.Fprint(w, `</div></div>`)
}

// RecommendationsResults handles the HTMX form post and renders result cards.
func (h *Handler) RecommendationsResults(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if h.RecommendationStore == nil || h.RecommendationRegistry == nil {
		http.Error(w, "recommendations unavailable", http.StatusServiceUnavailable)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	precision := recommendation.PrecisionPolicy(r.FormValue("precision_policy"))
	engine := reactive.NewEngine(reactive.Options{
		Store:    h.RecommendationStore,
		Registry: h.RecommendationRegistry,
		Config:   h.RecommendationConfig,
	})
	outcome, err := engine.Run(r.Context(), reactive.Request{
		ActorUserID:     "local",
		Source:          "web",
		Query:           r.FormValue("query"),
		LocationRef:     r.FormValue("location_ref"),
		NamedLocation:   r.FormValue("named_location"),
		PrecisionPolicy: precision,
		ResultCount:     h.RecommendationConfig.Ranking.StandardResultCount,
	})
	if err != nil {
		http.Error(w, "recommendation request failed", http.StatusInternalServerError)
		return
	}
	h.renderRecommendationCards(w, outcome)
}

// RecommendationDetail renders one recommendation provenance panel.
func (h *Handler) RecommendationDetail(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if h.RecommendationStore == nil {
		http.Error(w, "recommendations unavailable", http.StatusServiceUnavailable)
		return
	}
	recommendationID := strings.TrimSpace(chi.URLParam(r, "id"))
	rec, err := h.RecommendationStore.GetRecommendation(r.Context(), recommendationID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	why, err := h.RecommendationStore.ExplainRecommendation(r.Context(), recommendationID)
	if err != nil {
		http.Error(w, "recommendation explanation unavailable", http.StatusInternalServerError)
		return
	}
	_, _ = fmt.Fprintf(w, `<div class="container"><h1>%s</h1><section><h2>Why</h2><ul>`, template.HTMLEscapeString(rec.Title))
	for _, reason := range why.Explanation {
		_, _ = fmt.Fprintf(w, `<li>%s</li>`, template.HTMLEscapeString(reason))
	}
	_, _ = fmt.Fprintf(w, `</ul><p>Provider calls issued: %t</p></section><section><h2>Sources</h2><ul>`, why.ProviderCallsIssued)
	for _, badge := range rec.ProviderBadges {
		_, _ = fmt.Fprintf(w, `<li>%s</li>`, template.HTMLEscapeString(badge.Label))
	}
	_, _ = fmt.Fprint(w, `</ul></section></div>`)
}

// RecommendationFeedback handles HTMX feedback actions from recommendation cards.
func (h *Handler) RecommendationFeedback(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if h.RecommendationStore == nil {
		http.Error(w, "recommendations unavailable", http.StatusServiceUnavailable)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid feedback form", http.StatusBadRequest)
		return
	}
	recommendationID := strings.TrimSpace(chi.URLParam(r, "id"))
	result, err := h.RecommendationStore.RecordFeedback(r.Context(), recstore.FeedbackInput{
		RecommendationID: recommendationID,
		ActorUserID:      "local",
		FeedbackType:     r.FormValue("feedback_type"),
		SourceWatchID:    r.FormValue("source_watch_id"),
		PreferenceKey:    r.FormValue("preference_key"),
		CorrectionKind:   r.FormValue("correction_kind"),
		Payload:          map[string]any{"surface": "web"},
	})
	if err != nil {
		http.Error(w, "feedback failed", http.StatusBadRequest)
		return
	}
	state := "recorded"
	if result.SuppressionEffect.Applied {
		state = "suppressed"
	}
	_, _ = fmt.Fprintf(w, `<div class="feedback-state" data-feedback-state="%s"><strong>%s</strong><span>%s</span></div>`, template.HTMLEscapeString(state), template.HTMLEscapeString(result.Acknowledgement), template.HTMLEscapeString(result.SuppressionEffect.Reason))
}

// RecommendationPreferencesPage renders active preference corrections.
func (h *Handler) RecommendationPreferencesPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if h.RecommendationStore == nil {
		http.Error(w, "recommendations unavailable", http.StatusServiceUnavailable)
		return
	}
	view, err := h.RecommendationStore.ListPreferences(r.Context(), "local")
	if err != nil {
		http.Error(w, "preferences unavailable", http.StatusInternalServerError)
		return
	}
	_, _ = fmt.Fprint(w, `<div class="container"><h1>Recommendations &gt; Preferences</h1><section><h2>Active corrections</h2>`)
	if len(view.ActiveCorrections) == 0 {
		_, _ = fmt.Fprint(w, `<p>No active corrections</p>`)
	} else {
		_, _ = fmt.Fprint(w, `<table><thead><tr><th>Preference</th><th>Correction</th><th>ID</th><th></th></tr></thead><tbody>`)
		for _, correction := range view.ActiveCorrections {
			_, _ = fmt.Fprintf(w, `<tr><td>%s</td><td>%s</td><td>%s</td><td><button type="button">Revoke</button></td></tr>`, template.HTMLEscapeString(correction.PreferenceKey), template.HTMLEscapeString(correction.CorrectionKind), template.HTMLEscapeString(correction.ID))
		}
		_, _ = fmt.Fprint(w, `</tbody></table>`)
	}
	_, _ = fmt.Fprint(w, `</section></div>`)
}

func (h *Handler) renderRecommendationCards(w http.ResponseWriter, outcome recstore.RenderedRequest) {
	if outcome.Clarification != nil {
		_, _ = fmt.Fprintf(w, `<div class="notice"><p>%s</p>`, template.HTMLEscapeString(outcome.Clarification.Question))
		for _, choice := range outcome.Clarification.Choices {
			_, _ = fmt.Fprintf(w, `<button type="button">%s</button>`, template.HTMLEscapeString(choice))
		}
		_, _ = fmt.Fprint(w, `</div>`)
		return
	}
	for _, rec := range outcome.Recommendations {
		_, _ = fmt.Fprintf(w, `<article class="card recommendation-card" data-recommendation-id="%s"><h2>%d. %s</h2><p>`, template.HTMLEscapeString(rec.ID), rec.Rank, template.HTMLEscapeString(rec.Title))
		for _, badge := range rec.ProviderBadges {
			if badge.URL != "" {
				_, _ = fmt.Fprintf(w, `<a class="badge" href="%s">%s</a> `, template.HTMLEscapeString(badge.URL), template.HTMLEscapeString(badge.Label))
				continue
			}
			_, _ = fmt.Fprintf(w, `<span class="badge">%s</span> `, template.HTMLEscapeString(badge.Label))
		}
		_, _ = fmt.Fprint(w, `</p><ul>`)
		if rec.DistanceLabel != "" {
			_, _ = fmt.Fprintf(w, `<li>%s distance: %s</li>`, template.HTMLEscapeString(rec.DistanceBasis), template.HTMLEscapeString(rec.DistanceLabel))
		}
		for _, reason := range rec.Rationale {
			_, _ = fmt.Fprintf(w, `<li>%s</li>`, template.HTMLEscapeString(reason))
		}
		_, _ = fmt.Fprintf(w, `</ul><a href="/recommendations/%s">Why?</a><form hx-post="/recommendations/%s/feedback" hx-target="closest article" method="post"><input type="hidden" name="feedback_type" value="not_interested"><button type="submit">Not interested</button></form></article>`, template.HTMLEscapeString(rec.ID), template.HTMLEscapeString(rec.ID))
	}
}
