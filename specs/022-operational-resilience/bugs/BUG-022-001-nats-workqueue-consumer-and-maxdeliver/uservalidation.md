# BUG-022-001 User Validation

> Parent acceptance items (under [`specs/022-operational-resilience/uservalidation.md`](../../uservalidation.md)):
> NATS reliability / chaos resilience scope acceptance items that depend on
> `./smackerel.sh test integration` exiting 0 on the three named tests.

## Checklist

- [x] BUG-022-001-OWNERSHIP: Existing bug packet is reopened and routable without creating a duplicate.
  - **Steps:**
    1. Inspect the existing BUG-022-001 folder.
    2. Restore missing bug-owned artifacts.
    3. Mark state as `in_progress` and route implementation to `bubbles.implement`.
  - **Expected:** `bug.md` and `scenario-manifest.json` exist; `state.json.status` and `certification.status` are `in_progress`; the runtime-fix items below remain unchecked until source changes and fresh passing tests exist.
  - **Evidence:** report.md § Reopened Regression - 2026-04-29; state.json `currentHandoff.toAgent = bubbles.implement`.
  - **Notes:** Ownership/control-plane repair only; this does not claim the NATS runtime failure is fixed.

- [x] BUG-022-001-A: `TestNATS_PublishSubscribe_Artifacts` passes on the live
      integration stack with no `err_code=10100 "filtered consumer not unique
      on workqueue stream"` from `js.CreateOrUpdateConsumer`.
  - **Steps:**
    1. `./smackerel.sh down && ./smackerel.sh up`
    2. `./smackerel.sh test integration -- -run TestNATS_PublishSubscribe_Artifacts`
  - **Expected:** test exits PASS; no `err_code=10100` in the output.
  - **Verify:** raw `go test` output captured in `report.md` § Test Evidence.
  - **Evidence:** report.md § Post-Fix Integration Proof - 2026-04-29 - `timeout 600 ./smackerel.sh test integration` exit 0; target test PASS.
  - **Notes:** Reconciled by `bubbles.plan` after fresh implementation/test evidence; final certification remains validation-owned.

- [x] BUG-022-001-B: `TestNATS_PublishSubscribe_Domain` passes on the live
      integration stack with no `err_code=10100`.
  - **Steps:**
    1. `./smackerel.sh down && ./smackerel.sh up`
    2. `./smackerel.sh test integration -- -run TestNATS_PublishSubscribe_Domain`
  - **Expected:** test exits PASS; no `err_code=10100` in the output.
  - **Verify:** raw `go test` output captured in `report.md` § Test Evidence.
  - **Evidence:** report.md § Post-Fix Integration Proof - 2026-04-29 - `timeout 600 ./smackerel.sh test integration` exit 0; target test PASS.
  - **Notes:** Reconciled by `bubbles.plan` after fresh implementation/test evidence; final certification remains validation-owned.

- [x] BUG-022-001-C: `TestNATS_Chaos_MaxDeliverExhaustion` passes - exactly
      zero messages are observed after `MaxDeliver=3` exhaustion.
  - **Steps:**
    1. `./smackerel.sh down && ./smackerel.sh up`
    2. `./smackerel.sh test integration -- -run TestNATS_Chaos_MaxDeliverExhaustion`
  - **Expected:** test exits PASS; no `expected 0 messages after MaxDeliver
    exhaustion, got N>0` line in the output.
  - **Verify:** raw `go test` output captured in `report.md` § Test Evidence.
  - **Evidence:** report.md § Post-Fix Integration Proof - 2026-04-29 - `timeout 600 ./smackerel.sh test integration` exit 0; target test PASS and MaxDeliver zero-redelivery assertion preserved.
  - **Notes:** Reconciled by `bubbles.plan` after fresh implementation/test evidence; final certification remains validation-owned.

- [x] BUG-022-001-D: Full `./smackerel.sh test integration` run shows no
      collateral regressions introduced by the fix.
  - **Steps:**
    1. `./smackerel.sh down && ./smackerel.sh up`
    2. `./smackerel.sh test integration`
  - **Expected:** all previously-passing integration tests still pass.
  - **Evidence:** report.md § Post-Fix Integration Proof - 2026-04-29 - `timeout 600 ./smackerel.sh test integration` exit 0; `tests/integration`, `tests/integration/agent`, and `tests/integration/drive` pass.
  - **Notes:** Broad E2E collateral proof is also recorded in report.md § Repo Check And Broad E2E Proof with `timeout 3600 ./smackerel.sh test e2e` exit 0. Final certification remains validation-owned.

## Reopened status - 2026-04-29

The runtime checklist items above were re-checked after fresh implementation
and test evidence showed the exact BUG-022-001 validation surface passing again.
This reconciliation does not mark final certification done; `bubbles.validate`
still owns certification status and parent acceptance recertification.

## Re-certification of parent acceptance items

The parent `specs/022-operational-resilience/uservalidation.md` acceptance
items that currently depend on green NATS integration tests are intentionally
NOT flipped here. `bubbles.validate` will re-certify those parent items after
this bug closes.

## Items checked by `bubbles.implement` with executed evidence

The historical 2026-04-26 checked state was reopened, and the 2026-04-29
implementation/test evidence now supports the bug-owned runtime acceptance
items above. Certification remains pending validation-owned state transition.
