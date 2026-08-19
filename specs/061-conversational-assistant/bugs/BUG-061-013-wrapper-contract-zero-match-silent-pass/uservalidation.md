# User Validation: BUG-061-013

Items describing behaviour that already holds are checked. Items describing behaviour this bug will
deliver are unchecked until a human has run the steps and observed them — no agent may check those.

This packet is newly filed and **no fix is implemented**, so every delivery item below is unchecked.

## Checklist

### [Defect] BUG-061-013 The wrapper contract passes on a locator that matches nothing

- [x] **What:** The anchored regex at `internal/deploy/envsubst_wrapper_contract_test.go:82` finds no
  match in `scripts/runtime/go-integration.sh`, yet the live subtest for that wrapper reports PASS.
  - **Steps:**
    1. `grep -nE '^\s*go\s+test\b' scripts/runtime/go-integration.sh`
    2. `grep -n 'go test' scripts/runtime/go-integration.sh`
    3. `./smackerel.sh test unit --go --go-run 'TestEnvsubstWrapperContract' --verbose`
  - **Expected:** Step 1 exits 1 with no output. Step 2 shows line 76 beginning `if ! go test`.
    Step 3 exits 0 with `--- PASS: TestEnvsubstWrapperContract_LiveWrappers/go-integration.sh`.
  - **Verify:** Observed exactly that at HEAD `4895d446`.
  - **Evidence:** report.md → REPRO-1, REPRO-2, REPRO-3
  - **Notes:** This bounds the fix. The wrapper is correct and the runtime ordering genuinely holds
    (`ensure_envsubst` line 14 precedes `go test` line 76). The implementing agent must not "fix"
    this by rewriting the wrapper.

- [x] **What:** No user-facing behaviour is affected. This is a test-effectiveness defect only.
  - **Steps:**
    1. `grep -n 'ensure_envsubst' scripts/runtime/go-integration.sh`
  - **Expected:** `ensure_envsubst "go-integration"` appears at line 14, before the invocation at
    line 76 — so the ordering the contract asserts is in fact satisfied today.
  - **Verify:** Observed at HEAD `4895d446`.
  - **Evidence:** report.md → REPRO-2
  - **Notes:** Recorded explicitly so severity is not inflated during the fix. Nothing is broken for
    a user; the detector is what cannot fail.

### [Bug Fix] BUG-061-013 A locator that finds nothing makes the test RED

- [ ] **What:** A tracked wrapper whose `go test` invocation the matcher cannot locate is REJECTED,
  with an error naming the locator rather than blaming the wrapper.
  - **Steps:**
    1. `./smackerel.sh test unit --go --go-run 'TestEnvsubstWrapperContract_AdversarialRejectsUnlocatableInvocation' --verbose`
  - **Expected:** The fixture is rejected, and the error text says the invocation could not be
    LOCATED and that the matcher may need widening.
  - **Notes:** Also confirm the fixture has teeth — run it against the pre-fix function and observe
    the false GREEN. A fixture that is red only after the fix does not prove it can detect the
    regression.

- [ ] **What:** The conditional-and-piped form actually in use is located, and the ordering check is
  genuinely live again for `go-integration.sh`.
  - **Steps:**
    1. `./smackerel.sh test unit --go --go-run 'TestEnvsubstWrapperContract_LiveWrappers' --verbose`
    2. `./smackerel.sh test unit --go --go-run 'TestEnvsubstWrapperContract_AdversarialRejectsConditionalCallAfterGoTest' --verbose`
  - **Expected:** Step 1 passes for all four wrappers. Step 2 rejects a fixture that writes the
    line-76 form with `ensure_envsubst` moved after it, failing with the ORDERING error rather than
    the locator error.
  - **Notes:** This is the row most likely to hide a bad fix. A widened regex that matches something
    inert would make step 1 green while step 2 still fails to detect inverted ordering — which is
    why the two are checked separately rather than together.

- [ ] **What:** The three pre-existing adversarial sub-tests still reject their fixtures, so the fix
  did not blunt detection that already worked.
  - **Steps:**
    1. `./smackerel.sh test unit --go --go-run 'TestEnvsubstWrapperContract_Adversarial' --verbose`
  - **Expected:** Missing-source, source-without-call, and call-after-go-test are each still
    rejected with their original error substrings.
  - **Notes:** Widening a matcher is the classic way to accidentally relax a neighbouring assertion.

- [ ] **What:** The change is confined to the detector; no runtime script was touched.
  - **Steps:**
    1. `git diff --stat`
    2. `git diff -- scripts/`
  - **Expected:** Step 1 lists exactly `internal/deploy/envsubst_wrapper_contract_test.go`. Step 2 is
    empty.
  - **Notes:** An empty `scripts/` diff is the evidence that the wrapper was not edited to satisfy
    the detector.
