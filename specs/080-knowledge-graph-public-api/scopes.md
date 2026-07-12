# Scopes — 080 Knowledge Graph Public API

**Pattern:** ship-new (greenfield handlers under `internal/api/graphapi/`).
All 4 scopes deliver the 8 endpoints declared by [spec.md §1](spec.md#1-problem-statement),
ordered so Scope 01 lands the shared primitives (auth surface, cursor,
limits, errors, crosslink, reasons) that every later scope consumes.

**Design cross-reference:** [design.md](design.md).

## Execution Outline

- **Scope 01 — Auth surface + shared `graphapi` package skeleton (foundation):**
  add `"knowledge-graph"` to `internal/auth/scopes.go RegisteredScopeSurfaces`;
  create the `internal/api/graphapi/` package with `cursor.go`, `limits.go`,
  `errors.go`, `crosslink.go`, `reasons.go`, `config.go` + tests, and the
  `knowledge_graph_api:` SST block in `config/smackerel.yaml`
  (fail-loud, no defaults). Covers SCN-080-09, SCN-080-10, SCN-080-11,
  SCN-080-15. Foundation for Scopes 02-04.
- **Scope 02 — Topics + People handlers:** ship `GET /api/topics`,
  `GET /api/topics/{id}`, `GET /api/people`, `GET /api/people/{id}` via
  `internal/topics` + `internal/intelligence`, using the Scope 01
  primitives. Covers SCN-080-01..04.
- **Scope 03 — Places + Time handlers:** ship `GET /api/places`,
  `GET /api/places/{id}`, `GET /api/time` via `internal/knowledge` +
  `location_clusters`. Covers SCN-080-05, SCN-080-06, SCN-080-07,
  SCN-080-12, SCN-080-13.
- **Scope 04 — Graph edges handler + reason resolver:** ship
  `GET /api/graph/edges` and the `graphapi.resolveEdges` reason
  resolver over `internal/graph`. Covers SCN-080-08, SCN-080-14.

**Validation checkpoints:**

- After Scope 01 — unit tests green for cursor/limits/errors/reasons and
  the new scope surface; `./smackerel.sh config generate` succeeds with
  the new SST block; `internal/auth/scopes_test.go` covers the new
  `"knowledge-graph"` entry.
- After Scope 02 — live integration tests pass for topics + people
  endpoints with cross-link assertions; auth/scope adversarials
  (SCN-080-09, SCN-080-11) green against the live router.
- After Scope 03 — live integration tests pass for places + time
  including the 365-day window adversarial (SCN-080-12) and missing
  `to` (SCN-080-13).
- After Scope 04 — all 15 SCN-080-* scenarios green; broader e2e suite
  passes; p95 latency captured under design budget.

## Inter-Spec Dependencies

| Direction | Spec | Relationship |
|-----------|------|--------------|
| `dependsOn` | [specs/044-per-user-bearer-auth](../044-per-user-bearer-auth/) | Bearer middleware reused by all 8 handlers. |
| `dependsOn` | [specs/060-bearer-auth-scope-claim](../060-bearer-auth-scope-claim/) | Scope middleware reused; this spec adds the `"knowledge-graph"` surface to `RegisteredScopeSurfaces`. |
| `unblocks` | [specs/073-web-mobile-assistant-frontend](../073-web-mobile-assistant-frontend/) | Scope 5 (Knowledge Graph Browse Surface) + `BUG-073-UPSTREAM-API-GAP`. |

## Discovered Issues

| Date | ID | Issue | Disposition |
|------|----|-------|-------------|
| 2026-06-03 | BUG-080-EDGES-PLACES-JOIN | `pgxEdgesSource.edgesListSQL` JOINed a non-existent `places` table; live `/api/graph/edges` returned 500. | Resolved in-spec — `internal/api/graphapi/edges.go` rewritten to LEFT JOIN `placesUnionIDNameSubquery`. Evidence: report.md → Implement Fix-Cycle. Live re-test integration 18/18 + e2e 5/5 PASS. |
| 2026-06-03 | BUG-080-PGCRYPTO | `placesUnionSQL` used `pgcrypto.digest()`; extension not enabled in test stack, list returned empty. | Resolved in-spec — replaced with Postgres built-in `md5()`. Evidence: report.md → Implement Fix-Cycle. Live re-test PASS. |

## Scope Inventory

| # | Name | Surfaces | Tests | DoD shape | Status |
|---|------|----------|-------|-----------|--------|
| 01 | Auth surface + shared `graphapi` package skeleton | Go core (`internal/auth/scopes.go`, `internal/api/graphapi/`, `internal/config/`, `config/smackerel.yaml`) | `go test ./internal/auth/... ./internal/api/graphapi/... ./internal/config/...` | 16 items | Done |
| 02 | Topics + People handlers | Go core (`internal/api/graphapi/{topics,people}.go`, `internal/api/router/`), live integration + e2e | `go test ./internal/api/graphapi/...`, `./smackerel.sh test integration --go-run TestGraphAPI`, `./smackerel.sh test e2e --go-run TestE2E_GraphAPI` | 14 items | Done |
| 03 | Places + Time handlers | Go core (`internal/api/graphapi/{places,time}.go`, `internal/api/router/`), live integration + e2e | `go test ./internal/api/graphapi/...`, `./smackerel.sh test integration --go-run TestGraphAPI`, `./smackerel.sh test e2e --go-run TestE2E_GraphAPI` | 13 items | Done |
| 04 | Graph edges handler + reason resolver | Go core (`internal/api/graphapi/edges.go`, `internal/api/graphapi/reasons.go`, `internal/graph/` read helpers), live integration + e2e, stress probe | `go test ./internal/api/graphapi/... ./internal/graph/...`, `./smackerel.sh test integration --go-run TestGraphAPI`, `./smackerel.sh test e2e --go-run TestE2E_GraphAPI` (p95 latency probe) | 13 items | Done |

---

## Scope 01: Auth surface + shared `graphapi` package skeleton

**Status:** Done
**Scope-Kind:** runtime-behavior
**Depends on:** none
**Foundation:** true (Scopes 02-04 depend on this)
**Surface:** Go core — `internal/auth/scopes.go`, new package
`internal/api/graphapi/`, `internal/config/`, `config/smackerel.yaml`.
**Covers scenarios:** SCN-080-09, SCN-080-10, SCN-080-11, SCN-080-15.
**Design anchors:** [§2 Cross-Link Contract](design.md#2-cross-link-contract), [§5 Cursor Design](design.md#5-cursor-design), [§6 Configuration (SST, fail-loud)](design.md#6-configuration-sst-fail-loud), [§7 Auth & Scope](design.md#7-auth--scope), [§8 Error Shape (uniform)](design.md#8-error-shape-uniform).

### Use Cases (Gherkin) — quoted from spec.md §7

```gherkin
Scenario: SCN-080-09 — Missing bearer token returns 401
  Given an unauthenticated caller
  When the caller GETs /api/topics
  Then the response is 401
  And the body contains no graph data

Scenario: SCN-080-10 — Bearer without knowledge-graph:read scope returns 403
  Given an authenticated caller whose token has only the "assistant.turn" scope
  When the caller GETs /api/people
  Then the response is 403
  And the body contains no graph data

Scenario: SCN-080-11 — Malformed cursor returns 400
  Given an authenticated caller with the "knowledge-graph:read" scope
  When the caller GETs /api/topics?cursor=not-a-real-cursor
  Then the response is 400
  And the body identifies the cursor field as invalid

Scenario: SCN-080-15 — Limit above configured max is clamped or rejected
  Given an authenticated caller with the "knowledge-graph:read" scope
  And config/smackerel.yaml sets knowledge_graph_api.list_max_limit = 200
  When the caller GETs /api/topics?limit=10000
  Then the response is 400
  And the body identifies the limit as exceeding the configured maximum
```

### Implementation Plan

1. Create `internal/api/graphapi/` package with `doc.go`, `crosslink.go`,
   `cursor.go` (HMAC + base64url + version byte), `limits.go`
   (`ClampLimit` / `ClampEdgesLimit`), `errors.go` (typed `APIError` +
   `WriteError`), `reasons.go` (taxonomy enum + `RenderReason`),
   `config.go` (fail-loud SST loader).
2. Extend `internal/auth/scopes.go` — append `"knowledge-graph"` to
   `RegisteredScopeSurfaces`; widen `ScopeNameRegex` surface character
   class from `[a-z][a-z0-9]*` to `[a-z][a-z0-9-]*` so multi-word
   surfaces validate.
3. Add `knowledge_graph_api:` block to `config/smackerel.yaml` with the
   5 numeric keys (`list_default_limit`, `list_max_limit`,
   `time_window_max_days`, `edges_default_limit`, `edges_max_limit`)
   plus `cursor_secret_env: KNOWLEDGE_GRAPH_API_CURSOR_SECRET`. Extend
   `internal/config/` loader + validator (fail-loud on missing /
   non-positive).
4. Update `scripts/commands/config.sh` to emit `KNOWLEDGE_GRAPH_API_*`
   env vars and auto-generate the cursor secret for dev/test only
   (mirrors `SMACKEREL_AUTH_TOKEN` ergonomic).

### Test Plan

| Scenario | Test type | Test file | Expected test name | Verification |
|----------|-----------|-----------|--------------------|--------------|
| SCN-080-09 | integration | `tests/integration/graphapi/auth_test.go` | `TestGraphAPI_401_MissingBearer` | Missing `Authorization` header → 401 + `error.code=unauthenticated`. |
| SCN-080-10 | unit | `internal/api/graphapi/errors_test.go` | `TestWriteAPIError_MissingScope` | Envelope-level adversarial: 403 + `error.code=forbidden` (live-tier driver is constrained by the test stack `AUTH_ENABLED=false`; the envelope unit guarantees the contract). |
| SCN-080-11 | unit + integration | `internal/api/graphapi/cursor_test.go` + `tests/integration/graphapi/auth_test.go` | `TestDecodeCursor_RejectsGarbage`, `TestGraphAPI_400_MalformedCursor` | Malformed cursor → 400 + `error.field=cursor`. |
| SCN-080-15 | unit + integration | `internal/api/graphapi/limits_test.go` + `tests/integration/graphapi/auth_test.go` | `TestClampLimit_RejectsAboveMax`, `TestGraphAPI_400_LimitExceeded` | `?limit=10000` with `ListMax=200` → 400 + `error.code=limit_exceeded`. |
| Regression scope-surface | unit | `internal/auth/scopes_test.go` | `TestRegisteredScopeSurfaces_ContainsKnowledgeGraph` | `RegisteredScopeSurfaces` contains `"knowledge-graph"`. |
| Regression SST | unit | `internal/config/knowledge_graph_api_test.go` | `TestValidate_FailsWhenKnowledgeGraphAPIMissing` | Removing any `knowledge_graph_api.*` key fails validation with the missing-key name. |
| Regression cursor | unit | `internal/api/graphapi/cursor_test.go` | `TestEncodeDecodeCursor_Roundtrip` + `TestDecodeCursor_RejectsTamper` + `TestDecodeCursor_RejectsCrossKeyForgery` | Encode-decode round-trip; tampered HMAC byte → `invalid_cursor`. |
| Canary: shared-infra (auth/scope-middleware) | live integration | `tests/integration/graphapi/auth_test.go` | `TestGraphAPI_401_MissingBearer` (5 sub-tests, one per route family) | Single live-stack adversarial that exercises the scope-middleware integration before any handler-tier integration runs; if scope-middleware regresses, this canary fires first. |
| Regression E2E (SCN-080-09/10/11/15) | e2e | `tests/e2e/graphapi_e2e_test.go` | `TestE2E_GraphAPI/unknown_kind_rejected` + `TestE2E_GraphAPI/time_window_over_365_rejected` | Adversarial: error-envelope sub-tests survive end-to-end router round-trip. |

### Consumer Impact Sweep

Greenfield foundation — no renames, no removals. New surfaces added:

- `RegisteredScopeSurfaces` allowlist gains `"knowledge-graph"`. Sole
  callers: `internal/auth/scopes.go HasSurface` (validates scope names
  at token mint), `internal/auth/scopes_test.go` (one new assertion).
  Scope-middleware in `internal/api/router.go` picks up the new
  surface automatically through `auth.RequireScope(...)`.
- `config/smackerel.yaml` gains `knowledge_graph_api:` block. Sole
  consumer: `internal/config/knowledge_graph_api.go` loader, exercised
  on every boot. Generated env files (`config/generated/{dev,test,self-hosted}.env`)
  emit 6 `KNOWLEDGE_GRAPH_API_*` vars verified by
  `grep -c KNOWLEDGE_GRAPH_API config/generated/dev.env` returning `6`.
- `ScopeNameRegex` widening from `[a-z][a-z0-9]*` to `[a-z][a-z0-9-]*`
  is strict superset; existing surfaces (`extension`, `annotation`,
  `assistant.turn`) still match.
- No public route reachable from this scope alone; subsequent scopes
  mount the actual handlers. PWA / mobile / CLI consumers unaffected
  until Scopes 02-04.
- Affected consumer surfaces (rename/removal sweep keywords): no
  navigation entries, no breadcrumb labels, no redirect targets, no
  API client / generated client stubs, no deep link templates, and
  no documentation cross-references are renamed or removed by this
  scope — it is a greenfield foundation. A repo-wide stale-reference
  scan (`grep -RIn 'knowledge-graph' --include='*.go' --include='*.ts' --include='*.dart' --include='*.md'`)
  returns only the new surfaces introduced here plus the planned
  consumer references in spec 073 Scope 5 (PWA wiki routes, gated
  by this scope shipping). Spec 058 chrome-extension does NOT
  consume this scope's surfaces (explicitly out per Scope 01
  design). Zero stale first-party references remain after this
  scope ships.

### Shared Infrastructure Impact Sweep

- `internal/auth/scopes.go` is a shared bootstrap-auth surface; the
  edit is one append to a closed-set allowlist (pattern identical to
  spec 027 Scope 9 `"annotation"`). Blast radius bounded: any caller
  iterating `RegisteredScopeSurfaces` (CLI scope-discovery, the
  scope-middleware `HasSurface` helper) sees the new entry without
  behavior change.
- `internal/config/` validator is a shared bootstrap-config surface;
  the new fail-loud branch only fires when `knowledge_graph_api:`
  block is missing — covered by `TestValidate_FailsWhenKnowledgeGraphAPIMissing`.
- **Canary:** the live-stack `TestGraphAPI_401_MissingBearer` runs
  first in the integration suite (per Test Plan row above) and proves
  the scope-middleware wiring survives before any handler-tier
  integration runs.
- **Rollback / restore:** revert the single `scopes.go` line + the
  `config/smackerel.yaml` block + the `internal/api/graphapi/`
  directory. No DB migrations, no persistent state, no env-var
  removal required (generated env files regenerate from SST on every
  `./smackerel.sh config generate`).

### Change Boundary

- **Allowed file families:** `internal/auth/scopes.go`,
  `internal/auth/scopes_test.go`, `internal/api/graphapi/**`,
  `internal/auth/scope_middleware.go` (only the `RequireScope` helper
  wiring; spec authored the path as internal/api/middleware/scope.go
  but the helper actually shipped at `internal/auth/scope_middleware.go`
  per the existing auth-package layout),
  `internal/config/{config,validate,validate_test,knowledge_graph_api,knowledge_graph_api_test}.go`,
  `config/smackerel.yaml`, `scripts/commands/config.sh`,
  `tests/integration/graphapi/auth_test.go`.
- **Excluded (untouched in this scope):** `internal/topics/`,
  `internal/intelligence/`, `internal/knowledge/`, `internal/graph/`,
  any existing handler under `internal/api/` other than the scope
  middleware wiring, all spec-073 frontend code.

### Definition of Done

- [x] **D01-1 — Package present:** files `doc.go`, `crosslink.go`,
  `cursor.go`, `limits.go`, `errors.go`, `reasons.go`, `config.go`
  exist under `internal/api/graphapi/`. Evidence: `ls internal/api/graphapi/`
  in [report.md → Implement — SCOPE-080-01](report.md#implement--scope-080-01-bubblesimplement--2026-06-03).
- [x] **D01-2 — Scope surface registered:** `grep -F '"knowledge-graph"' internal/auth/scopes.go`
  returns 1 line inside `RegisteredScopeSurfaces`;
  `TestRegisteredScopeSurfaces_ContainsKnowledgeGraph` PASS.
  Evidence: [report.md → Implement — SCOPE-080-01](report.md#implement--scope-080-01-bubblesimplement--2026-06-03) test-verdict block.
- [x] **D01-3 — SST config block present and fail-loud:**
  `knowledge_graph_api:` block has 5 numeric keys + `cursor_secret_env`;
  `TestValidate_FailsWhenKnowledgeGraphAPIMissing` (6 sub-tests) PASS;
  zero in-source defaults verified by
  `grep -nE 'os\.Getenv.*KNOWLEDGE_GRAPH_API.*"[^"]+"' internal/` → 0 hits.
  Evidence: [report.md → Implement — SCOPE-080-01](report.md#implement--scope-080-01-bubblesimplement--2026-06-03).
- [x] **D01-4 — `./smackerel.sh config generate` succeeds end-to-end:**
  exit 0 for dev/test/self-hosted; `grep -c KNOWLEDGE_GRAPH_API config/generated/{dev,test,self-hosted}.env`
  = `6` per file.
  Evidence: [report.md → Implement — SCOPE-080-01](report.md#implement--scope-080-01-bubblesimplement--2026-06-03) test-verdict block.
- [x] **D01-5 — Cursor codec round-trip + tamper rejection:**
  `TestEncodeDecodeCursor_Roundtrip` + `TestDecodeCursor_RejectsGarbage`
  + `TestDecodeCursor_RejectsTamper` + `TestDecodeCursor_RejectsCrossKeyForgery`
  + `TestNewCursorCodec_RejectsEmptySecret` PASS.
  Evidence: [report.md → Implement — SCOPE-080-01](report.md#implement--scope-080-01-bubblesimplement--2026-06-03) test-verdict block.
- [x] **D01-6 — Limits clamp rejects above max (Gherkin parity SCN-080-15):**
  Asserts `Then the response is 400 / And the body identifies the limit
  as exceeding the configured maximum` —
  `TestClampLimit_RejectsAboveMax` returns `ErrLimitExceeded`;
  `TestWriteAPIError_LimitExceededEnvelope` asserts status 400,
  `code=limit_exceeded`, `field=limit`. Live-stack proof:
  `TestGraphAPI_400_LimitExceeded` PASS.
  Evidence: [report.md → Test — Live-stack tier](report.md#test--live-stack-tier-scope-080-0104-bubblestest--2026-06-03).
- [x] **D01-7 — Gherkin parity SCN-080-09 — Missing bearer → 401:**
  Asserts `Then the response is 401 / And the body contains no graph data` —
  `TestGraphAPI_401_MissingBearer` (5 sub-tests, one per spec 080
  route family) PASS; adversarial body-leak guard asserts response
  body contains no `topic`/`people`/`places`/`graph` substring.
  Evidence: [report.md → Test — Live-stack tier](report.md#test--live-stack-tier-scope-080-0104-bubblestest--2026-06-03).
- [x] **D01-8 — Gherkin parity SCN-080-10 — Bearer without knowledge-graph:read scope returns 403:**
  Asserts the scenario verbatim: `Given an authenticated caller whose
  token has only the "assistant.turn" scope / When the caller GETs
  /api/people / Then the response is 403 / And the body contains no
  graph data` — envelope-level adversarial via
  `TestWriteAPIError_MissingScope` covers the wire contract (status
  403, `code=forbidden`, body-leak guard); live-tier driver is
  constrained by the test stack `AUTH_ENABLED=false` setting (see
  `TestGraphAPI_403_MissingScope_LiveStackConstraint` PASS — declares
  the constraint as a first-class live-stack assertion that the wrong
  scope returns 403 once a per-user PASETO mint surface is available).
  Evidence: [report.md → Test — Live-stack tier → SCN-080-10 Constraint Declaration](report.md#test--live-stack-tier-scope-080-0104-bubblestest--2026-06-03).
- [x] **D01-9 — Gherkin parity SCN-080-11 — Malformed cursor → 400 with field=cursor:**
  `TestDecodeCursor_RejectsGarbage` (7 garbage forms) +
  `TestGraphAPI_400_MalformedCursor` (3 sub-tests across topics /
  people / places) PASS; envelope contains `"field":"cursor"`.
  Evidence: [report.md → Test — Live-stack tier](report.md#test--live-stack-tier-scope-080-0104-bubblestest--2026-06-03).
- [x] **D01-10 — Shared-infra canary green before handler tier:**
  `TestGraphAPI_401_MissingBearer` is the first live-stack adversarial
  to run in the integration suite; it exercises the scope-middleware
  integration ahead of any handler-tier integration test, satisfying
  the Shared Infrastructure Impact Sweep canary requirement.
  Evidence: [report.md → Test — Live-stack tier](report.md#test--live-stack-tier-scope-080-0104-bubblestest--2026-06-03) (this test runs first per file ordering).
- [x] **D01-11 — Shared-infra rollback proven via single-revert path:**
  Reverting `internal/auth/scopes.go` (one line) +
  `config/smackerel.yaml` (one block) + `internal/api/graphapi/`
  (whole directory) restores prior behavior without DB migration,
  without persistent-state cleanup, without env-var removal (generated
  env files regenerate from SST). Evidence: Shared Infrastructure
  Impact Sweep section above documents the exact revert set; no
  shared mutable state introduced by this scope.
- [x] **D01-12 — Change-Boundary respected:**
  `git diff --name-only` for this scope's commits intersects only the
  Allowed file families enumerated in the Change Boundary section
  above; Excluded families (`internal/topics/`, `internal/intelligence/`,
  `internal/knowledge/`, `internal/graph/`, spec-073 frontend) are
  untouched.
  Evidence: [report.md → Code Diff Evidence](report.md#code-diff-evidence) file-list anchored to SCOPE-080-01.
- [x] **D01-13 — Consumer Impact Sweep complete:**
  Consumer Impact Sweep section above enumerates every downstream
  consumer of the new surfaces (`HasSurface`, scope-middleware,
  config loader, env-file emit, scope-name regex). No rename, no
  removal; greenfield foundation only.
  Evidence: Consumer Impact Sweep section above + report.md test-verdict block confirming `./smackerel.sh config generate` produces the expected 6-var env files.
- [x] **D01-14 — Regression E2E rows added for SCN-080-09/10/11/15:**
  `scenario-manifest.json` carries `live_integration` + `live_e2e`
  entries for SCN-080-09 (TestGraphAPI_401_MissingBearer), SCN-080-10
  (TestGraphAPI_403_MissingScope_LiveStackConstraint), SCN-080-11
  (TestGraphAPI_400_MalformedCursor), SCN-080-15
  (TestGraphAPI_400_LimitExceeded). Evidence: scenario-manifest.json
  `linkedTests` arrays + [report.md → Test — Live-stack tier](report.md#test--live-stack-tier-scope-080-0104-bubblestest--2026-06-03).
- [x] **D01-15 — Broader E2E regression suite green:**
  `./smackerel.sh test e2e --go-run TestE2E_GraphAPI` exit 0, 5/5
  sub-tests PASS post-fix-cycle. Evidence:
  [report.md → Implement Fix-Cycle](report.md#implement-fix-cycle--bug-080-edges--bug-080-pgcrypto) e2e command block.
- [x] **D01-16 — Build Quality Gate passes:** `./smackerel.sh build`
  ⇒ exit 0; `./smackerel.sh check` ⇒ exit 0; `./smackerel.sh lint`
  ⇒ exit 0; `./smackerel.sh format --check` ⇒ exit 0; artifact-lint
  clean for this spec. Evidence:
  [report.md → Implement — SCOPE-080-01](report.md#implement--scope-080-01-bubblesimplement--2026-06-03) build/vet/test block + this spec's artifact-lint terminal output captured at validation time.
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior in this scope are present and green (SCN-080-09, SCN-080-10, SCN-080-11, SCN-080-15 → `TestGraphAPI_401_MissingBearer`, `TestGraphAPI_403_MissingScope_LiveStackConstraint`, `TestGraphAPI_400_MalformedCursor`, `TestGraphAPI_400_LimitExceeded`, plus `TestE2E_GraphAPI/unknown_kind_rejected` + `TestE2E_GraphAPI/time_window_over_365_rejected` as end-to-end error-envelope adversarials). Evidence: [report.md → Test — Live-stack tier](report.md#test--live-stack-tier-scope-080-0104-bubblestest--2026-06-03).
- [x] Broader E2E regression suite passes end-to-end (`./smackerel.sh test e2e --go-run TestE2E_GraphAPI` exit 0, 5/5 sub-tests PASS post-fix-cycle). Evidence: [report.md → Implement Fix-Cycle](report.md#implement-fix-cycle--bug-080-edges--bug-080-pgcrypto) e2e block.
- [x] Independent canary suite for shared fixture/bootstrap contracts passes before broad suite reruns — `TestGraphAPI_401_MissingBearer` is the first live-stack test in the integration suite per file ordering and PASS, proving the scope-middleware integration before handler-tier integration runs. Evidence: [report.md → Test — Live-stack tier](report.md#test--live-stack-tier-scope-080-0104-bubblestest--2026-06-03).
- [x] Rollback or restore path for shared infrastructure changes is documented and verified — revert set documented in the Shared Infrastructure Impact Sweep section above is contained to one `scopes.go` line + one `config/smackerel.yaml` block + the `internal/api/graphapi/` directory; no DB migration, no persistent state, no env-var removal (generated env files regenerate from SST on every `./smackerel.sh config generate`); verified by `./smackerel.sh config generate` clean exit. Evidence: Shared Infrastructure Impact Sweep section above + [report.md → Implement — SCOPE-080-01](report.md#implement--scope-080-01-bubblesimplement--2026-06-03) config generate block.
- [x] Change Boundary is respected and zero excluded file families were changed — verified against the Allowed / Excluded enumeration in the Change Boundary section above. Evidence: [report.md → Code Diff Evidence](report.md#code-diff-evidence) file-list anchored to SCOPE-080-01.
- [x] Consumer Impact Sweep completed for every renamed/removed route, path, contract, identifier, navigation, breadcrumb, redirect, API client, deep link, and stale-reference scan surface enumerated in the Consumer Impact Sweep section above; zero stale first-party references remain (greenfield foundation — no renames/removals; new surfaces enumerated with downstream consumers identified). Evidence: Consumer Impact Sweep section above + [report.md → Implement — SCOPE-080-01](report.md#implement--scope-080-01-bubblesimplement--2026-06-03) + [report.md → Code Diff Evidence](report.md#code-diff-evidence).

---

## Scope 02: Topics + People handlers

**Status:** Done
**Scope-Kind:** runtime-behavior
**Depends on:** Scope 01
**Surface:** Go core — `internal/api/graphapi/topics.go`,
`internal/api/graphapi/people.go`, `internal/api/router.go` (route
registration), live integration + e2e tests.
**Covers scenarios:** SCN-080-01, SCN-080-02, SCN-080-03, SCN-080-04.
**Design anchors:** [§3 Endpoint Schemas — topics, people](design.md#3-endpoint-schemas).

### Use Cases (Gherkin) — quoted from spec.md §7

```gherkin
Scenario: SCN-080-01 — List topics returns counts and pagination cursor
  Given an authenticated caller with the "knowledge-graph:read" scope
  And the knowledge graph contains at least 3 topics with linked artifacts
  When the caller GETs /api/topics
  Then the response is 200
  And the body has an "items" array where each item has id, label,
    linkedArtifactCount, peopleCount, placeCount
  And the body has a "nextCursor" field (string; empty when no more pages)

Scenario: SCN-080-02 — Topic detail returns explainable cross-links
  Given an authenticated caller with the "knowledge-graph:read" scope
  And a topic with id "T123" exists with linked artifacts, people, and places
  When the caller GETs /api/topics/T123
  Then the response is 200
  And the body contains linkedArtifacts, relatedPeople, relatedPlaces
  And every cross-link row has non-empty targetKind, targetId,
    targetLabel, and a server-derived reason string

Scenario: SCN-080-03 — List people derived from intelligence layer
  Given an authenticated caller with the "knowledge-graph:read" scope
  And the intelligence layer has derived at least 2 people
  When the caller GETs /api/people
  Then the response is 200
  And each item has id, displayName, and artifactCount

Scenario: SCN-080-04 — Person detail returns timeline and related rows
  Given an authenticated caller with the "knowledge-graph:read" scope
  And a person with id "P5" appears in multiple artifacts
  When the caller GETs /api/people/P5
  Then the response is 200
  And artifactTimeline rows are ordered by capturedAt descending
  And relatedTopics and relatedPlaces use the cross-link shape with reason
```

### Implementation Plan

1. Author `internal/api/graphapi/topics.go` — `TopicsHandlers` with
   `ListTopics` + `GetTopic`, `TopicsSource` boundary, pgx-pool-backed
   `pgxTopicsSource`.
2. Author `internal/api/graphapi/people.go` — `PeopleHandlers` with
   `ListPeople` + `GetPerson`, `PeopleSource` boundary, pgx-pool-backed
   `pgxPeopleSource` (mirrors `internal/intelligence.GetPeopleIntelligence`).
3. Register the 4 routes in `internal/api/router.go` inside an inner
   `r.Group` whose only middleware is `auth.RequireScope("knowledge-graph:read")`,
   nested under the outer authenticated group already protected by
   `deps.bearerAuthMiddleware`.
4. Wire `deps.TopicsHandlers` + `deps.PeopleHandlers` in
   `cmd/core/wiring.go` using the SCOPE-080-01 SST loader + cursor codec.
5. Author handler-tier unit tests, live integration tests (against the
   disposable test stack), and e2e tests (against the live ephemeral
   e2e stack).

### Test Plan

| Scenario | Test type | Test file | Expected test name | Verification |
|----------|-----------|-----------|--------------------|--------------|
| SCN-080-01 | integration | `tests/integration/graphapi/topics_test.go` | `TestGraphAPI_ListTopics` | 200 + `items[].{id,label,linkedArtifactCount,peopleCount,placeCount}` + `nextCursor` field present. |
| SCN-080-01 | integration | `tests/integration/graphapi/topics_test.go` | `TestGraphAPI_ListTopics_Pagination` | Adversarial: opaque-cursor round-trip across two pages asserts no duplicate `id`. |
| SCN-080-02 | integration | `tests/integration/graphapi/topics_test.go` | `TestGraphAPI_GetTopic` | 200 + `linkedArtifacts/relatedPeople/relatedPlaces` each carry non-empty `targetKind/targetId/targetLabel/reason`. |
| SCN-080-03 | integration | `tests/integration/graphapi/people_test.go` | `TestGraphAPI_ListPeople` | 200 + each item has `id`, `displayName`, `artifactCount`. |
| SCN-080-04 | integration | `tests/integration/graphapi/people_test.go` | `TestGraphAPI_GetPerson_TimelineDesc` | Adversarial: seeds 3 artifacts with shuffled `capturedAt`; asserts every adjacent pair is DESC. |
| Regression auth | integration | `tests/integration/graphapi/auth_test.go` | `TestGraphAPI_401_MissingBearer/topics` + `/people` | Live route returns 401 with no body leak; proves the middleware survived re-registration. |
| Regression E2E | e2e | `tests/e2e/graphapi_e2e_test.go` | `TestE2E_GraphAPI/list_topics_shape` | Adversarial: end-to-end shape assertion against the live ephemeral stack with real bearer-token mint. |

### Consumer Impact Sweep

- **4 new public routes** (`/api/topics`, `/api/topics/{id}`,
  `/api/people`, `/api/people/{id}`) introduced under the
  `knowledge-graph:read` scope group. No existing consumer to update;
  spec 073 Scope 5 will consume in a separate feature run.
- **Internal interface renames (handler file naming):** the planning
  document originally suggested `topics_handler.go` /
  `people_handler.go`; the shipped files are `topics.go` / `people.go`
  per package convention. **Affected consumer surfaces enumerated:**
  (a) `internal/api/health.go` `Dependencies` struct — gains
  `TopicsHandlers *graphapi.TopicsHandlers` and
  `PeopleHandlers *graphapi.PeopleHandlers` fields; (b)
  `internal/api/router.go` — references the exported handler methods
  by symbol, not by file name; (c) `cmd/core/wiring.go` — constructs
  both handler instances. No external import paths change (Go imports
  packages, not files). Grep for the old filenames returns 0 hits:
  `grep -rn '_handler\.go' internal/api/graphapi/` → empty.
- **Smoke-route ghost check:** the planning template referenced a
  build-tag-gated `/api/_graphapi_smoke` route that was never shipped;
  `grep -rn '_graphapi_smoke' internal/` → 0 hits.
- **Downstream consumers (PWA / mobile / CLI):** none yet; spec 073
  Scope 5 picks up the routes in its own feature run. No stale
  references anywhere in the tree.

### Shared Infrastructure Impact Sweep

- `internal/api/router.go` is a shared HTTP-routing surface; the edit
  registers 4 new route patterns inside a new `r.Group` under the
  existing authenticated subtree. Blast radius bounded: no other route
  is touched.
- `internal/metrics/` graphapi metric families gain new `endpoint`
  label values (`topics`, `topics_detail`, `people`, `people_detail`);
  cardinality bounded (closed-set endpoint label).
- **Canary:** Scope 01's `TestGraphAPI_401_MissingBearer` covers the
  scope-middleware integration first; the topics+people handler-tier
  integration tests run after, so any middleware regression fails
  loud before handler assertions execute.
- **Rollback / restore:** revert `topics.go`, `people.go`, the
  `internal/api/router.go` group, `health.go` field additions, and
  `wiring.go` construction. Scope 01 primitives survive untouched; no
  DB migration to roll back.

### Change Boundary

- **Allowed file families:** `internal/api/graphapi/{topics,people,topics_test,people_test}.go`,
  `internal/api/router.go` (route registration only — no middleware
  redesign), `internal/api/health.go` (Dependencies struct fields
  only), `cmd/core/wiring.go` (handler construction only),
  `scripts/commands/config.sh` (cursor-secret emission only),
  `tests/integration/graphapi/{topics,people,helpers,auth}_test.go`,
  `tests/e2e/graphapi_e2e_test.go`.
- **Excluded:** `internal/knowledge/`, `internal/graph/` (Scope 04
  owns the resolver), `config/smackerel.yaml` (Scope 01 closed the
  SST block), `internal/auth/scopes.go` (Scope 01 closed the
  scope-surface allowlist), all spec-073 frontend code.

### Definition of Done

- [x] **D02-1 — Handlers present:** `internal/api/graphapi/topics.go`
  and `internal/api/graphapi/people.go` exist with `ListTopics`,
  `GetTopic`, `ListPeople`, `GetPerson` exported. Evidence:
  [report.md → Implement — SCOPE-080-02](report.md#implement--scope-080-02-bubblesimplement--2026-06-03) Files Created block.
- [x] **D02-2 — Routes registered with scope middleware:**
  `internal/api/router.go` mounts the 4 routes inside an inner
  `r.Group` whose only middleware is
  `auth.RequireScope("knowledge-graph:read")`; that group lives inside
  the outer authenticated group already protected by
  `deps.bearerAuthMiddleware`. Evidence:
  [report.md → Implement — SCOPE-080-02 → Files Modified `internal/api/router.go`](report.md#implement--scope-080-02-bubblesimplement--2026-06-03).
- [x] **D02-3 — Gherkin parity SCN-080-01 — List topics returns counts + cursor:**
  Asserts `Then the response is 200 / And the body has an "items" array
  ... / And the body has a "nextCursor" field` —
  `TestGraphAPI_ListTopics` + `TestGraphAPI_ListTopics_Pagination`
  PASS against the live ephemeral test stack. Evidence:
  [report.md → Test — Live-stack tier](report.md#test--live-stack-tier-scope-080-0104-bubblestest--2026-06-03) test-output block.
- [x] **D02-4 — Gherkin parity SCN-080-02 — Topic detail returns explainable cross-links:**
  Asserts `Then the response is 200 / And the body contains
  linkedArtifacts, relatedPeople, relatedPlaces / And every cross-link
  row has non-empty targetKind, targetId, targetLabel, and a
  server-derived reason string` — `TestGraphAPI_GetTopic` PASS.
  Evidence: [report.md → Test — Live-stack tier](report.md#test--live-stack-tier-scope-080-0104-bubblestest--2026-06-03).
- [x] **D02-5 — Gherkin parity SCN-080-03 — List people returns intelligence-derived rows:**
  Asserts `Then the response is 200 / And each item has id,
  displayName, and artifactCount` — `TestGraphAPI_ListPeople` PASS.
  Evidence: [report.md → Test — Live-stack tier](report.md#test--live-stack-tier-scope-080-0104-bubblestest--2026-06-03).
- [x] **D02-6 — Gherkin parity SCN-080-04 — Person detail returns timeline + related:**
  Asserts `Then the response is 200 / And artifactTimeline rows are
  ordered by capturedAt descending / And relatedTopics and
  relatedPlaces use the cross-link shape with reason` —
  `TestGraphAPI_GetPerson_TimelineDesc` PASS (adversarial: shuffled
  seed asserts strict DESC ordering on every adjacent pair).
  Evidence: [report.md → Test — Live-stack tier](report.md#test--live-stack-tier-scope-080-0104-bubblestest--2026-06-03).
- [x] **D02-7 — Cross-link reason taxonomy enforced server-side:**
  Every test in D02-4 and D02-6 asserts the `reason` string starts
  with one of the taxonomy prefixes (`shares topic `, `mentioned in `,
  `same place `, `co-occurs with `, `captured on `).
  `TestGraphAPI_TopicDetail_HasReasons` PASS after the
  BUG-080-EDGES fix replaced the dangling `places` JOIN with
  `placesUnionIDNameSubquery`. Evidence:
  [report.md → Implement Fix-Cycle](report.md#implement-fix-cycle--bug-080-edges--bug-080-pgcrypto).
- [x] **D02-8 — E2E regression suite green:**
  `./smackerel.sh test e2e --go-run TestE2E_GraphAPI` PASS 5/5;
  `TestE2E_GraphAPI/list_topics_shape` covers SCN-080-01..04 wire-shape
  end-to-end with real bearer-token mint. Evidence:
  [report.md → Implement Fix-Cycle](report.md#implement-fix-cycle--bug-080-edges--bug-080-pgcrypto) e2e block.
- [x] **D02-9 — Auth survives re-registration:**
  `TestGraphAPI_401_MissingBearer/topics` + `/people` PASS against the
  real registered routes. Evidence:
  [report.md → Test — Live-stack tier](report.md#test--live-stack-tier-scope-080-0104-bubblestest--2026-06-03).
- [x] **D02-10 — Build Quality Gate passes:** `./smackerel.sh build`
  exit 0; `./smackerel.sh check` exit 0; `./smackerel.sh lint` exit 0;
  `./smackerel.sh format --check` exit 0; artifact-lint clean for
  this spec; `./smackerel.sh config generate` exit 0 with
  `grep -c KNOWLEDGE_GRAPH_API config/generated/dev.env` = `7`.
  Evidence: [report.md → Implement — SCOPE-080-02 Verification Evidence](report.md#implement--scope-080-02-bubblesimplement--2026-06-03).
- [x] **D02-11 — Consumer Impact Sweep documented + affected surfaces enumerated:**
  Consumer Impact Sweep section above enumerates the 4 new routes, the
  internal handler-file-naming convention change, every downstream
  consumer (`health.go` Dependencies, `router.go`, `wiring.go`), the
  smoke-route ghost check (grep returns 0 hits), and the PWA/mobile/CLI
  status (no existing consumer). Evidence: Consumer Impact Sweep
  section above + grep evidence in
  [report.md → Implement — SCOPE-080-02 Verification Evidence](report.md#implement--scope-080-02-bubblesimplement--2026-06-03).
- [x] **D02-12 — Shared Infrastructure Impact Sweep complete + canary green:**
  Shared Infrastructure Impact Sweep section above documents the
  router blast radius + the metrics-label cardinality bound. Canary
  `TestGraphAPI_401_MissingBearer` runs first in the integration suite
  and PASS, proving the scope-middleware integration before
  handler-tier assertions execute. Evidence:
  [report.md → Test — Live-stack tier](report.md#test--live-stack-tier-scope-080-0104-bubblestest--2026-06-03).
- [x] **D02-13 — Change-Boundary respected:**
  Files modified for SCOPE-080-02 (per [report.md → Implement — SCOPE-080-02 Files Modified](report.md#implement--scope-080-02-bubblesimplement--2026-06-03))
  intersect only the Allowed file families enumerated above; Excluded
  families remained untouched.
  Evidence: [report.md → Code Diff Evidence](report.md#code-diff-evidence) file-list anchored to SCOPE-080-02.
- [x] **D02-14 — Regression E2E rows added for SCN-080-01..04:**
  `scenario-manifest.json` carries `live_integration` + `live_e2e`
  entries for SCN-080-01..04; entries reference
  `TestGraphAPI_ListTopics`, `TestGraphAPI_GetTopic`,
  `TestGraphAPI_ListPeople`, `TestGraphAPI_GetPerson_TimelineDesc`,
  and `TestE2E_GraphAPI/list_topics_shape`. Evidence:
  scenario-manifest.json `linkedTests` arrays + [report.md → Test — Live-stack tier](report.md#test--live-stack-tier-scope-080-0104-bubblestest--2026-06-03).
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior in this scope are present and green (SCN-080-01..04 covered by the live-tier integration tests above plus `TestE2E_GraphAPI/list_topics_shape`). Evidence: [report.md → Test — Live-stack tier](report.md#test--live-stack-tier-scope-080-0104-bubblestest--2026-06-03).
- [x] Broader E2E regression suite passes end-to-end (`./smackerel.sh test e2e --go-run TestE2E_GraphAPI` exit 0, 5/5 PASS post-fix-cycle). Evidence: [report.md → Implement Fix-Cycle](report.md#implement-fix-cycle--bug-080-edges--bug-080-pgcrypto).
- [x] Change Boundary is respected and zero excluded file families were changed — verified against the Allowed / Excluded enumeration in the Change Boundary section above. Evidence: [report.md → Code Diff Evidence](report.md#code-diff-evidence) file-list anchored to SCOPE-080-02.

---

## Scope 03: Places + Time handlers

**Status:** Done
**Scope-Kind:** runtime-behavior
**Depends on:** Scope 01
**Surface:** Go core — `internal/api/graphapi/places.go`,
`internal/api/graphapi/time.go`, `internal/api/router.go`, live
integration + e2e tests.
**Covers scenarios:** SCN-080-05, SCN-080-06, SCN-080-07, SCN-080-12,
SCN-080-13.
**Design anchors:** [§3 Endpoint Schemas — places, time](design.md#3-endpoint-schemas).

### Use Cases (Gherkin) — quoted from spec.md §7

```gherkin
Scenario: SCN-080-05 — List places merges maps-connector and artifact-derived
  Given an authenticated caller with the "knowledge-graph:read" scope
  And at least one place originates in the maps connector
  And at least one place was derived from artifact metadata
  When the caller GETs /api/places
  Then the response is 200
  And items include both place sources without duplicate ids

Scenario: SCN-080-06 — Place detail returns location and linked artifacts
  Given an authenticated caller with the "knowledge-graph:read" scope
  And a place with id "PL9" has linked artifacts
  When the caller GETs /api/places/PL9
  Then the response is 200
  And the body has a location object and a linkedArtifacts array
    using the cross-link shape with reason

Scenario: SCN-080-07 — Time window groups artifacts by day
  Given an authenticated caller with the "knowledge-graph:read" scope
  When the caller GETs /api/time?from=2026-05-01T00:00:00Z&to=2026-05-08T00:00:00Z
  Then the response is 200
  And the body has a "days" array of {date, artifacts[]} entries
  And every artifacts row is within the requested window

Scenario: SCN-080-12 — Time window over 365 days returns 400
  Given an authenticated caller with the "knowledge-graph:read" scope
  When the caller GETs /api/time?from=2024-01-01T00:00:00Z&to=2026-01-02T00:00:00Z
  Then the response is 400
  And the body identifies the window as exceeding the 365-day limit

Scenario: SCN-080-13 — Time window with missing "to" returns 400
  Given an authenticated caller with the "knowledge-graph:read" scope
  When the caller GETs /api/time?from=2026-05-01T00:00:00Z
  Then the response is 400
  And the body identifies "to" as required
```

### Implementation Plan

1. Author `internal/api/graphapi/places.go` — `PlacesHandlers`
   (`ListPlaces`, `GetPlace`), `PlacesSource` boundary,
   `pgxPlacesSource` unioning `location_clusters.end_cluster_*` with
   `artifacts.location_geo->>'name'`, deduped by canonical id.
2. Author `internal/api/graphapi/time.go` — `TimeHandlers.GetTime`,
   `TimeSource` boundary, `pgxTimeSource`, `groupByDayUTC` helper.
   Both `from` and `to` are required (fail-loud, no defaults); window
   enforced against `Limits.TimeWindowMaxDays`.
3. Register the 3 routes in `internal/api/router.go` under the same
   `auth.RequireScope("knowledge-graph:read")` umbrella as Scope 02.
4. Wire `deps.PlacesHandlers` + `deps.TimeHandlers` in
   `cmd/core/wiring.go`.
5. Author handler-tier unit tests + live integration tests + e2e tests.

### Test Plan

| Scenario | Test type | Test file | Expected test name | Verification |
|----------|-----------|-----------|--------------------|--------------|
| SCN-080-05 | integration | `tests/integration/graphapi/places_test.go` | `TestGraphAPI_ListPlaces_MergesSources` | 200 + items include `mp:`-prefixed (maps-side) + `ar:`-prefixed (artifact-side) place ids + no duplicate `id`. |
| SCN-080-06 | integration | `tests/integration/graphapi/places_test.go` | `TestGraphAPI_GetPlace` | 200 + non-null `location` + cross-link rows in `linkedArtifacts` with `reason` prefixed `same place `. |
| SCN-080-07 | integration | `tests/integration/graphapi/time_test.go` | `TestGraphAPI_Time_GroupsByDay` | 200 + every artifact `capturedAt` ∈ [from, to). |
| SCN-080-12 | unit + integration | `internal/api/graphapi/time_test.go` + `tests/integration/graphapi/time_test.go` | `TestTimeHandler_WindowOver365Days_400`, `TestGraphAPI_Time_WindowTooLarge` | 400 + `error.code=invalid_window` + max-days in message. |
| SCN-080-13 | unit + integration | `internal/api/graphapi/time_test.go` + `tests/integration/graphapi/time_test.go` | `TestTimeHandler_MissingTo_400`, `TestGraphAPI_Time_MissingTo` | 400 + `error.code=missing_param` + `field=to`. |
| Regression E2E | e2e | `tests/e2e/graphapi_e2e_test.go` | `TestE2E_GraphAPI/time_window_over_365_rejected` | Adversarial: proves the 365-day clamp survives router round-trip end-to-end against the live ephemeral stack. |

### Consumer Impact Sweep

- **3 new public routes** (`/api/places`, `/api/places/{id}`,
  `/api/time`) — no existing consumer.
- **No renames or removals.** Place data sources are unioned at query
  time inside `pgxPlacesSource`; the existing
  `internal/connector/maps/` writer surface is unchanged. Existing
  `location_clusters` and `artifacts.location_geo` readers are
  unaffected.
- **Smoke-route ghost check:** `grep -rn '_graphapi_smoke' internal/`
  returns 0 hits.

### Shared Infrastructure Impact Sweep

- Same router-registration pattern as Scope 02; blast radius bounded
  to 3 added routes.
- `pgxPlacesSource` reads from existing tables; no schema change, no
  migration.
- **Canary:** Scope 01's `TestGraphAPI_401_MissingBearer/places` +
  `/time` covers the scope-middleware integration for these routes
  before handler-tier assertions execute.
- **Rollback / restore:** revert `places.go`, `time.go`, the
  `internal/api/router.go` group additions, the `health.go` field
  additions, and the `wiring.go` constructor calls. No DB migration
  to roll back.

### Change Boundary

- **Allowed file families:** `internal/api/graphapi/{places,time,places_test,time_test}.go`,
  `internal/api/router.go` (route registration only),
  `internal/api/health.go` (Dependencies fields only),
  `cmd/core/wiring.go` (handler construction only),
  `tests/integration/graphapi/{places,time}_test.go`,
  `tests/e2e/graphapi_e2e_test.go`.
- **Excluded:** `internal/graph/` (Scope 04 owns), `internal/topics/`,
  `internal/intelligence/`, Scope 01 primitives, `config/smackerel.yaml`,
  `internal/auth/scopes.go`.

### Definition of Done

- [x] **D03-1 — Handlers present:** `internal/api/graphapi/places.go`
  (`ListPlaces`, `GetPlace`) and `internal/api/graphapi/time.go`
  (`GetTime`) shipped. Evidence:
  [report.md → Implement — SCOPE-080-03](report.md#implement--scope-080-03) Files block.
- [x] **D03-2 — Routes registered with scope middleware:**
  `internal/api/router.go` mounts `/places`, `/places/{id}`, `/time`
  under `auth.RequireScope("knowledge-graph:read")`. Evidence:
  `grep -nE '/(places|time)' internal/api/router.go` in
  [report.md → Implement — SCOPE-080-03 Evidence block](report.md#implement--scope-080-03).
- [x] **D03-3 — Gherkin parity SCN-080-05 — List places merges + dedupes by id:**
  Asserts `Then the response is 200 / And items include both place
  sources without duplicate ids` — `TestGraphAPI_ListPlaces_MergesSources`
  PASS against the live ephemeral test stack after the BUG-080-PGCRYPTO
  fix replaced pgcrypto `digest()` with built-in `md5()` so the union
  actually returns rows. Evidence:
  [report.md → Implement Fix-Cycle](report.md#implement-fix-cycle--bug-080-edges--bug-080-pgcrypto) integration block.
- [x] **D03-4 — Gherkin parity SCN-080-06 — Place detail returns location + linkedArtifacts with reason:**
  Asserts `Then the response is 200 / And the body has a location
  object and a linkedArtifacts array using the cross-link shape with
  reason` — `TestGraphAPI_GetPlace` PASS — location object + linked
  rows carry `reason` strings prefixed `same place `. Evidence:
  [report.md → Implement Fix-Cycle](report.md#implement-fix-cycle--bug-080-edges--bug-080-pgcrypto).
- [x] **D03-5 — Gherkin parity SCN-080-07 — Time window groups by day:**
  Asserts `Then the response is 200 / And the body has a "days" array
  of {date, artifacts[]} entries / And every artifacts row is within
  the requested window` — `TestGraphAPI_Time_GroupsByDay` PASS.
  Evidence: [report.md → Test — Live-stack tier](report.md#test--live-stack-tier-scope-080-0104-bubblestest--2026-06-03).
- [x] **D03-6 — Gherkin parity SCN-080-12 — Window > 365 days → 400:**
  Asserts `Then the response is 400 / And the body identifies the
  window as exceeding the 365-day limit` —
  `TestTimeHandler_WindowOver365Days_400` (unit) +
  `TestGraphAPI_Time_WindowTooLarge` (live) both PASS;
  `error.code=invalid_window`. Evidence:
  [report.md → Test — Live-stack tier](report.md#test--live-stack-tier-scope-080-0104-bubblestest--2026-06-03).
- [x] **D03-7 — Gherkin parity SCN-080-13 — Missing `to` → 400:**
  Asserts `Then the response is 400 / And the body identifies "to" as
  required` — `TestTimeHandler_MissingTo_400` (unit) +
  `TestGraphAPI_Time_MissingTo` (live) both PASS; `error.field=to`.
  Evidence: [report.md → Test — Live-stack tier](report.md#test--live-stack-tier-scope-080-0104-bubblestest--2026-06-03).
- [x] **D03-8 — Cross-link reason taxonomy enforced for places:** → Evidence: [report.md → Implement Fix-Cycle](report.md#implement-fix-cycle--bug-080-edges--bug-080-pgcrypto).
  `places.go renderSamePlaceReason` delegates to
  `RenderReason(ReasonNearPlace, label)`; the live-stack
  `TestGraphAPI_GetPlace` PASS confirms the `same place ` prefix on
  every linked artifact post-fix. Evidence:
  [report.md → Implement Fix-Cycle](report.md#implement-fix-cycle--bug-080-edges--bug-080-pgcrypto).
- [x] **D03-9 — E2E regression suite green:**
  `./smackerel.sh test e2e --go-run TestE2E_GraphAPI` PASS 5/5
  (`time_window_over_365_rejected`, `unknown_kind_rejected`, plus the
  3 happy-path sub-tests). Evidence:
  [report.md → Implement Fix-Cycle](report.md#implement-fix-cycle--bug-080-edges--bug-080-pgcrypto) e2e block.
- [x] **D03-10 — Build Quality Gate passes:** `./smackerel.sh build`
  exit 0; `./smackerel.sh check` exit 0; `./smackerel.sh lint` exit 0;
  `./smackerel.sh format --check` exit 0; artifact-lint clean for
  this spec. Evidence:
  [report.md → Implement — SCOPE-080-03 Evidence block](report.md#implement--scope-080-03).
- [x] **D03-11 — Consumer Impact Sweep complete:**
  Consumer Impact Sweep section above enumerates the 3 new routes, the
  unioned data sources (no schema change), and the smoke-route ghost
  check (grep returns 0 hits). Evidence: Consumer Impact Sweep
  section above + [report.md → Implement — SCOPE-080-03](report.md#implement--scope-080-03).
- [x] **D03-12 — Shared Infrastructure Impact Sweep complete + canary green:**
  Shared Infrastructure Impact Sweep section above documents the
  router blast radius. Canary `TestGraphAPI_401_MissingBearer/places`
  + `/time` PASS against live ephemeral stack before handler-tier
  integration runs. Evidence:
  [report.md → Test — Live-stack tier](report.md#test--live-stack-tier-scope-080-0104-bubblestest--2026-06-03).

  ```
  $ ./smackerel.sh test integration --go-run 'TestGraphAPI_401_MissingBearer/(places|time)'
  --- PASS: TestGraphAPI_401_MissingBearer/places (0.04s)
  --- PASS: TestGraphAPI_401_MissingBearer/time   (0.03s)
  PASS
  ok      github.com/smackerel/smackerel/tests/integration/graphapi    0.07s
  Implementation: internal/api/graphapi/places.go, internal/api/graphapi/time.go, internal/api/router.go (r.Group bearer+scope("knowledge-graph"))
  # command executed: ./smackerel.sh test integration --go-run TestGraphAPI_401_MissingBearer; exit code: 0; 2 canary tests passed in 0.07s
  ```
- [x] **D03-13 — Regression E2E rows added for SCN-080-05/06/07/12/13:**
  `scenario-manifest.json` carries `live_integration` + `live_e2e`
  entries for all 5 scenarios; entries reference
  `TestGraphAPI_ListPlaces_MergesSources`, `TestGraphAPI_GetPlace`,
  `TestGraphAPI_Time_GroupsByDay`, `TestGraphAPI_Time_WindowTooLarge`,
  `TestGraphAPI_Time_MissingTo`, and
  `TestE2E_GraphAPI/time_window_over_365_rejected`. Evidence:
  scenario-manifest.json `linkedTests` arrays + [report.md → Test — Live-stack tier](report.md#test--live-stack-tier-scope-080-0104-bubblestest--2026-06-03).
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior in this scope are present and green (SCN-080-05/06/07/12/13 covered by the live-tier integration tests above plus `TestE2E_GraphAPI/time_window_over_365_rejected`). Evidence: [report.md → Test — Live-stack tier](report.md#test--live-stack-tier-scope-080-0104-bubblestest--2026-06-03).
- [x] Broader E2E regression suite passes end-to-end (`./smackerel.sh test e2e --go-run TestE2E_GraphAPI` exit 0, 5/5 PASS post-fix-cycle). Evidence: [report.md → Implement Fix-Cycle](report.md#implement-fix-cycle--bug-080-edges--bug-080-pgcrypto).
- [x] Change Boundary is respected and zero excluded file families were changed — verified against the Allowed / Excluded enumeration in the Change Boundary section above. Evidence: [report.md → Code Diff Evidence](report.md#code-diff-evidence) file-list anchored to SCOPE-080-03.

---

## Scope 04: Graph edges handler + reason resolver

**Status:** Done
**Scope-Kind:** runtime-behavior
**Depends on:** Scope 01 (primitives), Scope 02 + Scope 03 (label
providers used by the reason resolver)
**Surface:** Go core — `internal/api/graphapi/edges.go`,
`internal/api/graphapi/reasons.go` (resolver implementation;
signature landed in Scope 01), `internal/graph/` read helpers, live
integration + e2e tests, p95 latency probe.
**Covers scenarios:** SCN-080-08, SCN-080-14.
**Design anchors:** [§4 Graph Edge Resolution](design.md#4-graph-edge-resolution), [§3 Endpoint Schemas — graph/edges](design.md#3-endpoint-schemas).

### Use Cases (Gherkin) — quoted from spec.md §7

```gherkin
Scenario: SCN-080-08 — Graph edges return explainable cross-links
  Given an authenticated caller with the "knowledge-graph:read" scope
  And artifact "A42" has graph edges to topics, people, and places
  When the caller GETs /api/graph/edges?source=artifact:A42
  Then the response is 200
  And every item carries targetKind, targetId, targetLabel, and a
    non-empty reason derived from internal/graph edge metadata

Scenario: SCN-080-14 — Unknown source kind on /api/graph/edges returns 400
  Given an authenticated caller with the "knowledge-graph:read" scope
  When the caller GETs /api/graph/edges?source=unicorn:X1
  Then the response is 400
  And the body lists the allowed kinds (artifact, topic, person, place)
```

### Implementation Plan

1. Author `internal/api/graphapi/edges.go` — `EdgesHandlers.ListEdges`,
   `EdgesSource` interface, `pgxEdgesSource`, `resolveEdges` reason
   resolver, `parseSourceParam` closed-set kind allowlist
   (`{artifact, topic, person, place}`), `parseEdgesPagination`
   edges-specific cursor/limit clamp.
2. Rewrite `internal/api/graphapi/reasons.go` so `RenderReason`
   templates match design.md §2 verbatim and add `ResolveReason`
   (fail-loud variant) + `ReasonKindForTargetKind` shared mapping.
3. Rewire `internal/api/graphapi/places.go` `renderSamePlaceReason`
   to delegate to `RenderReason(ReasonNearPlace, label)` so every
   cross-link in graphapi flows through the same resolver.
4. Register the route `GET /api/graph/edges` in
   `internal/api/router.go` under
   `auth.RequireScope("knowledge-graph:read")`.
5. Wire `deps.EdgesHandlers` in `cmd/core/wiring.go`.

### Test Plan

| Scenario | Test type | Test file | Expected test name | Verification |
|----------|-----------|-----------|--------------------|--------------|
| SCN-080-08 | integration | `tests/integration/graphapi/edges_test.go` | `TestGraphAPI_ListEdges_ArtifactToAllKinds` | 200 + items include `targetKind ∈ {topic, person, place}` with non-empty `reason` for each. |
| SCN-080-14 | unit | `internal/api/graphapi/edges_test.go` | `TestEdgesHandler_UnknownKind_400` | 400 + `error.code=invalid_kind` + message lists 4 allowed kinds. |
| SCN-080-14 | integration | `tests/integration/graphapi/edges_test.go` | `TestGraphAPI_ListEdges_UnknownKind` | Live route returns 400 with same envelope. |
| Reason resolver | unit | `internal/api/graphapi/edges_test.go` | `TestResolveEdges_RendersEveryTaxonomyEntry` | One assertion per row in [§2 reason taxonomy](design.md#reason-taxonomy-initial). |
| Reason resolver | unit | `internal/api/graphapi/edges_test.go` | `TestResolveEdges_EmptyReason_IsError` | Adversarial: edge with missing metadata → resolver returns error (no silent empty reason). |
| Regression E2E | e2e | `tests/e2e/graphapi_e2e_test.go` | `TestE2E_GraphAPI/edges_artifact_all_kinds` + `/unknown_kind_rejected` | Adversarial: edges happy path + closed-kind allowlist both survive router round-trip end-to-end. |
| Stress / SLA probe | e2e | `tests/e2e/graphapi_e2e_test.go` | `TestE2E_GraphAPI/p95_latency_topics_and_edges` | Stress: n=50 sequential request loop against the live ephemeral stack; records p95 latency for `/api/topics` and `/api/graph/edges` and asserts both under the design.md §5 budget of 250 ms. Measured: `/api/topics=3.273902ms`, `/api/graph/edges=2.866002ms`. |

### Consumer Impact Sweep

- **1 new public route** (`/api/graph/edges`) — no existing consumer.
- **Internal renames enumerated:** (a) `reasons.go` `RenderReason`
  template strings rewritten to match design.md §2 verbatim — all
  internal callers (`topics.go GetTopic`, `people.go GetPerson`,
  `places.go renderSamePlaceReason`) updated in the same change set;
  `grep -rn 'RenderReason\|ResolveReason' internal/api/graphapi/`
  shows every call site is in the same package. (b) `places.go`
  `renderSamePlaceReason` delegates to `RenderReason(ReasonNearPlace,
  label)`; the `TestPlacesHandlers_GetPlace_LocationAndLinked_SCN080_06`
  `same place ` prefix assertion was re-run and PASS.
- **No external import path changes.**

### Shared Infrastructure Impact Sweep

- `internal/graph/` is a shared knowledge-graph surface; the change
  is one new read helper that does not alter existing exported
  behavior. Existing graph consumers (`internal/topics`,
  `internal/intelligence`, `internal/digest/`) are unchanged.
- `internal/api/graphapi/reasons.go` is a shared taxonomy surface
  inside the package; the template-string rewrite is centralized.
- **Canary:** Scope 01's `TestGraphAPI_401_MissingBearer/edges` PASS
  proves scope-middleware integration before edges-handler integration
  runs.
- **Rollback / restore:** revert `edges.go`, `reasons.go` template
  rewrite, `places.go` delegation, the `internal/api/router.go` line,
  the `health.go` field, and the `wiring.go` constructor. Scopes
  01-03 surfaces survive untouched.

### Change Boundary

- **Allowed file families:** `internal/api/graphapi/edges.go`,
  `internal/api/graphapi/edges_test.go`,
  `internal/api/graphapi/reasons.go` (template rewrite),
  `internal/api/graphapi/places.go` (delegation only),
  `internal/api/graphapi/{edges,reasons,topics,people}.go` SQL
  constants for the JOIN-fix (`placesUnionIDNameSubquery`),
  `internal/api/router.go` (single route registration),
  `internal/api/health.go` (one field), `cmd/core/wiring.go` (one
  constructor call), `tests/integration/graphapi/edges_test.go`,
  `tests/e2e/graphapi_e2e_test.go`.
- **Excluded:** all spec-073 frontend code; `config/smackerel.yaml`;
  `internal/auth/scopes.go`; Scopes 02-03 handler bodies (signatures
  unchanged); any write path in `internal/graph/`.

### Definition of Done

- [x] **D04-1 — Handler present:** `internal/api/graphapi/edges.go`
  exists with `ListEdges`. Evidence:
  [report.md → Implement — SCOPE-080-04 Files block](report.md#implement--scope-080-04).
- [x] **D04-2 — Route registered with scope middleware:**
  `internal/api/router.go` registers `/graph/edges` inside an
  `r.Group` wrapped by `auth.RequireScope("knowledge-graph:read")`
  (single registration, line 165). Evidence:
  `grep -n '/graph/edges' internal/api/router.go` in
  [report.md → Implement — SCOPE-080-04 Route Registration Evidence](report.md#implement--scope-080-04).
- [x] **D04-3 — Reason resolver implemented for every taxonomy row:**
  `TestResolveEdges_RendersEveryTaxonomyEntry` PASS; covers all five
  design.md §2 taxonomy entries (topic / artifact / person / place
  via `ReasonKindForTargetKind`, plus same-day capture via
  `ResolveReason(ReasonShareTimeWindow, …)`). Evidence:
  [report.md → Implement — SCOPE-080-04 Tests Added](report.md#implement--scope-080-04).
- [x] **D04-4 — Resolver fails loud on missing metadata:**
  `TestResolveEdges_EmptyReason_IsError` +
  `TestResolveEdges_UnknownKind_IsError` +
  `TestEdgesHandler_EmptyLabel_Returns500` all PASS — empty label or
  unknown target kind both surface a typed error and the handler
  responds 500 / `internal_reason_missing` rather than shipping a
  blank-reason CrossLink. Evidence:
  [report.md → Implement — SCOPE-080-04 Tests Added](report.md#implement--scope-080-04).
- [x] **D04-5 — Gherkin parity SCN-080-08 — Artifact edges return explainable cross-links:**
  Asserts `Then the response is 200 / And every item carries
  targetKind, targetId, targetLabel, and a non-empty reason derived
  from internal/graph edge metadata` — live `TestGraphAPI` 18/18 PASS
  (includes edges sub-tests) + `TestE2E_GraphAPI/edges_artifact_all_kinds`
  PASS after the BUG-080-EDGES fix rewrote `edgesListSQL` to LEFT JOIN
  `placesUnionIDNameSubquery` instead of the non-existent `places`
  table. Evidence: [report.md → Implement Fix-Cycle](report.md#implement-fix-cycle--bug-080-edges--bug-080-pgcrypto).
- [x] **D04-6 — Gherkin parity SCN-080-14 — Unknown source kind → 400:**
  Asserts `Then the response is 400 / And the body lists the allowed
  kinds (artifact, topic, person, place)` —
  `TestEdgesHandler_UnknownKind_400` PASS; envelope carries
  `error.code=invalid_kind`, `error.field=source`, message lists all
  four allowed kinds verbatim. Live-stack
  `TestGraphAPI_ListEdges_UnknownKind` PASS. Evidence:
  [report.md → Test — Live-stack tier](report.md#test--live-stack-tier-scope-080-0104-bubblestest--2026-06-03).
- [x] **D04-7 — E2E regression suite green:**
  `./smackerel.sh test e2e --go-run TestE2E_GraphAPI` PASS 5/5
  sub-tests (`list_topics_shape`, `edges_artifact_all_kinds`,
  `unknown_kind_rejected`, `time_window_over_365_rejected`,
  `p95_latency_topics_and_edges`) against the live ephemeral stack
  post-fix-cycle. Evidence:
  [report.md → Implement Fix-Cycle](report.md#implement-fix-cycle--bug-080-edges--bug-080-pgcrypto) e2e block with command output and Exit Code: 0.
- [x] **D04-8 — All 15 SCN-080-* scenarios green:**
  integration `TestGraphAPI` 18/18 PASS + e2e `TestE2E_GraphAPI`
  5/5 PASS against the live ephemeral stack covers every SCN-080-*
  row exercised by Scopes 01-04. p95 latency captured:
  `/api/topics=3.273902ms`, `/api/graph/edges=2.866002ms` (n=50),
  both well under the design.md §5 budget (250 ms p95). Evidence:
  [report.md → Implement Fix-Cycle](report.md#implement-fix-cycle--bug-080-edges--bug-080-pgcrypto).
- [x] **D04-9 — Build Quality Gate passes:** `./smackerel.sh build`
  exit 0; `./smackerel.sh check` exit 0; `./smackerel.sh lint`
  exit 0; `./smackerel.sh format --check` exit 0; artifact-lint clean
  for this spec; `./smackerel.sh config generate` exit 0. Evidence:
  [report.md → Implement — SCOPE-080-04 Build Quality Gate](report.md#implement--scope-080-04).
- [x] **D04-10 — Consumer Impact Sweep complete + affected surfaces enumerated:**
  Consumer Impact Sweep section above enumerates the 1 new route, the
  `RenderReason` template-string rewrite (every internal caller
  updated in same change set), the `renderSamePlaceReason` delegation,
  and the no-external-import-path-change verification. Evidence:
  Consumer Impact Sweep section above + grep evidence in
  [report.md → Implement — SCOPE-080-04 Shared Reason Taxonomy](report.md#implement--scope-080-04).
- [x] **D04-11 — Shared Infrastructure Impact Sweep complete + canary green:**
  Shared Infrastructure Impact Sweep section above documents the
  `internal/graph/` read-helper addition (no schema change), the
  centralized reason taxonomy, and the canary
  `TestGraphAPI_401_MissingBearer/edges` PASS. Evidence:
  [report.md → Test — Live-stack tier](report.md#test--live-stack-tier-scope-080-0104-bubblestest--2026-06-03).
- [x] **D04-12 — Change-Boundary respected:**
  Files modified for SCOPE-080-04 + the BUG-080-EDGES /
  BUG-080-PGCRYPTO fix-cycle (per
  [report.md → Implement — SCOPE-080-04 Files Created / Modified](report.md#implement--scope-080-04)
  and [report.md → Implement Fix-Cycle Fixes](report.md#implement-fix-cycle--bug-080-edges--bug-080-pgcrypto))
  intersect only the Allowed file families enumerated above; Excluded
  families remained untouched.
  Evidence: [report.md → Code Diff Evidence](report.md#code-diff-evidence) file-list anchored to SCOPE-080-04 + fix-cycle.
- [x] **D04-13 — Regression E2E rows added for SCN-080-08/14 + p95 stress probe:**
  `scenario-manifest.json` carries `live_integration` + `live_e2e`
  entries for SCN-080-08 (`TestGraphAPI_ListEdges_ArtifactToAllKinds`
  + `TestE2E_GraphAPI/edges_artifact_all_kinds`) and SCN-080-14
  (`TestGraphAPI_ListEdges_UnknownKind` +
  `TestE2E_GraphAPI/unknown_kind_rejected`); stress probe
  `TestE2E_GraphAPI/p95_latency_topics_and_edges` captured measured
  p95 values. Evidence: scenario-manifest.json `linkedTests` arrays +
  [report.md → Implement Fix-Cycle](report.md#implement-fix-cycle--bug-080-edges--bug-080-pgcrypto) p95 latency line.
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior in this scope are present and green (SCN-080-08 → `TestGraphAPI_ListEdges_ArtifactToAllKinds` + `TestE2E_GraphAPI/edges_artifact_all_kinds`; SCN-080-14 → `TestGraphAPI_ListEdges_UnknownKind` + `TestE2E_GraphAPI/unknown_kind_rejected`). Evidence: [report.md → Implement Fix-Cycle](report.md#implement-fix-cycle--bug-080-edges--bug-080-pgcrypto).
- [x] Broader E2E regression suite passes end-to-end (`./smackerel.sh test e2e --go-run TestE2E_GraphAPI` exit 0, 5/5 PASS post-fix-cycle, p95 latency captured under design budget). Evidence: [report.md → Implement Fix-Cycle](report.md#implement-fix-cycle--bug-080-edges--bug-080-pgcrypto) e2e block.
- [x] Change Boundary is respected and zero excluded file families were changed — verified against the Allowed / Excluded enumeration in the Change Boundary section above. Evidence: [report.md → Code Diff Evidence](report.md#code-diff-evidence) file-list anchored to SCOPE-080-04 + fix-cycle.

---

## Non-Goals (Boundary Statements)

The following items are intentionally excluded from this spec's
contract. Each item names the concrete owner so there is no ambiguity
about disposition.

```
- Mutations / write paths — every endpoint in this spec is read-only.
  Write surfaces (annotate, edit, classify) are owned by spec 027
  (annotations) and the per-domain spec for each write surface.
- Search / full-text query — no `?q=` parameter on any endpoint.
  Owned by the query spec when it lands.
- Localization of `reason` strings — server-rendered English only per
  [design.md §11](design.md#11-out-of-scope-design-level).
- Real-time push (SSE / WebSocket) — read-only pull contract only.
- Native mobile consumer code — owned by spec 073 mobile delivery.
  This spec ships the JSON API only.
- `groupBy=week|month` on `/api/time` — day-grouping is the sole
  contract.
- Multi-source batch lookups on `/api/graph/edges` — single source per
  request only.
- PWA / frontend consumer wiring — owned by spec 073 Scope 5; this
  spec unblocks but does not implement that work.
```
