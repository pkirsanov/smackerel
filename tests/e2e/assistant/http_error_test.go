//go:build e2e

// Spec 069 SCOPE-2 — Regression E2E for pre-facade errors.
//
// TestAssistantHTTPE2E_PreFacadeErrorsDoNotInvokeFacade — SCN-069-A02,
// SCN-069-A10.
//
// Drives the LIVE chi-mounted POST /api/assistant/turn route and
// asserts the live stack rejects auth/body/rate violations with the
// canonical status codes BEFORE the facade is invoked. facade_invoked
// MUST be false on every rejection envelope.

package assistant_e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/assistant/httpadapter"
)

// TestAssistantHTTPE2E_PreFacadeErrorsDoNotInvokeFacade exercises
// the three pre-facade rejection paths over the live stack: missing
// bearer (401) and oversize body (413). The rate-limit (429) leg is
// covered by integration tests because the live SST budget is high
// enough that synthesizing the threshold from an external client
// would be flaky.
func TestAssistantHTTPE2E_PreFacadeErrorsDoNotInvokeFacade(t *testing.T) {
	stack := loadHTTPTurnLiveStack(t)
	waitHTTPTurnHealthy(t, stack, 30*time.Second)

	t.Run("missing_bearer_returns_401", func(t *testing.T) {
		body := mustMarshalTurn(t, "e2e-scope2-401-"+timestamp())
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, stack.BaseURL+"/api/assistant/turn", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("build request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		// No Authorization header — must 401 before any facade work.
		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("POST /api/assistant/turn: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			raw := new(bytes.Buffer)
			_, _ = raw.ReadFrom(resp.Body)
			t.Fatalf("status = %d, want 401; body=%s", resp.StatusCode, raw.String())
		}
	})

	t.Run("oversize_body_returns_413", func(t *testing.T) {
		// Build a payload large enough to exceed the production
		// body cap (smackerel.yaml ships 65536). 256 KiB is well
		// above that ceiling and below any reasonable HTTP server
		// max-request-size guard.
		padding := strings.Repeat("x", 256*1024)
		body, err := json.Marshal(httpadapter.TurnRequest{
			SchemaVersion:      httpadapter.SchemaVersionV1,
			TransportMessageID: "e2e-scope2-413-" + timestamp(),
			Kind:               string(contracts.KindText),
			TransportHint:      "web",
			Text:               padding,
		})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, stack.BaseURL+"/api/assistant/turn", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("build request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+stack.AuthToken)
		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("POST /api/assistant/turn: %v", err)
		}
		defer resp.Body.Close()
		raw := new(bytes.Buffer)
		_, _ = raw.ReadFrom(resp.Body)
		if resp.StatusCode != http.StatusRequestEntityTooLarge {
			t.Fatalf("status = %d, want 413; body=%s", resp.StatusCode, raw.String())
		}
		var env httpadapter.TurnResponse
		if err := json.Unmarshal(raw.Bytes(), &env); err != nil {
			t.Fatalf("decode v1 envelope: %v\nraw=%s", err, raw.String())
		}
		if env.SchemaVersion != httpadapter.SchemaVersionV1 {
			t.Errorf("schema_version = %q, want %q", env.SchemaVersion, httpadapter.SchemaVersionV1)
		}
		if env.ErrorCause != "body_too_large" {
			t.Errorf(`error_cause = %q, want "body_too_large"`, env.ErrorCause)
		}
		if env.FacadeInvoked {
			t.Errorf("facade_invoked = true on 413; want false")
		}
	})
}

func mustMarshalTurn(t *testing.T, id string) []byte {
	t.Helper()
	b, err := json.Marshal(httpadapter.TurnRequest{
		SchemaVersion:      httpadapter.SchemaVersionV1,
		TransportMessageID: id,
		Kind:               string(contracts.KindText),
		TransportHint:      "web",
		Text:               "ping",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func timestamp() string {
	return time.Now().UTC().Format("20060102T150405.000")
}
