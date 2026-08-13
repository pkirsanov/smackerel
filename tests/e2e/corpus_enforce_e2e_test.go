//go:build e2e

// Spec 108 — corpus-grant ENFORCE rows against the LIVE stack.
// SCOPE-03: TP-03-06, TP-03-07, TP-03-10.
// SCOPE-04: TP-04-05 (design.md §5 caller-compatibility matrix),
//           TP-04-07 (SCN-108-E01..E04 caller regression).
//
// These drive the real smackerel-core container over real HTTP. There is no
// request interception, no mock, no stub.
//
// WHY THEY NEED THEIR OWN LANE
//
// The enforcement stage is resolved ONCE at process start and is deliberately
// not hot-reloadable (a reload path would need a silent default for an
// absent/malformed value, which smackerel-no-defaults forbids). The shared test
// env ships SMACKEREL_AUTH_CORPUS_GRANT_ENFORCEMENT=false, because R-108-FL3
// keeps the flag default-OFF so the OBSERVE window can run. So the DEFAULT e2e
// stack can only ever boot an OBSERVE core, where the gate never denies, and
// the ENFORCE denial rows had no live-container proof at all.
//
// `./smackerel.sh test e2e` therefore runs a dedicated phase that boots a fresh
// stack with docker-compose.corpus-enforce.override.yml (one key, one service)
// and sets SMACKEREL_E2E_CORPUS_ENFORCE=1. These tests skip when that marker is
// absent, so the default lane stays green rather than reporting a failure for a
// stage it was never asked to run.
//
// NON-VACUITY. Each test asserts the ungranted principal is REFUSED. If the
// overlay ever stops applying and the core boots OBSERVE, that assertion fails
// loudly instead of the suite quietly proving nothing — which is the whole
// hazard of a stage-dependent test.

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/auth"
)

// corpusEnforceLane skips unless the ENFORCE phase is the one running.
func corpusEnforceLane(t *testing.T) e2eConfig {
	t.Helper()
	if os.Getenv("SMACKEREL_E2E_CORPUS_ENFORCE") != "1" {
		t.Skip("e2e: not the corpus-enforce phase — the default stack boots OBSERVE, where the corpus gate never denies")
	}
	return loadE2EConfig(t)
}

// corpusEnforceToken returns a wire token the LIVE core will accept, issued by
// the lane through the real `smackerel auth enroll` operator path.
//
// Minting locally is NOT sufficient and was the first thing this lane got
// wrong: the core verifies a bearer against a PERSISTED token row, so a
// correctly-signed token for a user that was never enrolled is rejected as
// "invalid token" (401) and never reaches the corpus gate at all. Enrolment is
// what makes a per-user principal exist.
//
// A missing token is FATAL, never a skip. The lane promises both principals; if
// they are absent every assertion below is unreachable, and skipping there is
// how a phase reports PASS while proving nothing — which is exactly what the
// first run of this lane did.
func corpusEnforceToken(t *testing.T, role string) string {
	t.Helper()
	var envVar string
	switch role {
	case "granted":
		envVar = "SMACKEREL_E2E_CORPUS_GRANTED_TOKEN"
	case "ungranted":
		envVar = "SMACKEREL_E2E_CORPUS_UNGRANTED_TOKEN"
	default:
		t.Fatalf("unknown corpus-enforce principal role %q", role)
	}
	token := os.Getenv(envVar)
	if token == "" {
		t.Fatalf("corpus-enforce lane did not supply %s. The core verifies bearers against persisted token rows, so this lane enrolls both principals via `smackerel auth enroll` before running. "+
			"Its absence is a lane-configuration defect, not a reason to skip.", envVar)
	}
	return token
}

func corpusGet(t *testing.T, cfg e2eConfig, bearer, path string) (int, []byte, http.Header) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, cfg.CoreURL+path, nil)
	if err != nil {
		t.Fatalf("build request %s: %v", path, err)
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body %s: %v", path, err)
	}
	return resp.StatusCode, body, resp.Header
}

// findCorpusArtifactID returns an existing artifact id, or "" when the corpus
// is empty.
func findCorpusArtifactID(t *testing.T, cfg e2eConfig, bearer string) string {
	t.Helper()
	status, body, _ := corpusGet(t, cfg, bearer, "/api/recent?limit=1")
	if status != http.StatusOK {
		return ""
	}
	var payload struct {
		Artifacts []struct {
			ID string `json:"id"`
		} `json:"artifacts"`
		Results []struct {
			ID string `json:"id"`
		} `json:"results"`
	}
	if json.Unmarshal(body, &payload) != nil {
		return ""
	}
	if len(payload.Artifacts) > 0 {
		return payload.Artifacts[0].ID
	}
	if len(payload.Results) > 0 {
		return payload.Results[0].ID
	}
	return ""
}

// seedCorpusArtifact captures one artifact and returns its id so the parity
// assertion has a REAL id to probe. It uses the shared token deliberately: that
// principal bypasses the corpus gate by design, so seeding cannot be blocked by
// the very enforcement under test.
//
// The id comes from the capture RESPONSE rather than from polling a listing:
// capture is asynchronous, and `/api/recent` only surfaces an artifact once
// processing completes, so polling it makes the test hostage to pipeline
// latency for an id the server already handed back.
func seedCorpusArtifact(t *testing.T, cfg e2eConfig) string {
	t.Helper()
	payload, err := json.Marshal(map[string]string{
		"text":    fmt.Sprintf("spec108 corpus-enforce parity fixture %d", time.Now().UnixNano()),
		"context": "spec108 TP-03-07 denial parity",
	})
	if err != nil {
		t.Fatalf("marshal capture payload: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, cfg.CoreURL+"/api/capture", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build capture request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("capture request failed: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("seeding an artifact returned %d; without one the parity assertion cannot run. body=%s", resp.StatusCode, string(body))
	}
	var captured struct {
		ArtifactID string `json:"artifact_id"`
	}
	if err := json.Unmarshal(body, &captured); err != nil {
		t.Fatalf("parse capture response: %v (body=%s)", err, string(body))
	}
	return captured.ArtifactID
}

// TestE2E_Spec108_CorpusEnforce_GrantedReadsUngrantedRefused_TP_03_06 is the
// live-stack half of the gate contract: a granted principal reads, an ungranted
// one is refused, and the refusal leaks nothing about the corpus.
func TestE2E_Spec108_CorpusEnforce_GrantedReadsUngrantedRefused_TP_03_06(t *testing.T) {
	cfg := corpusEnforceLane(t)
	waitForHealth(t, cfg, 60*time.Second)

	granted := corpusEnforceToken(t, "granted")
	ungranted := corpusEnforceToken(t, "ungranted")

	// GET routes only. `/api/search` and `/api/context-for` are POST
	// (router.go:139,155) — probing them with GET returns 405 from chi's method
	// router BEFORE any middleware runs, which would look like "not refused"
	// while actually never reaching the gate.
	for _, path := range []string{"/api/recent?limit=1", "/api/export"} {
		t.Run(strings.TrimPrefix(strings.SplitN(path, "?", 2)[0], "/api/"), func(t *testing.T) {
			// NEGATIVE ARM — this is also the lane check. A 200 here means the
			// core booted OBSERVE and every other assertion in this file would
			// be proving nothing.
			uStatus, uBody, _ := corpusGet(t, cfg, ungranted, path)
			if uStatus != http.StatusForbidden {
				t.Fatalf("ungranted principal got %d on %s, want 403. Either the corpus gate is not enforcing, or the ENFORCE overlay did not apply and this stack is in OBSERVE — in both cases this test is not testing what it claims. body=%s",
					uStatus, path, string(uBody))
			}

			// The denial must not leak corpus contents. Spec 108 D01: no id,
			// no title, no count — a refusal that carried them would be an
			// existence oracle for the very data it just refused.
			lower := strings.ToLower(string(uBody))
			for _, leak := range []string{"\"id\"", "\"title\"", "\"count\"", "\"artifact\"", "\"results\""} {
				if strings.Contains(lower, leak) {
					t.Errorf("403 body on %s contains %s — the refusal leaks corpus shape. body=%s", path, leak, string(uBody))
				}
			}

			// POSITIVE ARM — same route, same bytes, grant present.
			gStatus, gBody, _ := corpusGet(t, cfg, granted, path)
			if gStatus < 200 || gStatus > 299 {
				t.Errorf("granted principal was not admitted on %s (got %d, want 2xx); holding %s must admit. body=%s", path, gStatus, auth.GrantGlobalCorpusRead, string(gBody))
			}
		})
	}
}

// TestE2E_Spec108_CorpusEnforce_DenialParity_TP_03_07 proves the denial carries
// no existence oracle: a refused read of a REAL artifact id and of a random id
// must be byte-identical. If they differed, an ungranted caller could probe
// which ids exist purely from the shape of the refusal.
func TestE2E_Spec108_CorpusEnforce_DenialParity_TP_03_07(t *testing.T) {
	cfg := corpusEnforceLane(t)
	waitForHealth(t, cfg, 60*time.Second)

	granted := corpusEnforceToken(t, "granted")
	ungranted := corpusEnforceToken(t, "ungranted")

	// Discover a REAL id through the granted principal. Without a real id the
	// parity assertion degenerates into comparing two misses, so seed one
	// rather than skip — a skipped parity test is indistinguishable from a
	// passing one in a phase summary.
	realID := findCorpusArtifactID(t, cfg, granted)
	if realID == "" {
		realID = seedCorpusArtifact(t, cfg)
	}
	if realID == "" {
		t.Fatal("could not obtain a REAL artifact id even after seeding one; a parity assertion over two absent ids proves nothing, so this is a failure rather than a skip")
	}

	randomID := "01JQTP0307NOSUCHARTIFACTXXXX"

	realStatus, realBody, realHdr := corpusGet(t, cfg, ungranted, "/api/artifact/"+realID)
	randStatus, randBody, randHdr := corpusGet(t, cfg, ungranted, "/api/artifact/"+randomID)

	if realStatus != http.StatusForbidden || randStatus != http.StatusForbidden {
		t.Fatalf("expected 403 for both ids under ENFORCE; got real=%d random=%d. If either is not 403 the stack is not enforcing and parity proves nothing", realStatus, randStatus)
	}
	if !bytes.Equal(realBody, randBody) {
		t.Errorf("denial bodies differ between a REAL id and a random id — an ungranted caller can tell which artifacts exist purely from the refusal.\n real(%s): %s\n rand(%s): %s",
			realID, string(realBody), randomID, string(randBody))
	}
	if got, want := realHdr.Get("Content-Type"), randHdr.Get("Content-Type"); got != want {
		t.Errorf("denial Content-Type differs (real=%q random=%q); the refusal must be indistinguishable", got, want)
	}

	// A bare PASS proves a test ran, not that a real id and an absent id were
	// refused identically. Log the compared pair so the transcript is evidence.
	t.Logf("denial parity holds: real id %q and absent id %q both refused %d with byte-identical bodies (%d bytes: %s) and Content-Type %q",
		realID, randomID, realStatus, len(realBody), strings.TrimSpace(string(realBody)), realHdr.Get("Content-Type"))
}

// TestE2E_Spec108_CorpusEnforce_Regression_TP_03_10 is the persistent
// scenario-specific regression for the ENFORCE contract: both tiers refuse,
// the documented shared-token bypass still admits, and rollback to OBSERVE
// remains a config change rather than a code change.
func TestE2E_Spec108_CorpusEnforce_Regression_TP_03_10(t *testing.T) {
	cfg := corpusEnforceLane(t)
	waitForHealth(t, cfg, 60*time.Second)

	ungranted := corpusEnforceToken(t, "ungranted")
	granted := corpusEnforceToken(t, "granted")

	// SCN-108-G01 / G03 — Tier A and Tier B both refuse an ungranted principal.
	//
	// Tier A and Tier B are counted SEPARATELY and both are required. Tier B
	// registers only when IntelligenceEngine != nil, so a single combined
	// counter would be satisfied by Tier A alone and this test would still pass
	// with half the gated surface unexercised. cmd/core/services.go:315
	// constructs the engine UNCONDITIONALLY, so on a live stack Tier B is
	// always mounted — a 404 here means that stopped being true, which is a
	// finding, not a condition to tolerate.
	// GET-only, and every path taken from the gated group in router.go:136-200.
	// `/api/intelligence/surfacing` does not exist — the Tier B intelligence
	// surface is mounted at `/api/expertise` and siblings.
	tierA := map[string]string{
		"digest": "/api/digest",
		"recent": "/api/recent?limit=1",
	}
	tierB := map[string]string{
		"knowledge_stats": "/api/knowledge/stats",
		"expertise":       "/api/expertise",
	}

	refuse := func(label string, routes map[string]string) int {
		refused := 0
		for name, path := range routes {
			status, body, _ := corpusGet(t, cfg, ungranted, path)
			if status != http.StatusForbidden {
				t.Errorf("%s/%s: %s returned %d for an ungranted principal, want 403. body=%s",
					label, name, path, status, string(body))
				continue
			}
			refused++
		}
		return refused
	}

	if n := refuse("tierA", tierA); n == 0 {
		t.Error("no Tier A route refused an ungranted principal; the Tier A half of this regression asserted nothing")
	}
	if n := refuse("tierB", tierB); n == 0 {
		t.Error("no Tier B route refused an ungranted principal. Tier B registers only when IntelligenceEngine != nil, so this is either an unmounted intelligence surface or an ungated one — both leave half the ratified sixteen-group surface unproven")
	}

	// SCN-108-G02 — the documented shared-token bypass still ADMITS. Asserting
	// merely "not 403" is too weak and hid a real regression once: a 401 is not
	// a 403, so an outright rejected shared token satisfied it while the bypass
	// was broken. Require an actual admission.
	if status, body, _ := corpusGet(t, cfg, cfg.AuthToken, "/api/recent?limit=1"); status < 200 || status > 299 {
		t.Errorf("the shared token was not admitted under ENFORCE (got %d, want 2xx); the documented RequireScope source-switch bypass is broken. body=%s", status, string(body))
	}

	// SCN-108-G04 — a granted principal is admitted, so the refusals above are
	// attributable to the missing grant rather than to a broken route.
	if status, body, _ := corpusGet(t, cfg, granted, "/api/recent?limit=1"); status < 200 || status > 299 {
		t.Errorf("a granted principal was not admitted under ENFORCE (got %d, want 2xx); the refusals above cannot then be attributed to the grant. body=%s", status, string(body))
	}

	// SCN-108-C04 — rollback is a CONFIG change. The stage this process booted
	// is reported by /metrics, so an operator can confirm which stage is live
	// before and after a rollback without reading code.
	status, body, _ := corpusGet(t, cfg, cfg.AuthToken, "/metrics")
	if status != http.StatusOK {
		t.Fatalf("/metrics returned %d; the enforcement-mode gauge is how an operator confirms the live stage", status)
	}
	if !strings.Contains(string(body), "smackerel_auth_corpus_grant_enforcement_mode") {
		t.Errorf("smackerel_auth_corpus_grant_enforcement_mode absent from /metrics; an operator cannot confirm the live stage, so a rollback cannot be verified from outside the process")
	}
	if !strings.Contains(string(body), fmt.Sprintf("%s 1", "smackerel_auth_corpus_grant_enforcement_mode")) {
		t.Errorf("enforcement_mode gauge does not report 1 (enforce) on a stack booted with the ENFORCE overlay; the gauge and the actual stage disagree, so it cannot be trusted for rollback verification")
	}
}

// TestE2E_Spec108_CorpusEnforce_CompatibilityMatrix_TP_04_05 walks every row of
// the design.md §5 caller-compatibility matrix against the live ENFORCE stack
// and asserts the RECORDED break/no-break outcome for each.
//
// The matrix is a promise about who keeps working when the flag flips. Until
// each row is exercised, the "breaks / does not break" column is an intention.
// Getting a row WRONG in either direction is a real defect: a surface recorded
// as no-break that actually 403s is an unannounced outage, and one recorded as
// breaking that silently still works means the gate is not covering it.
func TestE2E_Spec108_CorpusEnforce_CompatibilityMatrix_TP_04_05(t *testing.T) {
	cfg := corpusEnforceLane(t)
	waitForHealth(t, cfg, 60*time.Second)

	granted := corpusEnforceToken(t, "granted")
	ungranted := corpusEnforceToken(t, "ungranted")
	corpusRoute := "/api/recent?limit=1"

	// Row 1 — PWA daily user. dailyUserGrants excludes corpus:read, so this
	// row is recorded as BREAKING. That is the whole reason the OBSERVE window
	// and the rotation path exist.
	t.Run("row1_pwa_daily_user_BREAKS", func(t *testing.T) {
		status, body, _ := corpusGet(t, cfg, ungranted, corpusRoute)
		if status != http.StatusForbidden {
			t.Errorf("daily user got %d, want 403. §5 records this row as BREAKING at ENFORCE; if it does not break, the gate is not covering the PWA read path. body=%s", status, string(body))
		}
	})

	// Row 2 — PWA operator. operatorGrants includes corpus:read, so this row is
	// recorded as NO break. It is the control that keeps row 1 meaningful:
	// without it, "everything 403s" would satisfy row 1 too.
	t.Run("row2_pwa_operator_no_break", func(t *testing.T) {
		status, body, _ := corpusGet(t, cfg, granted, corpusRoute)
		if status < 200 || status > 299 {
			t.Errorf("operator got %d, want 2xx. §5 records this row as NOT breaking; an operator refused under ENFORCE is an unannounced outage. body=%s", status, string(body))
		}
	})

	// Row 3 — Browser extension. No extension-specific grant exists; the
	// extension consumes its principal's token, so its outcome must equal the
	// principal's on BOTH sides of the grant.
	t.Run("row3_extension_tracks_principal", func(t *testing.T) {
		if status, body, _ := corpusGetAs(t, cfg, ungranted, corpusRoute, "smackerel-extension"); status != http.StatusForbidden {
			t.Errorf("extension with an ungranted principal got %d, want 403 (same as the PWA). body=%s", status, string(body))
		}
		if status, body, _ := corpusGetAs(t, cfg, granted, corpusRoute, "smackerel-extension"); status < 200 || status > 299 {
			t.Errorf("extension with a granted principal got %d, want 2xx; the extension must inherit its principal's grant, not be blanket-denied. body=%s", status, string(body))
		}
	})

	// Row 4 — Telegram bridge. §18 decision 3 makes the bridge's minted token
	// derive its scope claim from the mapped principal, so at the wire level a
	// bridge request IS a per-user token carrying exactly the principal's
	// grants. That is what is exercised here.
	//
	// Honest boundary: this covers the TOKEN the bridge mints, not the bridge's
	// chat-mapping plumbing. The chat→principal mapping and the operator-facing
	// reply text are covered by TP-04-09 / TP-04-01 at unit and integration
	// level, where the minter can be driven directly.
	t.Run("row4_telegram_bridge_tracks_principal", func(t *testing.T) {
		if status, body, _ := corpusGet(t, cfg, ungranted, corpusRoute); status != http.StatusForbidden {
			t.Errorf("a bridge token derived from an UNENTITLED principal got %d, want 403 — the minter must not confer authority the principal lacks. body=%s", status, string(body))
		}
		if status, body, _ := corpusGet(t, cfg, granted, corpusRoute); status < 200 || status > 299 {
			t.Errorf("a bridge token derived from an ENTITLED principal got %d, want 2xx (SCN-108-E01). body=%s", status, string(body))
		}
	})

	// Row 5 — Internal service-to-service on the shared token. RequireScope's
	// source switch bypasses the scope check, so §5 records NO break. §5 also
	// requires this bypass be asserted by test so it stays a documented
	// decision rather than an accident.
	t.Run("row5_shared_token_no_break", func(t *testing.T) {
		status, body, _ := corpusGet(t, cfg, cfg.AuthToken, corpusRoute)
		if status < 200 || status > 299 {
			t.Errorf("the shared token got %d, want 2xx. §5 records the service-to-service row as NOT breaking; internal enrichment reads would go down. body=%s", status, string(body))
		}
	})

	// Row 6 — Bootstrap session. Recorded as NO break because RequireScope
	// bypasses SessionSourceBootstrap.
	//
	// NOT constructible over HTTP, and saying so is more honest than
	// manufacturing a request that resembles one. Every non-test reference to
	// SessionSourceBootstrap is a CONSUMER (scope_middleware.go,
	// corpus_grant_gate.go, cmd/core/wiring.go); no production path builds such
	// a session for an inbound request, so there is no bearer an e2e client
	// could present to reach that branch. The bypass is asserted directly
	// against auth.RequireScope by TP-03-03, where the session can be
	// constructed. Recorded here so the matrix row is accounted for rather than
	// silently skipped.
	t.Run("row6_bootstrap_covered_at_integration", func(t *testing.T) {
		t.Log("row 6 (bootstrap) is not constructible over HTTP — no production path builds a bootstrap session for an inbound request; the bypass is asserted directly in TP-03-03")
	})

	// Row 7 — Unauthenticated probes. §5 records these as ungated, and an
	// over-broad mount that swept them in would take down scraping and
	// orchestrator health checks.
	t.Run("row7_unauthenticated_probes_no_break", func(t *testing.T) {
		// corpusGetAs with an empty bearer omits the Authorization header
		// entirely. corpusGet would send a literal "Bearer ", which is a
		// malformed credential rather than the absent one a Prometheus scrape
		// actually presents.
		for _, path := range []string{"/metrics", "/readyz", "/api/health"} {
			status, _, _ := corpusGetAs(t, cfg, "", path, "")
			if status == http.StatusForbidden {
				t.Errorf("%s returned 403 under ENFORCE; §5 records the probe row as ungated, and gating it breaks Prometheus scraping and orchestrator health checks", path)
			}
		}
	})
}

// TestE2E_Spec108_CorpusEnforce_CallerRegression_TP_04_07 is the persistent
// scenario-specific regression for SCN-108-E01 through E04 against the live
// stack.
func TestE2E_Spec108_CorpusEnforce_CallerRegression_TP_04_07(t *testing.T) {
	cfg := corpusEnforceLane(t)
	waitForHealth(t, cfg, 60*time.Second)

	granted := corpusEnforceToken(t, "granted")
	ungranted := corpusEnforceToken(t, "ungranted")

	// SCN-108-E01 — a bridge token for an ENTITLED principal keeps working
	// across the corpus command surface, not just one route. The Telegram
	// commands map onto search/digest/recent/knowledge, so a regression that
	// broke only one of them would slip past a single-route check.
	for _, path := range []string{"/api/recent?limit=1", "/api/digest", "/api/knowledge/stats"} {
		if status, body, _ := corpusGet(t, cfg, granted, path); status == http.StatusForbidden {
			t.Errorf("SCN-108-E01: an entitled principal was refused on %s (403); Telegram corpus commands must keep working. body=%s", path, string(body))
		}
	}

	// SCN-108-E04 (adversarial) — the minter must not confer authority the
	// principal lacks. This is the case a naive "Telegram works" test passes
	// while the real defect ships.
	status, body, _ := corpusGet(t, cfg, ungranted, "/api/recent?limit=1")
	if status != http.StatusForbidden {
		t.Errorf("SCN-108-E04: a principal WITHOUT corpus:read reached the corpus (status=%d); authority must come from the principal, never from the minter. body=%s", status, string(body))
	}

	// The refusal must read as a PERMANENT, operator-actionable condition, not
	// a transient one. 403 says "you may not"; 429/503 say "try again", which
	// would send a bridge into a retry loop that can never succeed.
	if status == http.StatusTooManyRequests || status == http.StatusServiceUnavailable || status == http.StatusRequestTimeout {
		t.Errorf("SCN-108-E04: the refusal used a TRANSIENT status (%d); a missing grant is permanent until an operator rotates the token, and a retryable code would drive an endless retry loop", status)
	}

	// SCN-108-E03 — extension inherits its principal's grant, both directions.
	if s, b, _ := corpusGetAs(t, cfg, ungranted, "/api/recent?limit=1", "smackerel-extension"); s != http.StatusForbidden {
		t.Errorf("SCN-108-E03: extension with an ungranted principal got %d, want the same 403 as the PWA. body=%s", s, string(b))
	}
	if s, b, _ := corpusGetAs(t, cfg, granted, "/api/recent?limit=1", "smackerel-extension"); s == http.StatusForbidden {
		t.Errorf("SCN-108-E03: extension with a GRANTED principal was refused; it must inherit the grant. body=%s", string(b))
	}

	// SCN-108-E02 — the external guest-context consumer is not silently broken.
	// It authenticates on the shared token, which RequireScope bypasses.
	if s, b, _ := corpusGet(t, cfg, cfg.AuthToken, "/api/recent?limit=1"); s < 200 || s > 299 {
		t.Errorf("SCN-108-E02: the shared-token consumer got %d, want 2xx; the external guest-context path must not break at ENFORCE. body=%s", s, string(b))
	}
}

// corpusGetAs issues a request with a client-identifying header so the
// extension surface can be driven as the extension. An empty bearer sends no
// Authorization header at all, which is what the unauthenticated probe row
// needs.
func corpusGetAs(t *testing.T, cfg e2eConfig, bearer, path, client string) (int, []byte, http.Header) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, cfg.CoreURL+path, nil)
	if err != nil {
		t.Fatalf("build request %s: %v", path, err)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	if client != "" {
		req.Header.Set("X-Smackerel-Client", client)
	}
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body %s: %v", path, err)
	}
	return resp.StatusCode, body, resp.Header
}

// corpusRepoRoot locates the repo from this test's own source path.
func corpusRepoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed — cannot locate the repo root")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}

// corpusMetricSamples returns the raw /metrics lines for one metric family.
func corpusMetricSamples(t *testing.T, cfg e2eConfig, metricName string) []string {
	t.Helper()
	status, body, _ := corpusGetAs(t, cfg, "", "/metrics", "")
	if status != http.StatusOK {
		t.Fatalf("GET /metrics returned %d, want 200", status)
	}
	var out []string
	for _, line := range strings.Split(string(body), "\n") {
		if strings.HasPrefix(line, metricName+"{") {
			out = append(out, line)
		}
	}
	return out
}

// TestE2E_Spec108_CorpusEnforce_RunbookQueryShape_TP_05_04 proves the
// UC-108-001 runbook in docs/Operations.md is EXECUTABLE, not aspirational:
// the series it names really exist on the live /metrics surface and really
// carry the labels the documented query groups by.
//
// A runbook that names a label the metric does not emit fails at the worst
// possible moment — during the pre-flip go/no-go review, when the operator is
// deciding whether anyone gets locked out.
//
// # WHICH SERIES CAN EXIST IN THIS LANE, AND WHY
//
// `..._would_deny_total` is a COUNTERFACTUAL: "this would have been denied
// under ENFORCE, but was allowed under OBSERVE". Under ENFORCE it cannot fire
// at all, and that is by construction rather than by accident — chi runs the
// group-level `r.Use(auth.RequireScope(...))` BEFORE the route-level
// `r.With(corpusGate.Observe(...))` (router.go:131-134), so an ungranted
// principal is refused before the gate ever records. The denial is then real,
// not hypothetical, and lands on `smackerel_auth_scope_rejected_total`.
//
// An earlier version of this test demanded would-deny samples HERE and failed.
// Forcing them to appear would have meant either mounting the gate ahead of
// the enforcement half (breaking the never-denies guarantee) or relaxing the
// assertion to nothing. Both are worse than asserting each series where it
// legitimately exists:
//
//   - allowed + gauge + real denial → asserted here, on the ENFORCE stack
//   - would-deny shape              → asserted by the OBSERVE-stage integration
//     test, which drives all sixteen route groups with an ungranted principal
//     (TestIntegration_CorpusGrantObserve_UngrantedPrincipalIsCountedOnAllSixteenGroups)
//
// That split matches reality: the operator runs the UC-108-001 query DURING
// the OBSERVE window, which is the only stage where its answer is meaningful.
func TestE2E_Spec108_CorpusEnforce_RunbookQueryShape_TP_05_04(t *testing.T) {
	cfg := corpusEnforceLane(t)
	waitForHealth(t, cfg, 60*time.Second)

	granted := corpusEnforceToken(t, "granted")
	ungranted := corpusEnforceToken(t, "ungranted")

	// Granted traffic populates the allowed counter; ungranted traffic
	// produces a REAL denial on this stack.
	corpusGet(t, cfg, granted, "/api/recent?limit=1")
	if status, _, _ := corpusGet(t, cfg, ungranted, "/api/recent?limit=1"); status != http.StatusForbidden {
		t.Fatalf("the ungranted principal got %d, want 403; without a real denial the scope_rejected assertion below would prove nothing", status)
	}

	// The denominator series, with every label the documented query groups by.
	samples := corpusMetricSamples(t, cfg, "smackerel_auth_corpus_grant_allowed_total")
	if len(samples) == 0 {
		t.Fatal("smackerel_auth_corpus_grant_allowed_total exposes no samples on the live /metrics surface; the documented runbook query would return empty and an operator would read that as 'no traffic'")
	}
	for _, label := range []string{"user_id=", "route_group=", "session_source="} {
		if !strings.Contains(samples[0], label) {
			t.Errorf("allowed_total samples do not carry %q — the documented UC-108-001 query groups by it, so the runbook is not executable. sample=%s", strings.TrimSuffix(label, "="), samples[0])
		}
	}

	// The ENFORCE-stage denial must be attributable to a principal, or the
	// operator cannot tell WHO was refused after the flip.
	rejected := corpusMetricSamples(t, cfg, "smackerel_auth_scope_rejected_total")
	if len(rejected) == 0 {
		t.Error("smackerel_auth_scope_rejected_total exposes no samples despite a real 403 on this stack; a post-flip denial would be invisible to the operator")
	} else {
		joined := strings.Join(rejected, "\n")
		if !strings.Contains(joined, "user_id=") || !strings.Contains(joined, auth.GrantGlobalCorpusRead) {
			t.Errorf("scope_rejected samples do not identify the principal and the required scope; post-flip triage needs both. samples=%s", joined)
		}
	}

	// Step 1 of the runbook reads this gauge to confirm the stage.
	status, body, _ := corpusGetAs(t, cfg, "", "/metrics", "")
	if status != http.StatusOK || !strings.Contains(string(body), "smackerel_auth_corpus_grant_enforcement_mode") {
		t.Error("the enforcement_mode gauge is absent from /metrics; the runbook's 'confirm the stage' step cannot be performed")
	}
}

// TestE2E_Spec108_CorpusEnforce_PermanentInvariants_TP_05_06 is the persistent
// regression for SCN-108-R01..R05, asserting only the PERMANENT invariants.
//
// It deliberately does NOT pin `next` at false: bubbles.train flipping the
// OWNING train ON after a clean observation window is the intended end state,
// not a regression. Pinning it would make the spec's own success look like a
// failure and pressure a future operator to work around this test.
func TestE2E_Spec108_CorpusEnforce_PermanentInvariants_TP_05_06(t *testing.T) {
	cfg := corpusEnforceLane(t)
	waitForHealth(t, cfg, 60*time.Second)
	root := corpusRepoRoot(t)

	read := func(rel string) string {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		return string(raw)
	}

	t.Run("R01_flag_declared_in_both_bundles_and_mvp_never_ON", func(t *testing.T) {
		mvp := read("config/feature-flags.mvp.yaml")
		for name, body := range map[string]string{
			"next": read("config/feature-flags.next.yaml"),
			"mvp":  mvp,
		} {
			if !strings.Contains(body, "corpusGrantEnforcement:") {
				t.Errorf("%s bundle no longer declares corpusGrantEnforcement; an undeclared flag is undefined, and the resolution path is fail-loud", name)
			}
		}
		if regexp.MustCompile(`corpusGrantEnforcement:\s*true`).MatchString(mvp) {
			t.Error("the NON-OWNING train mvp has corpusGrantEnforcement default-ON — this is the G111 violation condition")
		}
		if !strings.Contains(mvp, "owning_spec: specs/108-corpus-grant-enforcement/") {
			t.Error("the mvp metadata block no longer names the owning spec; flag provenance would be unrecoverable")
		}
	})

	t.Run("R02_sst_key_stays_default_free", func(t *testing.T) {
		gen := read("scripts/commands/config.sh")
		if !strings.Contains(gen, "required_value auth.corpus_grant_enforcement") {
			t.Error("config.sh no longer resolves auth.corpus_grant_enforcement fail-loud via required_value")
		}
		if strings.Contains(gen, "SMACKEREL_AUTH_CORPUS_GRANT_ENFORCEMENT:-") {
			t.Error("a ${VAR:-default} fallback was reintroduced; a default silently decides the enforcement stage for the operator")
		}
	})

	t.Run("R03_runbook_query_shape_holds_on_the_live_surface", func(t *testing.T) {
		corpusGet(t, cfg, corpusEnforceToken(t, "granted"), "/api/recent?limit=1")
		samples := corpusMetricSamples(t, cfg, "smackerel_auth_corpus_grant_allowed_total")
		if len(samples) == 0 {
			t.Fatal("the allowed counter exposes no samples; the documented query cannot return its documented shape")
		}
		if !strings.Contains(samples[0], "user_id=") {
			t.Errorf("the allowed counter lost its user_id label; per-principal coverage becomes uncomputable and criterion 1(b) silently reverts to operator attestation. sample=%s", samples[0])
		}
	})

	t.Run("R04_release_packet_records_capability", func(t *testing.T) {
		packet := read("docs/releases/v1/features.md")
		for _, token := range []string{"108-corpus-grant-enforcement", "corpusGrantEnforcement"} {
			if !strings.Contains(packet, token) {
				t.Errorf("docs/releases/v1/features.md no longer records %q; the shipped capability would vanish from the release record", token)
			}
		}
	})

	t.Run("R05_retirement_contract_stays_recorded", func(t *testing.T) {
		// Markdown emphasis sits between words ("deleted **together**"), so
		// strip it before matching or the assertion fails on formatting.
		flat := regexp.MustCompile(`\s+`).ReplaceAllString(
			strings.NewReplacer("*", "", "_", "", "`", "").Replace(strings.ToLower(read("docs/Operations.md"))), " ")
		for _, clause := range []struct{ pattern, why string }{
			{`train plus one cycle|train \+ one cycle`, "without a stated lifetime the flag becomes permanent"},
			{`deleted together|retire together`, "retiring the flag but keeping the observe branch leaves a permanent bypass surface"},
			{`unconditional`, "the end state must be stated or removing the flag looks like removing the protection"},
			{`bubbles\.train`, "an unowned retirement is nobody's job"},
		} {
			if !regexp.MustCompile(clause.pattern).MatchString(flat) {
				t.Errorf("the retirement contract lost a clause (/%s/) — %s", clause.pattern, clause.why)
			}
		}
	})
}
