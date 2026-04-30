package api

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smackerel/smackerel/internal/config"
	photolib "github.com/smackerel/smackerel/internal/connector/photos"
)

type PhotosHandlers struct {
	store  *photolib.Store
	config config.PhotosConfig
}

type photoConnectorResponse struct {
	Provider             string   `json:"provider"`
	Enabled              bool     `json:"enabled"`
	Capabilities         []string `json:"capabilities"`
	SupportedAPIVersions []string `json:"supported_api_versions,omitempty"`
	PollIntervalSeconds  int      `json:"poll_interval_seconds,omitempty"`
}

func NewPhotosHandlers(store *photolib.Store, cfg config.PhotosConfig) *PhotosHandlers {
	return &PhotosHandlers{store: store, config: cfg}
}

func (h *PhotosHandlers) ListConnectors(w http.ResponseWriter, r *http.Request) {
	connectors := []photoConnectorResponse{
		{
			Provider:             "immich",
			Enabled:              h.config.Enabled && h.config.Providers.Immich.Enabled,
			Capabilities:         []string{"read", "monitor", "faces_read"},
			SupportedAPIVersions: h.config.Providers.Immich.SupportedAPIVersions,
			PollIntervalSeconds:  h.config.Providers.Immich.PollIntervalSeconds,
		},
	}
	writeJSON(w, http.StatusOK, map[string]any{"connectors": connectors})
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
	writeJSON(w, http.StatusOK, map[string]any{"photo": record})
}
