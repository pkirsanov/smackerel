# Recipe: Retro Quality Sweep (Retro → Simplify → Harden → Verify)

> *"The liquor finds the mess, boys. Then the whole crew sweeps it clean."*

---

## The Situation

You want something more targeted than a global quality sweep and more thorough than `retro-to-simplify` or `retro-to-harden` alone.

Start with retro, let it identify the hotspot files that actually keep causing pain, then run a deterministic cleanup-and-hardening chain on those areas.

This is the best fit when you want **data-driven targeting** plus a **predictable quality workflow**.

## The Command

```
/bubbles.workflow  <feature> mode: retro-quality-sweep
```

Safer for fragile shared fixtures or bootstrap infrastructure:

```
/bubbles.workflow  <feature> mode: retro-quality-sweep gitIsolation: true autoCommit: scope maxScopeMinutes: 60 maxDodMinutes: 30
```

Or in plain English:

```
/bubbles.workflow  use retro to find the hotspots, then simplify and harden them
```

## What Each Phase Does

| Phase | Agent | What Happens |
|-------|-------|-------------|
| **retro** | Jim Lahey (Bottle) | Analyzes bug magnets, coupling, bus factor, and churn to identify the noisiest hotspot files. |
| **simplify** | Donny | Removes duplication, flattens complexity, and strips unnecessary scaffolding from those hotspots first. |
| **harden** | Conky | Pressure-tests the cleaned-up areas for weak error handling, fragile logic, and missing edge-case coverage. |
| **gaps** | Phil Collins | Finds missing behaviors, coverage holes, and plan-to-implementation drift in the same hotspot areas. |
| **implement** | Julian | Applies the necessary fixes and additions surfaced by harden + gaps. |
| **test + regression** | Trinity + Steve French | Proves the hotspot fixes work and did not break neighboring specs. |
| **stabilize + devops + security** | Bill + Tommy + Cyrus | Checks reliability, operational concerns, and security on the changed surface. |
| **validate + audit + docs** | Randy + Ted + J-Roc | Runs gates, compliance, and documentation sync before closeout. |

## Shared Infrastructure Safety

If the hotspot set includes shared fixtures, harnesses, global setup, or auth/session/bootstrap infrastructure, the workflow now expects planning artifacts to capture blast radius, canary coverage, rollback/restore, and explicit change boundaries before the cleanup chain can legitimately close. Use the safer invocation above for those refactors so validated milestone commits and isolated git state are available during the sweep.

## When To Use This Over Other Modes

| Situation | Mode |
|-----------|------|
| You only want to simplify the hotspots | `retro-to-simplify` |
| You only want to harden the hotspots | `retro-to-harden` |
| You want a read-only diagnosis first | `retro-to-review` |
| You want random adversarial probing across many specs | `stochastic-quality-sweep` |
| You want retro to pick the hotspots, then run a full deterministic cleanup/hardening chain | **`retro-quality-sweep`** |

## Related Recipes

- [Data-Driven Simplification](retro-driven-simplify.md) — retro → simplify only
- [Data-Driven Hardening](retro-driven-harden.md) — retro → harden only
- [Quality Sweep](quality-sweep.md) — full deterministic sweep without retro targeting
- [Code Health Analysis](code-health-analysis.md) — retro only, no remediation