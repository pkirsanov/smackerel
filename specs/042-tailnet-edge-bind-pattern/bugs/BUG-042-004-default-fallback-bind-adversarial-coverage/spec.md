# Bug: BUG-042-004 — Compose contract test missing adversarial coverage for default-fallback bind on smackerel-core/ml

## Classification

- **Type:** DevOps defect — static-file contract test coverage gap (regression-only risk)
- **Severity:** P3 — LOW (live `deploy/compose.deploy.yml` is contract-compliant today, the existing assertion code already rejects the forbidden forms; the gap is a future-regression hole if anyone ever relaxes the prefix check)
- **Parent Spec:** 042 — Tailnet-Edge Bind Pattern (compose contract owner)
- **Workflow Mode:** test-to-doc
- **Status:** Fixed
- **Discovered By:** 2026-05-14 self-hosted readiness re-scan (finding HL-RESCAN-009)

## Problem Statement

`internal/deploy/compose_contract_test.go` is the static-file invariant test that locks the spec 042 tailnet-edge bind contract for `deploy/compose.deploy.yml`. The contract requires every `smackerel-core` and `smackerel-ml` host port mapping to use the fail-loud SST form `${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:` per Gate G028 (NO-DEFAULTS / fail-loud SST policy). Two specifically forbidden alternative forms are:

1. **Spec 020 literal form:** `127.0.0.1:` — the original loopback-only bind, deprecated by spec 042 because it cannot be overridden by the deploy adapter at apply time.
2. **Default-fallback form:** `${HOST_BIND_ADDRESS:-127.0.0.1}:` — silently falls back to 127.0.0.1 when `HOST_BIND_ADDRESS` is unset, defeating the deploy adapter's contractual obligation to inject the real bind address explicitly. Forbidden by Gate G028.

The pre-fix adversarial test suite covered:

- **`TestComposeContract_AdversarialLiteralBind`** — only the literal `127.0.0.1:` form for `smackerel-core` (one fixture; `smackerel-ml` was uncovered).
- **`TestComposeContract_AdversarialOllamaLiteralBind`** — both literal `127.0.0.1:` and default-fallback `${HOST_BIND_ADDRESS:-127.0.0.1}:` for the `ollama` service (added by BUG-042-003).
- No coverage at all for the **default-fallback** form on `smackerel-core` or `smackerel-ml`.
- No coverage at all for the **literal `127.0.0.1:`** form on `smackerel-ml`.

The existing `assertComposeContract` function does already reject every forbidden form via `strings.HasPrefix(p, requiredCorePrefix)` and `strings.HasPrefix(p, requiredMLPrefix)` — both prefix constants embed the full fail-loud `:?` substitution string, so any non-conforming bind form fails. But there was NO adversarial test that proved this rejection. If a future maintainer ever relaxed the check to a too-loose substring like `strings.Contains(p, "${HOST_BIND_ADDRESS:")` (which would match BOTH the fail-loud `:?` form AND the forbidden `:-` default-fallback form), the existing tests would not catch the regression.

The defect was a coverage gap in the contract test, not a bug in the assertion code: the live file is correct today, and the assertion is correct today, but no static-file lock prevents an accidental relaxation of the assertion. This collapsed the spec 042 / Gate G028 defense-in-depth from "the assertion AND the test prove the form" to "the assertion alone proves the form" for the two backend services.

## Detection

| Aspect | Detail |
|---|---|
| Trigger | self-hosted readiness re-scan (system review session 2026-05-14) |
| Finding | HL-RESCAN-009 |
| Severity | P3 (live file is correct today; assertion is correct today; the gap is a defense-in-depth weakness against a future relaxation of the assertion) |
| Audit method | Inspected `internal/deploy/compose_contract_test.go` adversarial test surface. Observed `TestComposeContract_AdversarialLiteralBind` only covers smackerel-core literal-bind; `TestComposeContract_AdversarialOllamaLiteralBind` covers both literal AND default-fallback for ollama; no equivalent existed for smackerel-core default-fallback or for smackerel-ml literal/default-fallback. Cross-referenced the BUG-042-003 close-out which added the analogous coverage for ollama. Confirmed RED→GREEN by temporarily relaxing the smackerel-core prefix check to `strings.Contains(p, "${HOST_BIND_ADDRESS:")` and re-running the new adversarial test. |

## Acceptance Criteria

- AC-1: A new persistent in-tree adversarial test `TestComposeContract_AdversarialDefaultFallbackBind` exists in `internal/deploy/compose_contract_test.go` with three table-driven sub-cases covering: (a) `smackerel-core` with `${HOST_BIND_ADDRESS:-127.0.0.1}:` default-fallback bind; (b) `smackerel-ml` with `${HOST_BIND_ADDRESS:-127.0.0.1}:` default-fallback bind; (c) `smackerel-ml` with literal `127.0.0.1:` bind (spec 020 form).
- AC-2: Each sub-case asserts `assertComposeContract` returns a non-nil error mentioning the violating service name (`smackerel-core` or `smackerel-ml`) AND mentioning at least one of the regression-target anchor terms (`spec 020`, `${HOST_BIND_ADDRESS:-127.0.0.1}`, `Gate-G028`, `fail-loud`).
- AC-3: Each sub-case includes attribution to `HL-RESCAN-009` in either the test docstring or the failure-case error message, so a future regression points back to this bug.
- AC-4: `TestComposeContract_LiveFile` continues to PASS GREEN against the unchanged `deploy/compose.deploy.yml` — the new test is purely additive (the live file already complies; the addition only locks the assertion's strictness against future relaxation).
- AC-5: Pre-existing adversarial tests (`TestComposeContract_AdversarialLiteralBind`, `TestComposeContract_AdversarialInfraHasPorts`, `TestComposeContract_AdversarialMultiPortsBypass`, `TestComposeContract_AdversarialMLMultiPortsBypass`, `TestComposeContract_AdversarialNetworkModeHostBypass`, `TestComposeContract_AdversarialOllamaLiteralBind`) continue to PASS GREEN unchanged — the new test does not over-reach into adjacent contract surfaces.
- AC-6: RED proof captured: temporarily relaxing the smackerel-core prefix check in `assertComposeContract` from `strings.HasPrefix(p, requiredCorePrefix)` to `strings.Contains(p, "${HOST_BIND_ADDRESS:")` (a too-loose substring check that would accept the default-fallback form) causes the new test's smackerel-core sub-case to FAIL with the expected `the contract is tautological` error, while the smackerel-ml sub-cases still PASS (because the ml branch is untouched). Restoring the strict check returns the suite to all-PASS GREEN.

## Out of Scope

- Adding contract enforcement to `docker-compose.yml` (the dev compose file). The dev compose file uses different port-mapping conventions and is governed by HL-RESCAN-012 (P3) plus a separate set of gates.
- Adding adversarial coverage for the `prometheus` service literal-bind / default-fallback regression. That coverage gap is HL-RESCAN-010 (P3), tracked as a separate bug packet.
- Editing `specs/042-tailnet-edge-bind-pattern/design.md` or `specs/042-tailnet-edge-bind-pattern/report.md` to document the new test (foreign-owned parent-spec content; outside `bubbles.devops` mode edit scope).
- Strengthening the `requiredCorePrefix` / `requiredMLPrefix` constant declarations themselves. The constants are already correct character-for-character against the live compose file; the defect is in the test surface.
- Cosign-signing or attesting the compose file itself (unrelated to the static-file invariant contract).
