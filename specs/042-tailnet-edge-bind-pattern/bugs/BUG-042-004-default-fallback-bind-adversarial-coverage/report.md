# Report: BUG-042-004 — Compose contract test missing adversarial coverage for default-fallback bind on smackerel-core/ml

## Summary

Closes self-hosted readiness re-scan finding **HL-RESCAN-009** (P3): the spec 042 / Gate G028 compose contract test in `internal/deploy/compose_contract_test.go` previously had no adversarial sub-case proving that `assertComposeContract` rejects the **default-fallback `${HOST_BIND_ADDRESS:-127.0.0.1}:` bind form on `smackerel-core` / `smackerel-ml`** (Gate G028 NO-DEFAULTS violation), and no adversarial sub-case proving rejection of the **literal `127.0.0.1:` spec 020 form on `smackerel-ml`** (the smackerel-core literal-bind case was already covered by the pre-existing `TestComposeContract_AdversarialLiteralBind`). The runtime behavior of `assertComposeContract` was correct — it rejects all three forms today — but the test surface had a coverage gap that would let a future maintainer accidentally relax the strict `strings.HasPrefix` check (e.g., to a too-loose `strings.Contains` substring match) without any test failing RED.

The fix is **purely additive at the test layer**: a single new persistent in-tree test function `TestComposeContract_AdversarialDefaultFallbackBind` with three table-driven sub-cases. The function pins three guarantees:

1. `assertComposeContract` rejects `smackerel-core.ports[0] = "${HOST_BIND_ADDRESS:-127.0.0.1}:${CORE_HOST_PORT}:${CORE_CONTAINER_PORT}"` (default-fallback bind on core).
2. `assertComposeContract` rejects `smackerel-ml.ports[0] = "${HOST_BIND_ADDRESS:-127.0.0.1}:${ML_HOST_PORT}:${ML_CONTAINER_PORT}"` (default-fallback bind on ml).
3. `assertComposeContract` rejects `smackerel-ml.ports[0] = "127.0.0.1:${ML_HOST_PORT}:${ML_CONTAINER_PORT}"` (literal `127.0.0.1:` bind on ml).

Each sub-case asserts the rejection error mentions the violating service name AND at least one of the regression-target anchor terms `[spec 020, ${HOST_BIND_ADDRESS:-127.0.0.1}, Gate-G028, fail-loud]`. The test docstring + each sub-case's failure-case `t.Fatalf` message both carry HL-RESCAN-009 attribution so a future regression points back to this bug.

**Workflow Mode:** test-to-doc — the fix is the test addition itself plus one bug-packet that documents it. No production runtime code changes; no `assertComposeContract` change; no `requiredCorePrefix` / `requiredMLPrefix` constant change; no `deploy/compose.deploy.yml` change.

**Parent re-scan:** self-hosted readiness re-scan 2026-05-14, finding HL-RESCAN-009. Sequential per-finding bug-packet workflow.

## Completion Statement

All seven bug-packet artifacts (`spec.md`, `design.md`, `scopes.md`, `scenario-manifest.json`, `report.md`, `uservalidation.md`, `state.json`) authored. Single code change in `internal/deploy/compose_contract_test.go` (+112 lines, 0 lines deleted). Targeted contract suite GREEN. Cross-package smoke (`./smackerel.sh test unit --go`) GREEN. RED proof captured by temporarily relaxing the smackerel-core branch of `assertComposeContract` from strict `strings.HasPrefix` to a too-loose `strings.Contains` substring check — the smackerel-core sub-case correctly FAILS with the expected `the contract is tautological` message while the two smackerel-ml sub-cases continue to PASS unchanged (positive cross-check that the RED is correctly attributed to the relaxed branch). All eight pre-existing `TestComposeContract_*` adversarial tests + `TestComposeContract_LiveFile` canary + `TestVulnGateContract_LiveFile` canary + `TestBundleHashContract_*` canary suite all PASS unchanged.

**Status: SHIP_IT** — pending audit verdict in [uservalidation.md](uservalidation.md).

## Implementation Code Diff

### Code Diff Evidence

**Claim Source:** executed

```text
$ git diff --stat internal/deploy/compose_contract_test.go
 internal/deploy/compose_contract_test.go | 112 +++++++++++++++++++++++++++++++
 1 file changed, 112 insertions(+)
```

```text
$ grep -n 'TestComposeContract_AdversarialDefaultFallbackBind\|HL-RESCAN-009\|the contract is tautological' internal/deploy/compose_contract_test.go | head -30
275:            t.Fatal("adversarial contract test failed: literal 127.0.0.1: prefix on smackerel-core was accepted (the contract is tautological — it would NOT catch a regression to the spec 020 form)")
307:            t.Fatal("adversarial contract test failed: postgres ports block was accepted (the contract is tautological — it would NOT catch a regression that re-publishes a host port for postgres)")
346:            t.Fatal("adversarial contract test failed: multi-ports fixture with second entry '0.0.0.0:8443:8080' was accepted (the contract is tautological — it would NOT catch a regression that adds a non-loopback host port mapping after a compliant first entry; BUG-042-001 multi-ports bypass is reintroduced)")
499:                            t.Fatalf("adversarial contract test failed: fixture with %s.network_mode=host was accepted (the contract is tautological — it would NOT catch a regression to host networking; BUG-042-002 network_mode bypass is reintroduced)", tc.service)
567:                            t.Fatalf("adversarial contract test failed: ollama port %q was accepted (the contract is tautological — it would NOT catch a regression to the spec 020 literal form or to the default-fallback form for ollama)", tc.port)
580:// TestComposeContract_AdversarialDefaultFallbackBind proves the contract
608:// Discovered: self-hosted readiness re-scan finding HL-RESCAN-009 (P3),
611:func TestComposeContract_AdversarialDefaultFallbackBind(t *testing.T) {
665:                            t.Fatalf("adversarial contract test failed: fixture with %s using forbidden bind form was accepted (the contract is tautological — it would NOT catch a regression to ${HOST_BIND_ADDRESS:-127.0.0.1}: default-fallback or literal 127.0.0.1: form; HL-RESCAN-009 default-fallback / literal-bind ml-side coverage gap is reintroduced)", tc.service)
673:                    // to prove the rejection lands on the right contract. The HL-RESCAN-009
685:                            t.Fatalf("adversarial contract test failed: error did not mention any of [spec 020, ${HOST_BIND_ADDRESS:-127.0.0.1}, Gate-G028, fail-loud] (the regression target this HL-RESCAN-009 guard locks): %v", err)
```

**Interpretation:** the new function `TestComposeContract_AdversarialDefaultFallbackBind` is declared at line 611 with a docstring opening at line 580 explicitly attributing HL-RESCAN-009 (line 608). The failure-case message at line 665 carries `HL-RESCAN-009 default-fallback / literal-bind ml-side coverage gap is reintroduced`. The anchor-list assertion at line 685 mentions `[spec 020, ${HOST_BIND_ADDRESS:-127.0.0.1}, Gate-G028, fail-loud]` — the four regression-target anchor terms required by DD-2.

**Excluded surfaces (negative evidence — also in the diff):** zero lines changed in `assertComposeContract` (function body unchanged), zero lines changed in the `requiredCorePrefix` / `requiredMLPrefix` / `requiredPrometheusPrefix` / `requiredOllamaPrefix` constants, zero lines changed in `deploy/compose.deploy.yml`, zero lines changed in `docker-compose.yml`. The change boundary is bounded to one file (one new test function) plus the BUG-042-004 packet artifacts under `specs/042-tailnet-edge-bind-pattern/bugs/BUG-042-004-default-fallback-bind-adversarial-coverage/`. Confirmed by `git diff --stat` reporting `1 file changed, 112 insertions(+)`.

## Test Evidence

### Validation Evidence

**Claim Source:** executed

#### Targeted contract suite (full canary + new)

```text
$ go test -count=1 -v -run '^TestComposeContract$|^TestComposeContract_AdversarialLiteralBind$|^TestComposeContract_AdversarialInfraHasPorts$|^TestComposeContract_AdversarialMultiPortsBypass$|^TestComposeContract_AdversarialMLMultiPortsBypass$|^TestComposeContract_AdversarialNetworkModeHostBypass$|^TestComposeContract_AdversarialOllamaLiteralBind$|^TestComposeContract_AdversarialDefaultFallbackBind$|^TestComposeContract_LiveFile$|^TestVulnGateContract_LiveFile$|^TestBundleHashContract' ./internal/deploy/...
... (full sub-test trace; only relevant tail shown for evidence) ...
=== RUN   TestComposeContract_AdversarialDefaultFallbackBind
=== RUN   TestComposeContract_AdversarialDefaultFallbackBind/smackerel-core_uses_${HOST_BIND_ADDRESS:-127.0.0.1}:_default-fallback_bind_(forbidden_by_Gate_G028)
    compose_contract_test.go:687: adversarial OK: forbidden bind form on smackerel-core is rejected with: contract violation: services.smackerel-core.ports[0]="${HOST_BIND_ADDRESS:-127.0.0.1}:${CORE_HOST_PORT}:${CORE_CONTAINER_PORT}" does not start with required prefix "${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:${CORE_HOST_PORT}:" (a literal 127.0.0.1: prefix is the spec 020 form, a default-fallback ${HOST_BIND_ADDRESS:-127.0.0.1} form is the pre-Gate-G028 form, and any other non-fail-loud bind is forbidden by spec 042 — Gate G028 NO-DEFAULTS requires the fail-loud ${HOST_BIND_ADDRESS:?...} form so compose aborts at start time if HOST_BIND_ADDRESS is unset; BUG-042-001 closes the multi-ports bypass that previously only checked ports[0])
=== RUN   TestComposeContract_AdversarialDefaultFallbackBind/smackerel-ml_uses_${HOST_BIND_ADDRESS:-127.0.0.1}:_default-fallback_bind_(forbidden_by_Gate_G028)
    compose_contract_test.go:687: adversarial OK: forbidden bind form on smackerel-ml is rejected with: contract violation: services.smackerel-ml.ports[0]="${HOST_BIND_ADDRESS:-127.0.0.1}:${ML_HOST_PORT}:${ML_CONTAINER_PORT}" does not start with required prefix "${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:${ML_HOST_PORT}:" (a literal 127.0.0.1: prefix is the spec 020 form, a default-fallback ${HOST_BIND_ADDRESS:-127.0.0.1} form is the pre-Gate-G028 form, and any other non-fail-loud bind is forbidden by spec 042 — Gate G028 NO-DEFAULTS requires the fail-loud ${HOST_BIND_ADDRESS:?...} form so compose aborts at start time if HOST_BIND_ADDRESS is unset; BUG-042-001 closes the multi-ports bypass that previously only checked ports[0])
=== RUN   TestComposeContract_AdversarialDefaultFallbackBind/smackerel-ml_uses_literal_127.0.0.1:_bind_(spec_020_form)
    compose_contract_test.go:687: adversarial OK: forbidden bind form on smackerel-ml is rejected with: contract violation: services.smackerel-ml.ports[0]="127.0.0.1:${ML_HOST_PORT}:${ML_CONTAINER_PORT}" does not start with required prefix "${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:${ML_HOST_PORT}:" (a literal 127.0.0.1: prefix is the spec 020 form, a default-fallback ${HOST_BIND_ADDRESS:-127.0.0.1} form is the pre-Gate-G028 form, and any other non-fail-loud bind is forbidden by spec 042 — Gate G028 NO-DEFAULTS requires the fail-loud ${HOST_BIND_ADDRESS:?...} form so compose aborts at start time if HOST_BIND_ADDRESS is unset; BUG-042-001 closes the multi-ports bypass that previously only checked ports[0])
--- PASS: TestComposeContract_AdversarialDefaultFallbackBind (0.00s)
    --- PASS: TestComposeContract_AdversarialDefaultFallbackBind/smackerel-core_uses_${HOST_BIND_ADDRESS:-127.0.0.1}:_default-fallback_bind_(forbidden_by_Gate_G028) (0.00s)
    --- PASS: TestComposeContract_AdversarialDefaultFallbackBind/smackerel-ml_uses_${HOST_BIND_ADDRESS:-127.0.0.1}:_default-fallback_bind_(forbidden_by_Gate_G028) (0.00s)
    --- PASS: TestComposeContract_AdversarialDefaultFallbackBind/smackerel-ml_uses_literal_127.0.0.1:_bind_(spec_020_form) (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/deploy  0.038s
```

**Interpretation:** all 3 new sub-cases of `TestComposeContract_AdversarialDefaultFallbackBind` PASS. The full canary suite (`TestComposeContract_LiveFile`, `TestComposeContract_AdversarialLiteralBind`, `TestComposeContract_AdversarialInfraHasPorts`, `TestComposeContract_AdversarialMultiPortsBypass`, `TestComposeContract_AdversarialMLMultiPortsBypass`, `TestComposeContract_AdversarialNetworkModeHostBypass` 5 sub-cases, `TestComposeContract_AdversarialOllamaLiteralBind` 2 sub-cases, `TestVulnGateContract_LiveFile`, `TestBundleHashContract_*`) all PASS unchanged. Wall clock: 0.038s. Exit status: 0 / `PASS`.

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

#### RED phase — relax the smackerel-core branch of `assertComposeContract`

Temporarily replaced the smackerel-core prefix check inside `assertComposeContract` from strict `strings.HasPrefix(p, requiredCorePrefix)` to a too-loose `strings.Contains(p, "${HOST_BIND_ADDRESS:")` substring match. This is the most realistic accidental relaxation a future maintainer might introduce: it would match BOTH the fail-loud `${HOST_BIND_ADDRESS:?...}` form (correct) AND the forbidden default-fallback `${HOST_BIND_ADDRESS:-127.0.0.1}` form (incorrect — silently accepted).

```text
$ go test -count=1 -v -run '^TestComposeContract_AdversarialDefaultFallbackBind$' ./internal/deploy/...
=== RUN   TestComposeContract_AdversarialDefaultFallbackBind
=== RUN   TestComposeContract_AdversarialDefaultFallbackBind/smackerel-core_uses_${HOST_BIND_ADDRESS:-127.0.0.1}:_default-fallback_bind_(forbidden_by_Gate_G028)
    compose_contract_test.go:665: adversarial contract test failed: fixture with smackerel-core using forbidden bind form was accepted (the contract is tautological — it would NOT catch a regression to ${HOST_BIND_ADDRESS:-127.0.0.1}: default-fallback or literal 127.0.0.1: form; HL-RESCAN-009 default-fallback / literal-bind ml-side coverage gap is reintroduced)
=== RUN   TestComposeContract_AdversarialDefaultFallbackBind/smackerel-ml_uses_${HOST_BIND_ADDRESS:-127.0.0.1}:_default-fallback_bind_(forbidden_by_Gate_G028)
    compose_contract_test.go:687: adversarial OK: forbidden bind form on smackerel-ml is rejected with: ...
=== RUN   TestComposeContract_AdversarialDefaultFallbackBind/smackerel-ml_uses_literal_127.0.0.1:_bind_(spec_020_form)
    compose_contract_test.go:687: adversarial OK: forbidden bind form on smackerel-ml is rejected with: ...
--- FAIL: TestComposeContract_AdversarialDefaultFallbackBind (0.00s)
    --- FAIL: TestComposeContract_AdversarialDefaultFallbackBind/smackerel-core_uses_${HOST_BIND_ADDRESS:-127.0.0.1}:_default-fallback_bind_(forbidden_by_Gate_G028) (0.00s)
    --- PASS: TestComposeContract_AdversarialDefaultFallbackBind/smackerel-ml_uses_${HOST_BIND_ADDRESS:-127.0.0.1}:_default-fallback_bind_(forbidden_by_Gate_G028) (0.00s)
    --- PASS: TestComposeContract_AdversarialDefaultFallbackBind/smackerel-ml_uses_literal_127.0.0.1:_bind_(spec_020_form) (0.00s)
FAIL
exit status 1
FAIL    github.com/smackerel/smackerel/internal/deploy  0.009s
```

**Interpretation (RED):** the smackerel-core sub-case FAILS with the expected `the contract is tautological — it would NOT catch a regression to ${HOST_BIND_ADDRESS:-127.0.0.1}: default-fallback or literal 127.0.0.1: form; HL-RESCAN-009 default-fallback / literal-bind ml-side coverage gap is reintroduced` message — the precise breadcrumb a future maintainer needs. The two smackerel-ml sub-cases PASS unchanged because the relaxation only touched the smackerel-core branch of `assertComposeContract` — this is the **positive cross-check** that proves the new test correctly attributes failures to the specific service branch under test (it doesn't false-FAIL the ml sub-cases, and it doesn't false-PASS the core sub-case).

#### GREEN phase — restore strict `strings.HasPrefix`

Restored the smackerel-core branch back to `strings.HasPrefix(p, requiredCorePrefix)` via `replace_string_in_file`.

```text
$ go test -count=1 -v -run '^TestComposeContract_AdversarialDefaultFallbackBind$' ./internal/deploy/...
=== RUN   TestComposeContract_AdversarialDefaultFallbackBind
=== RUN   TestComposeContract_AdversarialDefaultFallbackBind/smackerel-core_uses_${HOST_BIND_ADDRESS:-127.0.0.1}:_default-fallback_bind_(forbidden_by_Gate_G028)
    compose_contract_test.go:687: adversarial OK: forbidden bind form on smackerel-core is rejected with: ...
=== RUN   TestComposeContract_AdversarialDefaultFallbackBind/smackerel-ml_uses_${HOST_BIND_ADDRESS:-127.0.0.1}:_default-fallback_bind_(forbidden_by_Gate_G028)
    compose_contract_test.go:687: adversarial OK: forbidden bind form on smackerel-ml is rejected with: ...
=== RUN   TestComposeContract_AdversarialDefaultFallbackBind/smackerel-ml_uses_literal_127.0.0.1:_bind_(spec_020_form)
    compose_contract_test.go:687: adversarial OK: forbidden bind form on smackerel-ml is rejected with: ...
--- PASS: TestComposeContract_AdversarialDefaultFallbackBind (0.00s)
    --- PASS: TestComposeContract_AdversarialDefaultFallbackBind/smackerel-core_uses_${HOST_BIND_ADDRESS:-127.0.0.1}:_default-fallback_bind_(forbidden_by_Gate_G028) (0.00s)
    --- PASS: TestComposeContract_AdversarialDefaultFallbackBind/smackerel-ml_uses_${HOST_BIND_ADDRESS:-127.0.0.1}:_default-fallback_bind_(forbidden_by_Gate_G028) (0.00s)
    --- PASS: TestComposeContract_AdversarialDefaultFallbackBind/smackerel-ml_uses_literal_127.0.0.1:_bind_(spec_020_form) (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/deploy  0.009s
```

**Interpretation (GREEN):** restoring the strict `strings.HasPrefix` check returns all 3 sub-cases to PASS. RED→GREEN proof complete: the new test function would catch a real-world relaxation regression (substring softening on the smackerel-core branch) AND correctly attributes the failure to the relaxed branch (the ml sub-cases stayed PASS through the perturbation, providing positive cross-check coverage).

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

**Interpretation:** the full Go unit suite (every `internal/*` and `cmd/*` package, plus `tests/e2e/agent`, `tests/integration`, `tests/stress/readiness`) runs clean — every package returns `ok` (or `[no test files]` for empty packages). No test in any other package regresses as a side effect of the BUG-042-004 test addition. The new function is bounded to `internal/deploy/compose_contract_test.go` and its only caller is the `go test` runner itself, so cross-package blast radius is zero by construction.

#### Canary suite

**Claim Source:** executed

The targeted `^TestComposeContract$|^TestComposeContract_Adversarial...|^TestComposeContract_LiveFile$|^TestVulnGateContract_LiveFile$|^TestBundleHashContract` regex run (Validation Evidence > Targeted contract suite) shows:

- `TestComposeContract_LiveFile` — PASS (positive canary; `deploy/compose.deploy.yml` complies).
- `TestComposeContract_AdversarialLiteralBind` — PASS (BUG-042 spec 042 canary; smackerel-core literal bind rejected).
- `TestComposeContract_AdversarialInfraHasPorts` — PASS (BUG-042 spec 042 canary; postgres host port re-publish rejected).
- `TestComposeContract_AdversarialMultiPortsBypass` — PASS (BUG-042-001 canary; multi-ports bypass on smackerel-core rejected).
- `TestComposeContract_AdversarialMLMultiPortsBypass` — PASS (BUG-042-001 canary; multi-ports bypass on smackerel-ml rejected).
- `TestComposeContract_AdversarialNetworkModeHostBypass` (5 sub-cases) — all PASS (BUG-042-002 canary; network_mode=host rejected on every operator-facing service).
- `TestComposeContract_AdversarialOllamaLiteralBind` (2 sub-cases) — both PASS (BUG-042-003 canary; ollama literal-bind + default-fallback rejected).
- `TestVulnGateContract_LiveFile` — PASS (spec 047 vuln-gate contract canary).
- `TestBundleHashContract_LiveFile` + 4 adversarial sub-tests — all PASS (BUG-047-001 bundle-hash contract canary).

**Interpretation:** zero canary regression. The new test is purely additive (no change to `assertComposeContract`, no change to constants, no change to live compose), so the canaries cannot regress as a side effect — and they didn't.

#### Regression Evidence

**Claim Source:** executed

The new `TestComposeContract_AdversarialDefaultFallbackBind` is a **persistent in-tree adversarial Go unit test** that runs automatically on:

- every developer `./smackerel.sh test unit --go` invocation (local pre-push)
- every CI `unit-tests` job (the `unit-tests` matrix invokes `go test ./...` against the full module)
- every `go test ./internal/deploy/...` invocation

A future regression that relaxes the smackerel-core OR smackerel-ml prefix check in `assertComposeContract` (e.g., from strict `strings.HasPrefix` to too-loose `strings.Contains` substring match) will FAIL RED at the `unit-tests` gate before merge. This is verified by the RED→GREEN proof under Test Evidence.

The compose contract surface is a **static-file invariant**: there is no daemon, no container, no network, no concurrency, no cross-process state involved in the assertion. The Go test suite IS the contract enforcement layer. No additional regression test type (integration, e2e, stress) is necessary or applicable.

#### Constraint Adherence

**Claim Source:** executed

- **Change boundary:** confirmed by `git diff --stat` — exactly one file modified (`internal/deploy/compose_contract_test.go`, +112 / -0 lines) plus the BUG-042-004 packet artifacts. No production runtime Go/Python code modified, no `assertComposeContract` body edited, no constant declaration edited, no `deploy/compose.deploy.yml` edited, no `docker-compose.yml` edited, no foreign-owned `specs/**` directory edited.
- **No defaults regression:** Gate G028 NO-DEFAULTS / fail-loud SST policy is strengthened by this fix — the test now explicitly proves rejection of the forbidden `${HOST_BIND_ADDRESS:-127.0.0.1}:` default-fallback form on both `smackerel-core` AND `smackerel-ml`.
- **PII scan:** no real hostnames, no real IPs, no real Tailscale identifiers, no real Linux usernames in any of the seven packet artifacts. The only address strings are the generic `127.0.0.1` loopback and the `${HOST_BIND_ADDRESS:?...}` SST substitution form — both explicitly allow-listed by the repo policy.
- **Bubbles.devops mode discipline:** all bug-local artifacts (this packet) authored; foreign-owned parent-spec files (`specs/042-tailnet-edge-bind-pattern/spec.md`, `design.md`, `scopes.md`, `state.json`, `uservalidation.md`, `report.md`) NOT edited.
- **Single-bug-scope discipline:** the change addresses HL-RESCAN-009 (P3) only. The remaining self-hosted readiness re-scan findings (HL-RESCAN-010 prometheus mechanically identical pattern, HL-RESCAN-011..014) are tracked under their own per-finding bug-packets in the parent self-hosted-readiness-rescan-2026-05-14 sweep.

## Verdict (DevOps)

**Status: SHIP_IT** — pending audit verdict in [uservalidation.md](uservalidation.md).
