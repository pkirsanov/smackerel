---
description: How to operate the trunk + release-train + per-train-flag-bundle model. Use when cutting/promoting/rolling-back/retiring a release train, when declaring a feature flag, or when designing a feature spec that targets a specific train.
---

# Release-Train Model

## Mental Model

**Trunk = `main`.** Always green. Every commit produces a signed immutable build (G074).

**Release train = a named, long-lived ship-line.** Operator-chosen string id (`mvp`, `v1.0`, `2026-q3`, `hardening`, anything). Each train has:

| Field | Values | Meaning |
|---|---|---|
| `phase` | `active` / `maintained` / `frozen` / `retired` | Lifecycle stage |
| `target_slot` | `prod` / `staging` / `none` | Where promotes can land |
| `flags_bundle` | path to YAML | Per-train feature-flag default set |
| `retention` | string (e.g. `7d-daily,4w-weekly,12m-monthly`) | Backup retention for this train |
| `pii` | `none` / `pseudonymized` / `encrypted-only` | Backup PII classification (G120) |

**Feature flag = the switch that gates trunk code per-train.** A spec declares `flagsIntroduced: [<flag>]` in `state.json`. The flag is default-ON in the spec's `releaseTrain`, default-OFF in every other train (G111).

## The Five Operations (owned by `bubbles.train`)

| Operation | Verb | What happens | Status ceiling |
|---|---|---|---|
| **Cut** | `release cut <train>` | Tag trunk at SHA; CI builds signed candidate (digests + per-train bundle). NO deploy. | `train_cut` |
| **Promote** | `release promote <train> <slot>` | Pointer-swap knb manifest to candidate digests+bundle. Requires soak evidence + backup freshness (G112) + restore-drill currency (G113). | `train_promoted` |
| **Rollback** | `release rollback <train>` | Pointer-swap to previous manifest commit. Pure git history op. Never rebuilds. | `train_rolled_back` |
| **Retire** | `release retire <train>` | Transitions phase to `retired`. Requires all train's flags cleaned up. | `train_retired` |
| **Flag audit** | `release flag-audit` | Lists flags whose owning train graduated > 1 cycle. Packets cleanup work. | (audit-only) |

## Train Phase Lifecycle

```
active ──────► maintained ──────► frozen ──────► retired
   ▲                │                  │              │
   │ (cuts+promotes)│ (cuts only,      │ (no cuts,    │ (read-only)
   │                │  no promotes)    │  flags must  │
   │                │                  │  be cleaned) │
   └────────────────┴──────────────────┴──────────────┘
                  reverse transitions allowed
                  via release-train-cut mode
```

## Feature Flag Discipline (G111 — NON-NEGOTIABLE)

For every spec with `state.json` containing:

```json
{
  "releaseTrain": "mvp",
  "flagsIntroduced": ["new_payment_flow", "fast_checkout"]
}
```

`release-train-guard.sh` verifies:

1. `config/feature-flags.mvp.yaml` exists.
2. In `mvp` bundle: `flags.new_payment_flow: true` is allowed (default ON for owning train).
3. In `config/feature-flags.v1.yaml`, `config/feature-flags.experimental.yaml`, etc.: `flags.new_payment_flow: false` REQUIRED. Default-ON on a non-owning train is a BLOCKING violation.

## What Lives Where

| File | Owner | Read access |
|---|---|---|
| `config/release-trains.yaml` | `bubbles.train` | all |
| `config/feature-flags.<train>.yaml` | `bubbles.train` | `implement`, `upkeep` |
| `state.json` `releaseTrain` field | `bubbles.train` | all |
| `state.json` `flagsIntroduced` field | `bubbles.train` | all |
| `docs/Release_Trains.md` train roadmap | `bubbles.train` | `docs` (sync only) |
| knb `<product>/<target>/manifest.yaml` | `bubbles.devops` (via packet from `bubbles.train`) | `upkeep`, `audit`, `validate` |

## Cross-Domain Cooperative Reads (B2 boundary)

`bubbles.train` **MAY** read `/srv/backups/upkeep-ledger.jsonl` (owned by `bubbles.upkeep`) to gate `promote` on:
- Last backup within cadence (G112)
- Last restore-drill within cadence (G113)

`bubbles.train` **NEVER** writes to upkeep ledger. Failure routing goes through packet to `bubbles.upkeep`.

## Common Mistakes

| ❌ Wrong | ✅ Right |
|---|---|
| Branching `release/v1` from `main` and merging selectively | Trunk-based: tag SHA + per-train flag bundle |
| Hardcoding `mvp`/`v1`/`v2` as train names | Operator-chosen strings; framework agnostic |
| Adding a new spec without declaring `releaseTrain` | Spec template's `## Release Train` section is REQUIRED |
| Defaulting a new flag ON in every bundle "to test in v1 too" | Default-ON ONLY in owning train; OFF in all others |
| Rebuilding during `rollback` | Pointer-swap only; rebuild forbidden by mode constraint |
| Promoting without restore-drill within cadence | G113 blocks; run `upkeep restore-test` first |

## See Also

- Skill: `bubbles-config-bundle-per-train` (bundle authoring contract)
- Skill: `bubbles-flag-lifecycle` (naming, default-off, retirement triggers)
- Instructions: `bubbles-release-trains.instructions.md`
- Agent: `bubbles.train` (Detroit Velvet Smooth)
- Gates: G110, G111, G081 (Build-Once Deploy-Many Integrity)
