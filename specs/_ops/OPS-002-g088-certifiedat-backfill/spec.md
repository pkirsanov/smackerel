# OPS-002 Spec — Gate G088 top-level `certifiedAt` backlog remediation

## Goal

Bring the portfolio of certified specs into compliance with Gate G088
(`post_certification_spec_edit_gate`) by ensuring every `done` spec carries a
**faithful** top-level `certifiedAt`, without fabricating certification reviews.

## Actors

- **Operator / maintenance agent** — runs the remediation per spec.
- **Gate G088** (`post-cert-spec-edit-guard.sh`) — the consumer that reads
  top-level `certifiedAt`.
- **Anti-fabrication policy** — the constraint: no recert may claim a review
  that did not happen.

## Functional Requirements

- **FR-OPS-002-1**: Every `done` spec MUST carry a top-level `certifiedAt`.
- **FR-OPS-002-2**: A backfilled/recertified `certifiedAt` MUST be a **faithful**
  value — either (a) the spec's existing historical certification timestamp when
  no planning edit followed it (CLEAN-BACKFILL), or (b) a recert timestamp
  accompanied by a real `bubbles.spec-review` `reviewStatus: CURRENT` entry whose
  summary cites the actually-reviewed post-cert edits (NEEDS-RECERT / STALE).
- **FR-OPS-002-3**: NO blanket/bulk recert that asserts review without per-spec
  verification. The remediation MUST be per-spec and evidence-backed.
- **FR-OPS-002-4**: This packet executes the unambiguously-safe slice
  (CLEAN-BACKFILL) and TRACKS the review-gated remainder.

## Acceptance Scenarios

```gherkin
Scenario: SCN-OPS-002-1 Clean historical backfill
  Given a done spec with no top-level certifiedAt
  And no planning-truth commit exists after its faithful historical cert timestamp
  When the historical timestamp is surfaced to top-level certifiedAt
  Then Gate G088 passes with no fresh-review claim

Scenario: SCN-OPS-002-2 Review-gated recert is tracked, not fabricated
  Given a done spec whose planning truth was edited after its faithful cert timestamp
  When OPS-002 classifies it NEEDS-RECERT
  Then it is recorded in the tracked inventory for per-spec review
  And no certifiedAt is written that would assert an unperformed review

Scenario: SCN-OPS-002-3 Inventory is reproducible
  Given the analysis.py tool
  When it is run against the spec portfolio
  Then it deterministically classifies each done spec
  And the classification matches the committed inventory
```

## Out of Scope

- The 21 STALE specs and the 38 NEEDS-RECERT + 6 NO-SOURCE specs are TRACKED
  here but their faithful recert is per-spec, review-gated follow-on work.
- No runtime code, compose, or config changes.
