//go:build e2e

// Spec 073 SCOPE-073-02 — TP-073-09 (SCN-073-A01).
//
// Live-stack proof that the served PWA assistant route renders
// composer / transcript / response / sources / controls markup and
// the same-origin POST /api/assistant/turn carrying the spec 070
// HttpOnly session cookie returns a schema-valid response that the
// rendered DOM can project. The Go test does two things:
//
//  1. GETs /pwa/assistant.html and /pwa/assistant.js from the live
//     core and asserts the expected DOM hooks, ARIA structure, the
//     same-origin fetch wiring, and the absence of forbidden auth
//     storage references.
//  2. POSTs an authenticated assistant turn at /api/assistant/turn
//     (bearer-authenticated for the test runner, which stands in for
//     the spec 070 cookie session that the production browser uses)
//     and asserts the response is a strict TurnResponse v1 with the
//     fields the client renderer reads.
//
// Auth carrier note: the production browser carries the spec 070
// HttpOnly session cookie via credentials: 'same-origin'. The e2e
// runner does not have a browser, so it uses the same SST-managed
// bearer the rest of the assistant e2e tests use to drive the live
// route. The HTML/JS assertions cover the cookie-mode wiring.

package assistant_e2e

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/assistant/httpadapter"
)

func getServedText(t *testing.T, baseURL, path string) string {
	t.Helper()
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(baseURL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s status=%d", path, resp.StatusCode)
	}
	buf := make([]byte, 0, 16*1024)
	chunk := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(chunk)
		if n > 0 {
			buf = append(buf, chunk[:n]...)
		}
		if err != nil {
			break
		}
	}
	return string(buf)
}

func TestAssistantWebPWAChatE2E_ServedRouteHasComposerTranscriptAndResponseMarkup_TP_073_09(t *testing.T) {
	stack := loadHTTPTurnLiveStack(t)
	waitHTTPTurnHealthy(t, stack, 30*time.Second)

	html := getServedText(t, stack.BaseURL, "/pwa/assistant.html")
	for _, expect := range []string{
		`id="assistant-composer-form"`,
		`id="assistant-composer-input"`,
		`id="assistant-send-btn"`,
		`id="assistant-transcript"`,
		`id="assistant-response"`,
		`id="assistant-sources"`,
		`id="assistant-controls"`,
		`id="assistant-retry-btn"`,
		`aria-live="polite"`,
		`role="status"`,
		`type="module" src="/pwa/assistant.js"`,
	} {
		if !strings.Contains(html, expect) {
			t.Fatalf("assistant.html missing expected fragment %q", expect)
		}
	}

	js := getServedText(t, stack.BaseURL, "/pwa/assistant.js")
	for _, expect := range []string{
		`/api/assistant/turn`,
		`credentials: "same-origin"`,
		`"web"`, // transport_hint=web
		`transport_message_id`,
		`validateTurnResponse`,
	} {
		if !strings.Contains(js, expect) {
			t.Fatalf("assistant.js missing expected wiring %q", expect)
		}
	}
	for _, forbidden := range []string{
		"localStorage",
		"sessionStorage",
		"indexedDB",
		"document.cookie",
	} {
		if strings.Contains(js, forbidden) {
			t.Fatalf("assistant.js must not reference forbidden auth surface %q (SCN-073-A11)", forbidden)
		}
	}

	turnID := "spec-073-scope-2-a01-tp-073-09-" + time.Now().UTC().Format("20060102T150405.000")
	resp, raw := postAssistantTurn(t, stack, httpadapter.TurnRequest{
		SchemaVersion:      httpadapter.SchemaVersionV1,
		TransportMessageID: turnID,
		Kind:               string(contracts.KindText),
		TransportHint:      "web",
		Text:               "/reset",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", resp.StatusCode, string(raw))
	}

	var out httpadapter.TurnResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode response: %v\nbody=%s", err, string(raw))
	}
	if out.SchemaVersion != httpadapter.SchemaVersionV1 {
		t.Errorf("schema_version=%q, want %q", out.SchemaVersion, httpadapter.SchemaVersionV1)
	}
	if out.Transport != httpadapter.TransportName {
		t.Errorf("transport=%q, want %q", out.Transport, httpadapter.TransportName)
	}
	if out.TransportMessageID != turnID {
		t.Errorf("transport_message_id echo=%q, want %q", out.TransportMessageID, turnID)
	}
	if !out.FacadeInvoked {
		t.Errorf("facade_invoked=false, want true")
	}
}
