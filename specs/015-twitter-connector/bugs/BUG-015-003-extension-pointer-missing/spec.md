# Spec: [BUG-015-003] Spec 015 must record forward pointer to spec 056 for API/hybrid implementation

## Expected Behavior

### EB-1: state.json carries a forward pointer to spec 056
`specs/015-twitter-connector/state.json` MUST include a schema-valid forward-pointer entry naming `056-twitter-api-connector`, scoped to the `SyncModeAPI` + `SyncModeHybrid` implementation. The exact field placement follows the additive top-level-array recipe proven in `specs/020-security-hardening/bugs/BUG-020-007-supersession-pointer-missing/` (which used `supersessions[]` and passed artifact-lint + state-transition-guard). The implementing agent MAY use `extensions[]` with an `extendedBy` field, or `supersessions[]` with a `supersededBy` field — whichever the v3 control-plane state schema accepts. Naming MUST reflect semantics: spec 056 EXTENDS spec 015 (archive code is still in use); it does NOT supersede the whole spec.

### EB-2: spec.md carries an inline forward-pointer note at the top of the document
`specs/015-twitter-connector/spec.md` MUST open with an inline note (banner / blockquote near the front matter) that:
- names `056-twitter-api-connector` explicitly,
- states that spec 015's deliverables are the **archive path only** (`SyncModeArchive`),
- states that the API/hybrid implementation (`SyncModeAPI`, `SyncModeHybrid`) lives in spec 056.

### EB-3: spec.md "API Access Strategy" section carries a mirrored inline note
The "API Access Strategy — Critical Design Decision" section starting at L72 MUST carry a second inline note (above or immediately under the section heading) pointing at `specs/056-twitter-api-connector/` as the implementation home for Options B / C / E (Hybrid).

### EB-4: design.md carries a mirrored inline forward-pointer note
`specs/015-twitter-connector/design.md` MUST carry the same inline note at the top of the document pointing readers to `specs/056-twitter-api-connector/` for the API/hybrid code.

### EB-5: No runtime code changes
This bug is artifact-only. The fix MUST NOT touch any file outside `specs/015-twitter-connector/`. Code already matches reality (archive in this package's twitter.go; API in spec 056's api.go).

## Acceptance Criteria
1. `grep -n '056-twitter-api-connector' specs/015-twitter-connector/state.json` returns at least one match in a schema-valid forward-pointer field.
2. `grep -n '056-twitter-api-connector' specs/015-twitter-connector/spec.md` returns at least two matches (top-of-document banner + API Access Strategy section note).
3. `grep -n '056-twitter-api-connector' specs/015-twitter-connector/design.md` returns at least one match at the top of the document.
4. `git diff --name-only` for the fix touches only files under `specs/015-twitter-connector/`.
5. `bash .github/bubbles/scripts/artifact-lint.sh specs/015-twitter-connector` passes.
6. `bash .github/bubbles/scripts/artifact-lint.sh specs/015-twitter-connector/bugs/BUG-015-003-extension-pointer-missing` passes.

## Out of Scope
- Re-running the Twitter connector test suite (no runtime code changes).
- Editing `internal/connector/twitter/**` or any other code, compose, or test file.
- Editing spec 056 (it already names spec 015 as predecessor; the missing link is the reverse pointer in spec 015).
- Rewriting spec 015's "API Access Strategy" section in place — the inline note preserves history while pointing readers at the implementation home.
