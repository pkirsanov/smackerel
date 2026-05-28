// Spec 061 SCOPE-05 §17.3 — webhook handler behavior matrix unit tests.
// Covers (a) missing-secret → 401, (b) wrong-secret (constant-time) →
// 401, (c) malformed JSON → 400, (d) empty body → 400, (e) valid +
// message → 200 + dispatch, (f) valid + callback → 200 + dispatch,
// (g) oversize body → 413, (h) non-POST → 405. Adversarial regression
// (TestWebhookHandler_UsesConstantTimeCompare) AST-greps the
// implementation file to fail if a future regression replaces the
// constant-time compare with a plain `==`.
package telegram

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const testWebhookSecret = "test-webhook-secret-deadbeef-cafebabe-12345"

// recordingDispatcher captures every dispatch call so tests can assert
// the §17.3 dispatch matrix without spinning up a real Bot.
type recordingDispatcher struct {
	mu        sync.Mutex
	messages  []*tgbotapi.Message
	callbacks []*tgbotapi.CallbackQuery
}

func (r *recordingDispatcher) DispatchMessage(_ context.Context, msg *tgbotapi.Message) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.messages = append(r.messages, msg)
}

func (r *recordingDispatcher) DispatchCallback(_ context.Context, cb *tgbotapi.CallbackQuery) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.callbacks = append(r.callbacks, cb)
}

func (r *recordingDispatcher) messageCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.messages)
}

func (r *recordingDispatcher) callbackCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.callbacks)
}

func newTestHandler(t *testing.T) (http.Handler, *recordingDispatcher) {
	t.Helper()
	disp := &recordingDispatcher{}
	h := NewWebhookHandler(WebhookHandlerOptions{
		Dispatcher: disp,
		Secret:     testWebhookSecret,
	})
	return h, disp
}

func postWebhook(handler http.Handler, body []byte, secretHeader string, method string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/v1/telegram/webhook", bytes.NewReader(body))
	if secretHeader != "" {
		req.Header.Set(WebhookHeaderSecretToken, secretHeader)
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

// (a) Missing secret header → 401.
func TestWebhookHandler_MissingSecretHeader_Returns401(t *testing.T) {
	h, disp := newTestHandler(t)
	rr := postWebhook(h, []byte(`{}`), "", http.MethodPost)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("missing header: want 401, got %d (body=%q)", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "missing_secret_token") {
		t.Errorf("missing header: want body containing missing_secret_token, got %q", rr.Body.String())
	}
	if disp.messageCount() != 0 || disp.callbackCount() != 0 {
		t.Errorf("missing header: dispatch must NOT fire; got %d/%d", disp.messageCount(), disp.callbackCount())
	}
}

// (b) Wrong secret → 401, no dispatch. Adversarial: the wrong value is
// a *similar* string (not empty, not obviously short) to ensure the
// constant-time compare path is the one being exercised. An equality
// regression that short-circuits on first-byte mismatch would still
// pass this functional assertion (it only catches outcome, not timing)
// — see TestWebhookHandler_UsesConstantTimeCompare for the source-
// level guard.
func TestWebhookHandler_WrongSecret_Returns401(t *testing.T) {
	h, disp := newTestHandler(t)
	wrong := "test-webhook-secret-deadbeef-cafebabe-12346" // last char differs
	if wrong == testWebhookSecret {
		t.Fatal("test bug: wrong secret should not equal real secret")
	}
	rr := postWebhook(h, []byte(`{}`), wrong, http.MethodPost)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("wrong secret: want 401, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "invalid_secret_token") {
		t.Errorf("wrong secret: want body containing invalid_secret_token, got %q", rr.Body.String())
	}
	if disp.messageCount() != 0 || disp.callbackCount() != 0 {
		t.Errorf("wrong secret: dispatch must NOT fire; got %d/%d", disp.messageCount(), disp.callbackCount())
	}
}

// (b2) A secret of different LENGTH must also fail (subtle.ConstantTimeCompare
// returns 0 when lengths differ).
func TestWebhookHandler_WrongSecretLength_Returns401(t *testing.T) {
	h, _ := newTestHandler(t)
	rr := postWebhook(h, []byte(`{}`), "short", http.MethodPost)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("wrong length: want 401, got %d", rr.Code)
	}
}

// (c) Malformed JSON → 400.
func TestWebhookHandler_MalformedJSON_Returns400(t *testing.T) {
	h, disp := newTestHandler(t)
	rr := postWebhook(h, []byte(`{not-json`), testWebhookSecret, http.MethodPost)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("malformed JSON: want 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "invalid_update_json") {
		t.Errorf("malformed JSON: want body invalid_update_json, got %q", rr.Body.String())
	}
	if disp.messageCount()+disp.callbackCount() != 0 {
		t.Errorf("malformed JSON: dispatch must NOT fire")
	}
}

// (d) Empty body → 400.
func TestWebhookHandler_EmptyBody_Returns400(t *testing.T) {
	h, _ := newTestHandler(t)
	rr := postWebhook(h, []byte(``), testWebhookSecret, http.MethodPost)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("empty body: want 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "empty_request_body") {
		t.Errorf("empty body: want body empty_request_body, got %q", rr.Body.String())
	}
}

// (e) Valid message update → 200 + DispatchMessage invoked exactly once
// with the parsed *tgbotapi.Message.
func TestWebhookHandler_ValidMessage_Dispatches200(t *testing.T) {
	h, disp := newTestHandler(t)
	body := []byte(`{"update_id":42,"message":{"message_id":7,"date":1700000000,"chat":{"id":12345,"type":"private"},"from":{"id":12345,"is_bot":false,"first_name":"Test"},"text":"hello world"}}`)
	rr := postWebhook(h, body, testWebhookSecret, http.MethodPost)
	if rr.Code != http.StatusOK {
		t.Fatalf("valid message: want 200, got %d (body=%q)", rr.Code, rr.Body.String())
	}
	if disp.messageCount() != 1 {
		t.Fatalf("valid message: want 1 dispatch, got %d", disp.messageCount())
	}
	if disp.callbackCount() != 0 {
		t.Errorf("valid message: callback dispatch must NOT fire")
	}
	if disp.messages[0].Text != "hello world" {
		t.Errorf("dispatched message text: want %q, got %q", "hello world", disp.messages[0].Text)
	}
	if disp.messages[0].Chat == nil || disp.messages[0].Chat.ID != 12345 {
		t.Errorf("dispatched chat ID: want 12345, got %+v", disp.messages[0].Chat)
	}
}

// (f) Valid callback update → 200 + DispatchCallback invoked.
func TestWebhookHandler_ValidCallback_Dispatches200(t *testing.T) {
	h, disp := newTestHandler(t)
	body := []byte(`{"update_id":43,"callback_query":{"id":"cb1","from":{"id":12345,"is_bot":false,"first_name":"Test"},"message":{"message_id":8,"date":1700000001,"chat":{"id":12345,"type":"private"}},"data":"assistant:confirm:ok"}}`)
	rr := postWebhook(h, body, testWebhookSecret, http.MethodPost)
	if rr.Code != http.StatusOK {
		t.Fatalf("valid callback: want 200, got %d", rr.Code)
	}
	if disp.callbackCount() != 1 {
		t.Fatalf("valid callback: want 1 dispatch, got %d", disp.callbackCount())
	}
	if disp.messageCount() != 0 {
		t.Errorf("valid callback: message dispatch must NOT fire")
	}
	if disp.callbacks[0].Data != "assistant:confirm:ok" {
		t.Errorf("dispatched callback data: want %q, got %q", "assistant:confirm:ok", disp.callbacks[0].Data)
	}
}

// (e2) Valid update with neither Message nor CallbackQuery → 200, no
// dispatch (e.g., an edited_message or channel_post type that this
// handler ignores by design).
func TestWebhookHandler_EmptyUpdate_Returns200_NoDispatch(t *testing.T) {
	h, disp := newTestHandler(t)
	body := []byte(`{"update_id":44}`)
	rr := postWebhook(h, body, testWebhookSecret, http.MethodPost)
	if rr.Code != http.StatusOK {
		t.Fatalf("empty update: want 200, got %d", rr.Code)
	}
	if disp.messageCount()+disp.callbackCount() != 0 {
		t.Errorf("empty update: dispatch must NOT fire")
	}
}

// (g) Oversize body → 413.
func TestWebhookHandler_OversizeBody_Returns413(t *testing.T) {
	h, disp := newTestHandler(t)
	// Build a body slightly over the cap.
	big := make([]byte, WebhookMaxBodyBytes+1024)
	for i := range big {
		big[i] = 'A'
	}
	rr := postWebhook(h, big, testWebhookSecret, http.MethodPost)
	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversize: want 413, got %d", rr.Code)
	}
	if disp.messageCount()+disp.callbackCount() != 0 {
		t.Errorf("oversize: dispatch must NOT fire")
	}
}

// (h) Non-POST method → 405.
func TestWebhookHandler_NonPOST_Returns405(t *testing.T) {
	h, _ := newTestHandler(t)
	for _, m := range []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		rr := postWebhook(h, []byte(`{}`), testWebhookSecret, m)
		if rr.Code != http.StatusMethodNotAllowed {
			t.Errorf("method=%s: want 405, got %d", m, rr.Code)
		}
	}
}

// Adversarial source-level guard: the constant-time compare MUST be
// the function authenticating the secret. A future regression that
// replaced subtle.ConstantTimeCompare with `provided == h.secret`
// would silently introduce a timing oracle; this test fails if the
// canonical token disappears from the implementation file.
func TestWebhookHandler_UsesConstantTimeCompare(t *testing.T) {
	src, err := os.ReadFile("webhook_handler.go")
	if err != nil {
		t.Fatalf("read webhook_handler.go: %v", err)
	}
	if !strings.Contains(string(src), "subtle.ConstantTimeCompare(") {
		t.Fatal("webhook_handler.go MUST authenticate the secret via subtle.ConstantTimeCompare (spec 061 §17.3 design.md auth model); the call site appears to have been removed or renamed — this would expose a timing attack")
	}
	// Belt-and-braces: an explicit `==` comparison between provided
	// and h.secret bytes would be the regression form. Reject any
	// such pattern.
	if strings.Contains(string(src), `provided == h.secret`) {
		t.Fatal("webhook_handler.go uses plain == on the secret; replace with subtle.ConstantTimeCompare (spec 061 §17.3)")
	}
}

// Construction guard: NewWebhookHandler panics on empty secret.
func TestNewWebhookHandler_EmptySecretPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on empty secret")
		}
	}()
	disp := &recordingDispatcher{}
	_ = NewWebhookHandler(WebhookHandlerOptions{Dispatcher: disp, Secret: ""})
}

// Construction guard: NewWebhookHandler panics when neither Bot nor
// Dispatcher is provided.
func TestNewWebhookHandler_NoDispatcherPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on nil dispatcher and nil bot")
		}
	}()
	_ = NewWebhookHandler(WebhookHandlerOptions{Secret: "x"})
}
