# Recipe: Data-Driven Code Review (Retro → Review)

> *"The liquor shows me where to look. The Green Bastard tells me what's broken."*

---

## The Situation

You want a code review, but you have a large codebase and don't know where to focus. Instead of reviewing everything, let the retro agent identify the highest-risk files (bug magnets, tightly coupled modules, knowledge silos), then run a targeted code review on just those areas.

This is a **read-only diagnostic** — it produces findings and recommendations without changing code.

## The Command

```
/bubbles.workflow  <feature> mode: retro-to-review
```

Or in plain English:

```
/bubbles.workflow  find the riskiest code areas and review them
```

## What Each Phase Does

| Phase | Agent | What Happens |
|-------|-------|-------------|
| **retro** | Jim Lahey (Bottle) | Analyzes bug density, co-change coupling, bus factor, and churn trends. Identifies the highest-risk files. |
| **code-review** | Green Bastard | Engineering-only review of the identified hotspot files — quality, correctness, maintainability, security. |
| **docs** | J-Roc | Documents findings for future reference. |

## What You Get

- A retro report with hotspot data (bug magnets, coupling, bus factor)
- A code review focused on the highest-risk files identified by retro
- Prioritized action items from the code review
- No code changes — this is diagnosis only

## Acting On Findings

After the review, pick the right follow-through:

| Finding | Next Step |
|---------|-----------|
| Bug magnets need simplification | `/bubbles.workflow  <feature> mode: retro-to-simplify` |
| Fragile areas need hardening | `/bubbles.workflow  <feature> mode: retro-to-harden` |
| The whole hotspot area needs cleanup + hardening + verification | `/bubbles.workflow  <feature> mode: retro-quality-sweep` |
| Architecture issues need redesign | `/bubbles.workflow  <feature> mode: redesign-existing` |
| Specific files need focused improvement | `/bubbles.workflow  <feature> mode: improve-existing` |

## When To Use This Over Direct Code Review

| Situation | Approach |
|-----------|----------|
| You already know which files to review | `/bubbles.code-review scope: path:...` |
| You want data to guide the review scope | **`retro-to-review`** |
| You want a full system/product review | `/bubbles.system-review` |
| You want to review then immediately fix | `retro-to-simplify` or `retro-to-harden` |

## Related Recipes

- [Code Review Directly](review-code-directly.md) — targeted review without retro analysis
- [Review First, Then Improve](review-then-improve.md) — review → choose improvement workflow
- [Code Health Analysis](code-health-analysis.md) — retro hotspots only, no code review
