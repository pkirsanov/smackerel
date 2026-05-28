package contracts

import "context"

// TransportAdapter is the canonical capability ⇄ transport boundary.
// One implementation per inbound channel (v1: Telegram only). All
// adapters are expected to be thin shells around the capability
// facade — see spec.md §6.2 for the MUST / MUST NOT contract.
//
// Source of truth: design.md §2.3.
type TransportAdapter interface {
	// Name returns the closed-vocabulary transport name
	// (must match AssistantMessage.Transport).
	Name() string

	// Translate converts a transport-specific inbound payload into
	// the canonical AssistantMessage. Implementations MUST strip
	// transport markup and resolve the user identity via Identity()
	// or an equivalent transport-side step.
	Translate(ctx context.Context, payload TransportPayload) (AssistantMessage, error)

	// Render delivers an AssistantResponse to the supplied
	// TransportIdentity. Implementations MUST honor CaptureRoute
	// (invoking the local capture path) and MUST NOT introduce any
	// rendering branch keyed on closed-vocabulary tokens the
	// capability layer does not expose.
	Render(ctx context.Context, identity TransportIdentity, resp AssistantResponse) error

	// Identity maps a transport payload to the canonical
	// TransportIdentity (transport name + smackerel user id).
	Identity(ctx context.Context, payload TransportPayload) (TransportIdentity, error)

	// Start begins serving the transport. It is given the
	// capability facade so adapter-internal background loops can
	// call Handle directly without re-resolving the dependency.
	Start(ctx context.Context, a Assistant) error

	// Stop drains in-flight requests and shuts the adapter down.
	Stop(ctx context.Context) error
}

// TransportPayload is the opaque transport-specific inbound type
// (e.g. *tgbotapi.Update for Telegram). The capability layer never
// inspects it; only the adapter knows the concrete shape.
type TransportPayload interface{}

// TransportIdentity is the per-turn resolution of a transport
// payload's owner. (UserID, Transport) is the conversation primary
// key (see design.md §6.1 — assistant_conversations table).
type TransportIdentity struct {
	UserID    string
	Transport string
}
