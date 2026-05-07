---
name: bubbles-product-principle-discovery
description: Surface product principles from existing repo evidence (constitution, design docs, capability ledger, README) without fabrication. Use when bootstrapping the Product Direction Surfaces trio in a product repo, or when refreshing an existing Product-Principles.md with evidence-cited surfaced principles.
---

# Product Principle Discovery

## Goal
Help an owner produce a `Product-Principles.md` whose surfaced principles are entirely traceable to existing repo evidence — never fabricated.

## Portability
Portable governance skill. Project-agnostic.

## Non-Negotiables
- NEVER fabricate a principle. Every surfaced principle MUST cite source evidence.
- Surfaced principles MUST be flagged "Surfaced for owner approval — not yet ratified" until the owner explicitly ratifies.
- Engineering principles already encoded in the constitution or another binding doc MUST be marked "Already ratified" with the source citation, not re-surfaced as new.
- Cross-product principles (when two products integrate) MUST be encoded in BOTH repos' `Product-Principles.md` with matching wording.

## When To Use
- Bootstrapping the Product Direction Surfaces trio in a fresh product repo
- Refreshing an existing `Product-Principles.md` after major product evolution
- Cross-product integration work that introduces a new shared boundary principle
- Owner consult sessions that need a structured way to surface direction without inventing it

## Inputs This Skill Reads
- `.specify/memory/constitution.md` (engineering authority — already-ratified principles live here)
- `docs/<product-design-doc>.md` (product design — primary surfacing source)
- `docs/Capability_Ledger.md` (when present — actual current-state truth)
- `README.md` (positioning + target audience signals)
- Any `docs/releases/<phase>/vision.md` (per-phase intent)
- Any `docs/plans/<phase>/` (active planning context)
- Cross-product specs (e.g., `specs/<NNN-cross-product-bridge>/`)
- Existing `.github/instructions/*.md` (already-binding policies that hint at unstated principles)

## Surfacing Workflow

### Step 1 — Inventory Already-Ratified
Read the constitution and binding instruction files. Build a list of engineering principles that are already binding. These do NOT get surfaced — they get cited.

### Step 2 — Read Product Design Evidence
For every candidate principle, find its source. Acceptable sources:
- Explicit statement in design doc (e.g., "design doc §1.6: Companion Boundary")
- Repeated emphasis across multiple docs (3+ mentions of the same conviction)
- Owner statement captured in a spec or runbook
- Cross-product contract that asserts a boundary

### Step 3 — Classify Each Candidate
| Type | Status | Action |
|------|--------|--------|
| Already in constitution / binding instruction | Already ratified | Cite source, no surfacing |
| Strong evidence in design docs / capability ledger | Surfaced for owner approval | Surface with evidence trace |
| Implied but no clear evidence | DO NOT surface | Flag for owner consult |
| Owner verbal direction with no written source | DO NOT surface | Ask owner to write it down first |

### Step 4 — Produce `Product-Principles.md`
Use the structure mandated by `docs/guides/PRODUCT_DIRECTION_SURFACES.md`:
- Reference to engineering principles (constitution / agent-common.md)
- Numbered list of product-level principles
- Each principle: Status, one-paragraph statement, source evidence trace, what it means for product decisions
- "Surfacing Process" table mapping each principle to source documents
- "Ratification Process" steps

### Step 5 — Produce `product-principles.instructions.md`
Companion enforcement file. For each surfaced principle:
- Status: "Advisory until ratified"
- Per-principle grep checks for forbidden patterns
- Spec authoring rule (every spec touching the principle area MUST include Product Principle Alignment section)

### Step 6 — Owner Consult
Present the surfaced trio and ask the owner to:
- Confirm or reject each surfaced principle
- Edit wording where the surfaced statement doesn't match owner intent
- Add principles the owner thinks are missing (with new source evidence)
- Approve ratification of any surfaced principles ready to become binding

### Step 7 — Apply Ratifications
For each principle the owner ratifies:
- Update `Product-Principles.md`: status changes from "Surfaced for owner approval — not yet ratified" to "Ratified YYYY-MM-DD"
- Update `product-principles.instructions.md`: per-principle status changes from "Advisory until ratified" to "BLOCKING"
- Add ratified date to both files

## Anti-Patterns To Avoid

| Anti-Pattern | Why It's Wrong | Do Instead |
|--------------|----------------|------------|
| Inferring a principle from "common SaaS practice" | Fabrication — no repo evidence | Skip; ask owner |
| Surfacing engineering rules as new product principles | Duplication of constitution | Cite the constitution |
| Auto-ratifying surfaced principles to ship faster | Bypasses owner judgment | Always wait for explicit approval |
| Surfacing 20+ principles to look thorough | Dilutes what matters | Surface only well-evidenced principles |
| Cross-product principle in only one repo | Asymmetric enforcement | Encode in BOTH repos with matching wording |

## Output Shape
- A drafted `Product-Principles.md` with surfaced principles + evidence trace
- A drafted `.github/instructions/product-principles.instructions.md` companion
- A list of "Open Questions" for owner consult (principles surfaced but uncertain wording, or implied principles with no evidence)
- A list of "Already Ratified" engineering principles cited from the constitution
- A clear next action: "Owner review and ratify" or "Surface more evidence first"

## Quality Bar
A discovery pass is useful when:
- Every surfaced principle has a citable source in the repo
- No surfaced principle duplicates an already-ratified engineering principle
- The owner can read the surfaced list and easily approve/reject/edit each one
- Cross-product principles appear in both repos with matching wording

## References
- `docs/guides/PRODUCT_DIRECTION_SURFACES.md` (the convention this skill implements)
- `skills/bubbles-repo-readiness/SKILL.md` (companion: trio presence check)
- `skills/bubbles-skill-authoring/SKILL.md` (skill format authority)
- Reference downstream installations have already adopted this convention as the validation set
