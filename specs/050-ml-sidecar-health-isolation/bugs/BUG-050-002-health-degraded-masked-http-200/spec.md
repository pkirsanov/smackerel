# Spec: BUG-050-002 — ML sidecar `/health` returns `degraded` with HTTP 200 (status-code consumers blind)

**Parent Spec:** 050-ml-sidecar-health-isolation
**Discovered:** 2026-07-08 (redteam adversarial interrogation of the LIVE smackerel prod deployment on `<deploy-host>`, finding F1)
**Mode:** bugfix-fastlane (Go `/api/health` half committed at HEAD; ML sidecar `/health` half completed + certified this session)
**Release Train:** mvp

## Use Cases

- **UC-01 — A degraded ML sidecar is visible to a status-code consumer.** The ML sidecar's
  `GET /health` computes an aggregate `status` of `up` / `degraded` (e.g. `degraded` when NATS is
  disconnected). An operator / monitoring caller MUST be able to detect the `degraded` state via the
  HTTP status code, not only by parsing the JSON body — otherwise every status-code-only consumer
  (the Docker `HEALTHCHECK`, `verify.sh`, a blackbox monitor) is blind to a degraded sidecar.

- **UC-02 — The container liveness contract is preserved.** The DEFAULT `GET /health` (no query
  param — exactly what the Docker `HEALTHCHECK`'s `urllib.request.urlopen('.../health')` sends, which
  RAISES on ANY non-2xx) MUST stay HTTP 200 even when the sidecar is `degraded`. A `degraded` process
  is still ALIVE; returning 503 on the default path would mark the container unhealthy and
  flap/restart it. Liveness (200, for the container) and readiness (status-aware, for the operator)
  MUST NOT be conflated.

- **UC-03 — One opt-in semantics across both health surfaces spec 050 owns.** The ML sidecar's
  opt-in MUST mirror the Go core `/api/health` opt-in already shipped for this bug
  (`healthStrictRequested` in `internal/api/health.go`): `?strict=true|1|yes` (case-insensitive)
  returns 503 when the aggregate status is not `up`. An operator uses the SAME query param on both
  surfaces.

- **UC-04 — Backward compatibility for in-process callers.** The default (non-strict) `health()`
  MUST keep returning a subscriptable JSON object so existing in-process callers/tests
  (`test_health_model_loaded.py`, `test_main.py`, `test_embedder.py` — the BUG-050-001 regressions)
  that index `health()["status"]` / `["model_loaded"]` keep working unchanged.

## Functional Requirements

- **FR-01 — Opt-in readiness signal on the ML `/health`.** `ml/app/main.py`'s `/health` handler MUST
  accept a `strict` query parameter. When the caller opts in (`?strict=true|1|yes`,
  case-insensitive) AND the aggregate `status` is not `up`, the handler MUST return HTTP 503. When
  strict + `up`, it MUST return HTTP 200.

- **FR-02 — Default path is byte-for-byte unchanged (liveness).** The DEFAULT `GET /health` (no
  `strict`, or a non-truthy `strict` value) MUST return the SAME plain JSON body with an
  unconditional HTTP 200, even when the status is `degraded`.

- **FR-03 — Body shape preserved.** The response body MUST keep the existing
  `{status, nats, model_loaded}` shape in ALL cases (strict/default, up/degraded). Only the HTTP
  status code differs on the opt-in path.

- **FR-04 — Truthy-variant parity with the Go contract.** The `strict` opt-in MUST accept the same
  truthy variants the Go `healthStrictRequested` accepts (`1`, `true`, `yes`, case-insensitive) and
  treat every other value (including absent / `false` / `0`) as the default liveness path.

- **FR-05 — Automated unit regressions (no running stack).** The behavior MUST be proven by
  FastAPI-`TestClient` HTTP-layer unit regressions: an adversarial `?strict=true`-degraded-returns-503
  test (RED against the pre-fix unconditional 200), a strict-healthy-returns-200 test, a
  DEFAULT-degraded-stays-200 non-destabilization test, truthy/falsey variant coverage, and a
  body-shape / backward-compat guard.

## Acceptance Criteria

- **AC-01** — ML `/health?strict=true` returns HTTP 503 when degraded (NATS disconnected)
  (`test_strict_degraded_returns_503`, adversarial) — FAILS against the pre-fix unconditional 200,
  passes with the fix.
- **AC-02** — ML `/health?strict={1,true,TRUE,Yes,yes}` all return 503 when degraded
  (`test_strict_truthy_variants_return_503`, adversarial).
- **AC-03** — ML `/health?strict=1` returns HTTP 200 when the sidecar is up
  (`test_strict_healthy_returns_200`).
- **AC-04** — The DEFAULT ML `/health` (no `strict`) stays HTTP 200 when degraded — the
  container-liveness non-destabilization invariant (`test_default_degraded_stays_200`).
- **AC-05** — A non-truthy `strict` value (absent / `false` / `0` / garbage) stays HTTP 200 when
  degraded (`test_strict_falsey_and_absent_stay_200_when_degraded`).
- **AC-06** — The default in-process `health()` returns a subscriptable dict (not a `Response`), so
  the BUG-050-001 regressions and `test_main.py` keep working
  (`test_default_returns_plain_dict_not_response`).
- **AC-07** — `./smackerel.sh test unit --python` passes GREEN with exit 0 (the new
  `test_health_strict_degraded.py` suite AND the BUG-050-001 `test_health_model_loaded.py` suite
  included — no regression).
- **AC-08** — `./smackerel.sh test unit --go --go-run 'TestHealthHandler|TestCheckMLSidecar|TestReadyz'`
  passes GREEN with exit 0 — the sibling BUG-050-001 Go health tests and the already-committed Go
  `?strict` half are not regressed.
- **AC-09** — `bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix
  ml/tests/test_health_strict_degraded.py` reports an adversarial signal with 0 violations (exit 0).
- **AC-10** — `./smackerel.sh check` exits 0 and `./smackerel.sh lint` exits 0 (no finding on the
  changed `ml/app/main.py` or the new test).
- **AC-11** — The fix is present in the working tree / fix commit: `ml/app/main.py`'s `/health`
  accepts `strict` and returns 503 on the opt-in degraded path (Code Diff Evidence cites the fix
  commit). The Go `?strict` half (`healthStrictRequested` in `internal/api/health.go`) is
  git-verified present at HEAD (the earlier partial fix, read-only).
- **AC-12** — `bash .github/bubbles/scripts/state-transition-guard.sh <bug-dir>` returns exit 0 at
  `done`; `bash .github/bubbles/scripts/artifact-lint.sh <bug-dir>` returns PASSED.
- **AC-13** — The completion delta touches ONLY `ml/app/main.py` +
  `ml/tests/test_health_strict_degraded.py`; the certification packet touches ONLY the BUG-050-002
  bug folder. The Go health surface + the sibling BUG-050-001 tests are verified read-only and NOT
  re-changed.
- **AC-14** — The live "operator/monitoring detects a degraded RUNNING sidecar via
  `GET /health?strict=true` → 503" outcome is certified on the redteam live proof-of-record + the
  unit-proven mechanism; the consumer adoption (knb `verify.sh` / monitoring parsing `?strict`) and
  the fresh full-stack re-run against a running core + ML sidecar + a signed redeploy are routed to
  `bubbles.devops` as a NON-GATING operational step (`redeployRequired: true`).

### Single-Capability Justification

This bug completes a SINGLE existing behavior — the truthfulness of the HEALTH SIGNAL a `degraded`
health surface exposes to a status-code consumer — across the two health endpoints spec 050 owns:
the Go core `/api/health` (opt-in already shipped) and the ML sidecar `/health` (completed here). It
introduces NO new capability, NO new provider/adapter/strategy, and NO second implementation or
variant. `/health` remains the single ML-sidecar health endpoint; the `_health_strict_requested`
helper is a single call-time parse of one query parameter mirroring the existing Go
`healthStrictRequested`. No capability-foundation / concrete-implementations split applies, so no
Domain Capability Model is required. The `connectors` token, where it appears, refers only to the
disconnected-connector aggregate driver already documented in the parent finding and is not a new
capability here.
