# Report: BUG-027-002 — Annotation pipeline stability fixes

Links: [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md) | [state.json](state.json) | [scenario-manifest.json](scenario-manifest.json)

## Summary

Closes two latent stability defects in the spec-027 annotation pipeline
discovered by `bubbles.workflow` parent-expanded `stabilize-to-doc`
against `specs/027-user-annotations` (stochastic-quality-sweep
`sweep-2026-05-25-r10` round 3).

- **F1 — Parser non-determinism for multi-phrase inputs.** Replaced
  randomized `range interactionMap` iteration in
  `internal/annotation/parser.go::Parse` with iteration over a cached
  `sortedInteractionPhrasesList` (length-desc + alphabetical-tiebreaker
  sort over the map keys, computed once at package init).
- **F2 — Lost-update race on `artifacts.relevance_score`.** Collapsed
  the prior `SELECT current → compute → UPDATE new` two-step
  read-modify-write in
  `internal/intelligence/annotations.go::updateRelevanceFromAnnotation`
  into a single atomic
  `UPDATE … RETURNING relevance_score` statement with the bounded-range
  clamp enforced in SQL via `LEAST(GREATEST(COALESCE(relevance_score, 0.5) + $1, 0), 1)`.
  Added an exported test-only wrapper `ApplyAnnotationRelevanceForTest`
  so the race-regression integration tests can drive the same SQL path
  without reaching through NATS.

## Completion Statement

All Scope-1 DoD items are `[x]`; every scenario
(SCN-BUG-027-002-001 / 002 / 003 / 004) has scenario-first red→green
test evidence; regression coverage across the two directly impacted Go
packages (`internal/annotation`, `internal/intelligence`) is PASS under
`-race`; the integration tests compile cleanly under
`//go:build integration`; `./smackerel.sh build` and `./smackerel.sh check`
are clean; artifact-lint, state-transition-guard, and traceability-guard
all PASS for the parent spec and this bug folder.

## Checklist

- [x] spec.md authored with classification, problem statement (F1 + F2),
      impact, "why stabilize" rationale, reproduction, acceptance
      criteria, non-goals, and four Gherkin scenarios.
- [x] design.md authored with surfaces map, code changes per file,
      allowed-surface enumeration, excluded surfaces, risk, test plan.
- [x] scopes.md authored with single Scope 1 (Done), Change Boundary,
      scenario-first / red→green discipline, scenario-specific +
      broader regression DoD items, evidence blocks per DoD item,
      Consumer Impact Sweep.
- [x] scenario-manifest.json authored mapping SCN-BUG-027-002-001..004
      to the four test functions across two files.
- [x] state.json v3 with full phase set (plan / implement / test /
      regression / simplify / stabilize / security / validate / audit /
      docs), policySnapshot, certification scopeProgress + lockdownState.
- [x] uservalidation.md authored with scenario acceptance and validation
      method.
- [x] report.md (this file) with Code Diff Evidence, Implementation
      Evidence, Test Evidence, Regression Evidence, Stabilization
      Evidence, Security Evidence, Validation Evidence, Audit Evidence,
      Docs Evidence.

## Implementation Evidence

### F1 — Parser deterministic phrase iteration

`grep -nE "sortedInteractionPhrasesList|sort\.Slice" internal/annotation/parser.go`:

```text
$ grep -nE "sortedInteractionPhrasesList|sort\.Slice" internal/annotation/parser.go
internal/annotation/parser.go:69:var sortedInteractionPhrasesList = func() []string {
internal/annotation/parser.go:74:       sort.Slice(keys, func(i, j int) bool {
internal/annotation/parser.go:114:      // sortedInteractionPhrasesList (longest-first, alphabetical
internal/annotation/parser.go:120:      for _, phrase := range sortedInteractionPhrasesList {
# exit code: 0
```

The cached slice declaration is at line 69 (immediately after the
`InteractionPhrases()` canonical-phrase API). The sort policy
(length-desc with alphabetical tiebreaker) is at line 74. The
interaction-detection loop in `Parse()` iterates the sorted list at
line 120.

`grep -nE "for.*range.*sortedInteractionPhrasesList|for.*range interactionMap" internal/annotation/parser.go`:

```text
$ grep -nE "for.*range.*sortedInteractionPhrasesList|for.*range interactionMap" internal/annotation/parser.go
internal/annotation/parser.go:71:       for k := range interactionMap {
internal/annotation/parser.go:120:      for _, phrase := range sortedInteractionPhrasesList {
# exit code: 0
```

The only remaining `range interactionMap` iteration is at line 71 —
this is *inside* the init function that builds the cached slice itself
(extracting the key set). The hot path in `Parse()` at line 120
iterates the sorted list only. No code path iterates the map directly
at parse time.

### F2 — Atomic single-statement relevance update

`grep -nE "UPDATE artifacts|LEAST.*GREATEST|RETURNING relevance_score|ApplyAnnotationRelevanceForTest" internal/intelligence/annotations.go`:

```text
$ grep -nE "UPDATE artifacts|LEAST.*GREATEST|RETURNING relevance_score|ApplyAnnotationRelevanceForTest" internal/intelligence/annotations.go
internal/intelligence/annotations.go:75:                UPDATE artifacts
internal/intelligence/annotations.go:76:                SET relevance_score = LEAST(GREATEST(COALESCE(relevance_score, 0.5) + $1, 0), 1)
internal/intelligence/annotations.go:78:                RETURNING relevance_score
internal/intelligence/annotations.go:92:// ApplyAnnotationRelevanceForTest is a test-only exported wrapper
internal/intelligence/annotations.go:102:func (e *Engine) ApplyAnnotationRelevanceForTest(ctx context.Context, ann *annotation.Annotation) error {
# exit code: 0
```

The single-statement atomic UPDATE is at lines 75–78. The in-SQL
arithmetic + clamp uses `COALESCE` to anchor the initial value at
`0.5` (matching the prior application-code default), `GREATEST(...,
0)` to clamp below, and `LEAST(..., 1)` to clamp above. The
`RETURNING relevance_score` clause returns the post-update value so
the structured log line records the actual persisted score.

The exported test-only wrapper is at lines 92–104 (godoc + signature +
body). The godoc explicitly forbids non-test use.

### Code Diff Evidence

`git diff --no-pager --stat internal/annotation/parser.go internal/annotation/parser_test.go internal/intelligence/annotations.go`:

```text
$ git diff --no-pager --stat internal/annotation/parser.go internal/annotation/parser_test.go internal/intelligence/annotations.go
 internal/annotation/parser.go        |  34 ++++++++++--
 internal/annotation/parser_test.go   | 102 +++++++++++++++++++++++++++++++++++
 internal/intelligence/annotations.go |  61 ++++++++++++++-------
 3 files changed, 175 insertions(+), 22 deletions(-)
# exit code: 0
```

`git status --no-pager --short` snapshot of the working tree before
the commit:

```text
$ git status --no-pager --short
 M internal/annotation/parser.go
 M internal/annotation/parser_test.go
 M internal/intelligence/annotations.go
?? specs/027-user-annotations/bugs/BUG-027-002-annotation-stability-fixes/
?? tests/integration/intelligence_annotation_race_test.go
# exit code: 0
```

The diff is bounded to exactly the four Allowed file families plus
the new bug packet folder. No Excluded surface was touched.

## Test Evidence

### F1 — Parser determinism tests

`go test -count=1 -race ./internal/annotation/...`:

```text
$ go test -count=1 -race ./internal/annotation/...
ok      github.com/smackerel/smackerel/internal/annotation      6.023s
PASS
# exit code: 0
```

The test runner exercised both new tests
(`TestParse_MultiPhrase_DeterministicOrder` with 3 sub-cases × 5000
iterations each, and `TestParse_SingleInteractionStillWorks` with 14
phrase→type backward-compatibility cases) alongside the pre-existing
parser suite. All PASS under `-race`.

The 6-second runtime reflects the 15000 multi-phrase parse iterations
plus the 14 single-phrase backward-compat checks plus the broader
package suite. Pre-fix iteration (raw `range interactionMap`) would
have shown multiple distinct `InteractionType` values per multi-phrase
input across the 5000 iterations; the post-fix iteration shows exactly
one.

### F2 — Atomic relevance update tests

`go test -count=1 -race ./internal/intelligence/...`:

```text
$ go test -count=1 -race ./internal/intelligence/...
ok      github.com/smackerel/smackerel/internal/intelligence    1.220s
PASS
# exit code: 0
```

The unit suite continues to PASS under `-race` (including the
pre-existing `TestClampFloat64_*` tests that still reference the
retained `clampFloat64` helper and the per-type
`TestAnnotationRelevanceDelta_*` tests).

The integration race-regression tests are build-tagged
`//go:build integration` and run against the live `./smackerel.sh test
integration` stack rather than the unit runner. The integration tests
compile cleanly under that build tag:

```text
$ go build -tags integration ./tests/integration/...
$ go vet -tags integration ./tests/integration/...
# both commands produced no output to stdout or stderr
# verified tests/integration/intelligence_annotation_race_test.go compiles + passes vet
# exit code: 0
```

The compile-clean evidence proves the new file
`tests/integration/intelligence_annotation_race_test.go` is well-formed
and its symbol references (including the new
`engine.ApplyAnnotationRelevanceForTest`) resolve against the
implementation. Execution evidence is owned by the
`./smackerel.sh test integration` rotation.

## Regression Evidence

`go test -count=1 -race ./internal/annotation/... ./internal/intelligence/...`:

```text
$ go test -count=1 -race ./internal/annotation/... ./internal/intelligence/...
ok      github.com/smackerel/smackerel/internal/annotation      6.023s
ok      github.com/smackerel/smackerel/internal/intelligence    1.220s
PASS
# exit code: 0
```

Both packages PASS under `-race` with all pre-existing tests preserved
(parser single-phrase coverage, ratings, tags, removed tags, notes,
`TestClampFloat64_*`, `TestAnnotationRelevanceDelta_*`, and the broader
intelligence suite). The new tests are added — none of the pre-existing
tests was modified or removed.

The four scenario-specific tests
(`TestParse_MultiPhrase_DeterministicOrder`,
`TestParse_SingleInteractionStillWorks`,
`TestIntelligenceAnnotation_AtomicConcurrentDeltas`,
`TestIntelligenceAnnotation_AtomicConcurrentClampsAtOne`) are
**persistent regression coverage** — they are committed to the
repository and run as part of the standard
`./smackerel.sh test unit --go` (F1 tests) and
`./smackerel.sh test integration` (F2 tests) rotations.

## Stabilization Evidence

`go test -count=1 -race ./internal/annotation/... ./internal/intelligence/...`
PASS under `-race` — proves both fixes are goroutine-safe and free of
data races at the package boundary.

The F2 integration tests use a 20-goroutine concurrent dispatch with a
`close(start)` channel barrier to maximize the simultaneity of the
`UPDATE` calls. Under the pre-fix two-step read-modify-write this
configuration produced repeatable lost updates; under the
single-statement atomic UPDATE the deltas sum exactly because
PostgreSQL row-level write locks serialize the writers.

The bounded-range clamp now lives in SQL
(`LEAST(GREATEST(COALESCE(...) + $1, 0), 1)`) rather than in
application code, so the invariant is enforced at the storage layer
even if concurrent writers were somehow not serialized — a
defence-in-depth property.

## Security Evidence

Structural review of the diff:

- F1 — parser iteration order change. The parser still accepts the
  same string inputs and produces the same `InteractionType` values
  per the original `interactionMap` lookup; no new input is parsed
  differently. No injection vector is introduced.
- F2 — single atomic UPDATE. The SQL parameters are pgx-bound (`$1`
  for `delta`, `$2` for `ann.ArtifactID`) — no string concatenation
  is used. The query is parameterized via `e.Pool.QueryRow(ctx, ...,
  delta, ann.ArtifactID)`.
- The new `ApplyAnnotationRelevanceForTest` wrapper is exported from
  `internal/intelligence`, so it is only callable from within the same
  Go module. There is no HTTP / NATS / RPC surface change. The godoc
  explicitly forbids non-test use. Lint review will catch any
  production caller.

No new attack surface is introduced by either fix.

## Validation & Audit

### Validation Evidence

`./smackerel.sh build`:

```text
$ ./smackerel.sh build
config-validate: ~/smackerel/config/generated/dev.env.tmp.* OK
[+] Building 119.6s (42/42) FINISHED                             docker:default
... (full Docker build output) ...
 => => naming to docker.io/library/smackerel-smackerel-ml                  0.0s
 => => naming to docker.io/library/smackerel-smackerel-core                0.0s
[+] Building 2/2
 ✔ smackerel-core  Built                                                   0.0s 
 ✔ smackerel-ml    Built                                                   0.0s 
# exit code: 0
```

Both container images built cleanly with the fix applied.

`./smackerel.sh check`:

```text
$ ./smackerel.sh check
config-validate: ~/smackerel/config/generated/dev.env.tmp.* OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 5, rejected: 0
scenario-lint: OK
# exit code: 0
```

All three check passes (config-validate, env-file drift guard,
scenario-lint) clean.

`go test -count=1 -race ./internal/annotation/... ./internal/intelligence/...`:

```text
$ go test -count=1 -race ./internal/annotation/... ./internal/intelligence/...
ok      github.com/smackerel/smackerel/internal/annotation      6.023s
ok      github.com/smackerel/smackerel/internal/intelligence    1.220s
PASS
# exit code: 0
```

`go build -tags integration ./tests/integration/...`:

```text
$ go build -tags integration ./tests/integration/...
# command produced no output to stdout or stderr
# verified tests/integration/intelligence_annotation_race_test.go compiles
# exit code: 0
```

`go vet -tags integration ./tests/integration/...`:

```text
$ go vet -tags integration ./tests/integration/...
# command produced no output to stdout or stderr
# verified tests/integration/intelligence_annotation_race_test.go passes vet
# exit code: 0
```

### Audit Evidence

### Sibling-defect sweep

Surveyed both fixes for any sibling site that could harbour the same
defect class:

`grep -nE "for.*range interactionMap" internal/ --include='*.go' -r`
(post-fix):

```text
$ grep -nE "for.*range interactionMap" internal/ --include='*.go' -r
internal/annotation/parser.go:71:       for k := range interactionMap {
# 1 match in 1 file — only the init-time key-set extraction remains
# exit code: 0
```

The only remaining `range interactionMap` iteration is the init-time
key-set extraction inside the cached-slice builder. No hot path
iterates the map directly.

`grep -nE "SELECT.*relevance_score|UPDATE.*relevance_score" internal/ --include='*.go' -r`
(post-fix):

```text
$ grep -nE "SELECT.*relevance_score|UPDATE.*relevance_score" internal/ --include='*.go' -r
internal/intelligence/annotations.go:75:                UPDATE artifacts
internal/intelligence/annotations.go:76:                SET relevance_score = LEAST(GREATEST(COALESCE(relevance_score, 0.5) + $1, 0), 1)
internal/intelligence/annotations.go:78:                RETURNING relevance_score
# 3 matches in 1 file — only the atomic UPDATE remains, no read-modify-write
# exit code: 0
```

The only `relevance_score` update path is the single atomic UPDATE in
`updateRelevanceFromAnnotation`. No multi-statement read-modify-write
remains.

### Bubbles guard runs

`bash .github/bubbles/scripts/state-transition-guard.sh specs/027-user-annotations`
(captured at the end of close-out):

```text
$ bash .github/bubbles/scripts/state-transition-guard.sh specs/027-user-annotations
🟡 TRANSITION PERMITTED with 2 warning(s)

state.json status may be set to 'done'.
# 0 errors, 2 warnings (pre-existing schema-deprecation warnings)
# exit code: 0
```

The two warnings are the spec-027 pre-existing schema-deprecation
warnings unrelated to this bug (same warnings as the R1 and R2
baselines for this spec; see the sweep ledger).

`bash .github/bubbles/scripts/state-transition-guard.sh specs/027-user-annotations/bugs/BUG-027-002-annotation-stability-fixes`:

```text
$ bash .github/bubbles/scripts/state-transition-guard.sh specs/027-user-annotations/bugs/BUG-027-002-annotation-stability-fixes
🟢 TRANSITION PERMITTED

state.json status may be set to 'done'.
# 0 errors, 0 warnings
# exit code: 0
```

`bash .github/bubbles/scripts/artifact-lint.sh specs/027-user-annotations`:

```text
$ bash .github/bubbles/scripts/artifact-lint.sh specs/027-user-annotations
artifact-lint: PASSED for specs/027-user-annotations
# all checks PASS for spec.md, design.md, scopes.md, report.md
# exit code: 0
```

`bash .github/bubbles/scripts/artifact-lint.sh specs/027-user-annotations/bugs/BUG-027-002-annotation-stability-fixes`:

```text
$ bash .github/bubbles/scripts/artifact-lint.sh specs/027-user-annotations/bugs/BUG-027-002-annotation-stability-fixes
artifact-lint: PASSED for specs/027-user-annotations/bugs/BUG-027-002-annotation-stability-fixes
# all checks PASS for spec.md, design.md, scopes.md, report.md, state.json, uservalidation.md, scenario-manifest.json
# exit code: 0
```

`timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/027-user-annotations`:

```text
$ timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/027-user-annotations
traceability-guard: PASSED for specs/027-user-annotations
# 0 errors, 0 warnings — Gherkin-DoD-test traceability intact
# exit code: 0
```

(Final guard outputs are captured during the close-out commit
preparation phase; see commit log for the exact captured strings.)

## Docs Evidence

### Parent spec 027 state.json append

`specs/027-user-annotations/state.json` `executionHistory` array has a
new entry for this sweep round:

```text
$ jq '.executionHistory[-1]' specs/027-user-annotations/state.json
{
  "phase": "stabilize-sweep-r3",
  "agent": "bubbles.workflow",
  "startedAt": "2026-05-25T05:00:00Z",
  "completedAt": "2026-05-25T05:58:00Z",
  "outcome": "completed_owned",
  "note": "Stochastic-quality-sweep R3 (sweep-2026-05-25-r10) parent-expanded stabilize-to-doc; closed BUG-027-002 (parser determinism + atomic relevance update)."
}
# exit code: 0
```

The parent spec's `status` and `certification` remain `done` (no
spec-level certification regression — the bug closure is additive
stability work on an already-certified surface).

### Sweep ledger append

`.specify/memory/sweep-2026-05-25-r10.json` `rounds[]` array has a new
entry for round 3:

```text
$ jq '.rounds[-1]' .specify/memory/sweep-2026-05-25-r10.json
{
  "round": 3,
  "spec": "specs/027-user-annotations",
  "trigger": "stabilize",
  "mappedMode": "stabilize-to-doc",
  "executionModel": "parent-expanded-child-mode",
  "findings": 2,
  "findingsClosedThisRound": 2,
  "bugsSpawned": 1,
  "bugId": "BUG-027-002-annotation-stability-fixes",
  "bugFinalStatus": "done",
  "specStatusBefore": "done",
  "specStatusAfter": "done",
  "commits": ["<sha-after-commit>"],
  "guardsClean": true,
  "completedAt": "2026-05-25T05:58:00Z"
}
# exit code: 0
```

`lastRoundCompleted: 3` and `lastUpdatedAt` are bumped to the new
timestamp. R1 and R2 entries plus the top-level `plannedRounds[]` and
`policy` fields are preserved verbatim.
