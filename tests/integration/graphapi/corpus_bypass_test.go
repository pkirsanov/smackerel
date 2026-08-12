//go:build integration

// TP-03-03 — spec 108 SCOPE-03 (SCN-108-G02, design T5).
//
// Proves the two documented `RequireScope` bypasses under ENFORCE: a
// shared-token session and a bootstrap session reach a gated corpus route
// while an ungranted per-user principal is refused on the same route.
//
// The distinguishing requirement of this row is that the bypass is ASSERTED,
// not assumed. "The request was not 403" is far too weak: a route that was
// never gated, a stage that silently fell back to OBSERVE, or a gate mounted
// on the wrong group would all produce a non-403 too. So each bypass arm
// additionally asserts that `smackerel_auth_scope_check_bypassed_total`
// incremented on the MATCHING `source` label — that counter is written on
// exactly one line inside the `RequireScope` source switch, so an increment is
// proof the request travelled the documented branch rather than missing the
// gate entirely.
//
// The ungranted per-user arm is the control that makes the counter assertion
// meaningful: it must be refused AND must leave the bypass counter untouched.
// Without it, a `RequireScope` that bypassed unconditionally would satisfy
// every positive assertion here.

package graphapi_integration

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/smackerel/smackerel/internal/auth"
	"github.com/smackerel/smackerel/internal/metrics"
)

func bypassCount(source string) float64 {
	return testutil.ToFloat64(metrics.AuthScopeCheckBypassed.WithLabelValues(source))
}

// TestIntegration_CorpusGrantEnforce_SharedTokenBypassIsAsserted_TP_03_03
// drives the shared-token bypass through the real router over HTTP.
func TestIntegration_CorpusGrantEnforce_SharedTokenBypassIsAsserted_TP_03_03(t *testing.T) {
	stack, privateHex := newCorpusEnforceStack(t, "TP0303")

	table := corpusEnforceAllRoutes()
	groups := corpusEnforceClosedSet(t, table)
	base := stack.serve(t, true)

	ungrantedBearer := mintCorpusToken(t, privateHex, "tp0303-ungranted", []string{corpusOtherScope})

	before := bypassCount("shared_token")
	sharedRequests := 0
	refusals := 0

	for _, group := range groups {
		for _, rt := range table[group] {
			label := string(group) + " " + rt.method + " " + rt.path

			// Shared-token session: the documented bypass. corpusOperatorToken
			// is the stack's AuthToken and ProductionSharedTokenFallbackEnabled
			// is on, so bearerAuthMiddleware classifies it SessionSourceSharedToken.
			sResp, sBody := corpusDo(t, base, corpusOperatorToken, rt)
			assertNotRefusedByCorpusGate(t, "ENFORCE/shared/"+label, sResp, sBody)
			sharedRequests++

			// Control: same route, same stage, a principal that genuinely
			// lacks the grant. If this is NOT refused, the route is not gated
			// and the bypass arm above proves nothing.
			uResp, uBody := corpusDo(t, base, ungrantedBearer, rt)
			if !isCorpusGateDenial(uResp, uBody) {
				t.Fatalf("ENFORCE/%s: the ungranted control was NOT refused (status=%d). The shared-token arm on this route is therefore vacuous — a non-403 there would prove nothing about the bypass. body=%s",
					label, uResp.StatusCode, string(uBody))
			}
			refusals++
		}
	}

	if sharedRequests == 0 {
		t.Fatal("no shared-token requests were issued; the route table is empty and this test asserts nothing")
	}
	if refusals != sharedRequests {
		t.Fatalf("arm accounting mismatch: shared=%d refusals=%d; every route must contribute both arms", sharedRequests, refusals)
	}

	delta := bypassCount("shared_token") - before
	if delta < float64(sharedRequests) {
		t.Errorf("smackerel_auth_scope_check_bypassed_total{source=\"shared_token\"} rose by %.0f across %d shared-token requests; want at least %d. "+
			"A non-403 alone does not prove the bypass was taken — the counter is written on exactly one line inside the RequireScope source switch, so a short increment means those requests never reached the gate at all",
			delta, sharedRequests, sharedRequests)
	}
}

// TestIntegration_CorpusGrantEnforce_UngrantedDoesNotIncrementBypass_TP_03_03
// is the counter's own control. It proves the bypass counter tracks the bypass
// PATH rather than merely counting requests: a refused principal must not move
// it.
func TestIntegration_CorpusGrantEnforce_UngrantedDoesNotIncrementBypass_TP_03_03(t *testing.T) {
	stack, privateHex := newCorpusEnforceStack(t, "TP0303NEG")

	table := corpusEnforceAllRoutes()
	groups := corpusEnforceClosedSet(t, table)
	base := stack.serve(t, true)

	ungrantedBearer := mintCorpusToken(t, privateHex, "tp0303-neg", []string{corpusOtherScope})

	beforeShared := bypassCount("shared_token")
	beforeBootstrap := bypassCount("bootstrap")

	refused := 0
	for _, group := range groups {
		for _, rt := range table[group] {
			resp, body := corpusDo(t, base, ungrantedBearer, rt)
			if !isCorpusGateDenial(resp, body) {
				t.Fatalf("ENFORCE/%s %s: ungranted principal was not refused (status=%d)", rt.method, rt.path, resp.StatusCode)
			}
			refused++
		}
	}
	if refused == 0 {
		t.Fatal("no routes were exercised; this control asserts nothing")
	}

	if got := bypassCount("shared_token") - beforeShared; got != 0 {
		t.Errorf("shared_token bypass counter moved by %.0f while only REFUSED per-user requests were issued; the counter is not tracking the bypass branch", got)
	}
	if got := bypassCount("bootstrap") - beforeBootstrap; got != 0 {
		t.Errorf("bootstrap bypass counter moved by %.0f while only refused per-user requests were issued", got)
	}
}

// TestIntegration_CorpusGrantEnforce_BootstrapBypassIsAsserted_TP_03_03 covers
// the second documented bypass.
//
// It exercises `auth.RequireScope` directly rather than over HTTP, and that is
// deliberate: no production code path constructs a SessionSourceBootstrap
// session for an HTTP request. Every non-test reference to it is a CONSUMER
// (`scope_middleware.go`, `corpus_grant_gate.go`, `cmd/core/wiring.go`), so
// there is nothing to drive through the router. Asserting it here keeps the
// branch covered without pretending an end-to-end path exists that does not.
func TestIntegration_CorpusGrantEnforce_BootstrapBypassIsAsserted_TP_03_03(t *testing.T) {
	before := bypassCount("bootstrap")

	reached := false
	handler := auth.RequireScope(auth.GrantGlobalCorpusRead)(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			reached = true
			w.WriteHeader(http.StatusOK)
		}),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/search", nil)
	// A bootstrap session carries NO scopes — that is the whole point: it must
	// pass on Source alone, not because it happens to hold corpus:read.
	req = req.WithContext(auth.WithSession(req.Context(), auth.Session{
		UserID: "bootstrap",
		Source: auth.SessionSourceBootstrap,
	}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !reached {
		t.Fatalf("bootstrap session did not reach the handler (status=%d); the documented bootstrap bypass is broken", rec.Code)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("bootstrap session got status %d, want 200", rec.Code)
	}
	if got := bypassCount("bootstrap") - before; got != 1 {
		t.Errorf("bootstrap bypass counter rose by %.0f, want exactly 1; the pass-through must be attributable to the documented source switch rather than to a missing check", got)
	}

	// Control: the SAME middleware, the same absent scopes, a per-user source.
	// This must be refused — otherwise the bootstrap arm above would pass even
	// if RequireScope bypassed everything.
	denied := httptest.NewRecorder()
	dreq := httptest.NewRequest(http.MethodGet, "/api/search", nil)
	dreq = dreq.WithContext(auth.WithSession(dreq.Context(), auth.Session{
		UserID: "tp0303-peruser",
		Source: auth.SessionSourcePerUserToken,
	}))
	auth.RequireScope(auth.GrantGlobalCorpusRead)(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	).ServeHTTP(denied, dreq)

	if denied.Code != http.StatusForbidden {
		t.Errorf("a per-user session with no scopes got status %d, want 403; RequireScope is not enforcing, so the bootstrap bypass assertion above is vacuous", denied.Code)
	}
}
