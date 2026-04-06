# Recipe: Choose The Right Review

> *"Different job, different trailer, boys."*

---

## The Situation

You know you need a review, but you are not sure whether the job is engineering-only or a broader product/system assessment.

## Quick Rule

Use `bubbles.code-review` for code.

Use `bubbles.system-review` for features, components, journeys, UX, or the whole system.

## Use `bubbles.code-review` When

- you want engineering findings only
- you are reviewing repositories, services, packages, modules, paths, or symbols
- you care about correctness, complexity, reliability, test quality, or security in code
- you do not want product, UX, accessibility, or journey critique

Example:

```
/bubbles.code-review  profile: engineering-sweep scope: service:gateway output: summary-doc
```

## Use `bubbles.system-review` When

- you want to assess a feature or component as a user-facing system
- you care about usability, accessibility, flow clarity, duplication across flows, or feature coherence
- you want runtime execution and real-user behavior included
- you want to turn cross-cutting findings into specs later

Example:

```
/bubbles.system-review  mode: full scope: feature:booking output: summary-doc
```

## Use A Workflow When

Use `bubbles.workflow` only after you already know you want follow-through work such as planning, implementation, testing, validation, or audit.

Examples:

```
/bubbles.workflow  improve-existing for booking
/bubbles.workflow  product-to-delivery for notification-system
/bubbles.workflow  harden-gaps-to-doc for 042-catalog-assistant
```

## The Practical Split

- `bubbles.code-review` = engineering diagnosis
- `bubbles.system-review` = holistic diagnosis
- `bubbles.workflow` = execution after diagnosis