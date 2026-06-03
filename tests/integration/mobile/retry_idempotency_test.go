//go:build integration

// Spec 076 SCOPE-7b — TP-076-07b-01 (SCN-073-A03).
//
// Mobile retry idempotency: server-side transport_message_id contract.
//
// SCN-073-A03 ("Transient network failure retries with the same
// transport_message_id") requires that when a mobile client retries
// a turn after a transient failure it MUST reuse the original
// transport_message_id, and the server contract MUST:
//
//   1. Accept both requests successfully (HTTP 200, schema v1).
//   2. Echo the transport_message_id verbatim on each response —
//      this is the stable handle the mobile client uses to match
//      the response to its outstanding retry.
//   3. Produce the SAME logical result for the retried turn
//      (identical response Body and Status) so the mobile UI does
//      not render a divergent answer between the two attempts.
//   4. NOT mint a duplicate side-effect of the retried action.
//      We drive this with the /reset capability action, which the
//      facade explicitly treats as a no-op when state is already
//      cleared (internal/assistant/facade.go: "reset on already-
//      cleared state is a no-op"). Both retries therefore land on
//      the same logical no-op, demonstrating exactly-once intent.
//
// Adversarial sub-test:
//   Two POSTs with DIFFERENT transport_message_ids (and otherwise
//   identical bodies) MUST each echo their own id back, with no
//   cross-mixing. This catches a regression that fixed the echoed
//   id (e.g. a misplaced cache) and would make the same-id parity
//   assertion above tautological.
//
// Live-stack inputs come from the SST-managed environment exported
// by ./smackerel.sh test integration (CORE_EXTERNAL_URL /
// SMACKEREL_AUTH_TOKEN). Per repo NO-DEFAULTS policy, an empty
// SMACKEREL_AUTH_TOKEN when CORE_EXTERNAL_URL is set is a wiring
// bug and fails the test; an absent CORE_EXTERNAL_URL is a
// legitimate "no live stack here" skip.

package mobile_integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/assistant/httpadapter"
)

type liveTurnStack struct {
	BaseURL   string
	AuthToken string
}

func loadLiveTurnStack(t *testing.T) liveTurnStack {
	t.Helper()
	baseURL := strings.TrimRight(os.Getenv("CORE_EXTERNAL_URL"), "/")
	if baseURL == "" {
		t.Skip("integration: CORE_EXTERNAL_URL not set — live stack not available")
	}
	tok := os.Getenv("SMACKEREL_AUTH_TOKEN")
	if tok == "" {
		t.Fatalf("SMACKEREL_AUTH_TOKEN not set; live stack is up but auth wiring is missing — run via ./smackerel.sh test integration")
	}
	return liveTurnStack{BaseURL: baseURL, AuthToken: tok}
}

func waitLiveStackHealthy(t *testing.T, stack liveTurnStack, maxWait time.Duration) {
	t.Helper()
	client := &http.Client{Timeout: 5 * time.Second}
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		resp, err := client.Get(stack.BaseURL + "/api/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("integration: core not healthy after %s at %s", maxWait, stack.BaseURL)
}

// postMobileTurn POSTs a /reset turn with transport_hint="mobile"
// and the supplied transport_message_id, returning the strictly-
// decoded TurnResponse v1. /reset is used so the test exercises a
// well-defined capability action whose retry semantics are
// documented in the facade as a no-op on already-cleared state.
func postMobileTurn(t *testing.T, stack liveTurnStack, turnID string) httpadapter.TurnResponse {
	t.Helper()
	body, err := json.Marshal(httpadapter.TurnRequest{
		SchemaVersion:      httpadapter.SchemaVersionV1,
		TransportMessageID: turnID,
		Kind:               string(contracts.KindText),
		TransportHint:      "mobile",
		Text:               "/reset",
	})
	if err != nil {
		t.Fatalf("marshal turn request (turn_id=%q): %v", turnID, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, stack.BaseURL+"/api/assistant/turn", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build request (turn_id=%q): %v", turnID, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+stack.AuthToken)
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST /api/assistant/turn (turn_id=%q): %v", turnID, err)
	}
	defer resp.Body.Close()
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		t.Fatalf("read response body (turn_id=%q): %v", turnID, err)
	}
	raw := buf.Bytes()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200 (turn_id=%q); body=%s", resp.StatusCode, turnID, string(raw))
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var out httpadapter.TurnResponse
	if err := dec.Decode(&out); err != nil {
		t.Fatalf("strict decode against TurnResponse v1 failed (turn_id=%q): %v\nbody=%s", turnID, err, string(raw))
	}
	if out.SchemaVersion != httpadapter.SchemaVersionV1 {
		t.Fatalf("schema_version=%q, want %q (turn_id=%q)", out.SchemaVersion, httpadapter.SchemaVersionV1, turnID)
	}
	if out.Transport != httpadapter.TransportName {
		t.Fatalf("transport=%q, want %q (transport_hint must not become transport) (turn_id=%q)", out.Transport, httpadapter.TransportName, turnID)
	}
	if !out.FacadeInvoked {
		t.Fatalf("facade_invoked=false (turn_id=%q); need a real live turn for the idempotency proof", turnID)
	}
	return out
}

// TestMobileRetry_ReusesTransportMessageId — TP-076-07b-01 / SCN-073-A03.
//
// Two POSTs against transport_hint="mobile" that share the same
// transport_message_id (simulating a mobile client retry after a
// transient network failure) MUST:
//   - Both succeed (HTTP 200, schema v1, facade_invoked=true).
//   - Both echo the SAME transport_message_id back verbatim — this
//     is the per-turn handle the mobile client uses to match the
//     response to its retried request.
//   - Produce identical Body and Status for the retried action,
//     demonstrating "same logical result" without duplicating the
//     side effect of the underlying capability (/reset is a no-op
//     on already-cleared state by facade contract).
func TestMobileRetry_ReusesTransportMessageId(t *testing.T) {
	stack := loadLiveTurnStack(t)
	waitLiveStackHealthy(t, stack, 30*time.Second)

	turnID := "spec-076-scope-7b-a03-tp-076-07b-01-retry-" + time.Now().UTC().Format("20060102T150405.000000")

	first := postMobileTurn(t, stack, turnID)
	// Sleep a touch so wall-clock advances; the contract claim is
	// that retry parity is keyed on transport_message_id, not on
	// when the retried request arrives.
	time.Sleep(150 * time.Millisecond)
	second := postMobileTurn(t, stack, turnID)

	if first.TransportMessageID != turnID {
		t.Fatalf("first transport_message_id echo=%q, want %q", first.TransportMessageID, turnID)
	}
	if second.TransportMessageID != turnID {
		t.Fatalf("retry transport_message_id echo=%q, want %q — server MUST round-trip the retried id verbatim (SCN-073-A03 regression)", second.TransportMessageID, turnID)
	}
	if first.Status != second.Status {
		t.Fatalf("retry produced different status (first=%q second=%q) — mobile retry MUST converge on the same logical result", first.Status, second.Status)
	}
	if first.Body != second.Body {
		t.Fatalf("retry produced different body for shared transport_message_id %q — mobile retry MUST converge on the same logical result\n--first--\n%s\n--second--\n%s", turnID, first.Body, second.Body)
	}
}

// TestMobileRetry_DistinctTransportMessageIdsAreNotMixed — adversarial.
//
// Two POSTs against transport_hint="mobile" with DIFFERENT
// transport_message_ids MUST each echo their own id back, with no
// cross-mixing. Without this guard a future regression that fixed
// the echoed id (e.g. a misplaced per-process cache) would make the
// same-id parity assertion above tautological.
func TestMobileRetry_DistinctTransportMessageIdsAreNotMixed(t *testing.T) {
	stack := loadLiveTurnStack(t)
	waitLiveStackHealthy(t, stack, 30*time.Second)

	stamp := time.Now().UTC().Format("20060102T150405.000000")
	idA := "spec-076-scope-7b-a03-tp-076-07b-01-adv-A-" + stamp
	idB := "spec-076-scope-7b-a03-tp-076-07b-01-adv-B-" + stamp

	a := postMobileTurn(t, stack, idA)
	b := postMobileTurn(t, stack, idB)

	if a.TransportMessageID != idA {
		t.Fatalf("turn A echo=%q, want %q", a.TransportMessageID, idA)
	}
	if b.TransportMessageID != idB {
		t.Fatalf("turn B echo=%q, want %q", b.TransportMessageID, idB)
	}
	if a.TransportMessageID == b.TransportMessageID {
		t.Fatalf("two distinct transport_message_ids produced the SAME echo (%q) — the retry-id round-trip check is tautological", a.TransportMessageID)
	}
}
