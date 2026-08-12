//go:build integration

package graphapi_integration

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/smackerel/smackerel/internal/auth"
)

// Spec 108 SCOPE-04 — caller remediation rows that can be proven against the
// real router: TP-04-03 (token rotation, SCN-108-E02), TP-04-04 (extension
// tracks its principal, SCN-108-E03) and TP-04-06 (bootstrap canary).

// TestIntegration_CorpusGrant_RotationGrantsDailyUserAccess_TP_04_03 is
// SCN-108-E02: the remediation for a denied daily user is a TOKEN ROTATION, not
// a feature-flag change.
//
// That distinction is the whole point of the row. If the only way to restore a
// user were flipping the stage back, enforcement would be all-or-nothing for
// the entire deployment and the OBSERVE window would be the only safe state.
// Proving the same principal can be fixed individually, while the stage stays
// ENFORCE, is what makes per-principal remediation real.
func TestIntegration_CorpusGrant_RotationGrantsDailyUserAccess_TP_04_03(t *testing.T) {
	stack, privateHex := newCorpusEnforceStack(t, "CORPUSROTATE")

	// ONE router, ENFORCE, built once and never rebuilt. Every assertion below
	// runs against this same instance, so "no feature-flag change" is a
	// property of the test's construction rather than a claim in a comment.
	base := stack.serve(t, true)

	const dailyUser = "tp0403-daily-user"
	probe := corpusEnforceRoute{method: http.MethodGet, path: "/api/recent?limit=1"}

	// BEFORE — the daily user's real grant set: a genuine product scope that is
	// simply not corpus. Not an empty scope list, which would prove only that
	// an unscoped token is refused.
	before := mintCorpusToken(t, privateHex, dailyUser, []string{corpusOtherScope})
	resp, body := corpusDo(t, base, before, probe)
	if !isCorpusGateDenial(resp, body) {
		t.Fatalf("the daily user was NOT denied under ENFORCE (status=%d); without a real denial the rotation below proves nothing", resp.StatusCode)
	}
	assertCorpusDenialIsClean(t, "tp0403/before-rotation", resp, body)

	// AFTER — the SAME principal id, rotated to carry corpus:read ALONGSIDE its
	// existing grant. Keeping the original scope is deliberate: a rotation that
	// silently dropped it would "fix" corpus access by breaking the annotation
	// surface, which is a regression disguised as a remediation.
	after := mintCorpusToken(t, privateHex, dailyUser, []string{corpusOtherScope, auth.GrantGlobalCorpusRead})
	resp, body = corpusDo(t, base, after, probe)
	assertNotRefusedByCorpusGate(t, "tp0403/after-rotation", resp, body)

	// The pre-rotation token must STILL be refused. If the router keyed access
	// off the user id rather than the presented token's claims, the old bearer
	// would start working the moment any token for that user carried the grant
	// — a privilege leak that the happy path above cannot detect.
	resp, body = corpusDo(t, base, before, probe)
	if !isCorpusGateDenial(resp, body) {
		t.Errorf("the PRE-rotation token was accepted after a new token was issued for the same principal (status=%d); "+
			"authority must come from the presented token's claims, not from the user id", resp.StatusCode)
	}
}

// TestIntegration_CorpusGrant_ExtensionTracksItsPrincipal_TP_04_04 is
// SCN-108-E03: the browser extension has NO grant of its own. Its outcome is
// exactly its principal's outcome.
//
// The risk this guards is an extension-specific carve-out — a special scope, or
// a header the middleware treats as privileged — which would let the extension
// read the corpus for a user who cannot. Because the extension authenticates as
// the same per-user bearer as the PWA, the assertion is that the two are
// INDISTINGUISHABLE, byte for byte.
func TestIntegration_CorpusGrant_ExtensionTracksItsPrincipal_TP_04_04(t *testing.T) {
	stack, privateHex := newCorpusEnforceStack(t, "CORPUSEXT")
	base := stack.serve(t, true)

	probe := corpusEnforceRoute{method: http.MethodGet, path: "/api/recent?limit=1"}

	granted := mintCorpusToken(t, privateHex, "tp0404-granted", []string{auth.GrantGlobalCorpusRead})
	ungranted := mintCorpusToken(t, privateHex, "tp0404-ungranted", []string{corpusOtherScope})

	// Ungranted principal: the PWA request and the extension request must
	// produce the SAME refusal. Comparing bodies rather than just status codes
	// is what would catch an extension-specific branch that refuses with a
	// different envelope — or admits with a different one.
	pwaResp, pwaBody := corpusDo(t, base, ungranted, probe)
	extResp, extBody := corpusDoAs(t, base, ungranted, probe, "smackerel-extension")

	if !isCorpusGateDenial(pwaResp, pwaBody) {
		t.Fatalf("the ungranted principal was not refused on the PWA path (status=%d); the comparison below would be vacuous", pwaResp.StatusCode)
	}
	if extResp.StatusCode != pwaResp.StatusCode {
		t.Errorf("extension request returned %d but the same principal via the PWA path returned %d; "+
			"the extension outcome must track its principal exactly", extResp.StatusCode, pwaResp.StatusCode)
	}
	if !bytes.Equal(extBody, pwaBody) {
		t.Errorf("extension and PWA denials differ for the SAME principal.\n  pwa: %s\n  ext: %s", string(pwaBody), string(extBody))
	}

	// Granted principal: the extension must be admitted too — the guard cuts
	// both ways. An extension blanket-denied regardless of its principal would
	// also "track" nothing.
	extResp, extBody = corpusDoAs(t, base, granted, probe, "smackerel-extension")
	assertNotRefusedByCorpusGate(t, "tp0404/extension-granted", extResp, extBody)

	// No extension-specific grant may exist in the closed scope-surface set. A
	// behavioural match today could still be undone tomorrow by adding one, so
	// the registry is asserted directly.
	for _, surface := range auth.RegisteredScopeSurfaces {
		if surface == "extension-corpus" || surface == "extension:corpus" {
			t.Errorf("an extension-specific corpus surface %q is registered; the extension must inherit its principal's grants, not hold its own", surface)
		}
	}
}

// TestIntegration_CorpusGrant_BootstrapFixtureCanary_TP_04_06 is the Scope 04
// canary over the shared session/token bootstrap fixtures.
//
// TP-04-03's negative case depends on a daily-user fixture that genuinely lacks
// corpus:read. If a future change widened the shared fixture's grant set, that
// negative case would silently stop testing anything while still passing. This
// asserts the fixture's shape directly, independently runnable.
func TestIntegration_CorpusGrant_BootstrapFixtureCanary_TP_04_06(t *testing.T) {
	stack, privateHex := newCorpusEnforceStack(t, "CORPUSBOOTCANARY")
	base := stack.serve(t, true)
	probe := corpusEnforceRoute{method: http.MethodGet, path: "/api/recent?limit=1"}

	// 1. The daily-user fixture still resolves WITHOUT corpus:read.
	daily := mintCorpusToken(t, privateHex, "tp0406-daily", []string{corpusOtherScope})
	resp, body := corpusDo(t, base, daily, probe)
	if !isCorpusGateDenial(resp, body) {
		t.Errorf("the daily-user fixture was NOT refused (status=%d) — it has acquired corpus:read somewhere, "+
			"which would make TP-04-03's negative case pass while testing nothing", resp.StatusCode)
	}

	// 2. `dailyUserGrants` itself must not have been widened. The behavioural
	// check above is downstream of this; asserting the roster directly names
	// the cause rather than the symptom.
	for _, g := range auth.GrantsForRole(auth.RoleDailyUser) {
		if g == auth.GrantGlobalCorpusRead {
			t.Errorf("dailyUserGrants now contains %s — §18 decision 2 makes that permanently forbidden, "+
				"and it would silently grant every daily user corpus access", auth.GrantGlobalCorpusRead)
		}
	}

	// 3. The operator fixture still works — the canary must not pass simply
	// because everything is refused.
	resp, body = corpusDo(t, base, corpusOperatorToken, probe)
	assertNotRefusedByCorpusGate(t, "tp0406/operator", resp, body)
}

// corpusDoAs drives a request with an extra client-identifying header, so the
// extension path can be exercised as the extension without a second helper.
func corpusDoAs(t *testing.T, base, bearer string, rt corpusEnforceRoute, client string) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest(rt.method, base+rt.path, nil)
	if err != nil {
		t.Fatalf("build %s %s: %v", rt.method, rt.path, err)
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("X-Smackerel-Client", client)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", rt.method, rt.path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body for %s %s: %v", rt.method, rt.path, err)
	}
	return resp, body
}
