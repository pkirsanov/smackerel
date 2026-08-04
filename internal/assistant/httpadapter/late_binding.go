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
// Binding is FAIL-CLOSED (spec 069 harden HARDEN-069-H01): the
// adapter is served only when the SCOPE-2 pre-facade middleware chain
// is ALSO installed. Facade wiring runs in a background goroutine
// while the chi listener is already serving POST /api/assistant/turn,
// so a partially bound handler is reachable by real traffic; serving
// the bare adapter there would skip the assistant:turn scope gate,
// the per-user rate limit, and the body-size cap that bounds the
// adapter's io.ReadAll. A partially bound handler refuses instead.
//
// Until BOTH are installed, requests receive HTTP 503 with the
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
// concurrent access; calling with nil clears the chain, which returns
// the handler to the fail-closed 503 state.
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
	c := h.chain.Load()
	if a == nil || c == nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"schema_version":"v1","transport":"web","status":"unavailable","error_cause":"assistant_http_not_ready","facade_invoked":false}`))
		return
	}
	(*c)(a).ServeHTTP(w, r)
}
