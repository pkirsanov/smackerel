# Execution Report: 026 — Domain-Aware Structured Extraction

Links: [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md)

---

## Summary

Spec 026 introduces domain-specific structured extraction as an additional LLM pass after universal processing. Recipe and product are the initial domain schemas. All 9 scopes completed.

---

## Scope Evidence

### Scope 1 — DB Migration & Domain Data Types
- Migration `015_domain_extraction.sql` adds `domain_data` JSONB column and extraction tracking to artifacts table.

### Scope 2 — Domain Schema Registry
- `internal/domain/registry.go` implements schema registry mapping content types and URL qualifiers to prompt contracts.

### Scope 3 — NATS Domain Extraction Subjects & Go Publisher
- DOMAIN stream and `domain.>` subjects added to `config/nats_contract.json`.
- Go publisher wired in pipeline to dispatch domain extraction after standard processing.

### Scope 4 — ML Sidecar Domain Extraction Handler
- Python handler processes NATS domain extraction requests, applies prompt contracts, returns structured data.

### Scope 5 — Recipe Extraction Prompt Contract
- `config/prompt_contracts/recipe-extraction-v1.yaml` — extracts ingredients, steps, nutrition, servings from recipe content.

### Scope 6 — Product Extraction Prompt Contract
- `config/prompt_contracts/product-extraction-v1.yaml` — extracts price, specs, ratings from product pages.

### Scope 7 — Pipeline Integration
- Domain extraction runs as an additional pipeline stage after embedding, using NATS async dispatch.

### Scope 8 — Search Extension
- Search results include domain-extracted structured data when available.

### Scope 9 — Telegram Display
- Telegram bot formats domain-extracted data (recipe ingredients, product prices) in artifact detail responses.

---

## Security Probe — 2026-04-20 (stochastic-quality-sweep round)

**Trigger:** `security` via `security-to-doc` child workflow
**Scope:** All domain extraction surfaces — Go core (`internal/domain/`, `internal/extract/`, `internal/pipeline/`, `internal/api/`, `internal/telegram/`), Python ML sidecar (`ml/app/domain.py`), SQL migration, NATS message contracts, dependencies.

### OWASP Top 10 Audit Summary

| Category | Status | Evidence |
|----------|--------|----------|
| A01 Broken Access Control | Clean | Auth middleware applied to all API routes via `bearerAuthMiddleware`/`webAuthMiddleware` with `subtle.ConstantTimeCompare`. Domain data inherits artifact-level access control. |
| A02 Cryptographic Failures | Clean | No secrets in domain extraction code. API keys passed as runtime config parameters. |
| A03 Injection (SQL) | Clean | All DB queries use parameterized placeholders (`$N`). Domain filters (`domain_data->>'domain' = $N`, `domain_data @> jsonb_build_object(...)`) use args array. `textSearch` escapes ILIKE patterns via `stringutil.EscapeLikePattern`. |
| A03 Injection (Command) | Clean | No shell exec, `os/exec`, or subprocess calls in domain extraction path. |
| A04 Insecure Design | Clean | Domain extraction is additive (fail-open, never blocks ingestion). Registry loads from server-controlled config directory. |
| A05 Security Misconfiguration | Clean | NATS message size limits implicit. Body size capped at 1MB via `http.MaxBytesReader`. Content truncated to 15000 chars before ML sidecar. |
| A06 Vulnerable Components | Clean | Go deps pinned at recent versions (pgx v5.7.2, nats.go v1.37.0). Python deps pinned (fastapi 0.115.0, litellm 1.50.0, pydantic 2.10.0). No known critical CVEs at review time. |
| A07 Auth Failures | Clean | Domain extraction triggered internally via NATS (no external auth surface). Search API requires bearer token. |
| A08 Data Integrity | Clean | `ValidateDomainExtractRequest` and `ValidateDomainExtractResponse` enforce required fields. JSON parsing validates structure. |
| A09 Logging/Monitoring | Clean | All domain extraction events logged with structured fields (artifact_id, contract_version, processing_ms). Prometheus metrics for extraction status. Dead-letter routing on exhausted retries. |
| A10 SSRF | Clean | Domain extraction operates on already-fetched content (no URL fetching). Upstream `extract.go` has comprehensive SSRF protection (IP validation, DNS rebinding guard, redirect validation). |

### Specific Checks Performed

1. **SQL injection in domain search filters** — `vectorSearch()`, `textSearch()`, `timeRangeSearch()` all use `$N` parameterized queries for `domain_data`, `ingredient`, and `domain` filters. No string interpolation of user input into SQL.
2. **Path traversal in registry loader** — `LoadRegistry()` uses `os.ReadDir` (returns basenames only) + `filepath.Join` with server-configured `contractsDir`. No user-controlled path components.
3. **IDOR on domain data** — Domain extraction updates use artifact IDs from internal NATS messages, not user-facing endpoints. No direct artifact ID manipulation surface.
4. **XSS in Telegram formatting** — `reply()` uses `tgbotapi.NewMessage` with no `ParseMode` set (plain text). LLM-generated strings in recipe/product cards cannot inject formatting.
5. **Unsafe deserialization** — Go uses `encoding/json` (safe). Python uses `json.loads` (safe). No pickle, yaml.load, or eval.
6. **Secret exposure** — No hardcoded secrets. `api_key` passed as runtime parameter to ML sidecar.
7. **Dependency audit** — All pinned versions reviewed; no known critical CVEs.
8. **Input validation boundaries** — API body limit (1MB), query length limit (10000), content truncation (15000 chars), NATS message size check.
9. **DNS rebinding** — `ssrfSafeTransport()` validates resolved IPs at connect-time in the extraction layer (upstream of domain extraction).

### Informational Notes (Defense-in-Depth — No Action Required)

1. **ML sidecar JSON Schema validation is minimal** — `ml/app/domain.py` validates LLM output is parseable JSON with a `domain` field but does not run full JSON Schema validation against the contract's `extraction_schema`. Since the data source is a controlled LLM system prompt and storage is typed JSONB, this is a data quality concern, not a security vulnerability.
2. **No explicit size check on DomainExtractResponse.DomainData** — NATS implicit message size limits bound this, but an explicit Go-side check before DB storage would add defense-in-depth.

### Conclusion

**No actionable security vulnerabilities found.** The domain extraction implementation follows security best practices: parameterized SQL, bounded inputs, fail-open with logging, internal-only NATS triggering, and proper auth on API surfaces. The two informational notes are defense-in-depth hardening opportunities, not exploitable vulnerabilities.

---

## Security Re-Scan — 2026-04-21 (stochastic-quality-sweep round)

**Trigger:** `security` via `security-to-doc` child workflow
**Verdict:** Clean — no new findings.

Re-scanned all domain extraction surfaces against OWASP Top 10. Confirmed:
- All SQL queries remain parameterized (`$N` placeholders with args arrays)
- `LoadRegistry()` path traversal guard intact (`os.ReadDir` + `filepath.Join`)
- NATS subjects are constants (`SubjectDomainExtract`, `SubjectDomainExtracted`)
- Content size bounds enforced (`maxDomainContentChars`, `MaxNATSMessageSize`)
- LLM timeout at 30s, retry cap at 2
- No new dependencies or code changes since last scan

CLI verification:
- `./smackerel.sh check` — passed (config in sync, env_file drift guard OK)
- `./smackerel.sh lint` — passed (Go + Python clean)
- `./smackerel.sh test unit` — 236 passed, 0 failed

---

## Gaps Analysis & Fix — 2026-04-21 (stochastic-quality-sweep round)

**Trigger:** `gaps` via `gaps-to-doc` child workflow
**Scope:** All 9 scopes — spec/design/scopes vs actual implementation comparison.

### Findings

| # | Gap | Severity | Scope | Status |
|---|-----|----------|-------|--------|
| G1 | `domain_data` not selected in search SQL; `SearchResult.DomainData` field never populated | Medium | 8 | Fixed |
| G2 | `PriceMax` parsed by `parseDomainIntent` but never applied as search filter | Medium | 8 | Fixed |
| G3 | No +0.15 domain score boost for domain-matched search results (spec DoD item) | Medium | 8 | Fixed |
| G4 | Multi-ingredient "and" parsing broken — "recipes with lemon and garlic" only captures "lemon" | Medium | 8 | Fixed |

### Fix Details

**G1 — domain_data in search SELECT/Scan:**
- Added `a.domain_data` to the vector search SELECT clause in `internal/api/search.go`
- Added `domainData []byte` scan target and populated `r.DomainData` when non-empty
- File: [internal/api/search.go](../../internal/api/search.go)

**G2 — PriceMax filter:**
- Added `PriceMax float64` field to `SearchFilters` struct
- Wired `intent.PriceMax` into `req.Filters.PriceMax` in domain intent integration
- Added SQL filter: `AND (a.domain_data->'price'->>'amount')::float <= $N`
- File: [internal/api/search.go](../../internal/api/search.go)

**G3 — Domain score boost:**
- Added +0.15 similarity boost (capped at 1.0) when `req.Filters.Domain != ""` and artifact has `domain_data`
- Boost applied after annotation boost, before relevance classification
- File: [internal/api/search.go](../../internal/api/search.go)

**G4 — Multi-ingredient parsing:**
- Removed `and` from the regex stop-words (`ingredientIntentRe`), allowing "and"-separated terms to flow into the captured group
- Added post-capture split on both `,` and ` and ` to extract individual ingredients
- Strengthened `TestParseDomainIntent_RecipeMultipleIngredients` to assert exact count (2) instead of `>= 1`
- Added `TestParseDomainIntent_LemonAndGarlic` (spec T8-02) and `TestParseDomainIntent_DishesWithMushrooms` (spec T8-05)
- Files: [internal/api/domain_intent.go](../../internal/api/domain_intent.go), [internal/api/domain_intent_test.go](../../internal/api/domain_intent_test.go)

### Verification

- `./smackerel.sh test unit` — all Go packages pass (including `internal/api` with new tests), 236 Python tests pass
- `./smackerel.sh build` — clean compilation

---

## Test Gap Probe — 2026-04-21 (stochastic-quality-sweep R74)

**Trigger:** `test` via `test-to-doc` child workflow
**Scope:** All 9 scopes — test plan vs actual test coverage comparison.

### Methodology

Compared every test plan row (T1-01 through T9-08) from `scopes.md` against actual test files in the codebase. Checked for:
- Missing test functions mapped to test plan IDs
- Missing test files referenced by test plans
- Gherkin scenario coverage gaps

### Test Coverage Findings

| # | Gap | Severity | Scope | Test Plan IDs | Status |
|---|-----|----------|-------|---------------|--------|
| TG1 | No unit tests for `DomainResultSubscriber.handleDomainExtracted` or `publishDomainExtractionRequest` | Medium | 3, 7 | T3-01 to T3-07, T7-01 to T7-04 | Fixed — added `domain_subscriber_test.go` |
| TG2 | No unit tests for domain search filter serialization or domain intent → filter mapping | Medium | 8 | T8-06, T8-07 | Fixed — added `domain_filter_test.go` |
| TG3 | No integration test `tests/integration/domain_extraction_test.go` | Low | 7 | T7-05 to T7-07 | Documented — requires live stack |

### Gap Detail

**TG1 — DomainResultSubscriber and publisher unit tests:**

The `DomainResultSubscriber` in `internal/pipeline/domain_subscriber.go` and the `publishDomainExtractionRequest` method on `ResultSubscriber` had zero unit test coverage. The test plan called for T3-01 through T3-07 in `subscriber_test.go` and T7-01 through T7-04 for pipeline integration logic.

**Fix:** Created [internal/pipeline/domain_subscriber_test.go](../../internal/pipeline/domain_subscriber_test.go) with:
- `TestHandleDomainExtracted_SuccessPayload` — validates successful response structure (T3-05)
- `TestHandleDomainExtracted_FailurePayload` — validates failure response structure (T3-06)
- `TestHandleDomainExtracted_InvalidJSONDetected` — verifies bad JSON is detected (T3-07)
- `TestHandleDomainExtracted_MissingArtifactIDRejected` — missing artifact_id caught
- `TestDomainResultSubscriber_NewCreation` — constructor produces valid subscriber
- `TestDomainResultSubscriber_StopBeforeStart` — no panic on unstarted Stop
- `TestDomainResultSubscriber_DoubleStartFails` — rejects duplicate Start
- `TestDomainResultSubscriber_StartAfterStopFails` — rejects Start after Stop
- `TestPublishDomainExtractionRequest_NilRegistrySkips` — nil registry returns nil (T3-02)

Note: Full DB-dependent publisher tests (T3-01, T3-03, T3-04) and handler DB-write tests (T3-05 complete, T3-06 complete) require a `pgxpool.Pool` mock or live DB, which is integration-category. The validation and serialization layers are tested at unit level.

**TG2 — Domain search filter tests:**

The test plan called for T8-06 (`addDomainFilters` JSONB SQL for recipe ingredients) and T8-07 (`addDomainFilters` JSONB SQL for product price). The implementation inlined the filter logic into `vectorSearch()` rather than extracting a separate `addDomainFilters` function, so no tests existed for these code paths.

**Fix:** Created [internal/api/domain_filter_test.go](../../internal/api/domain_filter_test.go) with:
- `TestSearchFilters_DomainFieldSerialization` — domain + ingredient round-trip (T8-06)
- `TestSearchFilters_PriceMaxSerialization` — PriceMax round-trip (T8-07)
- `TestSearchFilters_DomainOmittedWhenEmpty` — omitempty correctness
- `TestSearchResult_DomainDataSerialization` — domain_data present/absent in results
- `TestDomainIntentToSearchFilters` — full intent → filter mapping for recipe and product (T8-06, T8-07)
- `TestDomainIntentDoesNotOverrideExplicitFilters` — explicit filters take precedence

**TG3 — Integration tests (documented, not fixed):**

The test plan references `tests/integration/domain_extraction_test.go` for T7-05 through T7-07 (recipe artifact → domain_data in DB, article artifact → no extraction, short-content → skipped). These require a running PostgreSQL + NATS + ML sidecar stack. The E2E test `tests/e2e/domain_e2e_test.go` partially covers this path. Full integration tests are deferred to live-stack testing (spec 031).

### Existing Coverage Summary (No Gaps)

| Scope | Test File | Status |
|-------|-----------|--------|
| 1 — DB Migration & Types | `internal/pipeline/domain_types_test.go` | T1-01 to T1-05 covered |
| 1 — Migration | `tests/integration/db_migration_test.go` | Domain columns verified |
| 2 — Registry | `internal/domain/registry_test.go` | T2-01 to T2-07 covered + real contracts |
| 4 — ML Sidecar | `ml/tests/test_domain.py` | T4-01 to T4-08 covered |
| 5 — Recipe Contract | `internal/domain/registry_test.go` + `ml/tests/test_domain.py` | T5-01 to T5-05 covered |
| 6 — Product Contract | `internal/domain/registry_test.go` + `ml/tests/test_domain.py` | T6-01 to T6-05 covered |
| 8 — Search Intent | `internal/api/domain_intent_test.go` | T8-01 to T8-05 covered |
| 8 — E2E Search | `tests/e2e/domain_e2e_test.go` | T8-08 covered |
| 9 — Telegram Display | `internal/telegram/format_test.go` | T9-01 to T9-08 covered |

### Verification

```
./smackerel.sh test unit — all Go packages pass, 236 Python tests pass
./smackerel.sh lint — clean
```

---

## Simplification Probe — 2026-04-22 (stochastic-quality-sweep round)

**Trigger:** `simplify` via `simplify-to-doc` child workflow
**Scope:** All domain extraction implementation surfaces — Go core (`internal/domain/`, `internal/pipeline/`, `internal/api/`, `internal/telegram/`), Python ML sidecar (`ml/app/domain.py`).

### Methodology

Reviewed all implementation files for:
- Unnecessary abstraction layers
- Duplicate code or data paths
- Dead code or unused exports
- Redundant DB queries
- Over-engineered control flow
- Consolidation opportunities

### Findings

| # | Finding | Severity | File | Status |
|---|---------|----------|------|--------|
| S-001 | Two separate DB queries to `artifacts` table for the same row in `publishDomainExtractionRequest` | Low | `internal/pipeline/subscriber.go` | Fixed |

### Detail

**S-001 — Consolidate two DB round-trips into one:**

`publishDomainExtractionRequest` made two separate `QueryRow` calls against the same `artifacts` row:
1. `SELECT COALESCE(source_url, '') FROM artifacts WHERE id = $1` — for URL qualifier matching
2. `SELECT COALESCE(content_raw, ''), COALESCE(title, ''), COALESCE(summary, '') FROM artifacts WHERE id = $1` — for extraction content

Merged into a single query selecting all four columns in one round-trip. Eliminates one DB call per domain extraction dispatch. Behavior unchanged.

### Areas Reviewed — No Findings

- **`internal/domain/registry.go`** (~150 lines) — Clean single-purpose package. Two maps (byContentType, byURLPattern), LoadRegistry/Match/Count. No over-engineering.
- **`internal/pipeline/domain_types.go`** (~70 lines) — Minimal request/response types + validation. Defense-in-depth `maxDomainDataBytes` constant is justified.
- **`internal/pipeline/domain_subscriber.go`** (~200 lines) — Separate lifecycle subscriber for domain.extracted. The split from `ResultSubscriber` is justified: different NATS stream, different consumer config, independent shutdown.
- **`internal/api/domain_intent.go`** (~93 lines) — Five pre-compiled regexes at package level. Clean dispatch pattern. No dead paths.
- **`internal/telegram/format.go`** domain section (~200 lines) — Struct-per-domain approach with constants for truncation limits. Clean switch dispatch.
- **`ml/app/domain.py`** (~270 lines) — Hardcoded prompts per domain type are simpler than dynamic YAML loading (design drift noted but not a simplification target; current approach is actually the simpler path).

### Verification

```
./smackerel.sh test unit — all Go packages pass (internal/pipeline recompiled), 263 Python tests pass
./smackerel.sh check — config in sync, env_file drift guard OK
```
