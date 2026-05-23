package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/smackerel/smackerel/internal/notification"
)

type manualIngestRequest struct {
	SourceType       string            `json:"source_type"`
	SourceInstanceID string            `json:"source_instance_id"`
	Title            string            `json:"title"`
	Body             string            `json:"body"`
	Severity         string            `json:"severity"`
	Subject          string            `json:"subject"`
	Service          string            `json:"service"`
	Domain           string            `json:"domain"`
	Intent           string            `json:"intent"`
	DeliveryMetadata map[string]string `json:"delivery_metadata"`
	SourceFields     map[string]string `json:"source_specific_fields"`
	LoopMetadata     map[string]string `json:"loop_metadata"`
}

type snoozeIncidentRequest struct {
	DurationMinutes int    `json:"duration_minutes"`
	Reason          string `json:"reason"`
}

type approvalDecisionRequest struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

func (h *NotificationHandlers) ManualIngest(w http.ResponseWriter, r *http.Request) {
	var req manualIngestRequest
	if !decodeJSONBody(w, r, &req, "invalid_notification_ingest", "request body must be valid notification JSON") {
		return
	}
	if req.SourceType == "" || req.SourceInstanceID == "" || req.Body == "" {
		writeError(w, http.StatusBadRequest, "invalid_notification_ingest", "source_type, source_instance_id, and body are required")
		return
	}
	now := time.Now().UTC()
	enabled := true
	if err := h.store.EnsureSourceInstance(r.Context(), notification.SourceInstanceConfig{SourceType: req.SourceType, SourceInstanceID: req.SourceInstanceID, SourceForm: notification.SourceFormManual, Enabled: &enabled, ConfigHash: "sha256:" + req.SourceInstanceID, SecretRefNames: []string{"MANUAL_INGEST_AUTH_CONTEXT"}, RedactedMetadata: map[string]string{"actor": "authenticated_operator"}}, now); err != nil {
		writeError(w, http.StatusBadRequest, "notification_source_invalid", err.Error())
		return
	}
	metadata := req.DeliveryMetadata
	if metadata == nil {
		metadata = map[string]string{"actor": "authenticated_operator"}
	}
	fields := req.SourceFields
	if fields == nil {
		fields = map[string]string{}
	}
	hints := map[string]string{"title": req.Title, "body": req.Body, "severity": req.Severity, "subject": req.Subject, "service": req.Service, "domain": req.Domain, "intent": req.Intent}
	result, err := h.service.Process(r.Context(), notification.SourceEventEnvelope{SourceType: req.SourceType, SourceInstanceID: req.SourceInstanceID, SourceForm: notification.SourceFormManual, ObservedAt: now, RawPayloadKind: notification.RawPayloadKindText, RawPayload: []byte(req.Body), DeliveryMetadata: metadata, SourceSpecificFields: fields, MappingHints: hints, LoopMetadata: req.LoopMetadata}, now)
	if err != nil {
		writeError(w, http.StatusBadRequest, "notification_ingest_failed", err.Error())
		return
	}
	response := map[string]any{"receipt": result.Receipt, "notification_id": result.Notification.ID, "incident_id": result.Incident.ID, "decision_id": result.Decision.ID}
	if result.Approval != nil {
		response["approval_id"] = result.Approval.ID
	}
	writeJSON(w, http.StatusCreated, response)
}

func (h *NotificationHandlers) ListEvents(w http.ResponseWriter, r *http.Request) {
	events, err := h.store.ListNotifications(r.Context(), 100)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "notification_events_unavailable", "failed to load notification events")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}

func (h *NotificationHandlers) GetEvent(w http.ResponseWriter, r *http.Request) {
	detail, err := h.store.GetEventDetail(r.Context(), chi.URLParam(r, "event_id"))
	if err != nil {
		status := http.StatusInternalServerError
		code := "notification_event_unavailable"
		message := "failed to load notification event"
		if errors.Is(err, pgx.ErrNoRows) {
			status = http.StatusNotFound
			code = "notification_event_not_found"
			message = "notification event not found"
		}
		writeError(w, status, code, message)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (h *NotificationHandlers) ListIncidents(w http.ResponseWriter, r *http.Request) {
	incidents, err := h.store.ListIncidents(r.Context(), 100)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "notification_incidents_unavailable", "failed to load notification incidents")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"incidents": incidents})
}

func (h *NotificationHandlers) GetIncident(w http.ResponseWriter, r *http.Request) {
	incident, err := h.store.GetIncident(r.Context(), chi.URLParam(r, "incident_id"))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "notification_incident_not_found", "notification incident not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "notification_incident_unavailable", "failed to load notification incident")
		return
	}
	writeJSON(w, http.StatusOK, incident)
}

func (h *NotificationHandlers) ListSuppressions(w http.ResponseWriter, r *http.Request) {
	suppressions, err := h.store.ListSuppressions(r.Context(), 100)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "notification_suppressions_unavailable", "failed to load notification suppressions")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"suppressions": suppressions})
}

func (h *NotificationHandlers) ListOutputs(w http.ResponseWriter, r *http.Request) {
	deliveries, err := h.store.ListDeliveries(r.Context(), 100)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "notification_outputs_unavailable", "failed to load notification outputs")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"outputs": deliveries})
}

func (h *NotificationHandlers) Status(w http.ResponseWriter, r *http.Request) {
	summary, err := h.store.StatusSummary(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "notification_status_unavailable", "failed to load notification status")
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (h *NotificationHandlers) Summary(w http.ResponseWriter, r *http.Request) {
	summary, err := h.store.StatusSummary(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "notification_summary_unavailable", "failed to load notification summary")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"summary": summary, "message": "notification intelligence summary emphasizes incidents, suppressed noise, unresolved items, and output attempts"})
}

func (h *NotificationHandlers) SnoozeIncident(w http.ResponseWriter, r *http.Request) {
	incidentID := chi.URLParam(r, "incident_id")
	if _, err := h.store.GetIncident(r.Context(), incidentID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "notification_incident_not_found", "notification incident not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "notification_incident_unavailable", "failed to load notification incident")
		return
	}
	var req snoozeIncidentRequest
	if !decodeJSONBody(w, r, &req, "invalid_notification_snooze", "request body must include duration_minutes and reason") {
		return
	}
	if req.DurationMinutes < 1 || strings.TrimSpace(req.Reason) == "" {
		writeError(w, http.StatusBadRequest, "invalid_notification_snooze", "duration_minutes must be positive and reason is required")
		return
	}
	now := time.Now().UTC()
	expiresAt := now.Add(time.Duration(req.DurationMinutes) * time.Minute)
	suppression, err := h.store.CreateSuppression(r.Context(), notification.Suppression{IncidentID: incidentID, Kind: notification.SuppressionUserPreference, Scope: map[string]any{"incident_id": incidentID, "duration_minutes": req.DurationMinutes}, Reason: strings.TrimSpace(req.Reason), StartsAt: now, ExpiresAt: &expiresAt, CreatedAt: now})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "notification_snooze_failed", "failed to record notification snooze")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"suppression": suppression})
}

func (h *NotificationHandlers) ListQuietWindows(w http.ResponseWriter, r *http.Request) {
	quietWindows, err := h.store.ListQuietWindows(r.Context(), 100)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "notification_quiet_windows_unavailable", "failed to load notification quiet windows")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"quiet_windows": quietWindows})
}

func (h *NotificationHandlers) GetApproval(w http.ResponseWriter, r *http.Request) {
	detail, err := h.store.GetApprovalDetail(r.Context(), chi.URLParam(r, "approval_id"))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "notification_approval_not_found", "notification approval not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "notification_approval_unavailable", "failed to load notification approval")
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (h *NotificationHandlers) RecordApprovalDecision(w http.ResponseWriter, r *http.Request) {
	var req approvalDecisionRequest
	if !decodeJSONBody(w, r, &req, "invalid_notification_approval_decision", "request body must include decision and reason") {
		return
	}
	if strings.TrimSpace(req.Decision) == "" || strings.TrimSpace(req.Reason) == "" {
		writeError(w, http.StatusBadRequest, "invalid_notification_approval_decision", "decision and reason are required")
		return
	}
	detail, err := h.store.RecordApprovalDecision(r.Context(), notification.ApprovalDecision{ApprovalRequestID: chi.URLParam(r, "approval_id"), Decision: strings.TrimSpace(req.Decision), ActorKind: notification.ActorOperator, ActorRef: "authenticated_operator", Channel: "api", Reason: strings.TrimSpace(req.Reason), CreatedAt: time.Now().UTC()})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "notification_approval_not_found", "notification approval not found")
			return
		}
		writeError(w, http.StatusBadRequest, "notification_approval_decision_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, detail)
}
