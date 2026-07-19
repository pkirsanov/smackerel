//go:build e2e

// Spec 073 SCOPE-073-02 — TP-073-10 (SCN-073-A03).
//
// Live regression that the web client's retry, which by contract
// reuses the original transport_message_id, results in server-side
// idempotent deduplication. Two POSTs with the SAME
// transport_message_id MUST return responses with the same
// trace.assistant_turn_id and the same body text — the server
// treated the second call as a retry of the first.
//
// Adversarial sub-test:
// Two POSTs with DIFFERENT transport_message_ids MUST yield
// DIFFERENT trace.assistant_turn_id values. This catches a future
// regression that silently makes assistant_turn_id stable per
// session (which would make the parity assertion above tautological).

package assistant_e2e

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/assistant/httpadapter"
)

const retryE2ETurnText = "/weather in barcelona"

func postNeutralTurn(t *testing.T, stack httpTurnLiveStack, turnID string) httpadapter.TurnResponse {
	t.Helper()
	resp, raw := postAssistantTurn(t, stack, httpadapter.TurnRequest{
		SchemaVersion:      httpadapter.SchemaVersionV1,
		TransportMessageID: turnID,
		Kind:               string(contracts.KindText),
		TransportHint:      "web",
		Text:               retryE2ETurnText,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200 (turn_id=%q); body=%s", resp.StatusCode, turnID, string(raw))
	}
	var out httpadapter.TurnResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode response (turn_id=%q): %v\nbody=%s", turnID, err, string(raw))
	}
	if out.TransportMessageID != turnID {
		t.Fatalf("transport_message_id echo=%q, want %q", out.TransportMessageID, turnID)
	}
	if !out.FacadeInvoked {
		t.Fatalf("facade_invoked=false for turn_id=%q; need a real live turn for the parity proof", turnID)
	}
	t.Logf("turn_id=%q assistant_turn_id=%q agent_trace_id=%q status=%q body=%q",
		turnID, out.Trace.AssistantTurnID, out.Trace.AgentTraceID, out.Status, out.Body)
	return out
}

func TestAssistantWebPWARetryE2E_SameTransportMessageIDDedupes_TP_073_10(t *testing.T) {
	stack := loadHTTPTurnLiveStack(t)
	waitHTTPTurnHealthy(t, stack, 30*time.Second)
	isolateSharedHTTPConversation(t)

	turnID := "spec-073-scope-2-a03-tp-073-10-retry-" + time.Now().UTC().Format("20060102T150405.000000")

	first := postNeutralTurn(t, stack, turnID)
	// Sleep a touch so wall-clock advances; dedup must still hold.
	time.Sleep(150 * time.Millisecond)
	second := postNeutralTurn(t, stack, turnID)

	if first.Trace.AssistantTurnID == "" || second.Trace.AssistantTurnID == "" {
		t.Fatalf("trace.assistant_turn_id must be non-empty: first=%q second=%q", first.Trace.AssistantTurnID, second.Trace.AssistantTurnID)
	}
	if first.Trace.AssistantTurnID != second.Trace.AssistantTurnID {
		t.Fatalf("retry with same transport_message_id produced different assistant_turn_id (%q vs %q) — dedup contract violated", first.Trace.AssistantTurnID, second.Trace.AssistantTurnID)
	}
	if first.Body != second.Body {
		t.Fatalf("retry with same transport_message_id produced different body:\n--first--\n%s\n--second--\n%s", first.Body, second.Body)
	}
}

func TestAssistantWebPWARetryE2E_DifferentTransportMessageIDsAreDistinct_TP_073_10_Adversarial(t *testing.T) {
	stack := loadHTTPTurnLiveStack(t)
	waitHTTPTurnHealthy(t, stack, 30*time.Second)
	isolateSharedHTTPConversation(t)

	stamp := time.Now().UTC().Format("20060102T150405.000000")
	idA := "spec-073-scope-2-a03-tp-073-10-adv-A-" + stamp
	idB := "spec-073-scope-2-a03-tp-073-10-adv-B-" + stamp

	a := postNeutralTurn(t, stack, idA)
	b := postNeutralTurn(t, stack, idB)

	if a.Trace.AssistantTurnID == "" || b.Trace.AssistantTurnID == "" {
		t.Fatalf("trace.assistant_turn_id must be non-empty: a=%q b=%q", a.Trace.AssistantTurnID, b.Trace.AssistantTurnID)
	}
	if a.Trace.AssistantTurnID == b.Trace.AssistantTurnID {
		t.Fatalf("two distinct transport_message_id values produced the SAME assistant_turn_id (%q) — the dedup-by-id check is tautological", a.Trace.AssistantTurnID)
	}
}
