# BUG-052-001 Scopes

## Scope 1 — Add config/searxng symlink to bundle-secret sandbox

**Status:** Done

### Gherkin Scenarios

See [spec.md](spec.md) SCN-BUG-052-001-1, SCN-BUG-052-001-2, SCN-BUG-052-001-3.

### Implementation

- Add `symlink(repoRoot/config/searxng, tmpRoot/config/searxng)` to the sandbox
  helper in `internal/deploy/bundle_secret_contract_test.go`, grouped with the
  other config-input symlinks.
- Update the file-header doc comment to list `config/searxng/` among the
  symlinked inputs.

### Test Plan

| ID | Test | File | Expectation |
|----|------|------|-------------|
| T-BUG-052-001-1 | `TestBundleSecretContract_NoLiteralSecretsInHomeLab` + A1/A2/A3/A4 | `internal/deploy/bundle_secret_contract_test.go` | All five sub-tests pass (red→green) |
| T-BUG-052-001-2 | Adversarial-preservation re-run | `internal/deploy/bundle_secret_contract_test.go` | A1 drift / A2 leakage / A3 determinism / A4 opt-out still enforce original expectations |

### Definition of Done

- [x] `config/searxng` symlink added to the sandbox helper (red→green)
  - Evidence: see report.md "Code Diff Evidence" + "Test Evidence (red→green)".
- [x] Doc comment enumerating symlinked config inputs updated to include `config/searxng/`
  - Evidence: report.md "Code Diff Evidence" header-comment hunk.
- [x] `go test -run TestBundleSecretContract ./internal/deploy/...` passes (all 5 sub-tests)
  - Evidence: report.md "Test Evidence (red→green)" → `ok ... 56.014s`.
- [x] Adversarial guarantees (A1 drift, A2 leakage, A3 determinism, A4 opt-out) preserved — no weakened assertion
  - Evidence: only the sandbox symlink changed; zero assertion lines touched (report.md Code Diff Evidence).
- [x] Change Boundary respected — only `bundle_secret_contract_test.go` changed; zero runtime/source/cert files touched
  - Evidence: report.md "Change Boundary Verification" git diff --name-only.
