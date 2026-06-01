// Package httpadapter — SCOPE-2 pre-facade middleware chain.
//
// PreFacadeChain composes the three middlewares spec 069 SCOPE-2 lands
// in front of the HTTP adapter:
//
//  1. auth.RequireScope(cfg.RequiredScope) — gates the request on the
//     spec-060-approved scope claim. Bearer-auth has already populated
//     the session by the time this middleware runs (the chi route is
//     mounted inside the bearer-auth group); a missing required scope
//     emits 403 with the canonical {"error":"scope_required"} body
//     before facade invocation.
//  2. perUserRateLimit — httprate-based per-user (Session.UserID)
//     limiter with the SST-configured per-minute budget. Emits 429
//     with the v1 envelope and facade_invoked=false. The key falls
//     back to the bearer-auth source so dev shared-token requests
//     (no UserID) still share a sane bucket.
//  3. bodySizeCap — buffers up to BodySizeMaxBytes+1 bytes; if the
//     stream contains more it emits 413 with the v1 envelope and
//     facade_invoked=false, before any JSON decode or facade call.
//
// CORS, auth (bearer), router-level real-IP, and request-id
// middlewares are owned by internal/api/router.go and are NOT part of
// this chain — SCOPE-2 only adds the assistant-route-local layers
// that sit between bearer-auth and the adapter.
package httpadapter

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/auth"
)

// PreFacadeChain returns a single chi-compatible middleware that
// composes the SCOPE-2 layers in the order documented above. Panics
// (at construction time) when cfg violates the SST contract — the
// validator in internal/config/assistant.go already enforces these
// rules, but a panic here surfaces a wiring bug fail-loud.
func PreFacadeChain(cfg HTTPTransportConfig) func(http.Handler) http.Handler {
	if cfg.RequiredScope == "" {
		panic("httpadapter.PreFacadeChain: cfg.RequiredScope is required")
	}
	if cfg.RateLimitPerUserPerMinute < 1 {
		panic("httpadapter.PreFacadeChain: cfg.RateLimitPerUserPerMinute must be >= 1")
	}
	if cfg.BodySizeMaxBytes < 1 {
		panic("httpadapter.PreFacadeChain: cfg.BodySizeMaxBytes must be >= 1")
	}
	scope := auth.RequireScope(cfg.RequiredScope)
	rate := perUserRateLimit(cfg.RateLimitPerUserPerMinute)
	body := bodySizeCap(cfg.BodySizeMaxBytes)
	return func(next http.Handler) http.Handler {
		return scope(rate(body(next)))
	}
}

// perUserRateLimit returns an httprate middleware keyed by the
// authenticated user id (falls back to the session source + remote
// addr when the session carries no user id, e.g. dev shared-token).
// The rejection emits the v1 wire envelope so callers always parse
// the same shape.
func perUserRateLimit(perMinute int) func(http.Handler) http.Handler {
	return httprate.Limit(
		perMinute,
		time.Minute,
		httprate.WithKeyFuncs(keyByAssistantUser),
		httprate.WithLimitHandler(func(w http.ResponseWriter, r *http.Request) {
			writePreFacadeError(w, r, http.StatusTooManyRequests, "rate_limited")
		}),
	)
}

func keyByAssistantUser(r *http.Request) (string, error) {
	sess, ok := auth.SessionFromContext(r.Context())
	if !ok {
		// RequireScope upstream should have already 500'd this case;
		// keep a deterministic non-empty key so the limiter doesn't
		// panic on an empty key in the (unreachable) misordering case.
		return "no-session", nil
	}
	if sess.UserID != "" {
		return "user:" + sess.UserID, nil
	}
	return fmt.Sprintf("src:%s:%s", sess.Source, r.RemoteAddr), nil
}

// bodySizeCap rejects requests whose body exceeds max bytes. We read
// up to max+1 bytes; if the read produces more than max, emit 413
// before any JSON decode or facade call. The buffered bytes are
// re-injected into r.Body so the downstream adapter sees the same
// payload without an extra read.
func bodySizeCap(max int) func(http.Handler) http.Handler {
	limit := int64(max)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > limit {
				writePreFacadeError(w, r, http.StatusRequestEntityTooLarge, "body_too_large")
				return
			}
			limited := http.MaxBytesReader(w, r.Body, limit)
			buf, err := io.ReadAll(limited)
			if err != nil {
				var maxErr *http.MaxBytesError
				if errors.As(err, &maxErr) {
					writePreFacadeError(w, r, http.StatusRequestEntityTooLarge, "body_too_large")
					return
				}
				writePreFacadeError(w, r, http.StatusBadRequest, "invalid_assistant_turn")
				return
			}
			_ = r.Body.Close()
			r.Body = io.NopCloser(bytes.NewReader(buf))
			r.ContentLength = int64(len(buf))
			next.ServeHTTP(w, r)
		})
	}
}

// writePreFacadeError emits the v1 wire envelope for a pre-facade
// rejection. transport_message_id is best-effort: pre-decode 413
// rejections do not have it; the field is emitted as "" so the wire
// shape stays stable.
func writePreFacadeError(w http.ResponseWriter, r *http.Request, status int, code string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	out := TurnResponse{
		SchemaVersion: SchemaVersionV1,
		Transport:     TransportName,
		Status:        string(contracts.StatusUnavailable),
		ErrorCause:    code,
		Sources:       []SourceJSON{},
		FacadeInvoked: false,
		EmittedAt:     time.Now().UTC().Format(time.RFC3339Nano),
		Trace:         TraceJSON{RequestID: middleware.GetReqID(r.Context())},
	}
	_ = json.NewEncoder(w).Encode(out)
}
