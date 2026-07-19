# User Validation: BUG-050-001 — Core `/api/health` fabricates ML sidecar `model_loaded:true`

**Closure status:** Fixed & Verified (source fix committed in `f26dbdd9`; reconciled + certified this session)

## User-facing impact

- **Operators / DevOps:** Core's authenticated `/api/health` now tells the TRUTH about the ML
  sidecar. `services.ml_sidecar.model_loaded` mirrors what the sidecar itself reports from its own
  `/health` — `false` when the embedding model has not loaded yet, `true` once it has, and no
  `model_loaded` claim at all when the sidecar is reachable but its body is unreadable. Before the
  fix, core hardcoded `model_loaded:true` on any `200`, so a sidecar whose model had not loaded was
  actively misreported as ready. A self-declared `degraded` sidecar now surfaces into the aggregate
  instead of being overridden to `up`.
- **ML sidecar:** The sidecar's own `/health` now reports the embedder's CURRENT state via a call-time
  `is_model_loaded()` read, so once the model lazily loads on the first `generate_embedding()` the
  endpoint flips to `model_loaded:true` — instead of the previous behavior where a
  `from .embedder import _model` value binding froze it at `false` forever.
- **Auditors:** The ML-health anti-fabrication finding (redteam F7 core + F8 sidecar) is closed at the
  runtime-source level. The `model_loaded` value is honest end to end (sidecar → core → operator) and
  is proven by real in-repo unit tests on both surfaces (Go `internal/api` + Python `ml/tests`).
- **End users:** Not applicable — this is an internal health/observability surface with no end-user UI.

## Acceptance

- AC-01..AC-13 from `spec.md` all pass; full evidence captured in `report.md`.
- The certification packet is scoped to the BUG-050-001 bug folder only; the source fix is verified
  read-only and not re-changed.

## Sign-off

Reconcile + certification (bugfix-fastlane, parent-expanded by `bubbles.iterate`) terminates with the
bug `done` on disk, `state-transition-guard` passing at `done`, and the fresh full-stack `/api/health`
re-check (with a signed redeploy) routed to `bubbles.devops` as a non-gating operational step. No
further in-repo follow-up work is required for BUG-050-001.

## Checklist

- [x] Core reflects `model_loaded:false` from the sidecar body (`TestCheckMLSidecar_ModelNotLoaded`, adversarial, GREEN).
- [x] Core reflects a self-declared `degraded` sidecar status (`TestCheckMLSidecar_DegradedBody`, GREEN).
- [x] Core reports `up` with NO `model_loaded` claim on a reachable-but-unparseable body (`TestCheckMLSidecar_ReachableUnparseableBody`, GREEN).
- [x] The ML sidecar `/health` reports the embedder's live `model_loaded` (`test_health_model_loaded.py`, adversarial, GREEN).
- [x] Healthy / down paths preserved (`TestCheckMLSidecar_HealthyResponse` / `_UnhealthyResponse` / `_ConnectionRefused`, GREEN).
- [x] `./smackerel.sh test unit --go --go-run 'TestCheckMLSidecar|TestHealthHandler_MLSidecar'` GREEN (`UNIT_GO_EXIT=0`).
- [x] `./smackerel.sh test unit --python` GREEN (`622 passed, 2 skipped`, `UNIT_PY_EXIT=0`).
- [x] `regression-quality-guard.sh --bugfix internal/api/health_test.go` reports an adversarial signal, 0 violations (`RQG_EXIT=0`).
- [x] `./smackerel.sh check` exit 0; `./smackerel.sh lint` exit 0.
- [x] The body-decode arm + `is_model_loaded()` call-time read are present at HEAD (`f26dbdd9`); the five fix files are git-verified.
- [x] Zero source files re-changed by this packet; the fix is verified read-only. `format --check` names only the pre-existing unrelated `internal/config/release_trains_contract_test.go` (outside this boundary).
- [x] Bug marked FIXED & VERIFIED in bug.md; `state.json` `status` = `done`, `certification.status` = `done`.
- [x] Live "core `/api/health` reflects the running ML sidecar's real `model_loaded`" re-check (rebuild + signed redeploy + full-stack `/api/health`) routed to `bubbles.devops` as a non-gating operational step (`redeployRequired: true`).
