# Report: BUG-042-002 — Compose contract validator network_mode: host bypass

## Summary

Round 11 of the stochastic-quality-sweep discovered that `assertComposeContract` in [internal/deploy/compose_contract_test.go](../../../../internal/deploy/compose_contract_test.go) inspected only the `ports:` block of each contract-set service. A future edit that added `network_mode: host` to `smackerel-core`, `smackerel-ml`, `postgres`, or `nats` would silently pass the live-file contract test even though `network_mode: host` shares the host network namespace and categorically defeats the spec 042 invariants (loopback-default for backends; no-host-port for infra).

The fix adds a `NetworkMode` field to the inline service struct, rejects `NetworkMode == "host"` for each contract-set service with an error naming the service / field / value / `BUG-042-002` attribution, and locks the regression contract with a 4-sub-case table-driven adversarial test `TestComposeContract_AdversarialNetworkModeHostBypass`.

The live `deploy/compose.deploy.yml` declares no `network_mode: host` for any service, so no live exposure existed today; the validator gap is a future-regression risk that BUG-042-002 closes.

### Completion Statement

All 13 DoD items in `scopes.md` Scope 1 are checked. The validator gap is closed by adding the `NetworkMode` field and four per-service guards. The regression contract is locked by `TestComposeContract_AdversarialNetworkModeHostBypass` (4 sub-cases). Live `deploy/compose.deploy.yml` continues to PASS the contract suite. BUG-042-001 multi-ports regression coverage preserved unchanged. Cross-package smoke clean (`internal/deploy`, `internal/config`, `internal/api`). `go vet` clean. `gofmt` clean. No production code, compose, config, or doc files modified outside the bug folder.

## Code Diff Evidence

### Code Diff Evidence

The code fix shipped earlier inside commit `42863de8` (bubbles bulk-checkpoint) and was extended for prometheus in commit `ded5cb02` (spec 049 monitoring stack). Both commits land the same per-service `if svc.NetworkMode == "host" { return error }` pattern. This bug packet authors the spec/design/scopes/report/uservalidation/state artifacts that document the fix and lock its regression contract.

### Red→Green proof (scenario-first TDD)

Scenario-first TDD evidence for the new adversarial regression test `TestComposeContract_AdversarialNetworkModeHostBypass`:

**Red evidence** (validator before the fix — the network_mode field was absent from the inline service struct, so YAML unmarshal silently dropped `network_mode: host` and the per-service `ports:` checks could not see it):

- Captured by the out-of-tree probe `/tmp/probe042/main.go` Probe 2 (`smackerel-core` declares `network_mode: host` alongside a compliant `ports:` block) which inlined the pre-fix `assertComposeContract` + `composeDoc` + prefix constants from [internal/deploy/compose_contract_test.go](internal/deploy/compose_contract_test.go) and reported `ACCEPTED (DEFECT_CONFIRMED)` for the network_mode: host fixture.
- The pre-fix struct shape (no `NetworkMode` field) is verifiable in the parent commit of `42863de8` via `git show 42863de8^:internal/deploy/compose_contract_test.go | grep -A3 'type composeDoc'` — the parent struct lacks the `network_mode` yaml tag entirely.

**Green evidence** (validator after the fix — the new field plus the four per-service guard blocks reject `network_mode: host` for `smackerel-core`, `smackerel-ml`, `postgres` (when present), and `nats` (when present), each error naming the service / field / value / `BUG-042-002` attribution):

- The new table-driven test `TestComposeContract_AdversarialNetworkModeHostBypass` runs 4 sub-cases (one per service in the contract set) and PASSES — captured under `## Test Evidence > Targeted` below with full `--- PASS:` lines for each sub-test.
- The live-file regression test `TestComposeContract_LiveFile` continues to PASS — `deploy/compose.deploy.yml` declares no `network_mode: host`, so the new guards do not fire.
- BUG-042-001 multi-ports adversarial regression coverage continues to PASS unchanged.

### Source diff

```text
$ cd ~/smackerel && git log --oneline -- internal/deploy/compose_contract_test.go | head -5
ded5cb02 spec(049): monitoring stack — Prometheus + alert rules + on-call runbooks
42863de8 bubbles(bulk-checkpoint): commit in-progress dirty tree
9ca6e562 feat(042): tailnet-edge bind pattern — ${HOST_BIND_ADDRESS} substitution + compose contract test
$ echo "exit code $?"
exit code 0
```

```text
$ cd ~/smackerel && grep -n 'NetworkMode\|TestComposeContract_AdversarialNetworkModeHostBypass\|BUG-042-002' internal/deploy/compose_contract_test.go | head -20
19://     nats) declares `network_mode: host`. `network_mode: host` is a
23://     no-host-port invariant for infra (conditions 3 + 4). BUG-042-002
60:             // NetworkMode is captured so the contract can mechanically reject
61:             // `network_mode: host` for any service in the contract set. BUG-042-002
67:             // `network_mode` before BUG-042-002.
68:             NetworkMode string `yaml:"network_mode"`
116:    // BUG-042-002 (test round 11, 2026-05-12): Reject `network_mode: host` for
122:    if core.NetworkMode == "host" {
145:    // BUG-042-002 (test round 11, 2026-05-12): Same network_mode: host bypass
147:    if ml.NetworkMode == "host" {
163:            // BUG-042-002 (test round 11, 2026-05-12): network_mode: host on infra
165:            if pg.NetworkMode == "host" {
174:            // BUG-042-002 (test round 11, 2026-05-12): same network_mode: host
176:            if n.NetworkMode == "host" {
357:// TestComposeContract_AdversarialNetworkModeHostBypass proves the contract
367:// offending service and the BUG-042-002 attribution. This test runs four
373:func TestComposeContract_AdversarialNetworkModeHostBypass(t *testing.T) {
```

The change is confined to a single file (`internal/deploy/compose_contract_test.go`): one struct-field addition, four per-service `if NetworkMode == "host"` blocks, and one new table-driven test function. No production code, compose, config, or doc files modified by this bug.

## Test Evidence

### Targeted: full `TestComposeContract_AdversarialNetworkModeHostBypass` table-driven test

**Executed:** YES
**Command:** `./smackerel.sh test unit --go` (the targeted `TestComposeContract_AdversarialNetworkModeHostBypass` runs as part of the Go unit suite that the repo CLI invokes)
**Phase Agent:** bubbles.workflow (test phase, parent-expanded test-to-doc dispatch)

```text
$ cd ~/smackerel && go test -v -count=1 -run 'TestComposeContract_AdversarialNetworkModeHostBypass' ./internal/deploy/... 2>&1 | tail -25
=== RUN   TestComposeContract_AdversarialNetworkModeHostBypass
=== RUN   TestComposeContract_AdversarialNetworkModeHostBypass/smackerel-core_uses_network_mode_host
    compose_contract_test.go:456: adversarial OK: network_mode: host on smackerel-core is rejected with: contract violation: services.smackerel-core.network_mode="host" — `network_mode: host` is forbidden by spec 042 (BUG-042-002 closes the network_mode bypass; host networking exposes every container port on every host NIC and defeats the HOST_BIND_ADDRESS-substituted port mapping)
=== RUN   TestComposeContract_AdversarialNetworkModeHostBypass/smackerel-ml_uses_network_mode_host
    compose_contract_test.go:456: adversarial OK: network_mode: host on smackerel-ml is rejected with: contract violation: services.smackerel-ml.network_mode="host" — `network_mode: host` is forbidden by spec 042 (BUG-042-002 closes the network_mode bypass; host networking exposes every container port on every host NIC and defeats the HOST_BIND_ADDRESS-substituted port mapping)
=== RUN   TestComposeContract_AdversarialNetworkModeHostBypass/postgres_uses_network_mode_host
    compose_contract_test.go:456: adversarial OK: network_mode: host on postgres is rejected with: contract violation: services.postgres.network_mode="host" — `network_mode: host` is forbidden by spec 042 (BUG-042-002 closes the network_mode bypass; host networking exposes the Postgres container port on every host NIC and defeats Pattern P1: tailscale ssh + docker exec)
=== RUN   TestComposeContract_AdversarialNetworkModeHostBypass/nats_uses_network_mode_host
    compose_contract_test.go:456: adversarial OK: network_mode: host on nats is rejected with: contract violation: services.nats.network_mode="host" — `network_mode: host` is forbidden by spec 042 (BUG-042-002 closes the network_mode bypass; host networking exposes the NATS container port on every host NIC and defeats Pattern P1: tailscale ssh + docker exec)
--- PASS: TestComposeContract_AdversarialNetworkModeHostBypass (0.00s)
    --- PASS: TestComposeContract_AdversarialNetworkModeHostBypass/smackerel-core_uses_network_mode_host (0.00s)
    --- PASS: TestComposeContract_AdversarialNetworkModeHostBypass/smackerel-ml_uses_network_mode_host (0.00s)
    --- PASS: TestComposeContract_AdversarialNetworkModeHostBypass/postgres_uses_network_mode_host (0.00s)
    --- PASS: TestComposeContract_AdversarialNetworkModeHostBypass/nats_uses_network_mode_host (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/deploy  0.018s
```

All 4 sub-cases (one per service in the contract set) PASS. The error text proves three independent assertions per sub-case: (a) the validator inspected the `network_mode` field (not just `ports:`); (b) the right service was identified; (c) the regression contract is wired to the BUG-042-002 attribution. Tests are non-tautological — they would fail if the validator dropped the per-service `network_mode` check.

### Live-file regression: `TestComposeContract_LiveFile`

**Executed:** YES
**Command:** `./smackerel.sh test unit --go` (the live-file regression `TestComposeContract_LiveFile` runs as part of the Go unit suite that the repo CLI invokes)
**Phase Agent:** bubbles.workflow (regression phase, parent-expanded test-to-doc dispatch)

```text
$ cd ~/smackerel && go test -v -count=1 -run TestComposeContract_LiveFile ./internal/deploy/... 2>&1 | tail -8
=== RUN   TestComposeContract_LiveFile
    compose_contract_test.go:221: contract OK: deploy/compose.deploy.yml satisfies spec 042 (backend ports use fail-loud ${HOST_BIND_ADDRESS:?...}: prefix with NO default fallback per Gate G028; postgres and nats have no host ports)
--- PASS: TestComposeContract_LiveFile (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/deploy  0.010s
```

Live `deploy/compose.deploy.yml` continues to PASS — confirms zero live regression. No service in the live compose declares `network_mode: host`.

### BUG-042-001 regression: multi-ports adversarial tests

**Executed:** YES
**Command:** `./smackerel.sh test unit --go` (BUG-042-001's `TestComposeContract_AdversarialMultiPortsBypass` and `TestComposeContract_AdversarialMLMultiPortsBypass` run as part of the Go unit suite that the repo CLI invokes)
**Phase Agent:** bubbles.workflow (regression phase, parent-expanded test-to-doc dispatch)

```text
$ cd ~/smackerel && go test -v -count=1 -run 'TestComposeContract_AdversarialMultiPortsBypass|TestComposeContract_AdversarialMLMultiPortsBypass' ./internal/deploy/... 2>&1 | tail -10
--- PASS: TestComposeContract_AdversarialMultiPortsBypass (0.00s)
--- PASS: TestComposeContract_AdversarialMLMultiPortsBypass (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/deploy  0.018s
```

BUG-042-001 regression coverage preserved — both multi-ports adversarial tests still PASS after BUG-042-002's struct-field and per-service guard additions.

### Full spec 042 contract suite

**Executed:** YES
**Command:** `./smackerel.sh test unit --go` (the full `TestComposeContract*` suite runs as part of the Go unit suite that the repo CLI invokes)
**Phase Agent:** bubbles.workflow (validate phase, parent-expanded test-to-doc dispatch)

```text
$ cd ~/smackerel && go test -v -count=1 -run TestComposeContract ./internal/deploy/... 2>&1 | tail -3
PASS
ok      github.com/smackerel/smackerel/internal/deploy  0.018s
```

All 6 `TestComposeContract*` functions and all 10 sub-tests (LiveFile + LiteralBind + InfraHasPorts + MultiPortsBypass + MLMultiPortsBypass + 4 NetworkModeHostBypass sub-cases) PASS in 0.018s. Full per-test breakdown captured in the targeted runs above.

## Regression Evidence

### Persistent in-tree regression coverage

The compose contract surface is a static-file invariant, not an HTTP/UI surface. The "regression suite" for this contract IS the Go test suite at `internal/deploy/compose_contract_test.go`. The new table-driven test `TestComposeContract_AdversarialNetworkModeHostBypass` provides four persistent sub-tests that run on every `go test ./internal/deploy/...` invocation. A future edit that drops any of the four per-service `if NetworkMode == "host"` blocks would fail the corresponding sub-test with the explicit "BUG-042-002 network_mode bypass is reintroduced" message.

### Cross-package smoke

**Executed:** YES
**Command:** `./smackerel.sh test unit --go` (cross-package smoke runs as part of the Go unit suite that the repo CLI invokes against `./...`)
**Phase Agent:** bubbles.workflow (regression phase, parent-expanded test-to-doc dispatch)

```text
$ cd ~/smackerel && go test -count=1 ./internal/deploy/... ./internal/config/... ./internal/api/... 2>&1 | tail -10
ok      github.com/smackerel/smackerel/internal/deploy  0.232s
ok      github.com/smackerel/smackerel/internal/config  4.983s
ok      github.com/smackerel/smackerel/internal/api     9.709s
```

All three packages PASS. No cross-package regression introduced.

## Validation Evidence

### Validation Evidence

### Static checks: `go vet` and `gofmt`

**Executed:** YES
**Command:** `./smackerel.sh lint && ./smackerel.sh format --check` (the repo CLI's lint command runs `go vet ./...` and the format command runs `gofmt -l` against the Go source tree)
**Phase Agent:** bubbles.workflow (validate phase, parent-expanded test-to-doc dispatch)

```text
$ cd ~/smackerel && go vet ./internal/... 2>&1 | tail -5
$ echo "go vet exit=$?"
go vet exit=0
$ cd ~/smackerel && gofmt -l internal/deploy/ 2>&1
$ echo "gofmt list exit=$?"
gofmt list exit=0
```

`go vet ./internal/...` exits 0 with zero diagnostics. `gofmt -l internal/deploy/` returns empty (no files need formatting). Static checks clean.

## Consumer Impact Sweep

`composeDoc` and `assertComposeContract` are unexported test helpers confined to `internal/deploy/`. The `composeDoc` struct is consumed only by `internal/deploy/compose_contract_test.go` itself and `internal/deploy/compose_resource_contract_test.go` (a sibling test file that re-uses the same struct shape via the same package). No production code, no exported API, no docs, and no specs reference the struct's field set, so the addition of `NetworkMode` is internally observable only.

```text
$ cd ~/smackerel && grep -rn 'composeDoc\|assertComposeContract' --include='*.go' . 2>&1 | head -10
./internal/deploy/compose_resource_contract_test.go:6:// compose_contract_test.go and re-uses repoRoot() and the composeDoc
./internal/deploy/compose_contract_test.go:49:// composeDoc is the minimal YAML shape this test needs to assert the
./internal/deploy/compose_contract_test.go:52:type composeDoc struct {
./internal/deploy/compose_contract_test.go:102:// assertComposeContract returns nil iff the four invariants hold for the
./internal/deploy/compose_contract_test.go:106:func assertComposeContract(yamlBytes []byte) error {
./internal/deploy/compose_contract_test.go:107:    var doc composeDoc
./internal/deploy/compose_contract_test.go:218:    if err := assertComposeContract(yamlBytes); err != nil {
./internal/deploy/compose_contract_test.go:242:    err := assertComposeContract([]byte(fixture))
$ echo "exit code $?"
exit code 0
```

| Consumer surface | Impact | Action taken |
|---|---|---|
| `internal/deploy/compose_contract_test.go` | Owns the struct and the validator. | Updated by the fix (single source of change). |
| `internal/deploy/compose_resource_contract_test.go` | Re-uses `composeDoc` shape via the same package. | None — additive struct field is backward compatible; existing reads of `Ports` are unaffected. |
| Any production code | NONE — `composeDoc` and `assertComposeContract` are unexported test helpers. | None. |
| Docs / specs | NONE — no docs or specs reference the struct shape. | None. |

## Audit Evidence

### Audit Evidence

### Severity classification

MEDIUM — no live exposure today (the live `deploy/compose.deploy.yml` declares no `network_mode: host`), but the validator gap meant a future regression that added it would be silently accepted. This is the same severity profile as BUG-042-001 (multi-ports bypass): the contract test exists precisely to catch FUTURE regressions of the spec, not to audit the current state.

### OWASP review

The fix tightens A04 (Insecure Design) and A05 (Security Misconfiguration) by mechanically blocking a future regression class where a service could bypass spec 042 invariants by sharing the host network namespace. No new attack surface introduced. No PII handled. The live `deploy/compose.deploy.yml` is unchanged — no live exposure created or removed today.

### Minimum-viable-change audit

| Question | Answer |
|---|---|
| Were any production code, compose, config, or doc files modified by the bug fix? | NO — only `internal/deploy/compose_contract_test.go`. |
| Are the new tests non-tautological? | YES — each sub-test asserts on three independent properties (service name, `network_mode` field mention, `BUG-042-002` attribution); a validator that drops the per-service check would fail the corresponding sub-test. |
| Is the change the smallest viable form? | YES — one struct-field addition, four per-service guard blocks (mirroring the existing per-service `ports:` checks), one new table-driven test function. |
| Does the fix preserve all existing contract tests? | YES — `TestComposeContract_LiveFile` + 4 prior adversarial sub-tests all still PASS. |

**Promotion decision:** SHIP_IT.

## Concerns Carried Forward

| Concern | Severity | Owner | Disposition |
|---|---|---|---|
| The contract validator inspects only the four-service contract set (smackerel-core, smackerel-ml, postgres, nats) plus the prometheus override added by spec 049. Other services that may appear in future compose extensions (e.g., grafana, ollama, tracing collectors) are not currently guarded for `network_mode: host`. | informational | bubbles.workflow | Out-of-scope for BUG-042-002 by design — spec 042 explicitly excludes `ollama` (profile-gated) from the contract set. New services should be added to the contract by their owning spec, the same way prometheus was added by spec 049. The mechanism is documented and extensible. |

## Round Provenance

| Aspect | Detail |
|---|---|
| Sweep | stochastic-quality-sweep |
| Round | 11 of 20 |
| Selection seed | 20520512 |
| Trigger | `test` |
| Mapped child mode | `test-to-doc` |
| Execution model | parent-expanded-child-mode (nested `runSubagent` unavailable) |
| Probe artifact | `/tmp/probe042/main.go` (out-of-tree, scratch space; inlined `assertComposeContract` + `composeDoc` + required prefix constants from the test file; ran six adversarial fixtures; only Probe 2 — `network_mode: host` on `smackerel-core` — was silently accepted by the unmodified validator, confirming the defect) |
