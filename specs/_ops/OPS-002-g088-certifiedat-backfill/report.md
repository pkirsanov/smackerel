# OPS-002 Report — Gate G088 `certifiedAt` backlog remediation

## Summary

The 20-round stochastic quality sweep surfaced a recurring Gate G088
(`post_certification_spec_edit_gate`) finding: most `done` specs lack a
top-level `certifiedAt` or have one predating a later planning edit. This packet
classifies the full backlog with a reproducible tool, executes the single
unambiguously-safe clean backfill (spec 018), and tracks the review-gated
remainder. It deliberately does NOT bulk-recertify — doing so without per-spec
review would fabricate certification reviews and violate the repo
anti-fabrication policy.

## Classified Inventory

```text
$ python3 specs/_ops/OPS-002-g088-certifiedat-backfill/analysis.py
...
specs/064-open-ended-knowledge-agent: NEEDS-RECERT cert=2026-06-02 (certification.completedAt) plan-commit=2026-06-05
specs/070-web-username-password-login: NEEDS-RECERT cert=2026-06-01 (certification.certifiedAt) plan-commit=2026-06-05

CLEAN-BACKFILL: 0
NEEDS-RECERT: 38
NO-SOURCE: 6
```

(`CLEAN-BACKFILL: 0` after this packet because the single clean spec — 018 — has
already been backfilled below; before remediation it was `CLEAN-BACKFILL: 1`.)

Portfolio totals at discovery: **66 of 78 done specs failed G088** — 45 MISSING
top-level `certifiedAt`, 21 STALE (have `certifiedAt`, later planning edit). The
45 MISSING split into 1 CLEAN-BACKFILL, 38 NEEDS-RECERT, 6 NO-SOURCE.

## Clean-Backfill Execution (spec 018)

Surfaced the existing historical certification timestamp to the top level. The
faithful source is `lastUpdatedAt = 2026-05-24T22:00:00Z`, which is AFTER the
spec's last planning-truth commit (`2026-05-24T20:49:41Z`), so no planning edit
follows it and G088 clears with NO fresh-review claim.

```text
$ bash .github/bubbles/scripts/post-cert-spec-edit-guard.sh specs/018-financial-markets-connector
post-cert-spec-edit-guard: PASS Gate G088 (post_certification_spec_edit_gate) - spec=specs/018-financial-markets-connector status=done certifiedAt=2026-05-24T22:00:00Z trackedFiles=3
G088_RC=0
```

The backfill is annotated in `specs/018-financial-markets-connector/state.json`
via `certifiedAtBackfillNote` documenting the source and the
no-review-claimed boundary.

## Test Evidence

### T-OPS-002-1 — `analysis.py` reproduces the classification

```text
$ python3 specs/_ops/OPS-002-g088-certifiedat-backfill/analysis.py
specs/064-open-ended-knowledge-agent: NEEDS-RECERT cert=2026-06-02 (certification.completedAt) plan-commit=2026-06-05
specs/070-web-username-password-login: NEEDS-RECERT cert=2026-06-01 (certification.certifiedAt) plan-commit=2026-06-05

CLEAN-BACKFILL: 0
NEEDS-RECERT: 38
NO-SOURCE: 6
```

### T-OPS-002-2 — spec 018 passes G088 after backfill

```text
$ bash .github/bubbles/scripts/post-cert-spec-edit-guard.sh specs/018-financial-markets-connector
post-cert-spec-edit-guard: PASS Gate G088 (post_certification_spec_edit_gate) - spec=specs/018-financial-markets-connector status=done certifiedAt=2026-05-24T22:00:00Z trackedFiles=3
G088_RC=0
```

### T-OPS-002-3 — JSON validity of the backfilled state

```text
$ python3 -c "import json;json.load(open('specs/018-financial-markets-connector/state.json'));print('JSON OK')"
JSON OK
```

## Anti-Fabrication Boundary

`git log` of planning-file edits in the post-cert window shows the post-cert
commits are a **mix** of benign housekeeping (tier-1..6 drift cleanup, OPS-001
banner sweep, release-planning) AND substantive work ("ship Structured Intent
Compiler", "design amendment", "rescope close-out", "operator ratifies
uservalidation", per-round convergence closures). Because a recert's
`bubbles.spec-review CURRENT` entry asserts those edits were reviewed, writing
one without a real per-spec review would be fabrication. The 38 NEEDS-RECERT, 6
NO-SOURCE, and 21 STALE specs are therefore tracked below for per-spec,
review-gated remediation rather than bulk-stamped.

## Tracked Remediation Queue

| Class | Count | Faithful remediation (per spec) |
|-------|-------|---------------------------------|
| NEEDS-RECERT | 38 | Review post-cert planning edits; if trustworthy, add `certifiedAt`=now + `bubbles.spec-review CURRENT` entry citing the reviewed commits; else demote/route. |
| NO-SOURCE | 6 | Reconstruct the cert moment from git history of the certifying commit; then as NEEDS-RECERT. |
| STALE | 21 | Same as NEEDS-RECERT (already has a `certifiedAt`; bump + review entry). |

Per-spec precedent recipes already exist and are proven: BUG-029-007,
BUG-024-004, BUG-049-002, BUG-054-001 (each closed this class for one spec with
real evidence).

## Change Boundary Verification

Only the OPS-002 packet and the single clean-backfill spec changed; no planning
truth of any target spec, no runtime/config:

```text
$ git diff --name-only
specs/018-financial-markets-connector/state.json
specs/_ops/OPS-002-g088-certifiedat-backfill/analysis.py
specs/_ops/OPS-002-g088-certifiedat-backfill/bug.md
specs/_ops/OPS-002-g088-certifiedat-backfill/design.md
specs/_ops/OPS-002-g088-certifiedat-backfill/report.md
specs/_ops/OPS-002-g088-certifiedat-backfill/scenario-manifest.json
specs/_ops/OPS-002-g088-certifiedat-backfill/scopes.md
specs/_ops/OPS-002-g088-certifiedat-backfill/state.json
specs/_ops/OPS-002-g088-certifiedat-backfill/uservalidation.md
```

## Completion Statement

**Classification + clean-backfill + tracking: Complete.** `analysis.py`
deterministically classifies the portfolio; spec 018 is faithfully backfilled
(G088 PASS); the review-gated remainder (38 NEEDS-RECERT + 6 NO-SOURCE + 21
STALE) is tracked with a per-spec faithful remediation contract. No fabricated
reviews. This packet terminates at `status: specs_hardened` — the bulk recert is
review-gated follow-on work, consistent with the sweep's own recommendation and
the OPS-001 precedent.

## Remediation Execution Addendum (2026-06-06)

The review-gated remainder was subsequently executed via per-spec faithful
review (classifier `classify.py` added: it lists, per spec, every post-cert
commit that touched planning truth and confirms whether they are all in the
benign documentation-reconciliation campaign). Each spec was reviewed against
its REAL post-cert `git diff` hunks and recertified (state.json-only: top-level
`certifiedAt`/`certifiedBy` + a `bubbles.spec-review` `reviewStatus: CURRENT`
executionHistory entry citing the specific commits reviewed). NO-SOURCE specs
had their certification moment reconstructed from the git commit that set
`status: done`.

Result:

- **64 specs recertified** (16 benign-only + 41 substantive-but-review-confirmed
  promotion-closeout/incidental + 7 no-source-reconstructed), each with real
  per-spec commit citations. Zero fabricated reviews.
- **1 genuine finding (spec 042)** — a real post-promotion contract change
  (`15e1c453`) had reset 2 scopes to "Not started" with 26 unchecked DoD against
  a `done` status. This was NOT rubber-stamped; it was routed to a
  `reconcile-to-doc` cycle (BUG-042-001) that re-verified all 26 DoD against the
  shipped+tested fail-loud `HOST_BIND_ADDRESS` contract
  (`internal/deploy/compose_contract_test.go` PASS), re-ticked them with
  evidence, restored both scopes to Done, and recertified.
- **Portfolio G088 status: 77 PASS / 0 FAIL** (was 12 PASS / 66 FAIL at
  discovery).

No bulk timestamp stamp: every recert is backed by a real per-spec diff review.

