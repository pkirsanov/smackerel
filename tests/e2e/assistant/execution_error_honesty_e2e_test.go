//go:build e2e

// BUG-061-008 — execution-error honesty asserted over the WIRE.
//
// SCN-061-008-01 / SCN-061-008-03, live. Every other proof this packet
// cites is an in-package Go unit test that constructs an agent.Final
// in-process and never crosses a transport boundary. This is the
// regression test that drives the LIVE chi-mounted
// POST /api/assistant/turn route and asserts the envelope a real
// client receives when the EXECUTOR FAILS.
//
// The contract under test: when a band-HIGH turn terminates on a
// non-OK outcome — provider error, timeout, schema failure, invalid
// tool return, input-schema violation, loop limit — the user MUST be
// told honestly that it failed. It must NEVER render the band-LOW
// capture acknowledgement ("saved as an idea"), because that reports
// a broken system as a successful capture, which is the exact
// deception this bug exists to remove.
//
// WHY THIS PATH IS REACHABLE HERE, AND NOT A HOPEFUL PROMPT. The
// disposable e2e stack deliberately runs without a usable model, so
// any turn that reaches the model path terminates on an execution
// error rather than an answer. That is not a limitation being worked
// around — it is precisely the failure class BUG-061-008 governs, so
// this lane exercises the packet's own subject matter naturally. The
// sibling BUG-061-009 test observes the same stack property from the
// other side (see high_band_refusal_e2e_test.go); the two packets
// assert different halves of the same envelope contract.
//
// Deterministic band selection, not a hopeful prompt. `/ask` is a v1
// slash shortcut (assistant.SlashShortcuts) that sets
// IntentEnvelope.ScenarioID explicitly, so agent.Router takes the
// by-id fast path and Borderline returns BandHigh for
// ReasonExplicitScenarioID regardless of TopScore. The band-high
// precondition therefore cannot drift with model behaviour.
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

// internalVocabulary is implementation language that must never reach a
// user-visible body. BUG-061-008's security phase established that the
// pre-fix strings leaked exactly this class ("internal validation
// failure", "request exceeded internal limits") and that the
// replacements name no internal component. Asserting it here binds that
// finding to the wire instead of leaving it as prose in a report.
var internalVocabulary = []string{
	"internal validation failure",
	"exceeded internal limit",
	"goroutine",
	"panic:",
	"/workspace/",
	".go:",
	"nil pointer",
	"sql:",
	"dial tcp",
}

func TestAssistantHTTPE2E_ExecutionErrorSurfacesHonestly(t *testing.T) {
	stack := loadHTTPTurnLiveStack(t)
	waitHTTPTurnHealthy(t, stack, 30*time.Second)

	req := httpadapter.TurnRequest{
		SchemaVersion:      httpadapter.SchemaVersionV1,
		TransportMessageID: "e2e-bug061008-exec-error-" + timestamp(),
		Kind:               string(contracts.KindText),
		TransportHint:      "web",
		Text:               "/ask summarise the operating covenants of the Vandermeer-Kaltrix accord",
	}

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
			t.Skipf("assistant HTTP adapter never bound within 60s (still 503 assistant_http_not_ready) — the route is not up on this run, so no execution-error conclusion is available; last body=%s", string(raw))
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

	t.Logf("live envelope: status=%q error_cause=%q capture_route=%v sources=%d body=%q",
		env.Status, env.ErrorCause, env.CaptureRoute, len(env.Sources), env.Body)

	if !env.FacadeInvoked {
		t.Fatalf("facade_invoked = false — the turn never reached the facade, so nothing about execution-error honesty was exercised; body=%s", string(raw))
	}

	// --- Honesty invariant, unconditional. Holds for EVERY terminal
	// outcome of a band-high turn, so it fails on a masked execution
	// error no matter which failure mode the executor hit. ---

	if env.Status == string(contracts.StatusSavedAsIdea) {
		t.Errorf("status = %q on a band-HIGH /ask turn — BUG-061-008 violated: the router matched and executed a scenario, so reporting it as a band-low capture masks an execution outcome; error_cause=%q body=%q", env.Status, env.ErrorCause, env.Body)
	}
	if env.CaptureRoute {
		t.Errorf("capture_route = true on a band-HIGH /ask turn — BUG-061-008 violated: capture-as-fallback is band-LOW only; status=%q body=%q", env.Status, env.Body)
	}
	if strings.Contains(strings.ToLower(env.Body), captureAckSubstring) {
		t.Errorf("body = %q contains %q — BUG-061-008 violated: an execution outcome was rendered as a filed-away idea", env.Body, captureAckSubstring)
	}

	// --- Terminal-shape closure. A band-high turn has exactly two
	// truthful shapes. There is no third branch and no early return. ---

	switch env.Status {
	case string(contracts.StatusAnswered):
		if len(env.Sources) == 0 {
			t.Errorf("status = %q with zero sources — a requires_provenance scenario answered without citing anything; body=%q", env.Status, env.Body)
		}
	case string(contracts.StatusUnavailable):
		// This is the BUG-061-008 path. The failure MUST be typed: an
		// untyped failure is indistinguishable from a generic transport
		// error, which is what made the masking possible.
		if strings.TrimSpace(env.ErrorCause) == "" {
			t.Errorf("status = unavailable with an empty error_cause — an execution error must carry a typed cause so transports can render it honestly; body=%q", env.Body)
		}
		if !isKnownErrorCause(env.ErrorCause) {
			t.Errorf("error_cause = %q is outside contracts.AllErrorCauses %v — the closed vocabulary is the contract", env.ErrorCause, contracts.AllErrorCauses)
		}
		if strings.TrimSpace(env.Body) == "" {
			t.Errorf("status = unavailable with an empty body — the user is shown nothing at all, which is a silent failure; error_cause=%q", env.ErrorCause)
		}
	default:
		t.Errorf("status = %q — a band-HIGH turn must terminate as %q (with sources) or %q (with a typed cause); body=%q",
			env.Status, contracts.StatusAnswered, contracts.StatusUnavailable, env.Body)
	}

	// --- Disclosure floor, unconditional. Binds the security phase's
	// finding to the wire: a failure body names no internal component. ---

	lowerBody := strings.ToLower(env.Body)
	for _, leak := range internalVocabulary {
		if strings.Contains(lowerBody, leak) {
			t.Errorf("body = %q contains internal implementation vocabulary %q — a user-visible failure must not disclose internals; error_cause=%q", env.Body, leak, env.ErrorCause)
		}
	}

	// A failure carries no confirm card and no disambiguation prompt:
	// both invite the user to continue a turn that did not survive.
	if env.Status == string(contracts.StatusUnavailable) {
		if env.ConfirmCard != nil {
			t.Errorf("confirm_card non-nil on a failed turn; want nil (got %+v)", env.ConfirmCard)
		}
		if env.DisambiguationPrompt != nil {
			t.Errorf("disambiguation_prompt non-nil on a failed turn; want nil (got %+v)", env.DisambiguationPrompt)
		}
	}
}
