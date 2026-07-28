package graphapi

// privacy.go — BUG-080-001 SCOPE-02 durable-cache privacy contract.
//
// scopes.md SCOPE-02 "Security And Privacy" requires: "Authenticated
// responses and cursors use private/no-store semantics and never enter
// durable browser storage." design.md "Security And Privacy" restates
// the same rule from the client side: "authenticated Graph API
// responses are network-only and excluded from caches."
//
// Graph responses carry private knowledge-graph content — labels,
// topology, counts, and opaque cursors. Without an explicit no-store
// directive a shared proxy or a browser cache may DURABLY retain that
// content, and a later unauthenticated request can be served it from
// cache without ever reaching the auth boundary. That is also exactly
// what the repository's "NO CACHES AS DATA SOURCE" policy forbids.
//
// This file is the SINGLE definition of the header contract. Both
// enforcement points import it, so the value can never drift between
// them:
//
//	1. graphapi's two response writers — writeJSON (success) and
//	   WriteError (every typed error; WriteAPIError and
//	   GraphCapability.WriteDisabled both funnel through it). Every
//	   graph family, list and detail, success and error and the
//	   disabled 503, exits through exactly one of those two functions,
//	   so stamping them covers the whole graph response surface without
//	   touching a single handler.
//	2. internal/api's global securityHeadersMiddleware, which already
//	   emits a bare `no-store` on every response and therefore covers
//	   the pre-handler 401 that bearerAuthMiddleware writes ABOVE the
//	   graph route group (see the e2e test for the live proof of both).

import "net/http"

const (
	// CacheControlHeader is the response header carrying the contract.
	CacheControlHeader = "Cache-Control"

	// CacheControlPrivateNoStore is the exact directive set graph
	// responses advertise. `private` forbids a shared cache (proxy,
	// CDN) from storing the response at all; `no-store` forbids ANY
	// cache — shared or private, memory or disk — from retaining it.
	// Together they are the scopes.md "private/no-store semantics".
	//
	// This value intentionally REPLACES the bare `no-store` that the
	// global securityHeadersMiddleware set earlier in the chain: the
	// last Header().Set before WriteHeader wins, and the graph API
	// owns the stricter contract for its own private content rather
	// than inheriting it from an unrelated global middleware that a
	// future edit could weaken without any graph test noticing.
	CacheControlPrivateNoStore = "private, no-store"
)

// SetPrivateNoStore stamps the private/no-store contract on w. It MUST
// be called before WriteHeader — once the status line is written the
// header map is frozen and the directive would be silently dropped.
func SetPrivateNoStore(w http.ResponseWriter) {
	w.Header().Set(CacheControlHeader, CacheControlPrivateNoStore)
}
