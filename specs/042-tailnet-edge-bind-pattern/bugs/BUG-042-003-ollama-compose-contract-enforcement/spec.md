# Bug: BUG-042-003 — Ollama service exempt from spec 042 compose contract enforcement

## Classification

- **Type:** DevOps defect — static-file contract coverage gap (regression-only risk)
- **Severity:** P2 — MEDIUM (live `deploy/compose.deploy.yml` is contract-compliant today; gap is a future-regression hole)
- **Parent Spec:** 042 — Tailnet-Edge Bind Pattern (compose contract owner)
- **Workflow Mode:** test-to-doc
- **Status:** Fixed
- **Discovered By:** 2026-05-14 home-lab readiness re-scan (finding HL-RESCAN-005)

## Problem Statement

`internal/deploy/compose_contract_test.go` is the static-file invariant test that locks the spec 042 tailnet-edge bind contract for `deploy/compose.deploy.yml`. The contract enforces:

1. `smackerel-core` and `smackerel-ml` host port mappings use the fail-loud SST form `${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:` per Gate G028.
2. `postgres` and `nats` declare NO host port (Pattern P1: `tailscale ssh + docker exec`).
3. `prometheus` (profile-gated, spec 049) inherits the same fail-loud SST form.
4. NO service in the contract set declares `network_mode: host` (BUG-042-002 close-out).

The `ollama` service, profile-gated by `profiles: [ollama]`, publishes a host port via the same fail-loud SST form on line 243 of the live compose file:

```yaml
ports:
  - "${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:${OLLAMA_HOST_PORT}:${OLLAMA_CONTAINER_PORT}"
```

The contract test, however, did NOT enforce this — `ollama` was missing from `assertComposeContract` entirely. A future edit reverting `ollama` to literal `127.0.0.1:` (spec 020 form) or to the forbidden `${HOST_BIND_ADDRESS:-127.0.0.1}` default-fallback form (forbidden by Gate G028) would slip past `TestComposeContract_LiveFile` and ship to home-lab.

The defect was a coverage gap in the contract: the live file was correct by convention, but no static-file lock prevented a regression. This collapsed the spec 042 / Gate G028 defense-in-depth to "trust whatever convention says" for the ollama service, while every other operator-facing service in the compose set was mechanically enforced.

## Detection

| Aspect | Detail |
|---|---|
| Trigger | Home-lab readiness re-scan (system review session 2026-05-14) |
| Finding | HL-RESCAN-005 |
| Severity | P2 (live file is correct today; the gap is regression-only — a future bad edit would not be caught at pre-merge) |
| Audit method | Inspected `internal/deploy/compose_contract_test.go` for which service names `assertComposeContract` enforces; observed only `smackerel-core`, `smackerel-ml`, `postgres`, `nats`, `prometheus`. Cross-referenced `deploy/compose.deploy.yml` for which services use `${HOST_BIND_ADDRESS:?...}` substitution; found `ollama` (line 243) was substituted but unenforced. Confirmed RED→GREEN by temporarily removing the ollama enforcement block from a draft fix and re-running the new adversarial sub-tests. |

## Acceptance Criteria

- AC-1: `internal/deploy/compose_contract_test.go` declares a `requiredOllamaPrefix` constant matching the live `deploy/compose.deploy.yml` line 243 form for `ollama`.
- AC-2: `assertComposeContract` enforces (a) ollama port-mapping prefix MUST equal `requiredOllamaPrefix` for every entry in `ollama.ports`; (b) `ollama.network_mode` MUST NOT equal `"host"`. Both checks are skipped when the ollama service block is absent (mirrors the prometheus / postgres / nats optional-service pattern).
- AC-3: A new persistent in-tree adversarial test `TestComposeContract_AdversarialOllamaLiteralBind` runs two sub-cases: literal `127.0.0.1:` form rejection AND default-fallback `${HOST_BIND_ADDRESS:-127.0.0.1}:` form rejection. Both sub-cases assert non-nil error mentioning `ollama` AND `BUG-042-003` attribution.
- AC-4: The existing `TestComposeContract_AdversarialNetworkModeHostBypass` table-driven sweep gains an `ollama uses network_mode host` sub-case, asserting the contract function rejects the regression with an error mentioning `ollama` AND either `BUG-042-002` or `BUG-042-003` attribution.
- AC-5: `TestComposeContract_LiveFile` continues to PASS GREEN against the unchanged `deploy/compose.deploy.yml` — the new ollama enforcement is purely additive (the live file already complies; the addition only locks the compliance against future drift).
- AC-6: RED proof captured: temporarily removing the new ollama enforcement block from `assertComposeContract` (keeping the new tests in place) causes the three new ollama tests to FAIL with the expected error messages, while every OTHER test in the contract suite continues to PASS. Restoring the enforcement returns the suite to GREEN.

## Out of Scope

- Adding ollama enforcement to `docker-compose.yml` (the dev compose file). The dev compose file uses different port-mapping conventions and is governed by a separate set of gates (HL-RESCAN-012 covers its `${VAR:-default}` form sweep; this fix is bounded to `deploy/compose.deploy.yml` enforcement).
- Editing `specs/042-tailnet-edge-bind-pattern/design.md` or `specs/042-tailnet-edge-bind-pattern/report.md` to remove stale `${HOST_BIND_ADDRESS:-127.0.0.1}` references in narrative text (foreign-owned spec content; outside `bubbles.devops` mode edit scope).
- Adding contract enforcement for any OTHER service that uses `HOST_BIND_ADDRESS` substitution but is not currently enforced (only ollama is in the immediate scope of HL-RESCAN-005; future findings would handle other services individually).
- Cosign-signing or attesting the compose file itself (unrelated to the static-file invariant contract).
