---
applyTo: "**"
---

# Release Trains Policy (NON-NEGOTIABLE)

> This instructions file is loaded into every agent context for repos that have
> adopted release trains. It encodes the rules from
> [`bubbles-release-train-model`](../skills/bubbles-release-train-model/SKILL.md),
> [`bubbles-config-bundle-per-train`](../skills/bubbles-config-bundle-per-train/SKILL.md),
> and [`bubbles-flag-lifecycle`](../skills/bubbles-flag-lifecycle/SKILL.md).

## The Rule

**Trunk-based development with named release trains. Every spec declares its train. Every feature flag is default-ON in exactly one train. No long-lived feature branches.**

## What This Means For Agents

### When Writing A Spec (`bubbles.analyst`, `bubbles.plan`)

- `state.json` MUST include `releaseTrain: <train-id>` where `<train-id>` is a string that EXISTS in `config/release-trains.yaml`.
- `state.json` MUST include `flagsIntroduced: []` array (empty is fine if no flags).
- `spec.md` MUST include a `## Release Train` section declaring the target train and the default-off behavior on other trains.
- Refuse to create a spec that targets a non-existent train. Surface to operator first.

### When Implementing A Feature (`bubbles.implement`)

- For every flag in `state.json` `flagsIntroduced`, the code MUST read the flag from an env var with NO fallback default. Missing env var = startup failure.
- The flag bundle entries (`config/feature-flags.<train>.yaml`) MUST be edited as part of the implementation:
  - `true` in the owning train's bundle
  - `false` in EVERY OTHER train's bundle
- `release-train-guard.sh` enforces this at pre-push. There is no `--skip` flag.

### When Running A Release Train Operation (`bubbles.train`)

- Only train ids declared in `config/release-trains.yaml` are valid.
- Cut/promote/rollback/retire are the only operations. No ad-hoc operations.
- Promote REQUIRES backup-freshness (G112) + restore-drill currency (G113). Refuse if missing.
- Rollback NEVER rebuilds. Pure pointer-swap from `git show HEAD~1:...`.
- Retire REQUIRES all flags on the train are cleaned up first.

### When Doing Any Other Work

- DO NOT mutate `config/release-trains.yaml` or `config/feature-flags.<train>.yaml` from any agent other than `bubbles.train`. Packet the change.
- DO NOT mutate knb-side `<product>/<target>/manifest.yaml` from any agent other than `bubbles.devops` (via packet from `bubbles.train`).
- DO READ those surfaces freely if needed for decision-making (B2 cooperative boundary).

## Forbidden Patterns

| ❌ Forbidden | ✅ Required |
|---|---|
| Branching `release/v1.0` from main and merging selectively | Trunk + train + flag bundle |
| Long-lived feature branches | Default-off flag on trunk |
| Hardcoded train names in framework code | Operator-chosen strings; framework agnostic |
| `if env == "prod"` runtime checks | Flag reads via env var, decided by which bundle the env got |
| Flag with fallback default in code (`env::var().unwrap_or(false)`) | Fail-fast: `env::var().expect("FLAG required")` |
| Flag default-ON in multiple trains | Default-ON in exactly one (the owning train) |
| Spec without `releaseTrain` field | Reject; surface to operator before proceeding |
| Bypass via `release-train-guard.sh --skip` | No such flag; never will be |
| Rebuild during rollback | Pointer-swap only |

## Pre-Push Wiring

The repo's pre-push hook MUST call:

```bash
bash .github/bubbles/scripts/release-train-guard.sh "$(pwd)" || exit 1
```

`release-train-guard.sh` exit codes:
- `0` — clean
- `1` — violations (printed to stderr; commit blocked)

## See Also

- Skill: `bubbles-release-train-model`
- Skill: `bubbles-config-bundle-per-train`
- Skill: `bubbles-flag-lifecycle`
- Agent: `bubbles.train` (Detroit Velvet Smooth)
- Gates: G110, G111
