# Execution Reports

Links: [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md)

---

## Regression Analysis — 2026-04-22 (regression-to-doc, repeat)

**Trigger:** Child workflow of stochastic-quality-sweep
**Mode:** regression-to-doc (repeat)
**Verdict:** 1 finding (cross-spec lint regression), FIXED

### Findings

| ID | Surface | Severity | Description | Status |
|----|---------|----------|-------------|--------|
| REG-034-R2-001 | `ml/app/intelligence.py`, `ml/tests/test_intelligence_handlers.py` | Low | Cross-spec lint regression: 4 ruff errors introduced by Phase 5 intelligence work — unused `base_url` variable (F841), unnecessary f-string prefix (F541), unsorted imports (I001), unused `pytest` import (F401). Broke `./smackerel.sh lint` globally. | FIXED |

### Fixes Applied

**REG-034-R2-001 — Cross-Spec Lint Regression:**
- Removed unused `base_url` variable from `ml/app/intelligence.py` line 52
- Removed unnecessary `f` prefix from string literal on line 55 (no placeholders)
- Fixed import sorting in `ml/tests/test_intelligence_handlers.py`
- Removed unused `import pytest` in same test file
- Files: `ml/app/intelligence.py`, `ml/tests/test_intelligence_handlers.py`

### Test Baseline

| Category | Result |
|----------|--------|
| Go unit tests | All 41 packages passing |
| Python unit tests | 257 passed, 3 warnings (unrelated to expenses) |
| Build | All images built successfully |
| Lint | All checks passed (after fix) |
| Config check | Config in sync with SST, env_file drift guard OK |

### Cross-Spec Conflict Scan

| Surface | Specs Checked | Verdict |
|---------|--------------|---------|
| Route collisions (`/api/expenses/*`) | All API-registering specs | CLEAN — properly namespaced |
| Shared JSONB metadata (`metadata ? 'expense'`) | All metadata-writing specs | CLEAN — dedicated `expense` key |
| ML sidecar synthesis pipeline (`receipt_detection`) | 025, 026 | CLEAN — additive second pass |
| Config namespace (`EXPENSES_*` env vars) | All config-consuming specs | CLEAN — no prefix collisions |
| Telegram commands (`/expense`) | 027, 028, 035, 036 | CLEAN — no command name conflicts |
| Digest sections (`ExpenseDigestSection`) | 012, 025 | CLEAN — separate conditional producer |
| Router registration (`internal/api/router.go`) | All API-registering specs | CLEAN — guarded by handler nil check |

### Design Contradiction Check

| Concern | Analysis | Verdict |
|---------|----------|---------|
| Metadata-first pattern (expenses as artifact metadata) | Consistent with project architecture | CLEAN |
| Amounts as strings (no float arithmetic) | Design mandates `CAST(...AS NUMERIC)` for PostgreSQL aggregation only | CLEAN |
| Pagination (offset/limit) | IMP-034-001 fix from prior improve-existing round still intact | CLEAN |

### Verification

| Check | Result |
|-------|--------|
| `./smackerel.sh build` | All images built successfully |
| `./smackerel.sh test unit` | All Go packages OK, 257 Python tests passed |
| `./smackerel.sh lint` | All checks passed |
| `./smackerel.sh check` | Config in sync, env_file drift guard OK |

---

## Improve Analysis — 2026-04-22 (improve-existing, repeat)

**Trigger:** Child workflow of stochastic-quality-sweep
**Mode:** improve-existing
**Verdict:** 3 findings, ALL FIXED

### Findings

| ID | Surface | Severity | Description | Status |
|----|---------|----------|-------------|--------|
| IMP-034-001 | `internal/api/expenses.go` List handler | Medium | List endpoint lacked pagination — hardcoded `LIMIT 50` with no offset/cursor parameter, making it impossible to retrieve beyond first 50 results | FIXED |
| IMP-034-002 | `internal/digest/expenses.go` Assemble | Low | Dead code: `currTotals := make(map[string]float64)` allocated but never populated; explicit `_ = currTotals` silencer present | FIXED |
| IMP-034-003 | `internal/intelligence/expenses.go` Classify | Low | Redundant vendor comparison: `strings.EqualFold(bv, vendorLower)` duplicated by `strings.EqualFold(bv, expense.Vendor)` since EqualFold is already case-insensitive | FIXED |

### Fixes Applied

**IMP-034-001 — List Pagination:**
- Added `offset` query parameter (default 0, non-negative integer)
- Added `limit` query parameter (default 50, range 1–200)
- Response `meta` now includes `limit` and `offset` fields for pagination context
- Files: `internal/api/expenses.go` (List handler), `internal/api/expenses_test.go` (pagination parameter tests)

**IMP-034-002 — Dead Code Removal:**
- Removed unused `currTotals` variable and its `_ = currTotals` silencer from `Assemble()`
- Kept the second per-currency totals query (correct behavior — first query groups by classification+currency, second aggregates cross-classification per currency)
- File: `internal/digest/expenses.go`

**IMP-034-003 — Redundant Comparison:**
- Replaced `strings.ToLower(expense.Vendor)` + dual `strings.EqualFold` with single `strings.EqualFold(bv, expense.Vendor)`
- File: `internal/intelligence/expenses.go`

### Verification

| Check | Result |
|-------|--------|
| `./smackerel.sh build` | All images built successfully |
| `./smackerel.sh test unit` | All Go packages OK, 236 Python tests passed |
| `./smackerel.sh lint` | All checks passed |

---

## DevOps Analysis — 2026-04-21 (devops-to-doc)

**Trigger:** Child workflow of stochastic-quality-sweep R52
**Mode:** devops-to-doc
**Verdict:** CLEAN — zero findings across build, CI/CD, config pipeline, deployment, and monitoring

### Build Pipeline (Docker)

| Surface | Status | Evidence |
|---------|--------|----------|
| Go core Dockerfile | CLEAN | Multi-stage build, `VERSION`/`COMMIT_HASH`/`BUILD_TIME` args, OCI labels, non-root user (`smackerel`), minimal alpine runtime |
| Python ML Dockerfile | CLEAN | Multi-stage build, CPU-only PyTorch, OCI labels, non-root user (`smackerel`), `.dist-info`/`__pycache__` stripped |
| docker-compose.yml core service | CLEAN | `env_file` from generated config, healthcheck on `/api/health`, resource limits (512M), `no-new-privileges` security opt |
| docker-compose.yml ML service | CLEAN | Mounts `config/prompt_contracts:/app/prompt_contracts:ro` (includes `receipt-extraction-v1.yaml`), healthcheck, resource limits (2G) |
| docker-compose.prod.yml | CLEAN | `restart: always`, memory limit overrides, json-file logging with rotation (`max-size: 50m`, `max-file: 5`), prod healthcheck on `/readyz` |

### CI/CD (.github/workflows/ci.yml)

| Job | Status | Evidence |
|-----|--------|----------|
| lint-and-test | CLEAN | Runs `./smackerel.sh lint` + `./smackerel.sh test unit`, Go 1.24, Python 3.12, SHA-pinned actions |
| build | CLEAN | Runs `./smackerel.sh build` with version/commit/time build args |
| push-images | CLEAN | Tags and pushes to GHCR on version tags, both `smackerel-core` and `smackerel-ml` |
| integration | CLEAN | Applies all migrations (`internal/db/migrations/*.sql` including `019_expense_tracking.sql`), runs against real PostgreSQL + NATS |

### Config Pipeline (SST)

| Check | Status | Evidence |
|-------|--------|----------|
| Config generation | CLEAN | `scripts/commands/config.sh` lines 332–357 emit all 16 expense env vars |
| dev.env output | CLEAN | Lines 151–166: `EXPENSES_ENABLED` through `EXPENSES_CATEGORIES` present with correct values |
| test.env output | CLEAN | Lines 151–166: identical variable set with correct values |
| JSON-encoded values | CLEAN | `IMAP_EXPENSE_LABELS`, `EXPENSES_BUSINESS_VENDORS`, `EXPENSES_CATEGORIES` use `yaml_get_json` |
| SST compliance | CLEAN | `./smackerel.sh check` → "Config is in sync with SST, env_file drift guard: OK" |
| Feature toggle | CLEAN | `EXPENSES_ENABLED=false` default when config section absent → feature safely disabled |

### Service Wiring

| Surface | Status | Evidence |
|---------|--------|----------|
| `cmd/core/main.go` | CLEAN | Lines 161–175: `ExpenseHandler` wired when `cfg.ExpensesEnabled`, vendor alias seeding on startup |
| `internal/api/router.go` | CLEAN | Line 128: route registration guarded by `if deps.ExpenseHandler != nil` |
| `ml/app/synthesis.py` | CLEAN | Imports `detect_receipt_content` from `receipt_detection` module |
| `internal/telegram/bot.go` | CLEAN | Line 319: `case "expense":` dispatches to `handleExpenseCommand` |

### Database Migrations

| Check | Status | Evidence |
|-------|--------|----------|
| Migration file | CLEAN | `internal/db/migrations/019_expense_tracking.sql` — vendor_aliases, expense_suggestions, expense_suggestion_suppressions |
| Expense query indexes | CLEAN | GIN index on `metadata->'expense'`, B-tree on expense date and vendor |
| CI migration step | CLEAN | Alphabetical `*.sql` glob picks up `019_expense_tracking.sql` |
| Rollback documented | CLEAN | SQL comments include `DROP TABLE IF EXISTS` rollback statements |

### Monitoring/Observability

| Check | Status | Notes |
|-------|--------|-------|
| Prometheus metrics module | INFORMATIONAL | `internal/metrics/metrics.go` has general counters (artifacts ingested, capture, search, digest). No expense-specific counters (extraction, classification, export, suggestion accept/dismiss). Expense operations are covered by the general artifact ingestion and domain extraction counters. Not a blocking gap — enhancement opportunity for future observability spec. |
| Health endpoints | CLEAN | Dev: `/api/health`, Prod: `/readyz` — both cover overall service health including expense handler |

### CLI Verification

| Command | Result |
|---------|--------|
| `./smackerel.sh check` | Config in sync with SST, env_file drift guard OK |
| `./smackerel.sh lint` | All checks passed |
| `./smackerel.sh test unit` | All Go packages OK, 236 Python tests passed (3 unrelated warnings) |

---

## Reconciliation Analysis — 2026-04-21 (reconcile-to-doc)

**Trigger:** Child workflow of stochastic-quality-sweep  
**Mode:** reconcile-to-doc  
**Verdict:** CLEAN — zero drift between claimed and implemented state

### Claimed-vs-Implemented State

| Surface | Claimed (state.json / scopes.md) | Actual (codebase) | Verdict |
|---------|----------------------------------|--------------------|---------|
| state.json status | `done`, 9 scopes certified | All files exist, all tests pass | MATCH |
| Scope 01 — Config & Pipeline | Done | `config/smackerel.yaml` expenses section (line 48), `scripts/commands/config.sh` emits 16 env vars, `internal/config/config.go` ExpenseConfig struct | MATCH |
| Scope 02 — Receipt Detection | Done | `ml/app/receipt_detection.py` heuristics, `config/prompt_contracts/receipt-extraction-v1.yaml`, `ml/app/synthesis.py` imports receipt_detection (line 15) | MATCH |
| Scope 03 — Data Model & Migration | Done | `internal/domain/expense.go` structs, `internal/db/migrations/019_expense_tracking.sql` (vendor_aliases, expense_suggestions, expense_suggestion_suppressions, indexes) | MATCH |
| Scope 04 — Classification Engine | Done | `internal/intelligence/expenses.go` ExpenseClassifier with 7-level rule priority chain | MATCH |
| Scope 05 — Vendor Normalization | Done | `internal/intelligence/expenses.go` VendorNormalizer with LRU cache, `internal/intelligence/vendor_seeds.go` pre-seeded aliases | MATCH |
| Scope 06 — Expense API | Done | `internal/api/expenses.go` (List, Get, Export, Correct, Classify, AcceptSuggestion, DismissSuggestion), `internal/api/router.go` line 128 registers routes | MATCH |
| Scope 07 — CSV Export | Done | `internal/api/expenses.go` Export method with standard and QuickBooks format | MATCH |
| Scope 08 — Telegram Commands | Done | `internal/telegram/expenses.go` all 11 formats, `internal/telegram/bot.go` routes expense commands (line 319) | MATCH |
| Scope 09 — Digest Integration | Done | `internal/digest/expenses.go` ExpenseDigestSection, integrated in `internal/digest/generator.go` | MATCH |

### Wiring Integration Check

| Wiring Point | Previously Noted Gap | Current State | Verdict |
|-------------|---------------------|---------------|---------|
| `cmd/core/main.go` route registration | "routes not in cmd/core/main.go" | Lines 161–175: `ExpenseHandler` wired when `cfg.ExpensesEnabled` | RESOLVED |
| `ml/app/synthesis.py` receipt import | "synthesis.py not importing receipt_detection" | Line 15: `from .receipt_detection import detect_receipt_content` | RESOLVED |
| `internal/telegram/bot.go` command routing | "expense commands not routed in telegram/bot.go" | Line 319: `case "expense":` dispatches to `handleExpenseCommand` | RESOLVED |
| `internal/db/migrations/` migration file | "no dedicated migration file" | `019_expense_tracking.sql` exists with all tables and indexes | RESOLVED |

### Test Verification

| Category | Result |
|----------|--------|
| Go unit tests | All packages OK (cached) |
| Python unit tests | 236 passed, 3 warnings (unrelated to expenses) |
| Config check | Config in sync with SST, env_file drift guard OK |

### State Metadata Cleanup

| Item | Before | After |
|------|--------|-------|
| `state.json` notes | Listed 4 stale wiring gaps | Updated to reflect all gaps resolved |
| `executionHistory[implement].summary` | Referenced wiring gaps as open | Updated to confirm all wiring complete |
| `lastUpdatedAt` | 2026-04-19 | 2026-04-21 |

---

## Regression Analysis — 2026-04-21 (regression-to-doc)

**Trigger:** Child workflow of stochastic-quality-sweep  
**Mode:** regression-to-doc  
**Verdict:** CLEAN — zero findings

### Test Baseline

| Category | Result |
|----------|--------|
| Go unit tests | All passing (40+ packages, cached) |
| Python unit tests | 236 passed, 3 warnings (unrelated to expenses) |
| Lint | All checks passed |
| Format | 33 files unchanged |
| Config check | Config in sync, env_file drift guard OK |

### Cross-Spec Conflict Scan

| Surface | Specs Checked | Verdict |
|---------|--------------|---------|
| Route collisions (`/api/expenses/*`) | 025, 026, 027, 028, 035, 036 | CLEAN — properly namespaced, no overlaps |
| Shared JSONB metadata (`metadata ? 'expense'`) | All metadata-writing specs | CLEAN — dedicated `expense` key, no cross-contamination |
| ML sidecar synthesis pipeline (`ml/app/synthesis.py`) | 025, 026 | CLEAN — receipt detection is additive second pass, standard extraction unaffected |
| Config namespace (`EXPENSES_*` env vars) | All config-consuming specs | CLEAN — no prefix collisions |
| Telegram commands (`/expense`) | 027, 028, 035, 036 | CLEAN — no command name conflicts |
| Digest sections (`ExpenseDigestSection`) | 012, 025 | CLEAN — separate conditional producer |
| Router registration (`internal/api/router.go`) | All API-registering specs | CLEAN — guarded by `if deps.ExpenseHandler != nil` |

### Design Contradiction Check

| Concern | Analysis | Verdict |
|---------|----------|---------|
| Metadata-first pattern (expenses as artifact metadata) | Consistent with project architecture — all features store domain data in `metadata` JSONB | CLEAN |
| Amounts as strings (no float arithmetic) | Design explicitly mandates `CAST(...AS NUMERIC)` for PostgreSQL aggregation only | CLEAN |
| Receipt heuristic false positives on recipe/meal content | Heuristic is intentionally broad (best-effort), LLM extraction is the quality gate | CLEAN by design |
| Config SST enforcement | All 16 expense env vars flow from `smackerel.yaml` → config generate → env vars → Go parsing | CLEAN |

### Coverage

No coverage decrease detected. Expense-specific test files:
- `internal/config/validate_test.go` — config parsing + fail-loud
- `internal/api/expenses_test.go` — API endpoint validation
- `internal/intelligence/expenses_test.go` — classification engine
- `internal/digest/expenses_test.go` — digest section assembly + word limit
- `internal/telegram/expenses_test.go` — Telegram formatting
- `internal/domain/expense_test.go` — domain model
- `ml/tests/test_receipt_detection.py` — heuristic rules
- `ml/tests/test_receipt_extraction.py` — extraction schema validation

---

## Security Analysis — 2026-04-21 (security-to-doc)

**Trigger:** Child workflow of stochastic-quality-sweep
**Mode:** security-to-doc
**Verdict:** 1 finding (SEC-034-001), FIXED

### Security Scan Summary

| OWASP Category | Surface | Verdict |
|----------------|---------|---------|
| A01 Broken Access Control | All expense API routes behind `bearerAuthMiddleware` with `subtle.ConstantTimeCompare` | CLEAN |
| A02 Cryptographic Failures | Auth token compared via constant-time comparison; no secrets in logs | CLEAN |
| A03 Injection — SQL | All SQL queries use parameterized `$N` args via pgx | CLEAN |
| A03 Injection — CSV | `sanitizeCSVCell()` prefixes `=`, `+`, `-`, `@`, `\t`, `\r`, `\n` with `'` (OWASP recommendation) | CLEAN |
| A03 Injection — LIKE | `VendorNormalizer.Normalize` escapes `%` and `_` before LIKE query | CLEAN |
| A04 Insecure Design | Amount pattern caps at 10 digits; string length limits on vendor/notes/payment_method | CLEAN |
| A05 Security Misconfiguration | Security headers set (CSP, X-Frame-Options, X-Content-Type-Options, Referrer-Policy, Permissions-Policy, Cache-Control) | CLEAN |
| A06 Vulnerable Components | No expense-specific third-party dependencies beyond existing stack | CLEAN |
| A07 Auth Failures | Bearer auth enforced; dev-mode pass-through is intentional for local dev | CLEAN |
| A08 Software/Data Integrity | User corrections are sticky (`user_corrected: true`); re-extraction never overwrites corrected fields | CLEAN |
| A09 Logging/Monitoring | Auth failures logged with path and remote_addr; no sensitive data in error responses | CLEAN |
| A10 SSRF | No outbound requests from expense handlers | CLEAN |
| ReDoS | Receipt detection caps input at 100,000 chars; regex patterns are non-backtracking | CLEAN |
| Memory exhaustion | **FINDING SEC-034-001** — PATCH and POST handlers missing `http.MaxBytesReader` | **FIXED** |

### Finding: SEC-034-001 — Missing Request Body Size Limit

**Severity:** Medium
**OWASP:** A04 Insecure Design / resource exhaustion
**Location:** `internal/api/expenses.go` — `Correct()` and `ClassifyEndpoint()` handlers
**Description:** Both handlers called `json.NewDecoder(r.Body).Decode(&req)` without applying `http.MaxBytesReader` first. An attacker with a valid token could send an arbitrarily large request body to exhaust server memory. Compare with `capture.go` and `bookmarks.go` which already apply body limits.
**Fix:** Added `r.Body = http.MaxBytesReader(w, r.Body, maxExpenseBodySize)` (64 KB) at the start of both handlers. Added `maxExpenseBodySize` constant.
**Tests added:** `TestExpenseCorrect_OversizedBody`, `TestClassifyEndpoint_OversizedBody` in `internal/api/expenses_test.go`
**Verification:** `./smackerel.sh test unit` — all pass; `./smackerel.sh lint` — all checks passed; `./smackerel.sh format --check` — 33 files unchanged

### Files Modified

| File | Change |
|------|--------|
| `internal/api/expenses.go` | Added `maxExpenseBodySize` const (64 KB); added `http.MaxBytesReader` to `Correct()` and `ClassifyEndpoint()` |
| `internal/api/expenses_test.go` | Added `TestExpenseCorrect_OversizedBody` and `TestClassifyEndpoint_OversizedBody` |

---

## Stability Analysis — 2026-04-21 (stabilize-to-doc)

**Trigger:** Child workflow of stochastic-quality-sweep R89
**Mode:** stabilize-to-doc
**Verdict:** 1 finding (STB-034-001), FIXED. 1 documented (STB-034-002), acceptable.

### Stability Scan Summary

| Surface | Verdict | Details |
|---------|---------|---------|
| API handlers — request validation | CLEAN | All endpoints validate inputs, enforce body size limits, return structured errors |
| API handlers — database nil guard | CLEAN | List, Get, Correct, ClassifyEndpoint handle nil pool via query failure |
| API handlers — suggestion atomicity | **FIXED STB-034-001** | AcceptSuggestion and DismissSuggestion now wrapped in transactions |
| Classification engine — rule chain | CLEAN | 7-level priority chain is deterministic; nil config fields do not panic (existing chaos test) |
| Vendor normalizer — cache | CLEAN | LRU-style cache with eviction, negative caching, thread-safe via RWMutex |
| CSV export — streaming | CLEAN | Streams rows from DB cursor directly to HTTP response; context cancellation propagated via `r.Context()` |
| CSV export — row limit | CLEAN | `ExpensesExportMaxRows` enforced before streaming; 413 returned when exceeded |
| Digest assembly — query resilience | CLEAN | Each sub-query logs warning on failure and continues; digest degrades gracefully |
| Digest — word limit enforcement | CLEAN | EnforceWordLimit drops low-priority sections first; existing chaos tests cover 1000-item and tight-limit cases |
| Receipt detection — input capping | CLEAN | `MAX_TEXT_LENGTH = 100,000` prevents pathological regex behavior |
| Telegram — conversation state | CLEAN | TTL-based sweep goroutine with `Stop()` signal; thread-safe store |
| Suggestion generation — N+1 queries | **DOCUMENTED STB-034-002** | Per-candidate DB queries; acceptable for single-user with 100-row LIMIT |

### Finding: STB-034-001 — Non-Atomic Suggestion Accept/Dismiss

**Severity:** Medium
**Category:** Data consistency / reliability
**Location:** `internal/api/expenses.go` — `AcceptSuggestion()` and `DismissSuggestion()` handlers

**Description:** Both handlers performed multiple database mutations (read suggestion → update artifact → update suggestion → optionally create suppression) without wrapping them in a transaction. If any intermediate step failed:
- **AcceptSuggestion:** Artifact classification could be changed while the suggestion remained "pending", allowing double-accept.
- **DismissSuggestion:** Suggestion could be marked "dismissed" without creating the suppression entry, causing re-suggestion.

Additionally, both handlers would panic with a nil pointer dereference if invoked with a nil database pool.

**Fix:**
1. Wrapped both handlers in `pool.Begin(ctx)` / `tx.Commit(ctx)` transactions with `defer tx.Rollback(ctx)`
2. Added `SELECT ... FOR UPDATE` row-level locking on the suggestion read to prevent concurrent races
3. Added nil pool guard before `pool.Begin()` to return 500 gracefully instead of panicking
4. Promoted DismissSuggestion's suppression insert from warning-swallowed to transactional

**Tests added:** `TestAcceptSuggestion_NilPool`, `TestDismissSuggestion_NilPool` in `internal/api/expenses_test.go`
**Verification:** `./smackerel.sh test unit` — all pass; `./smackerel.sh lint` — all checks passed

### Documented: STB-034-002 — GenerateSuggestions N+1 Query Pattern

**Severity:** Low (acceptable for single-user system)
**Category:** Performance
**Location:** `internal/intelligence/expenses.go` — `GenerateSuggestions()`

**Description:** The method queries up to 100 candidates, then runs 2 queries per candidate (suppression check + business history count). For a single-user system with local PostgreSQL, this is acceptable — the method runs in a scheduled background job, not a latency-sensitive request path.

### Files Modified

| File | Change |
|------|--------|
| `internal/api/expenses.go` | Wrapped `AcceptSuggestion()` and `DismissSuggestion()` in DB transactions with `FOR UPDATE` row locking; added nil pool guards |
| `internal/api/expenses_test.go` | Added `TestAcceptSuggestion_NilPool` and `TestDismissSuggestion_NilPool` |

### CLI Verification

| Command | Result |
|---------|--------|
| `./smackerel.sh test unit` | All Go packages OK, 236 Python tests passed |
| `./smackerel.sh lint` | All checks passed |

---

## Simplify Analysis — 2026-04-21 (simplify-to-doc)

**Trigger:** Child workflow of stochastic-quality-sweep
**Mode:** simplify-to-doc
**Verdict:** 2 findings (SMP-034-001, SMP-034-002), both FIXED

### Simplify Scan Summary

| Surface | Verdict | Details |
|---------|---------|---------|
| Domain model (`internal/domain/expense.go`) | CLEAN | Focused types, no unnecessary abstractions, safe defaults via `NewExpenseMetadata()` |
| Classification engine (`internal/intelligence/expenses.go`) | **FIXED SMP-034-001** | Duplicate `containsField` helper replaced with stdlib `slices.Contains` |
| Vendor normalizer (`internal/intelligence/expenses.go`) | CLEAN | Simple cache-with-eviction strategy appropriate for single-user system |
| Vendor seeds (`internal/intelligence/vendor_seeds.go`) | CLEAN | Static data, no logic to simplify |
| API handlers (`internal/api/expenses.go`) | **FIXED SMP-034-001, SMP-034-002** | Duplicate `containsStr` helper replaced; missing date range validation added to Export |
| CSV export (`internal/api/expenses.go`) | CLEAN | Streams directly from DB cursor, CSV injection protection, proper filename sanitization |
| Digest assembly (`internal/digest/expenses.go`) | CLEAN | Graceful degradation, word limit enforcement, clear priority ordering |
| Telegram formatting (`internal/telegram/expenses.go`) | CLEAN | Focused format functions per UX spec, vendor truncation for message safety |
| Receipt detection (`ml/app/receipt_detection.py`) | CLEAN | Well-focused module, input length cap, clear rule documentation |
| Prompt contract (`config/prompt_contracts/receipt-extraction-v1.yaml`) | CLEAN | Proper JSON Schema, explicit extraction rules |
| Config pipeline (`scripts/commands/config.sh`) | CLEAN | 16 env vars properly mapped from YAML |

### Finding: SMP-034-001 — Duplicate String-Contains Helpers

**Severity:** Low (code hygiene)
**Category:** Code reuse
**Locations:**
- `internal/intelligence/expenses.go` — `containsField(slice []string, item string) bool`
- `internal/api/expenses.go` — `containsStr(slice []string, item string) bool`

**Description:** Both functions were identical hand-rolled implementations of `slices.Contains` from the Go standard library (available since Go 1.21; project uses Go 1.24). Two packages duplicated the same 6-line helper.

**Fix:** Replaced both with `slices.Contains` from stdlib. Updated one test (`TestContainsField_EdgeCases` → `TestSlicesContains_EdgeCases`) that directly tested the deleted helper. Added `"slices"` import to both packages.

### Finding: SMP-034-002 — Missing Date Range Validation in Export Endpoint

**Severity:** Low (consistency gap)
**Category:** Quality / consistency
**Location:** `internal/api/expenses.go` — `Export()` handler

**Description:** The `List()` handler validated that `from` date is before `to` date and returned `INVALID_DATE_RANGE` error. The `Export()` handler validated date format but missed the `from > to` range check — an inconsistency between the two endpoints that share the same filter surface.

**Fix:** Added `from > to` validation to `Export()` matching the existing `List()` behavior. Added `TestExport_InvalidDateRange` adversarial test.

### Files Modified

| File | Change |
|------|--------|
| `internal/intelligence/expenses.go` | Added `"slices"` import; replaced `containsField()` call with `slices.Contains()`; removed `containsField` function |
| `internal/intelligence/expenses_test.go` | Added `"slices"` import; updated `TestContainsField_EdgeCases` → `TestSlicesContains_EdgeCases` to use `slices.Contains` |
| `internal/api/expenses.go` | Added `"slices"` import; replaced `containsStr()` call with `slices.Contains()`; removed `containsStr` function; added `from > to` date range validation to `Export()` |
| `internal/api/expenses_test.go` | Added `TestExport_InvalidDateRange` |

### Reviewed But Clean (No Findings)

| Area | Notes |
|------|-------|
| Filter builder duplication between List and Export | ~35 lines of overlapping filter construction. Decided against extraction: the two methods serve different filter sets (List has vendor/currency/needs_review; Export has format/quickbooks), and the abstraction cost outweighs the duplication for a stable, tested surface. |
| VendorNormalizer cache eviction | Uses clear-half strategy instead of LRU. Acceptable for single-user system — simple, predictable, no external dependency. |
| Digest summary double-query | Two similar queries (classification+currency grouped, currency-only grouped). Second only runs when TotalCount > 0. Clear and readable; merging adds complexity for negligible gain. |
| Python receipt_detection module | Well-isolated, well-documented, clean separation from synthesis pipeline. No simplification needed. |

### CLI Verification

| Command | Result |
|---------|--------|
| `./smackerel.sh build` | Both images built successfully |
| `./smackerel.sh test unit` | All Go packages OK, 236 Python tests passed |
| `./smackerel.sh lint` | All checks passed |

---

<!-- Report entries will be added below as scopes are implemented -->
