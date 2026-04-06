# Recipe: Review A Feature Or System

> *"You gotta watch the whole show, boys."*

---

## The Situation

You want to review a feature, component, journey, surface, or the whole system as an integrated product before deciding what to fix, simplify, or turn into specs.

This is not a full workflow run. It is a holistic assessment pass.

## The Command

```
/bubbles.system-review  mode: full scope: feature:booking output: summary-doc
```

Or for the whole system:

```
/bubbles.system-review  scope: full-system output: summary-only
```

Or to promote findings into specs:

```
/bubbles.system-review  mode: full scope: component:dashboard output: create-specs
```

## Modes

System review modes are defined in `bubbles/system-review.yaml`.

| Mode | Use It For |
|------|------------|
| `product` | product value, UX coherence, simplification, and engineering signals |
| `runtime` | real-user execution, runtime reliability, and validation |
| `trust` | trust, security, validation, and audit concerns |
| `full` | complete cross-domain system review |

## What You Get Back

Every run uses the same output shape:

1. Review scope
2. System summary
3. Findings by lens
4. Cross-domain conflicts
5. Prioritized actions
6. Spec promotion candidates
7. Artifact outputs

## When To Use This Instead Of Code Review

Use `bubbles.system-review` when:
- you want to inspect a feature or component as a user-facing system
- you care about UX, accessibility, runtime behavior, trust, or duplication across flows
- you want code findings connected to product and operational consequences
- you want selected findings promoted into specs later

Use `bubbles.code-review` when:
- you want engineering-only review on code paths, services, packages, modules, or the full repo
- you do not want UX/product/runtime critique