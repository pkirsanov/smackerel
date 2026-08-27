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

- [x] **What:** A tracked wrapper whose `go test` invocation the matcher cannot locate is REJECTED,
  with an error naming the locator rather than blaming the wrapper.
  - **Steps:**
    1. `./smackerel.sh test unit --go --go-run 'TestEnvsubstWrapperContract_AdversarialRejectsUnlocatableInvocation' --verbose`
  - **Expected:** The fixture is rejected, and the error text says the invocation could not be
    LOCATED and that the matcher may need widening.
  - **Notes:** Also confirm the fixture has teeth — run it against the pre-fix function and observe
    the false GREEN. A fixture that is red only after the fix does not prove it can detect the
    regression.

- [x] **What:** The conditional-and-piped form actually in use is located, and the ordering check is
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

- [x] **What:** The three pre-existing adversarial sub-tests still reject their fixtures, so the fix
  did not blunt detection that already worked.
  - **Steps:**
    1. `./smackerel.sh test unit --go --go-run 'TestEnvsubstWrapperContract_Adversarial' --verbose`
  - **Expected:** Missing-source, source-without-call, and call-after-go-test are each still
    rejected with their original error substrings.
  - **Notes:** Widening a matcher is the classic way to accidentally relax a neighbouring assertion.

- [x] **What:** The change is confined to the detector; no runtime script was touched.
  - **Steps:**
    1. `git diff --stat`
    2. `git diff -- scripts/`
  - **Expected:** Step 1 lists exactly `internal/deploy/envsubst_wrapper_contract_test.go`. Step 2 is
    empty.
  - **Notes:** An empty `scripts/` diff is the evidence that the wrapper was not edited to satisfy
    the detector.

## Human Acceptance Record

- acceptedBy: pkirsanov
- acceptedAt: 2026-08-27
- method: external-record
- record: Operator directive in the working session on 2026-08-27, verbatim "human gates approved, check all uservalidations, continue".

### What was observed before the boxes were checked

The four items above were not checked on the strength of the directive alone. Each was
re-executed on 2026-08-27 and the observation is recorded here, because the whole point of
this packet is that a green exit code can mean the selector matched nothing. Named
`--- PASS` lines were therefore read directly rather than the lane's exit status:

```text
$ ./smackerel.sh test unit --go --go-run 'TestEnvsubstWrapperContract' --verbose
--- PASS: TestEnvsubstWrapperContract_HelperExistsAndIsExecutable (0.00s)
--- PASS: TestEnvsubstWrapperContract_LiveWrappers (0.00s)
    --- PASS: TestEnvsubstWrapperContract_LiveWrappers/go-unit.sh (0.00s)
    --- PASS: TestEnvsubstWrapperContract_LiveWrappers/go-integration.sh (0.00s)
    --- PASS: TestEnvsubstWrapperContract_LiveWrappers/go-e2e.sh (0.00s)
    --- PASS: TestEnvsubstWrapperContract_LiveWrappers/go-stress.sh (0.00s)
--- PASS: TestEnvsubstWrapperContract_AdversarialRejectsMissingSource (0.00s)
--- PASS: TestEnvsubstWrapperContract_AdversarialRejectsSourceWithoutCall (0.00s)
--- PASS: TestEnvsubstWrapperContract_AdversarialRejectsCallAfterGoTest (0.00s)
--- PASS: TestEnvsubstWrapperContract_AdversarialRejectsUnlocatableInvocation (0.00s)
--- PASS: TestEnvsubstWrapperContract_AdversarialRejectsConditionalCallAfterGoTest (0.00s)
ok      github.com/smackerel/smackerel/internal/deploy  0.019s
Exit Code: 0
```

Every name the four items depend on is present in that list, so the selector matched what it
was supposed to match. `--- FAIL` count across the run was `0`.

Change confinement was checked against history rather than a working-tree diff, because the
fix is already committed and `git diff` would be trivially empty:

```text
$ git diff 40a9e942^..HEAD -- scripts/runtime/ | wc -l
0
$ git show --name-only --format='' 40a9e942 | grep -v '^specs/'
internal/deploy/envsubst_wrapper_contract_test.go
Exit Code: 0
```

`scripts/runtime/` is byte-identical across the entire range, and the fix commit touched
exactly one file outside the packet's own artifacts: the detector. That is the stronger
form of the original check, since it holds over every commit since the fix rather than only
at the moment the diff was taken.

