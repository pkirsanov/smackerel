//go:build e2e

// Spec 076 SCOPE-4a — shared facade-readiness wait for the NL routing
// e2e tests. The live stack's /api/health returns healthy as soon as
// the core HTTP server is listening, but the assistant facade wires
// asynchronously after the ML sidecar reports ready. Until that wire-
// up completes, POST /api/assistant/turn returns HTTP 503 with
// error_cause="assistant_http_not_ready". This helper polls a benign
// /reset turn until the facade reports invoked, then returns.

package assistant_e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/assistant/httpadapter"
)

// waitAssistantFacadeReady polls POST /api/assistant/turn with a
// benign /reset payload until the facade returns a 200 OR the
// supplied deadline elapses. On deadline elapsed the helper fails
// the test with the last observed status/body so the wiring bug is
// visible.
func waitAssistantFacadeReady(t *testing.T, stack httpTurnLiveStack, maxWait time.Duration) {
	t.Helper()
	deadline := time.Now().Add(maxWait)
	var lastStatus int
	var lastBody []byte
	for time.Now().Before(deadline) {
		req := httpadapter.TurnRequest{
			SchemaVersion:      httpadapter.SchemaVersionV1,
			TransportMessageID: "e2e-readiness-" + time.Now().UTC().Format("20060102T150405.000000"),
			Kind:               string(contracts.KindText),
			TransportHint:      "web",
			Text:               "/reset",
		}
		resp, raw := postAssistantTurnNoFatal(t, stack, req)
		lastStatus = resp.StatusCode
		lastBody = raw
		if resp.StatusCode == http.StatusOK {
			var env httpadapter.TurnResponse
			if err := json.Unmarshal(raw, &env); err == nil && env.FacadeInvoked {
				return
			}
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("e2e: assistant facade did not become ready within %s; last_status=%d body=%s",
		maxWait, lastStatus, string(lastBody))
}

// postAssistantTurnNoFatal is the non-fatal variant of postAssistantTurn
// for readiness polling: it returns errors as zero-value responses
// instead of failing the test, so the poll loop can retry.
func postAssistantTurnNoFatal(t *testing.T, stack httpTurnLiveStack, req httpadapter.TurnRequest) (*http.Response, []byte) {
	t.Helper()
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal turn request: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, stack.BaseURL+"/api/assistant/turn", strings.NewReader(string(body)))
	if err != nil {
		return &http.Response{StatusCode: 0}, nil
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+stack.AuthToken)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return &http.Response{StatusCode: 0}, nil
	}
	defer resp.Body.Close()
	buf := make([]byte, 0, 4096)
	chunk := make([]byte, 4096)
	for {
		n, _ := resp.Body.Read(chunk)
		if n > 0 {
			buf = append(buf, chunk[:n]...)
		}
		if n < len(chunk) {
			break
		}
	}
	return resp, buf
}
