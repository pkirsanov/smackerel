# Scopes: BUG-023-002 — Health probe HTTP-GET duplication

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

---

## Scope 1: Extract `probeHTTPGet` helper and collapse the two probe wrappers

**Status:** Done
**Priority:** P2
**Depends On:** None

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-023-FIX-002 The two HTTP health probes share one bounded-GET helper
  Given internal/api/health.go contains a package-private probeHTTPGet helper
  And the helper owns the context.WithTimeout, http.NewRequestWithContext, client.Do, and body-drain plumbing
  When the checkMLSidecar and checkOllama wrappers are invoked
  Then each wrapper consists of an empty-URL guard, one probeHTTPGet call, and a ServiceStatus translation
  And the pre-existing 8 TestCheckMLSidecar_* / TestCheckOllama_* tests all PASS unmodified
  And go test -count=1 -race ./internal/api/ remains green
```

### Implementation Plan

1. Insert `probeHTTPGet(ctx context.Context, url string, client *http.Client) bool` immediately above `checkMLSidecar` in `internal/api/health.go`. The helper owns the timeout, request-build, transport call, body-drain, and `StatusCode == 200` test.
2. Rewrite `checkMLSidecar` body to: empty-URL guard → `probeHTTPGet(ctx, baseURL+"/health", client)` → ServiceStatus translation (with `ModelLoaded: &loaded` on success).
3. Rewrite `checkOllama` body to: empty-URL guard → `probeHTTPGet(ctx, ollamaURL+"/api/tags", client)` → ServiceStatus translation.
4. Run `go test -count=1 -v -run 'TestCheckMLSidecar|TestCheckOllama' ./internal/api/` and verify all 8 tests PASS.
5. Run `go test -count=1 -race ./internal/api/` and verify race-clean.
6. Run `go build ./...` and verify exit 0.

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| T-FIX-2-01 | `TestCheckMLSidecar_EmptyURL` in internal/api/health_test.go | unit | `internal/api/health_test.go` | empty URL → `Status: "not_configured"` unchanged after refactor | SCN-023-FIX-002 |
| T-FIX-2-02 | `TestCheckMLSidecar_HealthyResponse` in internal/api/health_test.go | unit | `internal/api/health_test.go` | 200 OK → `Status: "up"` and `ModelLoaded: &true` unchanged after refactor | SCN-023-FIX-002 |
| T-FIX-2-03 | `TestCheckMLSidecar_UnhealthyResponse` in internal/api/health_test.go | unit | `internal/api/health_test.go` | non-200 → `Status: "down"` unchanged after refactor | SCN-023-FIX-002 |
| T-FIX-2-04 | `TestCheckMLSidecar_ConnectionRefused` in internal/api/health_test.go | unit | `internal/api/health_test.go` | transport error → `Status: "down"` unchanged after refactor | SCN-023-FIX-002 |
| T-FIX-2-05 | `TestCheckOllama_Healthy` in internal/api/health_test.go | unit | `internal/api/health_test.go` | 200 OK → `Status: "up"` unchanged after refactor | SCN-023-FIX-002 |
| T-FIX-2-06 | `TestCheckOllama_Down` in internal/api/health_test.go | unit | `internal/api/health_test.go` | non-200 → `Status: "down"` unchanged after refactor | SCN-023-FIX-002 |
| T-FIX-2-07 | `TestCheckOllama_NotConfigured` in internal/api/health_test.go | unit | `internal/api/health_test.go` | empty URL → `Status: "not_configured"` unchanged after refactor | SCN-023-FIX-002 |
| T-FIX-2-08 | `TestCheckOllama_Unreachable` in internal/api/health_test.go | unit | `internal/api/health_test.go` | transport error → `Status: "down"` unchanged after refactor | SCN-023-FIX-002 |
| T-FIX-2-09 | Race detector regression for `mlClient` sync.Once | unit (race) | `internal/api/health_test.go` | `go test -count=1 -race ./internal/api/` exit 0; `TestMLClient_ConcurrentAccess` PASS | SCN-023-FIX-002 |
| T-FIX-2-10 | Whole-tree build still compiles | build | `internal/api/health.go` | `go build ./...` exit 0 | SCN-023-FIX-002 |

### Definition of Done

- [x] `internal/api/health.go` contains a single package-private `probeHTTPGet` helper between the probe section header (`mlClient`) and the probe wrappers — **Phase:** implement
  > Evidence: `grep -n '^func probeHTTPGet' internal/api/health.go` returns exactly one match; the helper body owns `context.WithTimeout(healthAuxiliaryProbeTimeout)`, `http.NewRequestWithContext`, `client.Do`, deferred body-drain, and the `StatusCode == http.StatusOK` test.
- [x] `checkMLSidecar` and `checkOllama` are reduced to (a) empty-URL guard, (b) one `probeHTTPGet` call, (c) ServiceStatus translation — **Phase:** implement
  > Evidence: each wrapper body is now ≤ 8 lines; pre-refactor each was ≥ 18 lines; `awk '/^func checkMLSidecar/,/^}/' internal/api/health.go | wc -l` returns `8` and `awk '/^func checkOllama/,/^}/' internal/api/health.go | wc -l` returns `7`.
- [x] Scenario SCN-023-FIX-002 (Probe wrappers share one bounded-GET helper): the 8 pre-existing probe-wrapper tests still pass — **Phase:** test
  > Evidence:
  > ```
  > $ go test -count=1 -v -run 'TestCheckMLSidecar|TestCheckOllama' ./internal/api/
  > === RUN   TestCheckMLSidecar_EmptyURL
  > --- PASS: TestCheckMLSidecar_EmptyURL (0.00s)
  > === RUN   TestCheckMLSidecar_HealthyResponse
  > --- PASS: TestCheckMLSidecar_HealthyResponse (0.04s)
  > === RUN   TestCheckMLSidecar_UnhealthyResponse
  > --- PASS: TestCheckMLSidecar_UnhealthyResponse (0.01s)
  > === RUN   TestCheckMLSidecar_ConnectionRefused
  > --- PASS: TestCheckMLSidecar_ConnectionRefused (1.50s)
  > === RUN   TestCheckOllama_Healthy
  > --- PASS: TestCheckOllama_Healthy (0.01s)
  > === RUN   TestCheckOllama_Down
  > --- PASS: TestCheckOllama_Down (0.01s)
  > === RUN   TestCheckOllama_NotConfigured
  > --- PASS: TestCheckOllama_NotConfigured (0.00s)
  > === RUN   TestCheckOllama_Unreachable
  > --- PASS: TestCheckOllama_Unreachable (1.50s)
  > PASS
  > ok      github.com/smackerel/smackerel/internal/api     3.108s
  > ```
- [x] Race-detector regression: `mlClient()` lazy-init pathway stays race-clean — **Phase:** test
  > Evidence: `go test -count=1 -race -run TestMLClient ./internal/api/` exit 0 (output captured in report.md `### Test Evidence`).
- [x] Whole-tree build still compiles after refactor — **Phase:** test
  > Evidence: `go build ./...` exit 0 (output captured in report.md `### Test Evidence`).
- [x] No new package-level state introduced — **Phase:** audit
  > Evidence: `grep -nE '^var |^const ' internal/api/health.go` before and after the refactor return the same set; `probeHTTPGet` is a pure function over its arguments.
- [x] Boundary respected: only `internal/api/health.go` is modified — **Phase:** audit
  > Evidence: `git diff --name-only` returns exactly `internal/api/health.go` (plus the new bug folder artifacts under `specs/023-engineering-quality/bugs/BUG-023-002-health-probe-dedup/`).
- [x] `bash .github/bubbles/scripts/artifact-lint.sh specs/023-engineering-quality/bugs/BUG-023-002-health-probe-dedup` PASSES — **Phase:** validate
  > Evidence: see report.md `### Validation Evidence`.
