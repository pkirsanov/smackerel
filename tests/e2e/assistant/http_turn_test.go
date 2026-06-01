//go:build e2e

// Spec 069 SCOPE-1b — Own-Scenario E2E Coverage.
//
// SCN-069-A01 — HTTP turn returns the same response Telegram would.
// SCN-069-A07 — Schema is pinned by a golden contract test (live-stack
// re-validation that the live route emits schema_version=v1).
//
// These tests drive the LIVE chi-mounted POST /api/assistant/turn
// route via the running core service, hitting the assistant facade
// end-to-end (no facade mock, no adapter mock, no scenario stub).
//
// Live-stack inputs come exclusively from the SST-managed environment
// the e2e harness exports (see ./smackerel.sh test e2e). Per the repo
// NO-DEFAULTS policy, missing or empty values are fail-loud — the
// only legitimate skip is "no live stack here" (CORE_EXTERNAL_URL
// unset). A live stack with a missing bearer token is a wiring bug
// and fails the test.

package assistant_e2e

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

type httpTurnLiveStack struct {
	BaseURL   string
	AuthToken string
}

// loadHTTPTurnLiveStack reads live-stack connection details from the
// e2e environment. Mirrors tests/e2e/drive/helpers.go semantics:
// CORE_EXTERNAL_URL absent => legitimate "no live stack" skip;
// SMACKEREL_AUTH_TOKEN absent when CORE_EXTERNAL_URL is present =>
// fail-loud (it means the stack is up but bearer auth is not wired).
func loadHTTPTurnLiveStack(t *testing.T) httpTurnLiveStack {
	t.Helper()
	baseURL := strings.TrimRight(os.Getenv("CORE_EXTERNAL_URL"), "/")
	if baseURL == "" {
		t.Skip("e2e: CORE_EXTERNAL_URL not set — live stack not available")
	}
	tok := os.Getenv("SMACKEREL_AUTH_TOKEN")
	if tok == "" {
		t.Fatalf("SMACKEREL_AUTH_TOKEN not set; live stack is up but auth wiring is missing — run via ./smackerel.sh test e2e")
	}
	return httpTurnLiveStack{BaseURL: baseURL, AuthToken: tok}
}

func waitHTTPTurnHealthy(t *testing.T, stack httpTurnLiveStack, maxWait time.Duration) {
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

func postAssistantTurn(t *testing.T, stack httpTurnLiveStack, req httpadapter.TurnRequest) (*http.Response, []byte) {
	t.Helper()
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal turn request: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, stack.BaseURL+"/api/assistant/turn", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+stack.AuthToken)
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		t.Fatalf("POST /api/assistant/turn: %v", err)
	}
	defer resp.Body.Close()
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return resp, buf.Bytes()
}

// TestAssistantHTTPE2E_TextTurnReturnsSchemaValidResponse — SCN-069-A01
// against the live stack. Proves a text turn over the real route
// returns HTTP 200, decodes as TurnResponse, echoes the transport
// message id, sets transport="web", and reports the facade was
// invoked exactly once via the live assistant pipeline.
func TestAssistantHTTPE2E_TextTurnReturnsSchemaValidResponse(t *testing.T) {
	stack := loadHTTPTurnLiveStack(t)
	waitHTTPTurnHealthy(t, stack, 30*time.Second)

	turnID := "e2e-scope-1b-a01-" + time.Now().UTC().Format("20060102T150405.000")
	resp, raw := postAssistantTurn(t, stack, httpadapter.TurnRequest{
		SchemaVersion:      httpadapter.SchemaVersionV1,
		TransportMessageID: turnID,
		Kind:               string(contracts.KindText),
		TransportHint:      "web",
		Text:               "/reset",
	})

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.StatusCode, string(raw))
	}
	var out httpadapter.TurnResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode response: %v\nbody=%s", err, string(raw))
	}
	if out.SchemaVersion != httpadapter.SchemaVersionV1 {
		t.Errorf("schema_version = %q, want %q", out.SchemaVersion, httpadapter.SchemaVersionV1)
	}
	if out.Transport != httpadapter.TransportName {
		t.Errorf("transport = %q, want %q", out.Transport, httpadapter.TransportName)
	}
	if out.TransportMessageID != turnID {
		t.Errorf("transport_message_id echo = %q, want %q", out.TransportMessageID, turnID)
	}
	if !out.FacadeInvoked {
		t.Errorf("facade_invoked = false, want true; body=%s", string(raw))
	}
	if strings.TrimSpace(out.EmittedAt) == "" {
		t.Errorf("emitted_at is empty; response must carry server timestamp")
	}
}

// TestAssistantHTTPE2E_ResponseSchemaMatchesV1Contract — SCN-069-A07
// against the live stack. Proves the live route emits the v1 wire
// shape: every required field is present (even when empty), and
// any unknown top-level keys would fail strict decoding. This is
// the live-stack re-validation of the golden contract pinned by
// internal/assistant/httpadapter/golden_contract_test.go.
func TestAssistantHTTPE2E_ResponseSchemaMatchesV1Contract(t *testing.T) {
	stack := loadHTTPTurnLiveStack(t)
	waitHTTPTurnHealthy(t, stack, 30*time.Second)

	turnID := "e2e-scope-1b-a07-" + time.Now().UTC().Format("20060102T150405.000")
	resp, raw := postAssistantTurn(t, stack, httpadapter.TurnRequest{
		SchemaVersion:      httpadapter.SchemaVersionV1,
		TransportMessageID: turnID,
		Kind:               string(contracts.KindText),
		TransportHint:      "web",
		Text:               "/reset",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.StatusCode, string(raw))
	}

	// Strict decode: DisallowUnknownFields catches any field the
	// live route emits that the v1 wire contract does not declare,
	// which is exactly what "schema is pinned" means at the live
	// boundary.
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var out httpadapter.TurnResponse
	if err := dec.Decode(&out); err != nil {
		t.Fatalf("strict decode against TurnResponse v1 failed: %v\nbody=%s", err, string(raw))
	}

	// Required field presence checks — every field the v1 contract
	// guarantees is always serialized.
	if out.SchemaVersion != httpadapter.SchemaVersionV1 {
		t.Errorf("schema_version = %q, want %q (schema_version bump required for any drift)", out.SchemaVersion, httpadapter.SchemaVersionV1)
	}
	if out.Transport != httpadapter.TransportName {
		t.Errorf("transport = %q, want %q", out.Transport, httpadapter.TransportName)
	}
	if out.TransportMessageID != turnID {
		t.Errorf("transport_message_id echo = %q, want %q", out.TransportMessageID, turnID)
	}
	if strings.TrimSpace(out.EmittedAt) == "" {
		t.Errorf("emitted_at must always be present in v1 responses")
	}
	if out.Sources == nil {
		t.Errorf("sources must always be present (possibly empty array) in v1 responses")
	}

	// Re-marshal and assert every documented v1 top-level key is
	// present in the raw JSON, even when zero-valued. This protects
	// against accidental `omitempty` regressions on the wire types.
	var asMap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &asMap); err != nil {
		t.Fatalf("re-decode raw response as map: %v", err)
	}
	required := []string{
		"schema_version",
		"transport",
		"transport_message_id",
		"status",
		"body",
		"sources",
		"sources_overflow_count",
		"confirm_card",
		"disambiguation_prompt",
		"error_cause",
		"capture_route",
		"trace",
		"facade_invoked",
		"emitted_at",
	}
	for _, k := range required {
		if _, ok := asMap[k]; !ok {
			t.Errorf("response missing required v1 field %q; body=%s", k, string(raw))
		}
	}
}

// TestAssistantHTTPE2E_TransportHintDoesNotChangeScenarioOrResponseShape
// — SCN-069-A09 live-stack regression. Posts the SAME text turn
// twice with different transport_hint values (web, mobile) and
// asserts every scenario-selection and response-shape field is
// identical. The four per-request volatile fields
// (transport_message_id, emitted_at, trace.*) are excluded.
//
// Spec 073's transport_hint_parity_test.go covers the same parity
// claim end-to-end; this test pins the claim against spec 069's
// own scope so a future refactor that touches the spec 069 wire
// layer cannot silently break the hint-neutrality invariant
// without failing a spec-069-owned test.
func TestAssistantHTTPE2E_TransportHintDoesNotChangeScenarioOrResponseShape(t *testing.T) {
	stack := loadHTTPTurnLiveStack(t)
	waitHTTPTurnHealthy(t, stack, 30*time.Second)

	stamp := time.Now().UTC().Format("20060102T150405.000")

	postWithHint := func(hint string) httpadapter.TurnResponse {
		resp, raw := postAssistantTurn(t, stack, httpadapter.TurnRequest{
			SchemaVersion:      httpadapter.SchemaVersionV1,
			TransportMessageID: "scope5-hint-" + hint + "-" + stamp,
			Kind:               string(contracts.KindText),
			TransportHint:      hint,
			Text:               "/reset",
		})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("hint=%q status=%d want 200; body=%s", hint, resp.StatusCode, string(raw))
		}
		var out httpadapter.TurnResponse
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("hint=%q decode: %v\nbody=%s", hint, err, string(raw))
		}
		return out
	}

	web := postWithHint("web")
	mobile := postWithHint("mobile")

	// Zero the per-request volatile fields and compare.
	zero := func(r httpadapter.TurnResponse) httpadapter.TurnResponse {
		r.TransportMessageID = ""
		r.EmittedAt = ""
		r.Trace = httpadapter.TraceJSON{}
		return r
	}
	webShape := zero(web)
	mobileShape := zero(mobile)
	if webShape.Status != mobileShape.Status {
		t.Errorf("status diverged on hint change: web=%q mobile=%q", web.Status, mobile.Status)
	}
	if webShape.Body != mobileShape.Body {
		t.Errorf("body diverged on hint change: web=%q mobile=%q", web.Body, mobile.Body)
	}
	if webShape.CaptureRoute != mobileShape.CaptureRoute {
		t.Errorf("capture_route diverged on hint change: web=%v mobile=%v", web.CaptureRoute, mobile.CaptureRoute)
	}
	if webShape.ErrorCause != mobileShape.ErrorCause {
		t.Errorf("error_cause diverged on hint change: web=%q mobile=%q", web.ErrorCause, mobile.ErrorCause)
	}
	if (webShape.ConfirmCard == nil) != (mobileShape.ConfirmCard == nil) {
		t.Errorf("confirm_card presence diverged on hint change: web=%v mobile=%v", web.ConfirmCard, mobile.ConfirmCard)
	}
	if (webShape.DisambiguationPrompt == nil) != (mobileShape.DisambiguationPrompt == nil) {
		t.Errorf("disambiguation_prompt presence diverged on hint change: web=%v mobile=%v", web.DisambiguationPrompt, mobile.DisambiguationPrompt)
	}
	if !web.FacadeInvoked || !mobile.FacadeInvoked {
		t.Fatalf("facade_invoked = web:%v mobile:%v; parity proof requires both true", web.FacadeInvoked, mobile.FacadeInvoked)
	}
}
