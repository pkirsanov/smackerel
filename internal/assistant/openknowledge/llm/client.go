// Package llm — Spec 064 SCOPE-04 LLM bridge client.
//
// Typed Go client for the Python ML sidecar's POST /llm/chat endpoint
// (see ml/app/routes/chat.py). The wire format is kept in lock-step
// with ml/app/schemas.py through the shared fixture at
// internal/assistant/openknowledge/llm/testdata/chat_fixture.json —
// both client_test.go and ml/tests/test_tool_roundtrip.py decode the
// same bytes.
//
// Location rationale: spec 061 design §10 + §11.3 forbid a top-level
// internal/assistant/llm/ package (it would re-introduce a parallel
// spec 037 substrate). This LLM bridge is scope-local to spec 064
// (open-ended knowledge agent), so it lives under
// internal/assistant/openknowledge/llm/.
//
// NO-DEFAULTS (Gate G028, smackerel-no-defaults): this package never
// reads its own configuration from the environment. The caller (the
// agent loop wired in SCOPE-09) MUST supply EndpointURL, AuthToken,
// and Timeout via Config. Empty values surface as typed construction
// errors rather than silent fallbacks.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Sentinel errors. Use errors.Is to identify them.
var (
	// ErrInvalidConfig is returned by NewClient when Config is empty.
	ErrInvalidConfig = errors.New("llm: invalid client config")
	// ErrMalformedResponse is returned when the sidecar's JSON body
	// does not satisfy the typed ChatResponse contract.
	ErrMalformedResponse = errors.New("llm: malformed sidecar response")
	// ErrSidecarStatus is returned when the sidecar replies with a
	// non-2xx status. The wrapped error preserves the status code in
	// its message.
	ErrSidecarStatus = errors.New("llm: sidecar status")
)

// Role enumerates the conversation roles supported by the bridge.
// Values are the wire strings; new roles MUST be added on both sides.
type Role string

const (
	RoleSystem     Role = "system"
	RoleUser       Role = "user"
	RoleAssistant  Role = "assistant"
	RoleToolCall   Role = "tool_call"
	RoleToolResult Role = "tool_result"
)

// StopReason enumerates the planner's terminal states for a single
// turn.
type StopReason string

const (
	StopEndTurn StopReason = "end_turn"
	StopToolUse StopReason = "tool_use"
)

// Tool is a JSONSchema-described capability surfaced to the planner.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// ToolCall is the planner's request to invoke a tool.
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ChatMessage is a single conversation turn. Field population by role
// MUST match ml/app/schemas.py::ChatMessage._validate_role_shape.
type ChatMessage struct {
	Role       Role       `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ChatRequest is the input envelope.
//
// Spec 096 SCOPE-03 — the per-request provider-credential seam. Provider,
// APIBase, APIKey, and ProviderParams are ADDITIVE: every one is omitempty,
// so a zero-value ChatRequest (the spec 064/088/089 no-override Ollama caller)
// serializes byte-for-byte the pre-096 wire shape and the sidecar's
// _dispatch_live takes the unchanged Ollama branch. A hosted dispatch carries
// Provider plus the per-request cleartext APIKey the Go core decrypted from the
// SCOPE-02 vault; Model continues to carry the BACKEND model id (the
// DispatchResolver strips the provider qualifier — the sidecar recomposes
// "<kind>/<backend-id>" from Provider + Model). APIKey is transient: it is
// never logged, traced, or persisted (design §6.2 / §11.5).
type ChatRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Tools       []Tool        `json:"tools,omitempty"`
	MaxTokens   *int          `json:"max_tokens,omitempty"`
	Temperature *float64      `json:"temperature,omitempty"`
	// Provider is the routing discriminant from the closed registry kind
	// vocabulary ("ollama" | "anthropic" | "openai" | "azure-foundry" |
	// "google" | "bedrock"). Empty (omitted) keeps the byte-for-byte Ollama
	// path on the sidecar.
	Provider string `json:"provider,omitempty"`
	// APIBase is the provider endpoint/base URL (today's OLLAMA_URL for
	// ollama; Azure endpoint; OpenAI base_url; …). nil omits the field.
	APIBase *string `json:"api_base,omitempty"`
	// APIKey is the cleartext credential the Go core decrypted per request.
	// nil for ollama. Used transiently by the sidecar; never logged/persisted.
	APIKey *string `json:"api_key,omitempty"`
	// ProviderParams carries non-secret per-kind routing extras (Azure
	// api_version+deployment, OpenAI org, Vertex project+location, Bedrock
	// region). nil omits the field.
	ProviderParams map[string]any `json:"provider_params,omitempty"`
}

// ChatResponse is the sidecar's reply.
type ChatResponse struct {
	StopReason StopReason `json:"stop_reason"`
	Text       *string    `json:"text,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	TokensUsed int        `json:"tokens_used"`
}

// Result is the agent-loop-friendly view of a ChatResponse. Exactly
// one of FinalText or ToolCalls is populated based on StopReason.
type Result struct {
	StopReason StopReason
	FinalText  string
	ToolCalls  []ToolCall
	TokensUsed int
}

// Config is the operator-supplied runtime contract.
type Config struct {
	EndpointURL string
	AuthToken   string
	Timeout     time.Duration
	TestModeHdr string // optional: X-OpenKnowledge-Test-Mode passthrough for contract tests.
	HTTPClient  *http.Client
}

// Client is the typed wrapper around the sidecar /llm/chat endpoint.
type Client struct {
	cfg        Config
	httpClient *http.Client
}

// NewClient validates cfg and returns a ready-to-use Client. Missing
// EndpointURL or non-positive Timeout fail loudly (G028).
func NewClient(cfg Config) (*Client, error) {
	var errs []string
	if strings.TrimSpace(cfg.EndpointURL) == "" {
		errs = append(errs, "EndpointURL is required")
	}
	if cfg.Timeout <= 0 {
		errs = append(errs, "Timeout must be > 0")
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("%w: %s", ErrInvalidConfig, strings.Join(errs, "; "))
	}
	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: cfg.Timeout}
	}
	return &Client{cfg: cfg, httpClient: hc}, nil
}

// Chat POSTs req to <EndpointURL>/llm/chat and returns the decoded
// Result. The HTTP client honours ctx for cancellation; cfg.Timeout
// caps the call.
func (c *Client) Chat(ctx context.Context, req ChatRequest) (Result, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return Result{}, fmt.Errorf("llm: marshal request: %w", err)
	}
	url := strings.TrimRight(c.cfg.EndpointURL, "/") + "/llm/chat"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return Result{}, fmt.Errorf("llm: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	if c.cfg.AuthToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.cfg.AuthToken)
	}
	if c.cfg.TestModeHdr != "" {
		httpReq.Header.Set("X-OpenKnowledge-Test-Mode", c.cfg.TestModeHdr)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return Result{}, fmt.Errorf("llm: transport: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return Result{}, fmt.Errorf("llm: read body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Result{}, fmt.Errorf("%w: %d: %s", ErrSidecarStatus, resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var parsed ChatResponse
	dec := json.NewDecoder(bytes.NewReader(respBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&parsed); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrMalformedResponse, err)
	}
	return interpret(parsed)
}

// interpret enforces the StopReason → field-population contract on the
// Go side. The Python schema enforces the same shape; mismatch implies
// the sidecar drifted from the contract.
func interpret(r ChatResponse) (Result, error) {
	switch r.StopReason {
	case StopEndTurn:
		if r.Text == nil {
			return Result{}, fmt.Errorf("%w: stop_reason=end_turn requires text", ErrMalformedResponse)
		}
		if len(r.ToolCalls) > 0 {
			return Result{}, fmt.Errorf("%w: stop_reason=end_turn must not carry tool_calls", ErrMalformedResponse)
		}
		return Result{StopReason: StopEndTurn, FinalText: *r.Text, TokensUsed: r.TokensUsed}, nil
	case StopToolUse:
		if r.Text != nil {
			return Result{}, fmt.Errorf("%w: stop_reason=tool_use must not carry text", ErrMalformedResponse)
		}
		if len(r.ToolCalls) == 0 {
			return Result{}, fmt.Errorf("%w: stop_reason=tool_use requires tool_calls", ErrMalformedResponse)
		}
		for i, tc := range r.ToolCalls {
			if strings.TrimSpace(tc.ID) == "" {
				return Result{}, fmt.Errorf("%w: tool_calls[%d] missing id", ErrMalformedResponse, i)
			}
			if strings.TrimSpace(tc.Name) == "" {
				return Result{}, fmt.Errorf("%w: tool_calls[%d] missing name", ErrMalformedResponse, i)
			}
		}
		return Result{StopReason: StopToolUse, ToolCalls: r.ToolCalls, TokensUsed: r.TokensUsed}, nil
	default:
		return Result{}, fmt.Errorf("%w: unknown stop_reason %q", ErrMalformedResponse, r.StopReason)
	}
}
