# Recipe: Data-Driven Hardening (Retro → Harden)

> *"The liquor shows me the weak spots, Randy. Then we harden them up before the shit winds come."*

---

## The Situation

You want to harden existing code — close gaps, fix fragile spots, improve error handling — but you want to focus on the areas that actually cause problems. The retro agent analyzes git history to find bug magnets and worsening hotspots, then the hardening pipeline targets those areas.

## The Command

```
/bubbles.workflow  <feature> mode: retro-to-harden
```

Or in plain English:

```
/bubbles.workflow  find the weakest code areas and harden them
```

## What Each Phase Does

| Phase | Agent | What Happens |
|-------|-------|-------------|
| **retro** | Jim Lahey (Bottle) | Analyzes git history to find bug magnets, worsening hotspots, and fragile areas. |
| **harden** | Conky | Deep hardening pass on identified hotspot files — error handling, edge cases, compliance. |
| **gaps** | Phil Collins | Finds missing test coverage, undocumented behavior, spec holes in the hotspot areas. |
| **implement** | Julian | Implements fixes for issues found by harden and gaps. |
| **test** | Trinity | Verifies all fixes work and coverage is complete. |
| **regression** | Steve French | Cross-spec regression scan. |
| **simplify** | Donny | Cleans up any complexity introduced by hardening. |
| **stabilize** | Shitty Bill | Infrastructure and reliability checks. |
| **security** | Cyrus | Security scan on changed areas. |
| **validate + audit** | Randy + Ted | Gates, evidence, compliance. |
| **docs** | J-Roc | Documentation sync. |

## When To Use This Over `harden-gaps-to-doc`

| Situation | Mode |
|-----------|------|
| You already know which areas need hardening | `harden-gaps-to-doc` |
| You want data to guide where to harden | **`retro-to-harden`** |
| You're doing a pre-release quality pass on a large codebase | **`retro-to-harden`** |
| You want the full deterministic sweep everywhere | `harden-gaps-to-doc` |

## Related Recipes

- [Code Health Analysis](code-health-analysis.md) — standalone hotspot analysis
- [Post-Implementation Hardening](post-impl-hardening.md) — hardening without retro targeting
- [Data-Driven Simplification](retro-driven-simplify.md) — retro → simplify instead of harden
- [Quality Sweep](quality-sweep.md) — full quality sweep with full-delivery
