# Recipe: Data-Driven Simplification (Retro → Simplify)

> *"The liquor tells me which trailers need the most work, Randy. Then Donny tapes them up."*

---

## The Situation

You want to simplify code, but you don't know where to start. Instead of guessing, let the retro agent analyze git history to find the real problem areas — bug magnets, files with high churn, hidden coupling — then simplify those files first.

## The Command

```
/bubbles.workflow  <feature> mode: retro-to-simplify
```

Safer for fragile shared fixtures or bootstrap infrastructure:

```
/bubbles.workflow  <feature> mode: retro-to-simplify gitIsolation: true autoCommit: scope maxScopeMinutes: 60 maxDodMinutes: 30
```

Or in plain English:

```
/bubbles.workflow  find the worst code hotspots and simplify them
```

## What Each Phase Does

| Phase | Agent | What Happens |
|-------|-------|-------------|
| **retro** | Jim Lahey (Bottle) | Analyzes git history for bug magnets, co-change coupling, bus factor, churn trends. Produces a hotspot report with recommended targets. |
| **simplify** | Donny | Simplifies the identified hotspot files — removes duplication, flattens over-engineering, reduces complexity. |
| **test** | Trinity | Proves behavior still works after simplification. |
| **regression** | Steve French | Ensures simplification didn't break anything else. |
| **validate** | Randy | Gates and evidence checks. |
| **audit** | Ted Johnson | Final compliance check. |
| **docs** | J-Roc | Updates documentation to reflect simplified code. |

## Shared Infrastructure Safety

If retro points at shared fixtures, harnesses, global setup, or auth/session/bootstrap code, do not jump straight into broad cleanup. The workflow must first plan a Shared Infrastructure Impact Sweep, define an independent canary suite for downstream contracts, declare a Change Boundary, and document rollback or restore before the simplification phase can close.

## When To Use This Over `simplify-to-doc`

| Situation | Mode |
|-----------|------|
| You already know which files to simplify | `simplify-to-doc` |
| You want data to guide what to simplify | **`retro-to-simplify`** |
| The codebase is large and you need to prioritize | **`retro-to-simplify`** |
| You just finished a feature and want to clean up recent files | `simplify-to-doc` |

## Related Recipes

- [Code Health Analysis](code-health-analysis.md) — standalone hotspot analysis without simplification
- [Simplify Existing Code](simplify-existing-code.md) — simplify without retro analysis
- [Data-Driven Hardening](retro-driven-harden.md) — retro → harden instead of simplify
