// Package api — trusted-proxy-gated real-IP middleware.
//
// BUG-020-005, F-SEC-R30-001 (sweep-2026-05-23-r30 round 15):
// chi.middleware.RealIP applied unconditionally allows any caller to
// rewrite r.RemoteAddr via X-Forwarded-For / X-Real-IP / True-Client-IP
// headers, which trivially bypasses httprate.LimitByIP per-IP rate
// limits (spec 020 R-004) and lets attackers forge slog `remote_addr`
// fields in webAuthMiddleware / bearerAuthMiddleware warning lines.
//
// This middleware honours those headers ONLY when the connecting TCP
// peer is in the SST-configured runtime.trusted_proxies CIDR allowlist
// (Dependencies.TrustedProxies). When the allowlist is empty (the
// default for the committed dev and deploy bundles) the middleware is
// a no-op identity and r.RemoteAddr keeps its raw value.
//
// Deploy adapters that DO front the stack with a known reverse proxy
// (e.g., host Caddy on a tailnet bind) populate trusted_proxies in
// their operator-private overlay.

package api

import (
	"log/slog"
	"net"
	"net/http"
	"strings"
)

// trustedProxyRealIPMiddleware returns an http middleware that conditionally
// overwrites r.RemoteAddr with a client-supplied forwarded-IP header value
// (True-Client-IP, X-Real-IP, or the first comma-separated entry of
// X-Forwarded-For, in that order). The overwrite happens ONLY when the
// connecting TCP peer's IP is inside one of the CIDRs listed in
// deps.TrustedProxies.
//
// When deps.TrustedProxies is empty, the returned middleware is an identity
// pass-through — no forwarded header is ever honoured. This is the
// secure-by-default contract; per-IP rate limits and slog `remote_addr`
// fields key on the raw TCP peer.
//
// CIDR parsing happens once at construction time. Invalid CIDR entries
// are logged at startup (slog.Error) and dropped; the middleware never
// silently grants trust on a malformed configuration entry.
func (deps *Dependencies) trustedProxyRealIPMiddleware() func(http.Handler) http.Handler {
	if len(deps.TrustedProxies) == 0 {
		// Empty allowlist → identity middleware. Forwarded headers are
		// ignored regardless of which TCP peer sends them.
		return func(next http.Handler) http.Handler { return next }
	}

	trustedNets := make([]*net.IPNet, 0, len(deps.TrustedProxies))
	for _, entry := range deps.TrustedProxies {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		_, ipNet, err := net.ParseCIDR(entry)
		if err != nil {
			// Fail-loud: an operator typo MUST be visible in logs and
			// MUST NOT silently grant trust. Drop the offending entry
			// and continue with the rest (the remaining entries are
			// still safe; this entry simply produces no trust).
			slog.Error("trusted_proxies CIDR parse failed; entry ignored",
				"entry", entry,
				"error", err.Error())
			continue
		}
		trustedNets = append(trustedNets, ipNet)
	}

	// If every entry was malformed, behave as if the allowlist were
	// empty (secure-by-default).
	if len(trustedNets) == 0 {
		return func(next http.Handler) http.Handler { return next }
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if peerIP := tcpPeerIP(r.RemoteAddr); peerIP != nil && ipInAnyNet(peerIP, trustedNets) {
				if forwarded := firstForwardedClientIP(r); forwarded != "" {
					// chi.middleware.RealIP overwrites r.RemoteAddr to
					// the bare IP string. We follow the same shape so
					// httprate.LimitByIP and downstream consumers see
					// the forwarded client IP unchanged.
					r.RemoteAddr = forwarded
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// tcpPeerIP returns the IP portion of an http.Request.RemoteAddr value
// ("host:port"), or nil if the host portion cannot be parsed.
func tcpPeerIP(remoteAddr string) net.IP {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		// Some test harnesses set RemoteAddr without a port; treat the
		// whole string as the host.
		host = remoteAddr
	}
	return net.ParseIP(host)
}

// ipInAnyNet reports whether ip is contained in any of the given networks.
func ipInAnyNet(ip net.IP, nets []*net.IPNet) bool {
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// firstForwardedClientIP returns the first non-empty, parseable client-IP
// value from True-Client-IP, X-Real-IP, or the leftmost entry of
// X-Forwarded-For (in that order). Returns "" if none parses.
//
// Header preference order mirrors chi.middleware.RealIP so an operator
// migrating from the chi default sees the same client-IP semantics
// without surprises.
func firstForwardedClientIP(r *http.Request) string {
	if v := strings.TrimSpace(r.Header.Get("True-Client-IP")); v != "" {
		if ip := net.ParseIP(v); ip != nil {
			return ip.String()
		}
	}
	if v := strings.TrimSpace(r.Header.Get("X-Real-IP")); v != "" {
		if ip := net.ParseIP(v); ip != nil {
			return ip.String()
		}
	}
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		// X-Forwarded-For is a comma-separated chain; the leftmost
		// entry is the original client per the de-facto convention
		// (and per chi.middleware.RealIP).
		if comma := strings.IndexByte(v, ','); comma >= 0 {
			v = v[:comma]
		}
		v = strings.TrimSpace(v)
		if v != "" {
			if ip := net.ParseIP(v); ip != nil {
				return ip.String()
			}
		}
	}
	return ""
}
