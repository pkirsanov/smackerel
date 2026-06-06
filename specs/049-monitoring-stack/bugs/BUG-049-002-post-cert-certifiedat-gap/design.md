# Design: BUG-049-002 — `certifiedAt` recertification gap on spec 049

## Current Truth

`.github/bubbles/scripts/post-cert-spec-edit-guard.sh` (Gate G088) is the
authoritative enforcer for "no silent planning-truth drift after
certification". The relevant lines (excerpted from the live guard):

```bash
certified_type="$(state_type_for 'if has("certifiedAt") then (.certifiedAt | type) else "missing" end')"
if [[ "$certified_type" != "string" ]]; then
  echo "post-cert-spec-edit-guard: G088 requires top-level certifiedAt for certified spec $spec_rel (status=$status)" >&2
  exit 2
fi
# ...
latest_current_review="$(jq -r '
  [
    .executionHistory[]?
    | select((.agent? // "") == "bubbles.spec-review")
    | select(((.reviewStatus? // .reviewVerdict? // .verdict? // "") | ascii_upcase) == "CURRENT")
    | (.runCompletedAt? // .completedAt? // .reviewedAt? // empty)
    | select(type == "string" and length > 0)
  ]
  | sort
  | last // ""
' "$STATE_FILE")"
```

The guard's PASS branches are:

1. **No post-cert edits** to planning truth files since `certifiedAt` →
   plain PASS.
2. **Post-cert edits exist** but a `bubbles.spec-review` entry in
   `executionHistory` carries `reviewStatus: CURRENT` whose
   `runCompletedAt` is at or after the latest edit → PASS with the
   `currentSpecReview` annotation.
3. **Post-cert edits exist** and `requiresRevalidation: true` is set →
   PASS with that annotation.

Spec 049 currently has post-cert edits (additive successor notices on
`spec.md` and `design.md`) and NO top-level `certifiedAt`. The guard's
first check fails, blocking the state-transition guard at Check 30.

The relevant post-cert edits are:

- `19b31c0a` (2026-05-28T05:07:50Z) — OPS-001 sweep added a one-line
  `**Status:**` banner to `spec.md`.
- `fb2a4266` (2026-06-01T04:10:49Z) — added a "Successor Notice"
  forward-pointer block to `spec.md` and a "Design Successor Note"
  forward-pointer block to `design.md`. The blocks themselves declare:
  *"This spec stays `done`; the additions amend metric names only, not
  the monitoring contract."*

The legitimate path through the guard is branch 2: record a
`bubbles.spec-review` CURRENT entry whose timestamp is at or after the
latest post-cert edit (2026-06-01T04:10:49Z), and set top-level
`certifiedAt` accordingly.

## Proposed Design

### Architecture

This bug is data-only. No source code or operator docs change. The fix
touches exactly one file:

- `specs/049-monitoring-stack/state.json`
  - Add top-level `certifiedAt: "<RFC3339>"` field reflecting the
    recertification moment (now, 2026-06-05).
  - Append one entry to top-level `executionHistory[]` recording:
    ```json
    {
      "agent": "bubbles.spec-review",
      "phase": "spec-review-recertification",
      "phasesExecuted": ["spec-review"],
      "reviewStatus": "CURRENT",
      "runStartedAt": "2026-06-05T<HH:MM:SS>Z",
      "runCompletedAt": "2026-06-05T<HH:MM:SS>Z",
      "completedAt": "2026-06-05T<HH:MM:SS>Z",
      "outcome": "post_cert_additive_successor_notices_ratified",
      "summary": "..."
    }
    ```
  - Mirror the same entry into `execution.executionHistory[]` so the
    canonical and legacy paths both carry it.

The `reviewStatus: CURRENT` value is the key the guard pattern-matches on
(`ascii_upcase` on `.reviewStatus // .reviewVerdict // .verdict`). The
`runCompletedAt` timestamp is what the guard's `latest_current_review_epoch`
extractor uses for the comparison `<= certified_epoch`.

### Sequencing

1. Run a real `bubbles.spec-review`-equivalent check inline: read each
   post-cert diff, confirm it is additive successor-pointer narrative,
   and record the verdict in the bug's `report.md` evidence section.
2. Update `state.json` with the new `certifiedAt` and the
   `bubbles.spec-review` entry, using strict IDE edit tools
   (`replace_string_in_file`), never shell redirection or `python -c`.
3. Re-run `post-cert-spec-edit-guard.sh` to prove the gate PASSES.
4. Re-run the full `state-transition-guard.sh` to prove the spec is
   promotable.
5. Re-run artifact-lint and traceability-guard to confirm no other
   regression was introduced.

### Why Not Set `requiresRevalidation: true`?

Branch 3 (`requiresRevalidation: true`) would also satisfy G088, but it
signals to downstream tooling that the spec is in a revalidation-pending
state. That is misleading here: the post-cert edits are already
non-invalidating successor notices declared as such in their own text;
the contract is genuinely current. Branch 2 (record a CURRENT spec-review
entry + bump `certifiedAt`) is the honest signal.

### Why Not Rebase / Squash The Post-Cert Edits Out?

The Successor Notice and OPS-001 status banner are real, valuable
governance content. They tell future readers that spec 049 stays the
metrics-pipeline anchor while specs 064 / 065 / 067 / 068 / 069 add
metric families through it. Removing them would erase information; the
correct response is to recertify.

## Test Strategy

| Test ID | Type | Location | Purpose |
|---------|------|----------|---------|
| T-BUG-049-002-001 | guard | `bash .github/bubbles/scripts/post-cert-spec-edit-guard.sh specs/049-monitoring-stack` | Exit 0, PASS message with `currentSpecReview` annotation. |
| T-BUG-049-002-002 | guard | `BUBBLES_AGENT_NAME=bubbles.validate bash .github/bubbles/scripts/state-transition-guard.sh specs/049-monitoring-stack` | `🟢 TRANSITION ALLOWED` overall verdict. |
| T-BUG-049-002-003 | guard | `bash .github/bubbles/scripts/artifact-lint.sh specs/049-monitoring-stack` | Lint still PASSES post-edit. |
| T-BUG-049-002-004 | guard | `timeout 120 bash .github/bubbles/scripts/traceability-guard.sh specs/049-monitoring-stack` | Traceability still 0 warnings. |
| T-BUG-049-002-005 | guard | `bash .github/bubbles/scripts/artifact-lint.sh specs/049-monitoring-stack/bugs/BUG-049-002-post-cert-certifiedat-gap` | Bug folder lint PASSES. |
| T-BUG-049-002-006 | guard | `BUBBLES_AGENT_NAME=bubbles.validate bash .github/bubbles/scripts/state-transition-guard.sh specs/049-monitoring-stack/bugs/BUG-049-002-post-cert-certifiedat-gap` | Bug folder transition guard PASSES at its terminal status. |
| T-BUG-049-002-007 | code regression | `go test ./internal/deploy/... -run 'TestMonitoring' -count=1` | All 5 monitoring contract files + adversarials still green; proves the data-only fix did not regress spec 049's runtime contracts. |
| T-BUG-049-002-008 (adversarial citation) | source | `.github/bubbles/scripts/post-cert-spec-edit-guard.sh` lines that exit non-zero when `certifiedAt` is missing OR when a post-cert edit exists without a CURRENT spec-review entry. | Demonstrate, via direct citation in `report.md`, that SCN-049-B004's adversarial branch is still enforced by the framework guard. |

## Risk Controls

- The fix is data-only and reversible by reverting `state.json` to the
  pre-fix shape.
- G088 itself is the regression mechanism: any future spec.md /
  design.md / scopes.md edit without recertification will re-trip the
  same block.
- Every guard re-run is captured in `report.md` as raw terminal output
  (no truncation, no PII).
- No git operations performed by the agent until the user reviews; the
  user owns the eventual commit.

## Open Questions

- **OQ-BUG-049-002-A:** Should the `bubbles.spec-review` entry live in
  the parent spec only, or also be mirrored in the bug folder's
  `state.json`?
  **Resolution:** Parent spec only. The bug folder records its own
  closure history; the parent's `bubbles.spec-review` entry is the
  governance signal G088 reads.
- **OQ-BUG-049-002-B:** Should the new `certifiedAt` use a wall-clock
  `now`, or the latest post-cert edit timestamp (2026-06-01T04:10:49Z)?
  **Resolution:** Use `now` (2026-06-05). Setting `certifiedAt` to
  before-or-equal-to the edit timestamp would not satisfy the
  recertification semantics; the framework wants `certifiedAt` to mean
  "ratified as of this moment".

### Single-Implementation Justification

This bug is a single data-only schema repair on one artifact
(`specs/049-monitoring-stack/state.json`). There is no second
implementation, no adapter, no strategy, no plugin to choose between.
The framework guard
`.github/bubbles/scripts/post-cert-spec-edit-guard.sh` (Gate G088) is
the single enforcement surface and is framework-immutable. No
additional implementation variant or foundation/overlay split is
warranted.
