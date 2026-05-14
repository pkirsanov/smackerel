# Scopes: BUG-042-003 — Ollama service exempt from spec 042 compose contract enforcement

## Scope 1: Ollama compose contract enforcement + adversarial RED→GREEN coverage

**Status:** Done

**Files:**
- [internal/deploy/compose_contract_test.go](../../../../internal/deploy/compose_contract_test.go) (new `requiredOllamaPrefix` constant + ollama enforcement block in `assertComposeContract` + new `TestComposeContract_AdversarialOllamaLiteralBind` with two sub-cases + new `ollama uses network_mode host` sub-case in `TestComposeContract_AdversarialNetworkModeHostBypass`)

### Use Cases

```gherkin
Feature: Ollama compose service is enforced by the spec 042 tailnet-edge bind contract
  Scenario: SCN-042-003-A — requiredOllamaPrefix constant declared with fail-loud SST form
    Given internal/deploy/compose_contract_test.go has a const block declaring required service-prefix constants
    When the source file is parsed by the Go compiler
    Then a `requiredOllamaPrefix` constant exists with value `${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:${OLLAMA_HOST_PORT}:` matching the live deploy/compose.deploy.yml line 243 form character-for-character

  Scenario: SCN-042-003-B — assertComposeContract rejects ollama with literal 127.0.0.1: prefix (spec 020 form)
    Given a compose fixture where ollama.ports[0] = "127.0.0.1:${OLLAMA_HOST_PORT}:${OLLAMA_CONTAINER_PORT}"
    When assertComposeContract is called with the fixture
    Then it returns a non-nil error mentioning "ollama" AND "BUG-042-003" AND the literal violating prefix

  Scenario: SCN-042-003-C — assertComposeContract rejects ollama with default-fallback ${HOST_BIND_ADDRESS:-127.0.0.1}: form
    Given a compose fixture where ollama.ports[0] = "${HOST_BIND_ADDRESS:-127.0.0.1}:${OLLAMA_HOST_PORT}:${OLLAMA_CONTAINER_PORT}"
    When assertComposeContract is called with the fixture
    Then it returns a non-nil error mentioning "ollama" AND "BUG-042-003" AND identifying the violating prefix
    And the :? (fail-loud) substitution form is the ONLY accepted form per Gate G028

  Scenario: SCN-042-003-D — assertComposeContract rejects ollama.network_mode=host
    Given a compose fixture where ollama declares network_mode: host
    When assertComposeContract is called with the fixture
    Then it returns a non-nil error mentioning "ollama" AND "network_mode" AND either "BUG-042-002" or "BUG-042-003" attribution

  Scenario: SCN-042-003-E — TestComposeContract_LiveFile preserved as positive canary
    Given the live deploy/compose.deploy.yml at HEAD
    When TestComposeContract_LiveFile reads the file and calls assertComposeContract
    Then the function returns nil and the test PASSES
    And the additive ollama enforcement does not break the existing baseline
```

### Implementation Plan

1. **`internal/deploy/compose_contract_test.go` const block:** Add `requiredOllamaPrefix = `${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:${OLLAMA_HOST_PORT}:`` next to the existing three required prefixes. Add a comment block above it naming BUG-042-003 / HL-RESCAN-005 and explaining why the constant exists.
2. **`internal/deploy/compose_contract_test.go` `assertComposeContract`:** After the prometheus block (just before the final `return nil`), add the ollama optional-service block: `if oll, ok := doc.Services["ollama"]; ok { ... }` that checks (a) `oll.NetworkMode != "host"` (else return error mentioning `ollama` + `network_mode` + `BUG-042-003`); (b) every entry in `oll.Ports` starts with `requiredOllamaPrefix` (else return error mentioning `ollama` + `BUG-042-003` + the violating prefix). Mirror the prometheus block exactly.
3. **`internal/deploy/compose_contract_test.go` new test `TestComposeContract_AdversarialOllamaLiteralBind`:** Table-driven with two sub-cases: `literal 127.0.0.1 bind (spec 020 form)` and `default-fallback ${HOST_BIND_ADDRESS:-127.0.0.1} bind (forbidden by Gate G028)`. Each sub-case builds a compose fixture, calls `assertComposeContract`, asserts non-nil error AND error mentions `ollama` AND error mentions `BUG-042-003`.
4. **`internal/deploy/compose_contract_test.go` new sub-case in `TestComposeContract_AdversarialNetworkModeHostBypass`:** Add `ollama uses network_mode host` as the fifth case in the table-driven sweep. Loosen the BUG-042-002 attribution check to accept either `BUG-042-002` OR `BUG-042-003` (the new ollama branch carries the latter; the four pre-existing services carry the former).
5. **RED→GREEN proof:** Capture FAIL output by temporarily replacing the new ollama enforcement block in `assertComposeContract` with `_ = requiredOllamaPrefix` (no-op), running the contract suite, and observing the three new ollama sub-tests FAIL while every other test PASSES. Restore the enforcement block via `replace_string_in_file` and re-run to confirm GREEN.
6. Confine all changes to `internal/deploy/compose_contract_test.go` plus the bug-packet artifacts in `specs/042-tailnet-edge-bind-pattern/bugs/BUG-042-003-ollama-compose-contract-enforcement/`. No production runtime Go/Python code, no compose, no `config/smackerel.yaml`, no other `specs/**`.

### Test Plan

- **RED→GREEN proof (scenario-first TDD):** Temporarily replace the ollama enforcement block in `assertComposeContract` with a no-op (`_ = requiredOllamaPrefix`), keeping the new tests in place. Re-run `./smackerel.sh test unit --go --segment deploy` (or direct `go test -run TestComposeContract ./internal/deploy/`). Observe `TestComposeContract_AdversarialOllamaLiteralBind/literal_127.0.0.1_bind_(spec_020_form)` FAIL, `TestComposeContract_AdversarialOllamaLiteralBind/default-fallback_${HOST_BIND_ADDRESS:-127.0.0.1}_bind_(forbidden_by_Gate_G028)` FAIL, `TestComposeContract_AdversarialNetworkModeHostBypass/ollama_uses_network_mode_host` FAIL — three FAIL with explicit error messages naming the missing assertion. All other tests (`TestComposeContract_LiveFile`, `TestComposeContract_AdversarialLiteralBind`, `TestComposeContract_AdversarialInfraHasPorts`, `TestComposeContract_AdversarialMultiPortsBypass`, `TestComposeContract_AdversarialMLMultiPortsBypass`, four pre-existing sub-cases of `TestComposeContract_AdversarialNetworkModeHostBypass`) PASS unchanged. Restore the enforcement block via `replace_string_in_file` and re-run → all PASS GREEN. Captured in report.md > Test Evidence > Red→Green proof (scenario-first TDD).
- **Targeted contract suite:** `./smackerel.sh test unit --go --segment deploy` runs the full `internal/deploy/...` Go test suite including `TestComposeContract_*` family — all PASS.
- **Adversarial isolation:** Each new ollama adversarial sub-test fails RED only when the ollama assertion clause it targets is reverted; the other adversarial sub-tests in the suite (literal-bind, infra-has-ports, multi-ports, network_mode for non-ollama services) still PASS. This proves the new locks are independently enforced (non-tautological).
- **Cross-package smoke:** `./smackerel.sh test unit --go` covers the full unit suite (`internal/deploy/...`, `internal/config/...`, `internal/auth/...`, `internal/api/...`) — all PASS, no regression.
- **Static checks:** `./smackerel.sh lint` clean; `gofmt -l internal/deploy/` empty.
- **Live spec 042 + spec 049 contracts preserved:** `TestComposeContract_LiveFile` (the canary that locks the live `deploy/compose.deploy.yml` is contract-compliant) still PASSES — the ollama addition is purely additive and the live file already complies, so the canary is undisturbed.

#### Test Plan Coverage Matrix

| Scenario / Behavior | Test Type | File | Test ID | Adversarial? | Regression E2E |
|---|---|---|---|---|---|
| SCN-042-003-A: requiredOllamaPrefix constant declared | unit (Go static-file lint) | internal/deploy/compose_contract_test.go | TestComposeContract_LiveFile (indirect, via the prefix being used in the assertion) | YES — fails RED via TestComposeContract_AdversarialOllamaLiteralBind if the constant is missing or wrong | Persistent in-tree adversarial Go test that runs on every `./smackerel.sh test unit --go` invocation. The compose contract is a static-file invariant; the regression suite IS the Go test suite itself. |
| SCN-042-003-B: literal 127.0.0.1 bind rejected | unit (Go static-file lint) | internal/deploy/compose_contract_test.go | TestComposeContract_AdversarialOllamaLiteralBind/literal_127.0.0.1_bind_(spec_020_form) | YES — fails RED if the ollama prefix check is removed | Same as above. |
| SCN-042-003-C: default-fallback bind rejected | unit (Go static-file lint) | internal/deploy/compose_contract_test.go | TestComposeContract_AdversarialOllamaLiteralBind/default-fallback_${HOST_BIND_ADDRESS:-127.0.0.1}_bind_(forbidden_by_Gate_G028) | YES — fails RED if the ollama prefix check is removed | Same as above. |
| SCN-042-003-D: network_mode host rejected | unit (Go static-file lint) | internal/deploy/compose_contract_test.go | TestComposeContract_AdversarialNetworkModeHostBypass/ollama_uses_network_mode_host | YES — fails RED if the ollama network_mode check is removed | Same as above. |
| SCN-042-003-E: live file canary preserved | unit (Go static-file lint) | internal/deploy/compose_contract_test.go | TestComposeContract_LiveFile | NO (positive canary) | Same as above. |
| Canary: BUG-042-001 + BUG-042-002 contracts preserved | unit (Go static-file lint) | internal/deploy/compose_contract_test.go | TestComposeContract_AdversarialLiteralBind, AdversarialInfraHasPorts, AdversarialMultiPortsBypass, AdversarialMLMultiPortsBypass, four pre-existing AdversarialNetworkModeHostBypass sub-cases | YES (canaries) | Pre-existing spec 042 + bug regression coverage; preserved unchanged. |
| Canary: spec 047 vuln-gate contract preserved | unit (Go static-file lint) | internal/deploy/build_workflow_vuln_gate_contract_test.go | TestVulnGateContract_LiveFile | NO (canary) | Pre-existing spec 047 contract; preserved unchanged. |
| Canary: BUG-047-001 bundle-hash contract preserved | unit (Go static-file lint) | internal/deploy/build_workflow_bundle_hash_contract_test.go | TestBundleHashContract_LiveFile + 4 adversarial sub-tests | YES (canaries) | Pre-existing BUG-047-001 contract; preserved unchanged. |

### Definition of Done

- [x] `internal/deploy/compose_contract_test.go` declares a `requiredOllamaPrefix` constant matching `deploy/compose.deploy.yml` line 243 form character-for-character. [SCN-042-003-A]
   → Evidence: `grep -n 'requiredOllamaPrefix' internal/deploy/compose_contract_test.go` returns the constant declaration. See report.md > Code Diff Evidence.
- [x] The const block has a BUG-042-003 / HL-RESCAN-005 attribution comment explaining why the new constant exists. [SCN-042-003-A]
   → Evidence: same grep returns the comment block above the constant. See report.md > Code Diff Evidence.
- [x] `assertComposeContract` enforces `oll.NetworkMode != "host"` for the ollama service when present. [SCN-042-003-D]
   → Evidence: `grep -n 'oll.NetworkMode\|services.ollama.network_mode' internal/deploy/compose_contract_test.go` returns the check. See report.md > Code Diff Evidence.
- [x] `assertComposeContract` enforces `strings.HasPrefix(p, requiredOllamaPrefix)` for every entry in `oll.Ports` when present. [SCN-042-003-A, B, C]
   → Evidence: same grep returns the per-entry prefix check. See report.md > Code Diff Evidence.
- [x] The ollama enforcement block uses the optional-service pattern (`if oll, ok := doc.Services["ollama"]; ok { ... }`) — skipped when ollama is absent in the fixture. [SCN-042-003-A]
   → Evidence: same grep shows the `if ... ok` guard. See report.md > Code Diff Evidence.
- [x] The ollama branch error messages mention `ollama` AND `BUG-042-003` for the prefix violations. [SCN-042-003-B, C]
   → Evidence: `grep -n 'BUG-042-003' internal/deploy/compose_contract_test.go` returns the error format strings. See report.md > Code Diff Evidence.
- [x] `TestComposeContract_AdversarialOllamaLiteralBind` exists with two table-driven sub-cases: `literal 127.0.0.1 bind (spec 020 form)` and `default-fallback ${HOST_BIND_ADDRESS:-127.0.0.1} bind (forbidden by Gate G028)`. [SCN-042-003-B, C]
   → Evidence: `grep -n 'TestComposeContract_AdversarialOllamaLiteralBind\|literal 127.0.0.1 bind\|default-fallback' internal/deploy/compose_contract_test.go` returns both the test function and both sub-case names. See report.md > Code Diff Evidence.
- [x] Both sub-cases of `TestComposeContract_AdversarialOllamaLiteralBind` assert non-nil error AND error mentions `ollama` AND error mentions `BUG-042-003`. [SCN-042-003-B, C]
   → Evidence: same grep returns the assertion calls. See report.md > Code Diff Evidence.
- [x] `TestComposeContract_AdversarialNetworkModeHostBypass` gains an `ollama uses network_mode host` sub-case as the fifth entry in the table. [SCN-042-003-D]
   → Evidence: `grep -n 'ollama uses network_mode host\|ollama.*network_mode: host' internal/deploy/compose_contract_test.go` returns the new sub-case fixture. See report.md > Code Diff Evidence.
- [x] The `TestComposeContract_AdversarialNetworkModeHostBypass` attribution check accepts `BUG-042-002` OR `BUG-042-003` (the new ollama branch carries the latter). [SCN-042-003-D]
   → Evidence: `grep -n 'BUG-042-002.*BUG-042-003\|BUG-042-003.*BUG-042-002' internal/deploy/compose_contract_test.go` returns the OR-check. See report.md > Code Diff Evidence.
- [x] RED proof captured: temporarily replacing the ollama enforcement block in `assertComposeContract` with `_ = requiredOllamaPrefix` causes the three new ollama sub-tests to FAIL while every OTHER test PASSES. [SCN-042-003-B, C, D]
   → Evidence: see report.md > Test Evidence > Red→Green proof (scenario-first TDD).
- [x] GREEN proof captured: restoring the ollama enforcement block returns the suite to all-PASS. [SCN-042-003-A through E]
   → Evidence: see report.md > Test Evidence > Red→Green proof (scenario-first TDD) — restore step.
- [x] `TestComposeContract_LiveFile` continues to PASS GREEN against the unchanged `deploy/compose.deploy.yml`. [SCN-042-003-E]
   → Evidence: `./smackerel.sh test unit --go --segment deploy` shows `--- PASS: TestComposeContract_LiveFile`. See report.md > Validation Evidence.
- [x] Targeted suite: `./smackerel.sh test unit --go --segment deploy` (or `go test -run TestComposeContract ./internal/deploy/`) PASS — all 7 top-level tests, all sub-tests, exit code 0. [SCN-042-003-A through E]
   → Evidence: see report.md > Validation Evidence > Targeted contract suite.
- [x] Cross-package smoke clean: full `./smackerel.sh test unit --go` PASS (`internal/config/...`, `internal/auth/...`, `internal/api/...`, `internal/deploy/...`). [Broader regression]
   → Evidence: see report.md > Validation Evidence > Cross-package smoke.
- [x] Static checks: `./smackerel.sh lint` clean; `gofmt -l internal/deploy/` empty. [Broader regression]
   → Evidence: see report.md > Validation Evidence > Static checks.
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior. [SCN-042-003-A through E]
   → Evidence: persistent in-tree `TestComposeContract_AdversarialOllamaLiteralBind` (2 sub-tests) + new ollama sub-case in `TestComposeContract_AdversarialNetworkModeHostBypass` + `TestComposeContract_LiveFile` canary — all run on every `./smackerel.sh test unit --go` invocation. The compose contract surface is a static-file invariant; the regression suite IS the Go test suite itself. See report.md > Audit Evidence > Regression Evidence.
- [x] Broader E2E regression suite passes — full `internal/deploy/...` Go test suite plus cross-package smoke against `internal/config`, `internal/auth`, `internal/api` all PASS, including the spec 042 BUG-042-001 + BUG-042-002 regression suites and the BUG-047-001 bundle-hash contract. [Broader regression]
   → Evidence: `./smackerel.sh test unit --go` returns `ok` for every internal/* package. See report.md > Audit Evidence > Cross-package smoke.
- [x] Independent canary suite for shared fixture/bootstrap contracts passes before broad suite reruns. [Spec 042 BUG-042-001 + BUG-042-002 canary + spec 047 vuln-gate canary + BUG-047-001 bundle-hash canary]
   → Evidence: `TestComposeContract_AdversarialLiteralBind`, `TestComposeContract_AdversarialInfraHasPorts`, `TestComposeContract_AdversarialMultiPortsBypass`, `TestComposeContract_AdversarialMLMultiPortsBypass`, four pre-existing sub-cases of `TestComposeContract_AdversarialNetworkModeHostBypass`, `TestVulnGateContract_LiveFile`, `TestBundleHashContract_LiveFile` + 4 sub-tests — all PASS unchanged. The new ollama enforcement is purely additive — it adds a new branch + new tests WITHOUT touching the existing assertion clauses or test fixtures. Running these canaries before the broader suite reruns proves the new contract did not over-reach into adjacent surfaces. See report.md > Audit Evidence > Canary suite.
- [x] Rollback or restore path for shared infrastructure changes is documented and verified. [Shared Infrastructure Impact Sweep]
   → Evidence: rollback is a single git revert of the BUG-042-003 commit. The new ollama enforcement is purely additive and the live file already complies, so revert is safe — no live-file mismatch could result. Restore is the same git revert. Verified by the RED proof step which temporarily disables the enforcement (no-op replacement), confirms expected FAIL output, then restores. See report.md > Code Diff Evidence + Test Evidence > Red→Green proof (scenario-first TDD).
- [x] Change Boundary respected. The fix touches only `internal/deploy/compose_contract_test.go` plus the bug-packet artifacts. No production runtime Go/Python code, no `deploy/compose.deploy.yml`, no `docker-compose.yml`, no `config/smackerel.yaml`, no other `specs/**`, no `Dockerfile`, no `ml/...`, no CI workflow, no scripts.
   → Evidence: `git status --short` shows only allowed-family files. See report.md > Code Diff Evidence.
- [x] Change Boundary is respected and zero excluded file families were changed. [Allowed file families + Excluded surfaces enumerated below]
   → Evidence: `git status --short` shows only allowed-family files. Zero changes to excluded surfaces. See report.md > Code Diff Evidence.

### Shared Infrastructure Impact Sweep

`internal/deploy/compose_contract_test.go` is a **shared bootstrap helper** in the static-file-invariant sense: it is the in-tree contract that proves `deploy/compose.deploy.yml` complies with spec 042 + spec 049 + Gate G028. Changes to its assertion function affect every `./smackerel.sh test unit --go` run and every pre-merge / pre-push validation. The BUG-042-003 fix has the following blast radius:

- **Direct downstream consumers:** every CI job + every developer pre-push run executes `./smackerel.sh test unit --go`, which includes `TestComposeContract_*`. The new ollama enforcement runs on every invocation. The live `deploy/compose.deploy.yml` already complies, so no CI run will RED-fail as a direct consequence of the new contract.
- **Operator-side fan-out:** none. The contract test is in-tree only — no operator workflow consumes it. The deploy-adapter overlay's `apply.sh` + `verify.sh` operate against the compose file directly; they do not reach into the contract test.
- **Adapter-side fan-out:** none. Same reason as above.
- **Test infrastructure (canary surface):** the eight pre-existing `TestComposeContract_*` adversarial sub-tests (literal-bind, infra-has-ports, multi-ports core/ml, four network_mode sub-cases for non-ollama services), plus `TestVulnGateContract_LiveFile`, plus `TestBundleHashContract_*` — all PASS unchanged. The natural precedence ordering (alphabetical test naming) means most of these run BEFORE the new ollama test in any single invocation, providing a "if the canaries fail first, don't trust the new contract" signal.
- **Generated-artifact contract:** none — no generated artifact changes.
- **Bootstrap contract for downstream specs:** spec 042 (tailnet-edge bind), spec 049 (Prometheus on tailnet edge), and the future spec for any additional service that adopts the fail-loud SST form all benefit from the strengthened enforcement. The pattern is explicit: "every operator-facing service in the compose file has a corresponding required prefix constant + enforcement block."
- **Rollback path:** see the corresponding DoD item — single `git revert`; no live-file mismatch possible because the live file already complies.
- **Ordering / timing / storage / session / context / role / blast radius:** no impact. The fix runs as a static-file inspection on every Go test invocation; no daemon state, no shared cache, no cross-process ordering concern.

### Change Boundary

**Allowed file families (this fix may modify):**

- `internal/deploy/compose_contract_test.go` — the static-file contract test being extended (the only code change point)
- `specs/042-tailnet-edge-bind-pattern/bugs/BUG-042-003-ollama-compose-contract-enforcement/**` — this bug packet's seven artifacts

**Excluded surfaces (this fix MUST NOT touch):**

- `deploy/compose.deploy.yml` — the live compose file already complies; editing it would either be a no-op or a real change, both outside HL-RESCAN-005's scope
- `docker-compose.yml` — dev compose file is governed by HL-RESCAN-012 (P3) and a separate set of gates
- `specs/042-tailnet-edge-bind-pattern/spec.md`, `specs/042-tailnet-edge-bind-pattern/design.md`, `specs/042-tailnet-edge-bind-pattern/scopes.md`, `specs/042-tailnet-edge-bind-pattern/state.json`, `specs/042-tailnet-edge-bind-pattern/uservalidation.md` — foreign-owned parent-spec content; outside `bubbles.devops` mode edit scope
- `specs/042-tailnet-edge-bind-pattern/report.md` — same reason
- Production runtime Go code under `internal/auth/...`, `internal/config/...`, `internal/api/...`, `internal/deploy/` (other than the `*_test.go` file being edited), `cmd/...` — the bug is in the contract-test surface, not in the runtime
- Python ML sidecar under `ml/...` — unrelated
- `config/smackerel.yaml` — the SST source has nothing to do with compose contract testing
- `Dockerfile`, `ml/Dockerfile` — unrelated
- `.github/workflows/*` — unrelated; the contract test is invoked by the existing `unit-tests` job
- `scripts/...` — unrelated
- Any other `specs/**` directory — single-bug-scope discipline

### Regression E2E Coverage

| Scenario | Test ID | File | Type | Adversarial? |
|---|---|---|---|---|
| SCN-042-003-A: requiredOllamaPrefix constant declared | TestComposeContract_LiveFile (indirect) + TestComposeContract_AdversarialOllamaLiteralBind | internal/deploy/compose_contract_test.go | unit (Go static-file lint) | YES — fails RED if constant missing or wrong |
| SCN-042-003-B: literal 127.0.0.1 bind rejected | TestComposeContract_AdversarialOllamaLiteralBind/literal_127.0.0.1_bind_(spec_020_form) | same as above | unit (Go static-file lint) | YES — fails RED if ollama prefix check removed |
| SCN-042-003-C: default-fallback bind rejected | TestComposeContract_AdversarialOllamaLiteralBind/default-fallback_${HOST_BIND_ADDRESS:-127.0.0.1}_bind_(forbidden_by_Gate_G028) | same as above | unit (Go static-file lint) | YES — fails RED if ollama prefix check removed |
| SCN-042-003-D: network_mode host rejected | TestComposeContract_AdversarialNetworkModeHostBypass/ollama_uses_network_mode_host | same as above | unit (Go static-file lint) | YES — fails RED if ollama network_mode check removed |
| SCN-042-003-E: live file canary preserved | TestComposeContract_LiveFile | same as above | unit (Go static-file lint) | NO (positive canary) |
| Canary: BUG-042-001 multi-ports preserved | TestComposeContract_AdversarialMultiPortsBypass + TestComposeContract_AdversarialMLMultiPortsBypass | same as above | unit (Go static-file lint) | YES (canaries) |
| Canary: BUG-042-002 network_mode preserved | TestComposeContract_AdversarialNetworkModeHostBypass (4 pre-existing sub-cases) | same as above | unit (Go static-file lint) | YES (canaries) |
| Canary: spec 047 vuln-gate contract preserved | TestVulnGateContract_LiveFile | internal/deploy/build_workflow_vuln_gate_contract_test.go | unit (Go static-file lint) | NO (positive canary) |
| Canary: BUG-047-001 bundle-hash contract preserved | TestBundleHashContract_LiveFile + 4 sub-tests | internal/deploy/build_workflow_bundle_hash_contract_test.go | unit (Go static-file lint) | YES (canaries) |
