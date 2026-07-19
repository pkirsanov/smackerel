# Scopes: BUG-050-002 — Expose a degraded ML sidecar via an opt-in `?strict` HTTP status

> **Plan Status:** Fixed and certified `done` (2026-07-19). The ML sidecar `/health` gains an opt-in
> `?strict=true|1|yes` that returns HTTP 503 when the sidecar status is not `up` (e.g. NATS
> disconnected), while the DEFAULT `/health` (what the Docker `HEALTHCHECK` sends) stays a plain 200
> — byte-for-byte unchanged — so a degraded-but-alive sidecar is never restart-flapped. This mirrors
> the Go core `/api/health` `?strict` opt-in already committed at HEAD for this bug. All DoD items are
> complete with fresh current-session evidence (Python unit lane GREEN 630 passed, Go health lane
> GREEN, regression guard `0 violations`, check/lint clean); the full bugfix-fastlane specialist
> pipeline executed and `state-transition-guard` passes at `done`.
>
> **Mode:** `bugfix-fastlane`  ·  **Release Train:** `mvp`
>
> **Authoritative inputs:** [bug.md](bug.md), [spec.md](spec.md), [design.md](design.md),
> [scenario-manifest.json](scenario-manifest.json)

## Execution Outline

| # | Scope | Owner | Depends On | Status |
|---|-------|-------|------------|--------|
| 1 | Expose a degraded ML sidecar via an opt-in `?strict` HTTP 503, default liveness 200 unchanged | `bubbles.implement` | none | Done |

## Scope 1: Expose a degraded ML sidecar via an opt-in `?strict` HTTP 503 (default liveness 200 unchanged)

**Scope-Kind:** contract-only

**Status:** Done

**Owner:** `bubbles.implement`

**Depends On:** none

> Contract-only: the in-repo deliverable is an HTTP-contract change on the ML sidecar `/health`
> endpoint, proven by FastAPI-`TestClient` HTTP-layer unit tests (real ASGI status codes), not a live
> multi-container E2E surface. The contract is the `/health` `?strict` opt-in
> (`?strict=true|1|yes` ⇒ 503 when status != up; default ⇒ 200), proven by `TestClient(app)` tests
> that exercise the real status code. The end-to-end "a RUNNING degraded sidecar surfaces 503 on
> `?strict`, and knb `verify.sh` / monitoring adopt it" property is an aggregate/deploy
> characteristic with no in-repo live E2E surface (it requires a running core + ML sidecar + NATS
> stack + the out-of-repo knb `verify.sh`); its live evidence is the committed redteam observation
> (bug.md), and the fresh full-stack re-check + consumer adoption is owned by `bubbles.devops` as a
> non-gating operational step. This scope therefore declares no runtime-behavior multi-container E2E
> rows (v4.1.0 `contract-only` opt-out); every declared scenario maps to a concrete HTTP-contract
> asserting test.

### Gherkin Scenarios (Regression Tests)

```gherkin
Feature: the ML sidecar /health exposes a degraded state via an opt-in ?strict HTTP status (BUG-050-002)

  Scenario: A degraded sidecar surfaces 503 on the opt-in ?strict path
    Given the ML sidecar is degraded (NATS disconnected) so /health computes status:degraded
    When an operator calls GET /health?strict=true
    Then the sidecar returns HTTP 503 with body status:degraded (adversarial: fails against the pre-fix unconditional 200)

  Scenario: The ?strict opt-in accepts the same truthy variants as the Go contract
    Given the ML sidecar is degraded
    When an operator calls GET /health?strict=<v> for v in {1, true, TRUE, Yes, yes}
    Then every case-insensitive truthy variant yields HTTP 503

  Scenario: The DEFAULT /health stays 200 when degraded (container-liveness non-destabilization)
    Given the ML sidecar is degraded
    When the Docker HEALTHCHECK sends the DEFAULT GET /health (no ?strict param)
    Then the sidecar returns HTTP 200 with body status:degraded, so a degraded-but-alive sidecar is not restart-flapped

  Scenario: The ?strict opt-in stays 200 when the sidecar is up
    Given the ML sidecar is up (NATS connected)
    When an operator calls GET /health?strict=1
    Then the sidecar returns HTTP 200 with body status:up (no false alarm)
```

> The falsey/absent-`strict`-stays-200 path (`test_strict_falsey_and_absent_stay_200_when_degraded`)
> and the backward-compat dict-not-Response guard (`test_default_returns_plain_dict_not_response`) are
> preservation behaviors asserted as regression guards rather than as new Gherkin scenarios, keeping
> every declared scenario mapped to a concrete asserting adversarial/invariant test.

### Implementation Files

- `ml/app/main.py` — `/health` accepts a `strict` query param; the default (non-strict) path returns
  a plain dict (unconditional 200, byte-for-byte unchanged, subscriptable for in-process callers); the
  opt-in `?strict=true|1|yes` path returns a `JSONResponse` with `status_code=503` when the status is
  not `up`, else 200; the new `_health_strict_requested` helper parses the truthy variants mirroring
  the Go `healthStrictRequested`. Import adds `JSONResponse`.
- `ml/tests/test_health_strict_degraded.py` — the FastAPI-`TestClient` HTTP-layer regression suite
  (adversarial 503-on-degraded + truthy variants, strict-healthy-200, DEFAULT-degraded-stays-200
  non-destabilization, falsey/absent-stay-200, backward-compat dict-not-Response).

The Go `?strict` half (`internal/api/health.go` `healthStrictRequested` + the status-code selection in
`HealthHandler`, and the `internal/api/health_test.go` strict tests) is the earlier partial fix,
already committed at HEAD, verified READ-ONLY and NOT re-changed here. Both files this scope's fix
touches are runtime source (not spec/docs), satisfying the G053/G093 delivery-delta via the git-backed
proof in report.md → "Code Diff Evidence".

### Change Boundary

| Allowed file families | Excluded surfaces |
|-----------------------|-------------------|
| `ml/app/main.py` (the ML `/health` `?strict` opt-in) | any Go source (`internal/api/health.go`, `internal/api/health_test.go` — the earlier partial fix, verified READ-ONLY, NOT re-changed) |
| `ml/tests/test_health_strict_degraded.py` (new regression) | the sibling `BUG-050-001` bug folder and its tests (`ml/tests/test_health_model_loaded.py` — verified not regressed) |
| files in this bug directory (`specs/050-ml-sidecar-health-isolation/bugs/BUG-050-002-health-degraded-masked-http-200/`) | the parent spec 050 artifacts (`state.json`, `report.md`, `spec.md`, `design.md`, `scopes.md`) |
| — | the knb `<deployment-owner>/<product>/<target>/` verify-step + monitoring adoption (out-of-repo, routed to bubbles.devops) |
| — | the dev/prod `docker-compose*.yml` healthcheck-target alignment (routed to bubbles.devops) |
| — | `internal/config/release_trains_contract_test.go` (pre-existing, unrelated gofmt finding — outside this boundary) |
| — | any build, deploy, host mutation, or push; any unrelated spec / bug / deploy config |

### Test Plan

| ID | Scenario | Category | Location / Command Surface | Required Assertion |
|----|----------|----------|----------------------------|--------------------|
| `TP-1` | Strict degraded ⇒ 503 | `unit` adversarial | `ml/tests/test_health_strict_degraded.py::test_strict_degraded_returns_503` via `./smackerel.sh test unit --python` | `GET /health?strict=true` + degraded ⇒ `resp.status_code == 503`, body `status == "degraded"` |
| `TP-2` | Truthy variants ⇒ 503 | `unit` adversarial | `…::test_strict_truthy_variants_return_503` via `./smackerel.sh test unit --python` | `?strict` in {1,true,TRUE,Yes,yes} + degraded ⇒ all 503 |
| `TP-3` | Strict healthy ⇒ 200 | `unit` | `…::test_strict_healthy_returns_200` via `./smackerel.sh test unit --python` | `?strict=1` + up ⇒ `200`, body `status == "up"` |
| `TP-4` | Default degraded stays 200 | `unit` non-destabilization | `…::test_default_degraded_stays_200` via `./smackerel.sh test unit --python` | default `/health` + degraded ⇒ `200`, body still `status == "degraded"` |
| `TP-5` | Falsey/absent stays 200 | `unit` | `…::test_strict_falsey_and_absent_stay_200_when_degraded` via `./smackerel.sh test unit --python` | `?strict` in {"",false,0,no,maybe} + degraded ⇒ `200` |
| `TP-6` | Default returns plain dict (backward-compat) | `unit` | `…::test_default_returns_plain_dict_not_response` via `./smackerel.sh test unit --python` | `health()` ⇒ subscriptable `dict`, NOT a `JSONResponse` |
| `TP-7` | Adversarial regression quality | `guard` | `bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix ml/tests/test_health_strict_degraded.py` | adversarial signal detected, 0 violations, no silent-pass bailout |
| `TP-8` | Python unit lane integrity (no regression) | `unit` | `./smackerel.sh test unit --python` | `ml/tests` GREEN (`630 passed, 2 skipped`), exit 0 — includes BUG-050-001 `test_health_model_loaded.py` |
| `TP-9` | Go health lane integrity (no regression) | `unit` | `./smackerel.sh test unit --go --go-run 'TestHealthHandler|TestCheckMLSidecar|TestReadyz'` | `ok internal/api`, exit 0 — the Go `?strict` half + BUG-050-001 tests not regressed |
| `TP-10` | Live: RUNNING degraded sidecar surfaces 503 + consumer adoption | e2e (proof-of-record) | committed redteam observation (bug.md) + `bubbles.devops` full-stack re-check + out-of-repo knb verify-step adoption | after redeploy, `GET /health?strict=true` on a degraded RUNNING sidecar returns 503; the out-of-repo knb verify-step / monitoring adopt it (non-gating) |

**Test Plan ↔ DoD parity:** `TP-1`..`TP-10` map one-to-one to the scenario / preservation / lane /
regression / live DoD items below.

### Definition of Done

- [x] A degraded sidecar surfaces HTTP 503 on the opt-in `?strict` path — `GET /health?strict=true`
  returns HTTP 503 when degraded (SCN-001 / `TP-1`, adversarial), never the pre-fix masking 200.
  - Evidence: report.md → "Test Evidence" GREEN (`test_strict_degraded_returns_503` PASS) + "Code Diff Evidence" (`JSONResponse(status_code=… 503 …)` opt-in path in `ml/app/main.py`).
- [x] The `?strict` opt-in accepts the Go truthy variants (`1|true|yes`, case-insensitive) ⇒ 503 when
  degraded (SCN-002 / `TP-2`, adversarial).
  - Evidence: report.md → "Test Evidence" GREEN (`test_strict_truthy_variants_return_503` PASS) + "Code Diff Evidence" (`_health_strict_requested` truthy set).
- [x] `GET /health?strict=1` returns HTTP 200 when the sidecar is up (SCN-004 / `TP-3`) — no false
  alarm.
  - Evidence: report.md → "Test Evidence" GREEN (`test_strict_healthy_returns_200` PASS).
- [x] The DEFAULT `/health` stays HTTP 200 when degraded — the container-liveness non-destabilization
  invariant (SCN-003 / `TP-4`).
  - Evidence: report.md → "Test Evidence" GREEN (`test_default_degraded_stays_200` PASS) + "Code Diff Evidence" (default path returns a plain dict ⇒ FastAPI 200).
- [x] A non-truthy `strict` value (absent / `false` / `0` / garbage) stays HTTP 200 when degraded
  (`TP-5`).
  - Evidence: report.md → "Test Evidence" GREEN (`test_strict_falsey_and_absent_stay_200_when_degraded` PASS).
- [x] The default in-process `health()` returns a subscriptable dict, NOT a `Response` — backward-compat
  for the BUG-050-001 regressions + `test_main.py` (`TP-6`).
  - Evidence: report.md → "Test Evidence" GREEN (`test_default_returns_plain_dict_not_response` PASS).
- [x] Python unit lane proven FRESH this session — `./smackerel.sh test unit --python` GREEN with exit
  0 (`630 passed, 2 skipped`), the new suite AND the BUG-050-001 `test_health_model_loaded.py` suite
  included (`TP-8`).
  - Evidence: report.md → "Current-Session Re-Verification → Fresh Python Unit Lane" (`630 passed, 2 skipped`, `PY_GREEN_EXIT=0`).
- [x] Go health lane proven FRESH this session — `./smackerel.sh test unit --go --go-run
  'TestHealthHandler|TestCheckMLSidecar|TestReadyz'` GREEN with exit 0; the Go `?strict` half + the
  sibling BUG-050-001 tests are NOT regressed (`TP-9`).
  - Evidence: report.md → "Current-Session Re-Verification → Fresh Go Health Lane" (`ok internal/api`, `GO_HEALTH_EXIT=0`).
- [x] Pre-fix adversarial regression FAILS (RED) and post-fix PASSES (GREEN).
  - Evidence: report.md → "Scenario-First TDD — RED → GREEN Ordering" (Python RED `2 failed, 628 passed` with the fix absent → GREEN `630 passed` this session).
- [x] Adversarial regression contains no silent-pass bailout patterns and carries an adversarial
  signal (`TP-7`).
  - Evidence: report.md → "Fresh Adversarial Regression Guard" (adversarial signal detected, 0 violations / 0 warnings, `RQG_BUGFIX_EXIT=0`).
- [x] Root cause confirmed and documented (bug.md + design.md).
  - Evidence: report.md → "Root Cause" + design.md → "Root Cause Analysis" + the live redteam observation in bug.md.
- [x] The completion delta is present in the fix commit, git-verified; the Go `?strict` half is
  git-verified present at HEAD.
  - Evidence: report.md → "Code Diff Evidence" (`ml/app/main.py` `?strict` opt-in + `_health_strict_requested`; the Go `healthStrictRequested` present at HEAD).
- [x] Config integrity + lint clean (`./smackerel.sh check` exit 0, `./smackerel.sh lint` exit 0).
  - Evidence: report.md → "Current-Session Re-Verification → Fresh Check / Fresh Lint" (`CHECK_EXIT=0`, `LINT_EXIT=0`, "All checks passed!").
- [x] Change Boundary is respected and zero excluded file families were changed — the completion delta
  is ONLY `ml/app/main.py` + `ml/tests/test_health_strict_degraded.py`; the Go health surface + the
  sibling BUG-050-001 tests are verified read-only (NOT re-changed); the certification packet touches
  ONLY the BUG-050-002 bug folder.
  - Evidence: report.md → "Audit Evidence" (`git status --short` scoped; no excluded family changed) + "Code Diff Evidence".
- [x] Bug marked as Fixed in bug.md.
  - Evidence: bug.md Status line flipped to FIXED & VERIFIED at certification; state.json `status` = `done` and `certification.status` = `done`.
- [x] Live "operator/monitoring detects a degraded RUNNING sidecar via `GET /health?strict=true` ⇒
  503" outcome certified on the source fix + the HTTP-contract unit regressions; the fresh full-stack
  re-check + the knb `verify.sh` / monitoring adoption (and signed redeploy) is owned by
  `bubbles.devops` as a non-gating operational step (`TP-10`, `redeployRequired: true`).
  - Evidence: bug.md redteam observation (live degraded body with HTTP 200) + report.md → "Redeploy / Live-Verification Note" (mechanism unit-proven; running-stack re-check + consumer adoption routed to bubbles.devops non-gating).
- [x] Build Quality Gate: `check` and `lint` clean of any BUG-050-002 delta; the full eight-phase
  bugfix-fastlane pipeline recorded with fresh evidence.
  - Evidence: report.md → "Current-Session Re-Verification" (`CHECK_EXIT=0`, `LINT_EXIT=0`; `format --check` names ONLY the pre-existing unrelated `internal/config/release_trains_contract_test.go`, outside this boundary) + "Parent-Expanded Specialist Phase Evidence" (implement, test, regression, simplify, stabilize, security, validate, audit).
