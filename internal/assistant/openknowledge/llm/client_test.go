package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fixture mirrors the shared JSON used by the Python parity test.
type fixture struct {
	Request         ChatRequest     `json:"request"`
	ResponseEndTurn json.RawMessage `json:"response_end_turn"`
	ResponseToolUse json.RawMessage `json:"response_tool_use"`
	Comment         string          `json:"_comment"`
}

func loadFixture(t *testing.T) fixture {
	t.Helper()
	path := filepath.Join("testdata", "chat_fixture.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var f fixture
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	return f
}

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c, err := NewClient(Config{
		EndpointURL: srv.URL,
		Timeout:     2 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

func TestLLMClient_NewClient_RejectsEmptyConfig(t *testing.T) {
	if _, err := NewClient(Config{}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("want ErrInvalidConfig, got %v", err)
	}
	if _, err := NewClient(Config{EndpointURL: "http://x", Timeout: 0}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("want ErrInvalidConfig for zero timeout, got %v", err)
	}
}

func TestLLMClient_FixtureRequestRoundTrip(t *testing.T) {
	// Schema-parity guard: encode the fixture's request through Go,
	// decode it back, and confirm equivalence. The Python side decodes
	// the same JSON in test_tool_roundtrip.py.
	f := loadFixture(t)
	encoded, err := json.Marshal(f.Request)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var round ChatRequest
	dec := json.NewDecoder(bytes.NewReader(encoded))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&round); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if round.Model != f.Request.Model || len(round.Messages) != len(f.Request.Messages) {
		t.Fatalf("round trip mismatch: %+v vs %+v", round, f.Request)
	}
	if round.Messages[2].Role != RoleToolCall || round.Messages[3].Role != RoleToolResult {
		t.Fatalf("role parity broken: %+v", round.Messages)
	}
}

func TestLLMClient_FinalText(t *testing.T) {
	f := loadFixture(t)
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(f.ResponseEndTurn)
	})
	res, err := c.Chat(context.Background(), f.Request)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if res.StopReason != StopEndTurn || res.FinalText == "" || len(res.ToolCalls) != 0 {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestLLMClient_ToolUse(t *testing.T) {
	f := loadFixture(t)
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(f.ResponseToolUse)
	})
	res, err := c.Chat(context.Background(), f.Request)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if res.StopReason != StopToolUse || len(res.ToolCalls) != 1 || res.ToolCalls[0].ID == "" {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestLLMClient_MalformedJSON(t *testing.T) {
	f := loadFixture(t)
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("{not json"))
	})
	_, err := c.Chat(context.Background(), f.Request)
	if !errors.Is(err, ErrMalformedResponse) {
		t.Fatalf("want ErrMalformedResponse, got %v", err)
	}
}

func TestLLMClient_HTTPError(t *testing.T) {
	f := loadFixture(t)
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	_, err := c.Chat(context.Background(), f.Request)
	if !errors.Is(err, ErrSidecarStatus) {
		t.Fatalf("want ErrSidecarStatus, got %v", err)
	}
}

func TestLLMClient_Timeout(t *testing.T) {
	f := loadFixture(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte("{}"))
	}))
	t.Cleanup(srv.Close)
	c, err := NewClient(Config{EndpointURL: srv.URL, Timeout: 20 * time.Millisecond})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := c.Chat(context.Background(), f.Request); err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestLLMClient_ContractViolation_ToolUseMissingID(t *testing.T) {
	// G021 adversarial: a tool_use response whose tool_call omits id
	// MUST surface as ErrMalformedResponse, not pass silently.
	f := loadFixture(t)
	bad := ChatResponse{
		StopReason: StopToolUse,
		ToolCalls:  []ToolCall{{ID: "", Name: "unit_convert", Arguments: json.RawMessage(`{}`)}},
	}
	body, _ := json.Marshal(bad)
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	})
	_, err := c.Chat(context.Background(), f.Request)
	if !errors.Is(err, ErrMalformedResponse) {
		t.Fatalf("want ErrMalformedResponse, got %v", err)
	}
}

func TestLLMClient_ContractViolation_EndTurnWithToolCalls(t *testing.T) {
	f := loadFixture(t)
	bad := ChatResponse{
		StopReason: StopEndTurn,
		Text:       strPtr("hi"),
		ToolCalls:  []ToolCall{{ID: "x", Name: "y"}},
	}
	body, _ := json.Marshal(bad)
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	})
	_, err := c.Chat(context.Background(), f.Request)
	if !errors.Is(err, ErrMalformedResponse) {
		t.Fatalf("want ErrMalformedResponse, got %v", err)
	}
}

func strPtr(s string) *string { return &s }
