# Bug Fix Design: [BUG-015-001] Twitter API Polling Path Deprecation

## Root Cause Analysis

### Investigation Summary
Investigated the source/spec drift flagged by `bubbles.validate` on 2026-04-26. Walked the Sync code path, the spec/scope claims, and the validation history. Confirmed that the Twitter connector ships a robust archive-import path with 127/127 unit tests, while the spec/scope claimed an additional API polling path that has no corresponding code.

### Root Cause
A prior workflow round promoted Scope 6 DoD checkboxes to `[x]` without a real implementation. The implementation surface is `internal/connector/twitter/twitter.go` (860 LOC) plus `twitter_test.go` (2355 LOC) — there is no `api.go`, no HTTP client, and no /2/users/:id endpoint call. The certification gate accepted the archive-only test surface as proof of the whole spec because the validate phase did not grep for API-specific identifiers. The 2026-04-26 replay caught the drift.

### Impact Analysis
- Affected components: documentation only — `specs/015-twitter-connector/{spec.md,scopes.md,uservalidation.md,report.md,state.json}` plus the new bug packet under `bugs/BUG-015-001-api-path-deprecated/`.
- Affected data: none (no migrations, no runtime state).
- Affected users: none — the archive path serves the real user value; the API path was never reachable so users never depended on it.

## Fix Design

### Solution Approach
Apply option (b) — formally deprecate the API path — by amending the documentation surface to match the certified archive-only implementation. The amendments preserve historical context (the API requirements move into a `Deferred / Non-Goals — API Path` section instead of being deleted) so future contributors can find the rationale and the bug back-link.

Specific edits:

1. **spec.md** — Add a `Deferred / Non-Goals — API Path` section at the bottom of the body that includes:
   - The four-bullet deprecation rationale (free-tier instability, archive parity, maintenance burden, original "Optional" intent).
   - A back-link to `bugs/BUG-015-001-api-path-deprecated/`.
   - The relocated R-008 contents and the SCN-TW-005 scenario, both annotated as "deferred — see BUG-015-001".
   - A banner at the top of the existing `## ⚠️ API Access Strategy` section pointing to BUG-015-001 so anyone reading the historical strategy table sees the deprecation immediately.
   - Goal #5 reworded from "Optional API polling — For users with API access, poll for new bookmarks and likes at configurable intervals" to "Optional API polling (Deferred — see BUG-015-001) — Originally planned but not implemented; archive-only surface is the certified path."

2. **scopes.md** — Update Phase Order entry 6 to `Deferred — see BUG-015-001`. Update the Scope Summary table row 6 `Status` to `Deferred`. Replace the entire `## Scope 06: API Client (Opt-In)` block with a `Deferred — see BUG-015-001` block that carries the rationale, the back-link, and a single explanatory line replacing the prior DoD list. The DoD list is intentionally removed (not commented-out) because keeping `[ ] VERIFIED FAIL` lines was the original credibility regression — the audit trail lives in report.md / scenario-manifest / git history instead.

3. **uservalidation.md** — Convert item 13 to `- ~~[ ] Optional API polling respects free-tier rate limits~~ — **Deferred (BUG-015-001)**` plus a one-line evidence block pointing to the bug packet. Update the `Validation Disposition` section to: "13 of 13 acceptance items resolved — 12 verified-pass against running code + 127 PASS unit tests, 1 deferred per BUG-015-001 (option (b) selected). Spec status promoted from `in_progress` → `done`."

4. **state.json** — Atomically transition:
   - `status: "in_progress"` → `"done"`
   - `certification.status: "in_progress"` → `"done"`, `certifiedAt: "2026-04-26T...Z"`, `certifiedBy: "bubbles.workflow"`
   - `execution.currentPhase: "validate"` → `"finalize"`, `execution.currentScope: "scope-06"` → `null`
   - `execution.completedScopes`: stays scope-01..05
   - `execution.completedPhaseClaims`: append `validate`, `audit`, `finalize` (already present from prior promotion; verify ordering stable)
   - `certification.completedScopes`: scope-01..05
   - `certification.scopeProgress[scope=6].status: "Reopened"` → `"Deferred"`; add `deferredReason` (option-(b) rationale) and `deferredAt` (2026-04-26)
   - Move existing `certification.reopenReason` into a `priorReopens` array for history; clear top-level `reopenReason`
   - Add `resolvedBugs: [{ bugId: "BUG-015-001", title, resolution: "deprecated", resolvedAt, link, priorReopens }]`
   - `executionHistory`: append entry for the bugfix-fastlane resolution
   - `lastUpdatedAt`: 2026-04-26 timestamp

5. **report.md** — Append `## BUG-015-001 Deprecation Resolution (2026-04-26)` with the option-(b) selection, files modified, governance/test evidence, and a back-link.

6. **scenario-manifest.json** (new, parent feature) — Map the 6 currently-locked scope-level scenarios (SCN-TW-ARC-001/002 → scope 1, SCN-TW-THR-001/002 → scope 2, SCN-TW-CONN-001/002 → scope 4) so the traceability-guard scenario-manifest cross-check stops failing on missing manifest. SCN-TW-005 (the API scenario) is intentionally excluded because it is now in the Deferred section, not in active scopes.

7. **Bug packet** (this folder) — bug.md, spec.md, design.md, scopes.md, report.md, uservalidation.md, scenario-manifest.json, state.json. Mirrors the BUG-006 packet shape so artifact-lint and traceability-guard pass against the bug folder if invoked separately.

### Alternative Approaches Considered
1. **Option (a) — Implement the API path.** Rejected per user decision: X v2 free-tier eligibility for bookmarks/likes endpoints is unstable, the archive path already covers the real user value, and adversarial coverage for OAuth2/PKCE/rate-limit/hybrid-merge would be heavy maintenance.
2. **Delete R-008 and SCN-TW-005 entirely.** Rejected: violates the user instruction "Preserve historical context (do not delete; mark as deferred with rationale)". Strikethrough/section relocation preserves the audit trail.
3. **Mark Scope 6 `Skipped` or `N/A`.** Rejected: those are not canonical scope statuses (the artifact-lint state-transition guard mechanically rejects non-canonical statuses). `Deferred` is canonical and well-precedented in other deprecation packets.
4. **Remove `bearer_token` / `api_enabled` from `config/smackerel.yaml` and `parseTwitterConfig`.** Rejected: that crosses the "documentation/spec-amendment only" boundary set by the user. Config-field cleanup, if desired, will be a separate spec.

### Affected Files
- `specs/015-twitter-connector/spec.md` — relocate R-008 + SCN-TW-005 to Deferred section; reword goal #5; add API-strategy banner.
- `specs/015-twitter-connector/scopes.md` — Scope 6 → Deferred block.
- `specs/015-twitter-connector/uservalidation.md` — item 13 → strikethrough non-applicable; disposition updated.
- `specs/015-twitter-connector/state.json` — promote status, defer scope-6, add resolvedBugs, preserve priorReopens.
- `specs/015-twitter-connector/report.md` — append deprecation resolution section.
- `specs/015-twitter-connector/scenario-manifest.json` — new file, 6 scenarios.
- `specs/015-twitter-connector/bugs/BUG-015-001-api-path-deprecated/{bug,spec,design,scopes,report,uservalidation}.md`, `scenario-manifest.json`, `state.json` — new bug packet.

No files under `internal/connector/twitter/`, `cmd/core/`, `config/smackerel.yaml`, or `docker-compose.yml` are modified.

### Regression Test Design
- Pre-fix: `cat specs/015-twitter-connector/state.json | jq .status` → `"in_progress"`; `bash artifact-lint.sh` PASS but state mismatched reality; `bash traceability-guard.sh` FAIL on missing scenario-manifest.
- Post-fix: state.json status `done`; both governance scripts exit 0; spec.md / scopes.md / uservalidation.md show Deferred markers and BUG-015-001 back-links.
- Adversarial: `grep -n '\[ \] VERIFIED FAIL' specs/015-twitter-connector/scopes.md specs/015-twitter-connector/uservalidation.md` returns 0 matches (would fail if the deprecation reverted).
- Production-safety adversarial: `git diff --name-only HEAD` shows no files under `internal/connector/twitter/` (would fail if a code change leaked into this documentation-only fix).
- Unit-test adversarial: `./smackerel.sh test unit` is green (would fail if any production code were touched).
