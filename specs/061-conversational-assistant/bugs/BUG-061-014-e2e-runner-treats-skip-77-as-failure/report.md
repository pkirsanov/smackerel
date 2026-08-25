# Report: BUG-061-014 — Filing and root-cause evidence

### Summary

Discovery-only invocation of `bubbles.bug` at revision
`3bec257660f5c4292d79e67d94391f51f72cdda0`, working tree clean. This pass filed
the packet and verified the defect. **No fix was implemented and no runtime file
was touched.** Every DoD item in `scopes.md` is unchecked, and both scopes are
`Not started`.

The defect was reported by the operator with a specific observation. Each claim
was re-verified here by execution rather than restated, and two of them needed
correction or qualification, recorded below.

Shell prompt echo lines are elided from the transcripts so no operator account
name or home path enters the artifact. Command text and output are otherwise
verbatim.

### Completion Statement

The bug is documented and its root cause is established. Implementation has not
started. `state.json` status is `in_progress` with `execution.currentPhase` set
to `analyze`, and `certification` is unwritten because no phase beyond discovery
has run.

---

## Repository binding

**Claim Source:** executed

```text
$ bash .github/bubbles/scripts/repository-binding-host-context.sh --session-log <host-provided> \
    --workspace-root <each host-declared root>
{"schemaVersion":1,"hostAdapter":"vscode-session-log","sessionId":"vscode-ad58d1923c2301065c1d41d950c10d83",
 "sessionControlFile":"<runtime control path>","sessionLogIdentity":"sha256:ad58d1923c2301065c1d41d950c10d83057f4891ecf5622eaa1bec14ab0d4ba2",
 "expectedControlRevision":55,"workspaceRoots":[...11 roots...]}
HOST_CONTEXT_EXIT=0

$ bash .github/bubbles/scripts/repository-binding.sh preflight \
    --session-id vscode-ad58d1923c2301065c1d41d950c10d83 \
    --session-control-file <runtime control path> \
    --expected-control-revision 55 --request-class STRUCTURED \
    --workspace-root <each canonical root> --repository-root <this repository>
REPOSITORY PREFLIGHT CONFIRMED repository=smackerel root=<this repository> source=explicit-repositoryRoot affinity=confirmed
PREFLIGHT_COMMITTED decision=rb:vscode-ad58d1923c2301065c1d41d950c10d83:56 revision=56 repository=smackerel root=<this repository>
PREFLIGHT_EXIT=0
```

---

## Revision under test

**Claim Source:** executed

```text
$ git rev-parse HEAD
3bec257660f5c4292d79e67d94391f51f72cdda0

$ git status --short
(no output — working tree clean)

$ date -u +%Y-%m-%dT%H:%M:%SZ
2026-08-25T02:52:34Z
```

---

## E-1 — The producer honours the skip convention

**Claim Source:** executed

```text
$ sed -n '30,50p' tests/e2e/run_all.sh
run_test() {
  local test_file="$1"
  local test_name
  test_name="$(basename "$test_file" .sh)"

  echo "--- Running: $test_name ---"
  set +e
  bash "$test_file" 2>&1
  local exit_code=$?
  set -e

  if [ $exit_code -eq 0 ]; then
    RESULTS+=("PASS: $test_name")
    PASSED=$((PASSED + 1))
  else
    RESULTS+=("FAIL: $test_name (exit=$exit_code)")
    FAILED=$((FAILED + 1))
  fi
  echo ""
}

$ grep -nA14 'reg_skip_with_blocker()' tests/e2e/assistant_regression/lib/regression_helpers.sh
36:reg_skip_with_blocker() {
37-  local bs="${1:?BS id required}"
38-  local reason="${2:?SKIP_REASON required}"
39-  echo "=== Spec 061 SCOPE-10 DoD #7 — $bs persistent regression fixture ==="
40-  echo "RESULT: SKIPPED"
41-  echo "SKIP_REASON: $reason"
42-  echo "FIXTURE_PATH: ${BASH_SOURCE[1]:-unknown}"
43-  echo "NOTE: this file is a persistent regression slot for $bs; the slot"
44-  echo "      is honored on every CI run so the skip itself is visible. The"
45-  echo "      round that closes \"$reason\" replaces this skip-77 with the"
46-  echo "      executed §18.5 assertion shape declared in the fixture body."
47-  exit 77
48-}
```

The two blocks are the whole defect in twenty-nine lines. The helper declares a
third outcome and the classifier has two branches.

---

## E-2 — Reproduction: the fixture exits 77 with a structured skip record

**Claim Source:** executed

```text
$ bash tests/e2e/assistant_regression/bs_004_notification_confirm.sh
=== Spec 061 SCOPE-10 DoD #7 — BS-004 persistent regression fixture ===
RESULT: SKIPPED
SKIP_REASON: SCOPE-04-NOTIFICATION-PROPOSAL-FIXTURE-NOT-YET-AUTHORED
FIXTURE_PATH: tests/e2e/assistant_regression/bs_004_notification_confirm.sh
NOTE: this file is a persistent regression slot for BS-004; the slot
      is honored on every CI run so the skip itself is visible. The
      round that closes "SCOPE-04-NOTIFICATION-PROPOSAL-FIXTURE-NOT-YET-AUTHORED" replaces this skip-77 with the
      executed §18.5 assertion shape declared in the fixture body.
FIXTURE_EXIT=77
```

Matches the operator's observation exactly: `RESULT: SKIPPED`, the named
`SKIP_REASON`, exit `77`.

---

## E-3 — Every skip slot behaves identically

**Claim Source:** executed

```text
$ for f in <the seven reg_skip_with_blocker fixtures>; do
    out=$(timeout 60 bash "$f" 2>&1); rc=$?
    echo "  exit=$rc  RESULT_line=$(printf '%s' "$out" | grep -m1 '^RESULT:')  $f"
  done
  exit=77  RESULT_line=RESULT: SKIPPED  tests/e2e/assistant_regression/bs_002_retrieval_qa.sh
  exit=77  RESULT_line=RESULT: SKIPPED  tests/e2e/assistant_regression/bs_004_notification_confirm.sh
  exit=77  RESULT_line=RESULT: SKIPPED  tests/e2e/assistant_regression/bs_005_ambiguous_disambig.sh
  exit=77  RESULT_line=RESULT: SKIPPED  tests/e2e/assistant_regression/bs_007_provenance_violation.sh
  exit=77  RESULT_line=RESULT: SKIPPED  tests/e2e/assistant_regression/bs_008_disabled_skill.sh
  exit=77  RESULT_line=RESULT: SKIPPED  tests/e2e/assistant_regression/bs_009_sst_missing_boot_failure.sh
  exit=77  RESULT_line=RESULT: SKIPPED  tests/e2e/assistant_acceptance_telegram_smoke.sh
LOOP_DONE
```

Seven for seven. All skips are unconditional at the top of the fixture, so none
requires a running stack to reproduce.

---

## E-4 — The second classifier, and which one the operator saw

**Claim Source:** executed

The operator's observed line carried a relative path,
`FAIL: assistant_regression/bs_004_notification_confirm.sh (exit=77)`. `run_test`
in `run_all.sh` uses `basename "$test_file" .sh`, so it could not have produced
that string. Searching for the emitter found a second, independent classifier.

```text
$ grep -rn 'FAIL: .*exit=' --include='*.sh' . | grep -v '/.git/'
./smackerel.sh:1963:          e2e_shell_results+=("FAIL: ${test_name} (exit=${status})")
./tests/e2e/run_all.sh:45:    RESULTS+=("FAIL: $test_name (exit=$exit_code)")
(other matches are per-lane Go/Python status echoes, not shell fixture classification)

$ grep -rn 'run_test()' --include='*.sh' . | grep -v '/.git/'
./tests/e2e/run_all.sh:30:run_test() {

$ sed -n '1954,1968p' smackerel.sh
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

Two findings. First, the defect is duplicated across two independent classifiers,
so a fix applied to one leaves the other intact. Second, `e2e_overall_status`
takes the raw child status, which is why the lane exits `77` rather than `1` —
the operator's reported lane exit is explained by this line.

---

## E-5 — The convention has a producer and no consumer

**Claim Source:** executed

```text
$ grep -rn 'exit 77' --include='*.sh' . | grep -v '/.git/'
./tests/e2e/assistant_regression/lib/regression_helpers.sh:47:  exit 77
./tests/e2e/assistant_regression/lib/regression_helpers.sh:70:# the fixture (exit 77) when the key is absent or empty. Used by
```

One emitter and one comment. Widening the search to every `77` token outside
`.github/bubbles/**` returned only the helper and fixture header comments that
describe the convention. **No runner code path anywhere in this repository reads
exit code 77.** The convention was written as prose and assumed to be implemented.

---

## E-6 — The second, opposite skip convention

**Claim Source:** executed

This was not in the reported defect and was found while measuring blast radius.
It changes the correct fix, so it is recorded as a first-class finding.

```text
$ grep -n -A18 'skip_unless_accel_tier()' tests/e2e/lib/helpers.sh
169:skip_unless_accel_tier() {
170-  local test_name="${1:?skip_unless_accel_tier requires a test name}"
171-  : "${SMACKEREL_HARDWARE_TIER:?SMACKEREL_HARDWARE_TIER must be set (cpu|accel) — see .smackerel.local.env.example}"
172-  case "$SMACKEREL_HARDWARE_TIER" in
173-    cpu)
174-      echo "SKIP: ${test_name} — cpu-tier hardware lacks accelerator; live-stack retrieval-qa loop overshoots 15s ceiling per SCOPE-06c Round 71e evidence. Run on accel-tier host for live-stack PASS."
175-      exit 0
176-      ;;
177-    accel)
178-      return 0
179-      ;;
180-    *)
181-      echo "[${test_name}] unknown SMACKEREL_HARDWARE_TIER=${SMACKEREL_HARDWARE_TIER} (expected cpu|accel)" >&2
182-      exit 2
183-      ;;
184-  esac
185-}

$ grep -rln 'skip_unless_accel_tier' tests/
tests/e2e/assistant_acceptance_capture.sh
tests/e2e/assistant_acceptance_notification.sh
tests/e2e/assistant_acceptance_retrieval.sh
tests/e2e/assistant_acceptance_weather.sh
tests/e2e/assistant_bs002_test.sh
tests/e2e/assistant_bs003_test.sh
tests/e2e/assistant_bs004_test.sh
tests/e2e/assistant_bs006_test.sh
tests/e2e/assistant_bs007_test.sh
tests/e2e/assistant_bs010_test.sh
tests/e2e/lib/helpers.sh
COUNT=11
```

A structured `SKIP:` message followed by `exit 0` is BUG-069-005's shape rendered
in shell: the skip is announced and the exit code reports success. Ten fixtures
consume it, excluding the defining helper.

The two conventions together are what establishes the root cause. Two authors on
different scopes reached opposite conclusions about how to signal a skip, and the
runner misreported both. That is the signature of a missing vocabulary value, not
of a coding mistake.

---

## E-7 — Blast radius, counted

**Claim Source:** executed

```text
$ grep -rln reg_skip_with_blocker tests/e2e/ | sort
tests/e2e/assistant_acceptance_telegram_smoke.sh
tests/e2e/assistant_regression/bs_002_retrieval_qa.sh
tests/e2e/assistant_regression/bs_004_notification_confirm.sh
tests/e2e/assistant_regression/bs_005_ambiguous_disambig.sh
tests/e2e/assistant_regression/bs_007_provenance_violation.sh
tests/e2e/assistant_regression/bs_008_disabled_skill.sh
tests/e2e/assistant_regression/bs_009_sst_missing_boot_failure.sh
tests/e2e/assistant_regression/lib/regression_helpers.sh
COUNT_FILES=8

$ for f in tests/e2e/assistant_regression/bs_*.sh; do
    echo "  skip77_calls=$(grep -c 'reg_skip_with_blocker' "$f")  $(basename "$f")"
  done
  skip77_calls=0  bs_001_capture_fallback.sh
  skip77_calls=1  bs_002_retrieval_qa.sh
  skip77_calls=0  bs_003_weather_happy_path.sh
  skip77_calls=1  bs_004_notification_confirm.sh
  skip77_calls=1  bs_005_ambiguous_disambig.sh
  skip77_calls=0  bs_006_weather_outage.sh
  skip77_calls=1  bs_007_provenance_violation.sh
  skip77_calls=1  bs_008_disabled_skill.sh
  skip77_calls=1  bs_009_sst_missing_boot_failure.sh
  skip77_calls=0  bs_010_telegram_e2e.sh
```

| Measure | Count |
|---|---|
| Files containing `reg_skip_with_blocker` | 8 |
| …call sites, excluding the defining helper | 7 |
| Files calling `skip_unless_accel_tier`, excluding the defining helper | 10 |
| **Fixtures whose reported colour is untrustworthy** | **17** |
| Required spec 061 BS slots currently skip-77 | 6 of 10 |
| Independent binary classifiers | 2 |
| Runner code paths reading exit 77 | 0 |

---

## E-8 — Verifying the operator's three reasons

The operator asked for each reason to be verified rather than repeated. Two
needed qualification.

### Reason 1 — "the broader e2e suite can never be green while any slot is legitimately skipped"

**Claim Source:** executed. **Verdict: correct in principle, not yet true of the default lane.**

```text
$ sed -n '2035,2093p' smackerel.sh | grep -c 'assistant'
0

$ grep -n 'PATTERN=' tests/e2e/run_all.sh
11:PATTERN="${1:-test_*.sh}"

$ ls tests/e2e/test_*.sh | grep -c 'assistant_regression\|telegram_smoke'
0
```

`smackerel.sh`'s `e2e_lifecycle_scripts` and `e2e_shared_scripts` arrays name no
`assistant_*` fixture, and `run_all.sh`'s default glob is flat `test_*.sh`, which
does not descend into `assistant_regression/`. So a bare `./smackerel.sh test e2e`
is not reddened by these fixtures today. The claim holds exactly for any
invocation that does include them — `--shell-run`, or `run_all.sh <pattern>` — and
it becomes true of the default lane the moment the required slots are wired in,
which spec 061 SCOPE-10 DoD #7 requires. The dependency runs the other way from
how it first appears: correcting the classifier is what makes that wiring safe,
rather than the wiring being needed to expose the defect.

### Reason 2 — "it inverts the concern of BUG-069-005"

**Claim Source:** executed. **Verdict: correct, and stronger than stated.**

```text
$ python3 -c "import json; d=json.load(open('specs/069-assistant-http-transport/bugs/BUG-069-005-required-e2e-false-green/state.json')); print(d['status'])"
blocked
```

`BUG-069-005` finding F-BUG069005-05 records five manifest-required Go tests
converting a missing control into `t.Skipf`, yielding "5 executed, 0 required
behavior passed, 5 skipped" with package exit 0. The inversion the operator
describes is real: there a skip is silent and green, here it is loud and red.

The relationship is stronger than an inversion, because E-6 shows this repository
also contains BUG-069-005's own failure mode in the shell lane. So this packet is
not merely the mirror image; it contains both images. That is why `scopes.md`
Scope 2 exists and why the fix cannot be "teach the runner to ignore 77".

### Reason 3 — "it is the direct cause of a blocked packet elsewhere"

**Claim Source:** executed. **Verdict: correct that it blocks the packet; the unblocking claim needs splitting.**

```text
$ python3 -c "import json; d=json.load(open('specs/069-assistant-http-transport/bugs/BUG-069-006-confirm-redemption-not-single-flight/state.json')); print(d['status'])"
blocked

$ grep -rn 'DIS-069-006-4' specs/069-assistant-http-transport/bugs/BUG-069-006-confirm-redemption-not-single-flight/
  scopes.md:190, report.md:351, report.md:2427, report.md:2432, report.md:2556,
  report.md:3088, state.json:210, state.json:254, state.json:311
```

The recorded `blockedReason` quotes this defect verbatim, including the runner
line `FAIL: assistant_regression/bs_004_notification_confirm.sh (exit=77)`, and
states that nineteen of twenty DoD items are closed on executed evidence with the
twentieth recorded as `DIS-069-006-4`.

The qualification: correcting the classifier stops `bs_004` being printed as a
failure and stops it incrementing the failure tally. It does not make `bs_004`
exercise the confirm flow — its executed branch is still unauthored, blocked on
`SCOPE-04-NOTIFICATION-PROPOSAL-FIXTURE-NOT-YET-AUTHORED`, which is spec 061
SCOPE-04 work with its own boundary. Whether `DIS-069-006-4` closes therefore
depends on which reading of "Broader E2E regression suite passes" governs that
packet. Under "the suite reports no failure" it closes; under "the confirm-flow
regression executed and passed" it does not. This packet claims only the first.
Claiming the second would be a second false statement of exactly the kind
`BUG-069-005` exists to prevent.

---

## E-9 — Identifier collision

**Claim Source:** executed

The packet was requested as `BUG-061-011`. That identifier is occupied.

```text
$ python3 -c "import json; d=json.load(open('specs/061-conversational-assistant/bugs/BUG-061-011-eval-gate-runs-in-no-automated-lane/state.json')); print(d['bugId'], d['status'])"
BUG-061-011 blocked

$ grep -rl 'BUG-061-011' specs/ --include='*.md' --include='*.json' | grep -v 'BUG-061-011-eval-gate-runs-in-no-automated-lane'
specs/064-open-ended-knowledge-agent/bugs/BUG-064-003-router-warmup-exceeds-fixed-deadline/{scopes,report,bug}.md, state.json
specs/061-conversational-assistant/bugs/BUG-061-013-wrapper-contract-zero-match-silent-pass/{scopes,report,spec,bug,design}.md, state.json
specs/061-conversational-assistant/bugs/BUG-061-012-model-supplied-identity-in-agent-tools/state.json
REF_COUNT=11

$ ls -1 specs/061-conversational-assistant/bugs/ | sed -E 's/^BUG-061-([0-9]+).*/\1/' | sort -n | tail -1
013
```

A second packet numbered `BUG-061-011` would make all eleven of those references
ambiguous. The highest existing identifier under spec 061 is `013`, so this
packet takes `BUG-061-014` and keeps the requested descriptive slug. The
deviation is recorded in `bug.md` under "Identifier note".

---

## E-10 — Requiredness has no manifest source today

**Claim Source:** executed

This determines the design answer to "what distinguishes required from optional".

```text
$ ls -1 specs/061-conversational-assistant/scenario-manifest.json
ls: cannot access 'specs/061-conversational-assistant/scenario-manifest.json': No such file or directory

$ grep -rln 'assistant_regression' specs/*/scenario-manifest.json specs/*/bugs/*/scenario-manifest.json
(no output)

$ grep -n 'persistent regression' specs/061-conversational-assistant/scopes.md
1347:- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior in spec 061
      (BS-001..010) authored under `tests/e2e/assistant_regression/` with one persistent file per
      BS scenario; ... **Round 83 close-out:** persistent-slot-existence satisfied — all 10 BS
      regression fixtures present ... (executed branch documented inline behind the substrate
      skip-77 where the supporting graph-seeding/stub container has not yet landed ...)
```

Spec 061 requires all ten BS slots by name, but no manifest names a shell
fixture, so requiredness cannot be resolved from manifest data. `design.md` D3
records three candidate sources, rejects the two that let a fixture
self-certify its own requiredness, and selects explicit declaration in the runner
— matching the idiom the repository already uses for `LIFECYCLE_TESTS` at
`run_all.sh:20` and the lane arrays at `smackerel.sh:2036-2092`.

---

## Test Evidence

**No tests were written or executed for a fix in this invocation, because no fix
was implemented.** The commands above are discovery evidence: they establish the
defect's existence, its duplication across two classifiers, and its blast radius.

The tests this packet requires are specified but not yet authored. `scopes.md`
declares them across two Test Plan tables with eleven and six rows, and every
corresponding DoD item is unchecked. The regression suite must include the four
adversarial cases in Scope 1 and the two in Scope 2, because the obvious wrong
fix — mapping exit `77` onto the existing pass branch — satisfies a naive reading
of the requirement while recreating `BUG-069-005`.

The pre-fix regression test does not exist yet and therefore has not been
observed failing. That observation is a DoD item, not a claim this report makes.

---

## Files changed in this invocation

**Claim Source:** executed — artifact creation only.

| Path | Change |
|---|---|
| `specs/061-conversational-assistant/bugs/BUG-061-014-e2e-runner-treats-skip-77-as-failure/bug.md` | Created |
| `specs/061-conversational-assistant/bugs/BUG-061-014-e2e-runner-treats-skip-77-as-failure/spec.md` | Created |
| `specs/061-conversational-assistant/bugs/BUG-061-014-e2e-runner-treats-skip-77-as-failure/design.md` | Created |
| `specs/061-conversational-assistant/bugs/BUG-061-014-e2e-runner-treats-skip-77-as-failure/scopes.md` | Created |
| `specs/061-conversational-assistant/bugs/BUG-061-014-e2e-runner-treats-skip-77-as-failure/report.md` | Created |
| `specs/061-conversational-assistant/bugs/BUG-061-014-e2e-runner-treats-skip-77-as-failure/uservalidation.md` | Created |
| `specs/061-conversational-assistant/bugs/BUG-061-014-e2e-runner-treats-skip-77-as-failure/state.json` | Created |

In `uservalidation.md`, the four defect items are checked because each was
observed by execution during this pass. Every delivery item is unchecked, and no
agent may check those.

No runtime file was modified. `tests/e2e/run_all.sh`, `smackerel.sh`,
`tests/e2e/lib/helpers.sh`, and every fixture remain at their state as of
`3bec257660f5c4292d79e67d94391f51f72cdda0`. `.github/bubbles/**` was read only;
it is framework-managed and is refreshed solely through the Bubbles installer or
upgrade command.

## Implementation Phase

Scope 1 only. Both shell E2E classifiers now treat SKIP as a first-class outcome.
Scope 2 — `skip_unless_accel_tier`, the false-green half — is untouched, because
it carries its own Change Boundary.

### What changed

Two classifiers, one rule. `run_test` in `tests/e2e/run_all.sh` gains `SKIPPED`
and `REQUIRED_SKIPPED` counters; `e2e_record_shell_result` in `smackerel.sh`
gains `e2e_shell_skips` and `e2e_required_shell_skips`. Both read the named
constant `77` rather than a bare literal.

Three design points carry the weight:

**Requiredness is declared runner-side**, in an explicit array matching the idiom
already used for `LIFECYCLE_TESTS`. A fixture therefore cannot downgrade itself
out of the required set by editing its own body, which is the same
self-certification weakness that permits silent skips in the first place.

**A required skip is still labelled SKIP.** The label stays honest while the
suite exit stays non-zero, because the behaviour the fixture covers is unproven.
Both halves are needed; the common wrong fix supplies only the first.

**The raw child status is never propagated.** A required skip sets the overall
status to `1`. Previously `e2e_record_shell_result` assigned the child status
straight into `e2e_overall_status`, which is why the lane exited `77` and
reported a skip code as its own result.

Output is captured with `tee` and `PIPESTATUS[0]`, so the fixture streams live
while the classifier still gets a copy to read `SKIP_REASON` from. Nothing is
swallowed.

### One correction applied during this phase

The first pass left `smackerel.sh` printing `SKIP_REASON in the fixture output
above` while `run_all.sh` carried the real reason. Two classifiers reporting the
same outcome differently is a smaller copy of the exact inconsistency this packet
exists to remove, so `smackerel.sh` now extracts and carries the reason as well.
The visible result changed from a pointer to the reason itself.

### Pre-fix RED — the regression suite fails against the unmodified runners

DoD item 7 asks whether the new contract suite actually detects the defect. That
cannot be reconstructed after the fix, so it was measured against a detached
worktree at the pre-fix commit with only the new contract files copied in:

```
$ git worktree add --detach /tmp/<wt> e8b40360^
$ grep -c 'SKIP_EXIT_CODE' /tmp/<wt>/tests/e2e/run_all.sh
0
$ cd /tmp/<wt> && bash tests/e2e/runner_contract/run_runner_contract.sh
  FAIL: test_rc_fail (exit=1)
  FAIL: test_rc_optional_skip (exit=77)
  FAIL AC-01-1 — skip fixture is reported as SKIP
  FAIL: test_rc_fail (exit=1)
  FAIL: test_rc_optional_skip (exit=77)
  FAIL AC-01-2 — skip fixture is NOT reported as FAIL
  Assertions run:    44
  Assertions failed: 28
exit: 1
```

The `grep -c` returning `0` confirms the worktree holds the unmodified runner.
`FAIL: test_rc_optional_skip (exit=77)` is the defect itself, printed by the old
classifier. The suite fails 28 of 44 assertions and exits 1.

The assertion count differs between runs — 44 pre-fix against 51 post-fix —
because several assertions are reached only once the runner emits a SKIP line at
all. That difference is itself evidence rather than noise: assertions that cannot
even execute against the old runner are exactly the ones describing the outcome
it could not express.

### Post-fix GREEN

```
$ bash tests/e2e/runner_contract/run_runner_contract.sh
  ok   AC-08-bs_004_notification_confirm — assistant_regression/bs_004_notification_confirm.sh classifies as SKIP
  ok   AC-08F-bs_004_notification_confirm — assistant_regression/bs_004_notification_confirm.sh does NOT classify as FAIL
  ok   AC-08-9 — each slot carries its own SKIP_REASON in the results block
  ok   AC-08-10 — a second slot carries a different SKIP_REASON
=========================================
  Runner-contract results
=========================================
  Assertions run:    51
  Assertions failed: 0
=========================================
exit: 0
```

### End-to-end through the real lane

The behaviour that made this packet exist, before and after:

```
BEFORE
$ ./smackerel.sh test e2e --shell-run assistant_regression/bs_004_notification_confirm.sh
  FAIL: assistant_regression/bs_004_notification_confirm.sh (exit=77)
exit: 77

AFTER
$ ./smackerel.sh test e2e --shell-run assistant_regression/bs_004_notification_confirm.sh
RESULT: SKIPPED
  SKIP: assistant_regression/bs_004_notification_confirm.sh (SCOPE-04-NOTIFICATION-PROPOSAL-FIXTURE-NOT-YET-AUTHORED)
  Skipped: 1
exit: 0
```

The fixture is unchanged. Only its classification moved, from a failure it never
was to the skip it always declared itself to be — carrying its reason into the
summary so the skip is more visible than before, not less.

### Build Quality Gate

```
$ ./smackerel.sh lint
exit: 0
$ ./smackerel.sh format --check
exit: 0
$ ./smackerel.sh test unit --go
exit: 0
$ bash .github/bubbles/scripts/artifact-lint.sh <packet-dir>
exit: 0
```

`docs/Testing.md` records the three-outcome contract, and `bug.md` reads
`**Status:** Fixed (Scope 1)`. The commit touches zero files under
`.github/bubbles/`.

### What this phase does NOT claim

Two DoD items stay unchecked, and the reasons are specific rather than
housekeeping.

**Mutation-proof of each adversarial case.** ADV-061-014-01 through 04 exist and
assert the right propositions — that a skip is not reported as PASS, that an
exit-1 control stays FAIL and is not reclassified, that a required skip yields a
non-zero exit, and that the classifier text is extracted by function name instead
of re-implemented. The pre-fix run demonstrates the suite detects the real defect.
What has not been done is applying each named wrong fix in turn and confirming
that its own case is the one that fails. That is a stronger claim than the
evidence here supports.

**Existing lanes pass with no new failures.** Unit, lint and format are exit 0.
The full shell E2E suite was not run end to end, so the item is left open rather
than closed on a partial lane set — which is the same discipline that kept the
broader-suite item open in BUG-069-006.

## Implementation Phase — Scope 2

The false-green half. Scope 1 stopped deliberate skips being reported as
failures; this stops ten fixtures being reported as **passes** while proving
nothing.

Of the two, this is the more dangerous. A false red is noisy and gets
investigated. A false green is silent and gets trusted, which is precisely the
condition `BUG-069-005` was opened for — reproduced here in shell rather than in
Go.

### The change

`skip_unless_accel_tier` in `tests/e2e/lib/helpers.sh` printed a structured
`SKIP:` line and then `exit 0`. It now exits `77` and emits a machine-readable
reason, so the Scope 1 classifiers lift it into the results block exactly as they
do for `reg_skip_with_blocker`:

```
    cpu)
      echo "SKIP: ${test_name} — cpu-tier hardware lacks accelerator; ..."
      echo "RESULT: SKIPPED"
      echo "SKIP_REASON: CPU-TIER-HARDWARE-LACKS-ACCELERATOR"
      exit 77
      ;;
```

The `accel` return path and the unknown-tier `exit 2` path are untouched. A
misconfigured host stays a hard error rather than becoming a benign skip — that
distinction is what ADV-061-014-06 exists to defend.

### A wrong RED, caught and corrected

The first RED run was not measuring what it claimed. The sandbox generated a
`helpers.sh` that defined no-op lifecycle stubs and *then* sourced the real
helpers, so the real `e2e_setup` overwrote the stub and the runner tried to boot
an actual stack. It died before reaching its results block, and the count
assertions failed with `missing [Passed: 1]` — the right verdict for the wrong
reason.

Source order is load-bearing here: the real helpers must come first so the
tracked `skip_unless_accel_tier` is the function under test, and the lifecycle
names are overridden after. With that corrected the RED measures the defect:

```
$ bash tests/e2e/runner_contract/run_tier_skip_contract.sh   # unmodified helper
  FAIL SCN-09-1 — SCN-061-014-09: cpu tier exits 77
  FAIL SCN-09-2 — SCN-061-014-09: cpu tier does NOT exit 0
  FAIL SCN-10-2 — SCN-061-014-10: it is NOT reported as PASS
  FAIL ADV-05-a — ADV-061-014-05: the passed count excludes the skipped fixture
  FAIL UNIF-1  — both skip helpers exit with the same code
  Assertions run:    23
  Assertions failed: 9
exit: 1
```

`SCN-10-2` is the load-bearing line. It fails because the fixture **was** reported
as `PASS` — the false green, stated as a measurement rather than an argument.

Worth noting the first RED had `SCN-10-2` *passing*, which looked like good news
and was actually the broken sandbox masking the defect. A RED that agrees with
you for the wrong reason is worse than one that disagrees.

### GREEN

```
$ bash tests/e2e/runner_contract/run_tier_skip_contract.sh
  Assertions run:    23
  Assertions failed: 0
exit: 0
```

Scope 1's driver is unaffected and still runs `51` assertions with `0` failures,
which is why Scope 2 got its own file: Scope 1's recorded evidence stays exactly
reproducible.

### All ten consumers, measured

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
```

Ten of ten, each carrying its reason.

### Required versus optional — the decision and why

The ten are deliberately **not** declared required.

Requiredness would make every cpu-tier run permanently non-green. That converts a
legitimate hardware gate into standing red, and a suite that is always red
carries no signal — the same loss of meaning, arrived at from the opposite
direction, as a suite that is always green.

Honesty comes from the classification instead. They report `SKIP` and land in the
`Skipped` tally, so a reader sees exactly how many behaviours went unproven on
this host. Declaring them required asserts "this suite must run on accel
hardware", which is an operator decision about CI topology and belongs with
whoever owns that hardware, not with the classifier.

Recorded in `docs/Testing.md` under "Why the tier-gated fixtures are not declared
required", alongside a table showing both producers now resolve to the same
reported outcome.

## Mutation Proof — every adversarial case kills its own wrong fix

The implement phase left this item open because asserting an adversarial case
*exists* is weaker than proving it *bites*. Each named wrong fix was applied in a
detached worktree and the suite re-run. A case earns its keep only if its own
assertion is the one that fails.

| Case | Wrong fix applied | Result |
|---|---|---|
| ADV-061-014-01 | map exit `77` onto the `PASS` branch | `FAIL ADV-01-a — skip fixture is NOT reported as PASS` |
| ADV-061-014-02 | drop `REQUIRED_SKIPPED` from the suite-exit condition | `FAIL ADV-02-a — required skip yields a non-zero suite exit` (exactly 1 assertion failed) |
| ADV-061-014-03 | broaden the skip branch to `-ne 0` | `FAIL ADV-03-a`, `ADV-03-b`, `ADV-03-c` |
| ADV-061-014-04 | disable the skip branch in `smackerel.sh`'s tracked classifier | `FAIL AC-02-1 — skip fixture is reported as SKIP in the shell results block` |
| ADV-061-014-05 | count a skipped fixture as passed as well | `FAIL ADV-05-a`, `ADV-05-d` |
| ADV-061-014-06 | collapse the unknown tier into the skip branch | `FAIL ADV-06-a`, `ADV-06-b` |

Two of these are worth calling out.

**ADV-02 killed with exactly one failing assertion.** Nothing else moved, which
is what a precisely-targeted case looks like — it discriminates that specific
wrong fix rather than tripping over collateral damage.

**ADV-04 is a property, not a payload.** It asserts the driver executes the
*tracked* classifier rather than a re-implementation. Mutating `smackerel.sh`'s
extracted block and watching the driver notice is the proof: a driver that had
copied the branch logic would have stayed green while the real runner was broken.
The other five mutations demonstrate the same property for `run_all.sh`, since
each one changed only the tracked file and each was detected.

## Simplify Phase

One finding, and it is this packet's own defect shape at smaller scale.

Requiredness is declared twice — `REQUIRED_TESTS` in `tests/e2e/run_all.sh` and
`e2e_required_shell_tests` in `smackerel.sh`. They were identical (36 entries
each) and `smackerel.sh` carried a comment saying its list "mirrors
REQUIRED_TESTS in tests/e2e/run_all.sh".

A comment does not hold. The original bug existed *because* two surfaces that had
to agree about the skip convention disagreed and nothing checked. Leaving the
required sets guarded by prose rebuilds that condition on a delay.

`SCN-061-014-13` now asserts the agreement mechanically: both lists are extracted
from the tracked files and compared. The non-emptiness check comes first, because
two empty lists would otherwise agree vacuously — and an empty required set means
no skip can ever redden either lane, which is the failure this whole packet is
about.

```
--- SCN-061-014-13 — both classifiers declare the same required set ---
  run_all.sh declares 36 required fixtures
  smackerel.sh declares 36 required fixtures
  ok   AC-09-1 — run_all.sh's required set is non-empty
  ok   AC-09-2 — smackerel.sh's required set is non-empty
  ok   AC-09-3 — both classifiers declare the same number of required fixtures
  ok   AC-09-4 — the two required sets are identical
  Assertions run:    55
  Assertions failed: 0
```

Proven to bite. Removing one entry from `smackerel.sh` alone, in a worktree:

```
drift injected: removed test_browser_sync.sh from smackerel.sh only
  run_all.sh declares 36 required fixtures
  smackerel.sh declares 35 required fixtures
  FAIL AC-09-3 — both classifiers declare the same number of required fixtures
  FAIL AC-09-4 — the two required sets are identical
  Assertions failed: 2
exit: 1
```

### Considered and rejected

**Extracting the classifier into a shared shell library sourced by both.**
`run_all.sh` already sources `tests/e2e/lib/helpers.sh`, but `smackerel.sh`
deliberately sources nothing from the test tree. A shared library would create a
CLI-to-test-tree dependency in order to remove a duplication that a test can hold
instead. Asserting the invariant is cheaper and gives the stronger guarantee: the
lists may live apart as long as they cannot disagree.

## Stabilize Phase

The classifier change added per-fixture I/O — a `mktemp`, a `tee`, and a `sed` —
to a runner that executes 61 shell fixtures. Four operational questions, one real
finding.

**`mktemp` failure is already fail-loud.** `run_all.sh` runs under `set -euo
pipefail`, so a failed `mktemp` aborts the runner rather than producing an empty
`$output_file` and a confusing downstream error. This is correct behaviour and
needs no guard.

**Cost is negligible relative to what it measures.** One `mktemp`, one `tee` and
one `sed` per fixture, against fixtures that boot Docker stacks. The `sed` runs
only on the skip branch, so the common paths pay only `tee`.

**Peak temp usage is one fixture's output, not the suite's.** The file is removed
at the end of each `run_test`, so a fixture with a large log does not accumulate
across the run.

**Finding, and its fix.** `rm -f "$output_file"` sat at the end of `run_test`
with no `trap`, so an interrupted run left the current fixture's temp file in
`TMPDIR`. Replaced with a single run-scoped file plus signal handlers, which is
both simpler than a per-fixture `mktemp` and leak-free on every catchable path:

```
RUN_OUTPUT_FILE="$(mktemp)"
cleanup_run_output() { rm -f "$RUN_OUTPUT_FILE"; }
trap cleanup_run_output EXIT
trap 'cleanup_run_output; exit 130' INT
trap 'cleanup_run_output; exit 143' TERM
```

The first attempt used `trap ... EXIT` alone. Testing it rather than assuming it
is what caught the gap: bash does **not** run an `EXIT` trap when killed by an
untrapped `SIGTERM`, which is exactly how the `timeout` wrapper in the CLI lane
stops this runner. `INT` and `TERM` are therefore handled explicitly, preserving
the conventional 130 and 143 exit codes.

The measurement also corrected a flaw in the test itself. The first run signalled
a wrapper PID while the real `run_all.sh` survived untouched, so "the trap did not
fire" was a false reading. Signalling the actual process gives the true behaviour:

```
before: 0
while running: 1  (pid 2910155)
TERM sent at t=2s while the 5s fixture is still in the foreground
t=4s, fixture still running: 1
t=9s, after the fixture completed: 0
[1]+  Exit 143
```

**The residual, stated precisely.** Cleanup is not instantaneous. Bash defers trap
handling until the current foreground command returns, so a `TERM` arriving
mid-fixture is honoured only once that fixture ends — visible above as the file
persisting at t=4s and gone at t=9s. And `SIGKILL` is uncatchable by design, so a
`timeout --kill-after` that escalates past a long fixture can still leave one
file behind. That is a property of POSIX signal handling, not a defect in the
cleanup: one `0600` file holding one fixture's output, in a directory that does
not survive reboot.

**A second finding, arrived at by accident.** Immediately after the trap change
both drivers reported failures — 23 in one, 6 in the other. The suites were fine;
my shell was not. An earlier probe had `export TMPDIR` to a scratch directory
which I then deleted, so `mktemp -d` failed, `WORK_ROOT` became the empty string,
and every sandbox path resolved to an absolute `/<name>`:

```
run_runner_contract.sh: line 116: /mixed/smackerel.sh: No such file or directory
run_runner_contract.sh: line 124: /mixed/tests/e2e/lib/helpers.sh: No such file or directory
  FAIL AC-01-1 — skip fixture is reported as SKIP
  FAIL AC-03-1 — SCN-061-014-03: passed count is 1
  Assertions failed: 23
```

Both drivers run under `set -uo pipefail` rather than `-e`, so the failed
`mktemp` was not fatal and the wall of downstream failures buried its own cause.
A driver that answers a missing `TMPDIR` with twenty-three unrelated assertion
failures is actively misleading, so both now check the root and say one thing:

```
$ TMPDIR=/nonexistent-dir-for-probe bash tests/e2e/runner_contract/run_runner_contract.sh
mktemp: failed to create directory via template '/nonexistent-dir-for-probe/...'
ERROR: could not create a sandbox root under /nonexistent-dir-for-probe
```

Clean runs are unaffected: 55/55 and 23/23.

## Security Phase

The classifier now reads text a fixture controls — `SKIP_REASON:` — and places it
into the results summary. That is a new data path from untrusted output into
operator-facing display, so it gets a real look rather than a formality.

**No command injection.** The reason is captured by command-substituting `sed`,
stored in a variable, interpolated into an array element, and echoed. Bash does
not re-evaluate variable contents, so substitution syntax in the reason is inert:

```
$ # fixture emits: SKIP_REASON: $(touch PWNED) `touch PWNED2` ;touch PWNED3
SKIP: hostile ($(touch /tmp/sec-probe/PWNED) `touch /tmp/sec-probe/PWNED2` ;touch /tmp/sec-probe/PWNED3)
--- did any injection fire? ---
NONE — no command substitution occurred
```

The reason renders literally, which is the desired outcome: hostile text is
visible rather than executed.

**A fixture cannot forge its own classification.** This is the property that
matters, because the whole packet is about the suite's colour being truthful.
Classification reads the exit code; printed text is informational only.

```
$ # fixture prints RESULT: SKIPPED and SKIP: forge, then exits 1
fixture exit code: 1
FAIL: forge (exit=1)
$ # fixture prints FAIL: everything is broken, then exits 0
PASS: forge2 (exit=0) — text ignored, correct
```

A failing fixture cannot disguise itself as a skip by printing skip markers, and
a passing one cannot be reddened by printing failure markers. Exiting 77 *is* the
declaration of a skip — that is the contract, and it requires actually exiting
77, which a fixture cannot do accidentally.

**Fixture output at rest is owner-only.** The temp file now holds fixture output
that may contain test-stack credentials. `mktemp` creates it `0600`:

```
$ f=$(mktemp); stat -c '%a %n' "$f"
600 /tmp/tmp.SgqbgBO7EG
```

Owner-only, and removed at the end of the fixture. The exposure window is one
fixture and the permission is correct.

**Considered, low severity.** A `SKIP_REASON` containing ANSI escape sequences
could distort the summary's rendering. It cannot alter classification, counts, or
the suite exit code, and the same fixture could already write escapes directly to
the terminal on the live-stream path that existed before this change. Recorded as
cosmetic rather than treated as a control.

## Full-Lane Phase — the all-lanes item found a regression I had introduced

This is the section the all-lanes DoD item exists to produce, and it did not
come back clean. The full shell E2E lane ran end to end and returned **exit 1**
with exactly one failure, and that failure was caused by this packet's own
Scope 1 change.

```
$ ./smackerel.sh test e2e
E2E_FULL_RC=1
  FAIL: test_timeout_process_cleanup.sh (exit=1)
  Total:  36
  Passed: 35
  Failed: 1
  Skipped: 0
```

The failing assertion is `BUG-031-004-SCN-001`, which exists to prove that
interrupting the runner terminates the child processes it started:

```
=== BUG-031-004-SCN-001: E2E interruption terminates child processes ===
Observed marker process for smackerel-e2e-timeout-cleanup-...-runner: 2340819
Interrupting nested E2E runner pid 2338010
Nested E2E runner did not exit after interruption
FAIL: nested E2E runner failed to exit after interruption
```

### Why it broke

To read a fixture's `SKIP_REASON` in the CLI classifier I had written:

```bash
e2e_run_child "$@" 2>&1 | tee "$capture"
status=${PIPESTATUS[0]}
```

`e2e_run_child` does not only run the child. It records the child's pid, its
process-group id and its run id in shell variables that the interrupt path later
reads to terminate that process group. A pipeline runs each element in a
subshell, so those assignments were written into a subshell that exited
immediately, and the parent was left holding nothing to kill.

Measured rather than reasoned about, because the claim is mechanical:

```
$ setter() { myglobal="CHILD_PID_12345"; echo "output from setter"; }
=== direct call (no pipeline) ===
output from setter
  after direct call, myglobal=[CHILD_PID_12345]
=== inside a pipeline (what my change introduced) ===
output from setter
  after pipeline, myglobal=[]
=== process substitution ===
  after process substitution, myglobal=[CHILD_PID_12345]
```

The third case is why process substitution is not the fix either. It preserves
the variable, but `output from setter` never appeared — the substitution is
asynchronous, so its output can be reordered or lost relative to the parent's.
Trading a cleanup guarantee for a display feature and then trading correct
output ordering to get the guarantee back is not a repair.

**This is the worst class of defect this packet has dealt with.** Classification
still looked entirely correct — every skip, pass and failure was reported
accurately, all 51 contract assertions passed, and the lane summary reconciled.
Nothing in the output said cleanup had been disarmed. Only a test that actually
interrupts a runner and looks for surviving children could see it.

### The fix, and what it costs

The CLI classifier no longer captures the child's output, so `e2e_run_child` is
called directly with no subshell of any kind. The now-unused `reason` parameter
is removed rather than left as a caller-less argument with a fallback message
saying `no SKIP_REASON emitted by X` — which would have been false, since the
fixture does emit one.

The cost is that the CLI's results line reads `SKIP: <name>` instead of
`SKIP: <name> (REASON)`. The reason is still on screen: the fixture streams it
live, a few lines above the summary. `run_all.sh` keeps full reason extraction
because its fixtures are separate processes with no shell state to lose, so its
pipeline is safe. The asymmetry is deliberate and commented at both sites.

Checked before choosing this: no contract assertion depends on CLI-side reason
text. `AC-04-1`, `AC-08-9` and `AC-08-10` all read `$RUNNER_OUT`, the
`run_all.sh` sandbox output; the CLI assertions `AC-02-*` check SKIP versus FAIL,
the tallies and the exit status. `docs/Testing.md` already attributes reason
reading to `run_all.sh` by name, so it needed no correction.

If the CLI summary should ever carry the reason, the sound design is for the skip
helpers to write it to a path passed in the environment, which both runners can
read without capturing anything. That is a change to the helper contract and to
every skip producer, so it is not folded into this packet.

### Contamination ruled out before accepting the result

A second lane was found running afterwards, which would have invalidated a
process-cleanup result. The timestamps show no overlap: `/tmp/e2e_full.txt` last
grew at `04:40:34` and `pgrep` returned `0` at `04:40:55`, while the other lane
first appeared at `04:42:35`. The failure was measured on an uncontaminated run.

### Focused post-fix verification

The exact cleanup carrier now passes after replacing the state-losing pipeline
with a direct `e2e_run_child` call. This is not a substitute for the full lane;
it proves the failed scenario and its adjacent Docker cleanup controls only.

```text
# BUG-061-014 focused interruption cleanup after direct-child repair
$ timeout 2700 ./smackerel.sh test e2e --shell-run test_timeout_process_cleanup.sh
exit: 0
lines: 40
sha256: 23d2a1218f91ed710a94d76cf49052ad3a32d7689b1bc7f3550547e4aae9da8a
PASS: BUG-031-004-SCN-002
Nested E2E runner returned nonzero after interruption: -1
Marker processes absent for smackerel-e2e-timeout-cleanup-21091-1787634992-runner
PASS: BUG-031-004-SCN-001
PASS: BUG-031-009-SCN-001
PASS: BUG-031-009-SCN-002
PASS: BUG-031-004 timeout process cleanup regression
Total:  1
Passed: 1
Failed: 0
Skipped: 0
```

Both classification contracts also pass with the direct-child call. The
failure-shaped lines in the first receipt are deliberate mutant fixtures inside
a contract command whose real exit is zero.

```text
# BUG-061-014 runner classification contracts after cleanup repair
$ timeout 2700 ./smackerel.sh test e2e --shell-run runner_contract/run_runner_contract.sh
exit: 0
lines: 621
sha256: 5b47a23a64bfdff8eddfcc1c113370e77cb11ade31cd284f8f0c7196e01ae065

# BUG-061-014 tier skip contracts after cleanup repair
$ timeout 2700 ./smackerel.sh test e2e --shell-run runner_contract/run_tier_skip_contract.sh
exit: 0
lines: 364
sha256: 609da684bd9a55860be16552b08fcd2e5464137949db49bb41a26bd7098c1cf0
```

The full-lane DoD remains open here. The earlier `35 passed / 1 failed` result
is retained above until an unfiltered `./smackerel.sh test e2e` completes green.

### The repair, verified on a clean lane

```
$ ./smackerel.sh test e2e
  Total:  36
  Passed: 36
  Failed: 0
  Skipped: 0
```

`test_timeout_process_cleanup.sh` passes, and with it the assertion that was
failing:

```
=== BUG-031-004-SCN-001: E2E interruption terminates child processes ===
Interrupting nested E2E runner pid 3859498
PASS: BUG-031-004-SCN-001
```

Two intermediate results along the way were NOT accepted as evidence, because
neither measured what it appeared to measure.

**A stuck port, not a defect.** The first verification run got past
`BUG-031-004-SCN-001` and then failed a later scenario, `BUG-031-009`. The cause
was in the log:

```
ERROR: Smackerel host port preflight timed out after 180s waiting for project-scoped port release.
Unavailable test port(s):
  - OLLAMA_HOST_PORT=47004 on 127.0.0.1:47004: [Errno 98] Address already in use
    owner: no process owner visible from /proc or docker ps
```

That port was debris from lanes I had force-killed while clearing the suite
lock. It was free minutes later, and `BUG-031-009` passes on the clean lane
above. Worth stating plainly: `BUG-031-009` had never been reached before,
because my regression aborted the fixture at `SCN-001` first. It was not
regressed by this packet; it was unmasked by fixing it.

**Four runs whose output I could not read.** A queue of `evidence-capture`
invocations ran the same checks while I was waiting. They completed, but
`evidence-capture` removes its temp file on exit and their output went to
terminals outside my reach. Citing a result I never saw would be exactly the
fabrication this packet keeps refusing, so the lane above is one I ran and
captured myself.

### One unrelated Go test flaked under load

The lane's overall exit was `1` despite the shell block being 36/36, from a Go
e2e test:

```
--- FAIL: TestQFDecisionSurfaceCardsRenderThroughLiveSearchAndArtifactDetail (2.01s)
    qf_decisions_surface_test.go:184: QF HTML surface missing "QF e2e surface thesis 1787638489659800753"
```

The artifact is submitted over NATS and the test then reads the HTML surface; the
log shows `connector artifact submitted for processing` immediately before, so the
read raced the processing. Three data points place it outside this packet:

| Run | Result |
|---|---|
| Lane before the cleanup repair | `PASS (2.14s)` |
| Lane after the cleanup repair, under full load | `FAIL (2.01s)` |
| Same test alone, `--go-run` | `PASS (2.72s)`, exit 0 |

And the change cannot reach it. Commit `213df4f0` touched `smackerel.sh` and this
`report.md` only, and the `smackerel.sh` hunk is inside the shell-fixture
classifier:

```
$ grep -rl 'e2e_record_shell_result\|e2e_run_shell_test' tests/ --include='*.go'
(no output — no Go test references either function)
```

Called what it is rather than smoothed over: a genuine load-dependent async race
in an unrelated package, pre-existing, and outside this packet's Change Boundary.

## Regression Phase

Four findings. The first is the one worth the phase existing.

### R1 — a document instructing the fix its own sibling section forbids

`docs/Testing.md` contained, as a normative instruction:

```
A 77 exit MUST be treated as PASS by any aggregating runner; the
structured `SKIP_REASON=...` record is the audit trail.
```

That is precisely `ADV-061-014-01`, the wrong fix this packet's adversarial case
exists to catch, written as guidance — and it sat in the SAME FILE as the
three-outcome contract stating that mapping `77` onto `PASS` is forbidden because
it recreates BUG-069-005. One document gave two opposite instructions.

This is also the likeliest explanation for how the original defect survived. An
engineer reaching for the documented rule would have implemented the false green.
Now:

```
A 77 exit MUST be classified `SKIP` by any aggregating runner — never
`PASS`, never `FAIL`. Mapping it onto `PASS` is forbidden: it reports an
unproven behaviour as a proven one, which is the false green BUG-069-005
was opened for.
```

### R2 — two sections, two different exit codes for one helper

The tier-gate table still read `| cpu | Emits SKIP: <name> line and exits 0 | 0 |`
and the prose still said "the test runner sees exit 0 on cpu hosts", while a
third section in the same file already said the helper "previously exited `0`".
Both now describe exit `77`, classified `SKIP`, counted in the `Skipped` tally.

### The drift is now asserted, not described

A related mismatch surfaced: the doc named `SKIP_REASON=cpu-tier-operator-defer-per-SCOPE-06c-PACKET-3`
while the helper emits `SKIP_REASON: CPU-TIER-HARDWARE-LACKS-ACCELERATOR` — a
different token AND a different separator, where the classifiers parse the colon
form.

Correcting the text alone would repeat the mistake the simplify phase already
diagnosed for the required sets: prose cannot hold a value that lives in code.
The tier driver now extracts the token from `helpers.sh` and asserts the doc
contains it, with the non-emptiness check first because `assert_contains` with an
empty needle passes vacuously:

```
  ok   DOC-1 — the helper emits a SKIP_REASON token (CPU-TIER-HARDWARE-LACKS-ACCELERATOR)
  ok   DOC-2 — docs/Testing.md documents the token the helper emits
  Assertions run:    25
  Assertions failed: 0
```

Proven to bite — worktree with the doc token replaced by a stale one:

```
  FAIL DOC-2 — docs/Testing.md documents the token the helper emits
  Assertions run:    25
  Assertions failed: 1
```

### R3 and R4 — foreign-owned, routed rather than edited

`specs/069-assistant-http-transport/bugs/BUG-069-006-.../state.json` states as
current fact that `bs_004` "is reported by the e2e runner as `FAIL: ... (exit=77)`"
and makes its unblocking condition "once ... bs_004 exits 0". Both premises are
now false: the runner reports `SKIP`, and `bs_004` is in neither required set, so
it contributes nothing to suite exit. Live DoD text in the parent
`specs/061-conversational-assistant/scopes.md` likewise asserts "exit 0" for the
tier helper in three places.

Both are outside this packet's Change Boundary and are recorded for their owners
rather than edited here. The closure rationale in each is unaffected; only the
mechanism description is stale.

### Searched and clean

- **Counter consumers:** no script, workflow or Go test parses the runners'
  `Passed:` / `Failed:` / `Total:` lines except this packet's own driver. `ci.yml`
  runs only `test unit` and `test integration`; `e2e-ui.yml` runs only
  `test e2e-ui`.
- **Required-set parity:** 36 entries each, symmetric difference empty, and the
  default lane arrays contain no tier-gated fixture — so no tier skip can redden
  the default lane.
- **BUG-069-005:** every hit in that packet concerns the Go lane (`t.Skipf`), not
  shell exit 77. The two packets are complementary.
