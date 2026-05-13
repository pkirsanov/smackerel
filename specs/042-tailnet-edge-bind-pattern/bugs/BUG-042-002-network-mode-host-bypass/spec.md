# Bug: BUG-042-002 — Compose contract validator silently accepts `network_mode: host`

## Classification

- **Type:** Code defect — incomplete validator coverage (security-relevant test integrity gap)
- **Severity:** MEDIUM (the live `deploy/compose.deploy.yml` does not declare `network_mode: host` for any service, so no live exposure exists today; however the contract test would silently accept a regression where a future edit adds `network_mode: host` to `smackerel-core`, `smackerel-ml`, `postgres`, or `nats`, which would categorically defeat the spec 042 loopback-default guard for backends and the no-host-port invariant for infra by sharing the host network namespace and exposing every container port on every host NIC)
- **Parent Spec:** 042 — Tailnet-Edge Bind Pattern (Home-Lab Compose Readiness)
- **Workflow Mode:** test-to-doc (parent: stochastic-quality-sweep round 11 of 20)
- **Status:** Fixed
- **Discovered By:** stochastic-quality-sweep (seed `20520512`), trigger=`test`, mapped child mode=`test-to-doc`, executionModel=`parent-expanded-child-mode`

## Problem Statement

`assertComposeContract` in [internal/deploy/compose_contract_test.go](../../../../internal/deploy/compose_contract_test.go) inspected ONLY the `ports:` block of each service. It silently ignored `network_mode`, even though `network_mode: host` is a categorical bypass of every spec 042 invariant the contract claims to enforce:

| Spec 042 invariant | What `network_mode: host` does to it |
|---|---|
| Backends bind via `${HOST_BIND_ADDRESS:-127.0.0.1}:` (loopback-default) | Bypassed — container shares the host network namespace; any port the process binds is visible on every host NIC, including public WAN. The HOST_BIND_ADDRESS-substituted port mapping is silently irrelevant. |
| Infra (postgres, nats) publishes NO host port (Pattern P1: `tailscale ssh + docker exec`) | Bypassed — the container's internal port (`5432`, `4222`) is directly reachable on every host NIC because the container shares the host network. Pattern P1 access is no longer the only access path. |

A future edit that left the `ports:` block compliant but added `network_mode: host` — for example to "fix" a host-loopback connection issue without understanding the spec 042 contract — would PASS the live-file contract test even though it directly violates the spec 042 intent.

The defect is the same shape as BUG-042-001 (Round 9 multi-ports bypass): the contract test enforces a narrow surface of the spec while leaving a categorical bypass unguarded, creating a false sense of security.

## Detection

| Aspect | Detail |
|---|---|
| Trigger | stochastic-quality-sweep `test` probe on spec 042 contract surface |
| Sweep round | 11 of 20 (selection seed `20520512`) |
| Mapped child mode | `test-to-doc` (parent-expanded; nested `runSubagent` unavailable) |
| File | [internal/deploy/compose_contract_test.go](../../../../internal/deploy/compose_contract_test.go) |
| Location | `assertComposeContract` — inspects only `Ports` field of each service struct, never reads `network_mode` |
| Pre-existing test coverage | `TestComposeContract_LiveFile` + 4 adversarial sub-tests (literal bind, infra ports, multi-ports core, multi-ports ml). NONE exercised `network_mode`. |

### Test-round probe artifact

Probe file: `/tmp/probe042/main.go` (out-of-tree, scratch space). Probe inlines a copy of `assertComposeContract`, `composeDoc`, and the `required*Prefix` constants from the test file, then runs six adversarial fixtures. Five are correctly rejected by the existing validator, one is silently accepted:

| Probe sub-test | Fixture | Old validator outcome | Defect classification |
|---|---|---|---|
| 1: long-form ports object on smackerel-core (`host_ip: 0.0.0.0`) | `core.ports = [{target: 8080, published: 41001, host_ip: 0.0.0.0}]` | ✅ rejected (yaml unmarshal into `[]string` blocks long-form) | Not a defect (defense in depth) |
| 2: `network_mode: host` on smackerel-core | `core.network_mode: host` + compliant ports block | ❌ ACCEPTED (bypass) | **DEFECT_CONFIRMED** |
| 3: `${HOST_BIND_ADDRESS}` without `:-127.0.0.1` default | `core.ports = ["${HOST_BIND_ADDRESS}:..."]` | ✅ rejected (strict prefix mismatch) | Not a defect |
| 4: bare container port "8080" (random host port on 0.0.0.0) | `core.ports = ["8080"]` | ✅ rejected (strict prefix mismatch) | Not a defect |
| 5: postgres long-form ports object on 0.0.0.0 | `postgres.ports = [{target: 5432, host_ip: 0.0.0.0}]` | ✅ rejected (yaml unmarshal into `[]string` blocks long-form) | Not a defect |
| 6: service rename (`smackerel_core` underscore typo) | service key `smackerel_core` instead of `smackerel-core` | ✅ rejected (`smackerel-core not found`) | Not a defect |

Only `network_mode: host` qualifies as a real defect. Probes 1, 3, 4, 5, 6 confirm the validator is correctly tight on the surfaces it inspects — but Probe 2 reveals the validator has zero coverage of the `network_mode` field, which is a categorically broader bypass than any of the prefix-level adversarial cases.

## Behavior Contract

**Pre-fix (defect):**
- `assertComposeContract` returns `nil` for any compose document where the `ports:` block of each service satisfies the existing rules, regardless of whether the service declares `network_mode: host`.
- A regression that adds `network_mode: host` to any of `smackerel-core`, `smackerel-ml`, `postgres`, or `nats` silently passes the live-file contract test, defeating spec 042 protection by sharing the host network namespace.

**Post-fix (required behavior):**
- `assertComposeContract` reads the `network_mode` field of each service in the contract set.
- If any of `smackerel-core`, `smackerel-ml`, `postgres`, or `nats` declares `network_mode: host`, the validator returns a non-nil error naming the offending service, the value `"host"`, and the BUG-042-002 attribution.
- The fix is parity-only for compliant inputs: the live `deploy/compose.deploy.yml` continues to PASS unchanged because no service declares `network_mode: host`.
- A new adversarial test (`TestComposeContract_AdversarialNetworkModeHostBypass`) with four table-driven sub-cases (one per service in the contract set) locks the regression contract.

## Acceptance Criteria

| ID | Criterion |
|---|---|
| BUG-042-002-AC-1 | `composeDoc` struct exposes the `network_mode` field via a `NetworkMode string yaml:"network_mode"` tag so the validator can read it. |
| BUG-042-002-AC-2 | `assertComposeContract` rejects `smackerel-core.network_mode == "host"` with an error naming `smackerel-core`, the field `network_mode`, and `BUG-042-002`. |
| BUG-042-002-AC-3 | Same rejection contract for `smackerel-ml.network_mode == "host"`. |
| BUG-042-002-AC-4 | Same rejection contract for `postgres.network_mode == "host"` (when the postgres service exists). |
| BUG-042-002-AC-5 | Same rejection contract for `nats.network_mode == "host"` (when the nats service exists). |
| BUG-042-002-AC-6 | A non-tautological adversarial test `TestComposeContract_AdversarialNetworkModeHostBypass` exists in `internal/deploy/compose_contract_test.go` with four table-driven sub-cases (one per service) that fail if the validator omits the `network_mode` check. |
| BUG-042-002-AC-7 | The live `deploy/compose.deploy.yml` continues to PASS the contract test unchanged — `TestComposeContract_LiveFile` remains green. |
| BUG-042-002-AC-8 | All BUG-042-001 regression tests (`TestComposeContract_AdversarialMultiPortsBypass` and `TestComposeContract_AdversarialMLMultiPortsBypass`) remain green — Round 9 work is preserved. |
| BUG-042-002-AC-9 | The full `internal/deploy/...` Go test suite remains green; no behavioral regression. No cross-package regression in `internal/config` or `internal/api`. |
| BUG-042-002-AC-10 | No production code, compose, config, or doc files modified. The fix is confined to `internal/deploy/compose_contract_test.go` (struct field + 4 inline guards + 1 new test function). |
