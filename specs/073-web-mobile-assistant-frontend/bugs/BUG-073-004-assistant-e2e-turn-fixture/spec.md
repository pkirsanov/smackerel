# Spec: BUG-073-004 - Contract-correct transport-hint parity E2E

## Problem Statement

Spec 073's live transport-hint parity test uses `/reset` to stand in for an
ordinary assistant turn. Reset is a stateful early capability short circuit,
while `transport_hint` is telemetry-only and the HTTP response transport is
canonically `web`.

## Requirements

- **R1 - Preserve production semantics.** Do not change reset tracing, HTTP
  transport naming, or transport-hint behavior merely to satisfy stale tests.
- **R2 - Real normal-turn fixture.** The parity test must exercise a
  non-command text turn through the live disposable stack and real facade.
- **R3 - Contract-relevant parity.** A `mobile` hint is accepted but does not
  alter route selection, response shape, permissions, tools, or the canonical
  response transport. Per-request IDs/timestamps/traces remain excluded from
  exact parity comparison.
- **R4 - Exact identity isolation.** Snapshot and restore only the current
  test identity's `(user_id, transport)` conversation row. Cleanup must run on
  test failure and must never globally mutate `assistant_conversations`.
- **R5 - Live fidelity.** E2E tests use the real production route, facade,
  PostgreSQL test store, and disposable stack with no internal mocks.

## Acceptance Scenarios

```gherkin
Feature: Contract-correct assistant parity E2E

  Scenario: Web and mobile hints preserve visible response parity
    Given the shared HTTP identity state is isolated with its prior row saved
    When equivalent ordinary text turns are sent with web and mobile hints
    Then both hints are accepted and contract-relevant response fields match
    And both responses identify the canonical HTTP transport as web

  Scenario: Shared identity state is restored exactly
    Given the shared HTTP identity may already have conversation state
    When the parity test completes or fails
    Then only that identity's web conversation row is restored byte-for-byte
    And no unrelated conversation row is changed
```

## Out Of Scope

- Changing `transport_hint` from telemetry metadata into routing input.
- Returning `transport="mobile"` from the HTTP adapter.
- Adding trace IDs to `/reset` or changing reset acknowledgement semantics.
- HTTP response deduplication, owned by BUG-069-004.

### Single-Capability Justification

This packet is a test-fixture correction and introduces NO new product
capability, provider, adapter, strategy, or variant (Gate G094). The
`transport_hint` and HTTP-adapter references above describe the EXISTING
production `internal/assistant/httpadapter` behavior — canonical response
transport `web`, telemetry-only hint — which this bugfix leaves unchanged. The
only change under test is a single capability: ordinary-turn transport-hint
parity (the E2E now sends a real text turn instead of the `/reset` short
circuit) plus an exact-key conversation-isolation test helper. No foundation,
no alternative implementations, and no shared capability surface are added, so a
Domain Capability Model does not apply.
