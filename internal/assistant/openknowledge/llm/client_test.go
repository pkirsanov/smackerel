package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

func TestSpec102GoUsesTypedMLBoundary(t *testing.T) {
	requestObserved := make(chan struct{}, 1)
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/llm/chat" {
			t.Errorf("path = %s, want /llm/chat", r.URL.Path)
		}
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode typed request: %v", err)
		}
		for _, required := range []string{"model", "messages", "max_tokens", "temperature"} {
			if _, ok := body[required]; !ok {
				t.Errorf("typed request missing %q: %s", required, body)
			}
		}
		for _, forbidden := range []string{"options", "num_ctx", "keep_alive", "prompt", "stream"} {
			if _, ok := body[forbidden]; ok {
				t.Errorf("Go typed request leaked Ollama field %q: %s", forbidden, body[forbidden])
			}
		}
		t.Logf("typed boundary: method=%s path=%s fields=%d", r.Method, r.URL.Path, len(body))
		requestObserved <- struct{}{}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"stop_reason":"end_turn","text":"typed-ok","tokens_used":3}`))
	})

	maxTokens := 256
	temperature := 0.1
	result, err := client.Chat(context.Background(), ChatRequest{
		Model:       "gemma4:26b",
		Messages:    []ChatMessage{{Role: RoleUser, Content: "summarize"}},
		MaxTokens:   &maxTokens,
		Temperature: &temperature,
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	<-requestObserved
	if result.StopReason != StopEndTurn || result.FinalText != "typed-ok" || result.TokensUsed != 3 {
		t.Fatalf("unexpected typed response: %+v", result)
	}
	t.Log("typed boundary: Go received the sidecar response without importing Ollama response semantics")
}

func TestSpec102GoOllamaEndpointsAreReadOnlyTagsOnly(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	walkRoots := []string{filepath.Join(repoRoot, "cmd"), filepath.Join(repoRoot, "internal")}
	tagsOwners := map[string]map[string]bool{}
	var generationEndpoints []string

	for _, walkRoot := range walkRoots {
		err := filepath.WalkDir(walkRoot, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			parsed, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if parseErr != nil {
				return parseErr
			}
			relative, relErr := filepath.Rel(repoRoot, path)
			if relErr != nil {
				return relErr
			}
			relative = filepath.ToSlash(relative)
			methods := map[string]bool{}
			ast.Inspect(parsed, func(node ast.Node) bool {
				switch typed := node.(type) {
				case *ast.BasicLit:
					if typed.Kind != token.STRING {
						return true
					}
					value, unquoteErr := strconv.Unquote(typed.Value)
					if unquoteErr != nil {
						return true
					}
					if strings.Contains(value, "/api/chat") || strings.Contains(value, "/api/generate") {
						generationEndpoints = append(generationEndpoints, relative+":"+value)
					}
					if strings.Contains(value, "/api/tags") {
						tagsOwners[relative] = methods
					}
				case *ast.SelectorExpr:
					identifier, ok := typed.X.(*ast.Ident)
					if ok && identifier.Name == "http" {
						methods[typed.Sel.Name] = true
					}
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatalf("scan production Go under %s: %v", walkRoot, err)
		}
	}

	if len(generationEndpoints) != 0 {
		t.Fatalf("Go production code contains direct Ollama generation endpoints: %v", generationEndpoints)
	}
	expectedTagsOwners := map[string]bool{
		"internal/api/health.go":                              true,
		"internal/api/model_connections_probe.go":             true,
		"internal/assistant/openknowledge/catalog/adapter.go": true,
	}
	if len(tagsOwners) != len(expectedTagsOwners) {
		t.Fatalf("direct Ollama probe inventory = %v, want %v", tagsOwners, expectedTagsOwners)
	}
	for owner := range expectedTagsOwners {
		methods, ok := tagsOwners[owner]
		if !ok {
			t.Errorf("missing expected /api/tags owner %s", owner)
			continue
		}
		if !methods["MethodGet"] || methods["MethodPost"] {
			t.Errorf("%s must use GET-only for /api/tags; methods=%v", owner, methods)
		}
		t.Logf("read-only Ollama probe: %s -> GET /api/tags", owner)
	}
}

func strPtr(s string) *string { return &s }
