//go:build e2e

// Spec 108 SCOPE-03 — corpus-grant ENFORCE rows against the LIVE stack
// (TP-03-06, TP-03-07, TP-03-10).
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
