package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/smackerel/smackerel/internal/auth"
	photolib "github.com/smackerel/smackerel/internal/connector/photos"
)

// PhotoUploadResponse is returned by POST /v1/photos/upload.
type PhotoUploadResponse struct {
	PhotoID         string                  `json:"photo_id"`
	ArtifactID      string                  `json:"artifact_id"`
	ConnectorID     string                  `json:"connector_id"`
	Provider        string                  `json:"provider"`
	ProviderRef     string                  `json:"provider_ref"`
	SourceChannel   photolib.SourceChannel  `json:"source_channel"`
	SourceRef       string                  `json:"source_ref"`
	DocumentGroupID string                  `json:"document_group_id,omitempty"`
	PageIndex       int                     `json:"document_page_index,omitempty"`
	Pipeline        photoUploadPipelineEcho `json:"pipeline"`
}

type photoUploadPipelineEcho struct {
	Name              string   `json:"name"`
	Stages            []string `json:"stages"`
	ClassificationFor string   `json:"classification_for_artifact_id,omitempty"`
}

// maxUploadBytes caps multipart parsing memory usage. Anything larger
// must already be rejected by the SST `photos.scan.max_file_size_bytes`
// gate before the body is buffered.
const maxUploadBytes = 64 << 20

// Upload handles `POST /v1/photos/upload` for the unified capture
// pipeline (Telegram, mobile, web, agent). The handler enforces:
//
//   - required `source_channel` (SCN-040-010 — uploads MUST preserve
//     the inbound channel),
//   - required `source_ref` so the channel can correlate the upload
//     with its message/intent identifier,
//   - optional `mode=document` + `document_group_id` for multi-page
//     scans (SCN-040-011),
//   - SST file size enforcement via `photos.scan.max_file_size_bytes`,
//   - uniform persistence through `Store.PublishPhotoEvent` so every
//     upload enters the same downstream pipeline (classify → routing →
//     sensitivity gate → audit).
func (h *PhotosHandlers) Upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_upload", "request must be multipart/form-data: "+err.Error())
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()

	channel := photolib.SourceChannel(strings.TrimSpace(r.FormValue("source_channel")))
	if channel == "" || !channel.Valid() {
		writeError(w, http.StatusBadRequest, "invalid_source_channel",
			"source_channel must be one of: telegram, mobile, web, agent")
		return
	}
	if channel == photolib.SourceChannelProvider {
		writeError(w, http.StatusBadRequest, "invalid_source_channel",
			"provider channel is reserved for connector scans, not uploads")
		return
	}
	sourceRef := strings.TrimSpace(r.FormValue("source_ref"))
	if sourceRef == "" {
		writeError(w, http.StatusBadRequest, "invalid_source_ref",
			"source_ref is required so the channel can correlate the upload")
		return
	}

	mode := strings.ToLower(strings.TrimSpace(r.FormValue("mode")))
	if mode == "" {
		mode = "single"
	}
	if mode != "single" && mode != "document" {
		writeError(w, http.StatusBadRequest, "invalid_mode", "mode must be either single or document")
		return
	}
	groupRef := strings.TrimSpace(r.FormValue("document_group_id"))
	pageIndex := 0
	if raw := strings.TrimSpace(r.FormValue("document_page_index")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			writeError(w, http.StatusBadRequest, "invalid_page_index", "document_page_index must be a positive integer")
			return
		}
		pageIndex = parsed
	}
	if mode == "document" && groupRef == "" {
		writeError(w, http.StatusBadRequest, "invalid_document_group", "document mode requires document_group_id")
		return
	}

	provider := strings.TrimSpace(r.FormValue("provider"))
	if provider == "" {
		provider = string(channel)
	}
	connectorID := strings.TrimSpace(r.FormValue("connector_id"))
	if connectorID == "" {
		connectorID = "photos-upload-" + string(channel)
	}
	caption := strings.TrimSpace(r.FormValue("caption"))

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing_file", "file part is required (field name: file)")
		return
	}
	defer file.Close()

	if h == nil || h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "photos_store_unavailable", "photo store is unavailable")
		return
	}
	if max := h.config.Scan.MaxFileSizeBytes; max > 0 && header.Size > max {
		writeError(w, http.StatusRequestEntityTooLarge, "file_too_large",
			fmt.Sprintf("file size %d exceeds configured maximum %d (PHOTOS_SCAN_MAX_FILE_SIZE_BYTES)", header.Size, max))
		return
	}

	contents, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusBadRequest, "upload_read_failed", "failed to read upload payload: "+err.Error())
		return
	}
	if len(contents) == 0 {
		writeError(w, http.StatusBadRequest, "empty_file", "file part must contain non-empty bytes")
		return
	}

	mimeType := header.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = mime.TypeByExtension(strings.ToLower(filepath.Ext(header.Filename)))
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	mediaRole := photolib.MediaRoleCameraOriginal
	if mode == "document" {
		mediaRole = photolib.MediaRoleDocumentScan
	}
	now := time.Now().UTC()
	bytesLen := int64(len(contents))
	providerRef := buildProviderRef(channel, sourceRef, groupRef, pageIndex)
	contentHash := contentHashForUpload(provider, providerRef, contents)
	rawProvider := map[string]any{
		"provider":          provider,
		"asset_id":          providerRef,
		"upload":            true,
		"channel":           string(channel),
		"channel_source":    sourceRef,
		"caption":           caption,
		"original_filename": header.Filename,
	}
	event := photolib.PhotoEvent{
		ProviderRef:       providerRef,
		Operation:         photolib.PhotoOpUpsert,
		ProviderMediaKind: "image",
		MediaRole:         mediaRole,
		ContentHash:       contentHash,
		Bytes:             &bytesLen,
		MIMEType:          mimeType,
		Filename:          fallbackFilename(header.Filename, providerRef, mimeType),
		CapturedAt:        now,
		UploadedAt:        now,
		EXIF:              map[string]any{"upload_channel": string(channel)},
		Albums:            []string{},
		Tags:              []string{},
		Faces:             []photolib.FaceClusterRef{},
		Sensitivity:       photolib.ProviderSensitivity{Level: photolib.SensitivityNone, Source: "upload"},
		RawProvider:       rawProvider,
		SourceChannel:     channel,
		SourceRef:         sourceRef,
		DocumentGroupRef:  groupRef,
		DocumentPageIndex: pageIndex,
	}
	record, err := h.store.PublishPhotoEvent(r.Context(), connectorID, provider, event)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "upload_persist_failed", err.Error())
		return
	}

	if err := h.store.WriteAuditEvent(r.Context(), photolib.AuditEvent{
		Action:    "photo_upload",
		PhotoID:   &record.ID,
		Connector: connectorID,
		Provider:  provider,
		Outcome:   "stored",
		Reason:    string(channel),
		Actor:     h.actorIDFromRequest(r),
		Metadata: map[string]any{
			"source_channel": string(channel),
			"source_ref":     sourceRef,
			"mode":           mode,
			"bytes":          bytesLen,
		},
		CreatedAt: now,
	}); err != nil {
		slog.Warn("photo upload audit failed", "photo_id", record.ID.String(), "error", err)
	}

	pipeline := photoUploadPipelineEcho{
		Name:              "photos.unified_pipeline",
		Stages:            []string{"persist", "classify", "routing", "sensitivity_gate"},
		ClassificationFor: record.ArtifactID,
	}
	resp := PhotoUploadResponse{
		PhotoID:       record.ID.String(),
		ArtifactID:    record.ArtifactID,
		ConnectorID:   record.ConnectorID,
		Provider:      record.Provider,
		ProviderRef:   record.ProviderRef,
		SourceChannel: record.SourceChannel,
		SourceRef:     record.SourceRef,
		PageIndex:     pageIndex,
		Pipeline:      pipeline,
	}
	if record.DocumentGroupID != nil {
		resp.DocumentGroupID = record.DocumentGroupID.String()
	}
	writeJSON(w, http.StatusCreated, resp)
}

// PhotoRevealRequest is the optional body for `POST /v1/photos/{id}/reveal`.
//
// Spec 044 Scope 02 (MIT-040-S-008 closure, 2026-05-08+): `actor_id`
// is permanently rejected from the request body. The handler derives
// the actor identity from the authenticated session attached by
// `bearerAuthMiddleware` (PASETO `sub` claim → `auth.Session.UserID`
// in production; shared-token sessions fall back to the dev/test
// ergonomic). Requests whose body still contains an `actor_id` JSON
// key are rejected with HTTP 400 `actor_id_in_body_forbidden`. The
// `X-Actor-Id` header is now ALSO rejected in production (`HTTP 400
// actor_id_in_header_forbidden`) — claim-binding closes the
// client-controlled-identity residual that MIT-040-S-003's partial
// closure left open.
type PhotoRevealRequest struct {
	TTLSeconds int `json:"ttl_seconds,omitempty"`
}

// PhotoRevealResponse is the response shape mint endpoint returns.
type PhotoRevealResponse struct {
	RevealToken string    `json:"reveal_token"`
	PhotoID     string    `json:"photo_id"`
	ExpiresAt   time.Time `json:"expires_at"`
	TTLSeconds  int       `json:"ttl_seconds"`
}

// MintReveal handles `POST /v1/photos/{id}/reveal`. The endpoint mints a
// short-lived reveal token bound to the requesting actor and the
// specified photo so retrieval surfaces (preview bytes, Telegram
// delivery) can authorise a single subsequent fetch. Audit rows are
// written for both mint and consume to satisfy SCN-040-012.
//
// Spec 044 Scope 02 closure of MIT-040-S-008 (2026-05-08+):
//   - Body `actor_id` continues to be rejected with HTTP 400
//     `actor_id_in_body_forbidden` (preserves the MIT-040-S-003
//     partial closure invariant).
//   - In `production` (auth.enabled=true), `X-Actor-Id` is rejected
//     with HTTP 400 `actor_id_in_header_forbidden` and the actor is
//     derived from the authenticated session
//     (`auth.UserIDFromContext(r.Context())`). When the session is
//     present but UserID is empty (shared-token fallback or bootstrap
//     session) the handler returns HTTP 400 `actor_id_required`
//     because the production code path can no longer downgrade to a
//     client-controlled value.
//   - In `development` / `test`, `X-Actor-Id` continues to be honored
//     (FR-AUTH-015 dev ergonomic) and the handler falls back to
//     `system` when no header AND no session-derived UserID is
//     present. This preserves the test stack ergonomic without
//     leaking client-controlled identity into production.
func (h *PhotosHandlers) MintReveal(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "id")))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_photo_id", "photo id must be a UUID")
		return
	}

	// MIT-040-S-003 partial closure — read the body once so we can
	// reject `actor_id` smuggling before any further parsing. The
	// 16 KiB cap matches the previous decoder limit.
	var bodyBytes []byte
	if r.ContentLength > 0 {
		bodyBytes, err = io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<14))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_reveal_payload", "request body could not be read")
			return
		}
	}
	if bytes.Contains(bodyBytes, []byte(`"actor_id"`)) {
		writeError(w, http.StatusBadRequest, "actor_id_in_body_forbidden",
			"actor_id in request body is forbidden")
		return
	}

	var request PhotoRevealRequest
	if len(bytes.TrimSpace(bodyBytes)) > 0 {
		if err := json.Unmarshal(bodyBytes, &request); err != nil && !errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, "invalid_reveal_payload", "request body must be JSON")
			return
		}
	}

	// Spec 044 Scope 02 (MIT-040-S-008 closure) — actor identity is
	// resolved against the authenticated session attached by
	// `bearerAuthMiddleware`. In production we additionally reject the
	// X-Actor-Id header (no client-controlled identity surface)
	// because PASETO `sub` is the single source of truth.
	headerActor := strings.TrimSpace(r.Header.Get("X-Actor-Id"))
	sessionUserID := auth.UserIDFromContext(r.Context())
	if h.environment == "production" {
		if headerActor != "" {
			writeError(w, http.StatusBadRequest, "actor_id_in_header_forbidden",
				"X-Actor-Id header is forbidden in production; identity is derived from the bearer token")
			return
		}
		if sessionUserID == "" {
			writeError(w, http.StatusBadRequest, "actor_id_required",
				"per-user bearer auth is required in production")
			return
		}
	}
	actor := sessionUserID
	if actor == "" {
		actor = headerActor
	}
	if actor == "" {
		actor = "system"
	}

	if h == nil || h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "photos_store_unavailable", "photo store is unavailable")
		return
	}
	record, err := h.store.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "photo_not_found", "photo not found")
		return
	}
	if record.Sensitivity == photolib.SensitivityNone {
		writeError(w, http.StatusConflict, "reveal_not_required", "photo is not sensitivity-gated")
		return
	}
	ttl := time.Duration(request.TTLSeconds) * time.Second
	if ttl <= 0 {
		ttl = h.revealTTL()
	}
	now := time.Now().UTC()
	token, err := h.store.MintRevealToken(r.Context(), photolib.MintRevealTokenInput{
		PhotoID: id,
		ActorID: actor,
		TTL:     ttl,
	}, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "mint_reveal_failed", err.Error())
		return
	}
	if err := h.store.WriteAuditEvent(r.Context(), photolib.AuditEvent{
		Action:    "sensitivity_reveal_minted",
		PhotoID:   &id,
		Outcome:   "minted",
		Reason:    string(record.Sensitivity),
		Actor:     actor,
		Metadata:  map[string]any{"reveal_token_id": token.ID.String(), "ttl_seconds": int(ttl.Seconds())},
		CreatedAt: now,
	}); err != nil {
		slog.Warn("reveal mint audit failed", "photo_id", id.String(), "error", err)
	}
	writeJSON(w, http.StatusCreated, PhotoRevealResponse{
		RevealToken: token.Plaintext,
		PhotoID:     id.String(),
		ExpiresAt:   token.ExpiresAt,
		TTLSeconds:  int(ttl.Seconds()),
	})
}

func (h *PhotosHandlers) revealTTL() time.Duration {
	if h == nil {
		return 60 * time.Second
	}
	if seconds := h.config.Policy.SensitivityRevealTTLSeconds; seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	return 60 * time.Second
}

func buildProviderRef(channel photolib.SourceChannel, sourceRef, groupRef string, pageIndex int) string {
	if groupRef != "" {
		return string(channel) + ":doc:" + groupRef + ":p" + strconv.Itoa(pageIndex)
	}
	return string(channel) + ":upload:" + sourceRef + ":" + uuid.NewString()
}

func contentHashForUpload(provider, providerRef string, contents []byte) string {
	if len(contents) == 0 {
		return "sha256:empty:" + provider + ":" + providerRef
	}
	return "sha256:upload:" + provider + ":" + providerRef + ":" + uuid.NewString()
}

func fallbackFilename(name, providerRef, mimeType string) string {
	if strings.TrimSpace(name) != "" {
		return name
	}
	ext := filepath.Ext(name)
	if ext == "" {
		exts, _ := mime.ExtensionsByType(mimeType)
		if len(exts) > 0 {
			ext = exts[0]
		}
	}
	return providerRef + ext
}
