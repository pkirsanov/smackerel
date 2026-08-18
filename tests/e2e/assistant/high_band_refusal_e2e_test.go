//go:build e2e

// BUG-061-009 — INV-HB-REFUSAL asserted over the WIRE.
//
// SCN-061-009-01 / SCN-061-009-02, live. Every other proof this packet
// cites is an in-package Go unit test that constructs responses
// in-process and never crosses a transport boundary. This is the
// regression test that drives the LIVE chi-mounted
// POST /api/assistant/turn route and asserts the envelope a real
// client receives.
//
// The contract under test: a band-HIGH `requires_provenance` turn that
// grounds nothing MUST refuse honestly. It must NEVER render the
// band-LOW capture acknowledgement ("saved as an idea"), because the
// user made a real, matched, executed request — telling them it was
// filed away as an idea is the exact deception this bug exists to
// remove.
//
// Deterministic band selection, not a hopeful prompt. `/ask` is a
// v1 slash shortcut (assistant.SlashShortcuts) that sets
// IntentEnvelope.ScenarioID explicitly, so agent.Router takes the
// by-id fast path and Borderline returns BandHigh for
// ReasonExplicitScenarioID regardless of TopScore. No embedding call,
// no confidence threshold, no LLM in the band decision. The band-high
// precondition of this test therefore cannot drift with model
// behaviour — only the refusal CAUSE can, and every honest cause is
// asserted below.
//
// WHY THIS TEST HAS NO STATUS-CONDITIONAL SKIP (read before editing).
// A sibling live test in this package places its assertions below a
// `t.Skipf` keyed on the status it was hunting, so the regression its
// own header advertises cannot fail it — a wrong status is reported as
// "not exercised" instead of "broken". `.github/copilot-instructions.md`
// forbids that ("Required tests MUST NOT use bailout returns"). Here
// EVERY branch asserts: there is no path on which this test passes
// without proving something. The only `t.Skip` is for the adapter
// being genuinely unbound (HTTP 503 assistant_http_not_ready), which
// is infrastructure availability, not an outcome.

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

// captureAckSubstring is the band-LOW acknowledgement phrase. Its
// presence in a band-HIGH body is the defect, in any casing.
const captureAckSubstring = "saved as an idea"

func TestAssistantHTTPE2E_HighBandUncitedRefusesHonestly(t *testing.T) {
	stack := loadHTTPTurnLiveStack(t)
	waitHTTPTurnHealthy(t, stack, 30*time.Second)

	// A question with no real referent: there is no artifact to
	// retrieve, no web page to cite, and no tool that computes it, so
	// the turn cannot be grounded. Nondeterminism is absorbed here, in
	// the input choice — never by relaxing an assertion below.
	req := httpadapter.TurnRequest{
		SchemaVersion:      httpadapter.SchemaVersionV1,
		TransportMessageID: "e2e-bug061009-hb-refusal-" + timestamp(),
		Kind:               string(contracts.KindText),
		TransportHint:      "web",
		Text:               "/ask what did the Grelthorne Assembly of Quibbenmoor decide at its 1847 winter session?",
	}

	// The assistant HTTP adapter binds late; the route can answer 503
	// assistant_http_not_ready for a short window after /api/health
	// reports 200. That is the adapter not being up yet — genuine
	// infrastructure unavailability, and the ONLY condition on which
	// this test declines to run. The budget stays well inside the
	// go-e2e.sh per-binary `-timeout 300s`.
	var (
		resp *http.Response
		raw  []byte
	)
	readinessDeadline := time.Now().Add(60 * time.Second)
	for {
		resp, raw = postAssistantTurn(t, stack, req)
		if resp.StatusCode != http.StatusServiceUnavailable || !strings.Contains(string(raw), "assistant_http_not_ready") {
			break
		}
		if time.Now().After(readinessDeadline) {
			t.Skipf("assistant HTTP adapter never bound within 60s (still 503 assistant_http_not_ready) — the route is not up on this run, so no INV-HB-REFUSAL conclusion is available; last body=%s", string(raw))
		}
		time.Sleep(2 * time.Second)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("HTTP status = %d, want 200; body=%s", resp.StatusCode, string(raw))
	}
	var env httpadapter.TurnResponse
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("decode turn response: %v\nbody=%s", err, string(raw))
	}

	// Record the exact envelope the live stack produced. This is the
	// evidence: which honest refusal cause the run exercised is a fact
	// about the stack, and it belongs in the log rather than in a
	// weakened assertion.
	t.Logf("live envelope: status=%q error_cause=%q capture_route=%v sources=%d body=%q",
		env.Status, env.ErrorCause, env.CaptureRoute, len(env.Sources), env.Body)

	if !env.FacadeInvoked {
		t.Fatalf("facade_invoked = false — the turn never reached the facade, so nothing about INV-HB-REFUSAL was exercised; body=%s", string(raw))
	}
	if env.TransportMessageID != req.TransportMessageID {
		t.Errorf("transport_message_id echo = %q, want %q", env.TransportMessageID, req.TransportMessageID)
	}

	// --- INV-HB-REFUSAL, unconditional. Holds for EVERY honest outcome
	// of a band-high turn, so it fails on a masked refusal no matter
	// which failure mode the executor hit on this run. ---

	if env.Status == string(contracts.StatusSavedAsIdea) {
		t.Errorf("status = %q on a band-HIGH /ask turn — INV-HB-REFUSAL violated: the router matched and executed a scenario, so this is a refusal masked as a band-low capture; body=%q", env.Status, env.Body)
	}
	if env.CaptureRoute {
		t.Errorf("capture_route = true on a band-HIGH /ask turn — INV-HB-REFUSAL violated: capture-as-fallback is band-LOW only; status=%q body=%q", env.Status, env.Body)
	}
	if strings.Contains(strings.ToLower(env.Body), captureAckSubstring) {
		t.Errorf("body = %q contains %q — INV-HB-REFUSAL violated: the user asked a real question and was told it was filed away as an idea", env.Body, captureAckSubstring)
	}
	if env.ConfirmCard != nil {
		t.Errorf("confirm_card non-nil on an ungrounded /ask turn; want nil (got %+v)", env.ConfirmCard)
	}
	if env.DisambiguationPrompt != nil {
		t.Errorf("disambiguation_prompt non-nil on an ungrounded /ask turn; want nil (got %+v)", env.DisambiguationPrompt)
	}

	// --- Honest-outcome closure, unconditional. A band-high turn has
	// exactly two truthful terminal shapes. Anything else fails; there
	// is no third branch and no early return. ---

	switch env.Status {
	case string(contracts.StatusAnswered):
		// It grounded. Then it MUST carry the provenance it claims.
		// An answered-with-zero-sources envelope is SCN-061-009-01
		// itself — the OK-but-uncited response the provenance gate
		// exists to refuse — so it fails here rather than passing as
		// "well, it answered".
		if len(env.Sources) == 0 {
			t.Errorf("status = %q with zero sources — a requires_provenance scenario answered without citing anything; the provenance gate must have refused this (SCN-061-009-01); body=%q", env.Status, env.Body)
		}
	case string(contracts.StatusUnavailable):
		// It refused. The refusal MUST be typed: an untyped refusal is
		// indistinguishable from a generic failure at the transport,
		// which is what made the masking possible in the first place.
		if strings.TrimSpace(env.ErrorCause) == "" {
			t.Errorf("status = unavailable with an empty error_cause — a refusal must carry a typed cause so transports can render it honestly; body=%q", env.Body)
		}
		if !isKnownErrorCause(env.ErrorCause) {
			t.Errorf("error_cause = %q is outside contracts.AllErrorCauses %v — the closed vocabulary is the contract", env.ErrorCause, contracts.AllErrorCauses)
		}
	default:
		t.Errorf("status = %q — a band-HIGH ungroundable turn must terminate as %q (with sources) or %q (with a typed cause); body=%q",
			env.Status, contracts.StatusAnswered, contracts.StatusUnavailable, env.Body)
	}

	// --- The packet's specific gate shape, bound in BOTH directions.
	// Unconditional: whichever side is present, the other is required.
	// This is what stops a future change from emitting the canonical
	// refusal text under some other cause, or the no_grounded_answer
	// cause under some other text. ---

	canonicalRefusal := contracts.CanonicalRefusalBodyFor(contracts.RefusalDefault)

	if env.ErrorCause == string(contracts.ErrNoGroundedAnswer) {
		if env.Status != string(contracts.StatusUnavailable) {
			t.Errorf("error_cause = %q but status = %q; want %q", env.ErrorCause, env.Status, contracts.StatusUnavailable)
		}
		if env.Body != canonicalRefusal {
			t.Errorf("error_cause = %q but body = %q; want the canonical refusal %q", env.ErrorCause, env.Body, canonicalRefusal)
		}
		if len(env.Sources) != 0 {
			t.Errorf("error_cause = %q with %d sources; a no-grounded-answer refusal MUST NOT surface partial provenance", env.ErrorCause, len(env.Sources))
		}
	}
	if env.Body == canonicalRefusal {
		if env.Status != string(contracts.StatusUnavailable) {
			t.Errorf("body is the canonical refusal but status = %q; want %q", env.Status, contracts.StatusUnavailable)
		}
		if env.ErrorCause != string(contracts.ErrNoGroundedAnswer) {
			t.Errorf("body is the canonical refusal but error_cause = %q; want %q — the canonical refusal text and the no_grounded_answer cause are one contract, not two", env.ErrorCause, contracts.ErrNoGroundedAnswer)
		}
	}
}

// isKnownErrorCause reports whether cause is in the closed
// contracts.AllErrorCauses vocabulary. Reading the vocabulary from the
// contracts package rather than restating it here means a new cause
// added upstream widens this check automatically instead of silently
// failing a test that hardcoded the old list.
func isKnownErrorCause(cause string) bool {
	for _, known := range contracts.AllErrorCauses {
		if cause == string(known) {
			return true
		}
	}
	return false
}
