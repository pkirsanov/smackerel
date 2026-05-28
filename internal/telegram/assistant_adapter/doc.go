// Package assistant_adapter is the Spec 061 SCOPE-05 reference
// implementation of contracts.TransportAdapter for Telegram.
//
// The adapter is a thin shell over the capability facade
// (internal/assistant.Facade). It owns ALL Telegram-specific concerns
// (tgbotapi types, MarkdownV2 escaping, inline-keyboard callback_data
// encoding, message-length budgets) and translates them to/from the
// transport-agnostic contracts package.
//
// Architectural invariants (enforced by
// internal/assistant/contracts/architecture_test.go):
//
//   - The capability layer (internal/assistant/*) MUST NOT import
//     this package or any other transport implementation.
//   - This package MAY import internal/assistant/contracts and
//     tgbotapi; it MUST NOT import the parent internal/telegram
//     package (a cycle would result because the bot wires this
//     adapter at startup). All cross-bridge plumbing flows through
//     the function-pointer hooks in Options (CaptureFn, UserResolver,
//     Sender).
//
// Construction is decoupled from the live tgbotapi.BotAPI via the
// Sender interface; tests substitute a recording fake (see
// adapter_test.go) so render and translate behavior can be asserted
// without a live Telegram session.
//
// The HandleUpdate convenience method is the single entry point used
// by internal/telegram.Bot.handleMessage. It runs Identity →
// Translate → Assistant.Handle → Render in one call and returns
// (handled, err) so the bot can fall through to its legacy
// handleTextCapture path when the assistant is not wired (the
// BS-001 regression-safe path).
package assistant_adapter
