# Spec: BUG-061-011 — Assistant acceptance gate must execute in an automated lane

## Purpose

Define the behaviour the repository's test tooling MUST exhibit so that the assistant acceptance gate cannot silently stop running. This is the specification the fix is measured against; it is not a description of current behaviour.

Scope boundary: this specification governs **execution and reporting of the gate**. It does not change the gate's thresholds, the corpus, or the harness classifier, and it makes no claim about corpus-grant enforcement (spec 108, D25/D28), which is a different axis of assurance.

### Single-Capability Justification

Gate G094's proportionality trigger fires on this packet, so this section records why one capability is the correct shape here and why a full domain-capability model would be ceremony.

**What actually triggered the gate.** All four trigger hits are the same word — *connector* — and not one of them is in this file or in `design.md`. They are the substring inside the path `internal/connector/` in `scopes.md`, at four places that each say the opposite of a capability family:

| `scopes.md` line | What the line says |
|---|---|
| 29 | lists `internal/connector/` among the trees this packet does **not** touch |
| 259, 289 | pasted lane transcript: `ok … /internal/connector/youtube … [no tests to run]` |
| 933 | again lists `internal/connector/` among the untouched trees |

Two exclusion lists and two lines of captured `go test` output. The gate matched a substring in evidence prose. No connector, adapter, provider, or driver is introduced, extended, or varied by this packet.

**The single capability.** *An existing acceptance gate is bound to an automated lane, and its execution is proved by a measured count instead of asserted by a comment.* R1–R7 are seven requirements on that one capability — where it runs (R1), what it emits (R2), when the lane refuses (R3), that the measurement can genuinely be zero (R4), how a focused run behaves (R5), that the binding is guarded (R6), and that stale prose is corrected (R7). Seven requirements on one capability, not seven capabilities.

**Why no variation axis exists at the spec level.** An axis needs two or more members a caller could choose between. This specification names exactly one of each: one gate (`TestAcceptanceGate_RoutingAccuracyAndCaptureFallback`), one lane that must run it, one marker vocabulary (a single fixed prefix, versioned `V1`), and one enforcement rule.

R5's full-lane/focused-run distinction is the nearest candidate, and it is deliberately not an axis: R5.1 and R5.4 describe two invocation modes of the *same* rule, and R5.3 forbids the focused mode from becoming a selectable bypass. A caller chooses a test selector; the rule's response to that choice is fixed and unconfigurable.

**What would change this.** A second gate needing the same binding — another corpus, or an equivalent floor in another lane — would make *which gate, which lane, which floor* a real axis and would justify promoting this section to a domain capability model. No second member exists today, and R2.3's single-fixed-prefix contract would have to be reopened first. Modelling the family now would be abstraction ahead of its second member.

## Definitions

| Term | Definition |
|------|-----------|
| **The gate** | `TestAcceptanceGate_RoutingAccuracyAndCaptureFallback` in `tests/eval/assistant/acceptance_test.go` (build tag `integration`). |
| **The integration lane** | `./smackerel.sh test integration` → `scripts/runtime/go-integration.sh`, the lane CI runs at `.github/workflows/ci.yml:241`. |
| **Full-lane run** | An integration-lane invocation with no `--go-run` / `--run` selector. This is what CI runs. |
| **Focused run** | An integration-lane invocation carrying a `--go-run` / `--run` selector. |
| **Executed-assertion count** | The number of corpus assertions the gate actually evaluated in a run, defined as `HarnessResult.Total + HarnessResult.CaptureExpected` — one routing assertion per corpus row plus one capture assertion per capture-expected row. Both fields already exist in `tests/eval/assistant/harness.go`. On the shipped corpus this is `150 + 60 = 210`. |
| **Vacuous pass** | A lane invocation that exits `0` without the gate having evaluated at least one assertion. |

## Requirements

### R1 — The gate executes in the integration lane

**R1.1** A full-lane run MUST compile and execute the package `./tests/eval/assistant`.

**R1.2** A full-lane run MUST execute the gate with `-tags integration`, against the shipped corpus, with the SST threshold variables `ASSISTANT_EVAL_ROUTING_ACCURACY_MIN` and `ASSISTANT_EVAL_CAPTURE_FALLBACK_MIN` present in the environment.

**R1.3** The gate's existing behaviour MUST NOT change: it still reads both floors via `os.Getenv`, still calls `t.Fatalf` with `"SST contract violation: <KEY> is empty"` when either is missing, and still fails when either measured rate falls below its floor.

### R2 — The run reports an executed-assertion count

**R2.1** The gate MUST emit its executed-assertion count in a single-line, machine-parseable form on every execution.

**R2.2** The emission MUST be **unconditional** with respect to the gate's own verdict. It MUST be produced when the gate passes and when the gate fails. It MUST NOT be nested inside the existing `if !t.Failed()` block, because a pass-only emission cannot distinguish "gate failed" from "gate never ran".

**R2.3** The emission MUST carry, at minimum, the executed-assertion count, the corpus row count, the capture-expected row count, and the two measured rates. It MUST be greppable by a fixed literal prefix that no other line in the lane's output produces.

**R2.4** The count MUST be derived from the harness result of the run that produced it. A constant, a hard-coded literal, or a value read from configuration does not satisfy R2.

### R3 — A vacuous pass fails loudly

**R3.1** A full-lane run MUST fail (non-zero exit) if no executed-assertion emission is present in its output.

**R3.2** A full-lane run MUST fail if the emitted executed-assertion count is `0`.

**R3.3** R3.1 and R3.2 MUST hold independently of the Go test verdict. Specifically, the lane MUST fail when `go test` exits `0` but the gate did not run. This is the exact shape of the present defect and is the condition R3 exists to catch.

**R3.4** The failure message MUST name the gate and the missing or zero count explicitly, so an operator reading only CI output can act without reading source.

**R3.5** When `go test` itself fails **and** the emission is absent or zero, the lane MUST still exit non-zero. Neither failure may mask the other into a pass.

### R4 — The count is capable of being zero

**R4.1** The executed-assertion count MUST be a genuine measurement, provably able to take the value `0` for an input that evaluates nothing. A count that is positive by construction makes R3.2 decorative.

**R4.2** R4.1 MUST be demonstrated by an executable test, not asserted in prose.

### R5 — Focused runs remain usable and cannot become a bypass

**R5.1** A focused run MUST remain able to select a subset of integration tests without the R3 assertion failing the lane merely because the selector excluded the gate.

**R5.2** When the R3 assertion is not enforced for a focused run, the lane MUST print an explicit, unmissable notice stating that the acceptance-gate assertion was not enforced for this invocation.

**R5.3** The R3 assertion MUST NOT be suppressible by any environment variable, flag, or file whose purpose is to skip it. No `SKIP_*`, `--no-*`, `--force`, `--insecure`, or equivalent may exist. The only condition under which R3 is not enforced is R5.1, and that condition MUST be visible in the output per R5.2.

**R5.4** CI's invocation is a full-lane run and therefore MUST always be subject to R3.

### R6 — Regression cannot recur silently

**R6.1** An executable test MUST fail if `./tests/eval/...` (or an equivalent selector covering `./tests/eval/assistant`) is removed from the integration lane's package list.

**R6.2** An executable test MUST fail if the lane's R3 assertion is removed or weakened to accept a missing emission or a zero count.

**R6.3** An executable test MUST fail if the gate's emission is made conditional on the gate passing.

**R6.4** The tests satisfying R6.1–R6.3 MUST run in a lane that is itself automated, and MUST NOT depend on the integration lane they are protecting.

**R6.5** Each test in R6.1–R6.3 MUST be adversarial: it MUST be demonstrated against a fixture that reproduces the broken condition and MUST fail on that fixture. A fixture set in which every case passes under the broken condition is tautological and does not satisfy R6.

### R7 — Stale claims are corrected

**R7.1** No surface in the repository may assert that the gate runs automatically unless R1 holds. Specifically, the header comment in `tests/eval/assistant/acceptance_test.go` and the R10-3 prose in `tests/e2e/assistant_regression_e2e_test.sh` MUST be true after the fix or MUST be corrected.

**R7.2** `docs/Testing.md` MUST document the R3 assertion and the R5 focused-run behaviour, so an operator who sees the notice from R5.2 knows what it means.

## Acceptance Criteria

| ID | Criterion | Satisfied when |
|----|-----------|----------------|
| AC-1 | Gate runs in CI | A full-lane run compiles `./tests/eval/assistant` and executes the gate |
| AC-2 | Count reported | The full-lane output contains exactly one executed-assertion emission with a value ≥ 1 |
| AC-3 | Absent emission fails | A lane run whose output lacks the emission exits non-zero with a named error |
| AC-4 | Zero count fails | A lane run reporting `executed_assertions=0` exits non-zero with a named error |
| AC-5 | Count can be zero | A test proves the count evaluates to `0` for an empty input |
| AC-6 | Package removal detected | Removing `./tests/eval/...` from the lane fails an automated test outside the integration lane |
| AC-7 | Assertion removal detected | Removing or weakening the lane's R3 assertion fails an automated test |
| AC-8 | Conditional emission detected | Moving the emission back inside `if !t.Failed()` fails an automated test |
| AC-9 | No bypass | No skip flag or environment bypass exists for R3 |
| AC-10 | Stage 1 criterion met | `./smackerel.sh test integration` reports the assistant acceptance gate with a non-zero executed-assertion count, per `docs/Product_Delivery_Plan.md` Stage 1 Done-when |
| AC-11 | Claims true | The three surfaces named in `bug.md` § *Three compounding false claims* are true after the fix, or corrected |

## Non-Goals

- Changing `assistant.eval.routing_accuracy_min` or `assistant.eval.capture_fallback_min`. Lowering either is explicitly a non-negotiable acceptance regression per `config/smackerel.yaml:1351-1353`.
- Changing the corpus, its size, or its label distribution.
- Changing the harness classifier's rules or its accuracy on the shipped corpus.
- Replacing the deterministic keyword classifier with a live LLM router. The harness is a structural gate by design.
- Anything about corpus-grant enforcement (spec 108, D25/D28). Out of scope and not made measurable by this work.

## Traceability

| Source | Reference |
|--------|-----------|
| Delivery plan item | `docs/Product_Delivery_Plan.md` § P3 (`D27` · Critical · Stage 1) |
| Stage 1 exit criterion | `docs/Product_Delivery_Plan.md` Stage 1 *Done when* block |
| Product direction row | `docs/Product_Direction_2026-07-31.md:230` |
| Gate under protection | `tests/eval/assistant/acceptance_test.go:41` |
| Threshold SST | `config/smackerel.yaml:1351-1353` |
| Owning feature | `specs/061-conversational-assistant/` SCOPE-10 |
