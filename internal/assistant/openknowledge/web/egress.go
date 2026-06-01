package web

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// ErrEgressDenied is returned by EgressAllowlistTransport.RoundTrip
// when the request URL host is not in the configured allowlist.
// Callers (the SearxNG adapter, future fetchers) surface this as
// ErrProviderUnreachable to the agent loop.
var ErrEgressDenied = errors.New("openknowledge/web: egress denied by allowlist")

// EgressAllowlistTransport is an http.RoundTripper that enforces a
// host-level egress allowlist before delegating to an inner
// transport. Deny-by-default (G021, G028): an empty allowlist
// rejects every request — there is NO silent fallback to allow-all.
//
// Comparison policy:
//   - Host comparison uses url.URL.Hostname() (port stripped) lowercased.
//   - Allowlist entries are exact host matches only — no wildcards in
//     v1; PKT-020-A asks spec 020 whether to layer wildcard support
//     and a network-layer firewall on top of this application-layer
//     gate.
//   - Schemes other than http/https are rejected outright (file://,
//     ftp://, etc.) so a planner-crafted URL cannot exfiltrate via
//     the filesystem or an unexpected protocol handler.
//   - Embedded userinfo (https://user:pass@host/) is respected — the
//     Hostname() extraction normalises away the userinfo so an
//     attacker cannot smuggle past the allowlist by prefixing
//     credentials.
//
// This is defence-in-depth at the application layer; the deploy
// adapter remains the canonical place for network-layer egress
// policy (firewall, container egress rules — see PKT-020-A).
type EgressAllowlistTransport struct {
	allowed map[string]struct{}
	inner   http.RoundTripper
}

// NewEgressAllowlistTransport constructs a transport. The inner
// transport MUST be non-nil; callers typically pass
// http.DefaultTransport or an http.Client.Transport.
//
// allowed entries are normalised (trimmed + lowercased) and must
// contain no scheme, path, port, or '@' character; constructor
// rejects malformed entries so a typo cannot become a silent
// allow-all.
//
// allowed may be empty — the resulting transport denies every
// request (deny-by-default).
func NewEgressAllowlistTransport(allowed []string, inner http.RoundTripper) (*EgressAllowlistTransport, error) {
	if inner == nil {
		return nil, fmt.Errorf("%w: nil inner transport", ErrInvalidConfig)
	}
	set := make(map[string]struct{}, len(allowed))
	for _, h := range allowed {
		norm := strings.ToLower(strings.TrimSpace(h))
		if norm == "" {
			return nil, fmt.Errorf("%w: empty allowlist entry", ErrInvalidConfig)
		}
		if strings.ContainsAny(norm, "/@ \t") {
			return nil, fmt.Errorf("%w: allowlist entry %q must be a bare host (no scheme, path, port, or userinfo)", ErrInvalidConfig, h)
		}
		if strings.Contains(norm, ":") {
			return nil, fmt.Errorf("%w: allowlist entry %q must not include a port (host-only match)", ErrInvalidConfig, h)
		}
		set[norm] = struct{}{}
	}
	return &EgressAllowlistTransport{allowed: set, inner: inner}, nil
}

// RoundTrip implements http.RoundTripper.
func (t *EgressAllowlistTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil || req.URL == nil {
		return nil, fmt.Errorf("%w: nil request or URL", ErrEgressDenied)
	}
	scheme := strings.ToLower(req.URL.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil, fmt.Errorf("%w: scheme %q not permitted", ErrEgressDenied, req.URL.Scheme)
	}
	host := strings.ToLower(req.URL.Hostname())
	if host == "" {
		return nil, fmt.Errorf("%w: empty host", ErrEgressDenied)
	}
	if _, ok := t.allowed[host]; !ok {
		return nil, fmt.Errorf("%w: host %q not in allowlist", ErrEgressDenied, host)
	}
	return t.inner.RoundTrip(req)
}
