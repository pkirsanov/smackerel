# Scopes: BUG-042-001 — Compose contract validator multi-ports bypass

## Scope 1: Validator iteration fix + adversarial regression tests

**Status:** Done

**File:** [internal/deploy/compose_contract_test.go](../../../../internal/deploy/compose_contract_test.go)

### Implementation Plan

1. Replace the single `[0]`-indexed prefix check in `assertComposeContract` for `smackerel-core.Ports` with a `for i, p := range core.Ports` loop that requires every entry to start with `requiredCorePrefix`.
2. Apply the identical loop pattern to `smackerel-ml.Ports`.
3. Update both error messages to name the violating index (`ports[%d]`), the violating value, and the BUG-042-001 attribution.
4. Append two new adversarial sub-tests after `TestComposeContract_AdversarialInfraHasPorts`:
   - `TestComposeContract_AdversarialMultiPortsBypass` — fixture with compliant `core.Ports[0]` and adversarial `core.Ports[1]="0.0.0.0:8443:8080"`.
   - `TestComposeContract_AdversarialMLMultiPortsBypass` — same shape for ml service.
5. Each new test asserts: (a) `err != nil`; (b) `err` mentions the service name; (c) `err` mentions `ports[1]` (proving the iterator covers index 1, not only `[0]`); (d) the multi-ports test for core also asserts `err` mentions `BUG-042-001` to lock the regression contract attribution.

### Test Plan

- Targeted re-run: `go test -v -count=1 -run TestComposeContract ./internal/deploy/...` — all 5 sub-tests PASS (3 existing + 2 new).
- Flake check: `go test -count=3 -run TestComposeContract ./internal/deploy/...` — 15/15 PASS.
- Red-then-green proof: temporarily revert the fix to the original `[0]`-only form, re-run the 2 new tests, observe they FAIL with the expected "tautological" / "BUG-042-001 reintroduced" messages, then restore the fix and re-run.
- Cross-package smoke: `go test -count=1 ./internal/deploy/... ./internal/config/... ./internal/api/...` — all PASS.
- Static checks: `go vet ./internal/...` clean; `gofmt -l internal/deploy/` empty.

#### Test Plan Coverage Matrix

| Scenario / Behavior | Test Type | File | Test ID | Adversarial? | Regression E2E |
|---|---|---|---|---|---|
| Multi-ports bypass on smackerel-core (BUG-042-001 primary) | unit (Go static-file lint) | internal/deploy/compose_contract_test.go | TestComposeContract_AdversarialMultiPortsBypass | YES — fails if validator reverts to `Ports[0]` | Regression E2E equivalent: persistent in-tree adversarial Go test that runs on every `go test ./internal/deploy/...` invocation. The compose contract surface is a static-file invariant, not an HTTP/UI surface, so the regression suite is the Go test suite itself. |
| Multi-ports bypass on smackerel-ml (BUG-042-001 mirror) | unit (Go static-file lint) | internal/deploy/compose_contract_test.go | TestComposeContract_AdversarialMLMultiPortsBypass | YES — fails if validator reverts to `Ports[0]` | Regression E2E equivalent: same as above for ml side. |
| Live-file contract preservation | unit (Go static-file lint) | internal/deploy/compose_contract_test.go | TestComposeContract_LiveFile | NO — positive case | Regression E2E equivalent: live `deploy/compose.deploy.yml` is parsed on every test run — protects spec-042 contract from any future edit. |
| Spec-020 literal form regression | unit (Go static-file lint) | internal/deploy/compose_contract_test.go | TestComposeContract_AdversarialLiteralBind | YES | Pre-existing regression coverage; preserved unchanged. |
| Postgres host-port re-publishing regression | unit (Go static-file lint) | internal/deploy/compose_contract_test.go | TestComposeContract_AdversarialInfraHasPorts | YES | Pre-existing regression coverage; preserved unchanged. |

### Definition of Done

- [x] Validator iterates every entry in `core.Ports` (loop replaces `[0]` indexing).
  ```
  internal/deploy/compose_contract_test.go (post-fix excerpt):
  for i, p := range core.Ports {
      if !strings.HasPrefix(p, requiredCorePrefix) {
          return fmt.Errorf("contract violation: services.smackerel-core.ports[%d]=%q does not start with required prefix %q (literal 127.0.0.1: prefix or any non-${HOST_BIND_ADDRESS:-127.0.0.1} bind is the spec 020 form and is forbidden by spec 042; BUG-042-001 closes the multi-ports bypass that previously only checked ports[0])", i, p, requiredCorePrefix)
      }
  }
  ```
- [x] Validator iterates every entry in `ml.Ports` (same pattern as core).
  ```
  internal/deploy/compose_contract_test.go (post-fix excerpt):
  for i, p := range ml.Ports {
      if !strings.HasPrefix(p, requiredMLPrefix) {
          return fmt.Errorf("contract violation: services.smackerel-ml.ports[%d]=%q does not start with required prefix %q (literal 127.0.0.1: prefix or any non-${HOST_BIND_ADDRESS:-127.0.0.1} bind is the spec 020 form and is forbidden by spec 042; BUG-042-001 closes the multi-ports bypass that previously only checked ports[0])", i, p, requiredMLPrefix)
      }
  }
  ```
- [x] `TestComposeContract_AdversarialMultiPortsBypass` exists and PASSES against the fixed validator.
  ```
  $ cd ~/smackerel && go test -v -count=1 -run TestComposeContract_AdversarialMultiPortsBypass ./internal/deploy/...
  === RUN   TestComposeContract_AdversarialMultiPortsBypass
      compose_contract_test.go:249: adversarial OK: multi-ports bypass on smackerel-core is rejected with: contract violation: services.smackerel-core.ports[1]="0.0.0.0:8443:8080" does not start with required prefix "${HOST_BIND_ADDRESS:-127.0.0.1}:${CORE_HOST_PORT}:" (literal 127.0.0.1: prefix or any non-${HOST_BIND_ADDRESS:-127.0.0.1} bind is the spec 020 form and is forbidden by spec 042; BUG-042-001 closes the multi-ports bypass that previously only checked ports[0])
  --- PASS: TestComposeContract_AdversarialMultiPortsBypass (0.00s)
  PASS
  ok      github.com/smackerel/smackerel/internal/deploy  0.014s
  EXIT_CODE=0
  ```
- [x] `TestComposeContract_AdversarialMLMultiPortsBypass` exists and PASSES against the fixed validator.
  ```
  $ cd ~/smackerel && go test -v -count=1 -run TestComposeContract_AdversarialMLMultiPortsBypass ./internal/deploy/...
  === RUN   TestComposeContract_AdversarialMLMultiPortsBypass
      compose_contract_test.go:277: adversarial OK: multi-ports bypass on smackerel-ml is rejected with: contract violation: services.smackerel-ml.ports[1]="0.0.0.0:9443:8081" does not start with required prefix "${HOST_BIND_ADDRESS:-127.0.0.1}:${ML_HOST_PORT}:" (literal 127.0.0.1: prefix or any non-${HOST_BIND_ADDRESS:-127.0.0.1} bind is the spec 020 form and is forbidden by spec 042; BUG-042-001 closes the multi-ports bypass that previously only checked ports[0])
  --- PASS: TestComposeContract_AdversarialMLMultiPortsBypass (0.00s)
  PASS
  ok      github.com/smackerel/smackerel/internal/deploy  0.014s
  EXIT_CODE=0
  ```
- [x] Existing `TestComposeContract_LiveFile` still PASSES (no regression on real `deploy/compose.deploy.yml`).
  ```
  === RUN   TestComposeContract_LiveFile
      compose_contract_test.go:144: contract OK: deploy/compose.deploy.yml satisfies spec 042 (backend ports use ${HOST_BIND_ADDRESS:-127.0.0.1}:; postgres and nats have no host ports)
  --- PASS: TestComposeContract_LiveFile (0.00s)
  ```
- [x] Existing `TestComposeContract_AdversarialLiteralBind` still PASSES (regression coverage for spec-020 literal form preserved).
  ```
  === RUN   TestComposeContract_AdversarialLiteralBind
      compose_contract_test.go:175: adversarial OK: literal 127.0.0.1: prefix on smackerel-core is rejected with: contract violation: services.smackerel-core.ports[0]="127.0.0.1:${CORE_HOST_PORT}:${CORE_CONTAINER_PORT}" does not start with required prefix "${HOST_BIND_ADDRESS:-127.0.0.1}:${CORE_HOST_PORT}:" (literal 127.0.0.1: prefix or any non-${HOST_BIND_ADDRESS:-127.0.0.1} bind is the spec 020 form and is forbidden by spec 042; BUG-042-001 closes the multi-ports bypass that previously only checked ports[0])
  --- PASS: TestComposeContract_AdversarialLiteralBind (0.00s)
  ```
- [x] Existing `TestComposeContract_AdversarialInfraHasPorts` still PASSES (regression coverage for postgres/nats port re-publishing preserved).
  ```
  === RUN   TestComposeContract_AdversarialInfraHasPorts
      compose_contract_test.go:207: adversarial OK: postgres ports block is rejected with: contract violation: services.postgres.ports is non-empty (got [127.0.0.1:5432:5432]) — postgres must have NO host port mapping per spec 042 (Pattern P1: tailscale ssh + docker exec)
  --- PASS: TestComposeContract_AdversarialInfraHasPorts (0.00s)
  ```
- [x] Adversarial red-then-green proof recorded (validator reverted to ports[0] form, both new tests FAIL; validator restored, both new tests PASS). Full evidence in [report.md](report.md#adversarial-red-then-green-proof).
- [x] No flake — 5 sub-tests × 3 iterations = 15/15 PASS in 0.014s.
- [x] `go vet ./internal/...` clean (no diagnostics).
- [x] `gofmt -l internal/deploy/` empty (formatting OK).
- [x] Cross-package smoke `go test -count=1 ./internal/deploy/... ./internal/config/... ./internal/api/...` all PASS.
- [x] No production code, compose, config, or doc files modified.
  ```
  $ cd ~/smackerel && git diff --stat HEAD -- ':!specs/'
   internal/deploy/compose_contract_test.go | 80 ++++++++++++++++++++++++++++++++--
   1 file changed, 76 insertions(+), 4 deletions(-)
  ```
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior. (For this spec the regression suite is the in-tree Go static-file lint test; spec 042 has no HTTP/UI surface. The two new adversarial sub-tests `TestComposeContract_AdversarialMultiPortsBypass` and `TestComposeContract_AdversarialMLMultiPortsBypass` provide persistent scenario-specific regression coverage that runs on every `go test ./internal/deploy/...` invocation.)
  ```
  $ cd ~/smackerel && go test -v -count=1 -run 'TestComposeContract_AdversarialMultiPortsBypass|TestComposeContract_AdversarialMLMultiPortsBypass' ./internal/deploy/... 2>&1 | tail -10
  --- PASS: TestComposeContract_AdversarialMultiPortsBypass (0.00s)
  --- PASS: TestComposeContract_AdversarialMLMultiPortsBypass (0.00s)
  PASS
  ok      github.com/smackerel/smackerel/internal/deploy  0.014s
  ```
- [x] Broader E2E regression suite passes. (For spec 042 "broader regression" is the full `internal/deploy/...` package suite plus cross-package smoke against `internal/config` and `internal/api`. The compose contract has no live HTTP/UI E2E surface; the static-file Go test suite IS the regression suite for this contract surface.)
  ```
  $ cd ~/smackerel && go test -count=1 ./internal/deploy/... ./internal/config/... ./internal/api/... 2>&1
  ok      github.com/smackerel/smackerel/internal/deploy
  ok      github.com/smackerel/smackerel/internal/config
  ok      github.com/smackerel/smackerel/internal/api
  ```

### Regression E2E Coverage

| Scenario | Test ID | File | Type | Adversarial? |
|---|---|---|---|---|
| BUG-042-001 multi-ports bypass on smackerel-core | TestComposeContract_AdversarialMultiPortsBypass | internal/deploy/compose_contract_test.go | unit (Go static-file lint) | YES — fails if validator reverts to `Ports[0]` |
| BUG-042-001 multi-ports bypass on smackerel-ml | TestComposeContract_AdversarialMLMultiPortsBypass | internal/deploy/compose_contract_test.go | unit (Go static-file lint) | YES — fails if validator reverts to `Ports[0]` |
| Spec 042 live-file contract (regression) | TestComposeContract_LiveFile | internal/deploy/compose_contract_test.go | unit (Go static-file lint) | NO — positive case |
| Spec-020 literal form regression | TestComposeContract_AdversarialLiteralBind | internal/deploy/compose_contract_test.go | unit (Go static-file lint) | YES |
| Postgres port re-publishing regression | TestComposeContract_AdversarialInfraHasPorts | internal/deploy/compose_contract_test.go | unit (Go static-file lint) | YES |
