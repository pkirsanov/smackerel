package httpadapter

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/auth"
)

type dedupTestFacade struct {
	mu           sync.Mutex
	calls        int
	started      chan struct{}
	release      <-chan struct{}
	fail         error
	captureRoute bool
}

func (f *dedupTestFacade) Handle(ctx context.Context, msg contracts.AssistantMessage) (contracts.AssistantResponse, error) {
	f.mu.Lock()
	f.calls++
	call := f.calls
	if f.started != nil && call == 1 {
		close(f.started)
	}
	f.mu.Unlock()
	if f.release != nil {
		select {
		case <-f.release:
		case <-ctx.Done():
			return contracts.AssistantResponse{}, ctx.Err()
		}
	}
	if f.fail != nil {
		return contracts.AssistantResponse{}, f.fail
	}
	now := time.Date(2026, 7, 19, 12, 0, call, 0, time.UTC)
	return contracts.AssistantResponse{
		Invocation:   &agent.InvocationResult{TraceID: fmt.Sprintf("trace-%s-%d", msg.UserID, call)},
		Status:       contracts.StatusAnswered,
		Body:         fmt.Sprintf("response-%s-%d", msg.UserID, call),
		CaptureRoute: f.captureRoute,
		EmittedAt:    now,
	}, nil
}

func (f *dedupTestFacade) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func newDedupTestHandler(t *testing.T, facade contracts.Assistant) http.Handler {
	return newDedupTestHandlerWithCapture(t, facade, func(context.Context, string, string, string) {})
}

func newDedupTestHandlerWithCapture(t *testing.T, facade contracts.Assistant, capture CaptureFn) http.Handler {
	t.Helper()
	adapter, err := NewHTTPAdapter(Options{
		Facade:  facade,
		Capture: capture,
		Clock:   time.Now,
		Config:  defaultConfig(),
	})
	if err != nil {
		t.Fatalf("NewHTTPAdapter: %v", err)
	}
	return middleware.RequestID(adapter)
}

func dedupRequestBody(t *testing.T, messageID, text string) []byte {
	t.Helper()
	body, err := json.Marshal(TurnRequest{
		SchemaVersion:      SchemaVersionV1,
		TransportMessageID: messageID,
		Kind:               string(contracts.KindText),
		TransportHint:      "web",
		Text:               text,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	return body
}

func performDedupRequest(handler http.Handler, userID string, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/assistant/turn", bytes.NewReader(body))
	req = req.WithContext(auth.WithSession(req.Context(), auth.Session{
		UserID: userID,
		Source: auth.SessionSourcePerUserToken,
	}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	return recorder
}

func decodeDedupResponse(t *testing.T, recorder *httptest.ResponseRecorder) TurnResponse {
	t.Helper()
	var response TurnResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, recorder.Body.String())
	}
	return response
}

func TestHTTPTurnDedup_SequentialReplayExecutesFacadeOnce(t *testing.T) {
	facade := &dedupTestFacade{}
	handler := newDedupTestHandler(t, facade)
	body := dedupRequestBody(t, "same-id", "weather in barcelona")
	firstRecorder := performDedupRequest(handler, "user-a", body)
	secondRecorder := performDedupRequest(handler, "user-a", body)
	first := decodeDedupResponse(t, firstRecorder)
	second := decodeDedupResponse(t, secondRecorder)
	if facade.callCount() != 1 {
		t.Fatalf("Facade.Handle calls=%d, want 1", facade.callCount())
	}
	if first.Trace.AssistantTurnID != second.Trace.AssistantTurnID || first.Body != second.Body || first.EmittedAt != second.EmittedAt {
		t.Fatalf("logical response was not replayed:\nfirst=%+v\nsecond=%+v", first, second)
	}
	if first.Trace.RequestID == "" || second.Trace.RequestID == "" || first.Trace.RequestID == second.Trace.RequestID {
		t.Fatalf("HTTP request IDs must be current per request: first=%q second=%q", first.Trace.RequestID, second.Trace.RequestID)
	}
}

func TestHTTPTurnDedup_ConcurrentReplayExecutesFacadeOnce(t *testing.T) {
	release := make(chan struct{})
	facade := &dedupTestFacade{started: make(chan struct{}), release: release}
	handler := newDedupTestHandler(t, facade)
	body := dedupRequestBody(t, "concurrent-id", "weather in barcelona")
	results := make(chan *httptest.ResponseRecorder, 2)
	go func() { results <- performDedupRequest(handler, "user-a", body) }()
	<-facade.started
	go func() { results <- performDedupRequest(handler, "user-a", body) }()
	close(release)
	first := decodeDedupResponse(t, <-results)
	second := decodeDedupResponse(t, <-results)
	if facade.callCount() != 1 {
		t.Fatalf("concurrent Facade.Handle calls=%d, want 1", facade.callCount())
	}
	if first.Trace.AssistantTurnID != second.Trace.AssistantTurnID || first.Body != second.Body {
		t.Fatalf("concurrent duplicate did not replay one result:\nfirst=%+v\nsecond=%+v", first, second)
	}
}

func TestHTTPTurnDedup_SameIDIsIsolatedAcrossUsers(t *testing.T) {
	facade := &dedupTestFacade{}
	handler := newDedupTestHandler(t, facade)
	body := dedupRequestBody(t, "shared-opaque-id", "weather in barcelona")
	userA := decodeDedupResponse(t, performDedupRequest(handler, "user-a", body))
	userB := decodeDedupResponse(t, performDedupRequest(handler, "user-b", body))
	if facade.callCount() != 2 {
		t.Fatalf("Facade.Handle calls=%d, want 2 for two users", facade.callCount())
	}
	if userA.Trace.AssistantTurnID == userB.Trace.AssistantTurnID || userA.Body == userB.Body {
		t.Fatalf("cross-user response leaked or collapsed:\nuserA=%+v\nuserB=%+v", userA, userB)
	}
}

func TestHTTPTurnDedup_ChangedPayloadConflictsWithoutReexecution(t *testing.T) {
	facade := &dedupTestFacade{}
	handler := newDedupTestHandler(t, facade)
	first := performDedupRequest(handler, "user-a", dedupRequestBody(t, "collision-id", "weather in barcelona"))
	second := performDedupRequest(handler, "user-a", dedupRequestBody(t, "collision-id", "weather in lisbon"))
	if first.Code != http.StatusOK || second.Code != http.StatusConflict {
		t.Fatalf("statuses first=%d second=%d, want 200/409; second body=%s", first.Code, second.Code, second.Body.String())
	}
	if facade.callCount() != 1 {
		t.Fatalf("Facade.Handle calls=%d, want 1 after collision", facade.callCount())
	}
	conflict := decodeDedupResponse(t, second)
	if conflict.ErrorCause != "transport_message_id_conflict" || conflict.FacadeInvoked {
		t.Fatalf("conflict response=%+v, want pre-facade transport_message_id_conflict", conflict)
	}
}

func TestHTTPTurnDedup_AcceptedFailureIsReplayed(t *testing.T) {
	facade := &dedupTestFacade{fail: errors.New("accepted failure")}
	handler := newDedupTestHandler(t, facade)
	body := dedupRequestBody(t, "failure-id", "weather in barcelona")
	first := decodeDedupResponse(t, performDedupRequest(handler, "user-a", body))
	second := decodeDedupResponse(t, performDedupRequest(handler, "user-a", body))
	if facade.callCount() != 1 {
		t.Fatalf("Facade.Handle calls=%d, want 1 for replayed failure", facade.callCount())
	}
	if first.ErrorCause != "assistant_turn_failed" || second.ErrorCause != first.ErrorCause || !first.FacadeInvoked || !second.FacadeInvoked {
		t.Fatalf("accepted failure not replayed safely:\nfirst=%+v\nsecond=%+v", first, second)
	}
}

func TestHTTPTurnDedup_CaptureRouteRunsCaptureOnce(t *testing.T) {
	facade := &dedupTestFacade{captureRoute: true}
	captureCalls := 0
	handler := newDedupTestHandlerWithCapture(t, facade, func(context.Context, string, string, string) {
		captureCalls++
	})
	body := dedupRequestBody(t, "capture-id", "remember this thought")
	first := performDedupRequest(handler, "user-a", body)
	second := performDedupRequest(handler, "user-a", body)
	if first.Code != http.StatusOK || second.Code != http.StatusOK {
		t.Fatalf("statuses first=%d second=%d, want 200/200", first.Code, second.Code)
	}
	if facade.callCount() != 1 || captureCalls != 1 {
		t.Fatalf("facade calls=%d capture calls=%d, want 1/1", facade.callCount(), captureCalls)
	}
}

func TestHTTPTurnDedup_CacheExpiresAndEvictsCompletedEntries(t *testing.T) {
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	cache, err := newTurnResponseCache(2, time.Minute, func() time.Time { return now })
	if err != nil {
		t.Fatalf("newTurnResponseCache: %v", err)
	}
	fingerprint := sha256.Sum256([]byte("request"))
	complete := func(messageID string) {
		lease, beginErr := cache.begin("user-a", messageID, fingerprint)
		if beginErr != nil || !lease.owner {
			t.Fatalf("begin %q: owner=%v err=%v", messageID, lease != nil && lease.owner, beginErr)
		}
		lease.complete(turnDedupResult{status: http.StatusOK, response: TurnResponse{TransportMessageID: messageID}})
	}
	complete("one")
	complete("two")
	complete("three")
	if lease, beginErr := cache.begin("user-a", "one", fingerprint); beginErr != nil || !lease.owner {
		t.Fatalf("oldest completed entry was not evicted: owner=%v err=%v", lease != nil && lease.owner, beginErr)
	}
	now = now.Add(2 * time.Minute)
	if lease, beginErr := cache.begin("user-a", "three", fingerprint); beginErr != nil || !lease.owner {
		t.Fatalf("expired entry was not removed: owner=%v err=%v", lease != nil && lease.owner, beginErr)
	}
}

func TestHTTPTurnDedup_CacheRejectsUniqueWorkWhenAllSlotsAreInFlight(t *testing.T) {
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	cache, err := newTurnResponseCache(1, time.Minute, func() time.Time { return now })
	if err != nil {
		t.Fatalf("newTurnResponseCache: %v", err)
	}
	fingerprint := sha256.Sum256([]byte("request"))
	first, err := cache.begin("user-a", "one", fingerprint)
	if err != nil || !first.owner {
		t.Fatalf("first begin: owner=%v err=%v", first != nil && first.owner, err)
	}
	if _, err := cache.begin("user-a", "two", fingerprint); !errors.Is(err, errTurnDedupCapacity) {
		t.Fatalf("second unique in-flight begin error=%v, want errTurnDedupCapacity", err)
	}
	if len(cache.entries) != 1 {
		t.Fatalf("cache entries=%d, want strict capacity 1", len(cache.entries))
	}
	first.complete(turnDedupResult{status: http.StatusOK})
	second, err := cache.begin("user-a", "two", fingerprint)
	if err != nil || !second.owner {
		t.Fatalf("completed entry did not make room: owner=%v err=%v", second != nil && second.owner, err)
	}
}
