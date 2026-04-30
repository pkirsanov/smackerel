package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smackerel/smackerel/internal/config"
	"github.com/smackerel/smackerel/internal/connector"
	photolib "github.com/smackerel/smackerel/internal/connector/photos"
	"github.com/smackerel/smackerel/internal/connector/photos/adapters/immich"
)

type PhotosHandlers struct {
	store   *photolib.Store
	config  config.PhotosConfig
	scanner *photolib.Scanner
}

type photoConnectorResponse struct {
	ConnectorID          string                                           `json:"connector_id,omitempty"`
	Provider             string                                           `json:"provider"`
	DisplayName          string                                           `json:"display_name"`
	Status               string                                           `json:"status"`
	Enabled              bool                                             `json:"enabled"`
	Capabilities         []string                                         `json:"capabilities"`
	CapabilityReport     map[photolib.Capability]photolib.CapabilityEntry `json:"capability_report,omitempty"`
	SupportedAPIVersions []string                                         `json:"supported_api_versions,omitempty"`
	PollIntervalSeconds  int                                              `json:"poll_interval_seconds,omitempty"`
	Scope                photolib.Scope                                   `json:"scope,omitempty"`
	Progress             photolib.ScanProgress                            `json:"progress"`
	Skips                []photolib.SkipEntry                             `json:"skips"`
	LastSyncAt           *time.Time                                       `json:"last_sync_at,omitempty"`
	MonitoringLagSeconds int                                              `json:"monitoring_lag_seconds"`
}

type photoConnectorRequest struct {
	Provider    string            `json:"provider"`
	ConnectorID string            `json:"connector_id,omitempty"`
	Config      map[string]string `json:"config"`
	Scope       photolib.Scope    `json:"scope"`
}

type photoSummary struct {
	PhotoID         string                          `json:"photo_id"`
	ArtifactID      string                          `json:"artifact_id"`
	Provider        string                          `json:"provider"`
	ProviderRef     string                          `json:"provider_ref"`
	Filename        string                          `json:"filename"`
	CapturedAt      *time.Time                      `json:"captured_at,omitempty"`
	Albums          []string                        `json:"albums"`
	MediaRole       photolib.MediaRole              `json:"media_role"`
	LifecycleState  string                          `json:"lifecycle_state"`
	Sensitivity     photolib.SensitivityLevel       `json:"sensitivity"`
	SensitivityTags []string                        `json:"sensitivity_labels"`
	MatchConfidence float64                         `json:"match_confidence"`
	Classification  photolib.ClassificationDecision `json:"classification"`
	Preview         map[string]any                  `json:"preview"`
}

type photoDetail struct {
	PhotoID                  string                          `json:"photo_id"`
	ArtifactID               string                          `json:"artifact_id"`
	ConnectorID              string                          `json:"connector_id"`
	Provider                 string                          `json:"provider"`
	ProviderRef              string                          `json:"provider_ref"`
	ProviderMediaKind        string                          `json:"provider_media_kind"`
	MediaRole                photolib.MediaRole              `json:"media_role"`
	MIMEType                 string                          `json:"mime_type"`
	Bytes                    *int64                          `json:"bytes,omitempty"`
	BytesEstimated           bool                            `json:"bytes_estimated"`
	Filename                 string                          `json:"filename"`
	CapturedAt               *time.Time                      `json:"captured_at,omitempty"`
	UploadedAt               *time.Time                      `json:"uploaded_at,omitempty"`
	Albums                   []string                        `json:"albums"`
	Tags                     []string                        `json:"tags"`
	LifecycleState           string                          `json:"lifecycle_state"`
	Sensitivity              photolib.SensitivityLevel       `json:"sensitivity"`
	SensitivityLabels        []string                        `json:"sensitivity_labels"`
	Classification           json.RawMessage                 `json:"classification"`
	ClassificationDecision   photolib.ClassificationDecision `json:"classification_view"`
	ClassificationConfidence *float64                        `json:"classification_confidence,omitempty"`
	RawProvider              json.RawMessage                 `json:"raw_provider"`
}

func NewPhotosHandlers(store *photolib.Store, cfg config.PhotosConfig) *PhotosHandlers {
	return &PhotosHandlers{
		store:   store,
		config:  cfg,
		scanner: photolib.NewScanner(store, photolib.ScannerConfig{MaxFileSizeBytes: cfg.Scan.MaxFileSizeBytes}),
	}
}

func (h *PhotosHandlers) ListConnectors(w http.ResponseWriter, r *http.Request) {
	connectors := []photoConnectorResponse{}
	if h.store != nil {
		states, err := h.store.ListConnectorStates(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "photo_connectors_failed", "failed to list photo connectors")
			return
		}
		for _, state := range states {
			connectors = append(connectors, photoConnectorFromState(state))
		}
	}
	if !hasProvider(connectors, "immich") {
		connectors = append(connectors, photoConnectorResponse{
			Provider:             "immich",
			DisplayName:          "Immich",
			Status:               statusFromConfig(h.config.Enabled && h.config.Providers.Immich.Enabled),
			Enabled:              h.config.Enabled && h.config.Providers.Immich.Enabled,
			Capabilities:         []string{"read", "monitor", "upload", "write_album", "write_tag", "write_favorite", "faces_read"},
			SupportedAPIVersions: h.config.Providers.Immich.SupportedAPIVersions,
			PollIntervalSeconds:  h.config.Providers.Immich.PollIntervalSeconds,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"connectors": connectors})
}

func (h *PhotosHandlers) TestConnector(w http.ResponseWriter, r *http.Request) {
	request, ok := decodePhotoConnectorRequest(w, r)
	if !ok {
		return
	}
	client, config, err := h.immichClientFromRequest(request)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_photo_connector", err.Error())
		return
	}
	report, err := client.ProbeCapabilities(r.Context(), config)
	if err != nil {
		writeError(w, http.StatusBadGateway, "photo_provider_probe_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "capabilities": report})
}

func (h *PhotosHandlers) Connect(w http.ResponseWriter, r *http.Request) {
	if h.store == nil || h.scanner == nil {
		writeError(w, http.StatusServiceUnavailable, "photos_store_unavailable", "photo store is unavailable")
		return
	}
	request, ok := decodePhotoConnectorRequest(w, r)
	if !ok {
		return
	}
	client, config, err := h.immichClientFromRequest(request)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_photo_connector", err.Error())
		return
	}
	if err := client.Connect(r.Context(), config); err != nil {
		writeError(w, http.StatusBadGateway, "photo_provider_connect_failed", err.Error())
		return
	}
	connectorID := request.ConnectorID
	if strings.TrimSpace(connectorID) == "" {
		connectorID = "photos-immich-" + uuid.NewString()
	}
	result, err := h.scanner.Scan(r.Context(), client, connectorID, request.Scope)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "photo_scan_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"connector_id": connectorID, "result": result})
}

func (h *PhotosHandlers) GetConnector(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "photos_store_unavailable", "photo store is unavailable")
		return
	}
	connectorID := chi.URLParam(r, "id")
	state, err := h.store.GetConnectorState(r.Context(), connectorID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "photo_connector_not_found", "photo connector not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "photo_connector_lookup_failed", "failed to look up photo connector")
		return
	}
	writeJSON(w, http.StatusOK, photoConnectorFromState(*state))
}

func (h *PhotosHandlers) Search(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "photos_store_unavailable", "photo store is unavailable")
		return
	}
	query := r.URL.Query().Get("q")
	limit := parsePhotoLimit(r.URL.Query().Get("limit"))
	results, err := h.store.Search(r.Context(), query, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "photo_search_failed", "failed to search photos")
		return
	}
	summaries := make([]photoSummary, 0, len(results))
	for _, result := range results {
		summaries = append(summaries, photoSummaryFromRecord(result.PhotoRecord, result.MatchConfidence))
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": summaries, "total": len(summaries)})
}

func (h *PhotosHandlers) GetPhoto(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "photos_store_unavailable", "photo store is unavailable")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_photo_id", "photo id must be a UUID")
		return
	}
	record, err := h.store.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "photo_not_found", "photo not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "photo_lookup_failed", "failed to look up photo")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"photo": photoDetailFromRecord(*record)})
}

func (h *PhotosHandlers) Preview(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func (h *PhotosHandlers) immichClientFromRequest(request photoConnectorRequest) (*immich.Client, connector.ConnectorConfig, error) {
	if request.Provider != "" && request.Provider != "immich" {
		return nil, connector.ConnectorConfig{}, fmt.Errorf("only immich photo connectors are supported in this scope")
	}
	baseURL := request.Config["base_url"]
	apiKey := request.Config["api_key"]
	if strings.TrimSpace(baseURL) == "" {
		baseURL = h.config.Providers.Immich.BaseURL
	}
	if strings.TrimSpace(apiKey) == "" {
		apiKey = h.config.Providers.Immich.APIKey
	}
	config := connector.ConnectorConfig{
		AuthType:    "api_key",
		Credentials: map[string]string{"api_key": apiKey},
		SourceConfig: map[string]any{
			"base_url": baseURL,
		},
		Enabled: true,
	}
	return immich.NewClient(http.DefaultClient), config, nil
}

func decodePhotoConnectorRequest(w http.ResponseWriter, r *http.Request) (photoConnectorRequest, bool) {
	defer r.Body.Close()
	var request photoConnectorRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be JSON")
		return photoConnectorRequest{}, false
	}
	if request.Provider == "" {
		request.Provider = "immich"
	}
	if request.Config == nil {
		request.Config = map[string]string{}
	}
	return request, true
}

func photoConnectorFromState(state photolib.ConnectorState) photoConnectorResponse {
	return photoConnectorResponse{
		ConnectorID:          state.ConnectorID,
		Provider:             state.Provider,
		DisplayName:          displayNameForPhotoProvider(state.Provider),
		Status:               state.Status,
		Enabled:              true,
		Capabilities:         capabilityNames(state.Capabilities.Capabilities),
		CapabilityReport:     state.Capabilities.Capabilities,
		PollIntervalSeconds:  0,
		Scope:                state.Scope,
		Progress:             state.Progress,
		Skips:                state.Skips,
		LastSyncAt:           state.LastSyncAt,
		MonitoringLagSeconds: state.MonitoringLagSeconds,
	}
}

func photoSummaryFromRecord(record photolib.PhotoRecord, matchConfidence float64) photoSummary {
	classification := decodeClassification(record.Classification)
	return photoSummary{
		PhotoID:         record.ID.String(),
		ArtifactID:      record.ArtifactID,
		Provider:        record.Provider,
		ProviderRef:     record.ProviderRef,
		Filename:        record.Filename,
		CapturedAt:      record.CapturedAt,
		Albums:          nonNilStringSlice(record.Albums),
		MediaRole:       record.MediaRole,
		LifecycleState:  record.LifecycleState,
		Sensitivity:     record.Sensitivity,
		SensitivityTags: nonNilStringSlice(record.SensitivityLabels),
		MatchConfidence: matchConfidence,
		Classification:  classification,
		Preview: map[string]any{
			"available":       true,
			"requires_reveal": record.Sensitivity != photolib.SensitivityNone,
			"url":             "/v1/photos/" + record.ID.String() + "/preview?size=thumb",
		},
	}
}

func photoDetailFromRecord(record photolib.PhotoRecord) photoDetail {
	return photoDetail{
		PhotoID:                  record.ID.String(),
		ArtifactID:               record.ArtifactID,
		ConnectorID:              record.ConnectorID,
		Provider:                 record.Provider,
		ProviderRef:              record.ProviderRef,
		ProviderMediaKind:        record.ProviderMediaKind,
		MediaRole:                record.MediaRole,
		MIMEType:                 record.MIMEType,
		Bytes:                    record.Bytes,
		BytesEstimated:           record.BytesEstimated,
		Filename:                 record.Filename,
		CapturedAt:               record.CapturedAt,
		UploadedAt:               record.UploadedAt,
		Albums:                   nonNilStringSlice(record.Albums),
		Tags:                     nonNilStringSlice(record.Tags),
		LifecycleState:           record.LifecycleState,
		Sensitivity:              record.Sensitivity,
		SensitivityLabels:        nonNilStringSlice(record.SensitivityLabels),
		Classification:           record.Classification,
		ClassificationDecision:   decodeClassification(record.Classification),
		ClassificationConfidence: record.ClassificationConfidence,
		RawProvider:              record.RawProvider,
	}
}

func decodeClassification(raw json.RawMessage) photolib.ClassificationDecision {
	var classification photolib.ClassificationDecision
	if len(raw) == 0 {
		return classification
	}
	_ = json.Unmarshal(raw, &classification)
	return classification
}

func capabilityNames(capabilities map[photolib.Capability]photolib.CapabilityEntry) []string {
	if len(capabilities) == 0 {
		return []string{"read", "monitor", "upload", "write_album", "write_tag", "write_favorite", "faces_read"}
	}
	values := make([]string, 0, len(capabilities))
	for capability, entry := range capabilities {
		if entry.Status == photolib.CapabilitySupported || entry.Status == photolib.CapabilityLimited {
			values = append(values, string(capability))
		}
	}
	return values
}

func hasProvider(connectors []photoConnectorResponse, provider string) bool {
	for _, connector := range connectors {
		if connector.Provider == provider {
			return true
		}
	}
	return false
}

func statusFromConfig(enabled bool) string {
	if enabled {
		return "configured"
	}
	return "disconnected"
}

func displayNameForPhotoProvider(provider string) string {
	if provider == "immich" {
		return "Immich"
	}
	return provider
}

func parsePhotoLimit(raw string) int {
	if strings.TrimSpace(raw) == "" {
		return 20
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 20
	}
	if value > 100 {
		return 100
	}
	return value
}

func nonNilStringSlice(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
