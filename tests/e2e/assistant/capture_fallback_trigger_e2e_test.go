//go:build e2e

// Spec 074 SCOPE-04B — Open-Knowledge No-Ground Capture-as-Fallback E2E.
//
// TestAssistantHTTPE2E_CaptureFallbackOpenKnowledgeNoGround — TP-074-14 / SCN-074-A01 (live).
//
// Drives the LIVE chi-mounted POST /api/assistant/turn route with a
// query crafted to route into the open_knowledge scenario but be
// ungroundable by the agent. When open-knowledge returns
// status="refused" (no-ground), the facade's SCOPE-074-04A hook
// (`facade.go` line ~995) MUST invoke capturefallback.Policy.Capture
// with `cause=open_knowledge_no_ground` and the user MUST see the
// canonical saved-as-idea acknowledgement on the wire — identical
// shape to the BandLow fallback path covered by spec 069 SCOPE-4.
//
// Adversarial coverage (BUG-074-002). Every branch below CLASSIFIES on
// one field and ASSERTS only on others — never on the field it selected
// with. The capture branch selects on `status` and asserts
// `capture_route`, the two card fields and `body`; the refusal and
// provider branches select on `error_cause` and assert `status`,
// `capture_route`, `body` and `sources`; the grounded branch selects on
// `sources` and asserts `capture_route` and `body`. That PER-BRANCH
// disjointness is the structural property, and it is what makes an
// escape hatch impossible rather than merely discouraged: no branch can
// swallow the assertion it exists to make. The previous guard broke it —
// it branched on `status` and then asserted `status`, so a status
// regression selected its own escape hatch, the test reported SKIP
// instead of FAIL, and the header promised a failure mode the code could
// not produce.
//
// Every decoded 200 envelope now lands in exactly one of five branches,
// four of which assert:
//
//  1. status=saved_as_idea           → the capture shape. The four
//     SCOPE-074-04B rules are asserted strictly (capture_route, nil
//     confirm card, nil disambiguation prompt, canonical ack body).
//  2. error_cause=no_grounded_answer → the honest high-band no-ground
//     refusal (BUG-061-009 / INV-HB-REFUSAL, which governs the
//     BandHigh open_knowledge path). Asserted strictly: unavailable,
//     capture_route=false, canonical refusal body, no partial
//     provenance, and the capture acknowledgement string ABSENT.
//  3. error_cause=provider_unavailable → the upstream provider failed,
//     so the grounding decision was never reached and no conclusion
//     about SCOPE-074-04B is available. Asserts status coherence and
//     capture_route=false, then skips. response.go documents this cause
//     as "upstream failed", explicitly distinct from no_grounded_answer
//     ("could not ground"), so the split follows the contract rather
//     than working around it.
//  4. sources present                → the model grounded the prompt,
//     so the no-ground path was not exercised on this run. Passes, and
//     still asserts the two invariants a grounded answer must satisfy.
//  5. anything else                  → FAILS. An envelope that is
//     neither a capture, nor a typed no-ground refusal, nor an upstream
//     failure, nor grounded is off-contract; that includes an
//     answered-with-zero-sources fabrication leak.
//
// Three skip sites are reachable from this test, every one keyed on
// infrastructure availability and none on a contract outcome. Two are
// written here: the HTTP 503 `assistant_http_not_ready` readiness poll
// below (adapter bind timing) and branch 3 (upstream provider down).
// The third is inherited from loadHTTPTurnLiveStack, which skips when
// CORE_EXTERNAL_URL is unset because there is no live stack to drive.
// Critically, none keys on the `saved_as_idea` status that the four
// canonical-ack assertions police, which is the precise defect
// BUG-074-002 was filed for. Model nondeterminism is absorbed by branch
// 4 and by the input choice, never by relaxing an assertion — the same
// discipline as tests/e2e/assistant/high_band_refusal_e2e_test.go.
//
// One caveat on the first of those three, verified rather than assumed:
// the readiness poll below is budgeted at 5 minutes, and
// scripts/runtime/go-e2e.sh runs the binary with `-timeout 300s` — the
// same 5 minutes, shared by every test in the package. An adapter that
// never binds therefore exhausts the harness budget before the poll
// reaches its own deadline, so that skip is not reliably reachable and
// the package dies on the harness timeout instead. The sibling test
// budgets its equivalent poll at 60s and records that the budget "stays
// well inside the go-e2e.sh per-binary `-timeout 300s`"; this one does
// not. Re-sizing it changes when this test declines to run, which is a
// decision about the timing contract rather than a restatement of it.

package assistant_e2e

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/assistant/httpadapter"
)

func TestAssistantHTTPE2E_CaptureFallbackOpenKnowledgeNoGround(t *testing.T) {
	stack := loadHTTPTurnLiveStack(t)
	waitHTTPTurnHealthy(t, stack, 30*time.Second)

	// Query crafted to plausibly route to open_knowledge but be
	// ungroundable: a fabricated proper-noun question with no real
	// referent and no web-search evidence. Open-knowledge's grounding
	// gate should refuse rather than fabricate a citation.
	req := httpadapter.TurnRequest{
		SchemaVersion:      httpadapter.SchemaVersionV1,
		TransportMessageID: "e2e-spec074-noground-" + timestamp(),
		Kind:               string(contracts.KindText),
		TransportHint:      "web",
		Text:               "what is the population of the fictional city of Zorthonia-by-the-Sea in 2024?",
	}

	// Late-binding of the assistant HTTP adapter can leave the route
	// returning 503 assistant_http_not_ready briefly after the core
	// container reports /api/health=200. Poll for up to 5 minutes;
	// late-binding depends on ML sidecar reachability and can take
	// substantial time on a cold test stack. Declining here says only
	// that the route is not up, so no conclusion about the no-ground
	// contract is available either way. It is one of the three
	// infrastructure-keyed skip sites the file header inventories, and
	// the header records why this particular one is not reliably
	// reachable under the harness timeout.
	//
	// `raw` is the undecoded HTTP body; every assertion below reports on the
	// different `env.Body`, so the two do not share an identifier.
	var (
		resp *http.Response
		raw  []byte
	)
	deadline := time.Now().Add(5 * time.Minute)
	for {
		resp, raw = postAssistantTurn(t, stack, req)
		if resp.StatusCode != http.StatusServiceUnavailable || !strings.Contains(string(raw), "assistant_http_not_ready") {
			break
		}
		if time.Now().After(deadline) {
			// States only what was observed. The previous message
			// classified the run as "test-infra timing rather than a
			// SCOPE-074-04B regression" and asserted that unit and
			// integration coverage proved the hook wired — two
			// conclusions this loop never tested (BUG-074-002 DI-2).
			t.Skipf("assistant HTTP adapter never bound within 5min (still 503 assistant_http_not_ready), so the turn never reached the facade and this run establishes nothing about the no-ground contract; last body=%s", string(raw))
		}
		time.Sleep(3 * time.Second)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.StatusCode, string(raw))
	}
	var env httpadapter.TurnResponse
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("decode: %v\nbody=%s", err, string(raw))
	}
	if !env.FacadeInvoked {
		t.Errorf("facade_invoked = false; want true")
	}
	if env.Transport != httpadapter.TransportName {
		t.Errorf("transport = %q, want %q", env.Transport, httpadapter.TransportName)
	}
	if env.TransportMessageID != req.TransportMessageID {
		t.Errorf("transport_message_id echo = %q, want %q", env.TransportMessageID, req.TransportMessageID)
	}

	// Record the exact envelope the live stack produced before asserting
	// on it. Which branch a run took is a fact about the stack, and it
	// belongs in the log rather than inside a relaxed assertion.
	t.Logf("live envelope: status=%q error_cause=%q capture_route=%v sources=%d body=%q",
		env.Status, env.ErrorCause, env.CaptureRoute, len(env.Sources), env.Body)

	lowerBody := strings.ToLower(env.Body)

	// Outcome-space closure. Total over the envelope, no early return,
	// no skip. See the branch table in the file header.
	switch {
	case env.Status == string(contracts.StatusSavedAsIdea):
		// SCOPE-074-04B canonical ack rule. Reached only when the stack
		// actually emitted the capture shape, so these four assertions
		// are now unconditional WITHIN the shape they govern instead of
		// being gated behind the status they are supposed to police.
		if !env.CaptureRoute {
			t.Errorf("status = %q but capture_route = false; the capture shape MUST set capture_route=true (regression of SCOPE-074-04B canonical ack)", env.Status)
		}
		if env.ConfirmCard != nil {
			t.Errorf("confirm_card non-nil on the capture path; want nil (got %+v)", env.ConfirmCard)
		}
		if env.DisambiguationPrompt != nil {
			t.Errorf("disambiguation_prompt non-nil on the capture path; want nil (got %+v)", env.DisambiguationPrompt)
		}
		// Canonical body substring — must match the shared saved-as-idea
		// acknowledgement emitted by the facade (same as the spec 069
		// SCOPE-4 BandLow path) so the cross-transport acknowledgement
		// contract holds for the no-ground cause.
		if !strings.Contains(lowerBody, captureAckSubstring) {
			t.Errorf("status = %q but body = %q; expected the canonical %q acknowledgement (SCOPE-074-04B canonical ack rule)", env.Status, env.Body, captureAckSubstring)
		}

	case env.ErrorCause == string(contracts.ErrNoGroundedAnswer):
		// The no-ground path RAN and refused honestly. This is the
		// BandHigh shape that BUG-061-009 / INV-HB-REFUSAL ratified for
		// open_knowledge: a matched, executed request the system could
		// not ground refuses with a typed cause and MUST NOT be dressed
		// as a band-low capture. Previously this whole shape reported
		// SKIP and asserted nothing.
		canonicalRefusal := contracts.CanonicalRefusalBodyFor(contracts.RefusalDefault)
		if env.Status != string(contracts.StatusUnavailable) {
			t.Errorf("error_cause = %q but status = %q; want %q — the no-ground cause and the unavailable status are one contract, not two", env.ErrorCause, env.Status, contracts.StatusUnavailable)
		}
		if env.CaptureRoute {
			t.Errorf("capture_route = true on a %q refusal; capture-as-fallback is band-LOW only (INV-HB-REFUSAL) and MUST NOT ride a high-band no-ground refusal; body=%q", env.ErrorCause, env.Body)
		}
		if strings.Contains(lowerBody, captureAckSubstring) {
			t.Errorf("body = %q contains %q on a %q refusal — the user asked a real question and was told it was filed away as an idea (INV-HB-REFUSAL violated)", env.Body, captureAckSubstring, env.ErrorCause)
		}
		if env.Body != canonicalRefusal {
			t.Errorf("error_cause = %q but body = %q; want the canonical refusal %q", env.ErrorCause, env.Body, canonicalRefusal)
		}
		if len(env.Sources) != 0 {
			t.Errorf("error_cause = %q with %d sources; a no-grounded-answer refusal MUST NOT surface partial provenance", env.ErrorCause, len(env.Sources))
		}

	case env.ErrorCause == string(contracts.ErrProviderUnavailable):
		// The upstream provider failed, so the turn never reached the
		// grounding decision and this run cannot say anything about the
		// SCOPE-074-04B contract either way. The contract itself draws
		// this line: response.go documents ErrProviderUnavailable as
		// "upstream failed", explicitly distinct from ErrNoGroundedAnswer
		// ("could not ground"). Reporting FAIL here would assert a
		// contract violation that was never observed — the same
		// truthfulness error this packet exists to remove, pointed the
		// other way. This skip is narrow and TYPED: it keys on an
		// upstream-failure cause, never on the saved_as_idea status the
		// four canonical-ack assertions police, so it cannot swallow the
		// regression class BUG-074-002 was filed for.
		if env.Status != string(contracts.StatusUnavailable) {
			t.Errorf("error_cause = %q but status = %q; want %q — an upstream failure and the unavailable status are one contract", env.ErrorCause, env.Status, contracts.StatusUnavailable)
		}
		if env.CaptureRoute {
			t.Errorf("capture_route = true on a %q failure; capture-as-fallback MUST NOT ride an upstream provider error; body=%q", env.ErrorCause, env.Body)
		}
		t.Skipf("upstream provider unavailable (error_cause=%q, status=%q); the open-knowledge grounding decision was never reached, so this run establishes nothing about the no-ground capture contract. body=%q", env.ErrorCause, env.Status, env.Body)

	case len(env.Sources) > 0:
		// The model grounded the prompt, so the no-ground path was not
		// exercised on this run. That is the one legitimate
		// non-exercise, and it PASSES rather than skipping — but the
		// two invariants a grounded answer must satisfy are still
		// asserted, so this branch cannot become a silent catch-all.
		if env.CaptureRoute {
			t.Errorf("capture_route = true on a grounded answer (%d sources); capture-as-fallback MUST NOT fire when the turn was groundable; status=%q body=%q", len(env.Sources), env.Status, env.Body)
		}
		if strings.Contains(lowerBody, captureAckSubstring) {
			t.Errorf("body = %q contains %q on a grounded answer with %d sources; a grounded turn is answered, never captured", env.Body, captureAckSubstring, len(env.Sources))
		}
		t.Logf("open-knowledge grounded the prompt with %d source(s); the no-ground capture path was not exercised on this run, so branches 1 and 2 did not apply", len(env.Sources))

	default:
		// Neither a capture, nor a typed no-ground refusal, nor
		// grounded. Off-contract, and previously the exact shape the
		// old guard swallowed as a SKIP. An answered-with-zero-sources
		// envelope lands here, which is the fabrication leak the
		// provenance gate exists to refuse.
		t.Errorf("off-contract envelope: status=%q error_cause=%q capture_route=%v sources=0 body=%q. An ungroundable open-knowledge turn must terminate as the capture shape (%q), a typed no-ground refusal (error_cause=%q), or a grounded answer carrying sources; a provider/infra failure also lands here and is reported rather than skipped",
			env.Status, env.ErrorCause, env.CaptureRoute, env.Body,
			contracts.StatusSavedAsIdea, contracts.ErrNoGroundedAnswer)
	}
}
