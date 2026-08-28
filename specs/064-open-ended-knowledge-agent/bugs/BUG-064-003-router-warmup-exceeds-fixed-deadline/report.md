# Report: BUG-064-003

**Bug:** [bug.md](bug.md) · **Spec:** [spec.md](spec.md) · **Design:** [design.md](design.md) · **Scopes:** [scopes.md](scopes.md)

> **Reading order.** The *Summary* and *Completion Statement* immediately below record the **filing session**, which was artifacts-and-analysis only by explicit operator direction. They are kept verbatim as the historical record and are **superseded** by the *Status update* block that follows them. The fix has since been implemented and verified at the integration and unit tiers.

---

### Summary

Bug filed with artifacts and root-cause analysis only. No fix was implemented,
by explicit operator direction. Scope 1 is Not Started and every Definition-of-Done
checkbox in [scopes.md](scopes.md) is unchecked.

The defect: `TestOpenKnowledgeRouting_FallbackToOpenKnowledge` wraps
`agent.NewRouter` in a hard-coded 30-second deadline, while `NewRouter` performs
79 sequential `POST /embed` round-trips against an ML sidecar whose
sentence-transformer may still be cold. The test's verdict is therefore a
function of sidecar warmth rather than of the SCOPE-12 routing contract it
exists to assert.

---

### Completion Statement

**NOT COMPLETE — and not claimed to be.** This session performed discovery,
reproduction-evidence review, and root-cause analysis. It produced six artifacts.
It changed no source file, ran no fix, and executed no integration lane.

Status is `in_progress` with `execution.currentPhase: "analysis"`: analysis is
finished, implementation has not begun. Promotion to any terminal status is not
warranted and is not requested.

---

### Status update — supersedes the two sections above

The fix has since been implemented and verified (working tree at HEAD
`3af96a02`, **uncommitted**). Three statements in the two sections above are now
false; they are left in place as the historical record and corrected here.

| Statement above | Corrected status |
|-----------------|------------------|
| "No fix was implemented" | A fix landed: an embedder-readiness gate ahead of the timed region, a router-construction budget derived from the embed-call count, fail-loud SST routing reads, and three new SST keys. |
| "Scope 1 is Not Started" | Scope 1 is **In Progress**. Implementation has landed; certification has not. |
| "every Definition-of-Done checkbox in scopes.md is unchecked" | **11 of 15** DoD checkboxes are checked, each carrying its own inline raw-output evidence. Was 10 of 15; the owner-decision item was recorded and checked on 2026-08-28 (see the session block below). |

### Session 2026-08-28 — lanes executed, and what they do NOT establish

Four commands were run against this tree state. Each block below is a verifiable
`evidence-capture.sh` receipt: the hash is re-derivable with `--verify`, which a
pasted transcript could never be.

| Command | Exit | sha256 (full output) |
|---|---|---|
| `./smackerel.sh test unit` | 0 | `bfb8433f35b5412a330f8b6863625bb66b3671144c2a457d1c4e7ee799d0fcee` |
| `./smackerel.sh test integration --go-run TestOpenKnowledgeRouting` | 0 | `750215b878a8fd8a09bd5b1a49c6943ce3030efd4748e9a2198cc5565a30b277` |
| `./smackerel.sh test integration --go-run TestOpenKnowledgeRouting` (re-run, wider window) | 0 | `fe9a9ee312a8c7cc5736cd137b5d04e10bdef6d5f908cc56bfa2c42ed2d2f39c` |
| `./smackerel.sh check && ./smackerel.sh lint && ./smackerel.sh format --check` | 0 | `769a440c8916a474cf29ca238cf9e8d512eb37af35006f81937fe1fd31b568e6` |

`artifact-lint.sh` on this packet also exits 0.

**What these receipts do NOT establish, stated plainly rather than left to be
inferred from a green exit code.** The focused integration runs exit 0, but exit
0 does NOT prove the three `TestOpenKnowledgeRouting_*` functions EXECUTED:
`go test -run` also exits 0 when the selector matches nothing. Both receipts
bound their output, and the go-integration section fell inside the omitted
region in each, so the `=== RUN` / `--- PASS` lines were never observed. T1 and
T2 are therefore recorded as **not proven**, not as passing. They are also not
failing: no failure-shaped line appeared in either receipt.

To close that gap, a future run should escalate to
`bash .github/bubbles/scripts/evidence-capture.sh --diagnostic -- ./smackerel.sh test integration --go-run 'TestOpenKnowledgeRouting'`,
which is the sanctioned full-transcript escalation and stamps the block so the
escalation is visible to a reviewer.

**Separate finding from the same session, applying to the whole repository.** The
UNFOCUSED lane fails: `./smackerel.sh test integration` exits 1 with
`ERROR: config-generate-time validation failed for env=test` and
`FAIL github.com/smackerel/smackerel/tests/integration 66.335s`
(receipt `7680fb96f151c091193a358de5ed76f9b5f97f6266d76afc0a4cdf1c3b38abae`).
That failure is in the ROOT `tests/integration` package, where
`tests/integration/config_validate_test.go` lives — a DIFFERENT package from
`tests/integration/agent`, which holds T1/T2. It is not attributable to this
session: every commit in it touched only `specs/` and `.specify/`, verified with
`git diff --name-only`. Recording it here because a red lane changes what any
later "integration passes" claim on this packet can mean.

**Claim Source: executed** — both lanes were executed in a prior pass and
preserved to logs. The line numbers and quoted text below were read in this
documentation pass from the recorded evidence blocks in [scopes.md](scopes.md);
neither lane was re-run here and neither log was re-read from disk here.

#### Integration tier — lane green, formerly-failing test now passes

**Command:** `./smackerel.sh test integration` — preserved at
`~/s064-integration.log` (8218 lines). Lane result `INTEGRATION_EXIT=0`, with
**zero** `--- FAIL` / `FAIL github` / `FAIL:` lines anywhere in the file.

```
3552:    openknowledge_routing_test.go:127: embedder warm-up gate passed: probes=1 latencies=[#1=464ms] qualifying=464ms elapsed=464ms
3554:    openknowledge_routing_test.go:149: router construction budget: embed_calls=79 per_call_budget=2s (AGENT_ROUTING_BUILD_PER_CALL_BUDGET_MS) derived_build_budget=2m38s
3556:    openknowledge_routing_test.go:186: query="weather in paris today" → weather_query (top_score=1.000, reason=similarity_match)
3558:    openknowledge_routing_test.go:186: query="explain quantum entanglement briefly" → open_knowledge (top_score=0.248, reason=fallback_clarify)
3560:    openknowledge_routing_test.go:186: query="what is 10F in C" → open_knowledge (top_score=0.763, reason=similarity_match)
3561:--- PASS: TestOpenKnowledgeRouting_FallbackToOpenKnowledge (11.99s)
3594:ok      github.com/smackerel/smackerel/tests/integration/agent  20.238s
7925:ASSISTANT_ACCEPTANCE_GATE_V1 executed_assertions=210 rows=150 capture_expected=60 routing_accuracy=1.0000 capture_fallback_rate=1.0000
8218:INTEGRATION_EXIT=0
```

`TestOpenKnowledgeRouting_FallbackToOpenKnowledge` now completes in `11.99s`
against the two pre-fix aborts of `32.14s` and `33.40s` quoted in *Test
Evidence* below. The evidence region spans log lines 3552–3570. The readiness
gate qualified on probe #1 at 464 ms **before** the timed region, and
construction then ran under a `2m38s` budget derived from `embed_calls=79`
rather than the old flat 30 s literal — so all three SCOPE-12 routing
assertions were reached and evaluated instead of the run dying inside the
warm-up loop.

#### Unit tier — green, with an exact-marker caveat

**Command:** `./smackerel.sh test unit --go --go-run 'BUG064003|RouterWarmup|Warmup' --verbose` — preserved at `~/s064-unit.log` (564 lines).

```
128:ok      github.com/smackerel/smackerel/internal/agent   1.682s
564:[go-unit] go test ./... finished OK
```

No `--- FAIL` or `FAIL github` line appears anywhere in that file. **State the
marker precisely: `~/s064-unit.log` contains no `UNIT_EXIT=` line.** The runner
did not write one, so no unit-lane exit code is quoted or asserted here. The
success evidence is exactly the runner's own terminator at line 564 plus the
package verdict at line 128 — nothing is inferred beyond them.

#### What remains open — 5 of 15 DoD items

Each is unchecked in [scopes.md](scopes.md) with its full reason recorded
inline there. Summarised, **not** restated as resolved:

| Unchecked DoD item | Recorded reason (summary) |
|--------------------|---------------------------|
| **T2** integration: unready embedder produces an explicit readiness failure | The green lane never exercised the branch, because the embedder **was** ready. The behaviour is proven at the `unit` tier only, so the row's declared `integration` tier has no evidence. |
| Pre-fix state: T3 and T4 FAIL against the unmodified tree | No RED-phase log exists. The guards were never run against a checkout without the fix, so their non-tautology is not demonstrated by execution. |
| Post-fix state: T1–T4 PASS | **3 of 4**, not 4 of 4. T1, T3 and T4 are checked on executed evidence; T2 is not. A composite tick would silently promote a 3-of-4 result. |
| Build Quality Gate (`check`, `lint`, `format --check`, artifact lint, docs aligned) | None of those gates was executed against this tree state, and no preserved log for them exists for this bug. |
| Owner decision on [design.md](design.md) § 5 question 1 (does the no-defaults policy bind test code?) | Still an open question with no recorded answer anywhere in this packet. The fix made it **non-blocking** by deleting the forbidden helper shape outright, but did not answer it. |

**Certification is not claimed.** Status remains `in_progress`. Promotion to any
terminal status is owned by `bubbles.validate` and has not happened.

#### Artifact set update

`uservalidation.md` — flagged as a known omission in *Artifacts Created* at the
end of this report — now exists in this bug folder. Its absence was the sole
`artifact-lint.sh` failure for this directory. `scenario-manifest.json` is still
absent and remains a known gap; sibling bugs BUG-064-001 and
BUG-CHAOS-20260605-001 carry one.

[scopes.md](scopes.md) contains two statements describing **this** report as
stale — its reviewer note near the top, and a sub-bullet under the Build Quality
Gate item. Both were accurate when written and are superseded by this block.
`scopes.md` is owned by `bubbles.plan` and was outside this pass's permitted
edit set, so they are flagged here rather than edited.

---

### Test Evidence

No tests were executed in this session. The operator captured the failure
evidence in a prior session and directed that
`./smackerel.sh test integration` (~20 minutes) not be re-run. All failure
output below is quoted verbatim from the operator's preserved logs.

**Claim Source: not-run** — for every test in the [scopes.md](scopes.md) Test
Plan (T1–T4). None of T1–T4 exists yet; they are planned regression coverage for
a fix that has not been written.

#### Preserved evidence — Run A, full lane (`~/bug011-integration-a9.log`)

**Claim Source:** executed — read from the preserved log this session at the
line numbers shown.

```
3964:=== RUN   TestOpenKnowledgeRouting_FallbackToOpenKnowledge
3965:    openknowledge_routing_test.go:128: build router: agent: NewRouter: embed scenario "recipe_search" intent_examples[4]: sidecar.Embed: POST /embed: Post "http://smackerel-ml:8081/embed": context deadline exceeded
3966:--- FAIL: TestOpenKnowledgeRouting_FallbackToOpenKnowledge (32.14s)
3967:=== RUN   TestOpenKnowledgeRouting_ScenarioHealthProbe
3968:--- PASS: TestOpenKnowledgeRouting_ScenarioHealthProbe (0.06s)
3969:=== RUN   TestOpenKnowledgeRouting_RelativeAGENT_SCENARIO_DIRResolvesAgainstRepoRoot
3973:--- PASS: TestOpenKnowledgeRouting_RelativeAGENT_SCENARIO_DIRResolvesAgainstRepoRoot (0.00s)
3996:FAIL    github.com/smackerel/smackerel/tests/integration/agent  39.561s
```

#### Preserved evidence — Run B, focused re-run (`~/bug011-flake-check.log`)

**Claim Source:** executed — read from the preserved log this session.

```
289:go-integration: applying -run selector: TestOpenKnowledgeRouting
293:=== RUN   TestOpenKnowledgeRouting_FallbackToOpenKnowledge
294:    openknowledge_routing_test.go:128: build router: agent: NewRouter: embed scenario "hospitality_concern_evaluate" intent_examples[1]: sidecar.Embed: POST /embed: Post "http://smackerel-ml:8081/embed": context deadline exceeded
295:--- FAIL: TestOpenKnowledgeRouting_FallbackToOpenKnowledge (33.40s)
296:=== RUN   TestOpenKnowledgeRouting_ScenarioHealthProbe
297:--- PASS: TestOpenKnowledgeRouting_ScenarioHealthProbe (0.04s)
302:--- PASS: TestOpenKnowledgeRouting_RelativeAGENT_SCENARIO_DIRResolvesAgainstRepoRoot (0.01s)
307:FAIL    github.com/smackerel/smackerel/tests/integration/agent  33.930s
480:FAIL: go-integration (exit=1)
```

Two consecutive runs, two different invocations, both `exit=1`, same failure
mode — but aborting at **different** points in the same warm-up loop. That
divergence is what makes the root cause derivable rather than guessed.

---

### Root-Cause Evidence

#### Embed-call census

**Claim Source:** executed — `python3` with `yaml.safe_load` over
`config/prompt_contracts/*.yaml`, sorted by scenario id to match the sort
`agent.NewRouter` itself applies (`internal/agent/router.go:151`).

```
  alert_timing_evaluate                3  cum=3
  annotation_classify                  4  cum=7
  e2e_ollama_smoke                     1  cum=8
  expertise_classify                   3  cum=11
  hospitality_concern_evaluate         3  cum=14
  notification_schedule                4  cum=18
  open_knowledge                       8  cum=26
  recipe_search                        9  cum=35
  recommendation_feedback              3  cum=38
  recommendation_reactive              3  cum=41
  recommendation_watch_evaluate        3  cum=44
  recommendation_why                   2  cum=46
  relationship_cooling_evaluate        3  cum=49
  resurface_evaluate                   3  cum=52
  retrieval_evergreen                  3  cum=55
  retrieval_qa                         5  cum=60
  weather_query                       19  cum=79

TOTAL sequential /embed calls in NewRouter = 79
recipe_search[4] is call # 31
hospitality_concern_evaluate[1] is call # 13
```

Derived from those ordinals and the ~30 s deadline:

| Run | Ordinal reached | Completed | Per-call latency | Budget the full 79 would need |
|-----|-----------------|-----------|------------------|-------------------------------|
| A | 31 | 30 | ≈1.0 s | ≈79 s |
| B | 13 | 12 | ≈2.5 s | ≈197 s |
| earlier pass | 79 | 79 | ≤0.38 s | <30 s |

**Claim Source: interpreted** — the ordinals and the 30 s deadline are measured;
the per-call latencies and required budgets are arithmetic derived from them.
The ~30 s router-build window is approximated by subtracting the probe embed and
scenario load from the 32.14 s / 33.40 s totals.

This is the correction to the initial "overshoots by 2–3 seconds" read: the
wall-clock overshoot is small only because the deadline truncates the work.
62% (run A) to 85% (run B) of the embed set was still outstanding at expiry.

#### The SST contradiction

**Claim Source:** executed — read from `config/smackerel.yaml:1628`.

```
config/smackerel.yaml:1628:    embed_timeout_ms: 30000 # REQUIRED: per-call timeout for the sidecar /embed roundtrip. Spec 064 SCOPE-17 raised from 500ms to accommodate cold sentence-transformer load on first startup; subsequent calls return in <50ms.
```

Production allocates 30 s for **one** cold embed call. The test allocates 30 s
for **79** of them, and separately caps each individual call at 5 s
(`sidecar.New(sidecarURL, token, 5*time.Second)`, line 89) — one sixth of the
SST value that is present in the container's own env-file
(`config/generated/test.env:489  ASSISTANT_ROUTING_EMBED_TIMEOUT_MS=30000`).

---

### Not-A-Flake / Not-Caused-By-The-Concurrent-Fix Evidence

1. **Not transient.** Reproduced 2/2 on consecutive runs (evidence blocks above),
   same failure mode, both `exit=1`.

2. **Not caused by BUG-061-011.** Those edits were fully present on disk (written
   21:17) when an earlier full integration run at ~21:29 exited 0 with this same
   test passing. Identical product code, different outcome ⇒ environmental /
   timing, not causal. **Claim Source: interpreted** — from operator-supplied file
   timestamps and run history, not re-verified this session.

3. **Ordering rules it out independently.** BUG-061-011 added `./tests/eval/...`
   to the lane's package list. `scripts/runtime/go-integration.sh` runs
   `go test -p 1 …` with that package listed last, and the full-lane log confirms
   the resulting order empirically:

   **Claim Source:** executed — read from the preserved log this session.

   ```
   3906:ok      github.com/smackerel/smackerel/tests/integration            121.566s
   3996:FAIL    github.com/smackerel/smackerel/tests/integration/agent       39.561s
   4010:ok      github.com/smackerel/smackerel/tests/integration/annotation   0.098s
   ...
   5073:ok      github.com/smackerel/smackerel/tests/integration/web          0.480s
   8429:ok      github.com/smackerel/smackerel/tests/eval/assistant           0.142s
   ```

   The failing package completes ~4400 log lines before the added package begins.

---

### Finding 2 Evidence — hard-coded constants vs the no-defaults policy

**Claim Source:** executed — all four reads below performed this session.

Production is already fail-loud:

```
internal/agent/config.go:155:            ConfidenceFloor: requireFloat("AGENT_ROUTING_CONFIDENCE_FLOOR", 0, 1),
internal/agent/config.go:156:            ConsiderTopN:    requireInt("AGENT_ROUTING_CONSIDER_TOP_N", 1),
```

The test is not:

```
tests/integration/agent/openknowledge_routing_test.go:119:  ConfidenceFloor:    parseFloatEnv("AGENT_ROUTING_CONFIDENCE_FLOOR", 0.65),
tests/integration/agent/openknowledge_routing_test.go:120:  ConsiderTopN:       parseIntEnv("AGENT_ROUTING_CONSIDER_TOP_N", 5),
tests/integration/agent/openknowledge_routing_test.go:217:func parseFloatEnv(key string, fallback float64) float64 {
tests/integration/agent/openknowledge_routing_test.go:229:func parseIntEnv(key string, fallback int) int {
```

The values are genuinely SST-managed and reach the container:

```
config/generated/test.env:484:AGENT_ROUTING_CONFIDENCE_FLOOR=0.65
config/generated/test.env:485:AGENT_ROUTING_CONSIDER_TOP_N=5
config/generated/test.env:489:ASSISTANT_ROUTING_EMBED_TIMEOUT_MS=30000
```

But the repository's mechanical SST guard deliberately excludes this file twice:

```
internal/config/sst_grep_guard_test.go:13://   - `*_test.go`, `*_test.py`, `ml/tests/` — test fixtures with explicit
internal/config/sst_grep_guard_test.go:75:    if strings.HasSuffix(path, "_test.go") {
internal/config/sst_grep_guard_test.go:82:    if strings.Contains(path, string(filepath.Separator)+"tests"+string(filepath.Separator)) {
```

**Determination recorded in [design.md](design.md) § 2, in summary:**

- `parseFloatEnv(..., 0.65)` / `parseIntEnv(..., 5)` — match the policy's
  forbidden shape ("any helper that silently supplies a runtime fallback value")
  and are the exact shape `BUG-020-003` and `BUG-020-008` removed from
  production; but they sit in a path the guard intentionally does not scan, so
  they are a policy-spirit and drift finding, **not** a gate violation. They are
  also inert today because `test.env` supplies matching values. Whether the
  prose binds test code is left as an owner decision, not asserted here.
- `30*time.Second` — **not** a no-defaults violation. It is not fallback syntax
  and shadows no SST key. The case against it is a design argument, made on the
  cold-start evidence, not a policy citation.
- `5*time.Second` per-call — **not** a no-defaults violation either, but a real
  divergence from `ASSISTANT_ROUTING_EMBED_TIMEOUT_MS=30000`. Not what failed
  here (no individual call exceeded 5 s in either run); a latent second failure
  mode.

---

### Uncertainty Declarations

1. **The ~30 s router-build window is an approximation.** Totals of 32.14 s and
   33.40 s include the probe embed and scenario load. The per-call latencies
   (≈1.0 s, ≈2.5 s) and required budgets (≈79 s, ≈197 s) inherit that
   approximation. They are the right order of magnitude and are more than
   sufficient to reject Option C, but they are not instrumented measurements.

2. **The "earlier pass at ~21:29" is operator-reported.** It was not re-verified
   this session. It is load-bearing for the not-caused-by-BUG-061-011 argument,
   though argument 3 (package ordering, verified from the log) reaches the same
   conclusion without it.

3. **The 79-call census reflects the working tree at HEAD `3af96a02`.** Adding
   any `intent_examples` entry raises it and silently erodes the margin, which is
   itself part of the defect.

4. **Whether the no-defaults prose binds test code is genuinely unresolved.**
   The prose has no carve-out; the guard has an explicit one. This report states
   both and declines to resolve it. See [design.md](design.md) § 5 question 1.

   **RESOLVED 2026-08-28 under operator delegation.** The answer recorded in
   [design.md](design.md) § 5 item 1 is YES with a distinction: the policy binds
   test code, but forbids the fallback FORM, not explicit test values. The
   guard's own rationale supports the narrow reading — `sst_grep_guard_test.go:13`
   exempts *"test fixtures with explicit intent to use deterministic test
   values"* — while its mechanism (a `_test.go` suffix skip at line 75, pinned by
   a meta-test at 285-310) is broader than that rationale and also exempts silent
   substitution. The residual gap is filed as **R-012** in
   [.specify/memory/open-work.md](../../../../.specify/memory/open-work.md),
   owner `bubbles.design`. The sentence above is left standing as the historical
   record rather than deleted.

---

### Invocation Audit

No subagents were invoked. The operator scoped this task to artifact creation
and root-cause analysis with an explicit instruction not to implement a fix, so
the `bugfix-fastlane` implementation phases (`bubbles.implement`, `bubbles.test`,
`bubbles.validate`, `bubbles.docs`) were correctly not dispatched.

`bubbles.design` and `bubbles.plan` were likewise not dispatched; `design.md` and
`scopes.md` were authored directly under the operator's explicit
artifacts-only instruction. This is recorded rather than omitted: under a normal
full `bugfix-fastlane` run those two artifacts are owned by those specialists,
and a later delivery-mode run should treat them as drafts subject to their
owners' review.

---

### Artifacts Created

| Artifact | State |
|----------|-------|
| `bug.md` | complete |
| `spec.md` | complete |
| `design.md` | complete |
| `scopes.md` | complete — as authored, Scope 1 Not Started with all DoD unchecked. SUPERSEDED: Scope 1 is now In Progress with 11 of 15 DoD checked (see the correction table above); this row records the artifact's state at creation, not its state today |
| `report.md` | this file |
| `state.json` | complete — `in_progress`, phase `analysis` |

Not created: `uservalidation.md` and `scenario-manifest.json`. Sibling bugs in
this spec carry `uservalidation.md` (all three) and `scenario-manifest.json`
(BUG-064-001, BUG-CHAOS-20260605-001). The operator scoped this task to exactly
six artifacts, so the omission is deliberate and is flagged here rather than
silently absorbed; it will need closing before this bug can pass artifact lint
or reach a terminal status.
