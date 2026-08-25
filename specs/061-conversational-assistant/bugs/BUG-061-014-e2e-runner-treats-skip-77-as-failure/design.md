# Design: BUG-061-014 — Root cause and fix directions

## Root cause

### The stated cause, and why it is incomplete

The immediate description is that `run_test()` lacks a branch for exit `77`. That
is true and it is not the root cause, because fixing only that leaves the same
defect intact in a second classifier and leaves ten fixtures reporting the
opposite lie.

### The actual root cause

**The runners' result vocabulary has two values; the outcome domain of a test
suite has three.**

A fixture can prove a behaviour, disprove it, or not run. Both classifiers encode
only the first two:

`tests/e2e/run_all.sh:41-47`

```bash
  if [ $exit_code -eq 0 ]; then
    RESULTS+=("PASS: $test_name"); PASSED=$((PASSED + 1))
  else
    RESULTS+=("FAIL: $test_name (exit=$exit_code)"); FAILED=$((FAILED + 1))
  fi
```

`smackerel.sh:1954-1968`

```bash
          if [[ "$status" -eq 0 ]]; then
            e2e_shell_results+=("PASS: ${test_name}"); return 0
          fi
          e2e_shell_results+=("FAIL: ${test_name} (exit=${status})")
          e2e_shell_failures=$((e2e_shell_failures + 1))
          if [[ "$e2e_overall_status" -eq 0 ]]; then
            e2e_overall_status="$status"
          fi
```

The state carried alongside is binary in the same way: `PASSED`/`FAILED` in
`run_all.sh`, `e2e_shell_failures` in `smackerel.sh`. There is nowhere for a third
outcome to be stored even if it were recognised.

### The forced-lie property

Because the domain is narrower than reality, an author of a skippable fixture
cannot choose an exit code that the runner will report truthfully. Both wrong
answers are present in this repository, authored independently:

| Helper | Location | Exit | Runner reports | Consumers | Failure mode |
|---|---|---|---|---|---|
| `reg_skip_with_blocker` | `tests/e2e/assistant_regression/lib/regression_helpers.sh:36-48` | `77` | `FAIL` | 7 | False red |
| `skip_unless_accel_tier` | `tests/e2e/lib/helpers.sh:169-185` | `0` | `PASS` | 10 | False green |

This is diagnostic. Two authors, working on different scopes, reached opposite
conclusions about how to signal the same thing, and both were misreported. That
is what a missing vocabulary value looks like from the outside — not a typo, but
a shape the interface cannot express.

### Why the convention was never implemented

`reg_skip_with_blocker` asserts the convention in prose:

```bash
# (consumable by the CI runner) and exits 77 — the Bubbles / shell
```

The parenthetical is the assumption. `grep -rn 'exit 77'` across the repository
returns only the producer and a comment; no runner code path reads `77`. The
convention has a writer and no reader. The fixture comments state that the slot
"is honored on every CI run so the skip itself is visible", which is half true:
the skip record does reach stdout, but the summary a reader scans overwrites its
meaning with `FAIL`.

### Why the summary loses the reason

Even where the skip is visible in the interleaved log, `RESULTS` stores only a
formatted string built from the name and exit code. The `SKIP_REASON` the fixture
emitted is never captured, so the results block — the part a reader trusts to
summarise the run — carries strictly less information than the log it summarises.

## Fix directions

### D1 — Introduce SKIP as a first-class outcome in both classifiers

Give each classifier a third branch and a third counter.

For `run_all.sh`: add `SKIPPED=0`, add a branch on `exit_code -eq 77` that appends
a `SKIP:` result and increments `SKIPPED`, and leave the `else` branch as the
failure path. `TOTAL` becomes `PASSED + FAILED + SKIPPED`, and the summary gains a
`Skipped:` line.

For `smackerel.sh`: add `e2e_shell_skips`, branch `e2e_record_shell_result` on
status `77` before the failure path, and — critically — do **not** assign `77`
into `e2e_overall_status` from that branch. Today the raw child status propagates,
which is why the lane exits `77`.

Both classifiers must implement the same rule. Divergence here means one fixture
has two reported outcomes depending on the entry point, which is the condition
that made this defect survive two independent code paths.

### D2 — Carry the reason into the results block

`reg_skip_with_blocker` already prints `SKIP_REASON: <token>` on stdout. Both
runners currently let the fixture's output stream straight through without
capture (`bash "$test_file" 2>&1` in `run_all.sh`). Capturing it while still
streaming it — the fixture's output must stay visible, per EB-4 — lets the
classifier extract the `SKIP_REASON` line and attach it to the result entry, so
the summary reads:

```text
  SKIP: bs_004_notification_confirm (SCOPE-04-NOTIFICATION-PROPOSAL-FIXTURE-NOT-YET-AUTHORED)
```

If capture proves awkward against the streaming requirement, the weaker
acceptable form prints the fixture path in the skip line so a reader can find the
record. The reason token is preferred because it is the thing that tells a reader
whether the skip is expected.

### D3 — Required versus optional, and what distinguishes them

This is the design question with the least obvious answer, so the reasoning is
recorded rather than only the conclusion.

**What the repository already knows.** Spec 061 SCOPE-10 DoD #7
(`specs/061-conversational-assistant/scopes.md:1347`) requires "one persistent
file per BS scenario" for BS-001..010. All ten slots are required by name. There
is no `scenario-manifest.json` under spec 061, and no manifest anywhere in
`specs/` names a shell fixture, so "required" cannot be resolved from manifest
data today.

**Three candidate sources for the distinction, and why two are rejected.**

1. *Infer from the fixture's own output.* `reg_skip_with_blocker` takes the BS id
   as its first argument, so a fixture already declares which scenario it covers.
   Rejected: this makes requiredness a property the test asserts about itself. A
   fixture could downgrade itself out of the required set by changing one string,
   which is the same self-certification weakness that lets a test skip itself
   silently.
2. *Infer from directory or filename.* Everything under
   `tests/e2e/assistant_regression/` is required. Rejected: it is implicit, it
   breaks the moment an optional fixture is placed there, and
   `assistant_acceptance_telegram_smoke.sh` already sits outside that directory
   while using the same helper.
3. *Declare it explicitly in the runner.* **Selected.** This matches the idiom the
   repository already uses for "this test is special": `LIFECYCLE_TESTS` in
   `run_all.sh:20`, and `e2e_lifecycle_scripts` / `e2e_shared_scripts` in
   `smackerel.sh:2036-2092`, are all explicit named arrays owned by the runner.
   Adding a `REQUIRED_TESTS` array is the same shape, is readable in one place,
   and cannot be altered by editing a fixture.

**The rule.** Classification and exit status are separated:

| Fixture exit | Classification | Counted in | Suite exit contribution |
|---|---|---|---|
| `0` | `PASS` | passed | none |
| `77`, fixture declared required | `SKIP` | skipped | non-zero |
| `77`, fixture not declared required | `SKIP` | skipped | none |
| any other non-zero | `FAIL` | failed | non-zero |

A required skip is reported as a skip and keeps the suite non-green. Both halves
matter. Reporting it as a skip is what makes the label honest; keeping the suite
non-green is what stops a required behaviour going unproven under a green badge —
the BUG-069-005 condition. Neither half is sufficient alone, and the common wrong
fix supplies only the first.

**Consequence, stated plainly.** Under this rule the six required BS slots that
skip today keep the suite's exit non-zero. That is the correct outcome: required
behaviour is genuinely unproven. What changes is that the suite now says
`SKIP: bs_004_notification_confirm (SCOPE-04-...-NOT-YET-AUTHORED)` instead of
`FAIL: ... (exit=77)`, so a reader learns the true reason rather than an invented
one. This packet corrects the label, not the coverage.

### D4 — Unify the two skip conventions

`skip_unless_accel_tier` should stop signalling a skip through `exit 0`. Its
current form is BUG-069-005's shape in shell: a structured message no runner
reads, followed by an exit code that reports success.

Once D1 lands, the correction is to exit `77` instead of `0` and keep the existing
`SKIP:` message. Its ten consuming fixtures then report `SKIP` rather than `PASS`.

This is a behavioural change for those ten fixtures and must be treated as one:
they move from silently green to visibly skipped, and whether any of them is
declared required under D3 determines whether the suite's exit status changes on
a `tier=cpu` host. That determination belongs in this packet because leaving it
undone means the false-green half of the defect survives a fix aimed at the
false-red half.

### D5 — Tests must drive the real classifier

Per ADV-4, runner-level tests invoke the actual runner against synthetic fixtures
with controlled exit codes and assert the resulting summary text and exit status.
A test that re-implements the branch logic in its own body would pass against a
runner that was never changed. The synthetic fixtures need three variants at
minimum — exit `0`, exit `77`, exit `1` — so that the exit-`1` control proves the
skip branch did not swallow real failures.

## Change boundary

### Included

| Path | Permitted change |
|---|---|
| `tests/e2e/run_all.sh` | Add the `SKIP` branch, the `SKIPPED` counter, the required-set declaration, and the summary line |
| `smackerel.sh` | Same, confined to `e2e_record_shell_result`, `e2e_print_shell_summary`, and the `e2e_overall_status` propagation they own |
| `tests/e2e/lib/helpers.sh` | `skip_unless_accel_tier` only — change its exit code per D4 |
| `tests/e2e/assistant_regression/lib/regression_helpers.sh` | Only if the emitted skip record needs a more machine-readable marker for D2; the `exit 77` value itself does not change |
| New runner-level test fixtures | New files under `tests/e2e/` for D5 |
| `specs/061-conversational-assistant/bugs/BUG-061-014-e2e-runner-treats-skip-77-as-failure/` | This packet's own artifacts |
| `docs/Testing.md` | Record the three-outcome contract and the required-set rule |

### Excluded

| Path | Reason |
|---|---|
| `.github/bubbles/**` | Framework-managed install artifacts. Refreshed only through the Bubbles installer or upgrade command, never patched locally. |
| The bodies of the seven `reg_skip_with_blocker` fixtures | Their declared assertion shapes are correct; authoring their executed branches is spec 061 SCOPE-04/06/07 work |
| `internal/**`, `cmd/**`, `ml/**` | No product code participates in this defect |
| `config/**`, `config/generated/**` | No configuration value participates in this defect |
| `specs/069-assistant-http-transport/**` | Another packet's artifacts; `DIS-069-006-4` is that packet's to close |
| Every other `specs/**` directory | Not this packet's artifacts |
| `smackerel.sh`'s lane arrays (`e2e_lifecycle_scripts`, `e2e_shared_scripts`) | Wiring the assistant fixtures into the default lane is spec 061 SCOPE-10 work; this packet only makes that wiring safe |

## Risk

| Risk | Assessment |
|---|---|
| The obvious wrong fix maps `77` to the pass path | Directly guarded by ADV-1. This is the highest-likelihood wrong turn because it makes the red line disappear, which superficially resembles success. |
| The skip branch is written as "any non-zero except failures we recognise" | Guarded by ADV-3's exit-`1` control. Only `77` is a skip. |
| D4 changes ten fixtures from green to visibly skipped | Intended, and it will look like a regression in the summary on `tier=cpu` hosts. It is a reporting correction, not new breakage: those fixtures were already not running. The corrected summary is the first time that fact is legible. |
| The two classifiers drift again | Mitigated by giving both the same rule and testing both. A shared helper is not available across the `smackerel.sh` / `tests/e2e/` split without new coupling, so the rule is duplicated deliberately and both copies are covered by AC-1 and AC-2. |
| Capturing fixture output for D2 suppresses live streaming | EB-4 forbids it. If capture and streaming cannot both be had cleanly, D2's weaker form is taken and the reason stays in the log rather than the summary. |
