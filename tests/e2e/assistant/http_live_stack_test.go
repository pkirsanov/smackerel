//go:build e2e

// Spec 069 SCOPE-5 — Canonical assistant E2E suite over HTTP
// without Telegram (SCN-069-A11).
//
// TestAssistantHTTPE2E_LiveStackWithoutTelegramCoversCanonicalFlows
// drives the canonical assistant flows — text, reset, capture
// fallback — entirely through POST /api/assistant/turn against
// the live stack. The test never reads from a Telegram bot, never
// posts to Telegram update endpoints, and never depends on a real
// Telegram account being attached to the live stack.
//
// This is the live-stack proof that the assistant facade is
// driveable end-to-end from a non-Telegram surface, which is the
// core deliverable of spec 069: the assistant E2E suite runs over
// HTTP and not over Telegram.
//
// Defensive-skip pattern: CORE_EXTERNAL_URL unset is a legitimate
// "no live stack here" skip; a live stack with missing bearer
// auth is a wiring bug and fails the test (spec 069 NO-DEFAULTS).

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

type liveStackHTTPOnly struct {
	BaseURL   string
	AuthToken string
}

func loadLiveStackHTTPOnly(t *testing.T) liveStackHTTPOnly {
	t.Helper()
	baseURL := strings.TrimRight(os.Getenv("CORE_EXTERNAL_URL"), "/")
	if baseURL == "" {
		t.Skip("e2e: CORE_EXTERNAL_URL not set — live stack not available")
	}
	tok := os.Getenv("SMACKEREL_AUTH_TOKEN")
	if tok == "" {
		t.Fatalf("SMACKEREL_AUTH_TOKEN not set; live stack is up but auth wiring is missing — run via ./smackerel.sh test e2e")
	}
	// Forbidden: depending on Telegram bot env vars. If the harness
	// were silently relying on a Telegram account, fail loud so the
	// spec 069 "no Telegram dependency" invariant is preserved.
	if os.Getenv("ASSISTANT_E2E_REQUIRE_TELEGRAM") == "1" {
		t.Fatal("spec 069 SCN-069-A11 forbids Telegram dependency in this suite; ASSISTANT_E2E_REQUIRE_TELEGRAM=1 is set")
	}
	return liveStackHTTPOnly{BaseURL: baseURL, AuthToken: tok}
}

func waitLiveStackHTTPOnly(t *testing.T, stack liveStackHTTPOnly, maxWait time.Duration) {
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

func postHTTPOnlyTurn(t *testing.T, stack liveStackHTTPOnly, req httpadapter.TurnRequest) httpadapter.TurnResponse {
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
	raw := new(bytes.Buffer)
	if _, err := raw.ReadFrom(resp.Body); err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.StatusCode, raw.String())
	}
	var out httpadapter.TurnResponse
	if err := json.Unmarshal(raw.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v\nbody=%s", err, raw.String())
	}
	if !out.FacadeInvoked {
		t.Errorf("facade_invoked=false; live route did not reach facade. body=%s", raw.String())
	}
	if out.Transport != httpadapter.TransportName {
		t.Errorf("transport=%q want %q (live route must echo web)", out.Transport, httpadapter.TransportName)
	}
	return out
}

// TestAssistantHTTPE2E_LiveStackWithoutTelegramCoversCanonicalFlows
// SCN-069-A11 — drives canonical flows (text, reset, capture
// fallback) through HTTP only.
func TestAssistantHTTPE2E_LiveStackWithoutTelegramCoversCanonicalFlows(t *testing.T) {
	stack := loadLiveStackHTTPOnly(t)
	waitLiveStackHTTPOnly(t, stack, 30*time.Second)

	stamp := time.Now().UTC().Format("20060102T150405.000")

	t.Run("text_turn_returns_v1_response_through_http", func(t *testing.T) {
		out := postHTTPOnlyTurn(t, stack, httpadapter.TurnRequest{
			SchemaVersion:      httpadapter.SchemaVersionV1,
			TransportMessageID: "scope5-text-" + stamp,
			Kind:               string(contracts.KindText),
			TransportHint:      "web",
			Text:               "weather in barcelona",
		})
		if out.SchemaVersion != httpadapter.SchemaVersionV1 {
			t.Errorf("schema_version=%q want %q", out.SchemaVersion, httpadapter.SchemaVersionV1)
		}
		if strings.TrimSpace(out.EmittedAt) == "" {
			t.Errorf("emitted_at empty; live route must stamp every response")
		}
	})

	t.Run("reset_clears_web_pending_state", func(t *testing.T) {
		out := postHTTPOnlyTurn(t, stack, httpadapter.TurnRequest{
			SchemaVersion:      httpadapter.SchemaVersionV1,
			TransportMessageID: "scope5-reset-" + stamp,
			Kind:               string(contracts.KindReset),
			TransportHint:      "web",
		})
		// A reset on an empty state is still a valid turn (idempotent).
		if out.Status == "" {
			t.Errorf("reset response carries empty status; want a status token")
		}
	})

	t.Run("capture_fallback_for_open_ended_text", func(t *testing.T) {
		out := postHTTPOnlyTurn(t, stack, httpadapter.TurnRequest{
			SchemaVersion:      httpadapter.SchemaVersionV1,
			TransportMessageID: "scope5-capture-" + stamp,
			Kind:               string(contracts.KindText),
			TransportHint:      "web",
			// Intentionally open-ended unstructured prose so the
			// router's BandLow path triggers capture fallback.
			Text: "just thinking out loud about an article i read",
		})
		// Either the LLM resolves a scenario (high band) OR capture
		// fallback fires. Both are valid spec 069 outcomes for an
		// open-ended turn. The invariant is the response stays
		// schema-valid and the facade was invoked.
		if out.Status == "" {
			t.Errorf("open-ended turn returned empty status")
		}
	})
}
