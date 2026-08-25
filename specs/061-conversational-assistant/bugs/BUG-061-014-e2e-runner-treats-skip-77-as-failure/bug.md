# BUG-061-014 — The E2E runners have no SKIP bucket, so every skip is reported as a lie

**Status:** Fixed (Scope 1) — SKIP is a first-class outcome in both shell E2E classifiers; Scope 2 (the false-green half, `skip_unless_accel_tier`) is a separate scope with its own Change Boundary
**Severity:** S2 — the suite's reported colour does not correspond to what the suite proved
**Parent Spec:** `061-conversational-assistant`
**Release Train:** `mvp`
**Discovered:** 2026-08-25 on exact revision `3bec257660f5c4292d79e67d94391f51f72cdda0` (working tree clean)
**Filed by:** `bubbles.bug`, discovery-only invocation

## Identifier note

This packet was requested as `BUG-061-011`. That identifier is occupied by
`BUG-061-011-eval-gate-runs-in-no-automated-lane` (status `blocked`), which is
referenced by 11 files across `specs/061-conversational-assistant/` and
`specs/064-open-ended-knowledge-agent/`. Reusing the number would make every one
of those references ambiguous. The highest identifier under spec 061 is `013`,
so this packet takes `BUG-061-014` and keeps the requested descriptive slug.

## Summary

The shell E2E test tree has a documented skip convention: `reg_skip_with_blocker()`
in `tests/e2e/assistant_regression/lib/regression_helpers.sh` prints a structured
skip record and exits `77`. Its own comment calls this "the Bubbles / shell
convention for 'skipped, not failed'", and its NOTE states that the persistent
regression slot "is honored on every CI run so the skip itself is visible."

No runner in this repository implements that convention. Both classifiers are
binary — a result is either exit 0 or a failure — so a deliberate, documented,
structured skip is counted and printed as a FAILURE.

The deeper defect is not one missing `elif`. **The runners' result vocabulary has
two values while the outcome domain of a test suite has three.** Because there is
no third bucket, every author of a skippable fixture must pick an exit code that
the runner will misreport, and this repository contains both wrong answers at once:

| Convention | Helper | Exit code | Runner reports | Fixtures |
|---|---|---|---|---|
| Structured blocker skip | `reg_skip_with_blocker` | `77` | **FAIL** — false red | 7 |
| Hardware-tier skip | `skip_unless_accel_tier` | `0` | **PASS** — false green | 10 |

Seventeen fixtures currently report a colour that does not describe what they did.

## Findings

| ID | Finding | Classification |
|----|---------|----------------|
| F-BUG061014-01 | `tests/e2e/run_all.sh:41-47` (`run_test`) classifies every non-zero exit as `FAIL`, incrementing `FAILED`. There is no `77` branch. | Runner classification defect |
| F-BUG061014-02 | `smackerel.sh:1954-1968` (`e2e_record_shell_result`) is a second, independent binary classifier with the same shape, and additionally propagates the raw child status into `e2e_overall_status`, so the lane exits `77`. | Runner classification defect (duplicate) |
| F-BUG061014-03 | The `77` convention has a producer and no consumer. Outside `.github/bubbles/**` (framework-managed) the only `77` references in the repository are the helper that emits it and fixture comments that describe it. Nothing reads it. | Convention asserted, never implemented |
| F-BUG061014-04 | `tests/e2e/lib/helpers.sh:169-185` (`skip_unless_accel_tier`) resolves the same missing bucket the opposite way: it prints a structured `SKIP:` line and exits `0`, so 10 fixtures are reported as PASS while proving nothing. | Same root cause, false-green half |
| F-BUG061014-05 | Six of spec 061 SCOPE-10 DoD #7's ten required per-BS regression slots (BS-002, BS-004, BS-005, BS-007, BS-008, BS-009) are `exit 77` today, so each required slot reports as a failure. | Required coverage mislabelled |
| F-BUG061014-06 | Neither classifier prints the fixture's `SKIP_REASON`. The reason is emitted by the fixture into the run log, but the results block that a reader scans shows only `FAIL: <name> (exit=77)`. | Diagnostic loss at the summary layer |
| F-BUG061014-07 | The 7 skip-77 fixtures are not in the default lane today: `smackerel.sh`'s `e2e_lifecycle_scripts` / `e2e_shared_scripts` arrays name no `assistant_*` fixture, and `run_all.sh` defaults to the flat glob `test_*.sh`. They are reached through `--shell-run`, or through `run_all.sh <pattern>`. | Reachability scoping — see "Blast radius" |

## Reproduction

Executed in this session, at `3bec257660f5c4292d79e67d94391f51f72cdda0`.

**R1 — the producer honours the convention.**

```text
$ bash tests/e2e/assistant_regression/bs_004_notification_confirm.sh
=== Spec 061 SCOPE-10 DoD #7 — BS-004 persistent regression fixture ===
RESULT: SKIPPED
SKIP_REASON: SCOPE-04-NOTIFICATION-PROPOSAL-FIXTURE-NOT-YET-AUTHORED
FIXTURE_PATH: tests/e2e/assistant_regression/bs_004_notification_confirm.sh
NOTE: this file is a persistent regression slot for BS-004; the slot
      is honored on every CI run so the skip itself is visible. The
      round that closes "SCOPE-04-NOTIFICATION-PROPOSAL-FIXTURE-NOT-YET-AUTHORED"
      replaces this skip-77 with the
      executed §18.5 assertion shape declared in the fixture body.
FIXTURE_EXIT=77
```

**R2 — every slot behaves identically.**

```text
  exit=77  RESULT_line=RESULT: SKIPPED  tests/e2e/assistant_regression/bs_002_retrieval_qa.sh
  exit=77  RESULT_line=RESULT: SKIPPED  tests/e2e/assistant_regression/bs_004_notification_confirm.sh
  exit=77  RESULT_line=RESULT: SKIPPED  tests/e2e/assistant_regression/bs_005_ambiguous_disambig.sh
  exit=77  RESULT_line=RESULT: SKIPPED  tests/e2e/assistant_regression/bs_007_provenance_violation.sh
  exit=77  RESULT_line=RESULT: SKIPPED  tests/e2e/assistant_regression/bs_008_disabled_skill.sh
  exit=77  RESULT_line=RESULT: SKIPPED  tests/e2e/assistant_regression/bs_009_sst_missing_boot_failure.sh
  exit=77  RESULT_line=RESULT: SKIPPED  tests/e2e/assistant_acceptance_telegram_smoke.sh
```

**R3 — the consumer does not.** Both classifiers, read verbatim:

`tests/e2e/run_all.sh:41-47`

```bash
  if [ $exit_code -eq 0 ]; then
    RESULTS+=("PASS: $test_name")
    PASSED=$((PASSED + 1))
  else
    RESULTS+=("FAIL: $test_name (exit=$exit_code)")
    FAILED=$((FAILED + 1))
  fi
```

`smackerel.sh:1954-1968`

```bash
        e2e_record_shell_result() {
          local test_name="$1"
          local status="$2"

          if [[ "$status" -eq 0 ]]; then
            e2e_shell_results+=("PASS: ${test_name}")
            return 0
          fi

          e2e_shell_results+=("FAIL: ${test_name} (exit=${status})")
          e2e_shell_failures=$((e2e_shell_failures + 1))
          if [[ "$e2e_overall_status" -eq 0 ]]; then
            e2e_overall_status="$status"
          fi
        }
```

`e2e_record_shell_result` is the classifier that produced the operator's observed
line, which carries the relative path rather than a basename:
`FAIL: assistant_regression/bs_004_notification_confirm.sh (exit=77)`.

Full command transcripts are in `report.md`.

## Blast radius

Counted, not estimated, at the revision above.

| Measure | Count | Source |
|---|---|---|
| Files containing `reg_skip_with_blocker` | 8 | `grep -rln reg_skip_with_blocker tests/e2e/` |
| …of which are call sites (excluding the defining helper) | 7 | 6 under `tests/e2e/assistant_regression/`, plus `tests/e2e/assistant_acceptance_telegram_smoke.sh` |
| Files calling `skip_unless_accel_tier` (excluding the defining helper) | 10 | `grep -rln skip_unless_accel_tier tests/` |
| **Fixtures whose reported colour is untrustworthy** | **17** | 7 false-red + 10 false-green |
| Required spec 061 BS slots currently skip-77 | 6 of 10 | BS-002, 004, 005, 007, 008, 009 |
| Independent binary classifiers to correct | 2 | `run_all.sh`, `smackerel.sh` |
| Runner code paths that read exit `77` | 0 | `grep -rn 'exit 77'` finds only the producer |

**Reachability, stated precisely rather than as an adjective.** The claim "the
broader e2e suite can never be green while any slot is legitimately skipped" is
directionally correct but does not describe the default lane as wired today. The
7 skip-77 fixtures appear in neither `smackerel.sh`'s hardcoded lane arrays nor
`run_all.sh`'s default `test_*.sh` glob, so a bare `./smackerel.sh test e2e` is
not currently reddened by them. The claim holds exactly for any invocation that
does include them — `--shell-run`, or `run_all.sh <pattern>` — and it becomes
true of the default lane the moment the required slots are wired into it, which
spec 061 SCOPE-10 DoD #7 requires. Wiring those required slots into a runner that
cannot express a skip would convert a documented skip into a permanently red
suite. Correcting the classifier is what makes that wiring safe to perform.

## Relationship to BUG-069-005

`specs/069-assistant-http-transport/bugs/BUG-069-005-required-e2e-false-green/`
(status `blocked`) records that five manifest-required Go `e2e-api` tests call
`t.Skipf` when a required control is absent, so the package exits 0 and the suite
reports green while proving none of the five behaviours. Its finding
F-BUG069005-05 names the result `5 executed, 0 required behavior passed, 5
skipped` with package exit 0.

The two packets are the same root cause seen from opposite sides, and neither is
a duplicate of the other:

| | BUG-069-005 | BUG-061-014 (this packet) |
|---|---|---|
| Layer | Go test lane | Shell test lane |
| Skip visibility | Silent — absorbed into exit 0 | Loud — structured record on stdout |
| Reported colour | Green | Red |
| Direction of the lie | Understates risk | Overstates failure |
| Shared consequence | The suite's colour does not describe what the suite proved | Same |

The precise relationship: BUG-069-005 is the false-green failure mode in Go;
this packet is the false-red failure mode in shell **plus** the false-green
failure mode in shell, because `skip_unless_accel_tier` reproduces BUG-069-005's
exact shape at `tests/e2e/lib/helpers.sh:169-185` — a structured `SKIP:` line
followed by `exit 0`. Any correction here must therefore not simply teach the
runner to ignore `77`; it must give skips a bucket of their own that is neither
pass nor fail, or it will trade one untrustworthy colour for the other.

## Relationship to BUG-069-006

`specs/069-assistant-http-transport/bugs/BUG-069-006-confirm-redemption-not-single-flight/`
is terminal `blocked`, and its `blockedReason` names this defect directly,
quoting the runner line `FAIL: assistant_regression/bs_004_notification_confirm.sh (exit=77)`.
Nineteen of its twenty DoD items are closed on executed evidence; the twentieth,
"Broader E2E regression suite passes", is recorded as `DIS-069-006-4`.

What correcting the classifier does and does not do, stated exactly:

- **Does:** stop `bs_004` being printed as `FAIL` and stop it incrementing the
  failure tally. The suite's report would name it as a skip with its
  `SKIP_REASON`, which is what the fixture already emits.
- **Does not:** make `bs_004` exercise the confirm flow. The fixture's executed
  branch is still unauthored, blocked on
  `SCOPE-04-NOTIFICATION-PROPOSAL-FIXTURE-NOT-YET-AUTHORED`, which is spec 061
  SCOPE-04 work with its own boundary.

So whether `DIS-069-006-4` closes depends on which reading of "Broader E2E
regression suite passes" governs. Under "the suite reports no failure", this
correction closes it. Under "the confirm-flow regression executed and passed", it
does not, and the item stays open until the notification-proposal fixture exists.
This packet claims only the first. Promising the second would be a second false
claim of exactly the kind BUG-069-005 was opened for.

## What is NOT broken

- `reg_skip_with_blocker` itself. It emits a structured, greppable record with the
  BS identifier, a stable `SKIP_REASON` token, and the fixture path. It is the
  better-behaved half of this system and needs no change to be classified
  correctly.
- The fixtures' declared assertion shapes. Each skip-77 slot documents its
  intended §18.5 assertion pattern and its adversarial guards inline. Those
  bodies are correct as written.
- Exit code `77` as the chosen value. It is the conventional shell skip code and
  collides with nothing this repository emits.
- The `run_all.sh` two-phase stack model and the CLI's lifecycle/shared split.
  Those are orthogonal to result classification.

## Origin

The convention was introduced with spec 061 SCOPE-10 close-out (Round 83), which
authored the persistent per-BS regression slots. The producer side was written
and the consumer side was assumed. `scopes.md:1347` records the slots as
satisfying "persistent-slot-existence", which is true, and the runner was never
asked whether it could report them.

## Related

- `specs/069-assistant-http-transport/bugs/BUG-069-005-required-e2e-false-green/` — the false-green sibling in the Go lane
- `specs/069-assistant-http-transport/bugs/BUG-069-006-confirm-redemption-not-single-flight/` — blocked on `DIS-069-006-4`, which quotes this defect
- `specs/061-conversational-assistant/scopes.md:1347` — SCOPE-10 DoD #7, the requirement that creates the ten per-BS slots
- `specs/061-conversational-assistant/scopes.md:1374` — the `skip_unless_accel_tier` bootstrap contract, which documents `exit 0` on `tier=cpu`
