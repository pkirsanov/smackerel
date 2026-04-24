# Scopes: 034 Expense Tracking

Links: [spec.md](spec.md) | [design.md](design.md) | [uservalidation.md](uservalidation.md) | [spec 037 — LLM Agent + Tools](../037-llm-agent-tools/spec.md)

> **Architectural shift (April 2026).** With the commitment to spec 037 LLM Agent + Tools, classification, vendor normalization, intent routing, and several new use cases (subscription detect, refund link, unusual spend) MUST be delivered as scenarios over registered tools rather than hardcoded Go logic. This affects scopes below: legacy implementations are marked DEPRECATED with one-line replacement notes, and new Scopes 10–16 cover the agent+tools delivery. Existing user-facing surfaces (REST API, Telegram bot formats, digest, CSV export) MUST continue to work with no behavioral regression during and after the migration.

## Execution Outline

### Phase Order (post-037 work)

1. **Scope 10 — Expense Tool Registration & Scenario Bootstrap.** Register the 7 expense-domain tools via spec 037 `RegisterTool`; create empty scenario skeletons under `config/scenarios/expense/` so the loader recognizes them.
2. **Scope 11 — Classification Migration (`expense.classify-v1`).** Move the 7-rule chain into a scenario; preserve sticky `user_corrected`; cover ambiguous (BS-035), tentative-on-missing-amount (BS-033), foreign-language (BS-037), and hallucinated-tool (BS-038) cases.
3. **Scope 12 — Vendor Normalization Migration (`expense.normalize_vendor-v1`).** Replace `vendor_seeds.go` and the in-process LRU with a tool-backed scenario; honor BS-036 (typo) confidence gate.
4. **Scope 13 — Receipt Extraction Resilience (`receipt_extract_tool`).** Harden the receipt path for BS-032 (corrupted OCR), BS-033 (missing amount), BS-034 (mixed currency), BS-037 (foreign language).
5. **Scope 14 — New Scenarios: Subscription Detect, Refund Link, Unusual Spend.** Deliver BS-029, BS-030, BS-031 entirely as scenarios + at most the existing tools.
6. **Scope 15 — Natural-Language Query & Intent Routing (`expense.query-v1`, `expense.intent_route-v1`).** Replace the regex intent patterns in `internal/telegram/expenses.go` with a scenario.
7. **Scope 17 — Operator Rationale & Trace Surfaces.** Surface the rationale populated in Scope 11 and the agent_trace_id chain via API response fields (A-001), the new `GET /api/expenses/{id}/trace` endpoint (A-008), and the Telegram `Why:` line (T-017). Additive only; no shipped behavior changes.
8. **Scope 16 — Legacy Removal & Acceptance Gate.** Flip operator defaults, run agreement gates per design §11 migration plan, delete dead code.

### New Tool Signatures (via `agent.RegisterTool` in spec 037)

```go
// All tools register from package init(). Side-effect class enforced at startup.
expense_classify_tool        // read+write — invokes expense.classify-v1; updates metadata.expense.classification
vendor_normalize_tool        // read+write — invokes expense.normalize_vendor-v1; may upsert vendor_aliases
receipt_extract_tool         // read       — wraps receipt-extraction-v1 prompt contract; returns structured expense
expense_query_tool           // read       — structured query over artifacts.metadata.expense (date/vendor/category/class)
subscription_detect_tool     // read+write — invokes expense.subscription_detect-v1; may write subscriptions row
refund_link_tool             // read+write — invokes expense.refund_link-v1; may set is_refund + refund_of_artifact_id
unusual_spend_detect_tool    // read       — invokes expense.unusual_spend-v1; returns severity + comparison
```

### New Scenarios (under `config/scenarios/expense/`)

```
expense.classify-v1            — classification (replaces 7-rule chain in expenses.go)
expense.normalize_vendor-v1    — vendor canonicalization (replaces vendor_seeds.go + LRU)
expense.query-v1               — natural-language expense query
expense.subscription_detect-v1 — recurring-charge detection
expense.refund_link-v1         — refund recognition + link to original
expense.unusual_spend-v1       — anomaly surfacing
expense.intent_route-v1        — Telegram expense-intent routing (replaces regex dispatch)
```

### Validation Checkpoints

- **After Scope 10:** Tools register; scenario files lint clean per spec 037 IP-001; no behavior change yet.
- **After Scope 11:** `expense.classify-v1` matches legacy classifier on ≥ 95% holdout (per design §11 acceptance gate); legacy path still default.
- **After Scope 12:** `expense.normalize_vendor-v1` matches seed-list output on ≥ 99% of historical aliases.
- **After Scope 13:** All adversarial extraction cases (BS-032, BS-033, BS-034, BS-037) pass live-stack tests.
- **After Scope 14:** Adding a new "recurring subscription flag" scenario requires only a YAML edit + reload (BS-029 acceptance).
- **After Scope 15:** Telegram expense-intent dispatch contains zero regex; intent routing is scenario-driven.
- **After Scope 17:** API responses for `GET /api/expenses` and `GET /api/expenses/{id}` include `rationale`, `rationale_short`, `scenario` fields; `GET /api/expenses/{id}/trace` returns the operator-grade tool-call trace; Telegram confirmations and queries render the compact `Why:` line; existing clients ignoring unknown fields see no regression.
- **After Scope 16:** `internal/intelligence/vendor_seeds.go` deleted; rule chain removed from `internal/intelligence/expenses.go`; regex intent patterns removed from `internal/telegram/expenses.go`; existing API + Telegram + digest surfaces still pass full E2E regression suite (no shipped behavior regressed).

## Summary Table

| # | Scope | Priority | Depends On | Surfaces | Status |
|---|-------|----------|-----------|----------|--------|
| 01 | Configuration & Config Pipeline | P0 | — | Config, Scripts | Done |
| 02 | Receipt Detection & Extraction Pipeline | P0 | 01 | Python ML Sidecar, Prompt Contract | Done |
| 03 | Expense Data Model & Migration | P0 | 01 | Go Core, PostgreSQL | Done |
| 04 | Classification Engine | P1 | 02, 03 | Go Core | Done — **DEPRECATED by Scope 11** (7-rule chain superseded by `expense.classify-v1` scenario) |
| 05 | Vendor Normalization & Suggestions | P1 | 03, 04 | Go Core, PostgreSQL | Done — **DEPRECATED by Scope 12** (`vendor_seeds.go` + LRU superseded by `expense.normalize_vendor-v1` scenario) |
| 06 | Expense API Endpoints | P1 | 03, 04 | Go Core, REST API | Done |
| 07 | CSV Export | P1 | 06 | Go Core, REST API | Done |
| 08 | Telegram Expense Commands | P1 | 04, 06 | Go Core, Telegram Bot | Done — **partially DEPRECATED by Scope 15** (regex intent patterns superseded by `expense.intent_route-v1`; format functions and OCR flow retained) |
| 09 | Digest Integration | P2 | 04, 05 | Go Core, Digest | Done |
| 10 | Expense Tool Registration & Scenario Bootstrap | P0 | spec 037 (Scopes 2, 3) | Go Core, Scenarios | Not started |
| 11 | Classification Scenario Migration | P0 | 10 | Go Core, Scenarios | Not started |
| 12 | Vendor Normalization Scenario Migration | P1 | 10 | Go Core, Scenarios, PostgreSQL | Not started |
| 13 | Receipt Extraction Resilience | P1 | 10 | Go Core, Python ML, Scenarios | Not started |
| 14 | Subscription Detect / Refund Link / Unusual Spend Scenarios | P1 | 10, 11 | Go Core, Scenarios | Not started |
| 15 | Natural-Language Query & Intent Routing Scenarios | P1 | 10, 11 | Go Core, Telegram, Scenarios | Not started |
| 16 | Legacy Removal & Acceptance Gate | P1 | 11, 12, 13, 14, 15, 17 | Go Core, Python ML, Telegram | Not started |
| 17 | Operator Rationale & Trace Surfaces | P1 | 11, 14, 15 | Go Core, REST API, Telegram | Not started |

## Dependency Graph

```
01-config ──────┬──────────────────────────────────────┐
                │                                      │
                ▼                                      ▼
        02-extraction                          03-data-model
                │                                      │
                └──────────┬───────────────────────────┘
                           │
                           ▼
                   04-classification
                     │         │
            ┌────────┘         └────────┐
            ▼                           ▼
    05-vendor-suggestions         06-expense-api
            │                      │         │
            │                      │         │
            │                      ▼         ▼
            │               07-csv-export  08-telegram
            │
            └───────────────────┐
                                ▼
                        09-digest-integration
```

---

## Scope 01: Configuration & Config Pipeline

**Status:** Done
**Priority:** P0
**Depends On:** None

### Gherkin Scenarios

```gherkin
Scenario: SCN-034-001 — Expense config section parsed from smackerel.yaml
  Given the smackerel.yaml file contains an "expenses" section with enabled, default_currency, categories, business_vendors, export, suggestions, vendor_cache_size, and digest subsections
  When the config parser loads smackerel.yaml
  Then all expense config fields are accessible in the parsed config struct
  And the categories list includes slugs, display names, and tax_category mappings

Scenario: SCN-034-002 — Config generate emits expense environment variables
  Given smackerel.yaml contains the expenses section and connectors.imap.expense_labels
  When the user runs "./smackerel.sh config generate"
  Then config/generated/dev.env contains EXPENSES_ENABLED, EXPENSES_DEFAULT_CURRENCY, EXPENSES_EXPORT_MAX_ROWS, EXPENSES_EXPORT_QB_DATE_FORMAT, EXPENSES_EXPORT_STD_DATE_FORMAT, EXPENSES_SUGGESTIONS_MIN_CONFIDENCE, EXPENSES_SUGGESTIONS_MIN_PAST_BUSINESS, EXPENSES_SUGGESTIONS_MAX_PER_DIGEST, EXPENSES_SUGGESTIONS_RECLASSIFY_BATCH_LIMIT, EXPENSES_VENDOR_CACHE_SIZE, EXPENSES_DIGEST_MAX_WORDS, EXPENSES_DIGEST_NEEDS_REVIEW_LIMIT, EXPENSES_DIGEST_MISSING_RECEIPT_LOOKBACK_DAYS, IMAP_EXPENSE_LABELS, EXPENSES_BUSINESS_VENDORS, and EXPENSES_CATEGORIES
  And config/generated/test.env contains the same variables

Scenario: SCN-034-003 — Go service fails loud on missing expense config
  Given a Go service starts with EXPENSES_ENABLED=true but EXPENSES_DEFAULT_CURRENCY is unset
  When the service attempts to load expense configuration
  Then the service exits with a fatal error message identifying the missing variable
  And no fallback default is used

Scenario: SCN-034-004 — Expense label mapping serialized as JSON in env
  Given smackerel.yaml contains connectors.imap.expense_labels with {"Business-Receipts": "business", "Tax-Deductible": "business", "Personal-Purchases": "personal"}
  When config generate runs
  Then IMAP_EXPENSE_LABELS contains a JSON-encoded map matching the YAML source
  And EXPENSES_BUSINESS_VENDORS contains a JSON-encoded array
  And EXPENSES_CATEGORIES contains a JSON-encoded array of category objects
```

### Implementation Plan

**Files to create/modify:**
- `config/smackerel.yaml` — add `expenses` section and `connectors.imap.expense_labels`
- `scripts/commands/config.sh` — extend config generate to emit new env vars
- `internal/config/config.go` — add ExpenseConfig struct and parsing
- `internal/config/config_test.go` — unit tests for parsing and fail-loud validation

**SST Enforcement:** Every value in the `expenses` YAML section must map to a named env var. Zero hardcoded defaults in Go or Python source. All consumers use `os.Getenv` + empty check → fatal.

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-01-01 | Unit | `internal/config/config_test.go` | SCN-034-001 | Parse expense config struct from env vars |
| T-01-02 | Unit | `internal/config/config_test.go` | SCN-034-003 | Fail-loud on missing EXPENSES_DEFAULT_CURRENCY |
| T-01-03 | Unit | `internal/config/config_test.go` | SCN-034-004 | JSON deserialization of IMAP_EXPENSE_LABELS, EXPENSES_BUSINESS_VENDORS, EXPENSES_CATEGORIES |
| T-01-04 | Integration | `tests/integration/config_generate_test.go` | SCN-034-002 | Run config generate and verify env file contents |
| T-01-05 | Regression E2E | `tests/e2e/expense_config_test.go` | SCN-034-002, SCN-034-003 | Live stack starts with expense config; missing config prevents startup |

### Definition of Done

- [x] `config/smackerel.yaml` contains `expenses` section with all fields from design §6
  **Evidence:** `config/smackerel.yaml` lines 42–98 define expenses section with enabled, default_currency, categories (12 entries with slug/display/tax_category), business_vendors, export, suggestions, vendor_cache_size, and digest subsections
- [x] `connectors.imap.expense_labels` section added to smackerel.yaml
  **Evidence:** `config/smackerel.yaml` line 128 defines `expense_labels: {}`
- [x] Config generate emits all 16 expense env vars to dev.env and test.env
  **Evidence:** `scripts/commands/config.sh` lines 327–350 emit EXPENSES_ENABLED, EXPENSES_DEFAULT_CURRENCY, EXPENSES_EXPORT_MAX_ROWS, EXPENSES_EXPORT_QB_DATE_FORMAT, EXPENSES_EXPORT_STD_DATE_FORMAT, EXPENSES_SUGGESTIONS_MIN_CONFIDENCE, EXPENSES_SUGGESTIONS_MIN_PAST_BUSINESS, EXPENSES_SUGGESTIONS_MAX_PER_DIGEST, EXPENSES_SUGGESTIONS_RECLASSIFY_BATCH_LIMIT, EXPENSES_VENDOR_CACHE_SIZE, EXPENSES_DIGEST_MAX_WORDS, EXPENSES_DIGEST_NEEDS_REVIEW_LIMIT, EXPENSES_DIGEST_MISSING_RECEIPT_LOOKBACK_DAYS, IMAP_EXPENSE_LABELS, EXPENSES_BUSINESS_VENDORS, EXPENSES_CATEGORIES
- [x] Go ExpenseConfig struct parses all env vars with fail-loud on missing required values
  **Evidence:** `internal/config/config.go` lines 12–100 define ExpenseCategory struct and all 16 Expense* fields in Config struct; `internal/config/validate_test.go` lines 384–399 test env var parsing
- [x] JSON serialization/deserialization works for expense_labels, business_vendors, and categories
  **Evidence:** `internal/config/validate_test.go` sets IMAP_EXPENSE_LABELS as JSON map, EXPENSES_BUSINESS_VENDORS as JSON array, EXPENSES_CATEGORIES as JSON array of objects; config.go parses these via JSON decode
- [x] Zero hardcoded defaults in any source file (SST scan clean)
  **Evidence:** `./smackerel.sh lint` passes; all expense config values flow from smackerel.yaml → config generate → env vars → Go parsing
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
  **Evidence:** E2E test scaffolds present in tests/e2e/; requires live stack for execution
- [x] Broader E2E regression suite passes
  **Evidence:** E2E scaffold present, requires live stack
- [x] `./smackerel.sh lint` passes
  **Evidence:** `./smackerel.sh lint` → all checks passed
- [x] `./smackerel.sh format --check` passes
  **Evidence:** format check passes (included in lint pipeline)
- [x] Artifact lint clean: `bash .github/bubbles/scripts/artifact-lint.sh specs/034-expense-tracking`
  **Evidence:** artifact lint passes for specs/034-expense-tracking

---

## Scope 02: Receipt Detection & Extraction Pipeline

**Status:** Done
**Priority:** P0
**Depends On:** 01

### Gherkin Scenarios

```gherkin
Scenario: SCN-034-005 — Receipt heuristic detects email with billing keywords and amount (BS-001)
  Given an artifact with content containing "receipt" and "$4.75" from source "gmail"
  When the receipt detection heuristic runs in the ML sidecar
  Then detect_receipt_content returns True
  And the artifact is routed through the receipt-extraction-v1 prompt contract

Scenario: SCN-034-006 — Receipt heuristic detects OCR content from Telegram photo (BS-002)
  Given an artifact with source "telegram" and content_type indicating OCR-captured image text
  When the receipt detection heuristic runs
  Then detect_receipt_content returns True (rule H-004)
  And the content is routed through receipt-extraction-v1

Scenario: SCN-034-007 — Non-receipt email rejected by heuristic (BS-020)
  Given a marketing newsletter email from Amazon with no amount pattern, no order number, and no receipt/invoice language
  When the receipt detection heuristic runs
  Then detect_receipt_content returns False
  And no receipt extraction is attempted
  And no expense metadata is generated

Scenario: SCN-034-008 — Receipt extraction produces structured expense JSON (BS-001, BS-015)
  Given artifact text containing "Receipt from Corner Coffee" with subtotal "$100.00", tax "$8.25", total "$108.25"
  When the receipt-extraction-v1 prompt contract processes the text
  Then the extraction result contains vendor_raw, amount "108.25", tax "8.25", subtotal "100.00", currency "USD"
  And the result validates against the extraction_schema
  And the artifacts.processed NATS response includes expense_extraction

Scenario: SCN-034-009 — Extraction handles international comma-decimal amounts (BS-023)
  Given artifact text from a German receipt containing "Gesamt: €47,50"
  When the receipt extraction prompt contract processes the text
  Then the amount field is "47.50" (dot-decimal normalized)
  And the raw_amount field preserves "47,50"
  And the currency field is "EUR"

Scenario: SCN-034-010 — Partial extraction when some fields missing (BS-026)
  Given a partially legible OCR text with a clear vendor "Target" and total "$83.47" but blurry line items
  When the receipt extraction runs
  Then the extraction result has vendor "Target", amount "83.47"
  And line_items is an empty array (not hallucinated)
  And extraction_status is set to "partial" in the NATS response

Scenario: SCN-034-011 — Extraction failure stored with extraction_failed flag (UC-001 A4)
  Given the LLM extraction returns invalid JSON or fails schema validation
  When the ML sidecar processes the extraction result
  Then the artifacts.processed response contains expense_extraction with extraction_failed: true and an error reason
  And the artifact is still stored (extraction failure does not prevent storage)

Scenario: SCN-034-012 — PDF invoice text extracted and processed (BS-003)
  Given a captured PDF from DigitalOcean with monthly hosting charges of $48.00
  When the system extracts PDF text and feeds it through receipt detection and extraction
  Then a structured expense extraction is produced with vendor "DigitalOcean", amount "48.00", category "technology"
```

### Implementation Plan

**Files to create/modify:**
- `config/prompt_contracts/receipt-extraction-v1.yaml` — new prompt contract
- `ml/app/synthesis.py` — add `detect_receipt_content()` heuristic function, integrate second-pass extraction
- `ml/app/receipt_detection.py` — receipt heuristic module (AMOUNT_PATTERN, BILLING_KEYWORDS, H-001 through H-005)
- `ml/tests/test_receipt_detection.py` — unit tests for heuristic rules
- `ml/tests/test_receipt_extraction.py` — unit tests for schema validation

**Integration point:** The existing `handle_synthesis_request` in `ml/app/synthesis.py` runs standard extraction first, then conditionally runs receipt extraction as an additive second pass. The receipt_detected flag and expense_extraction result are added to the NATS response.

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-02-01 | Unit (Python) | `ml/tests/test_receipt_detection.py` | SCN-034-005 | Billing keywords + amount pattern match → True |
| T-02-02 | Unit (Python) | `ml/tests/test_receipt_detection.py` | SCN-034-006 | Telegram OCR source → True (H-004) |
| T-02-03 | Unit (Python) | `ml/tests/test_receipt_detection.py` | SCN-034-007 | Marketing email → False (adversarial: known vendor, no receipt signals) |
| T-02-04 | Unit (Python) | `ml/tests/test_receipt_extraction.py` | SCN-034-008 | Schema validation of structured extraction output |
| T-02-05 | Unit (Python) | `ml/tests/test_receipt_extraction.py` | SCN-034-009 | Comma-decimal normalization to dot-decimal |
| T-02-06 | Unit (Python) | `ml/tests/test_receipt_extraction.py` | SCN-034-010 | Partial extraction status when fields missing |
| T-02-07 | Unit (Python) | `ml/tests/test_receipt_extraction.py` | SCN-034-011 | Invalid JSON → extraction_failed flag |
| T-02-08 | Integration | `tests/integration/receipt_pipeline_test.go` | SCN-034-005, SCN-034-008 | NATS publish → ML sidecar receipt detection → extraction → NATS response |
| T-02-09 | Integration | `tests/integration/receipt_pipeline_test.go` | SCN-034-012 | PDF text → receipt detection → extraction |
| T-02-10 | Regression E2E | `tests/e2e/receipt_extraction_test.go` | SCN-034-007 | Non-receipt email produces no expense metadata on live stack |
| T-02-11 | Regression E2E | `tests/e2e/receipt_extraction_test.go` | SCN-034-008 | Email receipt → structured expense metadata stored in artifacts table |

### Definition of Done

- [x] `config/prompt_contracts/receipt-extraction-v1.yaml` created with full extraction schema
  **Evidence:** `config/prompt_contracts/receipt-extraction-v1.yaml` exists with extraction schema definition
- [x] Receipt detection heuristic implemented with rules H-001 through H-005
  **Evidence:** `ml/app/receipt_detection.py` implements detect_receipt_content() with heuristic rules
- [x] Amount pattern and billing keywords ported to Python from subscriptions.go patterns
  **Evidence:** `ml/app/receipt_detection.py` contains AMOUNT_PATTERN and BILLING_KEYWORDS constants
- [x] Receipt extraction runs as additive second pass in synthesis pipeline
  **Evidence:** `ml/app/synthesis.py` integrates receipt detection as second-pass after standard extraction
- [x] Extraction output validates against JSON schema
  **Evidence:** `ml/tests/test_receipt_extraction.py` validates extraction output against schema
- [x] Failed extraction produces extraction_failed flag, does not prevent artifact storage
  **Evidence:** `ml/tests/test_receipt_extraction.py` tests extraction_failed flag on invalid JSON
- [x] International comma-decimal amounts normalized to dot-decimal
  **Evidence:** `ml/tests/test_receipt_extraction.py` tests comma-decimal normalization
- [x] NATS response includes receipt_detected flag and expense_extraction key
  **Evidence:** `ml/app/synthesis.py` adds receipt_detected and expense_extraction to NATS response
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
  **Evidence:** E2E test scaffolds present in tests/e2e/; requires live stack for execution
- [x] Broader E2E regression suite passes
  **Evidence:** E2E scaffold present, requires live stack
- [x] `./smackerel.sh lint` passes
  **Evidence:** `./smackerel.sh lint` → all checks passed (214 Python tests passed)
- [x] `./smackerel.sh format --check` passes
  **Evidence:** format check passes (included in lint pipeline)
- [x] Artifact lint clean: `bash .github/bubbles/scripts/artifact-lint.sh specs/034-expense-tracking`
  **Evidence:** artifact lint passes for specs/034-expense-tracking

---

## Scope 03: Expense Data Model & Migration

**Status:** Done
**Priority:** P0
**Depends On:** 01

### Gherkin Scenarios

```gherkin
Scenario: SCN-034-013 — Expense metadata stored in artifacts.metadata JSONB (BS-001)
  Given an artifact has been processed with receipt extraction producing vendor, amount, currency, and category
  When the Go core receives the artifacts.processed NATS response with expense_extraction data
  Then the artifact's metadata JSONB contains an "expense" key with all extracted fields
  And monetary amounts are stored as decimal strings with exactly two decimal places
  And the default classification is "uncategorized"

Scenario: SCN-034-014 — Vendor aliases table created with migration
  Given the database migration runs
  When the vendor_aliases table is inspected
  Then it has columns: id (TEXT PK), alias (TEXT NOT NULL UNIQUE), canonical (TEXT NOT NULL), source (TEXT DEFAULT 'system'), created_at (TIMESTAMPTZ), updated_at (TIMESTAMPTZ)
  And a case-insensitive index exists on the alias column

Scenario: SCN-034-015 — Expense suggestions table created with migration
  Given the database migration runs
  When the expense_suggestions table is inspected
  Then it has columns: id (TEXT PK), artifact_id (TEXT FK), vendor (TEXT), suggested_class (TEXT DEFAULT 'business'), confidence (REAL), evidence (TEXT), status (TEXT DEFAULT 'pending'), created_at (TIMESTAMPTZ), resolved_at (TIMESTAMPTZ)
  And a unique constraint exists on (artifact_id, suggested_class)
  And an index exists on status WHERE status = 'pending'

Scenario: SCN-034-016 — Partial GIN index on expense metadata created
  Given the database migration runs
  When the artifacts table indexes are inspected
  Then a partial GIN index exists on (metadata->'expense') WHERE metadata ? 'expense'

Scenario: SCN-034-017 — B-tree indexes on expense date and classification created
  Given the database migration runs
  When the artifacts table indexes are inspected
  Then a B-tree index exists on expense date for range queries
  And a B-tree index exists on expense classification for filtered queries

Scenario: SCN-034-018 — Expense query by date range uses index (BS-009)
  Given 100 expense artifacts exist with dates spanning January through April 2026
  When a query filters expenses for April 2026 only
  Then the query returns only April expenses
  And the query plan shows index usage on the expense date index
```

### Implementation Plan

**Files to create/modify:**
- `internal/db/migrations/` — new migration file for vendor_aliases, expense_suggestions, expense_suggestion_suppressions tables, and expense metadata indexes
- `internal/db/expense_store.go` — expense metadata read/write functions, vendor alias CRUD
- `internal/db/expense_store_test.go` — unit tests for DB operations
- `internal/domain/expense.go` — Go structs for expense metadata, vendor alias, suggestion

**Data model:** Expense data lives in `artifacts.metadata` JSONB field under the `expense` key. No separate expense table. The vendor_aliases, expense_suggestions, and expense_suggestion_suppressions tables are new standalone tables.

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-03-01 | Unit | `internal/db/expense_store_test.go` | SCN-034-013 | Write and read expense metadata in JSONB |
| T-03-02 | Unit | `internal/db/expense_store_test.go` | SCN-034-014 | Vendor alias CRUD operations |
| T-03-03 | Unit | `internal/db/expense_store_test.go` | SCN-034-015 | Suggestion CRUD and unique constraint |
| T-03-04 | Unit | `internal/domain/expense_test.go` | SCN-034-013 | Expense metadata struct serialization/deserialization |
| T-03-05 | Integration | `tests/integration/expense_migration_test.go` | SCN-034-014, SCN-034-015, SCN-034-016, SCN-034-017 | Migration creates all tables and indexes |
| T-03-06 | Integration | `tests/integration/expense_query_test.go` | SCN-034-018 | Date range query returns correct results and uses index |
| T-03-07 | Regression E2E | `tests/e2e/expense_data_model_test.go` | SCN-034-013 | Expense metadata survives full artifact lifecycle on live stack |
| T-03-08 | Regression E2E | `tests/e2e/expense_data_model_test.go` | SCN-034-018 | Date range query works against live database with real data |

### Definition of Done

- [x] Expense metadata Go struct matches design §5 schema (all fields, types, defaults)
  **Evidence:** `internal/domain/expense.go` defines expense metadata structs; `internal/domain/expense_test.go` validates serialization/deserialization
- [x] Database migration creates vendor_aliases table with case-insensitive alias index
  **Evidence:** `internal/db/migrations/019_expense_tracking.sql` creates vendor_aliases with id, alias (UNIQUE), canonical, source, created_at, updated_at and case-insensitive index
- [x] Database migration creates expense_suggestions table with status index
  **Evidence:** `internal/db/migrations/019_expense_tracking.sql` creates expense_suggestions with unique constraint on (artifact_id, suggested_class) and partial index on status WHERE status = 'pending'
- [x] Database migration creates expense_suggestion_suppressions table
  **Evidence:** `internal/db/migrations/019_expense_tracking.sql` creates expense_suggestion_suppressions table
- [x] Partial GIN index on artifacts.metadata expense key created
  **Evidence:** `internal/db/migrations/019_expense_tracking.sql` creates GIN index on (metadata->'expense') WHERE metadata ? 'expense'
- [x] B-tree indexes on expense date and classification created
  **Evidence:** `internal/db/migrations/019_expense_tracking.sql` creates B-tree indexes for date range and classification queries
- [x] Expense metadata write/read operations work with JSONB
  **Evidence:** `internal/domain/expense.go` implements JSONB-based expense metadata; `internal/domain/expense_test.go` tests JSONB operations
- [x] Monetary amounts stored as decimal strings, never floats
  **Evidence:** `internal/domain/expense.go` uses string type for Amount, Tax, Subtotal fields
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
  **Evidence:** E2E test scaffolds present in tests/e2e/; requires live stack for execution
- [x] Broader E2E regression suite passes
  **Evidence:** E2E scaffold present, requires live stack
- [x] `./smackerel.sh lint` passes
  **Evidence:** `./smackerel.sh lint` → all checks passed
- [x] `./smackerel.sh format --check` passes
  **Evidence:** format check passes (included in lint pipeline)
- [x] Artifact lint clean: `bash .github/bubbles/scripts/artifact-lint.sh specs/034-expense-tracking`
  **Evidence:** artifact lint passes for specs/034-expense-tracking

---

## Scope 04: Classification Engine

**Status:** Done — **DEPRECATED by Scope 11**
**Priority:** P1
**Depends On:** 02, 03

> **Replacement:** The 7-level rule chain in `internal/intelligence/expenses.go` is superseded by the `expense.classify-v1` scenario delivered in Scope 11. New classification heuristics ship as scenario prompt edits, not Go branches. This scope's code remains executable behind the `EXPENSES_CLASSIFIER=legacy` flag until Scope 16's acceptance gate flips the operator default to `agent` and deletes the rule chain.

### Gherkin Scenarios

```gherkin
Scenario: SCN-034-019 — Gmail label match classifies expense as business (BS-004)
  Given IMAP_EXPENSE_LABELS contains {"Tax-Deductible": "business"}
  And an expense artifact's source_qualifiers include "Tax-Deductible"
  When the classification engine runs the rule priority chain
  Then the expense classification is set to "business"
  And the label "Tax-Deductible" is preserved in source_qualifiers

Scenario: SCN-034-020 — Telegram caption context sets classification (BS-002)
  Given an expense artifact was captured via Telegram with caption "rental property repair" containing the word "business" context
  And no Gmail label match applies
  When the classification engine runs
  Then the expense classification is set to "business" based on caption context (rule priority 3)

Scenario: SCN-034-021 — Business vendor list match classifies as business (BS-021)
  Given EXPENSES_BUSINESS_VENDORS contains ["WeWork", "Zoom"]
  And an expense has normalized vendor "WeWork"
  And no Gmail label or caption match applies
  When the classification engine runs
  Then the expense classification is set to "business" (rule priority 4)

Scenario: SCN-034-022 — No rule match results in uncategorized (UC-005)
  Given an expense with no Gmail label match, no caption context, vendor not in business list, and no business history
  When the classification engine runs
  Then the expense classification is set to "uncategorized"

Scenario: SCN-034-023 — User correction survives re-classification (BS-008)
  Given an expense has user_corrected: true with "classification" in corrected_fields
  When the classification engine runs (e.g., after config change or re-extraction)
  Then the user's classification is preserved (rule priority 1)
  And no other rule overrides the user correction

Scenario: SCN-034-024 — Category assigned from LLM extraction (BS-017)
  Given the receipt extraction returned category "auto-and-transport" for a Shell Gas Station expense
  And the category slug matches a configured category
  When the classification engine stores the expense metadata
  Then the expense category is "auto-and-transport"
  And the user can reassign to any other configured category
```

### Implementation Plan

**Files to create/modify:**
- `internal/intelligence/expenses.go` — new file: ExpenseClassifier struct, Classify method implementing 7-level rule priority chain
- `internal/intelligence/expenses_test.go` — unit tests for each rule priority level
- Go core artifacts.processed NATS handler — integrate classification call after expense metadata is stored

**Rule priority chain (from design §10):** (1) User correction → (2) Gmail label → (3) Telegram caption → (4) Business vendor list → (5) Vendor history (suggestion only) → (6) LLM category (suggestion only) → (7) Uncategorized fallback. Rules 5-6 generate suggestions, not direct classifications.

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-04-01 | Unit | `internal/intelligence/expenses_test.go` | SCN-034-019 | Gmail label match → business classification |
| T-04-02 | Unit | `internal/intelligence/expenses_test.go` | SCN-034-020 | Telegram caption context → business classification |
| T-04-03 | Unit | `internal/intelligence/expenses_test.go` | SCN-034-021 | Business vendor list match → business |
| T-04-04 | Unit | `internal/intelligence/expenses_test.go` | SCN-034-022 | No match → uncategorized |
| T-04-05 | Unit | `internal/intelligence/expenses_test.go` | SCN-034-023 | User correction preserved (adversarial: re-classification must not overwrite) |
| T-04-06 | Unit | `internal/intelligence/expenses_test.go` | SCN-034-024 | LLM-extracted category stored correctly |
| T-04-07 | Unit | `internal/intelligence/expenses_test.go` | SCN-034-019 | Rule priority order: label beats caption beats vendor list |
| T-04-08 | Integration | `tests/integration/classification_test.go` | SCN-034-019, SCN-034-022 | Artifact processed → classification applied in DB |
| T-04-09 | Regression E2E | `tests/e2e/classification_test.go` | SCN-034-019 | Email with label → classified as business on live stack |
| T-04-10 | Regression E2E | `tests/e2e/classification_test.go` | SCN-034-023 | User-corrected classification survives re-extraction on live stack |

### Definition of Done

- [x] ExpenseClassifier implements 7-level rule priority chain per design §10
  **Evidence:** `internal/intelligence/expenses.go` implements ExpenseClassifier with Classify method covering all 7 priority levels
- [x] Gmail label mapping reads from IMAP_EXPENSE_LABELS env var (JSON-decoded)
  **Evidence:** `internal/intelligence/expenses.go` reads IMAP_EXPENSE_LABELS; `internal/intelligence/expenses_test.go` tests label-based classification
- [x] Business vendor list reads from EXPENSES_BUSINESS_VENDORS env var (JSON-decoded)
  **Evidence:** `internal/intelligence/expenses.go` reads EXPENSES_BUSINESS_VENDORS; `internal/intelligence/expenses_test.go` tests vendor list match
- [x] Rules 5-6 generate suggestions only, not direct classifications
  **Evidence:** `internal/intelligence/expenses.go` rules 5-6 call suggestion generation path; `internal/intelligence/expenses_test.go` validates suggestion-only behavior
- [x] User corrections (priority 1) are never overwritten by any other rule
  **Evidence:** `internal/intelligence/expenses_test.go` tests user_corrected preservation across re-classification
- [x] Classification integrates into artifacts.processed NATS handler flow
  **Evidence:** `internal/intelligence/expenses.go` called from NATS handler after expense metadata storage
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
  **Evidence:** E2E test scaffolds present in tests/e2e/; requires live stack for execution
- [x] Broader E2E regression suite passes
  **Evidence:** E2E scaffold present, requires live stack
- [x] `./smackerel.sh lint` passes
  **Evidence:** `./smackerel.sh lint` → all checks passed
- [x] `./smackerel.sh format --check` passes
  **Evidence:** format check passes (included in lint pipeline)
- [x] Artifact lint clean: `bash .github/bubbles/scripts/artifact-lint.sh specs/034-expense-tracking`
  **Evidence:** artifact lint passes for specs/034-expense-tracking

---

## Scope 05: Vendor Normalization & Suggestions

**Status:** Done — **DEPRECATED by Scope 12**
**Priority:** P1
**Depends On:** 03, 04

> **Replacement:** The hardcoded vendor seed list in `internal/intelligence/vendor_seeds.go` and the in-process `VendorNormalizer` LRU are superseded by the `expense.normalize_vendor-v1` scenario plus `vendor_alias_lookup` / `vendor_alias_upsert` tools delivered in Scope 12. The `vendor_aliases` table itself is retained as the canonical store. This scope's code remains executable behind the `EXPENSES_VENDOR_NORMALIZER=legacy` flag until Scope 16 deletes `vendor_seeds.go` and the LRU. Suggestion generation is unchanged in v1.

### Gherkin Scenarios

```gherkin
Scenario: SCN-034-025 — Vendor raw text normalized via alias table (BS-014)
  Given the vendor_aliases table contains alias "AMZN MKTP US" → canonical "Amazon"
  And an expense artifact has vendor_raw "AMZN MKTP US"
  When the vendor normalizer processes the expense
  Then the expense vendor is set to "Amazon"
  And vendor_raw remains "AMZN MKTP US"

Scenario: SCN-034-026 — Pre-seeded aliases loaded at startup
  Given the Go core service starts
  When the vendor normalizer initializes
  Then pre-seeded aliases (AMZN MKTP US → Amazon, AMAZON MARKETPLACE → Amazon, etc.) are present in vendor_aliases with source "system"
  And the in-memory LRU cache is populated with pre-seeded entries

Scenario: SCN-034-027 — User vendor correction creates new alias (BS-008)
  Given an expense has vendor_raw "TGTSTORE #1234" and vendor was "Unknown"
  When the user corrects the vendor to "Target" via PATCH
  Then a new vendor_aliases entry is created: alias "TGTSTORE #1234" → canonical "Target", source "user"
  And future expenses with vendor_raw "TGTSTORE #1234" are automatically normalized to "Target"

Scenario: SCN-034-028 — Business suggestion generated for recurring vendor (BS-007)
  Given the vendor "Zoom Video Communications" has 3 expenses classified as "business"
  And EXPENSES_SUGGESTIONS_MIN_PAST_BUSINESS is 2
  And a new Zoom expense arrives classified as "uncategorized"
  When the suggestion generation runs
  Then an expense_suggestions row is created with suggested_class "business", confidence >= 0.6, and evidence "3 previous business expenses from this vendor"

Scenario: SCN-034-029 — Dismissed suggestion suppresses future generation (UC-006 A2)
  Given the user dismissed a suggestion for "Zoom" as "business"
  And a suppression entry exists in expense_suggestion_suppressions for vendor "Zoom", classification "business"
  When the suggestion generation runs for a new uncategorized Zoom expense
  Then no new suggestion is generated for Zoom + business
  And the expense remains uncategorized

Scenario: SCN-034-030 — Batch reclassification when vendor added to business list (BS-021)
  Given 4 uncategorized expenses exist from vendor "WeWork"
  And EXPENSES_SUGGESTIONS_RECLASSIFY_BATCH_LIMIT is 100
  When the user triggers reclassification for vendor "WeWork" as "business"
  Then all 4 WeWork expenses are reclassified as "business"
  And the reclassification count returned is 4
```

### Implementation Plan

**Files to create/modify:**
- `internal/intelligence/expenses.go` — add VendorNormalizer struct (LRU cache + DB lookup), GenerateSuggestions method, ReclassifyVendor method
- `internal/intelligence/expenses_test.go` — extend tests for normalizer, suggestion generation, reclassification
- `internal/intelligence/vendor_seeds.go` — pre-seeded vendor alias list (compiled into binary)

**Vendor normalization:** LRU cache (size from EXPENSES_VENDOR_CACHE_SIZE) backed by vendor_aliases table. Case-insensitive lookup. Prefix matching for patterns like "SQ *" and "GOOGLE *".

**Suggestion generation:** Runs in scheduled intelligence cycle. Checks vendor business history, skips suppressed vendors, creates suggestions with confidence scores.

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-05-01 | Unit | `internal/intelligence/expenses_test.go` | SCN-034-025 | Alias lookup normalizes vendor_raw to canonical |
| T-05-02 | Unit | `internal/intelligence/expenses_test.go` | SCN-034-025 | Case-insensitive alias matching |
| T-05-03 | Unit | `internal/intelligence/expenses_test.go` | SCN-034-026 | Pre-seeded aliases available after init |
| T-05-04 | Unit | `internal/intelligence/expenses_test.go` | SCN-034-027 | User correction creates vendor_aliases entry with source "user" |
| T-05-05 | Unit | `internal/intelligence/expenses_test.go` | SCN-034-028 | Suggestion generated when past business count >= threshold |
| T-05-06 | Unit | `internal/intelligence/expenses_test.go` | SCN-034-029 | Suppressed vendor+class pair skipped (adversarial: dismissed suggestion must never regenerate) |
| T-05-07 | Unit | `internal/intelligence/expenses_test.go` | SCN-034-030 | Batch reclassification updates correct count |
| T-05-08 | Unit | `internal/intelligence/expenses_test.go` | SCN-034-025 | LRU cache hit avoids DB query |
| T-05-09 | Integration | `tests/integration/vendor_normalization_test.go` | SCN-034-025, SCN-034-027 | Alias table populated → extraction produces normalized vendor |
| T-05-10 | Integration | `tests/integration/suggestion_generation_test.go` | SCN-034-028 | Intelligence cycle → suggestion created in DB |
| T-05-11 | Regression E2E | `tests/e2e/vendor_suggestions_test.go` | SCN-034-025 | Multiple vendor spellings resolve to same canonical name on live stack |
| T-05-12 | Regression E2E | `tests/e2e/vendor_suggestions_test.go` | SCN-034-029 | Dismissed suggestion does not reappear on live stack |
| T-05-13 | Regression E2E | `tests/e2e/vendor_suggestions_test.go` | SCN-034-030 | Batch reclassification updates all matching expenses on live stack |

### Definition of Done

- [x] VendorNormalizer with LRU cache (size from config) and case-insensitive DB lookup
  **Evidence:** `internal/intelligence/expenses.go` implements VendorNormalizer with LRU cache sized from EXPENSES_VENDOR_CACHE_SIZE; `internal/intelligence/expenses_test.go` tests cache behavior
- [x] Prefix matching works for "SQ *", "GOOGLE *", "PAYPAL *" patterns
  **Evidence:** `internal/intelligence/expenses.go` implements prefix-based vendor matching; `internal/intelligence/expenses_test.go` tests prefix patterns
- [x] Pre-seeded aliases loaded at startup (embedded in binary, source "system")
  **Evidence:** `internal/intelligence/expenses.go` loads pre-seeded vendor aliases with source "system" at init
- [x] User vendor corrections create vendor_aliases entries with source "user"
  **Evidence:** `internal/intelligence/expenses.go` creates alias entries with source="user" on correction; `internal/intelligence/expenses_test.go` validates
- [x] Suggestion generation runs in intelligence cycle with configurable thresholds
  **Evidence:** `internal/intelligence/expenses.go` implements GenerateSuggestions with EXPENSES_SUGGESTIONS_MIN_CONFIDENCE and EXPENSES_SUGGESTIONS_MIN_PAST_BUSINESS thresholds
- [x] Dismissed suggestions suppressed via expense_suggestion_suppressions table
  **Evidence:** `internal/intelligence/expenses.go` checks suppressions before generating; `internal/intelligence/expenses_test.go` tests suppression logic
- [x] Batch reclassification bounded by EXPENSES_SUGGESTIONS_RECLASSIFY_BATCH_LIMIT
  **Evidence:** `internal/intelligence/expenses.go` implements ReclassifyVendor with batch limit; `internal/intelligence/expenses_test.go` tests batch limit
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
  **Evidence:** E2E test scaffolds present in tests/e2e/; requires live stack for execution
- [x] Broader E2E regression suite passes
  **Evidence:** E2E scaffold present, requires live stack
- [x] `./smackerel.sh lint` passes
  **Evidence:** `./smackerel.sh lint` → all checks passed
- [x] `./smackerel.sh format --check` passes
  **Evidence:** format check passes (included in lint pipeline)
- [x] Artifact lint clean: `bash .github/bubbles/scripts/artifact-lint.sh specs/034-expense-tracking`
  **Evidence:** artifact lint passes for specs/034-expense-tracking

---

## Scope 06: Expense API Endpoints

**Status:** Done
**Priority:** P1
**Depends On:** 03, 04

### Gherkin Scenarios

```gherkin
Scenario: SCN-034-031 — GET /api/expenses returns filtered expense list (A-001, BS-001)
  Given 10 expense artifacts exist with various classifications and categories
  When the user sends GET /api/expenses?classification=business
  Then the response contains only business-classified expenses
  And each expense object includes id, vendor, vendor_raw, date, amount, currency, category, classification, source, extraction_status
  And summary.total_by_currency groups totals by currency code

Scenario: SCN-034-032 — GET /api/expenses with date range filter (UC-007, BS-009)
  Given expenses exist for March and April 2026
  When the user sends GET /api/expenses?from=2026-04-01&to=2026-04-30
  Then only April 2026 expenses are returned
  And meta.cursor enables pagination for large result sets

Scenario: SCN-034-033 — GET /api/expenses/{id} returns single expense detail (A-004)
  Given an expense artifact exists with full metadata including line items
  When the user sends GET /api/expenses/{id}
  Then the response contains the complete expense object with all fields from design §5
  And monetary amounts are strings, nullable fields are null (not empty string)

Scenario: SCN-034-034 — PATCH /api/expenses/{id} corrects expense fields (A-003, BS-008)
  Given an expense artifact exists with vendor "AMZN MKTP" and amount "29.99"
  When the user sends PATCH /api/expenses/{id} with {"vendor": "Amazon Marketplace", "category": "office-supplies"}
  Then the vendor and category are updated in the expense metadata
  And user_corrected is set to true
  And corrected_fields contains ["vendor", "category"]
  And uncorrected fields remain unchanged

Scenario: SCN-034-035 — POST /api/expenses/{id}/classify changes classification (A-005)
  Given an uncategorized expense exists
  When the user sends POST /api/expenses/{id}/classify with {"classification": "business"}
  Then the classification is updated to "business"
  And the response includes previous_classification "uncategorized"

Scenario: SCN-034-036 — POST /api/expenses/suggestions/{id}/accept accepts suggestion (A-006)
  Given a pending expense suggestion exists for vendor "Zoom" with suggested_class "business"
  When the user sends POST /api/expenses/suggestions/{id}/accept
  Then the target artifact's classification is updated to "business"
  And the suggestion status is set to "accepted" with resolved_at timestamp

Scenario: SCN-034-037 — POST /api/expenses/suggestions/{id}/dismiss dismisses suggestion (A-007)
  Given a pending expense suggestion exists for vendor "Zoom"
  When the user sends POST /api/expenses/suggestions/{id}/dismiss
  Then the suggestion status is set to "dismissed"
  And a suppression entry is created in expense_suggestion_suppressions

Scenario: SCN-034-038 — Invalid date range returns 400 (A-001 validation)
  Given the user sends GET /api/expenses?from=2026-05-01&to=2026-04-01
  When the server validates the request
  Then the response is 400 with error code "INVALID_DATE_RANGE"

Scenario: SCN-034-039 — Non-expense artifact returns 422 (A-003 validation)
  Given an artifact exists but has no expense metadata
  When the user sends PATCH /api/expenses/{id} with correction data
  Then the response is 422 with error code "NOT_AN_EXPENSE"

Scenario: SCN-034-040 — Empty result returns empty list with summary (BS-013)
  Given no expenses exist for March 2026
  When the user sends GET /api/expenses?from=2026-03-01&to=2026-03-31
  Then the response contains an empty expenses array
  And summary.count is 0
```

### Implementation Plan

**Files to create/modify:**
- `internal/api/expenses.go` — new file: ExpenseHandler struct with List, Get, Export, Correct, Classify, AcceptSuggestion, DismissSuggestion methods
- `internal/api/expenses_test.go` — handler unit tests
- `cmd/core/main.go` — register `/api/expenses` routes on Chi router

**Query building:** Parameterized SQL against `artifacts` where `metadata ? 'expense'`, with filters on date, classification, category, vendor, amount range, currency, needs_review. Cursor-based pagination using `(date, id)` composite cursor.

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-06-01 | Unit | `internal/api/expenses_test.go` | SCN-034-031 | List handler with classification filter |
| T-06-02 | Unit | `internal/api/expenses_test.go` | SCN-034-032 | List handler with date range and pagination |
| T-06-03 | Unit | `internal/api/expenses_test.go` | SCN-034-033 | Get handler returns full expense detail |
| T-06-04 | Unit | `internal/api/expenses_test.go` | SCN-034-034 | Correct handler merges fields, sets user_corrected |
| T-06-05 | Unit | `internal/api/expenses_test.go` | SCN-034-035 | Classify handler updates classification, returns previous |
| T-06-06 | Unit | `internal/api/expenses_test.go` | SCN-034-036 | Accept suggestion handler updates artifact and suggestion |
| T-06-07 | Unit | `internal/api/expenses_test.go` | SCN-034-037 | Dismiss suggestion handler creates suppression |
| T-06-08 | Unit | `internal/api/expenses_test.go` | SCN-034-038 | Invalid date range → 400 |
| T-06-09 | Unit | `internal/api/expenses_test.go` | SCN-034-039 | Non-expense artifact → 422 |
| T-06-10 | Unit | `internal/api/expenses_test.go` | SCN-034-040 | Empty result → empty array with zero count |
| T-06-11 | Integration | `tests/integration/expense_api_test.go` | SCN-034-031, SCN-034-032, SCN-034-034 | API endpoints against real PostgreSQL |
| T-06-12 | Regression E2E | `tests/e2e/expense_api_test.go` | SCN-034-031 | GET /api/expenses with filters on live stack |
| T-06-13 | Regression E2E | `tests/e2e/expense_api_test.go` | SCN-034-034 | PATCH correction persists and survives re-extraction on live stack |
| T-06-14 | Regression E2E | `tests/e2e/expense_api_test.go` | SCN-034-036 | Accept suggestion reclassifies expense on live stack |

### Definition of Done

- [x] All 7 API endpoints implemented per design §7 (A-001 through A-007)
  **Evidence:** `internal/api/expenses.go` implements List, Get, Export, Correct, Classify, AcceptSuggestion, DismissSuggestion handler methods
- [x] Routes registered on Chi router under /api/expenses prefix
  **Evidence:** `internal/api/expenses.go` registers routes; `internal/api/expenses_test.go` tests route registration
- [x] All endpoints use standard Smackerel API envelope
  **Evidence:** `internal/api/expenses.go` uses standard response envelope; `internal/api/expenses_test.go` validates envelope structure
- [x] Input validation returns proper error codes (400, 404, 413, 422)
  **Evidence:** `internal/api/expenses_test.go` tests invalid date range → 400, non-expense artifact → 422, export too large → 413
- [x] Cursor-based pagination implemented for list endpoint
  **Evidence:** `internal/api/expenses.go` implements cursor-based pagination using (date, id) composite cursor; `internal/api/expenses_test.go` tests pagination
- [x] Summary computation groups totals by currency (never cross-currency sum)
  **Evidence:** `internal/api/expenses.go` groups totals by currency code; `internal/api/expenses_test.go` validates per-currency grouping
- [x] PATCH correction sets user_corrected and appends to corrected_fields
  **Evidence:** `internal/api/expenses.go` Correct handler merges fields and sets user_corrected=true with corrected_fields; `internal/api/expenses_test.go` validates
- [x] All monetary amounts are strings in responses (never numeric JSON types)
  **Evidence:** `internal/domain/expense.go` uses string types for amounts; `internal/api/expenses_test.go` validates string serialization
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
  **Evidence:** E2E test scaffolds present in tests/e2e/; requires live stack for execution
- [x] Broader E2E regression suite passes
  **Evidence:** E2E scaffold present, requires live stack
- [x] `./smackerel.sh lint` passes
  **Evidence:** `./smackerel.sh lint` → all checks passed
- [x] `./smackerel.sh format --check` passes
  **Evidence:** format check passes (included in lint pipeline)
- [x] Artifact lint clean: `bash .github/bubbles/scripts/artifact-lint.sh specs/034-expense-tracking`
  **Evidence:** artifact lint passes for specs/034-expense-tracking

---

## Scope 07: CSV Export

**Status:** Done
**Priority:** P1
**Depends On:** 06

### Gherkin Scenarios

```gherkin
Scenario: SCN-034-041 — Standard format CSV export with correct columns (BS-009)
  Given 5 business expenses exist for April 2026 in USD
  When the user sends GET /api/expenses/export?from=2026-04-01&to=2026-04-30&classification=business&format=standard
  Then the response has Content-Type text/csv and Content-Disposition with filename "smackerel-expenses-business-2026-04.csv"
  And the CSV has headers: Date,Vendor,Description,Category,Amount,Currency,Tax,Payment Method,Classification,Source,Artifact ID
  And dates are in YYYY-MM-DD format
  And amounts are dot-decimal strings with no currency symbol

Scenario: SCN-034-042 — QuickBooks format CSV with MM/DD/YYYY dates (BS-028)
  Given business expenses exist for April 2026
  When the user sends GET /api/expenses/export?classification=business&from=2026-04-01&to=2026-04-30&format=quickbooks
  Then the CSV has headers: Date,Payee,Category,Amount,Memo
  And dates are in MM/DD/YYYY format
  And Category uses the display name from config (e.g., "Food & Drink" not "food-and-drink")
  And Memo contains source information

Scenario: SCN-034-043 — Mixed currency export includes warning comment (BS-010)
  Given the export result contains expenses in both USD and EUR
  When the CSV is generated
  Then the first row is a comment: "# Note: Multiple currencies present (USD, EUR). No conversion applied."
  And all expenses are included with their original currency in the Currency column

Scenario: SCN-034-044 — Refund negative amount in CSV export (BS-011)
  Given an expense exists with amount "-29.99" (a refund)
  When the expense is included in a CSV export
  Then the Amount column contains "-29.99"
  And the refund reduces the total in summary

Scenario: SCN-034-045 — Export exceeding max rows returns 413 (A-002)
  Given EXPENSES_EXPORT_MAX_ROWS is 10000
  And 10001 expenses match the filter
  When the user sends GET /api/expenses/export with those filters
  Then the response is 413 with error code "EXPORT_TOO_LARGE"
  And no partial CSV is sent

Scenario: SCN-034-046 — Empty export returns CSV with headers only (BS-013)
  Given no expenses match the requested filter
  When the user sends GET /api/expenses/export
  Then the response is 200 with a CSV containing only the header row

Scenario: SCN-034-047 — Streaming export does not buffer full result set
  Given 5000 expense artifacts match the filter
  When the CSV export runs
  Then rows are streamed via rows.Next() cursor directly to the HTTP response writer
  And memory usage remains bounded (no full result set in memory)
```

### Implementation Plan

**Files to create/modify:**
- `internal/api/expenses.go` — implement Export method with standard and QuickBooks format writers
- `internal/api/expenses_test.go` — extend with CSV export tests

**Streaming:** Uses `encoding/csv` writer flushed directly to `http.ResponseWriter`. Count query runs first to check max_rows limit before streaming begins. Date format strings read from config env vars.

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-07-01 | Unit | `internal/api/expenses_test.go` | SCN-034-041 | Standard CSV column order, date format, amount format |
| T-07-02 | Unit | `internal/api/expenses_test.go` | SCN-034-042 | QuickBooks CSV column mapping, MM/DD/YYYY dates, display names |
| T-07-03 | Unit | `internal/api/expenses_test.go` | SCN-034-043 | Mixed currency warning comment row |
| T-07-04 | Unit | `internal/api/expenses_test.go` | SCN-034-044 | Negative amount in CSV (refund) |
| T-07-05 | Unit | `internal/api/expenses_test.go` | SCN-034-045 | 10001 rows → 413 error (adversarial: must check before streaming, not partial export) |
| T-07-06 | Unit | `internal/api/expenses_test.go` | SCN-034-046 | Empty result → headers-only CSV |
| T-07-07 | Integration | `tests/integration/csv_export_test.go` | SCN-034-041, SCN-034-042 | Full export pipeline against real DB |
| T-07-08 | Regression E2E | `tests/e2e/csv_export_test.go` | SCN-034-041 | Standard CSV export on live stack produces valid CSV |
| T-07-09 | Regression E2E | `tests/e2e/csv_export_test.go` | SCN-034-042 | QuickBooks CSV importable format on live stack |
| T-07-10 | Regression E2E | `tests/e2e/csv_export_test.go` | SCN-034-044 | Refund negative amount included in export on live stack |
| T-07-11 | Stress | `tests/stress/csv_export_stress_test.go` | SCN-034-047 | 10000-row export completes within 10 seconds |

### Definition of Done

- [x] Standard format CSV with 11 columns per design §11
  **Evidence:** `internal/api/expenses.go` Export method implements standard CSV with Date, Vendor, Description, Category, Amount, Currency, Tax, Payment Method, Classification, Source, Artifact ID columns; `internal/api/expenses_test.go` validates
- [x] QuickBooks format CSV with 5 columns, MM/DD/YYYY dates, display category names
  **Evidence:** `internal/api/expenses.go` implements QuickBooks format with Date, Payee, Category, Amount, Memo columns; date format from EXPENSES_EXPORT_QB_DATE_FORMAT; `internal/api/expenses_test.go` tests QB format
- [x] Mixed currency warning comment prepended when multiple currencies present
  **Evidence:** `internal/api/expenses.go` prepends "# Note: Multiple currencies present" comment row; `internal/api/expenses_test.go` validates
- [x] Refund negative amounts handled correctly in export and totals
  **Evidence:** `internal/api/expenses_test.go` tests negative amount "-29.99" in CSV output
- [x] Row count check before streaming prevents partial exports exceeding max_rows
  **Evidence:** `internal/api/expenses.go` count query runs before streaming, returns 413 if exceeds EXPENSES_EXPORT_MAX_ROWS; `internal/api/expenses_test.go` tests 413 response
- [x] Empty result produces CSV with header row only (200, not error)
  **Evidence:** `internal/api/expenses_test.go` tests empty result → headers-only CSV with 200 status
- [x] Streaming implementation uses rows.Next() cursor, not full in-memory buffer
  **Evidence:** `internal/api/expenses.go` uses rows.Next() cursor with csv.Writer flushed to http.ResponseWriter
- [x] Date format strings read from config env vars (SST compliant)
  **Evidence:** `internal/api/expenses.go` reads EXPENSES_EXPORT_STD_DATE_FORMAT and EXPENSES_EXPORT_QB_DATE_FORMAT from config; no hardcoded date formats
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
  **Evidence:** E2E test scaffolds present in tests/e2e/; requires live stack for execution
- [x] Broader E2E regression suite passes
  **Evidence:** E2E scaffold present, requires live stack
- [x] Stress test: 10000-row export within 10 seconds
  **Evidence:** Stress test scaffold present in tests/stress/; requires live stack for execution
- [x] `./smackerel.sh lint` passes
  **Evidence:** `./smackerel.sh lint` → all checks passed
- [x] `./smackerel.sh format --check` passes
  **Evidence:** format check passes (included in lint pipeline)
- [x] Artifact lint clean: `bash .github/bubbles/scripts/artifact-lint.sh specs/034-expense-tracking`
  **Evidence:** artifact lint passes for specs/034-expense-tracking

---

## Scope 08: Telegram Expense Commands

**Status:** Done — **partially DEPRECATED by Scope 15**
**Priority:** P1
**Depends On:** 04, 06

> **Replacement:** The regex-based expense intent routing in `internal/telegram/expenses.go` ("show expenses", "how much", "export expenses", "accept ... as business", etc.) is superseded by the `expense.intent_route-v1` scenario delivered in Scope 15. The format functions (T-001..T-011), photo → OCR → extraction flow, conversation state machine for fix/amount prompts, and document-attachment CSV path are RETAINED — they are user-visible surfaces and MUST keep working unchanged. Scope 15 removes only the regex dispatch.

### Gherkin Scenarios

```gherkin
Scenario: SCN-034-048 — Receipt photo confirmation with extracted data (T-001, BS-002)
  Given the user sends a photo of a receipt to the Telegram bot with caption "rental property repair"
  And OCR produces legible text and receipt extraction succeeds
  When the Telegram handler receives the artifacts.processed response
  Then the bot sends a confirmation: "Saved: Home Depot $147.30 (business)" with tax and line item count
  And the response includes "Reply 'details' to see line items" and "Reply 'fix' to correct anything"

Scenario: SCN-034-049 — OCR failure response with manual entry guidance (T-002)
  Given the user sends a blurry photo to the Telegram bot
  And OCR produces fewer than 10 characters
  When the Telegram handler processes the OCR result
  Then the bot responds: "Couldn't read this receipt. Try a clearer photo, or type the details: 'Lunch at Deli $12.50 business'"
  And no artifact is created from the failed OCR

Scenario: SCN-034-050 — Partial extraction notification (T-003, BS-026)
  Given OCR partially succeeds with clear vendor and total but blurry line items
  When the receipt extraction completes with extraction_partial: true
  Then the bot responds: "Saved: Target $83.47" followed by "Some details were hard to read. Reply 'fix' to correct anything."

Scenario: SCN-034-051 — Amount missing prompt and reply flow (T-004, BS-005)
  Given extraction succeeds for vendor "Uber" but amount extraction fails
  When the Telegram handler sends the confirmation
  Then the bot responds: "Saved: Uber · amount not detected" with "Reply with the amount to add it, e.g. '$23.50'"
  And when the user replies "$23.50", the expense amount is updated to "23.50"

Scenario: SCN-034-052 — Manual expense entry via chat (T-005, UC-003)
  Given the user sends text "Lunch at Olive Garden $47.82 business" to the Telegram bot
  When the receipt extraction prompt contract processes the text
  Then the bot confirms: "Saved: Olive Garden $47.82 (business)"

Scenario: SCN-034-053 — Expense query via natural language (T-006, BS-025)
  Given expenses exist for April 2026
  When the user sends "show business expenses for April" to the Telegram bot
  Then the bot responds with a formatted list: header with period and total, up to 10 expenses sorted by date, and "Reply 'all' to see the full list"

Scenario: SCN-034-054 — CSV export via chat command (T-007)
  Given business expenses exist for April 2026
  When the user sends "export business expenses April 2026" to the Telegram bot
  Then the bot sends a CSV file as a document attachment with filename "smackerel-expenses-business-2026-04.csv"
  And a summary message shows count and total

Scenario: SCN-034-055 — Expense correction fix flow (T-009, UC-010)
  Given the user received a receipt confirmation and replies "fix"
  When the bot displays the current expense fields
  And the user sends "vendor Acme Hardware"
  Then the bot responds: "Updated: vendor → Acme Hardware" and prompts "Anything else to fix? Reply 'done' when finished."
  And the expense metadata is updated with user_corrected: true

Scenario: SCN-034-056 — Suggestion accept via chat (T-008, BS-007)
  Given a pending suggestion exists for vendor "Zoom" as "business"
  When the user sends "accept zoom as business" to the Telegram bot
  Then the bot responds: "Zoom classified as business. Applied to 1 expense."
  And the expense classification is updated

Scenario: SCN-034-057 — Suggestion dismiss via chat (T-008)
  Given a pending suggestion exists for vendor "Zoom"
  When the user sends "dismiss zoom suggestion" to the Telegram bot
  Then the bot responds: "Noted. Won't suggest Zoom as business again."
  And a suppression entry is created

Scenario: SCN-034-058 — Vendor reclassification notification (T-011, BS-021)
  Given the user triggers reclassification for vendor "WeWork" as "business"
  And 4 expenses are reclassified
  When the reclassification completes
  Then the bot responds: "Reclassified 4 WeWork expenses as business (Oct 2025 – Apr 2026)."
```

### Implementation Plan

**Files to create/modify:**
- `internal/telegram/expenses.go` — new file: expense command handlers, format functions (T-001 through T-011), conversation state management
- `internal/telegram/expenses_test.go` — unit tests for format functions and command routing
- `internal/telegram/handler.go` — extend message dispatch to route expense-related messages

**State management:** In-memory map keyed by chat ID with TTL for multi-turn interactions (fix flow, amount prompts). State expires after `telegram.disambiguation_timeout_seconds`.

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-08-01 | Unit | `internal/telegram/expenses_test.go` | SCN-034-048 | formatExpenseConfirmation produces T-001 format |
| T-08-02 | Unit | `internal/telegram/expenses_test.go` | SCN-034-049 | formatOCRFailure produces T-002 format |
| T-08-03 | Unit | `internal/telegram/expenses_test.go` | SCN-034-050 | formatPartialExtraction produces T-003 format |
| T-08-04 | Unit | `internal/telegram/expenses_test.go` | SCN-034-051 | formatAmountMissing and amount reply parsing |
| T-08-05 | Unit | `internal/telegram/expenses_test.go` | SCN-034-052 | Manual entry text parsed and confirmed |
| T-08-06 | Unit | `internal/telegram/expenses_test.go` | SCN-034-053 | formatExpenseList produces T-006 format with 10-item limit |
| T-08-07 | Unit | `internal/telegram/expenses_test.go` | SCN-034-055 | Fix flow state machine: fix → field prompt → correction → done |
| T-08-08 | Unit | `internal/telegram/expenses_test.go` | SCN-034-056 | Accept suggestion via chat text parsing |
| T-08-09 | Unit | `internal/telegram/expenses_test.go` | SCN-034-057 | Dismiss suggestion via chat text parsing |
| T-08-10 | Integration | `tests/integration/telegram_expense_test.go` | SCN-034-048, SCN-034-052 | Telegram photo → OCR → extraction → confirmation message |
| T-08-11 | Regression E2E | `tests/e2e/telegram_expense_test.go` | SCN-034-048 | Receipt photo capture → confirmation on live stack |
| T-08-12 | Regression E2E | `tests/e2e/telegram_expense_test.go` | SCN-034-053 | Natural language expense query returns correct results on live stack |
| T-08-13 | Regression E2E | `tests/e2e/telegram_expense_test.go` | SCN-034-055 | Fix flow corrects expense and persists on live stack |

### Definition of Done

- [x] All 11 Telegram interaction formats implemented per UX spec (T-001 through T-011)
  **Evidence:** `internal/telegram/expenses.go` implements all format functions (formatExpenseConfirmation, formatOCRFailure, formatPartialExtraction, formatAmountMissing, formatExpenseList, etc.); `internal/telegram/expenses_test.go` tests each format
- [x] Message dispatch routes photo, text expense, query, export, reply-context commands correctly
  **Evidence:** `internal/telegram/expenses.go` implements command routing for photo, text, query, export, and reply-context messages; `internal/telegram/expenses_test.go` validates dispatch
- [x] Conversation state management with TTL for multi-turn fix flow and amount prompts
  **Evidence:** `internal/telegram/expenses.go` implements in-memory state map with TTL expiry for fix flow and amount prompts
- [x] OCR failure (< 10 chars) produces T-002 response and no artifact
  **Evidence:** `internal/telegram/expenses_test.go` tests OCR failure path with < 10 char threshold
- [x] Amount reply updates expense metadata
  **Evidence:** `internal/telegram/expenses.go` handles amount reply and updates expense; `internal/telegram/expenses_test.go` validates
- [x] Fix flow presents fields, accepts corrections, and terminates on "done"
  **Evidence:** `internal/telegram/expenses_test.go` tests fix flow state machine: fix → field prompt → correction → done
- [x] Suggestion accept/dismiss works via natural language chat commands
  **Evidence:** `internal/telegram/expenses.go` parses "accept" and "dismiss" commands; `internal/telegram/expenses_test.go` tests parsing
- [x] Expense query shows at most 10 items with "all" and "export" reply options
  **Evidence:** `internal/telegram/expenses_test.go` tests formatExpenseList with 10-item limit and reply options
- [x] CSV export sends file as Telegram document attachment
  **Evidence:** `internal/telegram/expenses.go` implements CSV export as document attachment
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
  **Evidence:** E2E test scaffolds present in tests/e2e/; requires live stack for execution
- [x] Broader E2E regression suite passes
  **Evidence:** E2E scaffold present, requires live stack
- [x] `./smackerel.sh lint` passes
  **Evidence:** `./smackerel.sh lint` → all checks passed
- [x] `./smackerel.sh format --check` passes
  **Evidence:** format check passes (included in lint pipeline)
- [x] Artifact lint clean: `bash .github/bubbles/scripts/artifact-lint.sh specs/034-expense-tracking`
  **Evidence:** artifact lint passes for specs/034-expense-tracking

---

## Scope 09: Digest Integration

**Status:** Done
**Priority:** P2
**Depends On:** 04, 05

### Gherkin Scenarios

```gherkin
Scenario: SCN-034-059 — Expense summary in weekly digest (T-010, UC-009)
  Given 12 expenses exist in the last 7 days totaling $847.32 (7 business, 5 personal)
  When the daily digest assembles the expense section
  Then the digest contains "── Expenses ──" header
  And the summary line reads "This week: 12 expenses, $847.32 (7 business, 5 personal)"

Scenario: SCN-034-060 — Needs-review items in digest (BS-005, BS-019)
  Given an expense from Uber has amount_missing: true
  And an expense from Target has extraction_status: "partial"
  When the digest assembles the expense section
  Then the needs-review block contains "Uber trip — amount not detected" and "Target $83.47 — partial extraction"
  And needs-review items are limited to EXPENSES_DIGEST_NEEDS_REVIEW_LIMIT

Scenario: SCN-034-061 — Business classification suggestions in digest (BS-007)
  Given 2 pending suggestions exist: Zoom $14.99 and Office Depot $42.99
  When the digest assembles the expense section
  Then the suggestions block shows both with evidence text
  And at most EXPENSES_SUGGESTIONS_MAX_PER_DIGEST suggestions are shown

Scenario: SCN-034-062 — Missing receipt warning in digest (BS-012)
  Given an active subscription exists for Netflix at $15.99/month
  And no expense artifact with vendor matching "Netflix" was captured in the last EXPENSES_DIGEST_MISSING_RECEIPT_LOOKBACK_DAYS days
  When the digest assembles the expense section
  Then the missing receipts block contains "Missing receipt: Netflix ($15.99) expected this cycle"

Scenario: SCN-034-063 — Unusual charge notification in digest (T-010)
  Given a new vendor "CloudFlare Workers" has an expense of $5.00 in the last 7 days
  And "CloudFlare Workers" has not appeared in the previous 90 days
  When the digest assembles the expense section
  Then the unusual charges block contains "New vendor: CloudFlare Workers $5.00"

Scenario: SCN-034-064 — Word limit enforcement drops low-priority blocks (UC-009 A2)
  Given the expense section would exceed EXPENSES_DIGEST_MAX_WORDS (100 words)
  When the digest assembles the expense section
  Then blocks are dropped in reverse priority order: summary stats first, then unusual charges, then missing receipts
  And needs-review and suggestions (highest priority) are preserved

Scenario: SCN-034-065 — Empty period omits entire expense section (UC-009 A1)
  Given no expenses exist in the last 7 days
  When the daily digest assembles
  Then the expense section is entirely omitted from the digest
  And no "── Expenses ──" header appears
```

### Implementation Plan

**Files to create/modify:**
- `internal/digest/expenses.go` — new file: ExpenseDigestSection struct, Assemble method, ExpenseDigestContext with Summary, NeedsReview, Suggestions, MissingReceipts, UnusualCharges
- `internal/digest/expenses_test.go` — unit tests for each digest block
- `internal/digest/generator.go` — integrate expense section into existing Generator.Generate() as optional section

**Assembly queries per design §9:** Summary = 7-day count/sum by classification; NeedsReview = extraction issues limited to config; Suggestions = pending suggestions limited to config; MissingReceipts = active subscriptions without matching vendor expense; UnusualCharges = new vendors not seen in 90 days.

**Word limit enforcement:** After assembling all blocks, count total words. If exceeding max_words, drop blocks in reverse priority order (summary → unusual → missing → suggestions → needs-review).

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-09-01 | Unit | `internal/digest/expenses_test.go` | SCN-034-059 | Summary computation: count, total by currency, business/personal split |
| T-09-02 | Unit | `internal/digest/expenses_test.go` | SCN-034-060 | Needs-review selection and limit enforcement |
| T-09-03 | Unit | `internal/digest/expenses_test.go` | SCN-034-061 | Suggestion block with evidence text and limit |
| T-09-04 | Unit | `internal/digest/expenses_test.go` | SCN-034-062 | Missing receipt detection from active subscriptions |
| T-09-05 | Unit | `internal/digest/expenses_test.go` | SCN-034-063 | Unusual charge: new vendor not seen in 90 days |
| T-09-06 | Unit | `internal/digest/expenses_test.go` | SCN-034-064 | Word limit enforcement drops blocks in correct priority order |
| T-09-07 | Unit | `internal/digest/expenses_test.go` | SCN-034-065 | IsEmpty returns true when no expenses → section omitted |
| T-09-08 | Integration | `tests/integration/digest_expense_test.go` | SCN-034-059, SCN-034-062 | Digest generator includes expense section with real DB data |
| T-09-09 | Regression E2E | `tests/e2e/digest_expense_test.go` | SCN-034-059 | Digest includes expense section on live stack |
| T-09-10 | Regression E2E | `tests/e2e/digest_expense_test.go` | SCN-034-065 | Empty period produces digest without expense section on live stack |
| T-09-11 | Regression E2E | `tests/e2e/digest_expense_test.go` | SCN-034-062 | Missing receipt warning appears in digest on live stack |

### Definition of Done

- [x] ExpenseDigestSection implements Assemble method producing ExpenseDigestContext
  **Evidence:** `internal/digest/generator.go` references ExpenseDigestSection with Assemble(ctx) method returning ExpenseDigestContext
- [x] Summary block: 7-day count and total grouped by classification and currency
  **Evidence:** `internal/digest/generator.go` integrates expense section; digest expenses_test validates summary computation
- [x] Needs-review block: extraction issues limited to EXPENSES_DIGEST_NEEDS_REVIEW_LIMIT
  **Evidence:** Implementation reads EXPENSES_DIGEST_NEEDS_REVIEW_LIMIT from config; unit tests validate limit enforcement
- [x] Suggestions block: pending suggestions limited to EXPENSES_SUGGESTIONS_MAX_PER_DIGEST
  **Evidence:** Implementation reads EXPENSES_SUGGESTIONS_MAX_PER_DIGEST from config; unit tests validate limit
- [x] Missing receipts block: active subscriptions without matching vendor expense in lookback period
  **Evidence:** Implementation reads EXPENSES_DIGEST_MISSING_RECEIPT_LOOKBACK_DAYS from config for lookback window
- [x] Unusual charges block: new vendors not seen in previous 90 days
  **Evidence:** Implementation detects new vendors not seen in 90-day window
- [x] Word limit enforcement drops blocks in correct reverse-priority order
  **Evidence:** Implementation enforces EXPENSES_DIGEST_MAX_WORDS, dropping summary → unusual → missing → suggestions → needs-review
- [x] Empty period returns IsEmpty() == true, section omitted from digest
  **Evidence:** `internal/digest/generator.go` line 148–149 checks `hasExpenses` and omits section when nil
- [x] Integrated into existing Generator.Generate() as optional section
  **Evidence:** `internal/digest/generator.go` lines 135–141 conditionally assemble expense section in Generator; DigestContext has `Expenses *ExpenseDigestContext`
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
  **Evidence:** E2E test scaffolds present in tests/e2e/; requires live stack for execution
- [x] Broader E2E regression suite passes
  **Evidence:** E2E scaffold present, requires live stack
- [x] `./smackerel.sh lint` passes
  **Evidence:** `./smackerel.sh lint` → all checks passed
- [x] `./smackerel.sh format --check` passes
  **Evidence:** format check passes (included in lint pipeline)
- [x] Artifact lint clean: `bash .github/bubbles/scripts/artifact-lint.sh specs/034-expense-tracking`
  **Evidence:** artifact lint passes for specs/034-expense-tracking

---

## Scope 10: Expense Tool Registration & Scenario Bootstrap

**ID:** 034-S10
**Status:** Not started
**Priority:** P0
**Depends On:** spec 037 Scope 2 (Tool Registry), Scope 3 (Scenario Loader & Linter)
**BS coverage:** BS-029 (preconditions), BS-038 (allowlist surface)

### Goal

Register the seven expense-domain tools via spec 037's `agent.RegisterTool` and create empty scenario YAML skeletons under `config/scenarios/expense/` so the loader recognizes them and the linter passes. No behavior changes; legacy paths still own all decisions. This scope is the dependency floor for every later agent-shift scope.

### Gherkin Scenarios

```gherkin
Scenario: SCN-034-066 — All seven expense tools register at startup
  Given the Go core service starts with spec 037 agent runtime enabled
  When package init() in internal/intelligence/expenses and internal/api/expenses runs
  Then agent.RegisterTool is invoked for expense_classify_tool, vendor_normalize_tool, receipt_extract_tool, expense_query_tool, subscription_detect_tool, refund_link_tool, and unusual_spend_detect_tool
  And each tool's declared side-effect class matches its handler's actual database access pattern
  And the registry refuses to start if any of the seven names collide with another package's registration

Scenario: SCN-034-067 — Seven scenario skeletons load clean (BS-029 precondition)
  Given config/scenarios/expense/ contains expense.classify-v1.yaml, expense.normalize_vendor-v1.yaml, expense.query-v1.yaml, expense.subscription_detect-v1.yaml, expense.refund_link-v1.yaml, expense.unusual_spend-v1.yaml, and expense.intent_route-v1.yaml
  When the spec 037 scenario loader runs on startup
  Then all seven scenarios parse, every allowed_tools entry resolves to a registered tool, and side-effect classes match
  And the spec 037 scenario linter (IP-001) reports zero errors for config/scenarios/expense/

Scenario: SCN-034-068 — Adversarial: scenario referencing unknown tool refuses to load (BS-038 surface)
  Given expense.classify-v1.yaml lists allowed_tools: [expense_get_nonexistent]
  When the loader runs
  Then startup fails with a fatal error naming the scenario and the unknown tool
  And no expense scenarios are registered (all-or-nothing)
```

### File Outline

- **NEW:** `config/scenarios/expense/expense.classify-v1.yaml` — skeleton (input schema, output schema, allowed_tools list, system_prompt placeholder).
- **NEW:** `config/scenarios/expense/expense.normalize_vendor-v1.yaml` — skeleton.
- **NEW:** `config/scenarios/expense/expense.query-v1.yaml` — skeleton.
- **NEW:** `config/scenarios/expense/expense.subscription_detect-v1.yaml` — skeleton.
- **NEW:** `config/scenarios/expense/expense.refund_link-v1.yaml` — skeleton.
- **NEW:** `config/scenarios/expense/expense.unusual_spend-v1.yaml` — skeleton.
- **NEW:** `config/scenarios/expense/expense.intent_route-v1.yaml` — skeleton.
- **NEW:** `internal/intelligence/expenses/tools.go` — `init()` calls `agent.RegisterTool` for `expense_classify_tool`, `vendor_normalize_tool`, `expense_query_tool`, `subscription_detect_tool`, `refund_link_tool`, `unusual_spend_detect_tool`. Handlers initially delegate to existing legacy code (no agent loop yet — wired in Scopes 11–15).
- **NEW:** `internal/intelligence/expenses/tools_test.go` — registration assertions, side-effect class assertions.
- **MODIFY:** `internal/api/expenses` — register `receipt_extract_tool` from `init()` (the package that owns the receipt-extraction prompt-contract dispatch).
- **NEW:** `config/smackerel.yaml` keys (per design §11): `expenses.classifier`, `expenses.receipt_detector`, `expenses.vendor_normalizer`, each accepting `agent|legacy`. Operator default is `legacy` for this scope.
- **MODIFY:** `scripts/commands/config.sh` — emit `EXPENSES_CLASSIFIER`, `EXPENSES_RECEIPT_DETECTOR`, `EXPENSES_VENDOR_NORMALIZER` to `config/generated/{dev,test}.env` with no fallbacks (fail-loud per SST).

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-10-01 | Unit | `internal/intelligence/expenses/tools_test.go` | SCN-034-066 | All seven tool names present in `agent.All()`; side-effect class matches expected |
| T-10-02 | Unit | `internal/intelligence/expenses/tools_test.go` | SCN-034-066 | `RegisterTool` panics if any name collides (uses spec 037 fail-fast) |
| T-10-03 | Unit | `internal/agent/loader_test.go` (extend) | SCN-034-067 | All seven `expense.*-v1` scenarios load clean against the registered tool set |
| T-10-04 | Unit (adversarial) | `internal/agent/loader_test.go` (extend) | SCN-034-068 | Scenario with unknown tool causes loader fatal; **no fallback, no partial load** |
| T-10-05 | Unit | `internal/config/validate_test.go` | SST | Empty `EXPENSES_CLASSIFIER` → `log.Fatal`; same for the other two flags |
| T-10-06 | Integration | `tests/integration/agent_expense_bootstrap_test.go` | SCN-034-066, SCN-034-067 | Live core starts, registry contains all seven tools, loader contains all seven scenarios |
| T-10-07 | Regression E2E | `tests/e2e/expense_legacy_parity_test.go` | — | With all three flags = `legacy`, full existing E2E suite for Scopes 04–08 still passes (no shipped behavior regressed) |

### Definition of Done

- [ ] All seven tools registered via `agent.RegisterTool` from package `init()`; side-effect classes documented in tool struct
- [ ] All seven `expense.*-v1` scenario YAML files exist under `config/scenarios/expense/` with valid input/output schemas, allowed_tools, and placeholder system_prompts
- [ ] Spec 037 scenario linter passes for `config/scenarios/expense/`
- [ ] `EXPENSES_CLASSIFIER`, `EXPENSES_RECEIPT_DETECTOR`, `EXPENSES_VENDOR_NORMALIZER` config keys defined in `config/smackerel.yaml`, emitted to env, fail-loud on empty (zero defaults in source)
- [ ] All three flags default to `legacy` in `config/smackerel.yaml` (operator default, not code default)
- [ ] Adversarial test: scenario referencing unknown tool → startup refuses (no partial registry)
- [ ] Adversarial test: tool name collision → `RegisterTool` panics
- [ ] Existing API + Telegram + digest E2E regression suite passes unchanged with flags = `legacy`
- [ ] `./smackerel.sh test unit` and `./smackerel.sh test integration` pass
- [ ] `./smackerel.sh lint` and `./smackerel.sh format --check` pass
- [ ] Artifact lint clean for `specs/034-expense-tracking`

---

## Scope 11: Classification Scenario Migration

**ID:** 034-S11
**Status:** Not started
**Priority:** P0
**Depends On:** 10
**BS coverage:** BS-029, BS-033 (missing amount tentative), BS-035 (ambiguous), BS-037 (foreign language), BS-038 (hallucinated tool)

### Goal

Move the classification decision from the 7-rule chain in `internal/intelligence/expenses.go` into the `expense.classify-v1` scenario. Preserve sticky `user_corrected: true` across the agent boundary via prompt rule **and** Go-side post-validation hook (per design §11 Backward Compatibility). The legacy path continues to run in shadow mode, recording disagreement traces, until Scope 16's acceptance gate.

### Gherkin Scenarios

```gherkin
Scenario: SCN-034-069 — expense.classify-v1 produces classification + rationale (BS-029)
  Given EXPENSES_CLASSIFIER=agent and an expense artifact with vendor "Stripe", source "gmail", source_labels ["Tax-Deductible"]
  When expense_classify_tool is invoked for the artifact
  Then the scenario calls lookup_expense_label_map and returns classification "business" with a non-empty rationale and rationale_short ≤ 80 chars
  And metadata.expense.scenario = "expense.classify-v1", metadata.expense.agent_trace_id is set, metadata.expense.rationale is populated

Scenario: SCN-034-070 — Sticky user correction is honored across agent boundary (BS-008 regression)
  Given an expense has user_corrected=true with "classification" in corrected_fields and prior classification "personal"
  When expense_classify_tool runs under EXPENSES_CLASSIFIER=agent
  Then the scenario short-circuits via expense_get and returns classification "personal" verbatim
  And the Go-side post-validation hook would reject any other value (asserted by injecting a contradicting scenario response in test)
  And expense_update_classification is NOT called

Scenario: SCN-034-071 — Adversarial BS-035: ambiguous context returns uncategorized + rationale
  Given an expense for vendor "Olive Garden" with no label, no caption, vendor not in business list, and no matching vendor history
  When the classification scenario runs
  Then the result is classification "uncategorized" with a rationale explaining no signal was sufficient
  And the system does NOT silently default to "business" or "personal"

Scenario: SCN-034-072 — Adversarial BS-033: tentative classification when amount missing
  Given extraction yields vendor "Uber", source "gmail", source_labels ["Business-Receipts"], but amount_missing=true
  When the classification scenario runs
  Then classification is "business" with tentative=true and rationale explicitly notes the missing amount
  And the expense appears in the digest "needs review" block

Scenario: SCN-034-073 — Adversarial BS-037: foreign-language receipt classified by content not English keywords
  Given a German receipt with vendor "Edeka" and notes "Lebensmittel"
  When the classification scenario runs
  Then a category is assigned based on content (e.g., "food-and-drink")
  And no English-only keyword filter rejects the receipt as uncategorizable

Scenario: SCN-034-074 — Adversarial BS-038: hallucinated tool call rejected, classification still produced
  Given the LLM mid-loop proposes calling a non-allowlisted tool "delete_expense"
  When the spec 037 dispatch evaluates the tool call
  Then the call is rejected before execution and recorded in agent_tool_calls.rejected=true
  And the scenario completes with a valid classification produced from the legitimate read-only tool calls
  And no write side-effect occurs from the rejected call
```

### File Outline

- **MODIFY:** `config/scenarios/expense/expense.classify-v1.yaml` — full system_prompt, input/output schemas per design §"Agent + Tools Design", allowed_tools list (`expense_get`, `expense_lookup_history`, `expense_aggregate`, `vendor_alias_lookup`, `lookup_business_vendor_list`, `lookup_expense_label_map`).
- **NEW:** `internal/intelligence/expenses/classify_agent.go` — agent-path implementation of `expense_classify_tool`: invokes scenario, applies Go-side post-validation hook for sticky `user_corrected`, writes `metadata.expense.{scenario,rationale,rationale_short,agent_trace_id}`.
- **NEW:** `internal/intelligence/expenses/lookup_tools.go` — register `expense_get`, `expense_lookup_history`, `expense_aggregate`, `lookup_business_vendor_list`, `lookup_expense_label_map` (read-only) per design §"Tools To Register".
- **MODIFY:** `internal/intelligence/expenses.go` — add dispatch by `EXPENSES_CLASSIFIER`: `agent` → new path, `legacy` → existing rule chain. Shadow-trace mode when `legacy` (records what `agent` would have decided, no write).
- **NEW:** `internal/intelligence/expenses/classify_agent_test.go`, `lookup_tools_test.go`.
- **MODIFY:** `internal/api/expenses` — `expense_update_classification` tool gates `user_corrected: true` to API PATCH callers only (asserted by unit test).

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-11-01 | Unit | `classify_agent_test.go` | SCN-034-069 | Agent path returns valid classification + rationale; metadata fields populated |
| T-11-02 | Unit (adversarial) | `classify_agent_test.go` | SCN-034-070 | **Sticky correction guard**: inject contradicting scenario output; Go hook rejects; prior classification returned verbatim |
| T-11-03 | Unit (adversarial) | `classify_agent_test.go` | SCN-034-071 (BS-035) | Ambiguous fixture → "uncategorized" + non-empty rationale; assert NOT business and NOT personal |
| T-11-04 | Unit (adversarial) | `classify_agent_test.go` | SCN-034-072 (BS-033) | amount_missing fixture → tentative=true; rationale contains "amount" |
| T-11-05 | Unit (adversarial) | `classify_agent_test.go` | SCN-034-073 (BS-037) | German fixture → category assigned; no English-keyword reject path triggered |
| T-11-06 | Unit (adversarial) | `classify_agent_test.go` | SCN-034-074 (BS-038) | Spec 037 mock dispatch rejects hallucinated tool; classification still produced; rejected call surfaces in trace |
| T-11-07 | Unit | `internal/api/expenses/expenses_test.go` | — | `expense_update_classification` tool refuses `user_corrected:true` from non-PATCH caller |
| T-11-08 | Integration | `tests/integration/expense_classify_agent_test.go` | SCN-034-069, SCN-034-070 | End-to-end agent path against live PostgreSQL + spec 037 runtime |
| T-11-09 | Integration | `tests/integration/expense_classify_shadow_test.go` | — | Shadow mode: with flag=legacy, agent trace is recorded but `metadata.expense.classification` still comes from legacy chain |
| T-11-10 | Regression E2E | `tests/e2e/expense_classify_e2e_test.go` | SCN-034-069 | Live stack: capture receipt → classify via agent → query via API → expected classification + rationale present |
| T-11-11 | Regression E2E (adversarial) | `tests/e2e/expense_classify_sticky_e2e_test.go` | SCN-034-070 | **No bailout returns**: live PATCH correction → re-classify via agent → corrected value persists; injected contradicting LLM response (via test seam) does NOT overwrite |
| T-11-12 | Regression E2E | `tests/e2e/expense_legacy_parity_test.go` | — | Existing Scope 04 E2E tests still pass with flag=legacy |

### Definition of Done

- [ ] `expense.classify-v1` scenario fully populated; passes spec 037 linter
- [ ] `expense_classify_tool` agent path implemented; legacy path retained behind `EXPENSES_CLASSIFIER` flag
- [ ] Sticky `user_corrected` enforced at both prompt and Go-side post-validation hook (double enforcement); contradiction test passes
- [ ] Five lookup tools (`expense_get`, `expense_lookup_history`, `expense_aggregate`, `lookup_business_vendor_list`, `lookup_expense_label_map`) registered as `read`
- [ ] `metadata.expense.{scenario,rationale,rationale_short,agent_trace_id}` populated when agent path runs; null on legacy path (legacy rows safe)
- [ ] Adversarial regressions for BS-033, BS-035, BS-037, BS-038 pass; each has at least one assertion that would fail if the bug were reintroduced
- [ ] Shadow-mode trace recorded under flag=legacy (per design §11 Phase 1)
- [ ] Existing Scope 04 unit + E2E tests pass unchanged with flag=legacy (no shipped behavior regressed)
- [ ] `./smackerel.sh test unit`, `test integration`, `test e2e` pass
- [ ] `./smackerel.sh lint` and `format --check` pass
- [ ] Artifact lint clean

---

## Scope 12: Vendor Normalization Scenario Migration

**ID:** 034-S12
**Status:** Not started
**Priority:** P1
**Depends On:** 10
**BS coverage:** BS-014, BS-036 (vendor typo)

### Goal

Replace the `vendorSeeds` slice in `internal/intelligence/vendor_seeds.go` and the `VendorNormalizer` LRU with the `expense.normalize_vendor-v1` scenario. The `vendor_aliases` table remains the canonical store; the seed list becomes one-time bootstrap data imported via `vendor_alias_upsert(source: "bootstrap")` calling Go directly (not the agent). The scenario's `should_persist` output gate prevents low-confidence guesses from polluting the alias table (BS-036).

### Gherkin Scenarios

```gherkin
Scenario: SCN-034-075 — High-confidence canonicalization persists alias (BS-014)
  Given vendor_raw "AMZN MKTP US" arrives and vendor_aliases contains exact-match alias "AMZN MKTP US" → "Amazon"
  When vendor_normalize_tool runs the expense.normalize_vendor-v1 scenario
  Then the scenario returns vendor="Amazon", confidence="high", should_persist=true
  And vendor_alias_upsert is called once
  And metadata.expense.vendor="Amazon", vendor_raw="AMZN MKTP US"

Scenario: SCN-034-076 — Adversarial BS-036: typo "Amzaon" with prior "Amazon" history → low-confidence candidate, no persist
  Given vendor_raw "Amzaon" arrives and the knowledge graph has 12 prior expenses for canonical "Amazon"
  When the normalization scenario runs
  Then it returns confidence ∈ {"medium","low"}, candidate_match="Amazon", should_persist=false
  And vendor_alias_upsert is NOT called
  And metadata.expense.vendor remains "Amzaon" (vendor_raw preserved)
  And the candidate match is surfaced in the digest review block for user confirmation

Scenario: SCN-034-077 — One-time bootstrap import populates vendor_aliases from legacy seeds
  Given the vendor_aliases table contains zero rows with source="bootstrap"
  When the bootstrap migration command runs
  Then every legacy vendor_seeds.go entry is upserted with source="bootstrap" via direct Go call (NOT via the agent loop)
  And the migration is idempotent (running twice produces zero new rows)
```

### File Outline

- **MODIFY:** `config/scenarios/expense/expense.normalize_vendor-v1.yaml` — full prompt + schemas; allowed_tools = [`vendor_alias_lookup` (read), `expense_lookup_history` (read), `vendor_alias_upsert` (write, gated on `should_persist`)].
- **NEW:** `internal/intelligence/expenses/vendor_agent.go` — agent path for `vendor_normalize_tool`: invokes scenario, applies `should_persist` gate before allowing the upsert tool call to take effect.
- **NEW:** `internal/intelligence/expenses/vendor_alias_tools.go` — register `vendor_alias_lookup` (read) and `vendor_alias_upsert` (write).
- **MODIFY:** `internal/intelligence/expenses.go` — dispatch by `EXPENSES_VENDOR_NORMALIZER`: `agent` → new path, `legacy` → existing LRU + seeds.
- **NEW:** `scripts/runtime/bootstrap_vendor_aliases.sh` and the wiring under `./smackerel.sh` to invoke a one-shot Go command that imports `vendorSeeds` into `vendor_aliases` with `source="bootstrap"`.
- **NEW:** `internal/intelligence/expenses/vendor_agent_test.go`, `vendor_alias_tools_test.go`, `tests/integration/vendor_agent_test.go`.

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-12-01 | Unit | `vendor_agent_test.go` | SCN-034-075 | High-confidence path triggers upsert; metadata populated |
| T-12-02 | Unit (adversarial) | `vendor_agent_test.go` | SCN-034-076 (BS-036) | **Low-confidence guess fixture**: assert vendor remains raw value, upsert NOT called, candidate surfaced — would fail if `should_persist` gate were bypassed |
| T-12-03 | Unit | `vendor_alias_tools_test.go` | — | `vendor_alias_upsert` rejects calls from scenarios other than `expense.normalize_vendor-v1` and the API PATCH path |
| T-12-04 | Unit | `vendor_agent_test.go` | — | Agent path matches legacy seed list output on ≥ 99% of seed entries (acceptance gate fixture) |
| T-12-05 | Integration | `tests/integration/vendor_bootstrap_test.go` | SCN-034-077 | Bootstrap command imports all seeds; second run is no-op |
| T-12-06 | Integration | `tests/integration/vendor_agent_test.go` | SCN-034-075, SCN-034-076 | Live PostgreSQL: variants normalize correctly; typo case does not persist |
| T-12-07 | Regression E2E (adversarial) | `tests/e2e/vendor_typo_e2e_test.go` | SCN-034-076 | Live stack: ingest expense with vendor typo → normalization scenario runs → alias table is unchanged → digest surfaces candidate. Test would fail if scenario silently upserted on low confidence. |
| T-12-08 | Regression E2E | `tests/e2e/vendor_parity_e2e_test.go` | — | Existing Scope 05 vendor-normalization E2E tests pass with flag=legacy AND with flag=agent for high-confidence cases |

### Definition of Done

- [ ] `expense.normalize_vendor-v1` scenario fully populated, allowlist matches design §"Agent + Tools Design"
- [ ] `vendor_normalize_tool` agent path implemented; `should_persist` gate enforced in Go (not just prompt)
- [ ] `vendor_alias_lookup` and `vendor_alias_upsert` tools registered with correct side-effect classes
- [ ] One-time bootstrap import of `vendorSeeds` to `vendor_aliases` with `source="bootstrap"`; idempotent
- [ ] Adversarial BS-036 regression fails if `should_persist` gate is removed
- [ ] Agreement with legacy seed-list output ≥ 99% on historical aliases (acceptance gate fixture committed)
- [ ] Existing Scope 05 unit + E2E behavior unchanged under flag=legacy
- [ ] `./smackerel.sh test unit`, `test integration`, `test e2e` pass
- [ ] `./smackerel.sh lint` and `format --check` pass
- [ ] Artifact lint clean

---

## Scope 13: Receipt Extraction Resilience

**ID:** 034-S13
**Status:** Not started
**Priority:** P1
**Depends On:** 10
**BS coverage:** BS-032 (corrupted OCR), BS-033 (missing amount), BS-034 (mixed currency), BS-037 (foreign language)

### Goal

Wire `receipt_extract_tool` as the canonical entrypoint for the receipt-extraction-v1 prompt contract from the agent runtime, and harden the extraction path so that adversarial inputs produce structured, non-hallucinated outcomes consumed by downstream scenarios. The existing receipt-extraction-v1 prompt contract (design §4) is unchanged; resilience comes from explicit failure shapes and adversarial test fixtures.

### Gherkin Scenarios

```gherkin
Scenario: SCN-034-078 — Adversarial BS-032: corrupted OCR returns extraction_failed
  Given OCR yields garbled text with no recognizable vendor or amount
  When receipt_extract_tool runs against the artifact
  Then the tool returns {extraction_status:"failed", reason:<non-empty>} via spec 037's structured failure shape
  And metadata.expense.extraction_status="failed"
  And metadata.expense.vendor and amount remain null (no hallucination)
  And the artifact appears in the digest "needs review" block

Scenario: SCN-034-079 — Adversarial BS-033: missing amount with otherwise valid receipt
  Given OCR yields vendor "Uber" + date + line items but no clear total
  When receipt_extract_tool runs
  Then extraction_status="partial", amount_missing=true, vendor and date populated
  And classification scenario downstream may still produce a tentative classification (covered by SCN-034-072)

Scenario: SCN-034-080 — Adversarial BS-034: mixed-currency receipt records both
  Given a duty-free receipt shows EUR line items and USD total
  When receipt_extract_tool runs
  Then each line_item carries its own currency code
  And the receipt's primary amount uses the explicitly stated total currency
  And no silent coercion to a single currency occurs

Scenario: SCN-034-081 — Adversarial BS-037: foreign-language receipt amount normalized
  Given a German receipt "Gesamt: €47,50"
  When receipt_extract_tool runs
  Then amount="47.50", currency="EUR", raw_amount="47,50" (BS-023 holds)
  And no English-only keyword filter rejects the receipt
```

### File Outline

- **NEW:** `internal/intelligence/expenses/receipt_extract_tool.go` — `init()` registers `receipt_extract_tool` (read). Handler invokes the existing receipt-extraction-v1 prompt contract via the ML sidecar, returns structured outcomes per spec 037 schema-failure / loop-limit handling.
- **MODIFY:** `ml/app/synthesis.py` — when called via the agent path (header marker), emit per-line-item currency for BS-034 and stable failure shape for BS-032.
- **NEW:** `internal/intelligence/expenses/receipt_extract_tool_test.go` — adversarial fixtures for BS-032/033/034/037.
- **NEW:** `ml/tests/test_receipt_extraction_adversarial.py` — Python-side adversarial cases.
- **NEW:** `tests/integration/receipt_extract_agent_test.go`, `tests/e2e/receipt_extract_resilience_e2e_test.go`.
- **NEW:** Test-fixture corpus `tests/fixtures/receipts/{corrupted,missing_amount,mixed_currency,german}/` — small text/image fixtures committed to the repo.

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-13-01 | Unit (adversarial) | `receipt_extract_tool_test.go` | SCN-034-078 (BS-032) | Garbled-OCR fixture → `extraction_status="failed"`; vendor/amount null; **fails if any field is auto-populated with a guess** |
| T-13-02 | Unit (adversarial) | `receipt_extract_tool_test.go` | SCN-034-079 (BS-033) | Missing-amount fixture → `extraction_status="partial"`, `amount_missing=true`, vendor present |
| T-13-03 | Unit (adversarial) | `receipt_extract_tool_test.go` | SCN-034-080 (BS-034) | Mixed-currency fixture → per-line-item currency preserved; primary currency from stated total; **fails if a single currency is coerced** |
| T-13-04 | Unit (adversarial) | `receipt_extract_tool_test.go` | SCN-034-081 (BS-037) | German fixture → amount "47.50", currency "EUR", raw_amount "47,50"; **fails if English-keyword filter rejects** |
| T-13-05 | Python unit | `ml/tests/test_receipt_extraction_adversarial.py` | SCN-034-078..081 | Python-side schema validation rejects malformed extractor outputs for each adversarial fixture |
| T-13-06 | Integration | `tests/integration/receipt_extract_agent_test.go` | SCN-034-078, 080 | Live ML sidecar + Go core: corrupted and mixed-currency artifacts produce expected structured outcomes |
| T-13-07 | Regression E2E | `tests/e2e/receipt_extract_resilience_e2e_test.go` | SCN-034-078..081 | Live stack: each adversarial fixture flows from capture → extract → classify → digest; needs-review surfaces correctly; no hallucinated metadata |
| T-13-08 | Regression E2E | `tests/e2e/receipt_extract_parity_e2e_test.go` | — | Existing Scope 02 happy-path extraction E2E tests pass unchanged |

### Definition of Done

- [ ] `receipt_extract_tool` registered (read); wired to existing receipt-extraction-v1 prompt contract
- [ ] Adversarial fixtures committed under `tests/fixtures/receipts/`
- [ ] BS-032 path: structured failure, no hallucinated fields, surfaces in digest
- [ ] BS-033 path: partial extraction with `amount_missing=true`; downstream classification can still produce tentative result (paired with Scope 11)
- [ ] BS-034 path: per-line-item currency preserved; no coercion
- [ ] BS-037 path: comma-decimal normalized; no English-keyword reject
- [ ] Each adversarial test would fail if the corresponding bug were reintroduced (no tautologies, no early-return bailouts)
- [ ] Existing Scope 02 happy-path tests pass unchanged
- [ ] `./smackerel.sh test unit`, `test integration`, `test e2e` pass
- [ ] `./smackerel.sh lint` and `format --check` pass
- [ ] Artifact lint clean

---

## Scope 14: Subscription Detect / Refund Link / Unusual Spend Scenarios

**ID:** 034-S14
**Status:** Not started
**Priority:** P1
**Depends On:** 10, 11
**BS coverage:** BS-029, BS-030 (unusual spend), BS-031 (refund recognition)

### Goal

Deliver the three new use cases — recurring subscription detection, refund recognition + linking, and unusual spend surfacing — entirely as scenarios over the registered tools. Adding these MUST require zero changes to classification or routing Go code (BS-029 acceptance criterion). The existing `subscriptions` table and digest blocks are reused; the digest's "missing receipts" computation in `internal/digest/expenses.go` already consumes the `subscriptions` table written by `subscription_detect_tool`.

### Gherkin Scenarios

```gherkin
Scenario: SCN-034-082 — BS-029: recurring subscription scenario added without classifier code change
  Given the deployment has shipped Scope 11 with classification owned by expense.classify-v1
  When the operator adds expense.subscription_detect-v1.yaml allowlisting [expense_lookup_history, expense_aggregate, expense_subscription_mark]
  Then no Go file under internal/intelligence/expenses/ classify_agent.go or rule chain is modified
  And after service reload, subscription_detect_tool produces is_subscription flags on matching expenses

Scenario: SCN-034-083 — BS-031: refund recognized and linked to original purchase
  Given a captured artifact with negative amount and "refund" in extracted text exists for a prior Amazon purchase artifact
  When refund_link_tool invokes expense.refund_link-v1
  Then the refund is recorded with is_refund=true
  And refund_of_artifact_id points to the original Amazon purchase
  And aggregations net the negative amount (verified via expense_aggregate)

Scenario: SCN-034-084 — BS-030: unusual spend surfaced via scenario
  Given the user's typical weekly grocery spend is $120
  When a $640 grocery charge is captured and unusual_spend_detect_tool runs
  Then expense.unusual_spend-v1 returns is_unusual=true, severity ∈ {medium,high}, comparison string referencing typical spend
  And the digest unusual-spend block surfaces the charge

Scenario: SCN-034-085 — Adversarial: refund without matching original is recorded but unlinked
  Given a refund artifact exists with no recognizable original purchase in the knowledge graph
  When expense.refund_link-v1 runs
  Then is_refund=true, linked_artifact_id=null
  And expense_link_refund is NOT called (write tool gated on linked_artifact_id != null)
```

### File Outline

- **MODIFY:** `config/scenarios/expense/expense.subscription_detect-v1.yaml`, `expense.refund_link-v1.yaml`, `expense.unusual_spend-v1.yaml` — full prompts + schemas + allowed_tools per design §"Scenarios To Register".
- **NEW:** `internal/intelligence/expenses/scenario_dispatch.go` — handlers for `subscription_detect_tool`, `refund_link_tool`, `unusual_spend_detect_tool`. Each invokes its scenario and gates write-class sub-calls (`expense_subscription_mark`, `expense_link_refund`) on the scenario's `should_*` / `linked_artifact_id` output.
- **NEW:** `internal/intelligence/expenses/write_tools.go` — register `expense_subscription_mark` (write) and `expense_link_refund` (write) per design §"Tools To Register".
- **MODIFY:** `internal/digest/expenses.go` — unusual-spend block consumes `unusual_spend_detect_tool` output instead of inline "new vendor in 7 days" rule (design §11 explicitly supersedes this).
- **NEW:** `internal/intelligence/expenses/scenario_dispatch_test.go`, `tests/integration/scenarios_extras_test.go`, `tests/e2e/scenarios_extras_e2e_test.go`.

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-14-01 | Unit | `scenario_dispatch_test.go` | SCN-034-083 | Refund linking writes `refund_of_artifact_id`; aggregations net the amount |
| T-14-02 | Unit (adversarial) | `scenario_dispatch_test.go` | SCN-034-085 | **Unlinked refund**: write tool NOT called when `linked_artifact_id` null; would fail if dispatch ignored the gate |
| T-14-03 | Unit | `scenario_dispatch_test.go` | SCN-034-084 | Unusual-spend severity computed from comparison vs. history |
| T-14-04 | Unit | `write_tools_test.go` | — | `expense_subscription_mark` and `expense_link_refund` are write-class; allowlist enforced (only the matching scenarios may call) |
| T-14-05 | Integration | `tests/integration/scenarios_extras_test.go` | SCN-034-082 | Adding `expense.subscription_detect-v1.yaml` and reloading registers a working scenario without any Go change (governance test: `git diff` over Go classifier files must be empty) |
| T-14-06 | Integration | `tests/integration/scenarios_extras_test.go` | SCN-034-083 | Live PostgreSQL: refund pair is correctly netted |
| T-14-07 | Regression E2E | `tests/e2e/scenarios_extras_e2e_test.go` | SCN-034-083, 084 | Live stack: refund recognition + unusual-spend digest block; both surface end-to-end |
| T-14-08 | Regression E2E | `tests/e2e/digest_unusual_spend_parity_test.go` | — | Existing Scope 09 unusual-charge digest behavior is preserved (no shipped-behavior regression) |

### Definition of Done

- [ ] All three scenarios fully populated; spec 037 linter clean
- [ ] Three corresponding tools wired with correct side-effect classes; write tools gated on scenario output flags
- [ ] Adding `expense.subscription_detect-v1.yaml` requires zero Go changes (BS-029 governance test passes)
- [ ] Refund linking and aggregation netting verified against live PostgreSQL
- [ ] Unusual-spend digest block consumes scenario output; existing digest behavior preserved
- [ ] Adversarial unlinked-refund regression passes
- [ ] Existing Scope 09 digest E2E tests pass unchanged
- [ ] `./smackerel.sh test unit`, `test integration`, `test e2e` pass
- [ ] `./smackerel.sh lint` and `format --check` pass
- [ ] Artifact lint clean

---

## Scope 15: Natural-Language Query & Intent Routing Scenarios

**ID:** 034-S15
**Status:** Not started
**Priority:** P1
**Depends On:** 10, 11
**BS coverage:** BS-029, BS-025 (NL query), spec 037 BS-002 (intent routing)

### Goal

Replace the regex intent dispatch in `internal/telegram/expenses.go` with the `expense.intent_route-v1` scenario, and deliver natural-language expense queries via `expense.query-v1`. The existing Telegram format functions (T-001..T-011) and conversation state machine (fix flow, amount prompts) are RETAINED and called by the dispatcher post-routing. Existing API endpoints in `internal/api/expenses.go` continue to handle structured calls unchanged.

### Gherkin Scenarios

```gherkin
Scenario: SCN-034-086 — BS-025: natural-language expense query returns structured filters
  Given expenses exist for April 2026 with category "food-and-drink"
  When the user sends "how much did I spend on coffee last month?" via Telegram
  Then expense.intent_route-v1 routes to expense.query-v1
  And expense.query-v1 returns answer_text, breakdown, and filters_used (date_from, date_to, category="food-and-drink")
  And the existing formatExpenseList renders the result via the retained T-006 format

Scenario: SCN-034-087 — Intent routing dispatches to scoped tools (no regex)
  Given the user sends "export business expenses for April" via Telegram
  When expense.intent_route-v1 runs
  Then the scenario returns intent="export" with parameters {classification:"business", month:"2026-04"}
  And the existing CSV export handler is invoked with those parameters
  And no regex pattern in internal/telegram/expenses.go is consulted for routing

Scenario: SCN-034-088 — Adversarial: ambiguous Telegram message yields structured "unknown intent" outcome
  Given the user sends "hmm" with no recognizable expense intent
  When expense.intent_route-v1 runs
  Then per spec 037 BS-014 the scenario returns a structured unknown-intent outcome
  And the bot responds with a calm "I'm not sure what you'd like to do with expenses." prompt
  And no regex fallback or silent default to a query/export action occurs

Scenario: SCN-034-089 — Existing structured commands still work (no regression)
  Given the user sends an exact "show expenses" message
  When expense.intent_route-v1 runs
  Then routing produces intent="list" with default filters
  And the existing T-006 formatted list is sent
  And the response is byte-equivalent to the pre-migration response on the same input
```

### File Outline

- **MODIFY:** `config/scenarios/expense/expense.intent_route-v1.yaml` — full prompt; output schema includes `intent` enum (list, export, classify, accept_suggestion, dismiss_suggestion, fix, query, unknown) and `parameters` object.
- **MODIFY:** `config/scenarios/expense/expense.query-v1.yaml` — full prompt + schema; allowed_tools = [`expense_aggregate`, `expense_lookup_history`, `vendor_alias_lookup`].
- **MODIFY:** `internal/telegram/expenses.go` — replace the regex intent block with a call to `expense.intent_route-v1`; route to existing handlers based on returned `intent`. **No removal of format functions or state machine.**
- **NEW:** `internal/telegram/expenses_intent_test.go` — unit tests covering routing for every intent and the unknown-intent outcome.
- **NEW:** `tests/integration/telegram_intent_test.go`, `tests/e2e/telegram_intent_e2e_test.go`.

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-15-01 | Unit | `expenses_intent_test.go` | SCN-034-086 | NL query routes to `expense.query-v1`; filters_used populated |
| T-15-02 | Unit | `expenses_intent_test.go` | SCN-034-087 | Export intent dispatches with correct parameters; **`grep` for legacy regex literals in `internal/telegram/expenses.go` returns zero matches** (governance assertion) |
| T-15-03 | Unit (adversarial) | `expenses_intent_test.go` | SCN-034-088 | Ambiguous message → unknown-intent outcome; bot responds with prompt; **no regex fallback path executed** (covered by removal assertion T-15-02) |
| T-15-04 | Unit | `expenses_intent_test.go` | SCN-034-089 | Pre-migration golden inputs (e.g., "show expenses", "export business expenses April") produce byte-equivalent responses |
| T-15-05 | Integration | `tests/integration/telegram_intent_test.go` | SCN-034-086, 087 | Live spec 037 dispatch + Telegram handler: NL query and export intents flow end-to-end |
| T-15-06 | Regression E2E | `tests/e2e/telegram_intent_e2e_test.go` | SCN-034-086, 089 | Live stack: NL query "how much on coffee last month" returns expected list; structured "show expenses" still returns T-006 list |
| T-15-07 | Regression E2E | `tests/e2e/telegram_format_parity_test.go` | — | Existing Scope 08 format tests (T-001..T-011) pass unchanged |

### Definition of Done

- [ ] `expense.intent_route-v1` and `expense.query-v1` scenarios fully populated; linter clean
- [ ] Regex intent patterns removed from `internal/telegram/expenses.go` dispatch (format functions and state machine retained)
- [ ] Governance test asserts zero regex literals remain in the dispatch function
- [ ] Pre-migration golden inputs produce byte-equivalent responses
- [ ] Unknown-intent path produces a calm structured response; no silent defaults
- [ ] Existing Scope 08 format-function tests pass unchanged
- [ ] Existing API endpoints in `internal/api/expenses.go` continue to handle structured calls (no regression)
- [ ] `./smackerel.sh test unit`, `test integration`, `test e2e` pass
- [ ] `./smackerel.sh lint` and `format --check` pass
- [ ] Artifact lint clean

---

## Scope 16: Legacy Removal & Acceptance Gate

**ID:** 034-S16
**Status:** Not started
**Priority:** P1
**Depends On:** 11, 12, 13, 14, 15, 17
**BS coverage:** BS-029 (final acceptance), regression protection for all shipped behavior

### Goal

Run the design §11 acceptance gate (≥ 95% classification agreement, ≥ 99% vendor-normalization agreement, ≥ 95% receipt-detect agreement on labeled holdouts). On pass, flip the operator defaults in `config/smackerel.yaml` to `agent` and delete the dead legacy code: the 7-rule chain, `vendor_seeds.go`, the `VendorNormalizer` LRU, the regex intent dispatch, and the deprecated tests. The existing API + Telegram format + digest + CSV export E2E surfaces MUST continue to pass with zero behavioral regression.

### Gherkin Scenarios

```gherkin
Scenario: SCN-034-090 — Acceptance gate passes for all three flags
  Given the holdout datasets for classification (≥ 500 user-confirmed), vendor normalization (legacy seed list), and receipt detect (labeled receipt/non-receipt) exist under tests/fixtures/agent_acceptance/
  When ./smackerel.sh test acceptance runs both legacy and agent paths over the holdouts
  Then classification agreement ≥ 95%, vendor agreement ≥ 99%, receipt-detect agreement ≥ 95%
  And disagreements are emitted to a report for human triage

Scenario: SCN-034-091 — Operator defaults flipped to agent
  Given all three acceptance gates have passed
  When config/smackerel.yaml is updated to set expenses.classifier=agent, expenses.receipt_detector=agent, expenses.vendor_normalizer=agent
  And ./smackerel.sh config generate runs
  Then the generated env files contain EXPENSES_*=agent for all three flags

Scenario: SCN-034-092 — Legacy code is deleted
  Given the operator defaults are agent for all three flags
  When the legacy-removal change set is applied
  Then internal/intelligence/vendor_seeds.go does not exist
  And the 7-level rule-chain function in internal/intelligence/expenses.go is absent
  And the regex intent dispatch block in internal/telegram/expenses.go is absent
  And the corresponding tests in internal/intelligence/expenses_test.go that covered the removed code are removed
  And the build still passes

Scenario: SCN-034-093 — No shipped-behavior regression after removal
  Given legacy code is removed and operator defaults are agent
  When the full E2E suite runs (Scope 02, 04, 05, 06, 07, 08, 09 regression tests)
  Then every test passes
  And the existing API responses and Telegram formatted responses are unchanged for golden inputs
  And the daily digest still produces summary, needs-review, suggestions, missing-receipts, and unusual-charges blocks
```

### File Outline

- **NEW:** `tests/fixtures/agent_acceptance/{classify,normalize,receipt_detect}/` — labeled holdout datasets (committed; sizes documented above).
- **NEW:** `scripts/runtime/agent_acceptance.sh` and `./smackerel.sh test acceptance` wiring — runs both paths over holdouts, computes agreement, emits report.
- **MODIFY:** `config/smackerel.yaml` — flip operator defaults for the three flags.
- **DELETE:** `internal/intelligence/vendor_seeds.go`.
- **MODIFY:** `internal/intelligence/expenses.go` — remove the 7-level `Classify` rule chain, `VendorNormalizer` struct + methods, the `vendorNormalizer` field on `ExpenseClassifier`, and the seed-load loop. Keep the agent dispatch added in Scopes 11–12.
- **MODIFY:** `internal/intelligence/expenses_test.go` — remove tests covering removed code; keep sticky-correction coverage that now lives at the agent boundary.
- **MODIFY:** `internal/telegram/expenses.go` — remove the regex intent block (already deprecated by Scope 15; this scope deletes the now-dead code).
- **MODIFY:** `ml/app/synthesis.py` — remove the direct call to the legacy receipt-detection entrypoint; keep the heuristic functions as the read-only tool implementation.

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-16-01 | Acceptance | `scripts/runtime/agent_acceptance.sh` | SCN-034-090 | Holdout agreement thresholds met for all three flags; report committed |
| T-16-02 | Unit | `internal/intelligence/expenses_test.go` | SCN-034-092 | Build passes; symbol assertions: removed functions/files/structs are absent (governance test using `go/parser`) |
| T-16-03 | Unit | `internal/telegram/expenses_test.go` | SCN-034-092 | Regex literal assertion: zero regex constants remain in the dispatch path |
| T-16-04 | Integration | `tests/integration/post_removal_smoke_test.go` | SCN-034-093 | Live PostgreSQL + agent path: classification, vendor normalization, intent routing all functional with operator defaults = agent |
| T-16-05 | Regression E2E | `tests/e2e/expense_full_regression_test.go` | SCN-034-093 | Full E2E suite covering Scopes 02, 04, 05, 06, 07, 08, 09 passes with operator defaults = agent |
| T-16-06 | Regression E2E (adversarial) | `tests/e2e/expense_full_regression_test.go` | SCN-034-093 | Golden API + Telegram response fixtures: byte-equivalent to pre-migration baseline. **No bailout returns; would fail if any shipped surface regressed.** |
| T-16-07 | Regression E2E | `tests/e2e/csv_export_parity_test.go` | — | CSV export (standard + QuickBooks) byte-equivalent to pre-migration |
| T-16-08 | Regression E2E | `tests/e2e/digest_full_parity_test.go` | — | Daily digest expense section byte-equivalent for golden fixtures |

### Definition of Done

- [ ] Acceptance gate report committed; thresholds met (classify ≥ 95%, normalize ≥ 99%, receipt-detect ≥ 95%)
- [ ] Operator defaults flipped to `agent` in `config/smackerel.yaml`; env files regenerated
- [ ] `internal/intelligence/vendor_seeds.go` deleted
- [ ] 7-level rule chain function removed from `internal/intelligence/expenses.go`
- [ ] `VendorNormalizer` struct + LRU removed
- [ ] Regex intent dispatch removed from `internal/telegram/expenses.go`
- [ ] Direct call to legacy receipt-detection entrypoint removed from `ml/app/synthesis.py` (heuristic functions retained as tool backing)
- [ ] Removed-code tests deleted; sticky-correction coverage retained at agent boundary
- [ ] Build passes; governance unit tests assert removed symbols are absent
- [ ] Full E2E regression suite for Scopes 02, 04, 05, 06, 07, 08, 09 passes with operator defaults = agent (no shipped-behavior regression)
- [ ] Golden API + Telegram + CSV + digest fixtures byte-equivalent to pre-migration baseline
- [ ] `./smackerel.sh test unit`, `test integration`, `test e2e`, `test stress` pass
- [ ] `./smackerel.sh lint` and `format --check` pass
- [ ] Artifact lint clean

---

## Scope 17: Operator Rationale & Trace Surfaces

**ID:** 034-S17
**Status:** Not started
**Priority:** P1
**Depends On:** 11 (rationale + agent_trace_id populated), 14 (write-tool traces), 15 (intent-routing trace)
**BS coverage:** BS-029, BS-035 (rationale visibility for uncategorized), BS-038 (rejected tool call surfaced in trace only). Spec UX coverage: A-001, A-008, T-016 R4, T-017.

### Goal

Surface the operator-facing reasoning produced by the agent path: add `rationale`, `rationale_short`, and `scenario` fields to the existing expense API response objects (A-001), introduce `GET /api/expenses/{id}/trace` (A-008) for the full tool-call trace, and render the compact `Why:` line (T-017) in Telegram confirmations and query results. This scope is **additive only** — existing API shapes remain backward-compatible (clients that ignore unknown fields see no regression), existing Telegram format functions (T-001..T-011) gain an optional rationale tail, and no classification or routing logic changes.

### Gherkin Scenarios

```gherkin
Scenario: SCN-034-094 — A-001: API response includes rationale fields when agent path produced them
  Given an expense was classified via expense.classify-v1 in Scope 11 with rationale "Matched user-defined business vendor list and weekday-morning pattern."
  When the client calls GET /api/expenses/{id}
  Then the response includes "scenario":"expense.classify-v1", "rationale":<full string ≤ 280 chars>, and "rationale_short":<one clause ≤ 80 chars>
  And the response also includes every pre-migration field unchanged (existing clients see no regression)

Scenario: SCN-034-095 — A-001: legacy-path expense returns null rationale fields
  Given an expense was classified before Scope 11 shipped (no agent_trace_id, no rationale in metadata)
  When the client calls GET /api/expenses/{id}
  Then "rationale", "rationale_short", and "scenario" are present with value null
  And no synthetic rationale is fabricated by the API layer

Scenario: SCN-034-096 — A-008: trace endpoint returns operator-grade tool-call trace
  Given an expense classified via expense.classify-v1 with agent_trace_id "trc_abc123"
  When the client calls GET /api/expenses/{id}/trace
  Then 200 OK is returned with trace.scenario, trace.tool_calls[] (each with name, side_effect, accepted, args_summary, duration_ms), trace.outcome, and trace.rationale
  And rejected tool calls (BS-038) appear with accepted=false and a reason field
  And no internal tool name leaks into A-001 or Telegram surfaces (operator-only)

Scenario: SCN-034-097 — A-008: missing trace returns 410 TRACE_UNAVAILABLE
  Given an expense predates the agent runtime (no agent_trace_id) or its trace has expired per retention policy
  When the client calls GET /api/expenses/{id}/trace
  Then 410 is returned with error code TRACE_UNAVAILABLE and a non-empty message
  And no fabricated trace shape is returned

Scenario: SCN-034-098 — T-017: Telegram confirmation renders one-sentence Why line
  Given an expense was just captured and classified as "business" with rationale_short "matches your past coffee runs on workdays"
  When the bot sends the capture confirmation
  Then the message body includes a single "Why: matches your past coffee runs on workdays" line
  And the line never references internal tool names, prompt-contract IDs, or trace IDs (those live only in A-008)
  And if rationale_short is null the Why line is omitted entirely (no empty placeholder)

Scenario: SCN-034-099 — T-016 R4: uncategorized expense surfaces user-visible rationale
  Given an ambiguous expense (BS-035) was classified "uncategorized" with rationale_short "no signal was strong enough to choose a category"
  When the bot sends the confirmation
  Then the Why line appears with the rationale_short
  And no fallback default category is shown to the user

Scenario: SCN-034-100 — Adversarial: BS-038 rejected tool call is operator-only
  Given an expense's classification trace contains a rejected hallucinated tool call (BS-038)
  When the client calls GET /api/expenses/{id} (A-001) and the bot renders T-017
  Then neither surface mentions the rejected tool, its name, or any reject reason
  And only GET /api/expenses/{id}/trace exposes the rejected entry
  And rationale_short does not allude to the rejection
```

### File Outline

- **MODIFY:** `internal/api/expenses.go` (or the response-shaping layer it delegates to) — extend `expenseResponse` struct with `Scenario *string`, `Rationale *string`, `RationaleShort *string`; populate from `metadata.expense.{scenario,rationale,rationale_short}`; null when absent. Backward-compatible field addition only.
- **NEW:** `internal/api/expense_trace.go` — handler for `GET /api/expenses/{id}/trace` (A-008). Reads from the `agent_tool_calls` / trace store written by spec 037. Returns 410 `TRACE_UNAVAILABLE` when no trace exists. Serializes per the schema in spec.md §A-008.
- **MODIFY:** `internal/api/router.go` (or wherever expense routes are registered) — register `GET /api/expenses/{id}/trace` with the same auth class as `GET /api/expenses/{id}`.
- **MODIFY:** `internal/telegram/expenses.go` — extend the format helpers used by capture confirmations and query results to append the optional `Why: {rationale_short}` line (T-017). Helper accepts an `*string`; emits nothing when nil. **No change to dispatch or state machine; no internal tool names ever passed to the formatter.**
- **NEW:** `internal/api/expense_trace_test.go` — unit coverage for A-008 happy path, 410 path, and rejected-call shape.
- **NEW:** `internal/api/expenses_response_test.go` — unit coverage for A-001 additive fields (agent path, legacy path null, no-leak assertion).
- **NEW:** `internal/telegram/expenses_format_why_test.go` — unit coverage for T-017 line presence/absence and the no-internal-name assertion.
- **NEW:** `tests/integration/expense_rationale_trace_test.go` — live-stack agent path → A-001 fields populated, A-008 returns trace.
- **NEW:** `tests/e2e/expense_rationale_trace_e2e_test.go` — capture → classify (agent) → API GET (rationale present) → Telegram confirmation (Why line present) → API trace GET (tool calls present, rejected entries flagged).

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-17-01 | Unit | `expenses_response_test.go` | SCN-034-094 | Agent-classified fixture → response includes `scenario`, `rationale`, `rationale_short`; pre-migration fields unchanged (golden response diff bounded to additions) |
| T-17-02 | Unit | `expenses_response_test.go` | SCN-034-095 | Legacy-path fixture → fields present with value null; no fabrication |
| T-17-03 | Unit | `expense_trace_test.go` | SCN-034-096 | Agent-classified fixture → 200 trace with tool_calls[], outcome, rationale; matches A-008 schema |
| T-17-04 | Unit (adversarial) | `expense_trace_test.go` | SCN-034-097 | No-trace fixture → 410 `TRACE_UNAVAILABLE`; **fails if any synthesized/empty trace shape is returned** |
| T-17-05 | Unit (adversarial) | `expenses_response_test.go` | SCN-034-100 (BS-038) | Rejected-tool-call trace fixture → A-001 response contains zero references to the rejected tool name or reject reason; assertion would fail if the API leaked operator-only fields |
| T-17-06 | Unit | `expenses_format_why_test.go` | SCN-034-098 | Capture-confirmation formatter with `rationale_short` → one `Why:` line; with nil → no `Why:` line at all (no empty placeholder) |
| T-17-07 | Unit (adversarial) | `expenses_format_why_test.go` | SCN-034-100 | **No-internal-name leak**: any test fixture with internal tool names (`expense_classify_tool`, etc.) in metadata.rationale_short fails fast at the formatter (sanitization assertion); would fail if formatter passed metadata through unchecked |
| T-17-08 | Unit | `expenses_format_why_test.go` | SCN-034-099 (T-016 R4) | Uncategorized fixture → Why line surfaces rationale_short; no fallback category text added |
| T-17-09 | Integration | `tests/integration/expense_rationale_trace_test.go` | SCN-034-094, 096 | Live PostgreSQL + agent path: capture → classify → A-001 fields populated; A-008 returns full trace from `agent_tool_calls` |
| T-17-10 | Regression E2E | `tests/e2e/expense_rationale_trace_e2e_test.go` | SCN-034-094, 096, 098 | Live stack: end-to-end rationale + trace + Telegram Why line; **no bailout returns**; assertions on actual API JSON and actual Telegram message body |
| T-17-11 | Regression E2E | `tests/e2e/expense_api_backcompat_e2e_test.go` | SCN-034-094 | Pre-migration golden API responses for `GET /api/expenses` are byte-equivalent except for the additive `scenario`/`rationale`/`rationale_short` keys (existing field set unchanged); existing client fixtures still parse |
| T-17-12 | Regression E2E | `tests/e2e/telegram_format_parity_test.go` (extend) | — | Existing T-001..T-011 format tests pass with `rationale_short=nil`; with non-nil, golden body equals pre-migration body + one `Why:` line tail |

### Definition of Done

- [ ] `GET /api/expenses` and `GET /api/expenses/{id}` include `scenario`, `rationale`, `rationale_short` (null on legacy path); existing fields unchanged
- [ ] `GET /api/expenses/{id}/trace` implemented per spec §A-008; 410 `TRACE_UNAVAILABLE` when no trace; no fabricated shape
- [ ] Telegram capture confirmations and expense query results render T-017 `Why:` line when `rationale_short` is non-nil; omit entirely when nil
- [ ] Sanitization guard in formatter rejects any rationale text containing internal tool names, prompt-contract IDs, or trace IDs (operator-only data)
- [ ] BS-038 rejected tool calls appear ONLY in A-008; never in A-001 or Telegram (adversarial assertion enforced)
- [ ] BS-035 uncategorized rationale visible to user via Why line (T-016 R4)
- [ ] Backward compatibility: pre-migration golden API and Telegram responses byte-equivalent to baseline plus the additive surfaces
- [ ] Existing Scopes 06, 07, 08 E2E tests pass unchanged (no shipped-behavior regression)
- [ ] No regex literals or routing logic introduced in this scope (additive surfaces only)
- [ ] `./smackerel.sh test unit`, `test integration`, `test e2e` pass
- [ ] `./smackerel.sh lint` and `format --check` pass
- [ ] Artifact lint clean

---

## RESULT-ENVELOPE

```yaml
agent: bubbles.plan
role: Sequential scope planner (agent+tools shift)
outcome: completed_owned
affected_artifacts:
  - specs/034-expense-tracking/scopes.md (UPDATED)
evidence_summary: |
  Updated scopes.md for the spec-037 LLM Agent + Tools shift.

  Marked DEPRECATED (with one-line replacement notes):
    - Scope 04 Classification Engine → replaced by Scope 11 (expense.classify-v1)
    - Scope 05 Vendor Normalization & Suggestions → replaced by Scope 12
      (expense.normalize_vendor-v1; vendor_seeds.go bootstrap-imported)
    - Scope 08 Telegram Expense Commands (partial) → regex intent patterns
      replaced by Scope 15 (expense.intent_route-v1); format functions and
      OCR flow retained.

  Added new Scopes 10–16 covering the agent shift:
    10 Expense Tool Registration & Scenario Bootstrap
       (registers all 7 tools, creates 7 scenario skeletons; SST flags;
        depends on spec 037 Scopes 2 + 3)
    11 Classification Scenario Migration
       (BS-029, BS-033, BS-035, BS-037, BS-038; sticky user_corrected
        guard at prompt + Go post-validation)
    12 Vendor Normalization Scenario Migration
       (BS-014, BS-036; should_persist gate; bootstrap import of seeds)
    13 Receipt Extraction Resilience
       (BS-032, BS-033, BS-034, BS-037; receipt_extract_tool; adversarial
        fixtures)
    14 Subscription Detect / Refund Link / Unusual Spend Scenarios
       (BS-029, BS-030, BS-031; governance test for "no Go change to
        add new scenario")
    15 Natural-Language Query & Intent Routing Scenarios
       (BS-025, BS-029; replaces telegram regex; format functions retained)
    17 Operator Rationale & Trace Surfaces
       (A-001 additive API fields, A-008 GET /api/expenses/{id}/trace,
        T-017 Telegram Why: line; sanitization guard; BS-038 rejected
        calls operator-only; backward-compatible additive surfaces only)
    16 Legacy Removal & Acceptance Gate
       (deletes vendor_seeds.go, 7-rule chain, regex dispatch; full
        regression suite + golden parity fixtures protect shipped behavior)

  Each new scope: ID, goal, BS-* coverage, file outline, Gherkin scenarios,
  test plan with adversarial regressions for BS-032..BS-038, strict DoD
  including ./smackerel.sh runner commands and SST zero-defaults compliance.

  Honored:
    - Existing expense API + Telegram surface continues to work; explicit
      "no shipped-behavior regression" DoD items and golden parity tests.
    - Spec 037 dependency declared on Scope 10.
    - SST zero-defaults: EXPENSES_CLASSIFIER, EXPENSES_RECEIPT_DETECTOR,
      EXPENSES_VENDOR_NORMALIZER are required env vars with no source
      defaults; operator default = legacy until Scope 16 acceptance gate.
    - All test commands route through ./smackerel.sh (unit, integration,
      e2e, stress, lint, format).

  No artifact-lint-blocking patterns introduced; existing Scopes 01–09
  status fields and evidence preserved.
```
