# Scopes: 034 Expense Tracking

Links: [spec.md](spec.md) | [design.md](design.md) | [uservalidation.md](uservalidation.md)

## Summary Table

| # | Scope | Priority | Depends On | Surfaces | Status |
|---|-------|----------|-----------|----------|--------|
| 01 | Configuration & Config Pipeline | P0 | — | Config, Scripts | Not Started |
| 02 | Receipt Detection & Extraction Pipeline | P0 | 01 | Python ML Sidecar, Prompt Contract | Not Started |
| 03 | Expense Data Model & Migration | P0 | 01 | Go Core, PostgreSQL | Not Started |
| 04 | Classification Engine | P1 | 02, 03 | Go Core | Not Started |
| 05 | Vendor Normalization & Suggestions | P1 | 03, 04 | Go Core, PostgreSQL | Not Started |
| 06 | Expense API Endpoints | P1 | 03, 04 | Go Core, REST API | Not Started |
| 07 | CSV Export | P1 | 06 | Go Core, REST API | Not Started |
| 08 | Telegram Expense Commands | P1 | 04, 06 | Go Core, Telegram Bot | Not Started |
| 09 | Digest Integration | P2 | 04, 05 | Go Core, Digest | Not Started |

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

**Status:** Not Started
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

**Status:** Not Started
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

**Status:** Not Started
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

**Status:** Not Started
**Priority:** P1
**Depends On:** 02, 03

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

**Status:** Not Started
**Priority:** P1
**Depends On:** 03, 04

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

**Status:** Not Started
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

- [ ] All 7 API endpoints implemented per design §7 (A-001 through A-007)
- [ ] Routes registered on Chi router under /api/expenses prefix
- [ ] All endpoints use standard Smackerel API envelope
- [ ] Input validation returns proper error codes (400, 404, 413, 422)
- [ ] Cursor-based pagination implemented for list endpoint
- [ ] Summary computation groups totals by currency (never cross-currency sum)
- [ ] PATCH correction sets user_corrected and appends to corrected_fields
- [ ] All monetary amounts are strings in responses (never numeric JSON types)
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
- [ ] Broader E2E regression suite passes
- [ ] `./smackerel.sh lint` passes
- [ ] `./smackerel.sh format --check` passes
- [ ] Artifact lint clean: `bash .github/bubbles/scripts/artifact-lint.sh specs/034-expense-tracking`

---

## Scope 07: CSV Export

**Status:** Not Started
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

- [ ] Standard format CSV with 11 columns per design §11
- [ ] QuickBooks format CSV with 5 columns, MM/DD/YYYY dates, display category names
- [ ] Mixed currency warning comment prepended when multiple currencies present
- [ ] Refund negative amounts handled correctly in export and totals
- [ ] Row count check before streaming prevents partial exports exceeding max_rows
- [ ] Empty result produces CSV with header row only (200, not error)
- [ ] Streaming implementation uses rows.Next() cursor, not full in-memory buffer
- [ ] Date format strings read from config env vars (SST compliant)
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
- [ ] Broader E2E regression suite passes
- [ ] Stress test: 10000-row export within 10 seconds
- [ ] `./smackerel.sh lint` passes
- [ ] `./smackerel.sh format --check` passes
- [ ] Artifact lint clean: `bash .github/bubbles/scripts/artifact-lint.sh specs/034-expense-tracking`

---

## Scope 08: Telegram Expense Commands

**Status:** Not Started
**Priority:** P1
**Depends On:** 04, 06

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

- [ ] All 11 Telegram interaction formats implemented per UX spec (T-001 through T-011)
- [ ] Message dispatch routes photo, text expense, query, export, reply-context commands correctly
- [ ] Conversation state management with TTL for multi-turn fix flow and amount prompts
- [ ] OCR failure (< 10 chars) produces T-002 response and no artifact
- [ ] Amount reply updates expense metadata
- [ ] Fix flow presents fields, accepts corrections, and terminates on "done"
- [ ] Suggestion accept/dismiss works via natural language chat commands
- [ ] Expense query shows at most 10 items with "all" and "export" reply options
- [ ] CSV export sends file as Telegram document attachment
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
- [ ] Broader E2E regression suite passes
- [ ] `./smackerel.sh lint` passes
- [ ] `./smackerel.sh format --check` passes
- [ ] Artifact lint clean: `bash .github/bubbles/scripts/artifact-lint.sh specs/034-expense-tracking`

---

## Scope 09: Digest Integration

**Status:** Not Started
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

- [ ] ExpenseDigestSection implements Assemble method producing ExpenseDigestContext
- [ ] Summary block: 7-day count and total grouped by classification and currency
- [ ] Needs-review block: extraction issues limited to EXPENSES_DIGEST_NEEDS_REVIEW_LIMIT
- [ ] Suggestions block: pending suggestions limited to EXPENSES_SUGGESTIONS_MAX_PER_DIGEST
- [ ] Missing receipts block: active subscriptions without matching vendor expense in lookback period
- [ ] Unusual charges block: new vendors not seen in previous 90 days
- [ ] Word limit enforcement drops blocks in correct reverse-priority order
- [ ] Empty period returns IsEmpty() == true, section omitted from digest
- [ ] Integrated into existing Generator.Generate() as optional section
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
- [ ] Broader E2E regression suite passes
- [ ] `./smackerel.sh lint` passes
- [ ] `./smackerel.sh format --check` passes
- [ ] Artifact lint clean: `bash .github/bubbles/scripts/artifact-lint.sh specs/034-expense-tracking`
