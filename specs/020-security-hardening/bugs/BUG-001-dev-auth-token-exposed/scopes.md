# Scopes: [BUG-001] Dev Auth Token Exposed as Functional Default

## Execution Outline

### Phase Order
1. **Scope 1 — Reject default token + change YAML default:** Add `dev-token-smackerel-2026` to placeholder list, add `dev-token-*` prefix check, change `config/smackerel.yaml` default to empty string, add regression tests.

### New Types & Signatures
No new types or signatures. Changes are to existing `Validate()` logic and config YAML.

### Validation Checkpoints
- After Scope 1: `./smackerel.sh test unit` passes with new rejection tests. `config/smackerel.yaml` has empty `auth_token`. Existing tests unaffected.

---

## Scope Summary

| # | Name | Surfaces | Key Tests | Status |
|---|------|----------|-----------|--------|
| 1 | Reject default token + change YAML default | `internal/config/config.go`, `config/smackerel.yaml`, `internal/config/validate_test.go` | Unit: placeholder rejection, prefix rejection, case-insensitive | [x] Done |

---

## Scope 1: Reject default token + change YAML default
**Status:** [x] Done

### Gherkin Scenarios

```gherkin
Feature: Reject guessable dev auth tokens

  Scenario: SCN-BUG001-01 — Committed default token is rejected
    Given SMACKEREL_AUTH_TOKEN is set to "dev-token-smackerel-2026"
    When config.Validate() runs
    Then it returns an error mentioning "placeholder" or "guessable"

  Scenario: SCN-BUG001-02 — Dev-token prefix pattern is rejected
    Given SMACKEREL_AUTH_TOKEN is set to "dev-token-myproject-2027"
    When config.Validate() runs
    Then it returns an error mentioning "dev-token-" pattern

  Scenario: SCN-BUG001-03 — Case-insensitive rejection
    Given SMACKEREL_AUTH_TOKEN is set to "DEV-TOKEN-SMACKEREL-2026"
    When config.Validate() runs
    Then it returns an error

  Scenario: SCN-BUG001-04 — Valid random token still accepted
    Given SMACKEREL_AUTH_TOKEN is set to a 48-char hex string from openssl rand
    When config.Validate() runs
    Then it returns no error

  Scenario: SCN-BUG001-05 — Empty token still accepted (dev mode)
    Given SMACKEREL_AUTH_TOKEN is empty
    When config.Validate() runs
    Then it returns no error (dev mode, startup WARN handles this)

  Scenario: SCN-BUG001-06 — YAML default is empty string
    Given config/smackerel.yaml is read
    When the auth_token field is inspected
    Then its value is "" (empty string)
```

### Implementation Plan

| File | Change |
|------|--------|
| `internal/config/config.go` | Add `"dev-token-smackerel-2026"` to `placeholders` slice. Add `strings.HasPrefix(strings.ToLower(c.AuthToken), "dev-token-")` check after the loop. |
| `config/smackerel.yaml` | Change `auth_token: "dev-token-smackerel-2026"` to `auth_token: ""` |
| `internal/config/validate_test.go` | Add `TestValidate_CommittedDefaultTokenRejected`, `TestValidate_DevTokenPrefixRejected`, `TestValidate_DevTokenCaseInsensitive` |

### Test Plan

| Type | File | Purpose | Scenarios Covered |
|------|------|---------|-------------------|
| Unit | `internal/config/validate_test.go` | Literal default rejected | SCN-BUG001-01 |
| Unit | `internal/config/validate_test.go` | Prefix pattern rejected | SCN-BUG001-02 |
| Unit | `internal/config/validate_test.go` | Case-insensitive match | SCN-BUG001-03 |
| Unit | `internal/config/validate_test.go` | Valid token passes | SCN-BUG001-04 |
| Unit | `internal/config/validate_test.go` | Empty token passes (dev mode) | SCN-BUG001-05 |
| Regression | `./smackerel.sh test unit` | All existing tests pass | SCN-BUG001-01 through SCN-BUG001-06 |

### Definition of Done

- [x] `dev-token-smackerel-2026` is in the placeholder reject list
  **Evidence:** `internal/config/config.go:865` — `"dev-token-smackerel-2026",` listed in placeholders slice (verified via `grep -n "dev-token" internal/config/config.go`).
- [x] `dev-token-*` prefix check added (case-insensitive)
  **Evidence:** `internal/config/config.go:872-874` — `if strings.HasPrefix(strings.ToLower(c.AuthToken), "dev-token-") { return fmt.Errorf(...) }`.
- [x] `config/smackerel.yaml` `auth_token` is `""` (empty string)
  **Evidence:** `config/smackerel.yaml:19` — `auth_token: "" # REQUIRED: set a secure random token (min 16 chars). Run: openssl rand -hex 24`.
- [x] Regression tests cover literal, prefix, case-insensitive, valid token, and empty token
  **Evidence:** `internal/config/validate_test.go:290-307` — `TestValidate_AuthTokenDevTokenPrefixRejected` table cases: `dev-token-smackerel-2026`, `dev-token-anything-here-1234`, `Dev-Token-MyProject-9999`. Empty/valid tokens covered by existing `TestValidate_AuthTokenExactly16Chars` and `setRequiredEnv` defaults.
- [x] `./smackerel.sh test unit` passes
  **Evidence:** Captured 2026-04-24 — `./smackerel.sh test unit` final summary `330 passed, 2 warnings in 11.48s` (Python ML) plus `ok github.com/smackerel/smackerel/internal/config 0.006s` for the focused Go run; see report.md Test Evidence.
- [x] No existing tests broken
  **Evidence:** Same `./smackerel.sh test unit` run reports `0 failed`; focused `go test -count=1 -v -run "TestValidate_AuthToken..."` shows all assertions PASS without touching other tests.
