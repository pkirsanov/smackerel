# Product Direction Surfaces

> **Status**: Convention adopted across Bubbles-installed product repos as of 2026-05-07.
> **Owners**: `bubbles.bootstrap`, `bubbles.setup`, `bubbles.repo-readiness` skill, future `bubbles.releases` agent.
> **Repos validated against this convention**: pattern adopted across multiple downstream Bubbles installations.

## Why This Convention Exists

Every product repo eventually needs to express **strategic direction** (not just engineering convention). Without a shared convention, each repo invents its own structure: some have an `INVESTOR_OVERVIEW.md` at the repo root, some have it under `docs/`, some have a `Product-Principles.md`, some embed principles inside the constitution, some have neither.

This drift creates two problems:

1. **Cross-repo agents** (a future `bubbles.releases` agent that produces a release packet from any installed repo, or a `bubbles.repo-readiness` skill that audits whether a repo is investor/operator-ready) cannot assume a stable file layout. They have to fish for direction signals.
2. **Human readers** (owner, investor, new contributor) have no consistent entry point for "what is this product, where is it going, what principles govern it?"

The Product Direction Surfaces convention fixes this by mandating a canonical 3-file trio in every Bubbles-installed product repo and a canonical phase model.

## The Canonical Trio (MANDATORY in every product repo)

Every product repo MUST have:

### 1. `docs/INVESTOR_OVERVIEW.md`

- **Audience**: Investor, operator, owner, new contributor
- **Content**:
  - Reading order pointing into the rest of the trio + release packets + design docs
  - Executive summary (1-3 sentences capturing the product thesis)
  - Phase Overview table (every shipping phase with status: ✅ delivered / 🔜 in progress / ⏳ planned)
  - Per-phase detail sections (Goal, Key Capabilities, Exit Criteria) for every phase in the table
  - Risk assessment table
  - Capital requirements summary
  - Strategic recommendations (3-7 bullet-ranked priorities)
  - "What's actually working today" section (factual current-state)
  - Documentation map pointing into release packets / design docs
- **Location**: `docs/INVESTOR_OVERVIEW.md` (NOT repo root — keeps repo-root README focused on developer onboarding)
- **Source-of-truth posture**: Capability claims MUST be cross-referenced against the canonical execution-state source for that repo (capability ledger, committed specs, design doc operational runbook). Never make standalone capability claims here.

### 2. `docs/Product-Principles.md`

- **Audience**: Owner, contributor, anyone making a product decision
- **Content**:
  - Reference to engineering principles (constitution / `agent-common.md`) — this doc does NOT duplicate engineering rules
  - Numbered list of product-level principles (1-N)
  - Each principle: Status (Surfaced for owner approval / Ratified YYYY-MM-DD), one-paragraph statement, source evidence trace, what it means for product decisions
  - "Surfacing Process" table mapping each principle to source documents that justify the surfacing
  - "Ratification Process" steps (how a Surfaced principle becomes Ratified)
  - Links to companion enforcement file (#3 below) and constitution
- **Status discipline**: Every surfaced principle MUST be flagged "Surfaced for owner approval — not yet ratified" until the owner explicitly ratifies. NO fabrication: surface only from existing repo evidence.
- **Already-ratified principles**: When the constitution or another binding doc already encodes a principle, mark it "Already ratified" with the source citation.

### 3. `.github/instructions/product-principles.instructions.md`

- **Audience**: Bubbles agents, PR review, CI hooks
- **applyTo**: `**`
- **Content**:
  - Per-principle enforcement section
  - Each principle: Status (Advisory until ratified / Already enforced via X / BLOCKING), grep checks for forbidden patterns, blocking patterns
  - Spec authoring rule: every spec touching the principle area MUST include a Product Principle Alignment section
  - Pre-Ratification Checklist (so the owner has a clear path from Surfaced → Ratified → BLOCKING)
- **Activation discipline**:
  - Until the corresponding principle is ratified in `Product-Principles.md`, enforcement is advisory
  - After ratification, enforcement becomes blocking and the markers MUST be updated

## Phase Model (REQUIRED for multi-phase repos)

Repos with a multi-phase release roadmap MUST express the phase model in the trio. The phase numbering and naming is per-repo and reflects actual product reality. Common patterns observed across downstream installations:

| Repo Pattern | Example Phase Model |
|--------------|---------------------|
| Pre-MVP-driven product | golden-mvp → mvp → v1.0 → v1.1 → v1.5 → v1.6 → v2.0 → v3.0 |
| Quant / regulated product | pre-mvp → mvp → v1.0 → v2.0 → v3.0 |
| Single-host → platform product | MVP (Single-Host Product Core) → v1.0 (Commercial Hardening) → v2.0 (Platform Control Plane) → v3.0 (Ecosystem track-based) |
| Phase-numbered ingestion-style product | Phase 1 Foundation → Phase 2 Passive Ingestion → Phase 3 Intelligence → Phase 4 Expansion → Phase 5 Advanced Intelligence |

The phase model lives in:
- `INVESTOR_OVERVIEW.md` Phase Overview table (canonical)
- Release packet structure: `docs/releases/<phase>/{vision,features,actions,business-plan,deployment,marketing,monetization,ops-scalability}.md` (8-doc standard packet — when the repo has a release-packet structure)
- Plans structure: `docs/plans/<phase>/P0N-<plan-name>.md` (when the repo has a plans-and-features split structure)

**NOT every repo needs both `docs/releases/` and `docs/plans/`.** A repo with mature release packets that already encode planning detail does NOT need a parallel `docs/plans/` (would create duplication). The convention is "one canonical planning structure per repo," not "always have both." Decide consciously and document the choice in the repo's `INVESTOR_OVERVIEW.md` or `docs/releases/README.md`.

## Carry-Forward And Vision Restatement (Multi-Phase Repos)

When a repo has multiple shipped or planned phases:

1. **Carry-forward table**: Each phase's `features.md` MUST include a "Carried Forward From Prior Phases" table listing prior-phase capabilities that remain in scope, with status preserved. This avoids the "did v2.0 quietly drop v1.0 features?" question.
2. **Vision restatement**: Each phase's `vision.md` MUST restate the product vision in the context of that phase. Avoid "see vision.md in v1.0" cross-references — restate inline so each phase's vision is self-contained.

Both apply only to repos with multi-phase release packets.

## Cross-Product Surfaces

When two products integrate (e.g., a personal-knowledge product paired with a domain-specific companion product, or a quant product paired with a research-context companion):

- The integration MUST be expressed as plans in BOTH repos with cross-references
- Schema versioning MUST be coordinated; cross-repo PRs are required for schema changes
- Each repo's Product-Principles.md MUST encode any cross-product boundary as an explicit principle (typically: "Companion Boundary" principle in one repo and a corresponding consumption principle in the other)
- Each repo's `product-principles.instructions.md` MUST encode the cross-product enforcement at the same severity as in-repo principles

## Enforcement

`bubbles.bootstrap` and `bubbles.setup` MUST verify trio presence when a Bubbles install is invoked against a product repo:

```bash
test -f docs/INVESTOR_OVERVIEW.md && \
test -f docs/Product-Principles.md && \
test -f .github/instructions/product-principles.instructions.md
```

Missing trio members trigger an interactive owner consult (per `bubbles.bootstrap` proposal-then-apply discipline) before adoption proceeds.

`bubbles.repo-readiness` skill includes a "trio present + non-stub" check in its readiness audit.

The future `bubbles.releases` agent assumes the trio exists; if missing, it routes to `bubbles.bootstrap` first.

## What This Convention Does NOT Do

- Does NOT mandate a specific phase taxonomy (per-repo)
- Does NOT mandate `docs/releases/` AND `docs/plans/` (mature release packets are sufficient)
- Does NOT mandate principle count or specific principle text (per-repo product reality)
- Does NOT replace the constitution as the engineering authority (that authority remains NON-NEGOTIABLE)
- Does NOT replace `agent-common.md` or `scope-workflow.md` as the universal agent governance (those remain authoritative)

## Adoption Pattern (Validation Set As Of Codification)

| Repo Profile | Trio Present | Phase Model | Notes |
|--------------|--------------|-------------|-------|
| Pre-MVP-driven product (8 phases) | ✅ Yes | 8 phases | Reference implementation |
| Quant / regulated product (5 phases) | ✅ Yes | 5 phases | Release packets are canonical planning structure (no parallel `docs/plans/`) |
| Single-host → platform product (4 phases) | ✅ Yes | 4 phases | Capability ledger is the truth source for current-state claims |
| Phase-numbered ingestion product (5 phases) | ✅ Yes | 5 phases | Phase model lives in design doc, mirrored in INVESTOR_OVERVIEW.md |

## Change Log

- 2026-05-07: Convention codified after a cross-project standardization pass across multiple downstream installations.
