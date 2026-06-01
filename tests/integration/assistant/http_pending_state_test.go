//go:build integration

// Spec 069 SCOPE-3 — HTTP pending-state isolation by (user, transport).
//
// TestAssistantHTTPPendingStateIsScopedByUserAndTransport — SCN-069-A03,
// SCN-069-A04.
//
// Drives the HTTP adapter directly (no live HTTP server) with a
// pending-state-aware recording facade that mirrors the assistant
// store contract: pending {ConfirmRef, DisambiguationRef} rows keyed
// by (UserID, Transport). The test asserts the HTTP adapter:
//
//   1. Forwards Kind=disambiguation requests with the wire
//      DisambiguationRef + DisambiguationChoice and Transport="web"
//      so the facade resolves the pending row exactly once.
//   2. Forwards Kind=confirm requests with the wire ConfirmRef +
//      ConfirmChoice and Transport="web" so the facade gates the
//      side-effect-bearing action exactly once.
//   3. Does NOT cross-resolve pending rows across users — a
//      callback from user B referencing user A's pending ref lands
//      against user B's (empty) pending and the gated action does
//      not execute.
//   4. Rejects stale callback refs (no matching pending) by routing
//      to the facade's capture-as-fallback path; the gated action
//      does not execute.
//
// The HTTP adapter is the unit-under-test for this scope; the
// facade behavior under pending-row keying is exercised by the
// confirm.Machine and facade unit tests. This integration test
// proves the HTTP wire layer preserves the (user, transport, ref,
// choice) tuple end-to-end.

package assistant_integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/assistant/httpadapter"
	"github.com/smackerel/smackerel/internal/auth"
)

// pendingFacade is a recording facade with per-(user, transport)
// pending state that mirrors the real assistant store key shape
// (uid|transport). It is intentionally minimal: enough to assert
// the HTTP adapter forwards the callback tuple verbatim.
type pendingFacade struct {
	mu          sync.Mutex
	calls       int
	delivered   []contracts.AssistantMessage
	pending     map[string]pendingRow
	actionFires int
	resetCalls  int
}

type pendingRow struct {
	confirmRef    string
	disambigRef   string
	disambigCount int
}

func (k pendingFacade) key(uid, tr string) string { return uid + "|" + tr }

func (p *pendingFacade) seed(uid, tr string, row pendingRow) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pending == nil {
		p.pending = map[string]pendingRow{}
	}
	p.pending[p.key(uid, tr)] = row
}

func (p *pendingFacade) Handle(_ context.Context, msg contracts.AssistantMessage) (contracts.AssistantResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	p.delivered = append(p.delivered, msg)
	row, hasPending := p.pending[p.key(msg.UserID, msg.Transport)]
	emit := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	switch msg.Kind {
	case contracts.KindDisambiguation:
		if hasPending && row.disambigRef != "" && row.disambigRef == msg.DisambiguationRef && msg.DisambiguationChoice >= 1 && msg.DisambiguationChoice <= row.disambigCount {
			delete(p.pending, p.key(msg.UserID, msg.Transport))
			p.actionFires++
			return contracts.AssistantResponse{
				Status:    contracts.StatusThinking,
				Body:      "resolved",
				EmittedAt: emit,
			}, nil
		}
		// Stale / cross-user / cross-transport — capture fallback.
		return contracts.AssistantResponse{
			Status:       contracts.StatusSavedAsIdea,
			CaptureRoute: true,
			Body:         "saved as an idea — no pending choice matched.",
			EmittedAt:    emit,
		}, nil
	case contracts.KindConfirm:
		if hasPending && row.confirmRef != "" && row.confirmRef == msg.ConfirmRef && msg.ConfirmChoice == contracts.ConfirmPositive {
			delete(p.pending, p.key(msg.UserID, msg.Transport))
			p.actionFires++
			return contracts.AssistantResponse{
				Status:    contracts.StatusReminderConfirmed,
				Body:      "action executed",
				EmittedAt: emit,
			}, nil
		}
		return contracts.AssistantResponse{
			Status:       contracts.StatusSavedAsIdea,
			CaptureRoute: true,
			Body:         "saved as an idea — no pending confirmation matched.",
			EmittedAt:    emit,
		}, nil
	default:
		if msg.Kind == contracts.KindReset {
			p.resetCalls++
			// Reset deletes only the (user, transport) row; other
			// transports for the same user remain intact (parity with
			// facade.contextStore.DeleteByKey).
			delete(p.pending, p.key(msg.UserID, msg.Transport))
			return contracts.AssistantResponse{
				Status:    contracts.StatusSavedAsIdea,
				Body:      "context reset.",
				EmittedAt: emit,
			}, nil
		}
		return contracts.AssistantResponse{
			Status:    contracts.StatusThinking,
			Body:      "ack",
			EmittedAt: emit,
		}, nil
	}
}

func newPendingAdapter(t *testing.T, facade contracts.Assistant) *httpadapter.HTTPAdapter {
	t.Helper()
	a, err := httpadapter.NewHTTPAdapter(httpadapter.Options{
		Facade:  facade,
		Capture: func(context.Context, string, string, string) {},
		Clock:   func() time.Time { return time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC) },
		Config: httpadapter.HTTPTransportConfig{
			Enabled:                true,
			SchemaVersion:          httpadapter.SchemaVersionV1,
			BodySizeMaxBytes:       1 << 20,
			TransportHintAllowlist: []string{"web", "mobile", "bridge"},
			RequiredScope:          "assistant.turn",
		},
	})
	if err != nil {
		t.Fatalf("NewHTTPAdapter: %v", err)
	}
	return a
}

func postCallback(t *testing.T, adapter *httpadapter.HTTPAdapter, userID string, req httpadapter.TurnRequest) httpadapter.TurnResponse {
	t.Helper()
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	httpReq := httptest.NewRequest(http.MethodPost, "/api/assistant/turn", bytes.NewReader(body))
	httpReq = httpReq.WithContext(auth.WithSession(httpReq.Context(), auth.Session{
		UserID: userID,
		Source: auth.SessionSourcePerUserToken,
	}))
	rr := httptest.NewRecorder()
	adapter.ServeHTTP(rr, httpReq)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var out httpadapter.TurnResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v\nbody=%s", err, rr.Body.String())
	}
	if out.SchemaVersion != httpadapter.SchemaVersionV1 {
		t.Errorf("schema_version = %q, want %q", out.SchemaVersion, httpadapter.SchemaVersionV1)
	}
	if out.Transport != httpadapter.TransportName {
		t.Errorf("transport = %q, want %q", out.Transport, httpadapter.TransportName)
	}
	if !out.FacadeInvoked {
		t.Errorf("facade_invoked = false; want true")
	}
	return out
}

func TestAssistantHTTPPendingStateIsScopedByUserAndTransport(t *testing.T) {
	t.Run("disambiguation_callback_resolves_pending_for_owning_user_and_web_transport", func(t *testing.T) {
		facade := &pendingFacade{}
		facade.seed("user-A", httpadapter.TransportName, pendingRow{disambigRef: "dr-A-1", disambigCount: 3})
		adapter := newPendingAdapter(t, facade)

		out := postCallback(t, adapter, "user-A", httpadapter.TurnRequest{
			SchemaVersion:        httpadapter.SchemaVersionV1,
			TransportMessageID:   "tm-disambig-A-1",
			Kind:                 string(contracts.KindDisambiguation),
			TransportHint:        "web",
			DisambiguationRef:    "dr-A-1",
			DisambiguationChoice: 2,
		})
		if out.Status != string(contracts.StatusThinking) {
			t.Errorf("status = %q, want %q (pending should have resolved)", out.Status, contracts.StatusThinking)
		}
		if out.CaptureRoute {
			t.Errorf("capture_route = true on resolved disambiguation; want false")
		}
		if facade.calls != 1 {
			t.Fatalf("facade calls = %d, want 1", facade.calls)
		}
		if facade.actionFires != 1 {
			t.Errorf("actionFires = %d, want 1 (resolved exactly once)", facade.actionFires)
		}
		delivered := facade.delivered[0]
		if delivered.Transport != httpadapter.TransportName {
			t.Errorf("delivered.Transport = %q, want %q", delivered.Transport, httpadapter.TransportName)
		}
		if delivered.UserID != "user-A" {
			t.Errorf("delivered.UserID = %q, want user-A", delivered.UserID)
		}
		if delivered.Kind != contracts.KindDisambiguation {
			t.Errorf("delivered.Kind = %q, want %q", delivered.Kind, contracts.KindDisambiguation)
		}
		if delivered.DisambiguationRef != "dr-A-1" {
			t.Errorf("delivered.DisambiguationRef = %q, want dr-A-1", delivered.DisambiguationRef)
		}
		if delivered.DisambiguationChoice != 2 {
			t.Errorf("delivered.DisambiguationChoice = %d, want 2", delivered.DisambiguationChoice)
		}
	})

	t.Run("confirm_callback_resolves_pending_and_executes_action_exactly_once", func(t *testing.T) {
		facade := &pendingFacade{}
		facade.seed("user-A", httpadapter.TransportName, pendingRow{confirmRef: "cr-A-1"})
		adapter := newPendingAdapter(t, facade)

		out := postCallback(t, adapter, "user-A", httpadapter.TurnRequest{
			SchemaVersion:      httpadapter.SchemaVersionV1,
			TransportMessageID: "tm-confirm-A-1",
			Kind:               string(contracts.KindConfirm),
			TransportHint:      "web",
			ConfirmRef:         "cr-A-1",
			ConfirmChoice:      string(contracts.ConfirmPositive),
		})
		if out.Status != string(contracts.StatusReminderConfirmed) {
			t.Errorf("status = %q, want %q", out.Status, contracts.StatusReminderConfirmed)
		}
		if facade.actionFires != 1 {
			t.Errorf("actionFires = %d, want exactly 1", facade.actionFires)
		}
		delivered := facade.delivered[0]
		if delivered.ConfirmRef != "cr-A-1" {
			t.Errorf("delivered.ConfirmRef = %q, want cr-A-1", delivered.ConfirmRef)
		}
		if delivered.ConfirmChoice != contracts.ConfirmPositive {
			t.Errorf("delivered.ConfirmChoice = %q, want %q", delivered.ConfirmChoice, contracts.ConfirmPositive)
		}
		if delivered.Transport != httpadapter.TransportName {
			t.Errorf("delivered.Transport = %q, want %q", delivered.Transport, httpadapter.TransportName)
		}
	})

	t.Run("cross_user_callback_does_not_resolve_other_users_pending", func(t *testing.T) {
		facade := &pendingFacade{}
		// User A has pending; user B does NOT.
		facade.seed("user-A", httpadapter.TransportName, pendingRow{confirmRef: "cr-A-secret"})
		adapter := newPendingAdapter(t, facade)

		// User B POSTs a confirm with user A's ref — should not execute.
		out := postCallback(t, adapter, "user-B", httpadapter.TurnRequest{
			SchemaVersion:      httpadapter.SchemaVersionV1,
			TransportMessageID: "tm-crossuser-B-1",
			Kind:               string(contracts.KindConfirm),
			TransportHint:      "web",
			ConfirmRef:         "cr-A-secret",
			ConfirmChoice:      string(contracts.ConfirmPositive),
		})
		if out.Status != string(contracts.StatusSavedAsIdea) {
			t.Errorf("status = %q, want %q (no pending for user-B)", out.Status, contracts.StatusSavedAsIdea)
		}
		if !out.CaptureRoute {
			t.Errorf("capture_route = false; want true on cross-user stale ref")
		}
		if facade.actionFires != 0 {
			t.Fatalf("actionFires = %d, want 0 (cross-user must NOT execute gated action)", facade.actionFires)
		}
		delivered := facade.delivered[0]
		if delivered.UserID != "user-B" {
			t.Errorf("delivered.UserID = %q, want user-B (HTTP adapter must NOT forge identity)", delivered.UserID)
		}
		// Confirm user A's pending row is still intact.
		if _, ok := facade.pending[facade.key("user-A", httpadapter.TransportName)]; !ok {
			t.Errorf("user-A pending was cleared by user-B callback; pending state cross-contaminated")
		}
	})

	t.Run("stale_callback_ref_rejects_without_executing_action", func(t *testing.T) {
		facade := &pendingFacade{}
		facade.seed("user-A", httpadapter.TransportName, pendingRow{disambigRef: "dr-A-real", disambigCount: 3})
		adapter := newPendingAdapter(t, facade)

		out := postCallback(t, adapter, "user-A", httpadapter.TurnRequest{
			SchemaVersion:        httpadapter.SchemaVersionV1,
			TransportMessageID:   "tm-stale-A-1",
			Kind:                 string(contracts.KindDisambiguation),
			TransportHint:        "web",
			DisambiguationRef:    "dr-A-STALE",
			DisambiguationChoice: 1,
		})
		if out.Status != string(contracts.StatusSavedAsIdea) {
			t.Errorf("status = %q, want %q (stale ref)", out.Status, contracts.StatusSavedAsIdea)
		}
		if !out.CaptureRoute {
			t.Errorf("capture_route = false; want true on stale disambig ref")
		}
		if facade.actionFires != 0 {
			t.Fatalf("actionFires = %d, want 0 (stale ref must NOT execute gated action)", facade.actionFires)
		}
		// Original pending row remains because stale ref did not match.
		if _, ok := facade.pending[facade.key("user-A", httpadapter.TransportName)]; !ok {
			t.Errorf("pending was cleared by stale ref; should remain intact for replay-safety")
		}
	})
}

// TestAssistantHTTPResetClearsOnlyWebTransportState — SCN-069-A05.
//
// Spec 069 SCOPE-4: the HTTP adapter forwards Kind=KindReset with
// Transport="web" so the facade deletes only the (user, web) pending
// row. Pending state for the same user on a different transport
// (e.g. telegram) MUST remain intact — parity with the facade's
// existing DeleteByKey(userID, transport) semantics.
func TestAssistantHTTPResetClearsOnlyWebTransportState(t *testing.T) {
	facade := &pendingFacade{}
	facade.seed("user-A", httpadapter.TransportName, pendingRow{confirmRef: "cr-web"})
	facade.seed("user-A", "telegram", pendingRow{confirmRef: "cr-telegram"})
	adapter := newPendingAdapter(t, facade)

	out := postCallback(t, adapter, "user-A", httpadapter.TurnRequest{
		SchemaVersion:      httpadapter.SchemaVersionV1,
		TransportMessageID: "tm-reset-A-1",
		Kind:               string(contracts.KindReset),
		TransportHint:      "web",
	})

	if out.Status != string(contracts.StatusSavedAsIdea) {
		t.Errorf("status = %q, want %q (reset acknowledgement)", out.Status, contracts.StatusSavedAsIdea)
	}
	if out.CaptureRoute {
		t.Errorf("capture_route = true on reset acknowledgement; want false")
	}
	if facade.resetCalls != 1 {
		t.Fatalf("resetCalls = %d, want 1", facade.resetCalls)
	}
	if facade.actionFires != 0 {
		t.Errorf("actionFires = %d, want 0 (reset must not fire gated actions)", facade.actionFires)
	}

	delivered := facade.delivered[0]
	if delivered.Kind != contracts.KindReset {
		t.Errorf("delivered.Kind = %q, want %q", delivered.Kind, contracts.KindReset)
	}
	if delivered.Transport != httpadapter.TransportName {
		t.Errorf("delivered.Transport = %q, want %q", delivered.Transport, httpadapter.TransportName)
	}
	if delivered.UserID != "user-A" {
		t.Errorf("delivered.UserID = %q, want user-A", delivered.UserID)
	}

	// Web row gone, telegram row intact.
	if _, ok := facade.pending[facade.key("user-A", httpadapter.TransportName)]; ok {
		t.Errorf("(user-A, web) pending row still present after HTTP reset; want cleared")
	}
	if _, ok := facade.pending[facade.key("user-A", "telegram")]; !ok {
		t.Errorf("(user-A, telegram) pending row cleared by HTTP reset; transport-scoping violated")
	}
}
