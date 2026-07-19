# Design: BUG-050-001 — Core `/api/health` fabricates ML sidecar `model_loaded:true`

## Current Truth (Phase 0.55 — solution-blind provenance probe)

**HEAD SHA:** `d8871e2728d0` (current at reconcile time 2026-07-19)
**Fix commit:** `f26dbdd9` ("fix(health,ml): clear false 'degraded' from unconfigured connectors; fix ML model_loaded stale-binding")
**Probed surface:** `internal/api/health.go` (`checkMLSidecar` + the `probeHTTPGet` helper it used to
reuse), `internal/api/health_test.go`, `ml/app/main.py` (`/health`), `ml/app/embedder.py`
(`is_model_loaded()`), and `ml/tests/test_health_model_loaded.py`.

### Findings — the defect and its blast radius

- **Core side (redteam F7).** `checkMLSidecar` reused the boolean-only `probeHTTPGet` helper (built
  for the Ollama liveness probe, which has no body contract) and then HARDCODED `Status:"up"` +
  `ModelLoaded:true` on any 200. `probeHTTPGet` drains and discards the body
  (`io.Copy(io.Discard, resp.Body)`), so the sidecar's `{"status","model_loaded"}` fields were
  structurally unreadable by this path. Core therefore ACTIVELY MISREPORTED the ML model as loaded:
  on the live prod deployment (`<deploy-host>`) the ML sidecar's own `/health` returned
  `{"status":"up","model_loaded":false}`, yet core's authenticated `/api/health` claimed
  `services.ml_sidecar = {"status":"up","model_loaded":true}` — a fabrication in the runtime health
  surface itself.
- **Sidecar side (redteam F8).** `ml/app/main.py` did `from .embedder import _model` and returned
  `"model_loaded": _model is not None`. A `from X import name` binds the importing module to the
  value AT IMPORT TIME (`None`), and that binding is NOT re-evaluated when `embedder._model` is later
  reassigned by the lazy load in `embedder._load_model()` (invoked on the first
  `generate_embedding()`). So even a sidecar whose model WAS loaded (and whose `POST /embed`
  returned 200) reported `model_loaded:false` PERMANENTLY. The two findings compound: an already-wrong
  sidecar self-report AND a core layer that discards it and invents its own value.

### Findings — the committed fix (present at HEAD)

`internal/api/health.go` — `checkMLSidecar` now issues its own bounded GET and REFLECTS the body:

```go
type mlHealthBody struct {
    Status      string `json:"status"`
    ModelLoaded bool   `json:"model_loaded"`
}
// … non-200 / transport error → down (unchanged) …
var body mlHealthBody
if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
    return ServiceStatus{Status: "up"} // reachable-but-unknown, no fabricated claim
}
status := body.Status
if status == "" {
    status = "up"
}
loaded := body.ModelLoaded
return ServiceStatus{Status: status, ModelLoaded: &loaded}
```

`ml/app/embedder.py` — a call-time reader; `ml/app/main.py` `/health` now uses it:

```python
def is_model_loaded() -> bool:
    """Report whether the embedding model is CURRENTLY loaded (reads the module
    global _model at CALL time, so /health never freezes on the import-time None)."""
    return _model is not None
# main.py: "model_loaded": is_model_loaded()
```

**Conclusion:** the fix is a runtime source change, already committed in `f26dbdd9`, present at HEAD,
and proven by real in-repo unit tests (Go `internal/api` + Python `ml/tests`). The reconcile path is:
verify the source fix read-only, re-run the Go and Python unit lanes FRESH this session, author the
full bugfix-fastlane packet, and certify to `done` — with the RUNNING-stack end-to-end re-check
routed to `bubbles.devops` as non-gating.

## Root Cause Analysis

Two independent truthfulness defects in the ML-health chain. (1) Core's `checkMLSidecar` treated the
ML sidecar like a bare liveness endpoint: it reused a helper that only distinguishes 200 from not-200
and then invented `model_loaded:true`, so the value core published bore no relationship to the
sidecar's actual state. (2) The ML sidecar's own `/health` froze `model_loaded` at the import-time
`None` because of a `from .embedder import _model` value binding, so it could never report the model
as loaded even after a successful lazy load. End to end, `model_loaded` was doubly untrustworthy: the
sidecar under-reported it and core over-reported it.

## Design Decisions

### DD-1 — Adopt the sibling bugfix-fastlane packet structure

Mirror the 8-artifact bugfix-fastlane layout used by the certified sibling `BUG-029-008` (a real
source-fix reconcile in this same session): `bug.md`, `spec.md`, `design.md`, `scopes.md`,
`scenario-manifest.json`, `report.md`, `state.json`, `uservalidation.md`. Use the `## Scope N: <Name>`
colon-format for the traceability guard's DoD fidelity.

### DD-2 — Fix core by REFLECTING, not by inventing

The right contract for a service whose `/health` carries a body is to decode the body and reflect it.
`checkMLSidecar` stops reusing `probeHTTPGet` (which is correct only for body-less liveness probes
like Ollama) and instead performs its own bounded GET + `json.Decode`. It maps: non-200/transport →
`down`; 200 + parseable → the body's `status` + `model_loaded`; 200 + unparseable → `up` with no
claim. This is the smallest change that makes the aggregate truthful without touching the Ollama probe
or the rest of the health handler.

### DD-3 — Anti-fabrication over convenience: no claim beats a guessed claim

On a reachable-but-unparseable body core reports `up` with `ModelLoaded == nil` rather than defaulting
to `true` (the old behavior) or `false` (a different fabrication). `ModelLoaded` is a `*bool` with
`json:"model_loaded,omitempty"`, so a nil claim is simply omitted from the aggregate — reachable but
unknown, honestly represented. This is the direct G021 (anti-fabrication) fix for the runtime surface.

### DD-4 — Sidecar reports live state via a call-time helper

`ml/app/embedder.py` gains `is_model_loaded()` — a one-line `return _model is not None` that reads the
module global at CALL time. `ml/app/main.py` imports the FUNCTION (not the value) and calls it in
`/health`. A function call re-reads the live global on every request, so once the lazy load reassigns
`embedder._model` the `/health` value flips to `true` — unlike the `from .embedder import _model`
value binding, which captured `None` at import time forever.

### DD-5 — Adversarial proof (the tests detect a regression)

Both surfaces carry adversarial regressions. Core:
`TestCheckMLSidecar_ModelNotLoaded` sends `{"status":"up","model_loaded":false}` and asserts core
reflects `false`; against the pre-fix hardcode (`loaded := true`) it FAILS. Sidecar:
`test_health_model_loaded_tracks_embedder_not_stale_import` loads the model AFTER importing `app.main`
and asserts `/health` reports `true`; against the pre-fix `from .embedder import _model` binding
(main's local `_model` frozen at `None`) it FAILS. These are the RED captures paired with the
current-session GREEN runs.

### DD-6 — Certification boundary: source verified read-only, packet is spec-only

The effective source fix is already committed (`f26dbdd9`). This reconcile packet verifies the fix
read-only and does NOT re-change any source file. All packet mutations land under the BUG-050-001 bug
folder. The G093 delivery-implementation-delta requirement is satisfied by the G053-compatible Code
Diff Evidence in `report.md`, which cites the real non-spec delivery files (`internal/api/health.go`,
`internal/api/health_test.go`, `ml/app/embedder.py`, `ml/app/main.py`,
`ml/tests/test_health_model_loaded.py`) shipped in `f26dbdd9`.

### DD-7 — Running-stack end-to-end re-check is a NON-GATING devops step

The source fix is baked into the built core + ML images. The already-running prod images keep the old
behavior until they are rebuilt (`./smackerel.sh config generate` + image build) and redeployed. A
fresh end-to-end confirmation — bring up a real core + ML sidecar + ollama stack, load the model, and
assert core's `/api/health` `services.ml_sidecar.model_loaded` mirrors the sidecar's own `/health` —
is routed to `bubbles.devops` as a non-gating operational step (`redeployRequired: true`). It does NOT
block certification: the mechanism is source-committed and unit-proven end to end (Go + Python), the
live mismatch is already committed as a redteam measurement, and the redeploy is an operational apply,
not a code change.

### Single-Implementation Justification

BUG-050-001 has exactly ONE implementation per tier and introduces no provider/adapter/strategy/variant
axis. Core's `checkMLSidecar` is the single ML-sidecar health probe; the fix rewrites that one function
to decode + reflect the sidecar body (the body-less `probeHTTPGet` helper it stopped reusing is
retained UNCHANGED for the Ollama liveness probe, which is a distinct, out-of-scope probe). The ML
sidecar's `is_model_loaded()` is a single call-time read of the one `embedder._model` module global.
There is no second implementation, no pluggable driver/channel, and no variation axis to model, so a
Capability Foundation / Concrete Implementations / Variation Axes split does not apply. The
`connectors` token in the quoted `f26dbdd9` subject belongs to the sibling BUG-050-002 (a separate
bug, outside this change boundary); it does not introduce a capability foundation here.

## Affected Files

**Delivery (already committed in `f26dbdd9`, verified read-only — NOT re-changed here):**

- `internal/api/health.go` — new `mlHealthBody` type; `checkMLSidecar` decodes the body and reflects
  `status` + `model_loaded` (+22 in `f26dbdd9`).
- `internal/api/health_test.go` — updated `TestCheckMLSidecar_HealthyResponse` (the old test encoded
  the bug: it asserted `ModelLoaded:true` on an empty body) + new `TestCheckMLSidecar_ModelNotLoaded`
  (adversarial), `TestCheckMLSidecar_DegradedBody`, `TestCheckMLSidecar_ReachableUnparseableBody`
  (+106 in `f26dbdd9`).
- `ml/app/embedder.py` — `is_model_loaded()` call-time reader (+16 in `f26dbdd9`).
- `ml/app/main.py` — `/health` calls `is_model_loaded()` (+7 in `f26dbdd9`).
- `ml/tests/test_health_model_loaded.py` — the F8 stale-binding regression suite (+91 in `f26dbdd9`).

**Certification packet (this reconcile):**

- `specs/050-ml-sidecar-health-isolation/bugs/BUG-050-001-ml-model-loaded-fabricated/` — all 8
  artifacts.

## Rollback

Pure git revert of the certification commits restores the prior `fixed_in_repo` stuck status; zero
runtime impact (the source fix in `f26dbdd9` is independent of this packet and is not reverted).
