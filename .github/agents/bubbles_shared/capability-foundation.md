# Capability-First Design Doctrine

Use this module when planning agents design work that may grow beyond one concrete implementation, screen, provider, or variant.

## Doctrine

When a feature introduces or extends a reusable capability, model the capability foundation first: domain primitives, contracts, lifecycle, extension points, and reusable UI primitives. Concrete providers, screens, classes, and variants are layered on top as adapters or overlays.

The deployment-target-adapter skill proves the pattern for deployment work. This doctrine generalizes it to every capability area where more than one concrete implementation is likely.

## Proportionality Clause

This doctrine applies only when at least one condition is true:

- The work introduces a brand-new capability with no existing foundation to extend.
- The work adds the Nth concrete implementation, provider, component, or variant where N >= 2.
- The design references adapter, provider, strategy, plugin, channel, driver, connector, or variant patterns.
- Two or more screens, features, or services share the same UI, data, or contract surface.

Otherwise the doctrine is out of scope. Single-implementation utilities, bug fixes, one-off scripts, and refactors inside an existing foundation are exempt. Premature abstraction is a worse violation than missing abstraction.

## Layer Ownership

| Layer | Owns | Required Section |
|-------|------|------------------|
| Analyst | Domain capability: entities, relationships, lifecycle, business policies | `spec.md` -> `## Domain Capability Model` |
| Design | Technical capability: contracts, extension points, foundation vs overlay split | `design.md` -> `## Capability Foundation`, `## Concrete Implementations`, `### Variation Axes` |
| UX | UI capability: reusable primitives and composition rules when two or more screens or cross-feature reuse exist | `spec.md` -> `### UI Primitives` under `## UI Wireframes` |
| Plan | Implementation order: foundation scopes precede overlay scopes when a split exists | `scopes.md` / `scopes/_index.md` dependency graph |

## Required Sections When Proportionality Applies

### Analyst (`spec.md`)

Include one of:

- `## Domain Capability Model`
- `### Single-Capability Justification`

A single-capability justification must name the concrete reason the work is exempt, such as an explicit one-off utility or a bug fix inside an existing foundation.

### Design (`design.md`)

Include either the full split:

- `## Capability Foundation`
- `## Concrete Implementations`
- `### Variation Axes`

`### Variation Axes` must list at least two axes, such as provider type, delivery channel, schema shape, authorization surface, storage behavior, runtime environment, or UI composition.

Or include:

- `### Single-Implementation Justification`

A single-implementation justification must explain why a foundation would be premature for this work.

### UX (`spec.md`)

When two or more screens or cross-feature UI reuse exist, include one of:

- `### UI Primitives` under `## UI Wireframes`
- `### Single-Screen Justification`

### Plan (`scopes.md` / `scopes/_index.md`)

When design splits a foundation from concrete implementations:

- One scope must be tagged `foundation:true`.
- Overlay or concrete implementation scopes must declare a `Depends On` value that names or references the foundation scope.

## Decision Tree

1. Is this a bug fix, narrow refactor, or one-off utility inside an existing foundation?
   - Yes: record a single-capability or single-implementation justification only if the artifact uses capability-trigger keywords.
   - No: continue.
2. Is this a new capability with no reusable foundation yet?
   - Yes: apply the doctrine.
   - No: continue.
3. Is this adding a second or later provider, adapter, channel, connector, strategy, plugin, variant, or screen using the same contract?
   - Yes: apply the doctrine.
   - No: continue.
4. Do two or more screens, features, services, or external consumers share the same surface?
   - Yes: apply the doctrine.
   - No: keep the design concrete and avoid premature abstraction.

## Right And Wrong Examples

### Wrong: Provider First

```markdown
## Design

Add ntfy notifications. Store ntfy URL and topic. Call ntfy from the booking service.
```

Problem: the provider is baked into the business capability, so email, webhook, or SMS delivery later require a redesign.

### Right: Capability First

```markdown
## Domain Capability Model

NotificationIntent records who needs to be notified, why, preferred channel, delivery policy, and lifecycle state.

## Capability Foundation

NotificationDispatcher accepts NotificationIntent and resolves it through provider adapters.

## Concrete Implementations

- ntfy adapter
- email adapter

### Variation Axes

- Provider protocol: HTTP publish, SMTP, webhook
- Delivery policy: immediate, digest, retryable
```

### Wrong: Premature Abstraction

```markdown
## Capability Foundation

Add a plugin system for a one-time CSV cleanup script.
```

Problem: there is no second implementation, no shared surface, and no planned reuse. This is over-engineering.

### Right: Explicit Exemption

```markdown
### Single-Implementation Justification

This is a one-off migration helper used once during data import. No second provider, screen, service, or shared contract is planned. A reusable foundation would add maintenance cost without removing real complexity.
```

## Validation IDs

- Analyst: AN5 checks the domain capability model or single-capability justification.
- Design: DE4 checks foundation/overlay split, concrete implementations, and variation axes.
- UX: UX9 checks UI primitives when two or more screens or cross-feature reuse exist.
- Plan: P4 checks foundation-to-overlay scope ordering when the split exists.

## Gate

Gate G094 (`capability_foundation_gate`) applies to specs created on or after the gate introduction date. Older specs are grandfathered until touched or recertified under the current framework epoch.
