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
- [x] `dev-token-*` prefix check added (case-insensitive)
- [x] `config/smackerel.yaml` `auth_token` is `""` (empty string)
- [x] Regression tests cover literal, prefix, case-insensitive, valid token, and empty token
- [x] `./smackerel.sh test unit` passes
- [x] No existing tests broken
