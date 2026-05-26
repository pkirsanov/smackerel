# Recipe: Design A Capability

> *"Build the trailer, then park the cars in it."*

## The Situation

You are designing something that may have more than one implementation: notification providers, payment channels, connector types, plugin strategies, UI variants, or shared screens across multiple features. You want Bubbles to model the reusable capability before it plans concrete implementations.

## Quick Start

```
/bubbles.workflow  design capability <capability-name>
```

Example:

```
/bubbles.workflow  design capability notifications with ntfy and email providers
```

This should run the normal planning chain:

```
bubbles.analyst -> bubbles.ux -> bubbles.design -> bubbles.plan
```

## What Bubbles Produces

| Artifact | Capability-First Output |
|----------|-------------------------|
| `spec.md` | `## Domain Capability Model` with domain primitives, relationships, lifecycle, and business policies |
| `spec.md` UX | `### UI Primitives` when two or more screens or cross-feature UI reuse exist |
| `design.md` | `## Capability Foundation`, `## Concrete Implementations`, and `### Variation Axes` |
| `scopes.md` | a foundation scope tagged `foundation:true`, then overlay/provider scopes that depend on it |

## Proportionality Check

Capability-first design applies only when at least one of these is true:

- The work introduces a brand-new capability with no existing foundation.
- The work adds the second or later provider, adapter, component, or variant.
- The design references adapter, provider, strategy, plugin, channel, driver, connector, or variant patterns.
- Two or more screens, features, or services share the same UI, data, or contract surface.

If none apply, keep the work concrete and avoid premature abstraction.

## Notification Example

```
/bubbles.workflow  design capability notifications with ntfy as the first provider and email as the second provider
```

Expected shape:

1. Analyst defines `NotificationIntent`, `NotificationAudience`, `NotificationPolicy`, and `DeliveryAttempt` in the Domain Capability Model.
2. UX defines shared UI primitives such as provider status badge, delivery timeline, and policy channel selector.
3. Design defines a `NotificationProviderAdapter` contract, dispatcher, policy resolver, and delivery audit store.
4. Plan creates a foundation scope first, then ntfy and email adapter scopes that depend on the foundation scope.

## Manual Path

```
/bubbles.analyst  define the domain capability model for <capability>
/bubbles.ux       define UI primitives for <capability> if multiple screens reuse them
/bubbles.design   create the capability foundation and concrete implementations for <capability>
/bubbles.plan     create foundation-first scopes for <capability>
```

## Validation

Gate G094 checks new specs for capability-first sections when proportionality applies. It is grandfathered for older specs by `state.json.createdAt`, so brownfield repositories are not retroactively blocked.

## Related

- [Capability Foundation Example](../examples/capability-foundation.example.md)
- [New Feature](new-feature.md)
- [Plan Only](plan-only.md)
- [Brainstorm an Idea](brainstorm-idea.md)
- `skills/bubbles-capability-foundation-design/SKILL.md`
