// Spec 061 SCOPE-04 — capability-layer audit boundary.
//
// Per design §6.3 every facade turn (including short-circuit paths)
// emits one audit record of kind="assistant_turn" so the operator
// can replay any conversation and reconstruct exactly what the
// capability did and why.
//
// The persistent column shape (assistant_proposal_payload JSONB) is
// added by SCOPE-08; for SCOPE-04 the audit interface is consumed
// through the AuditWriter abstraction below. The PostgreSQL-backed
// writer that targets the artifacts table lands in SCOPE-08 alongside
// the additive column. SCOPE-04 ships:
//
//   - The AuditWriter interface (consumed by the facade).
//   - A NoopAuditWriter default for non-PG wiring (tests, dev shells
//     that intentionally disable audit).
//
// All facade tests use NoopAuditWriter — the audit interface is
// exercised at the call boundary only. The full PG schema integration
// is SCOPE-08 territory and intentionally NOT in scope here.

package assistant

import (
	"context"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

// AuditTurn is the structured shape the facade hands to the writer
// for every turn. Field set follows design §6.3.
type AuditTurn struct {
	// UserID + Transport identify the conversation.
	UserID    string
	Transport string

	// TransportMessageID is the adapter-side opaque message id.
	TransportMessageID string

	// InboundKind is the canonical AssistantMessage.Kind discriminator.
	InboundKind contracts.MessageKind

	// InboundText is the raw inbound text (may be empty for
	// confirm / disambig / reset messages).
	InboundText string

	// Band is the borderline post-processor classification for the
	// turn ("" when no routing happened — shortcut fast-path or
	// reset / unresolvable-reference short-circuit).
	Band Band

	// RoutingDecision is the spec 037 routing decision when the
	// router ran; nil for shortcut / pre-router short-circuit paths.
	RoutingDecision *agent.RoutingDecision

	// InvocationResult is the spec 037 executor result when the
	// high-band path actually called the executor; nil otherwise.
	InvocationResult *agent.InvocationResult

	// Response is the AssistantResponse the facade emitted (may be
	// the post-provenance rewrite). Sources/ConfirmCard/Disambig
	// references are preserved as written.
	Response contracts.AssistantResponse

	// EmittedAt is the facade emit time (matches Response.EmittedAt).
	EmittedAt time.Time
}

// AuditWriter persists one AuditTurn per facade Handle call. The
// facade MUST NOT block the user response on the write — production
// implementations are expected to be non-blocking (NATS publish,
// channel buffer, etc.). NoopAuditWriter is in-process and
// synchronous; SCOPE-08 will swap in a PG/NATS-backed implementation.
type AuditWriter interface {
	Write(ctx context.Context, turn AuditTurn) error
}

// NoopAuditWriter discards every audit turn. Returned by
// NewNoopAuditWriter; safe for concurrent use.
type NoopAuditWriter struct{}

// NewNoopAuditWriter returns a writer that discards audit turns.
func NewNoopAuditWriter() *NoopAuditWriter { return &NoopAuditWriter{} }

// Write implements AuditWriter.
func (NoopAuditWriter) Write(_ context.Context, _ AuditTurn) error { return nil }
