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
	_, _ = fmt.Fprintf(w, `<div class="container"><h1>%s</h1><section><h2>Why</h2><ul>`, template.HTMLEscapeString(rec.Title))
	for _, reason := range rec.Rationale {
		_, _ = fmt.Fprintf(w, `<li>%s</li>`, template.HTMLEscapeString(reason))
	}
	_, _ = fmt.Fprint(w, `</ul></section><section><h2>Sources</h2><ul>`)
	for _, badge := range rec.ProviderBadges {
		_, _ = fmt.Fprintf(w, `<li>%s</li>`, template.HTMLEscapeString(badge.Label))
	}
	_, _ = fmt.Fprint(w, `</ul></section></div>`)
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
			_, _ = fmt.Fprintf(w, `<span class="badge">%s</span> `, template.HTMLEscapeString(badge.Label))
		}
		_, _ = fmt.Fprint(w, `</p><ul>`)
		for _, reason := range rec.Rationale {
			_, _ = fmt.Fprintf(w, `<li>%s</li>`, template.HTMLEscapeString(reason))
		}
		_, _ = fmt.Fprintf(w, `</ul><a href="/recommendations/%s">Why?</a></article>`, template.HTMLEscapeString(rec.ID))
	}
}
