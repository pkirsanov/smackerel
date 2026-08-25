# Scopes: BUG-061-014 — The E2E runners have no SKIP bucket

## Scope 1: SKIP becomes a first-class outcome in both shell E2E classifiers

**Scope ID:** `BUG-061-014-SCOPE-01`
**Status:** [ ] Not started
**Depends On:** none

### Change Boundary

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
| Runner classification | `functional` | new fixture driver under `tests/e2e/` | Drives `tests/e2e/run_all.sh` over synthetic exit-0/77/1 fixtures; asserts SKIP/PASS/FAIL entries, all three tallies, three-way total | `./smackerel.sh test e2e --shell-run <driver>` | No |
| Runner classification | `functional` | new fixture driver under `tests/e2e/` | Drives the `smackerel.sh` shell lane over the same synthetic fixtures; asserts the shell results block, tallies, and that lane exit is not 77 | `./smackerel.sh test e2e --shell-run <driver>` | No |
| Required-set exit rule | `functional` | new fixture driver under `tests/e2e/` | Required skip yields non-zero exit with zero failures; optional skip yields exit 0 | `./smackerel.sh test e2e --shell-run <driver>` | No |
| Negative control | `functional` | same driver | Exit-`1` fixture stays FAIL, counted, non-zero suite exit | `./smackerel.sh test e2e --shell-run <driver>` | No |
| Skip-reason surfacing | `functional` | same driver | `SKIP_REASON` token appears in the results block, not only the interleaved log | `./smackerel.sh test e2e --shell-run <driver>` | No |
| Existing-slot classification | `e2e-api` | the seven `reg_skip_with_blocker` fixtures | Each reports SKIP with its own reason under the corrected classifier | `./smackerel.sh test e2e --shell-run <each>` | Yes |
| Broader lane regression | `e2e-api` | existing shell E2E lane | The default lane's PASS/FAIL classification for non-skipping fixtures is unchanged | `./smackerel.sh test e2e` | Yes |
| Unit lane regression | `unit` | existing Go unit lane | No product code changed; lane stays green | `./smackerel.sh test unit` | No |
| Integration lane regression | `integration` | existing integration lane | No product code changed; lane stays green | `./smackerel.sh test integration` | Yes |
| Lint | `functional` | repository lint surface | Modified shell passes lint clean | `./smackerel.sh lint` | No |
| Format | `functional` | repository format surface | Modified shell passes format check clean | `./smackerel.sh format --check` | No |

### Definition of Done — 3-Part Validation

Each item requires implementation, validated behaviour, and inline raw evidence.

- [ ] Root cause confirmed and documented: the classifiers' result vocabulary has two values and the outcome domain has three
   - Raw output evidence (inline under this item, no references):
      ```
      [ACTUAL terminal/tool output, ≥10 lines]
      ```
- [ ] `run_test` in `tests/e2e/run_all.sh` classifies exit 77 as SKIP with its own counter
   - Raw output evidence (inline under this item, no references):
      ```
      [ACTUAL terminal/tool output, ≥10 lines]
      ```
- [ ] `e2e_record_shell_result` in `smackerel.sh` classifies exit 77 as SKIP and does not propagate 77 into the lane exit status
   - Raw output evidence (inline under this item, no references):
      ```
      [ACTUAL terminal/tool output, ≥10 lines]
      ```
- [ ] A skipped fixture increments neither the passed nor the failed tally, and Total reconciles to the three-way sum
   - Raw output evidence (inline under this item, no references):
      ```
      [ACTUAL terminal/tool output, ≥10 lines]
      ```
- [ ] The results block for a skipped fixture carries its `SKIP_REASON`, and fixture output is still streamed live
   - Raw output evidence (inline under this item, no references):
      ```
      [ACTUAL terminal/tool output, ≥10 lines]
      ```
- [ ] A required-set skip yields non-zero suite exit with zero failures; an optional skip yields exit 0
   - Raw output evidence (inline under this item, no references):
      ```
      [ACTUAL terminal/tool output, ≥10 lines]
      ```
- [ ] Pre-fix regression test FAILS against the unmodified runners
   - Raw output evidence (inline under this item, no references):
      ```
      [ACTUAL failing test output, ≥10 lines]
      ```
- [ ] Adversarial cases ADV-061-014-01 through ADV-061-014-04 exist and each fails if its named wrong fix is applied
   - Raw output evidence (inline under this item, no references):
      ```
      [ACTUAL test/setup evidence showing adversarial input and failing behaviour before the fix]
      ```
- [ ] Post-fix regression test PASSES
   - Raw output evidence (inline under this item, no references):
      ```
      [ACTUAL passing test output, ≥10 lines]
      ```
- [ ] Regression tests contain no silent-pass bailout patterns
   - Raw output evidence (inline under this item, no references):
      ```
      [ACTUAL scan output proving no failure-condition early-return paths]
      ```
- [ ] The seven existing `reg_skip_with_blocker` fixtures each report SKIP with their own reason
   - Raw output evidence (inline under this item, no references):
      ```
      [ACTUAL terminal/tool output, ≥10 lines]
      ```
- [ ] Existing unit, integration, and shell E2E lanes pass with no new failures
   - Raw output evidence (inline under this item, no references):
      ```
      [ACTUAL terminal/tool output, ≥10 lines]
      ```
- [ ] Change Boundary respected; `.github/bubbles/**` untouched
   - Raw output evidence (inline under this item, no references):
      ```
      [ACTUAL git diff --name-only output showing only permitted paths]
      ```
- [ ] Build Quality Gate: zero warnings, lint clean, format clean, artifact lint clean, `docs/Testing.md` records the three-outcome contract
   - Raw output evidence (inline under this item, no references):
      ```
      [ACTUAL terminal/tool output, ≥10 lines]
      ```
- [ ] `bug.md` status updated to Fixed
   - Raw output evidence (inline under this item, no references):
      ```
      [ACTUAL terminal/tool output, ≥10 lines]
      ```

---

## Scope 2: One skip convention, so the false-green half does not survive

**Scope ID:** `BUG-061-014-SCOPE-02`
**Status:** [ ] Not started
**Depends On:** `BUG-061-014-SCOPE-01`

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

Excluded: every path excluded by Scope 1, plus the other functions in
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
| Helper exit contract | `functional` | new fixture under `tests/e2e/` | `skip_unless_accel_tier` exits 77 on `tier=cpu`, returns on `tier=accel`, exits 2 on an unknown tier | `./smackerel.sh test e2e --shell-run <driver>` | No |
| Classification | `functional` | new fixture under `tests/e2e/` | A `tier=cpu` fixture is reported SKIP and excluded from the passed count | `./smackerel.sh test e2e --shell-run <driver>` | No |
| Negative control | `functional` | same driver | Unknown tier still reports FAIL | `./smackerel.sh test e2e --shell-run <driver>` | No |
| Consumer sweep | `e2e-api` | the ten `skip_unless_accel_tier` fixtures | Each reports SKIP on `tier=cpu` | `./smackerel.sh test e2e` | Yes |
| Lint | `functional` | repository lint surface | Modified shell passes lint clean | `./smackerel.sh lint` | No |
| Format | `functional` | repository format surface | Modified shell passes format check clean | `./smackerel.sh format --check` | No |

### Definition of Done — 3-Part Validation

- [ ] `skip_unless_accel_tier` exits 77 on `tier=cpu` and no longer exits 0
   - Raw output evidence (inline under this item, no references):
      ```
      [ACTUAL terminal/tool output, ≥10 lines]
      ```
- [ ] The `accel` return path and the unknown-tier `exit 2` path are unchanged
   - Raw output evidence (inline under this item, no references):
      ```
      [ACTUAL terminal/tool output, ≥10 lines]
      ```
- [ ] All ten consuming fixtures report SKIP on `tier=cpu` and none is counted as passed
   - Raw output evidence (inline under this item, no references):
      ```
      [ACTUAL terminal/tool output, ≥10 lines]
      ```
- [ ] Pre-fix regression test FAILS against the unmodified helper
   - Raw output evidence (inline under this item, no references):
      ```
      [ACTUAL failing test output, ≥10 lines]
      ```
- [ ] Adversarial cases ADV-061-014-05 and ADV-061-014-06 exist and each fails if its named wrong fix is applied
   - Raw output evidence (inline under this item, no references):
      ```
      [ACTUAL test/setup evidence showing adversarial input and failing behaviour before the fix]
      ```
- [ ] Post-fix regression test PASSES
   - Raw output evidence (inline under this item, no references):
      ```
      [ACTUAL passing test output, ≥10 lines]
      ```
- [ ] Regression tests contain no silent-pass bailout patterns
   - Raw output evidence (inline under this item, no references):
      ```
      [ACTUAL scan output proving no failure-condition early-return paths]
      ```
- [ ] The required-versus-optional decision for the ten accel-tier fixtures is recorded with its rationale
   - Raw output evidence (inline under this item, no references):
      ```
      [ACTUAL terminal/tool output, ≥10 lines]
      ```
- [ ] Existing unit, integration, and shell E2E lanes pass with no new failures
   - Raw output evidence (inline under this item, no references):
      ```
      [ACTUAL terminal/tool output, ≥10 lines]
      ```
- [ ] Change Boundary respected; `.github/bubbles/**` untouched
   - Raw output evidence (inline under this item, no references):
      ```
      [ACTUAL git diff --name-only output showing only permitted paths]
      ```
- [ ] Build Quality Gate: zero warnings, lint clean, format clean, artifact lint clean, `docs/Testing.md` aligned
   - Raw output evidence (inline under this item, no references):
      ```
      [ACTUAL terminal/tool output, ≥10 lines]
      ```
