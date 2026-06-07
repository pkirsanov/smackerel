# Scopes: BUG-058-INGEST-SCOPE-403

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

The extension ingest scope gate was wired with two separate scopes, which
403s every real per-user token. The fix restores the single canonical scope
`"extension:bookmarks,history"` (spec 060 / spec 058 contract) and pins it
with an end-to-end router regression test. Delivered via `bubbles-workflow
mode: bugfix-fastlane` (parent-expanded — the active runtime lacks
`runSubagent`). Single scope, Done.

## Scope 1 — Restore single-scope extension ingest gate

**Status:** Done
**Owner:** bubbles.workflow (parent-expanded bugfix-fastlane)

### Definition of Done

- [x] `internal/api/router.go` wires `auth.RequireScope("extension:bookmarks,history")` (one canonical scope); the two-scope form is removed and the adjacent comment matches the spec 060 / spec 058 contract
      → Evidence: report.md `### Code Diff Evidence` (BUILD_EXIT=0; VET=0)
- [x] Regression test `TestExtensionIngest_CanonicalScopeReachesHandler` (`internal/api/router_extension_scope_test.go`): a per-user token with `Scopes=["extension:bookmarks,history"]` reaches the ingest handler through the real `NewRouter` (not 403) — red before the fix, green after
      → Evidence: report.md `## Test Evidence` (red 403 body → green `ok`)
- [x] Adversarial twin `TestExtensionIngest_MissingScopeRejected`: a per-user token with no `scope` claim is still rejected `403` — the gate keeps enforcing
      → Evidence: report.md `## Test Evidence` (`--- PASS: TestExtensionIngest_MissingScopeRejected`)
- [x] Adversarial re-RED proof: temporarily reverting `router.go` to the two-scope form makes `TestExtensionIngest_CanonicalScopeReachesHandler` FAIL again (the test genuinely guards the wiring)
      → Evidence: report.md `## Test Evidence` (`REVERT_RC=1` re-RED transcript)
- [x] No token-format change, no schema migration, no change to shared-token/bootstrap bypass; the full `internal/api` package stays green
      → Evidence: report.md `## Test Evidence` (`ok ... internal/api 9.435s`)
- [x] `go build ./internal/api/...`, `go vet ./internal/api/` green
      → Evidence: report.md `### Code Diff Evidence` (BUILD=0; VET=0)
- [x] `SCN-058-INGEST-SCOPE-01..03` recorded in `scenario-manifest.json`
      → Evidence: `scenario-manifest.json`
- [x] Scenario-specific regression coverage for the fixed behavior — `TestExtensionIngest_CanonicalScopeReachesHandler` drives the real `POST /v1/connectors/extension/ingest` route through `NewRouter` and persists the canonical-scope-accepted invariant; it fails if the wiring regresses
      → Evidence: report.md `## Test Evidence` (red→green + re-RED transcript)
- [x] Broader regression suite passes — the full `internal/api` package (router, auth, handlers) runs green locally with the two new tests included
      → Evidence: report.md `## Test Evidence` (`ok ... internal/api 9.435s`)

### Test Plan

| ID | Test | File | Type | Scenario |
|----|------|------|------|----------|
| T-058-SCOPE-01 | TestExtensionIngest_CanonicalScopeReachesHandler | internal/api/router_extension_scope_test.go | regression (red→green, real-router httptest) | SCN-058-INGEST-SCOPE-01 |
| T-058-SCOPE-02 | TestExtensionIngest_MissingScopeRejected | internal/api/router_extension_scope_test.go | adversarial (real-router httptest) | SCN-058-INGEST-SCOPE-02 |
| T-058-SCOPE-03 | full internal/api package | internal/api/ | broader regression | SCN-058-INGEST-SCOPE-03 |

### Non-Goals

- Editing `specs/058-chrome-extension-bridge/report.md` L19 (documents the old
  two-scope form) — left for the spec owner to refresh when spec 058 unblocks,
  to keep the blocked parent's evidence stable.
- Any change to `auth.RequireScope` semantics (the middleware was correct).
