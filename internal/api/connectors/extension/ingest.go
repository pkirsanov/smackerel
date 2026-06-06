// Package extension implements the spec-058 server-side ingest
// handler for the Chrome Extension Bridge.
//
// POST /v1/connectors/extension/ingest accepts a JSON array of
// connector.RawArtifact items, enforces transport-level limits, runs
// per-item validation against the SST allowlist, resolves each item
// against the server-authoritative dedup store, and publishes fresh
// artifacts via the existing ArtifactPublisher. Per-item outcomes
// are returned in a single 200 response body; only transport-level
// failures (auth, body too large, batch too large, invalid JSON)
// produce 4xx status codes.
//
// Auth is enforced by the calling router via auth.RequireScope —
// this handler assumes a Session is present in the request context
// and a valid extension scope has already been verified.
package extension

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/smackerel/smackerel/internal/auth"
	"github.com/smackerel/smackerel/internal/config"
	"github.com/smackerel/smackerel/internal/connector"
	"github.com/smackerel/smackerel/internal/connector/ingest"
)

// expectedSourceID is the canonical RawArtifact.SourceID value the
// extension ingest path emits. Spec 058 design §2.1.
const expectedSourceID = "browser-extension"

// dedupWindowMinSeconds and dedupWindowMaxSeconds clamp the per-
// request Metadata.dedup_window_seconds override. Spec 058 design §6.
const (
	dedupWindowMinSeconds = 60
	dedupWindowMaxSeconds = 86400
)

// sourceDeviceIDRe enforces the spec 058 design §2.3 source_device_id
// contract at the server trust boundary: lowercase alphanumerics and
// hyphens only. The design's request flow (§3.2) requires the handler
// to "validate each ... Metadata field"; this is that enforcement for
// source_device_id, which would otherwise flow UNCHECKED into the
// dedup-key preimage (ingest.ComputeDedupKey) and the stored device id
// surfaced by the admin devices view. Enforcing the charset here (the
// options-page validator is the client-side mirror) blocks null-byte /
// control-char / whitespace / case / unicode injection at the boundary.
//
// Length bound: 64. The design body text says "1–32 chars", but the
// design's OWN auto-generated fallback is "auto-<uuidv4>" = 41 chars,
// so a 32 cap would reject conforming clients. 64 admits both the
// operator-set (≤32) and auto (41) forms while still bounding the
// input so a hostile client cannot bloat the stored device id or the
// dedup-key preimage with an unbounded string.
var sourceDeviceIDRe = regexp.MustCompile(`^[a-z0-9-]{1,64}$`)

// ArtifactPublisher is the interface satisfied by
// pipeline.RawArtifactPublisher. Declared locally to keep the
// handler decoupled from the pipeline package.
type ArtifactPublisher interface {
	PublishRawArtifact(ctx context.Context, artifact connector.RawArtifact) (string, error)
}

// Handler is the HTTP handler for POST /v1/connectors/extension/ingest.
type Handler struct {
	cfg   config.ExtensionIngestConfig
	pub   ArtifactPublisher
	dedup ingest.DedupStore
	// accepted is a map view of cfg.AcceptedContentTypes for O(1)
	// validation. Built once at construction.
	accepted map[string]struct{}
}

// NewHandler returns a configured ingest handler. Panics on a nil
// publisher or nil dedup store — those are wiring bugs and must
// surface at startup, not at the first request.
func NewHandler(cfg config.ExtensionIngestConfig, pub ArtifactPublisher, dedup ingest.DedupStore) *Handler {
	if pub == nil {
		panic("extension: NewHandler requires a non-nil ArtifactPublisher")
	}
	if dedup == nil {
		panic("extension: NewHandler requires a non-nil DedupStore")
	}
	accepted := make(map[string]struct{}, len(cfg.AcceptedContentTypes))
	for _, ct := range cfg.AcceptedContentTypes {
		accepted[ct] = struct{}{}
	}
	return &Handler{cfg: cfg, pub: pub, dedup: dedup, accepted: accepted}
}

// IngestItemOutcome is the per-item response shape. ArtifactID is
// populated for "accepted" and "deduped"; Error is populated only
// for "rejected".
type IngestItemOutcome struct {
	ClientEventID string `json:"client_event_id"`
	Outcome       string `json:"outcome"`
	ArtifactID    string `json:"artifact_id,omitempty"`
	Error         string `json:"error,omitempty"`
}

type ingestResponse struct {
	Items []IngestItemOutcome `json:"items"`
}

type ingestErrorResponse struct {
	Error string `json:"error"`
}

// ServeHTTP implements http.Handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Defensive: this handler is mounted behind bearerAuthMiddleware
	// and auth.RequireScope. The Session presence check is a
	// belt-and-suspenders guard against a future re-mount that
	// skips the bearer middleware.
	if _, ok := auth.SessionFromContext(r.Context()); !ok {
		writeError(w, http.StatusUnauthorized, "auth_required")
		return
	}

	// Body-size cap: Content-Length pre-check + MaxBytesReader as
	// defense-in-depth against missing/lying Content-Length.
	if r.ContentLength > h.cfg.MaxBodyBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "body_too_large")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, h.cfg.MaxBodyBytes)

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var items []connector.RawArtifact
	if err := dec.Decode(&items); err != nil {
		// http.MaxBytesReader signals body overflow via an
		// *http.MaxBytesError; surface that as 413 to match the
		// pre-check above.
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "body_too_large")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}

	if len(items) > h.cfg.MaxBatchItems {
		writeError(w, http.StatusUnprocessableEntity, "batch_too_large")
		return
	}

	resp := ingestResponse{Items: make([]IngestItemOutcome, 0, len(items))}
	for _, item := range items {
		resp.Items = append(resp.Items, h.processItem(r.Context(), item))
	}

	writeJSON(w, http.StatusOK, resp)
}

// processItem runs validation + dedup + publish for a single item
// and returns its outcome envelope. All errors are mapped to
// per-item rejection — no per-item error short-circuits the batch.
func (h *Handler) processItem(ctx context.Context, item connector.RawArtifact) IngestItemOutcome {
	clientEventID := readMetadataString(item.Metadata, "client_event_id")
	out := IngestItemOutcome{ClientEventID: clientEventID, Outcome: "rejected"}

	if item.SourceID != expectedSourceID {
		out.Error = "source_id_invalid"
		return out
	}
	if _, ok := h.accepted[item.ContentType]; !ok {
		out.Error = "content_type_not_accepted"
		return out
	}
	if strings.TrimSpace(item.URL) == "" {
		out.Error = "url_required"
		return out
	}
	if !(strings.HasPrefix(item.URL, "http://") || strings.HasPrefix(item.URL, "https://")) {
		out.Error = "url_scheme_invalid"
		return out
	}
	if item.CapturedAt.IsZero() {
		out.Error = "captured_at_required"
		return out
	}

	deviceID := readMetadataString(item.Metadata, "source_device_id")
	if deviceID == "" {
		out.Error = "metadata.source_device_id_required"
		return out
	}
	// Trust-boundary enforcement of the spec 058 design §2.3
	// source_device_id charset/length contract. Without this, a
	// malformed (or hostile) device id flows straight into the
	// dedup-key preimage and the admin devices view.
	if !sourceDeviceIDRe.MatchString(deviceID) {
		out.Error = "metadata.source_device_id_invalid"
		return out
	}

	bucket := computeBucket(item.ContentType, item.CapturedAt, item.Metadata, h.cfg.DefaultDedupWindowSeconds)
	key := ingest.ComputeDedupKey(item.URL, item.ContentType, deviceID, bucket)

	row := ingest.DedupRow{
		Key:            key,
		OwnerUserID:    ownerUserID(ctx),
		SourceID:       item.SourceID,
		ContentType:    item.ContentType,
		SourceDeviceID: deviceID,
		CapturedAt:     item.CapturedAt,
	}

	artifactID, deduped, err := h.dedup.ResolveOrPublish(ctx, row, func(ctx context.Context) (string, error) {
		return h.pub.PublishRawArtifact(ctx, item)
	})
	if err != nil {
		slog.Warn("extension ingest: publish/dedup failed",
			"client_event_id", clientEventID,
			"source_device_id", deviceID,
			"content_type", item.ContentType,
			"err", err)
		out.Error = "publish_failed"
		return out
	}

	out.ArtifactID = artifactID
	if deduped {
		out.Outcome = "deduped"
	} else {
		out.Outcome = "accepted"
	}
	return out
}

// computeBucket implements the spec 058 §2.3 bucketing rule:
//   - "bookmark" content type always uses bucket 0 (window is bypassed).
//   - Time-bucketed types use floor(captured_at_unix / window_seconds),
//     where window_seconds is the per-request override (clamped to
//     [60, 86400]) or the SST default when absent/out-of-range.
func computeBucket(contentType string, capturedAt time.Time, metadata map[string]any, defaultWindow int) int64 {
	if contentType == "bookmark" {
		return 0
	}
	window := defaultWindow
	if v, ok := metadata["dedup_window_seconds"]; ok {
		if candidate, ok := toInt(v); ok && candidate >= dedupWindowMinSeconds && candidate <= dedupWindowMaxSeconds {
			window = candidate
		}
	}
	if window <= 0 {
		// Defensive: should be impossible after Validate(); keep a
		// non-zero floor so we never divide by zero.
		window = dedupWindowMinSeconds
	}
	return capturedAt.Unix() / int64(window)
}

func toInt(v any) (int, bool) {
	switch x := v.(type) {
	case int:
		return x, true
	case int64:
		return int(x), true
	case float64:
		return int(x), true
	case json.Number:
		i, err := x.Int64()
		if err != nil {
			return 0, false
		}
		return int(i), true
	default:
		return 0, false
	}
}

func readMetadataString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

func ownerUserID(ctx context.Context) string {
	if sess, ok := auth.SessionFromContext(ctx); ok {
		return sess.UserID
	}
	return ""
}

func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, ingestErrorResponse{Error: code})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Warn("extension ingest: response encode failed", "err", fmt.Sprint(err))
	}
}
