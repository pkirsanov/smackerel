# BUG-050-001 — Core `/api/health` fabricates ML sidecar `model_loaded:true`

- **Severity:** MEDIUM (redteam **F7**)
- **Owning spec:** `050-ml-sidecar-health-isolation`
- **Source:** redteam adversarial interrogation of the LIVE smackerel prod deployment on evo-x2
- **Status:** FIXED IN-REPO (requires prod redeploy to take effect) — not pushed
- **Coordinates with:** [BUG-050-002](../BUG-050-002-health-degraded-masked-http-200/bug.md) (F1) — the ML sub-status feeds the aggregate health.

## Summary

`checkMLSidecar` in [internal/api/health.go](../../../../internal/api/health.go) hardcoded
`Status:"up"` + `ModelLoaded:true` on any `200` from the ML sidecar's `/health` and **never
parsed the response body**. The live ML sidecar reports `{"status":"up","model_loaded":false}`,
so core *actively misreported* the ML model as loaded — an anti-fabrication (G021) violation in
the runtime health surface itself.

## Reproduction

**Redteam (live prod, evo-x2):**

- Core authenticated `/api/health` → `services.ml_sidecar = {"status":"up","model_loaded":true}`.
- Direct ML `/health` → `{"status":"up","model_loaded":false}`.
- Core's claim (`model_loaded:true`) contradicts the sidecar's own report (`false`).

**In-repo static confirmation (pre-fix `checkMLSidecar`):**

```go
if !probeHTTPGet(ctx, baseURL+"/health", client) {   // probeHTTPGet returns only 200/not-200
    return ServiceStatus{Status: "down"}
}
loaded := true                                        // <-- hardcoded, body never read
return ServiceStatus{Status: "up", ModelLoaded: &loaded}
```

`probeHTTPGet` drains and discards the body (`io.Copy(io.Discard, resp.Body)`), so the
`model_loaded` / `status` fields were structurally unreadable by this path.

## Root cause

The ML-sidecar probe reused the boolean-only `probeHTTPGet` helper (built for the Ollama
liveness probe, which has no body contract) and then **invented** the `model_loaded` value
rather than decoding the sidecar's JSON self-report.

## Fix (in-repo)

`checkMLSidecar` now issues its own bounded GET and **reflects** the sidecar's self-report:

- Non-200 / transport error → `down` (unchanged).
- 200 + parseable body → `Status` and `ModelLoaded` taken from `{"status","model_loaded"}`.
- 200 + empty/unparseable body → `up` with **no** `model_loaded` claim (reachable-but-unknown;
  no fabrication).

Files:

- [internal/api/health.go](../../../../internal/api/health.go) — new `mlHealthBody` type; `checkMLSidecar` decodes the body.
- [internal/api/health_test.go](../../../../internal/api/health_test.go) — updated `TestCheckMLSidecar_HealthyResponse` (was encoding the bug: asserted `ModelLoaded:true` on an empty body) + new `TestCheckMLSidecar_ModelNotLoaded` (adversarial), `TestCheckMLSidecar_DegradedBody`, `TestCheckMLSidecar_ReachableUnparseableBody`.

## Test evidence

**RED (pre-fix source, new tests) — `./smackerel.sh test unit --go` with fixes stashed:**

```
--- FAIL: TestCheckMLSidecar_ModelNotLoaded (0.02s)
FAIL    github.com/smackerel/smackerel/internal/api     0.174s
___GO_RED_EXIT=1___
```

(Adversarial: the sidecar reports `model_loaded:false`; the pre-fix hardcode returns `true`.)

**GREEN (fix in place) — `./smackerel.sh test unit`:**

```
[go-unit] go test ./... finished OK
ok      github.com/smackerel/smackerel/internal/api     1.141s
___FULL_UNIT_EXIT=0___
```

## Redeploy note

The fix is in the built core runtime. The running prod image is **unchanged** until an
operator-gated redeploy of `smackerel-core`. No push / redeploy performed here.
