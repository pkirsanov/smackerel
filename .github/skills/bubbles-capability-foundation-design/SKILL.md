---
name: bubbles-capability-foundation-design
description: Enforce capability-first design when work introduces a reusable foundation, adds a second provider/component/variant, references adapter/provider/strategy/plugin/channel/driver/connector/variant patterns, or shares UI/data/contract surfaces across two or more screens, features, or services.
---

# Bubbles Capability Foundation Design

## Goal

Design the reusable capability first, then plug concrete implementations into it. The point is not abstraction for its own sake; it is preventing provider-, screen-, or class-specific designs from becoming the hidden foundation.

## Use This Skill When

- A feature introduces a brand-new capability with no existing foundation.
- A second or later provider, adapter, connector, channel, driver, strategy, plugin, component, screen, or variant is being added.
- Two or more screens, features, or services will share the same UI, data, or contract surface.
- A design already uses words like adapter, provider, strategy, plugin, channel, driver, connector, or variant.

Do not use this skill for one-off utilities, bug fixes inside an existing foundation, single-screen UI work, or narrow refactors with no new shared surface.

## Decision Tree

1. Is the work a bug fix, one-off script, or narrow refactor inside an existing foundation?
   - Yes: keep it concrete. If capability-trigger keywords appear, add a short single-implementation justification.
   - No: continue.
2. Is this the first version of a new capability?
   - Yes: model the foundation before implementations.
   - No: continue.
3. Is this the second or later implementation/provider/screen/variant?
   - Yes: model or extend the shared foundation first.
   - No: continue.
4. Will two or more screens, services, or features share a contract or UI/data surface?
   - Yes: define reusable primitives and composition rules.
   - No: avoid premature abstraction.

## Section Templates

### `spec.md` Domain Capability Model

```markdown
## Domain Capability Model

### Capability
[Name the capability in domain language.]

### Domain Primitives
| Primitive | Purpose | Lifecycle |
|-----------|---------|-----------|
| [Entity/value/event] | [why it exists] | [states/transitions] |

### Relationships
- [Primitive A] relates to [Primitive B] by [business rule].

### Business Policies
- [Policy that every concrete implementation must obey.]
```

### `design.md` Capability Foundation

```markdown
## Capability Foundation

### Foundation Contract
| Contract | Responsibility | Consumers |
|----------|----------------|-----------|
| [interface/service/type] | [what it guarantees] | [who depends on it] |

### Extension Points
- [Adapter/provider/strategy hook]: [what implementers must provide]

### Foundation-Owned Behavior
- [Behavior shared by all implementations]
```

### `design.md` Concrete Implementations

```markdown
## Concrete Implementations

### [Implementation 1]
- Foundation contract used: [name]
- Implementation-specific behavior: [details]

### [Implementation 2]
- Foundation contract used: [name]
- Implementation-specific behavior: [details]

### Variation Axes
| Axis | Options | Owned By Foundation? |
|------|---------|----------------------|
| Provider protocol | HTTP publish, SMTP, webhook | No |
| Delivery policy | immediate, digest, retryable | Yes |
```

### `spec.md` UI Primitives

```markdown
## UI Wireframes

### UI Primitives
| Primitive | Used By Screens | Composition Rule |
|-----------|-----------------|------------------|
| [component/control/state] | [screen list] | [reuse rule] |
```

### `scopes.md` Foundation Ordering

```markdown
## Scope 1: Notification Capability Foundation
**Tags:** foundation:true
**Depends On:** none

## Scope 2: ntfy Notification Adapter
**Depends On:** Scope 1 - Notification Capability Foundation

## Scope 3: Email Notification Adapter
**Depends On:** Scope 1 - Notification Capability Foundation
```

## Right And Wrong

Wrong:

```markdown
Add ntfy notifications directly to the booking workflow.
```

Right:

```markdown
Create a notification capability with delivery intent, provider adapter contract, retry policy, and audit lifecycle. Implement ntfy as the first adapter.
```

Wrong:

```markdown
Create a plugin framework for one one-off export script.
```

Right:

```markdown
### Single-Implementation Justification
This export is a one-time operator utility with no second provider, screen, service, or shared contract. A foundation would add maintenance cost without reducing real complexity.
```

## Proven Precedent

`bubbles-deployment-target-adapter` applies this same shape to deployment: the project owns a target-agnostic contract, and per-target adapters own concrete host knowledge. Capability-first design generalizes that proven foundation-plus-overlay pattern to product, UI, service, and integration capabilities.

## Validation Links

- AN5: domain capability model when proportionality applies.
- DE4: foundation/overlay split with at least two variation axes.
- UX9: UI primitives for multi-screen or cross-feature UI reuse.
- P4: foundation scopes precede overlay scopes.
- G094: mechanical capability foundation gate for new specs.
