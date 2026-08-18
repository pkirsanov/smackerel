# BUG-061-009 — Report

## Summary

Closes the last "saved as an idea" masking path: a band-high (matched + executed)
turn that cannot ground an answer is refused HONESTLY (`StatusUnavailable` +
`ErrNoGroundedAnswer` + "I don't have a sourced answer for that.") instead of the
misleading capture acknowledgement. Converts "saved as an idea" into a
band-LOW-only invariant (INV-HB-REFUSAL) with a cross-path invariant test so the
class cannot recur, and upgrades refusal-vs-answer distinguishability from a
user-visible "(saved as idea)" string to a structural (`Status`/`ErrorCause`)
contract.

## Completion Statement

SCOPE-01..05 implemented. INV-HB-REFUSAL is enforced end-to-end: the provenance
gate refuses OK-but-uncited turns into the honest `StatusUnavailable` +
`ErrNoGroundedAnswer` shape; `canonicalizeSuccessfulCaptureResponse(resp, band, …)`
emits the capture ack for `BandLow` only (band-high residual `StatusSavedAsIdea`
is converted to the honest refusal); the open_knowledge refusal renderer dropped
its `(saved as idea)` suffix and the 5 cause strings were reworded, making
refusal-vs-answer distinguishability structural. The dead
`provenance.EnforceRefusal` parallel refusal path was removed and the live
refused-turn path (facade source-assembler Override) is now the single
refusal-shaping path, directly unit-tested. Full Go suite + lint green.

## Test Evidence

### `./smackerel.sh test unit --go` — first run (3 EXPECTED failures pre-fix)

The whole module compiled; `internal/assistant` and `internal/assistant/provenance`
(the core changed packages) passed on the first run. Three expected failures
remained where ratified guards had to be updated for the new honest shape:

```
ok      github.com/smackerel/smackerel/internal/assistant       0.266s
--- FAIL: TestOpenKnowledgeAssembler_RefusedEnvelope (0.00s)
    wiring_assistant_openknowledge_test.go:152: refused body mismatch: ""
FAIL    github.com/smackerel/smackerel/cmd/core 1.098s
--- FAIL: TestAllErrorCauses_Exhaustive (0.00s)
    response_test.go:53: AllErrorCauses length 7 != declared 6
--- FAIL: TestGoldenCases_CoverEveryCombinationAxis (0.00s)
    response_test.go:468: ErrorCause "no_grounded_answer" not covered by any golden fixture
FAIL    github.com/smackerel/smackerel/internal/assistant/contracts     0.035s
ok      github.com/smackerel/smackerel/internal/assistant/provenance    0.020s
ok      github.com/smackerel/smackerel/internal/telegram        27.574s
ok      github.com/smackerel/smackerel/internal/telegram/assistant_adapter      0.026s
ok      github.com/smackerel/smackerel/internal/whatsapp/assistant_adapter      (cached)
```

The three failures were the update points, not regressions: (1) the existing
assembler refused-test asserted the pre-fix `SourceAssembly{Body,Cause}` shape —
updated to assert the honest `Override` (multi-cause); (2) the closed-vocabulary
`ErrorCause` count needed `ErrNoGroundedAnswer` (6→7); (3) a golden snapshot for
the new `no_grounded_answer` fixture was created.

### `./smackerel.sh test unit --go` — second run (all green, exit 0)

```
ok      github.com/smackerel/smackerel/cmd/core 0.935s
ok      github.com/smackerel/smackerel/internal/assistant/contracts     0.043s
=== exit: 0 ===
```

Every package passed (exit 0); the two previously-failing packages (`cmd/core`,
`internal/assistant/contracts`) are green, and no other package regressed.

### `./smackerel.sh lint` (Go vet + ruff + web assets)

```
Building wheels for collected packages: smackerel-ml
Successfully built smackerel-ml
Installing collected packages: … smackerel-ml
Successfully installed … smackerel-ml-0.1.0 …
All checks passed!
```

### `./smackerel.sh check` (SST / config / scenario-lint)

```
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 17, rejected: 0
scenario-lint: OK
```

## Artifact-lint

`bash .github/bubbles/scripts/artifact-lint.sh specs/061-conversational-assistant/bugs/BUG-061-009-…`

```
✅ Required artifact exists: spec.md / design.md / uservalidation.md / state.json / scopes.md / report.md
✅ Found DoD section in scopes.md
✅ All DoD bullet items use checkbox syntax in scopes.md
✅ Detected state.json status: in_progress
✅ report.md contains section matching: Summary / Completion Statement / Test Evidence
=== Anti-Fabrication Evidence Checks ===
✅ All checked DoD items in scopes.md have evidence blocks
✅ No unfilled evidence template placeholders in scopes.md
✅ No unfilled evidence template placeholders in report.md
✅ No repo-CLI bypass detected in report.md command evidence
Artifact lint PASSED.
=== artifact-lint exit: 0 ===
```

## Grounding-gap diagnosis and routing (SCOPE-05)

**Question:** why did `/ask how smackerel works as second brain or llm wiki?`
ground nothing (agent `status=success termination=final`, but zero sources →
refusal)?

**Evidence (config + observed behavior):**
- The open_knowledge agent's `tool_allowlist` is
  `[ internal_retrieval, web_search, unit_convert, calculator ]`
  (`config/smackerel.yaml`). So grounding a meta-question requires EITHER
  `web_search` returning citable pages OR `internal_retrieval` finding ingested
  smackerel-about-smackerel content.
- On the self-hosted env, `searxng_enabled: "${ENABLE_SEARXNG}"`
  (`config/smackerel.yaml` `environments.<target>`) is operator-gated — **and
  on `<deploy-host>` `searxng` IS running** (observed
  `smackerel-<target>-searxng-1` healthy, 30h uptime at deploy time). So
  `web_search` HAS a working provider; a disabled tool is NOT the cause here.
- A question about smackerel's own product almost certainly has **no ingested
  source** in the user's knowledge graph (the user has not captured smackerel's
  own docs), so `internal_retrieval` returns nothing.
- With no citable source, the agent synthesized from the LLM's own weights; the
  citeback verifier correctly rejected the uncited claims → empty sources → the
  provenance gate refused. **This is correct anti-fabrication.** The BUG-061-009
  fix makes that refusal *honest* ("I don't have a sourced answer for that.")
  instead of the misleading "saved as an idea."

**Conclusion:** the refusal itself is correct; the gap is that the user wants an
*answer*. Since `searxng` IS running, a disabled web tool is NOT the cause — the
meta-question still grounded nothing, which needs a deeper investigation, not a
facade change:
1. determine why `web_search` produced no accepted source for this query —
   either the agent did not select `web_search` for a product meta-question, or
   `searxng` returned no citable pages, or the citeback verifier rejected them;
   and/or
2. ingest smackerel's own product docs so `internal_retrieval` can answer
   meta-questions; and/or
3. a product decision to let `open_knowledge` answer general-knowledge questions
   with an explicit "unsourced / general knowledge" caveat (weakens
   `requires_provenance` — needs owner sign-off).

**Routing decision (completed).** The grounding gap was routed to
`BUG-061-010-open-knowledge-grounding-gap`, which exists at
`specs/061-conversational-assistant/bugs/BUG-061-010-open-knowledge-grounding-gap/`
(`bug.md` + `state.json`) and owns the three investigation lines above. That bug
is the owner of record for making `/ask` answer.

This paragraph previously read "(to be created)". That was true when written and
is false now, so it is corrected rather than preserved: a routing claim that
names an artifact which does not exist is indistinguishable from no routing at
all, and the correction is a truth repair, not a softer wording. BUG-061-009 owns
exactly one thing — the honest-refusal invariant (INV-HB-REFUSAL). It does NOT
claim to make `/ask` answer; it claims the refusal is now honest.

## Deploy Evidence (local-operator on `<deploy-host>`, `<target>`)

Built, operator-cosign-signed, and applied on `<deploy-host>` per the BUG-061-008
recipe. Source SHA `2e84a1b4`.

**Build (`./smackerel.sh build --target <target>`) — exit 0:**

```
[4/7] docker push (capture stable digests)
  core: ghcr.io/<operator>/smackerel-core@sha256:dc8963683bc87f6d07b5460a009755c8cd400b5dd56ae20d0f3094307e570c32
  ml:   ghcr.io/<operator>/smackerel-ml@sha256:ef16adc279b908b3777e9afbf9558dbbdfe9b8611b741c12cccc5d39db4c1b23
[5/7] cosign sign (operator key)   — core + ml signed
[6/7] syft SBOM + cosign attest    — core + ml attested
[8/9] oras push bundle + cosign sign — config-bundle <target>-2e84a1b4… signed
[9/9] emit local-build-manifest    — local-build-manifest-2e84a1b4….yaml
___SMKBUILD_EXIT=0
```

**Apply (`<knb-repo>/scripts/deploy/promote.sh --target <target> --product smackerel`, sudo -n) — exit 0:**

```
Verification for ghcr.io/<operator>/smackerel-core@… — The cosign claims were validated
Verification for ghcr.io/<operator>/smackerel-ml@…   — The cosign claims were validated
preconditions OK
▶ apply: pulling images by digest (core dc896368…, ml ef16adc2…)
▶ apply: running rollout strategy: recreate
  Container smackerel-<target>-smackerel-core-1 Recreated → Started
  Container smackerel-<target>-smackerel-ml-1   Recreated → Started
▶ verify: waiting for strict current-release health
  acceptance: core-digest=accepted
  acceptance: ml-digest=accepted
verify OK (strict current release accepted)
▶ apply: committing verified manifest pointer → apply OK
___SMKDEPLOY_EXIT=0
```

**Independent running verification (docker inspect):**

```
smackerel-<target>-smackerel-core-1 :: running health=healthy restarts=0
   ghcr.io/<operator>/smackerel-core@sha256:dc8963683bc87f6d07b5460a009755c8cd400b5dd56ae20d0f3094307e570c32
smackerel-<target>-smackerel-ml-1   :: running health=healthy restarts=0
   ghcr.io/<operator>/smackerel-ml@sha256:ef16adc279b908b3777e9afbf9558dbbdfe9b8611b741c12cccc5d39db4c1b23
```

Both new digests running, healthy, 0 restarts — matching the built+signed digests.

**Behavioral confirmation (operator-only):** the live Telegram `/ask` smoke test
is operator-verifiable (the prod assistant HTTP API requires a per-user PASETO
token; agents cannot send Telegram messages). The honest-refusal behavior is
proven at the unit level by the extended cross-path invariant test
(`TestExecutionErrorHonesty_*`), which exercises the exact open_knowledge
OK-but-uncited path and asserts `StatusUnavailable` + honest body, never the
capture acknowledgement.

---

### Regression Invariant Closure

**Phase:** regression · **Claim Source:** executed · **Live system:** no

**What the regression phase found.** The class-killer for INV-HB-REFUSAL in
`internal/assistant/facade_execution_error_honesty_test.go` sweeps a
**hand-written** list, `requiresProvenanceScenarios`. That list had drifted from
the SST: it named `weather_query`, `retrieval_qa`, and `recipe_search` but **not
`open_knowledge`** — the `/ask` scenario this bug was actually reported against,
and the only requires_provenance scenario with its own facade fast-path and its
own `OutcomeOK → StatusAnswered` mapping. So the sweep this packet cited as
proof that no band-high path can render the capture acknowledgement never
executed the path the bug was filed about. A hand-written sweep is a class-killer
only while it matches reality, and nothing failed when it stopped matching.

**What now prevents recurrence.**

1. `requiresProvenanceScenarios` now contains `open_knowledge`
   (`internal/assistant/facade_execution_error_honesty_test.go:36`), so the
   existing invariant sweep exercises the reported path rather than skipping it.
2. `internal/assistant/facade_high_band_invariant_coverage_test.go` (new) adds
   `TestRequiresProvenanceScenarios_ClosedOverSST`, which closes the sweep list
   over the SST. It loads `config/assistant/scenarios.yaml` — the same file the
   runtime manifest loads — and fails on drift in **both** directions: a
   requires_provenance scenario declared in the SST but absent from the sweep (an
   uncovered masking path), and a sweep entry the SST does not gate (a row that
   proves nothing). A closing `t.Fatal` on an empty declared set stops the
   assertion from passing vacuously if the manifest or its path is ever wrong.

The residual failure mode is now mechanical rather than editorial: adding a
requires_provenance scenario to the SST without extending the invariant sweep
fails the unit suite instead of silently shipping an uncovered copy of this bug.

**Verification — focused run of the two owning tests, executed in this session.**

```
./smackerel.sh test unit --go \
  --go-run 'TestRequiresProvenanceScenarios_ClosedOverSST|TestHighBandNeverMaskedAsSavedAsIdea' \
  --verbose
```

Key captured line, verbatim:

```
    facade_high_band_invariant_coverage_test.go:80: SST requires_provenance scenarios (all swept by the INV-HB-REFUSAL invariant): [open_knowledge recipe_search retrieval_qa weather_query]
```

**Exit code: 0.**

**Why this evidence and not a suite tail.** An earlier `./smackerel.sh test unit`
run did report green, but the Go phase printed the owning package as
`ok  github.com/smackerel/smackerel/internal/assistant  (cached)`. A cache hit is
not an execution: it asserts that a previous run of an identical tree passed, and
it prints nothing the two tests themselves produced. Pasting that suite tail as
proof of this fix would therefore have been proof of the wrong thing — worse, the
tail it produced showed unrelated shell tests and never named either Go test. The
focused run above changes the `-run` pattern, which is part of Go's test cache
key, so the package is re-executed rather than replayed. The line-80 output is
emitted by `TestRequiresProvenanceScenarios_ClosedOverSST` itself and lists the
four requires_provenance scenarios it read out of the SST, `open_knowledge`
among them — that log line only exists if the test actually ran, and its contents
only match if the sweep is genuinely closed over the manifest. That is the
evidence; the cached suite tail was not.

_PII note: absolute paths in this section's evidence were written in relative
form per the repository PII policy. No result, count, or exit code was altered._

---

<a id="simplify"></a>

### Simplify — Post-Implementation Review Of The Regression Diff

**Phase:** simplify · **Claim Source:** executed · **Live system:** no

**Review surface.** The two source files this packet touched most recently
(commit `5c24a74f`), and nothing else:

| File | Change under review |
|---|---|
| `internal/assistant/facade_execution_error_honesty_test.go` | `requiresProvenanceScenarios` gained `open_knowledge` (1 line + doc comment) |
| `internal/assistant/facade_high_band_invariant_coverage_test.go` | new, 81 lines, `TestRequiresProvenanceScenarios_ClosedOverSST` |

Three passes were run over that surface — code reuse, code quality, efficiency.
One finding was acted on. Three were considered and declined; they are recorded
below rather than omitted, so a reader can audit the *declines* as well as the
change.

---

#### Applied — F1 (quality): the second closure assertion could not detect the case its own message described

`TestRequiresProvenanceScenarios_ClosedOverSST` claims to fail on drift in
**both** directions. The first direction was closed. The second was not.

Both checks were derived from a single walk of `manifest.AllScenarioIDs()`, with
the "this sweep row proves nothing" case detected by peeking at `swept[id]`
inside the `continue` branch. A manifest walk can only ever see ids the manifest
still contains. An id **retired from `config/assistant/scenarios.yaml`
outright** — a scenario renamed or removed, leaving a stale entry in
`requiresProvenanceScenarios` — is never visited, so nothing reported it.

That gap was not cosmetic, because the other half of the invariant cannot
compensate for it: `TestHighBandNeverMaskedAsSavedAsIdea` builds a **synthetic**
manifest per id via `newTestManifest`, so a retired id keeps producing a green
sweep row forever. A stale row would go on being counted as coverage. That is
the same failure shape this packet's regression phase was created to close, one
level up.

The fix asks each question from the side that can see its own counterexample:
the manifest walk answers "is every gated scenario swept?", and a second short
walk over `requiresProvenanceScenarios` answers "does every swept id still
gate?". This also removes a conditional nested inside a skip branch, so the loop
body now does one thing.

```diff
+       allIDs := manifest.AllScenarioIDs()
+       gated := make(map[string]bool, len(allIDs))
        var declared, uncovered, notProvenanceBearing []string
-       for _, id := range manifest.AllScenarioIDs() {
+       for _, id := range allIDs {
                if !manifest.RequiresProvenance(id) {
-                       if swept[id] {
-                               notProvenanceBearing = append(notProvenanceBearing, id)
-                       }
                        continue
                }
+               gated[id] = true
                declared = append(declared, id)
                if !swept[id] {
                        uncovered = append(uncovered, id)
                }
        }
+       // Asked from the sweep side: an id retired from the SST is never visited
+       // by a manifest walk, so a manifest walk cannot report it.
+       for _, id := range requiresProvenanceScenarios {
+               if !gated[id] {
+                       notProvenanceBearing = append(notProvenanceBearing, id)
+               }
+       }
```

The assertion message was widened to match what the check now covers ("absent
from the manifest, or present and false"), and `AllScenarioIDs()` — which
allocates a fresh slice per call — is now called once instead of once per use.
`git diff --stat`: 1 file changed, 19 insertions, 9 deletions.

**Behavior on the current tree is unchanged.** The SST declares five scenarios,
four with `requires_provenance: true`, and all four are swept; every swept id is
gated. The change alters only what the test *can* catch on a future drift.

---

#### Verification

**1. Green — both owning tests, focused run (uncached), current tree.**

```
./smackerel.sh test unit --go \
  --go-run 'TestRequiresProvenanceScenarios_ClosedOverSST|TestHighBandNeverMaskedAsSavedAsIdea' \
  --verbose
```

```
=== RUN   TestRequiresProvenanceScenarios_ClosedOverSST
=== PAUSE TestRequiresProvenanceScenarios_ClosedOverSST
=== CONT  TestHighBandNeverMaskedAsSavedAsIdea
=== CONT  TestRequiresProvenanceScenarios_ClosedOverSST
    facade_high_band_invariant_coverage_test.go:90: SST requires_provenance scenarios (all swept by the INV-HB-REFUSAL invariant): [open_knowledge recipe_search retrieval_qa weather_query]
--- PASS: TestRequiresProvenanceScenarios_ClosedOverSST (0.00s)
```

```
ok      github.com/smackerel/smackerel/internal/assistant       0.414s
```

`FINAL_GREEN_EXIT=0`. Grep counts over the captured output: `--- FAIL`/`^FAIL`
lines = 0; `internal/assistant …(cached)` = 0, so the package genuinely
re-executed rather than replaying a prior result.

Note the log line is now `coverage_test.go:90`, where the regression phase
recorded `:80`. The shift is this section's comment edits above the assertion
block. The regression phase's recorded evidence above is left exactly as it was
captured — it is that phase's record of what it ran, and rewriting it here would
be tampering with someone else's evidence, not correcting it.

**2. Adversarial — the newly-closed edge actually fires.**

A retired id (`retired_scenario_probe`, present in no manifest) was temporarily
added to `requiresProvenanceScenarios`, and the closure test run alone:

```
    facade_high_band_invariant_coverage_test.go:83: requiresProvenanceScenarios names [retired_scenario_probe] which config/assistant/scenarios.yaml does NOT declare with requires_provenance: true (absent from the manifest, or present and false) — those sweep rows never exercise the provenance gate and prove nothing
--- FAIL: TestRequiresProvenanceScenarios_ClosedOverSST (0.00s)
FAIL    github.com/smackerel/smackerel/internal/assistant       0.507s
```

`ADVERSARIAL_PROBE_EXIT=1`.

**3. Counter-proof — the edge was genuinely open before this change.**

With the same retired id still in place, only the coverage test's body was
reverted to its pre-simplify form (`git stash push` on that one path), and the
run repeated:

```
    facade_high_band_invariant_coverage_test.go:80: SST requires_provenance scenarios (all swept by the INV-HB-REFUSAL invariant): [open_knowledge recipe_search retrieval_qa weather_query]
--- PASS: TestRequiresProvenanceScenarios_ClosedOverSST (0.00s)
ok      github.com/smackerel/smackerel/internal/assistant       0.304s
```

`PRESIMPLIFY_WITH_RETIRED_ID_EXIT=0`. The pre-simplify body passed with a stale
sweep row present; the simplified body fails on it. Without this step the change
would be an assertion about a gap rather than a demonstration of one. Both
temporary edits were then reverted (`git stash pop`, `git checkout --`) and
step 1 was re-run on the restored tree, which is the run quoted above.

**4. Hygiene.** `gofmt -l` on the changed file returned no output (exit 0). Vet
runs as part of `go test`; the package compiled and passed.

---

#### Considered and declined

**F2 (reuse, declined): triplicated real-manifest load.** The pattern
`LoadSkillsManifest(repoFile(t, "config", "assistant", "scenarios.yaml"),
func(string) (bool, bool) { return true, true })` now appears three times in
package `assistant` — twice pre-existing in `skills_manifest_loader_test.go`
(lines 51 and 102) and once in the new file — and `testhelpers_test.go` is the
package's established home for shared test helpers. A `loadRealManifest(t)`
helper is defensible. It was declined because collapsing it would edit a file
outside this packet's diff for a net saving of roughly a dozen lines, and
because the three call sites are not the same thing: the loader test pairs the
manifest load with a `config/prompt_contracts` directory load and needs the
blank tool imports for it, while the new call carries an inline comment
explaining *why* its resolver is permissive — context a helper would dilute. The
duplication is recorded here so a future consolidation has a starting point.

**F3 (quality, declined): duplicated narrative.** The `open_knowledge` drift
story is told at length twice — in the new file's 16-line header and again in
the `requiresProvenanceScenarios` doc comment. Trimming one is a prose
preference, not an evidence-driven improvement, so it was left alone.

**F4 (efficiency): nothing found beyond the one-call change above.** Both maps
were already pre-sized, the slices hold at most five elements, and the three
`sort.Strings` calls are on those same slices. There is no measurable cost here
and no reason to touch it.

**Boundaries respected.** `uservalidation.md` untouched (G136 requires the
author). `status` unchanged (`blocked`). Only `simplify` was appended to
`completedPhases`. Nothing under `.github/bubbles/` was modified. Nothing was
committed.

_PII note: this section's evidence quotes command output with absolute paths
rewritten in repo-relative form per the repository PII policy. No result, count,
or exit code was altered._

---

## Implementation Delta

### Code Diff Evidence

**Phase:** regression · **Claim Source:** executed · **Live system:** no

The two source changes this packet made most recently both landed in commit
`5c24a74f` ("fix(BUG-061-009): close the INV-HB-REFUSAL sweep over the scenario
SST"). Diff retrieved with, and reproduced from, this command executed in this
session from the repository root `<repo-root>`:

```
git show 5c24a74f -- internal/assistant/facade_execution_error_honesty_test.go internal/assistant/facade_high_band_invariant_coverage_test.go
```

Changed-path summary, from `git show --stat 5c24a74f` on the same two paths:

```
 .../facade_execution_error_honesty_test.go         | 13 ++--
 .../facade_high_band_invariant_coverage_test.go    | 81 ++++++++++++++++++++++
 2 files changed, 90 insertions(+), 4 deletions(-)
```

**1 — `internal/assistant/facade_execution_error_honesty_test.go` (modified).**
Complete hunk, verbatim:

```
diff --git a/internal/assistant/facade_execution_error_honesty_test.go b/internal/assistant/facade_execution_error_honesty_test.go
index 13ae26e6..48252925 100644
--- a/internal/assistant/facade_execution_error_honesty_test.go
+++ b/internal/assistant/facade_execution_error_honesty_test.go
@@ -25,10 +25,15 @@ import (
 )
 
 // requiresProvenanceScenarios is the closed set whose manifest sets
-// requires_provenance=true (skills_manifest_test.go asserts this exact set).
-// Every one is subject to the provenance gate and therefore to the masking
-// defect if the gate ever runs on a non-OK outcome.
-var requiresProvenanceScenarios = []string{"weather_query", "retrieval_qa", "recipe_search"}
+// requires_provenance=true. Every one is subject to the provenance gate and
+// therefore to the masking defect if the gate ever runs on a non-OK outcome.
+//
+// The set is closed over the SST by TestRequiresProvenanceScenarios_ClosedOverSST,
+// which reads config/assistant/scenarios.yaml directly — skills_manifest_test.go
+// spot-checks individual entries but never asserted the set was complete, which
+// is how open_knowledge (the `/ask` scenario BUG-061-009 was reported against)
+// stayed out of this sweep while the packet claimed it was covered.
+var requiresProvenanceScenarios = []string{"weather_query", "retrieval_qa", "recipe_search", "open_knowledge"}
 
 // errorOutcomes are non-OK executor outcomes that represent an execution
 // FAILURE (not a genuine no-answer). Each MUST surface honestly.
```

That one added element, `"open_knowledge"`, is the whole behavioural delta of
this file: it is what makes the existing `TestHighBandNeverMaskedAsSavedAsIdea`
sweep actually traverse the `/ask` path this bug was reported against.

**2 — `internal/assistant/facade_high_band_invariant_coverage_test.go` (new,
81 added lines).** Reproduced below as a labelled **excerpt**, not the whole
hunk. The elision is marked inline and is the only omission; every line shown is
verbatim from the command above, and Go's leading tabs are preserved. The
complete file lives at
`internal/assistant/facade_high_band_invariant_coverage_test.go`, and the full
hunk is reproducible with
`git show 5c24a74f -- internal/assistant/facade_high_band_invariant_coverage_test.go`.

```
diff --git a/internal/assistant/facade_high_band_invariant_coverage_test.go b/internal/assistant/facade_high_band_invariant_coverage_test.go
new file mode 100644
index 00000000..29f6f0d6
--- /dev/null
+++ b/internal/assistant/facade_high_band_invariant_coverage_test.go
@@ -0,0 +1,81 @@
+// BUG-061-009 (regression phase) — coverage closure for INV-HB-REFUSAL.
+//
+// TestHighBandNeverMaskedAsSavedAsIdea is the class-killer for the "saved as an
+// idea" masking, but it sweeps a HAND-WRITTEN list (requiresProvenanceScenarios).
+// A hand-written list is only a class-killer while it matches reality: a
+// requires_provenance scenario absent from it is an uncovered copy of the same
+// defect, and nothing fails when the SST gains one. That is not hypothetical —
+// `open_knowledge` (the `/ask` scenario BUG-061-009 was actually reported
+// against, and the only one with its own facade fast-path and its own
+// OutcomeOK→StatusAnswered mapping) was missing from the list while the packet
+// claimed the invariant covered it.
+//
+// This test closes the list over the SST: the covered set is checked against
+// config/assistant/scenarios.yaml, the same file the runtime manifest loads. It
+// fails on drift in either direction, so the invariant sweep cannot silently
+// stop covering the class it exists to kill.
+
+package assistant
+
+import (
+	"sort"
+	"testing"
+)
+
[EXCERPT MARKER — added lines 25 through 66 elided here: the doc comment on
TestRequiresProvenanceScenarios_ClosedOverSST, the LoadSkillsManifest call with
its permissive enable-key resolver, and the construction of the swept /
declared / uncovered / notProvenanceBearing sets. Nothing else is omitted.]
+
+	if len(uncovered) > 0 {
+		t.Errorf("requires_provenance scenario(s) %v are declared in config/assistant/scenarios.yaml but absent from requiresProvenanceScenarios %v — "+
+			"each is an uncovered high-band path that can mask a refusal as 'saved as an idea' (INV-HB-REFUSAL)",
+			uncovered, requiresProvenanceScenarios)
+	}
+	if len(notProvenanceBearing) > 0 {
+		t.Errorf("requiresProvenanceScenarios names %v which the SST does NOT mark requires_provenance — "+
+			"those sweep rows never exercise the provenance gate and prove nothing", notProvenanceBearing)
+	}
+	if len(declared) == 0 {
+		t.Fatal("manifest declared zero requires_provenance scenarios — the closure assertion would pass vacuously; the manifest or its path is wrong")
+	}
+	t.Logf("SST requires_provenance scenarios (all swept by the INV-HB-REFUSAL invariant): %v", declared)
+}
```

The two `t.Errorf` branches are what make the assertion bidirectional, and the
`t.Fatal` is what stops it passing vacuously — those three are the reason this
file is a coverage closure rather than another hand-maintained list. The
`t.Logf` on the last line is the line quoted as execution proof in
*Regression Invariant Closure* above.

---

## Stabilize — Stability, Performance, Reliability And Resource Assessment

<a id="stabilize-assessment"></a>

**Phase:** `stabilize` · **Agent:** `bubbles.stabilize` · **Date:** 2026-08-18
**Verdict:** ⚠️ PARTIALLY_STABLE — one finding, recorded and routed, not fixed
inline. See *Verdict* at the end of this section for why that token was chosen
over 🟢 and over 🛑.

### What was assessed

The change this packet ships is a refusal-path correctness fix plus two
test-only additions:

- **Runtime:** a high-band `requires_provenance` turn that returns OK-but-uncited
  now renders an honest refusal (`StatusUnavailable` + `ErrNoGroundedAnswer` +
  `CanonicalRefusalBody`, `CaptureRoute=false`) instead of the band-low capture
  acknowledgement. Implemented in `provenance/gate.go` `Enforce` and
  `facade.go` `canonicalizeSuccessfulCaptureResponse`, plus the adapter render
  paths.
- **Test-only (commit `5c24a74f`):** `open_knowledge` added to
  `requiresProvenanceScenarios`, and `TestRequiresProvenanceScenarios_ClosedOverSST`
  added to close that sweep over `config/assistant/scenarios.yaml`.

The assessment is scoped to that. A load test was not run, because nothing in
this change alters throughput, concurrency, allocation behaviour on the hot
path, or any external call — the reasoning for that claim is below, and it is
reasoning from the code, not an assumption.

### Finding: no performance, resource or reliability risk introduced

Four grounded reasons, each checked against the source rather than assumed:

1. **The refusal branch performs no additional work and allocates nothing.**
   `Enforce`'s rewrite is a fixed set of field assignments on a struct that is
   already materialized. `CanonicalRefusalBody` is a package-level `const`
   string (`provenance/gate.go`) and `contracts.CanonicalRefusalBodyFor` returns
   package-level `const` strings from a closed `switch` — neither allocates.
   `resp.Sources = nil` drops a slice reference, so the branch is marginally
   *cheaper* in retained memory than the pre-fix path, not more expensive. The
   pre-fix path assigned the same number of fields to reach the capture shape;
   this assigns them to reach the refusal shape.

2. **The branch is off the hot path by construction.** It can only fire on a
   turn that already completed an LLM invocation for a `requires_provenance`
   scenario and returned a body with no valid sources. Any cost it adds is
   dominated by the invocation that preceded it by orders of magnitude. The
   existing p95 guards (`tests/stress/assistant_facade_p95_test.go`,
   `tests/stress/openknowledge_p95_test.go`) already cover that hot path; this
   change does not move the code they measure.

3. **No metric-cardinality risk from the new `ErrorCause`.** `ErrNoGroundedAnswer`
   grows the closed `AllErrorCauses` vocabulary to 7 values, but `ErrorCause` is
   never used as a Prometheus label value anywhere in non-test `internal/` code
   — a repo-wide scan of `WithLabelValues` call sites returns zero matches that
   reference it (evidence below). It is consumed structurally, by adapters and
   tests, not as a label. The packet registered no new metric;
   `provenance.ViolationsCounter` predates it and its `(scenario_id, cause)`
   label pair is unchanged.

4. **No new goroutine, IO, lock, timer, or external call** appears anywhere on
   the changed path. The rewrite is synchronous field assignment inside a
   function that was already on the call path.

### Finding: the new test adds no new dependency class and no measurable cost

`TestRequiresProvenanceScenarios_ClosedOverSST` parses
`config/assistant/scenarios.yaml` — 77 lines, 3,353 bytes — once per run, and Go
reports it at `0.00s`, i.e. below the 10 ms reporting resolution. It resolves
that path through the pre-existing `repoFile`/`repoRoot` helper in
`testhelpers_test.go`, which seven other tests in the same package
(`skills_manifest_test.go`, `skills_manifest_loader_test.go`,
`scenarios_validator_test.go`) already use to read the same file. So it
introduces no new filesystem-dependency class, no new working-directory
coupling, and no new failure mode for hermetic or packaged test execution that
the package did not already carry.

### Finding: a real configuration→build coupling, accepted by design

Closing the sweep over the SST creates a genuine operational consequence that is
worth naming rather than discovering later as a surprise build break: **a
config-only edit can now fail the Go unit suite.** Adding a scenario with
`requires_provenance: true` to `config/assistant/scenarios.yaml` without also
adding its id to `requiresProvenanceScenarios` in
`internal/assistant/facade_execution_error_honesty_test.go` fails
`TestRequiresProvenanceScenarios_ClosedOverSST`.

This is the intended property — it is exactly what makes the invariant a
class-killer rather than a snapshot of one moment, and it is the drift the
regression phase caught. It is recorded here as a **posture note, not a defect**:
whoever next edits the scenario SST must update the sweep list in the same
change, and the test's failure message already says so in both directions.
Severity: low. Disposition: accepted-by-design.

### Observation: the invariant's guarantee is chokepoint-based, and two sites return before it

Not a defect, and not routed — recorded so a future author knows the shape of
the guarantee. The user-visible capture acknowledgement has exactly one literal
in the tree (`facade.go:60`) and exactly four writers:

| Site | Band | Reaches `canonicalizeSuccessfulCaptureResponse`? |
|---|---|---|
| `facade.go:1072` | low | yes — flows to the chokepoint at `facade.go:1416` |
| `facade.go:1130` | low | yes |
| `facade.go:1853` | low | this *is* the chokepoint's band-low branch |
| `facade.go:2194` | low | **no** — returns early, self-audits `BandLow` |

`facade.go:~2192` is the pending-disambiguation non-matching-reply path. It
emits the capture shape and returns before the chokepoint, persisting and
auditing itself as `BandLow`. That is defensible: no scenario matched that turn,
so it is band-low by construction — but the invariant holds there by *labelling*,
not by the chokepoint that mechanically enforces it everywhere else.

I checked the one other non-test `StatusSavedAsIdea` producer for a masking hole
and found none: `compiled_interactions.go:173` (confirm-negative) sets
`Status=StatusSavedAsIdea` with `Body: "Change cancelled."` and leaves
`CaptureRoute` unset, so the chokepoint's `!resp.CaptureRoute` guard returns it
unchanged — and the Telegram adapter's `statusPrefix` renders
`StatusSavedAsIdea` with no prefix and the response's own `Body`
(`render_outbound.go:288`), so it surfaces "Change cancelled.", never the capture
string. No user-visible masking exists on that path.

### Finding: the recorded deployment `sourceSha` is dangling (NOT caused by this packet)

`state.json.deployment.sourceSha` is `2e84a1b4df78c3b1932ddf91def2ededcf34cb9e`.
That object still exists and is the real fix commit
(`fix(assistant): BUG-061-009 — high-band turns never render 'saved as an idea'`,
2026-07-23), but it is **not an ancestor of HEAD** and **no branch contains it**.
History was rewritten after the deploy was recorded.

What this does and does not mean, stated precisely:

- **The pending operator smoke test remains valid.** The behaviour it exercises
  is byte-identical between that commit and HEAD: `provenance/gate.go`,
  `contracts/refusal.go`, `telegram/.../render_outbound.go`,
  `telegram/.../render_openknowledge.go`, `whatsapp/.../adapter.go` and
  `config/assistant/scenarios.yaml` are all UNCHANGED across that range. The one
  refusal-adjacent line that differs in `facade.go` is `turnBand = BandLow`
  inside the compiled-weather short-circuit, which feeds the structured log
  label at `facade.go:539` and not the enforcement path. `contracts/response.go`
  differs only by two additive fields (`AssistantTurnID`, `AgentTraceID`, from
  BUG-069-004). So the deployed image genuinely carries the fix under test.
- **What is lost is rebuildability from the record.** The deployed digest cannot
  be reproduced by checking out the `sourceSha` the deployment block names,
  because that SHA is on no branch. The provenance chain from record to artifact
  is broken even though the artifact is correct.
- **Not an incident.** Nothing is degraded and nothing is failing. Severity:
  medium (audit/provenance), not S1/S2.

Disposition: **recorded and routed, not fixed inline.** Correcting it requires
either re-recording the deployment against a reachable SHA or rebuilding and
redeploying from HEAD; both are deployment-record actions owned by
`bubbles.devops` / `bubbles.train`, not a stabilize edit. Logged as DI-4 below.

### Verification evidence

**Claim Source:** commands executed in this session, in this repository, at HEAD
`76a27269`. Absolute home paths in the transcripts below are rewritten to
`<repo-root>` to satisfy the repo PII gate; no command, count, exit code or
result was altered.

#### `./smackerel.sh check` — exit 0

```
$ ./smackerel.sh check
config-validate: <repo-root>/config/generated/dev.env.tmp.3230055 OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 17, rejected: 0
scenario-lint: OK
SMACKEREL_CHECK_EXIT=0
CHECK_WALL_SECONDS=14
```

Config is in sync with the SST and all 17 scenarios register with zero
rejections, so the config→build coupling described above is currently satisfied.

#### `./smackerel.sh test unit` — exit 0

```
# BUG-061-009 stabilize: ./smackerel.sh test unit
$ ./smackerel.sh test unit
exit: 0
lines: 441
sha256: e18130813126ab85652f3fa24dca41bf12b8e1bfab13b9f62be50ef5a5512d00
--- first 20 ---
oom-preflight: OK — 36579 MB available (need 6000 MB; swap used 670 MB).
disk-preflight: OK — C: 73 GB free (need 40 GB), WSL / 473 GB free (need 25 GB).
++ dirname /workspace/scripts/runtime/go-unit.sh
[go-unit] envsubst missing — installing gettext-base
+ source /workspace/scripts/runtime/_ensure_envsubst.sh
+ ensure_envsubst go-unit
+ local tag=go-unit
+ command -v envsubst
+ echo '[go-unit] envsubst missing — installing gettext-base'
+ apt-get update -qq
+ apt-get install -y --no-install-recommends gettext-base
Reading package lists...
Building dependency tree...
Reading state information...
The following NEW packages will be installed:
  gettext-base
0 upgraded, 1 newly installed, 0 to remove and 20 not upgraded.
Need to get 160 kB of archives.
After this operation, 660 kB of additional disk space will be used.
Get:1 http://deb.debian.org/debian bookworm/main amd64 gettext-base amd64 0.21-12 [160 kB]
--- omitted 401 line(s); sha256 above covers the full output ---
--- last 20 ---
  ...
1..2
# tests 2
# suites 0
# pass 2
# fail 0
# cancelled 0
# skipped 0
# todo 0
# duration_ms 142.106003
PASS: bug_077_002_login_session_reuse_test (SCN-077-BUG-002-01 / SCN-077-BUG-002-02)
[test unit] -> bash <repo-root>/tests/unit/web/spec_077_discovery_convention_test.sh
PASS: spec_077_discovery_convention_test (TP-077-02-01 / SCN-077-A02)
[test unit] -> bash <repo-root>/tests/unit/web/spec_077_no_stub_bodies_test.sh
PASS: spec_077_no_stub_bodies_test (TP-077-03-06 / SCN-077-A08)
[test unit] shell unit tests in tests/unit/web/ finished OK
[test unit] running 1 shell unit test(s) from tests/unit/docs/
[test unit] -> bash <repo-root>/tests/unit/docs/spec_077_test_category_parity_test.sh
PASS: spec_077_test_category_parity_test (TP-077-02-03 / SCN-077-A06)
[test unit] shell unit tests in tests/unit/docs/ finished OK
```

```
EVIDENCE_CAPTURE_EXIT=0
TEST_UNIT_WALL_SECONDS=117
```

The `apt-get` in the first 20 lines is a pre-existing, documented and
intentional bootstrap (`scripts/runtime/_ensure_envsubst.sh`, added by the
spec-052 chaos phase because the `golang:1.25.10-bookworm` base image omits
`gettext-base`). It is idempotent per container and unrelated to this packet.
Noting it because I observed it, not as a finding.

#### The packet's own tests genuinely re-execute (not cache-served)

A full-suite pass can report `internal/assistant` as `(cached)`, which asserts
only that some prior identical tree passed. A focused run defeats that, because
the `-run` pattern is part of Go's test cache key:

```
$ ./smackerel.sh test unit --go --go-run 'TestRequiresProvenanceScenarios_ClosedOverSST|TestHighBandNeverMaskedAsSavedAsIdea|TestExecutionErrorHonesty' --verbose
FOCUSED_EXIT=0
FOCUSED_WALL_SECONDS=33

ok      github.com/smackerel/smackerel/internal/assistant       0.280s

facade_high_band_invariant_coverage_test.go:90: SST requires_provenance scenarios (all swept by the INV-HB-REFUSAL invariant): [open_knowledge recipe_search retrieval_qa weather_query]

--- PASS: TestRequiresProvenanceScenarios_ClosedOverSST (0.00s)
--- PASS: TestExecutionErrorHonesty_OKNoSourcesRefusesHonestly (0.00s)
--- PASS: TestExecutionErrorHonesty_MetricIncrements (0.00s)
--- PASS: TestHighBandNeverMaskedAsSavedAsIdea (0.00s)
    (12 subtests: {weather_query, retrieval_qa, recipe_search, open_knowledge}
     x {provider-error, timeout, ok_uncited})

FAIL/--- FAIL line count: 0
```

`0.280s` rather than `(cached)` proves re-execution, and the
`facade_high_band_invariant_coverage_test.go:90` line exists only if the closure
test actually ran. All four swept scenarios are named, `open_knowledge` among
them. The `0.00s` per-test timings are the measurement behind the "no
measurable cost" claim above.

#### Metric-cardinality check

```
$ grep -rn 'WithLabelValues' internal/ --include='*.go' | grep -v '_test.go' | grep -iE 'errorcause'
ERRCAUSE_LABEL_EXIT=1   # 1 = no match: ErrorCause never becomes a metric label

$ grep -rn 'prometheus.New\|MustRegister' internal/assistant/provenance/gate.go internal/assistant/contracts/response.go internal/assistant/contracts/refusal.go
internal/assistant/provenance/gate.go:53:var ViolationsCounter = prometheus.NewCounterVec(
internal/assistant/provenance/gate.go:62:       prometheus.MustRegister(ViolationsCounter)
```

The only registration on the changed surface is the pre-existing
`ViolationsCounter`, whose labels this packet did not touch.

#### Deployment-provenance check

```
$ git rev-parse HEAD
76a27269b51308a2414ca348a226a73e476bccbe

$ git merge-base --is-ancestor 2e84a1b4df78c3b1932ddf91def2ededcf34cb9e HEAD
IS_ANCESTOR_EXIT=1   # 0=ancestor, 1=NOT an ancestor

$ git log -1 --format='%H %ad %s' 2e84a1b4df78c3b1932ddf91def2ededcf34cb9e
2e84a1b4df78c3b1932ddf91def2ededcf34cb9e Thu Jul 23 03:20:20 2026 +0000 fix(assistant): BUG-061-009 — high-band turns never render 'saved as an idea'

$ git branch -a --contains 2e84a1b4df78c3b1932ddf91def2ededcf34cb9e
CONTAINS_EXIT=0      # empty output: no local or remote branch contains it
```

```
$ for f in <refusal surface>; do git diff --quiet 2e84a1b4..HEAD -- "$f"; done
UNCHANGED  internal/assistant/provenance/gate.go
UNCHANGED  internal/assistant/contracts/refusal.go
CHANGED    internal/assistant/contracts/response.go
CHANGED    internal/assistant/facade.go
UNCHANGED  internal/telegram/assistant_adapter/render_outbound.go
UNCHANGED  internal/telegram/assistant_adapter/render_openknowledge.go
UNCHANGED  internal/whatsapp/assistant_adapter/adapter.go
UNCHANGED  config/assistant/scenarios.yaml
```

The two CHANGED files were inspected line by line; their diffs are the additive
BUG-069-004 turn-identity fields and the telemetry-only `turnBand = BandLow`
assignment described above, neither of which touches refusal enforcement.

### Verdict

**⚠️ PARTIALLY_STABLE.**

The change under assessment carries **no stability, performance, reliability or
resource risk**. That is the honest answer and it is supported by the four
reasons in the first finding, each read out of the source rather than assumed.
A load test was deliberately not run because nothing in the change moves a
measured hot path.

The token is not 🟢 because one real finding exists — the dangling deployment
`sourceSha` — and I did not fix it inline. It is not 🛑 because nothing is
degraded, no code defect was found, and the deployed artifact demonstrably
carries the correct behaviour; declaring the packet unstable would imply the
refusal fix is broken, which the evidence contradicts. The verdict definition
for ⚠️ assumes inline repair; I am deviating from that clause deliberately and
saying so, because the finding is a deployment-record fact that only a
re-record or a rebuild-and-redeploy can resolve, and both belong to
`bubbles.devops` / `bubbles.train`.

**Routed, not fixed here:**

| Finding | Owner | Why not stabilize |
|---|---|---|
| Dangling deployment `sourceSha` (DI-4) | `bubbles.devops` / `bubbles.train` | Resolution is a re-record or a rebuild-and-redeploy, not a code edit |
| Check 8A: `scopes.md` lacks the scenario-specific regression E2E DoD items and Test Plan rows the guard requires (3 blocks) | `bubbles.plan` | `scopes.md` is planning-owned; stabilize must not edit it |

Neither the configuration→build coupling nor the chokepoint-topology
observation is routed: the first is accepted by design, and the second is not a
defect.

---

## Audit — Adversarial Reversion Probe (`bubbles.audit`) — 2026-08-18

### Headline finding: the invariant is defended by two jointly load-bearing layers, and the test suite cannot tell them apart

The adversarial probe reverted the provenance-gate fix in
`internal/assistant/provenance/gate.go` (restoring `resp.Status =
contracts.StatusSavedAsIdea` / `resp.CaptureRoute = true`) and re-ran the two
invariant tests. **The suite still passed, exit 0.** Reverting the gate alone
does not fail the invariant.

Only when the facade layer in `internal/assistant/facade.go` was **also**
disabled did the suite fail, exit 1, on all four `requiresProvenanceScenarios`
entries' `ok_uncited` cases.

**Why this matters, stated plainly.** A reader who assumed single-layer
reversion was the adversarial proof would overestimate what these tests pin
down. They do not pin down the provenance gate's own contribution to
INV-HB-REFUSAL. They pin down the *conjunction* of gate and facade. Anyone
refactoring either layer on its own could remove that layer's honest-refusal
shaping and no test would object — the surviving layer would still produce a
conforming response, CI would stay green, and the reviewer would receive no
signal that the defence had been halved.

**What the redundancy is, and is not.** It is intentional, and the facade
states so in its own comment at `internal/assistant/facade.go:1826-1830`:
convert a band-high capture-shaped response to the honest refusal "so
INV-HB-REFUSAL holds structurally **even if an upstream path regresses into the
capture shape**". So the runtime property is genuinely good: either layer alone
still yields an honest response to the user. What the probe establishes is a
*test-sensitivity* limit, not a runtime defect. The suite asserts the observable
contract (`Status`, `Body`, `CaptureRoute`, `ErrorCause`) at the facade
boundary, and both layers independently satisfy that contract for this path, so
no assertion can distinguish "gate fixed" from "gate reverted, facade
compensating".

This is recorded rather than repaired because closing it means adding a
gate-level assertion that the gate itself emits the honest shape, which is a
planning-owned Test Plan change (`bubbles.plan`), not an audit edit.

### The probe

**Command (identical for both runs):**

```
./smackerel.sh test unit --go --go-run 'TestHighBandNeverMaskedAsSavedAsIdea|TestExecutionErrorHonesty_OKNoSourcesRefusesHonestly' --verbose
```

| Run | Layers reverted | Exit code | Meaning |
|---|---|---|---|
| 1 | `provenance/gate.go` only | **0** | Suite still passes. Gate reversion alone is invisible to the tests. |
| 2 | `provenance/gate.go` **and** `facade.go` | **1** | Suite fails. Both layers must be removed before any assertion fires. |

Run 2 reported `--- FAIL: TestHighBandNeverMaskedAsSavedAsIdea` and `--- FAIL:
TestExecutionErrorHonesty_OKNoSourcesRefusesHonestly`, failing on every
scenario's `ok_uncited` case. Representative verbatim assertion messages:

```
facade_execution_error_honesty_test.go:116: Status = saved_as_idea; a high-band no-source outcome (ok_uncited) MUST surface honestly, never masked
facade_execution_error_honesty_test.go:122: Body is the capture acknowledgement
facade_execution_error_honesty_test.go:125: CaptureRoute = true
facade_execution_error_honesty_test.go:128: ErrorCause empty
```

**Claim Source:** not-run

**Provenance of this block (read this before citing it).** The two probe runs
above were executed by the `bubbles.audit` phase and are recorded here as an
**observed result relayed to the recording agent**. The agent writing this
section did **not** execute them and deliberately did not reproduce them: the
probe is destructive to tracked source, both files are restored, and they must
stay that way. The exit codes and assertion text above are therefore reported
evidence, not this session's execution evidence, and are tagged `not-run` for
that reason. The corroboration in the next subsection *was* executed in this
session and is tagged accordingly.

### Corroboration executed in this session

The relayed probe result is not self-verifying, so the recording agent grounded
every structural claim it depends on against the committed tree at `HEAD`.

**Command:**

```
$ grep -nE 'MUST surface honestly, never masked|Body is the capture acknowledgement|CaptureRoute = true|ErrorCause empty' internal/assistant/facade_execution_error_honesty_test.go
116:   t.Errorf("Status = saved_as_idea; a high-band no-source outcome (%s) MUST surface honestly, never masked", tc.name)
122:   t.Errorf("Body is the capture acknowledgement; %s must surface an honest message, never 'saved as an idea'", tc.name)
125:   t.Errorf("CaptureRoute = true; a high-band %s must not be captured as an idea", tc.name)
128:   t.Errorf("ErrorCause empty; a high-band %s must carry a cause so the transport can render it honestly", tc.name)
163:   t.Errorf("Body is the capture acknowledgement; an OK-but-uncited refusal must be honest, never 'saved as an idea'")
169:   t.Errorf("CaptureRoute = true; a high-band refusal is not a capture")
ASSERT_GREP_EXIT=0

$ grep -nE 'func TestHighBandNeverMaskedAsSavedAsIdea|func TestExecutionErrorHonesty_OKNoSourcesRefusesHonestly' internal/assistant/*_test.go
internal/assistant/facade_execution_error_honesty_test.go:88:func TestHighBandNeverMaskedAsSavedAsIdea(t *testing.T) {
internal/assistant/facade_execution_error_honesty_test.go:141:func TestExecutionErrorHonesty_OKNoSourcesRefusesHonestly(t *testing.T) {
FUNC_GREP_EXIT=0

$ grep -nA8 'requiresProvenanceScenarios = ' internal/assistant/facade_execution_error_honesty_test.go
36:var requiresProvenanceScenarios = []string{"weather_query", "retrieval_qa", "recipe_search", "open_knowledge"}
SWEEP_GREP_EXIT=0
```

**Exit Code:** 0 (all three greps)

**Claim Source:** executed

Three facts the relayed result depends on are now grounded rather than assumed:
the four cited line numbers carry exactly the four quoted assertion messages;
both named test functions exist; and `requiresProvenanceScenarios` holds exactly
four entries, so "all four scenarios' `ok_uncited` cases" is an accurate count
and not a rounding of the failure list.

### Restoration verified: both probe-touched files are byte-identical to `HEAD`

The probe mutates tracked source. The audit phase restored both files with `git
checkout --`. The recording agent re-verified that restoration in this session
by blob hash, which is stronger than an empty `git diff` because it compares
content identity directly rather than relying on diff configuration.

**Command:**

```
$ git diff --stat HEAD -- internal/assistant/provenance/gate.go internal/assistant/facade.go
DIFF_STAT_EXIT=0

$ git status --porcelain -- internal/assistant/provenance/gate.go internal/assistant/facade.go
PORCELAIN_EXIT=0

$ for f in internal/assistant/provenance/gate.go internal/assistant/facade.go; do printf '%s worktree=%s head=%s\n' "$f" "$(git hash-object "$f")" "$(git rev-parse HEAD:"$f")"; done
internal/assistant/provenance/gate.go worktree=e728e50d115e3c61e437bc2905058c3e62c23eb9 head=e728e50d115e3c61e437bc2905058c3e62c23eb9
internal/assistant/facade.go          worktree=139510ffc375b310e2dd8c4309afe7b07e085edb head=139510ffc375b310e2dd8c4309afe7b07e085edb
```

**Exit Code:** 0

**Claim Source:** executed

Both `git diff --stat` and `git status --porcelain` emit nothing for these two
paths, and each worktree blob hash equals its `HEAD` blob hash. No residue of
the probe survives in the tree.

### Security sweep of the changed surface

Greps over the packet's changed surface found no hardcoded credentials and no
secrets written into log statements. The uncited model body is **replaced, not
leaked**: `internal/assistant/provenance/gate.go:117` assigns `resp.Body =
CanonicalRefusalBody` and `:120` assigns `resp.CaptureRoute = false`, so the
model's unsourced text is overwritten before the response leaves the gate rather
than being passed through alongside a refusal status. That distinction is the
security-relevant one — a refusal that still carried the ungrounded body would
present fabricated content to the user under an honest-looking status.

**Claim Source:** interpreted

**Interpretation:** the sweep commands were run by the audit phase and are
relayed here. The recording agent independently confirmed the two assignment
sites exist at the cited lines in the committed source (`grep -nE
'CanonicalRefusalBody|StatusSavedAsIdea|CaptureRoute'
internal/assistant/provenance/gate.go`, exit 0). The conclusion "replaced rather
than leaked" is a reading of those two assignments in context, not a single
unambiguous command signal, so it is labelled `interpreted` rather than
`executed`.

### Artifact lint

```
$ bash .github/bubbles/scripts/artifact-lint.sh specs/061-conversational-assistant/bugs/BUG-061-009-high-band-refusal-masked-as-saved-as-idea
PASSED
ARTIFACT_LINT_EXIT=0
```

**Claim Source:** interpreted

**Interpretation:** the audit phase reported `PASSED`, exit 0. The recording
agent did not re-run `artifact-lint.sh` directly, but ran the state transition
guard against this packet in this session, whose Check 13 independently
re-executes the same lint and reported `✅ PASS: Artifact lint passes (exit 0)`.
The claim is therefore corroborated through the guard rather than by a direct
invocation, which is why it is labelled `interpreted`.

### Audit verdict

⚠️ **PASS WITH ONE RECORDED FINDING.**

No defect was found in the shipped behaviour. The invariant holds, the tests
are real, the assertions are specific, the probe-touched files are restored
byte-identical to `HEAD`, and the security posture of the refusal path is sound.

The recorded finding is the coverage-topology fact in the headline: the two
enforcement layers are jointly load-bearing under the current assertions, so no
test fails when exactly one of them is removed. That is a limit on what this
suite proves, and it is stated here precisely so no later reader infers a
stronger guarantee from the packet's existing "class-killer" language than the
assertions actually deliver.

**Owned elsewhere, not edited here:**

| Finding | Owner | Why not audit |
|---|---|---|
| No assertion pins the provenance gate's own honest-refusal shape independently of the facade backstop | `bubbles.plan` | Closing it adds a Test Plan row and a DoD item; `scopes.md` planning content is planning-owned and audit must not author it |

---

<a id="security"></a>

## Security — Refusal-Path Disclosure Review (`bubbles.security`) — 2026-08-18

**Phase:** `security` · **Agent:** `bubbles.security` · **Date:** 2026-08-18 ·
**Claim Source:** executed · **Live system:** no

### The question

The fix makes the system say **more** on failure. Where a band-high turn
previously emitted a capture acknowledgement, it now emits an honest refusal
plus a populated `ErrorCause`. Additional output on a failure path is where
disclosure defects live. So: does the added output reveal anything it should
not?

### Verdict

The user-visible refusal surface is **security-neutral by construction**. Two
mechanisms carry that property, and neither depends on a reviewer noticing.
Three corrections to the findings as received are recorded below. Two residuals
are stated as open rather than cleared.

### Finding 1 — `ErrorCause` cannot carry raw upstream text

**VERIFIED, with three corrections.**

The type declaration and its documentation are exactly where claimed.
`<repo-root>/internal/assistant/contracts/response.go:187-188` carries the
comment "ErrorCause is the closed-vocabulary error discriminator populated when
Status == StatusUnavailable". Line 189 declares `type ErrorCause string`.

All eight constants match the list as given:

| Constant | Wire value | Line |
|---|---|---|
| `ErrNone` | `""` | `response.go:194` |
| `ErrProviderUnavailable` | `provider_unavailable` | `response.go:197` |
| `ErrMissingScope` | `missing_scope` | `response.go:200` |
| `ErrSlotMissing` | `slot_missing` | `response.go:203` |
| `ErrInternalError` | `internal_error` | `response.go:206` |
| `ErrNoMatch` | `no_match` | `response.go:211` |
| `ErrModelNotSwitchable` | `model_not_switchable` | `response.go:219` |
| `ErrNoGroundedAnswer` | `no_grounded_answer` | `response.go:229` |

**Correction 1a — `facade.go:710` is a string-to-enum conversion, not an
enum-to-enum conversion.** The finding described it as enum-to-enum. It is not.
The source field is declared `ErrorCause string` at
`<repo-root>/internal/assistant/legacyretirement/closedresponse.go:25`. That is
a plain Go `string`, not `contracts.ErrorCause`. So
`contracts.ErrorCause(closed.ErrorCause)` **widens an unconstrained string into
the enum type**. The premise was wrong.

The conclusion survives anyway, for a reason the original framing did not
supply. `ClosedResponseFor` is the only constructor of that struct, and it
hardcodes `ErrorCause: "retired_command_closed"` at `closedresponse.go:53`. The
value is a literal. Nothing is interpolated into it.

**Correction 1b — the live value set is nine, not eight.**
`"retired_command_closed"` reaches `contracts.ErrorCause` at runtime but appears
in no `const` block. The vocabulary is therefore closed **by construction site**,
not by the declared constant list. A reader auditing only the `const` block
would miss a live value. That is worth recording, because the safety argument
rests on enumerating construction sites rather than constants.

**Correction 1c — the enumeration of assignment shapes was incomplete.**
`internal/assistant/facade.go` holds 19 `ErrorCause` assignments. The finding
named two non-literal shapes. Two more exist:

| Site | Shape | Why it is closed |
|---|---|---|
| `facade.go:1337` | `resp.ErrorCause = assembly.Override.ErrorCause` | Source field is typed `ErrorCause` at `contracts/source_assembler.go:144` |
| `facade.go:1851` | `resp.ErrorCause = ""` | The `ErrNone` zero value, written as a literal |

Neither interpolates. `translateOutcomeToErrorCause` at `facade.go:1799-1806`
was checked directly and returns only `ErrProviderUnavailable` or `ErrNone`.

**The decisive check.** A repo-wide sweep of non-test Go for an `ErrorCause`
built from `Sprintf`, concatenation, `err.Error()`, or any format verb returned
**zero matches**. Exactly three `ErrorCause(` sites exist outside tests, all in
`facade.go`, all analysed above. The "no raw upstream error string" claim is
therefore substantiated by a negative sweep, not assumed from spot checks.

### Finding 2 — the refusal body is a constant that replaces rather than appends

**VERIFIED as stated. One scope boundary added.**

Every line citation is exact:

| Line | Statement | Effect |
|---|---|---|
| `gate.go:30` | `const CanonicalRefusalBody = "I don't have a sourced answer for that."` | Fixed string, no format verbs, no interpolation |
| `gate.go:117` | `resp.Body = CanonicalRefusalBody` | Plain assignment, so the ungrounded answer is **discarded** |
| `gate.go:118-119` | `StatusUnavailable` + `ErrNoGroundedAnswer` | Structural refusal discriminator |
| `gate.go:120` | `resp.CaptureRoute = false` | Leaves the capture path |
| `gate.go:123` | `resp.Sources = nil` | Drops partially-invalid provenance |

Because line 117 is `=` and not `+=`, the ungrounded model answer the gate
refused **cannot** reach the user through the gate's refusal path. That is the
core of the finding, and it holds.

**Strengthening the finding.** The transport does not always read the gate
constant, so checking `gate.go` alone would have been insufficient. Telegram
renders through `contracts.CanonicalRefusalBodyFor` at
`<repo-root>/internal/assistant/contracts/refusal.go:73`. That function is a
total switch over six package-level constants declared at `refusal.go:61-68`,
with a `default` arm returning the default body. Both body sources are closed.
The entire user-visible refusal vocabulary is six fixed strings.

**Scope boundary — one path preserves an existing body.** The facade backstop at
`facade.go:1834-1842` replaces the body **only** when it is empty or equal to
the capture acknowledgement. Otherwise it keeps the body it was handed. The
gate's replacement is unconditional. The backstop's is not. Finding 2's
"replaces rather than appends" is true of the gate, which is what it cites. Do
not generalise it to every honest-refusal path.

### Finding 3 — G034 repo floor is GREEN, and the earlier RED claim is FALSE

**VERIFIED. Run in this session, both ways.**

The invocation contract is as stated. The gate takes no spec-dir argument, and
passing one is a usage error.

```text
$ bash .github/bubbles/scripts/security-gate.sh specs/061-conversational-assistant/bugs/BUG-061-009-high-band-refusal-masked-as-saved-as-idea
security-gate: unknown argument: specs/061-conversational-assistant/bugs/BUG-061-009-high-band-refusal-masked-as-saved-as-idea
WITH_ARG_EXIT=2

$ bash .github/bubbles/scripts/security-gate.sh
[security-gate] OK — 931 tracked file(s), zero G034 findings
NO_ARG_EXIT=0
```

Both commands were run from `<repo-root>`. The repo floor is **GREEN**: 931
tracked files, zero G034 findings, exit 0.

The earlier assertion that "the G034 floor is RED" was tested against the real
gate and **did not hold**. It is recorded here as a failed verification rather
than dropped silently. A security finding relayed without execution is worse
than no finding, because it spends reviewer attention on a condition that does
not exist.

### Conclusion

The honest-refusal path is security-neutral **by construction**, not by
convention or reviewer vigilance. Two mechanisms carry it:

1. **A closed discriminator.** Every `ErrorCause` construction site writes a
   fixed literal. A future leak would require a new interpolating assignment,
   and the negative sweep above is the check that would catch one.
2. **A constant body.** Both refusal-body sources return one of six fixed
   strings, and the gate discards the ungrounded answer instead of appending to
   it.

The distinction matters. A property enforced by the shape of the code survives
a careless edit. A property enforced by habit does not.

### Residuals — stated, not cleared

| # | Residual | Status |
|---|---|---|
| SEC-R1 | Logging and telemetry on the refusal path | **not analysed** |
| SEC-R2 | Corpus-membership oracle in response shape | **unassessed** |

**SEC-R1.** This review covers the **user-visible** response surface only:
`Status`, `ErrorCause`, `Body`, `Sources`, and `CaptureRoute`. It did not
examine what the audit writer, the metrics counters, or structured logs record
when a refusal fires. One incidental observation is not a review: the counter at
`gate.go:106` is `ViolationsCounter.WithLabelValues(scenarioLabel, string(cause))`,
which is label-only and carries no turn text. That single site is not evidence
about the rest of the logging surface.

**SEC-R2.** An honest refusal is structurally distinguishable from an answer by
design. That is precisely what INV-HB-REFUSAL exists to guarantee. Whether that
distinguishability lets a user infer **whether a document exists** in a corpus
they do not own was not assessed. The question is noted as open. It is not
cleared, and nothing in this section should be read as clearing it.

---

## Validate — Independent Certification (`bubbles.validate`) — 2026-08-18

<a id="validate-certification"></a>

**Phase:** `validate` · **Agent:** `bubbles.validate` · **Date:** 2026-08-18 ·
**Claim Source:** executed · **Live system:** no

Independent certification at code HEAD `75eeb774`. ZERO code changed by this
run: no production file, no test file, no `scopes.md`, and no
`uservalidation.md`. The only files this run wrote are this section and
`state.json` (`completedPhases`, `certification.certifiedCompletedPhases`,
`certification.lockdownState`). `status` stays `blocked`.

Working tree at entry, executed this session:

```
$ git rev-parse --short HEAD
75eeb774
$ git status --porcelain -- internal/ cmd/ tests/ config/ docs/ .github/
$ git status --porcelain | wc -l
0
```

Both `git status` invocations returned zero lines, so every result below was
measured against committed source, not against uncommitted working-tree state.

**Path substitution declared.** Where a captured line printed the repository's
absolute checkout path, it is rendered `<repo-root>/…` here. That is the only
alteration made to any transcript in this section, and it is required by the
repository's no-absolute-path rule; nothing else was edited, reordered, or
summarised.

### Lane 1 — `./smackerel.sh test unit` — exit 0

Run through `.github/bubbles/scripts/evidence-capture.sh` so the receipt is
re-derivable rather than merely pasted:

```
# BUG-061-009 validate: ./smackerel.sh test unit
$ ./smackerel.sh test unit
exit: 0
lines: 441
sha256: 8f04bb694ee755d2d2c953bc532f03586c1763bba54e7a11284bfac4458eaeec
--- first 20 ---
oom-preflight: OK — 36292 MB available (need 6000 MB; swap used 1228 MB).
disk-preflight: OK — C: 76 GB free (need 40 GB), WSL / 488 GB free (need 25 GB).
++ dirname /workspace/scripts/runtime/go-unit.sh
[go-unit] envsubst missing — installing gettext-base
+ source /workspace/scripts/runtime/_ensure_envsubst.sh
+ ensure_envsubst go-unit
+ local tag=go-unit
+ command -v envsubst
+ echo '[go-unit] envsubst missing — installing gettext-base'
+ apt-get update -qq
+ apt-get install -y --no-install-recommends gettext-base
Reading package lists...
Building dependency tree...
Reading state information...
The following NEW packages will be installed:
  gettext-base
0 upgraded, 1 newly installed, 0 to remove and 20 not upgraded.
Need to get 160 kB of archives.
After this operation, 660 kB of additional disk space will be used.
Get:1 http://deb.debian.org/debian bookworm/main amd64 gettext-base amd64 0.21-12 [160 kB]
--- omitted 401 line(s); sha256 above covers the full output ---
--- last 20 ---
  ...
1..2
# tests 2
# suites 0
# pass 2
# fail 0
# cancelled 0
# skipped 0
# todo 0
# duration_ms 147.684489
PASS: bug_077_002_login_session_reuse_test (SCN-077-BUG-002-01 / SCN-077-BUG-002-02)
[test unit] -> bash <repo-root>/tests/unit/web/spec_077_discovery_convention_test.sh
PASS: spec_077_discovery_convention_test (TP-077-02-01 / SCN-077-A02)
[test unit] -> bash <repo-root>/tests/unit/web/spec_077_no_stub_bodies_test.sh
PASS: spec_077_no_stub_bodies_test (TP-077-03-06 / SCN-077-A08)
[test unit] shell unit tests in tests/unit/web/ finished OK
[test unit] running 1 shell unit test(s) from tests/unit/docs/
[test unit] -> bash <repo-root>/tests/unit/docs/spec_077_test_category_parity_test.sh
PASS: spec_077_test_category_parity_test (TP-077-02-03 / SCN-077-A06)
[test unit] shell unit tests in tests/unit/docs/ finished OK
```

`UNIT_EXIT=0`, 441 lines, wall 138s. Re-derive with
`bash .github/bubbles/scripts/evidence-capture.sh --verify 8f04bb694ee755d2d2c953bc532f03586c1763bba54e7a11284bfac4458eaeec -- ./smackerel.sh test unit`.

### Lane 2 — the two closure tests, forced to genuinely re-execute — exit 0

A full-suite pass is weak evidence for *these two* tests, because Go serves an
unchanged package from its test cache and prints `(cached)`; the lane would exit
0 without either test running. The `-run` pattern is part of Go's cache key, so
naming the tests explicitly forces real execution:

```
$ ./smackerel.sh test unit --go \
    --go-run 'TestRequiresProvenanceScenarios_ClosedOverSST|TestHighBandNeverMaskedAsSavedAsIdea' \
    --verbose

--- PASS: TestHighBandNeverMaskedAsSavedAsIdea (0.00s)
    --- PASS: TestHighBandNeverMaskedAsSavedAsIdea/weather_query/provider-error (0.00s)
    --- PASS: TestHighBandNeverMaskedAsSavedAsIdea/weather_query/timeout (0.00s)
    --- PASS: TestHighBandNeverMaskedAsSavedAsIdea/weather_query/ok_uncited (0.00s)
    --- PASS: TestHighBandNeverMaskedAsSavedAsIdea/retrieval_qa/provider-error (0.00s)
    --- PASS: TestHighBandNeverMaskedAsSavedAsIdea/retrieval_qa/timeout (0.00s)
    --- PASS: TestHighBandNeverMaskedAsSavedAsIdea/retrieval_qa/ok_uncited (0.00s)
    --- PASS: TestHighBandNeverMaskedAsSavedAsIdea/recipe_search/provider-error (0.00s)
    --- PASS: TestHighBandNeverMaskedAsSavedAsIdea/recipe_search/timeout (0.00s)
    --- PASS: TestHighBandNeverMaskedAsSavedAsIdea/recipe_search/ok_uncited (0.00s)
    --- PASS: TestHighBandNeverMaskedAsSavedAsIdea/open_knowledge/provider-error (0.00s)
    --- PASS: TestHighBandNeverMaskedAsSavedAsIdea/open_knowledge/timeout (0.00s)
    --- PASS: TestHighBandNeverMaskedAsSavedAsIdea/open_knowledge/ok_uncited (0.00s)
=== CONT  TestRequiresProvenanceScenarios_ClosedOverSST
    facade_high_band_invariant_coverage_test.go:90: SST requires_provenance scenarios (all swept by the INV-HB-REFUSAL invariant): [open_knowledge recipe_search retrieval_qa weather_query]
--- PASS: TestRequiresProvenanceScenarios_ClosedOverSST (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/assistant       0.558s
testing: warning: no tests to run
PASS
```

`SCOPED_GO_EXIT=0`, wall 82s. Three facts carry the certification, and each is
visible in the transcript rather than inferred:

1. **It really ran.** The package line reads `ok … internal/assistant 0.558s`,
   not `(cached)`. A measured duration is what distinguishes an execution from a
   cache hit.
2. **The sweep is closed over the SST, and the closure is not vacuous.** The
   test's own `t.Logf` names the set it read from `config/assistant/scenarios.yaml`:
   `[open_knowledge recipe_search retrieval_qa weather_query]` — four scenarios,
   printed by the code rather than asserted by this report. `open_knowledge` is
   present, which is the specific drift DI-2 recorded.
3. **The invariant sweep traverses all four.** Twelve subtests passed — each of
   the four scenarios against each of `provider-error`, `timeout`, `ok_uncited`.
   The `open_knowledge/ok_uncited` row is the exact `/ask` path this bug was
   reported against.

### Lane 3 — `bash .github/bubbles/scripts/artifact-lint.sh <packet>` — exit 0

```
✅ Detected state.json status: blocked
✅ Detected state.json workflowMode: bugfix-fastlane
✅ All DoD bullet items use checkbox syntax in scopes.md
✅ uservalidation separates automation readiness from human acceptance
✅ report.md contains section matching: Summary / Completion Statement / Test Evidence
=== Anti-Fabrication Evidence Checks ===
✅ All checked DoD items in scopes.md have evidence blocks
✅ No unfilled evidence template placeholders in scopes.md
✅ No unfilled evidence template placeholders in report.md
✅ No repo-CLI bypass detected in report.md command evidence
Artifact lint PASSED.
ARTIFACT_LINT_EXIT=0
```

### Phase-to-evidence cross-check

`completedPhases` carried eleven entries. Certification is not a restatement of
that list — each entry was checked against this report for a section that
actually evidences it, because a phase recorded with no evidence is a claim, not
a record. Section headings and `**Phase:**` provenance tags were enumerated with
`grep` over `report.md` this session.

| Phase in `completedPhases` | Evidencing section in `report.md` | Certified |
|---|---|---|
| `select` | none found | **no** |
| `bootstrap` | none found | **no** |
| `implement` | *Test Evidence* (pre-fix red → post-fix green, exit 0) + *Implementation Delta → Code Diff Evidence* (git-backed; guard Check 13B / G053 PASS) | yes |
| `test` | *Test Evidence* | yes |
| `harden` | none found — the string `harden` does not occur anywhere in `report.md` | **no** |
| `docs` | none found — no section, no `**Phase:** docs` tag, and no mention of either documentation surface | **no** |
| `regression` | *Regression Invariant Closure* (`**Phase:** regression`) | yes |
| `simplify` | *Simplify — Post-Implementation Review Of The Regression Diff* (`**Phase:** simplify`) | yes |
| `stabilize` | *Stabilize — Stability, Performance, Reliability And Resource Assessment* (`**Phase:** stabilize`) | yes |
| `security` | *Security — Refusal-Path Disclosure Review* (`**Phase:** security`) | yes |
| `audit` | *Audit — Adversarial Reversion Probe* | yes |
| `validate` | this section | yes |

Two entries deserve their reasoning stated rather than left to the table.

**`docs` is declined even though the documentation change is real.** Both
surfaces the packet claims carry the invariant do carry it —
`grep -c 'INV-HB-REFUSAL' docs/smackerel.md .github/copilot-instructions.md`
returned `1` for each. The work happened. What is missing is any record of it in
this report: no section, no phase tag, no reference to either file. Certifying a
phase on the strength of a grep this agent ran, rather than on the phase's own
recorded evidence, would invert the direction certification is supposed to run.
The change stands on its own; the phase claim does not.

**`implement` is certified on two artifacts, not one.** The *Code Diff Evidence*
block is tagged `**Phase:** regression` and documents commit `5c24a74f`, the
regression-phase SST-closure test — not the original SCOPE-01..05 delta. What
evidences the implementation is the red→green pair in *Test Evidence*: a first
run with three named pre-fix failures at the guard update points, then a second
run at exit 0. That pairing is execution evidence; the guard's Check 13B
independently passed the implementation-delta requirement.

The four declined phases are all outside the `bugfix-fastlane` required set
(`implement`, `test`, `regression`, `simplify`, `stabilize`, `security`,
`validate`, `audit`), so declining them withholds an unevidenced claim without
withholding anything G022 requires.

### `lockdownState` — what was checked before choosing a value

The recorded value is the string `n/a-no-locked-scenarios`, matching the
established convention in this repository (4 existing uses, e.g.
`specs/095-retrieval-strategy-routing/bugs/BUG-095-001-route-guard-compiler-provenance/state.json`).
It is accurate: the guard counts locked scenarios by matching
`"lockdown": true` in `scenario-manifest.json`, and this packet's manifest
contains zero such entries (`grep -cE '"lockdown"[[:space:]]*:[[:space:]]*true'`
returned `0`, exit 1).

One distinction is worth recording so the value is not read as more than it
says. This packet's manifest **does** mark four of its five scenarios
`"regressionProtected": true`. That is a different field from the one the
lockdown check reads, and `n/a-no-locked-scenarios` speaks only to the lockdown
concept — it is not a statement that the packet lacks regression protection.

### Verdict

The INV-HB-REFUSAL claim is **SUBSTANTIATED** at HEAD `75eeb774`: the full unit
lane is green, and the two tests that carry the invariant were forced past Go's
cache and passed, with the swept scenario set printed by the code and closed
over the scenario SST.

Certification is **partial and the status stays `blocked`**, for two reasons
that are not this agent's to clear:

- **G136 — human acceptance.** `uservalidation.md` does not establish human
  acceptance. Its four items are checked, but its own note says they are checked
  by default on the strength of the mechanical invariant tests. That is
  automation readiness, not a human exercising the deployed bot. Only the
  operator can supply it; an agent checking those items would fabricate the
  acceptance the gate exists to require. This file was not touched.
- **Four regression-E2E planning gaps (guard Check 8A).** `scopes.md` is missing
  the scenario-specific regression-E2E DoD item, the broader E2E regression suite
  DoD item, and the explicit scenario-specific regression-E2E Test Plan row.
  `scopes.md` is owned by `bubbles.plan`; this agent does not edit it.

> **Addendum — 2026-08-18, `bubbles.plan`.** The second bullet has since been
> actioned by the owning agent and is recorded here rather than edited in place,
> because the verdict above was accurate when written and rewriting another
> agent's certification would destroy the audit trail. `scopes.md` now carries
> the three Check 8A entries in a new *Packet-Wide Regression E2E Coverage — NOT
> DELIVERED* section. **They were added UNCHECKED.** No live E2E asserts this
> packet's honest-refusal wire contract; a search of the whole `tests/` tree for
> `ErrNoGroundedAnswer`, `no_grounded_answer`, `StatusUnavailable` and the
> canonical body string found none, and the one integration test that names
> INV-HB-REFUSAL exercises the cite-back verifier rather than the facade envelope.
>
> Check 8A's regexes accept `[ ]`, so Check 8A now reports green while zero E2E
> coverage exists — it asks whether coverage is *planned*, not *delivered*. **A
> green Check 8A on this packet must not be read as E2E coverage.** What prevents
> that from being a loophole is Check 4: this packet's `scenario-manifest.json`
> declares five scenarios but none carries a canonical `id`, so the resolver
> cannot key them and Check 4 falls back to the legacy checkbox basis, where
> unchecked DoD items are blocking.
>
> Guard measured before and after, both runs captured in this report:
> `failureCount: 5, failedChecks: []` → `failureCount: 2, failedChecks:
> [Check-4-completion]`. Four Check 8A planning-shape failures cleared and one
> real completion failure replaced them, so the packet is now blocked on the
> absence of E2E coverage rather than on the shape of its plan. The lower count
> is not increased readiness. G136 above is unaffected and the status stays
> `blocked`.

---

## Discovered Issues

Issues surfaced while working this packet that are not the reported defect. Each
carries a disposition and a concrete reference, so none survives as an
unattributed aside.

| # | Date | Issue | Disposition | Reference |
|---|------|-------|-------------|-----------|
| DI-1 | 2026-08-18 | `open_knowledge` grounds nothing for a question about smackerel's own product. The agent terminates `status=success termination=final` with zero citable sources, so the provenance gate refuses. The refusal is correct anti-fabrication; the gap is that the user wants an answer, and closing it needs retrieval/ingestion investigation, not a facade change. | **routed** — owned by a real bug artifact, not by this packet. BUG-061-009 owns only INV-HB-REFUSAL (make the refusal honest) and makes no claim about making `/ask` answer. | `BUG-061-010-open-knowledge-grounding-gap` at `specs/061-conversational-assistant/bugs/BUG-061-010-open-knowledge-grounding-gap/` (`bug.md`, `state.json`); diagnosis in *Grounding-gap diagnosis and routing (SCOPE-05)* above |
| DI-2 | 2026-08-18 | The `requiresProvenanceScenarios` sweep list in `internal/assistant/facade_execution_error_honesty_test.go` had drifted from the scenario SST: it named `weather_query`, `retrieval_qa`, `recipe_search` but not `open_knowledge` — the exact `/ask` path this bug was reported against — while the packet cited that sweep as proof no band-high path can render the capture acknowledgement. | **fixed-in-session** — `open_knowledge` added to the sweep, and `TestRequiresProvenanceScenarios_ClosedOverSST` added in `internal/assistant/facade_high_band_invariant_coverage_test.go` to close the list over `config/assistant/scenarios.yaml` bidirectionally, so the same drift now fails the unit suite instead of shipping silently. | commit `5c24a74f`; diff in *Code Diff Evidence* above; execution proof in *Regression Invariant Closure* above |
| DI-3 | 2026-08-18 | The `## Grounding-gap …` routing paragraph in this report named `BUG-061-010-open-knowledge-grounding-gap` as "(to be created)". That was accurate when written; the artifact has since been created, so the parenthetical had become a false statement about the state of the repository. | **fixed-in-session** — corrected to a completed routing decision citing the artifact's on-disk path, verified present before the edit. A routing claim naming a non-existent artifact is indistinguishable from no routing, which is why this was repaired rather than reworded. | `specs/061-conversational-assistant/bugs/BUG-061-010-open-knowledge-grounding-gap/` (`bug.md`, `state.json`); *Grounding-gap diagnosis and routing (SCOPE-05)* above |
| DI-4 | 2026-08-18 | `state.json.deployment.sourceSha` (`2e84a1b4…`) is a real commit — the BUG-061-009 fix commit — but it is not an ancestor of `HEAD` and no branch contains it, so history was rewritten after the deploy was recorded. The deployed digest therefore cannot be rebuilt from the SHA the deployment block names. The behaviour under test is unaffected: the whole refusal surface (`provenance/gate.go`, `contracts/refusal.go`, both adapters' render paths, `config/assistant/scenarios.yaml`) is byte-identical between that commit and `HEAD`, so the deployed image genuinely carries the fix and the pending operator smoke test stays valid. What is broken is the provenance chain from record to artifact, not the artifact. | **routed** — not fixable by a stabilize edit. Resolution is either re-recording the deployment against a reachable SHA or rebuilding and redeploying from `HEAD`; both are deployment-record actions owned by `bubbles.devops` / `bubbles.train`. Not an incident: nothing is degraded and nothing is failing. | git evidence in *Stabilize — Stability, Performance, Reliability And Resource Assessment → Deployment-provenance check* above; `state.json` `deployment` block |
| DI-5 | 2026-08-18 | **A live E2E cannot fail on the regression its own header advertises.** In `tests/e2e/assistant/capture_fallback_trigger_e2e_test.go`, `TestAssistantHTTPE2E_CaptureFallbackOpenKnowledgeNoGround` (spec 074 SCOPE-04B, TP-074-14 / SCN-074-A01) guards at line 98 with `if env.Status != string(contracts.StatusSavedAsIdea) { t.Skipf(...) }`. All four SCOPE-074-04B contract assertions sit *below* that guard: `capture_route=true` (line 103), nil `confirm_card` (106), nil `disambiguation_prompt` (109), and the canonical `saved as an idea` body (117). The header at line 16 claims that "if the facade … routed to a different status (regression of SCOPE-074-04B canonical-ack rule), this test would fail." A different status is exactly what triggers the skip, so that half of the header claim is false as written — the outcome is SKIP, not FAIL. **Stated precisely, because the defect is narrower than "the test does nothing":** the test is not inert wholesale — `facade_invoked` (83), `transport` (86) and `transport_message_id` echo (89) assert unconditionally, and the header's *other* half ("the facade silently dropped the no-ground capture") does remain enforceable via line 103, but only while the status is still `saved_as_idea`. What is unenforceable is any regression that moves the status *off* `saved_as_idea` — which is the canonical-ack rule itself. A second `t.Skipf` at line 71 (adapter not ready after 5 min) adds a further silent-pass path. This is a failure-condition early exit, the pattern `.github/copilot-instructions.md` → *Adversarial Regression Tests For Bug Fixes* explicitly forbids. | **routed — spec-074-owned, not this packet's to fix.** BUG-061-009 changed the facade's band-high refusal rendering, so it is reasonable to ask whether it perturbs this test; the honest answer is that **no contradiction was proven and none is asserted here.** Spec 074 SCOPE-04B governs an open_knowledge turn the agent *refused* (`status="refused"`) landing on `SavedAsIdea` + `capture_route=true`. BUG-061-009 governs a band-high `requires_provenance` turn that returns *OK but uncited*, rendering `StatusUnavailable` + `ErrNoGroundedAnswer`. Those are plausibly different branches and may legitimately coexist. **Not verified:** which branch the live stack actually takes for this test's fabricated-city prompt — that needs a live run, which was not performed. The defect that holds regardless of branch is the skip-guard: were that prompt ever to move onto the honest-refusal branch, this test would report SKIP rather than FAIL and would tell nobody. Severity is test-integrity, not production behaviour — no user-facing defect is implied by this finding. Recommended id to file against spec 074, **not filed here**: `BUG-074-002-noground-e2e-skip-guard-masks-canonical-ack-regression`, whose fix is to assert the envelope unconditionally (an unexpected status is the hunted regression, so it must fail) or, if LLM nondeterminism genuinely requires tolerance, to pin the no-ground branch with a deterministic stub so the assertions always run. | `tests/e2e/assistant/capture_fallback_trigger_e2e_test.go` lines 16, 71, 98–101, 103, 106, 109, 117 (verified by reading the file 2026-08-18; not edited — `tests/e2e/` is outside this packet's ownership). Contract sources: `specs/074-capture-as-fallback-policy/` SCOPE-04B; this packet's `spec.md` INV-HB-REFUSAL. Next id available under `specs/074-capture-as-fallback-policy/bugs/` (only `BUG-074-001-canonical-capture-response`, status `done`, exists and does not cover this). |
