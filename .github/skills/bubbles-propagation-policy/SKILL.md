---
description: How to author and maintain propagation-policy.yaml — the contract bubbles.propagate (J-Roc) reads to cherry-pick changes across release trains.
---

# bubbles-propagation-policy

`bubbles.propagate` (J-Roc) is the cross-train change-movement operator. Without `propagation-policy.yaml`, J-Roc refuses every operation and emits `blocked`. This skill explains how to write that policy.

## When to author this file

- You have **≥2 release trains** declared in `config/release-trains.yaml`.
- You want fixes / features that land on one train to flow to others without manual `git cherry-pick`.
- You want a single audit trail of which commits moved between which trains, when, by whom.

If you run only one train, you do not need a propagation policy.

## File location

J-Roc searches:
1. `propagation-policy.yaml` (repo root) — preferred
2. `config/propagation-policy.yaml`

If neither exists, J-Roc emits `blocked` with the template path.

## Schema (v1)

See `templates/propagation-policy.yaml.tmpl` for the canonical example. The required fields:

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `version` | int | yes | Currently `1` |
| `trains[]` | list | yes | Each id MUST exist in `config/release-trains.yaml` |
| `trains[].id` | string | yes | Train identifier |
| `trains[].role` | string | no | Informational: `incoming` / `staging` / `production` / `hotfix-only` |
| `defaultFlow[]` | list | yes | Directed graph of propagation edges |
| `defaultFlow[].from` | string | yes | Source train id |
| `defaultFlow[].to` | string | yes | Target train id |
| `defaultFlow[].auto` | bool | yes | `true` = forward processes without confirmation; `false` = lists with `--confirm` required |
| `defaultFlow[].receivingTrainValidationMode` | string | yes | `validate-only` \| `full-delivery` \| `none` |
| `defaultFlow[].validationSkipReason` | string | conditional | Required iff `receivingTrainValidationMode: none` |
| `defaultFlow[].backportable` | bool | no (default false) | If true, J-Roc may backport via this edge under approval guard |
| `backportRequiresApproval` | bool | yes | If true, backport refuses without `--approval-token=<sha>` |
| `ledgerPath` | string | no | Defaults to `propagation-ledger.yaml` |
| `options.defaultStrategy` | string | no | `cherry-pick` (default) or `merge` |
| `options.signOff` | bool | no | Whether to pass `--signoff` to cherry-pick |

## Edges, not chains

Each edge is independent. To express `experimental → mvp → prod`, declare two edges:

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

J-Roc processes each edge separately. The second only triggers if the first succeeded AND `mvp → prod` is auto, OR the operator explicitly invokes the next hop.

## Validation depth

| Mode | When to use |
|------|-------------|
| `validate-only` | Receiving train is short-lived (mvp, staging); want fast feedback |
| `full-delivery` | Receiving train is production; want every gate the trunk would enforce |
| `none` | Receiving train is build-only or experimental sandbox; MUST include `validationSkipReason` |

## Backport contract

Backport is intentionally awkward. The default policy ships with `backportRequiresApproval: true` and `backportable: false` on every edge. To enable a backport:

1. Mark the edge `backportable: true`.
2. Decide whether `backportRequiresApproval` stays true (recommended for any edge touching prod).
3. When operator invokes `/bubbles.workflow propagate-backport from prod to experimental`:
   - J-Roc emits `route_required` with `action: human-approval` listing the commits + diff summary.
   - Operator reviews, then re-invokes with `--approval-token=<sha>` (the sha is the approval anchor).
   - J-Roc records the approval token in the ledger entry.

## Ledger format

`propagation-ledger.yaml` (or path declared in policy) is append-only JSONL:

```json
{"timestamp":"2026-05-10T12:34:56Z","operator":"alice","operation":"forward","fromTrain":"experimental","toTrain":"mvp","commits":["abc123","def456"],"validationMode":"validate-only","validationOutcome":"passed","approvalToken":null}
{"timestamp":"2026-05-10T14:00:00Z","operator":"alice","operation":"backport","fromTrain":"prod","toTrain":"experimental","commits":["999aaa"],"validationMode":"validate-only","validationOutcome":"passed","approvalToken":"sha:abc1234567"}
```

G123 (propagation-ledger-recorded) enforces append-only discipline. Rewriting past entries fails the gate.

## Anti-patterns

| Don't | Why |
|-------|-----|
| Declare an edge with `auto: true` and `receivingTrainValidationMode: none` and no `validationSkipReason` | Hides risk — J-Roc refuses |
| Reference a train not in `config/release-trains.yaml` | DVS is the source of truth for trains — J-Roc refuses |
| Edit past ledger entries | Append-only is the audit contract — fails G123 |
| Use J-Roc for the initial cut of a release | That's DVS's job — J-Roc only moves what's already cut |
| Backport without `backportable: true` on the edge | Explicit opt-in is required — J-Roc refuses |

## Quick start

```bash
cp templates/propagation-policy.yaml.tmpl propagation-policy.yaml
# Edit trains[] to match your config/release-trains.yaml
# Edit defaultFlow[] to declare your edges
bash bubbles/scripts/propagation-policy-guard.sh
# Should exit 0. If not, fix the reported issues.
/bubbles.workflow propagate-audit   # safe first run, no mutation
```

## Gate reference

- **G121** propagation-policy-declared — file exists, schema valid
- **G122** propagation-validation-required — every edge has `receivingTrainValidationMode` and (if `none`) `validationSkipReason`
- **G123** propagation-ledger-recorded — every operation appends one ledger entry; entries are immutable
