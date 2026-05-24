# Design: BUG-023-002 — Health probe HTTP-GET duplication

> **Bug spec:** [spec.md](spec.md)
> **Parent:** [023 spec](../../spec.md) | [023 scopes](../../scopes.md) | [023 report](../../report.md)
> **Date:** May 24, 2026
> **Workflow Mode:** bugfix-fastlane (spawned by stochastic-quality-sweep `sweep-2026-05-23-r30`, round 18, trigger: simplify, mapped mode: simplify-to-doc)

---

## Root Cause

When spec 023 Scope 2 introduced the live Ollama probe (`SCN-023-06`), the new `checkOllama` function was copy-pasted from the pre-existing `checkMLSidecar` function and edited only at the URL-suffix and the success-status fields. The shared HTTP-GET-with-bounded-timeout + body-drain skeleton was never extracted, so spec 023 shipped two functions that implement the same probe shape with two different URL suffixes.

## Fix Approach

Extract the shared probe skeleton into a single package-private helper:

```go
// probeHTTPGet issues a bounded GET against url and reports whether the
// response status was 200 OK. The caller controls the URL composition,
// timeout (bounded by healthAuxiliaryProbeTimeout), and the success/failure
// translation. The response body is fully drained and closed before the
// helper returns so the underlying connection can be reused.
func probeHTTPGet(ctx context.Context, url string, client *http.Client) bool {
    probeCtx, cancel := context.WithTimeout(ctx, healthAuxiliaryProbeTimeout)
    defer cancel()

    req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, url, nil)
    if err != nil {
        return false
    }

    resp, err := client.Do(req)
    if err != nil {
        return false
    }
    defer func() {
        io.Copy(io.Discard, resp.Body)
        resp.Body.Close()
    }()

    return resp.StatusCode == http.StatusOK
}
```

Then collapse `checkMLSidecar` and `checkOllama` to thin wrappers:

```go
func checkMLSidecar(ctx context.Context, baseURL string, client *http.Client) ServiceStatus {
    if baseURL == "" {
        return ServiceStatus{Status: "not_configured"}
    }
    if !probeHTTPGet(ctx, baseURL+"/health", client) {
        return ServiceStatus{Status: "down"}
    }
    loaded := true
    return ServiceStatus{Status: "up", ModelLoaded: &loaded}
}

func checkOllama(ctx context.Context, ollamaURL string, client *http.Client) ServiceStatus {
    if ollamaURL == "" {
        return ServiceStatus{Status: "not_configured"}
    }
    if !probeHTTPGet(ctx, ollamaURL+"/api/tags", client) {
        return ServiceStatus{Status: "down"}
    }
    return ServiceStatus{Status: "up"}
}
```

## Why This Is Safe

- **Signature-preserving:** Both `checkMLSidecar` and `checkOllama` keep their existing exported-equivalent (package-private) signatures, return types, and call sites in `HealthHandler`.
- **Behaviour-preserving:** All four observable branches per function are preserved exactly:
  - empty URL → `{Status: "not_configured"}`
  - request build error → `{Status: "down"}` (covered by the `probeHTTPGet` `err != nil` short-circuit)
  - transport error → `{Status: "down"}` (covered by the `probeHTTPGet` `err != nil` short-circuit)
  - non-200 → `{Status: "down"}` (covered by the `probeHTTPGet` `false` branch)
  - 200 → `{Status: "up"}` (Ollama) / `{Status: "up", ModelLoaded: &loaded}` (ML)
- **No new race surface:** The helper has no package-level state; it consumes the caller-supplied `*http.Client` (which the existing `mlClient()` `sync.Once` continues to manage from `SCN-023-01`).
- **Existing tests still apply:** All 8 `TestCheckMLSidecar_*` / `TestCheckOllama_*` tests black-box the wrappers and continue to pass unmodified, providing regression coverage of the four-branch contract above.

## What This Is NOT

- This is **not** a public API change. `probeHTTPGet` is package-private.
- This is **not** an attempt to consolidate the two probes into one (their success branches diverge by the `ModelLoaded` field; merging the wrappers would force the caller to pass a `ServiceStatus` builder, which is worse than keeping two 6-line wrappers).
- This is **not** a behaviour change. No timeout, header, retry, or status-mapping logic is altered.

## Regression Test Strategy

The existing tests are the regression suite:

- `TestCheckMLSidecar_EmptyURL` covers the empty-URL branch for ML.
- `TestCheckMLSidecar_HealthyResponse` covers the 200 + `ModelLoaded: true` branch for ML.
- `TestCheckMLSidecar_UnhealthyResponse` covers the non-200 branch for ML.
- `TestCheckMLSidecar_ConnectionRefused` covers the transport-error branch for ML.
- `TestCheckOllama_Healthy` covers the 200 branch for Ollama.
- `TestCheckOllama_Down` covers the non-200 branch for Ollama.
- `TestCheckOllama_NotConfigured` covers the empty-URL branch for Ollama.
- `TestCheckOllama_Unreachable` covers the transport-error branch for Ollama.

All eight tests black-box the wrapper functions, so the refactor is observably a no-op when they continue to pass. `go test -count=1 -race ./internal/api/` adds the SCN-023-01 race-detector regression check (the `mlClient()` `sync.Once` lazy-init pathway is unchanged by this refactor and the race detector confirms it).

## Change Boundary

- Single file: `internal/api/health.go`
- Lines touched: the bodies of `checkMLSidecar` and `checkOllama` plus the insertion of `probeHTTPGet` immediately above them.
- No test file changes (the existing tests are the regression suite).
- No artifact changes to parent `specs/023-engineering-quality/scopes.md` / `spec.md` / `state.json` — the parent spec stays `done`; this is a child-bug refactor that preserves certified behaviour.
