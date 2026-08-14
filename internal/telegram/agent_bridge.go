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
	"log/slog"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/agent/userreply"
	"github.com/smackerel/smackerel/internal/auth"
)

// PrincipalResolver maps an inbound chat_id to the authenticated session the
// agent invocation runs under (BUG-061-012 R3.2).
//
// It MUST return a non-error result ONLY for a chat that resolves to a real
// mapped user holding real recorded grants. Every refusal — unmapped chat,
// unreadable grants, empty user_id — returns an error, and the bridge then
// invokes with NO principal so corpus tools fail closed.
//
// The empty-user_id case is called out because Bot.resolveActorUserID returns
// ("", nil) for an unmapped chat outside production. A resolver that forwarded
// that would hand the agent a principal whose UserID is empty, which reads as
// authenticated while identifying nobody.
type PrincipalResolver func(ctx context.Context, chatID int64) (auth.Session, error)

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

	// Principal resolves the inbound chat to the session the invocation runs
	// under. Optional: a nil resolver means no principal is ever injected, so
	// every grant-gated tool fails closed on this surface. That is the safe
	// default — a bridge wired without a resolver loses corpus retrieval
	// rather than serving it to an unidentified caller.
	Principal PrincipalResolver
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

	// BUG-061-012 R3.2. Resolve the caller BEFORE Invoke, so the session is
	// already on the context every tool receives. Injecting it later, or
	// letting a tool read an identity out of the model's arguments, is the
	// defect this closes.
	//
	// A refusal is not fatal to the turn: the invocation proceeds WITHOUT a
	// principal and grant-gated tools refuse themselves (SCN-06). Dropping the
	// message instead would take out the ungated capabilities too, for a chat
	// that may simply not need them.
	ctx = b.withPrincipal(ctx, chatID)

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

// withPrincipal returns ctx carrying the resolved session, or ctx unchanged
// when the chat cannot be resolved to one.
//
// The empty-UserID re-check is deliberate belt-and-braces on top of the
// resolver's own contract. auth.WithSession would happily store a session
// identifying nobody, and a downstream tool testing only `ok` would then treat
// it as authenticated.
func (b *AgentBridge) withPrincipal(ctx context.Context, chatID int64) context.Context {
	if b.Principal == nil {
		return ctx
	}
	sess, err := b.Principal(ctx, chatID)
	if err != nil {
		slog.Warn("telegram agent bridge: no principal for chat; grant-gated tools will refuse",
			"chat_id", chatID, "error", err)
		return ctx
	}
	if sess.UserID == "" || sess.Source == "" {
		slog.Warn("telegram agent bridge: resolver returned an unusable principal; treating the chat as unidentified",
			"chat_id", chatID, "source", string(sess.Source))
		return ctx
	}
	return auth.WithSession(ctx, sess)
}
