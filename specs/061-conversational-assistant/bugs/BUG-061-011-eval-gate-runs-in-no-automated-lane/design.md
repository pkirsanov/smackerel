# Bug Fix Design: BUG-061-011 — Assistant acceptance gate executes in no automated lane

## Root Cause Analysis

### Investigation Summary

Investigated in the filing session against commit `6ad1e8c98ce6e1aa35df779306c6a8835db172be`. The four implicated files are unmodified in the working tree, so every finding holds for both trees. Raw output is in [`report.md`](report.md).

The investigation traced four questions:

1. **Is the gate excluded from the untagged lane?** Yes. `tests/eval/assistant/acceptance_test.go:1` is `//go:build integration`. `scripts/runtime/go-unit.sh:67` runs `go test ./...` with no `-tags`, so the file is not compiled there. Its untagged package-mates (`corpus_validation_test.go`, `harness_test.go`) *are* compiled and do run in that lane — the package is reached, only the gate file is filtered out. This is the detail that makes the defect survive casual inspection: `./tests/eval/assistant` is not an unreached corner of the tree.

2. **Is the gate reached by the tagged lane?** No. `scripts/runtime/go-integration.sh:53` enumerates packages explicitly:
   `go_test_args+=(./tests/integration/... ./internal/notification/... ./internal/assistant/... ./internal/cardrewards/...)`.
   `grep -nE 'tests/eval|eval'` against that file exits `1` with zero matches. The package is never passed to `go test`, so the tag never gets a chance to match.

3. **Does any other lane run it?** No. `go-stress.sh:50` is scoped to `./tests/stress/readiness`. `tests/e2e/assistant_regression_e2e_test.sh` mentions the gate only in `echo` lines — `grep -nE 'go test|tests/eval|assistanteval|TestAcceptanceGate'` against that script matches nothing but `echo`. A repository-wide `grep -rn "TestAcceptanceGate"` returns the definition, documentation, spec transcripts, and those `echo` lines. There is no invoker.

4. **Does anything report an executed-assertion count?** No — and this is the finding that changes the size of the fix. See § *What the harness actually reports today*.

### Root Cause

**Two mechanisms were introduced with complementary intent and no link between them, and nothing asserts the link.**

`acceptance_test.go` opted *out* of the untagged lane deliberately — its header explains the reasoning: *"Build tag `integration` keeps it out of the default `go test ./...` pass so corpus development doesn't fight the gate."* That reasoning is sound. It then states the other half as settled fact: *"CI invokes `./smackerel.sh test integration` which sets `-tags integration` and the gate then runs."*

The first clause of that sentence is true. `.github/workflows/ci.yml:241` does invoke `./smackerel.sh test integration`, and `go-integration.sh:48` does set `-tags integration`. The conclusion does not follow, because the lane selects packages by an **explicit allow-list**, not by `./...`. A tag can only include a file inside a package that was already selected. The opt-out was executed; the opt-in was assumed.

The defect is therefore not a typo and not a deletion. It is a **missing invariant**. Two files hold two halves of one contract, neither references the other, and no test, lint, or gate observes the pair. The comment asserting the wiring is the only "link" that exists, and a comment cannot fail.

That is also why it stayed invisible: the failure mode of a missing test is silence. There is no error output. `go test` is not asked about a package it was not given, so it reports nothing about it, and the lane exits `0`.

### Why three surfaces repeat the false claim

The same unverified assumption propagated into documentation written from the source rather than from an observed run:

- `tests/eval/assistant/acceptance_test.go:15-17` — the origin of the claim.
- `docs/Testing.md:748` — presents `./smackerel.sh test integration --go-run TestAcceptanceGate_RoutingAccuracyAndCaptureFallback` as *"The standard invocation"*. `--go-run` is forwarded to `go-integration.sh` as `--run` (`smackerel.sh:1249-1260`), which becomes `go test -run <regex>` **within the four listed package trees**. The gate's package is not among them. `docs/Testing.md:755` gives the direct `go test -count=1 -tags integration -run ... ./tests/eval/assistant/...` form, which names the package explicitly — that invocation does work, and is the one spec 061's historical PASS evidence (`report.md:5399`, `:5705`) actually used.
- `tests/e2e/assistant_regression_e2e_test.sh:251-253` — narrates R10-3 as running in the integration lane. The script prints; it does not execute.

This matters for the fix design because it identifies **what the fix must make true**, not merely what it must add. Adding the package to the list retroactively makes claims 1 and 3 true. Claim 2 becomes true as well — but only for a selector that matches the gate, which introduces a new subtlety handled under § *The focused-run interaction*.

### Impact Analysis

- **Affected components:** `scripts/runtime/go-integration.sh` (lane), `tests/eval/assistant/` (gate, harness, corpus), `.github/workflows/ci.yml` (consumer of the lane), `docs/Testing.md` and `tests/e2e/assistant_regression_e2e_test.sh` (stale claims).
- **Affected data:** none. No runtime, schema, migration, or user-visible surface is touched. Blast radius is test tooling only.
- **Affected users:** none directly. The exposure is to *assurance*: assistant routing quality can degrade below the SST floors with no signal.
- **Delivery impact:** `docs/Product_Delivery_Plan.md` Stage 1 *Done when* requires *"the integration run reports the assistant acceptance gate with a non-zero executed-assertion count"*. No such count is emitted anywhere today, so the criterion is unmet by construction and Stage 1 cannot close.
- **Scope limit on the claim:** this gate measures assistant routing accuracy and capture-fallback coverage on an offline corpus, using the harness's deterministic keyword classifier as a structural proxy for the production router (`harness.go:1-14`). It does **not** measure corpus-grant enforcement. The D25/D28 grant holes carried by spec 108 are unaffected and must not be described as becoming measurable through this fix.

---

## What the harness actually reports today

The delivery plan's second instruction — *"assert in CI that the gate reported a non-zero executed-assertion count"* — presumes a reporting surface. The investigation found none. This section records what exists, because the fix must be designed against it rather than against an invented API.

**What exists.** `FormatReport(HarnessResult)` (`harness.go`) produces a multi-line human report containing the numbers that matter:

```
  total rows:                150
  intent correct:            150
  capture-expected rows:     60
  capture-fallback hits:     60
```

**Why it cannot be asserted on as-is — three independent reasons:**

1. **It is conditional on passing.** `acceptance_test.go` ends with
   `if !t.Failed() { fmt.Println(report) }`.
   The report is printed only when the gate passes; on failure it is embedded inside the `t.Errorf` message instead. So the presence of the report cannot distinguish "gate ran and failed" from "gate never ran" — which is precisely the discrimination R3 requires.
2. **It is prose, not a record.** Asserting on `total rows:` would mean grepping a human-formatted, space-padded line whose only contract is that a person can read it. Any cosmetic edit to `FormatReport` silently breaks the assertion, and a broken assertion in a guard fails *open*.
3. **There is no aggregate count.** `Total` and `CaptureExpected` are separate numbers. "Executed assertions" is not currently a named quantity anywhere in the repository.

**Conclusion:** the executed-assertion count does not exist today in any form, machine-readable or otherwise. The minimal emission required is specified below. This is the substantive half of the fix and it is not covered by the delivery plan's two-file estimate.

### Defining the count from fields that already exist

The harness already computes everything needed; only the aggregate is missing.

```
executedAssertions = HarnessResult.Total + HarnessResult.CaptureExpected
```

Justification: `Run()` evaluates one routing assertion for every corpus row (`intentOK := pred.Intent == r.GroundTruthIntent`, incrementing `Total`), and one capture assertion for every row whose `GroundTruthCaptureExpected` is true (incrementing `CaptureExpected`). The sum is exactly the number of ground-truth comparisons the run performed. On the shipped corpus: `150 + 60 = 210` (verified this session — 150 `id:` rows, 60 `ground_truth_capture_expected: true`).

Both summands are existing exported fields on an existing exported type. No new reporting API is invented; one derived accessor is added.

**Critically, the count can be zero.** `Run()` on a corpus with no rows returns `Total == 0` and `CaptureExpected == 0`. That is what makes R3.2 a real assertion rather than a decorative one, and R4 requires it be proven by test rather than argued here.

---

## Fix Design

### Solution Approach

Four coordinated changes. The first is the plan's stated fix; the second through fourth are what make it non-reversible.

#### Change 1 — Emit the count (`tests/eval/assistant/harness.go`)

Add two small, untagged, unit-testable functions beside the existing report formatter:

```go
// ExecutedAssertions reports how many ground-truth comparisons the run
// performed: one routing assertion per corpus row plus one capture
// assertion per capture-expected row. Zero means the gate evaluated
// nothing.
func ExecutedAssertions(r HarnessResult) int { return r.Total + r.CaptureExpected }

// GateMarkerPrefix is the fixed literal the integration lane greps for.
const GateMarkerPrefix = "ASSISTANT_ACCEPTANCE_GATE_V1"

// FormatGateMarker renders the single-line machine record the lane asserts on.
func FormatGateMarker(r HarnessResult) string { /* ... */ }
```

Marker shape — one line, fixed prefix, `key=value` pairs:

```
ASSISTANT_ACCEPTANCE_GATE_V1 executed_assertions=210 rows=150 capture_expected=60 routing_accuracy=1.0000 capture_fallback_rate=1.0000
```

These live in `harness.go` rather than in the test file for a specific reason: `harness.go` carries **no build tag**, so the marker's construction is unit-testable in the default `go test ./...` lane. Putting the logic inside the `integration`-tagged `acceptance_test.go` would make the marker itself only testable by the very lane whose correctness is in question.

#### Change 2 — Print it unconditionally (`tests/eval/assistant/acceptance_test.go`)

Emit the marker **before** the threshold comparisons, so it is produced on pass and on fail alike:

```go
r := Run(c)
fmt.Println(FormatGateMarker(r))   // unconditional — precedes any t.Errorf
report := FormatReport(r)
// ... existing threshold assertions unchanged ...
```

The existing `if !t.Failed() { fmt.Println(report) }` block stays as-is; it serves human evidence capture and is orthogonal.

`fmt.Println` rather than `t.Logf`: it matches the file's existing style, and it emits an unindented line at column zero, which keeps the lane's grep anchorable to `^` and immune to `go test -v` indentation changes.

Also in this change: correct the header comment (lines 14–17). After Change 3 the claim becomes true, but it should state *what makes* it true — the package list, not the tag alone — so the next reader does not repeat the inference that produced this bug.

#### Change 3 — Run it and assert on it (`scripts/runtime/go-integration.sh`)

Two edits.

**3a. Add the package** to line 53:

```bash
go_test_args+=(./tests/integration/... ./internal/notification/... ./internal/assistant/... ./internal/cardrewards/... ./tests/eval/...)
```

**3b. Assert the gate actually ran.** The script runs under `set -euo pipefail`, so the assertion must be structured to survive a `go test` failure rather than being short-circuited by `set -e`:

- Capture the stream with `tee` to a `mktemp` file so console output still streams live. The temp file is created outside `/workspace` and removed by a `trap`, so no untracked file appears in the repository tree.
- Record `go test`'s exit code without letting `set -e` abort first (`if ! ...; then rc=$?; fi` or an explicit `|| rc=$?`).
- Then evaluate the marker:
  - count lines matching `^ASSISTANT_ACCEPTANCE_GATE_V1 `;
  - **zero** matches → fail, naming the gate and stating that it did not run;
  - **more than one** match → fail as ambiguous (a second marker means either a duplicate run or a fabricated line; either way the count is not trustworthy);
  - exactly one → parse `executed_assertions=<N>`; `N` non-numeric or `N < 1` → fail, quoting the marker line.
- Exit non-zero if **either** the `go test` rc or the marker check failed (R3.5). Neither may mask the other.

#### The focused-run interaction

This is the subtlety the two-file framing hides, and getting it wrong makes the fix either useless or hostile.

`go-integration.sh` already accepts `--run <regex>` (from `./smackerel.sh test integration --go-run ...`). If the marker assertion ran unconditionally, then any focused run that legitimately selects some *other* integration test would fail the lane simply because the gate was filtered out. That would break a documented, useful workflow.

Rule: **enforce the marker assertion when `$go_run_selector` is empty.** CI's invocation (`ci.yml:241`) carries no selector, so CI is always covered, and the Stage 1 criterion is enforced exactly where it is claimed.

The hazard this creates is that `--go-run '.*'` becomes a shape that skips the guard. Three mitigations, all required:

1. When the assertion is skipped, print a loud, single-line notice naming what was not enforced — R5.2. Silence here would recreate this very bug in a new form.
2. The contract test (Change 4) asserts the guard is keyed on the **empty-selector** condition specifically, and that the notice literal is present. A future edit broadening the skip condition fails a test.
3. **No skip flag, no environment bypass.** No `SKIP_*`, `--no-verify`, `--force`, or equivalent may be introduced. The only non-enforcement path is the focused-run path, and it is visible in the output.

#### Change 4 — Make the regression detectable (`internal/deploy/eval_lane_contract_test.go`, new)

A static-file contract test that reads `scripts/runtime/go-integration.sh` and `tests/eval/assistant/acceptance_test.go` and asserts the pairing holds. This is the file the delivery plan's estimate omits, and it is the only part of the fix that prevents recurrence.

Placement in `internal/deploy` follows established repository precedent — `envsubst_wrapper_contract_test.go`, `assistant_e2e_package_contract_test.go`, and `ci_integration_topology_contract_test.go` all read `scripts/runtime/*` or workflow files and assert invariants over them, with adversarial sub-tests. The package is untagged, so these tests run in the unit lane (`go test ./...`), satisfying R6.4: **the guard does not depend on the lane it guards.** A guard living inside the integration lane could be disabled by the same edit that disables the gate.

Contract signals asserted over `go-integration.sh`:

- `./tests/eval/...` appears in the package list;
- the marker prefix literal `ASSISTANT_ACCEPTANCE_GATE_V1` appears;
- a non-zero comparison on the parsed count appears;
- the enforcement is conditioned on the empty run-selector;
- the skip notice literal appears;
- no bypass token (`SKIP_`, `--force`, `--no-verify`, `INSECURE_`) appears.

Contract signals asserted over `acceptance_test.go`:

- `FormatGateMarker` is called;
- the call is **not** inside the `if !t.Failed()` block.

### Adversarial regression design

R6.5 forbids a tautological fixture set. Each adversarial case below is a mutation that **passes under the broken condition** and must make the contract function return an error. Following the `assistant_e2e_package_contract_test.go` pattern, the contract is factored into a pure `assertEvalLaneContract(lane, gate string) error` so fixtures can be supplied as string literals with no filesystem dependency.

| # | Case | Fixture mutation | Must |
|---|------|------------------|------|
| A0 | Green baseline | Both files as fixed | **pass** |
| A1 | **The original bug** | Lane fixture with `./tests/eval/...` removed from the package list | fail |
| A2 | Package present, assertion absent | Lane fixture keeps the package but drops the marker check entirely | fail |
| A3 | Assertion accepts zero | Lane fixture greps the marker but compares `-ge 0` instead of `-ge 1` | fail |
| A4 | Conditional emission | Gate fixture calls `FormatGateMarker` **inside** `if !t.Failed()` | fail |
| A5 | Emission removed | Gate fixture never calls `FormatGateMarker` | fail |
| A6 | Bypass introduced | Lane fixture guards the assertion behind `[[ -z "${SKIP_EVAL_GATE:-}" ]]` | fail |
| A7 | Skip condition broadened | Lane fixture enforces only when an env var is set, rather than when the selector is empty | fail |

A1 is the direct anti-regression for this bug: it is the current file content, and the contract must reject it. A2 and A3 exist because "the package is in the list" is *not* the property that matters — a package can be present and still contribute zero executed assertions if the tag stops matching, the corpus fails to load, or every case is skipped. A4 is the mutation that would quietly re-open the pass/fail ambiguity described above.

Complementary harness-level tests in `tests/eval/assistant/harness_test.go` (untagged, unit lane):

| Test | Asserts |
|------|---------|
| `TestExecutedAssertions_CountsRoutingPlusCaptureRows` | small fixture corpus → count equals rows + capture-expected rows |
| `TestExecutedAssertions_ZeroOnEmptyCorpus` | **R4** — empty corpus yields exactly `0`, proving the lane's `≥ 1` check is not vacuous |
| `TestFormatGateMarker_SingleLineParseableWithPrefix` | one line, fixed prefix at column zero, `executed_assertions` parses back to the same integer |

`TestExecutedAssertions_ZeroOnEmptyCorpus` is the anti-tautology proof for R3.2. Without it, "assert count ≥ 1" could be a check on a quantity that is positive by construction — which would be a decorative gate protecting a decorative gate.

### Alternative Approaches Considered

1. **Add `./tests/eval/...` to the package list and stop there (the literal two-file reading).** Rejected. It satisfies R1 but not R2, R3, R4, or R6. The Stage 1 criterion explicitly demands a reported non-zero count, and none is emitted today. It also leaves the invariant unguarded: the identical regression could land again in one line with nothing turning red — which is how the bug arose in the first place.

2. **Assert on `--- PASS: TestAcceptanceGate_...` in the `-v` stream instead of a marker.** Rejected as *insufficient alone*, though it is a reasonable belt-and-braces addition. It proves the test ran, but not how many assertions it evaluated — a gate reduced to a single trivially-true comparison would still print `--- PASS`. The delivery plan asks for a count, and a count is what distinguishes "ran" from "measured something".

3. **`go test -json` parsed by a helper.** Rejected on cost/benefit. It gives the most rigorous run/skip/pass discrimination, but converts the whole integration lane's console output to JSON, degrading the readability of an already long log, and requires a parser to maintain. The single-line marker achieves the required discrimination for this gate at a fraction of the surface.

4. **Sentinel receipt file written by the test, checked by the lane.** Rejected. It is robust against output-format drift, but requires a new SST env key for the path (touching `scripts/commands/config.sh` and `config/smackerel.yaml`, both under strict zero-defaults policy), and risks writing an artifact into `/workspace`. Cost exceeds the benefit over the marker.

5. **Remove the `//go:build integration` tag so `go test ./...` picks the gate up.** Rejected. The tag exists for a stated, sound reason — keeping the accuracy gate out of the fast unit loop during corpus development. Removing it would make every corpus edit fight the threshold gate, which is the friction the tag was added to prevent.

6. **Add the gate to the e2e shell script that already narrates it.** Rejected. It would put a Go test behind a shell wrapper for no benefit, and `tests/e2e/assistant_regression_e2e_test.sh` is a documentation-shaped scaffold, not a runner. The correct correction there is to make its R10-3 prose true (which Change 3a does), not to give it an execution role.

---

## Honest size assessment

`docs/Product_Delivery_Plan.md` § P3 records **"Files (2)"** and **"Size: the smallest high-value fix on this list."** That estimate was tested against the code during this investigation and is **understated**. It is accurate for the *wiring* half and omits the *enforcement* half — which the plan's own "Exact change" sentence names as the second requirement.

| # | File | New? | Why required |
|---|------|------|--------------|
| 1 | `scripts/runtime/go-integration.sh` | no | Package list + marker assertion + focused-run rule (plan file 1) |
| 2 | `tests/eval/assistant/acceptance_test.go` | no | Unconditional marker emission + correct the stale header claim (plan file 2) |
| 3 | `tests/eval/assistant/harness.go` | no | `ExecutedAssertions` + `FormatGateMarker` — untagged so the marker is unit-testable; **the count does not exist today in any form** |
| 4 | `tests/eval/assistant/harness_test.go` | no | R4 proof that the count can be `0`, plus marker-format tests |
| 5 | `internal/deploy/eval_lane_contract_test.go` | **yes** | R6 adversarial regression — the only thing preventing recurrence |

**Realistic size: 5 files, 1 new.** Files 3 and 4 could be collapsed into file 2 at the cost of making the marker logic testable only by the `integration`-tagged lane, which weakens R4 to an untested claim. File 5 cannot be collapsed anywhere without violating R6.4, because a guard hosted inside the integration lane is disabled by the same edit that disables the gate.

Documentation follow-ups (`docs/Testing.md` for R7.2; optionally the R10-3 prose in `tests/e2e/assistant_regression_e2e_test.sh`, which Change 3a makes true) would extend this further.

The characterisation *"the smallest high-value fix on this list"* still holds in effort terms — this remains a small, low-blast-radius change with no runtime, schema, or user-visible surface. The specific **"Files (2)"** number does not.

---

## Complexity Tracking

| Decision | Simpler fix considered | Why rejected |
|----------|------------------------|--------------|
| Emit a machine-readable marker line | Grep the existing human `FormatReport` output for `total rows:` | The report prints only when the gate passes, so its presence cannot distinguish "failed" from "never ran" — exactly the discrimination R3 requires. It is also human-formatted prose with no stability contract, so a cosmetic edit would break the guard silently, and a broken guard fails open. |
| Add `ExecutedAssertions` / `FormatGateMarker` to untagged `harness.go` | Inline both in `acceptance_test.go` | `acceptance_test.go` is `//go:build integration`, so the marker would be testable only by the lane whose correctness is under question. Keeping it untagged puts R4's zero-count proof in the unit lane. |
| New `internal/deploy/eval_lane_contract_test.go` | Trust the lane edit to persist | The root cause *is* an unasserted invariant between two files. Repeating the pattern without a guard reproduces the bug's preconditions exactly. Follows existing precedent (`envsubst_wrapper_contract_test.go`, `assistant_e2e_package_contract_test.go`). |
| Enforce the marker assertion only on empty `--run` selector | Enforce unconditionally | Would break every legitimate focused integration run by failing the lane whenever the selector excluded the gate. Mitigated by a loud skip notice, a contract assertion on the skip condition, and a hard no-bypass rule. |
| Fail on more than one marker line | Accept the first match | Two markers mean either a duplicate run or an injected line; in both cases the parsed count is not attributable to a single verified run. Ambiguity in a guard should refuse, not guess. |
| Preserve `go test`'s exit code alongside the marker check | Let `set -e` abort on `go test` failure | Under `set -euo pipefail` an aborted script never reaches the marker check, so a run that both failed and skipped the gate would report only the first problem. R3.5 requires neither failure mask the other. |
