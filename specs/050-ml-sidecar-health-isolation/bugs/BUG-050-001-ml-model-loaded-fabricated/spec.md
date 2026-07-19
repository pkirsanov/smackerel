# Spec: BUG-050-001 — Core `/api/health` fabricates ML sidecar `model_loaded:true` (never parses the ML `/health` body)

**Parent Spec:** 050-ml-sidecar-health-isolation
**Discovered:** 2026-07-08 (redteam adversarial interrogation of the LIVE smackerel prod deployment on `<deploy-host>`, findings F7 core + F8 sidecar)
**Mode:** bugfix-fastlane (real source fix already committed; reconcile + certify)
**Release Train:** mvp

## Use Cases

- **UC-01 — Core reflects the ML sidecar's real `model_loaded`.** When core's authenticated
  `/api/health` reports `services.ml_sidecar`, the `model_loaded` value MUST be the value the ML
  sidecar actually reports from its own `GET /health` body — never a value core invents. On the live
  prod deployment the sidecar reported `{"status":"up","model_loaded":false}` while core claimed
  `model_loaded:true`; core MUST NOT contradict the sidecar's own self-report (anti-fabrication /
  G021 in the runtime health surface itself).

- **UC-02 — A self-declared degraded sidecar surfaces.** When the ML sidecar reports its own status
  as `degraded` (or any non-`up` value) in its `/health` body, core MUST reflect that status into the
  aggregate rather than overriding it to `up`, so an operator sees the sidecar's real condition.

- **UC-03 — Reachable-but-unknown is honest, not fabricated.** When the ML sidecar returns 200 with
  an empty or unparseable body, core MUST report the sidecar as reachable (`up`) WITHOUT attaching a
  fabricated `model_loaded` claim — reachable-but-unknown is a truthful state, an invented boolean is
  not.

- **UC-04 — The ML sidecar's own `/health` is truthful over time.** The ML sidecar's `GET /health`
  MUST report the embedder's CURRENT model-loaded state at call time. The embedding model is loaded
  lazily on the first `generate_embedding()`, long after the FastAPI module is imported, so a
  snapshot captured at import time is permanently wrong. `/health` MUST read the live embedder state
  so that once the model is loaded (and `POST /embed` returns 200) `model_loaded` becomes `true`.

## Functional Requirements

- **FR-01 — Core decodes the ML `/health` body.** `checkMLSidecar` in `internal/api/health.go` MUST
  issue its own bounded `GET <ml>/health` and DECODE the JSON body
  (`{"status": string, "model_loaded": bool}`) instead of reusing the boolean-only `probeHTTPGet`
  helper that drains and discards the body.

- **FR-02 — Reflect the sidecar's `status` and `model_loaded`.** On a 200 with a parseable body, core
  MUST take `Status` and `ModelLoaded` from the decoded body (a missing/empty `status` defaults to
  `up`). It MUST NOT hardcode `Status:"up"` + `ModelLoaded:true`.

- **FR-03 — Non-200 / transport error → `down` (unchanged).** A non-200 response or a transport error
  MUST continue to report the sidecar as `down`.

- **FR-04 — Empty/unparseable 200 body → `up` with no `model_loaded` claim.** A 200 whose body cannot
  be decoded MUST report `up` with `ModelLoaded == nil` (reachable-but-unknown), never a fabricated
  boolean.

- **FR-05 — The ML sidecar reports live `model_loaded`.** `ml/app/main.py`'s `/health` MUST call
  `embedder.is_model_loaded()` (a call-time read of the module global `embedder._model`) rather than
  binding to an import-time `from .embedder import _model` snapshot that freezes at `None` forever.

- **FR-06 — Automated unit regressions.** The behavior MUST be proven by unit regressions without a
  running stack: a core adversarial test that reflects `model_loaded:false`, a degraded-body test, a
  no-fabrication-on-unparseable-body test, and ML-sidecar stale-binding tests that flip `model_loaded`
  to `true` after a real lazy load.

## Acceptance Criteria

- **AC-01** — Core `checkMLSidecar` reflects `model_loaded:false` from the sidecar body
  (`TestCheckMLSidecar_ModelNotLoaded`, adversarial) — fails against the pre-fix `loaded := true`
  hardcode, passes with the fix.
- **AC-02** — Core reflects a self-declared `degraded` sidecar status
  (`TestCheckMLSidecar_DegradedBody`).
- **AC-03** — Core reports `up` with NO `model_loaded` claim on a reachable-but-unparseable body
  (`TestCheckMLSidecar_ReachableUnparseableBody`).
- **AC-04** — The ML sidecar `/health` reports `model_loaded:true` after a real lazy load, not the
  frozen import-time snapshot (`test_health_model_loaded_tracks_embedder_not_stale_import` +
  `test_health_model_loaded_true_after_generate_embedding`, adversarial).
- **AC-05** — The healthy path is preserved: a `{"status":"up","model_loaded":true}` body still
  yields `up` + `model_loaded:true` (`TestCheckMLSidecar_HealthyResponse`); non-200 / unreachable
  still yields `down` (`TestCheckMLSidecar_UnhealthyResponse`, `TestCheckMLSidecar_ConnectionRefused`).
- **AC-06** — `./smackerel.sh test unit --go --go-run 'TestCheckMLSidecar|TestHealthHandler_MLSidecar'`
  passes GREEN with exit 0.
- **AC-07** — `./smackerel.sh test unit --python` passes GREEN (the ML `test_health_model_loaded.py`
  suite included) with exit 0.
- **AC-08** — `bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix
  internal/api/health_test.go` reports an adversarial signal with 0 violations (exit 0).
- **AC-09** — `./smackerel.sh check` exits 0 and `./smackerel.sh lint` exits 0 (no finding on the
  changed `internal/api/health.go` or the ML sources).
- **AC-10** — The fix is present at HEAD: `checkMLSidecar` decodes the body and `ml/app/main.py`
  `/health` calls `embedder.is_model_loaded()` (Code Diff Evidence cites fix commit `f26dbdd9`).
- **AC-11** — `bash .github/bubbles/scripts/state-transition-guard.sh <bug-dir>` returns exit 0 at
  `done`; `bash .github/bubbles/scripts/artifact-lint.sh <bug-dir>` returns PASSED.
- **AC-12** — The certification packet touches ONLY paths under the BUG-050-001 bug folder; the
  source fix is verified read-only and NOT re-changed.
- **AC-13** — The live "core `/api/health` reflects the RUNNING ML sidecar's real `model_loaded`"
  outcome is certified on the committed live redteam proof-of-record + the unit-proven mechanism; the
  fresh live re-run against a running core + ML sidecar + ollama stack (and, if desired, a signed
  redeploy) is routed to `bubbles.devops` as a NON-GATING operational step (`redeployRequired: true`).

### Single-Capability Justification

This bug fixes a SINGLE existing behavior — the truthfulness of the `model_loaded` value the ML-health
surface reports — across the two tiers that ALREADY exist (the ML sidecar's `/health` self-report and
core's `checkMLSidecar` aggregate). It introduces NO new capability, NO new provider/adapter/strategy,
and NO second implementation or variant. `checkMLSidecar` remains the single ML-sidecar probe (the
separate Ollama liveness probe is unchanged and out of scope), and `is_model_loaded()` is a single
call-time read of the one embedder module global. The `connectors` token appears ONLY inside the
quoted fix-commit subject (`f26dbdd9`), which also carried the sibling BUG-050-002 connector-degraded
fix; that is a separate bug outside this bug's change boundary. No capability-foundation /
concrete-implementations split applies here, so no Domain Capability Model is required.
