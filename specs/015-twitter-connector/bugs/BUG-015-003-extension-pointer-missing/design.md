# Bug Fix Design: [BUG-015-003] Spec 015 forward pointer to spec 056

## Root Cause Analysis

### Investigation Summary
Spec-review pass surfaced P1-3: spec 015 declares `SyncMode = archive | api | hybrid` and devotes an entire "API Access Strategy" section (L72+) to the hybrid strategy, but ships archive-only code. Spec 056 (`twitter-api-connector`), certified `specs_hardened` on 2026-05-27, is the actual implementation of the `SyncModeAPI` / `SyncModeHybrid` path. Spec 056's spec.md L11–13 explicitly names spec 015 as its predecessor and states "Spec 015 shipped ... had zero HTTP client code, zero api.twitter.com/2/* calls". The runtime split matches: `internal/connector/twitter/twitter.go` + `archive.go` + `threads.go` + `normalizer.go` are spec-015 deliverables; `internal/connector/twitter/api.go` + `api_test.go` are spec-056 deliverables.

What never happened: the reverse pointer from spec 015 → spec 056 was never authored. `specs/015-twitter-connector/state.json` has no `extendedBy` / `extensions[]` / `supersessions[]` entry referencing `056-twitter-api-connector`. `specs/015-twitter-connector/spec.md` and `design.md` carry no inline forward-pointer note. A reader picking up spec 015 today cannot discover spec 056 from the artifacts in order.

### Root Cause
Process gap, not a code defect: spec 056 did not file a back-pointer edit against spec 015 when it shipped. The drift is artifact-only and the recipe to fix it is already proven by BUG-020-007-supersession-pointer-missing (additive top-level array in state.json + inline notes in spec.md + design.md, all under workflowMode `spec-scope-hardening` with `tdd.exempt`).

### Impact Analysis
- **Affected components:** `specs/015-twitter-connector/spec.md`, `specs/015-twitter-connector/design.md`, `specs/015-twitter-connector/state.json` — documentation only.
- **Affected data:** none.
- **Affected users:** any human or agent reading spec 015 to model new Twitter-connector work. Risk: they assume API/hybrid code is already under spec 015's deliverables, fail to discover spec 056, and either duplicate spec 056's work or file an incorrect bug against spec 015 for "missing" API code.
- **Affected runtime:** none. `internal/connector/twitter/{archive,threads,normalizer,twitter}.go` continue to deliver spec-015 archive behavior; `internal/connector/twitter/api.go` continues to deliver spec-056 API behavior.

## Fix Design

### Solution Approach
Artifact-only, three-file edit inside `specs/015-twitter-connector/`. This mirrors BUG-020-007 exactly:

1. **`state.json` — forward pointer.** Add a schema-valid forward-pointer entry naming `056-twitter-api-connector`, scoped to `SyncModeAPI` + `SyncModeHybrid`. Prefer the additive top-level-array form (`extensions[]` with `extendedBy`, mirroring the proven `supersessions[]` form from BUG-020-007); use `supersessions[]` with `supersededBy` ONLY if the schema rejects `extensions[]`. Semantics MUST reflect extension (not whole-spec supersession) — the archive code is still authoritatively owned by spec 015.
2. **`spec.md` — top-of-document inline note.** Add a banner blockquote near the front matter that names spec 056 and clarifies that spec 015 ships the archive path only; the API/hybrid path lives in spec 056.
3. **`spec.md` — API Access Strategy section note.** Add a second inline note above (or immediately under) the "API Access Strategy — Critical Design Decision" heading at L72 pointing at spec 056 as the implementation home for Options B / C / E.
4. **`design.md` — top-of-document inline note.** Mirror the spec.md banner at the top of design.md.

### Affected Files
| File | Change |
|------|--------|
| `specs/015-twitter-connector/state.json` | Add additive forward-pointer entry (`extensions[]` with `extendedBy` → `056-twitter-api-connector`, scoped to `SyncModeAPI`+`SyncModeHybrid`; fall back to `supersessions[]`/`supersededBy` if schema requires) |
| `specs/015-twitter-connector/spec.md` | Top-of-document forward-pointer banner + second inline note above "API Access Strategy" section |
| `specs/015-twitter-connector/design.md` | Top-of-document forward-pointer banner mirroring spec.md |

### Alternative Approaches Considered
1. **Rewrite spec 015 in place to remove the API/hybrid prose entirely.** Rejected — destroys historical context; spec 056 cites spec 015's strategy section as the design rationale.
2. **File a new spec that "owns" both paths and retire spec 015.** Rejected — spec 015's archive code is in production; retiring it requires migrating archive ownership, which is far out of scope.
3. **Touch `internal/connector/twitter/**`.** Rejected — code already correctly partitioned between the two specs; touching it expands the change boundary and triggers unnecessary runtime test review.
4. **Use only an `extendedBy` scalar at the top level of state.json.** Rejected — additive top-level scalars are less proven than the additive-array form (BUG-020-007 used `supersessions[]` and passed); array form scales if a third extension spec ever ships.

### Regression Test Design
Artifact-only drift. The regression test IS the artifact-shape check:

- **Pre-fix (adversarial) assertions — MUST FAIL on `main` before the fix:**
  1. `grep -q '056-twitter-api-connector' specs/015-twitter-connector/state.json` → exit 1.
  2. `grep -q '056-twitter-api-connector' specs/015-twitter-connector/spec.md` → exit 1.
  3. `grep -q '056-twitter-api-connector' specs/015-twitter-connector/design.md` → exit 1.
- **Post-fix assertions — MUST PASS after the fix:** the same three greps return exit 0; the spec.md grep returns ≥ 2 matches (banner + API Access Strategy note).
- **Adversarial proximity guard:** the spec.md "API Access Strategy" section heading (`grep -n 'API Access Strategy'`) MUST have a `056-twitter-api-connector` reference within ±5 lines. This catches the "annotated-then-someone-stripped-the-section-note" reintroduction path.
- **Change boundary:** `git diff --name-only` returns only paths under `specs/015-twitter-connector/`.
- **No bailout patterns:** the scenarios are explicit greps with explicit expected exit codes; no `if file_missing: return 0` early exits.

### Change Boundary
- ALLOWED: any file under `specs/015-twitter-connector/` (including this bug folder).
- FORBIDDEN: any file outside `specs/015-twitter-connector/` (no `internal/connector/twitter/**`, no `specs/056-*`, no `docs/**`, no `.github/**`).
- Enforcement: implementing agent MUST run `git diff --name-only` before claiming Done and reject the change if any path outside `specs/015-twitter-connector/` appears.
