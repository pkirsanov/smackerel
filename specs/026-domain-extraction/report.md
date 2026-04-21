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
