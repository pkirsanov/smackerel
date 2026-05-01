package web

import (
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/smackerel/smackerel/internal/recommendation/policy"
	recstore "github.com/smackerel/smackerel/internal/recommendation/store"
)

// RecommendationWatchesPage renders the saved watches list with HTMX controls
// for pause/resume/silence/edit/delete. SCN-039-038 (BS-021) is enforced by
// only listing user-created watches; passive behavior never adds rows here.
func (h *Handler) RecommendationWatchesPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if h.RecommendationStore == nil {
		http.Error(w, "recommendations unavailable", http.StatusServiceUnavailable)
		return
	}
	watches, err := h.RecommendationStore.ListWatches(r.Context(), "local")
	if err != nil {
		http.Error(w, "watch list unavailable", http.StatusInternalServerError)
		return
	}
	_, _ = fmt.Fprint(w, `<div class="container"><h1>Recommendations &gt; Watches</h1><a href="/recommendations/watches/new" class="action">New watch</a><table class="watch-list"><thead><tr><th>Name</th><th>Kind</th><th>Status</th><th>Last run</th><th>Next due</th><th>Consent</th><th></th></tr></thead><tbody>`)
	for _, watch := range watches {
		state := "active"
		switch {
		case !watch.Enabled:
			state = "paused"
		case watch.SilenceUntil != nil && watch.SilenceUntil.After(time.Now().UTC()):
			state = "silenced"
		}
		consentSummary := watchConsentSummary(watch)
		_, _ = fmt.Fprintf(w, `<tr data-watch-id="%s"><td><a href="/recommendations/watches/%s">%s</a></td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>`,
			template.HTMLEscapeString(watch.ID),
			template.HTMLEscapeString(watch.ID),
			template.HTMLEscapeString(watch.Name),
			template.HTMLEscapeString(watch.Kind),
			template.HTMLEscapeString(state),
			renderTime(watch.LastRunAt),
			renderTime(watch.NextDueAt),
			template.HTMLEscapeString(consentSummary),
		)
		_, _ = fmt.Fprintf(w, `<form hx-post="/recommendations/watches/%s/pause" hx-swap="outerHTML"><button type="submit">Pause</button></form>`, template.HTMLEscapeString(watch.ID))
		_, _ = fmt.Fprintf(w, `<form hx-post="/recommendations/watches/%s/resume" hx-swap="outerHTML"><button type="submit">Resume</button></form>`, template.HTMLEscapeString(watch.ID))
		_, _ = fmt.Fprintf(w, `<a href="/recommendations/watches/%s/edit">Edit</a> `, template.HTMLEscapeString(watch.ID))
		_, _ = fmt.Fprintf(w, `<form hx-delete="/recommendations/watches/%s?confirm=yes" hx-confirm="Delete watch '%s'? This cannot be undone." hx-swap="outerHTML"><button type="submit">Delete</button></form>`, template.HTMLEscapeString(watch.ID), template.HTMLEscapeString(watch.Name))
		_, _ = fmt.Fprint(w, `</td></tr>`)
	}
	_, _ = fmt.Fprint(w, `</tbody></table></div>`)
}

// RecommendationWatchEditorPage renders the create/edit form. The form posts
// to the API endpoint via HTMX. New watches start with an empty form; existing
// watches preload the current values.
func (h *Handler) RecommendationWatchEditorPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if h.RecommendationStore == nil {
		http.Error(w, "recommendations unavailable", http.StatusServiceUnavailable)
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	mode := "create"
	formAction := "/recommendations/watches"
	htmxMethod := "hx-post"
	var existing *recstore.WatchRecord
	if id != "" {
		watch, err := h.RecommendationStore.GetWatch(r.Context(), id)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		existing = &watch
		mode = "edit"
		formAction = "/recommendations/watches/" + id
		htmxMethod = "hx-put"
	}
	header := "Create watch"
	if mode == "edit" {
		header = "Edit watch"
	}
	_, _ = fmt.Fprintf(w, `<div class="container"><h1>%s</h1>`, template.HTMLEscapeString(header))
	if mode == "edit" {
		_, _ = fmt.Fprintf(w, `<p class="consent-summary">Current consent: %s</p>`, template.HTMLEscapeString(watchConsentSummary(*existing)))
	}
	_, _ = fmt.Fprintf(w, `<form %s="%s" hx-target="#watch-result" hx-encoding="application/json"><div id="watch-result"></div>`, htmxMethod, template.HTMLEscapeString(formAction))
	_, _ = fmt.Fprint(w, watchFormFields(existing))
	_, _ = fmt.Fprint(w, watchConsentFields(existing))
	_, _ = fmt.Fprint(w, `<button type="submit" data-confirm-flag="all">Save and confirm consent</button></form></div>`)
}

// RecommendationWatchDetailPage renders the latest runs and consent ledger.
func (h *Handler) RecommendationWatchDetailPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if h.RecommendationStore == nil {
		http.Error(w, "recommendations unavailable", http.StatusServiceUnavailable)
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	watch, err := h.RecommendationStore.GetWatch(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	_, _ = fmt.Fprintf(w, `<div class="container"><h1>%s</h1><p>Kind: %s &middot; Delivery: %s &middot; Precision: %s</p>`, template.HTMLEscapeString(watch.Name), template.HTMLEscapeString(watch.Kind), template.HTMLEscapeString(watch.DeliveryChannel), template.HTMLEscapeString(watch.LocationPrecision))
	_, _ = fmt.Fprint(w, `<section><h2>Consent ledger</h2><ol class="consent-ledger">`)
	for _, revision := range watch.Consent.Revisions {
		_, _ = fmt.Fprintf(w, `<li><strong>%s</strong> &mdash; %s &mdash; rate %d/%ds, precision %s, sources %s</li>`,
			template.HTMLEscapeString(revision.At.Format(time.RFC3339)),
			template.HTMLEscapeString(revision.Reason),
			revision.NamedValues.MaxAlerts,
			revision.NamedValues.WindowSeconds,
			template.HTMLEscapeString(revision.NamedValues.Precision),
			template.HTMLEscapeString(strings.Join(revision.NamedValues.Sources, ",")),
		)
	}
	_, _ = fmt.Fprint(w, `</ol></section>`)
	_, _ = fmt.Fprintf(w, `<a href="/recommendations/watches/%s/edit">Edit consent &amp; settings</a></div>`, template.HTMLEscapeString(watch.ID))
}

// RecommendationWatchPauseAction handles the HTMX pause control.
func (h *Handler) RecommendationWatchPauseAction(w http.ResponseWriter, r *http.Request) {
	if h.RecommendationStore == nil {
		http.Error(w, "recommendations unavailable", http.StatusServiceUnavailable)
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if err := h.RecommendationStore.PauseWatch(r.Context(), id, time.Now().UTC()); err != nil {
		http.Error(w, "pause failed", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprintf(w, `<tr data-watch-id="%s" data-watch-state="paused"><td colspan="7">Watch paused.</td></tr>`, template.HTMLEscapeString(id))
}

// RecommendationWatchResumeAction handles the HTMX resume control.
func (h *Handler) RecommendationWatchResumeAction(w http.ResponseWriter, r *http.Request) {
	if h.RecommendationStore == nil {
		http.Error(w, "recommendations unavailable", http.StatusServiceUnavailable)
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if err := h.RecommendationStore.ResumeWatch(r.Context(), id, time.Now().UTC()); err != nil {
		http.Error(w, "resume failed", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprintf(w, `<tr data-watch-id="%s" data-watch-state="active"><td colspan="7">Watch resumed.</td></tr>`, template.HTMLEscapeString(id))
}

// RecommendationWatchSilenceAction handles the HTMX silence control.
func (h *Handler) RecommendationWatchSilenceAction(w http.ResponseWriter, r *http.Request) {
	if h.RecommendationStore == nil {
		http.Error(w, "recommendations unavailable", http.StatusServiceUnavailable)
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	hours, err := strconv.Atoi(r.FormValue("hours"))
	if err != nil || hours < 1 || hours > 24*30 {
		http.Error(w, "hours must be between 1 and 720", http.StatusBadRequest)
		return
	}
	now := time.Now().UTC()
	until := now.Add(time.Duration(hours) * time.Hour)
	if err := h.RecommendationStore.SilenceWatch(r.Context(), id, until, now); err != nil {
		http.Error(w, "silence failed", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprintf(w, `<tr data-watch-id="%s" data-watch-state="silenced"><td colspan="7">Silenced until %s.</td></tr>`, template.HTMLEscapeString(id), until.Format(time.RFC3339))
}

// RecommendationWatchDeleteAction handles the HTMX confirmed delete.
func (h *Handler) RecommendationWatchDeleteAction(w http.ResponseWriter, r *http.Request) {
	if h.RecommendationStore == nil {
		http.Error(w, "recommendations unavailable", http.StatusServiceUnavailable)
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	confirm := r.URL.Query().Get("confirm")
	if confirm != "yes" {
		http.Error(w, "confirm=yes is required", http.StatusBadRequest)
		return
	}
	if err := h.RecommendationStore.DeleteWatch(r.Context(), id, time.Now().UTC()); err != nil {
		http.Error(w, "delete failed", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprintf(w, `<tr data-watch-id="%s" data-watch-state="deleted"><td colspan="7">Watch deleted.</td></tr>`, template.HTMLEscapeString(id))
}

func watchConsentSummary(watch recstore.WatchRecord) string {
	current := watch.Consent.Current
	parts := []string{
		fmt.Sprintf("rate %d/%ds", current.MaxAlerts, current.WindowSeconds),
		fmt.Sprintf("precision %s", current.Precision),
	}
	if len(current.Sources) > 0 {
		parts = append(parts, "sources "+strings.Join(current.Sources, ","))
	}
	if current.SponsoredAllowed {
		parts = append(parts, "sponsored allowed")
	}
	return strings.Join(parts, "; ")
}

func renderTime(t *time.Time) string {
	if t == nil {
		return "—"
	}
	return t.Format(time.RFC3339)
}

func watchFormFields(existing *recstore.WatchRecord) string {
	name, kind, precision, delivery, queue := "", "topic_keyword", "neighborhood", "telegram", "queue"
	maxAlerts, window, cooldown, freshness := 3, 3600, 86400, 86400
	if existing != nil {
		name = existing.Name
		kind = existing.Kind
		precision = existing.LocationPrecision
		delivery = existing.DeliveryChannel
		queue = existing.QueuePolicy
		maxAlerts = existing.MaxAlertsPerWindow
		window = existing.AlertWindowSeconds
		cooldown = existing.CooldownSeconds
		freshness = existing.FreshnessSeconds
	}
	return fmt.Sprintf(`
<label>Name <input name="name" value="%s" required></label>
<label>Kind <select name="kind">%s</select></label>
<label>Location precision <select name="location_precision">%s</select></label>
<label>Delivery channel <select name="delivery_channel">%s</select></label>
<label>Queue policy <select name="queue_policy">%s</select></label>
<label>Max alerts per window <input name="max_alerts_per_window" type="number" min="1" max="50" value="%d" required></label>
<label>Alert window seconds <input name="alert_window_seconds" type="number" min="60" max="86400" value="%d" required></label>
<label>Cooldown seconds <input name="cooldown_seconds" type="number" min="0" max="2592000" value="%d"></label>
<label>Freshness seconds <input name="freshness_seconds" type="number" min="0" max="2592000" value="%d"></label>
<label>Enabled <input name="enabled" type="checkbox" checked></label>
`,
		template.HTMLEscapeString(name),
		renderOptions("topic_keyword,location_radius,trip_context,price_drop", kind),
		renderOptions("exact,neighborhood,city", precision),
		renderOptions("telegram,web,pwa", delivery),
		renderOptions("queue,summarize,drop", queue),
		maxAlerts, window, cooldown, freshness,
	)
}

func watchConsentFields(existing *recstore.WatchRecord) string {
	scopeJSON := "{}"
	sources := ""
	sponsored := false
	if existing != nil {
		sources = strings.Join(existing.Consent.Current.Sources, ",")
		sponsored = existing.Consent.Current.SponsoredAllowed
	}
	checked := ""
	if sponsored {
		checked = " checked"
	}
	return fmt.Sprintf(`
<fieldset><legend>Consent confirmations</legend>
<label><input type="checkbox" name="consent_confirmation_scope_named" value="true" required> I confirm the watch scope (kind, anchors, keywords) is what I asked for.</label>
<label><input type="checkbox" name="consent_confirmation_sources_named" value="true" required> I confirm the providers/sources this watch may use.</label>
<label><input type="checkbox" name="consent_confirmation_rate_limit_named" value="true" required> I confirm the rate limit is appropriate.</label>
<label><input type="checkbox" name="consent_confirmation_precision_named" value="true" required> I confirm the location precision policy.</label>
<label><input type="checkbox" name="consent_confirmation_delivery_named" value="true" required> I confirm the delivery channel.</label>
<label><input type="checkbox" name="consent_confirmation_constraints_named" value="true" required> I confirm hard constraints (allergies, exclusions).</label>
<label><input type="checkbox" name="consent_confirmation_sponsored_named" value="true" required> I have decided whether sponsored results are allowed (currently %t).</label>
</fieldset>
<input type="hidden" name="consent_scope" value="%s">
<input type="hidden" name="consent_sources" value="%s">
<input type="hidden" name="sponsored_allowed" value="%t"%s>
`, sponsored, template.HTMLEscapeString(scopeJSON), template.HTMLEscapeString(sources), sponsored, checked)
}

func renderOptions(csv, current string) string {
	parts := strings.Split(csv, ",")
	out := strings.Builder{}
	for _, value := range parts {
		selected := ""
		if value == current {
			selected = " selected"
		}
		out.WriteString(fmt.Sprintf(`<option value="%s"%s>%s</option>`, template.HTMLEscapeString(value), selected, template.HTMLEscapeString(value)))
	}
	return out.String()
}

// RecommendationWatchConsentRejectionResponse is reused by tests to validate
// the expected error envelope when consent confirmation is missing.
type RecommendationWatchConsentRejectionResponse struct {
	Code            string   `json:"code"`
	Reason          string   `json:"reason"`
	MissingFlags    []string `json:"missing_flags"`
	BroadenedFields []string `json:"broadened_fields"`
}

var _ = errors.New
var _ = policy.ConsentReasonCreate
