# Spec: BUG-061-014 — A test runner must be able to say "skipped"

## Problem statement

A test suite has three possible outcomes for a fixture: it proved the behaviour,
it disproved the behaviour, or it did not run. The shell E2E runners in this
repository can express only two. Both `tests/e2e/run_all.sh` (`run_test`) and
`smackerel.sh` (`e2e_record_shell_result`) branch on `exit == 0` and treat
everything else as a failure.

The test tree already depends on the third outcome existing. Seven fixtures call
`reg_skip_with_blocker`, which prints `RESULT: SKIPPED` with a machine-readable
`SKIP_REASON` and exits `77` — described in its own source as "the Bubbles /
shell convention for 'skipped, not failed'". Ten further fixtures call
`skip_unless_accel_tier`, which prints a structured `SKIP:` line and exits `0`.

Because the runner offers no skip bucket, each of those conventions is forced
into a misreport. The first is printed as `FAIL`. The second is printed as
`PASS`. Seventeen fixtures therefore report a colour that does not describe what
they did, and a reader cannot tell from the results block which is which.

## Expected behaviour

### EB-1 — A skip is reported as a skip

When a shell E2E fixture exits `77`, the runner records the outcome as `SKIP`.
It appears in the results block as a skip, not as `PASS` and not as `FAIL`.

### EB-2 — A skip is counted in its own tally

`SKIP` outcomes are counted separately. They are added to neither the passed
count nor the failed count, and the summary prints the skipped count alongside
the other two. `Total` equals passed plus failed plus skipped.

### EB-3 — A skip carries its reason to the summary

The fixture already emits `SKIP_REASON` on stdout. The runner's results block
surfaces that reason next to the fixture name, so a reader scanning the summary
learns why the fixture did not run without searching the interleaved run log.

### EB-4 — Visibility does not weaken

A skipped fixture remains conspicuous. It is never rendered in a way that a
reader or a CI colour indicator could mistake for a fixture that ran and passed.
This requirement is load-bearing: an invisible skip is precisely the condition
`BUG-069-005` was opened for, and the correction here must not create it in the
course of removing a false failure.

### EB-5 — Required slots keep the suite honest

The runner distinguishes required fixtures from optional ones by explicit
declaration. A skip in a required fixture keeps the suite's exit status
non-zero, because required behaviour went unproven. A skip in an optional
fixture does not, because nothing that was promised went unproven.

In both cases the fixture is reported as `SKIP` and is excluded from the failure
count. Exit status and result classification are separate concerns: the exit
status answers "may this run be treated as a clean run", and the classification
answers "what did this fixture do".

### EB-6 — Real failures still fail

A fixture exiting non-zero for any value other than `77` is a failure, is
counted in the failure tally, and drives the suite's exit status non-zero. No
change to that path.

### EB-7 — One convention, not two

The two skip helpers resolve to the same reported outcome. `skip_unless_accel_tier`
stops signalling a skip through `exit 0`, which today is indistinguishable from
a pass, and adopts the same structured skip exit as `reg_skip_with_blocker`.

### EB-8 — Both classifiers agree

`tests/e2e/run_all.sh` and `smackerel.sh` produce the same classification for the
same fixture exit code. Two independent classifiers with divergent rules is how a
single fixture comes to have two different reported outcomes depending on which
entry point invoked it.

## Acceptance criteria

| ID | Criterion | Verification |
|----|-----------|--------------|
| AC-1 | A fixture exiting `77` is recorded `SKIP` by `run_all.sh` | Runner-level test over a synthetic exit-77 fixture |
| AC-2 | A fixture exiting `77` is recorded `SKIP` by `smackerel.sh`'s shell lane | Runner-level test over a synthetic exit-77 fixture |
| AC-3 | A skipped fixture increments neither `PASSED` nor `FAILED` | Assert all three tallies from a mixed pass/fail/skip run |
| AC-4 | The summary prints a `Skipped` tally and `Total` reconciles to the sum of the three | Assert the summary block text |
| AC-5 | The results line for a skipped fixture carries its `SKIP_REASON` | Assert the reason token appears in the results block, not only in the run log |
| AC-6 | A skip in a required fixture yields a non-zero suite exit status | Runner-level test with the fixture declared required |
| AC-7 | A skip in an optional fixture yields exit `0` when nothing else failed | Runner-level test with the fixture declared optional |
| AC-8 | A fixture exiting `1` is still `FAIL`, still counted, still non-zero suite exit | Negative control against the exit-77 path |
| AC-9 | `skip_unless_accel_tier` no longer exits `0` on `tier=cpu`, and its fixtures report `SKIP` | Assert the helper's exit code and one consuming fixture's classification |
| AC-10 | The seven existing `reg_skip_with_blocker` fixtures report `SKIP`, each with its reason | Run each fixture through the corrected classifier and assert the reported outcome |

## Adversarial requirements

These exist because the obvious wrong fix — mapping `77` to the existing pass
path — satisfies a naive reading of EB-1 while recreating BUG-069-005.

| ID | Requirement |
|----|-------------|
| ADV-1 | A test asserts that a skipped fixture is **not** counted as passed. A fix that maps `77` onto the `PASS` branch must fail this test. |
| ADV-2 | A test asserts that a required skip produces a non-zero suite exit. A fix that makes all skips benign must fail this test. |
| ADV-3 | A test asserts that exit `1` is still classified `FAIL`. A fix that broadens the skip branch to any non-zero exit must fail this test. |
| ADV-4 | The runner-level tests drive the real classifier code path, not a re-implementation of it. A test that reproduces the branch logic inside itself proves nothing about the runner. |

## Non-goals

These are named so the boundary is unambiguous, and each is owned elsewhere.

- **Authoring the executed branch of any skip-77 fixture.** The six required BS
  slots skip because their substrate — graph seeding, the notification-proposal
  fixture, the LLM no-source stub, the manifest hot-flip harness, the boot-failure
  harness — does not exist. That substrate is spec 061 SCOPE-04/06/07 work with
  its own boundary. This packet changes how a skip is reported, not whether the
  fixture can run.
- **Closing `DIS-069-006-4`.** `BUG-069-006` is blocked on the confirm-flow
  regression suite. Correcting the classifier stops that suite being printed as a
  failure; it does not make it exercise the confirm flow. Which of those the DoD
  item requires is a decision owned by that packet.
- **Wiring the assistant regression fixtures into the default e2e lane.** They are
  absent from `smackerel.sh`'s lane arrays and from `run_all.sh`'s default glob.
  Adding them is spec 061 SCOPE-10 work. This packet makes that addition safe by
  ensuring a documented skip is not reported as a failure when it happens.
- **The Go lane's `t.Skipf` behaviour.** That is `BUG-069-005`, which is already
  filed and blocked.

## Principle this restores

A test suite's report is a claim about what was proven. `PASS` claims the
behaviour was demonstrated. `FAIL` claims it was contradicted. A run that did
neither must say so in its own words, because folding it into either neighbour
converts an honest gap into a false statement — optimistic in one direction,
alarmist in the other. Both destroy the same property: that the colour of the
suite can be trusted without reading it.
