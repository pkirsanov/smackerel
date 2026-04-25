// Spec 037 Scope 9 — Telegram → agent bridge.
//
// The bridge converts an incoming Telegram message into an
// agent.IntentEnvelope, hands it to a Runner (router + executor),
// then renders the structured outcome to a Telegram reply via
// internal/agent/userreply.
//
// What the bridge guarantees (BS-014, BS-020, BS-021):
//
//   - The bot NEVER invents an answer. Every reply text is produced
//     by the userreply package from a concrete InvocationResult; no
//     code path here generates free-form text.
//   - Every reply ends with a trace ref so the operator can
//     investigate.
//   - Replies are ≤ 4 lines (enforced by the userreply package and
//     covered by unit tests there + e2e here).
//
// Why this is a separate file from bot.go:
//   - The existing bot dispatch routes by command (/find, /digest,
//     ...). The agent bridge is a new path that captures free-form
//     intents — wiring it into the bot's router is scope 10 work
//     (Migration Hooks). Scope 9 ships the bridge as a self-contained
//     callable unit so the API surface, tests, and (future) bot
//     glue can all use the same code path.
package telegram

import (
	"context"
	"errors"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/agent/userreply"
)

// AgentRunner is the bridge's only dependency on the agent runtime.
// The production wiring constructs one backed by the real router +
// executor; tests inject scripted runners that return canned outcomes.
//
// Mirrors api.AgentInvokeRunner deliberately so the same wiring object
// can satisfy both surfaces.
type AgentRunner interface {
	Invoke(ctx context.Context, env agent.IntentEnvelope) (*agent.InvocationResult, *agent.RoutingDecision)
	KnownIntents() []string
}

// AgentSender is the surface that actually delivers the rendered reply
// to the user. The production implementation wraps tgbotapi.SendMessage
// (see Bot.reply); tests substitute a recorder.
type AgentSender interface {
	SendMessage(ctx context.Context, chatID int64, text string) error
}

// AgentBridge owns the Runner+Sender pair. Construct one per process;
// it is safe for concurrent use.
type AgentBridge struct {
	Runner AgentRunner
	Sender AgentSender
}

// NewAgentBridge constructs the bridge. Both arguments are required;
// passing nil returns an error rather than producing a half-wired
// bridge that would later panic.
func NewAgentBridge(runner AgentRunner, sender AgentSender) (*AgentBridge, error) {
	if runner == nil {
		return nil, errors.New("telegram.NewAgentBridge: runner is required")
	}
	if sender == nil {
		return nil, errors.New("telegram.NewAgentBridge: sender is required")
	}
	return &AgentBridge{Runner: runner, Sender: sender}, nil
}

// Handle is the entry point a dispatcher (or a test) calls per
// inbound message. It builds the intent envelope, runs the agent,
// renders the reply via userreply, and sends it.
//
// Returns the InvocationResult and any send error for observability;
// the reply is sent best-effort regardless of whether the result was
// an `ok` outcome or any failure class. The bridge itself never
// short-circuits with a hard-coded message.
func (b *AgentBridge) Handle(ctx context.Context, chatID int64, text string) (*agent.InvocationResult, error) {
	if b == nil || b.Runner == nil || b.Sender == nil {
		return nil, errors.New("telegram.AgentBridge: not initialised")
	}

	env := agent.IntentEnvelope{
		Source:   "telegram",
		RawInput: text,
	}

	result, decision := b.Runner.Invoke(ctx, env)
	if result == nil {
		// Runner could not start. Fall back to the input-schema
		// violation reply so the user still gets a structured answer
		// (no free-form invention). The caller's logs must surface the
		// actual cause.
		fallback := &agent.InvocationResult{
			Outcome:       agent.OutcomeProviderError,
			OutcomeDetail: map[string]any{"error": "agent_runner_unavailable"},
		}
		reply := userreply.RenderTelegram(userreply.Inputs{
			Result:       fallback,
			KnownIntents: b.Runner.KnownIntents(),
		})
		_ = b.Sender.SendMessage(ctx, chatID, reply.Text)
		return nil, errors.New("telegram.AgentBridge: runner returned nil result")
	}

	reply := userreply.RenderTelegram(userreply.Inputs{
		Result:       result,
		Routing:      decision,
		KnownIntents: b.Runner.KnownIntents(),
	})
	if err := b.Sender.SendMessage(ctx, chatID, reply.Text); err != nil {
		return result, err
	}
	return result, nil
}
