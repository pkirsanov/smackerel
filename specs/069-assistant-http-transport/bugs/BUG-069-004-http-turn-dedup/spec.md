# Spec: BUG-069-004 - Auth-scoped HTTP turn response deduplication

## Problem Statement

The HTTP assistant adapter accepts a client idempotency key but has no response
deduplication. Identical retries execute the facade more than once.

## Requirements

- **R1 - Key scope.** Dedup keys include a one-way digest of authenticated user
  identity, canonical transport `web`, and `transport_message_id`.
- **R2 - At-most-once execution.** Sequential and concurrent requests sharing
  a key and semantic request body invoke the facade/capture path once.
- **R3 - Response replay.** Retries receive the original response's logical
  fields and assistant trace IDs. The retry's HTTP request ID remains current.
- **R4 - Cross-user privacy.** Different users using the same message ID never
  share cached responses or in-flight work. Tokens and raw user IDs are not
  retained in cache keys or response logs.
- **R5 - Bounded lifecycle.** The cache is process-local, concurrency-safe,
  bounded using the established transport-cache safety pattern, and expires
  entries under the explicit HTTP conversation TTL already loaded from SST.
- **R6 - Failure semantics.** An accepted request that returns an HTTP error is
  completed for its idempotency key so a retry cannot duplicate unknown partial
  side effects. Pre-facade validation/auth rejection is not cached.
- **R7 - Payload collision safety.** Reusing one key with a different semantic
  request body fails loud without invoking the facade again.
- **R8 - Contract stability.** No v1 request/response field changes;
  `transport_hint` remains telemetry-only.

## Acceptance Scenarios

```gherkin
Feature: Auth-scoped HTTP assistant turn deduplication

  Scenario: Sequential same-ID retry replays one logical turn
    Given an authenticated user submits a deterministic weather turn
    When the exact request is repeated with the same transport message ID
    Then the facade executes once and both responses share assistant turn ID and body

  Scenario: Concurrent same-ID retries collapse
    Given two requests with the same authenticated identity, message ID, and body
    When they arrive before the first facade execution completes
    Then one request owns execution and the other waits for and replays its result

  Scenario: Different IDs execute distinct turns
    Given otherwise equivalent requests have different message IDs
    When both are processed
    Then both execute and return different non-empty assistant turn IDs

  Scenario: Same message ID is isolated across users
    Given two authenticated users choose the same opaque message ID
    When both submit turns
    Then each executes within its own identity scope and receives only its own response

  Scenario: Same key with changed payload is rejected
    Given a completed idempotency key exists for one request body
    When the same user reuses it with a different body
    Then the adapter rejects the collision and does not execute the facade again
```

## Out Of Scope

- Cross-replica durable dedup; the current deployment owns one core ingress
  replica, matching the existing WhatsApp transport cache model.
- Changing client retry IDs or PWA retry state.
- Changing the assistant v1 wire schema.
