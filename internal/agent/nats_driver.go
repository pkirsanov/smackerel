// NATS-backed LLMDriver for the spec 037 Scope 5 execution loop.
//
// The Go executor calls Turn(); this implementation marshals the
// TurnRequest to JSON, requests it over the AGENT subject pair, and
// unmarshals the Python sidecar's normalized envelope into a
// TurnResponse. The reply path uses core NATS request/inbox semantics
// (mirrors the search.embed reply_subject pattern in
// internal/nats/client.go and ml/app/nats_client.py) so the request
// completes even when JetStream consumer lag would otherwise stall a
// stream-subscribed reply.
//
// The driver is safe for concurrent use; each Turn allocates its own
// inbox subscription via nc.RequestWithContext under the hood. There
// is no per-driver mutable state.

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/nats-io/nats.go"
)

// natsLLMDriver dispatches one TurnRequest over NATS and returns the
// Python sidecar's response.
type natsLLMDriver struct {
	nc      *nats.Conn
	subject string // "agent.invoke.request" by contract
}

// NewNATSLLMDriver builds a driver backed by an open core NATS
// connection. Defaults to the contract subject "agent.invoke.request".
func NewNATSLLMDriver(nc *nats.Conn) (LLMDriver, error) {
	return NewNATSLLMDriverOnSubject(nc, "agent.invoke.request")
}

// NewNATSLLMDriverOnSubject is the production constructor exposed for
// tests that need to address an alternate subject (e.g., to avoid
// racing with a co-tenant Python sidecar bound to the canonical
// subject in the same NATS server).
func NewNATSLLMDriverOnSubject(nc *nats.Conn, subject string) (LLMDriver, error) {
	if nc == nil {
		return nil, errors.New("agent.NewNATSLLMDriver: nc is required")
	}
	if subject == "" {
		return nil, errors.New("agent.NewNATSLLMDriver: subject is required")
	}
	return &natsLLMDriver{nc: nc, subject: subject}, nil
}

// natsRequestPayload extends TurnRequest with the reply_subject the
// Python sidecar uses to publish back to our inbox.
type natsRequestPayload struct {
	TurnRequest
	ReplySubject string `json:"reply_subject"`
}

// natsResponseEnvelope mirrors the JSON shape produced by
// ml/app/agent.handle_invoke.
type natsResponseEnvelope struct {
	ToolCalls []natsToolCall `json:"tool_calls"`
	Final     any            `json:"final"`
	Provider  string         `json:"provider"`
	Model     string         `json:"model"`
	Tokens    Tokens         `json:"tokens"`
	Outcome   string         `json:"outcome,omitempty"`
	Error     string         `json:"error,omitempty"`
	TraceID   string         `json:"trace_id,omitempty"`
}

type natsToolCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON-encoded by the sidecar
}

// Turn dispatches one LLM turn over NATS.
func (d *natsLLMDriver) Turn(ctx context.Context, req TurnRequest) (TurnResponse, error) {
	inbox := nats.NewInbox()
	sub, err := d.nc.SubscribeSync(inbox)
	if err != nil {
		return TurnResponse{}, fmt.Errorf("agent: subscribe inbox: %w", err)
	}
	defer func() { _ = sub.Unsubscribe() }()

	payload := natsRequestPayload{TurnRequest: req, ReplySubject: inbox}
	body, err := json.Marshal(payload)
	if err != nil {
		return TurnResponse{}, fmt.Errorf("agent: marshal request: %w", err)
	}
	if err := d.nc.Publish(d.subject, body); err != nil {
		return TurnResponse{}, fmt.Errorf("agent: publish request: %w", err)
	}

	msg, err := sub.NextMsgWithContext(ctx)
	if err != nil {
		// ctx-deadline manifests as ctx.Err() == DeadlineExceeded; the
		// executor's outer ctx check handles the timeout outcome.
		return TurnResponse{}, fmt.Errorf("agent: await response: %w", err)
	}

	var env natsResponseEnvelope
	if err := json.Unmarshal(msg.Data, &env); err != nil {
		return TurnResponse{}, fmt.Errorf("agent: unmarshal response: %w", err)
	}
	if env.Outcome == "provider-error" || env.Error != "" {
		return TurnResponse{}, fmt.Errorf("agent: provider error: %s", env.Error)
	}

	resp := TurnResponse{
		Provider: env.Provider,
		Model:    env.Model,
		Tokens:   env.Tokens,
	}
	for _, tc := range env.ToolCalls {
		resp.ToolCalls = append(resp.ToolCalls, LLMToolCall{
			Name:      tc.Name,
			Arguments: json.RawMessage(tc.Arguments),
		})
	}
	if env.Final != nil {
		switch v := env.Final.(type) {
		case string:
			resp.Final = json.RawMessage(v)
		default:
			b, mErr := json.Marshal(v)
			if mErr == nil {
				resp.Final = b
			}
		}
	}
	return resp, nil
}
