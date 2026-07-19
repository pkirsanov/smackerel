# User Validation: BUG-050-002 — ML sidecar `/health` returns `degraded` with HTTP 200

**Closure status:** Fixed & Verified (ML sidecar `?strict` opt-in completed + certified this session; the Go `/api/health` `?strict` half was already committed at HEAD)

## User-facing impact

- **Operators / DevOps:** A degraded ML sidecar is now DETECTABLE by a status-code consumer. Calling
  `GET /health?strict=true` (or `?strict=1` / `?strict=yes`) on a degraded sidecar (e.g. NATS
  disconnected) returns HTTP 503; on a healthy sidecar it returns 200. This is the SAME opt-in the Go
  core `/api/health` already exposes, so one query param detects a degraded state on both surfaces.
  Before the fix, the ML `/health` returned `{"status":"degraded"}` with HTTP 200, so a blackbox
  monitor / `verify.sh` keying on the status code was blind to the degraded state.
- **Container liveness:** The DEFAULT `GET /health` (exactly what the Docker `HEALTHCHECK` sends) is
  byte-for-byte unchanged — it stays HTTP 200 even when degraded, so a degraded-but-alive sidecar is
  NOT marked unhealthy / restart-flapped. Liveness (200, for the container) and readiness
  (status-aware, opt-in, for the operator) are now cleanly separated.
- **In-process callers:** The default `health()` still returns a subscriptable JSON object, so the
  BUG-050-001 regressions (`test_health_model_loaded.py`), `test_main.py`, and `test_embedder.py` keep
  working unchanged (`630 passed`).
- **Auditors:** The health-masking finding (redteam F1) is closed at the runtime-source level across
  both health surfaces spec 050 owns. A degraded state is honestly exposed to a status-code consumer
  via the opt-in `?strict` channel, proven by real in-repo FastAPI-`TestClient` HTTP-layer unit tests.
- **End users:** Not applicable — this is an internal health/observability surface with no end-user UI.

## Acceptance

- AC-01..AC-14 from `spec.md` all pass; full evidence captured in `report.md`.
- The completion delta is scoped to `ml/app/main.py` + `ml/tests/test_health_strict_degraded.py`; the
  certification packet is scoped to the BUG-050-002 bug folder only; the Go health surface + the
  sibling BUG-050-001 tests are verified read-only and not re-changed.

## Sign-off

Completion + certification (bugfix-fastlane, parent-expanded by `bubbles.iterate`) terminates with the
bug `done` on disk, `state-transition-guard` passing at `done`, and the fresh full-stack `?strict`
re-check (with a signed redeploy) + the knb `verify.sh` / monitoring adoption of `?strict` routed to
`bubbles.devops` as a non-gating operational step. No further in-repo work is required for BUG-050-002.

## Checklist

- [x] `GET /health?strict=true` returns HTTP 503 when degraded (`test_strict_degraded_returns_503`, adversarial, GREEN).
- [x] The `?strict` opt-in accepts the Go truthy variants (`1|true|yes`, case-insensitive) ⇒ 503 when degraded (`test_strict_truthy_variants_return_503`, adversarial, GREEN).
- [x] `GET /health?strict=1` returns HTTP 200 when the sidecar is up (`test_strict_healthy_returns_200`, GREEN).
- [x] The DEFAULT `/health` stays HTTP 200 when degraded — container-liveness non-destabilization (`test_default_degraded_stays_200`, GREEN).
- [x] A non-truthy `strict` value stays HTTP 200 when degraded (`test_strict_falsey_and_absent_stay_200_when_degraded`, GREEN).
- [x] The default in-process `health()` returns a subscriptable dict, not a `Response` (`test_default_returns_plain_dict_not_response`, GREEN).
- [x] `./smackerel.sh test unit --python` GREEN (`630 passed, 2 skipped`, `PY_GREEN_EXIT=0`).
- [x] `./smackerel.sh test unit --go --go-run 'TestHealthHandler|TestCheckMLSidecar|TestReadyz'` GREEN (`ok internal/api`, `GO_HEALTH_EXIT=0`) — the Go `?strict` half + BUG-050-001 tests not regressed.
- [x] `regression-quality-guard.sh --bugfix ml/tests/test_health_strict_degraded.py` reports an adversarial signal, 0 violations (`RQG_BUGFIX_EXIT=0`).
- [x] `./smackerel.sh check` exit 0; `./smackerel.sh lint` exit 0.
- [x] The ML `?strict` opt-in is in the fix commit; the Go `healthStrictRequested` half is git-verified present at HEAD.
- [x] Zero excluded families re-changed by this packet; the Go health surface + BUG-050-001 tests verified read-only. `format --check` names only the pre-existing unrelated `internal/config/release_trains_contract_test.go` (outside this boundary).
- [x] Bug marked FIXED & VERIFIED in bug.md; `state.json` `status` = `done`, `certification.status` = `done`.
- [x] Live "operator/monitoring detects a degraded RUNNING sidecar via `GET /health?strict=true` ⇒ 503" re-check (rebuild + signed redeploy + full-stack `?strict` + knb `verify.sh` adoption) routed to `bubbles.devops` as a non-gating operational step (`redeployRequired: true`).
