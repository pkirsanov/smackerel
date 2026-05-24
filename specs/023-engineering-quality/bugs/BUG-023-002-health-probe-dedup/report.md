# Report: BUG-023-002 — Health probe HTTP-GET duplication

Links: [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md)

---

## Summary

`internal/api/health.go` previously defined two near-identical service-health probe functions, `checkMLSidecar` and `checkOllama` (spec 023 Scope 2 surface). Each implemented the same five-step bounded-GET pattern (empty-URL guard, `context.WithTimeout(healthAuxiliaryProbeTimeout)`, `http.NewRequestWithContext`, `client.Do`, body-drain + `StatusCode == 200`), differing only in the URL suffix appended to the base URL and (for ML) the optional `ModelLoaded: &loaded` field set on success. This sweep round (stochastic-quality-sweep `sweep-2026-05-23-r30`, round 18, trigger `simplify`, mapped mode `simplify-to-doc`) extracted the shared skeleton into a single package-private `probeHTTPGet(ctx, url, client) bool` helper, leaving `checkMLSidecar` and `checkOllama` as thin wrappers (empty-URL guard + one helper call + ServiceStatus translation). All 8 pre-existing `TestCheckMLSidecar_*` / `TestCheckOllama_*` black-box tests continue to pass unmodified, providing the regression suite for the four observable branches per wrapper. The race-detector regression for `SCN-023-01` (`TestMLClient_ConcurrentAccess`) also remains green. Boundary respected: only `internal/api/health.go` is touched in production code.

## Completion Statement

All 8 DoD items in `scopes.md` Scope 1 are checked `[x]` with inline raw evidence. The refactor preserves the certified Scope 2 behaviour of the parent spec (`SCN-023-06` live Ollama probe, `SCN-023-01` race-free `mlClient`), so no parent-spec artifact change is required and the parent 023 spec stays certified `done`.

## Test Evidence

### Test Evidence — black-box wrapper tests

#### Probe wrappers — all 8 pre-existing tests still PASS

```
$ go test -count=1 -v -run 'TestCheckMLSidecar|TestCheckOllama' ./internal/api/
=== RUN   TestCheckMLSidecar_EmptyURL
--- PASS: TestCheckMLSidecar_EmptyURL (0.00s)
=== RUN   TestCheckMLSidecar_HealthyResponse
--- PASS: TestCheckMLSidecar_HealthyResponse (0.03s)
=== RUN   TestCheckMLSidecar_UnhealthyResponse
--- PASS: TestCheckMLSidecar_UnhealthyResponse (0.01s)
=== RUN   TestCheckMLSidecar_ConnectionRefused
--- PASS: TestCheckMLSidecar_ConnectionRefused (1.50s)
=== RUN   TestCheckOllama_Healthy
--- PASS: TestCheckOllama_Healthy (0.00s)
=== RUN   TestCheckOllama_Down
--- PASS: TestCheckOllama_Down (0.01s)
=== RUN   TestCheckOllama_NotConfigured
--- PASS: TestCheckOllama_NotConfigured (0.00s)
=== RUN   TestCheckOllama_Unreachable
--- PASS: TestCheckOllama_Unreachable (1.50s)
PASS
ok      github.com/smackerel/smackerel/internal/api     3.089s
```

**Claim Source:** executed.

#### Race-detector regression for `mlClient()` sync.Once (SCN-023-01)

```
$ go test -count=1 -race -run 'TestMLClient' ./internal/api/
(no race warnings emitted by the runtime)
ok      github.com/smackerel/smackerel/internal/api     1.082s
(exit 0)
```

**Claim Source:** executed.

#### Whole-tree build

```
$ go build ./...
(no output)
(exit 0)
```

**Claim Source:** executed.

#### Probe-section structure (post-refactor)

```
$ grep -n '^func probeHTTPGet' internal/api/health.go
586:func probeHTTPGet(ctx context.Context, url string, client *http.Client) bool {

$ awk '/^func probeHTTPGet/,/^}/' internal/api/health.go | wc -l
20

$ awk '/^func checkMLSidecar/,/^}/' internal/api/health.go | wc -l
10

$ awk '/^func checkOllama/,/^}/' internal/api/health.go | wc -l
9
```

Pre-refactor each wrapper was ≥ 28 lines (`checkMLSidecar` = 29, `checkOllama` = 28; ~57 lines of probe code combined). Post-refactor the two wrappers are 19 lines combined and the new `probeHTTPGet` helper is 20 lines (including its 6-line doc comment), so the duplicated boilerplate (timeout + request build + transport + body-drain) now exists exactly once.

## Validation Evidence

### Validation Evidence — behavioural and boundary checks

The behavioural envelope of both wrappers is preserved across four observable branches: empty URL → `not_configured`; transport error → `down`; non-200 response → `down`; 200 response → `up` (ML additionally sets `ModelLoaded: &loaded`). The 8 pre-existing `TestCheckMLSidecar_*` / `TestCheckOllama_*` tests in `internal/api/health_test.go` exercise all four branches per wrapper and all PASS post-refactor (output above). The race-detector regression `TestMLClient_ConcurrentAccess` for `SCN-023-01` PASS under `-race`, confirming the `mlClient()` `sync.Once` pathway was not touched. `go build ./...` exits 0.

#### Boundary check

```
$ git status --short | grep -E '(internal/api/health|specs/023)'
 M internal/api/health.go
?? specs/023-engineering-quality/bugs/BUG-023-002-health-probe-dedup/
```

Only `internal/api/health.go` is touched in production code; all other edits are confined to the new bug folder. No edits to parent `specs/023-engineering-quality/spec.md`, `scopes.md`, `state.json`, or any other spec.

### Audit Evidence

#### Artifact-lint and traceability scope

This bug is owned by `bubbles.workflow` running the parent-expanded `simplify-to-doc` child mode (statusCeiling `done`). Artifact-lint is invoked against this bug folder; the parent spec 023 already lives at the workspace certification ceiling and requires no audit re-run for this artifact-scoped change. Scenario coverage for the wrappers is owned by SCN-023-01/02/06/07 in the parent spec; the wrapper-tests referenced in `scopes.md` Test Plan rows T-FIX-2-01..T-FIX-2-10 are the regression suite for this fix.

#### Sweep context

- Parent sweep: `sweep-2026-05-23-r30`
- Round: 18
- Target: `specs/023-engineering-quality`
- Trigger: `simplify`
- Mapped child workflow mode: `simplify-to-doc`
- Execution model: parent-expanded-child-mode (this round's parent-expanded execution dispatched the simplify probe, identified the DRY-violation finding, and ran the full finding-owned planning + delivery + finalize chain inside this bug packet)
- 055 WIP under `cmd/core/` / `config/` / `internal/api/notifications*` / `internal/config/` was left untouched — only `internal/api/health.go` and this bug folder were modified

## Closure Summary

- 1 simplify finding identified, 1 finding closed in-round.
- 0 net production functions removed (extraction adds 1 helper); 0 new package-level state.
- 0 new tests added (the 8 black-box wrapper tests are the regression suite); 0 tests deleted; 0 tests modified.
- ~18 lines of code removed at the boilerplate level (57 lines of wrapper bodies → 19 lines of wrapper bodies; 20 new lines in `probeHTTPGet` of which 6 are doc comment).
