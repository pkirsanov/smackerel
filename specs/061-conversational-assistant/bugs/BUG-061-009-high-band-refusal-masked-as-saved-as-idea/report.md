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
