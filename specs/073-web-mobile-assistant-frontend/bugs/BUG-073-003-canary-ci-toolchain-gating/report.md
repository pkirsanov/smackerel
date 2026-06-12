# Report: BUG-073-003 — Cross-language canary CI toolchain gating

## Summary

The spec-073 cross-language renderer canary fails the `CI` `lint-and-test` job because the
Go unit runner is a Go-only container with no node/dart. Fix: treat toolchain ABSENCE as a
`t.Skip` (environment gap) while keeping every PRESENT-but-broken path fail-loud, and add a
dedicated `cross-language-canary` CI job (node + Flutter) so the canary still runs in CI.
A new contract test mechanically prevents the canary from silently dropping out of CI.

Design decision: **Option A** (dedicated CI job + skip-when-absent). Rationale in design.md.

## Completion Statement

Scoped fix delivered and verified on origin/main (`0bdfa6a9`): the spec-073 cross-language
renderer canary now SKIPS in the Go-only `lint-and-test` container (job GREEN) and RUNS with
real toolchains in the dedicated `cross-language-canary` CI job (job GREEN); drift detection
stays fail-loud when the toolchain is present; the skip decision and CI wiring are guarded by
non-tautological adversarial + contract tests. The CI `build` job and the `build` (build.yml)
workflow are GREEN. The scoped goal — `lint-and-test` GREEN + canary running in CI — is met.

Status is kept `in_progress` (NOT promoted to a guard-certified `done`): this is a
CI/test-infrastructure fix with no product runtime-behavior surface, so the bugfix-fastlane
done-gate's product-grade requirements (E2E-regression coverage, stress coverage,
scenario-manifest) do not apply; forcing them would require fabricated artifacts. The fix
itself is complete and irreversibly delivered on origin/main.

Out of scope (routed to operator, see ## Discovered Issues): unblocking the pipeline past the
long-failing canary surfaced a pre-existing backlog of `integration` and `E2E UI` failures
that this changeset does NOT touch and did NOT cause.

---

## Test Evidence

### Evidence 1 — Bug reproduced (RED): canary FAILS in the go-only container

Command (same container the CI `lint-and-test` unit lane uses):

```
~/smackerel$ ./smackerel.sh test unit --go \
    --go-run 'TestRenderDescriptorV1_CrossLanguageCanary|TestRenderDescriptorV1_DartPreCompiled_NoFallbackToDartRun' --verbose
[go-unit] starting go test ./...
+ go test -v -run 'TestRenderDescriptorV1_CrossLanguageCanary|TestRenderDescriptorV1_DartPreCompiled_NoFallbackToDartRun' -count=1 ./...
=== RUN   TestRenderDescriptorV1_CrossLanguageCanary
    render_descriptor_canary_test.go:125: node not on PATH; the spec 073 cross-language renderer canary requires both node and dart: exec: "node": executable file not found in $PATH
--- FAIL: TestRenderDescriptorV1_CrossLanguageCanary (0.00s)
=== RUN   TestRenderDescriptorV1_DartPreCompiled_NoFallbackToDartRun
    render_descriptor_canary_test.go:367: dart not on PATH; the spec 073 cross-language renderer canary requires dart: exec: "dart": executable file not found in $PATH
--- FAIL: TestRenderDescriptorV1_DartPreCompiled_NoFallbackToDartRun (0.00s)
FAIL    github.com/smackerel/smackerel/tests/unit/clients       0.006s
FAIL
[reproduction] non-zero exit (expected — this is the bug)
```

Container probe confirming the root cause (Go-only image):

```
$ docker run --rm golang:1.25.10-bookworm bash -c 'command -v node || echo "node ABSENT in golang image"; command -v dart || echo "dart ABSENT in golang image"'
node ABSENT in golang image
dart ABSENT in golang image
```

This matches CI run 27392353821 → step "Fail job if any unit tests failed" → `Go test
outcome: failure`.

### Evidence 2 — Drift-detection baseline (toolchains present, no current drift)

Host has node + Flutter-bundled dart + go. The canary RUNS and PASSES (proves no current
drift, and validates the exact command the dedicated CI job uses):

```
~/smackerel$ node --version; dart --version; go version
v22.22.0
Dart SDK version: 3.10.7 (stable) ... on "linux_x64"
go version go1.25.10 linux/amd64
$ go test -count=1 -v -run 'TestRenderDescriptorV1_CrossLanguageCanary|TestRenderDescriptorV1_DartPreCompiled_NoFallbackToDartRun' ./tests/unit/clients/...
--- PASS: TestRenderDescriptorV1_CrossLanguageCanary (0.45s)
    --- PASS: TestRenderDescriptorV1_CrossLanguageCanary/capture_acknowledgement (0.08s)
    --- PASS: TestRenderDescriptorV1_CrossLanguageCanary/confirm_accept_decline (0.06s)
    --- PASS: TestRenderDescriptorV1_CrossLanguageCanary/disambiguation (0.05s)
    --- PASS: TestRenderDescriptorV1_CrossLanguageCanary/error_retry (0.06s)
    --- PASS: TestRenderDescriptorV1_CrossLanguageCanary/text_only (0.06s)
    --- PASS: TestRenderDescriptorV1_CrossLanguageCanary/unknown_shape (0.05s)
    --- PASS: TestRenderDescriptorV1_CrossLanguageCanary/with_sources (0.09s)
--- PASS: TestRenderDescriptorV1_DartPreCompiled_NoFallbackToDartRun (0.00s)
PASS
ok      github.com/smackerel/smackerel/tests/unit/clients       3.241s
```

### Evidence 3 — Post-fix: canary SKIPS in the go-only container (GREEN)

```
~/smackerel$ ./smackerel.sh test unit --go \
    --go-run 'TestRenderDescriptorV1_CrossLanguageCanary|TestRenderDescriptorV1_DartPreCompiled_NoFallbackToDartRun|TestDecideRenderToolchain|TestCrossLanguageCanaryCIJob' --verbose
+ go test -v -run '...' -count=1 ./...
    render_descriptor_canary_test.go:165: node and dart not on PATH; the spec 073 cross-language renderer canary execs both renderers and requires both. Skipping (environment gap, not a code defect — BUG-073-003); the canary executes in CI in the dedicated `cross-language-canary` job (.github/workflows/ci.yml) and on developer machines with node + Flutter/dart installed.
--- SKIP: TestRenderDescriptorV1_CrossLanguageCanary (0.00s)
    render_descriptor_canary_test.go:411: node and dart not on PATH; ... Skipping (environment gap, not a code defect — BUG-073-003); ...
--- SKIP: TestRenderDescriptorV1_DartPreCompiled_NoFallbackToDartRun (0.00s)
ok      github.com/smackerel/smackerel/tests/unit/clients       0.012s
EXIT_CODE=0
```

Toolchain absence now degrades gracefully (SKIP, exit 0) instead of failing the lane.

### Evidence 4 — Skip-decision adversarial tests (non-tautological)

Same container run as Evidence 3 (no node/dart present); the decision is tested via an
injected stub `lookPath`, so it passes regardless of ambient PATH and would FAIL if the
decision regressed to always-run or always-skip:

```
=== RUN   TestDecideRenderToolchain_BothPresent_Available
--- PASS: TestDecideRenderToolchain_BothPresent_Available (0.00s)
=== RUN   TestDecideRenderToolchain_NodeAbsent_SkipsAndNamesNode
--- PASS: TestDecideRenderToolchain_NodeAbsent_SkipsAndNamesNode (0.00s)
=== RUN   TestDecideRenderToolchain_DartAbsent_SkipsAndNamesDart
--- PASS: TestDecideRenderToolchain_DartAbsent_SkipsAndNamesDart (0.00s)
=== RUN   TestDecideRenderToolchain_BothAbsent_Skips
--- PASS: TestDecideRenderToolchain_BothAbsent_Skips (0.00s)
ok      github.com/smackerel/smackerel/tests/unit/clients       0.012s
```

### Evidence 5 — Drift detection preserved (corrupted golden still fails loud)

With toolchains PRESENT, `text_only.descriptor.json` was deliberately corrupted (still
schema-valid). The canary failed loud (`t.Fatalf`), proving the present-but-drift path is
untouched. The fixture was then restored via `git checkout` (no permanent change):

```
$ go test -count=1 -v -run 'TestRenderDescriptorV1_CrossLanguageCanary/text_only' ./tests/unit/clients/...
=== RUN   TestRenderDescriptorV1_CrossLanguageCanary/text_only
    render_descriptor_canary_test.go:257: js renderer output deviates from golden
        --- js ---
        {"schema_version":"render-descriptor.v1","nodes":[{"kind":"text","text":"The weather in Palm Springs tomorrow is sunny, 78F."}]}
        --- golden ---
        { ... "text": "DRIFT-INJECTED-DELIBERATELY-BUG-073-003-restore-via-git-checkout" ... }
--- FAIL: TestRenderDescriptorV1_CrossLanguageCanary/text_only (0.19s)
FAIL    github.com/smackerel/smackerel/tests/unit/clients       4.648s
DRIFT_RUN_EXIT=1
$ git checkout -- tests/fixtures/assistant_response_v1/text_only.descriptor.json
$ git status --porcelain tests/fixtures/assistant_response_v1/text_only.descriptor.json
(no output — fixture restored clean)
```

Non-tautological pair: Evidence 3 = skip-when-absent; Evidence 5 = fail-when-present-but-drifted.

### Evidence 6 — CI-wiring contract test (canary can't silently drop out of CI)

Runs in the go-only container (parses ci.yml only); the live job is correctly wired and the
adversarial mutation sub-tests prove the validator catches a dropped job / missing Flutter /
missing canary run:

```
=== RUN   TestCrossLanguageCanaryCIJob_LiveFile
--- PASS: TestCrossLanguageCanaryCIJob_LiveFile (0.00s)
=== RUN   TestCrossLanguageCanaryCIJob_AdversarialMissingJob
--- PASS: TestCrossLanguageCanaryCIJob_AdversarialMissingJob (0.00s)
=== RUN   TestCrossLanguageCanaryCIJob_AdversarialMissingFlutter
--- PASS: TestCrossLanguageCanaryCIJob_AdversarialMissingFlutter (0.00s)
=== RUN   TestCrossLanguageCanaryCIJob_AdversarialMissingCanaryRun
--- PASS: TestCrossLanguageCanaryCIJob_AdversarialMissingCanaryRun (0.00s)
ok      github.com/smackerel/smackerel/internal/deploy  0.038s
```

### Evidence 7 — Full Go unit lane green + lint + format

```
~/smackerel$ ./smackerel.sh test unit --go
[go-unit] starting go test ./...
ok      github.com/smackerel/smackerel/tests/unit/clients       0.010s
ok      github.com/smackerel/smackerel/internal/deploy          (cached)
... (no FAIL lines across the whole module) ...
ok      github.com/smackerel/smackerel/web/pwa/tests            (cached)
[go-unit] go test ./... finished OK
FULL_GO_UNIT_EXIT=0

~/smackerel$ ./smackerel.sh format --check
65 files already formatted
FORMAT_EXIT=0

~/smackerel$ ./smackerel.sh lint
All checks passed!
Web validation passed
LINT_EXIT=0
```

The full Go unit lane (exactly what CI `lint-and-test` runs) is green with the canary
skipping — proving the CI failure is resolved.

### Evidence 8 — Post-push CI result

Pushed SHA `0bdfa6a9` (fast-forward `784a11b1..0bdfa6a9 -> main`). Per-workflow / per-job
result for the SHA:

```
$ gh run view 27394284069 --json jobs   # workflow: CI
  JOB lint-and-test:          completed/success   <-- SCOPED GOAL: GREEN (canary skips here)
  JOB cross-language-canary:  completed/success   <-- canary RUNS with node+Flutter in CI: GREEN
  JOB build:                  completed/success   <-- GREEN after `gh run rerun --failed` (initial
                                                      attempt hit transient HF connect flake:
                                                      "couldn't connect to https://huggingface.co"
                                                      while preloading all-MiniLM-L6-v2 in smackerel-ml)
  JOB integration:            completed/failure    <-- PRE-EXISTING, out of scope (see below)

$ gh run view 27394284025 --json jobs   # workflow: build (build.yml, BODM signed images)
  build-images / build-chrome-bridge / build-bundles (dev|home-lab|test) / publish-build-manifest
  ALL completed/success                          <-- GREEN

  Gitleaks workflow: completed/success           <-- GREEN
  E2E UI workflow (27394284129): completed/failure <-- PRE-EXISTING spec-083, out of scope (see below)
```

#### Pre-existing failures surfaced by unblocking the pipeline (NOT caused by this change)

Before this fix, `lint-and-test` failed at the canary on every main commit, so `build` and
`integration` were skipped and `integration`/`E2E UI` failures never ran. Greening
`lint-and-test` let the pipeline progress and surfaced a long-hidden backlog — the same
“surfacing” mechanism by which spec-021 surfaced this canary bug.

- `integration` job failures (my commit touches ZERO integration / CLI-auth files):
  ```
  cli_auth_passthrough_test.go:104: expected exit code 2 for `auth` with no subcommand, got 1
  --- FAIL: TestCLIAuthPassthrough_NoArgsExitsTwo
  --- FAIL: TestCLIAuthPassthrough_UnknownSubcommandExitsTwo
  --- FAIL: TestLocationNormalizeIntegration_OpenMeteoCanonicalLocations/...
  FAIL github.com/smackerel/smackerel/tests/integration            (cli_auth_passthrough)
  FAIL github.com/smackerel/smackerel/tests/integration/api
  FAIL github.com/smackerel/smackerel/tests/integration/assistant  (location normalize)
  FAIL github.com/smackerel/smackerel/tests/integration/mobile
  FAIL github.com/smackerel/smackerel/tests/integration/openknowledge
  (spec-083 TestCardRewardsExtractLiveStackAudited_E08 / TestCardRewardsMigration_AppliesCleanly PASSED)
  ```
- `E2E UI` workflow: pre-existing spec-083 card-rewards Playwright failures
  (`cardrewards_wallet.spec.ts`, `cardrewards_rotating_verify.spec.ts`: `/v1/web/login ... got 429`).
  E2E UI conclusion=failure on the last 6 main SHAs incl. `784a11b1` (parent), `20b2dafa`
  (the SHA in the report), and `a8d2abb2` — i.e. red on main BEFORE this push.

These are pre-existing, span multiple unrelated domains, and (for E2E UI) live in the
forbidden spec-083 WIP surface. They are routed to the operator as separate work; they are
NOT part of this bug's scope (lint-and-test GREEN + canary in CI), which is fully met.

## Discovered Issues

The cross-language canary skip-on-absence behavior (the word "skipping" above) is the
intended fix, not a deferral. The two items below are pre-existing failures surfaced (not
caused) by greening `lint-and-test`; both are dispositioned as routed-to-operator with a
concrete reference, per the Discovered-Issue Disposition contract.

| Date | Issue | Disposition | Reference |
|------|-------|-------------|-----------|
| 2026-06-12 | CI `integration` job pre-existing failures surfaced when greening `lint-and-test` unblocked the pipeline: `tests/integration` (`cli_auth_passthrough_test.go` `TestCLIAuthPassthrough_NoArgsExitsTwo` / `_UnknownSubcommandExitsTwo`, expected exit 2 got 1), `tests/integration/assistant` (`TestLocationNormalizeIntegration_OpenMeteoCanonicalLocations`), `tests/integration/{api,mobile,openknowledge}`. | ROUTED to operator. Pre-existing; not caused by this change (the changeset touches zero integration / CLI-auth files); out of scope for BUG-073-003 (scope = lint-and-test GREEN + canary running in CI). A separate bug should be filed. | specs/073-web-mobile-assistant-frontend/bugs/BUG-073-003-canary-ci-toolchain-gating/report.md (Evidence 8) |
| 2026-06-12 | CI `E2E UI` workflow pre-existing spec-083 card-rewards Playwright failures (`web/pwa/tests/cardrewards_wallet.spec.ts`, `web/pwa/tests/cardrewards_rotating_verify.spec.ts`: `/v1/web/login` returned `429`). E2E UI conclusion was `failure` on the last 6 `main` SHAs including parent `784a11b1` and report-SHA `20b2dafa` — red BEFORE this push. | ROUTED to operator. Pre-existing spec-083 WIP, explicitly excluded from this change; not caused by this fix. | specs/083-card-rewards-companion (operator WIP); report.md (Evidence 8) |
