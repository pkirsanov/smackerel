# Design: 026 Domain-Aware Structured Extraction

> **Spec:** [spec.md](spec.md)
> **Parent Design:** [docs/smackerel.md](../../docs/smackerel.md)
> **Depends On:** Phase 2 Ingestion (003), Knowledge Synthesis Layer (025)
> **Author:** bubbles.design
> **Date:** April 17, 2026
> **Status:** Draft

---

## Overview

Domain extraction adds a **second, optional LLM pass** after universal processing. When an artifact's `content_type` matches a registered domain extraction prompt contract, the system dispatches a domain-specific extraction request through NATS to the ML sidecar. The sidecar loads the domain contract, builds a targeted prompt, validates the LLM output against the contract's JSON Schema, and returns structured data. The Go core stores the validated result in a `domain_data` JSONB column on the artifacts table.

This design follows the same architectural pattern as the knowledge synthesis layer (spec 025): event-driven via NATS, contract-loaded from YAML, validated against JSON Schema, fail-open so domain extraction failures never block ingestion.

### Key Design Decisions

1. **Parallel to synthesis, not sequential** — Domain extraction fires alongside knowledge synthesis after `HandleProcessedResult`, not after synthesis completes. Both are independent enrichment passes.
2. **Same NATS stream** — Uses the existing `ARTIFACTS` stream with new subjects `domain.extract` / `domain.extracted` to avoid stream proliferation.
3. **Contract-driven dispatch** — The Go core loads domain contracts at startup, builds a `content_type → contract` index, and dispatches automatically. Adding a new domain is a YAML file + restart.
4. **JSONB storage** — All domain data lives in a single `domain_data JSONB` column, indexed with GIN for `@>` containment queries and JSONB path operators.
5. **Minimum content threshold** — Content shorter than 200 characters is skipped for domain extraction (too little signal).

---

## Architecture

### Component Interaction

```
                    ┌─────────────────────────────────────────────────────┐
                    │                   Go Core Runtime                   │
                    │                                                     │
                    │  ┌──────────────────┐   ┌────────────────────────┐  │
                    │  │  DomainRegistry  │   │  ResultSubscriber      │  │
                    │  │  (startup load)  │   │  handleMessage()       │  │
                    │  │                  │   │    ├─ HandleProcessed  │  │
                    │  │  content_type →  │   │    ├─ publishSynthesis │  │
                    │  │  *DomainContract │   │    └─ publishDomain ←─┼──┤
                    │  └──────────────────┘   └────────────────────────┘  │
                    │                                    │                │
                    │              NATS Publish: domain.extract           │
                    │                                    │                │
                    └────────────────────────────────────┼────────────────┘
                                                         │
                                                         ▼
                    ┌─────────────────────────────────────────────────────┐
                    │                   Python ML Sidecar                  │
                    │                                                     │
                    │  Subscribe: domain.extract                          │
                    │    ├─ load_domain_contract(version)                 │
                    │    ├─ build_domain_prompt(contract, content)        │
                    │    ├─ LLM call (litellm)                           │
                    │    ├─ validate_extraction(output, schema)           │
                    │    └─ Publish: domain.extracted                     │
                    └─────────────────────────────────────────────────────┘
                                                         │
                                                         ▼
                    ┌─────────────────────────────────────────────────────┐
                    │                   Go Core Runtime                   │
                    │                                                     │
                    │  Subscribe: domain.extracted                        │
                    │    ├─ Validate response                             │
                    │    ├─ UPDATE artifacts SET domain_data = $1,        │
                    │    │    domain_extraction_status = 'completed',     │
                    │    │    domain_schema_version = $2                  │
                    │    └─ Ack                                           │
                    └─────────────────────────────────────────────────────┘
```

### Pipeline Integration Point

Domain extraction hooks into `ResultSubscriber.handleMessage()` in [internal/pipeline/subscriber.go](../../internal/pipeline/subscriber.go), at the same level as the existing knowledge synthesis dispatch:

```go
// handleMessage processes a single artifacts.processed message.
func (rs *ResultSubscriber) handleMessage(ctx context.Context, msg jetstream.Msg) {
    // ... unmarshal, validate, HandleProcessedResult ...

    // Best-effort knowledge synthesis (existing — spec 025)
    if rs.KnowledgeEnabled && payload.Success {
        if err := rs.publishSynthesisRequest(ctx, &payload); err != nil {
            slog.Warn("synthesis publish failed (fail-open)", ...)
        }
    }

    // Best-effort domain extraction (new — spec 026)
    if rs.DomainRegistry != nil && payload.Success {
        if err := rs.publishDomainExtractionRequest(ctx, &payload); err != nil {
            slog.Warn("domain extraction publish failed (fail-open)", ...)
        }
    }

    _ = msg.Ack()
}
```

Both synthesis and domain extraction are **fail-open**: errors are logged but never block the ingestion ack.

---

## Data Model

### Migration: `015_domain_extraction.sql`

Next migration after `014_knowledge_layer.sql`:

```sql
-- 015_domain_extraction.sql
-- Domain-Aware Structured Extraction (spec 026).
-- Adds domain_data JSONB column and extraction tracking to artifacts.
--
-- ROLLBACK:
--   DROP INDEX IF EXISTS idx_artifacts_domain_data_gin;
--   DROP INDEX IF EXISTS idx_artifacts_domain_extraction_status;
--   DROP INDEX IF EXISTS idx_artifacts_domain_schema_version;
--   ALTER TABLE artifacts DROP COLUMN IF EXISTS domain_data;
--   ALTER TABLE artifacts DROP COLUMN IF EXISTS domain_extraction_status;
--   ALTER TABLE artifacts DROP COLUMN IF EXISTS domain_schema_version;
--   ALTER TABLE artifacts DROP COLUMN IF EXISTS domain_extracted_at;

-- Structured domain data — recipe ingredients, product specs, etc.
ALTER TABLE artifacts ADD COLUMN IF NOT EXISTS domain_data JSONB;

-- Extraction lifecycle: 'pending', 'completed', 'failed', 'skipped', NULL
-- NULL means no domain schema matched (no extraction attempted).
ALTER TABLE artifacts ADD COLUMN IF NOT EXISTS domain_extraction_status TEXT;

-- Which contract version produced this domain_data (e.g., 'recipe-extraction-v1').
ALTER TABLE artifacts ADD COLUMN IF NOT EXISTS domain_schema_version TEXT;

-- When domain extraction completed.
ALTER TABLE artifacts ADD COLUMN IF NOT EXISTS domain_extracted_at TIMESTAMPTZ;

-- GIN index for containment queries on domain_data.
-- Enables: WHERE domain_data @> '{"ingredients": [{"name": "chicken"}]}'
CREATE INDEX IF NOT EXISTS idx_artifacts_domain_data_gin
    ON artifacts USING gin (domain_data jsonb_path_ops);

-- Partial index for extraction lifecycle tracking.
CREATE INDEX IF NOT EXISTS idx_artifacts_domain_extraction_status
    ON artifacts (domain_extraction_status)
    WHERE domain_extraction_status IN ('pending', 'failed');

-- Index for schema version queries (e.g., re-extraction after schema update).
CREATE INDEX IF NOT EXISTS idx_artifacts_domain_schema_version
    ON artifacts (domain_schema_version)
    WHERE domain_schema_version IS NOT NULL;
```

### Domain Data Shape

The `domain_data` JSONB column stores the validated LLM output verbatim. Its internal structure is defined entirely by the domain contract's `extraction_schema`. Two initial schemas:

**Recipe (`domain_data` example):**
```json
{
  "domain": "recipe",
  "ingredients": [
    {"name": "chicken breast", "quantity": "2", "unit": "lbs", "preparation": "diced", "group": "main"},
    {"name": "olive oil", "quantity": "2", "unit": "tbsp", "preparation": "", "group": "main"}
  ],
  "steps": [
    {"number": 1, "instruction": "Heat olive oil in a large skillet over medium-high heat.", "duration_minutes": 2, "technique": "sauteing"},
    {"number": 2, "instruction": "Add chicken and cook until golden, about 5 minutes.", "duration_minutes": 5, "technique": "sauteing"}
  ],
  "techniques": ["sauteing", "braising"],
  "prep_time_minutes": 15,
  "cook_time_minutes": 30,
  "total_time_minutes": 45,
  "servings": 4,
  "cuisine": "Italian",
  "course": "main",
  "dietary_tags": ["gluten-free"],
  "difficulty": "medium",
  "equipment": ["large skillet", "cutting board"],
  "tips": ["Let chicken rest 5 minutes before slicing"],
  "nutrition_per_serving": {"calories": 350, "protein_g": 42, "carbs_g": 8, "fat_g": 15, "fiber_g": 2}
}
```

**Product (`domain_data` example):**
```json
{
  "domain": "product",
  "product_name": "Sony WH-1000XM5",
  "brand": "Sony",
  "model": "WH-1000XM5",
  "category": "headphones",
  "price": {"amount": 349.99, "currency": "USD"},
  "specs": [
    {"name": "Driver Size", "value": "30mm"},
    {"name": "Battery Life", "value": "30 hours"},
    {"name": "Weight", "value": "250g"}
  ],
  "pros": ["Industry-leading ANC", "Comfortable fit", "Long battery life"],
  "cons": ["No aptX support", "Cannot fold flat"],
  "rating": {"score": 4.5, "max": 5, "count": 12847},
  "availability": "in_stock",
  "comparison_notes": "Successor to XM4 with improved ANC and lighter build"
}
```

Every domain_data object MUST include a top-level `"domain"` field matching the contract name. This enables discriminated queries: `WHERE domain_data->>'domain' = 'recipe'`.

---

## API/Contracts

### Domain Extraction Prompt Contract Format

Domain contracts live alongside existing prompt contracts in `config/prompt_contracts/` and follow the same YAML structure as `ingest-synthesis-v1.yaml`, extended with domain-specific fields:

**`config/prompt_contracts/recipe-extraction-v1.yaml`:**
```yaml
version: "recipe-extraction-v1"
type: "domain-extraction"
description: "Extract structured recipe data from recipe content"

# Content type routing — which content_types trigger this contract
content_types:
  - "recipe"

# Optional URL pattern qualifiers — if present, also match articles from these domains
url_qualifiers:
  - "allrecipes"
  - "epicurious"
  - "foodnetwork"
  - "seriouseats"
  - "bonappetit"
  - "budgetbytes"

# Minimum content length (chars) — skip if content is too short
min_content_length: 200

system_prompt: |
  You are a recipe extraction engine. Extract structured recipe data from the
  provided content. Return ONLY valid JSON matching the output schema below.

  RULES:
  - Extract ALL ingredients with quantities, units, and preparation notes.
  - Group ingredients by section if the recipe has sections (e.g., "for the sauce").
  - Number steps sequentially. Estimate duration per step if not stated.
  - Identify cooking techniques used (e.g., sauteing, braising, blind baking).
  - Extract timing, servings, cuisine, course, difficulty, and equipment.
  - For dietary tags, infer from ingredients (e.g., no meat → vegetarian).
  - For nutrition, extract if stated; omit fields if not available.
  - If a field cannot be determined, use null (not empty string or zero).
  - Do NOT hallucinate ingredients or steps not present in the content.

extraction_schema:
  type: object
  required:
    - domain
    - ingredients
    - steps
  properties:
    domain:
      type: string
      const: "recipe"
    ingredients:
      type: array
      minItems: 1
      items:
        type: object
        required:
          - name
        properties:
          name:
            type: string
            maxLength: 200
          quantity:
            type: string
            maxLength: 50
          unit:
            type: string
            maxLength: 50
          preparation:
            type: string
            maxLength: 200
          group:
            type: string
            maxLength: 100
    steps:
      type: array
      minItems: 1
      items:
        type: object
        required:
          - number
          - instruction
        properties:
          number:
            type: integer
            minimum: 1
          instruction:
            type: string
            maxLength: 1000
          duration_minutes:
            type: integer
            minimum: 0
          technique:
            type: string
            maxLength: 100
    techniques:
      type: array
      items:
        type: string
        maxLength: 100
    prep_time_minutes:
      type: integer
      minimum: 0
    cook_time_minutes:
      type: integer
      minimum: 0
    total_time_minutes:
      type: integer
      minimum: 0
    servings:
      type: integer
      minimum: 1
    cuisine:
      type: string
      maxLength: 100
    course:
      type: string
      maxLength: 50
    dietary_tags:
      type: array
      items:
        type: string
        maxLength: 50
    difficulty:
      type: string
      enum:
        - easy
        - medium
        - hard
    equipment:
      type: array
      items:
        type: string
        maxLength: 100
    tips:
      type: array
      items:
        type: string
        maxLength: 500
    nutrition_per_serving:
      type: object
      properties:
        calories:
          type: number
        protein_g:
          type: number
        carbs_g:
          type: number
        fat_g:
          type: number
        fiber_g:
          type: number

validation_rules:
  max_ingredients: 100
  max_steps: 50
  max_techniques: 20

token_budget: 3000
temperature: 0.2
model_preference: "default"
```

**`config/prompt_contracts/product-extraction-v1.yaml`** follows the same structure with `content_types: ["product"]` and a product-specific `extraction_schema`.

### Contract Format Extension

The existing `PromptContract` struct in [internal/knowledge/contract.go](../../internal/knowledge/contract.go) is reused for loading. Domain contracts add two new YAML fields that the Go loader interprets:

```go
// DomainContract extends PromptContract with routing metadata for domain extraction.
type DomainContract struct {
    *knowledge.PromptContract

    // ContentTypes is the list of content types that trigger this contract.
    ContentTypes []string `yaml:"content_types" json:"content_types"`

    // URLQualifiers are substring patterns matched against artifact source_url.
    // If non-empty, an "article" content_type is also eligible when URL matches.
    URLQualifiers []string `yaml:"url_qualifiers" json:"url_qualifiers"`

    // MinContentLength is the minimum content character count for extraction.
    // Artifacts with shorter content are skipped.
    MinContentLength int `yaml:"min_content_length" json:"min_content_length"`
}
```

The existing `LoadContractsFromDir` function loads all YAML files. Domain contracts are identified by `type: "domain-extraction"` and parsed into `DomainContract` structs by the domain registry.

---

### NATS Topic Design

New subjects added to the existing `ARTIFACTS` stream:

| Subject | Direction | Stream | Purpose |
|---------|-----------|--------|---------|
| `domain.extract` | core → ml | ARTIFACTS | Domain extraction request |
| `domain.extracted` | ml → core | ARTIFACTS | Domain extraction result |

These subjects fit under the `ARTIFACTS` stream's `artifacts.>` pattern. However, since they use the `domain.` prefix, the stream config needs an additional subject: `domain.>`.

**Update to `config/nats_contract.json`:**
```json
{
  "domain.extract": {
    "direction": "core_to_ml",
    "response": "domain.extracted",
    "stream": "ARTIFACTS",
    "critical": false
  },
  "domain.extracted": {
    "direction": "ml_to_core",
    "request": "domain.extract",
    "stream": "ARTIFACTS",
    "critical": false
  }
}
```

And in the streams section, add `"domain.>"` to the ARTIFACTS stream's additional_subjects (or create a dedicated `DOMAIN` stream if subject isolation is preferred):

**Alternative — dedicated DOMAIN stream** (preferred for cleaner isolation):

| Subject | Direction | Stream | Purpose |
|---------|-----------|--------|---------|
| `domain.extract` | core → ml | DOMAIN | Domain extraction request |
| `domain.extracted` | ml → core | DOMAIN | Domain extraction result |

```json
"DOMAIN": { "subjects_pattern": "domain.>" }
```

**Decision: Dedicated DOMAIN stream.** This avoids coupling domain extraction's retention and consumer config to the high-throughput `ARTIFACTS` stream. Domain extraction has different retry semantics (max 3 attempts, fail-open) vs artifact processing (max 5, critical).

**Go constants** in [internal/nats/client.go](../../internal/nats/client.go):
```go
// Domain extraction subjects
SubjectDomainExtract   = "domain.extract"
SubjectDomainExtracted = "domain.extracted"
```

**Python constants** in [ml/app/nats_client.py](../../ml/app/nats_client.py):
```python
SUBSCRIBE_SUBJECTS.append("domain.extract")
PUBLISH_SUBJECTS.append("domain.extracted")
SUBJECT_RESPONSE_MAP["domain.extract"] = "domain.extracted"
```

### NATS Message Schemas

**`domain.extract` (core → ml):**
```go
// DomainExtractRequest is published to domain.extract by the Go core.
type DomainExtractRequest struct {
    ArtifactID      string `json:"artifact_id"`
    ContentType     string `json:"content_type"`
    Title           string `json:"title"`
    Summary         string `json:"summary"`
    ContentRaw      string `json:"content_raw"`
    SourceURL       string `json:"source_url"`
    ContractVersion string `json:"contract_version"`
    RetryCount      int    `json:"retry_count"`
    TraceID         string `json:"trace_id,omitempty"`
}
```

**`domain.extracted` (ml → core):**
```go
// DomainExtractResponse is published to domain.extracted by the ML sidecar.
type DomainExtractResponse struct {
    ArtifactID      string          `json:"artifact_id"`
    Success         bool            `json:"success"`
    Error           string          `json:"error,omitempty"`
    DomainData      json.RawMessage `json:"domain_data,omitempty"`
    ContractVersion string          `json:"contract_version"`
    ProcessingTimeMs int64          `json:"processing_time_ms"`
    ModelUsed       string          `json:"model_used"`
    TokensUsed      int             `json:"tokens_used"`
}
```

`DomainData` is `json.RawMessage` because the Go core does not need to interpret the domain-specific structure — it stores it verbatim in the JSONB column. Validation happens in the ML sidecar against the contract's JSON Schema.

---

## Go Core Implementation

### Domain Registry

New package: `internal/domain/registry.go`

```go
package domain

import (
    "fmt"
    "log/slog"
    "path/filepath"
    "strings"

    "github.com/smackerel/smackerel/internal/knowledge"
)

// Registry maps content types to domain extraction contracts.
type Registry struct {
    // byContentType maps content_type → DomainContract.
    byContentType map[string]*DomainContract

    // byURLPattern maps URL substring → DomainContract for article-type qualifiers.
    byURLPattern []urlPatternEntry

    // all stores all loaded domain contracts keyed by version.
    all map[string]*DomainContract
}

type urlPatternEntry struct {
    pattern  string
    contract *DomainContract
}

// DomainContract extends PromptContract with domain-specific routing metadata.
type DomainContract struct {
    *knowledge.PromptContract
    ContentTypes     []string `yaml:"content_types"`
    URLQualifiers    []string `yaml:"url_qualifiers"`
    MinContentLength int      `yaml:"min_content_length"`
}

// LoadRegistry loads all domain extraction contracts from a directory.
// Contracts are identified by type: "domain-extraction".
func LoadRegistry(contractsDir string) (*Registry, error) {
    allContracts, err := knowledge.LoadContractsFromDir(contractsDir)
    if err != nil {
        return nil, fmt.Errorf("load contracts: %w", err)
    }

    reg := &Registry{
        byContentType: make(map[string]*DomainContract),
        all:           make(map[string]*DomainContract),
    }

    for version, contract := range allContracts {
        if contract.Type != "domain-extraction" {
            continue
        }

        dc := &DomainContract{PromptContract: contract}
        // Parse domain-specific fields from raw YAML (content_types, url_qualifiers, min_content_length)
        // ... (parsed from the YAML alongside PromptContract)

        reg.all[version] = dc

        for _, ct := range dc.ContentTypes {
            if existing, ok := reg.byContentType[ct]; ok {
                return nil, fmt.Errorf("duplicate domain contract for content_type %q: %s and %s",
                    ct, existing.Version, version)
            }
            reg.byContentType[ct] = dc
        }

        for _, pattern := range dc.URLQualifiers {
            reg.byURLPattern = append(reg.byURLPattern, urlPatternEntry{
                pattern:  strings.ToLower(pattern),
                contract: dc,
            })
        }

        slog.Info("loaded domain contract",
            "version", version,
            "content_types", dc.ContentTypes,
            "url_qualifiers", len(dc.URLQualifiers),
        )
    }

    return reg, nil
}

// Match returns the domain contract for an artifact, or nil if none matches.
func (r *Registry) Match(contentType, sourceURL string) *DomainContract {
    if r == nil {
        return nil
    }

    // Direct content_type match
    if dc, ok := r.byContentType[contentType]; ok {
        return dc
    }

    // URL qualifier match (for articles from known domain sites)
    if sourceURL != "" {
        lowerURL := strings.ToLower(sourceURL)
        for _, entry := range r.byURLPattern {
            if strings.Contains(lowerURL, entry.pattern) {
                return entry.contract
            }
        }
    }

    return nil
}

// Count returns the number of loaded domain contracts.
func (r *Registry) Count() int {
    if r == nil {
        return 0
    }
    return len(r.all)
}
```

### Subscriber Extension

New method on `ResultSubscriber` in [internal/pipeline/subscriber.go](../../internal/pipeline/subscriber.go):

```go
// publishDomainExtractionRequest checks if the artifact's content type has a
// registered domain extraction contract and, if so, publishes a domain.extract
// request. Fail-open: errors are returned for logging but must never block ingestion.
func (rs *ResultSubscriber) publishDomainExtractionRequest(ctx context.Context, payload *NATSProcessedPayload) error {
    if rs.DomainRegistry == nil {
        return nil
    }

    // Load artifact content from DB (same pattern as publishSynthesisRequest)
    artifact, err := rs.KnowledgeStore.GetArtifactForSynthesis(ctx, payload.ArtifactID)
    if err != nil {
        return fmt.Errorf("load artifact for domain extraction: %w", err)
    }

    // Match content type against domain registry
    contract := rs.DomainRegistry.Match(artifact.ArtifactType, artifact.SourceURL)
    if contract == nil {
        return nil // No matching domain contract — not an error
    }

    // Check minimum content length
    if contract.MinContentLength > 0 && len(artifact.ContentRaw) < contract.MinContentLength {
        slog.Debug("domain extraction skipped: content too short",
            "artifact_id", payload.ArtifactID,
            "content_len", len(artifact.ContentRaw),
            "min_required", contract.MinContentLength,
        )
        // Mark as skipped
        _, _ = rs.DB.Exec(ctx,
            "UPDATE artifacts SET domain_extraction_status = 'skipped' WHERE id = $1",
            payload.ArtifactID,
        )
        return nil
    }

    // Truncate content for LLM context budget
    contentRaw := artifact.ContentRaw
    if len(contentRaw) > maxSynthesisContentChars {
        contentRaw = stringutil.TruncateUTF8(contentRaw, maxSynthesisContentChars)
    }

    req := &DomainExtractRequest{
        ArtifactID:      payload.ArtifactID,
        ContentType:     artifact.ArtifactType,
        Title:           artifact.Title,
        Summary:         artifact.Summary,
        ContentRaw:      contentRaw,
        SourceURL:       artifact.SourceURL,
        ContractVersion: contract.Version,
        RetryCount:      0,
    }

    data, err := json.Marshal(req)
    if err != nil {
        return fmt.Errorf("marshal domain extract request: %w", err)
    }

    if len(data) > MaxNATSMessageSize {
        return fmt.Errorf("domain extract payload too large: %d bytes", len(data))
    }

    // Mark as pending before publishing
    _, _ = rs.DB.Exec(ctx,
        "UPDATE artifacts SET domain_extraction_status = 'pending', domain_schema_version = $2 WHERE id = $1",
        payload.ArtifactID, contract.Version,
    )

    if err := rs.NATS.Publish(ctx, smacknats.SubjectDomainExtract, data); err != nil {
        return fmt.Errorf("publish domain.extract: %w", err)
    }

    slog.Info("domain extraction requested",
        "artifact_id", payload.ArtifactID,
        "contract", contract.Version,
        "content_type", artifact.ArtifactType,
    )
    return nil
}
```

### Domain Extraction Result Handler

New consumer in `ResultSubscriber.Start()` for `domain.extracted`, following the same pattern as the existing `artifacts.processed` and `digest.generated` consumers:

```go
// domain.extracted consumer
domainConsumer, err := rs.NATS.JetStream.CreateOrUpdateConsumer(ctx, "DOMAIN", jetstream.ConsumerConfig{
    Durable:       "smackerel-core-domain",
    FilterSubject: smacknats.SubjectDomainExtracted,
    AckPolicy:     jetstream.AckExplicitPolicy,
    MaxDeliver:    3, // Lower than artifacts — domain extraction is non-critical
    AckWait:       30 * time.Second,
})
```

Handler:

```go
// handleDomainMessage processes a single domain.extracted message.
func (rs *ResultSubscriber) handleDomainMessage(ctx context.Context, msg jetstream.Msg) {
    var resp DomainExtractResponse
    if err := json.Unmarshal(msg.Data(), &resp); err != nil {
        slog.Error("invalid domain.extracted payload", "error", err)
        _ = msg.Ack()
        return
    }

    if resp.ArtifactID == "" {
        slog.Error("domain.extracted: missing artifact_id")
        _ = msg.Ack()
        return
    }

    if !resp.Success {
        _, err := rs.DB.Exec(ctx, `
            UPDATE artifacts SET
                domain_extraction_status = 'failed',
                domain_extracted_at = NOW(),
                updated_at = NOW()
            WHERE id = $1
        `, resp.ArtifactID)
        if err != nil {
            slog.Error("update domain extraction failure", "artifact_id", resp.ArtifactID, "error", err)
        }
        slog.Warn("domain extraction failed",
            "artifact_id", resp.ArtifactID,
            "error", resp.Error,
        )
        _ = msg.Ack()
        return
    }

    // Store validated domain data
    _, err := rs.DB.Exec(ctx, `
        UPDATE artifacts SET
            domain_data = $2,
            domain_extraction_status = 'completed',
            domain_schema_version = $3,
            domain_extracted_at = NOW(),
            updated_at = NOW()
        WHERE id = $1
    `, resp.ArtifactID, resp.DomainData, resp.ContractVersion)
    if err != nil {
        slog.Error("store domain extraction result",
            "artifact_id", resp.ArtifactID,
            "error", err,
        )
        _ = msg.Nak()
        return
    }

    _ = msg.Ack()
    slog.Info("domain extraction complete",
        "artifact_id", resp.ArtifactID,
        "contract", resp.ContractVersion,
        "processing_ms", resp.ProcessingTimeMs,
        "model", resp.ModelUsed,
    )
}
```

---

## ML Sidecar Implementation

### New Handler: `ml/app/domain.py`

```python
"""Domain extraction consumer for the ML sidecar.

Subscribes to `domain.extract`, processes artifacts through domain-specific
LLM extraction using prompt contracts, validates output against JSON Schema,
and publishes results to `domain.extracted`.
"""

import json
import logging
import time
from typing import Any

import litellm
from litellm.exceptions import (
    InternalServerError,
    RateLimitError,
    ServiceUnavailableError,
)

from .synthesis import load_prompt_contract, validate_extraction, truncate_content

logger = logging.getLogger("smackerel-ml.domain")

MAX_DOMAIN_CONTENT_CHARS = 8000


def build_domain_prompt(contract: dict, artifact: dict) -> str:
    """Build the domain extraction LLM prompt from contract and artifact data."""
    parts = []

    system_prompt = contract.get("system_prompt", "")
    if system_prompt:
        parts.append(system_prompt)

    parts.append("\n--- CONTENT TO EXTRACT FROM ---")
    parts.append(f"Title: {artifact.get('title', 'Untitled')}")
    parts.append(f"Content Type: {artifact.get('content_type', 'unknown')}")

    if artifact.get("summary"):
        parts.append(f"Summary: {artifact['summary']}")

    content = artifact.get("content_raw", "")
    if content:
        content = truncate_content(content, MAX_DOMAIN_CONTENT_CHARS)
        parts.append(f"\nFull Content:\n{content}")

    schema = contract.get("extraction_schema", {})
    parts.append(
        f"\n--- OUTPUT FORMAT ---\nReturn ONLY valid JSON matching this schema:\n"
        f"{json.dumps(schema, indent=2)}"
    )

    return "\n".join(parts)


async def handle_domain_extract(
    data: dict,
    provider: str,
    model: str,
    api_key: str,
    ollama_url: str,
) -> dict:
    """Handle a domain.extract request."""
    artifact_id = data.get("artifact_id", "")
    contract_version = data.get("contract_version", "")
    start = time.time()

    try:
        contract = load_prompt_contract(contract_version)
    except (FileNotFoundError, ValueError) as e:
        logger.error("Domain contract load failed: %s", e)
        return {
            "artifact_id": artifact_id,
            "success": False,
            "error": str(e),
            "contract_version": contract_version,
            "processing_time_ms": _elapsed_ms(start),
        }

    prompt = build_domain_prompt(contract, data)
    model_name = f"{provider}/{model}" if provider not in ("openai", "") else model

    # Retry with exponential backoff (max 2 retries = 3 total attempts)
    max_attempts = 3
    backoff_delays = [1, 3]
    last_exc = None
    response = None

    for attempt in range(max_attempts):
        try:
            import asyncio

            response = await litellm.acompletion(
                model=model_name,
                messages=[{"role": "user", "content": prompt}],
                api_key=api_key,
                temperature=contract.get("temperature", 0.2),
                max_tokens=contract.get("token_budget", 3000),
                response_format={"type": "json_object"},
                timeout=30,
            )
            break
        except (RateLimitError, ServiceUnavailableError, InternalServerError) as exc:
            last_exc = exc
            if attempt < max_attempts - 1:
                delay = backoff_delays[attempt]
                logger.warning(
                    "Domain extraction LLM call failed (attempt %d/%d): %s",
                    attempt + 1, max_attempts, exc,
                )
                await asyncio.sleep(delay)

    if response is None:
        return {
            "artifact_id": artifact_id,
            "success": False,
            "error": f"LLM call failed after {max_attempts} attempts: {last_exc}",
            "contract_version": contract_version,
            "processing_time_ms": _elapsed_ms(start),
        }

    try:
        result_text = response.choices[0].message.content
        domain_data = json.loads(result_text)
    except (json.JSONDecodeError, IndexError, AttributeError) as e:
        return {
            "artifact_id": artifact_id,
            "success": False,
            "error": f"Invalid JSON from LLM: {e}",
            "contract_version": contract_version,
            "processing_time_ms": _elapsed_ms(start),
        }

    # Validate against contract's JSON Schema
    schema = contract.get("extraction_schema", {})
    valid, error_msg = validate_extraction(domain_data, schema)
    if not valid:
        return {
            "artifact_id": artifact_id,
            "success": False,
            "error": error_msg,
            "contract_version": contract_version,
            "processing_time_ms": _elapsed_ms(start),
        }

    tokens_used = response.usage.total_tokens if response.usage else 0

    return {
        "artifact_id": artifact_id,
        "success": True,
        "domain_data": domain_data,
        "contract_version": contract_version,
        "processing_time_ms": _elapsed_ms(start),
        "model_used": model_name,
        "tokens_used": tokens_used,
    }


def _elapsed_ms(start: float) -> int:
    return int((time.time() - start) * 1000)
```

### NATS Client Registration

In [ml/app/nats_client.py](../../ml/app/nats_client.py), add `domain.extract` to `SUBSCRIBE_SUBJECTS`, `domain.extracted` to `PUBLISH_SUBJECTS`, and `"domain.extract": "domain.extracted"` to `SUBJECT_RESPONSE_MAP`.

In the message dispatch handler (wherever subjects are routed to handlers), add:

```python
elif subject == "domain.extract":
    from .domain import handle_domain_extract
    result = await handle_domain_extract(data, provider, model, api_key, ollama_url)
```

---

## Search Extension

### Domain-Aware Query Detection

The search engine in [internal/api/search.go](../../internal/api/search.go) is extended with domain-attribute intent detection. This runs before embedding, at the same stage as `parseTemporalIntent`:

```go
// DomainIntent captures a detected domain-specific query intent.
type DomainIntent struct {
    Domain       string   // "recipe", "product", etc.
    Attributes   []string // searched attributes (e.g., ["chicken", "garlic"])
    PriceMax     float64  // product price ceiling (0 = no filter)
    PriceCurrency string  // "USD", etc.
    Cleaned      string   // query with domain markers removed for embedding
}

// parseDomainIntent detects domain-specific query patterns.
func parseDomainIntent(query string) *DomainIntent {
    lower := strings.ToLower(query)

    // Recipe ingredient patterns: "recipes with X", "recipes containing X and Y"
    recipePatterns := []string{
        `recipes?\s+(?:with|containing|using|that (?:have|use|include))\s+(.+)`,
        `(?:something|dishes?|meals?)\s+with\s+(.+?)(?:\s+for\s+|$)`,
    }
    for _, pattern := range recipePatterns {
        re := regexp.MustCompile(pattern)
        if m := re.FindStringSubmatch(lower); len(m) > 1 {
            ingredients := parseIngredientList(m[1])
            return &DomainIntent{
                Domain:     "recipe",
                Attributes: ingredients,
                Cleaned:    query,
            }
        }
    }

    // Product price patterns: "cameras under $500", "headphones below 200"
    pricePattern := regexp.MustCompile(`(.+?)\s+(?:under|below|less than|cheaper than)\s+\$?(\d+(?:\.\d+)?)`)
    if m := pricePattern.FindStringSubmatch(lower); len(m) > 2 {
        price, _ := strconv.ParseFloat(m[2], 64)
        return &DomainIntent{
            Domain:       "product",
            Attributes:   []string{strings.TrimSpace(m[1])},
            PriceMax:     price,
            PriceCurrency: "USD",
            Cleaned:      query,
        }
    }

    return nil
}
```

### JSONB Query Augmentation

When a `DomainIntent` is detected, the search augments the SQL query with JSONB conditions:

```go
// addDomainFilters augments a vector search query with domain-specific JSONB conditions.
func addDomainFilters(query string, args []any, argN int, intent *DomainIntent) (string, []any, int) {
    // Require domain_data to exist and match the domain
    query += fmt.Sprintf(" AND domain_data->>'domain' = $%d", argN)
    args = append(args, intent.Domain)
    argN++

    switch intent.Domain {
    case "recipe":
        // Search ingredients by name using JSONB containment
        for _, ingredient := range intent.Attributes {
            query += fmt.Sprintf(` AND EXISTS (
                SELECT 1 FROM jsonb_array_elements(domain_data->'ingredients') AS ing
                WHERE LOWER(ing->>'name') LIKE '%%' || LOWER($%d) || '%%'
            )`, argN)
            args = append(args, ingredient)
            argN++
        }

    case "product":
        if intent.PriceMax > 0 {
            query += fmt.Sprintf(` AND (domain_data->'price'->>'amount')::numeric <= $%d`, argN)
            args = append(args, intent.PriceMax)
            argN++
        }
    }

    return query, args, argN
}
```

### Combined Search Flow

The search pipeline becomes:

1. Parse temporal intent (existing)
2. Parse domain intent (new)
3. If domain intent detected → run domain-augmented vector search
4. If no domain results → fall back to pure semantic search
5. Graph expansion + re-ranking (existing)

Domain-matched results are boosted: their similarity score receives a +0.15 bonus, capped at 1.0, to rank domain-structural matches above pure semantic matches (BS-006).

### Search API Extension

New filter fields on `SearchFilters`:

```go
type SearchFilters struct {
    Type       string `json:"type,omitempty"`
    DateFrom   string `json:"date_from,omitempty"`
    DateTo     string `json:"date_to,omitempty"`
    Person     string `json:"person,omitempty"`
    Topic      string `json:"topic,omitempty"`
    Domain     string `json:"domain,omitempty"`     // explicit domain filter
    Ingredient string `json:"ingredient,omitempty"` // recipe ingredient search
}
```

New field on `SearchResult`:

```go
type SearchResult struct {
    // ... existing fields ...
    DomainData json.RawMessage `json:"domain_data,omitempty"` // domain-extracted data when present
}
```

---

## Telegram Display

### Domain-Enriched Artifact Rendering

The Telegram bot's format layer in [internal/telegram/format.go](../../internal/telegram/format.go) is extended to render domain cards using the text marker system (no emoji):

```go
// formatDomainCard renders domain-specific data for Telegram display.
func formatDomainCard(domainData json.RawMessage) string {
    if len(domainData) == 0 {
        return ""
    }

    var base struct {
        Domain string `json:"domain"`
    }
    if err := json.Unmarshal(domainData, &base); err != nil {
        return ""
    }

    switch base.Domain {
    case "recipe":
        return formatRecipeCard(domainData)
    case "product":
        return formatProductCard(domainData)
    default:
        return ""
    }
}

// formatRecipeCard renders a recipe summary for Telegram.
func formatRecipeCard(data json.RawMessage) string {
    var recipe struct {
        Ingredients []struct {
            Name     string `json:"name"`
            Quantity string `json:"quantity"`
            Unit     string `json:"unit"`
        } `json:"ingredients"`
        TotalTimeMinutes int      `json:"total_time_minutes"`
        Servings         int      `json:"servings"`
        Cuisine          string   `json:"cuisine"`
        Difficulty       string   `json:"difficulty"`
        DietaryTags      []string `json:"dietary_tags"`
    }
    if err := json.Unmarshal(data, &recipe); err != nil {
        return ""
    }

    var b strings.Builder
    b.WriteString(MarkerHeading + "Recipe Details\n")

    if recipe.TotalTimeMinutes > 0 {
        b.WriteString(fmt.Sprintf(MarkerInfo+"Time: %d min", recipe.TotalTimeMinutes))
        if recipe.Servings > 0 {
            b.WriteString(fmt.Sprintf(" | Serves: %d", recipe.Servings))
        }
        b.WriteString("\n")
    }
    if recipe.Cuisine != "" {
        b.WriteString(MarkerInfo + "Cuisine: " + recipe.Cuisine)
        if recipe.Difficulty != "" {
            b.WriteString(" | " + recipe.Difficulty)
        }
        b.WriteString("\n")
    }
    if len(recipe.DietaryTags) > 0 {
        b.WriteString(MarkerInfo + "Diet: " + strings.Join(recipe.DietaryTags, ", ") + "\n")
    }

    if len(recipe.Ingredients) > 0 {
        b.WriteString(MarkerHeading + "Ingredients\n")
        limit := 10 // Show first 10, truncate
        for i, ing := range recipe.Ingredients {
            if i >= limit {
                b.WriteString(fmt.Sprintf(MarkerContinued+"... and %d more\n", len(recipe.Ingredients)-limit))
                break
            }
            if ing.Quantity != "" {
                b.WriteString(fmt.Sprintf(MarkerListItem+"%s %s %s\n", ing.Quantity, ing.Unit, ing.Name))
            } else {
                b.WriteString(MarkerListItem + ing.Name + "\n")
            }
        }
    }

    return b.String()
}

// formatProductCard renders a product summary for Telegram.
func formatProductCard(data json.RawMessage) string {
    var product struct {
        ProductName string `json:"product_name"`
        Brand       string `json:"brand"`
        Price       struct {
            Amount   float64 `json:"amount"`
            Currency string  `json:"currency"`
        } `json:"price"`
        Rating struct {
            Score float64 `json:"score"`
            Max   float64 `json:"max"`
        } `json:"rating"`
        Pros []string `json:"pros"`
        Cons []string `json:"cons"`
    }
    if err := json.Unmarshal(data, &product); err != nil {
        return ""
    }

    var b strings.Builder
    b.WriteString(MarkerHeading + "Product Details\n")

    if product.Brand != "" {
        b.WriteString(MarkerInfo + "Brand: " + product.Brand + "\n")
    }
    if product.Price.Amount > 0 {
        b.WriteString(fmt.Sprintf(MarkerInfo+"Price: %s %.2f\n", product.Price.Currency, product.Price.Amount))
    }
    if product.Rating.Score > 0 {
        b.WriteString(fmt.Sprintf(MarkerInfo+"Rating: %.1f/%.0f\n", product.Rating.Score, product.Rating.Max))
    }

    if len(product.Pros) > 0 {
        b.WriteString(MarkerHeading + "Pros\n")
        for _, pro := range product.Pros[:min(5, len(product.Pros))] {
            b.WriteString(MarkerListItem + pro + "\n")
        }
    }
    if len(product.Cons) > 0 {
        b.WriteString(MarkerHeading + "Cons\n")
        for _, con := range product.Cons[:min(3, len(product.Cons))] {
            b.WriteString(MarkerListItem + con + "\n")
        }
    }

    return b.String()
}
```

---

## Schema Extensibility — End-to-End Workflow

Adding a new domain (e.g., `travel-extraction-v1`) requires **zero code changes**:

1. **Create YAML file:** `config/prompt_contracts/travel-extraction-v1.yaml` with `type: "domain-extraction"`, `content_types: ["article"]`, `url_qualifiers: ["lonelyplanet", "tripadvisor"]`, system_prompt, and extraction_schema.

2. **Restart service:** `./smackerel.sh down && ./smackerel.sh up`. The Go core's `LoadRegistry` picks up the new file. The ML sidecar's `load_prompt_contract` resolves it by version name.

3. **New artifacts auto-process:** When a new article from lonelyplanet.com is captured, `Registry.Match()` returns the travel contract. The pipeline publishes to `domain.extract` with `contract_version: "travel-extraction-v1"`.

4. **Backfill existing artifacts:** A CLI command or scheduled job can query `SELECT id FROM artifacts WHERE artifact_type = 'article' AND source_url LIKE '%lonelyplanet%' AND domain_data IS NULL` and re-publish extraction requests.

5. **Search works automatically:** If domain intent patterns are registered for the new domain, domain-augmented search applies. For generic domain fields, the GIN index on `domain_data` supports ad-hoc containment queries.

No Go compilation, no Python code changes, no migration needed.

---

## Security/Compliance

### Input Validation

| Boundary | Validation |
|----------|------------|
| Contract loading | Path traversal prevention (same as `synthesis.py`: `os.path.basename(version) != version`) |
| NATS publish | Payload size check against `MaxNATSMessageSize` (1MB) |
| NATS receive | `json.Unmarshal` + required field validation before DB write |
| Domain data storage | Validated against JSON Schema in ML sidecar before returning success |
| Content truncation | `stringutil.TruncateUTF8` for rune-safe truncation (no partial UTF-8) |
| Search JSONB queries | Parameterized queries (`$N` placeholders) — no SQL injection via domain_data paths |
| LLM prompt injection | Content is placed in a delimited section (`--- CONTENT TO EXTRACT FROM ---`), not interpolated into instructions |

### Data Privacy

Domain-extracted data inherits the same access controls as the parent artifact. No separate permission model needed. The `domain_data` JSONB field is covered by existing artifact-level access patterns.

### Content Length Limits

| Field | Limit | Rationale |
|-------|-------|-----------|
| `content_raw` to LLM | 8,000 chars | LLM context budget |
| NATS payload | 1 MB | NATS server default |
| `domain_data` JSONB | No explicit limit | Bounded by LLM token budget (3000 tokens ≈ 12KB JSON) |

---

## Observability

### Structured Logging

All domain extraction events use `slog` with consistent fields:

| Event | Level | Fields |
|-------|-------|--------|
| Domain extraction requested | Info | `artifact_id`, `contract`, `content_type` |
| Domain extraction skipped (short content) | Debug | `artifact_id`, `content_len`, `min_required` |
| Domain extraction skipped (no contract match) | — | Silent (not logged — happens for most artifacts) |
| Domain extraction complete | Info | `artifact_id`, `contract`, `processing_ms`, `model` |
| Domain extraction failed | Warn | `artifact_id`, `contract`, `error` |
| Domain contract loaded | Info | `version`, `content_types`, `url_qualifiers` |
| Domain data stored | Debug | `artifact_id`, `schema_version` |

### Metrics (future)

When Prometheus metrics are added:

- `smackerel_domain_extractions_total{contract, status}` — counter by contract version and status (completed/failed/skipped)
- `smackerel_domain_extraction_duration_ms{contract}` — histogram of processing time
- `smackerel_domain_extraction_tokens{contract}` — counter of tokens consumed

### Health Check Extension

The `/health` endpoint's detailed status should include:

```json
{
  "domain_registry": {
    "contracts_loaded": 2,
    "content_types": ["recipe", "product"]
  }
}
```

---

## Testing Strategy

### Unit Tests

| Test | File | What it validates |
|------|------|-------------------|
| `TestDomainRegistryLoad` | `internal/domain/registry_test.go` | Loads YAML contracts, builds content_type index, rejects duplicates |
| `TestDomainRegistryMatch` | `internal/domain/registry_test.go` | Direct match by content_type, URL qualifier match, no-match returns nil |
| `TestDomainExtractRequest_Validate` | `internal/pipeline/domain_types_test.go` | Required fields: artifact_id, content_type, contract_version |
| `TestDomainExtractResponse_Validate` | `internal/pipeline/domain_types_test.go` | Required fields: artifact_id; success=false allows empty domain_data |
| `TestPublishDomainExtractionRequest` | `internal/pipeline/subscriber_test.go` | Publishes when contract matches, skips when no match, skips when content too short |
| `TestHandleDomainMessage_Success` | `internal/pipeline/subscriber_test.go` | Stores domain_data, sets status=completed, acks |
| `TestHandleDomainMessage_Failure` | `internal/pipeline/subscriber_test.go` | Sets status=failed, acks (no infinite retry) |
| `TestParseDomainIntent` | `internal/api/search_test.go` | Recipe ingredient detection, product price detection, no false positives |
| `TestAddDomainFilters` | `internal/api/search_test.go` | Correct JSONB SQL generation for recipe ingredients, product price |
| `TestFormatRecipeCard` | `internal/telegram/format_test.go` | Renders ingredients, timing, dietary tags; truncates long lists |
| `TestFormatProductCard` | `internal/telegram/format_test.go` | Renders brand, price, rating, pros/cons |
| `test_build_domain_prompt` | `ml/tests/test_domain.py` | Prompt includes system_prompt, content, schema |
| `test_handle_domain_extract_success` | `ml/tests/test_domain.py` | Returns validated domain_data on valid LLM response |
| `test_handle_domain_extract_invalid_json` | `ml/tests/test_domain.py` | Returns error when LLM returns non-JSON |
| `test_handle_domain_extract_schema_invalid` | `ml/tests/test_domain.py` | Returns error when LLM output fails JSON Schema |

### Integration Tests

| Test | What it validates |
|------|-------------------|
| Recipe end-to-end | Capture recipe URL → universal processing → domain extraction → domain_data in DB → search by ingredient returns it |
| Product end-to-end | Capture product URL → domain extraction → domain_data → search by price filter |
| No-match passthrough | Capture article URL → no domain extraction → domain_extraction_status is NULL |
| Re-extraction | Update contract version → re-process → domain_data updated, domain_schema_version bumped |

### E2E Tests

| Test | Scenario |
|------|----------|
| BS-001 | Send recipe URL via Telegram → verify structured ingredients in response |
| BS-004 | Send news article → verify no domain_data populated |
| BS-005 | Send recipe URL with minimal content → verify graceful failure/skip |
| BS-006 | Search "recipes with chicken" across 10+ recipe artifacts → verify ingredient-matched results rank first |

### Contract Tests

| Test | What it validates |
|------|-------------------|
| NATS contract alignment | `domain.extract` and `domain.extracted` exist in `nats_contract.json` with correct stream/direction |
| Prompt contract validity | All `config/prompt_contracts/*-extraction-*.yaml` files load without error and have valid JSON Schema |

---

## Risks & Open Questions

### Risks

| Risk | Impact | Likelihood | Mitigation |
|------|--------|------------|------------|
| LLM hallucination in domain extraction | Medium — fabricated ingredients/prices in domain_data | Medium | JSON Schema validation rejects structurally wrong output; content-accuracy is hard to gate automatically. Confidence-based filtering in future. |
| Token cost increase | Low — domain extraction is an additional LLM call per eligible artifact | Medium | Only eligible artifacts (recipe/product content types) get domain extraction. Token budget per contract is capped (3000 tokens). |
| Schema evolution | Medium — changing a domain contract schema after artifacts are stored creates version mismatch | Low | `domain_schema_version` column tracks which version produced the data. Re-extraction backfill job can update to new schema. |
| URL qualifier false positives | Low — an article from a recipe site that isn't a recipe gets recipe extraction | Low | Content type detection in `extract.go` is the primary signal; URL qualifiers are secondary. LLM will return minimal/null fields for non-recipe content, which passes schema validation but has little value — acceptable. |
| JSONB query performance at scale | Medium — GIN index on domain_data may degrade with millions of rows and complex containment queries | Low (initially) | GIN `jsonb_path_ops` index is optimized for containment. Monitor query plans. Add partial indexes per domain type if needed: `CREATE INDEX ... ON artifacts USING gin (domain_data jsonb_path_ops) WHERE domain_data->>'domain' = 'recipe'`. |

### Open Questions

1. **Backfill mechanism:** Should there be a CLI command (`./smackerel.sh domain backfill --contract recipe-extraction-v1`) to re-process existing artifacts that now match a new or updated contract? Recommended: yes, as a Scope 3 item.

2. **Contract hot-reload vs restart:** The spec says "restart the service" to pick up new contracts. Should we support filesystem watching for hot-reload without restart? Recommended: defer to a future improvement — restart is acceptable for schema changes.

3. **Multi-domain artifacts:** Can an artifact match multiple domain contracts (e.g., a recipe that is also a product review)? Current design: first match wins (by content_type priority, then URL qualifier). Recommended: defer multi-domain support.

4. **Domain-specific embedding:** Should domain-extracted fields (ingredients, specs) be embedded separately for domain-attribute search? Current design uses JSONB containment queries on top of existing vector search. Dedicated domain embeddings would improve recall but add complexity and storage. Recommended: evaluate after initial deployment based on search quality metrics.

5. **Extraction confidence scores:** Should the LLM return per-field confidence scores? Current design relies on JSON Schema pass/fail. Recommended: add optional confidence field in a later contract version.
