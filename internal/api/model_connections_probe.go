// Spec 096 SCOPE-06 — the production ConnectionProbe: a bounded HTTP
// reachability + auth-class probe against a connection's configured endpoint,
// used by POST /v1/admin/model-connections/{id}/test.
//
// TRUTHFUL + FAIL-LOUD. The probe NEVER reports a false ok and NEVER substitutes
// Ollama: a connect-class failure is `unreachable`, a deadline is `timeout`, an
// HTTP 401/403 is `auth_failed`, a 5xx is `unreachable`, and only a non-rejected
// reachable response is `ok`. The handler additionally treats any non-ok /
// malformed result as a failure, so a probe defect can never surface as success.
//
// SCOPE BOUNDARY (C7). This generic probe verifies endpoint reachability +
// simple bearer/api-key auth class against the connection's configured
// base_url / endpoint. Provider-specific credential semantics (Anthropic's
// x-api-key header, Bedrock AWS SigV4, Vertex service-account exchange) and the
// providers' default endpoints are validated by the DEFERRED self-hosted
// bubbles.devops e2e leg (real reachability + real credentials), not in-repo. A
// hosted connection with no configured endpoint is reported `unreachable` here
// (honest: nothing to probe) rather than a fabricated ok.
package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/smackerel/smackerel/internal/assistant/openknowledge/connstore"
	"github.com/smackerel/smackerel/internal/config"
)

// httpDoer is the minimal HTTP surface the probe needs (injected so the wiring
// passes a real *http.Client and a future test can drive it with a fake).
type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// HTTPConnectionProbe is the production ConnectionProbe.
type HTTPConnectionProbe struct {
	client  httpDoer
	timeout time.Duration
}

// NewHTTPConnectionProbe builds the probe. timeout (> 0) bounds each probe; it
// comes from the SST llm.discovery.per_provider_timeout_ms.
func NewHTTPConnectionProbe(client httpDoer, timeout time.Duration) *HTTPConnectionProbe {
	return &HTTPConnectionProbe{client: client, timeout: timeout}
}

// Probe runs the live reachability + auth-class probe and returns a TRUTHFUL
// typed result. The decrypted secret is presented best-effort for simple
// bearer/api-key kinds so an unauthorized response surfaces as auth_failed; the
// secret is never logged.
func (p *HTTPConnectionProbe) Probe(ctx context.Context, conn config.ModelConnection, secret map[string]string) ProbeResult {
	probeURL := probeURLFor(conn)
	if probeURL == "" {
		return ProbeResult{Outcome: connstore.TestOutcomeFailed, Detail: ProbeDetailUnreachable}
	}
	pctx := ctx
	if p.timeout > 0 {
		var cancel context.CancelFunc
		pctx, cancel = context.WithTimeout(ctx, p.timeout)
		defer cancel()
	}
	req, err := http.NewRequestWithContext(pctx, http.MethodGet, probeURL, nil)
	if err != nil {
		return ProbeResult{Outcome: connstore.TestOutcomeFailed, Detail: ProbeDetailUnreachable}
	}
	if key := strings.TrimSpace(secret["api_key"]); key != "" {
		switch conn.Kind {
		case config.ModelConnectionKindAnthropic:
			req.Header.Set("x-api-key", key)
		default:
			req.Header.Set("Authorization", "Bearer "+key)
		}
	}
	resp, err := p.client.Do(req)
	if err != nil {
		if errors.Is(pctx.Err(), context.DeadlineExceeded) {
			return ProbeResult{Outcome: connstore.TestOutcomeFailed, Detail: ProbeDetailTimeout}
		}
		return ProbeResult{Outcome: connstore.TestOutcomeFailed, Detail: ProbeDetailUnreachable}
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return ProbeResult{Outcome: connstore.TestOutcomeFailed, Detail: ProbeDetailAuthFailed}
	case resp.StatusCode >= 500:
		return ProbeResult{Outcome: connstore.TestOutcomeFailed, Detail: ProbeDetailUnreachable}
	default:
		// 2xx / 3xx / 4xx-other: the endpoint is reachable and did not reject
		// the credential as unauthorized — reachability + auth-class ok.
		return ProbeResult{Outcome: connstore.TestOutcomeOK}
	}
}

// probeURLFor derives the probe URL from the connection's non-secret params.
// Ollama probes the keyless `/api/tags`; a hosted kind probes its configured
// base_url / endpoint. An absent endpoint yields "" (the caller reports
// unreachable — never a fabricated ok).
func probeURLFor(conn config.ModelConnection) string {
	base := ""
	for _, key := range []string{"base_url", "endpoint"} {
		if v, ok := conn.Params[key]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				base = strings.TrimSpace(s)
				break
			}
		}
	}
	if base == "" {
		return ""
	}
	if conn.Kind == config.ModelConnectionKindOllama {
		return strings.TrimRight(base, "/") + "/api/tags"
	}
	return base
}
