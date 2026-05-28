package contracts

import "context"

// Assistant is the facade interface implemented by
// internal/assistant.Facade and consumed by every TransportAdapter.
// The single method shape (Handle) keeps the capability/adapter
// boundary minimal and easy to substitute under test (see
// internal/assistant/facade_test.go fakeTransportAdapter).
//
// Source of truth: design.md §2.4.
type Assistant interface {
	// Handle drives one inbound turn: builds an agent.IntentEnvelope
	// from msg, routes via spec 037 agent.Router, applies the
	// borderline post-processor, dispatches to the executor (high
	// band only), enforces provenance, writes the audit row, and
	// returns the canonical AssistantResponse. Implementations MUST
	// be safe for concurrent calls per (UserID, Transport).
	Handle(ctx context.Context, msg AssistantMessage) (AssistantResponse, error)
}
