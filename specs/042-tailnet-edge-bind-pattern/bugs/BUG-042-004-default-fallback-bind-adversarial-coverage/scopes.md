# Scopes: BUG-042-004 — Compose contract test missing adversarial coverage for default-fallback bind on smackerel-core/ml

## Scope 1: Add adversarial coverage for default-fallback and ml-side literal bind regressions

**Status:** Done

**Files:**
- [internal/deploy/compose_contract_test.go](../../../../internal/deploy/compose_contract_test.go) (new `TestComposeContract_AdversarialDefaultFallbackBind` function with three table-driven sub-cases; no change to `assertComposeContract`, no change to constants)

### Use Cases

```gherkin
Feature: Compose contract test mechanically rejects default-fallback and literal bind forms for smackerel-core and smackerel-ml
  Scenario: SCN-042-004-A — assertComposeContract rejects smackerel-core with ${HOST_BIND_ADDRESS:-127.0.0.1}: default-fallback bind
    Given a compose fixture where smackerel-core.ports[0] = "${HOST_BIND_ADDRESS:-127.0.0.1}:${CORE_HOST_PORT}:${CORE_CONTAINER_PORT}"
    When assertComposeContract is called with the fixture
    Then it returns a non-nil error mentioning "smackerel-core" AND at least one of [spec 020, ${HOST_BIND_ADDRESS:-127.0.0.1}, Gate-G028, fail-loud]
    And the test docstring carries HL-RESCAN-009 attribution

  Scenario: SCN-042-004-B — assertComposeContract rejects smackerel-ml with ${HOST_BIND_ADDRESS:-127.0.0.1}: default-fallback bind
    Given a compose fixture where smackerel-ml.ports[0] = "${HOST_BIND_ADDRESS:-127.0.0.1}:${ML_HOST_PORT}:${ML_CONTAINER_PORT}"
    When assertComposeContract is called with the fixture
    Then it returns a non-nil error mentioning "smackerel-ml" AND at least one of [spec 020, ${HOST_BIND_ADDRESS:-127.0.0.1}, Gate-G028, fail-loud]

  Scenario: SCN-042-004-C — assertComposeContract rejects smackerel-ml with literal 127.0.0.1: bind (spec 020 form)
    Given a compose fixture where smackerel-ml.ports[0] = "127.0.0.1:${ML_HOST_PORT}:${ML_CONTAINER_PORT}"
    When assertComposeContract is called with the fixture
    Then it returns a non-nil error mentioning "smackerel-ml" AND at least one of [spec 020, ${HOST_BIND_ADDRESS:-127.0.0.1}, Gate-G028, fail-loud]
    And the smackerel-core branch is unaffected (positive cross-check)
```

### Implementation Plan

1. **`internal/deploy/compose_contract_test.go` new test function:** Append `TestComposeContract_AdversarialDefaultFallbackBind` to the end of the file (after `TestComposeContract_AdversarialOllamaLiteralBind`). The function uses a table-driven sweep with three sub-cases: smackerel-core default-fallback, smackerel-ml default-fallback, smackerel-ml literal-127.0.0.1.
2. **Per-sub-case assertion shape:** Each sub-case builds a compose YAML fixture string (mirrors the existing `TestComposeContract_AdversarialLiteralBind` fixture shape), calls `assertComposeContract([]byte(tc.fixture))`, asserts non-nil error, asserts error mentions the violating service name, asserts error mentions at least one of the anchor terms `[spec 020, ${HOST_BIND_ADDRESS:-127.0.0.1}, Gate-G028, fail-loud]`.
3. **HL-RESCAN-009 attribution:** the test function docstring opens with `// TestComposeContract_AdversarialDefaultFallbackBind proves the contract function catches a regression to the forbidden default-fallback ...` and explicitly names `HL-RESCAN-009` in the rationale section. Each sub-case `t.Fatalf` failure-case message also carries `HL-RESCAN-009` attribution so a future regression points back to this bug.
4. **RED→GREEN proof (scenario-first TDD):** Temporarily relax `strings.HasPrefix(p, requiredCorePrefix)` in the smackerel-core branch of `assertComposeContract` to `strings.Contains(p, "${HOST_BIND_ADDRESS:")` (a too-loose substring check that would accept the default-fallback form). Run the new test. Observe the smackerel-core sub-case FAIL with the expected `the contract is tautological` message; observe the two smackerel-ml sub-cases still PASS (because the ml branch is untouched). Restore the strict check via `replace_string_in_file` and re-run → all three sub-cases PASS GREEN.
5. **Confine the change boundary:** the only file modified is `internal/deploy/compose_contract_test.go` plus the bug-packet artifacts in `specs/042-tailnet-edge-bind-pattern/bugs/BUG-042-004-default-fallback-bind-adversarial-coverage/`. No production runtime Go/Python code, no `deploy/compose.deploy.yml` change, no `docker-compose.yml` change, no `config/smackerel.yaml` change, no other `specs/**`, no CI workflow.

### Test Plan

- **RED→GREEN proof (scenario-first TDD):** Temporarily soften the smackerel-core prefix check in `assertComposeContract` from strict `strings.HasPrefix(p, requiredCorePrefix)` to over-permissive `strings.Contains(p, "${HOST_BIND_ADDRESS:")`. Run `go test -count=1 -v -run TestComposeContract_AdversarialDefaultFallbackBind ./internal/deploy/...`. Observe `smackerel-core_uses_${HOST_BIND_ADDRESS:-127.0.0.1}:_default-fallback_bind_(forbidden_by_Gate_G028)` FAIL with the expected `the contract is tautological — it would NOT catch a regression to ${HOST_BIND_ADDRESS:-127.0.0.1}: default-fallback or literal 127.0.0.1: form; HL-RESCAN-009 default-fallback / literal-bind ml-side coverage gap is reintroduced` message. Observe the two smackerel-ml sub-cases still PASS (positive cross-check that the ml branch was not affected by the relaxation). Restore the strict check via `replace_string_in_file` → all three sub-cases PASS GREEN.
- **Targeted contract suite:** `go test -count=1 -v -run '^TestComposeContract' ./internal/deploy/...` runs all 8 top-level `TestComposeContract_*` functions plus all sub-tests — every one PASS in <1s wall-clock.
- **Adversarial isolation:** the new sub-cases each fail RED only when the smackerel-core OR smackerel-ml prefix check is relaxed. The other adversarial tests in the suite (literal-bind, infra-has-ports, multi-ports core/ml, network_mode for all 5 services, ollama literal/default-fallback) still PASS unchanged. This proves the new locks are independently enforced (non-tautological).
- **Cross-package smoke:** `./smackerel.sh test unit --go` covers the full unit suite (`internal/deploy/...`, `internal/config/...`, `internal/auth/...`, `internal/api/...`, `cmd/core/...`, etc.) — all PASS, no regression. The Smackerel runtime test runner outputs `[go-unit] go test ./... finished OK` on success.
- **Static checks:** `go vet ./internal/deploy/...` exit 0; `gofmt -l internal/deploy/` empty.
- **Live spec 042 + spec 049 contracts preserved:** `TestComposeContract_LiveFile` (the canary that locks the live `deploy/compose.deploy.yml` is contract-compliant) still PASSES — the new test is purely additive and the live file already complies, so the canary is undisturbed.

#### Test Plan Coverage Matrix

| Scenario / Behavior | Test Type | File | Test ID | Adversarial? | Regression E2E |
|---|---|---|---|---|---|
| SCN-042-004-A: smackerel-core default-fallback rejected | unit (Go static-file lint) | internal/deploy/compose_contract_test.go | TestComposeContract_AdversarialDefaultFallbackBind/smackerel-core_uses_${HOST_BIND_ADDRESS:-127.0.0.1}:_default-fallback_bind_(forbidden_by_Gate_G028) | YES — fails RED if smackerel-core prefix check is relaxed | Persistent in-tree adversarial Go test that runs on every `./smackerel.sh test unit --go` invocation. |
| SCN-042-004-B: smackerel-ml default-fallback rejected | unit (Go static-file lint) | internal/deploy/compose_contract_test.go | TestComposeContract_AdversarialDefaultFallbackBind/smackerel-ml_uses_${HOST_BIND_ADDRESS:-127.0.0.1}:_default-fallback_bind_(forbidden_by_Gate_G028) | YES — fails RED if smackerel-ml prefix check is relaxed | Same as above. |
| SCN-042-004-C: smackerel-ml literal 127.0.0.1 rejected | unit (Go static-file lint) | internal/deploy/compose_contract_test.go | TestComposeContract_AdversarialDefaultFallbackBind/smackerel-ml_uses_literal_127.0.0.1:_bind_(spec_020_form) | YES — fails RED if smackerel-ml prefix check is relaxed | Same as above. |
| Canary: TestComposeContract_LiveFile preserved | unit (Go static-file lint) | internal/deploy/compose_contract_test.go | TestComposeContract_LiveFile | NO (positive canary) | Pre-existing spec 042 contract; preserved unchanged. |
| Canary: BUG-042-001 + BUG-042-002 + BUG-042-003 contracts preserved | unit (Go static-file lint) | internal/deploy/compose_contract_test.go | TestComposeContract_AdversarialLiteralBind, AdversarialInfraHasPorts, AdversarialMultiPortsBypass, AdversarialMLMultiPortsBypass, AdversarialNetworkModeHostBypass (5 sub-cases), AdversarialOllamaLiteralBind (2 sub-cases) | YES (canaries) | Pre-existing spec 042 + bug regression coverage; preserved unchanged. |
| Canary: spec 047 vuln-gate contract preserved | unit (Go static-file lint) | internal/deploy/build_workflow_vuln_gate_contract_test.go | TestVulnGateContract_LiveFile | NO (canary) | Pre-existing spec 047 contract; preserved unchanged. |
| Canary: BUG-047-001 bundle-hash contract preserved | unit (Go static-file lint) | internal/deploy/build_workflow_bundle_hash_contract_test.go | TestBundleHashContract_LiveFile + 4 adversarial sub-tests | YES (canaries) | Pre-existing BUG-047-001 contract; preserved unchanged. |

### Definition of Done

- [x] `internal/deploy/compose_contract_test.go` declares a new persistent in-tree test function `TestComposeContract_AdversarialDefaultFallbackBind`. [SCN-042-004-A, B, C]
   → Evidence: `grep -n 'TestComposeContract_AdversarialDefaultFallbackBind' internal/deploy/compose_contract_test.go` returns the function declaration. See report.md > Code Diff Evidence.
- [x] The new function uses a table-driven sweep with exactly three sub-cases: smackerel-core default-fallback, smackerel-ml default-fallback, smackerel-ml literal-127.0.0.1. [SCN-042-004-A, B, C]
   → Evidence: same grep returns the three `name:` entries. See report.md > Code Diff Evidence.
- [x] Each sub-case builds a compose YAML fixture string and calls `assertComposeContract` with it. [SCN-042-004-A, B, C]
   → Evidence: `grep -n 'fixture: \|assertComposeContract(\[\]byte(tc.fixture))' internal/deploy/compose_contract_test.go` shows the fixture-build + call pattern. See report.md > Code Diff Evidence.
- [x] Each sub-case asserts `assertComposeContract` returns a non-nil error. [SCN-042-004-A, B, C]
   → Evidence: same grep returns the `if err == nil { t.Fatalf(...) }` block. See report.md > Code Diff Evidence.
- [x] Each sub-case asserts the rejection error mentions the violating service name (`smackerel-core` or `smackerel-ml`). [SCN-042-004-A, B, C]
   → Evidence: `grep -n 'strings.Contains(err.Error(), tc.service)' internal/deploy/compose_contract_test.go` returns the assertion. See report.md > Code Diff Evidence.
- [x] Each sub-case asserts the rejection error mentions at least one of the anchor terms `[spec 020, ${HOST_BIND_ADDRESS:-127.0.0.1}, Gate-G028, fail-loud]` via a `strings.Contains` loop. [SCN-042-004-A, B, C]
   → Evidence: `grep -n 'haveAnyAnchor\|spec 020\|Gate-G028\|fail-loud' internal/deploy/compose_contract_test.go` returns the anchor-list assertion. See report.md > Code Diff Evidence.
- [x] HL-RESCAN-009 attribution is present in either the test docstring or the failure-case `t.Fatalf` message. [SCN-042-004-A, B, C]
   → Evidence: `grep -n 'HL-RESCAN-009' internal/deploy/compose_contract_test.go` returns at least 2 hits (docstring + fail message). See report.md > Code Diff Evidence.
- [x] RED proof captured (scenario-first TDD): temporarily softening `strings.HasPrefix(p, requiredCorePrefix)` to `strings.Contains(p, "${HOST_BIND_ADDRESS:")` in the smackerel-core branch causes the smackerel-core sub-case to FAIL while the two smackerel-ml sub-cases still PASS (positive cross-check that the ml branch was untouched by the core relaxation). [SCN-042-004-A]
   → Evidence: see report.md > Test Evidence > Red→Green proof (scenario-first TDD).
- [x] GREEN proof captured: restoring the strict `strings.HasPrefix` check returns the suite to all-PASS. [SCN-042-004-A, B, C]
   → Evidence: see report.md > Test Evidence > Red→Green proof (scenario-first TDD) — restore step.
- [x] `TestComposeContract_LiveFile` continues to PASS GREEN against the unchanged `deploy/compose.deploy.yml`. [Canary]
   → Evidence: `go test -count=1 -v -run '^TestComposeContract_LiveFile$' ./internal/deploy/...` shows `--- PASS: TestComposeContract_LiveFile`. See report.md > Validation Evidence.
- [x] Targeted contract suite: `go test -count=1 -v -run '^TestComposeContract' ./internal/deploy/...` PASS — all 8 top-level tests, all sub-tests, exit code 0. [SCN-042-004-A, B, C + Canaries]
   → Evidence: see report.md > Validation Evidence > Targeted contract suite.
- [x] Cross-package smoke clean: full `./smackerel.sh test unit --go` PASS with `[go-unit] go test ./... finished OK`. [Broader regression]
   → Evidence: see report.md > Validation Evidence > Cross-package smoke.
- [x] Static checks: `go vet ./internal/deploy/...` exit 0; `gofmt -l internal/deploy/` empty. [Broader regression]
   → Evidence: see report.md > Validation Evidence > Static checks.
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior — persistent in-tree `TestComposeContract_AdversarialDefaultFallbackBind` with 3 sub-tests runs on every `./smackerel.sh test unit --go` invocation (CI + developer pre-push). The compose contract surface is a static-file invariant; the regression suite IS the Go test suite itself. [SCN-042-004-A, B, C]
   → Evidence: see report.md > Audit Evidence > Regression Evidence.
- [x] Broader E2E regression suite passes — full `internal/deploy/...` Go test suite plus cross-package smoke against every other internal/* package PASS, including the spec 042 BUG-042-001 + BUG-042-002 + BUG-042-003 regression suites and the BUG-047-001 bundle-hash contract. [Broader regression]
   → Evidence: `./smackerel.sh test unit --go` returns `[go-unit] go test ./... finished OK`. See report.md > Audit Evidence > Cross-package smoke.
- [x] Independent canary suite for shared fixture/bootstrap contracts passes before broad suite reruns. [Spec 042 BUG-042-001 + BUG-042-002 + BUG-042-003 canary + spec 047 vuln-gate canary + BUG-047-001 bundle-hash canary]
   → Evidence: `TestComposeContract_AdversarialLiteralBind`, `TestComposeContract_AdversarialInfraHasPorts`, `TestComposeContract_AdversarialMultiPortsBypass`, `TestComposeContract_AdversarialMLMultiPortsBypass`, all 5 sub-cases of `TestComposeContract_AdversarialNetworkModeHostBypass`, both sub-cases of `TestComposeContract_AdversarialOllamaLiteralBind`, `TestVulnGateContract_LiveFile`, `TestBundleHashContract_LiveFile` + 4 sub-tests — all PASS unchanged. The new test is purely additive (no change to `assertComposeContract`, no change to constants), so the canaries cannot regress as a side effect. See report.md > Audit Evidence > Canary suite.
- [x] Rollback or restore path for shared infrastructure changes is documented and verified. [Shared Infrastructure Impact Sweep]
   → Evidence: rollback is a single `git revert` of the BUG-042-004 commit. The new test is purely additive and no production runtime / live compose / constant declaration changes; revert is risk-free — no live-file or runtime-behavior mismatch could result. Restore is the same `git revert`. Verified by the RED proof step which temporarily relaxes the assertion (substring softening), confirms expected FAIL output, then restores. See report.md > Code Diff Evidence + Test Evidence > Red→Green proof (scenario-first TDD).
- [x] Consumer impact sweep complete and zero stale first-party references remain. [Consumer Impact Sweep]
   → Evidence: see Consumer Impact Sweep section above; `grep -rn 'TestComposeContract_AdversarialDefaultFallbackBind' --include='*.go' .` returns matches only inside `internal/deploy/compose_contract_test.go` (the declaration site); zero external references; no public API change; no rename anywhere; no breadcrumb / navigation / redirect / API client / generated client / deep link surface affected.
- [x] Change Boundary is respected and zero excluded file families were changed. [Change Boundary]
   → Evidence: see Change Boundary section above; `git diff --stat` reports exactly one file modified (`internal/deploy/compose_contract_test.go`, +112 / -0 lines) plus the seven BUG-042-004 packet artifacts. Every file family in the Excluded surfaces list is bit-identical to HEAD.
- [x] Stress coverage assessment (Gate G026): explicit stress/load coverage is NOT REQUIRED for this fix. The change is a single new persistent in-tree Go unit test that runs in <1s wall-clock; there is no latency, throughput, p95/p99, response-time, sla, or slo dimension that the change can move. The compose contract surface is a static-file invariant evaluated once per Go test invocation; no daemon, no concurrency, no sustained load. This DoD line documents the assessment for the Gate G026 lint. [Broader regression]
   → Evidence: the new test runs in <1s (see Validation Evidence > Targeted contract suite). No stress dimension applies.

### Shared Infrastructure Impact Sweep

`internal/deploy/compose_contract_test.go` is a **shared bootstrap helper** in the static-file-invariant sense: it is the in-tree contract that proves `deploy/compose.deploy.yml` complies with spec 042 + spec 049 + Gate G028. Changes to its assertion surface affect every `./smackerel.sh test unit --go` run and every pre-merge / pre-push validation. The BUG-042-004 fix has the following blast radius:

- **Direct downstream consumers:** every CI job + every developer pre-push run executes `./smackerel.sh test unit --go`, which includes `TestComposeContract_*`. The new `TestComposeContract_AdversarialDefaultFallbackBind` runs on every invocation. The existing `assertComposeContract` already rejects the forbidden forms, so no CI run will RED-fail as a direct consequence of the new test.
- **Operator-side fan-out:** none. The contract test is in-tree only — no operator workflow consumes it. The deploy-adapter overlay's `apply.sh` + `verify.sh` operate against the compose file directly; they do not reach into the contract test.
- **Adapter-side fan-out:** none. Same reason as above.
- **Test infrastructure (canary surface):** the eight pre-existing `TestComposeContract_*` adversarial sub-tests (literal-bind, infra-has-ports, multi-ports core/ml, five network_mode sub-cases, two ollama literal/default-fallback sub-cases), plus `TestVulnGateContract_LiveFile`, plus `TestBundleHashContract_*` — all PASS unchanged.
- **Generated-artifact contract:** none — no generated artifact changes.
- **Bootstrap contract for downstream specs:** spec 042 (tailnet-edge bind), spec 049 (Prometheus on tailnet edge) — both benefit from the strengthened defense-in-depth at the test layer. The pattern is explicit: "every operator-facing service in the compose file has both a positive canary AND adversarial sub-tests covering the literal and default-fallback regression vectors."
- **Rollback or restore plan:** see the corresponding DoD item — single `git revert`; no live-file or runtime-behavior mismatch possible because the new test is purely additive at the test layer.
- **Ordering / timing / storage / session / context / role / blast radius:** no impact. The fix runs as a static-file inspection on every Go test invocation; no daemon state, no shared cache, no cross-process ordering concern.

### Consumer Impact Sweep

This bug fix does **not** rename or remove any externally-visible interface, route, endpoint, contract, API, URL, slug, public symbol, deep link, breadcrumb, navigation entry, or generated client. The change is bounded to a single new package-private test function in `internal/deploy/compose_contract_test.go`:

- **No public API change.** `assertComposeContract`'s signature is preserved verbatim — the new test is a NEW caller of the existing function, not an alteration of the function itself. No HTTP route, no NATS subject, no CLI flag, no env-var name, no config-key, no URL path, no breadcrumb, no redirect surface, no generated client regeneration.
- **New private test added, nothing renamed.** `TestComposeContract_AdversarialDefaultFallbackBind` is a NEW test function (capitalized first letter so `go test` discovers it, but otherwise scoped to the `package deploy` test scope). It has zero external consumers because Go's test functions are not importable. No identifier rename anywhere.
- **Affected consumer surfaces enumerated:** the only consumer of `TestComposeContract_AdversarialDefaultFallbackBind` is the `go test` runner itself (driven by `./smackerel.sh test unit --go`). It is invoked automatically on every test run; no CI workflow change, no manual invocation step. There are no API client, generated client, breadcrumb, navigation, redirect, or stale-reference surfaces to sweep. No external documentation, no operator runbook references the new test by name.
- **Cross-package consumer surface:** zero. Test functions are not importable across packages. The new test lives entirely inside `package deploy` (test scope) and is only discoverable by the Go test runner.
- **Stale-reference scan:** zero stale first-party references remain. No previous version of `TestComposeContract_AdversarialDefaultFallbackBind` existed, so there is nothing to leave stale. The pre-existing `TestComposeContract_AdversarialLiteralBind` (which only covers smackerel-core literal bind) is preserved unchanged; the new test extends coverage to default-fallback for both services and literal for ml-side, NOT replacing the older test. `grep -rn 'TestComposeContract_AdversarialDefaultFallbackBind' --include='*.go' .` returns matches only inside `internal/deploy/compose_contract_test.go` (the declaration site); zero external references.

### Change Boundary

**Allowed file families (this fix may modify):**

- `internal/deploy/compose_contract_test.go` — the static-file contract test being extended (the only code change point)
- `specs/042-tailnet-edge-bind-pattern/bugs/BUG-042-004-default-fallback-bind-adversarial-coverage/**` — this bug packet's seven artifacts

**Excluded surfaces (this fix MUST NOT touch):**

- `internal/deploy/compose_contract_test.go` `assertComposeContract` function body — the assertion is correct as-is; the fix is at the test surface only
- `internal/deploy/compose_contract_test.go` `requiredCorePrefix` / `requiredMLPrefix` / `requiredPrometheusPrefix` / `requiredOllamaPrefix` constants — already correct
- `deploy/compose.deploy.yml` — the live compose file already complies; no change needed
- `docker-compose.yml` — dev compose file is governed by HL-RESCAN-012 (P3) and a separate set of gates
- `specs/042-tailnet-edge-bind-pattern/spec.md`, `specs/042-tailnet-edge-bind-pattern/design.md`, `specs/042-tailnet-edge-bind-pattern/scopes.md`, `specs/042-tailnet-edge-bind-pattern/state.json`, `specs/042-tailnet-edge-bind-pattern/uservalidation.md`, `specs/042-tailnet-edge-bind-pattern/report.md` — foreign-owned parent-spec content; outside `bubbles.devops` mode edit scope
- Production runtime Go code under `internal/auth/...`, `internal/config/...`, `internal/api/...`, `cmd/...` — the bug is in the contract-test surface, not in the runtime
- Python ML sidecar under `ml/...` — unrelated
- `config/smackerel.yaml` — the SST source has nothing to do with compose contract testing
- `Dockerfile`, `ml/Dockerfile` — unrelated
- `.github/workflows/*` — unrelated; the contract test is invoked by the existing `unit-tests` job
- `scripts/...` — unrelated
- Any other `specs/**` directory — single-bug-scope discipline

### Regression E2E Coverage

| Scenario | Test ID | File | Type | Adversarial? |
|---|---|---|---|---|
| SCN-042-004-A: smackerel-core default-fallback rejected | TestComposeContract_AdversarialDefaultFallbackBind/smackerel-core_uses_${HOST_BIND_ADDRESS:-127.0.0.1}:_default-fallback_bind_(forbidden_by_Gate_G028) | internal/deploy/compose_contract_test.go | unit (Go static-file lint) | YES — fails RED if smackerel-core prefix check is relaxed |
| SCN-042-004-B: smackerel-ml default-fallback rejected | TestComposeContract_AdversarialDefaultFallbackBind/smackerel-ml_uses_${HOST_BIND_ADDRESS:-127.0.0.1}:_default-fallback_bind_(forbidden_by_Gate_G028) | same as above | unit (Go static-file lint) | YES — fails RED if smackerel-ml prefix check is relaxed |
| SCN-042-004-C: smackerel-ml literal 127.0.0.1 rejected | TestComposeContract_AdversarialDefaultFallbackBind/smackerel-ml_uses_literal_127.0.0.1:_bind_(spec_020_form) | same as above | unit (Go static-file lint) | YES — fails RED if smackerel-ml prefix check is relaxed |
| Canary: TestComposeContract_LiveFile preserved | TestComposeContract_LiveFile | same as above | unit (Go static-file lint) | NO (positive canary) |
| Canary: BUG-042-001 multi-ports preserved | TestComposeContract_AdversarialMultiPortsBypass + TestComposeContract_AdversarialMLMultiPortsBypass | same as above | unit (Go static-file lint) | YES (canaries) |
| Canary: BUG-042-002 network_mode preserved | TestComposeContract_AdversarialNetworkModeHostBypass (5 sub-cases) | same as above | unit (Go static-file lint) | YES (canaries) |
| Canary: BUG-042-003 ollama enforcement preserved | TestComposeContract_AdversarialOllamaLiteralBind (2 sub-cases) | same as above | unit (Go static-file lint) | YES (canaries) |
| Canary: spec 047 vuln-gate contract preserved | TestVulnGateContract_LiveFile | internal/deploy/build_workflow_vuln_gate_contract_test.go | unit (Go static-file lint) | NO (positive canary) |
| Canary: BUG-047-001 bundle-hash contract preserved | TestBundleHashContract_LiveFile + 4 sub-tests | internal/deploy/build_workflow_bundle_hash_contract_test.go | unit (Go static-file lint) | YES (canaries) |
