//go:build integration

// Spec 096 SCOPE-05 (SCN-096-G03) — live-stack budget enforcement.
//
// DEFERRED LIVE LEG. This test drives the REAL Go core /ask path against the
// disposable ephemeral test stack to prove an exhausted month-to-date
// model_usage_ledger refuses a paid-model /ask BEFORE any billable provider
// call (no /llm/chat dispatch fires). It uses SYNTHETIC rates + a SYNTHETIC
// ledger seed, NEVER a real provider or credential (test-isolation +
// env-pollution policy). The unit tests
// (internal/assistant/openknowledge/agent/budget_preflight_modelcost_test.go)
// already prove the "refused before dispatch" behaviour in isolation; this
// leg proves the live wiring (CostFn over the SST rate table + the DB-backed
// usageledger + migration 062) binds end-to-end.
//
// As of SCOPE-05 the live wiring is in place but the end-to-end run is
// deferred to a downstream bubbles.devops self-hosted dispatch (paired with the
// SCOPE-03/04/06/07 live hosted-provider legs), so this test t.Skip's with an
// explicit message rather than failing until those preconditions are seeded.
//
// To enable once the paid-provider test fixture + seeded ledger ship:
//
//	SPEC096_BUDGET_LIVE_CORE_URL=http://localhost:<port> \
//	SPEC096_BUDGET_LIVE_AUTH_TOKEN=<test-bearer> \
//	    ./smackerel.sh test integration --go-run TestAsk_PaidModelExhaustedBudget_RefusedBeforeProviderCall_Spec096
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestAsk_PaidModelExhaustedBudget_RefusedBeforeProviderCall_Spec096(t *testing.T) {
	coreURL := os.Getenv("SPEC096_BUDGET_LIVE_CORE_URL")
	authToken := os.Getenv("SPEC096_BUDGET_LIVE_AUTH_TOKEN")
	if coreURL == "" || authToken == "" {
		t.Skip("SPEC096_BUDGET_LIVE_CORE_URL / SPEC096_BUDGET_LIVE_AUTH_TOKEN not set; the paid-provider fixture + seeded model_usage_ledger are deferred to the SCOPE-05 self-hosted live dispatch (alongside the SCOPE-03/04/06/07 live legs).")
	}

	client := &http.Client{Timeout: 30 * time.Second}

	// Reachability probe — skip rather than fail when the core is not running.
	probe, _ := http.NewRequest(http.MethodGet, coreURL+"/api/health", nil)
	if resp, err := client.Do(probe); err != nil {
		t.Skipf("core %s not reachable: %v", coreURL, err)
	} else {
		resp.Body.Close()
	}

	// A paid-model /ask whose claim-bound caller has an exhausted
	// month-to-date spend MUST be refused with a budget-exhaustion reason
	// before any billable provider dispatch.
	body, err := json.Marshal(map[string]any{
		"prompt": "Summarize my notes from last week.",
		"model":  "anthropic/claude-3-5-sonnet",
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, coreURL+"/api/assistant/ask", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build /ask request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+authToken)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST /ask: %v", err)
	}
	defer resp.Body.Close()

	var decoded struct {
		Status            string `json:"status"`
		TerminationReason string `json:"termination_reason"`
		RefusalReason     string `json:"refusal_reason"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode /ask response: %v", err)
	}

	if !strings.EqualFold(decoded.Status, "refused") {
		t.Fatalf("status=%q want refused (an exhausted paid-model budget must refuse before any provider call)", decoded.Status)
	}
	if decoded.TerminationReason != "cap_usd" {
		t.Fatalf("termination_reason=%q want cap_usd", decoded.TerminationReason)
	}
	if !strings.Contains(strings.ToLower(decoded.RefusalReason), "budget") &&
		!strings.Contains(strings.ToLower(decoded.RefusalReason), "usd") {
		t.Fatalf("refusal_reason=%q does not name the exhausted USD budget", decoded.RefusalReason)
	}
}
