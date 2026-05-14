# Report: BUG-042-005 — Compose contract test missing adversarial coverage for literal-bind / default-fallback bind on prometheus

## Summary

Closes home-lab readiness re-scan finding **HL-RESCAN-010** (P3): the spec 042 / spec 049 / Gate G028 compose contract test in `internal/deploy/compose_contract_test.go` previously had no adversarial sub-case proving that `assertComposeContract` rejects the **literal `127.0.0.1:` spec 020 form on `prometheus`** OR the **default-fallback `${HOST_BIND_ADDRESS:-127.0.0.1}:` form on `prometheus`** (Gate G028 NO-DEFAULTS violation). The runtime behavior of `assertComposeContract` was correct — it rejects both forms today via the strict `strings.HasPrefix(p, requiredPrometheusPrefix)` check — but no synthetic adversarial fixture proved the rejection lands on the prometheus contract surface, leaving a coverage gap that would let a future maintainer accidentally relax the strict prefix check (e.g., to a too-loose `strings.Contains` substring match) without any test failing RED.

The fix is **purely additive at the test layer**: a single new persistent in-tree test function `TestComposeContract_AdversarialPrometheusLiteralBindAndFallbackForms` with two table-driven sub-cases. The function pins two guarantees:

1. `assertComposeContract` rejects `prometheus.ports[0] = "127.0.0.1:${PROMETHEUS_HOST_PORT}:9090"` (literal `127.0.0.1:` bind on prometheus).
2. `assertComposeContract` rejects `prometheus.ports[0] = "${HOST_BIND_ADDRESS:-127.0.0.1}:${PROMETHEUS_HOST_PORT}:9090"` (default-fallback bind on prometheus).

Each sub-case asserts the rejection error mentions `prometheus` AND at least one of the regression-target anchor terms `[spec 049, spec 042, fail-loud, ${HOST_BIND_ADDRESS:?, ${HOST_BIND_ADDRESS:-127.0.0.1}, literal 127.0.0.1:]`. The test docstring + each sub-case's failure-case `t.Fatalf` message both carry HL-RESCAN-010 attribution so a future regression points back to this bug.

**Workflow Mode:** test-to-doc — the fix is the test addition itself plus one bug-packet that documents it. No production runtime code changes; no `assertComposeContract` change; no `requiredPrometheusPrefix` constant change; no `deploy/compose.deploy.yml` change.

**Parent re-scan:** home-lab readiness re-scan 2026-05-14, finding HL-RESCAN-010. Sequential per-finding bug-packet workflow; mechanically identical pattern to HL-RESCAN-009 (BUG-042-004) but targeting the `prometheus` service.

## Completion Statement

All seven bug-packet artifacts (`spec.md`, `design.md`, `scopes.md`, `scenario-manifest.json`, `report.md`, `uservalidation.md`, `state.json`) authored. Single code change in `internal/deploy/compose_contract_test.go` (+97 lines, 0 lines deleted). Targeted contract suite GREEN. Cross-package smoke (`./smackerel.sh test unit --go`) GREEN. RED proof captured by temporarily relaxing the prometheus branch of `assertComposeContract` from strict `strings.HasPrefix` to a too-loose `strings.Contains(p, "${HOST_BIND_ADDRESS:")` substring check — the prometheus default-fallback sub-case correctly FAILS with the expected `the contract is tautological` message while the prometheus literal-bind sub-case continues to PASS unchanged (positive cross-check that the relaxation specifically smuggles only the default-fallback form, since the literal `127.0.0.1:` does not contain the `${HOST_BIND_ADDRESS:` substring). All nine pre-existing `TestComposeContract_*` adversarial tests + `TestComposeContract_LiveFile` canary + `TestVulnGateContract_LiveFile` canary + `TestBundleHashContract_*` canary suite all PASS unchanged.

**Status: SHIP_IT** — see [uservalidation.md](uservalidation.md).

## Implementation Code Diff

### Code Diff Evidence

**Claim Source:** executed

```text
$ git diff --stat HEAD -- internal/deploy/compose_contract_test.go
 internal/deploy/compose_contract_test.go | 97 ++++++++++++++++++++++++++++++++
 1 file changed, 97 insertions(+)
```

```text
$ grep -n 'TestComposeContract_AdversarialPrometheusLiteralBindAndFallbackForms\|HL-RESCAN-010' internal/deploy/compose_contract_test.go | head -20
692:// TestComposeContract_AdversarialPrometheusLiteralBindAndFallbackForms proves
697:// Before HL-RESCAN-010, no synthetic adversarial test existed for
725:// Discovered: home-lab readiness re-scan finding HL-RESCAN-010 (P3),
730:func TestComposeContract_AdversarialPrometheusLiteralBindAndFallbackForms(t *testing.T) {
762:                            t.Fatalf("adversarial contract test failed: prometheus port %q was accepted (the contract is tautological — it would NOT catch a regression to the spec 020 literal form or to the default-fallback ${HOST_BIND_ADDRESS:-127.0.0.1} form for prometheus; HL-RESCAN-010 prometheus literal-bind / default-fallback coverage gap is reintroduced)", tc.port)
782:                            t.Fatalf("adversarial contract test failed: error did not mention any of [spec 049, spec 042, fail-loud, ${HOST_BIND_ADDRESS:?, ${HOST_BIND_ADDRESS:-127.0.0.1}, literal 127.0.0.1:] (the regression target this HL-RESCAN-010 guard locks): %v", err)
```

**Interpretation:** the new function `TestComposeContract_AdversarialPrometheusLiteralBindAndFallbackForms` is declared at line 730 with a docstring opening at line 692 explicitly attributing HL-RESCAN-010 (lines 697 + 725). The failure-case message at line 762 carries `HL-RESCAN-010 prometheus literal-bind / default-fallback coverage gap is reintroduced`. The anchor-list assertion at line 782 mentions `[spec 049, spec 042, fail-loud, ${HOST_BIND_ADDRESS:?, ${HOST_BIND_ADDRESS:-127.0.0.1}, literal 127.0.0.1:]` — the six regression-target anchor terms required by DD-2.

**Excluded surfaces (negative evidence — also in the diff):** zero lines changed in `assertComposeContract` (function body unchanged), zero lines changed in the `requiredCorePrefix` / `requiredMLPrefix` / `requiredPrometheusPrefix` / `requiredOllamaPrefix` constants, zero lines changed in `deploy/compose.deploy.yml`, zero lines changed in `docker-compose.yml`. The change boundary is bounded to one file (one new test function) plus the BUG-042-005 packet artifacts under `specs/042-tailnet-edge-bind-pattern/bugs/BUG-042-005-prometheus-default-fallback-bind-adversarial-coverage/`. Confirmed by `git diff --shortstat HEAD -- internal/deploy/compose_contract_test.go` reporting `1 file changed, 97 insertions(+)`.

## Test Evidence

### Validation Evidence

**Claim Source:** executed

#### Targeted contract suite (full canary + new)

```text
$ go test -count=1 -v -run '^TestComposeContract|^TestVulnGateContract|^TestBundleHashContract' ./internal/deploy/...
=== RUN   TestBundleHashContract_LiveFile
--- PASS: TestBundleHashContract_LiveFile (0.00s)
=== RUN   TestBundleHashContract_AdversarialMissingShaField
--- PASS: TestBundleHashContract_AdversarialMissingShaField (0.00s)
=== RUN   TestBundleHashContract_AdversarialMissingArtifactUpload
--- PASS: TestBundleHashContract_AdversarialMissingArtifactUpload (0.00s)
=== RUN   TestBundleHashContract_AdversarialMissingArtifactDownload
--- PASS: TestBundleHashContract_AdversarialMissingArtifactDownload (0.00s)
=== RUN   TestBundleHashContract_AdversarialMissingEnvExposure
--- PASS: TestBundleHashContract_AdversarialMissingEnvExposure (0.00s)
=== RUN   TestVulnGateContract_LiveFile
--- PASS: TestVulnGateContract_LiveFile (0.00s)
=== RUN   TestVulnGateContract_AdversarialMissingScan
--- PASS: TestVulnGateContract_AdversarialMissingScan (0.00s)
=== RUN   TestVulnGateContract_AdversarialScanAfterSign
--- PASS: TestVulnGateContract_AdversarialScanAfterSign (0.00s)
=== RUN   TestVulnGateContract_AdversarialWeakSeverity
--- PASS: TestVulnGateContract_AdversarialWeakSeverity (0.00s)
=== RUN   TestVulnGateContract_AdversarialNonBlockingExitCode
--- PASS: TestVulnGateContract_AdversarialNonBlockingExitCode (0.00s)
=== RUN   TestVulnGateContract_AdversarialMissingManifestEvidence
--- PASS: TestVulnGateContract_AdversarialMissingManifestEvidence (0.00s)
=== RUN   TestVulnGateContract_AdversarialIgnoreUnfixedFlipped
--- PASS: TestVulnGateContract_AdversarialIgnoreUnfixedFlipped (0.00s)
=== RUN   TestVulnGateContract_AdversarialMissingIgnoreUnfixedField
--- PASS: TestVulnGateContract_AdversarialMissingIgnoreUnfixedField (0.00s)
=== RUN   TestVulnGateContract_AdversarialMissingIgnoreUnfixedManifestKey
--- PASS: TestVulnGateContract_AdversarialMissingIgnoreUnfixedManifestKey (0.00s)
=== RUN   TestVulnGateContract_AdversarialMissingLimitSeveritiesForSarif
--- PASS: TestVulnGateContract_AdversarialMissingLimitSeveritiesForSarif (0.00s)
=== RUN   TestVulnGateContract_AdversarialLimitSeveritiesForSarifFalse
--- PASS: TestVulnGateContract_AdversarialLimitSeveritiesForSarifFalse (0.00s)
=== RUN   TestComposeContract_LiveFile
--- PASS: TestComposeContract_LiveFile (0.00s)
=== RUN   TestComposeContract_AdversarialLiteralBind
--- PASS: TestComposeContract_AdversarialLiteralBind (0.00s)
=== RUN   TestComposeContract_AdversarialInfraHasPorts
--- PASS: TestComposeContract_AdversarialInfraHasPorts (0.00s)
=== RUN   TestComposeContract_AdversarialMultiPortsBypass
--- PASS: TestComposeContract_AdversarialMultiPortsBypass (0.00s)
=== RUN   TestComposeContract_AdversarialMLMultiPortsBypass
--- PASS: TestComposeContract_AdversarialMLMultiPortsBypass (0.00s)
=== RUN   TestComposeContract_AdversarialNetworkModeHostBypass
--- PASS: TestComposeContract_AdversarialNetworkModeHostBypass (0.00s)
=== RUN   TestComposeContract_AdversarialOllamaLiteralBind
--- PASS: TestComposeContract_AdversarialOllamaLiteralBind (0.00s)
=== RUN   TestComposeContract_AdversarialDefaultFallbackBind
--- PASS: TestComposeContract_AdversarialDefaultFallbackBind (0.00s)
=== RUN   TestComposeContract_AdversarialPrometheusLiteralBindAndFallbackForms
--- PASS: TestComposeContract_AdversarialPrometheusLiteralBindAndFallbackForms (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/deploy  0.019s
```

**Interpretation:** all 2 new sub-cases of `TestComposeContract_AdversarialPrometheusLiteralBindAndFallbackForms` PASS. The full canary suite (all 9 `TestComposeContract_*` top-level tests including `LiveFile`, plus `TestVulnGateContract_*` 11 tests, plus `TestBundleHashContract_*` 5 tests) all PASS unchanged. Wall clock: 0.019s. Exit status: 0 / `PASS`.

#### Cross-package smoke

**Claim Source:** executed

```text
$ ./smackerel.sh test unit --go
... (full go test ./... output across cmd/* and internal/* packages — final 25 lines shown) ...
?       github.com/smackerel/smackerel/internal/recommendation  [no test files]
?       github.com/smackerel/smackerel/internal/recommendation/dedupe   [no test files]
?       github.com/smackerel/smackerel/internal/recommendation/graph    [no test files]
ok      github.com/smackerel/smackerel/internal/recommendation/location (cached)
ok      github.com/smackerel/smackerel/internal/recommendation/policy   (cached)
ok      github.com/smackerel/smackerel/internal/recommendation/provider (cached)
ok      github.com/smackerel/smackerel/internal/recommendation/quality  (cached)
ok      github.com/smackerel/smackerel/internal/recommendation/rank     (cached)
?       github.com/smackerel/smackerel/internal/recommendation/reactive [no test files]
ok      github.com/smackerel/smackerel/internal/recommendation/store    (cached)
ok      github.com/smackerel/smackerel/internal/recommendation/tools    (cached)
?       github.com/smackerel/smackerel/internal/recommendation/watch    [no test files]
ok      github.com/smackerel/smackerel/internal/scheduler       (cached)
ok      github.com/smackerel/smackerel/internal/stringutil      (cached)
ok      github.com/smackerel/smackerel/internal/telegram        (cached)
ok      github.com/smackerel/smackerel/internal/topics  (cached)
ok      github.com/smackerel/smackerel/internal/web     (cached)
ok      github.com/smackerel/smackerel/internal/web/icons       (cached)
ok      github.com/smackerel/smackerel/tests/e2e/agent  (cached)
ok      github.com/smackerel/smackerel/tests/integration        (cached) [no tests to run]
?       github.com/smackerel/smackerel/tests/integration/drive/fixtures [no test files]
ok      github.com/smackerel/smackerel/tests/stress/readiness   (cached)
?       github.com/smackerel/smackerel/web/pwa  [no test files]
+ echo '[go-unit] go test ./... finished OK'
[go-unit] go test ./... finished OK
```

**Interpretation:** `[go-unit] go test ./... finished OK` is the Smackerel runtime test runner's success marker for the full Go unit suite. No regression in any other internal/* package; the new test function is purely additive and confined to `internal/deploy/`.

#### Static checks

**Claim Source:** executed

```text
$ go vet ./internal/deploy/...
$ echo "exit=$?"
exit=0

$ gofmt -l internal/deploy/
$ echo "exit=$? lines=<empty>"
exit=0 lines=<empty>
```

**Interpretation:** zero `go vet` warnings, zero `gofmt -l` output (empty list ⇒ no formatting drift). Static surface is clean.

### Red→Green proof (scenario-first TDD)

**Claim Source:** executed

#### RED phase — relax the prometheus branch of `assertComposeContract`

Temporarily replaced the prometheus prefix check inside `assertComposeContract` from strict `strings.HasPrefix(p, requiredPrometheusPrefix)` to a too-loose `strings.Contains(p, "${HOST_BIND_ADDRESS:")` substring match. This is the most realistic accidental relaxation a future maintainer might introduce: it would match BOTH the fail-loud `${HOST_BIND_ADDRESS:?...}` form (correct) AND the forbidden default-fallback `${HOST_BIND_ADDRESS:-127.0.0.1}` form (incorrect — silently accepted), but would still reject the literal `127.0.0.1:` form because it does not contain the `${HOST_BIND_ADDRESS:` substring.

```text
$ go test -count=1 -v -run '^TestComposeContract_AdversarialPrometheusLiteralBindAndFallbackForms$' ./internal/deploy/...
=== RUN   TestComposeContract_AdversarialPrometheusLiteralBindAndFallbackForms
=== RUN   TestComposeContract_AdversarialPrometheusLiteralBindAndFallbackForms/literal_127.0.0.1_bind_(spec_020_form)
    compose_contract_test.go:784: adversarial OK: prometheus port "127.0.0.1:${PROMETHEUS_HOST_PORT}:9090" is rejected with: contract violation: services.prometheus.ports[0]="127.0.0.1:${PROMETHEUS_HOST_PORT}:9090" does not start with required prefix "${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:${PROMETHEUS_HOST_PORT}:" (spec 049 inherits the spec 042 tailnet-edge bind contract; Prometheus host port MUST use the fail-loud ${HOST_BIND_ADDRESS:?...} SST substitution so compose aborts at start time if HOST_BIND_ADDRESS is unset — no literal 127.0.0.1: prefix, no default-fallback ${HOST_BIND_ADDRESS:-127.0.0.1} form)
=== RUN   TestComposeContract_AdversarialPrometheusLiteralBindAndFallbackForms/default-fallback_${HOST_BIND_ADDRESS:-127.0.0.1}_bind_(forbidden_by_Gate_G028)
    compose_contract_test.go:762: adversarial contract test failed: prometheus port "${HOST_BIND_ADDRESS:-127.0.0.1}:${PROMETHEUS_HOST_PORT}:9090" was accepted (the contract is tautological — it would NOT catch a regression to the spec 020 literal form or to the default-fallback ${HOST_BIND_ADDRESS:-127.0.0.1} form for prometheus; HL-RESCAN-010 prometheus literal-bind / default-fallback coverage gap is reintroduced)
--- FAIL: TestComposeContract_AdversarialPrometheusLiteralBindAndFallbackForms (0.00s)
    --- PASS: TestComposeContract_AdversarialPrometheusLiteralBindAndFallbackForms/literal_127.0.0.1_bind_(spec_020_form) (0.00s)
    --- FAIL: TestComposeContract_AdversarialPrometheusLiteralBindAndFallbackForms/default-fallback_${HOST_BIND_ADDRESS:-127.0.0.1}_bind_(forbidden_by_Gate_G028) (0.00s)
FAIL
FAIL    github.com/smackerel/smackerel/internal/deploy  0.008s
FAIL
```

**Interpretation (RED):** the prometheus default-fallback sub-case FAILS with the expected `the contract is tautological — it would NOT catch a regression to the spec 020 literal form or to the default-fallback ${HOST_BIND_ADDRESS:-127.0.0.1} form for prometheus; HL-RESCAN-010 prometheus literal-bind / default-fallback coverage gap is reintroduced` message — the precise breadcrumb a future maintainer needs. The literal-bind sub-case PASSes unchanged because the relaxation only smuggles the default-fallback form (the literal `127.0.0.1:` does not contain the substring `${HOST_BIND_ADDRESS:`) — this is the **positive cross-check** that proves the new test correctly attributes failures to the specific regression vector under test (it doesn't false-FAIL the literal-bind sub-case, and it doesn't false-PASS the default-fallback sub-case).

#### GREEN phase — restore strict `strings.HasPrefix`

Restored the prometheus branch back to `strings.HasPrefix(p, requiredPrometheusPrefix)` via `replace_string_in_file`.

```text
$ go test -count=1 -v -run '^TestComposeContract_AdversarialPrometheusLiteralBindAndFallbackForms$' ./internal/deploy/...
=== RUN   TestComposeContract_AdversarialPrometheusLiteralBindAndFallbackForms
=== RUN   TestComposeContract_AdversarialPrometheusLiteralBindAndFallbackForms/literal_127.0.0.1_bind_(spec_020_form)
    compose_contract_test.go:784: adversarial OK: prometheus port "127.0.0.1:${PROMETHEUS_HOST_PORT}:9090" is rejected with: contract violation: services.prometheus.ports[0]="127.0.0.1:${PROMETHEUS_HOST_PORT}:9090" does not start with required prefix "${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:${PROMETHEUS_HOST_PORT}:" (spec 049 inherits the spec 042 tailnet-edge bind contract; Prometheus host port MUST use the fail-loud ${HOST_BIND_ADDRESS:?...} SST substitution so compose aborts at start time if HOST_BIND_ADDRESS is unset — no literal 127.0.0.1: prefix, no default-fallback ${HOST_BIND_ADDRESS:-127.0.0.1} form)
=== RUN   TestComposeContract_AdversarialPrometheusLiteralBindAndFallbackForms/default-fallback_${HOST_BIND_ADDRESS:-127.0.0.1}_bind_(forbidden_by_Gate_G028)
    compose_contract_test.go:784: adversarial OK: prometheus port "${HOST_BIND_ADDRESS:-127.0.0.1}:${PROMETHEUS_HOST_PORT}:9090" is rejected with: contract violation: services.prometheus.ports[0]="${HOST_BIND_ADDRESS:-127.0.0.1}:${PROMETHEUS_HOST_PORT}:9090" does not start with required prefix "${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:${PROMETHEUS_HOST_PORT}:" (spec 049 inherits the spec 042 tailnet-edge bind contract; Prometheus host port MUST use the fail-loud ${HOST_BIND_ADDRESS:?...} SST substitution so compose aborts at start time if HOST_BIND_ADDRESS is unset — no literal 127.0.0.1: prefix, no default-fallback ${HOST_BIND_ADDRESS:-127.0.0.1} form)
--- PASS: TestComposeContract_AdversarialPrometheusLiteralBindAndFallbackForms (0.00s)
    --- PASS: TestComposeContract_AdversarialPrometheusLiteralBindAndFallbackForms/literal_127.0.0.1_bind_(spec_020_form) (0.00s)
    --- PASS: TestComposeContract_AdversarialPrometheusLiteralBindAndFallbackForms/default-fallback_${HOST_BIND_ADDRESS:-127.0.0.1}_bind_(forbidden_by_Gate_G028) (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/deploy  0.014s
```

**Interpretation (GREEN):** restoring the strict `strings.HasPrefix` check returns both sub-cases to PASS. RED→GREEN proof complete: the new test function would catch a real-world relaxation regression (substring softening on the prometheus branch) AND correctly attributes the failure to the relaxed regression vector (the literal-bind sub-case stayed PASS through the perturbation, providing positive cross-check coverage).

## Audit Evidence

### Audit Evidence

<!-- audit-marker: blocking-gate-reviewed -->

#### Cross-package smoke

**Claim Source:** executed

```text
$ ./smackerel.sh test unit --go
... (full go test ./... output across cmd/* and internal/* packages — final 25 lines shown) ...
?       github.com/smackerel/smackerel/internal/recommendation  [no test files]
?       github.com/smackerel/smackerel/internal/recommendation/dedupe   [no test files]
?       github.com/smackerel/smackerel/internal/recommendation/graph    [no test files]
ok      github.com/smackerel/smackerel/internal/recommendation/location (cached)
ok      github.com/smackerel/smackerel/internal/recommendation/policy   (cached)
ok      github.com/smackerel/smackerel/internal/recommendation/provider (cached)
ok      github.com/smackerel/smackerel/internal/recommendation/quality  (cached)
ok      github.com/smackerel/smackerel/internal/recommendation/rank     (cached)
?       github.com/smackerel/smackerel/internal/recommendation/reactive [no test files]
ok      github.com/smackerel/smackerel/internal/recommendation/store    (cached)
ok      github.com/smackerel/smackerel/internal/recommendation/tools    (cached)
?       github.com/smackerel/smackerel/internal/recommendation/watch    [no test files]
ok      github.com/smackerel/smackerel/internal/scheduler       (cached)
ok      github.com/smackerel/smackerel/internal/stringutil      (cached)
ok      github.com/smackerel/smackerel/internal/telegram        (cached)
ok      github.com/smackerel/smackerel/internal/topics  (cached)
ok      github.com/smackerel/smackerel/internal/web     (cached)
ok      github.com/smackerel/smackerel/internal/web/icons       (cached)
ok      github.com/smackerel/smackerel/tests/e2e/agent  (cached)
ok      github.com/smackerel/smackerel/tests/integration        (cached) [no tests to run]
?       github.com/smackerel/smackerel/tests/integration/drive/fixtures [no test files]
ok      github.com/smackerel/smackerel/tests/stress/readiness   (cached)
?       github.com/smackerel/smackerel/web/pwa  [no test files]
+ echo '[go-unit] go test ./... finished OK'
[go-unit] go test ./... finished OK
```

**Interpretation:** the full Go unit suite (every `internal/*` and `cmd/*` package, plus `tests/e2e/agent`, `tests/integration`, `tests/stress/readiness`) runs clean — every package returns `ok` (or `[no test files]` for empty packages). No test in any other package regresses as a side effect of the BUG-042-005 test addition. The new function is bounded to `internal/deploy/compose_contract_test.go` and its only caller is the `go test` runner itself, so cross-package blast radius is zero by construction.

#### Canary suite

**Claim Source:** executed

The targeted `^TestComposeContract|^TestVulnGateContract|^TestBundleHashContract` regex run (Validation Evidence > Targeted contract suite) shows:

- `TestComposeContract_LiveFile` — PASS (positive canary; `deploy/compose.deploy.yml` complies).
- `TestComposeContract_AdversarialLiteralBind` — PASS (BUG-042 spec 042 canary; smackerel-core literal bind rejected).
- `TestComposeContract_AdversarialInfraHasPorts` — PASS (BUG-042 spec 042 canary; postgres host port re-publish rejected).
- `TestComposeContract_AdversarialMultiPortsBypass` — PASS (BUG-042-001 canary; multi-ports bypass on smackerel-core rejected).
- `TestComposeContract_AdversarialMLMultiPortsBypass` — PASS (BUG-042-001 canary; multi-ports bypass on smackerel-ml rejected).
- `TestComposeContract_AdversarialNetworkModeHostBypass` (5 sub-cases) — all PASS (BUG-042-002 canary; network_mode=host rejected on every operator-facing service including prometheus).
- `TestComposeContract_AdversarialOllamaLiteralBind` (2 sub-cases) — both PASS (BUG-042-003 canary; ollama literal-bind + default-fallback rejected).
- `TestComposeContract_AdversarialDefaultFallbackBind` (3 sub-cases) — all PASS (BUG-042-004 canary; smackerel-core/ml default-fallback + smackerel-ml literal-bind rejected).
- `TestVulnGateContract_LiveFile` + 10 adversarial sub-tests — all PASS (spec 047 vuln-gate contract canary).
- `TestBundleHashContract_LiveFile` + 4 adversarial sub-tests — all PASS (BUG-047-001 bundle-hash contract canary).

**Interpretation:** zero canary regression. The new test is purely additive (no change to `assertComposeContract`, no change to constants, no change to live compose), so the canaries cannot regress as a side effect — and they didn't.

#### Regression Evidence

**Claim Source:** executed

The new `TestComposeContract_AdversarialPrometheusLiteralBindAndFallbackForms` is a **persistent in-tree adversarial Go unit test** that runs automatically on:

- every developer `./smackerel.sh test unit --go` invocation (local pre-push)
- every CI `unit-tests` job (the `unit-tests` matrix invokes `go test ./...` against the full module)
- every `go test ./internal/deploy/...` invocation

A future regression that relaxes the prometheus prefix check in `assertComposeContract` (e.g., from strict `strings.HasPrefix` to too-loose `strings.Contains` substring match) will FAIL RED at the `unit-tests` gate before merge. This is verified by the RED→GREEN proof under Test Evidence.

The compose contract surface is a **static-file invariant**: there is no daemon, no container, no network, no concurrency, no cross-process state involved in the assertion. The Go test suite IS the contract enforcement layer. No additional regression test type (integration, e2e, stress) is necessary or applicable.

#### Constraint Adherence

**Claim Source:** executed

- **Change boundary:** confirmed by `git diff --shortstat HEAD -- internal/deploy/compose_contract_test.go` — exactly one file modified (`internal/deploy/compose_contract_test.go`, +97 / -0 lines) plus the BUG-042-005 packet artifacts. No production runtime Go/Python code modified, no `assertComposeContract` body edited, no constant declaration edited, no `deploy/compose.deploy.yml` edited, no `docker-compose.yml` edited, no foreign-owned `specs/**` directory edited.
- **No defaults regression:** Gate G028 NO-DEFAULTS / fail-loud SST policy is strengthened by this fix — the test now explicitly proves rejection of the forbidden `${HOST_BIND_ADDRESS:-127.0.0.1}:` default-fallback form on `prometheus` (closing the last operator-facing service coverage gap; smackerel-core/ml were closed by BUG-042-004, ollama by BUG-042-003).
- **PII scan:** no real hostnames, no real IPs, no real Tailscale identifiers, no real Linux usernames in any of the seven packet artifacts. The only address strings are the generic `127.0.0.1` loopback and the `${HOST_BIND_ADDRESS:?...}` SST substitution form — both explicitly allow-listed by the repo policy.
- **Bubbles.devops mode discipline:** all bug-local artifacts (this packet) authored; foreign-owned parent-spec files (`specs/042-tailnet-edge-bind-pattern/spec.md`, `design.md`, `scopes.md`, `state.json`, `uservalidation.md`, `report.md`, plus `specs/049-*/`) NOT edited.
- **Single-bug-scope discipline:** the change addresses HL-RESCAN-010 (P3) only. The remaining home-lab readiness re-scan findings (HL-RESCAN-011..014) are tracked under their own per-finding bug-packets in the parent home-lab-readiness-rescan-2026-05-14 sweep.

## Verdict (DevOps)

**Status: SHIP_IT** — see [uservalidation.md](uservalidation.md).
