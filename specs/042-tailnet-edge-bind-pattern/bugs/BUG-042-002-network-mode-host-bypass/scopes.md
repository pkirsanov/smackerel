# Scopes: BUG-042-002 — Compose contract validator network_mode: host bypass

## Scope 1: NetworkMode field + per-service guard + adversarial table-driven regression test

**Status:** Done

**File:** [internal/deploy/compose_contract_test.go](../../../../internal/deploy/compose_contract_test.go)

### Use Cases

```gherkin
Feature: assertComposeContract rejects network_mode: host on every contract-set service
  Scenario: SCN-042-002-A — smackerel-core network_mode: host bypass rejected
    Given a compose document where services.smackerel-core declares network_mode: host
    And the smackerel-core ports block is otherwise compliant with spec 042
    When assertComposeContract runs against that document
    Then it returns a non-nil error naming smackerel-core, the field network_mode, the value "host", and the BUG-042-002 attribution

  Scenario: SCN-042-002-B — smackerel-ml network_mode: host bypass rejected
    Given a compose document where services.smackerel-ml declares network_mode: host
    And the smackerel-ml ports block is otherwise compliant with spec 042
    When assertComposeContract runs against that document
    Then it returns a non-nil error naming smackerel-ml, the field network_mode, the value "host", and the BUG-042-002 attribution

  Scenario: SCN-042-002-C — postgres network_mode: host bypass rejected (Pattern P1 preserved)
    Given a compose document where services.postgres declares network_mode: host
    And the postgres ports block is empty (Pattern P1 compliant)
    When assertComposeContract runs against that document
    Then it returns a non-nil error naming postgres, the field network_mode, the value "host", and the BUG-042-002 attribution

  Scenario: SCN-042-002-D — nats network_mode: host bypass rejected (Pattern P1 preserved)
    Given a compose document where services.nats declares network_mode: host
    And the nats ports block is empty (Pattern P1 compliant)
    When assertComposeContract runs against that document
    Then it returns a non-nil error naming nats, the field network_mode, the value "host", and the BUG-042-002 attribution
```

### Implementation Plan

1. Add `NetworkMode string \`yaml:"network_mode"\`` to the inline service struct inside `composeDoc`.
2. Inside `assertComposeContract`, after each service is looked up but before the `ports:` checks, reject `NetworkMode == "host"` for `smackerel-core`, `smackerel-ml`, `postgres` (when present), and `nats` (when present). Each error names the service, the field `network_mode`, the value `"host"`, the spec 042 invariant the bypass defeats, and the `BUG-042-002` attribution.
3. Append a new table-driven test `TestComposeContract_AdversarialNetworkModeHostBypass` after `TestComposeContract_AdversarialMLMultiPortsBypass` with four sub-cases (one per service in the contract set). Each sub-case asserts (a) `err != nil`; (b) `err` mentions the service name; (c) `err` mentions `network_mode`; (d) `err` mentions `BUG-042-002` to lock the regression contract attribution.
4. Confine all changes to `internal/deploy/compose_contract_test.go` — no production code, compose, config, or doc edits.

### Test Plan

- Targeted re-run: `go test -v -count=1 -run 'TestComposeContract_AdversarialNetworkModeHostBypass' ./internal/deploy/...` — all 4 sub-tests PASS.
- Live-file regression: `go test -v -count=1 -run TestComposeContract_LiveFile ./internal/deploy/...` — still PASSES against real `deploy/compose.deploy.yml` (no live `network_mode: host` declared).
- Full spec 042 contract suite: `go test -v -count=1 -run TestComposeContract ./internal/deploy/...` — 6 functions / 10 sub-tests all PASS (including the 4 new BUG-042-002 sub-cases).
- BUG-042-001 regression: `go test -v -count=1 -run 'TestComposeContract_AdversarialMultiPortsBypass|TestComposeContract_AdversarialMLMultiPortsBypass' ./internal/deploy/...` — both still PASS.
- Cross-package smoke: `go test -count=1 ./internal/deploy/... ./internal/config/... ./internal/api/...` — all PASS.
- Static checks: `go vet ./internal/...` clean; `gofmt -l internal/deploy/` empty.

#### Test Plan Coverage Matrix

| Scenario / Behavior | Test Type | File | Test ID | Adversarial? | Regression E2E |
|---|---|---|---|---|---|
| SCN-042-002-A: smackerel-core `network_mode: host` bypass | unit (Go static-file lint) | internal/deploy/compose_contract_test.go | TestComposeContract_AdversarialNetworkModeHostBypass/smackerel-core_uses_network_mode_host | YES — fails if validator drops the smackerel-core network_mode check | Persistent in-tree adversarial Go sub-test that runs on every `go test ./internal/deploy/...` invocation. The compose contract surface is a static-file invariant, not an HTTP/UI surface, so the regression suite is the Go test suite itself. |
| SCN-042-002-B: smackerel-ml `network_mode: host` bypass | unit (Go static-file lint) | internal/deploy/compose_contract_test.go | TestComposeContract_AdversarialNetworkModeHostBypass/smackerel-ml_uses_network_mode_host | YES — fails if validator drops the smackerel-ml network_mode check | Same as above for ml side. |
| SCN-042-002-C: postgres `network_mode: host` bypass | unit (Go static-file lint) | internal/deploy/compose_contract_test.go | TestComposeContract_AdversarialNetworkModeHostBypass/postgres_uses_network_mode_host | YES — fails if validator drops the postgres network_mode check | Same as above for postgres side. |
| SCN-042-002-D: nats `network_mode: host` bypass | unit (Go static-file lint) | internal/deploy/compose_contract_test.go | TestComposeContract_AdversarialNetworkModeHostBypass/nats_uses_network_mode_host | YES — fails if validator drops the nats network_mode check | Same as above for nats side. |
| Live-file contract preservation | unit (Go static-file lint) | internal/deploy/compose_contract_test.go | TestComposeContract_LiveFile | NO — positive case | Live `deploy/compose.deploy.yml` is parsed on every test run — protects spec-042 contract from any future edit. |
| BUG-042-001 multi-ports regression (smackerel-core) | unit (Go static-file lint) | internal/deploy/compose_contract_test.go | TestComposeContract_AdversarialMultiPortsBypass | YES | Pre-existing regression coverage; preserved unchanged. |
| BUG-042-001 multi-ports regression (smackerel-ml) | unit (Go static-file lint) | internal/deploy/compose_contract_test.go | TestComposeContract_AdversarialMLMultiPortsBypass | YES | Pre-existing regression coverage; preserved unchanged. |

### Definition of Done

- [x] `composeDoc` inline service struct exposes `NetworkMode string \`yaml:"network_mode"\``. [SCN-042-002-A: smackerel-core network_mode: host bypass rejected]
   → Evidence: `grep -n 'NetworkMode string \`yaml:"network_mode"\`' internal/deploy/compose_contract_test.go` returns the struct-field line. See report.md §"Code Diff Evidence".
- [x] `assertComposeContract` rejects `smackerel-core.network_mode == "host"` with an error naming `smackerel-core`, `network_mode`, `"host"`, and `BUG-042-002`. [SCN-042-002-A: smackerel-core network_mode: host bypass rejected]
   → Evidence: targeted go test run shows the test rejects the fixture with the exact error text containing `services.smackerel-core.network_mode="host"` and `BUG-042-002 closes the network_mode bypass`. See report.md §"Test Evidence" (sub-test smackerel-core).
- [x] `assertComposeContract` rejects `smackerel-ml.network_mode == "host"` with the same contract. [SCN-042-002-B: smackerel-ml network_mode: host bypass rejected]
   → Evidence: targeted go test run shows the test rejects the fixture with the exact error text containing `services.smackerel-ml.network_mode="host"` and `BUG-042-002 closes the network_mode bypass`. See report.md §"Test Evidence" (sub-test smackerel-ml).
- [x] `assertComposeContract` rejects `postgres.network_mode == "host"` with the same contract (Pattern P1 attribution in error). [SCN-042-002-C: postgres network_mode: host bypass rejected (Pattern P1 preserved)]
   → Evidence: targeted go test run shows the test rejects the fixture with the exact error text containing `services.postgres.network_mode="host"`, `BUG-042-002`, and `Pattern P1: tailscale ssh + docker exec`. See report.md §"Test Evidence" (sub-test postgres).
- [x] `assertComposeContract` rejects `nats.network_mode == "host"` with the same contract (Pattern P1 attribution in error). [SCN-042-002-D: nats network_mode: host bypass rejected (Pattern P1 preserved)]
   → Evidence: targeted go test run shows the test rejects the fixture with the exact error text containing `services.nats.network_mode="host"`, `BUG-042-002`, and `Pattern P1: tailscale ssh + docker exec`. See report.md §"Test Evidence" (sub-test nats).
- [x] `TestComposeContract_AdversarialNetworkModeHostBypass` exists and PASSES with all 4 sub-cases. [SCN-042-002-A / B / C / D]
   → Evidence: `go test -v -count=1 -run 'TestComposeContract_AdversarialNetworkModeHostBypass' ./internal/deploy/...` exits 0 with 4/4 sub-tests PASS. See report.md §"Test Evidence" (parent test summary).
- [x] Existing `TestComposeContract_LiveFile` still PASSES (no live regression — `deploy/compose.deploy.yml` declares no `network_mode: host`).
   → Evidence: `go test -v -count=1 -run TestComposeContract_LiveFile ./internal/deploy/...` exits 0 with `--- PASS: TestComposeContract_LiveFile`. See report.md §"Test Evidence" (live-file).
- [x] Existing `TestComposeContract_AdversarialMultiPortsBypass` and `TestComposeContract_AdversarialMLMultiPortsBypass` still PASS (BUG-042-001 regression preserved).
   → Evidence: `go test -v -count=1 -run 'TestComposeContract_AdversarialMultiPortsBypass|TestComposeContract_AdversarialMLMultiPortsBypass' ./internal/deploy/...` exits 0 with both PASS. See report.md §"Test Evidence" (BUG-042-001 regression).
- [x] Existing `TestComposeContract_AdversarialLiteralBind` and `TestComposeContract_AdversarialInfraHasPorts` still PASS (spec-020 + infra-port regression preserved).
   → Evidence: full `TestComposeContract` suite shows all 6 functions and all 10 sub-tests PASS. See report.md §"Test Evidence" (full contract suite).
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior. [All four BUG-042-002 scenarios]
   → Evidence: persistent in-tree table-driven test `TestComposeContract_AdversarialNetworkModeHostBypass` with one sub-case per scenario (A/B/C/D) — runs on every `go test ./internal/deploy/...` invocation. The compose contract surface is a static-file invariant, not an HTTP/UI surface, so the regression suite is the Go test suite itself. See report.md §"Regression Evidence".
- [x] Broader E2E regression suite passes — full `internal/deploy/...` Go test suite plus cross-package smoke against `internal/config` and `internal/api` all PASS.
   → Evidence: `go test -count=1 ./internal/deploy/... ./internal/config/... ./internal/api/...` returns three `ok` lines. See report.md §"Regression Evidence" (cross-package smoke).
- [x] Static checks: `go vet ./internal/...` clean and `gofmt -l internal/deploy/` empty.
   → Evidence: see report.md §"Validation Evidence".
- [x] Consumer impact sweep complete and zero stale first-party references remain. The compose contract surface is internal-only (no exported API; the `assertComposeContract` function and `composeDoc` struct are unexported test helpers). No production callers, docs, or specs reference the validator's struct shape.
   → Evidence: `grep -rn 'composeDoc\|assertComposeContract' --include='*.go' .` shows occurrences only inside `internal/deploy/compose_contract_test.go`. See report.md §"Consumer Impact Sweep".
- [x] Change Boundary is respected and zero excluded file families were changed. Only `internal/deploy/compose_contract_test.go` modified; no production code, compose, config, or doc edits.
   → Evidence: `git diff --stat HEAD~1 HEAD -- ':!specs/'` shows a single-file change. See report.md §"Code Diff Evidence".

### Regression E2E Coverage

| Scenario | Test ID | File | Type | Adversarial? |
|---|---|---|---|---|
| SCN-042-002-A: smackerel-core network_mode: host bypass | TestComposeContract_AdversarialNetworkModeHostBypass/smackerel-core_uses_network_mode_host | internal/deploy/compose_contract_test.go | unit (Go static-file lint) | YES — fails if validator drops the smackerel-core check |
| SCN-042-002-B: smackerel-ml network_mode: host bypass | TestComposeContract_AdversarialNetworkModeHostBypass/smackerel-ml_uses_network_mode_host | internal/deploy/compose_contract_test.go | unit (Go static-file lint) | YES — fails if validator drops the smackerel-ml check |
| SCN-042-002-C: postgres network_mode: host bypass | TestComposeContract_AdversarialNetworkModeHostBypass/postgres_uses_network_mode_host | internal/deploy/compose_contract_test.go | unit (Go static-file lint) | YES — fails if validator drops the postgres check |
| SCN-042-002-D: nats network_mode: host bypass | TestComposeContract_AdversarialNetworkModeHostBypass/nats_uses_network_mode_host | internal/deploy/compose_contract_test.go | unit (Go static-file lint) | YES — fails if validator drops the nats check |
| Live-file contract preservation | TestComposeContract_LiveFile | internal/deploy/compose_contract_test.go | unit (Go static-file lint) | NO — positive case |
| BUG-042-001 multi-ports regression (core) | TestComposeContract_AdversarialMultiPortsBypass | internal/deploy/compose_contract_test.go | unit (Go static-file lint) | YES |
| BUG-042-001 multi-ports regression (ml) | TestComposeContract_AdversarialMLMultiPortsBypass | internal/deploy/compose_contract_test.go | unit (Go static-file lint) | YES |
| Spec-020 literal form regression | TestComposeContract_AdversarialLiteralBind | internal/deploy/compose_contract_test.go | unit (Go static-file lint) | YES |
| Postgres host-port re-publishing regression | TestComposeContract_AdversarialInfraHasPorts | internal/deploy/compose_contract_test.go | unit (Go static-file lint) | YES |
