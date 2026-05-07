package api

// Spec 040 Scope 5 — multi-provider capability governance, photo
// health aggregate, and provider-neutral search rerank.
//
// This file holds the new HTTP surfaces introduced for Scope 5:
//
//   • POST /v1/photos/connectors/{id}/capabilities/{capability}/exercise
//     Exercises a writer operation against the live provider; a typed
//     `ProviderLimitationError` from the provider package is translated
//     to `409 PROVIDER_LIMITATION` with the stable `limitation_code`
//     from the shared capability taxonomy. Both Immich and PhotoPrism
//     are accepted via the connector_id / provider routing.
//
//   • GET /v1/photos/health
//     Cross-provider aggregate of photo metrics (lifecycle distribution,
//     duplicate counts, removal queue size, quality histogram, capability
//     limit list, skip counts) so the PWA Photo Health dashboard renders
//     LIVE numbers — never placeholder values.
//
// The cross-provider search rerank lives in `crossProviderRerank`;
// `Search` calls it so the same `content_hash` from two providers
// collapses to a single merged row with both provider links surfaced.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/smackerel/smackerel/internal/connector"
	photolib "github.com/smackerel/smackerel/internal/connector/photos"
	"github.com/smackerel/smackerel/internal/connector/photos/adapters/immich"
	"github.com/smackerel/smackerel/internal/connector/photos/adapters/photoprism"
	"github.com/smackerel/smackerel/internal/metrics"
)

// PhotoCapabilityExerciseRequest is the body for the
// `/exercise` endpoint. The capability path parameter dictates which
// writer method to invoke.
type PhotoCapabilityExerciseRequest struct {
	Provider    string `json:"provider"`
	PhotoRef    string `json:"photo_ref"`
	TargetAlbum string `json:"target_album,omitempty"`
	TargetTag   string `json:"target_tag,omitempty"`
	FaceCluster string `json:"face_cluster,omitempty"`
	FaceName    string `json:"face_name,omitempty"`
	BaseURL     string `json:"base_url,omitempty"`
	APIKey      string `json:"api_key,omitempty"`
	APIToken    string `json:"api_token,omitempty"`
}

// providerLimitationEnvelope is the shape returned with `409
// PROVIDER_LIMITATION`. The PWA banner reads `limitation_code` to
// look up its localised banner copy from the same taxonomy registry.
type providerLimitationEnvelope struct {
	Error providerLimitationDetail `json:"error"`
}

type providerLimitationDetail struct {
	Code           string `json:"code"`
	LimitationCode string `json:"limitation_code"`
	Capability     string `json:"capability"`
	Message        string `json:"message"`
	Provider       string `json:"provider"`
	Status         string `json:"status"`
}

// ExerciseCapability invokes a single writer call so the capability
// governance contract can be observed end-to-end. The handler does
// NOT persist anything — a successful 2xx means the provider accepted
// the call; a 409 PROVIDER_LIMITATION means the capability is not
// supported and the limitation_code identifies the registered cause.
func (h *PhotosHandlers) ExerciseCapability(w http.ResponseWriter, r *http.Request) {
	capabilityParam := strings.TrimSpace(chi.URLParam(r, "capability"))
	capability := photolib.Capability(capabilityParam)
	if capabilityParam == "" {
		writeError(w, http.StatusBadRequest, "invalid_capability", "capability path parameter is required")
		return
	}
	var request PhotoCapabilityExerciseRequest
	if !decodeJSONBody(w, r, &request, "invalid_capability_request", "request body must be JSON") {
		return
	}
	if strings.TrimSpace(request.Provider) == "" {
		writeError(w, http.StatusBadRequest, "invalid_capability_request", "provider is required")
		return
	}
	provider := strings.ToLower(strings.TrimSpace(request.Provider))
	writer, providerName, err := h.writerForProvider(r.Context(), provider, request)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_capability_request", err.Error())
		return
	}
	if err := invokeCapability(r.Context(), writer, capability, request); err != nil {
		envelope, status := translateLimitationError(err, providerName)
		if envelope != nil {
			metrics.PhotosCapabilitiesLimitedTotal.WithLabelValues(emptyToUnknown(request.PhotoRef), providerName, string(capability)).Inc()
			writeJSON(w, status, envelope)
			return
		}
		writeError(w, http.StatusBadGateway, "photo_capability_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "capability": capability, "provider": providerName})
}

// HealthAggregate returns a single JSON document combining lifecycle
// distribution, duplicate counts, removal queue size, quality
// histogram, capability limit list, and skip counts. The PWA Photo
// Health dashboard renders this payload directly so every number is
// live data — never a placeholder.
func (h *PhotosHandlers) HealthAggregate(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "photos_store_unavailable", "photo store is unavailable")
		return
	}
	now := time.Now().UTC()
	threshold := h.config.Policy.LifecycleConfirmationThreshold
	lifecycle, err := h.store.SummarizeLifecycle(r.Context(), threshold, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "photo_health_failed", err.Error())
		return
	}
	clusters, err := h.store.ListClusters(r.Context(), "open")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "photo_health_failed", err.Error())
		return
	}
	duplicateByKind := map[string]int{}
	for _, cluster := range clusters {
		duplicateByKind[string(cluster.Kind)]++
	}
	removal, err := h.store.ListRemovalCandidates(r.Context(), "pending_review", 200)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "photo_health_failed", err.Error())
		return
	}
	quality, err := h.store.QualityHistogram(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "photo_health_failed", err.Error())
		return
	}
	skips := []map[string]any{}
	connectorBreakdown := []map[string]any{}
	if states, statesErr := h.store.ListConnectorStates(r.Context()); statesErr == nil {
		for _, state := range states {
			connectorBreakdown = append(connectorBreakdown, map[string]any{
				"connector_id":           state.ConnectorID,
				"provider":               state.Provider,
				"status":                 state.Status,
				"last_sync_at":           state.LastSyncAt,
				"monitoring_lag_seconds": state.MonitoringLagSeconds,
			})
			for _, skip := range state.Skips {
				skips = append(skips, map[string]any{
					"connector_id": state.ConnectorID,
					"provider":     state.Provider,
					"reason":       skip.Reason,
					"count":        skip.Count,
					"last_seen_at": skip.LastSeenAt,
				})
			}
		}
	}
	capabilityLimits := []map[string]any{}
	for _, descriptor := range photolib.AllLimitationDescriptors() {
		capabilityLimits = append(capabilityLimits, map[string]any{
			"capability":      string(descriptor.Capability),
			"status":          string(descriptor.Status),
			"limitation_code": string(descriptor.Code),
			"banner_title":    descriptor.BannerTitle,
			"banner_body":     descriptor.BannerBody,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"lifecycle":         lifecycle,
		"duplicates":        map[string]any{"by_kind": duplicateByKind, "total": len(clusters)},
		"removal_pending":   len(removal),
		"quality":           map[string]any{"buckets": quality},
		"connectors":        connectorBreakdown,
		"capability_limits": capabilityLimits,
		"skips":             skips,
	})
}

func (h *PhotosHandlers) writerForProvider(ctx context.Context, provider string, request PhotoCapabilityExerciseRequest) (photolib.ProviderWriter, string, error) {
	switch provider {
	case "immich":
		client, providerConfig, err := h.immichClientFromExerciseRequest(request)
		if err != nil {
			return nil, provider, err
		}
		if err := client.Connect(ctx, providerConfig); err != nil {
			return nil, provider, fmt.Errorf("immich connect failed: %w", err)
		}
		return client.Writer(), provider, nil
	case "photoprism":
		client, providerConfig, err := h.photoprismClientFromExerciseRequest(request)
		if err != nil {
			return nil, provider, err
		}
		if err := client.Connect(ctx, providerConfig); err != nil {
			return nil, provider, fmt.Errorf("photoprism connect failed: %w", err)
		}
		return client.Writer(), provider, nil
	}
	return nil, provider, fmt.Errorf("unsupported provider %q (must be immich or photoprism)", provider)
}

func (h *PhotosHandlers) immichClientFromExerciseRequest(request PhotoCapabilityExerciseRequest) (*immich.Client, connector.ConnectorConfig, error) {
	baseURL := strings.TrimSpace(request.BaseURL)
	apiKey := strings.TrimSpace(request.APIKey)
	if baseURL == "" {
		baseURL = h.config.Providers.Immich.BaseURL
	}
	if apiKey == "" {
		apiKey = h.config.Providers.Immich.APIKey
	}
	if baseURL == "" || apiKey == "" {
		return nil, connector.ConnectorConfig{}, fmt.Errorf("immich requires base_url + api_key")
	}
	client := immich.NewClient(http.DefaultClient)
	// MIT-040-S-006 — SST-injected upload-body cap.
	client.SetUploadMaxBytes(h.config.IOLimits.PhotoBinaryMaxBytes)
	return client, connector.ConnectorConfig{
		AuthType:     "api_key",
		Credentials:  map[string]string{"api_key": apiKey},
		SourceConfig: map[string]any{"base_url": baseURL},
		Enabled:      true,
	}, nil
}

func (h *PhotosHandlers) photoprismClientFromExerciseRequest(request PhotoCapabilityExerciseRequest) (*photoprism.Client, connector.ConnectorConfig, error) {
	baseURL := strings.TrimSpace(request.BaseURL)
	apiToken := strings.TrimSpace(request.APIToken)
	if baseURL == "" {
		baseURL = h.config.Providers.Photoprism.BaseURL
	}
	if apiToken == "" {
		apiToken = h.config.Providers.Photoprism.APIToken
	}
	if baseURL == "" || apiToken == "" {
		return nil, connector.ConnectorConfig{}, fmt.Errorf("photoprism requires base_url + api_token")
	}
	client := photoprism.NewClient(http.DefaultClient)
	// MIT-040-S-006 — SST-injected upload-body cap.
	client.SetUploadMaxBytes(h.config.IOLimits.PhotoBinaryMaxBytes)
	return client, connector.ConnectorConfig{
		AuthType:     "api_token",
		Credentials:  map[string]string{"api_token": apiToken},
		SourceConfig: map[string]any{"base_url": baseURL},
		Enabled:      true,
	}, nil
}

func invokeCapability(ctx context.Context, writer photolib.ProviderWriter, capability photolib.Capability, request PhotoCapabilityExerciseRequest) error {
	switch capability {
	case photolib.CapWriteAlbum:
		if request.PhotoRef == "" || request.TargetAlbum == "" {
			return fmt.Errorf("write_album requires photo_ref + target_album")
		}
		return writer.AddToAlbum(ctx, request.PhotoRef, request.TargetAlbum)
	case photolib.CapWriteTag:
		if request.PhotoRef == "" || request.TargetTag == "" {
			return fmt.Errorf("write_tag requires photo_ref + target_tag")
		}
		return writer.Tag(ctx, request.PhotoRef, request.TargetTag)
	case photolib.CapWriteFavorite:
		if request.PhotoRef == "" {
			return fmt.Errorf("write_favorite requires photo_ref")
		}
		return writer.Favorite(ctx, request.PhotoRef, true)
	case photolib.CapArchive:
		if request.PhotoRef == "" {
			return fmt.Errorf("archive requires photo_ref")
		}
		return writer.Archive(ctx, request.PhotoRef)
	case photolib.CapDelete:
		if request.PhotoRef == "" {
			return fmt.Errorf("delete requires photo_ref")
		}
		return writer.Delete(ctx, request.PhotoRef)
	case photolib.CapFacesWrite:
		if request.FaceCluster == "" || request.FaceName == "" {
			return fmt.Errorf("faces_write requires face_cluster + face_name")
		}
		return writer.RenameFaceCluster(ctx, request.FaceCluster, request.FaceName)
	}
	return fmt.Errorf("capability %q is not exercisable", capability)
}

// translateLimitationError detects whether the error returned by a
// writer is one of the typed `ProviderLimitationError` values defined
// by the adapter packages. When it is, the function returns the 409
// PROVIDER_LIMITATION envelope plus the appropriate status code; when
// it is not, the caller falls back to a generic 502 envelope.
func translateLimitationError(err error, providerName string) (*providerLimitationEnvelope, int) {
	var photoprismLimit *photoprism.ProviderLimitationError
	if errors.As(err, &photoprismLimit) {
		descriptor, _ := photolib.LimitationDescriptorFor(photoprismLimit.LimitationCode)
		envelope := &providerLimitationEnvelope{Error: providerLimitationDetail{
			Code:           "provider_limitation",
			LimitationCode: string(photoprismLimit.LimitationCode),
			Capability:     string(photoprismLimit.Capability),
			Message:        descriptor.BannerBody,
			Provider:       providerName,
			Status:         string(photoprismLimit.Status),
		}}
		return envelope, http.StatusConflict
	}
	return nil, http.StatusBadGateway
}

// crossProviderRerank merges photo summaries that share the same
// content_hash across providers. Provider-neutrality is preserved by
// keeping the highest-confidence row first and appending the merged
// provider list under the `providers` key in the preview map. When
// content_hash is empty (e.g. provider stripped EXIF entirely), the
// summary falls through unmerged so we never collapse genuinely
// distinct photos.
func crossProviderRerank(summaries []photoSummary) []photoSummary {
	merged := make([]photoSummary, 0, len(summaries))
	seen := map[string]int{}
	for _, summary := range summaries {
		key := strings.TrimSpace(summary.ContentHash)
		if key == "" {
			summary.Preview = ensureMap(summary.Preview)
			summary.Preview["providers"] = []string{summary.Provider}
			merged = append(merged, summary)
			continue
		}
		if previousIndex, ok := seen[key]; ok {
			previous := merged[previousIndex]
			if previous.Preview == nil {
				previous.Preview = map[string]any{}
			}
			providers, _ := previous.Preview["providers"].([]string)
			previous.Preview["providers"] = appendUnique(providers, summary.Provider)
			merged[previousIndex] = previous
			continue
		}
		summary.Preview = ensureMap(summary.Preview)
		summary.Preview["providers"] = []string{summary.Provider}
		seen[key] = len(merged)
		merged = append(merged, summary)
	}
	return merged
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func ensureMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}

func emptyToUnknown(value string) string {
	if strings.TrimSpace(value) == "" {
		return "unknown"
	}
	return value
}
