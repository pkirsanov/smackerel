# Design: 034 Expense Tracking

## 1. Overview

Expense tracking layers structured financial extraction, classification, and export onto the existing Smackerel ingestion pipeline. No new services, containers, or message queues are introduced. Receipts and invoices arriving through any ingestion channel (Gmail, Telegram photo, web capture, PDF, manual text) pass through a fast heuristic filter, then an LLM-powered receipt extraction prompt contract. Extracted expense metadata is stored in the existing `artifacts.metadata` JSONB field. Two new intelligence tables (`vendor_aliases` and `expense_suggestions`) support vendor normalization and business classification suggestions. A new expense section producer feeds the daily digest. Seven REST API endpoints expose query, export, correction, classification, and suggestion management. The Telegram bot extends its existing command handling for expense interactions.

### Guiding Principles

- **No new infrastructure.** Everything runs inside the existing Go core, Python ML sidecar, PostgreSQL, and NATS JetStream topology.
- **Metadata-first.** Expense data is artifact metadata, not a separate domain model. An expense is an artifact whose `metadata` contains a `expense` key.
- **Amounts as strings.** All monetary values stored and transported as decimal strings (`"147.30"`). No float arithmetic for display or aggregation — PostgreSQL `CAST(metadata->>'amount' AS NUMERIC)` handles summation.
- **SST compliance.** Every configurable value originates from `config/smackerel.yaml`. Source code reads from environment variables produced by `./smackerel.sh config generate`.
- **Corrections are sticky.** Fields marked `user_corrected` survive re-extraction. The system never overwrites a user correction.

---

## 2. Architecture

### Data Flow

```
┌──────────────────────────────────────────────────────────────────────────┐
│                        Ingestion Sources                                 │
│  Gmail connector ─┐                                                      │
│  Telegram photo  ─┤                                                      │
│  Web capture API ─┼─▶ artifacts.process (NATS) ─▶ ML Sidecar            │
│  Manual text     ─┤                                                      │
│  PDF import      ─┘                                                      │
└──────────────────────┬───────────────────────────────────────────────────┘
                       │
                       ▼
┌──────────────────────────────────────────────────────────────────────────┐
│  ML Sidecar: artifacts.process handler                                   │
│                                                                          │
│  1. Standard extraction (ingest-synthesis-v1) runs first                 │
│  2. Receipt Detection Heuristics (fast, pre-LLM):                        │
│     • billing keywords + amount pattern + vendor-header pattern          │
│     • If heuristic fires → route through receipt-extraction-v1           │
│  3. receipt-extraction-v1 prompt contract:                               │
│     • LLM extracts vendor, date, amount, currency, tax, line items      │
│     • Output validated against JSON Schema                               │
│  4. Result published to artifacts.processed (NATS)                       │
└──────────────────────┬───────────────────────────────────────────────────┘
                       │
                       ▼
┌──────────────────────────────────────────────────────────────────────────┐
│  Go Core: artifacts.processed handler                                    │
│                                                                          │
│  1. Store extraction result in artifacts.metadata.expense                 │
│  2. Classification Engine applies rule chain:                            │
│     a. Gmail label match → mapped classification                         │
│     b. Source-level rules (Telegram caption context)                      │
│     c. Vendor match against business_vendors config list                  │
│     d. LLM-extracted category fallback                                   │
│  3. Vendor normalization via vendor_aliases table                         │
│  4. Write final metadata to PostgreSQL                                   │
└──────────────────────┬───────────────────────────────────────────────────┘
                       │
          ┌────────────┼────────────┐
          ▼            ▼            ▼
   ┌───────────┐ ┌──────────┐ ┌──────────┐
   │ REST API  │ │ Telegram │ │  Digest  │
   │ 7 endpts  │ │ Bot cmds │ │ Section  │
   └───────────┘ └──────────┘ └──────────┘
```

### Component Ownership

| Component | Package | Language | Responsibility |
|-----------|---------|----------|---------------|
| Receipt heuristic filter | `ml/app/synthesis.py` | Python | Pre-LLM receipt detection within existing synthesis handler |
| Receipt extraction contract | `config/prompt_contracts/receipt-extraction-v1.yaml` | YAML | LLM prompt + output schema for receipt/invoice extraction |
| Classification engine | `internal/intelligence/expenses.go` | Go | Rule-based classification, vendor normalization, suggestion generation |
| Expense API handlers | `internal/api/expenses.go` | Go | REST endpoints for query, export, correction, classification, suggestions |
| Expense digest producer | `internal/digest/expenses.go` | Go | Digest section assembly: summary, review items, suggestions, missing receipts |
| Telegram expense commands | `internal/telegram/expenses.go` | Go | Chat handlers for expense queries, corrections, suggestion accept/dismiss |
| Vendor aliases store | `internal/intelligence/expenses.go` | Go | CRUD for vendor_aliases table, normalization lookup |
| CSV export engine | `internal/api/expenses.go` | Go | Standard and QuickBooks format CSV generation, streamed response |

---

## 3. Receipt Detection Heuristics

The heuristic filter runs in the ML sidecar's `artifacts.process` handler, **after** standard content extraction but **before** engaging the receipt extraction LLM call. Its purpose is to avoid expensive LLM calls on non-receipt content.

### Location

Added to `ml/app/synthesis.py` as a function `detect_receipt_content(text: str, content_type: str, source_id: str) -> bool` called within the existing `handle_synthesis_request` flow.

### Heuristic Rules (all case-insensitive)

An artifact is considered receipt-likely if **any** of these conditions hold:

| Rule | Pattern | Rationale |
|------|---------|-----------|
| H-001 | Content type is `bill` | Already classified by the pipeline |
| H-002 | `amount_pattern` matches AND at least one `billing_keyword` present | Existing patterns from `subscriptions.go` reused in Python |
| H-003 | Content contains `receipt` or `invoice` in title or first 500 chars AND `amount_pattern` matches | Explicit receipt language + amount |
| H-004 | Source is `telegram` AND content came from OCR (image capture) | User intent: photographing a receipt is the primary use case for OCR captures |
| H-005 | Email sender domain matches known receipt sender patterns (e.g., `receipts@`, `billing@`, `invoice@`, `noreply@` with receipt keywords in subject) | High-confidence email receipt detection |

### Amount Pattern (Python port)

```python
import re
AMOUNT_PATTERN = re.compile(
    r'(?:'
    r'\$\s*\d+\.?\d*'           # $9.99
    r'|\d+\.?\d*\s*(?:USD|EUR|GBP|CAD|AUD)'  # 9.99 USD
    r'|(?:USD|EUR|GBP|CAD|AUD)\s*\d+\.?\d*'  # USD 9.99
    r'|€\s*\d+[.,]?\d*'        # €47,50
    r'|£\s*\d+\.?\d*'          # £25.00
    r')',
    re.IGNORECASE
)

BILLING_KEYWORDS = [
    "charge", "receipt", "billing", "subscription", "monthly",
    "annual", "renewal", "payment", "invoice", "order",
    "total", "subtotal", "tax", "tip", "amount due",
    "transaction", "purchase",
]
```

### Behavior

- If the heuristic returns `True`, the artifact text is additionally passed through the `receipt-extraction-v1` prompt contract.
- The standard synthesis extraction (`ingest-synthesis-v1`) still runs regardless — the receipt extraction is **additive**, not a replacement.
- If the heuristic returns `False`, no receipt extraction is attempted. No LLM cost incurred for non-receipt content.
- The heuristic result is included in the `artifacts.processed` NATS response as `"receipt_detected": true/false` so the Go core knows whether to run classification.

---

## 4. Receipt Extraction Prompt Contract

File: `config/prompt_contracts/receipt-extraction-v1.yaml`

```yaml
version: "receipt-extraction-v1"
type: "domain-extraction"
description: "Extract structured expense/receipt data from receipt text, invoice text, or OCR output"

content_types:
  - "email"
  - "bill"
  - "note"
  - "media"

min_content_length: 20

system_prompt: |
  You are a receipt and invoice extraction engine. Extract structured expense
  data from the provided content. The content may be OCR output (noisy), email
  text, PDF-extracted text, or user-typed natural language.

  Return ONLY valid JSON matching the output schema below.

  RULES:
  - Extract the vendor/merchant name. Normalize obvious abbreviations
    (e.g., "AMZN MKTP" → keep raw, let the application normalize).
  - Extract the total amount with currency. If currency is ambiguous, default
    to "USD".
  - Parse date in ISO 8601 (YYYY-MM-DD). If no date, use null.
  - Extract tax, tip, and subtotal separately if present.
  - Extract line items if visible. Do NOT hallucinate line items from a total.
  - For amounts, return string representations with exactly two decimal places
    (e.g., "147.30", not "147.3" or 147.30).
  - If the content is a refund, the amount should be negative (e.g., "-29.99").
  - If a field cannot be determined, use null (not empty string or zero).
  - Preserve the raw vendor text in vendor_raw even when you can identify
    the canonical name.
  - For international receipts with comma decimals (e.g., "47,50"), normalize
    to dot decimal ("47.50") in the amount field and preserve original in
    raw_amount.

extraction_schema:
  type: object
  required:
    - domain
    - vendor_raw
  properties:
    domain:
      type: string
      const: "expense"
    vendor:
      type: string
      description: "Cleaned/canonical vendor name if recognizable"
    vendor_raw:
      type: string
      description: "Original vendor text as extracted"
    date:
      type: string
      format: date
      description: "Transaction date in YYYY-MM-DD"
    amount:
      type: string
      pattern: "^-?\\d+\\.\\d{2}$"
      description: "Total amount as decimal string"
    raw_amount:
      type: string
      description: "Original amount text before normalization"
    currency:
      type: string
      pattern: "^[A-Z]{3}$"
      description: "ISO 4217 currency code"
    subtotal:
      type: string
      pattern: "^-?\\d+\\.\\d{2}$"
    tax:
      type: string
      pattern: "^-?\\d+\\.\\d{2}$"
    tip:
      type: string
      pattern: "^-?\\d+\\.\\d{2}$"
    payment_method:
      type: string
      description: "e.g., visa-ending-4242, cash, paypal"
    category:
      type: string
      description: "Expense category slug"
    line_items:
      type: array
      items:
        type: object
        required:
          - description
        properties:
          description:
            type: string
          amount:
            type: string
            pattern: "^-?\\d+\\.\\d{2}$"
          quantity:
            type: string
    notes:
      type: string
      description: "Any additional context extracted from the content"
```

### Integration with Existing Synthesis Pipeline

The receipt extraction runs as a second pass within `ml/app/synthesis.py`:

1. The existing `handle_synthesis_request` processes the artifact through `ingest-synthesis-v1`.
2. If `detect_receipt_content()` returns `True`, the content is additionally passed through `receipt-extraction-v1` via `load_prompt_contract("receipt-extraction-v1")`.
3. The extraction result is validated against the schema using the existing `validate_extraction()` function.
4. The validated result is included in the `artifacts.processed` NATS response under a new `expense_extraction` key alongside the existing `extraction` key.
5. If extraction fails validation, `expense_extraction` is set to `{"extraction_failed": true, "error": "<reason>"}`.

---

## 5. Data Model

### Expense Metadata Schema (within `artifacts.metadata` JSONB)

Every artifact identified as expense-bearing gets an `expense` key in its metadata:

```json
{
  "expense": {
    "vendor": "Corner Coffee",
    "vendor_raw": "SQ *CORNER COFFEE",
    "date": "2026-04-03",
    "amount": "4.75",
    "raw_amount": null,
    "currency": "USD",
    "subtotal": null,
    "tax": null,
    "tip": null,
    "payment_method": null,
    "category": "food-and-drink",
    "classification": "business",
    "line_items": [],
    "notes": null,
    "extraction_status": "complete",
    "extraction_partial": false,
    "amount_missing": false,
    "user_corrected": false,
    "corrected_fields": [],
    "source_qualifiers": ["Business-Receipts"]
  }
}
```

| Field | Type | Description | Nullable |
|-------|------|-------------|----------|
| `vendor` | string | Normalized vendor name | No (default: `"Unknown"`) |
| `vendor_raw` | string | Original extracted vendor text | No |
| `date` | string (YYYY-MM-DD) | Transaction date | Yes |
| `amount` | string (decimal) | Total amount including tax and tip | Yes (if `amount_missing: true`) |
| `raw_amount` | string | Original amount text before normalization | Yes |
| `currency` | string (ISO 4217) | Currency code | No (default: `"USD"`) |
| `subtotal` | string (decimal) | Pre-tax, pre-tip amount | Yes |
| `tax` | string (decimal) | Tax amount | Yes |
| `tip` | string (decimal) | Tip/gratuity amount | Yes |
| `payment_method` | string | Payment method identifier | Yes |
| `category` | string | Expense category slug | Yes (default: `"uncategorized"`) |
| `classification` | string | `business`, `personal`, or `uncategorized` | No (default: `"uncategorized"`) |
| `line_items` | array | Line item objects with description, amount, quantity | No (default: `[]`) |
| `notes` | string | User or extracted notes | Yes |
| `extraction_status` | string | `complete`, `partial`, `failed` | No |
| `extraction_partial` | bool | True when some fields could not be extracted | No |
| `amount_missing` | bool | True when amount was not extractable | No |
| `user_corrected` | bool | True when any field has been manually corrected | No |
| `corrected_fields` | array of strings | Which fields the user has corrected | No (default: `[]`) |
| `source_qualifiers` | array of strings | Gmail labels or source-level qualifiers | No (default: `[]`) |

### Querying Expense Artifacts

Expense artifacts are identified by the presence of the `expense` key in metadata:

```sql
-- Find all expense artifacts
SELECT id, title, metadata->'expense' AS expense
FROM artifacts
WHERE metadata ? 'expense'
ORDER BY (metadata->'expense'->>'date')::date DESC;

-- Aggregate by currency (amounts as strings, cast for summation)
SELECT
  metadata->'expense'->>'currency' AS currency,
  COUNT(*) AS count,
  SUM(CAST(metadata->'expense'->>'amount' AS NUMERIC)) AS total
FROM artifacts
WHERE metadata ? 'expense'
  AND metadata->'expense'->>'classification' = 'business'
  AND (metadata->'expense'->>'date')::date BETWEEN '2026-04-01' AND '2026-04-30'
GROUP BY metadata->'expense'->>'currency';
```

### PostgreSQL Indexes

Add a GIN index on the `expense` metadata key for efficient filtering:

```sql
-- Partial GIN index: only artifacts with expense metadata
CREATE INDEX idx_artifacts_expense_metadata
  ON artifacts USING gin ((metadata->'expense'))
  WHERE metadata ? 'expense';

-- B-tree index on expense date for range queries
CREATE INDEX idx_artifacts_expense_date
  ON artifacts (((metadata->'expense'->>'date')::date))
  WHERE metadata ? 'expense'
  AND metadata->'expense'->>'date' IS NOT NULL;

-- B-tree index on classification for filtered queries
CREATE INDEX idx_artifacts_expense_classification
  ON artifacts ((metadata->'expense'->>'classification'))
  WHERE metadata ? 'expense';
```

### Vendor Aliases Table

```sql
CREATE TABLE vendor_aliases (
  id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
  alias       TEXT NOT NULL,
  canonical   TEXT NOT NULL,
  source      TEXT NOT NULL DEFAULT 'system',  -- 'system' or 'user'
  created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(alias)
);

CREATE INDEX idx_vendor_aliases_alias ON vendor_aliases (LOWER(alias));
```

- `alias`: the raw vendor text (e.g., `"AMZN MKTP US"`)
- `canonical`: the normalized name (e.g., `"Amazon"`)
- `source`: `"system"` for pre-seeded entries, `"user"` for user corrections that create new aliases

When an expense is stored, the Go core looks up `vendor_raw` in `vendor_aliases` (case-insensitive). If a match is found, `vendor` is set to the canonical name. Otherwise, `vendor` is set to the LLM-extracted vendor name.

User corrections to the `vendor` field on an expense also create a vendor alias entry (`source = 'user'`) mapping the `vendor_raw` to the user's corrected value.

### Expense Suggestions Table

```sql
CREATE TABLE expense_suggestions (
  id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
  artifact_id     TEXT NOT NULL REFERENCES artifacts(id),
  vendor          TEXT NOT NULL,
  suggested_class TEXT NOT NULL DEFAULT 'business',
  confidence      REAL NOT NULL,
  evidence        TEXT NOT NULL,
  status          TEXT NOT NULL DEFAULT 'pending',  -- 'pending', 'accepted', 'dismissed'
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  resolved_at     TIMESTAMPTZ,
  UNIQUE(artifact_id, suggested_class)
);

CREATE INDEX idx_expense_suggestions_status ON expense_suggestions (status) WHERE status = 'pending';
```

### Suppressed Suggestions Table

```sql
CREATE TABLE expense_suggestion_suppressions (
  id             TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
  vendor         TEXT NOT NULL,
  classification TEXT NOT NULL,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(vendor, classification)
);
```

When a user dismisses a suggestion, a row is inserted here. Future suggestion generation skips vendor+classification pairs present in this table.

---

## 6. Configuration

### Additions to `config/smackerel.yaml`

#### Under `connectors.imap` — Expense Label Mapping

```yaml
connectors:
  imap:
    sync_schedule: "*/15 * * * *"
    expense_labels:
      # Map Gmail labels to expense classifications.
      # Keys are Gmail label names (case-sensitive match against Gmail API).
      # Values are classification strings: "business", "personal".
      # Labels not listed here have no effect on expense classification.
      # Example:
      # Business-Receipts: business
      # Tax-Deductible: business
      # Personal-Purchases: personal
```

#### New Top-Level `expenses` Section

```yaml
expenses:
  enabled: true

  # Default currency for manual entries and when extraction cannot determine currency.
  # Must be ISO 4217.
  default_currency: "USD"

  # Expense categories available for classification.
  # Slugs used in metadata; display names used in exports and Telegram responses.
  categories:
    - slug: "food-and-drink"
      display: "Food & Drink"
      tax_category: "Meals"
    - slug: "transportation"
      display: "Transportation"
      tax_category: "Car and Truck Expenses"
    - slug: "office-supplies"
      display: "Office Supplies"
      tax_category: "Office Expenses"
    - slug: "technology"
      display: "Technology"
      tax_category: "Other Expenses"
    - slug: "travel"
      display: "Travel"
      tax_category: "Travel"
    - slug: "utilities"
      display: "Utilities"
      tax_category: "Utilities"
    - slug: "home-improvement"
      display: "Home Improvement"
      tax_category: "Repairs and Maintenance"
    - slug: "auto-and-transport"
      display: "Auto & Transport"
      tax_category: "Car and Truck Expenses"
    - slug: "subscriptions"
      display: "Subscriptions"
      tax_category: "Other Expenses"
    - slug: "professional-services"
      display: "Professional Services"
      tax_category: "Legal and Professional Services"
    - slug: "insurance"
      display: "Insurance"
      tax_category: "Insurance"
    - slug: "advertising"
      display: "Advertising"
      tax_category: "Advertising"
    - slug: "education"
      display: "Education"
      tax_category: "Other Expenses"
    - slug: "health"
      display: "Health"
      tax_category: "Other Expenses"
    - slug: "entertainment"
      display: "Entertainment"
      tax_category: "Other Expenses"
    - slug: "other"
      display: "Other"
      tax_category: "Other Expenses"

  # Vendors that are always classified as business.
  # Vendor name matching is case-insensitive and uses the normalized vendor name.
  business_vendors: []
  # Example:
  # - "WeWork"
  # - "Zoom"
  # - "DigitalOcean"

  # Export settings
  export:
    max_rows: 10000
    quickbooks_date_format: "01/02/2006"  # Go time format for MM/DD/YYYY
    standard_date_format: "2006-01-02"    # Go time format for YYYY-MM-DD

  # Suggestion engine settings
  suggestions:
    min_confidence: 0.6
    min_past_business_count: 2
    max_per_digest: 3
    reclassify_batch_limit: 100

  # Vendor normalization cache
  vendor_cache_size: 500

  # Digest section settings
  digest:
    max_words: 100
    needs_review_limit: 5
    missing_receipt_lookback_days: 35
```

### Config Generation Pipeline

The `./smackerel.sh config generate` command must be extended to:

1. Read the new `expenses` and `connectors.imap.expense_labels` sections.
2. Emit the following environment variables into `config/generated/dev.env` and `config/generated/test.env`:

```
EXPENSES_ENABLED=true
EXPENSES_DEFAULT_CURRENCY=USD
EXPENSES_EXPORT_MAX_ROWS=10000
EXPENSES_EXPORT_QB_DATE_FORMAT=01/02/2006
EXPENSES_EXPORT_STD_DATE_FORMAT=2006-01-02
EXPENSES_SUGGESTIONS_MIN_CONFIDENCE=0.6
EXPENSES_SUGGESTIONS_MIN_PAST_BUSINESS=2
EXPENSES_SUGGESTIONS_MAX_PER_DIGEST=3
EXPENSES_SUGGESTIONS_RECLASSIFY_BATCH_LIMIT=100
EXPENSES_VENDOR_CACHE_SIZE=500
EXPENSES_DIGEST_MAX_WORDS=100
EXPENSES_DIGEST_NEEDS_REVIEW_LIMIT=5
EXPENSES_DIGEST_MISSING_RECEIPT_LOOKBACK_DAYS=35
IMAP_EXPENSE_LABELS=<JSON-encoded map>
EXPENSES_BUSINESS_VENDORS=<JSON-encoded array>
EXPENSES_CATEGORIES=<JSON-encoded array>
```

### SST Enforcement

| Value | SST Location | Consumer | Forbidden Pattern |
|-------|-------------|----------|-------------------|
| `default_currency` | `expenses.default_currency` | Go: `os.Getenv("EXPENSES_DEFAULT_CURRENCY")` | `currency := "USD"` in source |
| `max_rows` | `expenses.export.max_rows` | Go: `os.Getenv("EXPENSES_EXPORT_MAX_ROWS")` | `const maxRows = 10000` in source |
| `expense_labels` | `connectors.imap.expense_labels` | Go: `os.Getenv("IMAP_EXPENSE_LABELS")` | Hardcoded label-to-class map |
| `business_vendors` | `expenses.business_vendors` | Go: `os.Getenv("EXPENSES_BUSINESS_VENDORS")` | Hardcoded vendor list |
| `categories` | `expenses.categories` | Go: `os.Getenv("EXPENSES_CATEGORIES")` | Hardcoded category list |
| `min_confidence` | `expenses.suggestions.min_confidence` | Go: `os.Getenv("EXPENSES_SUGGESTIONS_MIN_CONFIDENCE")` | `if confidence > 0.6` in source |

All consumers must fail-loud if the environment variable is missing:

```go
// REQUIRED: fail-loud
maxRows := os.Getenv("EXPENSES_EXPORT_MAX_ROWS")
if maxRows == "" {
    log.Fatal("EXPENSES_EXPORT_MAX_ROWS is required")
}
```

---

## 7. API Endpoints

All endpoints live in `internal/api/expenses.go` and are registered on the existing Chi router under the `/api/expenses` prefix. Responses use the standard Smackerel API envelope (`{"ok": true, "data": {...}, "meta": {...}}`).

### Route Registration

```go
// In cmd/core/main.go or the router setup function:
r.Route("/api/expenses", func(r chi.Router) {
    r.Get("/", expenseHandler.List)          // A-001
    r.Get("/export", expenseHandler.Export)   // A-002
    r.Get("/{id}", expenseHandler.Get)        // A-004
    r.Patch("/{id}", expenseHandler.Correct)  // A-003
    r.Post("/{id}/classify", expenseHandler.Classify) // A-005
    r.Post("/suggestions/{id}/accept", expenseHandler.AcceptSuggestion)  // A-006
    r.Post("/suggestions/{id}/dismiss", expenseHandler.DismissSuggestion) // A-007
})
```

### Handler Struct

```go
type ExpenseHandler struct {
    Pool           *pgxpool.Pool
    ClassifyEngine *intelligence.ExpenseClassifier
}
```

### A-001: GET /api/expenses — List and Filter

**Query building:** Constructs a parameterized SQL query against `artifacts` where `metadata ? 'expense'`, applying filters from query parameters:

- `from`/`to` → `(metadata->'expense'->>'date')::date BETWEEN $N AND $M`
- `classification` → `metadata->'expense'->>'classification' = $N`
- `category` → `metadata->'expense'->>'category' = $N`
- `vendor` → `LOWER(metadata->'expense'->>'vendor') LIKE '%' || LOWER($N) || '%'`
- `amount_min`/`amount_max` → `CAST(metadata->'expense'->>'amount' AS NUMERIC) >= $N`
- `currency` → `metadata->'expense'->>'currency' = $N`
- `needs_review` → `metadata->'expense'->>'extraction_status' != 'complete' OR metadata->'expense'->>'amount_missing' = 'true'`
- Cursor-based pagination using `(date, id)` composite cursor, consistent with existing `ExportHandler` pattern.

**Summary computation:** A second query (or CTE) computes `count` and `total_by_currency`:

```sql
SELECT
  metadata->'expense'->>'currency' AS currency,
  COUNT(*) AS count,
  SUM(CAST(metadata->'expense'->>'amount' AS NUMERIC))::text AS total
FROM artifacts
WHERE metadata ? 'expense'
  AND <same filters>
GROUP BY metadata->'expense'->>'currency';
```

**Validation:** Input validation for date ranges, currency codes (3 uppercase letters), amount ranges. Returns `400` with specific error codes on failure.

### A-002: GET /api/expenses/export — CSV Export

**Implementation:**

1. Execute the same filter query as A-001 (without cursor/limit, up to `EXPENSES_EXPORT_MAX_ROWS`).
2. If result count exceeds `max_rows`, return `413 EXPORT_TOO_LARGE`.
3. Set `Content-Type: text/csv` and `Content-Disposition` header with filename pattern `smackerel-expenses-{classification}-{YYYY-MM}.csv`.
4. Stream CSV rows using `encoding/csv` writer flushed directly to `http.ResponseWriter`. No in-memory buffer of the full CSV.
5. Format selection via `format` query parameter:
   - `standard`: Date (YYYY-MM-DD), Vendor, Description, Category, Amount, Currency, Tax, Payment Method, Classification, Source, Artifact ID
   - `quickbooks`: Date (MM/DD/YYYY), Payee, Category, Amount, Memo
6. Mixed currency warning: if multiple currencies detected, prepend a `# Note: Multiple currencies present (USD, EUR). No conversion applied.` comment row.
7. Empty result: CSV with header row only, no error.

### A-003: PATCH /api/expenses/{id} — Correction

**Implementation:**

1. Look up artifact by ID; verify `metadata ? 'expense'`. Return `404 EXPENSE_NOT_FOUND` or `422 NOT_AN_EXPENSE`.
2. Validate input fields: `amount` must be `^\d+\.\d{2}$`, `currency` must be ISO 4217, `classification` must be in allowed set.
3. Merge corrections into existing `metadata.expense` — only provided fields are updated.
4. Set `user_corrected: true` and append corrected field names to `corrected_fields`.
5. If `vendor` is corrected, also create a `vendor_aliases` entry mapping `vendor_raw` → new vendor (source: `"user"`).
6. JSONB update uses PostgreSQL `jsonb_set` or full metadata write (since multiple fields may change).

### A-004: GET /api/expenses/{id} — Detail

Single artifact lookup with full expense metadata. Returns `404` if not found or not an expense.

### A-005: POST /api/expenses/{id}/classify — Classification Change

Shorthand for PATCH with only `classification`. Records `previous_classification` in response. Updates `corrected_fields` to include `"classification"`.

### A-006: POST /api/expenses/suggestions/{id}/accept

1. Look up suggestion by ID; verify status is `pending`.
2. Update the target artifact's `metadata.expense.classification` to the suggested value.
3. Set suggestion status to `accepted`, `resolved_at` to now.
4. Return the suggestion details and count of expenses updated.

### A-007: POST /api/expenses/suggestions/{id}/dismiss

1. Look up suggestion by ID; verify status is `pending`.
2. Set suggestion status to `dismissed`, `resolved_at` to now.
3. Insert a row into `expense_suggestion_suppressions` for the vendor+classification pair.
4. Return confirmation.

---

## 8. Telegram Integration

### Location

New file: `internal/telegram/expenses.go`, extending the existing Telegram bot command handling in `internal/telegram/`.

### Command Routing

The existing Telegram message handler dispatches based on message content. Expense-related messages are detected by:

1. **Photo messages:** If a photo is received and OCR produces text, the receipt extraction pipeline runs. The Telegram handler listens for the `artifacts.processed` NATS response and sends the T-001/T-002/T-003/T-004 confirmation format.
2. **Text matching expense patterns:** Messages containing dollar amounts or expense keywords (`"expense"`, `"spent"`, `"cost"`, `"receipt"`) route to the expense text parser (manual entry, T-005).
3. **Query patterns:** Messages matching `"show expenses"`, `"how much"`, `"export expenses"` route to the expense query handler (T-006, T-007).
4. **Reply context:** `"details"`, `"fix"`, `"done"`, `"accept ... as business"`, `"dismiss ... suggestion"` are handled in reply context to the most recent expense interaction.
5. **Amount replies:** When the bot is awaiting an amount (state: `awaiting_amount`), a message matching `^\$?\d+\.?\d*(\s*(USD|EUR|GBP|CAD|AUD))?$` updates the expense.

### State Management

Conversation state for multi-turn expense interactions (fix flow, amount prompts) uses an in-memory map keyed by chat ID with a TTL (consistent with the existing `disambiguation_timeout_seconds` config pattern):

```go
type expenseConversationState struct {
    LastExpenseID string
    AwaitingField string // "amount", "fix_field", ""
    ExpiresAt     time.Time
}
```

State expires after `telegram.disambiguation_timeout_seconds` (default: 120 seconds).

### Response Formatting

All Telegram responses follow the UX spec (T-001 through T-011). The format functions live in `internal/telegram/expenses.go`:

- `formatExpenseConfirmation(expense)` → T-001 format
- `formatExpenseDetail(expense)` → T-001 detail expansion
- `formatOCRFailure()` → T-002 format
- `formatPartialExtraction(expense)` → T-003 format
- `formatAmountMissing(expense)` → T-004 format
- `formatExpenseList(expenses, filter)` → T-006 format
- `formatExpenseCSVMessage(count, total, incomplete)` → T-007 format
- `formatFixPrompt(expense)` → T-009 format
- `formatFieldUpdated(field, value)` → T-009 update confirmation

### OCR → Receipt Extraction Flow

For Telegram photo captures:

1. User sends photo → existing Telegram handler downloads file, publishes to `keep.ocr.request` NATS subject.
2. OCR result arrives via `keep.ocr.response`.
3. If OCR text < 10 chars → send T-002 failure response, no artifact created.
4. If OCR text ≥ 10 chars → create artifact via capture pipeline, which triggers `artifacts.process`.
5. The ML sidecar's receipt heuristic detects the OCR content as receipt-like (H-004), runs extraction.
6. `artifacts.processed` response arrives with `expense_extraction` data.
7. Telegram handler formats and sends appropriate confirmation (T-001, T-003, T-004).

---

## 9. Digest Integration

### Location

New file: `internal/digest/expenses.go`

### Expense Section Producer

```go
// ExpenseDigestSection produces the expense section for the daily digest.
type ExpenseDigestSection struct {
    Pool *pgxpool.Pool
}

// Assemble gathers expense digest context for the current period.
func (s *ExpenseDigestSection) Assemble(ctx context.Context) (*ExpenseDigestContext, error) { ... }
```

### Integration Point

The existing `Generator.Generate()` in `internal/digest/generator.go` assembles context sections (action items, overnight artifacts, hot topics, hospitality, knowledge health). The expense section is added as a new optional section:

```go
// In Generator.Generate():
if expensesEnabled {
    expCtx, err := s.expenseSection.Assemble(ctx)
    if err != nil {
        slog.Warn("failed to assemble expense digest context", "error", err)
    } else if !expCtx.IsEmpty() {
        digestCtx.Expenses = expCtx
    }
}
```

### ExpenseDigestContext

```go
type ExpenseDigestContext struct {
    PeriodStart     string                    `json:"period_start"`
    PeriodEnd       string                    `json:"period_end"`
    Summary         *ExpenseDigestSummary     `json:"summary,omitempty"`
    NeedsReview     []ExpenseDigestReviewItem `json:"needs_review,omitempty"`
    Suggestions     []ExpenseDigestSuggestion `json:"suggestions,omitempty"`
    MissingReceipts []ExpenseDigestMissing    `json:"missing_receipts,omitempty"`
    UnusualCharges  []ExpenseDigestUnusual    `json:"unusual_charges,omitempty"`
}

func (c *ExpenseDigestContext) IsEmpty() bool {
    return c.Summary == nil &&
        len(c.NeedsReview) == 0 &&
        len(c.Suggestions) == 0 &&
        len(c.MissingReceipts) == 0 &&
        len(c.UnusualCharges) == 0
}
```

### Assembly Queries

| Block | Query |
|-------|-------|
| **Summary** | Count and sum of expenses in the last 7 days, grouped by classification and currency |
| **Needs review** | Artifacts where `extraction_status != 'complete'` or `amount_missing = true`, created in last 7 days, limited to `EXPENSES_DIGEST_NEEDS_REVIEW_LIMIT` |
| **Suggestions** | Pending suggestions from `expense_suggestions` where `status = 'pending'`, limited to `EXPENSES_SUGGESTIONS_MAX_PER_DIGEST` |
| **Missing receipts** | Active subscriptions (from `subscriptions` table) where no expense artifact with matching vendor exists in the last `EXPENSES_DIGEST_MISSING_RECEIPT_LOOKBACK_DAYS` days |
| **Unusual charges** | Expenses from new vendors (vendor not seen in the previous 90 days) in the last 7 days |

### DigestContext Extension

The existing `DigestContext` struct gains a new field:

```go
type DigestContext struct {
    // ... existing fields ...
    Expenses *ExpenseDigestContext `json:"expenses,omitempty"`
}
```

The ML sidecar's digest assembly prompt contract (`digest-assembly-v1`) receives this context and renders the `── Expenses ──` section following the priority order defined in the UX spec (T-010).

---

## 10. Classification Engine

### Location

`internal/intelligence/expenses.go` — new file in the existing intelligence package.

### Rule Priority Chain

Classification is applied when an artifact's expense metadata is first written, and can be re-applied on demand (e.g., when config changes). Rules are evaluated in order; the first match wins:

| Priority | Rule | Source | Result |
|----------|------|--------|--------|
| 1 | User correction exists (`user_corrected: true` and `"classification"` in `corrected_fields`) | Artifact metadata | Preserved; skip all rules |
| 2 | Gmail label matches `expense_labels` config map | `IMAP_EXPENSE_LABELS` env var | Mapped classification |
| 3 | Telegram caption contains `"business"` or `"personal"` | Artifact metadata `notes` / capture context | Extracted keyword |
| 4 | Vendor name matches `business_vendors` config list | `EXPENSES_BUSINESS_VENDORS` env var | `"business"` |
| 5 | Vendor name matches a vendor in `vendor_aliases` that has been user-classified as business ≥ `min_past_business_count` times | DB query | `"business"` (generates suggestion, not auto-classify) |
| 6 | LLM-extracted category is a known business-typical category | Config categories with `tax_category` | No auto-classify; may generate suggestion |
| 7 | No rule matches | — | `"uncategorized"` |

**Important:** Rules 5 and 6 generate **suggestions**, not automatic classifications. Only rules 1–4 directly set the classification. This prevents the system from silently reclassifying expenses without user consent.

### Vendor Normalization

```go
type VendorNormalizer struct {
    Pool  *pgxpool.Pool
    cache map[string]string // LRU cache, size from EXPENSES_VENDOR_CACHE_SIZE
}

func (n *VendorNormalizer) Normalize(vendorRaw string) (canonical string, found bool) {
    // 1. Check in-memory cache
    // 2. Query vendor_aliases table (case-insensitive)
    // 3. Cache result (positive and negative)
    // 4. Return canonical name or original
}
```

Pre-seeded aliases are loaded at startup from a static list embedded in the Go binary (not config — these are application knowledge, not user config). The user augments via corrections. Example pre-seeded entries:

| Alias | Canonical |
|-------|-----------|
| `AMZN MKTP US` | Amazon |
| `AMZN MKTP` | Amazon |
| `AMAZON MARKETPLACE` | Amazon |
| `SQ *` (prefix match) | Square (merchant after `*`) |
| `GOOGLE *` (prefix match) | Google (service after `*`) |
| `PAYPAL *` (prefix match) | PayPal (merchant after `*`) |

Prefix matching uses `LIKE` queries: `LOWER(alias) || '%'` matches the start of `vendor_raw`.

### Suggestion Generation

Runs as part of the scheduled intelligence cycle (alongside `DetectSubscriptions`, `ProduceBillAlerts`):

```go
func (e *ExpenseClassifier) GenerateSuggestions(ctx context.Context) error {
    // 1. Find uncategorized/personal expenses from the last 30 days
    // 2. For each, check if the vendor has >= min_past_business_count
    //    business-classified expenses
    // 3. Skip vendors in expense_suggestion_suppressions
    // 4. Skip artifacts that already have a pending suggestion
    // 5. Create suggestion with confidence score and evidence text
}
```

### Batch Reclassification

When the user adds a vendor to `business_vendors` config (detected on next config reload or via chat command), or when the user runs "reclassify {vendor} as business":

```go
func (e *ExpenseClassifier) ReclassifyVendor(ctx context.Context, vendor string, classification string) (int, error) {
    // 1. Find uncategorized expenses matching vendor (case-insensitive)
    // 2. Limit to EXPENSES_SUGGESTIONS_RECLASSIFY_BATCH_LIMIT
    // 3. Update metadata.expense.classification
    // 4. Return count of updated expenses
}
```

---

## 11. CSV Export Engine

### Location

`internal/api/expenses.go` — part of the `ExpenseHandler.Export` method.

### Standard Format

```
Date,Vendor,Description,Category,Amount,Currency,Tax,Payment Method,Classification,Source,Artifact ID
2026-04-03,Corner Coffee,SQ *CORNER COFFEE,food-and-drink,4.75,USD,,,,gmail,01HWXYZ...
```

- Date: YYYY-MM-DD (from `EXPENSES_EXPORT_STD_DATE_FORMAT`)
- Vendor: normalized vendor name
- Description: `vendor_raw` (original extracted text)
- Empty cells for null fields (no `"N/A"`, no `"null"`)
- Amounts as decimal strings, no currency symbol, no thousands separator

### QuickBooks Format (BS-028)

```
Date,Payee,Category,Amount,Memo
04/03/2026,Corner Coffee,Food & Drink,4.75,Source: gmail
```

- Date: MM/DD/YYYY (from `EXPENSES_EXPORT_QB_DATE_FORMAT`)
- Payee: normalized vendor name
- Category: display name from categories config
- Amount: dot-decimal, no currency symbol. Negative for refunds.
- Memo: concatenation of notes and source. If notes is empty, just source.

### Streaming Implementation

```go
func (h *ExpenseHandler) Export(w http.ResponseWriter, r *http.Request) {
    // 1. Parse and validate query parameters (same as List, minus cursor/limit)
    // 2. Count matching rows; if > max_rows, return 413
    // 3. Detect format parameter (standard or quickbooks)
    // 4. Set Content-Type and Content-Disposition headers
    // 5. Create csv.Writer wrapping http.ResponseWriter
    // 6. Write header row
    // 7. If mixed currencies detected in count query, write comment row
    // 8. Execute query with rows.Next() loop, writing each row directly
    // 9. csv.Writer.Flush() at the end
}
```

No full in-memory buffer. Rows are streamed as they are read from PostgreSQL, keeping memory usage bounded regardless of export size.

---

## Agent + Tools Design

> **Status — Reconciliation with spec 037.** Sections 3, 10, and parts of 8
> describe the *legacy* implementation that predates the agent runtime. They
> remain accurate as a starting point but are SUPERSEDED by this section
> for all new code paths. Specifically:
>
> - Section 3 (Receipt Detection Heuristics in
>   [ml/app/receipt_detection.py](../../ml/app/receipt_detection.py)) is
>   superseded by the `receipt_detect-v1` scenario below.
> - Section 10 (the 7-level rule chain in
>   [internal/intelligence/expenses.go](../../internal/intelligence/expenses.go))
>   is superseded by the `expense_classify-v1` scenario.
> - The pre-seeded vendor map in
>   [internal/intelligence/vendor_seeds.go](../../internal/intelligence/vendor_seeds.go)
>   is superseded by the `vendor_normalize-v1` scenario calling the
>   `vendor_alias_lookup` and `vendor_alias_upsert` tools over PostgreSQL.
>
> This design CONSUMES the agent runtime defined in
> [specs/037-llm-agent-tools/design.md](../037-llm-agent-tools/design.md).
> It does NOT redesign scenario loading, tool registration, allowlist
> enforcement, schema validation, the LLM tool-calling loop, intent
> routing, or the `agent_traces` / `agent_tool_calls` schema. Anything
> below that touches those concerns is a CONSUMER concern (input shape,
> output shape, allowlist composition, retention notes) only.

### Scope

This section specifies, for the expense-tracking domain:

1. The scenarios to register under `config/scenarios/expenses/`.
2. The tools to register from `internal/intelligence/expenses` and
   `internal/api/expenses` (Go-side) via `RegisterTool` per spec 037.
3. The migration plan from the legacy code paths in sections 3, 8, and 10.
4. The storage shape changes on `artifacts.metadata.expense` and the new
   trace-exposing endpoint.
5. The mapping from BS-032..BS-038 adversarial cases to scenario / tool
   paths and to the user-visible UX responses defined in T-016 R1..R7.
6. The Go files / functions / data that become dead code once migration
   completes.
7. How backward compatibility (sticky `user_corrected`) is preserved
   across the agent boundary.

### Scenarios To Register

All scenarios live under `config/scenarios/expenses/` and follow the
prompt-contract shape from spec 037. Side-effect classes (`read` /
`write` / `external`) on `allowed_tools` MUST match the side-effect class
declared at tool registration; mismatch is a startup error per spec 037.

#### `expense_classify-v1`

- **Replaces:** the 7-level rule chain in `ExpenseClassifier.Classify` in
  [internal/intelligence/expenses.go](../../internal/intelligence/expenses.go)
  (see design section 10, "Rule Priority Chain"). Pre-extracted vendor,
  source, captured user notes, and source labels remain the same input
  signals; the decision logic moves into the scenario prompt.
- **Spec traceability:** UC-005 (Main Flow + A1, A2, A3), BS-029, BS-035,
  BS-038. UX surfaces T-012, T-016 R4, T-017, A-008.
- **Input shape (rendered into the system prompt):**
  ```json
  {
    "artifact_id": "01HW...",
    "expense": {
      "vendor_raw": "string",
      "vendor": "string|null",
      "amount": "decimal-string|null",
      "currency": "ISO-4217|null",
      "date": "YYYY-MM-DD|null",
      "extraction_status": "ok|partial|failed",
      "user_corrected": false,
      "corrected_fields": []
    },
    "context": {
      "source": "gmail|telegram|web|manual|pdf",
      "source_labels": ["string"],
      "captured_notes": "string|null"
    }
  }
  ```
- **Allowed tools (all `read` except where noted):**
  - `expense_get` (read)
  - `expense_lookup_history` (read)
  - `expense_aggregate` (read)
  - `vendor_alias_lookup` (read)
  - `lookup_business_vendor_list` (read) — exposes the
    `EXPENSES_BUSINESS_VENDORS` SST list as a tool, not as in-prompt config.
  - `lookup_expense_label_map` (read) — exposes the Gmail
    `IMAP_EXPENSE_LABELS` SST map as a tool.
- **Output shape (validated against scenario JSON Schema):**
  ```json
  {
    "classification": "business|personal|uncategorized",
    "category": "string|null",
    "rationale": "string",
    "rationale_short": "string (≤ 80 chars)",
    "tentative": false
  }
  ```
- **Sticky-correction contract:** before deciding, the scenario MUST call
  `expense_get` for the artifact and short-circuit to the existing
  classification when `user_corrected == true` AND `"classification" ∈
  corrected_fields`. This is enforced at both the prompt level and via a
  Go-side post-validation hook that rejects any output that contradicts a
  sticky correction (see "Backward Compatibility" below).
- **Failure handling:** on schema-failure, loop-limit, or LLM error
  (handled by spec 037 generically) the artifact is left at its prior
  classification (or `uncategorized` for new artifacts) and surfaced in the
  digest "needs review" block — matching UC-005 A3.
- **Retention / eviction:** none specific. Trace rows follow the global
  spec-037 retention (30 days hot in PostgreSQL).

#### `vendor_normalize-v1`

- **Replaces:** the static `vendorSeeds` slice in
  [internal/intelligence/vendor_seeds.go](../../internal/intelligence/vendor_seeds.go)
  AND the `VendorNormalizer.Normalize` cache lookup logic in
  [internal/intelligence/expenses.go](../../internal/intelligence/expenses.go).
  Real captured vendor history plus user overrides become the source of
  truth; the seed list becomes one-time bootstrap data (see migration plan).
- **Spec traceability:** BS-014, BS-036, IP-003, UX T-016 R5.
- **Input shape:**
  ```json
  {
    "vendor_raw": "string",
    "source": "gmail|telegram|web|manual|pdf",
    "amount_hint": "decimal-string|null"
  }
  ```
- **Allowed tools:**
  - `vendor_alias_lookup` (read)
  - `expense_lookup_history` (read) — to check whether vendor candidates
    have prior captured expenses.
  - `vendor_alias_upsert` (write) — only when the scenario reaches a
    high-confidence canonicalization that should be remembered. The output
    schema MUST include `should_persist: bool` and the Go-side dispatch
    only honors the upsert call when this flag is true; lower-confidence
    candidates are returned without persisting (BS-036 "leave as captured
    rather than guessing").
- **Output shape:**
  ```json
  {
    "vendor": "string",
    "vendor_raw": "string",
    "confidence": "high|medium|low",
    "candidate_match": "string|null",
    "rationale_short": "string"
  }
  ```
- **Retention / eviction:** the per-process LRU cache that lived in
  `VendorNormalizer` is removed; lookups go through the `vendor_aliases`
  table behind the tool. PostgreSQL handles caching. If profiling later
  shows a hot path, a tool-side LRU may be reintroduced inside
  `vendor_alias_lookup` only.

#### `receipt_detect-v1`

- **Replaces:** the heuristic rules in
  [ml/app/receipt_detection.py](../../ml/app/receipt_detection.py)
  (design section 3) as the canonical decision point. Per spec 037
  guidance, the existing Python heuristic MAY remain as an inexpensive
  read-only tool the scenario consults — see `receipt_heuristic_score`.
- **Spec traceability:** BS-020, BS-029, IP-002.
- **Input shape:**
  ```json
  {
    "artifact_id": "01HW...",
    "source": "gmail|telegram|web|manual|pdf",
    "subject": "string|null",
    "from_domain": "string|null",
    "text_excerpt": "string (truncated to N chars by Go side)"
  }
  ```
- **Allowed tools:**
  - `receipt_heuristic_score` (read) — wraps the existing Python heuristic
    callable and returns its score and which signals fired. Implemented as
    a Go tool that round-trips through the ML sidecar via the existing
    `artifacts.process` boundary; no new NATS subjects.
  - `vendor_alias_lookup` (read) — to recognise known vendors as a signal.
  - `expense_lookup_history` (read) — to recognise repeat-sender patterns.
- **Output shape:**
  ```json
  {
    "is_receipt": true,
    "rationale_short": "string"
  }
  ```
- **Retention / eviction:** none specific.

#### `expense_query-v1`

- **Purpose:** handle free-form Telegram queries ("how much did I spend on
  coffee last month?") and emit a structured response that the existing
  Telegram formatter (T-012) renders.
- **Spec traceability:** BS-029, UX T-012, UX T-017.
- **Input shape:**
  ```json
  {
    "user_text": "string",
    "now": "RFC3339",
    "user_timezone": "IANA"
  }
  ```
- **Allowed tools:**
  - `expense_aggregate` (read)
  - `expense_lookup_history` (read)
  - `vendor_alias_lookup` (read)
- **Output shape:**
  ```json
  {
    "answer_text": "string",
    "breakdown": [{"label": "string", "amount": "decimal-string", "currency": "ISO-4217"}],
    "rationale_short": "string",
    "filters_used": {"date_from": "...", "date_to": "...", "vendor": "...", "category": "..."}
  }
  ```

#### `subscription_detect-v1`

- **Purpose:** recognize recurring charges and propose a subscription flag.
  Replaces ad-hoc subscription seeding logic that would otherwise grow
  inside the existing `DetectSubscriptions` intelligence cycle.
- **Spec traceability:** BS-029, UX T-013.
- **Input shape:**
  ```json
  {
    "vendor": "string",
    "lookback_days": 180
  }
  ```
- **Allowed tools:**
  - `expense_lookup_history` (read)
  - `expense_aggregate` (read)
  - `expense_subscription_mark` (write) — only when `should_mark: true`.
- **Output shape:**
  ```json
  {
    "is_subscription": true,
    "cadence": "weekly|monthly|quarterly|yearly|irregular",
    "typical_amount": "decimal-string",
    "currency": "ISO-4217",
    "should_mark": true,
    "rationale_short": "string"
  }
  ```
- **Retention / eviction:** scenario invocations are scheduled — not
  per-artifact — and rate-limited at the caller (the existing intelligence
  scheduler in `internal/intelligence/`). No new infrastructure.

#### `unusual_spend-v1`

- **Purpose:** anomaly detection on incoming expenses. Replaces the
  current "new vendor in last 7 days" rule embedded in the digest section
  10 of this design.
- **Spec traceability:** BS-030, UX T-014, digest D-002.
- **Input shape:**
  ```json
  {
    "expense": { "...": "same shape as expense_classify-v1 input.expense" },
    "category": "string|null"
  }
  ```
- **Allowed tools:**
  - `expense_aggregate` (read)
  - `expense_lookup_history` (read)
- **Output shape:**
  ```json
  {
    "is_unusual": true,
    "severity": "low|medium|high",
    "comparison": "string (e.g., '5x typical weekly grocery spend')",
    "rationale_short": "string"
  }
  ```

#### `refund_recognize-v1`

- **Purpose:** identify negative amounts / refund-language artifacts and
  link them to the original purchase via the knowledge graph.
- **Spec traceability:** BS-031, UX T-015.
- **Input shape:**
  ```json
  {
    "expense": { "...": "same shape as expense_classify-v1 input.expense" },
    "extracted_text_excerpt": "string"
  }
  ```
- **Allowed tools:**
  - `expense_lookup_history` (read)
  - `expense_get` (read)
  - `expense_link_refund` (write) — only when `linked_artifact_id` is set.
- **Output shape:**
  ```json
  {
    "is_refund": true,
    "linked_artifact_id": "01HW...|null",
    "rationale_short": "string"
  }
  ```

### Tools To Register

All tools register via `RegisterTool` from package `init()` per spec 037
(no central tools.go enumeration). Each tool declares a side-effect class;
mismatched allowlist entries refuse to start. Schemas below are the
JSON-Schema-equivalent field shapes, not the literal schema text.

#### `expense_lookup_history` — read

- **Owner:** `internal/intelligence/expenses` (data ownership matches the
  legacy classifier package).
- **Input:** `{ vendor?: string, vendor_raw?: string, category?: string,
  date_from?: YYYY-MM-DD, date_to?: YYYY-MM-DD, classification?:
  "business"|"personal"|"uncategorized", limit?: int (≤ 50) }`
- **Output:** `{ expenses: [ExpenseSummary], truncated: bool }` where
  `ExpenseSummary = { artifact_id, vendor, amount, currency, date,
  category, classification, user_corrected }`.
- **Side-effect class:** `read`. No mutations.

#### `expense_get` — read

- **Owner:** `internal/intelligence/expenses`.
- **Input:** `{ artifact_id: string }`.
- **Output:** the full `expense` metadata sub-document plus
  `user_corrected`, `corrected_fields`, `is_subscription`, `is_refund`.
- **Side-effect class:** `read`. Used by the sticky-correction contract.

#### `expense_update_classification` — write

- **Owner:** `internal/api/expenses` (the package that owns the
  user-correction PATCH semantics today).
- **Input:** `{ artifact_id: string, classification:
  "business"|"personal"|"uncategorized", category?: string,
  user_corrected: bool }`.
- **Output:** `{ updated: bool, prior: { classification, category } }`.
- **Side-effect class:** `write`. The `user_corrected` field MUST come
  from the caller — scenarios MUST set it `true` only when the input
  envelope itself originates from an explicit user correction. Scenario
  output MUST NOT pass `user_corrected: true` based on inference.

#### `expense_aggregate` — read

- **Owner:** `internal/intelligence/expenses`.
- **Input:** `{ group_by?:
  "vendor"|"category"|"classification"|"day"|"week"|"month",
  filter: { vendor?, category?, classification?, date_from?, date_to? } }`.
- **Output:** `{ rows: [{ key, sum, count, currency }], mixed_currency:
  bool }`.
- **Side-effect class:** `read`. Implemented over the same `CAST(metadata
  ->>'amount' AS NUMERIC)` query pattern as section 5.

#### `vendor_alias_lookup` — read

- **Owner:** `internal/intelligence/expenses`.
- **Input:** `{ vendor_raw: string, mode?: "exact"|"prefix"|"fuzzy",
  limit?: int (≤ 20) }`.
- **Output:** `{ matches: [{ canonical, alias, confidence }] }`.
- **Side-effect class:** `read`. Replaces the in-process LRU lookup.

#### `vendor_alias_upsert` — write

- **Owner:** `internal/intelligence/expenses`.
- **Input:** `{ alias: string, canonical: string, source:
  "scenario"|"user_correction"|"bootstrap" }`.
- **Output:** `{ created: bool, updated: bool }`.
- **Side-effect class:** `write`. Only `vendor_normalize-v1` and the
  user-correction PATCH path are allowlisted to call it. Bootstrap (the
  one-time seed import) calls it with `source: "bootstrap"` directly via
  Go, NOT via the agent.

#### `expense_link_refund` — write

- **Owner:** `internal/intelligence/expenses`.
- **Input:** `{ refund_artifact_id: string, original_artifact_id: string }`.
- **Output:** `{ linked: bool }`.
- **Side-effect class:** `write`. Sets `is_refund: true` and a
  `refund_of_artifact_id` field on the refund's metadata. Aggregations
  net the negative amount per BS-031.

#### `expense_subscription_mark` — write

- **Owner:** `internal/intelligence/expenses`.
- **Input:** `{ vendor: string, cadence: string, typical_amount:
  decimal-string, currency: ISO-4217 }`.
- **Output:** `{ marked: bool, subscription_id: string }`.
- **Side-effect class:** `write`. Writes to the existing `subscriptions`
  table referenced in section 9 ("missing receipts" digest computation).

#### `receipt_heuristic_score` — read (already implied above)

- **Owner:** `internal/intelligence/expenses` (Go wrapper); the underlying
  evaluator stays in
  [ml/app/receipt_detection.py](../../ml/app/receipt_detection.py) for
  v1 and is invoked over the existing `artifacts.process` boundary.
- **Input:** `{ subject?, from_domain?, text_excerpt }`.
- **Output:** `{ score: float, signals: [string] }`.
- **Side-effect class:** `read`.

### Migration Plan

Phased removal of the legacy code paths. Each phase is verifiable, and
the legacy code remains executable behind a feature flag until the
acceptance gate is met.

#### Feature Flag (SST, no default)

A new SST key in [config/smackerel.yaml](../../config/smackerel.yaml):

```yaml
expenses:
  classifier: agent  # or: legacy
```

- Generated as `EXPENSES_CLASSIFIER` in
  `config/generated/{dev,test}.env` by `./smackerel.sh config generate`.
- Read in Go via `os.Getenv("EXPENSES_CLASSIFIER")` followed by an
  empty-check that calls `log.Fatal` per the SST zero-defaults rule
  ("FORBIDDEN: `getEnv("KEY", "fallback")`"). No silent default to either
  value — the operator MUST set it explicitly.
- Same enforcement for `expenses.receipt_detector` (`agent|legacy`) and
  `expenses.vendor_normalizer` (`agent|legacy`) so the three legacy paths
  flip independently.

#### Phases

1. **Land the agent paths behind the flag.** All three flags default to
   `legacy` in `config/smackerel.yaml` (the OPERATOR default — code still
   has no default). Scenarios + tools are registered. Trace writes happen
   even on the `legacy` path: every `legacy`-classified expense records a
   shadow `expense_classify-v1` invocation in `agent_traces` for offline
   comparison. The shadow path does NOT write `expense_update_classification`.
2. **Backfill / re-evaluation in batches.** A one-shot operator command
   (`./smackerel.sh ...` extension under
   [scripts/runtime/](../../scripts/runtime/)) walks
   `metadata.expense.classification IS NULL` artifacts and invokes
   `expense_classify-v1`, recording old (null) → new in
   `agent_tool_calls`. Bounded by a `--batch-size` flag (no default;
   operator-supplied). Same pattern for `vendor_normalize-v1` against
   `vendor_raw` rows whose canonical is the legacy seed-list output.
3. **Acceptance gate.** Before flipping any flag's operator default to
   `agent`, the agent path MUST match the legacy path on a labeled
   holdout dataset:
   - `expense_classify`: ≥ 95% agreement on ≥ 500 user-confirmed
     classifications, computed by re-running both paths over the holdout
     and diffing the result. Disagreements MUST be surfaced for human
     review and either accepted (legacy was wrong) or filed as scenario
     prompt regressions.
   - `vendor_normalize`: ≥ 99% agreement on the existing seed-list
     mappings (effectively a regression suite — the seed entries become
     fixtures).
   - `receipt_detect`: ≥ 95% agreement on a labeled set of receipt /
     non-receipt emails sampled from prior captures.
4. **Flip the operator default.** `expenses.classifier: agent` etc.
   becomes the default in `config/smackerel.yaml`. The legacy paths
   remain compiled in for one further release in case rollback is needed.
5. **Delete legacy code.** See "What Gets Deleted" below. The vendor
   seeds are imported once via a bootstrap migration that calls
   `vendor_alias_upsert` directly (bypassing the agent loop) with
   `source: "bootstrap"`. After this migration runs, `vendor_seeds.go`
   is deleted.

### Storage Changes

`metadata.expense` (JSONB sub-document on `artifacts.metadata`, schema in
section 5.1) gains four optional fields, all `null`-safe for legacy rows:

```json
{
  "scenario": "expense_classify-v1",
  "rationale": "Vendor 'Stripe' has 14 prior business-classified charges; ...",
  "rationale_short": "Repeat business vendor",
  "agent_trace_id": "01HW..."
}
```

- `scenario`: the scenario id+version that produced the value. `null`
  for legacy rows or for fields produced by user correction.
- `rationale`: full prose rationale string. Surfaced in T-017 expandable
  display and in `GET /api/expenses/{id}/trace`.
- `rationale_short`: ≤ 80 chars. Surfaced in T-012 / T-013 / T-017
  compact display, A-001 list response, and digest blocks.
- `agent_trace_id`: foreign key to `agent_traces.id` from spec 037.
  `null` when the value did not come from the agent (legacy classifier,
  user correction, or bootstrap). Same field is reused by every
  scenario that touches the expense (`expense_classify-v1`,
  `vendor_normalize-v1`, `unusual_spend-v1`, etc.) — when multiple
  scenarios contribute, the most recent agent invocation's id wins for
  the artifact-level field; per-field provenance lives in the trace.

A new endpoint joins the trace:

#### `A-008: GET /api/expenses/{id}/trace`

- Already specified in spec.md UX A-008. This design wires it to
  `agent_traces` + `agent_tool_calls`:
  - Look up the artifact's `metadata.expense.agent_trace_id`.
  - If null → 404 with `{ "error": "no_agent_trace" }`.
  - If set → join `agent_traces` and `agent_tool_calls` and return the
    spec-037 trace shape unchanged. Including `rejected_calls` for
    BS-038 visibility per the UX spec.
- Implemented as a thin handler in `internal/api/expenses.go`. The agent
  trace store is owned by spec 037; this design only consumes it.

### Failure-Mode Mapping (BS-032..BS-038)

| BS | Scenario / Tool Path | Enforcement Layer | User-Visible Response |
|----|----------------------|-------------------|------------------------|
| BS-032 Corrupted OCR | `receipt_detect-v1` returns `is_receipt: false` OR `expense_classify-v1` schema-failure | spec 037 schema validator + receipt-detect short-circuit | UX T-016 R1 — "Couldn't read this receipt clearly. Stored as-is." Artifact stored with `extraction_status: failed`. Surfaced in digest "needs review". No hallucinated vendor / amount. |
| BS-033 Missing amount, otherwise valid | `expense_classify-v1` allowed to produce `tentative: true` with `amount_missing: true` carried in metadata | scenario output schema (`tentative` field) | UX T-016 R2 — tentative classification with explicit "amount missing" rationale; appears in digest "needs review". |
| BS-034 Mixed currency | extraction stores per-line-item currency (section 5); `expense_classify-v1` consumes the dual-currency record without coercing | extraction prompt contract (unchanged) + scenario prompt | UX T-016 R3 — both currencies surfaced verbatim; aggregations use BS-010 mixed-currency rules; no silent coercion. |
| BS-035 Ambiguous business / personal | `expense_classify-v1` returns `classification: "uncategorized"` + rationale | scenario prompt + Go-side post-validation rejecting fabricated confidence | UX T-016 R4 + T-017 — uncategorized + compact rationale. May be re-evaluated later when more user-corrected similars exist (UC-005 A2). |
| BS-036 Vendor typo | `vendor_normalize-v1`; high-confidence ⇒ `vendor_alias_upsert` write; low ⇒ `should_persist: false` | scenario output `confidence` field + Go gate on `should_persist` | UX T-016 R5 — silent normalization on high confidence; surfaced candidate match on low confidence; `vendor_raw` always preserved. |
| BS-037 Foreign-language receipt | extraction handles language; `expense_classify-v1` reasons over translated context (LLM-native) | scenario prompt; no English-only keyword filter exists in the agent path | UX T-016 R6 — category determined from receipt content; amount normalized to dot-decimal per BS-023. |
| BS-038 Hallucinated tool call | spec-037 allowlist enforcement rejects the call before execution; recorded in `agent_tool_calls.rejected = true` | spec 037 dispatch (NOT this design) | UX T-016 R7 — user sees the same response as a normal "no classification" outcome. Operator sees the rejected call in `GET /api/expenses/{id}/trace` (A-008). |

### What Gets Deleted

Once the migration acceptance gate passes for all three flags and one
release of dual-path operation is shipped, the following becomes dead
code and is removed in the same change set that flips the operator
defaults to `agent`:

- [internal/intelligence/expenses.go](../../internal/intelligence/expenses.go):
  - `func (ec *ExpenseClassifier) Classify(...)` — the entire 7-level
    rule chain.
  - `type VendorNormalizer struct { ... }` and its methods
    (`Normalize`, `Invalidate`, `put`, plus the `NewVendorNormalizer`
    constructor) — the in-process LRU and seed-bootstrap path.
  - The seed-load loop at lines ~150-160 that ranges over `vendorSeeds`.
  - The `vendorNormalizer` field on `ExpenseClassifier` and its wiring
    in the constructor; replaced by direct tool invocation from the
    relevant scenarios.
- [internal/intelligence/vendor_seeds.go](../../internal/intelligence/vendor_seeds.go):
  the entire file is deleted after the one-time bootstrap migration runs
  in production. The seed data lives only in `vendor_aliases` rows after
  migration.
- [ml/app/receipt_detection.py](../../ml/app/receipt_detection.py):
  retained as a callable invoked by the `receipt_heuristic_score` tool;
  the public Python entrypoint that is currently called directly from
  `ml/app/synthesis.py` is removed and the synthesis path goes through
  `receipt_detect-v1` instead. The heuristic functions themselves stay
  as they are still consulted by the tool.
- [internal/intelligence/expenses_test.go](../../internal/intelligence/expenses_test.go):
  tests covering the deleted `Classify` rule chain and the
  `VendorNormalizer` LRU are removed. Tests covering sticky
  `user_corrected` behavior MOVE to the agent test surface and become
  scenario-level adversarial tests.
- The hardcoded expense-intent regex / keyword branches in any
  `internal/telegram/` files that fan out to expense handlers (no such
  file is present today; the existing telegram package has no
  `expense*` or `intent*` files, but spec 037 explicitly removes the
  Telegram regex/switch dispatcher pattern as an anti-pattern). When
  any such branches are added in the interim, they MUST be deleted at
  the same time the agent path becomes default. The kept regex
  examples (e.g., "show expenses", "export expenses MONTH") remain in
  the codebase as MUST-handle examples in the intent-routing test
  suite ONLY — not as production dispatch logic.

### Backward Compatibility

- **Sticky `user_corrected` is preserved across the agent boundary.** The
  `expense_classify-v1` scenario MUST call `expense_get` first; when the
  result has `user_corrected == true` AND `"classification" ∈
  corrected_fields`, the scenario MUST short-circuit and emit the prior
  classification verbatim. A Go-side post-validation hook on the
  scenario's output rejects any contradicting result (it returns the
  prior classification regardless and records a scenario-defect trace
  event). This double-enforcement (prompt + Go gate) is deliberate:
  spec 037's generic schema validation cannot encode "must equal another
  field's prior value", so this domain rule lives here.
- **User corrections flow through `expense_update_classification` with
  `user_corrected: true`** from the existing PATCH handler in
  `internal/api/expenses.go`. The PATCH handler is the ONLY caller in
  the codebase that may pass `user_corrected: true`; scenario-driven
  writes always pass `false`. This invariant is asserted by a unit test
  on the tool registration.
- **Legacy rows without `scenario` / `agent_trace_id` continue to work.**
  All four new fields are nullable. `GET /api/expenses` (A-001) returns
  empty strings for `rationale` / `rationale_short` and omits
  `scenario` / `agent_trace_id` when null, matching the field-rules
  table in spec.md UX A-001.
- **Existing extraction prompt contracts are unchanged.**
  `receipt-extraction-v1` (section 4) keeps its current schema; the
  agent layer only governs detection (`receipt_detect-v1`) and post-
  extraction reasoning, not the extraction shape itself. This matches
  spec 037's "Existing extraction prompt contracts ... are unchanged.
  Migration to scenarios is opt-in per contract."

### Constraints Honored

- **No agent runtime redesign.** All scenario loading, tool dispatch,
  allowlist enforcement, JSON Schema validation, the LLM tool-calling
  loop, intent routing, and the `agent_traces` / `agent_tool_calls`
  schema are defined in
  [specs/037-llm-agent-tools/design.md](../037-llm-agent-tools/design.md).
- **SST zero-defaults.** `EXPENSES_CLASSIFIER`,
  `EXPENSES_RECEIPT_DETECTOR`, `EXPENSES_VENDOR_NORMALIZER` MUST be set
  in `config/smackerel.yaml` (operator default `legacy` until the
  acceptance gate flips it). Code reads with no fallback and
  `log.Fatal` on empty.
- **Existing package boundaries preserved.** Tools register from
  `internal/intelligence/expenses` (data-owning package) and
  `internal/api/expenses` (HTTP-owning package). No new top-level
  packages, no central `tools.go`, no cross-package data ownership
  inversions.

---

## 12. Testing Strategy

### Test Type Mapping

| Test Type | Category | What It Validates | Spec Traceability |
|-----------|----------|-------------------|-------------------|
| Go unit: receipt heuristic (Python port) | unit | Billing keywords + amount pattern matching, edge cases | BS-020, UC-001, IP-002 |
| Go unit: classification engine | unit | Rule priority chain, label mapping, vendor matching, suggestion skip logic | UC-005, BS-004, BS-007, BS-021 |
| Go unit: vendor normalizer | unit | Alias lookup, prefix matching, cache behavior, user correction alias creation | BS-014, IP-003 |
| Go unit: CSV export formatting | unit | Standard and QuickBooks column mapping, date formatting, refund handling, mixed currency warning | UC-008, BS-009, BS-010, BS-011, BS-028 |
| Go unit: expense API handlers | unit | Request validation, error codes, JSONB query construction, pagination | A-001 through A-007 |
| Go unit: digest section producer | unit | Summary computation, needs-review selection, suggestion limit, missing receipt detection, word limit enforcement | UC-009, T-010 |
| Python unit: receipt detection heuristic | unit | H-001 through H-005 rule evaluation, false positive rejection | BS-020, IP-002 |
| Python unit: receipt extraction contract | unit | Schema validation of extraction output, edge cases (missing fields, negative amounts, comma decimals) | UC-001, BS-015, BS-023, BS-024 |
| Go unit: Telegram expense formatting | unit | T-001 through T-011 response format compliance, edge cases | UX spec surface 1 |
| Integration: email → extraction → metadata | integration | Gmail connector sync → ML sidecar receipt extraction → expense metadata stored in artifacts table | UC-001, BS-001 |
| Integration: OCR → extraction → metadata | integration | Telegram photo → OCR → receipt heuristic → extraction → expense metadata | UC-002, BS-002 |
| Integration: classification after extraction | integration | Artifact processed → classification engine runs → correct classification applied | UC-005, BS-004 |
| Integration: suggestion generation | integration | Intelligence cycle → suggestion created for uncategorized vendor with business history | UC-006, BS-007 |
| Integration: vendor normalization | integration | Alias table populated → extraction produces normalized vendor | BS-014 |
| E2E: full expense capture to export | e2e-api | Email receipt → extraction → query via API → CSV export → validate CSV content | BS-001, BS-009 |
| E2E: Telegram receipt to query | e2e-api | Photo → OCR → extraction → query "show expenses" → verify expense in list | BS-002, T-001, T-006 |
| E2E: correction and sticky override | e2e-api | PATCH correction → re-extraction → verify correction survives | UC-010, BS-008 |
| E2E: suggestion accept/dismiss | e2e-api | Generate suggestion → accept via API → verify reclassification | UC-006, BS-007 |
| Stress: batch receipt processing | stress | 500 receipt-bearing artifacts queued → extraction completes within timeout per artifact → no drops | BS-018 |
| Stress: large export | stress | 10,000 expense artifacts → CSV export completes within 10 seconds | NFR: Performance |

### Adversarial Test Cases

| Test | Adversarial Property | Target Bug |
|------|---------------------|------------|
| Marketing email from known vendor → no expense metadata | Non-receipt must be rejected even when vendor is known | BS-020: false positive receipt detection |
| Expense with `user_corrected: true` survives re-extraction | Re-extraction must check corrected_fields before overwriting | BS-008: sticky correction bypass |
| Dismissed suggestion vendor → no new suggestion generated | Suppression table must be checked during generation | UC-006 A2: suggestion re-generation after dismissal |
| Amount with comma decimal → dot decimal in export | Normalization must happen before storage | BS-023: international receipt formatting |
| Refund with negative amount → reduces total in aggregation | Aggregation SUM must handle negatives | BS-011: refund sign handling |
| 10,001 rows → 413 error, not partial export | Row count check must happen before streaming begins | A-002: export size enforcement |

---

## 13. Risks & Open Questions

### Technical Risks

| Risk | Impact | Mitigation |
|------|--------|------------|
| LLM extraction quality varies across receipt formats | Incorrect vendor, amount, or date extraction | Receipt heuristic pre-filters reduce noise; extraction_status tracks quality; user correction is always available |
| OCR quality on photos is inconsistent | Unusable text from blurry photos | Existing Tesseract + Ollama vision fallback; explicit failure messaging (T-002); manual entry fallback |
| JSONB query performance at scale | Slow expense queries with large artifact table | Partial GIN index and B-tree indexes on expense date/classification; query plan monitoring |
| Vendor normalization cold start | Many aliases needed before normalization is useful | Pre-seed ~100 common vendor aliases; every user correction improves the map |
| Config reload for business_vendors changes | User edits YAML but reclassification doesn't happen until restart | Document that `./smackerel.sh config generate` + restart is needed; or implement config file watch (future) |
| CSV export memory for streaming | Very large exports could still pressure memory | Streaming implementation uses `rows.Next()` cursor; no full result set in memory |

### Open Questions (from spec, with design recommendations)

| Question | Spec Reference | Design Recommendation |
|----------|---------------|----------------------|
| Category taxonomy: IRS Schedule C vs. custom? | OQ-1 | Use simplified custom slugs with a `tax_category` mapping field in config. Covers IRS reporting needs without forcing IRS jargon on daily use. |
| Pre-seed vendor aliases or learn from scratch? | OQ-2 | Pre-seed ~100 common aliases compiled into the binary. User corrections add to the `vendor_aliases` table. The pre-seeded set is static and not configurable. |
| PDF attachment: extract from email body or PDF? | OQ-3 | Try PDF extraction first (usually more structured). If PDF extraction returns empty or fails, fall back to email body text. Both paths feed through the same receipt extraction contract. |
| Batch reclassification: immediate or scheduled? | OQ-4 | Immediate, bounded to `reclassify_batch_limit` (default: 100). Returns count to user. If more than `batch_limit` match, a follow-up message notes remaining count for next cycle. |

### Future Considerations (out of scope for v1)

- **Cross-domain expense intelligence (IP-001):** Linking expenses to calendar events, email threads, and person entities. Requires the knowledge graph to be mature.
- **Bank feed integration:** Parsing bank statements or connecting to bank APIs for automatic transaction matching.
- **Receipt image storage:** Storing the original receipt image as an attachment to the artifact.
- **Multi-currency aggregation:** Converting amounts for cross-currency reporting.
- **Config file watching:** Hot-reloading `smackerel.yaml` changes without restart for business_vendors and expense_labels updates.

---

## RESULT-ENVELOPE

```yaml
agent: bubbles.design
role: Technical design document author
outcome: COMPLETED
affected_artifacts:
  - specs/034-expense-tracking/design.md (CREATED)
evidence_summary: |
  Created design.md covering all 13 required sections:
  1. Overview — architecture intent, five guiding principles
  2. Architecture — component diagram, data flow, ownership table
  3. Receipt Detection Heuristics — 5 rules (H-001 to H-005), Python amount pattern, integration point
  4. Receipt Extraction Prompt Contract — full YAML schema following product-extraction-v1 pattern
  5. Data Model — expense metadata schema, 3 new tables (vendor_aliases, expense_suggestions, expense_suggestion_suppressions), PostgreSQL indexes, query patterns
  6. Configuration — smackerel.yaml additions (connectors.imap.expense_labels, expenses section), config generation pipeline, SST enforcement table
  7. API Endpoints — 7 endpoints (A-001 to A-007), route registration, handler struct, query construction, validation
  8. Telegram Integration — command routing, state management, OCR flow, 11 format functions
  9. Digest Integration — ExpenseDigestSection producer, DigestContext extension, 5 assembly queries
  10. Classification Engine — 7-level rule priority chain, vendor normalizer, suggestion generator, batch reclassification
  11. CSV Export Engine — standard and QuickBooks formats, streaming implementation
  12. Testing Strategy — 16 test types mapped to spec scenarios, 6 adversarial test cases
  13. Risks & Open Questions — 6 technical risks, 4 resolved OQs with recommendations, future considerations

  Design builds on existing codebase: QualifierConfig (imap.go), billingKeywords/amountPattern (subscriptions.go), ProduceBillAlerts (alert_producers.go), OCR pipeline (ocr.py), synthesis pipeline (synthesis.py), prompt contract pattern (product-extraction-v1.yaml), capture API (capture.go), tier assignment (tier.go, constants.go), digest generator (generator.go), standard API envelope.

  SST compliance verified: all config values defined in smackerel.yaml first, consumed via env vars, fail-loud enforcement documented per-value.

  No modifications to spec.md. No new services, containers, or message queues.
```
