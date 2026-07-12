# BUG-050-002 — `/api/health` returns `degraded` with HTTP 200 (all status-code consumers blind)

- **Severity:** HIGH (redteam **F1**)
- **Owning spec:** `050-ml-sidecar-health-isolation`
- **Source:** redteam adversarial interrogation of the LIVE smackerel prod deployment on <deploy-host>
- **Status:** PARTIALLY FIXED IN-REPO (non-destabilizing part) + ROUTED (consumer adoption) — not pushed

## Summary

`HealthHandler` computes an aggregate `overall` status of `healthy`/`degraded` but always writes
`http.StatusOK`. The Docker `HEALTHCHECK` (`wget --spider …`) and the knb adapter `verify.sh`
both key on `200`, so **all three layers are blind to `degraded`** — a degraded prod core looks
healthy to every automated consumer.

## Reproduction

**Redteam (live prod):** `GET /api/health` → body `{"status":"degraded"}` with **HTTP 200**.

**In-repo static confirmation (pre-fix):** the handler ended with an unconditional
`writeJSON(w, http.StatusOK, resp)` regardless of `overall`. A DB-down unit case
(`TestHealthHandler_DBDown`) already proved `resp.Status == "degraded"` — but nothing asserted
the **status code**, because it was always 200.

## Root cause

`/api/health` conflates **liveness** (is the process up?) with **health/readiness** (is the
system fully healthy?) and resolves the conflict by always returning 200. There was no
status-aware signal any status-code consumer (Docker healthcheck, verify.sh, monitoring) could
read.

## Design care (why NOT flip to 503-on-degraded)

Naively returning `503` on `degraded` would make the Docker `HEALTHCHECK` mark the container
**unhealthy** and trigger restart-flap / fail the deploy — a `degraded` process is still *alive*.
The correct model is **liveness (200, for the container) vs readiness/health (status-aware, for
the operator)**. A dedicated `/readyz` liveness probe already exists (`ReadyzHandler`, DB-only,
200/503).

## Fix (in-repo, non-destabilizing)

Added an **opt-in** status-aware signal to `/api/health`:

- `GET /api/health?strict=true|1|yes` → `503` when `overall != "healthy"`, else `200`.
- Default `GET /api/health` (no param — exactly what the Docker liveness HEALTHCHECK sends) is
  **byte-for-byte unchanged** (always `200`). Zero container-flap risk.

The operator / monitoring / knb `verify.sh` path opts into `?strict=true` to detect `degraded`
via HTTP status without touching the container liveness contract.

Files:

- [internal/api/health.go](../../../../internal/api/health.go) — `healthStrictRequested(r)` + status-code selection at the writeJSON site.
- [internal/api/health_test.go](../../../../internal/api/health_test.go) — `TestHealthHandler_StrictDegradedReturns503` (adversarial), `TestHealthHandler_StrictHealthyReturns200`, `TestHealthHandler_DefaultDegradedStays200` (non-destabilization invariant).

## Routed (out of this repo / follow-up)

1. **knb** `<deployment-owner>/<product>/<target>/verify.sh` + shared-observability alerting → adopt
   `/api/health?strict=true` (or parse the JSON `status` field). `verify.sh` lives in the **knb**
   repo (`<deployment-owner>/<product>/<target>/verify.sh`), out of scope for this in-repo change → **routed to
   bubbles.devops / knb**.
2. **Healthcheck-target consistency:** dev [docker-compose.yml](../../../../docker-compose.yml)
   core healthcheck still targets `/api/health`, while `docker-compose.prod.yml` targets
   `/readyz`. Left unchanged to avoid touching the container liveness contract in this bounded
   fix → **routed to bubbles.devops** for a deliberate liveness-target decision.

## Test evidence

**RED (pre-fix source, new tests) — `./smackerel.sh test unit --go` with fixes stashed:**

```
--- FAIL: TestHealthHandler_StrictDegradedReturns503 (0.00s)
FAIL    github.com/smackerel/smackerel/internal/api     0.174s
___GO_RED_EXIT=1___
```

(`TestHealthHandler_DefaultDegradedStays200` correctly PASSED on both pre- and post-fix — it
asserts the unchanged liveness contract.)

**GREEN (fix in place) — `./smackerel.sh test unit`:**

```
[go-unit] go test ./... finished OK
ok      github.com/smackerel/smackerel/internal/api     1.141s
___FULL_UNIT_EXIT=0___
```

## Redeploy note

The `?strict` capability is in the built core runtime; the running prod image is **unchanged**
until an operator-gated redeploy. The routed knb `verify.sh` adoption can only be wired **after**
the redeployed core exposes `?strict`. No push / redeploy performed here.
