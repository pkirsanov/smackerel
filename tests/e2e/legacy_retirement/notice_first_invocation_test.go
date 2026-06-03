//go:build e2e

// Spec 076 SCOPE-6d — TP-076-06-01 / SCN-075-A01.
//
// Live-stack e2e proof that a retired-command turn during the open
// window returns the OPTIONAL schema-v1 `notice` payload and the
// primary body remains non-empty (the notice is an addendum, never
// a replacement). This is the canonical-path mirror of the deep
// renderer test at tests/e2e/assistant/legacy_retirement_notice_test.go;
// it intentionally focuses on the wire contract only so SCOPE-6d's
// 10-test matrix stays narrow and orthogonal.

package legacyretirement_e2e

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

type liveStack struct {
	BaseURL     string
	AuthToken   string
	WindowState string
	WindowID    string
}

func loadStack(t *testing.T) liveStack {
	t.Helper()
	base := strings.TrimRight(os.Getenv("CORE_EXTERNAL_URL"), "/")
	if base == "" {
		t.Skip("e2e: CORE_EXTERNAL_URL not set — live stack not available")
	}
	tok := os.Getenv("SMACKEREL_AUTH_TOKEN")
	if tok == "" {
		t.Fatal("SMACKEREL_AUTH_TOKEN not set; live stack is up but auth wiring is missing — wiring bug")
	}
	state := os.Getenv("LEGACY_RETIREMENT_WINDOW_STATE")
	if state == "" {
		t.Fatal("LEGACY_RETIREMENT_WINDOW_STATE not set; spec 075 SST missing — wiring bug")
	}
	id := os.Getenv("LEGACY_RETIREMENT_WINDOW_ID")
	if id == "" {
		t.Fatal("LEGACY_RETIREMENT_WINDOW_ID not set; spec 075 SST missing — wiring bug")
	}
	return liveStack{BaseURL: base, AuthToken: tok, WindowState: state, WindowID: id}
}

func waitHealthy(t *testing.T, base string) {
	t.Helper()
	client := &http.Client{Timeout: 5 * time.Second}
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		req, _ := http.NewRequest(http.MethodGet, base+"/api/health", nil)
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("core not healthy at %s", base)
}

// waitAssistantReady probes /api/assistant/turn with a benign POST
// and returns once the response is not the readiness-stub
// "assistant_http_not_ready" 503 from the assistant HTTP wiring.
// The /api/health gate reports healthy before the assistant subsystem
// finishes wiring up, so live-stack assistant tests that don't wait
// here can flake on a cold stack.
func waitAssistantReady(t *testing.T, stack liveStack) {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		status, body := postTurn(t, stack, "ping", "readiness-probe-"+time.Now().UTC().Format("150405.000000"))
		if status != http.StatusServiceUnavailable {
			return
		}
		if !bytes.Contains(body, []byte("assistant_http_not_ready")) {
			return
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("assistant HTTP not ready after 60s at %s", stack.BaseURL)
}

func postTurn(t *testing.T, stack liveStack, text, turnID string) (int, []byte) {
	t.Helper()
	req := httpadapter.TurnRequest{
		SchemaVersion:      httpadapter.SchemaVersionV1,
		TransportMessageID: turnID,
		Kind:               string(contracts.KindText),
		TransportHint:      "web",
		Text:               text,
	}
	body, _ := json.Marshal(req)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	httpReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, stack.BaseURL+"/api/assistant/turn", bytes.NewReader(body))
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
		t.Fatalf("read body: %v", err)
	}
	return resp.StatusCode, buf.Bytes()
}

// TestRetirement_FirstInvocationShowsOneNoticeAndServesIntent drives
// SCN-075-A01 against the live stack: a fresh user invoking a retired
// command during the open window MUST receive (a) a populated notice
// addendum and (b) a non-empty primary body — proving the notice is
// an addendum, never a replacement.
func TestRetirement_FirstInvocationShowsOneNoticeAndServesIntent(t *testing.T) {
	stack := loadStack(t)
	if stack.WindowState != "open" {
		t.Skipf("LEGACY_RETIREMENT_WINDOW_STATE=%q — SCN-075-A01 only exercises the open branch", stack.WindowState)
	}
	waitHealthy(t, stack.BaseURL)
	waitAssistantReady(t, stack)

	turnID := "tp-076-06-01-" + time.Now().UTC().Format("20060102T150405.000000")
	status, raw := postTurn(t, stack, "/weather", turnID)
	if status != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", status, string(raw))
	}
	var out httpadapter.TurnResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode: %v\nbody=%s", err, string(raw))
	}
	if out.SchemaVersion != httpadapter.SchemaVersionV1 {
		t.Errorf("schema_version=%q, want %q", out.SchemaVersion, httpadapter.SchemaVersionV1)
	}
	if out.Notice == nil {
		t.Fatalf("response.notice nil for retired /weather during open window; body=%s", string(raw))
	}
	if out.Notice.Command != "/weather" {
		t.Errorf("notice.command=%q, want /weather", out.Notice.Command)
	}
	if strings.TrimSpace(out.Notice.ReplacementExample) == "" {
		t.Error("notice.replacement_example empty — SST copy missing")
	}
	if strings.TrimSpace(out.Notice.CopyKey) == "" {
		t.Error("notice.copy_key empty — ledger dedup key missing")
	}
	if out.Notice.WindowID != stack.WindowID {
		t.Errorf("notice.window_id=%q, want %q", out.Notice.WindowID, stack.WindowID)
	}
	// Adversarial: primary body MUST remain non-empty. A renderer
	// regression that swallowed the body would fail here.
	if strings.TrimSpace(out.Body) == "" {
		t.Fatalf("response.body empty — notice MUST NOT replace primary body; raw=%s", string(raw))
	}
}
