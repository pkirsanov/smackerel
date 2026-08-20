# Scopes: BUG-061-013 — Make the wrapper-ordering contract fail when it cannot see the invocation

## Scope 1: Zero-match is a hard failure, and the matcher recognises real shell forms

**Scope ID:** `BUG-061-013-SCOPE-01`
**Status:** Done
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
| T-08 | Regression E2E — `scripts/runtime/go-e2e.sh`, a tracked wrapper this contract governs, still runs end-to-end with `ensure_envsubst` ordered before `go test` | `e2e-api` | `scripts/runtime/go-e2e.sh` | `./smackerel.sh test e2e` | Yes |

`load` is **not** in this plan, and neither is `stress`. The reason is stated rather than assumed:
the change is confined to a `_test.go` file in `internal/deploy` with no production consumer, so
there is no end-user runtime surface for those categories to exercise, and manufacturing one for a
regex change inside a Go contract test would be ceremony rather than coverage.

T-06 and T-08 are the deliberate exceptions, and they are included for one shared reason.
`envsubstTrackedWrappers` names **four** wrappers — `go-unit.sh`, `go-integration.sh`, `go-e2e.sh`,
`go-stress.sh` — and T-03 asserts all four pass. That assertion is **static**: it reads each wrapper
as text and never executes it. A guarantee about a wrapper is worthless if the wrapper it describes
no longer runs, so the lanes that actually execute those wrappers are the live regression surface
for this change. T-07 runs `go-unit.sh`, T-06 runs `go-integration.sh` (the wrapper whose ordering
guarantee is being restored), and T-08 runs `go-e2e.sh`.

`go-stress.sh` remains statically asserted only. That residual is **recorded rather than closed**:
it was never one of the blind wrappers — its bare `go test` form always matched — so running a full
stress lane for symmetry alone would add cost without adding signal. It is written down here so the
gap is visible rather than implied.

### Definition of Done

> **Evidence method.** The two red-then-green items require observing the verdict of
> `assertEnvsubstWrapperContract` on the SAME fixtures on BOTH sides of the change. That was done
> with a **temporary probe file**, `internal/deploy/zz_bug061013_prefix_probe_test.go`, which called
> the real function in the real package and printed its verdict. The probe was run once against the
> unmodified function, once against the fixed function, and **deleted before commit** — it is not
> part of the delivered diff. This is stated plainly because the pre-fix observation cannot come
> from committed code: the committed regressions assert the post-fix behaviour, so running them
> pre-fix would show a failing assertion, not the silent pass itself. The probe records the silent
> pass verbatim.

- [x] SCN-01 — a tracked wrapper whose invocation the matcher cannot locate is REJECTED by `assertEnvsubstWrapperContract`; absence returns a non-nil error rather than falling through to `return nil` (T-01)

  **Claim Source:** executed · **Executed:** YES (this session) · **Exit Code:** 0

  ```
  $ ./smackerel.sh test unit --go --go-run 'TestEnvsubstWrapperContract' --verbose
  === RUN   TestEnvsubstWrapperContract_AdversarialRejectsUnlocatableInvocation
  --- PASS: TestEnvsubstWrapperContract_AdversarialRejectsUnlocatableInvocation (0.00s)
  ok      github.com/smackerel/smackerel/internal/deploy  0.014s
  ```

  The `=== RUN` line is load-bearing, not decoration: it proves the new regression actually
  EXECUTES. A test that is never selected passes vacuously, which is the same failure mode this
  bug is about, so exit 0 alone would not have settled it.

  Source of the branch (`internal/deploy/envsubst_wrapper_contract_test.go`):

  ```go
  goTestIdx := envsubstGoTestRE.FindStringIndex(src)
  if goTestIdx == nil {
      return fmt.Errorf("%s: could not LOCATE a `go test` invocation. ...", wrapperName)
  }
  if goTestIdx[0] < callIdx[0] { ... }
  ```

- [x] SCN-02 — the rejection message states the invocation could not be LOCATED and that the matcher may need widening, so a reader does not waste time auditing a correct wrapper (T-02)

  **Claim Source:** executed · **Executed:** YES (this session) · **Exit Code:** 0

  The test asserts BOTH substrings, so the message cannot drift into blaming the wrapper without
  going red. The real returned string, captured from the post-fix probe:

  ```
  ERROR PROBEVERDICT SCN-01-unlocatable-invocation => RED-REJECTION err="SCN-01-unlocatable-invocation:
  could not LOCATE a `go test` invocation. This wrapper is in envsubstTrackedWrappers precisely
  because it runs `go test`, so a zero match means the matcher is blind — the wrapper is NOT
  necessarily wrong and MUST NOT be rewritten to satisfy the pattern. The matcher may need widening:
  extend envsubstGoTestRE to recognise the invocation form actually in use"
  ```

  (The probe block truncates each lifted line at 300 chars; the full string is the one compiled in
  the source and asserted by the two `strings.Contains` checks.)

- [x] SCN-03 — the matcher locates `scripts/runtime/go-integration.sh:76`, whose invocation begins `if ! go test`, and the `ensure_envsubst` ordering is compared against it (T-03)

  **Claim Source:** executed · **Executed:** YES (this session) · **Exit Code:** 0

  ```
  $ ./smackerel.sh test unit --go --go-run 'TestEnvsubstWrapperContract' --verbose
  === RUN   TestEnvsubstWrapperContract_LiveWrappers
  === RUN   TestEnvsubstWrapperContract_LiveWrappers/go-unit.sh
  === RUN   TestEnvsubstWrapperContract_LiveWrappers/go-integration.sh
  === RUN   TestEnvsubstWrapperContract_LiveWrappers/go-e2e.sh
  === RUN   TestEnvsubstWrapperContract_LiveWrappers/go-stress.sh
  --- PASS: TestEnvsubstWrapperContract_LiveWrappers (0.00s)
      --- PASS: TestEnvsubstWrapperContract_LiveWrappers/go-unit.sh (0.00s)
      --- PASS: TestEnvsubstWrapperContract_LiveWrappers/go-integration.sh (0.00s)
      --- PASS: TestEnvsubstWrapperContract_LiveWrappers/go-e2e.sh (0.00s)
      --- PASS: TestEnvsubstWrapperContract_LiveWrappers/go-stress.sh (0.00s)
  ok      github.com/smackerel/smackerel/internal/deploy  0.014s
  ```

  A green `go-integration.sh` subtest is exactly what the defect produced, so on its own it proves
  nothing. What upgrades it to evidence is the SCN-04 item below: that fixture reports a concrete
  match offset (112) inside the `if ! go test` line, which is only reachable if the matcher landed
  on the real invocation.

- [x] SCN-05 — all four tracked wrappers pass on their true ordering with no assertion weakened; the regex comment at line 79-81 is updated so it does not still claim whitespace is the only permitted prefix (T-03)

  **Claim Source:** executed · **Executed:** YES (this session) · **Exit Code:** 0

  All four subtests PASS in the block above. No existing assertion was removed or relaxed — the
  only edit to existing logic replaced `if goTestIdx != nil && goTestIdx[0] < callIdx[0]` with an
  explicit absence branch plus the unchanged comparison, which is strictly stronger. The stale
  comment was replaced; it now enumerates the permitted prefixes and names the forms still NOT
  matched (`env VAR=x`, `timeout N`, `x=$(…)`), so the matcher's remaining blindness is documented
  rather than implied.

- [x] **ADVERSARIAL — red-then-green for the zero-match branch.** A fixture whose invocation the matcher cannot find MUST make the test go RED. Demonstrated by running the new fixture against the PRE-FIX `assertEnvsubstWrapperContract` and recording the GREEN false pass, then against the post-fix function and recording the RED rejection (T-01)

  **Claim Source:** executed · **Executed:** YES (this session, both sides)

  PRE-FIX (unmodified function at HEAD `d08013e6`) — sha256 `9e2e3abf9acc74bccc1750758bc462cd4c777ee465b9bab9e936014e688569fd`:

  ```
  $ timeout 900 ./smackerel.sh test unit --go --go-run TestZZBug061013PreFixProbe
  exit: 1
  ERROR PROBEVERDICT SCN-01-unlocatable-invocation => GREEN-FALSE-PASS (returned nil)
  ```

  POST-FIX (same fixture, same command) — sha256 `c049764422438dd910fddcff2f6d4ec20d9d9ccecac4309d3efc64945f9ac169`:

  ```
  $ timeout 900 ./smackerel.sh test unit --go --go-run TestZZBug061013PreFixProbe
  exit: 1
  ERROR PROBEVERDICT SCN-01-unlocatable-invocation => RED-REJECTION err="... could not LOCATE a `go test` invocation ..."
  ```

  `nil` → error on identical input. The fixture therefore has teeth; it is not merely red-after-fix.

- [x] **ADVERSARIAL — red-then-green for the restored ordering check.** SCN-04: with the matcher widened, moving `ensure_envsubst` after the line-76 form MUST fail with the ORDERING error. Recorded pre-fix (passes — the defect) and post-fix (fails — the fix). This is the item that proves step 2 widened the matcher onto the real invocation rather than onto something inert (T-04)

  **Claim Source:** executed · **Executed:** YES (this session, both sides)

  PRE-FIX — same capture as above:

  ```
  ERROR PROBEVERDICT SCN-04-conditional-form-with-call-after-go-test => GREEN-FALSE-PASS (returned nil)
  ```

  POST-FIX:

  ```
  ERROR PROBEVERDICT SCN-04-conditional-form-with-call-after-go-test => RED-REJECTION err="SCN-04-conditional-form-with-call-after-go-test:
  `go test` invocation (offset 112) appears BEFORE the `ensure_envsubst` call (offset 195); envsubst
  must be ensured BEFORE any go test runs that may shell out to s..."
  ```

  Two things are proven here that a bare non-nil error would not prove. First, it is the ORDERING
  error, not the locator error — the committed test asserts that specific substring, so a widening
  that matched something inert would surface the locator error and go red. Second, offset 112 falls
  inside the `if ! go test …` line of the fixture, which is the byte range the pre-fix pattern
  could not reach at all.

  ```
  $ ./smackerel.sh test unit --go --go-run 'TestEnvsubstWrapperContract' --verbose
  === RUN   TestEnvsubstWrapperContract_AdversarialRejectsConditionalCallAfterGoTest
  --- PASS: TestEnvsubstWrapperContract_AdversarialRejectsConditionalCallAfterGoTest (0.00s)
  ```

- [x] The three pre-existing adversarial sub-tests still reject their fixtures with their original error substrings — the fix did not blunt existing detection (T-05)

  **Claim Source:** executed · **Executed:** YES (this session) · **Exit Code:** 0

  ```
  === RUN   TestEnvsubstWrapperContract_AdversarialRejectsMissingSource
  --- PASS: TestEnvsubstWrapperContract_AdversarialRejectsMissingSource (0.00s)
  === RUN   TestEnvsubstWrapperContract_AdversarialRejectsSourceWithoutCall
  --- PASS: TestEnvsubstWrapperContract_AdversarialRejectsSourceWithoutCall (0.00s)
  === RUN   TestEnvsubstWrapperContract_AdversarialRejectsCallAfterGoTest
  --- PASS: TestEnvsubstWrapperContract_AdversarialRejectsCallAfterGoTest (0.00s)
  ```

  Their substring assertions (`missing source line`, `never calls`, ``BEFORE the `ensure_envsubst`
  call``) are untouched in the diff, so a pass means each still rejects its fixture for the original
  reason. The third one is the meaningful check: its fixture uses the bare `go test ./...` form, so
  it confirms the widened pattern still matches the narrow shape it always matched.

- [x] The integration lane still runs, so the ordering guarantee being restored still describes a live wrapper (T-06)

  **Claim Source:** executed · **Executed:** YES (this session)

  Captured with `evidence-capture.sh` because the raw lane emits 10,355 lines. **Two lines are
  redacted:** the two `config-validate:` lines named an absolute checkout path under the operator's
  home directory, which this repository forbids committing; `<repo-root>` is substituted. The
  redaction is disclosed rather than silent, because an evidence block that presents itself as
  verbatim while having been edited is exactly the kind of small untruth this packet exists to
  remove. The `sha256` covers the true unedited output, so the `verify` line below re-derives it.

  ```
  # BUG-061-013 T-06: integration lane (the wrapper whose ordering guarantee is restored)
  $ timeout 2400 ./smackerel.sh test integration
  exit: 0
  lines: 10355
  sha256: 579e9a146fa39bc4f20c066cb044800dd1071080469b7fd4eccdb18e75df6ba0
  --- first 20 ---
  oom-preflight: OK — 35186 MB available (need 6000 MB; swap used 1298 MB).
  disk-preflight: OK — C: 56 GB free (need 40 GB), WSL / 464 GB free (need 25 GB).
  config-validate: <repo-root>/config/generated/test.env.tmp.2441050 OK
  Smackerel pre-flight resource check: OK
    RAM  available: 35329 MB (required >= 6000 MB)
    Disk available: 475185 MB / 464.0 GB (required >= 15 GB)
  oom-preflight: OK — 35159 MB available (need 6000 MB; swap used 1296 MB).
  disk-preflight: OK — C: 56 GB free (need 40 GB), WSL / 464 GB free (need 25 GB).
  config-validate: <repo-root>/config/generated/test.env.tmp.2447562 OK
  Smackerel pre-flight resource check: OK
    RAM  available: 35177 MB (required >= 6000 MB)
    Disk available: 475184 MB / 464.0 GB (required >= 15 GB)
  Preparing disposable test stack...
  Building disposable test stack images before up (freshness convention)...
  #0 building with "default" instance using docker driver
  #1 [smackerel-ml internal] load build definition from Dockerfile
  #1 transferring dockerfile: 4.87kB done
  --- failure-shaped lines from the omitted region ---
          ERROR: model envelope validation failed (spec 045 FR-045-002): model envelope exceeded:
          LLM_MODEL="bug-045-fixture-llm-20gib" requires 20480 MiB ... but OLLAMA_MEMORY_LIMIT="8G"
          resolves to 8192 MiB ...
          ERROR: config-generate-time validation failed for env=test (see above)
  --- omitted 10315 line(s); sha256 above covers the full output ---
  --- last 20 ---
   Container smackerel-test-postgres-1  Removed
   Container smackerel-test-intent-compiler-provider-1  Removed
   Container smackerel-test-smackerel-ml-1  Removed
   Container smackerel-test-nats-1  Removed
   Volume smackerel-test-nats-data  Removed
   Volume smackerel-test-ollama-data  Removed
   Volume smackerel-test-postgres-data  Removed
   Network smackerel-test_default  Removed
  ```

  <!-- verify: bash .github/bubbles/scripts/evidence-capture.sh --verify 579e9a146fa39bc4f20c066cb044800dd1071080469b7fd4eccdb18e75df6ba0 -- timeout 2400 ./smackerel.sh test integration -->

  The two `ERROR:` lines the capture lifts out of the omitted region are **not** a lane failure, and
  saying so is not a reassurance — it is checkable. They are emitted by a deliberate negative
  fixture: `tests/integration/config_validate_test.go` rewrites the model name to
  `bug-045-fixture-llm-20gib` (20480 MiB) against an 8G envelope precisely to assert that
  config-validate REJECTS it. A run in which those lines were absent would mean that assertion had
  stopped firing. The lane verdict is the `exit: 0` above, and the teardown block shows the
  disposable stack was removed rather than leaked.

- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior — the behavior this fix changes is a *static* guarantee about `scripts/runtime/go-e2e.sh`, one of the four wrappers in `envsubstTrackedWrappers`; the persistent regression that keeps that guarantee attached to reality is the e2e lane, which EXECUTES that wrapper end-to-end (T-08)

  **Claim Source:** executed — by the ORCHESTRATOR (`bubbles.goal`) in this session; OBSERVED, not
  produced, by `bubbles.test` · **Exit Code:** 0

  ```
  # BUG-061-013 T-08: e2e lane (scripts/runtime/go-e2e.sh)
  $ timeout 3300 ./smackerel.sh test e2e
  exit: 0
  lines: 4528
  sha256: eb5e8fa3c316b1f2212bf96e721d54779b3dc07a2b159513da4f1ab084f6cea4
  --- first 20 ---
  Running isolated lifecycle shell E2E: test_timeout_process_cleanup.sh
  === BUG-031-004-SCN-002: regression detects surviving child work ===
  Detector reported surviving child work: ...
  PASS: BUG-031-004-SCN-002
  === BUG-031-004-SCN-001: E2E interruption terminates child processes ===
  Nested E2E runner returned nonzero after interruption: -1
  PASS: BUG-031-004-SCN-001
  === BUG-031-009-SCN-001/002: interrupted Docker Go runner is reaped before teardown ===
  --- failure-shaped lines from the omitted region ---
  FAIL: Services did not become healthy within 8s
  --- omitted 4488 line(s); sha256 above covers the full output ---
  ```

  <!-- verify: bash .github/bubbles/scripts/evidence-capture.sh --verify eb5e8fa3c316b1f2212bf96e721d54779b3dc07a2b159513da4f1ab084f6cea4 -- timeout 3300 ./smackerel.sh test e2e -->

  **What this item does and does not claim.** It does NOT claim the e2e lane exercises the Go
  contract test — it does not; T-01…T-05 do that, in-process. It claims the narrower thing that is
  actually at stake: SCN-05 asserts all four tracked wrappers pass, but that assertion is **static**
  text analysis, so it would keep passing about a wrapper that no longer runs at all. T-08 is the
  live half for `go-e2e.sh`, exactly as T-07 is for `go-unit.sh` and T-06 for `go-integration.sh`.

  The `FAIL:` line is a deliberately-broken stack, verified in source rather than assumed. That
  string comes from `e2e_wait_healthy` (`tests/e2e/lib/helpers.sh:103`), whose timeout is a
  parameter; the literal `8s` pins it, since every other call site passes `120`. The only
  `e2e_wait_healthy 8` is `tests/e2e/test_postgres_readiness_gate.sh:24` — the `SCN-002-BUG-002-001`
  canary, which runs `smackerel_compose "$TEST_ENV" stop postgres` on purpose and then fails itself
  if the gate *passes*: `if [ "$READINESS_EXIT" -eq 0 ]; then e2e_fail "Readiness gate passed even
  though postgres was stopped"`. Its absence would break the canary, not its presence.

- [x] Broader E2E regression suite passes — the full `./smackerel.sh test e2e` lane exits 0 end-to-end, so nothing elsewhere in the e2e surface regressed while this detector was repaired (T-08)

  **Claim Source:** executed — by the ORCHESTRATOR (`bubbles.goal`) in this session; OBSERVED, not
  produced, by `bubbles.test` · **Exit Code:** 0

  Same run as the item above; the tail window is shown here because it is the tail that carries the
  suite-level verdict, and the two windows are non-overlapping views of one capture:

  ```
  $ timeout 3300 ./smackerel.sh test e2e
  exit: 0
  lines: 4528
  sha256: eb5e8fa3c316b1f2212bf96e721d54779b3dc07a2b159513da4f1ab084f6cea4
  --- last 20 ---
   Container smackerel-test-nats-1  Removed
   Volume smackerel-test-postgres-data  Removed
   Network smackerel-test_default  Removed
  PASS: go-e2e-corpus-enforce
  Running project-scoped test stack teardown (exit cleanup, timeout 180s)...
  ```

  Three things carry the verdict, and the exit code alone is the weakest of them. `exit: 0` is the
  lane result; `PASS: go-e2e-corpus-enforce` is a named terminal check rather than a bare status,
  so the suite reached its end instead of short-circuiting; and the `Removed` lines show the
  disposable stack torn down rather than leaked, which matters because a leaked stack is how a
  later lane inherits state it did not create. Exit 0 is deliberately not leaned on by itself here
  — a lane exiting 0 while a check inside it quietly declines to run is this bug's own failure
  mode.

- [x] The full unit lane exits 0 with no newly failing test attributable to this change (T-07)

  **Claim Source:** executed · **Executed:** YES (this session) · **Exit Code:** 0

  sha256 `b6b3a1afcb6a7d342e539e591c67b1c45fa5d63520a127032f0b53db96d8c3a1`:

  ```
  $ timeout 1800 ./smackerel.sh test unit --go
  exit: 0
  lines: 209
  ...
  ok      github.com/smackerel/smackerel/tests/observability      (cached)
  ok      github.com/smackerel/smackerel/tests/stress/readiness   (cached)
  ok      github.com/smackerel/smackerel/tests/unit/clients       (cached)
  ?       github.com/smackerel/smackerel/web/pwa  [no test files]
  ok      github.com/smackerel/smackerel/web/pwa/tests    (cached)
  + echo '[go-unit] go test ./... finished OK'
  [go-unit] go test ./... finished OK
  ```

- [x] Change Boundary is respected and zero excluded file families were changed — the delivered fix touches only the allowed surface, and the test phase added no source change at all

  **Claim Source:** executed · **Executed:** YES (this session, by `bubbles.test`) · **Exit Code:** 0

  > **Why this item exists, stated plainly rather than left to look like boilerplate.** It was added
  > during the test phase because guard Check 8D flipped from *not applicable* to *applicable*: its
  > trigger is a case-insensitive `\b(refactor|simplify|cleanup|repair|hotspot)\b` scan of the whole
  > scope file, and the T-08 evidence block above quotes the verbatim line
  > `Running project-scoped test stack teardown (exit **cleanup**, timeout 180s)`. The word is
  > inside captured terminal output, not a scope classification — this scope is a detector fix, not
  > a refactor. The evidence was NOT edited to dodge the trigger, because silently reshaping a
  > capture to change a gate's verdict is a worse defect than the one this packet fixes. The item is
  > answered on its merits instead, and it happens to be a claim worth making anyway.

  ```
  $ git status --porcelain
   M specs/061-conversational-assistant/bugs/BUG-061-013-.../report.md
   M specs/061-conversational-assistant/bugs/BUG-061-013-.../scopes.md
   M specs/061-conversational-assistant/bugs/BUG-061-013-.../state.json

  $ git diff --name-only HEAD -- . ':!specs/'
  (end of list)

  $ git diff --stat HEAD -- scripts/runtime/ internal/
  (end of diffstat)

  $ git show --stat --format='%H %s' 40a9e942 --
  40a9e9422954177d7d4fe0554d75e227a6079402 fix(BUG-061-013): a wrapper-contract zero match is now a hard failure
   internal/deploy/envsubst_wrapper_contract_test.go  | 100 ++++++++-
   .../report.md                                      | 242 +++++++++++++++++++++
   .../scopes.md                                      | 236 +++++++++++++++++++-
   3 files changed, 563 insertions(+), 15 deletions(-)

  $ git show --name-only --format='' 40a9e942 -- scripts/runtime/
  (end)
  ```

  Two independent containment claims, each with its own command. **The delivered fix**: commit
  `40a9e942` has exactly one non-artifact path, `internal/deploy/envsubst_wrapper_contract_test.go`
  — the sole surface the Change Boundary allows — and querying that same commit restricted to
  `scripts/runtime/` returns nothing, so not one excluded wrapper was touched. **The test phase**:
  `git diff --name-only HEAD -- . ':!specs/'` returns empty, so this phase changed no code at all;
  the three modified files are its own packet artifacts.

  The `scripts/runtime/` result is the load-bearing half and the reason both queries are run rather
  than just the first. The cheapest way to turn this lane green was never to fix the detector — it
  was to revert `go-integration.sh:76` to a bare `go test`, which the old regex would have matched
  immediately. That edit would have satisfied the contract while re-breaking the BUG-061-011 eval
  gate the conditional form exists to serve, and it would have left the zero-match hole open for
  the next wrapper written in an unanticipated shape. An empty result there is the proof that the
  subject was left alone and the detector was fixed instead.

- [x] `git diff --stat` shows exactly one changed file, `internal/deploy/envsubst_wrapper_contract_test.go`; `scripts/runtime/` is byte-identical to HEAD (spec AC-5)

  **Claim Source:** executed · **Executed:** YES (this session) · **Exit Code:** 0

  ```
  $ git diff --stat
   internal/deploy/envsubst_wrapper_contract_test.go | 100 +++++++++++++++++++++-
   1 file changed, 96 insertions(+), 4 deletions(-)

  $ git diff --stat -- scripts/runtime/
  $ git diff -- scripts/runtime/ | wc -l
  scripts/runtime diff lines: 0

  $ git status --porcelain
   M internal/deploy/envsubst_wrapper_contract_test.go
  ```

  Zero diff lines under `scripts/runtime/` is the load-bearing half. The cheapest way to turn this
  lane green was to revert `go-integration.sh:76` to a bare `go test`, which would have satisfied
  the old regex while re-breaking the eval gate that the conditional form exists to serve and
  leaving the zero-match hole open. The subject was not edited; the detector was.

- [x] Build Quality Gate: `./smackerel.sh lint` and `./smackerel.sh format --check` exit 0 with zero warnings; `bash .github/bubbles/scripts/artifact-lint.sh` on this packet exits 0; no findings left unresolved

  **Claim Source:** executed · **Executed:** YES (this session)

  ```
  $ timeout 1200 ./smackerel.sh lint
  exit: 0
  sha256: 99b1b3b48cd86513f07beda05d11a96c8498ace02b972caa6c8c0f057ec77367
    OK: Extension versions match (1.0.0)
  Web validation passed

  $ timeout 900 ./smackerel.sh format --check
  exit: 0
  sha256: 67a766acc6e1887340721dbace5ce85cb3216d6f68ba380859df43635914630d
  78 files already formatted
  ```

  <!-- ARTIFACT-LINT-EVIDENCE -->

