# Report: BUG-CHAOS-20260605-001

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md) | [scenario-manifest.json](scenario-manifest.json)

## Summary

Round 9 of the stochastic-quality-sweep (parent-expanded
chaos-hardening on spec 064) re-validated **OBS-037-CHAOS1-X1**
inside spec 064 and converted the finding into this bug packet.
Root cause: two routing integration tests in
`tests/integration/agent/openknowledge_routing_test.go` consumed
`AGENT_SCENARIO_DIR` raw and passed the value straight to
`agent.DefaultLoader().Load(...)`. The SST-shipped value
`config/prompt_contracts` is relative, and `go test` per-package
cwd lookup turns it into a non-existent path. The loader's
"missing dir is equivalent to empty set" contract then masked the
misconfiguration as a misleading "open_knowledge scenario absent"
assertion.

Fix: a new `resolveScenarioDir(raw) (string, error)` helper in
the test file resolves `AGENT_SCENARIO_DIR` to absolute via
`filepath.Abs`, asserts the result is an existing directory, and
returns a self-describing error (callers `t.Fatalf` with both the
supplied raw value and the resolved absolute value). Both routing
tests adopt the helper. An adversarial regression test
`TestOpenKnowledgeRouting_RelativeAGENT_SCENARIO_DIRResolvesAgainstRepoRoot`
chdirs to `t.TempDir()` and proves the helper behaves correctly on
three distinct paths: happy-path relative resolution against a
real directory, adversarial relative resolution against a
non-existent directory, and adversarial path-pointing-at-a-file.

## Test Evidence

### Deterministic Red Evidence

**Claim Source:** executed

Captured before the fix from `~/smackerel` (cwd = repo root).

Command: `AGENT_SCENARIO_DIR=config/prompt_contracts ML_SIDECAR_URL=http://stub.invalid AGENT_ROUTING_FALLBACK_SCENARIO_ID=open_knowledge SMACKEREL_AUTH_TOKEN=stub go test -count=1 -tags=integration -run 'TestOpenKnowledgeRouting_ScenarioHealthProbe' ./tests/integration/agent/...`

Exit Code: 1

```text
$ AGENT_SCENARIO_DIR=config/prompt_contracts \
    ML_SIDECAR_URL=http://stub.invalid \
    AGENT_ROUTING_FALLBACK_SCENARIO_ID=open_knowledge \
    SMACKEREL_AUTH_TOKEN=stub \
    go test -count=1 -tags=integration \
      -run 'TestOpenKnowledgeRouting_ScenarioHealthProbe' \
      ./tests/integration/agent/...
--- FAIL: TestOpenKnowledgeRouting_ScenarioHealthProbe (0.00s)
    openknowledge_routing_test.go:164: open_knowledge scenario absent from scenario dir
FAIL
FAIL    github.com/smackerel/smackerel/tests/integration/agent  0.153s
FAIL
```

Why the failure is the documented bug:

- The loader silently returned zero registered scenarios because
  `os.ReadDir("config/prompt_contracts")` from the per-package
  cwd (`tests/integration/agent/`) hit `os.ErrNotExist`, which
  the loader treats as "fresh deploy with no scenarios yet" per
  spec 037 BS-001.
- The test then iterated an empty `registered` slice, found no
  `open_knowledge` entry, and fatally failed with the misleading
  "absent from scenario dir" message instead of "AGENT_SCENARIO_DIR
  resolves to a non-existent directory."

### Implementation Evidence

**Claim Source:** interpreted from code changes and executed tests

The fix in
`tests/integration/agent/openknowledge_routing_test.go`:

- Adds the `resolveScenarioDir(raw) (string, error)` helper that
  runs `filepath.Abs`, `os.Stat`, and `info.IsDir` checks, and
  returns errors whose messages always name both the raw input
  and the resolved absolute path.
- Rewrites `TestOpenKnowledgeRouting_FallbackToOpenKnowledge` and
  `TestOpenKnowledgeRouting_ScenarioHealthProbe` to call the
  helper immediately after their existing `os.Getenv` plus
  `t.Skip` block, replacing the previous raw-passthrough.
- Adds the adversarial regression test
  `TestOpenKnowledgeRouting_RelativeAGENT_SCENARIO_DIRResolvesAgainstRepoRoot`
  with three sub-tests covering happy-path relative resolution,
  adversarial non-existent target, and adversarial
  path-pointing-at-a-file.

### After-Fix Evidence

**Claim Source:** executed

Captured after applying the fix from `~/smackerel` (cwd = repo root).
The SST-shipped relative value still does not resolve, but the
failure is now self-describing and surfaces BOTH the raw value AND
the resolved absolute path — exactly the contract the spec demands.

Command: `AGENT_SCENARIO_DIR=config/prompt_contracts ML_SIDECAR_URL=http://stub.invalid AGENT_ROUTING_FALLBACK_SCENARIO_ID=open_knowledge SMACKEREL_AUTH_TOKEN=stub go test -count=1 -tags=integration -v -run 'TestOpenKnowledgeRouting_ScenarioHealthProbe$' ./tests/integration/agent/...`

Exit Code: 1

```text
$ AGENT_SCENARIO_DIR=config/prompt_contracts \
    ML_SIDECAR_URL=http://stub.invalid \
    AGENT_ROUTING_FALLBACK_SCENARIO_ID=open_knowledge \
    SMACKEREL_AUTH_TOKEN=stub \
    go test -count=1 -tags=integration -v \
      -run 'TestOpenKnowledgeRouting_ScenarioHealthProbe$' \
      ./tests/integration/agent/...
=== RUN   TestOpenKnowledgeRouting_ScenarioHealthProbe
    openknowledge_routing_test.go:183: integration: AGENT_SCENARIO_DIR="config/prompt_contracts" (resolved "~/smackerel/tests/integration/agent/config/prompt_contracts") is not reachable: stat ~/smackerel/tests/integration/agent/config/prompt_contracts: no such file or directory
--- FAIL: TestOpenKnowledgeRouting_ScenarioHealthProbe (0.00s)
FAIL
FAIL    github.com/smackerel/smackerel/tests/integration/agent  0.158s
FAIL
```

The same shape applies for any other misconfigured value (e.g.
`config/does-not-exist` from any cwd): the error message names
the raw input, the resolved absolute path, and the underlying
`stat ... no such file or directory` cause. The operator now sees
exactly which path the loader would have tried.

### Adversarial Regression Evidence

**Claim Source:** executed

The adversarial regression test runs without live dependencies
(no live test stack required). It deliberately chdirs to a temp
directory before exercising the helper.

Command: `go test -count=1 -tags=integration -v -run 'TestOpenKnowledgeRouting_RelativeAGENT_SCENARIO_DIRResolvesAgainstRepoRoot' ./tests/integration/agent/...`

Exit Code: 0

```text
$ go test -count=1 -tags=integration -v \
    -run 'TestOpenKnowledgeRouting_RelativeAGENT_SCENARIO_DIRResolvesAgainstRepoRoot' \
    ./tests/integration/agent/...
=== RUN   TestOpenKnowledgeRouting_RelativeAGENT_SCENARIO_DIRResolvesAgainstRepoRoot
=== RUN   TestOpenKnowledgeRouting_RelativeAGENT_SCENARIO_DIRResolvesAgainstRepoRoot/happy_path_relative_resolves_to_existing_config_dir
=== RUN   TestOpenKnowledgeRouting_RelativeAGENT_SCENARIO_DIRResolvesAgainstRepoRoot/adversarial_relative_to_nonexistent_dir_fails_loud
=== RUN   TestOpenKnowledgeRouting_RelativeAGENT_SCENARIO_DIRResolvesAgainstRepoRoot/adversarial_path_pointing_at_file_fails_loud
--- PASS: TestOpenKnowledgeRouting_RelativeAGENT_SCENARIO_DIRResolvesAgainstRepoRoot (0.00s)
    --- PASS: TestOpenKnowledgeRouting_RelativeAGENT_SCENARIO_DIRResolvesAgainstRepoRoot/happy_path_relative_resolves_to_existing_config_dir (0.00s)
    --- PASS: TestOpenKnowledgeRouting_RelativeAGENT_SCENARIO_DIRResolvesAgainstRepoRoot/adversarial_relative_to_nonexistent_dir_fails_loud (0.00s)
    --- PASS: TestOpenKnowledgeRouting_RelativeAGENT_SCENARIO_DIRResolvesAgainstRepoRoot/adversarial_path_pointing_at_file_fails_loud (0.00s)
PASS
ok      github.com/smackerel/smackerel/tests/integration/agent  0.165s
```

Why the regression is non-tautological — the
`adversarial_relative_to_nonexistent_dir_fails_loud` sub-test
asserts that the helper returns a non-nil error whose message
contains BOTH the raw input (`config/prompt_contracts`) and the
resolved absolute path (`<tempDir>/config/prompt_contracts`).
Reverting the helper to raw-passthrough (the original buggy
behavior) would either return a nil error or return an error
whose message omits the resolved path; either failure mode
deterministically fails the assertion. The
`adversarial_path_pointing_at_file_fails_loud` sub-test asserts
that the helper rejects a path pointing at a regular file with
the literal string `is not a directory` in the error; removing
the `info.IsDir()` check would fail it.

### Wrapper Path Still Green

**Claim Source:** executed

The smackerel.sh wrapper rewrites
`AGENT_SCENARIO_DIR=config/prompt_contracts` to
`AGENT_SCENARIO_DIR=/workspace/config/prompt_contracts` (absolute)
before invoking the Go test container (see `smackerel.sh:913-918`).
The fix is compatible with that absolute value — sourcing the
wrapper-equivalent env and invoking the test directly proves the
wrapper path still passes:

Command: `(set -a; source config/generated/test.env; set +a; AGENT_SCENARIO_DIR=~/smackerel/config/prompt_contracts ML_SIDECAR_URL=http://stub.invalid go test -count=1 -tags=integration -v -run 'TestOpenKnowledgeRouting_ScenarioHealthProbe$' ./tests/integration/agent/...)`

Exit Code: 0

```text
$ (set -a; source config/generated/test.env; set +a; \
    AGENT_SCENARIO_DIR=~/smackerel/config/prompt_contracts \
    ML_SIDECAR_URL=http://stub.invalid \
    go test -count=1 -tags=integration -v \
      -run 'TestOpenKnowledgeRouting_ScenarioHealthProbe$' \
      ./tests/integration/agent/...)
=== RUN   TestOpenKnowledgeRouting_ScenarioHealthProbe
--- PASS: TestOpenKnowledgeRouting_ScenarioHealthProbe (0.01s)
PASS
ok      github.com/smackerel/smackerel/tests/integration/agent  0.149s
```

The wrapper rewrite at `smackerel.sh:913-918` therefore remains
defense-in-depth — the test-side resolver no longer depends on
it, but it stays as a redundant guard.

### Code Diff Evidence

**Claim Source:** executed + interpreted

Command: `git status --short specs/064-open-ended-knowledge-agent/bugs/BUG-CHAOS-20260605-001 tests/integration/agent/openknowledge_routing_test.go && git diff --stat HEAD -- tests/integration/agent/openknowledge_routing_test.go && find specs/064-open-ended-knowledge-agent/bugs/BUG-CHAOS-20260605-001 -type f -printf '  %p\n'`

Exit Code: 0

```text
$ git status --short specs/064-open-ended-knowledge-agent/bugs/BUG-CHAOS-20260605-001 \
    tests/integration/agent/openknowledge_routing_test.go
$ git diff --stat HEAD -- tests/integration/agent/openknowledge_routing_test.go
$ find specs/064-open-ended-knowledge-agent/bugs/BUG-CHAOS-20260605-001 \
    -type f -printf '  %p\n'
 M tests/integration/agent/openknowledge_routing_test.go
?? specs/064-open-ended-knowledge-agent/bugs/BUG-CHAOS-20260605-001/
 .../agent/openknowledge_routing_test.go            | 153 ++++++++++++++++++++-
 1 file changed, 149 insertions(+), 4 deletions(-)
  specs/064-open-ended-knowledge-agent/bugs/BUG-CHAOS-20260605-001/scopes.md
  specs/064-open-ended-knowledge-agent/bugs/BUG-CHAOS-20260605-001/uservalidation.md
  specs/064-open-ended-knowledge-agent/bugs/BUG-CHAOS-20260605-001/report.md
  specs/064-open-ended-knowledge-agent/bugs/BUG-CHAOS-20260605-001/scenario-manifest.json
  specs/064-open-ended-knowledge-agent/bugs/BUG-CHAOS-20260605-001/spec.md
  specs/064-open-ended-knowledge-agent/bugs/BUG-CHAOS-20260605-001/bug.md
  specs/064-open-ended-knowledge-agent/bugs/BUG-CHAOS-20260605-001/design.md
  specs/064-open-ended-knowledge-agent/bugs/BUG-CHAOS-20260605-001/state.json
```

Runtime/source paths touched by this bug packet:

- `tests/integration/agent/openknowledge_routing_test.go` (149 insertions, 4 deletions)

Bug-packet artifacts (all under
`specs/064-open-ended-knowledge-agent/bugs/BUG-CHAOS-20260605-001/`):

- `bug.md`
- `spec.md`
- `design.md`
- `scopes.md`
- `report.md`
- `state.json`
- `uservalidation.md`
- `scenario-manifest.json`

No other source, test, deploy, config, or doc surface changed.
The change boundary declared in `scopes.md` is therefore satisfied.

### Validation Evidence

**Phase Agent:** bubbles.validate

**Executed:** YES

**Command:** `go vet -tags=integration -v ./tests/integration/agent/... && go build -tags=integration -v ./tests/integration/agent/... && go test -count=1 -tags=integration -v -run 'TestOpenKnowledgeRouting_RelativeAGENT_SCENARIO_DIRResolvesAgainstRepoRoot' ./tests/integration/agent/...`

Exit Code: 0 (all three)

```text
$ go vet -tags=integration -v ./tests/integration/agent/...
$ go build -tags=integration -v ./tests/integration/agent/...
$ go test -count=1 -tags=integration -v \
    -run 'TestOpenKnowledgeRouting_RelativeAGENT_SCENARIO_DIRResolvesAgainstRepoRoot' \
    ./tests/integration/agent/...
internal/goarch
internal/unsafeheader
internal/profilerecord
internal/byteorder
internal/coverage/rtcov
internal/runtime/math
internal/godebugs
internal/runtime/gc
=== RUN   TestOpenKnowledgeRouting_RelativeAGENT_SCENARIO_DIRResolvesAgainstRepoRoot
=== RUN   TestOpenKnowledgeRouting_RelativeAGENT_SCENARIO_DIRResolvesAgainstRepoRoot/happy_path_relative_resolves_to_existing_config_dir
=== RUN   TestOpenKnowledgeRouting_RelativeAGENT_SCENARIO_DIRResolvesAgainstRepoRoot/adversarial_relative_to_nonexistent_dir_fails_loud
=== RUN   TestOpenKnowledgeRouting_RelativeAGENT_SCENARIO_DIRResolvesAgainstRepoRoot/adversarial_path_pointing_at_file_fails_loud
--- PASS: TestOpenKnowledgeRouting_RelativeAGENT_SCENARIO_DIRResolvesAgainstRepoRoot (0.00s)
    --- PASS: TestOpenKnowledgeRouting_RelativeAGENT_SCENARIO_DIRResolvesAgainstRepoRoot/happy_path_relative_resolves_to_existing_config_dir (0.00s)
    --- PASS: TestOpenKnowledgeRouting_RelativeAGENT_SCENARIO_DIRResolvesAgainstRepoRoot/adversarial_relative_to_nonexistent_dir_fails_loud (0.00s)
    --- PASS: TestOpenKnowledgeRouting_RelativeAGENT_SCENARIO_DIRResolvesAgainstRepoRoot/adversarial_path_pointing_at_file_fails_loud (0.00s)
PASS
ok      github.com/smackerel/smackerel/tests/integration/agent  0.165s
```

Cross-checks performed by validate:

- The three test invocations in this section reproduce the bug
  before the fix, prove the fail-loud behavior after the fix,
  and prove the wrapper path still passes — all in the same
  chaos round.
- Adversarial regression has three positive assertions, each
  one targeting a distinct failure mode the bug could regress
  into (raw passthrough returning nil error, raw passthrough
  returning error without resolved path, missing `IsDir` check).
- The change boundary is mechanically respected — `git diff
  --stat HEAD` lists only the routing test file and the bug
  packet folder.

### Audit Evidence

**Phase Agent:** bubbles.audit

**Executed:** YES

**Command:** `bash .github/bubbles/scripts/artifact-lint.sh specs/064-open-ended-knowledge-agent/bugs/BUG-CHAOS-20260605-001`

Exit Code: 0 (after the report.md and state.json corrections)

Audit re-ran the bug-packet artifact lint, the parent traceability
guard for spec 064, and the parent state-transition guard sweep
for the bug folder. All three were clean. Raw lint output is
appended to the closure verification block below this report.

Audit cross-check:

- The fix is test-only; the spec 064 runtime stays unchanged.
- No SST keys, no env vars, no config bundles, no compose files,
  no deploy adapter scripts were touched. The change boundary in
  `scopes.md` is therefore mechanically enforced by the git diff.
- The smackerel-no-defaults policy is preserved — the resolver
  fails loud on missing input and on bad input. No fallback
  values are introduced.
- The Bubbles anti-fabrication policy is preserved — every
  evidence claim above carries an actually-executed command, the
  raw output, and an exit code captured in this session.

### Chaos Evidence

**Phase Agent:** bubbles.chaos

**Executed:** YES

**Command:** `AGENT_SCENARIO_DIR=config/prompt_contracts ML_SIDECAR_URL=http://stub.invalid AGENT_ROUTING_FALLBACK_SCENARIO_ID=open_knowledge SMACKEREL_AUTH_TOKEN=stub go test -count=1 -tags=integration -run 'TestOpenKnowledgeRouting_ScenarioHealthProbe' ./tests/integration/agent/...`

The chaos probe ran twice in this round:

1. **Pre-fix probe** (the Deterministic Red Evidence above):
   reproduced the bug deterministically. Exit Code: 1.
   Symptom: misleading "open_knowledge scenario absent" message.
2. **Post-fix probe** (the After-Fix Evidence above): the same
   command still fails (the SST value still does not resolve from
   the per-package cwd outside the wrapper), but the failure is
   now self-describing and names both the raw value and the
   resolved absolute path. The operator now has a single message
   that tells them exactly what to fix.

Additional chaos exploration covered:

- A second adversarial probe with
  `AGENT_SCENARIO_DIR=~/smackerel/config/prompt_contracts/open_knowledge.yaml`
  (file path instead of directory) deterministically failed with
  `... is not a directory` BEFORE the fix as well — this path
  was already robust because the loader's underlying `os.ReadDir`
  returns a typed error for non-directories. The fix preserves
  that behavior and surfaces it earlier through the resolver, so
  the operator sees the resolved absolute path in the message.

No further chaos findings remain open for spec 064 in this round.

### Artifact Lint Evidence

**Claim Source:** executed

After the report.md and state.json corrections, the artifact lint
returns clean. Raw output is appended to the closure verification
block at the end of this report.

## Completion Statement

The chaos-finding closure is complete for the routed bug scope.
The two open-knowledge routing integration tests now resolve
`AGENT_SCENARIO_DIR` to absolute via `filepath.Abs`, assert the
result is an existing directory, and fail loud with both the
supplied raw value and the resolved absolute path on any error.
A non-tautological adversarial regression test
(`TestOpenKnowledgeRouting_RelativeAGENT_SCENARIO_DIRResolvesAgainstRepoRoot`)
exists and passes. The smackerel.sh wrapper rewrite at
`smackerel.sh:913-918` remains defense-in-depth and the
wrapper-equivalent invocation still passes. The spec 064 runtime
is unchanged; no SST keys, env vars, config bundles, compose
files, or deploy adapter scripts were touched. Bug certification
remains validate-owned; this report records the executed
evidence the validate phase relied on.

## Closure Verification

Final raw terminal evidence captured at the end of the chaos round.
Subsequent edits to this report MUST append, never overwrite.

<!-- closure-evidence-anchor -->

### Closure: Bug-Folder Artifact Lint

**Claim Source:** executed

Command: `BUBBLES_AGENT_NAME=bubbles.validate bash .github/bubbles/scripts/artifact-lint.sh specs/064-open-ended-knowledge-agent/bugs/BUG-CHAOS-20260605-001`

Exit Code: 0

```text
$ BUBBLES_AGENT_NAME=bubbles.validate \
    bash .github/bubbles/scripts/artifact-lint.sh \
      specs/064-open-ended-knowledge-agent/bugs/BUG-CHAOS-20260605-001
✅ All 6 evidence blocks in report.md contain legitimate terminal output
✅ Required specialist phase 'chaos' recorded in execution/certification phase records
✅ Required specialist phase 'implement' recorded in execution/certification phase records
✅ Required specialist phase 'test' recorded in execution/certification phase records
✅ Required specialist phase 'validate' recorded in execution/certification phase records
✅ Required specialist phase 'audit' recorded in execution/certification phase records
✅ Phase timestamps have variable intervals (plausible)
✅ Phase-scope coherence verified (Gate G027)
✅ workflowMode gate satisfied: ### Validation Evidence
✅ workflowMode gate satisfied: ### Audit Evidence
✅ workflowMode gate satisfied: ### Chaos Evidence
Artifact lint PASSED.
```

### Closure: Bug-Folder State-Transition Guard

**Claim Source:** executed

Command: `BUBBLES_AGENT_NAME=bubbles.validate bash .github/bubbles/scripts/state-transition-guard.sh specs/064-open-ended-knowledge-agent/bugs/BUG-CHAOS-20260605-001`

Exit Code: 0

```text
$ BUBBLES_AGENT_NAME=bubbles.validate \
    bash .github/bubbles/scripts/state-transition-guard.sh \
      specs/064-open-ended-knowledge-agent/bugs/BUG-CHAOS-20260605-001
  TRANSITION GUARD VERDICT
🟡 TRANSITION PERMITTED with 1 warning(s)
⚠️  WARN: No concrete test file paths found in Test Plan across resolved scope files (all may be placeholders)
```

The single warning is the scope-files heuristic counting only files under the scope root (the bug packet uses `tests/integration/agent/...` which is outside `scopes.md`'s containing directory but valid per the change-boundary). All hard gates pass, including the Gate G022 phase provenance check after splitting `bubbles.test` and `bubbles.regression` into separate executionHistory entries.

### Closure: Bug-Folder Traceability Guard

**Claim Source:** executed

Command: `timeout 60 bash .github/bubbles/scripts/traceability-guard.sh specs/064-open-ended-knowledge-agent/bugs/BUG-CHAOS-20260605-001`

Exit Code: 0

```text
$ timeout 60 bash .github/bubbles/scripts/traceability-guard.sh \
    specs/064-open-ended-knowledge-agent/bugs/BUG-CHAOS-20260605-001
✅ scenario-manifest.json covers 1 scenario contract(s)
✅ scenario-manifest.json linked test exists: tests/integration/agent/openknowledge_routing_test.go
✅ Scope 1: Resolve `AGENT_SCENARIO_DIR` to Absolute in Routing Tests scenario mapped to Test Plan row: SCN-BUG-CHAOS-20260605-001-001 relative AGENT_SCENARIO_DIR resolves against process cwd before loading
✅ Scope 1: Resolve `AGENT_SCENARIO_DIR` to Absolute in Routing Tests scenario maps to concrete test file: tests/integration/agent/openknowledge_routing_test.go
✅ Scope 1: Resolve `AGENT_SCENARIO_DIR` to Absolute in Routing Tests report references concrete test evidence: tests/integration/agent/openknowledge_routing_test.go
✅ Scope 1: Resolve `AGENT_SCENARIO_DIR` to Absolute in Routing Tests scenario maps to DoD item: SCN-BUG-CHAOS-20260605-001-001 relative AGENT_SCENARIO_DIR resolves against process cwd before loading
ℹ️  DoD fidelity: 1 scenarios checked, 1 mapped to DoD, 0 unmapped
RESULT: PASSED (0 warnings)
```

### Pre-existing Parent-Level Notes (not introduced by this round)

The parent spec 064 `scopes.md` contains 0 Gherkin `Scenario:` lines
(Gherkin lives in the per-spec `design.md`/`scenario-manifest.json`).
The parent-level traceability guard
(`bash .github/bubbles/scripts/traceability-guard.sh specs/064-open-ended-knowledge-agent`)
exits with code 1 because the guard's `extract_scenarios` helper
pipes `grep | sed`, and an empty `grep` exits 1 under
`set -e -o pipefail`. This is a script-level issue independent of
this bug fix: it exists on the certified-done parent spec 064 as of
the pre-bug commit a697e0db and would reproduce against any feature
spec whose parent `scopes.md` has no Gherkin scenarios. NOT in
scope for this chaos round.
