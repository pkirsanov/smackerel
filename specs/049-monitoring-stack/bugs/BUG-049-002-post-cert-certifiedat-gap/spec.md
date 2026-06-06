# BUG-049-002 — Spec 049 state.json missing top-level `certifiedAt` blocks Gate G088

| Field | Value |
|-------|-------|
| Parent spec | `specs/049-monitoring-stack/` |
| Discovered by | Sweep round 10 (`stochastic-quality-sweep` → `harden-to-doc` mapped from `harden` trigger) |
| Discovered at HEAD | `git rev-parse HEAD` at run-start (recorded in report.md) |
| Severity | medium |
| Class | governance · state.json recertification gap · Gate G088 |
| Status | open |

## Problem Statement

`bash .github/bubbles/scripts/state-transition-guard.sh specs/049-monitoring-stack`
fails at Check 30 (Gate G088 — `post_certification_spec_edit_gate`) with:

```text
post-cert-spec-edit-guard: G088 requires top-level certifiedAt for certified
spec specs/049-monitoring-stack (status=done)
```

`specs/049-monitoring-stack/state.json` has `status: "done"` and
`certification.status: "done"`, but no top-level `certifiedAt` field. Gate
G088 requires every spec with `status` in `{done, done_with_concerns}` to
carry a top-level `certifiedAt` so the guard can compare it against git
history for any post-certification edits to planning truth
(`spec.md`, `design.md`, `scopes.md`).

The schema gap matters because spec 049's planning truth WAS edited after
the original 2026-05-13 certification:

| Commit | Date | File | Change |
|--------|------|------|--------|
| `19b31c0a` | 2026-05-28T05:07:50Z | `spec.md` | OPS-001 sweep added `**Status:** Done (certified per state.json)` banner |
| `fb2a4266` | 2026-06-01T04:10:49Z | `spec.md` | Added "Successor Notice" forward-pointer to specs 064 and 068 |
| `fb2a4266` | 2026-06-01T04:10:49Z | `design.md` | Added "Design Successor Note" forward-pointer to specs 065, 067, 068, 069 |

Inspection of each diff (`git show fb2a4266 -- specs/049-monitoring-stack/spec.md`
and `git show fb2a4266 -- specs/049-monitoring-stack/design.md`) confirms the
edits are **strictly additive successor-pointer narrative**. They do not
change:

- Requirements `FR-049-001..005`.
- Gherkin scenarios `SCN-049-M01..M04`.
- DoD items in `scopes.md`.
- Test contracts in `internal/deploy/monitoring_*_test.go`.

The Successor Notice itself states: *"This spec stays `done`; the additions
amend metric names only, not the monitoring contract."*

So the underlying contract is unchanged; the state.json schema is simply
out of step with the post-G088 governance model.

## Why It Matters

1. **Promotion blocker.** Any future state-transition guard run on spec 049
   exits non-zero. Round 10 of the active sweep cannot certify
   `specs/049-monitoring-stack` as `done` until G088 passes, even though
   every per-spec contract test and gate (artifact-lint, traceability,
   regression-baseline) is green.
2. **Sweep audit chain.** Sibling sweeps (rounds 1–9) that touch spec 049
   inherit the same block. Repairing the state.json schema unblocks the
   active sweep AND every future sweep that lands on this spec.
3. **Governance precedent.** Other certified specs in the repo
   (`052`, `054`, `057`, `071`, `075`) already carry top-level
   `certifiedAt`. Spec 049 is one of the legacy certifications still on
   the pre-G088 shape.

## Scenarios (Gherkin)

### SCN-049-B003 — Certified spec carries top-level certifiedAt

```gherkin
Given specs/049-monitoring-stack/state.json with status "done"
When  bash .github/bubbles/scripts/post-cert-spec-edit-guard.sh
      specs/049-monitoring-stack is invoked
Then  the guard exits 0
And   it reports "PASS Gate G088 (post_certification_spec_edit_gate)"
And   the message names a non-empty top-level certifiedAt timestamp.
```

### SCN-049-B004 — Adversarial: G088 still rejects a future post-cert edit without recertification

```gherkin
Given specs/049-monitoring-stack/state.json with status "done"
And   a non-empty top-level certifiedAt set at the moment of recertification
When  a hypothetical post-cert edit lands on spec.md after that certifiedAt
And   neither requiresRevalidation:true nor a fresh bubbles.spec-review
      CURRENT entry is recorded
Then  post-cert-spec-edit-guard.sh exits non-zero
And   it names the offending commit and file in its diagnostic output.
```

> SCN-049-B004 is enforced by the existing `post-cert-spec-edit-guard.sh`
> logic. The bug's regression evidence demonstrates this branch by reading
> the guard's source and citing the conditional that triggers the block,
> avoiding any real planning-truth churn.

## Out Of Scope

- Re-running every spec-049 contract test in CI; that was already done at
  initial certification on 2026-05-13 and is re-run at every
  `./smackerel.sh test unit --go` invocation. Re-execution as part of the
  bug's regression evidence is sufficient.
- Rewriting the Successor Notice content. The additive successor-pointer
  narrative is correct and stays as is.
- Tightening G088's logic. The gate is framework-managed and immutable
  from this repo (`.github/bubbles/scripts/post-cert-spec-edit-guard.sh`).
  This bug only updates state.json data.
- Touching the parent spec's `spec.md`, `design.md`, `scopes.md` content.
  Planning truth is unchanged.

## Acceptance Criteria

1. `specs/049-monitoring-stack/state.json` carries a top-level
   `certifiedAt: <RFC3339>` timestamp reflecting the moment of
   recertification (post-additive-successor-notice review).
2. `specs/049-monitoring-stack/state.json` `executionHistory` records a
   `bubbles.spec-review` entry with `reviewStatus: CURRENT` whose
   `runCompletedAt` is at or before the new top-level `certifiedAt`,
   so future G088 runs can satisfy the
   `latest_current_review_epoch <= certified_epoch` PASS branch.
3. `bash .github/bubbles/scripts/post-cert-spec-edit-guard.sh
   specs/049-monitoring-stack` exits 0 with a PASS message.
4. `BUBBLES_AGENT_NAME=bubbles.validate bash
   .github/bubbles/scripts/state-transition-guard.sh
   specs/049-monitoring-stack` reports
   `🟢 TRANSITION ALLOWED` (or warnings-only).
5. `bash .github/bubbles/scripts/artifact-lint.sh
   specs/049-monitoring-stack` still PASSES.
6. `timeout 120 bash .github/bubbles/scripts/traceability-guard.sh
   specs/049-monitoring-stack` still PASSES with 0 warnings.
7. The bug folder itself satisfies artifact-lint and state-transition
   guard for its terminal status.

## Product Principle Alignment

This bug supports Principle 8 ("Trust Through Transparency") — a certified
spec must carry a verifiable `certifiedAt` so anyone walking the repo can
prove the contract has been ratified as of a known moment. It also
supports Principle 9 ("Design For Restart, Not Perfection"): the fix is
data-only and reversible, with the regression test (Gate G088) already
present in the framework.

### Single-Capability Justification

This bug is a single data-only schema repair on one artifact
(`specs/049-monitoring-stack/state.json`). It does NOT introduce a new
capability, a second provider/component/variant, or any adapter/strategy/
plugin pattern. The framework already owns the singular capability
“state.json schema + Gate G088 recertification ledger” in
`.github/bubbles/scripts/post-cert-spec-edit-guard.sh`. No
multi-implementation foundation is warranted.
