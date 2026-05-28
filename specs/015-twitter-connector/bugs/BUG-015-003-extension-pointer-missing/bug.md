# Bug: [BUG-015-003] Spec 015 declares SyncMode=api|hybrid but ships archive-only — no forward pointer to spec 056

## Summary
`specs/015-twitter-connector` declares the `Connector` config schema with `SyncMode = archive | api | hybrid` and dedicates an entire "API Access Strategy" section to the hybrid strategy, but ships **archive-only** code: zero HTTP client, zero `api.twitter.com/2/*` calls, zero OAuth2 bearer-token validation, zero rate-limit handling. Spec `056-twitter-api-connector` (certified `specs_hardened` on 2026-05-27) is the actual implementation of the API/hybrid path and explicitly names spec 015 as its predecessor — but spec 015 carries no reverse pointer to spec 056. A reader of spec 015 today is misled into believing the API/hybrid code is already in this package. This is artifact-only documentation drift; the runtime code is correct (archive in 015's twitter.go; API in 056's `api.go` / `api_test.go`).

## Severity
- [ ] Critical
- [ ] High
- [x] Medium — stale governance doc; misleads any reader (human or agent) into expecting API code under spec 015's deliverables
- [ ] Low

## Status
- [x] Reported
- [x] Confirmed (reproduced — evidence is the spec text itself)
- [x] In Progress
- [x] Fixed
- [x] Verified
- [x] Closed

## Reproduction Steps
1. `grep -nE "056|api-connector|superseded|extended by" specs/015-twitter-connector/spec.md` → 0 hits.
2. `grep -nE "056|api-connector|superseded|extended by" specs/015-twitter-connector/design.md` → 0 hits.
3. Read `specs/015-twitter-connector/spec.md` L72–92 — declares Hybrid (Option E) as the connector's strategy.
4. Read `specs/056-twitter-api-connector/spec.md` L11–13 — explicitly names spec 015 as predecessor and states "Spec 015 shipped ... had zero HTTP client code, zero api.twitter.com/2/* calls".
5. Inspect `internal/connector/twitter/` — `twitter.go` / `archive.go` / `threads.go` / `normalizer.go` are the spec-015 archive deliverables; `api.go` / `api_test.go` are the spec-056 API/hybrid deliverables.
6. Read `specs/015-twitter-connector/state.json` — no `extendedBy` / `extensions[]` / `supersessions[]` field referencing `056-twitter-api-connector`.

## Expected Behavior
- `specs/015-twitter-connector/state.json` carries a schema-valid forward pointer (e.g., additive `extensions[]` entry with `extendedBy: "056-twitter-api-connector"`, scoped to `SyncModeAPI` + `SyncModeHybrid` implementation) following the BUG-020-007 additive-array recipe.
- `specs/015-twitter-connector/spec.md` carries an inline note at the top of the document directing readers to spec 056 for the API/hybrid implementation; the "API Access Strategy" section (L72+) carries a second inline note pointing at spec 056 as the implementation home for Options B/C/E.
- `specs/015-twitter-connector/design.md` carries a mirrored inline note at the top of the document pointing readers to spec 056 for the API/hybrid code.

## Actual Behavior
- Spec 015 reads as if `SyncMode = api | hybrid` is implemented under its own deliverables.
- No forward pointer in `state.json`; no inline note in `spec.md` / `design.md`.
- A new reader cannot discover spec 056 from spec 015 by reading the artifacts in order.

## Environment
- Repo: `smackerel` @ current `main`
- Affected artifacts: `specs/015-twitter-connector/spec.md`, `specs/015-twitter-connector/design.md`, `specs/015-twitter-connector/state.json`
- Authoritative successor for API/hybrid mode: `specs/056-twitter-api-connector/`
- Code reality: `internal/connector/twitter/twitter.go` (archive — 015); `internal/connector/twitter/api.go` + `api_test.go` (API — 056)

## Error Output
```
$ grep -cE "056|api-connector|superseded|extended by" specs/015-twitter-connector/spec.md specs/015-twitter-connector/design.md
specs/015-twitter-connector/spec.md:0
specs/015-twitter-connector/design.md:0

$ grep -c "056-twitter-api-connector" specs/015-twitter-connector/state.json
0

$ grep -nE "Spec 015 shipped|predecessor" specs/056-twitter-api-connector/spec.md | head -3
11:- **Predecessor:** [specs/015-twitter-connector/](../015-twitter-connector/) — delivered the `SyncModeArchive` path ... Declared the `SyncModeAPI` and `SyncModeHybrid` constants but left them unimplemented.
```

## Root Cause (filled after analysis)
Process gap, not a code defect: spec 056 shipped the API/hybrid implementation and recorded spec 015 as its predecessor, but the back-pointer from spec 015 → spec 056 was never authored. Surfaced by spec-review P1-3. Recipe to follow: BUG-020-007-supersession-pointer-missing (artifact-only supersession pointer, additive `supersessions[]`/`extensions[]` form in state.json + inline notes in spec.md/design.md, all under workflowMode `spec-scope-hardening` with `tdd.exempt`).

## Related
- Feature: `specs/015-twitter-connector/`
- Extended by: `specs/056-twitter-api-connector/` (certified `specs_hardened` 2026-05-27)
- Recipe bug: `specs/020-security-hardening/bugs/BUG-020-007-supersession-pointer-missing/`
- Spec-review finding: P1-3
