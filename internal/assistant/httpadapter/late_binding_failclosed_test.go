package httpadapter

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/auth"
)

// Spec 069 harden — HARDEN-069-H01 fail-closed late binding.
//
// cmd/core/wiring_assistant_facade.go installs the adapter and the
// SCOPE-2 PreFacadeChain through two SEPARATE atomic stores, and the
// whole facade wiring runs in a background goroutine
// (cmd/core/main.go: `go runAssistantFacadeWiringWithRetry(...)`)
// while the chi listener is ALREADY serving POST /api/assistant/turn.
// Any request that lands after SetAdapter but before SetMiddleware is
// therefore served by the BARE adapter, with none of the SCOPE-2
// protections that route depends on:
//
//   - auth.RequireScope("assistant:turn")  — 403 scope gate
//   - perUserRateLimit                     — 429 budget
//   - bodySizeCap / http.MaxBytesReader    — 413 cap in front of the
//     adapter's unbounded io.ReadAll(r.Body)
//
// The BUG-069-001 regression in cmd/core only drives the wiring AFTER
// it has fully completed, so it cannot see this window.
//
// These tests pin the fail-closed invariant STRUCTURALLY: an adapter
// bound without its middleware chain MUST NOT serve. They are
// adversarial — each one passes trivially once the handler refuses,
// and each one FAILS if ServeHTTP ever falls back to serving the bare
// adapter again (which is exactly the pre-harden behavior).

// bindWindowAdapter builds a real *HTTPAdapter whose facade records
// whether the capability layer was reached.
func bindWindowAdapter(t *testing.T) (*HTTPAdapter, *stubFacade) {
	t.Helper()
	facade := &stubFacade{response: contracts.AssistantResponse{
		Status:    contracts.StatusSavedAsIdea,
		Body:      "ok",
		EmittedAt: time.Unix(1735689600, 0).UTC(),
	}}
	adapter, err := NewHTTPAdapter(Options{
		Facade:  facade,
		Capture: func(context.Context, string, string, string) {},
		Clock:   func() time.Time { return time.Unix(1735689600, 0).UTC() },
		Config:  defaultConfig(),
	})
	if err != nil {
		t.Fatalf("NewHTTPAdapter: %v", err)
	}
	return adapter, facade
}

func bindWindowTurnBody(t *testing.T, id, text string) []byte {
	t.Helper()
	b, err := json.Marshal(TurnRequest{
		SchemaVersion:      SchemaVersionV1,
		TransportMessageID: id,
		Kind:               string(contracts.KindText),
		TransportHint:      "web",
		Text:               text,
	})
	if err != nil {
		t.Fatalf("marshal turn: %v", err)
	}
	return b
}

func postThroughLateBound(t *testing.T, h http.Handler, sess auth.Session, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/assistant/turn", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = int64(len(body))
	req = req.WithContext(auth.WithSession(req.Context(), sess))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

// TestLateBoundHandlerRefusesAdapterWithoutMiddlewareChain proves the
// scope gate cannot be bypassed through the bind window. The session
// is a per-user PASETO principal WITHOUT the assistant:turn claim —
// exactly the caller RequireScope exists to reject with 403. With the
// chain absent the facade must still never be reached.
func TestLateBoundHandlerRefusesAdapterWithoutMiddlewareChain(t *testing.T) {
	adapter, facade := bindWindowAdapter(t)

	h := NewLateBoundHandler()
	// The transient wiring state: adapter installed, chain not yet.
	h.SetAdapter(adapter)

	rr := postThroughLateBound(t, h, auth.Session{
		UserID: "u-no-scope",
		Source: auth.SessionSourcePerUserToken,
		Scopes: []string{"capture:write"}, // assistant:turn deliberately absent
	}, bindWindowTurnBody(t, "harden-069-h01-scope", "what is the weather"))

	if facade.calls != 0 {
		t.Errorf("facade.calls = %d, want 0 — the capability layer was reached through the bind window with no scope gate in front of it", facade.calls)
	}
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 — an adapter bound without its PreFacadeChain MUST fail closed, not serve unprotected. body=%s", rr.Code, rr.Body.String())
	}

	var out TurnResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode refusal envelope: %v\nbody=%s", err, rr.Body.String())
	}
	if out.ErrorCause != "assistant_http_not_ready" {
		t.Errorf("error_cause = %q, want assistant_http_not_ready", out.ErrorCause)
	}
	if out.FacadeInvoked {
		t.Errorf("facade_invoked = true, want false on a fail-closed refusal")
	}
}

// TestLateBoundHandlerRefusesOversizedBodyWithoutMiddlewareChain proves
// the body cap cannot be bypassed through the bind window. The body is
// far larger than BodySizeMaxBytes; without bodySizeCap the adapter's
// io.ReadAll(r.Body) is unbounded, so a caller could stream arbitrary
// bytes into core memory before any validation runs.
func TestLateBoundHandlerRefusesOversizedBodyWithoutMiddlewareChain(t *testing.T) {
	adapter, facade := bindWindowAdapter(t)

	h := NewLateBoundHandler()
	h.SetAdapter(adapter)

	oversized := bindWindowTurnBody(t, "harden-069-h01-body", strings.Repeat("A", defaultConfig().BodySizeMaxBytes+4096))
	if len(oversized) <= defaultConfig().BodySizeMaxBytes {
		t.Fatalf("test body is %d bytes, must exceed the %d-byte cap to be adversarial", len(oversized), defaultConfig().BodySizeMaxBytes)
	}

	rr := postThroughLateBound(t, h, auth.Session{
		UserID: "u-shared",
		Source: auth.SessionSourceSharedToken,
	}, oversized)

	if facade.calls != 0 {
		t.Errorf("facade.calls = %d, want 0 — an over-cap body reached the capability layer through the bind window", facade.calls)
	}
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 — an over-cap body must not be read by an adapter whose bodySizeCap is not installed. body=%s", rr.Code, rr.Body.String())
	}
}

// TestLateBoundHandlerServesOnceMiddlewareChainIsInstalled is the
// counterpart that keeps the fail-closed rule from degenerating into
// "always 503": with BOTH the chain and the adapter installed, a
// properly scoped caller is served normally and the facade IS reached.
func TestLateBoundHandlerServesOnceMiddlewareChainIsInstalled(t *testing.T) {
	adapter, facade := bindWindowAdapter(t)

	h := NewLateBoundHandler()
	h.SetMiddleware(PreFacadeChain(defaultConfig()))
	h.SetAdapter(adapter)

	rr := postThroughLateBound(t, h, auth.Session{
		UserID: "u-scoped",
		Source: auth.SessionSourcePerUserToken,
		Scopes: []string{"assistant:turn"},
	}, bindWindowTurnBody(t, "harden-069-h01-ok", "what is the weather"))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for a fully bound handler and a correctly scoped caller. body=%s", rr.Code, rr.Body.String())
	}
	if facade.calls != 1 {
		t.Errorf("facade.calls = %d, want 1 — a fully bound handler must still serve", facade.calls)
	}
}
