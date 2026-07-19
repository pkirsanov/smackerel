# Design: BUG-050-002 — ML sidecar `/health` returns `degraded` with HTTP 200

## Current Truth (Phase 0.55 — solution-blind provenance probe)

**HEAD SHA (at completion time 2026-07-19):** `040fcbcd` (before the BUG-050-002 fix commit)
**Prior partial fix (Go half):** committed at HEAD in `internal/api/health.go` — `healthStrictRequested(r)`
+ the status-code selection at the `writeJSON` site of `HealthHandler` (opt-in `?strict=true` → 503
when the Go aggregate is not `healthy`; default 200 unchanged).
**Probed surface:** `ml/app/main.py` (`/health`), `internal/api/health.go` (`HealthHandler` +
`healthStrictRequested`), `internal/api/health_test.go` (the Go `?strict` tests), the ML sidecar
Docker `HEALTHCHECK` in `docker-compose.yml`, and the existing ML health tests
(`ml/tests/test_health_model_loaded.py`, `ml/tests/test_main.py`, `ml/tests/test_embedder.py`).

### Findings — the defect and its blast radius

- **The masking (redteam F1).** `ml/app/main.py`'s `/health` computed
  `"status": "up" if nats_connected else "degraded"` and returned that dict directly. FastAPI
  serialises a returned dict with an UNCONDITIONAL HTTP 200, so a `degraded` sidecar (NATS
  disconnected) reported `{"status":"degraded", …}` with **HTTP 200**. Every status-code-only
  consumer was therefore blind to the degraded state: the Docker `HEALTHCHECK`
  (`docker-compose.yml`: `python -c "import urllib.request; urllib.request.urlopen('.../health')"`)
  only distinguishes 2xx from an exception, a blackbox monitor keying on `200` sees healthy, and the
  knb `verify.sh` (out of this repo) keys on the status code. This is the ML-sidecar analogue of the
  Go core `/api/health` masking the same finding F1 already fixed on the Go side.

- **Why a naive 503-on-degraded is WRONG (the destabilization trap).** The ML `/health` is the
  container liveness probe. The Docker `HEALTHCHECK` calls `urllib.request.urlopen('.../health')`,
  which RAISES `HTTPError` on ANY non-2xx. If the default `/health` returned 503 whenever the sidecar
  was `degraded`, the Docker healthcheck would mark a still-ALIVE-but-degraded sidecar UNHEALTHY and
  flap/restart it — a self-inflicted outage. A `degraded` process (e.g. NATS reconnecting) is still
  alive and must keep serving. Liveness (200, for the container) and readiness (status-aware, for the
  operator) are DIFFERENT contracts and must not be conflated.

- **The already-shipped Go half sets the pattern.** The Go core `/api/health` fixed exactly this by
  adding an OPT-IN `?strict=true` that returns 503 only when the operator asks, while the default
  (what the Docker healthcheck sends) stays 200. The ML sidecar was left masking because that partial
  fix only touched the Go surface; this bug's owning spec is the ML sidecar, so the ML `/health` is
  the natural completion.

### Findings — the completion (this session)

`ml/app/main.py` — `/health` gains an opt-in `strict` query parameter and a shared parse helper:

```python
@app.get("/health")
async def health(strict: str = ""):
    nats_connected = nats_client is not None and nats_client.is_connected
    status = "up" if nats_connected else "degraded"
    body = {"status": status, "nats": "connected" if nats_connected else "disconnected",
            "model_loaded": is_model_loaded()}
    if not _health_strict_requested(strict):
        return body                       # DEFAULT: plain dict ⇒ unconditional 200 (liveness)
    return JSONResponse(content=body, status_code=200 if status == "up" else 503)  # opt-in readiness

def _health_strict_requested(strict: str) -> bool:
    return strict.strip().lower() in {"1", "true", "yes"}  # mirrors Go healthStrictRequested
```

**Conclusion:** the ML-sidecar completion is a small, backward-compatible change confined to the
`/health` handler. The default liveness path is byte-for-byte unchanged (a plain dict ⇒ 200) and
in-process callers keep getting a subscriptable dict; only the explicit `?strict` opt-in adds a
status-aware 503. It is proven by FastAPI-`TestClient` HTTP-layer unit regressions without a running
stack.

## Root Cause Analysis

`/health` conflated liveness with readiness and resolved the conflict by always returning HTTP 200:
it computed a truthful `degraded` in the body but discarded that signal at the HTTP layer because a
returned dict is always 200 in FastAPI. There was no status-aware channel any status-code consumer
(Docker `HEALTHCHECK`, `verify.sh`, monitoring) could read, so a degraded-but-alive sidecar was
indistinguishable from a healthy one to every automated consumer. The fix adds the missing
readiness channel WITHOUT touching the liveness channel: an opt-in `?strict` that returns 503 on
`degraded`, leaving the default (what the container healthcheck sends) at 200.

## Design Decisions

### DD-1 — Adopt the sibling bugfix-fastlane packet structure

Mirror the 8-artifact bugfix-fastlane layout used by the certified sibling `BUG-050-001` in this same
parent spec (`bug.md`, `spec.md`, `design.md`, `scopes.md`, `scenario-manifest.json`, `report.md`,
`state.json`, `uservalidation.md`), with the `## Scope N: <Name>` colon-format for the traceability
guard's DoD fidelity.

### DD-2 — Opt-in readiness, not a default flip (non-destabilization)

The right model for a health surface that is ALSO a container liveness probe is: keep the default at
200 (liveness — a degraded process is still alive) and add an explicit opt-in that exposes readiness
via the status code. `?strict=true|1|yes` → 503 on `degraded`; default → 200 always. This is the
smallest change that makes `degraded` visible to a status-code consumer without any risk of
restart-flapping the container. It deliberately mirrors the Go core `?strict` opt-in already shipped
for this bug, so the operator uses ONE query param across both health surfaces.

### DD-3 — Backward-compat: the default path returns a plain dict

The non-strict path returns the SAME plain dict as before, so FastAPI serialises it as an
unconditional 200 AND in-process callers keep a subscriptable object. This is essential:
`ml/tests/test_health_model_loaded.py` (the BUG-050-001 regression suite), `ml/tests/test_main.py`,
and `ml/tests/test_embedder.py` all call `health()` with no argument and index the result
(`["status"]`, `["model_loaded"]`, `"status" in response`). Only the explicit `?strict` path returns
a `JSONResponse` (whose status code is inspectable). A body-shape/backward-compat guard test locks
this in.

### DD-4 — Parse parity with the Go contract

`_health_strict_requested` accepts exactly the truthy variants the Go `healthStrictRequested`
accepts (`1`, `true`, `yes`, case-insensitive, trimmed) and treats everything else — including
absent, `false`, `0`, garbage — as the default liveness path. One opt-in vocabulary across both
surfaces.

### DD-5 — Adversarial proof (the test detects a regression)

`test_strict_degraded_returns_503` drives a FastAPI `TestClient` (no context manager ⇒ the
NATS-connecting lifespan does NOT run, matching `test_metrics.py`, so `nats_client` stays `None` ⇒
`degraded`) and asserts `GET /health?strict=true` → 503. Against the pre-fix unconditional 200 it
FAILS (the RED capture). `test_default_degraded_stays_200` is the paired NON-DESTABILIZATION invariant
(default `/health` stays 200 when degraded) that would fail if a future change naively flipped the
default to 503.

### DD-6 — Certification boundary: ML-sidecar delta + spec-only packet

The completion delta is `ml/app/main.py` + `ml/tests/test_health_strict_degraded.py`. The Go
`?strict` half (`internal/api/health.go`, `internal/api/health_test.go`) is the earlier partial fix,
already committed at HEAD, and is verified READ-ONLY (NOT re-changed). All packet mutations land under
the BUG-050-002 bug folder. The G093 delivery-implementation-delta requirement is satisfied by the
G053-compatible Code Diff Evidence in `report.md`, which cites the real non-spec delivery files
(`ml/app/main.py`, `ml/tests/test_health_strict_degraded.py`).

### DD-7 — Running-stack + consumer-adoption re-check is a NON-GATING devops step

The `?strict` capability is baked into the built ML image; the already-running prod image keeps the
old behavior until it is rebuilt (`./smackerel.sh config generate` + image build) and redeployed. Two
follow-ups are routed to `bubbles.devops` as NON-GATING operational steps (`redeployRequired: true`):
(1) the knb `<deployment-owner>/<product>/<target>/verify.sh` + shared-observability alerting should
adopt `GET /health?strict=true` (503-on-degraded) or parse the JSON `status` — that file lives in the
knb repo, out of this repo's boundary; and (2) a fresh full-stack re-check that a running degraded
sidecar surfaces 503 on `?strict`. Neither blocks certification: the mechanism is source-committed and
unit-proven, and the live masking is already a committed redteam measurement.

### Single-Implementation Justification

BUG-050-002 has exactly ONE implementation per surface and introduces no
provider/adapter/strategy/variant axis. `/health` is the single ML-sidecar health endpoint; the fix
adds one opt-in query parameter and one call-time parse helper (`_health_strict_requested`) that
mirrors the existing Go `healthStrictRequested`. There is no second implementation, no pluggable
driver/channel, and no variation axis to model, so a Capability Foundation / Concrete Implementations
/ Variation Axes split does not apply.

## Affected Files

**Delivery (completion this session):**

- `ml/app/main.py` — `/health` accepts a `strict` query param; default path returns a plain dict
  (unconditional 200, byte-for-byte unchanged); `?strict=true|1|yes` returns 503 when the status is
  not `up`; new `_health_strict_requested` helper mirrors the Go contract. Import adds `JSONResponse`.
- `ml/tests/test_health_strict_degraded.py` — new FastAPI-`TestClient` HTTP-layer regression suite
  (adversarial 503-on-degraded, truthy-variant parity, strict-healthy-200, DEFAULT-degraded-stays-200
  non-destabilization, falsey/absent-stay-200, backward-compat dict-not-Response).

**Delivery (earlier partial fix, already committed at HEAD — verified read-only, NOT re-changed):**

- `internal/api/health.go` — `healthStrictRequested(r)` + the 503-on-`?strict`-degraded status-code
  selection in `HealthHandler`.
- `internal/api/health_test.go` — `TestHealthHandler_StrictDegradedReturns503`,
  `TestHealthHandler_StrictHealthyReturns200`, `TestHealthHandler_DefaultDegradedStays200`.

**Certification packet (this reconcile):**

- `specs/050-ml-sidecar-health-isolation/bugs/BUG-050-002-health-degraded-masked-http-200/` — all 8
  artifacts.

## Rollback

Pure git revert of the two BUG-050-002 commits restores the prior `partially_fixed_in_repo` status and
removes the ML-sidecar `?strict` opt-in; the default `/health` returns to always-200 (the pre-fix
behavior). Zero runtime impact on the container liveness contract (the default path is unchanged by
the fix). The already-committed Go `?strict` half is independent and is not affected by a BUG-050-002
revert.
