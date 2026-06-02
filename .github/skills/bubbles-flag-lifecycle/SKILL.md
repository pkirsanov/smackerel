---
description: Feature-flag lifecycle ownership — who introduces, who maintains, who retires. Includes the "flag dies with its train + 1 cycle" rule and the cleanup-audit trigger. Use when introducing a flag, when reviewing flag inventory, when retiring a train, or when responding to flag-cleanup-audit findings.
---

# Feature Flag Lifecycle

## Ownership Chain (owned by `bubbles.train`)

```
introduce ──► live (default-ON in owning train, OFF elsewhere) ──► graduate ──► retire
   │                          │                                      │           │
spec author              implement uses                       owning train      cleanup
declares in           env var read at                         transitions       spec opened
state.json            startup, no fallback                    to maintained     by bubbles.train
                                                              or frozen
```

## The "Flag Dies + 1 Cycle" Rule

A feature flag introduced by a spec on train X **MUST be retired ≤ 1 cycle after train X graduates** (transitions from `active` → `maintained` → `frozen`).

- Train X cuts and promotes for N cycles while flag is live.
- Train X transitions to `maintained` → flag is still allowed (gives operators time to roll out everywhere).
- Train X transitions to `frozen` → flag MUST be retired before the next monthly `flag-cleanup-audit` run.
- Train X transitions to `retired` → ANY remaining live flag is a BLOCKING violation (gate refuses `release-train-retire`).

This prevents trunk from accumulating dead conditional code.

## Cleanup Trigger Sources

1. **Calendar** — Monthly `upkeep-flag-cleanup` mode runs `release-train-flag-audit.sh`. Identifies overdue flags. Packets to `bubbles.train`.
2. **Train retire** — `release-train-retire` mode refuses to transition if any spec on the train still declares `flagsIntroduced: [...]`. Forces cleanup first.
3. **Manual** — Operator runs `<repo>.sh release flag-audit`. Same script, on-demand.

## What Gets Cleaned Up (Full List)

For flag `new_payment_flow` introduced by `specs/220-new-payment-flow/`:

| Surface | Action |
|---|---|
| `config/feature-flags.<owning-train>.yaml` | Remove entry from `flags:` and `metadata:` |
| `config/feature-flags.<every-other-train>.yaml` | Remove the `false` entry |
| Service startup code | Remove env var read + `expect()`/`KeyError` paranoid check |
| Service business code | Remove `if new_payment_flow { ... }` conditional; keep the on-path |
| `state.json` of owning spec | Remove `new_payment_flow` from `flagsIntroduced` array |
| CI / deploy bundle generator | (no change — it just reads the bundle which no longer contains the flag) |
| Tests | Remove flag-specific test variants that exercised both states |

## Flag Naming Discipline (NON-NEGOTIABLE)

| Rule | Example | Why |
|---|---|---|
| No train name in flag name | ❌ `mvp_payment_flow` → ✅ `payment_flow` | Flag outlives the train |
| No version suffix unless intentionally versioned | ❌ `pricing_v1` → ✅ `pricing` | Use versioning only when both versions must coexist long-term |
| Snake case | ❌ `newPaymentFlow` → ✅ `new_payment_flow` | Cross-language uniformity in env vars |
| Verb-noun or feature-noun | ✅ `enable_fast_checkout`, `use_pricing_v2` | Communicates intent |
| Bool only | If you need an enum, use config not a flag | Flags are switches, not knobs |

## Anti-Patterns

| ❌ Wrong | ✅ Right |
|---|---|
| Letting flag survive its train's retirement | Cleanup BEFORE `release-train-retire` |
| Renaming a flag to "carry forward" to a new train | New flag, new spec, new owning train. Retire old flag. |
| Defaulting a "killer flag" ON to "force adoption" everywhere | Flags are per-train decisions; default-ON only in owning train |
| Using flags for permanent config (e.g., `database_url_pattern`) | That's config, not a flag |
| Reading flag with a fallback default in code | Fail-fast `expect()`/`KeyError`; missing flag = startup failure |

## Failure Modes

| Symptom | Cause | Fix |
|---|---|---|
| `release-train-guard.sh` fails G111 | Flag default-ON in non-owning train | Set to `false` in offending bundle |
| `release-train-retire` refuses | Live flag still on retiring train | Run cleanup audit + cleanup spec first |
| Flag-cleanup-audit shows N overdue flags | Train graduated > 1 cycle ago | Open cleanup spec for each |
| Service startup fails: `NEW_PAYMENT_FLOW env required` | Flag exists in code but not in bundle | Either add to bundles OR remove from code |

## See Also

- Skill: `bubbles-release-train-model` (trains + phases)
- Skill: `bubbles-config-bundle-per-train` (bundle authoring)
- Agent: `bubbles.train` (owns lifecycle)
- Gates: G111 (default-off), G110 (release-train-discipline)
