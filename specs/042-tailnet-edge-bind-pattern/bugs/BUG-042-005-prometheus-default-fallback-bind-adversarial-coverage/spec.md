# Bug: BUG-042-005 — Compose contract test missing adversarial coverage for literal-bind / default-fallback bind on prometheus

## Classification

- **Type:** DevOps defect — static-file contract test coverage gap (regression-only risk)
- **Severity:** P3 — LOW (live `deploy/compose.deploy.yml` is contract-compliant today, the existing assertion code already rejects the forbidden forms; the gap is a future-regression hole if anyone ever relaxes the prefix check)
- **Parent Spec:** 042 — Tailnet-Edge Bind Pattern (compose contract owner; spec 049 inherits the bind contract for the prometheus service)
- **Workflow Mode:** test-to-doc
- **Status:** Fixed
- **Discovered By:** 2026-05-14 home-lab readiness re-scan (finding HL-RESCAN-010)

## Problem Statement

`internal/deploy/compose_contract_test.go` is the static-file invariant test that locks the spec 042 tailnet-edge bind contract for `deploy/compose.deploy.yml`. The contract requires every host port mapping for `smackerel-core`, `smackerel-ml`, `ollama`, AND `prometheus` to use the fail-loud SST form `${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:` per Gate G028 (NO-DEFAULTS / fail-loud SST policy). Spec 049 (monitoring stack) inherits the spec 042 bind contract for `prometheus`. Two specifically forbidden alternative forms are:

1. **Spec 020 literal form:** `127.0.0.1:` — the original loopback-only bind, deprecated by spec 042 because it cannot be overridden by the deploy adapter at apply time.
2. **Default-fallback form:** `${HOST_BIND_ADDRESS:-127.0.0.1}:` — silently falls back to 127.0.0.1 when `HOST_BIND_ADDRESS` is unset, defeating the deploy adapter's contractual obligation to inject the real bind address explicitly. Forbidden by Gate G028.

The pre-fix adversarial test suite covered:

- **`TestComposeContract_AdversarialLiteralBind`** — only the literal `127.0.0.1:` form for `smackerel-core`.
- **`TestComposeContract_AdversarialOllamaLiteralBind`** — both literal `127.0.0.1:` and default-fallback `${HOST_BIND_ADDRESS:-127.0.0.1}:` for the `ollama` service (added by BUG-042-003).
- **`TestComposeContract_AdversarialDefaultFallbackBind`** — default-fallback for both `smackerel-core` and `smackerel-ml`, plus the literal `127.0.0.1:` form for `smackerel-ml` (added by BUG-042-004).
- **NO coverage at all** for either forbidden form on the `prometheus` service.

The existing `assertComposeContract` function does already reject every forbidden form for `prometheus` via `strings.HasPrefix(p, requiredPrometheusPrefix)` — the prefix constant embeds the full fail-loud `:?` substitution string, so any non-conforming bind form fails. But there was NO adversarial test that proved this rejection. If a future maintainer ever relaxed the check to a too-loose substring like `strings.Contains(p, "${HOST_BIND_ADDRESS:")` (which would match BOTH the fail-loud `:?` form AND the forbidden `:-` default-fallback form), only the live-file test (`TestComposeContract_LiveFile`) would catch a regression in the live file itself, but no test would catch a relaxation of the assertion code.

The defect was a coverage gap in the contract test, not a bug in the assertion code: the live file is correct today, and the assertion is correct today, but no static-file lock prevents an accidental relaxation of the prometheus prefix check. This collapsed the spec 042 / spec 049 / Gate G028 defense-in-depth from "the assertion AND the test prove the form" to "the assertion alone proves the form" for the `prometheus` service.

## Detection

| Aspect | Detail |
|---|---|
| Trigger | Home-lab readiness re-scan (system review session 2026-05-14) |
| Finding | HL-RESCAN-010 |
| Severity | P3 (live file is correct today; assertion is correct today; the gap is a defense-in-depth weakness against a future relaxation of the assertion) |
| Audit method | Inspected `internal/deploy/compose_contract_test.go` adversarial test surface. Confirmed: `TestComposeContract_AdversarialLiteralBind` covers only smackerel-core literal-bind; `TestComposeContract_AdversarialOllamaLiteralBind` covers both forms for ollama; BUG-042-004's `TestComposeContract_AdversarialDefaultFallbackBind` covers smackerel-core/ml default-fallback + smackerel-ml literal-bind; nothing covers prometheus at all. Cross-referenced the BUG-042-003 close-out (ollama analog) and BUG-042-004 close-out (core/ml analog) which established the exact pattern this fix replicates. Confirmed RED→GREEN by temporarily relaxing the prometheus prefix check in `assertComposeContract` to `strings.Contains(p, "${HOST_BIND_ADDRESS:")` and re-running the new adversarial test. |

## Acceptance Criteria

- AC-1: A new persistent in-tree adversarial test `TestComposeContract_AdversarialPrometheusLiteralBindAndFallbackForms` exists in `internal/deploy/compose_contract_test.go` with two table-driven sub-cases covering: (a) `prometheus` with literal `127.0.0.1:` bind (spec 020 form); (b) `prometheus` with `${HOST_BIND_ADDRESS:-127.0.0.1}:` default-fallback bind (forbidden by Gate G028).
- AC-2: Each sub-case asserts `assertComposeContract` returns a non-nil error mentioning `prometheus` AND mentioning at least one of the regression-target anchor terms (`spec 049`, `spec 042`, `fail-loud`, `${HOST_BIND_ADDRESS:?`, `${HOST_BIND_ADDRESS:-127.0.0.1}`, `literal 127.0.0.1:`).
- AC-3: Each sub-case includes attribution to `HL-RESCAN-010` in either the test docstring or the failure-case error message, so a future regression points back to this bug.
- AC-4: `TestComposeContract_LiveFile` continues to PASS GREEN against the unchanged `deploy/compose.deploy.yml` — the new test is purely additive (the live file already complies; the addition only locks the assertion's strictness against future relaxation).
- AC-5: Pre-existing adversarial tests (`TestComposeContract_AdversarialLiteralBind`, `TestComposeContract_AdversarialInfraHasPorts`, `TestComposeContract_AdversarialMultiPortsBypass`, `TestComposeContract_AdversarialMLMultiPortsBypass`, `TestComposeContract_AdversarialNetworkModeHostBypass`, `TestComposeContract_AdversarialOllamaLiteralBind`, `TestComposeContract_AdversarialDefaultFallbackBind`) continue to PASS GREEN unchanged — the new test does not over-reach into adjacent contract surfaces.
- AC-6: RED proof captured: temporarily relaxing the prometheus prefix check in `assertComposeContract` from `strings.HasPrefix(p, requiredPrometheusPrefix)` to `strings.Contains(p, "${HOST_BIND_ADDRESS:")` (a too-loose substring check that would accept the default-fallback form) causes the new test's default-fallback sub-case to FAIL with the expected `the contract is tautological` error, while the literal-bind sub-case still PASSes (because the literal form does not contain the substring `${HOST_BIND_ADDRESS:` — proving the relaxation specifically smuggles the default-fallback form). Restoring the strict check returns the suite to all-PASS GREEN.

## Out of Scope

- Adding contract enforcement to `docker-compose.yml` (the dev compose file). The dev compose file uses different port-mapping conventions and is governed by HL-RESCAN-012 (P3) plus a separate set of gates.
- Adding adversarial coverage for any other service (smackerel-core, smackerel-ml, ollama). Those gaps were closed by BUG-042-003 (ollama) and BUG-042-004 (core/ml).
- Editing `specs/042-tailnet-edge-bind-pattern/design.md`, `specs/042-tailnet-edge-bind-pattern/report.md`, or `specs/049-*/` content to document the new test (foreign-owned parent-spec content; outside `bubbles.devops` mode edit scope).
- Strengthening the `requiredPrometheusPrefix` constant declaration itself. The constant is already correct character-for-character against the live compose file; the defect is in the test surface.
- Cosign-signing or attesting the compose file itself (unrelated to the static-file invariant contract).
