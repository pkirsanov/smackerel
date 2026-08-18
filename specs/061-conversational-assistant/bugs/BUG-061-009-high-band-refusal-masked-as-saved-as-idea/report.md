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

## Discovered Issues

Issues surfaced while working this packet that are not the reported defect. Each
carries a disposition and a concrete reference, so none survives as an
unattributed aside.

| # | Date | Issue | Disposition | Reference |
|---|------|-------|-------------|-----------|
| DI-1 | 2026-08-18 | `open_knowledge` grounds nothing for a question about smackerel's own product. The agent terminates `status=success termination=final` with zero citable sources, so the provenance gate refuses. The refusal is correct anti-fabrication; the gap is that the user wants an answer, and closing it needs retrieval/ingestion investigation, not a facade change. | **routed** — owned by a real bug artifact, not by this packet. BUG-061-009 owns only INV-HB-REFUSAL (make the refusal honest) and makes no claim about making `/ask` answer. | `BUG-061-010-open-knowledge-grounding-gap` at `specs/061-conversational-assistant/bugs/BUG-061-010-open-knowledge-grounding-gap/` (`bug.md`, `state.json`); diagnosis in *Grounding-gap diagnosis and routing (SCOPE-05)* above |
| DI-2 | 2026-08-18 | The `requiresProvenanceScenarios` sweep list in `internal/assistant/facade_execution_error_honesty_test.go` had drifted from the scenario SST: it named `weather_query`, `retrieval_qa`, `recipe_search` but not `open_knowledge` — the exact `/ask` path this bug was reported against — while the packet cited that sweep as proof no band-high path can render the capture acknowledgement. | **fixed-in-session** — `open_knowledge` added to the sweep, and `TestRequiresProvenanceScenarios_ClosedOverSST` added in `internal/assistant/facade_high_band_invariant_coverage_test.go` to close the list over `config/assistant/scenarios.yaml` bidirectionally, so the same drift now fails the unit suite instead of shipping silently. | commit `5c24a74f`; diff in *Code Diff Evidence* above; execution proof in *Regression Invariant Closure* above |
| DI-3 | 2026-08-18 | The `## Grounding-gap …` routing paragraph in this report named `BUG-061-010-open-knowledge-grounding-gap` as "(to be created)". That was accurate when written; the artifact has since been created, so the parenthetical had become a false statement about the state of the repository. | **fixed-in-session** — corrected to a completed routing decision citing the artifact's on-disk path, verified present before the edit. A routing claim naming a non-existent artifact is indistinguishable from no routing, which is why this was repaired rather than reworded. | `specs/061-conversational-assistant/bugs/BUG-061-010-open-knowledge-grounding-gap/` (`bug.md`, `state.json`); *Grounding-gap diagnosis and routing (SCOPE-05)* above |
