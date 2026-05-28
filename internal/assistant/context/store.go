// Package context owns the spec 061 SCOPE-04 capability-layer
// conversation state surface per design §6.
//
// Responsibilities:
//   - Persist the per-(user_id, transport) short-window working context
//     (most recent ContextTurn entries + pending confirm/disambig
//     references) in PostgreSQL via the assistant_conversations table
//     (migration 041).
//   - Drop conversations whose last_activity_at falls outside the
//     configured idle TTL (ticker.go runs SweepIdle on the period from
//     assistant.context.idle_sweep_interval).
//   - Resolve user reference expressions ("that one", "open 2") against
//     the most recent ContextTurn's source list (reference_resolver.go).
//
// The package is consumed by internal/assistant/facade.go ONLY. No
// transport package imports this package directly (the architecture
// test in internal/assistant/contracts enforces that).
//
// Package name is "assistantctx" (NOT "context") to avoid shadowing
// the stdlib "context" package inside this file's own imports.
package assistantctx

import (
	"context"
	"time"
)

// Conversation is the in-memory projection of one row in the
// assistant_conversations table. It is created lazily on the first
// Persist for a given (UserID, Transport).
//
// WorkingContext stores up to assistant.context.window_turns most-
// recent ContextTurn entries (cap enforced by the facade, NOT by the
// store). The store is opaque to the cap — it round-trips whatever
// the facade hands it.
type Conversation struct {
	UserID          string
	Transport       string
	WorkingContext  WorkingContext
	PendingConfirm  *PendingConfirm
	PendingDisambig *PendingDisambig
	LastActivityAt  time.Time
	SchemaVersion   int
}

// WorkingContext is the bounded, FIFO-ordered turn history the
// capability uses for short-window reference resolution. Order is
// significant: index 0 is the oldest, len-1 is the newest. The
// reference resolver looks at the newest entry only.
type WorkingContext struct {
	Turns []ContextTurn `json:"turns"`
}

// ContextTurn is one historical turn in the conversation window.
// Source IDs are the artifact / external-provider identifiers attached
// to the prior AssistantResponse — reference resolution against
// "that one" / numeric ("open 2") consults this list.
type ContextTurn struct {
	UserText  string    `json:"user_text"`
	Body      string    `json:"body"`
	SourceIDs []string  `json:"source_ids"`
	EmittedAt time.Time `json:"emitted_at"`
}

// PendingConfirm is the persisted shadow of an outstanding
// ConfirmCard. The capability writes one row on propose, deletes /
// rewrites on confirm-or-timeout. ConfirmRef is the ULID handed to
// the adapter as callback_data.
type PendingConfirm struct {
	ConfirmRef     string    `json:"confirm_ref"`
	ScenarioID     string    `json:"scenario_id"`
	ProposedAction string    `json:"proposed_action"`
	Payload        []byte    `json:"payload"`
	ExpiresAt      time.Time `json:"expires_at"`
}

// PendingDisambig is the persisted shadow of an outstanding
// DisambiguationPrompt. The capability writes one row when the
// borderline-band post-processor fires, deletes on user-selection or
// idle-sweep.
type PendingDisambig struct {
	DisambiguationRef string             `json:"disambiguation_ref"`
	Choices           []DisambigChoiceID `json:"choices"`
	ExpiresAt         time.Time          `json:"expires_at"`
}

// DisambigChoiceID is the persisted shape of a single DisambiguationChoice.
// Only the scenario id (or the SaveAsNoteChoiceID sentinel) + the 1-indexed
// number is preserved across the persist boundary; labels are re-rendered
// from the skills manifest on the second turn.
type DisambigChoiceID struct {
	Number int    `json:"number"`
	ID     string `json:"id"`
}

// Store is the persistence interface the facade depends on. The
// PostgreSQL implementation lives in pg_store.go; tests substitute an
// in-memory implementation via the InMemoryStore in testing.go.
//
// Concurrency: implementations MUST be safe for concurrent calls
// across all four methods. The (UserID, Transport) primary key
// guarantees per-user single-row semantics on the database side.
type Store interface {
	// Load returns the existing Conversation row for (userID, transport)
	// or a freshly-zeroed Conversation with that primary key set when
	// no row exists. The bool indicates whether a row was found.
	Load(ctx context.Context, userID, transport string) (Conversation, bool, error)

	// Persist upserts the supplied conversation. Implementations MUST
	// honor LastActivityAt (do not silently overwrite with NOW()) —
	// the facade is the source of truth for the timestamp.
	Persist(ctx context.Context, conv Conversation) error

	// DeleteByKey removes the conversation row for (userID, transport).
	// Used by the /reset slash command and by the idle-sweep ticker on
	// per-row cleanup. Idempotent (no error when the row does not exist).
	DeleteByKey(ctx context.Context, userID, transport string) error

	// SweepIdle deletes every conversation row whose LastActivityAt is
	// strictly before NOW() - idleTTL. Returns the number of rows
	// removed so the ticker can record a per-sweep metric.
	SweepIdle(ctx context.Context, idleTTL time.Duration) (int64, error)

	// CountActiveByTransport returns the per-transport row count of
	// the assistant_conversations table. A transport with zero rows
	// MAY be omitted from the returned map; callers MUST treat
	// "missing key" as count=0. Spec 061 SCOPE-09 — the
	// ActiveThreadsRefresher samples this method periodically to
	// refresh the smackerel_assistant_active_threads gauge.
	CountActiveByTransport(ctx context.Context) (map[string]int, error)
}
