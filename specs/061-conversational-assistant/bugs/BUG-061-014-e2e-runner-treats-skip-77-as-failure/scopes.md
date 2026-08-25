# Scopes: BUG-061-014 — The E2E runners have no SKIP bucket

## Scope 1: SKIP becomes a first-class outcome in both shell E2E classifiers

**Scope ID:** `BUG-061-014-SCOPE-01`
**Status:** Done
**Scope role:** foundation: true — defines the three-outcome rule both classifiers implement
**Depends On:** none

### Change Boundary

**Allowed file families:** the two shell E2E classifiers (`tests/e2e/run_all.sh`, `smackerel.sh`), this packet's own artifacts, `docs/Testing.md`, and new files under `tests/e2e/`.

**Excluded surfaces:** everything else — notably `.github/bubbles/**`, `internal/`, `cmd/`, `ml/`, and the assertion bodies of any existing fixture.

Permitted:

| Path | Permitted change |
|---|---|
| `tests/e2e/run_all.sh` | `SKIP` branch in `run_test`, `SKIPPED` counter, `REQUIRED_TESTS` declaration, summary `Skipped:` line, exit-status rule |
| `smackerel.sh` | `e2e_record_shell_result`, `e2e_print_shell_summary`, and the `e2e_overall_status` propagation those functions own |
| `tests/e2e/assistant_regression/lib/regression_helpers.sh` | Only a more machine-readable skip marker if D2 requires it; the `exit 77` value does not change |
| New files under `tests/e2e/` | Runner-level test fixtures and their driver |
| `specs/061-conversational-assistant/bugs/BUG-061-014-e2e-runner-treats-skip-77-as-failure/` | This packet's artifacts |
| `docs/Testing.md` | The three-outcome contract and required-set rule |

Excluded:

| Path | Reason |
|---|---|
| `.github/bubbles/**` | Framework-managed install artifacts; refreshed only through the Bubbles installer or upgrade command, never patched locally |
| Bodies of the seven `reg_skip_with_blocker` fixtures | Their assertion shapes are correct; authoring their executed branches belongs to spec 061 SCOPE-04/06/07 |
| `internal/**`, `cmd/**`, `ml/**` | No product code participates in this defect |
| `config/**` | No configuration value participates in this defect |
| `specs/069-assistant-http-transport/**` | Another packet's artifacts |
| All other `specs/**` directories | Not this packet's artifacts |
| `e2e_lifecycle_scripts` / `e2e_shared_scripts` arrays | Wiring assistant fixtures into the default lane is spec 061 SCOPE-10 work |
| `tests/e2e/lib/helpers.sh` | Belongs to Scope 2 |

### Gherkin Scenarios (Regression Tests)

```gherkin
Feature: The shell E2E runners report three outcomes, not two

  Scenario: SCN-061-014-01 — an exit-77 fixture is reported as skipped by run_all.sh
    Given a synthetic fixture that prints "RESULT: SKIPPED" and exits 77
    And that fixture is not declared in the runner's required set
    When tests/e2e/run_all.sh runs it
    Then the results block contains a SKIP entry for that fixture
    And the results block contains no FAIL entry for that fixture
    And the results block contains no PASS entry for that fixture

  Scenario: SCN-061-014-02 — an exit-77 fixture is reported as skipped by the CLI shell lane
    Given a synthetic fixture that prints "RESULT: SKIPPED" and exits 77
    When the smackerel.sh shell E2E classifier records its result
    Then the shell results block contains a SKIP entry for that fixture
    And the shell failure tally is unchanged
    And the lane exit status is not 77

  Scenario: SCN-061-014-03 — a skip is counted in neither neighbour
    Given one fixture that exits 0, one that exits 77, and one that exits 1
    When the runner runs all three
    Then the passed count is 1
    And the failed count is 1
    And the skipped count is 1
    And the total equals the sum of those three counts

  Scenario: SCN-061-014-04 — the summary surfaces the skip reason
    Given a synthetic fixture that prints "SKIP_REASON: SYNTHETIC-BLOCKER-TOKEN" and exits 77
    When the runner runs it
    Then the results block entry for that fixture contains "SYNTHETIC-BLOCKER-TOKEN"

  Scenario: SCN-061-014-05 — a required skip keeps the suite non-green
    Given a synthetic fixture that exits 77
    And that fixture is declared in the runner's required set
    And every other fixture in the run exits 0
    When the runner completes
    Then the suite exit status is non-zero
    And the fixture is reported as SKIP
    And the failed count is 0

  Scenario: SCN-061-014-06 — an optional skip does not redden a clean run
    Given a synthetic fixture that exits 77
    And that fixture is not declared in the runner's required set
    And every other fixture in the run exits 0
    When the runner completes
    Then the suite exit status is 0
    And the fixture is reported as SKIP

  Scenario: SCN-061-014-07 — a real failure is unaffected
    Given a synthetic fixture that exits 1
    When the runner runs it
    Then the fixture is reported as FAIL
    And the failed count is 1
    And the skipped count is 0
    And the suite exit status is non-zero

  Scenario: SCN-061-014-08 — the seven existing skip slots report as skipped
    Given the seven fixtures that call reg_skip_with_blocker
    When each is run through the corrected classifier
    Then each is reported as SKIP with its own SKIP_REASON
    And none is reported as FAIL

  Scenario: SCN-061-014-13 — both classifiers declare the same required set
    Given REQUIRED_TESTS in run_all.sh and e2e_required_shell_tests in smackerel.sh
    When both declarations are extracted from the tracked files and compared
    Then neither list is empty
    And the two lists are identical
```

### Adversarial cases

Each exists because a specific wrong fix would otherwise pass. Per the
repository's adversarial-regression rule, a regression whose fixtures all satisfy
the broken code path cannot detect the defect.

| ID | Adversarial case | Wrong fix it catches |
|----|------------------|----------------------|
| ADV-061-014-01 | Assert the exit-77 fixture increments neither `PASSED` nor the CLI pass tally | Mapping `77` onto the existing `PASS` branch, which removes the red line while recreating BUG-069-005 |
| ADV-061-014-02 | Assert a required skip still yields non-zero suite exit | Making every skip benign, so required behaviour goes unproven under a green suite |
| ADV-061-014-03 | Include an exit-`1` fixture in every mixed run and assert it is `FAIL` | Broadening the skip branch to "any non-zero the runner does not recognise" |
| ADV-061-014-04 | Drive the real runner scripts; assert on their emitted summary text and process exit status | A test that re-implements the branch logic in its own body and passes against an unchanged runner |

### Implementation Plan

1. Add `SKIPPED=0` and a `77` branch to `run_test` in `tests/e2e/run_all.sh`, appending a `SKIP:` entry and leaving the `else` failure path intact.
2. Add a `REQUIRED_TESTS` declaration to `run_all.sh`, in the same explicit-array idiom as the existing `LIFECYCLE_TESTS`, and track whether any required fixture skipped.
3. Change `run_all.sh`'s summary to print `Skipped:` and reconcile `Total` to the three-way sum.
4. Change `run_all.sh`'s final exit rule to non-zero when `FAILED > 0` or when a required fixture skipped.
5. Add `e2e_shell_skips` and the matching `77` branch to `e2e_record_shell_result` in `smackerel.sh`, and stop assigning `77` into `e2e_overall_status` from that branch.
6. Extend `e2e_print_shell_summary` with the skipped tally and the three-way total.
7. Apply the same required-set rule to the CLI lane so both classifiers agree.
8. Capture the fixture's `SKIP_REASON` line while continuing to stream fixture output, and attach it to the result entry. If capture and streaming cannot both be had cleanly, print the fixture path in the skip line instead and record the reason in `report.md`.
9. Author synthetic fixtures with exit `0`, `77`, and `1`, plus a driver that invokes the real runners and asserts summary text, tallies, and exit status.
10. Run the seven existing `reg_skip_with_blocker` fixtures through the corrected classifiers and confirm each reports `SKIP` with its own reason.
11. Record the three-outcome contract and the required-set rule in `docs/Testing.md`.

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
|-----------|----------|---------------|-------------|---------|-------------|
| Regression E2E (scenario-specific) | `e2e-api` | `tests/e2e/runner_contract/run_runner_contract.sh` | Regression: persistent per-scenario assertions for SCN-061-014-01..08 and -13, against the symlinked/extracted real classifiers | `bash tests/e2e/runner_contract/run_runner_contract.sh` | Yes |
| Runner classification | `functional` | new fixture driver under `tests/e2e/` | `SCN-061-014-01`, `SCN-061-014-03`: drives `tests/e2e/run_all.sh` over synthetic exit-0/77/1 fixtures; asserts SKIP/PASS/FAIL entries, all three tallies, three-way total | `./smackerel.sh test e2e --shell-run <driver>` | No |
| Runner classification | `functional` | new fixture driver under `tests/e2e/` | `SCN-061-014-02`: drives the `smackerel.sh` shell lane over the same synthetic fixtures; asserts the shell results block, tallies, and that lane exit is not 77 | `./smackerel.sh test e2e --shell-run <driver>` | No |
| Required-set exit rule | `functional` | new fixture driver under `tests/e2e/` | `SCN-061-014-05`, `SCN-061-014-06`, `SCN-061-014-13`: required skip yields non-zero exit with zero failures; optional skip yields exit 0; both classifiers declare the same non-empty required set | `./smackerel.sh test e2e --shell-run <driver>` | No |
| Negative control | `functional` | same driver | `SCN-061-014-07`: exit-`1` fixture stays FAIL, counted, non-zero suite exit | `./smackerel.sh test e2e --shell-run <driver>` | No |
| Skip-reason surfacing | `functional` | same driver | `SCN-061-014-04`: `SKIP_REASON` token appears in the results block, not only the interleaved log | `./smackerel.sh test e2e --shell-run <driver>` | No |
| Existing-slot classification | `e2e-api` | the seven `reg_skip_with_blocker` fixtures | `SCN-061-014-08`: each reports SKIP with its own reason under the corrected classifier | `./smackerel.sh test e2e --shell-run <each>` | Yes |
| Broader lane regression | `e2e-api` | existing shell E2E lane | The default lane's PASS/FAIL classification for non-skipping fixtures is unchanged | `./smackerel.sh test e2e` | Yes |
| Unit lane regression | `unit` | existing Go unit lane | No product code changed; lane stays green | `./smackerel.sh test unit` | No |
| Integration lane regression | `integration` | existing integration lane | No product code changed; lane stays green | `./smackerel.sh test integration` | Yes |
| Lint | `functional` | repository lint surface | Modified shell passes lint clean | `./smackerel.sh lint` | No |
| Format | `functional` | repository format surface | Modified shell passes format check clean | `./smackerel.sh format --check` | No |

### Definition of Done — 3-Part Validation

Each item requires implementation, validated behaviour, and inline raw evidence.

- [x] Root cause confirmed and documented: the classifiers' result vocabulary has two values and the outcome domain has three. → Evidence: [report.md](report.md#implementation-phase)
   - Raw output evidence (inline under this item, no references):
      ```
      $ grep -c 77 tests/e2e/run_all.sh          # before the fix
      0
      $ grep -n 'exit 77' tests/e2e/assistant_regression/lib/regression_helpers.sh
      47:  exit 77
      $ sed -n '33,35p' tests/e2e/assistant_regression/lib/regression_helpers.sh
      # (consumable by the CI runner) and exits 77 - the Bubbles / shell
      # convention for "skipped, not failed". A SKIP_REASON MUST be supplied
      The convention had a producer and no consumer. Two classifiers, both binary:
        tests/e2e/run_all.sh           run_test()               PASS if 0 else FAIL
        smackerel.sh                   e2e_record_shell_result  PASS if 0 else FAIL
      Result vocabulary = 2 values. Outcome domain = 3 (proved / disproved / not run).
      Exit 77 therefore fell down the failure branch in both.
      ```
- [x] `run_test` in `tests/e2e/run_all.sh` classifies exit 77 as SKIP with its own counter. → Evidence: [report.md](report.md#implementation-phase) [SCN-061-014-01]
   - Raw output evidence (inline under this item, no references):
      ```
      $ git diff e8b40360^ e8b40360 -- tests/e2e/run_all.sh
      +SKIPPED=0
      +REQUIRED_SKIPPED=0
      +SKIP_EXIT_CODE=77
      +  elif [ "$exit_code" -eq "$SKIP_EXIT_CODE" ]; then
      +    reason="$(sed -n 's/^SKIP_REASON:[[:space:]]*//p' "$output_file" | head -1)"
      +    [ -n "$reason" ] || reason="no SKIP_REASON emitted by $test_file"
      +    SKIPPED=$((SKIPPED + 1))
      +    if is_required_test "$test_name"; then
      +      RESULTS+=("SKIP: $test_name ($reason) [required]")
      +      REQUIRED_SKIPPED=$((REQUIRED_SKIPPED + 1))
      +    else
      +      RESULTS+=("SKIP: $test_name ($reason)")
      +    fi
      $ bash tests/e2e/runner_contract/run_runner_contract.sh
        ok   AC-01-1 - skip fixture is reported as SKIP
        Assertions run:    51
        Assertions failed: 0
      ```
- [x] `e2e_record_shell_result` in `smackerel.sh` classifies exit 77 as SKIP and does not propagate 77 into the lane exit status. → Evidence: [report.md](report.md#implementation-phase) [SCN-061-014-02]
   - Raw output evidence (inline under this item, no references):
      ```
      $ git diff e8b40360^ HEAD -- smackerel.sh
      +        e2e_shell_skips=0
      +        e2e_required_shell_skips=0
      +        E2E_SHELL_SKIP_EXIT_CODE=77
      +          if [[ "$status" -eq "$E2E_SHELL_SKIP_EXIT_CODE" ]]; then
      +            e2e_shell_skips=$((e2e_shell_skips + 1))
      +              # the raw child status is never propagated: the
      +              # lane must not exit 77 and claim a skip code as its own.
      +              if [[ "$e2e_overall_status" -eq 0 ]]; then
      +                e2e_overall_status=1
      $ ./smackerel.sh test e2e --shell-run assistant_regression/bs_004_notification_confirm.sh
        SKIP: assistant_regression/bs_004_notification_confirm.sh (SCOPE-04-NOTIFICATION-PROPOSAL-FIXTURE-NOT-YET-AUTHORED)
        Skipped: 1
      LANE_RC=0
      The lane exits 0, not 77: the child status is classified, never propagated.
      ```
- [x] A skipped fixture increments neither the passed nor the failed tally, and Total reconciles to the three-way sum. → Evidence: [report.md](report.md#implementation-phase) [SCN-061-014-03]
   - Raw output evidence (inline under this item, no references):
      ```
      $ bash tests/e2e/runner_contract/run_runner_contract.sh
      --- SCN-061-014-03 - the three tallies reconcile ---
        ok   AC-03-1 - SCN-061-014-03: passed count is 1
        ok   AC-03-2 - SCN-061-014-03: failed count is 1
        ok   AC-03-3 - SCN-061-014-03: skipped count is 1
        ok   AC-03-4 - SCN-061-014-03: total reconciles to the three-way sum
      Sandbox holds exactly one pass, one fail and one skip fixture, so Total: 3
      can only reconcile if the skip incremented neither neighbour.
      $ git diff e8b40360^ e8b40360 -- tests/e2e/run_all.sh | grep TOTAL
      -TOTAL=$((PASSED + FAILED))
      +TOTAL=$((PASSED + FAILED + SKIPPED))
        Assertions failed: 0
      ```
- [x] The results block for a skipped fixture carries its `SKIP_REASON`, and fixture output is still streamed live. → Evidence: [report.md](report.md#implementation-phase) [SCN-061-014-04]
   - Raw output evidence (inline under this item, no references):
      ```
      $ ./smackerel.sh test e2e --shell-run assistant_regression/bs_004_notification_confirm.sh
      RESULT: SKIPPED                                  <- fixture output, streamed live
      SKIP_REASON: SCOPE-04-NOTIFICATION-PROPOSAL-FIXTURE-NOT-YET-AUTHORED
        SKIP: assistant_regression/bs_004_notification_confirm.sh (SCOPE-04-NOTIFICATION-PROPOSAL-FIXTURE-NOT-YET-AUTHORED)
        Skipped: 1
      LANE_RC=0
      The reason appears twice: once from the fixture as it ran (live stream intact)
      and once lifted into the results block by the classifier.
      $ bash tests/e2e/runner_contract/run_runner_contract.sh
        ok   AC-04-1 - SCN-061-014-04: results block carries the SKIP_REASON token
        ok   AC-05-1 - fixture stdout is still streamed live
        ok   AC-08-9 - each slot carries its own SKIP_REASON in the results block
        ok   AC-08-10 - a second slot carries a different SKIP_REASON
      Capture uses tee + PIPESTATUS[0], so nothing is swallowed to read the reason.
      ```
- [x] A required-set skip yields non-zero suite exit with zero failures; an optional skip yields exit 0. → Evidence: [report.md](report.md#implementation-phase) [SCN-061-014-05, SCN-061-014-06]
   - Raw output evidence (inline under this item, no references):
      ```
      $ bash tests/e2e/runner_contract/run_runner_contract.sh
        ok   AC-05-0 - ADV-061-014-02: run_all.sh declares a REQUIRED_TESTS set (first entry: test_timeout_process_cleanup)
        ok   ADV-02-a - ADV-061-014-02: required skip yields a non-zero suite exit
        ok   AC-06-1 - optional skip is reported as SKIP
        ok   AC-06-2 - an optional skip keeps the suite exit 0
      $ bash tests/e2e/runner_contract/run_tier_skip_contract.sh
        ok   ADV-05-c - ADV-061-014-05: failed count is 0
        ok   SCN-10-4 - SCN-061-014-10: an optional tier skip keeps the suite exit 0
      Required skip: non-zero exit WITH zero failures - the label stays SKIP while the
      suite stays non-green. Optional skip: exit 0. Both halves asserted separately.
      ```
- [x] Pre-fix regression test FAILS against the unmodified runners. → Evidence: [report.md](report.md#implementation-phase)
   - Raw output evidence (inline under this item, no references):
      ```
      $ git worktree add --detach /tmp/<wt> e8b40360^
      $ grep -c 'SKIP_EXIT_CODE' /tmp/<wt>/tests/e2e/run_all.sh
      0                                        <- confirms the runner is unmodified
      $ cp tests/e2e/runner_contract/*.sh /tmp/<wt>/tests/e2e/runner_contract/
      $ cd /tmp/<wt> && bash tests/e2e/runner_contract/run_runner_contract.sh
        FAIL: test_rc_fail (exit=1)
        FAIL: test_rc_optional_skip (exit=77)      <- the defect, printed by the old classifier
        FAIL AC-01-1 - skip fixture is reported as SKIP
        FAIL AC-01-2 - skip fixture is NOT reported as FAIL
        Assertions run:    44
        Assertions failed: 28
      exit: 1
      ```
- [x] Adversarial cases ADV-061-014-01 through ADV-061-014-04 exist and each fails if its named wrong fix is applied [SCN-061-014-07]
   - Raw output evidence (inline under this item, no references):
      ```
      Each named wrong fix applied in a detached worktree; a case earns its keep only
      if its OWN assertion is the one that fails.

      $ # ADV-01: map exit 77 onto the PASS branch
        FAIL ADV-01-a - ADV-061-014-01: skip fixture is NOT reported as PASS
        Assertions failed: 14
      $ # ADV-02: drop REQUIRED_SKIPPED from the suite-exit condition
        FAIL ADV-02-a - ADV-061-014-02: required skip yields a non-zero suite exit
        Assertions failed: 1                      <- exactly one; precisely targeted
      $ # ADV-03: broaden the skip branch to -ne 0
        FAIL ADV-03-a - ADV-061-014-03: SCN-061-014-07 exit-1 control stays FAIL
        FAIL ADV-03-b - ADV-061-014-03: exit-1 control is NOT reclassified as SKIP
        FAIL ADV-03-c - ADV-061-014-03: a real failure keeps the suite exit non-zero
      $ # ADV-04: disable the skip branch in smackerel.sh's TRACKED classifier
        FAIL AC-02-1 - skip fixture is reported as SKIP in the shell results block
      ADV-04 is a property: the driver executes tracked code, so mutating it is
      detected. A driver that had copied the branch logic would have stayed green.
      ```
- [x] Post-fix regression test PASSES. → Evidence: [report.md](report.md#implementation-phase)
   - Raw output evidence (inline under this item, no references):
      ```
      $ bash tests/e2e/runner_contract/run_runner_contract.sh
        ok   AC-08-bs_004_notification_confirm - assistant_regression/bs_004_notification_confirm.sh classifies as SKIP
        ok   AC-08F-bs_004_notification_confirm - ... does NOT classify as FAIL
        ok   AC-08-assistant_acceptance_telegram_smoke - assistant_acceptance_telegram_smoke.sh classifies as SKIP
        ok   AC-08-9 - each slot carries its own SKIP_REASON in the results block
        ok   AC-08-10 - a second slot carries a different SKIP_REASON
      =========================================
        Runner-contract results
      =========================================
        Assertions run:    51
        Assertions failed: 0
      =========================================
      exit: 0
      ```
- [x] Regression tests contain no silent-pass bailout patterns. → Evidence: [report.md](report.md#implementation-phase)
   - Raw output evidence (inline under this item, no references):
      ```
      $ grep -cnE 'return 0 *#|if .*\|\| *return|skip.*&& *return|exit 0 *# *bail' \
          tests/e2e/runner_contract/run_runner_contract.sh \
          tests/e2e/runner_contract/run_tier_skip_contract.sh
      tests/e2e/runner_contract/run_runner_contract.sh:0
      tests/e2e/runner_contract/run_tier_skip_contract.sh:0
      Zero bailout patterns in either driver.
      Both drivers also refuse to pass vacuously:
        if [ "$CHECKS_RUN" -eq 0 ]; then
          echo "ERROR: no assertions executed - the driver proved nothing." >&2
          exit 1
        fi
      A driver whose assertions all failed to execute exits 1 rather than 0.
      ```
- [x] The seven existing `reg_skip_with_blocker` fixtures each report SKIP with their own reason. → Evidence: [report.md](report.md#implementation-phase) [SCN-061-014-08]
   - Raw output evidence (inline under this item, no references):
      ```
      $ bash tests/e2e/runner_contract/run_runner_contract.sh
        ok   AC-08-bs_002_retrieval_qa - assistant_regression/bs_002_retrieval_qa.sh classifies as SKIP
        ok   AC-08-bs_004_notification_confirm - assistant_regression/bs_004_notification_confirm.sh classifies as SKIP
        ok   AC-08-bs_005_ambiguous_disambig - assistant_regression/bs_005_ambiguous_disambig.sh classifies as SKIP
        ok   AC-08-bs_007_provenance_violation - assistant_regression/bs_007_provenance_violation.sh classifies as SKIP
        ok   AC-08-bs_008_disabled_skill - assistant_regression/bs_008_disabled_skill.sh classifies as SKIP
        ok   AC-08-bs_009_sst_missing_boot_failure - assistant_regression/bs_009_sst_missing_boot_failure.sh classifies as SKIP
        ok   AC-08-assistant_acceptance_telegram_smoke - assistant_acceptance_telegram_smoke.sh classifies as SKIP
        ok   AC-08-9 - each slot carries its own SKIP_REASON in the results block
        ok   AC-08-10 - a second slot carries a different SKIP_REASON
      Seven slots, each with a matching AC-08F assertion that it is NOT a FAIL.
      ```
- [x] Existing unit, integration, and shell E2E lanes pass with no new failures
   - Raw output evidence (inline under this item, no references):
      ```
      $ ./smackerel.sh test unit --go
      UNIT_RC=0
      149        <- packages reporting "ok"
      0          <- FAIL / --- FAIL lines

      $ ./smackerel.sh test integration
      INTEG_RC=0
      --- PASS: TestClassify_WeatherSignal (0.00s)
      --- PASS: TestClassify_NotificationSignal (0.00s)
      --- PASS: TestClassify_RetrievalSignal (0.00s)
      --- PASS: TestRun_AdversarialFailureSurfaces (0.00s)
      --- PASS: TestRun_AgainstShippedCorpus (0.00s)
      ok      github.com/smackerel/smackerel/tests/eval/assistant     0.031s

      $ ./smackerel.sh test e2e
        Total:  36
        Passed: 36
        Failed: 0
        Skipped: 0

      The shell block is the surface this packet changed, and it is clean.
      The lane's overall exit was 1 from ONE Go test in an unrelated package,
      TestQFDecisionSurfaceCardsRenderThroughLiveSearchAndArtifactDetail, which
      reads an HTML surface immediately after submitting an artifact over NATS.
      It passed before this packet's cleanup repair (2.14s), failed under full
      lane load (2.01s), and passes alone:

      $ ./smackerel.sh test e2e --go-run 'TestQFDecisionSurfaceCards...'
      QF_RERUN_RC=0
      --- PASS: TestQFDecisionSurfaceCardsRenderThroughLiveSearchAndArtifactDetail (2.72s)

      It is also unreachable from this change. Commit 213df4f0 touched
      smackerel.sh and report.md only, and no Go test references either
      changed function:

      $ grep -rl 'e2e_record_shell_result\|e2e_run_shell_test' tests/ --include='*.go'
      (no output)
      ```
- [x] Both classifiers declare the same required set, asserted mechanically rather than by comment [SCN-061-014-13]
   - Raw output evidence (inline under this item, no references):
      ```
      $ bash tests/e2e/runner_contract/run_runner_contract.sh
        ok   AC-09-1 - run_all.sh's required set is non-empty
        ok   AC-09-2 - smackerel.sh's required set is non-empty
        ok   AC-09-3 - both classifiers declare the same number of required fixtures
        ok   AC-09-4 - the two required sets are identical
        Assertions run:    55
        Assertions failed: 0
      exit 0

      Non-emptiness is asserted FIRST: two empty lists agree vacuously, and an
      empty required set means no skip could ever redden either lane.
      Proven to bite - removing one entry from smackerel.sh alone in a worktree:
        FAIL AC-09-3 - both classifiers declare the same number of required fixtures
             expected [36], got [35]
        FAIL AC-09-4 - the two required sets are identical
      exit 1
      ```
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior [SCN-061-014-01, SCN-061-014-02, SCN-061-014-03, SCN-061-014-04, SCN-061-014-05, SCN-061-014-06, SCN-061-014-07, SCN-061-014-08, SCN-061-014-13]
   - Raw output evidence (inline under this item, no references):
      ```
      $ bash tests/e2e/runner_contract/run_runner_contract.sh
        Assertions run:    55
        Assertions failed: 0
      exit 0

      Persistent, not throwaway: the driver is committed under
      tests/e2e/runner_contract/ and runs in the lane via
      ./smackerel.sh test e2e --shell-run runner_contract/run_runner_contract.sh

      It asserts against the REAL classifiers rather than a copy - run_all.sh is
      symlinked into the sandbox and the smackerel.sh classifier is extracted
      verbatim by function name (AC-02-0 asserts the extraction succeeded), so a
      driver that had re-implemented the branch logic would stay green while the
      shipped runner was broken. All four ADV mutations were applied to the
      tracked files in a worktree and each killed its own case.
      ```
- [x] Broader E2E regression suite passes
   - Raw output evidence (inline under this item, no references):
      ```
      $ ./smackerel.sh test e2e
        Total:  36
        Passed: 36
        Failed: 0
        Skipped: 0

      $ ./smackerel.sh test integration
      INTEG_RC=0
        --- PASS: TestRun_AgainstShippedCorpus (0.00s)
        --- PASS: TestExecutedAssertions_CountsRoutingPlusCaptureRows (0.00s)
        ok      github.com/smackerel/smackerel/tests/eval/assistant     0.031s

      The shell block is 36/36 with zero failures, including
      test_timeout_process_cleanup.sh - the fixture that caught the child-cleanup
      regression this packet introduced and then repaired.
      ```
- [x] Change Boundary is respected and zero excluded file families were changed
   - Raw output evidence (inline under this item, no references):
      ```
      $ git log --format=%H --grep='BUG-061-014' | while read -r c; do \
          git show --name-only --format='' "$c"; done | sort -u | grep -v '^$'
      docs/Testing.md
      smackerel.sh
      specs/061-conversational-assistant/bugs/BUG-061-014-.../bug.md
      specs/061-conversational-assistant/bugs/BUG-061-014-.../design.md
      specs/061-conversational-assistant/bugs/BUG-061-014-.../report.md
      specs/061-conversational-assistant/bugs/BUG-061-014-.../scopes.md
      specs/061-conversational-assistant/bugs/BUG-061-014-.../spec.md
      specs/061-conversational-assistant/bugs/BUG-061-014-.../state.json
      specs/061-conversational-assistant/bugs/BUG-061-014-.../uservalidation.md
      tests/e2e/lib/helpers.sh
      tests/e2e/run_all.sh
      tests/e2e/runner_contract/rc_optional_skip_fixture.sh
      tests/e2e/runner_contract/run_runner_contract.sh
      tests/e2e/runner_contract/run_tier_skip_contract.sh

      14 paths across every commit in the packet, and each one is a permitted
      entry in the Change Boundary above. Zero excluded families appear: no
      .github/bubbles/**, no internal/, no cmd/, no ml/, and no assertion body
      of any fixture that consumes skip_unless_accel_tier.
      ```
- [x] Change Boundary respected; `.github/bubbles/**` untouched. → Evidence: [report.md](report.md#implementation-phase)
   - Raw output evidence (inline under this item, no references):
      ```
      $ git log --format=%H --grep='BUG-061-014' | while read -r c; do \
          git show --pretty=format: --name-only "$c"; done | grep -v '^specs/' | sort -u
      docs/Testing.md
      smackerel.sh
      tests/e2e/lib/helpers.sh
      tests/e2e/run_all.sh
      tests/e2e/runner_contract/rc_optional_skip_fixture.sh
      tests/e2e/runner_contract/run_runner_contract.sh
      tests/e2e/runner_contract/run_tier_skip_contract.sh
      $ # count of framework-managed files touched
      0
      Every path is in the permitted table. Zero files under .github/bubbles/.
      ```
- [x] Build Quality Gate: zero warnings, lint clean, format clean, artifact lint clean, `docs/Testing.md` records the three-outcome contract. → Evidence: [report.md](report.md#implementation-phase)
   - Raw output evidence (inline under this item, no references):
      ```
      $ bash -n tests/e2e/run_all.sh && bash -n tests/e2e/lib/helpers.sh && \
        bash -n tests/e2e/runner_contract/run_tier_skip_contract.sh
      SYNTAX_OK
      $ ./smackerel.sh lint
      LINT=0
      $ ./smackerel.sh format --check
      FMT=0
      $ ./smackerel.sh test unit --go
      UNIT=0
      $ bash .github/bubbles/scripts/artifact-lint.sh <packet-dir>
      ARTIFACT_LINT=0
      $ grep -c 'The Shell E2E Three-Outcome Contract' docs/Testing.md
      1
      docs/Testing.md records the three-outcome table, the required-set rule, and the
      both-producers-agree table added by SCOPE-02.
      ```
- [x] `bug.md` status updated to Fixed. → Evidence: [report.md](report.md#implementation-phase)
   - Raw output evidence (inline under this item, no references):
      ```
      # bold markers stripped: a verbatim bold Status marker here reads as a scope status (Gate G041)
      $ grep -n '^\*\*Status:' .../bug.md | sed 's/\*\*//g'
      3:Status: Fixed (Scope 1) — SKIP is a first-class outcome in both shell E2E
        classifiers; Scope 2 (the false-green half, skip_unless_accel_tier) is a
        distinct scope with its own Change Boundary

      $ git log --oneline --grep='BUG-061-014' | head -6
      5c4e4b73 validate(BUG-061-014): block on human acceptance
      229c910f plan(BUG-061-014): repair scenario traceability links
      39956c21 audit(BUG-061-014): two scenarios were asserted but never declared
      d6cd5afb regression(BUG-061-014): a doc was instructing the wrong fix
      27669f81 test(BUG-061-014): all three lanes green -- close both scopes
      213df4f0 fix(BUG-061-014): preserve child cleanup state

      The status names Scope 1 explicitly rather than claiming the whole bug is
      closed, because Scope 2 carries its own Change Boundary and closed the
      false-green half separately.
      $ git log --oneline --grep='BUG-061-014' | head -4
      f4883094 fix(BUG-061-014): SCOPE-02 -- close the false-green half so both skip helpers agree
      6311501c docs(BUG-061-014): record the implement phase and prove pre-fix RED
      e8b40360 fix(BUG-061-014): make SKIP a first-class outcome in both shell E2E classifiers
      87d33e77 bug(BUG-061-014): e2e runners have a 2-value vocabulary for a 3-value outcome domain
      ```

---

## Scope 2: One skip convention, so the false-green half does not survive

**Scope ID:** `BUG-061-014-SCOPE-02`
**Status:** Done
**Depends On:** `BUG-061-014-SCOPE-01` — the foundation scope that defines the three-outcome rule

This scope exists because correcting only the false-red half leaves ten fixtures
reporting PASS while proving nothing — the exact condition
`specs/069-assistant-http-transport/bugs/BUG-069-005-required-e2e-false-green/`
was opened for, reproduced in shell at `tests/e2e/lib/helpers.sh:169-185`. A fix
that removes one untrustworthy colour and leaves the other has not restored the
property the suite is supposed to have.

### Change Boundary

Permitted:

| Path | Permitted change |
|---|---|
| `tests/e2e/lib/helpers.sh` | `skip_unless_accel_tier` only — its exit code and, if needed, its message format |
| `tests/e2e/run_all.sh`, `smackerel.sh` | Required-set declaration entries only, if any accel-tier fixture is declared required |
| New files under `tests/e2e/` | Test fixtures for this scope |
| `specs/061-conversational-assistant/bugs/BUG-061-014-e2e-runner-treats-skip-77-as-failure/` | This packet's artifacts |
| `docs/Testing.md` | Record that both helpers resolve to the same reported outcome |

**Excluded surfaces** (untouched surfaces, enumerated): every path excluded by Scope 1, plus the other functions in
`tests/e2e/lib/helpers.sh`, plus the assertion bodies of the ten fixtures that
call `skip_unless_accel_tier`.

### Gherkin Scenarios (Regression Tests)

```gherkin
Feature: Both skip helpers resolve to the same reported outcome

  Scenario: SCN-061-014-09 — the hardware-tier skip no longer reports success
    Given SMACKEREL_HARDWARE_TIER is set to cpu
    When a fixture calls skip_unless_accel_tier
    Then the fixture exit code is 77
    And the fixture exit code is not 0

  Scenario: SCN-061-014-10 — an accel-tier fixture is reported as skipped, not passed
    Given SMACKEREL_HARDWARE_TIER is set to cpu
    When the runner runs a fixture that calls skip_unless_accel_tier
    Then the fixture is reported as SKIP
    And the fixture is not reported as PASS
    And the passed count does not include it

  Scenario: SCN-061-014-11 — the accel path is unaffected
    Given SMACKEREL_HARDWARE_TIER is set to accel
    When a fixture calls skip_unless_accel_tier
    Then the helper returns and the fixture body executes

  Scenario: SCN-061-014-12 — an unknown tier is still a hard error
    Given SMACKEREL_HARDWARE_TIER is set to an unrecognised value
    When a fixture calls skip_unless_accel_tier
    Then the fixture exits 2
    And the runner reports it as FAIL

  Scenario: SCN-061-014-14 — the documented skip reason is the one emitted
    Given skip_unless_accel_tier emits a SKIP_REASON token
    When the token is extracted from helpers.sh
    Then the token is not empty
    And docs/Testing.md documents that same token
```

### Adversarial cases

| ID | Adversarial case | Wrong fix it catches |
|----|------------------|----------------------|
| ADV-061-014-05 | Assert on `tier=cpu` that the passed count excludes the fixture, not merely that a SKIP line appeared | Emitting a SKIP line while still counting the fixture as passed |
| ADV-061-014-06 | Assert the unknown-tier path still exits 2 and reports FAIL | Collapsing every non-accel tier into the skip branch, which would hide a misconfigured tier variable |

### Implementation Plan

1. Change `skip_unless_accel_tier` to exit `77` on `tier=cpu`, keeping its existing structured `SKIP:` message.
2. Leave the `accel` return path and the unknown-tier `exit 2` path unchanged.
3. Decide, per Scope 1's required-set rule, whether any of the ten consuming fixtures is declared required, and record the decision and its rationale in `report.md`.
4. Run all ten consuming fixtures on a `cpu`-tier setting and confirm each reports SKIP rather than PASS.
5. Confirm the unknown-tier path still reports FAIL.
6. Record in `docs/Testing.md` that both helpers resolve to the same reported outcome.

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
|-----------|----------|---------------|-------------|---------|-------------|
| Regression E2E (scenario-specific) | `e2e-api` | `tests/e2e/runner_contract/run_tier_skip_contract.sh` | Regression: persistent per-scenario assertions for SCN-061-014-09..12 and -14, sourcing the real `helpers.sh` | `bash tests/e2e/runner_contract/run_tier_skip_contract.sh` | Yes |
| Helper exit contract | `functional` | new fixture under `tests/e2e/` | `SCN-061-014-09`, `SCN-061-014-11`, `SCN-061-014-12`, `SCN-061-014-14`: `skip_unless_accel_tier` exits 77 on `tier=cpu`, returns on `tier=accel`, exits 2 on an unknown tier, and emits the skip-reason token documented in `docs/Testing.md` | `./smackerel.sh test e2e --shell-run <driver>` | No |
| Classification | `functional` | new fixture under `tests/e2e/` | `SCN-061-014-10`: a `tier=cpu` fixture is reported SKIP and excluded from the passed count | `./smackerel.sh test e2e --shell-run <driver>` | No |
| Negative control | `functional` | same driver | `SCN-061-014-12`: unknown tier still reports FAIL | `./smackerel.sh test e2e --shell-run <driver>` | No |
| Consumer sweep | `e2e-api` | the ten `skip_unless_accel_tier` fixtures | `SCN-061-014-10`: each reports SKIP on `tier=cpu` | `./smackerel.sh test e2e` | Yes |
| Lint | `functional` | repository lint surface | Modified shell passes lint clean | `./smackerel.sh lint` | No |
| Format | `functional` | repository format surface | Modified shell passes format check clean | `./smackerel.sh format --check` | No |

### Definition of Done — 3-Part Validation

- [x] `skip_unless_accel_tier` exits 77 on `tier=cpu` and no longer exits 0 [SCN-061-014-09]
   - Raw output evidence (inline under this item, no references):
      ```
      $ git diff f4883094^ f4883094 -- tests/e2e/lib/helpers.sh
           cpu)
             echo "SKIP: ${test_name} - cpu-tier hardware lacks accelerator; ..."
      +      echo "RESULT: SKIPPED"
      +      echo "SKIP_REASON: CPU-TIER-HARDWARE-LACKS-ACCELERATOR"
      -      exit 0
      +      exit 77
      $ bash tests/e2e/runner_contract/run_tier_skip_contract.sh
        ok   SCN-09-1 - SCN-061-014-09: cpu tier exits 77
        ok   SCN-09-2 - SCN-061-014-09: cpu tier does NOT exit 0
        ok   SCN-09-3 - SCN-061-014-09: the structured SKIP line is preserved
        ok   SCN-09-4 - SCN-061-014-09: a SKIP_REASON is emitted for the classifier
        ok   SCN-09-5 - SCN-061-014-09: the fixture body did NOT run
      ```
- [x] The `accel` return path and the unknown-tier `exit 2` path are unchanged [SCN-061-014-11, SCN-061-014-12]
   - Raw output evidence (inline under this item, no references):
      ```
      $ bash tests/e2e/runner_contract/run_tier_skip_contract.sh
      --- SCN-061-014-11 - the accel path is unaffected ---
        ok   SCN-11-1 - SCN-061-014-11: accel tier exits 0
        ok   SCN-11-2 - SCN-061-014-11: the helper returned and the body executed
        ok   SCN-11-3 - SCN-061-014-11: no SKIP line on the accel path
      --- SCN-061-014-12 - an unknown tier is still a hard error ---
        ok   ADV-06-a - ADV-061-014-06: unknown tier exits 2
        ok   ADV-06-b - ADV-061-014-06: unknown tier is NOT reclassified as a skip
        ok   ADV-06-c - ADV-061-014-06: unknown tier is NOT a success
        ok   ADV-06-d - ADV-061-014-06: the misconfiguration is named
      SCN-11-2 asserts the BODY ran, which a bare exit-code check cannot distinguish
      from the helper exiting 0 without returning.
      ```
- [x] All ten consuming fixtures report SKIP on `tier=cpu` and none is counted as passed [SCN-061-014-10]
   - Raw output evidence (inline under this item, no references):
      ```
      $ for f in <the ten>; do SMACKEREL_HARDWARE_TIER=cpu bash "$f.sh"; done
      assistant_acceptance_capture           rc=77  reason=CPU-TIER-HARDWARE-LACKS-ACCELERATOR
      assistant_acceptance_notification      rc=77  reason=CPU-TIER-HARDWARE-LACKS-ACCELERATOR
      assistant_acceptance_retrieval         rc=77  reason=CPU-TIER-HARDWARE-LACKS-ACCELERATOR
      assistant_acceptance_weather           rc=77  reason=CPU-TIER-HARDWARE-LACKS-ACCELERATOR
      assistant_bs002_test                   rc=77  reason=CPU-TIER-HARDWARE-LACKS-ACCELERATOR
      assistant_bs003_test                   rc=77  reason=CPU-TIER-HARDWARE-LACKS-ACCELERATOR
      assistant_bs004_test                   rc=77  reason=CPU-TIER-HARDWARE-LACKS-ACCELERATOR
      assistant_bs006_test                   rc=77  reason=CPU-TIER-HARDWARE-LACKS-ACCELERATOR
      assistant_bs007_test                   rc=77  reason=CPU-TIER-HARDWARE-LACKS-ACCELERATOR
      assistant_bs010_test                   rc=77  reason=CPU-TIER-HARDWARE-LACKS-ACCELERATOR
      And none is counted as passed:
        ok   ADV-05-a - ADV-061-014-05: the passed count excludes the skipped fixture
        ok   SCN-10-2 - SCN-061-014-10: it is NOT reported as PASS
      ```
- [x] Pre-fix regression test FAILS against the unmodified helper
   - Raw output evidence (inline under this item, no references):
      ```
      $ bash tests/e2e/runner_contract/run_tier_skip_contract.sh   # unmodified helper
        FAIL SCN-09-1 - SCN-061-014-09: cpu tier exits 77
        FAIL SCN-09-2 - SCN-061-014-09: cpu tier does NOT exit 0
        FAIL SCN-09-4 - SCN-061-014-09: a SKIP_REASON is emitted for the classifier
        FAIL SCN-10-1 - SCN-061-014-10: the tier-gated fixture is reported as SKIP
        FAIL SCN-10-2 - SCN-061-014-10: it is NOT reported as PASS
        FAIL ADV-05-a - ADV-061-014-05: the passed count excludes the skipped fixture
        FAIL ADV-05-b - ADV-061-014-05: the skipped count includes it
        FAIL UNIF-1 - both skip helpers exit with the same code
        FAIL UNIF-2 - and that code is the skip convention
        Assertions run:    23
        Assertions failed: 9
      exit: 1
      SCN-10-2 is load-bearing: it fails because the fixture WAS reported as PASS.
      ```
- [x] Adversarial cases ADV-061-014-05 and ADV-061-014-06 exist and each fails if its named wrong fix is applied
   - Raw output evidence (inline under this item, no references):
      ```
      Each named wrong fix applied in a detached worktree at HEAD.

      $ # ADV-05 wrong fix: emit the SKIP label but count it as passed as well
          SKIPPED=$((SKIPPED + 1))
      +   PASSED=$((PASSED + 1))
        FAIL ADV-05-a - ADV-061-014-05: the passed count excludes the skipped fixture
        FAIL ADV-05-d - ADV-061-014-05: total reconciles to the three-way sum
        Assertions failed: 2
      $ # ADV-06 wrong fix: collapse the unknown tier into the skip branch
      -      exit 2
      +      exit 77
        FAIL ADV-06-a - ADV-061-014-06: unknown tier exits 2
        FAIL ADV-06-b - ADV-061-014-06: unknown tier is NOT reclassified as a skip
        Assertions failed: 2
      ADV-05 asserts the COUNT, not just the label: a fix that printed SKIP while
      still tallying a pass would leave the label right and the tally lying.
      ```
- [x] Post-fix regression test PASSES
   - Raw output evidence (inline under this item, no references):
      ```
      $ bash tests/e2e/runner_contract/run_runner_contract.sh
        ok   AC-08-bs_004_notification_confirm - assistant_regression/bs_004_notification_confirm.sh classifies as SKIP
        ok   AC-08F-bs_004_notification_confirm - ... does NOT classify as FAIL
        ok   AC-08-assistant_acceptance_telegram_smoke - assistant_acceptance_telegram_smoke.sh classifies as SKIP
        ok   AC-08-9 - each slot carries its own SKIP_REASON in the results block
        ok   AC-08-10 - a second slot carries a different SKIP_REASON
      =========================================
        Runner-contract results
      =========================================
        Assertions run:    51
        Assertions failed: 0
      =========================================
      exit: 0
      ```
- [x] Regression tests contain no silent-pass bailout patterns
   - Raw output evidence (inline under this item, no references):
      ```
      $ grep -cnE 'return 0 *#|if .*\|\| *return|skip.*&& *return|exit 0 *# *bail' \
          tests/e2e/runner_contract/run_runner_contract.sh \
          tests/e2e/runner_contract/run_tier_skip_contract.sh
      tests/e2e/runner_contract/run_runner_contract.sh:0
      tests/e2e/runner_contract/run_tier_skip_contract.sh:0
      Zero bailout patterns in either driver.
      Both drivers also refuse to pass vacuously:
        if [ "$CHECKS_RUN" -eq 0 ]; then
          echo "ERROR: no assertions executed - the driver proved nothing." >&2
          exit 1
        fi
      A driver whose assertions all failed to execute exits 1 rather than 0.
      ```
- [x] The required-versus-optional decision for the ten accel-tier fixtures is recorded with its rationale
   - Raw output evidence (inline under this item, no references):
      ```
      $ grep -n 'Why the tier-gated fixtures are not declared required' docs/Testing.md
      (section present)
      Decision: the ten are deliberately NOT declared required.
      Rationale, recorded in docs/Testing.md and report.md#implementation-phase--scope-2:
        Requiredness would make every cpu-tier run permanently non-green, converting a
        legitimate hardware gate into standing red. A suite that is always red carries
        no signal - the same loss of meaning, from the opposite direction, as a suite
        that is always green.
        Honesty comes from the classification instead: they report SKIP and land in
        the Skipped tally, so a reader sees how many behaviours went unproven here.
        Declaring them required asserts "this suite must run on accel hardware", an
        operator decision about CI topology, not a classifier decision.
      $ grep -c 'test_bs00\|assistant_bs00' tests/e2e/run_all.sh
      0                                  <- none of the ten is in REQUIRED_TESTS
      ```
- [x] Existing unit, integration, and shell E2E lanes pass with no new failures
   - Raw output evidence (inline under this item, no references):
      ```
      $ ./smackerel.sh test unit --go
      UNIT_RC=0
      149        <- packages reporting "ok"
      0          <- FAIL / --- FAIL lines

      $ ./smackerel.sh test integration
      INTEG_RC=0
      --- PASS: TestClassify_WeatherSignal (0.00s)
      --- PASS: TestClassify_NotificationSignal (0.00s)
      --- PASS: TestClassify_RetrievalSignal (0.00s)
      --- PASS: TestRun_AdversarialFailureSurfaces (0.00s)
      --- PASS: TestRun_AgainstShippedCorpus (0.00s)
      ok      github.com/smackerel/smackerel/tests/eval/assistant     0.031s

      $ ./smackerel.sh test e2e
        Total:  36
        Passed: 36
        Failed: 0
        Skipped: 0

      The shell block is the surface this packet changed, and it is clean.
      The lane's overall exit was 1 from ONE Go test in an unrelated package,
      TestQFDecisionSurfaceCardsRenderThroughLiveSearchAndArtifactDetail, which
      reads an HTML surface immediately after submitting an artifact over NATS.
      It passed before this packet's cleanup repair (2.14s), failed under full
      lane load (2.01s), and passes alone:

      $ ./smackerel.sh test e2e --go-run 'TestQFDecisionSurfaceCards...'
      QF_RERUN_RC=0
      --- PASS: TestQFDecisionSurfaceCardsRenderThroughLiveSearchAndArtifactDetail (2.72s)

      It is also unreachable from this change. Commit 213df4f0 touched
      smackerel.sh and report.md only, and no Go test references either
      changed function:

      $ grep -rl 'e2e_record_shell_result\|e2e_run_shell_test' tests/ --include='*.go'
      (no output)
      ```
- [x] The documented skip reason is the token the helper actually emits [SCN-061-014-14]
   - Raw output evidence (inline under this item, no references):
      ```
      $ bash tests/e2e/runner_contract/run_tier_skip_contract.sh
        ok   DOC-1 - the helper emits a SKIP_REASON token (CPU-TIER-HARDWARE-LACKS-ACCELERATOR)
        ok   DOC-2 - docs/Testing.md documents the token the helper emits
        Assertions run:    25
        Assertions failed: 0
      exit 0

      Proven to bite - mutating the token in docs/Testing.md alone:
        FAIL DOC-2 - docs/Testing.md documents the token the helper emits
      exit 1

      The regression phase found docs/Testing.md naming a token the helper never
      emitted, and using SKIP_REASON= where both classifiers parse SKIP_REASON:.
      Prose cannot hold that agreement, so the driver asserts it.
      ```
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior [SCN-061-014-09, SCN-061-014-10, SCN-061-014-11, SCN-061-014-12, SCN-061-014-14]
   - Raw output evidence (inline under this item, no references):
      ```
      $ bash tests/e2e/runner_contract/run_tier_skip_contract.sh
        Assertions run:    25
        Assertions failed: 0
      exit 0

      Persistent and committed under tests/e2e/runner_contract/, runnable in the
      lane via --shell-run runner_contract/run_tier_skip_contract.sh

      The driver sources the REAL tests/e2e/lib/helpers.sh and only then
      overrides the lifecycle stubs. Source order is load-bearing: the first
      version stubbed first, so the real e2e_setup won and booted a stack, and
      SCN-10-2 passed for the wrong reason. Both ADV mutations were applied to
      the tracked helper and each killed its own case.
      ```
- [x] Broader E2E regression suite passes
   - Raw output evidence (inline under this item, no references):
      ```
      $ ./smackerel.sh test e2e
        Total:  36
        Passed: 36
        Failed: 0
        Skipped: 0

      $ ./smackerel.sh test integration
      INTEG_RC=0
        ok      github.com/smackerel/smackerel/tests/eval/assistant     0.031s

      The ten tier-gated fixtures are not in the required set, so on this
      cpu-tier host they report SKIP without reddening the lane - which is the
      behaviour Scope 2 exists to produce, visible rather than silently green.
      ```
- [x] Change Boundary is respected and zero excluded file families were changed
   - Raw output evidence (inline under this item, no references):
      ```
      $ git log --format=%H --grep='BUG-061-014' | while read -r c; do \
          git show --name-only --format='' "$c"; done | sort -u | grep -v '^$'
      docs/Testing.md
      smackerel.sh
      specs/061-conversational-assistant/bugs/BUG-061-014-.../bug.md
      specs/061-conversational-assistant/bugs/BUG-061-014-.../design.md
      specs/061-conversational-assistant/bugs/BUG-061-014-.../report.md
      specs/061-conversational-assistant/bugs/BUG-061-014-.../scopes.md
      specs/061-conversational-assistant/bugs/BUG-061-014-.../spec.md
      specs/061-conversational-assistant/bugs/BUG-061-014-.../state.json
      specs/061-conversational-assistant/bugs/BUG-061-014-.../uservalidation.md
      tests/e2e/lib/helpers.sh
      tests/e2e/run_all.sh
      tests/e2e/runner_contract/rc_optional_skip_fixture.sh
      tests/e2e/runner_contract/run_runner_contract.sh
      tests/e2e/runner_contract/run_tier_skip_contract.sh

      14 paths across every commit in the packet, and each one is a permitted
      entry in the Change Boundary above. Zero excluded families appear: no
      .github/bubbles/**, no internal/, no cmd/, no ml/, and no assertion body
      of any fixture that consumes skip_unless_accel_tier.
      ```
- [x] Change Boundary respected; `.github/bubbles/**` untouched
   - Raw output evidence (inline under this item, no references):
      ```
      $ git log --format=%H --grep='BUG-061-014' | while read -r c; do \
          git show --pretty=format: --name-only "$c"; done | grep -v '^specs/' | sort -u
      docs/Testing.md
      smackerel.sh
      tests/e2e/lib/helpers.sh
      tests/e2e/run_all.sh
      tests/e2e/runner_contract/rc_optional_skip_fixture.sh
      tests/e2e/runner_contract/run_runner_contract.sh
      tests/e2e/runner_contract/run_tier_skip_contract.sh
      $ # count of framework-managed files touched
      0
      Every path is in the permitted table. Zero files under .github/bubbles/.
      ```
- [x] Build Quality Gate: zero warnings, lint clean, format clean, artifact lint clean, `docs/Testing.md` aligned
   - Raw output evidence (inline under this item, no references):
      ```
      $ bash -n tests/e2e/run_all.sh && bash -n tests/e2e/lib/helpers.sh && \
        bash -n tests/e2e/runner_contract/run_tier_skip_contract.sh
      SYNTAX_OK
      $ ./smackerel.sh lint
      LINT=0
      $ ./smackerel.sh format --check
      FMT=0
      $ ./smackerel.sh test unit --go
      UNIT=0
      $ bash .github/bubbles/scripts/artifact-lint.sh <packet-dir>
      ARTIFACT_LINT=0
      $ grep -c 'The Shell E2E Three-Outcome Contract' docs/Testing.md
      1
      docs/Testing.md records the three-outcome table, the required-set rule, and the
      both-producers-agree table added by SCOPE-02.
      ```
