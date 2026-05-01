package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/smackerel/smackerel/internal/recommendation/policy"
	recstore "github.com/smackerel/smackerel/internal/recommendation/store"
)

// RecommendationWatchService is the persistence boundary used by the watch
// API. It is implemented by *recstore.Store; the interface keeps the handler
// testable with adversarial fakes.
type RecommendationWatchService interface {
	CreateWatch(ctx context.Context, input recstore.WatchInput, consent policy.ConsentRecord, now time.Time) (recstore.WatchRecord, error)
	UpdateWatchWithConsent(ctx context.Context, input recstore.WatchInput, consent policy.ConsentRecord, now time.Time) (recstore.WatchRecord, error)
	GetWatch(ctx context.Context, id string) (recstore.WatchRecord, error)
	ListWatches(ctx context.Context, actorUserID string) ([]recstore.WatchRecord, error)
	PauseWatch(ctx context.Context, id string, now time.Time) error
	ResumeWatch(ctx context.Context, id string, now time.Time) error
	SilenceWatch(ctx context.Context, id string, until time.Time, now time.Time) error
	DeleteWatch(ctx context.Context, id string, now time.Time) error
}

// RecommendationWatchTriggerEvaluator is the optional evaluator surface used
// by the synchronous trigger endpoint exposed for live-stack tests and manual
// admin operations.
type RecommendationWatchTriggerEvaluator interface {
	EvaluateWatchSync(ctx context.Context, watchID, triggerKind string, triggerContext map[string]any) (RecommendationWatchTriggerResult, error)
}

// RecommendationWatchTriggerResult is the renderer-safe summary returned by a
// synchronous trigger evaluation.
type RecommendationWatchTriggerResult struct {
	WatchRunID        string         `json:"watch_run_id"`
	Status            string         `json:"status"`
	DeliveryDecision  string         `json:"delivery_decision"`
	Delivered         int            `json:"delivered"`
	Withheld          int            `json:"withheld"`
	RawCandidates     int            `json:"raw_candidates"`
	WithheldReasons   map[string]int `json:"withheld_reasons"`
	RecommendationIDs []string       `json:"recommendation_ids"`
}

// RecommendationWatchHandlers exposes the watch CRUD/control surface for spec 039 Scope 4.
type RecommendationWatchHandlers struct {
	service   RecommendationWatchService
	evaluator RecommendationWatchTriggerEvaluator
}

// SetTriggerEvaluator installs the synchronous trigger evaluator. Optional;
// when nil, the trigger endpoint returns 503 Service Unavailable.
func (h *RecommendationWatchHandlers) SetTriggerEvaluator(eval RecommendationWatchTriggerEvaluator) {
	h.evaluator = eval
}

// NewRecommendationWatchHandlers returns a wired handlers instance. Panics if
// the service is nil — a nil service would make every request fail and that
// is a configuration bug, not a runtime condition to silence.
func NewRecommendationWatchHandlers(service RecommendationWatchService) *RecommendationWatchHandlers {
	if service == nil {
		panic("api: recommendation watch service is required")
	}
	return &RecommendationWatchHandlers{service: service}
}

type watchRequest struct {
	Name               string            `json:"name"`
	Kind               string            `json:"kind"`
	Enabled            bool              `json:"enabled"`
	Scope              map[string]any    `json:"scope"`
	Filters            map[string]any    `json:"filters"`
	AllowedSources     []string          `json:"allowed_sources"`
	Schedule           map[string]any    `json:"schedule"`
	MaxAlertsPerWindow int               `json:"max_alerts_per_window"`
	AlertWindowSeconds int               `json:"alert_window_seconds"`
	CooldownSeconds    int               `json:"cooldown_seconds"`
	QuietHours         map[string]any    `json:"quiet_hours"`
	LocationPrecision  string            `json:"location_precision"`
	DeliveryChannel    string            `json:"delivery_channel"`
	QueuePolicy        string            `json:"queue_policy"`
	FreshnessSeconds   int               `json:"freshness_seconds"`
	Consent            consentBlock      `json:"consent"`
	Confirmation       confirmationBlock `json:"consent_confirmation"`
}

type consentBlock struct {
	Scope            map[string]any `json:"scope"`
	Sources          []string       `json:"sources"`
	DeliveryChannel  string         `json:"delivery_channel"`
	MaxAlerts        int            `json:"max_alerts"`
	WindowSeconds    int            `json:"window_seconds"`
	Precision        string         `json:"precision"`
	HardConstraints  []string       `json:"hard_constraints"`
	SponsoredAllowed bool           `json:"sponsored_allowed"`
}

type confirmationBlock struct {
	ScopeNamed       bool `json:"scope_named"`
	SourcesNamed     bool `json:"sources_named"`
	RateLimitNamed   bool `json:"rate_limit_named"`
	PrecisionNamed   bool `json:"precision_named"`
	DeliveryNamed    bool `json:"delivery_named"`
	ConstraintsNamed bool `json:"constraints_named"`
	SponsoredNamed   bool `json:"sponsored_named"`
}

type silenceRequest struct {
	UntilSeconds int `json:"until_seconds"`
}

// CreateWatch handles POST /api/recommendations/watches.
func (h *RecommendationWatchHandlers) CreateWatch(w http.ResponseWriter, r *http.Request) {
	var req watchRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be valid watch JSON")
		return
	}
	if errCode, msg := validateWatchRequest(req); errCode != "" {
		writeError(w, http.StatusBadRequest, errCode, msg)
		return
	}

	now := time.Now().UTC()
	draft := req.Consent.toNamedValues(req)

	// Creation is always a CONSENT_REQUIRED gate at first attempt. The caller
	// MUST send all confirmation flags so the user has explicitly named scope,
	// sources, rate limit, precision, delivery, hard constraints, and sponsored.
	decision := policy.EvaluateConsent(policy.ConsentRecord{}, draft, false, req.Enabled)
	if err := policy.CheckConfirmation(decision, req.Confirmation.toPolicy()); err != nil {
		respondConsentRequired(w, err)
		return
	}

	consent := policy.ApplyRevision(policy.ConsentRecord{}, draft, decision.Reason, now)
	record, err := h.service.CreateWatch(r.Context(), req.toInput(""), consent, now)
	if err != nil {
		writeError(w, http.StatusBadRequest, "watch_create_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, watchResponse(record))
}

// UpdateWatch handles PUT /api/recommendations/watches/{id}.
func (h *RecommendationWatchHandlers) UpdateWatch(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing_watch_id", "watch id is required")
		return
	}
	current, err := h.service.GetWatch(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "watch_not_found", "watch not found")
		return
	}
	var req watchRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be valid watch JSON")
		return
	}
	if errCode, msg := validateWatchRequest(req); errCode != "" {
		writeError(w, http.StatusBadRequest, errCode, msg)
		return
	}
	now := time.Now().UTC()
	draft := req.Consent.toNamedValues(req)
	decision := policy.EvaluateConsent(current.Consent, draft, current.Enabled, req.Enabled)
	if err := policy.CheckConfirmation(decision, req.Confirmation.toPolicy()); err != nil {
		respondConsentRequired(w, err)
		return
	}
	updatedConsent := policy.ApplyRevision(current.Consent, draft, decision.Reason, now)
	record, err := h.service.UpdateWatchWithConsent(r.Context(), req.toInput(id), updatedConsent, now)
	if err != nil {
		writeError(w, http.StatusBadRequest, "watch_update_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, watchResponse(record))
}

// GetWatch handles GET /api/recommendations/watches/{id}.
func (h *RecommendationWatchHandlers) GetWatch(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing_watch_id", "watch id is required")
		return
	}
	record, err := h.service.GetWatch(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "watch_not_found", "watch not found")
		return
	}
	writeJSON(w, http.StatusOK, watchResponse(record))
}

// ListWatches handles GET /api/recommendations/watches.
func (h *RecommendationWatchHandlers) ListWatches(w http.ResponseWriter, r *http.Request) {
	records, err := h.service.ListWatches(r.Context(), "local")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "watch_list_failed", "failed to list watches")
		return
	}
	out := make([]map[string]any, 0, len(records))
	for _, record := range records {
		out = append(out, watchResponse(record))
	}
	writeJSON(w, http.StatusOK, map[string]any{"watches": out})
}

// PauseWatch handles POST /api/recommendations/watches/{id}/pause.
func (h *RecommendationWatchHandlers) PauseWatch(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing_watch_id", "watch id is required")
		return
	}
	if err := h.service.PauseWatch(r.Context(), id, time.Now().UTC()); err != nil {
		writeError(w, http.StatusNotFound, "watch_not_found", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "paused", "watch_id": id})
}

// ResumeWatch handles POST /api/recommendations/watches/{id}/resume.
func (h *RecommendationWatchHandlers) ResumeWatch(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing_watch_id", "watch id is required")
		return
	}
	if err := h.service.ResumeWatch(r.Context(), id, time.Now().UTC()); err != nil {
		writeError(w, http.StatusNotFound, "watch_not_found", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "resumed", "watch_id": id})
}

// SilenceWatch handles POST /api/recommendations/watches/{id}/silence.
func (h *RecommendationWatchHandlers) SilenceWatch(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing_watch_id", "watch id is required")
		return
	}
	var req silenceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must include until_seconds")
		return
	}
	if req.UntilSeconds < 60 || req.UntilSeconds > 60*60*24*90 {
		writeError(w, http.StatusBadRequest, "invalid_until_seconds", "until_seconds must be between 60 and 7776000")
		return
	}
	now := time.Now().UTC()
	until := now.Add(time.Duration(req.UntilSeconds) * time.Second)
	if err := h.service.SilenceWatch(r.Context(), id, until, now); err != nil {
		writeError(w, http.StatusNotFound, "watch_not_found", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "silenced", "watch_id": id, "silence_until": until})
}

// DeleteWatch handles DELETE /api/recommendations/watches/{id}.
func (h *RecommendationWatchHandlers) DeleteWatch(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing_watch_id", "watch id is required")
		return
	}
	confirm := r.URL.Query().Get("confirm")
	if confirm != "yes" {
		writeError(w, http.StatusBadRequest, "delete_confirmation_required", "delete requires ?confirm=yes")
		return
	}
	if err := h.service.DeleteWatch(r.Context(), id, time.Now().UTC()); err != nil {
		writeError(w, http.StatusNotFound, "watch_not_found", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "deleted", "watch_id": id})
}

// TriggerWatch handles POST /api/recommendations/watches/{id}/trigger.
// The endpoint runs a synchronous evaluation against the live store and
// returns the run summary. Used by spec 039 Scope 4 e2e tests and admin
// operations to bypass the cron poller in deterministic scenarios.
func (h *RecommendationWatchHandlers) TriggerWatch(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing_watch_id", "watch id is required")
		return
	}
	if h.evaluator == nil {
		writeError(w, http.StatusServiceUnavailable, "evaluator_unavailable", "watch evaluator is not wired")
		return
	}
	var req struct {
		TriggerKind    string         `json:"trigger_kind"`
		TriggerContext map[string]any `json:"trigger_context"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be valid trigger JSON")
		return
	}
	if req.TriggerKind == "" {
		req.TriggerKind = "manual"
	}
	if req.TriggerContext == nil {
		req.TriggerContext = map[string]any{}
	}
	result, err := h.evaluator.EvaluateWatchSync(r.Context(), id, req.TriggerKind, req.TriggerContext)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "watch_trigger_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// validateWatchRequest enforces the BS-022 rule that scope/kind/precision are
// always explicitly named and the BS-018/BS-007/BS-009 enums for kind, queue
// policy, location precision, and delivery channel.
func validateWatchRequest(req watchRequest) (string, string) {
	if strings.TrimSpace(req.Name) == "" {
		return "invalid_name", "name is required"
	}
	switch req.Kind {
	case "location_radius", "topic_keyword", "trip_context", "price_drop":
	default:
		return "invalid_kind", "kind must be one of location_radius, topic_keyword, trip_context, price_drop"
	}
	switch req.LocationPrecision {
	case "exact", "neighborhood", "city":
	default:
		return "invalid_location_precision", "location_precision must be exact, neighborhood, or city"
	}
	switch req.DeliveryChannel {
	case "telegram", "web", "pwa":
	default:
		return "invalid_delivery_channel", "delivery_channel must be telegram, web, or pwa"
	}
	switch req.QueuePolicy {
	case "queue", "summarize", "drop":
	default:
		return "invalid_queue_policy", "queue_policy must be queue, summarize, or drop"
	}
	if req.MaxAlertsPerWindow < 1 || req.MaxAlertsPerWindow > 50 {
		return "invalid_max_alerts", "max_alerts_per_window must be between 1 and 50"
	}
	if req.AlertWindowSeconds < 60 || req.AlertWindowSeconds > 60*60*24 {
		return "invalid_alert_window", "alert_window_seconds must be between 60 and 86400"
	}
	if req.CooldownSeconds < 0 || req.CooldownSeconds > 60*60*24*30 {
		return "invalid_cooldown", "cooldown_seconds must be between 0 and 2592000"
	}
	if req.FreshnessSeconds < 0 || req.FreshnessSeconds > 60*60*24*30 {
		return "invalid_freshness", "freshness_seconds must be between 0 and 2592000"
	}
	return "", ""
}

func respondConsentRequired(w http.ResponseWriter, err error) {
	var consentErr *policy.ErrConsentRequired
	if !errors.As(err, &consentErr) {
		writeError(w, http.StatusInternalServerError, "consent_check_failed", err.Error())
		return
	}
	body := map[string]any{
		"code":             consentErr.Code,
		"reason":           consentErr.Reason,
		"missing_flags":    consentErr.MissingFlags,
		"broadened_fields": consentErr.BroadenedFields,
		"message":          "Consent confirmation is required before saving this watch",
	}
	writeJSON(w, http.StatusUnprocessableEntity, body)
}

func (b consentBlock) toNamedValues(req watchRequest) policy.ConsentNamedValues {
	scope := b.Scope
	if scope == nil {
		scope = req.Scope
	}
	delivery := b.DeliveryChannel
	if delivery == "" {
		delivery = req.DeliveryChannel
	}
	maxAlerts := b.MaxAlerts
	if maxAlerts == 0 {
		maxAlerts = req.MaxAlertsPerWindow
	}
	windowSeconds := b.WindowSeconds
	if windowSeconds == 0 {
		windowSeconds = req.AlertWindowSeconds
	}
	precision := b.Precision
	if precision == "" {
		precision = req.LocationPrecision
	}
	sources := b.Sources
	if sources == nil {
		sources = req.AllowedSources
	}
	hard := b.HardConstraints
	if hard == nil {
		hard = []string{}
	}
	return policy.ConsentNamedValues{
		Scope:            cloneAny(scope),
		Sources:          append([]string{}, sources...),
		DeliveryChannel:  delivery,
		MaxAlerts:        maxAlerts,
		WindowSeconds:    windowSeconds,
		Precision:        precision,
		HardConstraints:  append([]string{}, hard...),
		SponsoredAllowed: b.SponsoredAllowed,
	}
}

func (b confirmationBlock) toPolicy() policy.ConsentConfirmation {
	return policy.ConsentConfirmation{
		ScopeNamed:       b.ScopeNamed,
		SourcesNamed:     b.SourcesNamed,
		RateLimitNamed:   b.RateLimitNamed,
		PrecisionNamed:   b.PrecisionNamed,
		DeliveryNamed:    b.DeliveryNamed,
		ConstraintsNamed: b.ConstraintsNamed,
		SponsoredNamed:   b.SponsoredNamed,
	}
}

func (req watchRequest) toInput(id string) recstore.WatchInput {
	scope := req.Scope
	if scope == nil {
		scope = map[string]any{}
	}
	filters := req.Filters
	if filters == nil {
		filters = map[string]any{}
	}
	schedule := req.Schedule
	if schedule == nil {
		schedule = map[string]any{}
	}
	quiet := req.QuietHours
	if quiet == nil {
		quiet = map[string]any{}
	}
	allowed := req.AllowedSources
	if allowed == nil {
		allowed = []string{}
	}
	return recstore.WatchInput{
		ID:                 id,
		ActorUserID:        "local",
		Name:               req.Name,
		Kind:               req.Kind,
		Enabled:            req.Enabled,
		Scope:              scope,
		Filters:            filters,
		AllowedSources:     allowed,
		Schedule:           schedule,
		MaxAlertsPerWindow: req.MaxAlertsPerWindow,
		AlertWindowSeconds: req.AlertWindowSeconds,
		CooldownSeconds:    req.CooldownSeconds,
		QuietHours:         quiet,
		LocationPrecision:  req.LocationPrecision,
		DeliveryChannel:    req.DeliveryChannel,
		QueuePolicy:        req.QueuePolicy,
		FreshnessSeconds:   req.FreshnessSeconds,
	}
}

func watchResponse(record recstore.WatchRecord) map[string]any {
	return map[string]any{
		"id":                    record.ID,
		"actor_user_id":         record.ActorUserID,
		"name":                  record.Name,
		"kind":                  record.Kind,
		"enabled":               record.Enabled,
		"scope":                 record.Scope,
		"filters":               record.Filters,
		"allowed_sources":       record.AllowedSources,
		"schedule":              record.Schedule,
		"max_alerts_per_window": record.MaxAlertsPerWindow,
		"alert_window_seconds":  record.AlertWindowSeconds,
		"cooldown_seconds":      record.CooldownSeconds,
		"quiet_hours":           record.QuietHours,
		"location_precision":    record.LocationPrecision,
		"delivery_channel":      record.DeliveryChannel,
		"queue_policy":          record.QueuePolicy,
		"freshness_seconds":     record.FreshnessSeconds,
		"consent":               record.Consent,
		"last_run_at":           record.LastRunAt,
		"next_due_at":           record.NextDueAt,
		"silence_until":         record.SilenceUntil,
		"created_at":            record.CreatedAt,
		"updated_at":            record.UpdatedAt,
		"deleted_at":            record.DeletedAt,
	}
}

func cloneAny(values map[string]any) map[string]any {
	if values == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

// ensure unused imports stay needed.
var _ = fmt.Sprintf
