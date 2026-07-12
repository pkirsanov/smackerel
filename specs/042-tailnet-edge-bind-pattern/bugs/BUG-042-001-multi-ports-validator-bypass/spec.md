# Bug: BUG-042-001 — Compose contract validator only inspects ports[0] (multi-ports bypass)

## Classification

- **Type:** Code defect — incomplete validator coverage (security-relevant test integrity gap)
- **Severity:** MEDIUM (the live `deploy/compose.deploy.yml` is currently single-port-only and contract-compliant, so no live exposure exists today; however the contract test would silently accept a regression where a future edit adds a second port mapping like `0.0.0.0:8443:8080` that exposes the API on every host NIC, defeating the spec 042 loopback-default guard)
- **Parent Spec:** 042 — Tailnet-Edge Bind Pattern (Self-Hosted Compose Readiness)
- **Workflow Mode:** chaos-hardening (parent: stochastic-quality-sweep round 9 of 20)
- **Status:** Fixed
- **Discovered By:** stochastic-quality-sweep (seed `20520512`), trigger=`chaos`, mapped child mode=`chaos-hardening`, executionModel=`parent-expanded-child-mode`

## Problem Statement

`assertComposeContract` in [internal/deploy/compose_contract_test.go](../../../../internal/deploy/compose_contract_test.go) only inspected the FIRST entry of each service's `ports` slice when enforcing the spec 042 bind contract:

```go
if !strings.HasPrefix(core.Ports[0], requiredCorePrefix) {
    return fmt.Errorf("contract violation: services.smackerel-core.ports[0]=%q does not start with required prefix %q (literal 127.0.0.1: prefix is the spec 020 form and is forbidden by spec 042)", core.Ports[0], requiredCorePrefix)
}
```

The same `[0]`-only check existed for `smackerel-ml`. This means a future edit that left the first entry compliant but added a second adversarial entry — for example:

```yaml
services:
  smackerel-core:
    ports:
      - "${HOST_BIND_ADDRESS:-127.0.0.1}:${CORE_HOST_PORT}:${CORE_CONTAINER_PORT}"
      - "0.0.0.0:8443:8080"   # <-- BYPASS: exposes API on every host NIC
```

— would PASS the contract test even though it directly violates the spec 042 intent of "every published host port for these services uses the configurable `HOST_BIND_ADDRESS` bind, defaulting to loopback".

The defect violates Gate G028 (`requireImplementationRealityScan`) and the chaos-hardening constraint `requireProtectedRegressionContracts`: the live-file contract test creates a false sense of security because its protection is only one-port-deep.

## Detection

| Aspect | Detail |
|---|---|
| Trigger | stochastic-quality-sweep `chaos` probe on spec 042 contract surface |
| Sweep round | 9 of 20 (selection seed `20520512`) |
| Mapped child mode | `chaos-hardening` (parent-expanded; nested `runSubagent` unavailable) |
| File | [internal/deploy/compose_contract_test.go](../../../../internal/deploy/compose_contract_test.go) |
| Location | `assertComposeContract` — `core.Ports[0]` and `ml.Ports[0]` indexed checks |
| Pre-existing test coverage | `TestComposeContract_AdversarialLiteralBind` (only mutates index 0); `TestComposeContract_AdversarialInfraHasPorts` (only checks postgres). Neither test exercised a multi-element `ports` slice for the backend services. |

### Chaos probe artifact

Probe file: `/tmp/smackerel-chaos-round9/main.go` (out-of-tree, scratch space). Probe inlines a copy of `assertComposeContract`, `composeDoc`, and the two `required*Prefix` constants from the test file, then runs three adversarial fixtures:

| Probe sub-test | Fixture | Old validator outcome | Defect classification |
|---|---|---|---|
| MultiPortsBypass | `core.ports = ["${HOST_BIND_ADDRESS:-127.0.0.1}:${CORE_HOST_PORT}:${CORE_CONTAINER_PORT}", "0.0.0.0:8443:8080"]` | ❌ ACCEPTED (bypass) | **DEFECT_CONFIRMED** |
| HostBindAddrEmptyDefault | `core.ports = ["${HOST_BIND_ADDRESS-127.0.0.1}:${CORE_HOST_PORT}:${CORE_CONTAINER_PORT}"]` (no colon, so empty value falls through) | ✅ rejected | Not a defect (different prefix string) |
| PostgresLongFormPort | `postgres.ports = [{ target: 5432, published: 5432, host_ip: 0.0.0.0 }]` | ✅ rejected (yaml unmarshal into `[]string` blocks long-form) | Not a defect (defense in depth) |

Only `MultiPortsBypass` qualifies as a real defect. The other two probes confirm the validator is correctly tight on prefix exact-match and on yaml shape.

## Behavior Contract

**Pre-fix (defect):**
- `assertComposeContract` returns `nil` for any compose document where `core.Ports[0]` and `ml.Ports[0]` satisfy the prefix, regardless of what additional port entries declare.
- A regression that adds a second non-loopback port mapping silently passes the live-file contract test, defeating spec 042 protection.

**Post-fix (required behavior):**
- `assertComposeContract` iterates over EVERY entry in `core.Ports` and `ml.Ports`. Each entry must start with the required `${HOST_BIND_ADDRESS:-127.0.0.1}:${CORE_HOST_PORT}:` (or `${ML_HOST_PORT}:`) prefix.
- A regression that adds a second non-loopback port mapping is rejected with an error naming the violating index (`ports[1]`), the violating value, and the BUG-042-001 attribution.
- The fix is parity-only for compliant inputs: the live `deploy/compose.deploy.yml` continues to PASS unchanged because every entry already satisfies the prefix.
- Two new adversarial sub-tests (`TestComposeContract_AdversarialMultiPortsBypass` for core and `TestComposeContract_AdversarialMLMultiPortsBypass` for ml) lock the regression contract.

## Acceptance Criteria

| ID | Criterion |
|---|---|
| BUG-042-001-AC-1 | `assertComposeContract` validates every entry in `core.Ports` (not only `[0]`); a fixture with a compliant `[0]` and a non-compliant `[1]` returns a non-nil error. |
| BUG-042-001-AC-2 | `assertComposeContract` validates every entry in `ml.Ports` (not only `[0]`); same regression coverage as AC-1 for ml. |
| BUG-042-001-AC-3 | The error message names the violating index (e.g., `ports[1]`), the violating value, and includes BUG-042-001 attribution so future agents can trace the regression contract. |
| BUG-042-001-AC-4 | A non-tautological adversarial test (`TestComposeContract_AdversarialMultiPortsBypass`) exists in `internal/deploy/compose_contract_test.go` that fails if the validator reverts to `Ports[0]`-only inspection. |
| BUG-042-001-AC-5 | A parallel non-tautological adversarial test (`TestComposeContract_AdversarialMLMultiPortsBypass`) exists for the ml service. |
| BUG-042-001-AC-6 | The live `deploy/compose.deploy.yml` continues to PASS the contract test unchanged — `TestComposeContract_LiveFile` remains green. |
| BUG-042-001-AC-7 | The full `internal/deploy/...` Go test suite remains green; no behavioral regression. No cross-package regression in `internal/config` or `internal/api`. |
| BUG-042-001-AC-8 | No production code, compose, config, or doc files modified. The fix is confined to `internal/deploy/compose_contract_test.go` (validator function + 2 new adversarial tests). |
