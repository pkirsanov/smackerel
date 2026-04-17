# Scopes

Links: [spec.md](spec.md) | [design.md](design.md) | [uservalidation.md](uservalidation.md)

**TDD Policy:** scenario-first — tests written alongside implementation per scope, with failing targeted tests preceding green evidence for each Gherkin scenario.

---

## Execution Outline

### Phase Order

1. **Scope 1 — DB Migration & Domain Data Types** — Migration `015_domain_extraction.sql` + Go types for domain extraction request/response + artifact model extension. Foundation for all subsequent scopes.
2. **Scope 2 — Domain Schema Registry** — `internal/domain/registry.go` contract loader + content_type→contract index + URL qualifier matching. Enables pipeline dispatch.
3. **Scope 3 — NATS Domain Extraction Subjects & Go Publisher** — NATS contract update + DOMAIN stream + Go publisher in `ResultSubscriber` + domain extracted result handler. First extraction messages flow through NATS.
4. **Scope 4 — ML Sidecar Domain Extraction Handler** — Python `domain.extract` consumer + prompt builder + LLM call + JSON Schema validation + `domain.extracted` publisher. First end-to-end domain extraction path.
5. **Scope 5 — Recipe Extraction Prompt Contract** — `recipe-extraction-v1.yaml` prompt contract + recipe-specific validation rules + unit tests against fixture content. First concrete domain.
6. **Scope 6 — Product Extraction Prompt Contract** — `product-extraction-v1.yaml` prompt contract + product-specific validation rules + unit tests. Second concrete domain.
7. **Scope 7 — Pipeline Integration** — Wire domain extraction dispatch into `handleMessage` after universal processing + status lifecycle (pending→completed/failed/skipped) + content length gating. Extraction fires for real artifacts.
8. **Scope 8 — Search Extension** — Domain intent detection (recipe ingredients, product price) + JSONB query augmentation + domain score boosting + `SearchResult.DomainData` field. Domain data becomes queryable.
9. **Scope 9 — Telegram Display** — Recipe card rendering + product card rendering + domain card dispatch in artifact formatting. Domain data reaches the user.

### New Types & Signatures

**Go (`internal/domain/`):**
- `type Registry struct` — maps content_type → DomainContract, URL pattern entries
- `type DomainContract struct` — extends `knowledge.PromptContract` with ContentTypes, URLQualifiers, MinContentLength
- `func LoadRegistry(contractsDir string) (*Registry, error)`
- `func (r *Registry) Match(contentType, sourceURL string) *DomainContract`
- `func (r *Registry) Count() int`

**Go (`internal/pipeline/`):**
- `type DomainExtractRequest struct` — ArtifactID, ContentType, Title, Summary, ContentRaw, SourceURL, ContractVersion, RetryCount, TraceID
- `type DomainExtractResponse struct` — ArtifactID, Success, Error, DomainData (json.RawMessage), ContractVersion, ProcessingTimeMs, ModelUsed, TokensUsed
- `func (rs *ResultSubscriber) publishDomainExtractionRequest(ctx, payload) error`
- `func (rs *ResultSubscriber) handleDomainMessage(ctx, msg) `

**Go (`internal/nats/`):**
- `SubjectDomainExtract = "domain.extract"`
- `SubjectDomainExtracted = "domain.extracted"`

**Go (`internal/api/`):**
- `type DomainIntent struct` — Domain, Attributes, PriceMax, PriceCurrency, Cleaned
- `func parseDomainIntent(query string) *DomainIntent`
- `func addDomainFilters(query, args, argN, intent) (string, []any, int)`
- `SearchFilters.Domain`, `SearchFilters.Ingredient` fields
- `SearchResult.DomainData json.RawMessage` field

**Go (`internal/telegram/`):**
- `func formatDomainCard(domainData json.RawMessage) string`
- `func formatRecipeCard(data json.RawMessage) string`
- `func formatProductCard(data json.RawMessage) string`

**Python (`ml/app/domain.py`):**
- `async def handle_domain_extract(data, provider, model, api_key, ollama_url) -> dict`
- `def build_domain_prompt(contract, artifact) -> str`

**SQL (`internal/db/migrations/015_domain_extraction.sql`):**
- `ALTER TABLE artifacts ADD COLUMN domain_data JSONB`
- `ALTER TABLE artifacts ADD COLUMN domain_extraction_status TEXT`
- `ALTER TABLE artifacts ADD COLUMN domain_schema_version TEXT`
- `ALTER TABLE artifacts ADD COLUMN domain_extracted_at TIMESTAMPTZ`
- GIN index on `domain_data`, partial indexes on `domain_extraction_status` and `domain_schema_version`

### Validation Checkpoints

- After Scope 1: `./smackerel.sh test unit` — migration applies, Go types compile, type validation tests pass
- After Scope 2: `./smackerel.sh test unit` — registry loads contracts, matches by content_type and URL qualifier, rejects duplicates
- After Scope 3: `./smackerel.sh test unit` — NATS subjects defined, publisher serializes correctly, result handler stores domain_data
- After Scope 4: `./smackerel.sh test unit` — ML sidecar builds prompts, validates schema, handles LLM failures
- After Scope 5: `./smackerel.sh test unit` — recipe contract loads, recipe fixture validates against schema
- After Scope 6: `./smackerel.sh test unit` — product contract loads, product fixture validates against schema
- After Scope 7: `./smackerel.sh test unit` + `./smackerel.sh test integration` — end-to-end: artifact → domain extraction → domain_data in DB
- After Scope 8: `./smackerel.sh test unit` + `./smackerel.sh test e2e` — search by ingredient/price returns domain-matched results
- After Scope 9: `./smackerel.sh test unit` — Telegram renders recipe/product cards correctly; full regression: `./smackerel.sh test unit` + `./smackerel.sh test integration` + `./smackerel.sh test e2e`

---

## Scope Summary

| # | Name | Surfaces | Key Tests | Status |
|---|------|----------|-----------|--------|
| 1 | DB Migration & Domain Data Types | PostgreSQL, Go types | unit: migration applies, type validation | Not Started |
| 2 | Domain Schema Registry | Go core | unit: contract loading, matching, duplicate rejection | Not Started |
| 3 | NATS Subjects & Go Publisher | Go pipeline, NATS | unit: publisher, result handler, NATS contract alignment | Not Started |
| 4 | ML Sidecar Domain Handler | Python ML | unit: prompt build, schema validation, error handling | Not Started |
| 5 | Recipe Extraction Contract | Config, Python ML | unit: contract loads, fixture validates, prompt correct | Not Started |
| 6 | Product Extraction Contract | Config, Python ML | unit: contract loads, fixture validates, prompt correct | Not Started |
| 7 | Pipeline Integration | Go pipeline, Go core | unit: dispatch logic; integration: artifact→domain_data round-trip | Not Started |
| 8 | Search Extension | Go API | unit: intent detection, JSONB filters; e2e: search by ingredient/price | Not Started |
| 9 | Telegram Display | Go telegram | unit: recipe card, product card rendering | Not Started |

---

## Scope 1: DB Migration & Domain Data Types

**Status:** Not Started
**Priority:** P0
**Depends On:** Spec 025 Scope 1 (Knowledge Store migration 014 must exist)

### Gherkin Scenarios

```gherkin
Scenario: Domain extraction columns are added by migration
  Given the database has the existing schema through migration 014
  When migration 015_domain_extraction.sql is applied
  Then the artifacts table has columns domain_data, domain_extraction_status, domain_schema_version, domain_extracted_at
  And a GIN index exists on domain_data with jsonb_path_ops
  And a partial index exists on domain_extraction_status for 'pending' and 'failed' rows
  And a partial index exists on domain_schema_version for non-NULL rows

Scenario: Domain extraction request type validates required fields
  Given a DomainExtractRequest struct
  When ArtifactID is empty
  Then validation fails with "artifact_id required"
  When ContractVersion is empty
  Then validation fails with "contract_version required"
  When all required fields are set
  Then validation passes

Scenario: Domain extraction response handles success and failure
  Given a DomainExtractResponse struct
  When Success is true and DomainData is populated
  Then the response is valid
  When Success is false and Error is populated
  Then the response is valid with empty DomainData
  When ArtifactID is empty
  Then validation fails regardless of Success
```

### Implementation Plan

**Files to create:**
- `internal/db/migrations/015_domain_extraction.sql` — migration adding domain columns and indexes
- `internal/pipeline/domain_types.go` — `DomainExtractRequest`, `DomainExtractResponse` structs with validation

**Files to modify:**
- None — this scope is pure additions

**Config SST:** No config changes needed for this scope. Migration numbering follows existing sequence.

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T1-01 | unit | `internal/pipeline/domain_types_test.go` | SCN-026-01 | DomainExtractRequest validates required fields (artifact_id, contract_version) |
| T1-02 | unit | `internal/pipeline/domain_types_test.go` | SCN-026-01 | DomainExtractRequest accepts valid input |
| T1-03 | unit | `internal/pipeline/domain_types_test.go` | SCN-026-01 | DomainExtractResponse validates artifact_id required |
| T1-04 | unit | `internal/pipeline/domain_types_test.go` | SCN-026-01 | DomainExtractResponse success=true requires DomainData |
| T1-05 | unit | `internal/pipeline/domain_types_test.go` | SCN-026-01 | DomainExtractResponse success=false allows empty DomainData |
| T1-06 | unit | `internal/db/migrations_test.go` | SCN-026-01 | Migration 015 applies cleanly after 014 |

### Definition of Done

- [ ] Migration `015_domain_extraction.sql` adds `domain_data JSONB`, `domain_extraction_status TEXT`, `domain_schema_version TEXT`, `domain_extracted_at TIMESTAMPTZ` to artifacts
  > **Phase:** implement

- [ ] GIN index `idx_artifacts_domain_data_gin` created with `jsonb_path_ops` on `domain_data`
  > **Phase:** implement

- [ ] Partial indexes created on `domain_extraction_status` (pending/failed) and `domain_schema_version` (non-NULL)
  > **Phase:** implement

- [ ] `DomainExtractRequest` struct with Validate() method rejects empty artifact_id and contract_version
  > **Phase:** implement

- [ ] `DomainExtractResponse` struct with Validate() method enforces artifact_id required, DomainData required when success=true
  > **Phase:** implement

- [ ] All unit tests pass: `./smackerel.sh test unit`
  > **Phase:** test

---

## Scope 2: Domain Schema Registry

**Status:** Not Started
**Priority:** P0
**Depends On:** Scope 1

### Gherkin Scenarios

```gherkin
Scenario: Domain contracts are loaded from YAML at startup
  Given valid domain-extraction YAML files exist in config/prompt_contracts/
  When LoadRegistry is called with the contracts directory
  Then all files with type "domain-extraction" are parsed into DomainContract structs
  And the registry maps each content_type to its contract
  And the registry count matches the number of domain contracts loaded
  And non-domain contracts (type != "domain-extraction") are ignored

Scenario: Duplicate content_type in domain contracts is rejected
  Given two domain-extraction YAML files both claim content_type "recipe"
  When LoadRegistry is called
  Then it returns an error mentioning the duplicate content_type and both contract versions

Scenario: Registry matches artifact by content_type
  Given a registry loaded with recipe-extraction-v1 (content_types: ["recipe"])
  When Match is called with contentType="recipe" and any sourceURL
  Then it returns the recipe-extraction-v1 contract

Scenario: Registry matches artifact by URL qualifier
  Given a registry loaded with recipe-extraction-v1 (url_qualifiers: ["allrecipes", "epicurious"])
  When Match is called with contentType="article" and sourceURL containing "allrecipes.com"
  Then it returns the recipe-extraction-v1 contract
  When Match is called with contentType="article" and sourceURL containing "cnn.com"
  Then it returns nil

Scenario: Registry returns nil for unmatched content type
  Given a registry loaded with recipe and product contracts
  When Match is called with contentType="article" and sourceURL="https://cnn.com/news"
  Then it returns nil
```

### Implementation Plan

**Files to create:**
- `internal/domain/registry.go` — `Registry`, `DomainContract`, `LoadRegistry`, `Match`, `Count`
- `internal/domain/registry_test.go` — unit tests

**Files to modify:**
- None — pure new package

**Design reference:** `design.md` § "Go Core Implementation → Domain Registry"

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T2-01 | unit | `internal/domain/registry_test.go` | SCN-026-02 | LoadRegistry loads domain-extraction contracts, ignores others |
| T2-02 | unit | `internal/domain/registry_test.go` | SCN-026-02 | LoadRegistry rejects duplicate content_type across contracts |
| T2-03 | unit | `internal/domain/registry_test.go` | SCN-026-02 | Match returns contract for direct content_type match |
| T2-04 | unit | `internal/domain/registry_test.go` | SCN-026-02 | Match returns contract for URL qualifier match |
| T2-05 | unit | `internal/domain/registry_test.go` | SCN-026-02 | Match returns nil for unmatched content_type and URL |
| T2-06 | unit | `internal/domain/registry_test.go` | SCN-026-02 | Count returns correct number of loaded domain contracts |
| T2-07 | unit | `internal/domain/registry_test.go` | SCN-026-02 | Match on nil registry returns nil (no panic) |

### Definition of Done

- [ ] `internal/domain/registry.go` package created with `Registry`, `DomainContract`, `LoadRegistry`, `Match`, `Count`
  > **Phase:** implement

- [ ] `LoadRegistry` correctly parses `type: "domain-extraction"` contracts and ignores other types
  > **Phase:** implement

- [ ] `LoadRegistry` returns error on duplicate `content_type` across contracts
  > **Phase:** implement

- [ ] `Match` returns correct contract for direct content_type match
  > **Phase:** implement

- [ ] `Match` returns correct contract for URL qualifier substring match (case-insensitive)
  > **Phase:** implement

- [ ] `Match` returns nil when no contract matches; nil receiver returns nil (no panic)
  > **Phase:** implement

- [ ] All unit tests pass: `./smackerel.sh test unit`
  > **Phase:** test

---

## Scope 3: NATS Domain Extraction Subjects & Go Publisher

**Status:** Not Started
**Priority:** P0
**Depends On:** Scope 1, Scope 2

### Gherkin Scenarios

```gherkin
Scenario: NATS contract includes domain extraction subjects
  Given the nats_contract.json file
  When domain extraction subjects are defined
  Then "domain.extract" has direction "core_to_ml" with response "domain.extracted"
  And "domain.extracted" has direction "ml_to_core" with request "domain.extract"
  And both subjects use the DOMAIN stream
  And both subjects are marked critical: false

Scenario: Go publisher serializes and publishes domain extraction request
  Given a processed artifact with content_type "recipe" and a matching domain contract
  When publishDomainExtractionRequest is called
  Then a DomainExtractRequest is marshalled to JSON
  And published to subject "domain.extract"
  And the artifact's domain_extraction_status is set to "pending"
  And the artifact's domain_schema_version is set to the contract version

Scenario: Go publisher skips artifacts with no matching domain contract
  Given a processed artifact with content_type "article" and no matching domain contract
  When publishDomainExtractionRequest is called
  Then no NATS message is published
  And the artifact's domain_extraction_status remains NULL

Scenario: Go publisher skips artifacts with content below minimum length
  Given a processed artifact with content_type "recipe" and content of 50 characters
  And the recipe contract has min_content_length 200
  When publishDomainExtractionRequest is called
  Then no NATS message is published
  And the artifact's domain_extraction_status is set to "skipped"

Scenario: Domain extraction result handler stores successful result
  Given a domain.extracted message with success=true and valid domain_data
  When handleDomainMessage processes the message
  Then the artifact's domain_data column is set to the response domain_data
  And domain_extraction_status is set to "completed"
  And domain_extracted_at is set to now
  And the message is acked

Scenario: Domain extraction result handler records failure
  Given a domain.extracted message with success=false and an error message
  When handleDomainMessage processes the message
  Then the artifact's domain_extraction_status is set to "failed"
  And domain_extracted_at is set to now
  And the message is acked (no infinite retry)
```

### Implementation Plan

**Files to create:**
- `internal/nats/domain_subjects.go` — `SubjectDomainExtract`, `SubjectDomainExtracted` constants (or add to existing client.go)

**Files to modify:**
- `config/nats_contract.json` — add `domain.extract` and `domain.extracted` entries + DOMAIN stream
- `internal/pipeline/subscriber.go` — add `publishDomainExtractionRequest` method, `handleDomainMessage` method, `DomainRegistry` field on `ResultSubscriber`

**Design reference:** `design.md` § "NATS Topic Design", "Subscriber Extension", "Domain Extraction Result Handler"

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T3-01 | unit | `internal/pipeline/subscriber_test.go` | SCN-026-03 | publishDomainExtractionRequest publishes when contract matches |
| T3-02 | unit | `internal/pipeline/subscriber_test.go` | SCN-026-03 | publishDomainExtractionRequest skips when no contract matches |
| T3-03 | unit | `internal/pipeline/subscriber_test.go` | SCN-026-03 | publishDomainExtractionRequest skips when content too short, sets status=skipped |
| T3-04 | unit | `internal/pipeline/subscriber_test.go` | SCN-026-03 | publishDomainExtractionRequest sets status=pending and schema_version before publish |
| T3-05 | unit | `internal/pipeline/subscriber_test.go` | SCN-026-03 | handleDomainMessage stores domain_data on success, sets status=completed |
| T3-06 | unit | `internal/pipeline/subscriber_test.go` | SCN-026-03 | handleDomainMessage sets status=failed on failure response, acks |
| T3-07 | unit | `internal/pipeline/subscriber_test.go` | SCN-026-03 | handleDomainMessage acks invalid JSON payload (no redelivery loop) |
| T3-08 | contract | `tests/integration/nats_contract_test.go` | SCN-026-03 | domain.extract and domain.extracted in nats_contract.json with correct stream/direction |

### Definition of Done

- [ ] NATS constants `SubjectDomainExtract` and `SubjectDomainExtracted` defined in Go
  > **Phase:** implement

- [ ] `config/nats_contract.json` updated with `domain.extract` and `domain.extracted` entries in DOMAIN stream
  > **Phase:** implement

- [ ] `ResultSubscriber.DomainRegistry` field added; `publishDomainExtractionRequest` method implemented
  > **Phase:** implement

- [ ] Publisher sets `domain_extraction_status = 'pending'` and `domain_schema_version` before publishing
  > **Phase:** implement

- [ ] Publisher skips when no contract matches (no publish, status stays NULL)
  > **Phase:** implement

- [ ] Publisher skips when content below `min_content_length` (no publish, status set to 'skipped')
  > **Phase:** implement

- [ ] `handleDomainMessage` stores domain_data and sets status=completed on success
  > **Phase:** implement

- [ ] `handleDomainMessage` sets status=failed on failure, acks message (no infinite retry)
  > **Phase:** implement

- [ ] All unit tests pass: `./smackerel.sh test unit`
  > **Phase:** test

---

## Scope 4: ML Sidecar Domain Extraction Handler

**Status:** Not Started
**Priority:** P0
**Depends On:** Scope 3

### Gherkin Scenarios

```gherkin
Scenario: ML sidecar builds domain extraction prompt from contract and artifact
  Given a recipe-extraction-v1 contract with system_prompt and extraction_schema
  And an artifact with title, summary, and content_raw
  When build_domain_prompt is called
  Then the prompt includes the system_prompt text
  And the prompt includes the artifact title, content_type, and summary
  And the prompt includes the truncated content_raw (max 8000 chars)
  And the prompt includes the extraction_schema as JSON

Scenario: ML sidecar calls LLM and validates output against schema
  Given a valid domain extraction prompt
  When the LLM returns valid JSON matching the contract's extraction_schema
  Then handle_domain_extract returns success=true with the parsed domain_data
  And processing_time_ms and tokens_used are populated

Scenario: ML sidecar retries on transient LLM failure
  Given a valid domain extraction prompt
  When the first LLM call raises RateLimitError
  And the second LLM call succeeds
  Then handle_domain_extract returns success=true with the result from attempt 2

Scenario: ML sidecar fails after max retries
  Given a valid domain extraction prompt
  When all 3 LLM call attempts raise ServiceUnavailableError
  Then handle_domain_extract returns success=false
  And the error message mentions all 3 attempts failed

Scenario: ML sidecar rejects invalid JSON from LLM
  Given a valid domain extraction prompt
  When the LLM returns text that is not valid JSON
  Then handle_domain_extract returns success=false with "Invalid JSON from LLM"

Scenario: ML sidecar rejects schema-invalid JSON from LLM
  Given a valid domain extraction prompt
  When the LLM returns valid JSON that does not match the extraction_schema
  Then handle_domain_extract returns success=false with schema validation error
```

### Implementation Plan

**Files to create:**
- `ml/app/domain.py` — `handle_domain_extract`, `build_domain_prompt`, `_elapsed_ms`
- `ml/tests/test_domain.py` — unit tests

**Files to modify:**
- `ml/app/nats_client.py` — add `domain.extract` to `SUBSCRIBE_SUBJECTS`, `domain.extracted` to `PUBLISH_SUBJECTS`, routing in message dispatch

**Design reference:** `design.md` § "ML Sidecar Implementation"

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T4-01 | unit | `ml/tests/test_domain.py` | SCN-026-04 | build_domain_prompt includes system_prompt, title, content, schema |
| T4-02 | unit | `ml/tests/test_domain.py` | SCN-026-04 | build_domain_prompt truncates content_raw at 8000 chars |
| T4-03 | unit | `ml/tests/test_domain.py` | SCN-026-04 | handle_domain_extract returns success on valid LLM JSON matching schema |
| T4-04 | unit | `ml/tests/test_domain.py` | SCN-026-04 | handle_domain_extract retries on RateLimitError, succeeds on attempt 2 |
| T4-05 | unit | `ml/tests/test_domain.py` | SCN-026-04 | handle_domain_extract fails after 3 failed attempts |
| T4-06 | unit | `ml/tests/test_domain.py` | SCN-026-04 | handle_domain_extract returns error on invalid JSON response |
| T4-07 | unit | `ml/tests/test_domain.py` | SCN-026-04 | handle_domain_extract returns error on schema-invalid JSON |
| T4-08 | unit | `ml/tests/test_domain.py` | SCN-026-04 | handle_domain_extract returns error when contract file not found |

### Definition of Done

- [ ] `ml/app/domain.py` created with `handle_domain_extract` and `build_domain_prompt`
  > **Phase:** implement

- [ ] `build_domain_prompt` assembles system_prompt + artifact fields + schema into LLM prompt, truncates content at 8000 chars
  > **Phase:** implement

- [ ] `handle_domain_extract` calls LLM with retry (3 attempts, exponential backoff) for transient errors
  > **Phase:** implement

- [ ] `handle_domain_extract` validates LLM output against contract's `extraction_schema` using JSON Schema
  > **Phase:** implement

- [ ] `handle_domain_extract` returns `success=false` with descriptive error for invalid JSON, schema failure, and max retries exhausted
  > **Phase:** implement

- [ ] `ml/app/nats_client.py` routes `domain.extract` to handler and publishes result to `domain.extracted`
  > **Phase:** implement

- [ ] All unit tests pass: `./smackerel.sh test unit`
  > **Phase:** test

---

## Scope 5: Recipe Extraction Prompt Contract

**Status:** Not Started
**Priority:** P1
**Depends On:** Scope 2, Scope 4

### Gherkin Scenarios

```gherkin
Scenario: Recipe prompt contract loads and validates (BS-007 partial)
  Given config/prompt_contracts/recipe-extraction-v1.yaml exists
  And it has type "domain-extraction" and content_types ["recipe"]
  When the domain registry loads it
  Then the contract has version "recipe-extraction-v1"
  And content_types contains "recipe"
  And url_qualifiers include "allrecipes", "epicurious", "foodnetwork"
  And min_content_length is 200
  And extraction_schema is a valid JSON Schema requiring "domain", "ingredients", "steps"

Scenario: Recipe extraction produces valid structured data (BS-001)
  Given a recipe URL has been processed with universal extraction
  And the content contains ingredients, steps, and timing information
  When domain extraction runs with the recipe-extraction-v1 contract
  Then the domain_data contains a "domain" field set to "recipe"
  And the domain_data contains an "ingredients" array with name, quantity, unit per item
  And the domain_data contains a "steps" array with numbered instructions
  And the domain_data contains techniques, timing, servings, and cuisine fields where available

Scenario: Recipe schema rejects invalid extraction output
  Given a recipe extraction LLM response with empty ingredients array
  When validated against the recipe extraction_schema
  Then validation fails because ingredients requires minItems 1
```

### Implementation Plan

**Files to create:**
- `config/prompt_contracts/recipe-extraction-v1.yaml` — full recipe domain contract

**Files to modify:**
- None — contract-driven, no code changes

**Design reference:** `design.md` § "Domain Extraction Prompt Contract Format" (recipe-extraction-v1)

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T5-01 | unit | `internal/domain/registry_test.go` | SCN-026-05 | recipe-extraction-v1.yaml loads via LoadRegistry with correct fields |
| T5-02 | unit | `ml/tests/test_domain.py` | SCN-026-05 | Recipe fixture JSON validates against recipe extraction_schema |
| T5-03 | unit | `ml/tests/test_domain.py` | SCN-026-05 | Empty ingredients array fails recipe schema validation |
| T5-04 | unit | `ml/tests/test_domain.py` | SCN-026-05 | Missing "domain" field fails recipe schema validation |
| T5-05 | unit | `ml/tests/test_domain.py` | SCN-026-05 | build_domain_prompt with recipe contract includes recipe system_prompt |

### Definition of Done

- [ ] `config/prompt_contracts/recipe-extraction-v1.yaml` created with type `domain-extraction`, content_types `["recipe"]`, url_qualifiers, min_content_length, system_prompt, and extraction_schema
  > **Phase:** implement

- [ ] Extraction schema requires `domain` (const "recipe"), `ingredients` (array, minItems 1), and `steps` (array, minItems 1)
  > **Phase:** implement

- [ ] Contract loads successfully via `LoadRegistry` and matches content_type "recipe"
  > **Phase:** implement

- [ ] A realistic recipe fixture validates against the schema; an empty-ingredients fixture is rejected
  > **Phase:** implement

- [ ] All unit tests pass: `./smackerel.sh test unit`
  > **Phase:** test

---

## Scope 6: Product Extraction Prompt Contract

**Status:** Not Started
**Priority:** P1
**Depends On:** Scope 2, Scope 4

### Gherkin Scenarios

```gherkin
Scenario: Product prompt contract loads and validates
  Given config/prompt_contracts/product-extraction-v1.yaml exists
  And it has type "domain-extraction" and content_types ["product"]
  When the domain registry loads it
  Then the contract has version "product-extraction-v1"
  And content_types contains "product"
  And url_qualifiers include "amazon", "ebay", "bestbuy"
  And extraction_schema is a valid JSON Schema requiring "domain", "product_name"

Scenario: Product extraction produces valid structured data (BS-002)
  Given a product URL has been processed with universal extraction
  And the content contains product name, brand, specs, and pricing
  When domain extraction runs with the product-extraction-v1 contract
  Then the domain_data contains a "domain" field set to "product"
  And the domain_data contains product_name, brand, category
  And the domain_data contains price with amount and currency
  And the domain_data contains specs as name-value pairs

Scenario: Product schema rejects invalid extraction output
  Given a product extraction LLM response with missing product_name
  When validated against the product extraction_schema
  Then validation fails because product_name is required
```

### Implementation Plan

**Files to create:**
- `config/prompt_contracts/product-extraction-v1.yaml` — full product domain contract

**Files to modify:**
- None — contract-driven, no code changes

**Design reference:** `design.md` § "Domain Extraction Prompt Contract Format" (product-extraction-v1), `spec.md` § "Domain Extraction Schemas"

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T6-01 | unit | `internal/domain/registry_test.go` | SCN-026-06 | product-extraction-v1.yaml loads via LoadRegistry with correct fields |
| T6-02 | unit | `ml/tests/test_domain.py` | SCN-026-06 | Product fixture JSON validates against product extraction_schema |
| T6-03 | unit | `ml/tests/test_domain.py` | SCN-026-06 | Missing product_name fails product schema validation |
| T6-04 | unit | `ml/tests/test_domain.py` | SCN-026-06 | Missing "domain" field fails product schema validation |
| T6-05 | unit | `ml/tests/test_domain.py` | SCN-026-06 | build_domain_prompt with product contract includes product system_prompt |

### Definition of Done

- [ ] `config/prompt_contracts/product-extraction-v1.yaml` created with type `domain-extraction`, content_types `["product"]`, url_qualifiers, system_prompt, and extraction_schema
  > **Phase:** implement

- [ ] Extraction schema requires `domain` (const "product") and `product_name` (string)
  > **Phase:** implement

- [ ] Contract loads successfully via `LoadRegistry` and matches content_type "product"
  > **Phase:** implement

- [ ] A realistic product fixture validates against the schema; a missing-product_name fixture is rejected
  > **Phase:** implement

- [ ] All unit tests pass: `./smackerel.sh test unit`
  > **Phase:** test

---

## Scope 7: Pipeline Integration

**Status:** Not Started
**Priority:** P0
**Depends On:** Scope 1, Scope 2, Scope 3, Scope 4, Scope 5, Scope 6

### Gherkin Scenarios

```gherkin
Scenario: Domain extraction dispatches after universal processing for matching artifact (BS-001)
  Given an artifact with content_type "recipe" has completed universal processing
  And the domain registry has a recipe-extraction-v1 contract loaded
  When handleMessage processes the artifacts.processed message
  Then publishDomainExtractionRequest is called
  And a domain.extract NATS message is published with contract_version "recipe-extraction-v1"
  And domain_extraction_status is set to "pending"

Scenario: Domain extraction is skipped for non-matching artifact (BS-004)
  Given an artifact with content_type "article" has completed universal processing
  And no domain extraction contract matches "article" or the artifact's source URL
  When handleMessage processes the artifacts.processed message
  Then publishDomainExtractionRequest returns nil (no match)
  And no domain.extract NATS message is published
  And domain_extraction_status remains NULL
  And no error is logged

Scenario: Domain extraction is fail-open (BS-005)
  Given an artifact with content_type "recipe" has completed universal processing
  And the NATS publish to domain.extract fails
  When handleMessage processes the artifacts.processed message
  Then the error is logged as a warning
  And the artifacts.processed message is still acked
  And universal processing data is preserved

Scenario: Domain extraction fires in parallel with knowledge synthesis
  Given an artifact has completed universal processing
  And both knowledge synthesis and domain extraction are enabled
  When handleMessage processes the artifacts.processed message
  Then publishSynthesisRequest and publishDomainExtractionRequest are both called
  And neither blocks the other
  And the artifacts.processed message is acked after both

Scenario: Content below minimum length is skipped
  Given an artifact with content_type "recipe" and content_raw of 50 characters
  And the recipe contract has min_content_length 200
  When handleMessage processes the artifacts.processed message
  Then domain_extraction_status is set to "skipped"
  And no domain.extract NATS message is published

Scenario: Domain extraction lifecycle completes end-to-end
  Given a recipe URL is captured and universal processing succeeds
  When domain extraction request is published and ML sidecar processes it
  And the ML sidecar returns success with validated domain_data
  Then the artifact's domain_data JSONB column is populated
  And domain_extraction_status is "completed"
  And domain_schema_version is "recipe-extraction-v1"
  And domain_extracted_at is set
```

### Implementation Plan

**Files to modify:**
- `internal/pipeline/subscriber.go` — wire `publishDomainExtractionRequest` into `handleMessage`, add `DomainRegistry` field, add DOMAIN stream consumer for `domain.extracted`
- `cmd/core/main.go` (or `services.go`) — load domain registry at startup, pass to ResultSubscriber

**Design reference:** `design.md` § "Pipeline Integration Point", "Subscriber Extension", "Domain Extraction Result Handler"

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T7-01 | unit | `internal/pipeline/subscriber_test.go` | SCN-026-07 | handleMessage calls publishDomainExtractionRequest when DomainRegistry is set |
| T7-02 | unit | `internal/pipeline/subscriber_test.go` | SCN-026-07 | handleMessage skips domain extraction when DomainRegistry is nil |
| T7-03 | unit | `internal/pipeline/subscriber_test.go` | SCN-026-07 | handleMessage acks even when publishDomainExtractionRequest fails (fail-open) |
| T7-04 | unit | `internal/pipeline/subscriber_test.go` | SCN-026-07 | Domain extraction and synthesis both called for eligible artifact |
| T7-05 | integration | `tests/integration/domain_extraction_test.go` | SCN-026-07 | Recipe artifact → domain.extract → ML sidecar → domain.extracted → domain_data in DB |
| T7-06 | integration | `tests/integration/domain_extraction_test.go` | SCN-026-07 | Article artifact → no domain extraction, domain_extraction_status is NULL |
| T7-07 | integration | `tests/integration/domain_extraction_test.go` | SCN-026-07 | Short-content recipe artifact → status=skipped, no domain.extract published |

### Definition of Done

- [ ] `ResultSubscriber` has `DomainRegistry *domain.Registry` field wired at startup
  > **Phase:** implement

- [ ] `handleMessage` calls `publishDomainExtractionRequest` after universal processing, parallel to synthesis
  > **Phase:** implement

- [ ] Domain extraction is fail-open: publish errors are logged as warnings, message is still acked
  > **Phase:** implement

- [ ] Content below `min_content_length` is skipped with status="skipped"
  > **Phase:** implement

- [ ] DOMAIN stream consumer for `domain.extracted` created in `ResultSubscriber.Start()`
  > **Phase:** implement

- [ ] Domain registry loaded at service startup from `config/prompt_contracts/` directory
  > **Phase:** implement

- [ ] Integration test: recipe artifact → domain_data populated in DB with status=completed
  > **Phase:** test

- [ ] Integration test: article artifact → no domain extraction attempted
  > **Phase:** test

- [ ] All unit tests pass: `./smackerel.sh test unit`
  > **Phase:** test

- [ ] Integration tests pass: `./smackerel.sh test integration`
  > **Phase:** test

---

## Scope 8: Search Extension

**Status:** Not Started
**Priority:** P1
**Depends On:** Scope 7

### Gherkin Scenarios

```gherkin
Scenario: Search detects recipe ingredient intent (BS-006)
  Given the user queries "recipes with chicken"
  When parseDomainIntent is called
  Then it returns DomainIntent with Domain="recipe" and Attributes=["chicken"]

Scenario: Search detects multi-ingredient intent
  Given the user queries "recipes with lemon and garlic"
  When parseDomainIntent is called
  Then it returns DomainIntent with Domain="recipe" and Attributes=["lemon", "garlic"]

Scenario: Search detects product price intent (BS-002 partial)
  Given the user queries "cameras under $500"
  When parseDomainIntent is called
  Then it returns DomainIntent with Domain="product", Attributes=["cameras"], PriceMax=500

Scenario: Search returns nil for non-domain queries
  Given the user queries "interesting articles about AI"
  When parseDomainIntent is called
  Then it returns nil

Scenario: JSONB filters augment search for recipe ingredients (BS-006)
  Given 50 recipe artifacts with structured ingredient data exist
  And the user searches "something with lemon and garlic for dinner"
  When the search executes with domain intent detected
  Then results include artifacts where domain_data ingredients contain "lemon" AND "garlic"
  And domain-matched results are boosted above pure semantic matches

Scenario: JSONB filters augment search for product price
  Given product artifacts with price data exist
  And the user searches "headphones under $200"
  When the search executes with domain intent detected
  Then results include artifacts where domain_data price amount <= 200
  And domain-matched results are boosted above pure semantic matches

Scenario: Search falls back to semantic when no domain results
  Given no recipe artifacts exist
  When the user searches "recipes with chicken"
  And the domain JSONB query returns zero results
  Then the search falls back to pure semantic search
  And results are returned from standard vector similarity

Scenario: SearchResult includes domain_data when present
  Given an artifact has domain_data populated
  When the artifact appears in search results
  Then the SearchResult includes the domain_data field
```

### Implementation Plan

**Files to modify:**
- `internal/api/search.go` — add `parseDomainIntent`, `addDomainFilters`, integrate into search pipeline, add `DomainData` to `SearchResult`, add `Domain`/`Ingredient` to `SearchFilters`

**Files to create:**
- `internal/api/domain_search.go` — (optional, if search.go is too large) domain intent parsing and filter functions

**Design reference:** `design.md` § "Search Extension"

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T8-01 | unit | `internal/api/search_test.go` | SCN-026-08 | parseDomainIntent detects "recipes with chicken" → recipe/[chicken] |
| T8-02 | unit | `internal/api/search_test.go` | SCN-026-08 | parseDomainIntent detects "recipes with lemon and garlic" → recipe/[lemon, garlic] |
| T8-03 | unit | `internal/api/search_test.go` | SCN-026-08 | parseDomainIntent detects "cameras under $500" → product/[cameras]/PriceMax=500 |
| T8-04 | unit | `internal/api/search_test.go` | SCN-026-08 | parseDomainIntent returns nil for "interesting articles about AI" |
| T8-05 | unit | `internal/api/search_test.go` | SCN-026-08 | parseDomainIntent detects "dishes with mushrooms" → recipe/[mushrooms] |
| T8-06 | unit | `internal/api/search_test.go` | SCN-026-08 | addDomainFilters generates correct JSONB SQL for recipe ingredients |
| T8-07 | unit | `internal/api/search_test.go` | SCN-026-08 | addDomainFilters generates correct JSONB SQL for product price ceiling |
| T8-08 | e2e | `tests/e2e/domain_search_test.go` | SCN-026-08 | Search "recipes with chicken" returns ingredient-matched recipes ranked first |
| T8-09 | e2e | `tests/e2e/domain_search_test.go` | SCN-026-08 | Search with no domain results falls back to semantic search |

### Definition of Done

- [ ] `parseDomainIntent` detects recipe ingredient patterns ("recipes with X", "dishes with X and Y")
  > **Phase:** implement

- [ ] `parseDomainIntent` detects product price patterns ("X under $N", "X below N")
  > **Phase:** implement

- [ ] `parseDomainIntent` returns nil for non-domain queries (no false positives)
  > **Phase:** implement

- [ ] `addDomainFilters` generates parameterized JSONB SQL for recipe ingredient containment
  > **Phase:** implement

- [ ] `addDomainFilters` generates parameterized JSONB SQL for product price ceiling
  > **Phase:** implement

- [ ] Domain-matched search results receive +0.15 score boost (capped at 1.0)
  > **Phase:** implement

- [ ] Search falls back to pure semantic search when domain JSONB query returns zero results
  > **Phase:** implement

- [ ] `SearchResult.DomainData` populated when artifact has domain_data
  > **Phase:** implement

- [ ] `SearchFilters` extended with `Domain` and `Ingredient` optional fields
  > **Phase:** implement

- [ ] All unit tests pass: `./smackerel.sh test unit`
  > **Phase:** test

- [ ] E2E tests pass: `./smackerel.sh test e2e`
  > **Phase:** test

---

## Scope 9: Telegram Display

**Status:** Not Started
**Priority:** P2
**Depends On:** Scope 7

### Gherkin Scenarios

```gherkin
Scenario: Recipe artifact renders recipe card in Telegram (BS-001 display)
  Given an artifact has domain_data with domain="recipe"
  And the domain_data contains ingredients, timing, cuisine, and dietary_tags
  When the artifact is formatted for Telegram display
  Then the output includes a "Recipe Details" heading
  And the output includes timing and servings info
  And the output includes cuisine and difficulty
  And the output includes dietary tags
  And the output includes up to 10 ingredients with quantities
  And ingredients beyond 10 show "... and N more"

Scenario: Product artifact renders product card in Telegram (BS-002 display)
  Given an artifact has domain_data with domain="product"
  And the domain_data contains brand, price, rating, pros, and cons
  When the artifact is formatted for Telegram display
  Then the output includes a "Product Details" heading
  And the output includes brand name
  And the output includes price with currency
  And the output includes rating score
  And the output includes up to 5 pros and 3 cons

Scenario: Artifact without domain_data renders normally
  Given an artifact has no domain_data (NULL or empty)
  When the artifact is formatted for Telegram display
  Then formatDomainCard returns empty string
  And the standard formatting is used without domain section

Scenario: Unknown domain type in domain_data is handled gracefully
  Given an artifact has domain_data with domain="travel" (no renderer registered)
  When the artifact is formatted for Telegram display
  Then formatDomainCard returns empty string
  And no error is raised
```

### Implementation Plan

**Files to modify:**
- `internal/telegram/format.go` — add `formatDomainCard`, `formatRecipeCard`, `formatProductCard` functions; integrate `formatDomainCard` into artifact display formatting

**Design reference:** `design.md` § "Telegram Display"

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T9-01 | unit | `internal/telegram/format_test.go` | SCN-026-09 | formatRecipeCard renders timing, servings, cuisine, dietary_tags |
| T9-02 | unit | `internal/telegram/format_test.go` | SCN-026-09 | formatRecipeCard renders up to 10 ingredients with quantities, truncates remainder |
| T9-03 | unit | `internal/telegram/format_test.go` | SCN-026-09 | formatProductCard renders brand, price, rating, pros, cons |
| T9-04 | unit | `internal/telegram/format_test.go` | SCN-026-09 | formatProductCard limits pros to 5 and cons to 3 |
| T9-05 | unit | `internal/telegram/format_test.go` | SCN-026-09 | formatDomainCard returns empty string for nil/empty domain_data |
| T9-06 | unit | `internal/telegram/format_test.go` | SCN-026-09 | formatDomainCard returns empty string for unknown domain type |
| T9-07 | unit | `internal/telegram/format_test.go` | SCN-026-09 | formatDomainCard dispatches to recipe renderer for domain="recipe" |
| T9-08 | unit | `internal/telegram/format_test.go` | SCN-026-09 | formatDomainCard dispatches to product renderer for domain="product" |

### Definition of Done

- [ ] `formatDomainCard` dispatches to domain-specific renderer based on `domain_data->>'domain'`
  > **Phase:** implement

- [ ] `formatRecipeCard` renders timing, servings, cuisine, difficulty, dietary_tags, and up to 10 ingredients
  > **Phase:** implement

- [ ] `formatRecipeCard` truncates ingredient list beyond 10 with "... and N more" message
  > **Phase:** implement

- [ ] `formatProductCard` renders brand, price (with currency), rating (score/max), up to 5 pros and 3 cons
  > **Phase:** implement

- [ ] `formatDomainCard` returns empty string for nil/empty domain_data and unknown domain types (no errors)
  > **Phase:** implement

- [ ] `formatDomainCard` integrated into artifact display formatting in Telegram format layer
  > **Phase:** implement

- [ ] All unit tests pass: `./smackerel.sh test unit`
  > **Phase:** test
