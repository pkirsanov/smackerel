// Spec 075 SCOPE-6.1 — structured deprecation-notice payload attached
// to AssistantResponse when the facade Policy decides an open
// deprecation-window turn is the first one in the dedup ledger for
// (user, retired_command, window_id).
//
// NoticePayload is transport-neutral: per-transport renderers (PWA,
// WhatsApp, Mobile, Telegram) read it and render a one-line addendum
// without blocking the primary NL response. The facade Policy
// populates Command/ReplacementExample/CopyKey/WindowID; renderers
// MUST NOT mutate the payload.

package contracts

// NoticePayload is the legacy-retirement deprecation notice metadata
// transports render alongside the primary response body. Fields are
// exactly the contract listed in scopes.md §"New Types & Signatures"
// for spec 075 Scope 6:
//
//	NoticePayload{Command, ReplacementExample, CopyKey, WindowID}
//
// CopyKey is a stable catalog identifier (spec 066 ID) renderers use
// to look up locale-specific copy in their own copy tables; it is
// NOT user-facing text.
type NoticePayload struct {
	// Command is the retired command token the user typed
	// (e.g. "/weather"). Used by renderers to compose the
	// addendum.
	Command string

	// ReplacementExample is the plain-English alternative shown
	// alongside the notice (e.g. "weather in Barcelona tomorrow").
	ReplacementExample string

	// CopyKey is the catalog identifier (spec 066 ID) renderers use
	// as a lookup key into their per-transport copy tables. It is
	// NEVER rendered verbatim.
	CopyKey string

	// WindowID identifies the deprecation window the dedup ledger
	// keyed on. Renderers MAY include it in instrumentation so the
	// monitoring dashboards can correlate transport-side telemetry
	// with the server-side ledger row.
	WindowID string
}
