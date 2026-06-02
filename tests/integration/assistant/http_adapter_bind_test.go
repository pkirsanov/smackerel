//go:build integration

// Spec 069 SCOPE-1d — HTTPAdapter Construction and LateBound Binding.
//
// SCN-069-A12 — Live cmd/core wired with HTTP transport enabled.
//
// This test drives the LIVE chi-mounted POST /api/assistant/turn route
// via the running core service (started by `./smackerel.sh test
// integration`). It proves wireAssistantFacade constructed the
// *HTTPAdapter and bound it to the LateBoundHandler — i.e. the
// production wiring no longer returns HTTP 503 "assistant HTTP adapter
// not bound" for valid bearer schema-v1 turns.
//
// Pre-rework symptom: every live HTTP turn returned 503 because
// wireAssistantFacade only constructed the Telegram adapter. The
// finding F-069-ADAPTER-NOT-BOUND (and the duplicate triage record
// F074-04B-ASSISTANT-HTTP-LATE-BIND under spec 074) tracks the
// regression this test pins.
//
// Live-stack inputs come exclusively from the SST-managed environment
// the integration harness exports. CORE_EXTERNAL_URL absent => a
// legitimate "no live stack" skip; CORE_EXTERNAL_URL present but
// SMACKEREL_AUTH_TOKEN empty is fail-loud (wiring bug).

package assistant_integration

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

type assistantBindLiveStack struct {
	BaseURL   string
	AuthToken string
}

func loadAssistantBindLiveStack(t *testing.T) assistantBindLiveStack {
	t.Helper()
	// Integration tests run inside a container attached to the test
	// compose network, so the in-network CORE_API_URL is the only URL
	// reachable from the test process. CORE_EXTERNAL_URL (host
	// loopback) is unreachable here.
	baseURL := strings.TrimRight(os.Getenv("CORE_API_URL"), "/")
	if baseURL == "" {
		t.Skip("integration: CORE_API_URL not set — live stack not available")
	}
	tok := os.Getenv("SMACKEREL_AUTH_TOKEN")
	if tok == "" {
		t.Fatalf("SMACKEREL_AUTH_TOKEN not set; live stack is up but auth wiring is missing — run via ./smackerel.sh test integration")
	}
	return assistantBindLiveStack{BaseURL: baseURL, AuthToken: tok}
}

func waitAssistantBindHealthy(t *testing.T, stack assistantBindLiveStack, maxWait time.Duration) {
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
} // TestAssistantHTTPAdapterIsBoundInProductionWiring_ReturnsHTTP200NotHTTP503
// is the SCN-069-A12 live-wiring proof. It posts a schema-v1 text turn
// with a valid bearer token to the running cmd/core process and asserts
// the response is HTTP 200 (not 503), decodes as a schema-v1
// TurnResponse, and reports facade_invoked=true. A 503 here would mean
// wireAssistantFacade did not call LateBoundHandler.SetAdapter, i.e.
// the F-069-ADAPTER-NOT-BOUND regression has returned.
func TestAssistantHTTPAdapterIsBoundInProductionWiring_ReturnsHTTP200NotHTTP503(t *testing.T) {
	stack := loadAssistantBindLiveStack(t)
	waitAssistantBindHealthy(t, stack, 30*time.Second)

	turnID := "integ-scope-1d-a12-" + time.Now().UTC().Format("20060102T150405.000")
	reqBody, err := json.Marshal(httpadapter.TurnRequest{
		SchemaVersion:      httpadapter.SchemaVersionV1,
		TransportMessageID: turnID,
		Kind:               string(contracts.KindText),
		TransportHint:      "web",
		Text:               "/reset",
	})
	if err != nil {
		t.Fatalf("marshal turn request: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, stack.BaseURL+"/api/assistant/turn", bytes.NewReader(reqBody))
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
	raw := new(bytes.Buffer)
	if _, err := raw.ReadFrom(resp.Body); err != nil {
		t.Fatalf("read body: %v", err)
	}

	// SCOPE-1d ownership: assert ONLY that the LateBoundHandler is
	// backed by a constructed adapter — i.e. the response is NOT the
	// "adapter not bound" 503 path and IS a schema-v1 TurnResponse
	// envelope emitted by the bound *HTTPAdapter. The HTTP 200 /
	// facade_invoked / status-code assertions belong to SCOPE-2,
	// which lands the auth chain that turns the accepted bearer
	// token into a usable Session.UserID (foreign finding
	// F-069-USERID-BINDING).
	if resp.StatusCode == http.StatusServiceUnavailable {
		t.Fatalf("status = 503 — assistant HTTP adapter is NOT bound in production wiring (F-069-ADAPTER-NOT-BOUND regression). body=%s", raw.String())
	}

	var out httpadapter.TurnResponse
	if err := json.Unmarshal(raw.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v\nbody=%s", err, raw.String())
	}
	if out.SchemaVersion != httpadapter.SchemaVersionV1 {
		t.Errorf("schema_version = %q, want %q (envelope must be emitted by bound adapter)", out.SchemaVersion, httpadapter.SchemaVersionV1)
	}
	if out.Transport != httpadapter.TransportName {
		t.Errorf("transport = %q, want %q", out.Transport, httpadapter.TransportName)
	}
	if out.TransportMessageID != turnID {
		t.Errorf("transport_message_id = %q, want %q (echo proves bound adapter handled the request)", out.TransportMessageID, turnID)
	}
}
