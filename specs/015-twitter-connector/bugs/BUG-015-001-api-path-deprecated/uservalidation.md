# User Validation Checklist — [BUG-015-001] Twitter API Polling Path Deprecation

## Checklist

- [x] Baseline checklist initialized for BUG-015-001 Twitter API polling path deprecation
- [x] spec.md has a `Deferred / Non-Goals — API Path` section with the BUG-015-001 link and four-bullet rationale
- [x] spec.md goal #5 is reworded as deferred and the API Access Strategy section carries a deprecation banner
- [x] scopes.md Scope 6 status is `Deferred`; no `[ ] VERIFIED FAIL` lines remain anywhere in the spec
- [x] uservalidation.md item 13 is a strikethrough non-applicable entry pointing to BUG-015-001
- [x] state.json status and certification.status are both `done`; completedScopes is scope-01..scope-05; scope-6 progress entry has `Deferred`; resolvedBugs contains BUG-015-001
- [x] report.md has a `## BUG-015-001 Deprecation Resolution (2026-04-26)` section with files modified, governance/test evidence
- [x] scenario-manifest.json exists at the parent feature root with the 6 active scope-level scenarios
- [x] `bash .github/bubbles/scripts/artifact-lint.sh specs/015-twitter-connector` exits 0
- [x] `bash .github/bubbles/scripts/traceability-guard.sh specs/015-twitter-connector` exits 0
- [x] `./smackerel.sh test unit` exits 0 with the twitter package green
- [x] No files under `internal/connector/twitter/` were modified by this bug fix

Unchecked items indicate a user-reported regression.
