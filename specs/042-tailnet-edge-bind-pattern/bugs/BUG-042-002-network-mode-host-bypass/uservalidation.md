# User Validation: BUG-042-002 — Compose contract validator network_mode: host bypass

## Checklist

- [x] **The defect (network_mode: host bypass) is real.** Empirically reproduced via out-of-tree round-11 probe at `/tmp/probe042/main.go`. The unmodified validator silently accepted a fixture where `core.network_mode: host` was paired with a compliant `core.ports[0]`.
- [x] **The fix is the minimum viable change.** One `NetworkMode` field added to `composeDoc`; four per-service `if NetworkMode == "host"` guard blocks added to `assertComposeContract` (one per service in the contract set, mirroring the existing per-service `ports:` checks); one new table-driven test function `TestComposeContract_AdversarialNetworkModeHostBypass` with four sub-cases. Confined to a single file.
- [x] **The live `deploy/compose.deploy.yml` still PASSES the contract test.** `TestComposeContract_LiveFile` continues to PASS — confirms zero regression on the real deploy file (no service in the live compose declares `network_mode: host`).
- [x] **The four new adversarial sub-tests are non-tautological.** Each sub-case asserts three independent properties: error mentions service name, error mentions the `network_mode` field, and error mentions the `BUG-042-002` attribution. A validator that drops the per-service check would fail the corresponding sub-test with the explicit "BUG-042-002 network_mode bypass is reintroduced" message.
- [x] **The fix preserves the existing 5 contract tests.** `TestComposeContract_LiveFile` + `TestComposeContract_AdversarialLiteralBind` + `TestComposeContract_AdversarialInfraHasPorts` + `TestComposeContract_AdversarialMultiPortsBypass` + `TestComposeContract_AdversarialMLMultiPortsBypass` all still PASS.
- [x] **No cross-package regression.** Smoke test of `internal/deploy/...` + `internal/config/...` + `internal/api/...` all PASS.
- [x] **No production code, compose, config, or doc files modified.** The bug fix is confined to `internal/deploy/compose_contract_test.go`. Bug-packet artifacts under `specs/042-tailnet-edge-bind-pattern/bugs/BUG-042-002-network-mode-host-bypass/` document the close-out.
- [x] **Severity classification is correct.** MEDIUM — no live exposure today (the live compose declares no `network_mode: host`), but the validator gap means a future regression that added it would be silently accepted. Same severity profile as BUG-042-001.
- [x] **Discovery context is accurate.** Stochastic-quality-sweep round 11 of 20, seed `20520512`, trigger=`test`, mapped child mode=`test-to-doc`, executionModel=`parent-expanded-child-mode` (nested `runSubagent` unavailable in the test-to-doc sub-agent runtime).
- [x] **Spec 042 parent is unaffected.** Parent spec status remains `done`. The bug fixes a test-integrity gap discovered AFTER the parent spec's bugfix-fastlane chain completed; it does not invalidate any of the 6 SCN-042-* scenarios — instead, it strengthens the protection of every spec 042 invariant by closing a categorically broader bypass than the BUG-042-001 multi-ports case.

## Sign-off

- **Bug:** BUG-042-002
- **Workflow mode:** test-to-doc
- **Parent workflow:** stochastic-quality-sweep round 11 of 20 (seed 20520512)
- **Status:** Fixed and validated
- **Decision:** SHIP_IT
