# Recipe: Propagate Changes Across Trains

**Persona:** J-Roc (`bubbles.propagate`).
**When to use:** You have 2+ release trains and a fix landed on one needs to move to others.

---

## The natural-language way (preferred)

Just tell `bubbles.super` what you want:

```
> ship this to v2 and v3
> backport that prod fix to experimental
> what's missing on prod
> propagation drift report
```

`bubbles.super` reads `bubbles/intent-routes.yaml`, matches your phrase, and dispatches the right propagation operation. You never have to remember the mode names.

---

## If you prefer to invoke a workflow directly

```
/bubbles.workflow propagate-forward
/bubbles.workflow propagate-forward from experimental
/bubbles.workflow propagate-backport from prod to experimental
/bubbles.workflow propagate-audit
```

---

## First-time setup (once per repo)

J-Roc needs `propagation-policy.yaml`. Copy the template:

```bash
cp templates/propagation-policy.yaml.tmpl propagation-policy.yaml
```

Edit `trains[]` to match your `config/release-trains.yaml`. Edit `defaultFlow[]` to declare your edges. Most repos want:

```yaml
defaultFlow:
  - from: experimental
    to: mvp
    auto: true
    receivingTrainValidationMode: validate-only
  - from: mvp
    to: prod
    auto: false
    receivingTrainValidationMode: full-delivery
```

Validate:

```bash
bash bubbles/scripts/propagation-policy-guard.sh
```

Should exit 0.

---

## Forward (the common case)

You landed a fix on `experimental`. You want it on `mvp` and (eventually) `prod`.

```
> ship this to mvp and prod
```

J-Roc will:

1. Read the policy.
2. Find new commits on `experimental` not yet on `mvp`.
3. Route the cherry-pick to `bubbles.devops` (Tommy Bean).
4. Route the receiving-train validation per `receivingTrainValidationMode` (e.g. `validate-only` for mvp).
5. Append one ledger entry to `propagation-ledger.yaml`.
6. If `mvp → prod` edge is `auto: true`, repeat for prod. If `auto: false`, list the next hop and stop.

**Output:** verdict `🟢 PROPAGATED` per hop, with commit shas + validation outcome.

---

## Backport (rare, guarded)

You landed a hotfix on `prod` (e.g. via cherry-pick from an emergency branch) and need it back on `experimental` so trunk doesn't drift.

```
> backport that prod fix to experimental
```

J-Roc will:

1. Verify the edge has `backportable: true` in policy.
2. Emit `🟡 AWAITING APPROVAL` with the commit diff summary.
3. Refuse to proceed.

You review the commits. If they're safe to backport, re-invoke:

```
/bubbles.workflow propagate-backport from prod to experimental --approval-token=<the-prod-commit-sha>
```

J-Roc cherry-picks, validates, records the approval token in the ledger.

---

## Audit (read-only)

```
> what's missing on prod
> propagation drift report
```

J-Roc lists commits on each upstream train that have not yet propagated to the downstream train per policy edges. No mutation. Use this before a release window to know what's drifted.

---

## Recovery from failure

If receiving-train validation fails after cherry-pick:

1. J-Roc emits `route_required` to `bubbles.devops` for revert.
2. Tommy reverts the cherry-pick.
3. J-Roc appends a `validationOutcome: failed` ledger entry.
4. Output verdict `🔴 BLOCKED` with remediation.

Most common remediation: fix the underlying issue on the source train first, then re-propagate.

---

## What J-Roc never does

- Cuts releases (that's DVS / `bubbles.train`).
- Runs `git cherry-pick` directly (that's Tommy / `bubbles.devops`).
- Edits `config/release-trains.yaml` (that's DVS).
- Edits feature flag bundles (that's DVS).
- Rewrites past ledger entries (G123 forbids).

If you need any of those, route to the right agent.

---

## Gate reference

| Gate | What it checks |
|------|----------------|
| G121 | `propagation-policy.yaml` exists and parses |
| G122 | Every edge has `receivingTrainValidationMode`; `none` requires `validationSkipReason` |
| G123 | Every operation appends one immutable ledger entry |
| G111 | (inherited from DVS) Off-train flags stay default-OFF after propagation |
| G117 | (inherited) Ledger is append-only |

---

## Quote

> *"Same fix, every park, knawmsayin?"* — J-Roc
