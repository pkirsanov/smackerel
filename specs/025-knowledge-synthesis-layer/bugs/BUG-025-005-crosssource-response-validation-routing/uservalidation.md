# User Validation Checklist: BUG-025-005

## Checklist

- [x] Baseline bug packet correctly describes the cross-source response validation symptom supplied by the operator.
- [x] Expected behavior preserves artifact validation and existing digest/photo/unknown-subject semantics.

Unchecked items indicate a user-reported regression.

## Goal

- Goal: trustworthy publish-time validation for cross-source synthesis results.
- Success signal: valid concept results publish without artifact-schema errors, while malformed results enter poison handling before publication.

## Journey Steps

| Step | User Intent | Observed | Evidence | Friction |
|------|-------------|----------|----------|----------|
| 1 | Stabilize cross-source synthesis | False outgoing validation errors were observed while results still published | `bug.md#actual-behavior` | broken |

## Open Refinements

None recorded at packet initialization.