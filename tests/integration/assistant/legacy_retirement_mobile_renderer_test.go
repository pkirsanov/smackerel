//go:build integration

// Spec 075 SCOPE-6.4 — TP-075-22.
//
// Integration row: the mobile transport (HTTP adapter with
// TransportHint="mobile") surfaces the spec 075
// LegacyRetirementNotice in the chat thread without any modal /
// out-of-band interruption. The wire path is the same JSON schema
// the PWA consumes; mobile chat clients render the optional
// `notice` field as an inline chat addendum.
//
// Live-system rationale: drives the HTTP adapter end-to-end so the
// JSON wire surface (transport_hint=mobile + schema_version=v1 +
// optional notice) is exercised the same way the live core
// service exposes it.

package assistant_integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/assistant/httpadapter"
	"github.com/smackerel/smackerel/internal/auth"
)

type mobileNoticeFacade struct{}

func (mobileNoticeFacade) Handle(_ context.Context, _ contracts.AssistantMessage) (contracts.AssistantResponse, error) {
	return contracts.AssistantResponse{
		Status:    contracts.StatusThinking,
		Body:      "Sunny, 22°C tomorrow.",
		EmittedAt: time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC),
		LegacyRetirementNotice: &contracts.NoticePayload{
			Command:            "/weather",
			ReplacementExample: "weather in Barcelona tomorrow",
			CopyKey:            "spec066-weather",
			WindowID:           "tp-075-22-window",
		},
	}, nil
}

func TestMobileTransport_TP_075_22_NoticeInlineNotModal(t *testing.T) {
	a, err := httpadapter.NewHTTPAdapter(httpadapter.Options{
		Facade:  mobileNoticeFacade{},
		Capture: func(context.Context, string, string, string) {},
		Clock:   func() time.Time { return time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC) },
		Config: httpadapter.HTTPTransportConfig{
			Enabled:                true,
			SchemaVersion:          httpadapter.SchemaVersionV1,
			BodySizeMaxBytes:       1 << 20,
			ConversationTTL:        time.Hour,
			TransportHintAllowlist: []string{"web", "mobile", "bridge"},
			RequiredScope:          "assistant.turn",
		},
	})
	if err != nil {
		t.Fatalf("NewHTTPAdapter: %v", err)
	}

	req := httpadapter.TurnRequest{
		SchemaVersion:      httpadapter.SchemaVersionV1,
		TransportMessageID: "tm-mobile-075-22",
		Kind:               string(contracts.KindText),
		Text:               "/weather tomorrow",
		TransportHint:      "mobile",
	}
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	httpReq := httptest.NewRequest(http.MethodPost, "/api/assistant/turn", bytes.NewReader(body))
	httpReq = httpReq.WithContext(auth.WithSession(httpReq.Context(), auth.Session{
		UserID: "user-mobile-075-22",
		Source: auth.SessionSourcePerUserToken,
	}))
	rr := httptest.NewRecorder()
	a.ServeHTTP(rr, httpReq)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var out httpadapter.TurnResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v\nbody=%s", err, rr.Body.String())
	}
	if out.SchemaVersion != httpadapter.SchemaVersionV1 {
		t.Errorf("schema_version=%q, want v1 (notice is additive, MUST NOT bump version)", out.SchemaVersion)
	}
	if out.Notice == nil {
		t.Fatalf("Notice nil on mobile transport response; want populated payload. body=%s", rr.Body.String())
	}
	if out.Notice.Command != "/weather" {
		t.Errorf("notice.command=%q, want /weather", out.Notice.Command)
	}
	if out.Notice.ReplacementExample != "weather in Barcelona tomorrow" {
		t.Errorf("notice.replacement_example=%q", out.Notice.ReplacementExample)
	}
	if out.Notice.CopyKey != "spec066-weather" {
		t.Errorf("notice.copy_key=%q", out.Notice.CopyKey)
	}
	if out.Notice.WindowID != "tp-075-22-window" {
		t.Errorf("notice.window_id=%q", out.Notice.WindowID)
	}
	if out.Body != "Sunny, 22°C tomorrow." {
		t.Errorf("primary body must be preserved unmodified; got %q", out.Body)
	}

	// Non-modal proof: the response JSON has no separate alert/modal
	// channel — the notice is a top-level inline field that mobile
	// chat clients render in the thread alongside the primary body.
	// Adversarial: any future schema addition that introduces an
	// out-of-band modal/interruption channel for legacy retirement
	// must update this assertion deliberately.
	var raw map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode raw: %v", err)
	}
	for _, forbidden := range []string{"modal", "alert", "interrupt", "blocking_notice"} {
		if _, ok := raw[forbidden]; ok {
			t.Errorf("response carries forbidden out-of-band field %q; legacy retirement notice must be inline only", forbidden)
		}
	}
}
