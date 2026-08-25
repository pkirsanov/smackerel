# User Validation: BUG-061-014

Items describing behaviour that already holds are checked, because each was
observed by execution during discovery. Items describing behaviour this bug will
deliver are unchecked until a human has run the steps and observed them — no
agent may check those.

This packet is newly filed and **no fix is implemented**, so every delivery item
below is unchecked.

## Checklist

### [Defect] BUG-061-014 A documented skip is reported as a failure

- [x] **What:** A fixture that prints `RESULT: SKIPPED` and exits `77` is reported by the
  runner as `FAIL`, and the lane exits `77`.
  - **Steps:**
    1. `bash tests/e2e/assistant_regression/bs_004_notification_confirm.sh; echo "exit=$?"`
    2. `sed -n '30,50p' tests/e2e/run_all.sh`
    3. `sed -n '1954,1968p' smackerel.sh`
  - **Expected:** Step 1 prints `RESULT: SKIPPED` with
    `SKIP_REASON: SCOPE-04-NOTIFICATION-PROPOSAL-FIXTURE-NOT-YET-AUTHORED` and exits `77`.
    Steps 2 and 3 each show a two-branch classifier whose `else` treats every non-zero exit
    as a failure, with no branch for `77`.
  - **Verify:** Observed exactly that at HEAD `3bec257660f5c4292d79e67d94391f51f72cdda0`.
  - **Evidence:** report.md → E-1, E-2, E-4
  - **Notes:** This bounds the fix. The fixture and its helper are correct as written; the
    implementing agent must not "fix" this by changing the fixture's exit code.

- [x] **What:** All seven fixtures that use the skip helper behave identically, so this is a
  convention-wide defect rather than one misbehaving file.
  - **Steps:**
    1. `grep -rln reg_skip_with_blocker tests/e2e/`
    2. Run each of the seven call sites and record its exit code.
  - **Expected:** Step 1 lists eight files, one of which is the helper that defines the
    function. Step 2 yields `exit=77` and `RESULT: SKIPPED` for all seven call sites.
  - **Verify:** Observed at the HEAD above.
  - **Evidence:** report.md → E-3, E-7

- [x] **What:** No runner anywhere in the repository reads exit code `77`. The convention has
  a producer and no consumer.
  - **Steps:**
    1. `grep -rn 'exit 77' --include='*.sh' . | grep -v '/.git/'`
  - **Expected:** Only `tests/e2e/assistant_regression/lib/regression_helpers.sh` appears —
    line 47 emitting it and line 70 describing it in a comment. No runner branch matches.
  - **Verify:** Observed at the HEAD above.
  - **Evidence:** report.md → E-5
  - **Notes:** Recorded so the fix is understood as implementing a convention rather than
    repairing one.

- [x] **What:** A second, opposite skip convention exists in the same test tree and is
  reported as a pass.
  - **Steps:**
    1. `grep -n -A18 'skip_unless_accel_tier()' tests/e2e/lib/helpers.sh`
    2. `grep -rln 'skip_unless_accel_tier' tests/`
  - **Expected:** Step 1 shows a structured `SKIP:` message followed by `exit 0` on
    `tier=cpu`. Step 2 lists eleven files, one of which defines the helper.
  - **Verify:** Observed at the HEAD above.
  - **Evidence:** report.md → E-6
  - **Notes:** This is why the packet has two scopes. Correcting only the false-red half
    leaves ten fixtures reporting green while proving nothing, which is the condition
    `BUG-069-005` was opened for.

### [Bug Fix] BUG-061-014 The runner can say "skipped"

- [ ] **What:** A fixture exiting `77` is reported as `SKIP` by both runners, and is counted
  in neither the passed nor the failed tally.
  - **Steps:**
    1. Run the runner-level driver over synthetic fixtures exiting `0`, `77`, and `1`.
    2. Read the summary block emitted by `tests/e2e/run_all.sh`.
    3. Read the shell results block emitted by the `smackerel.sh` e2e lane.
  - **Expected:** Each summary shows one passed, one failed, one skipped, and a `Total` that
    equals the sum of the three. The exit-`77` fixture appears on a `SKIP` line.
  - **Notes:** Also run the driver against the pre-fix runners and observe the exit-`77`
    fixture reported as `FAIL`. A test that is only green after the fix does not prove it
    could have detected the defect.

- [ ] **What:** A skipped fixture is not silently absorbed into the pass count.
  - **Steps:**
    1. Run the driver and assert the passed count excludes the exit-`77` fixture.
  - **Expected:** The passed count is exactly the number of exit-`0` fixtures.
  - **Notes:** This is the row most likely to hide a bad fix. Mapping `77` onto the existing
    `PASS` branch removes the red line and looks like success, while recreating the exact
    false-green failure recorded in `BUG-069-005`. Assert the count, not the presence of a
    `SKIP` string.

- [ ] **What:** A skip in a required fixture keeps the suite's exit status non-zero, while
  still being reported as a skip rather than a failure.
  - **Steps:**
    1. Run the driver with an exit-`77` fixture declared in the runner's required set and
       every other fixture exiting `0`.
    2. Run the same driver with that fixture not declared required.
  - **Expected:** Step 1 exits non-zero with a failed count of `0` and the fixture on a
    `SKIP` line. Step 2 exits `0` with the fixture still on a `SKIP` line.
  - **Notes:** Both halves matter. Reporting it as a skip is what makes the label honest;
    keeping the suite non-green is what stops required behaviour going unproven under a
    green badge.

- [ ] **What:** A real failure is unaffected — exit `1` is still `FAIL`, still counted, still
  drives a non-zero suite exit.
  - **Steps:**
    1. Run the driver and assert the exit-`1` fixture's classification, the failed count, and
       the suite exit status.
  - **Expected:** `FAIL`, failed count `1`, non-zero suite exit.
  - **Notes:** This is the negative control against a fix that broadens the skip branch to
    "any non-zero exit the runner does not recognise".

- [ ] **What:** The summary tells a reader why a fixture skipped, without making them search
  the interleaved run log.
  - **Steps:**
    1. Run the driver over a fixture printing `SKIP_REASON: SYNTHETIC-BLOCKER-TOKEN`.
    2. Read the results block.
  - **Expected:** The results entry for that fixture contains `SYNTHETIC-BLOCKER-TOKEN`, and
    the fixture's own output was still streamed live during the run.
  - **Notes:** Capturing output to extract the reason must not suppress live streaming. If
    both cannot be had cleanly, the fixture path in the skip line is the acceptable weaker
    form, and the reason for taking it belongs in `report.md`.

- [ ] **What:** The seven existing skip slots each report `SKIP` with their own reason.
  - **Steps:**
    1. Run each of the seven `reg_skip_with_blocker` fixtures through the corrected
       classifier.
  - **Expected:** Seven `SKIP` lines, each carrying that fixture's own `SKIP_REASON`. No
    `FAIL` line among them.

- [ ] **What:** The hardware-tier skip stops reporting success, so the false-green half does
  not survive the fix.
  - **Steps:**
    1. With `SMACKEREL_HARDWARE_TIER=cpu`, run a fixture that calls `skip_unless_accel_tier`
       and record its exit code.
    2. Run it through the corrected classifier and assert the passed count excludes it.
    3. With `SMACKEREL_HARDWARE_TIER` set to an unrecognised value, confirm the fixture still
       exits `2` and is reported `FAIL`.
  - **Expected:** Step 1 exits `77`, not `0`. Step 2 shows the fixture on a `SKIP` line and
    absent from the passed count. Step 3 still reports `FAIL`.
  - **Notes:** Step 3 is the control against collapsing every non-`accel` tier into the skip
    branch, which would hide a misconfigured tier variable as a benign skip.

- [ ] **What:** The change is confined to test runners and helpers; no product code and no
  framework-managed file was touched.
  - **Steps:**
    1. `git diff --name-only`
    2. `git diff --name-only -- internal/ cmd/ ml/ config/ .github/bubbles/`
  - **Expected:** Step 1 lists only paths named in this packet's Change Boundary. Step 2 is
    empty.
  - **Notes:** An empty step 2 is the evidence that `.github/bubbles/**` — which is
    framework-managed and refreshed only through the Bubbles installer or upgrade command —
    was left untouched.
