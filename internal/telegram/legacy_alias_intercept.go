// Spec 066 SCOPE-2 — Telegram retired-alias interceptor.
//
// Wires the spec 075 legacyretirement.Policy into the Telegram
// transport. For every inbound slash command that the SCOPE-1
// classifier flags as LegacyCommandRetiredAlias the interceptor:
//
//   - WindowOpen   → rewrites "/find ACL tags" → "find ACL tags"
//     using the SCOPE-1 PromptTemplate table and routes
//     the synthetic plain-text message through the
//     assistant adapter. When the policy reports
//     ShowNotice the user receives a one-time notice
//     rendered from LEGACY_RETIREMENT_NOTICE_COPY_PER_COMMAND;
//     the notice-ledger write happens inside the policy
//     via NoticeLedger.MarkShown.
//   - WindowPaused → rewrites + routes through the adapter exactly
//     like WindowOpen but suppresses any new notice
//     (spec 075 safety-mode contract).
//   - WindowClosed → replies with the canonical unknown-command copy
//     from LEGACY_RETIREMENT_POST_WINDOW_UNKNOWN_RESPONSE_COPY
//     and SHORT-CIRCUITS the legacy handler entirely
//     (SCN-066-A05).
//
// The interceptor owns no SST surface and no migration of its own;
// every persistent contract is provided by spec 075.
package telegram

import (
	"context"
	"fmt"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/smackerel/smackerel/internal/assistant/legacyretirement"
)

// LegacyAliasInterceptor wraps a spec 075 legacyretirement.Policy
// so the Telegram bot can intercept retired slash commands before
// dispatching them to the legacy command handlers.
type LegacyAliasInterceptor struct {
	policy legacyretirement.Policy
	clock  func() time.Time
}

// assistantFacadeUpstreamCtxKey is the context key set by callers
// that route a Telegram message through the assistant facade
// upstream (e.g. integration tests / future production wiring once
// the facade Policy is the source of truth). When present and set
// to true, the Telegram retired-alias interceptor short-circuits
// without rewriting or re-dispatching, preventing double-dispatch
// against the facade's own SCOPE-6.1 Policy.Handle path.
//
// Spec 075 SCOPE-6.4 (TP-075-23).
type assistantFacadeUpstreamKey struct{}

// AssistantFacadeUpstreamKey is the exported context key callers
// use to mark a turn as already-dispatched-by-facade. Value must
// be `true` to take effect.
var AssistantFacadeUpstreamKey = assistantFacadeUpstreamKey{}

// WithAssistantFacadeUpstream returns a derived context with the
// upstream flag set to true.
func WithAssistantFacadeUpstream(ctx context.Context) context.Context {
	return context.WithValue(ctx, AssistantFacadeUpstreamKey, true)
}

// IsAssistantFacadeUpstream reports whether ctx carries the
// upstream marker set by WithAssistantFacadeUpstream.
func IsAssistantFacadeUpstream(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, _ := ctx.Value(AssistantFacadeUpstreamKey).(bool)
	return v
}

// NewLegacyAliasInterceptor returns an interceptor bound to a
// non-nil Policy. clock is optional; nil falls back to time.Now.
func NewLegacyAliasInterceptor(policy legacyretirement.Policy, clock func() time.Time) (*LegacyAliasInterceptor, error) {
	if policy == nil {
		return nil, fmt.Errorf("telegram: LegacyAliasInterceptor requires non-nil Policy")
	}
	if clock == nil {
		clock = time.Now
	}
	return &LegacyAliasInterceptor{policy: policy, clock: clock}, nil
}

// SetLegacyAliasInterceptor wires the spec 066 SCOPE-2 retired-alias
// interceptor into an already-constructed Bot. Production wiring
// (cmd/core/wiring.go) calls this once after NewBot and before
// Start. Safe to call exactly once at startup; the field is
// read-only thereafter.
func (b *Bot) SetLegacyAliasInterceptor(i *LegacyAliasInterceptor) {
	b.legacyAliasInterceptor = i
}

// legacyAliasPromptFor returns the canonical natural-language prompt
// for a retired command token by substituting {args} in the SCOPE-1
// PromptTemplate. The lookup is total over the closed retired-alias
// table; ok=false signals the token is not retired.
func legacyAliasPromptFor(cmd, args string) (string, bool) {
	for _, a := range retiredAliasTable {
		if a.Command != cmd {
			continue
		}
		args = strings.TrimSpace(args)
		out := strings.ReplaceAll(a.PromptTemplate, "{args}", args)
		return strings.TrimSpace(out), true
	}
	return "", false
}

// interceptLegacyAlias runs the spec 066 SCOPE-2 decision for an
// inbound slash command. Returns (true, nil) when the bot MUST stop
// further dispatch for this message; (false, nil) when the
// interceptor is not wired, the command is not retired, or the
// policy did not match.
func (b *Bot) interceptLegacyAlias(ctx context.Context, msg *tgbotapi.Message, updateID int) (bool, error) {
	if b == nil || b.legacyAliasInterceptor == nil || msg == nil || msg.Chat == nil || !msg.IsCommand() {
		return false, nil
	}
	// Spec 075 SCOPE-6.4 (TP-075-23) — when the upstream facade is
	// already the Policy authority for this turn, the legacy alias
	// interceptor MUST short-circuit. It returns (false, nil) so
	// the bot's normal dispatch path proceeds and the assistant
	// facade's SCOPE-6.1 Policy.Handle path owns the decision.
	if IsAssistantFacadeUpstream(ctx) {
		return false, nil
	}
	cmd := msg.Command()
	if ClassifyCommand(cmd) != LegacyCommandRetiredAlias {
		return false, nil
	}
	chatID := msg.Chat.ID
	// resolveActorUserID returns "" in dev/test when no mapping is
	// configured; that's an acceptable bucket key for the ledger.
	userID, _ := b.resolveActorUserID(chatID)

	rawCmd := "/" + cmd
	args := strings.TrimSpace(msg.CommandArguments())
	rawText := rawCmd
	if args != "" {
		rawText = rawCmd + " " + args
	}

	decision, err := b.legacyAliasInterceptor.policy.Handle(ctx, legacyretirement.AssistantTurn{
		UserID:     userID,
		Transport:  "telegram",
		RawText:    rawText,
		ReceivedAt: b.legacyAliasInterceptor.clock(),
	})
	if err != nil {
		return false, fmt.Errorf("telegram: legacy alias policy: %w", err)
	}
	if !decision.Matched {
		return false, nil
	}

	switch decision.EffectiveState {
	case legacyretirement.WindowClosed:
		copyBody := strings.TrimSpace(decision.Command.ReplacementExample)
		if copyBody == "" {
			return false, fmt.Errorf("telegram: legacy alias closed-window copy empty for %q", cmd)
		}
		_ = b.reply(chatID, copyBody)
		return true, nil

	case legacyretirement.WindowOpen, legacyretirement.WindowPaused:
		rewritten, ok := legacyAliasPromptFor(cmd, args)
		if !ok {
			return false, nil
		}
		if decision.ShowNotice {
			notice := strings.TrimSpace(decision.Command.NoticeCopy)
			if notice == "" {
				return false, fmt.Errorf("telegram: legacy alias notice copy empty for %q", cmd)
			}
			_ = b.reply(chatID, notice)
		}
		if b.assistantAdapter == nil || !b.assistantAdapter.IsBound() {
			// No facade wired (dev/test install): emit the rewritten
			// prompt so the user always sees a response and the
			// legacy handler is NOT re-invoked.
			_ = b.reply(chatID, rewritten)
			return true, nil
		}
		synthetic := *msg
		synthetic.Text = rewritten
		synthetic.Entities = nil
		update := &tgbotapi.Update{UpdateID: updateID, Message: &synthetic}
		handled, herr := b.assistantAdapter.HandleUpdate(ctx, update)
		if herr != nil {
			return true, fmt.Errorf("telegram: legacy alias adapter handle: %w", herr)
		}
		if !handled {
			_ = b.reply(chatID, rewritten)
		}
		return true, nil

	default:
		return false, fmt.Errorf("telegram: legacy alias policy returned unknown state %q", decision.EffectiveState)
	}
}
