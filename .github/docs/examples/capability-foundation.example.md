# Annotated Example: Capability-First Notification Design

> This is a reference example, not a template. It shows how capability-first design appears across `spec.md`, `design.md`, and `scopes.md` for a notification capability with multiple concrete providers.

---

# ARTIFACT 1: spec.md

# Notification Capability

## Summary

Operators need one way to declare notification intent while the system can deliver that intent through multiple providers. The first concrete implementations are ntfy and email, but business policy must stay provider-neutral.

## Outcome Contract

**Intent:** Users and operators can declare notification-worthy events once and rely on policy-driven delivery through configured providers.
**Success Signal:** A notification intent created for an event is delivered through the configured provider and recorded with provider-independent status.
**Hard Constraints:** Provider failures must not erase the original notification intent; provider-specific fields must not leak into domain scenarios.
**Failure Condition:** Adding a second provider requires changing the domain event model or rewriting callers.

## Domain Capability Model

### Capability

Notification delivery turns domain events into auditable delivery attempts without requiring callers to know provider details.

### Domain Primitives

| Primitive | Purpose | Lifecycle |
|-----------|---------|-----------|
| NotificationIntent | Provider-neutral request to notify an audience about an event | requested -> queued -> delivered / failed |
| NotificationAudience | Actor, group, or endpoint class that should receive the message | active / disabled |
| NotificationPolicy | Business rule for timing, retry, and channel selection | draft -> active -> retired |
| DeliveryAttempt | One concrete provider attempt for one intent | pending -> sent / failed / retried |

### Relationships

- A `NotificationIntent` has one audience and one active policy.
- A `NotificationPolicy` selects one or more provider adapters.
- A `DeliveryAttempt` belongs to exactly one intent and one provider adapter.

### Business Policies

- Callers create intents, not provider requests.
- Provider adapters may fail independently; the intent remains auditable.
- Retry limits and quiet hours are policy decisions, not adapter decisions.

## Use Cases

### UC-001: Send provider-neutral event notification
- **Actor:** System operator
- **Preconditions:** A notification policy is active and at least one provider adapter is configured.
- **Main Flow:** The system creates an intent, resolves the policy, dispatches through the selected adapter, and records the attempt.
- **Postconditions:** The operator can inspect provider-independent status.

## Business Scenarios

### BS-001: Notification intent survives provider failure
Given a configured notification policy with ntfy as the selected provider
When the provider rejects the delivery attempt
Then the notification intent remains visible with failed delivery status
And retry policy determines whether another attempt is queued

## UI Wireframes

### UI Primitives

| Primitive | Used By Screens | Composition Rule |
|-----------|-----------------|------------------|
| Provider status badge | Provider list, notification detail | Always shows provider name plus last health state |
| Delivery timeline | Notification detail, audit drawer | Uses the same event order and status vocabulary everywhere |
| Policy channel selector | Policy editor, setup wizard | Renders configured provider adapters from the foundation registry |

### Screen: Notification Policy Editor

```text
[Policy name]
[Audience selector]
[Channel selector: ntfy, email]
[Retry policy]
[Quiet hours]
[Save]
```

### Screen: Notification Detail

```text
[Intent summary]
[Provider status badge]
[Delivery timeline]
[Retry action]
```

### Screen: Provider Setup

```text
[Provider type]
[Provider settings]
[Test provider]
[Enable]
```

---

# ARTIFACT 2: design.md

# Notification Capability Design

## Design Brief

### Current State

Notification behavior is not yet represented as a reusable capability. Provider-specific delivery would otherwise leak into callers.

### Target State

Introduce a provider-neutral notification foundation that accepts intents and dispatches through adapter implementations. ntfy and email are concrete adapters layered on the foundation.

### Resolved Decisions

- Callers create `NotificationIntent` records.
- Provider adapters implement a shared dispatch contract.
- Delivery status is recorded at the attempt level.

## Capability Foundation

### Foundation Contract

| Contract | Responsibility | Consumers |
|----------|----------------|-----------|
| NotificationDispatcher | Accept provider-neutral intents and coordinate policy resolution | Domain services and workflow jobs |
| NotificationProviderAdapter | Send one provider-specific attempt and return provider result | Dispatcher |
| NotificationPolicyResolver | Select channels and retry behavior for an intent | Dispatcher |
| DeliveryAuditStore | Persist intent and attempt lifecycle events | Dispatcher and UI |

### Extension Points

- Provider adapter: validates provider configuration, sends a provider-specific attempt, normalizes success/failure result.
- Policy resolver: maps intent type and audience to channels, retry limits, and quiet-hour behavior.
- Status renderer: maps provider-independent lifecycle states to UI badges and timelines.

### Foundation-Owned Behavior

- Intent creation and validation.
- Policy resolution.
- Attempt lifecycle recording.
- Retry eligibility evaluation.
- Provider-independent status vocabulary.

## Concrete Implementations

### ntfy Adapter

- Foundation contract used: `NotificationProviderAdapter`.
- Sends messages through ntfy-compatible publish semantics.
- Owns ntfy topic and priority mapping.
- Does not own retry policy or audience selection.

### Email Adapter

- Foundation contract used: `NotificationProviderAdapter`.
- Sends messages through email delivery infrastructure.
- Owns subject/body rendering and address validation.
- Does not own retry policy or audience selection.

### Variation Axes

| Axis | Options | Foundation Responsibility |
|------|---------|---------------------------|
| Provider protocol | ntfy publish, email send, webhook callback | Adapter contract only |
| Delivery timing | immediate, digest, quiet-hours delayed | Foundation policy |
| Retry semantics | no retry, bounded retry, dead-letter | Foundation policy |
| Audience address shape | topic, email address, webhook URL | Adapter validation |

## Testing And Validation Strategy

| Scenario | Test Type | Assertion |
|----------|-----------|-----------|
| BS-001 | integration | Provider failure records failed attempt without deleting intent |
| ntfy adapter | e2e-api | Intent dispatch creates ntfy delivery attempt |
| email adapter | e2e-api | Intent dispatch creates email delivery attempt |

---

# ARTIFACT 3: scopes.md

# Notification Capability Scopes

## Execution Outline

### Phase Order

1. Notification foundation: create provider-neutral domain model, dispatcher contract, policy resolution, and audit lifecycle.
2. ntfy adapter: implement first provider against the foundation contract.
3. Email adapter: implement second provider against the same foundation contract.
4. UI composition: wire shared provider badges, delivery timeline, and policy channel selector into all notification screens.

### New Types And Signatures

- `NotificationIntent`
- `NotificationProviderAdapter`
- `NotificationDispatcher`
- `DeliveryAttempt`

### Validation Checkpoints

- Foundation tests run before any provider adapter.
- Each adapter has provider-specific tests plus shared adapter-contract tests.
- UI tests verify primitives render consistently across all screens.

## Scope 1: Notification Capability Foundation

**Status:** Not Started
**Tags:** foundation:true
**Depends On:** none

### Gherkin Scenarios

Scenario: Provider-neutral intent is persisted
Given an active notification policy
When a domain service creates a notification intent
Then the intent is stored with provider-independent status
And no provider-specific field is required from the caller

### Implementation Plan

- Define intent, policy, provider adapter, dispatcher, and delivery attempt contracts.
- Persist provider-independent lifecycle events.
- Add shared adapter-contract tests.

### Test Plan

| Test Type | Category | File/Location | Description | Live System |
|-----------|----------|---------------|-------------|-------------|
| Unit | `unit` | `notification_foundation_test` | Intent validation and policy resolution | No |
| Integration | `integration` | `notification_dispatcher_integration_test` | Intent creates delivery attempt through a fake external provider boundary | Yes |

### Definition of Done

- [ ] Notification foundation contracts exist and are used by dispatcher tests.
- [ ] Provider-independent lifecycle tests pass.

## Scope 2: ntfy Provider Adapter

**Status:** Not Started
**Depends On:** Scope 1 - Notification Capability Foundation

### Gherkin Scenarios

Scenario: ntfy adapter sends a delivery attempt
Given an active notification policy that selects ntfy
When the dispatcher processes a notification intent
Then the ntfy adapter creates one delivery attempt
And the attempt result is recorded with provider-independent status

### Implementation Plan

- Implement ntfy adapter behind `NotificationProviderAdapter`.
- Map ntfy-specific result into shared delivery result.

### Test Plan

| Test Type | Category | File/Location | Description | Live System |
|-----------|----------|---------------|-------------|-------------|
| Unit | `unit` | `ntfy_adapter_test` | ntfy payload mapping | No |
| E2E API | `e2e-api` | `notification_ntfy_e2e_test` | Dispatch intent through ntfy provider path | Yes |

### Definition of Done

- [ ] ntfy adapter uses the foundation provider contract.
- [ ] ntfy delivery result records provider-independent status.

## Scope 3: Email Provider Adapter

**Status:** Not Started
**Depends On:** Scope 1 - Notification Capability Foundation

### Gherkin Scenarios

Scenario: email adapter sends a delivery attempt
Given an active notification policy that selects email
When the dispatcher processes a notification intent
Then the email adapter creates one delivery attempt
And the attempt result is recorded with provider-independent status

### Implementation Plan

- Implement email adapter behind `NotificationProviderAdapter`.
- Reuse foundation retry and audit behavior.

### Test Plan

| Test Type | Category | File/Location | Description | Live System |
|-----------|----------|---------------|-------------|-------------|
| Unit | `unit` | `email_adapter_test` | email payload mapping | No |
| E2E API | `e2e-api` | `notification_email_e2e_test` | Dispatch intent through email provider path | Yes |

### Definition of Done

- [ ] email adapter uses the foundation provider contract.
- [ ] email delivery result records provider-independent status.
