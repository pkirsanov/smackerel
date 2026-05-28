package assistant_adapter

// identity.go is intentionally minimal: the Identity() method is
// implemented inline on Adapter in adapter.go because the resolution
// logic is a single delegation to the injected UserResolver.
//
// This file exists to document the contract: identity is resolved at
// the transport boundary (spec 044) BEFORE any capability-layer code
// runs. The capability facade never sees chat_id; it only ever sees
// the canonical Smackerel user_id.
//
// The capability layer trusts the adapter to refuse unmapped chats
// at the boundary; the audit table records turns by (UserID,
// Transport) and an empty UserID is a contract violation that the
// facade rejects.
