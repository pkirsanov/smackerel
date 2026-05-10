// Spec 044 Scope 03 — Test helpers exposed to external test packages
// (e.g. tests/integration). These constructors set ONLY the fields the
// per-user PASETO claim-binding chain consults; they intentionally do
// NOT wire the Telegram API client, the assembler, or any other
// runtime surface. Callers that need a fully-wired bot must use the
// production NewBot constructor.
//
// Why a separate file (no build tag)?
//
//   - tests/integration is package `integration` (not `telegram`), so
//     it cannot reach unexported fields like Bot.environment or
//     Bot.userMapping.
//   - Putting the helper in a `_test.go` file would make it invisible
//     to external test packages.
//   - Gating with `//go:build test` would force callers to compile with
//     a special tag, which the integration runner does not set.
//
// The helper is documented as test-only via the function name suffix
// (`ForTest`) and the `Test` prefix on the type name.
package telegram

// NewBotForTest constructs a minimal *Bot whose environment and
// userMapping fields are set so callers can exercise:
//
//   - Bot.resolveActorUserID(chatID) — the chat → user resolver
//   - PerUserTokenMinter.MintForChat(chatID) — the helper that calls
//     resolveActorUserID then mints a per-user PASETO
//
// All other fields stay at their zero values; callers MUST NOT invoke
// methods that touch the Telegram API client, the assembler, or the
// background polling loop.
func NewBotForTest(environment string, userMapping map[int64]string) *Bot {
	return &Bot{
		environment: environment,
		userMapping: userMapping,
	}
}
