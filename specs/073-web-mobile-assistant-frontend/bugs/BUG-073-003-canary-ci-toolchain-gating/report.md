# Report: BUG-073-003 — Cross-language canary CI toolchain gating

## Summary

The spec-073 cross-language renderer canary fails the `CI` `lint-and-test` job because the
Go unit runner is a Go-only container with no node/dart. Fix: treat toolchain ABSENCE as a
`t.Skip` (environment gap) while keeping every PRESENT-but-broken path fail-loud, and add a
dedicated `cross-language-canary` CI job (node + Flutter) so the canary still runs in CI.
A new contract test mechanically prevents the canary from silently dropping out of CI.

Design decision: **Option A** (dedicated CI job + skip-when-absent). Rationale in design.md.

## Completion Statement

Fix implemented and fully validated locally: the canary SKIPS in the Go-only unit container
(greening CI `lint-and-test`), drift detection stays fail-loud when the toolchain is present,
the skip decision is covered by non-tautological adversarial tests, and a contract test
mechanically keeps the canary running in the dedicated `cross-language-canary` CI job. Full
Go unit lane, lint, and format are green. Terminal certification (`done`) is gated on the
post-push CI run confirming `lint-and-test` GREEN.

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

_(filled after push)_
