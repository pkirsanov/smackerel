# Smackerel Release Trains

Owned by `bubbles.train` (Detroit Velvet Smooth).

## Current Trains

| Train | Phase | Slot | Purpose |
|-------|-------|------|---------|
| `mvp` | active | self-hosted | Current self-hosted train; ingest + digest + delivery |
| `next` | active | staging | Next promotion candidate; synthesis + multi-source coord |

See [`config/release-trains.yaml`](../config/release-trains.yaml).

## Operator Commands

`./smackerel.sh release ...` (wiring pending). Until then, framework guards run via:
- `bash .github/bubbles/scripts/release-train-guard.sh .`
- `bash .github/bubbles/scripts/release-train-flag-audit.sh .`

## Pre-Promote Gates

G110, G111, G112, G113, G114 (prod-slot), G115, G116, G117-G120.

## See Also

- [`Upkeep_Runbook.md`](Upkeep_Runbook.md)
- BCDR plan (knb-side): `<knb-repo>/docs/BCDR_Plan.md`
- Per-target deploy adapter: `<deployment-owner>/<product>/<target>/`
- Framework skill: `bubbles-release-train-model`
