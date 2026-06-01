package httpadapter

import (
	"net/http"
	"sync/atomic"
)

// LateBoundHandler is an http.Handler placeholder whose backing
// *HTTPAdapter is installed after construction. cmd/core wires this
// into api.Dependencies BEFORE api.NewRouter runs (because the route
// table is registered eagerly) and calls SetAdapter from
// wireAssistantFacade once the facade has been constructed.
//
// Until SetAdapter is called, requests receive HTTP 503 with the
// stable error code "assistant_http_not_ready" so callers can
// distinguish "not yet bound" from "disabled by SST".
// middlewareChain is the function-pointer type stored in the
// LateBoundHandler.chain atomic. Named so atomic.Pointer can hold a
// typed cell rather than the bare function value (which is not a
// comparable type).
type middlewareChain = func(http.Handler) http.Handler

type LateBoundHandler struct {
	adapter atomic.Pointer[HTTPAdapter]
	chain   atomic.Pointer[middlewareChain]
}

// NewLateBoundHandler returns an empty late-bound handler.
func NewLateBoundHandler() *LateBoundHandler { return &LateBoundHandler{} }

// SetAdapter installs the backing adapter. Safe for concurrent
// access with in-flight requests.
func (h *LateBoundHandler) SetAdapter(a *HTTPAdapter) { h.adapter.Store(a) }

// SetMiddleware installs the SCOPE-2 pre-facade middleware chain
// (built by PreFacadeChain) that wraps the adapter. Safe for
// concurrent access; calling with nil clears the chain.
func (h *LateBoundHandler) SetMiddleware(chain func(http.Handler) http.Handler) {
	if chain == nil {
		h.chain.Store(nil)
		return
	}
	c := middlewareChain(chain)
	h.chain.Store(&c)
}

// ServeHTTP implements http.Handler.
func (h *LateBoundHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a := h.adapter.Load()
	if a == nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"schema_version":"v1","transport":"web","status":"unavailable","error_cause":"assistant_http_not_ready","facade_invoked":false}`))
		return
	}
	var handler http.Handler = a
	if c := h.chain.Load(); c != nil {
		handler = (*c)(a)
	}
	handler.ServeHTTP(w, r)
}
