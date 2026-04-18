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
