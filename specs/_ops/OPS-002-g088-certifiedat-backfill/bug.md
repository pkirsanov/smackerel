# OPS-002: Gate G088 top-level `certifiedAt` backlog across certified specs

- **Type:** Operational remediation packet (portfolio-wide)
- **Owner:** bubbles.workflow
- **Status:** specs_hardened (planning/tracking ceiling — bulk recert is per-spec review-gated)
- **Discovered:** Stochastic quality sweep (20-round) — recurring G088 findings in rounds 5, 7, 11, 13, 15, 16, 19, 20; promoted to a tracked systemic item.

## Problem

Gate G088 (`post_certification_spec_edit_gate`) requires every `done` spec to
carry a **top-level** `certifiedAt` field and forbids planning-truth edits
(`spec.md` / `design.md` / `scopes.md`) committed after it without
recertification.

G088 was introduced **after** most specs were certified. A portfolio scan
(`analysis.py`) finds **66 of 78 done specs** fail G088:

| Class | Count | Meaning |
|-------|-------|---------|
| MISSING top-level `certifiedAt` | 45 | Older state.json schema; `certification.status=done` but no top-level field. G088 short-circuits demanding the field. |
| STALE | 21 | Has top-level `certifiedAt`, but a planning-truth edit was committed after it. |

Of the 45 MISSING, a deeper analysis (faithful historical cert timestamp vs
latest planning commit) splits them into:

| Sub-class | Count | Faithful remediation |
|-----------|-------|----------------------|
| CLEAN-BACKFILL | 1 (spec 018) | Surface the existing historical cert timestamp to the top level. No later planning edit ⇒ G088 clears with **no review claim**. |
| NEEDS-RECERT | 38 | Planning truth was committed AFTER the faithful historical cert timestamp. A faithful recert requires **per-spec review** of those edits. |
| NO-SOURCE | 6 | No derivable historical cert timestamp; requires per-spec reconstruction. |

## Why this is NOT a bulk rubber-stamp

The post-cert planning commits are **heterogeneous** — not uniformly benign
housekeeping. `git log` of planning-file edits in the post-cert window shows a
mix of:

- benign housekeeping (tier-1..6 drift cleanup, OPS-001 banner sweep, release-planning), AND
- **substantive work** ("ship Structured Intent Compiler", "design amendment",
  "rescope close-out", "operator ratifies uservalidation", round-by-round
  convergence closures).

Asserting a blanket "CURRENT spec-review" recert across 44 specs WITHOUT actually
reviewing each spec's post-cert edits would be **fabrication** — a direct
violation of the repo's central anti-fabrication policy
(`.github/skills/bubbles-anti-fabrication/`). Therefore the bulk recert is
correctly scoped as **per-spec, review-gated** work, exactly as the sweep
recommended ("a dedicated improve-existing or harden-gaps-to-doc sweep").

## Reproduction

```text
$ python3 specs/_ops/OPS-002-g088-certifiedat-backfill/analysis.py
...
CLEAN-BACKFILL: 1
NEEDS-RECERT: 38
NO-SOURCE: 6
```

(plus 21 STALE specs that have `certifiedAt` but a later planning edit — see
`state-transition-guard` / `post-cert-spec-edit-guard` per spec.)
