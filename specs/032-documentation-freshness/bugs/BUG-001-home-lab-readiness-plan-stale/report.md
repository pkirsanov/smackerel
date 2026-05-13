# Report: BUG-001 Home-Lab Readiness Docs Belong Outside Product Repo

## Summary

Planning packet created for the remaining documentation freshness findings from the home-lab readiness review. This packet is intentionally documentation-only and records the corrected ownership boundary: Smackerel owns generic product-side deployment contracts, while target-specific home-lab readiness belongs to knb spec `003-smackerel-home-lab-adapter-readiness` and target-adapter documentation.

## Completion Statement

This bug is not complete. The planning artifacts are created and scoped for product-to-planning mode. No documentation edits, runtime edits, source edits, config edits, CI edits, compose edits, or adapter edits are claimed in this report.

## Test Evidence

No runtime tests were executed for this planning-only artifact creation. Artifact lint was run separately by the workflow and its result is reported in the workflow result envelope.

## Planning Evidence

- New bug folder created under `specs/032-documentation-freshness/bugs/BUG-001-home-lab-readiness-plan-stale`.
- Findings captured: V-006, V-010, V-020, V-004, DOC-001, and D-001 correction.
- Scope 1 plans removal, retirement, or migration of home-lab-specific `docs/Home_Lab_Deployment_Plan.md` content out of Smackerel.
- Scope 2 plans `docs/Deployment.md` alignment around generic deployment contracts.
- Runtime/source/config/docs files were not modified by this planning packet.
