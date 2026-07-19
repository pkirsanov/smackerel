//go:build e2e

// Spec 073 SCOPE-1e — Transport-hint parity regression (e2e-api).
//
// SCN-073-A08: posts the SAME scenario-neutral text turn against
// the live spec 069 endpoint twice — once with
// transport_hint="web" and once with transport_hint="mobile" —
// and proves the visible response shape is identical when the
// only varying input is the closed-vocabulary hint.
//
// Identity comparison is structural: every v1 wire field is
// compared EXCEPT the four fields that are intentionally
// per-request volatile and unrelated to scenario selection or
// shape (transport_message_id, trace.*, emitted_at). Any other
// divergence — status, body content, sources, controls, error
// cause, capture_route — would mean the hint is silently
// branching scenario selection, which violates the spec 069
// "hints are telemetry only" invariant.

package assistant_e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/assistant/httpadapter"
)

type parityLiveStack struct {
	BaseURL   string
	AuthToken string
}

const transportHintParityTurnText = "/weather in barcelona"

func loadParityLiveStack(t *testing.T) parityLiveStack {
	t.Helper()
	baseURL := strings.TrimRight(os.Getenv("CORE_EXTERNAL_URL"), "/")
	if baseURL == "" {
		t.Skip("e2e: CORE_EXTERNAL_URL not set — live stack not available")
	}
	tok := os.Getenv("SMACKEREL_AUTH_TOKEN")
	if tok == "" {
		t.Fatalf("SMACKEREL_AUTH_TOKEN not set; live stack is up but auth wiring is missing — run via ./smackerel.sh test e2e")
	}
	return parityLiveStack{BaseURL: baseURL, AuthToken: tok}
}

func waitParityHealthy(t *testing.T, stack parityLiveStack, maxWait time.Duration) {
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
	t.Fatalf("e2e: core not healthy after %s at %s", maxWait, stack.BaseURL)
}

func postParityTurn(t *testing.T, stack parityLiveStack, turnID, hint string) httpadapter.TurnResponse {
	t.Helper()
	body, err := json.Marshal(httpadapter.TurnRequest{
		SchemaVersion:      httpadapter.SchemaVersionV1,
		TransportMessageID: turnID,
		Kind:               string(contracts.KindText),
		TransportHint:      hint,
		Text:               transportHintParityTurnText,
	})
	if err != nil {
		t.Fatalf("marshal turn request: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, stack.BaseURL+"/api/assistant/turn", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+stack.AuthToken)
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST /api/assistant/turn (hint=%q): %v", hint, err)
	}
	defer resp.Body.Close()
	raw := new(bytes.Buffer)
	if _, err := raw.ReadFrom(resp.Body); err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (hint=%q); body=%s", resp.StatusCode, hint, raw.String())
	}
	dec := json.NewDecoder(bytes.NewReader(raw.Bytes()))
	dec.DisallowUnknownFields()
	var out httpadapter.TurnResponse
	if err := dec.Decode(&out); err != nil {
		t.Fatalf("strict decode against TurnResponse v1 failed (hint=%q): %v\nbody=%s", hint, err, raw.String())
	}
	return out
}

// shapeOnly returns a copy of the response with all per-request
// volatile fields zeroed so two responses with different hints can
// be compared for scenario-selection / response-shape parity.
func shapeOnly(r httpadapter.TurnResponse) httpadapter.TurnResponse {
	r.TransportMessageID = ""
	r.EmittedAt = ""
	r.Trace = httpadapter.TraceJSON{}
	return r
}

func requireNormalParityTurn(t *testing.T, hint string, response httpadapter.TurnResponse) {
	t.Helper()
	if strings.EqualFold(strings.TrimSpace(response.Body), "context reset.") {
		t.Fatalf("hint=%q parity fixture reached the /reset short circuit; parity requires an ordinary text turn", hint)
	}
	if response.Trace.AssistantTurnID == "" {
		t.Fatalf("hint=%q parity fixture produced no assistant_turn_id; parity requires an ordinary traced turn", hint)
	}
}

// TestAssistantTransportHintParity_WebAndMobileShareResponseShape —
// SCN-073-A08 live regression. Sends the SAME scenario-neutral
// turn body with hint="web" and hint="mobile" and asserts the
// visible response shape (status, body, sources, sources_overflow_count,
// confirm_card, disambiguation_prompt, error_cause, capture_route,
// facade_invoked, schema_version, transport) is byte-equal between
// the two runs. Any divergence outside the explicitly-excluded
// volatile fields proves the hint silently altered scenario
// selection or rendering — exactly what spec 069 forbids.
func TestAssistantTransportHintParity_WebAndMobileShareResponseShape(t *testing.T) {
	stack := loadParityLiveStack(t)
	waitParityHealthy(t, stack, 30*time.Second)
	isolation := isolateSharedHTTPConversation(t)

	stamp := time.Now().UTC().Format("20060102T150405.000")
	web := postParityTurn(t, stack, "spec-073-scope-1e-a08-parity-web-"+stamp, "web")
	isolation.reset(t)
	mobile := postParityTurn(t, stack, "spec-073-scope-1e-a08-parity-mobile-"+stamp, "mobile")
	requireNormalParityTurn(t, "web", web)
	requireNormalParityTurn(t, "mobile", mobile)

	webShape := shapeOnly(web)
	mobileShape := shapeOnly(mobile)
	if !reflect.DeepEqual(webShape, mobileShape) {
		webJSON, _ := json.MarshalIndent(webShape, "", "  ")
		mobileJSON, _ := json.MarshalIndent(mobileShape, "", "  ")
		t.Fatalf("transport_hint must not alter visible response shape.\n--- web (hint=\"web\") ---\n%s\n--- mobile (hint=\"mobile\") ---\n%s", webJSON, mobileJSON)
	}

	// Sanity: parity must not be the trivial "both responses
	// were empty/error" parity. Require the live facade actually
	// ran for both calls — this is what guarantees the parity
	// claim covers real scenario selection, not a short-circuit.
	if !web.FacadeInvoked || !mobile.FacadeInvoked {
		t.Fatalf("facade_invoked = web:%v mobile:%v; parity proof requires both to be true", web.FacadeInvoked, mobile.FacadeInvoked)
	}
	if web.SchemaVersion != httpadapter.SchemaVersionV1 || mobile.SchemaVersion != httpadapter.SchemaVersionV1 {
		t.Fatalf("schema_version = web:%q mobile:%q, want %q both", web.SchemaVersion, mobile.SchemaVersion, httpadapter.SchemaVersionV1)
	}
}

// TestAssistantTransportHintParity_AdversarialDivergentBodiesDetected —
// adversarial proof that the parity check above is NOT tautological.
// Two responses whose Body fields differ MUST be flagged as a
// shape divergence by shapeOnly+DeepEqual. If a future refactor
// accidentally widened the excluded-volatile list (e.g. zeroed
// Body too), this test would catch it.
func TestAssistantTransportHintParity_AdversarialDivergentBodiesDetected(t *testing.T) {
	a := httpadapter.TurnResponse{
		SchemaVersion:      httpadapter.SchemaVersionV1,
		Transport:          httpadapter.TransportName,
		TransportMessageID: "vol-A",
		Status:             "ok",
		Body:               "alpha body",
		FacadeInvoked:      true,
		EmittedAt:          "2026-06-01T00:00:00Z",
	}
	b := a
	b.TransportMessageID = "vol-B"
	b.EmittedAt = "2026-06-01T00:00:01Z"
	b.Body = "beta body" // <- the divergence the parity check must catch

	if reflect.DeepEqual(shapeOnly(a), shapeOnly(b)) {
		t.Fatal("shapeOnly+DeepEqual failed to flag divergent Body; parity check is tautological")
	}
}
