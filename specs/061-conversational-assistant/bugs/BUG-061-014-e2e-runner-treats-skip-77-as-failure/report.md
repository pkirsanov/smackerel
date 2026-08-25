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
