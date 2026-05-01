package web

import (
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	recstore "github.com/smackerel/smackerel/internal/recommendation/store"
)

// TripDossierPage renders the per-trip recommendation block. The render shape
// follows the design Component Tree:
//
//	TripDossier
//	  └─ RecommendationGroupByCategory   (one block per category)
//	      └─ DossierRecommendationRow    (one row per delivered recommendation)
//	          └─ VariantGroup            (collapsed near-duplicates)
//
// Variants come from the parent recommendation's quality_decisions[] entries
// where kind == "diversity" (BS-027 / SCN-039-043). Recommendations whose
// underlying candidate canonical_fact contains a `trip_id` matching the URL
// parameter are included; nothing else is.
func (h *Handler) TripDossierPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if h.RecommendationStore == nil {
		http.Error(w, "recommendations unavailable", http.StatusServiceUnavailable)
		return
	}
	tripID := strings.TrimSpace(chi.URLParam(r, "trip_id"))
	if tripID == "" {
		http.Error(w, "missing trip_id", http.StatusBadRequest)
		return
	}
	groups, err := h.RecommendationStore.ListRecommendationsForTrip(r.Context(), tripID)
	if err != nil {
		http.Error(w, "trip dossier: "+err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, `<div class="container trip-dossier" data-testid="trip-dossier" data-trip-id="%s">`, template.HTMLEscapeString(tripID))
	fmt.Fprintf(w, `<h1>Trip dossier — %s</h1>`, template.HTMLEscapeString(tripID))
	if len(groups) == 0 {
		_, _ = fmt.Fprint(w, `<p class="muted" data-testid="trip-dossier-empty">No delivered recommendations are linked to this trip yet.</p></div>`)
		return
	}
	for _, group := range groups {
		fmt.Fprintf(w, `<section class="recommendation-group" data-testid="recommendation-group" data-category="%s">`, template.HTMLEscapeString(group.Category))
		fmt.Fprintf(w, `<h2 class="group-heading">%s</h2><ul class="dossier-rows">`, template.HTMLEscapeString(humanCategoryLabel(group.Category)))
		for _, rec := range group.Recommendations {
			renderTripDossierRow(w, rec)
		}
		_, _ = fmt.Fprint(w, `</ul></section>`)
	}
	_, _ = fmt.Fprint(w, `</div>`)
}

// renderTripDossierRow writes one DossierRecommendationRow block for the
// supplied recommendation. The row carries:
//   - the rank position
//   - the recommendation title (linked to the detail page)
//   - the first rationale line
//   - provider attribution badges
//   - a VariantGroup `<details>` block when diversity grouping fired
func renderTripDossierRow(w http.ResponseWriter, rec recstore.TripDossierRecommendation) {
	fmt.Fprintf(w,
		`<li class="dossier-row" data-testid="dossier-recommendation-row" data-recommendation-id="%s" data-rank="%d">`,
		template.HTMLEscapeString(rec.ID), rec.Rank)
	fmt.Fprintf(w,
		`<div class="dossier-row-header"><span class="rank">#%d</span> <a class="title" href="/recommendations/%s">%s</a></div>`,
		rec.Rank, template.HTMLEscapeString(rec.ID), template.HTMLEscapeString(rec.Title))
	if len(rec.Rationale) > 0 {
		fmt.Fprintf(w, `<p class="rationale">%s</p>`, template.HTMLEscapeString(rec.Rationale[0]))
	}
	if len(rec.ProviderBadges) > 0 {
		_, _ = fmt.Fprint(w, `<ul class="provider-badges">`)
		for _, badge := range rec.ProviderBadges {
			fmt.Fprintf(w,
				`<li class="provider-badge"><span class="provider-id">%s</span> <span class="provider-label">%s</span></li>`,
				template.HTMLEscapeString(badge.ProviderID), template.HTMLEscapeString(badge.Label))
		}
		_, _ = fmt.Fprint(w, `</ul>`)
	}
	if len(rec.Variants) > 0 {
		fmt.Fprintf(w,
			`<details class="variant-group" data-testid="variant-group" data-variant-count="%d"><summary>+%d similar option%s</summary><ul class="variants">`,
			len(rec.Variants), len(rec.Variants), pluralSuffix(len(rec.Variants)))
		for _, variant := range rec.Variants {
			fmt.Fprintf(w,
				`<li class="variant" data-canonical-key="%s">%s</li>`,
				template.HTMLEscapeString(variant.CanonicalKey), template.HTMLEscapeString(variant.Title))
		}
		_, _ = fmt.Fprint(w, `</ul></details>`)
	}
	_, _ = fmt.Fprint(w, `</li>`)
}

func humanCategoryLabel(category string) string {
	switch category {
	case "place":
		return "Places"
	case "product":
		return "Products"
	case "deal":
		return "Deals"
	case "event":
		return "Events"
	case "content":
		return "Content"
	default:
		return category
	}
}

func pluralSuffix(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
