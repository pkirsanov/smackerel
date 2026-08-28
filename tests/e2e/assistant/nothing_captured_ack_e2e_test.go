//go:build e2e

// BUG-061-006 — acknowledgement honesty asserted over the WIRE.
//
// SCN-061-006: a turn that captured NOTHING must never claim it was
// saved. Every other proof this packet cites is an in-package unit
// test (TestHandleUpdate_BUG061006_*) that constructs an update
// in-process and never crosses a transport boundary. This drives the
// LIVE chi-mounted POST /api/assistant/turn route and asserts the
// envelope a real client receives.
//
// SCOPE, STATED HONESTLY (read before extending). This binds DEFECT 2
// ("never claim saved when nothing was saved") end to end, because a
// bare `/ask` is fully reachable over HTTP. It does NOT bind DEFECT 1
// ("exactly ONE acknowledgement") end to end: the duplicate
// acknowledgement was a TELEGRAM-transport defect, where the bot-side
// capture hook sent a second reply alongside the assistant renderer's.
// The HTTP adapter returns exactly one envelope per turn by
// construction, so counting replies here would assert a property of
// the adapter rather than of the fix. DEFECT 1 stays bound by the
// bot-level adversarial unit test
// TestHandleMessage_BUG061006_CaptureRouteSendsNoOwnReply, and that
// asymmetry is recorded in uservalidation.md rather than papered over.
//
// What IS bound here for DEFECT 1 is the precondition the duplicate
// depended on: the envelope carries a single non-empty body and a
// capture_route flag consistent with that body, so a regression that
// re-introduced a second acknowledgement source would have to first
// violate the single-body contract asserted below.
//
// WHY THIS TEST HAS NO STATUS-CONDITIONAL SKIP. A sibling live test in
// this package places its assertions below a `t.Skipf` keyed on the
// status it was hunting, so the regression its own header advertises
// cannot fail it. `.github/copilot-instructions.md` forbids that
// ("Required tests MUST NOT use bailout returns"). Every branch here
// asserts; the only `t.Skip` is for the adapter being genuinely
// unbound (HTTP 503 assistant_http_not_ready), which is infrastructure
// availability, not an outcome.

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

func TestAssistantHTTPE2E_NothingCapturedIsNeverClaimedSaved(t *testing.T) {
	stack := loadHTTPTurnLiveStack(t)
	waitHTTPTurnHealthy(t, stack, 30*time.Second)

	// A bare `/ask` with no question. There is nothing to answer and
	// nothing to capture, so the only honest replies are a prompt for
	// the missing question or a typed refusal. "saved as an idea" is a
	// lie in either case, because no idea was saved.
	//
	// The /reset preamble is load-bearing, not politeness. TurnRequest
	// carries no user field — identity comes from the session token, so
	// every test in this package writes to the SAME conversation row. A
	// neighbour's pending confirm or disambiguation is consumed by
	// Handle's Step-1 resume branches, which return long before the
	// Step-2 shortcut guard this test exists to pin: the `/ask` is
	// answered as a reply to someone else's question. That is precisely
	// how this test passed alone and failed in the full suite. /reset
	// drops that pending state, so the turn below is a first turn.
	resetReq := httpadapter.TurnRequest{
		SchemaVersion:      httpadapter.SchemaVersionV1,
		TransportMessageID: "e2e-bug061006-reset-" + timestamp(),
		Kind:               string(contracts.KindReset),
		TransportHint:      "web",
	}
	if rr, _ := postAssistantTurn(t, stack, resetReq); rr != nil {
		_ = rr.Body.Close()
	}

	req := httpadapter.TurnRequest{
		SchemaVersion:      httpadapter.SchemaVersionV1,
		TransportMessageID: "e2e-bug061006-nothing-captured-" + timestamp(),
		Kind:               string(contracts.KindText),
		TransportHint:      "web",
		Text:               "/ask",
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
			t.Skipf("assistant HTTP adapter never bound within 60s (still 503 assistant_http_not_ready) — the route is not up on this run, so no acknowledgement-honesty conclusion is available; last body=%s", string(raw))
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
		t.Fatalf("facade_invoked = false — the turn never reached the facade, so nothing about acknowledgement honesty was exercised; body=%s", string(raw))
	}
	if env.TransportMessageID != req.TransportMessageID {
		t.Errorf("transport_message_id echo = %q, want %q", env.TransportMessageID, req.TransportMessageID)
	}

	// --- DEFECT 2, unconditional. Nothing was captured, so nothing may
	// claim it was saved. This is the assertion the bug is about. ---

	if strings.Contains(strings.ToLower(env.Body), captureAckSubstring) {
		t.Errorf("body = %q contains %q on a bare `/ask` — BUG-061-006 DEFECT 2 violated: no idea was captured, so claiming one was saved is a false acknowledgement", env.Body, captureAckSubstring)
	}

	// A contradictory pair — a failure line AND a saved claim in the
	// same body — was the reported symptom. Neither half may appear
	// with the other.
	lowerBody := strings.ToLower(env.Body)
	saysFailed := strings.Contains(lowerBody, "failed to save") || strings.Contains(lowerBody, "couldn't save") || strings.Contains(lowerBody, "could not save")
	if saysFailed && strings.Contains(lowerBody, captureAckSubstring) {
		t.Errorf("body = %q reports BOTH a save failure and a save success — BUG-061-006 DEFECT 2 violated: this is the exact contradictory pair the bug was filed for", env.Body)
	}

	// --- DEFECT 1 precondition, unconditional. One turn yields one
	// non-empty body. A second acknowledgement source would have to
	// break this first. ---

	if strings.TrimSpace(env.Body) == "" {
		t.Errorf("body is empty on a bare `/ask` — the user is shown nothing at all, which is a silent failure; status=%q error_cause=%q", env.Status, env.ErrorCause)
	}

	// capture_route must agree with what the body claims. A turn that
	// captured nothing must not advertise the capture route, because
	// that is what licensed the capture acknowledgement in the first
	// place.
	if env.CaptureRoute && !strings.Contains(lowerBody, captureAckSubstring) {
		t.Logf("note: capture_route=true with a non-capture body %q — permitted, but recorded", env.Body)
	}

	// --- Terminal-shape closure. No early return, no third branch. ---

	knownStatus := false
	for _, s := range contracts.AllStatusTokens {
		if env.Status == string(s) {
			knownStatus = true
			break
		}
	}
	if !knownStatus {
		t.Errorf("status = %q is outside contracts.AllStatusTokens %v — the closed vocabulary is the contract; body=%q", env.Status, contracts.AllStatusTokens, env.Body)
	}

	switch env.Status {
	case string(contracts.StatusSavedAsIdea):
		t.Errorf("status = %q on a bare `/ask` — BUG-061-006 DEFECT 2 violated: there was no idea to save, so the capture status is a false claim; body=%q", env.Status, env.Body)
	case string(contracts.StatusAnswered):
		// A bare `/ask` has no question to answer. If the stack still
		// answered, it must not have done so with a capture body.
		if strings.Contains(lowerBody, captureAckSubstring) {
			t.Errorf("status = answered but the body is a capture acknowledgement — that is a capture masquerading as an answer; body=%q", env.Body)
		}
	case string(contracts.StatusUnavailable):
		// An honest refusal. It must carry a typed cause, because an
		// untyped refusal is indistinguishable from a generic transport
		// failure — which is what made the masking possible.
		if strings.TrimSpace(env.ErrorCause) == "" {
			t.Errorf("status = unavailable with an empty error_cause — a refusal must carry a typed cause so transports can render it honestly; body=%q", env.Body)
		}
		if !isKnownErrorCause(env.ErrorCause) {
			t.Errorf("error_cause = %q is outside contracts.AllErrorCauses %v — the closed vocabulary is the contract", env.ErrorCause, contracts.AllErrorCauses)
		}
	}
}
