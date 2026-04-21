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
