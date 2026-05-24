# Bug: BUG-023-002 — Health probe HTTP-GET duplication (`checkMLSidecar` / `checkOllama`)

## Classification

- **Type:** Code-quality / simplify (DRY violation in spec 023 Scope 2 surface)
- **Severity:** LOW (no runtime defect; refactor that removes ~30 lines of duplicated HTTP-probe boilerplate)
- **Parent Spec:** 023 — Engineering Quality
- **Workflow Mode:** bugfix-fastlane (sweep-spawned, simplify-to-doc round 18 — sweep-2026-05-23-r30)
- **Status:** Fixed

## Problem Statement

`internal/api/health.go` defines two near-identical service-health probe functions, `checkMLSidecar` (lines 581-609) and `checkOllama` (lines 612-639). Both implement the same five-step pattern:

1. Empty-URL guard → return `ServiceStatus{Status: "not_configured"}`.
2. Bounded `context.WithTimeout(ctx, healthAuxiliaryProbeTimeout)`.
3. `http.NewRequestWithContext(probeCtx, http.MethodGet, url, nil)` → on error return `ServiceStatus{Status: "down"}`.
4. `client.Do(req)` → on error return `ServiceStatus{Status: "down"}`; on success defer `io.Copy(io.Discard, resp.Body)` + `resp.Body.Close()`.
5. `resp.StatusCode == http.StatusOK` → success status; else `ServiceStatus{Status: "down"}`.

The only meaningful differences between the two functions are:

- The path suffix joined to the base URL (`"/health"` vs `"/api/tags"`).
- `checkMLSidecar` sets the optional `ModelLoaded: &loaded` field on success; `checkOllama` does not.

This shape produces ~58 lines (excluding doc comments) of which ~38 lines are byte-identical boilerplate. A future third probe (e.g. agent service, embeddings service) would copy the same boilerplate again, accumulating drift risk (timeout, body-drain, header-skip behaviour). Spec 023's Scope 2 was specifically about cleaning up health-probe surface (Ollama/Telegram live probes + intelligence-handler `writeJSON` consolidation); the residual duplication between the two HTTP probes is in-scope for the same surface.

## Reproduction (Pre-fix)

```
$ wc -l internal/api/health.go
867 internal/api/health.go
$ awk '/^func checkMLSidecar/,/^}/' internal/api/health.go | wc -l
29
$ awk '/^func checkOllama/,/^}/' internal/api/health.go | wc -l
28
$ diff \
    <(awk '/^func checkMLSidecar/,/^}/' internal/api/health.go | sed -E 's/checkMLSidecar/X/; s/baseURL/U/; s|/health|/PATH|; s/loaded := true/_/; s/ModelLoaded: &loaded//') \
    <(awk '/^func checkOllama/,/^}/' internal/api/health.go | sed -E 's/checkOllama/X/; s/ollamaURL/U/; s|/api/tags|/PATH|; s/loaded := true/_/')
# (only trivial differences remain — URL-suffix string + optional ModelLoaded field)
```

The duplication is structural, not coincidental — both functions are spec-023-owned surface dedicated to "issue a bounded HTTP GET and translate the response to a `ServiceStatus`."

## Acceptance Criteria

- [x] `internal/api/health.go` exposes a single private helper `probeHTTPHealth(ctx context.Context, url string, client *http.Client) bool` that owns the bounded `context.WithTimeout` + `http.NewRequestWithContext` + `client.Do` + body-drain + `StatusCode == 200` translation
- [x] `checkMLSidecar` and `checkOllama` retain their existing signatures, doc comments, and observable behaviour (return value shape, `ModelLoaded` field on ML success, all four pre-existing branches: `not_configured`, `down`, `up`, `up + ModelLoaded`)
- [x] All 8 pre-existing `TestCheckMLSidecar_*` and `TestCheckOllama_*` tests continue to PASS unmodified
- [x] `go build ./...` succeeds
- [x] `go test -count=1 -race ./internal/api/` PASSES (no new race introduced; pre-existing `TestMLClient_ConcurrentAccess` continues green)
- [x] No new package-level state is introduced; the helper is package-private and stateless
- [x] No change to spec 023's parent `scopes.md` Scope 1/2/3 DoD (this is a child-bug refactor that preserves the certified behaviour, it does not promote new behaviour)
- [x] `bash .github/bubbles/scripts/artifact-lint.sh specs/023-engineering-quality/bugs/BUG-023-002-health-probe-dedup` PASSES
