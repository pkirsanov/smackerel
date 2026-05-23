package api

import (
	"context"
	"net/http"
	"time"

	"github.com/smackerel/smackerel/internal/notification"
)

type NotificationSourceStatusProvider interface {
	ListSourceStatuses(ctx context.Context) ([]notification.SourceStatus, error)
}

type NotificationHandlers struct {
	sources NotificationSourceStatusProvider
	store   *notification.Store
	service *notification.Service
}

func NewNotificationHandlers(store *notification.Store, service *notification.Service) *NotificationHandlers {
	if store == nil {
		panic("api: notification store is required")
	}
	if service == nil {
		panic("api: notification service is required")
	}
	return &NotificationHandlers{sources: store, store: store, service: service}
}

type notificationSourcesResponse struct {
	Sources []notificationSourceStatusResponse `json:"sources"`
}

type notificationSourceStatusResponse struct {
	SourceType            string            `json:"source_type"`
	SourceInstanceID      string            `json:"source_instance_id"`
	SourceForm            string            `json:"source_form"`
	Enabled               bool              `json:"enabled"`
	ConfigHash            string            `json:"config_hash"`
	SecretRefNames        []string          `json:"secret_ref_names"`
	RedactedMetadata      map[string]string `json:"redacted_metadata"`
	HealthState           string            `json:"health_state"`
	LastEventAt           *time.Time        `json:"last_event_at,omitempty"`
	LastSuccessfulCheckAt *time.Time        `json:"last_successful_check_at,omitempty"`
	RetryCount            int               `json:"retry_count"`
	LastErrorKind         string            `json:"last_error_kind,omitempty"`
	LastErrorRedacted     string            `json:"last_error_redacted,omitempty"`
	HealthObservedAt      *time.Time        `json:"health_observed_at,omitempty"`
}

func (h *NotificationHandlers) ListSources(w http.ResponseWriter, r *http.Request) {
	statuses, err := h.sources.ListSourceStatuses(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "notification_sources_unavailable", "failed to load notification source statuses")
		return
	}
	response := notificationSourcesResponse{Sources: make([]notificationSourceStatusResponse, 0, len(statuses))}
	for _, status := range statuses {
		var observedAt *time.Time
		if !status.Health.ObservedAt.IsZero() {
			observedAtValue := status.Health.ObservedAt
			observedAt = &observedAtValue
		}
		response.Sources = append(response.Sources, notificationSourceStatusResponse{
			SourceType:            status.Config.SourceType,
			SourceInstanceID:      status.Config.SourceInstanceID,
			SourceForm:            string(status.Config.SourceForm),
			Enabled:               status.Config.Enabled,
			ConfigHash:            status.Config.ConfigHash,
			SecretRefNames:        append([]string(nil), status.Config.SecretRefNames...),
			RedactedMetadata:      cloneNotificationMetadata(status.Config.RedactedMetadata),
			HealthState:           string(status.Health.State),
			LastEventAt:           status.Health.LastEventAt,
			LastSuccessfulCheckAt: status.Health.LastSuccessfulCheckAt,
			RetryCount:            status.Health.RetryCount,
			LastErrorKind:         status.Health.LastErrorKind,
			LastErrorRedacted:     status.Health.LastErrorRedacted,
			HealthObservedAt:      observedAt,
		})
	}
	writeJSON(w, http.StatusOK, response)
}

func cloneNotificationMetadata(values map[string]string) map[string]string {
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
