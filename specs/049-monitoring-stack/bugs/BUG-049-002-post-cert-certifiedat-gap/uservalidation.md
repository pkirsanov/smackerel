# User Validation: BUG-049-002 — `certifiedAt` recertification gap on spec 049

## Checklist

- [x] The post-cert edits on `specs/049-monitoring-stack/spec.md` and
      `specs/049-monitoring-stack/design.md` are additive
      successor-pointer narrative only.
- [x] The fix is data-only on `specs/049-monitoring-stack/state.json`
      (no source code, operator docs, or planning-truth content
      changes).
- [x] Gate G088 (`post_certification_spec_edit_gate`) is the framework
      mechanism that re-blocks any future planning-truth drift without
      recertification.
- [x] No `git commit` / `git push` actions are taken by the agent;
      the user owns the eventual git operation.
- [x] The bug folder ships with all six required artifacts (`spec.md`,
      `design.md`, `scopes.md`, `report.md`, `uservalidation.md`,
      `state.json`).
