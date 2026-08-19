# Scopes: BUG-061-013 — Make the wrapper-ordering contract fail when it cannot see the invocation

## Scope 1: Zero-match is a hard failure, and the matcher recognises real shell forms

**Scope ID:** `BUG-061-013-SCOPE-01`
**Status:** Not Started
**Depends On:** none

### Change Boundary

**Allowed surfaces:** `internal/deploy/envsubst_wrapper_contract_test.go` — this file only.

**Excluded surfaces, with justification** (these are load-bearing exclusions, not caution):

| Surface | Why it must not change |
|---|---|
| `scripts/runtime/go-integration.sh` | The wrapper is **correct**. Reverting line 76 to a bare `go test` would turn the lane green while re-breaking the BUG-061-011 eval gate the conditional form exists to serve, and would leave the zero-match hole open for the next wrapper. Editing the subject to satisfy a broken detector is the inverse of a fix. |
| `scripts/runtime/{go-unit,go-e2e,go-stress}.sh` | Unaffected; their invocations already match. Touching them would confound the before/after signal. |
| `scripts/runtime/_ensure_envsubst.sh` | The install path is not implicated. |
| `specs/061-conversational-assistant/bugs/BUG-061-011-*/` | A regression phase is in flight against that packet. |

**Consumer impact sweep:** the changed surface is a `_test.go` file in `internal/deploy`. It has no
production consumer, exports no symbol used outside its own package tests, and participates in no
build target other than `./smackerel.sh test unit`. The only observable effect is the verdict of
`TestEnvsubstWrapperContract_*`.

### Gherkin Scenarios (Regression Tests)

```gherkin
Feature: BUG-061-013 A wrapper-ordering contract cannot pass on nothing

  Scenario: SCN-01 A tracked wrapper whose invocation the matcher cannot locate is REJECTED
    Given a wrapper that sources _ensure_envsubst.sh and calls ensure_envsubst
    And its test invocation is written in a form the matcher does not recognise
    When assertEnvsubstWrapperContract evaluates that wrapper
    Then it returns an error naming the LOCATOR as the thing that failed
    And it does not return nil

  Scenario: SCN-02 The error distinguishes a blind matcher from a malformed wrapper
    Given a wrapper whose invocation cannot be located
    When assertEnvsubstWrapperContract returns its error
    Then the message states the invocation could not be LOCATED
    And the message says the matcher may need widening rather than blaming the wrapper

  Scenario: SCN-03 The conditional-and-piped form in use today is located
    Given scripts/runtime/go-integration.sh at HEAD, whose line 76 begins "if ! go test"
    When the live wrapper contract evaluates it
    Then the matcher locates that invocation
    And the ensure_envsubst ordering is genuinely compared against it

  Scenario: SCN-04 The restored ordering check has teeth on the previously-blind wrapper
    Given the matcher now locates go-integration.sh's invocation
    When ensure_envsubst is moved to AFTER that invocation
    Then the live subtest for go-integration.sh FAILS
    And it fails with the ordering error, not the locator error

  Scenario: SCN-05 All four tracked wrappers still pass on their true ordering
    Given go-unit.sh, go-integration.sh, go-e2e.sh and go-stress.sh unmodified at HEAD
    When TestEnvsubstWrapperContract_LiveWrappers runs
    Then every subtest passes
    And no existing assertion was weakened to achieve it
```

### Implementation Plan

1. Add an explicit zero-match branch to `assertEnvsubstWrapperContract`, replacing the
   `goTestIdx != nil &&` short-circuit at line 108. Absence returns an error whose text names the
   locator, matching the shape already used for the source-line and call locators.
2. Widen `envsubstGoTestRE` (line 82) so a leading conditional, list operator, or pipeline segment
   does not defeat it. Update the regex's own comment, which currently claims "Whitespace-leading is
   OK" as the full allowance and would otherwise become false.
3. Add the SCN-01/SCN-02 adversarial fixture — a wrapper the matcher cannot read — asserting REJECT
   and asserting the error substring.
4. Add the SCN-04 adversarial fixture — the line-76 form with `ensure_envsubst` moved after it —
   asserting the **ordering** error specifically, so step 2 cannot pass by making the matcher match
   something harmless.

Steps 1 and 2 must land in the same change: step 1 alone leaves the lane RED on a true finding, and
step 2 alone turns the lane green while leaving the actual defect intact.

### Test Plan

| # | Test | Category | File | Command | Live |
|---|---|---|---|---|---|
| T-01 | SCN-01 — unlocatable invocation is rejected | `unit` | `internal/deploy/envsubst_wrapper_contract_test.go` | `./smackerel.sh test unit --go --go-run 'TestEnvsubstWrapperContract_AdversarialRejectsUnlocatableInvocation' --verbose` | No |
| T-02 | SCN-02 — error names the locator, not the wrapper | `unit` | `internal/deploy/envsubst_wrapper_contract_test.go` | `./smackerel.sh test unit --go --go-run 'TestEnvsubstWrapperContract_AdversarialRejectsUnlocatableInvocation' --verbose` | No |
| T-03 | SCN-03 + SCN-05 — all four live wrappers, ordering genuinely compared | `unit` | `internal/deploy/envsubst_wrapper_contract_test.go` | `./smackerel.sh test unit --go --go-run 'TestEnvsubstWrapperContract_LiveWrappers' --verbose` | No |
| T-04 | SCN-04 — conditional form with inverted ordering is rejected | `unit` | `internal/deploy/envsubst_wrapper_contract_test.go` | `./smackerel.sh test unit --go --go-run 'TestEnvsubstWrapperContract_AdversarialRejectsConditionalCallAfterGoTest' --verbose` | No |
| T-05 | Pre-existing adversarial trio still has teeth (no weakening) | `unit` | `internal/deploy/envsubst_wrapper_contract_test.go` | `./smackerel.sh test unit --go --go-run 'TestEnvsubstWrapperContract_Adversarial' --verbose` | No |
| T-06 | The integration wrapper the guard protects still runs | `integration` | `scripts/runtime/go-integration.sh` | `./smackerel.sh test integration` | Yes |
| T-07 | No regression in the unit lane | `unit` | whole lane | `./smackerel.sh test unit` | No |

`e2e`, `stress`, and `load` are **not** in this plan, and the reason is stated rather than assumed:
the change is confined to a `_test.go` file in `internal/deploy` with no production consumer, so
there is no runtime surface for those categories to exercise. T-06 is included because
`go-integration.sh` is the wrapper whose ordering guarantee is being restored, and the guarantee is
worthless if the wrapper it describes no longer runs.

### Definition of Done

- [ ] SCN-01 — a tracked wrapper whose invocation the matcher cannot locate is REJECTED by `assertEnvsubstWrapperContract`; absence returns a non-nil error rather than falling through to `return nil` (T-01)
- [ ] SCN-02 — the rejection message states the invocation could not be LOCATED and that the matcher may need widening, so a reader does not waste time auditing a correct wrapper (T-02)
- [ ] SCN-03 — the matcher locates `scripts/runtime/go-integration.sh:76`, whose invocation begins `if ! go test`, and the `ensure_envsubst` ordering is compared against it (T-03)
- [ ] SCN-05 — all four tracked wrappers pass on their true ordering with no assertion weakened; the regex comment at line 79-81 is updated so it does not still claim whitespace is the only permitted prefix (T-03)
- [ ] **ADVERSARIAL — red-then-green for the zero-match branch.** A fixture whose invocation the matcher cannot find MUST make the test go RED. Demonstrated by running the new fixture against the PRE-FIX `assertEnvsubstWrapperContract` and recording the GREEN false pass, then against the post-fix function and recording the RED rejection. A fixture that is red only after the fix, with no pre-fix observation, does not prove the fixture has teeth (T-01)
- [ ] **ADVERSARIAL — red-then-green for the restored ordering check.** SCN-04: with the matcher widened, moving `ensure_envsubst` after the line-76 form MUST fail with the ORDERING error. Recorded pre-fix (passes — the defect) and post-fix (fails — the fix). This is the item that proves step 2 widened the matcher onto the real invocation rather than onto something inert (T-04)
- [ ] The three pre-existing adversarial sub-tests still reject their fixtures with their original error substrings — the fix did not blunt existing detection (T-05)
- [ ] The integration lane still runs, so the ordering guarantee being restored still describes a live wrapper (T-06)
- [ ] The full unit lane exits 0 with no newly failing test attributable to this change (T-07)
- [ ] `git diff --stat` shows exactly one changed file, `internal/deploy/envsubst_wrapper_contract_test.go`; `scripts/runtime/` is byte-identical to HEAD (spec AC-5)
- [ ] Build Quality Gate: `./smackerel.sh lint` and `./smackerel.sh format --check` exit 0 with zero warnings; `bash .github/bubbles/scripts/artifact-lint.sh` on this packet exits 0; no deferred findings
