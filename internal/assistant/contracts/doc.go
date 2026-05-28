// Package contracts holds the canonical, transport-agnostic types
// exchanged between the spec 061 conversational-assistant capability
// layer and every TransportAdapter (Telegram in v1; WhatsApp / web /
// mobile in later phases).
//
// These are the ONLY net-new top-level type surface introduced by
// spec 061. All runtime mechanics (intent routing, scenario loading,
// tool execution, tracing) reuse the existing spec 037 substrate in
// internal/agent unchanged.
//
// Authoritative source: specs/061-conversational-assistant/design.md §2
// (canonical contracts) + §10 (module layout) + §11.3 (build-time
// architecture tests).
//
// Architecture invariants enforced by architecture_test.go in this
// package:
//
//  1. Forbidden-package existence: none of
//     internal/assistant/{router,registry,executor,tracer,loader,llm,nats}/
//     directories may exist. Their existence would re-introduce a
//     parallel agent substrate that spec 037 already owns and that
//     spec 061 spec.md §3.1.4 + design.md §10 explicitly forbid.
//
//  2. Import direction: nothing under internal/assistant/... may
//     import any internal/<transport>/... package (telegram, whatsapp,
//     webchat, mobile). Capability MUST NOT depend on transports.
package contracts
