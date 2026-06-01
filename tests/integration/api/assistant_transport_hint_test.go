//go:build integration

// Spec 073 SCOPE-1e — Closed-vocabulary transport_hint integration.
//
// SCN-073-A08: the web client sends transport_hint="web", the shared
// mobile client (iOS and Android) sends transport_hint="mobile",
// and the live spec 069 assistant endpoint accepts both values
// as telemetry-only. The hint MUST NOT alter scenario selection or
// response shape, and an unknown hint MUST be rejected pre-facade
// by the closed-vocabulary allowlist.
//
// Live-stack inputs come from the SST-managed environment exported
// by ./smackerel.sh test integration (CORE_EXTERNAL_URL /
// SMACKEREL_AUTH_TOKEN). Per repo NO-DEFAULTS policy, an empty
// SMACKEREL_AUTH_TOKEN when CORE_EXTERNAL_URL is present is a
// wiring bug and fails the test; an absent CORE_EXTERNAL_URL is a
// legitimate "no live stack here" skip.

package api_integration

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

func postTurnWithHint(t *testing.T, stack liveTurnStack, turnID, hint string) (*http.Response, []byte) {
	t.Helper()
	body, err := json.Marshal(httpadapter.TurnRequest{
		SchemaVersion:      httpadapter.SchemaVersionV1,
		TransportMessageID: turnID,
		Kind:               string(contracts.KindText),
		TransportHint:      hint,
		Text:               "/reset",
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
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return resp, buf.Bytes()
}

// TestAssistantTransportHint_WebAndMobileAreAcceptedAsTelemetry —
// SCN-073-A08. Posts the same scenario-neutral text turn against
// the live spec 069 endpoint three times: once with
// transport_hint="web" (the web client's value) and twice with
// transport_hint="mobile" (the shared Flutter client's value, used
// by both the iOS and Android surfaces). All three MUST return
// HTTP 200 and decode strictly against TurnResponse v1.
func TestAssistantTransportHint_WebAndMobileAreAcceptedAsTelemetry(t *testing.T) {
	stack := loadLiveTurnStack(t)
	waitLiveStackHealthy(t, stack, 30*time.Second)

	stamp := time.Now().UTC().Format("20060102T150405.000")
	cases := []struct {
		surface string
		hint    string
	}{
		{"web", "web"},
		{"mobile-ios", "mobile"},
		{"mobile-android", "mobile"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.surface, func(t *testing.T) {
			turnID := "spec-073-scope-1e-a08-" + c.surface + "-" + stamp
			resp, raw := postTurnWithHint(t, stack, turnID, c.hint)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want 200; body=%s", resp.StatusCode, string(raw))
			}
			dec := json.NewDecoder(bytes.NewReader(raw))
			dec.DisallowUnknownFields()
			var out httpadapter.TurnResponse
			if err := dec.Decode(&out); err != nil {
				t.Fatalf("strict decode against TurnResponse v1 failed (hint=%q): %v\nbody=%s", c.hint, err, string(raw))
			}
			if out.SchemaVersion != httpadapter.SchemaVersionV1 {
				t.Errorf("schema_version = %q, want %q", out.SchemaVersion, httpadapter.SchemaVersionV1)
			}
			if out.Transport != httpadapter.TransportName {
				t.Errorf("transport = %q, want %q (transport_hint must not become transport)", out.Transport, httpadapter.TransportName)
			}
			if out.TransportMessageID != turnID {
				t.Errorf("transport_message_id echo = %q, want %q", out.TransportMessageID, turnID)
			}
			if !out.FacadeInvoked {
				t.Errorf("facade_invoked = false, want true (hint=%q)", c.hint)
			}
		})
	}
}

// TestAssistantTransportHint_UnknownHintRejectedBeforeFacade —
// adversarial guard for SCN-073-A08. Proves the closed-vocabulary
// allowlist is enforced server-side: a value outside {"web",
// "mobile", "bridge"} MUST be rejected with HTTP 400 before the
// facade is invoked. This is the regression that would catch a
// future change loosening the allowlist into open vocabulary.
func TestAssistantTransportHint_UnknownHintRejectedBeforeFacade(t *testing.T) {
	stack := loadLiveTurnStack(t)
	waitLiveStackHealthy(t, stack, 30*time.Second)

	turnID := "spec-073-scope-1e-a08-adv-" + time.Now().UTC().Format("20060102T150405.000")
	resp, raw := postTurnWithHint(t, stack, turnID, "carrier-pigeon")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for unknown transport_hint; body=%s", resp.StatusCode, string(raw))
	}
}
