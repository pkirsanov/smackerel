# Recipe: Release Trains — Cut + Promote + Rollback + Retire

Operator-facing recipe for the four release-train operations owned by `bubbles.train` (Detroit Velvet Smooth).

## Mental Model

| Operation | When to use | What happens |
|---|---|---|
| `release cut <train>` | Trunk is at a stable SHA you want to ship | CI builds signed candidate (digests + bundle). NO deploy. |
| `release promote <train> <slot>` | Candidate has soaked successfully on lower slot | Pointer-swap knb manifest → candidate's digests+bundle |
| `release rollback <train>` | Promoted release is bad | Pointer-swap to previous manifest commit |
| `release retire <train>` | Train is no longer needed | Transition phase to `retired` after flag cleanup |
| `release flag-audit` | Periodic check (monthly is automatic via upkeep) | List flags overdue for retirement |

## Cut

```bash
# In the product repo (e.g., <product>/)
./<product>.sh release cut mvp
```

What runs (workflow mode `release-train-cut`):
1. `release-train-guard.sh` — verifies train exists, all flags discipline OK
2. CI is triggered with `train=mvp` parameter
3. CI builds per-train config bundle, signs with cosign, pushes to ghcr
4. CI writes `build-manifest-<sha>-mvp.yaml` artifact
5. Spec status set to `train_cut`; ledger entry written

Cut produces evidence — no deployment yet. To deploy, run `promote`.

## Promote

```bash
./<product>.sh release promote mvp prod
```

Pre-flight (workflow mode `release-train-promote`):
- G110: train exists, phase allows promote
- G112: backup ledger has fresh entry (≤ cadence)
- G113: restore-drill ledger has fresh entry (≤ cadence) — if stale, run `./<product>.sh upkeep restore-test mvp` first
- G114: BCDR-drill ledger has fresh entry (quarterly cadence) for prod slot

Execution:
1. `bubbles.train` reads the latest candidate manifest for `mvp`
2. Packets to `bubbles.devops`: `apply.sh` with new digests + bundle
3. `bubbles.devops` runs adapter `apply.sh` (in knb-side `<product>/<target>/`)
4. `bubbles.train` commits the manifest update with structured message: `train(<product>/home-lab): promote mvp -> abc12345`
5. Spec status set to `train_promoted`

## Rollback

```bash
./<product>.sh release rollback mvp
```

Pure pointer-swap. No rebuild.

1. `bubbles.train` reads `git show HEAD~1:knb/<product>/home-lab/manifest.yaml`
2. Packets to `bubbles.devops`: `apply.sh` with PREVIOUS digests + bundle
3. Commit: `train(<product>/home-lab): rollback mvp -> def67890`
4. Spec status set to `train_rolled_back`

## Retire

```bash
./<product>.sh release retire experimental
```

Pre-flight (workflow mode `release-train-retire`):
- G111: all flags introduced by specs on this train MUST be cleaned up first
- If any flag still lives in code, retire is REFUSED — run `release flag-audit` to find them, then open cleanup specs

Execution:
1. `bubbles.train` updates `config/release-trains.yaml` setting `phase: retired`
2. Commit: `train(<product>): retire experimental train`
3. Spec status set to `train_retired`

## Common Failure Modes

| Symptom | Cause | Fix |
|---|---|---|
| Cut fails with "G111 violation" | Flag default-ON on non-owning train | Edit the offending bundle to set flag `false` |
| Promote fails with "G112 backup stale" | No successful backup ledger entry within cadence | Run `./<repo>.sh upkeep backup` first |
| Promote fails with "G113 restore-drill stale" | No successful restore-test within cadence | Run `./<repo>.sh upkeep restore-test <train>` first |
| Promote fails with "G116 offsite required" | `offsite_required: true` + `OFFSITE_BACKEND=local-only` | Configure USB or cloud offsite; update `params.yaml` |
| Rollback fails with "no previous manifest" | First-ever apply | Cannot rollback first deployment; only forward |
| Retire fails with "live flags" | Flags introduced by train still in code | Open flag-cleanup specs via `bubbles.train` |

## See Also

- Skill: `bubbles-release-train-model`
- Skill: `bubbles-flag-lifecycle`
- Recipe: `upkeep-monthly.md` (backup + restore-drill cadence)
- Agent: `bubbles.train`
