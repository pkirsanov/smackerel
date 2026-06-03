---
applyTo: "**"
---

# Smackerel Product Principles Enforcement

> **STATUS**: Ratified 2026-06-03 by owner; BINDING. Until 2026-06-03 this file was advisory; from this date forward, principles 1–10 are blocking.
>
> This file is the agent-facing enforcement layer. When this file disagrees with `Product-Principles.md`, the principles document wins; this file MUST be updated to match. The [`.specify/memory/constitution.md`](../../.specify/memory/constitution.md) engineering principles (C1-C10) remain NON-NEGOTIABLE on their own track.

---

## How This File Works

`docs/Product-Principles.md` is the human-readable product strategy. The constitution defines the binding engineering principles (C1-C10). This file translates each ratified **product principle** (1-10) into:

1. **Spec authoring requirements** (what every new feature spec MUST include)
2. **Enforcement grep checks** (mechanical detection of violations in code)
3. **Blocking patterns** (forbidden anti-patterns that block PR merge)

---

## Spec Authoring Rule

**Every new feature spec under `specs/`** that touches one of the principle areas MUST include a `## Product Principle Alignment` section declaring:

- Which principle(s) (1-10) the feature implements or extends
- If the feature appears to violate a principle, why the deviation is justified
- Evidence linking the feature back to the principle's source document (`docs/Product-Principles.md` and/or `docs/smackerel.md`)

Specs missing this section MUST be rejected by `/bubbles.plan` and `/bubbles.design` before scopes are written. After ratification (2026-06-03), this rule is blocking.

---

## Per-Principle Enforcement (Activated After Ratification — 2026-06-03)

Each principle below has enforcement actions. After ratification (2026-06-03), the actions are blocking and enforced via grep in PR review and pre-push.

### Principle 1 — Observe First, Ask Second

```bash
# Detect features adding "tag at capture" / "classify at capture" UX (BLOCKING after ratification)
grep -rn 'requireTag\|requireClassification\|tagAtCapture\|categorize.*at.*capture' internal/ web/ ml/

# Detect features that block on user input before passive ingestion can proceed
grep -rn 'blockingUserInput\|requireUserChoice\|awaitUserClassification' internal/
```

| Action | Status |
|--------|--------|
| Spec MUST justify why inference cannot replace user input at capture time | BLOCKING (enforced via grep in PR review + pre-push) |
| Features requiring user organization/tagging at capture MUST justify why observation cannot infer | BLOCKING (enforced via grep in PR review + pre-push) |

### Principle 2 — Vague In, Precise Out

```bash
# Detect retrieval features that require exact field/date/tag input (BLOCKING after ratification)
grep -rn 'exactMatch\|requireExactDate\|requireExactTag\|requireFieldName' internal/

# Verify semantic search is the default retrieval path
grep -rn 'pgvector\|semanticSearch\|llmRerank' internal/
```

| Action | Status |
|--------|--------|
| Retrieval features requiring exact metadata MUST be reclassified as auxiliary, not primary | BLOCKING (enforced via grep in PR review + pre-push) |
| Semantic search via pgvector + LLM re-ranking remains the primary retrieval contract | BLOCKING (enforced via grep in PR review + pre-push) |

### Principle 3 — Knowledge Breathes (Lifecycle, Not Static)

```bash
# Detect new persisted artifact types that don't participate in lifecycle (BLOCKING after ratification)
grep -rn 'CREATE TABLE' internal/db/migrations/ | grep -v 'lifecycle_state\|topic_state\|state'

# Verify topic lifecycle states implemented
grep -rn 'emerging\|active\|hot\|cooling\|dormant\|archived' internal/
```

| Action | Status |
|--------|--------|
| Every new artifact type MUST declare its lifecycle (promotion/decay path) | BLOCKING (enforced via grep in PR review + pre-push) |
| Permanent state without lifecycle management MUST be rejected | BLOCKING (enforced via grep in PR review + pre-push) |

### Principle 4 — Source-Qualified Processing

```bash
# Detect connectors that strip source metadata (BLOCKING after ratification)
grep -rn 'fn.*Connector\|func.*Connector' internal/connectors/ | xargs grep -L 'metadata\|sourceMetadata\|labels'
```

| Action | Status |
|--------|--------|
| Every connector spec MUST declare what source metadata it preserves | BLOCKING (enforced via grep in PR review + pre-push) |
| Connectors stripping metadata for "simplicity" MUST be rejected | BLOCKING (enforced via grep in PR review + pre-push) |

### Principle 5 — One Graph, Many Views

```bash
# Detect new artifact types creating parallel data stores (BLOCKING after ratification)
grep -rn 'CREATE TABLE' internal/db/migrations/ | grep -v 'artifact\|graph\|topic\|entity\|connection'

# Detect parallel search index attempts
grep -rn 'elasticsearch\|opensearch\|meilisearch\|tantivy' internal/
```

| Action | Status |
|--------|--------|
| New artifact types MUST extend the existing knowledge graph | BLOCKING (enforced via grep in PR review + pre-push) |
| Parallel storage/search backends MUST be rejected without explicit cross-graph integration | BLOCKING (enforced via grep in PR review + pre-push) |

### Principle 6 — Invisible By Default, Felt Not Heard

```bash
# Detect notification additions (BLOCKING after ratification — must clear actionability bar)
grep -rn 'sendNotification\|pushAlert\|notifyUser' internal/ ml/

# Detect status-update prompts (forbidden by default)
grep -rn 'processedItems\|capturedToday\|ingestedThisWeek' internal/ web/
```

| Action | Status |
|--------|--------|
| New notifications MUST clear actionability bar (per spec authoring section in spec.md) | BLOCKING (enforced via grep in PR review + pre-push) |
| Status-update prompts ("we processed X") MUST be rejected | BLOCKING (enforced via grep in PR review + pre-push) |
| System-initiated prompts MUST honor the < 3 per week budget (per design doc §1.4) | BLOCKING (enforced via grep in PR review + pre-push) |

### Principle 7 — Small, Frequent, Actionable Output

```bash
# Detect long-form output features (BLOCKING after ratification — must justify)
grep -rn 'multiPageDigest\|longFormSynthesis\|weeklyEssay' internal/ ml/

# Verify digest output length targets
grep -rn 'maxDigestLength\|maxDigestItems\|phoneScreenFit' internal/
```

| Action | Status |
|--------|--------|
| Long-form output features MUST justify why phone-screen-fit version cannot deliver the value | BLOCKING (enforced via grep in PR review + pre-push) |
| Daily digest read time MUST honor the < 2 minute target (per design doc §1.4) | BLOCKING (enforced via grep in PR review + pre-push) |

### Principle 8 — Trust Through Transparency

```bash
# Detect synthesis output without source attribution (BLOCKING after ratification)
grep -rn 'synthesize\|generateDigest\|generateInsight' internal/ ml/ | xargs grep -L 'sourceLink\|sourceArtifactID\|citation'

# Verify Model Compensations enforcement (constitution requirement)
grep -rn 'persistSynthesis\|saveDigest' internal/ | xargs grep -L 'validateSchema\|attachSourceLinks'
```

| Action | Status |
|--------|--------|
| Every synthesis/digest/insight producer MUST attach source links | Already enforced (constitution Model Compensations table) |
| Schema validation + source-link attachment MUST occur before persistence | Already enforced (constitution Model Compensations table) |

### Principle 9 — Design For Restart, Not Perfection

```bash
# Detect backlog/guilt-inducing UX (BLOCKING after ratification)
grep -rn 'unreadCount\|missedItems\|backlogCount\|youHave.*unread' web/ internal/

# Detect punishment-on-return patterns
grep -rn 'requireReviewBeforeUse\|catchUpRequired' web/ internal/
```

| Action | Status |
|--------|--------|
| Returning UX MUST default to "ask system what mattered while away" — no backlog screen | BLOCKING (enforced via grep in PR review + pre-push) |
| Unread/missed counters that punish absence MUST be rejected | BLOCKING (enforced via grep in PR review + pre-push) |

### Principle 10 — QF Companion Boundary (NON-NEGOTIABLE Cross-Product)

```bash
# Detect financial-action features in Smackerel (BLOCKING after ratification — cross-product principal review required)
grep -rn 'approveTrade\|changeMandate\|executeOrder\|financialAdvice' internal/ ml/ web/

# Verify QF packet metadata preservation
grep -rn 'QFDecisionPacket\|CalibrationBadge\|DataProvenanceBadge' internal/
```

| Action | Status |
|--------|--------|
| Smackerel MUST NOT initiate trade approval, mandate change, execution, or financial advice | BLOCKING (enforced via grep in PR review + pre-push) |
| QF packet metadata (`CalibrationBadge`, `DataProvenanceBadge`, packet IDs, intent/scenario IDs, trace IDs, deep links) MUST be preserved without modification | BLOCKING (enforced via grep in PR review + pre-push) |
| `PersonalEvidenceBundle` exports MUST include source, sensitivity, consent, provenance metadata | BLOCKING (enforced via grep in PR review + pre-push) |
| Cross-product schema changes MUST update QF spec 063 FIRST, then Smackerel | BLOCKING (enforced via grep in PR review + pre-push) |

---

## Pre-Ratification Checklist (Historical Record — Completed 2026-06-03)

Before flipping this file from advisory to binding:

- [x] Owner has reviewed every principle in `docs/Product-Principles.md` (1-10)
- [x] Owner has ratified each principle (replaced "Surfaced for owner approval" with "Ratified YYYY-MM-DD")
- [x] Each enforcement action above has a corresponding test or grep check
- [x] Existing codebase has been scanned with each grep check; existing violations are either fixed or documented as exemptions
- [x] This file's "Status: advisory until ratified" markers are removed and replaced with "BLOCKING"

(Ratified 2026-06-03 by owner.) This file is now BLOCKING; the constitution remains the sole NON-NEGOTIABLE engineering authority on its own track.

---

## Cross-References

- [`docs/Product-Principles.md`](../../docs/Product-Principles.md) — Full principle text (surfaced for owner approval)
- [`.specify/memory/constitution.md`](../../.specify/memory/constitution.md) — Engineering principles (NON-NEGOTIABLE; C1-C10)
- [`docs/smackerel.md`](../../docs/smackerel.md) — Authoritative product and architecture design (source for all surfaced principles)
- [`docs/INVESTOR_OVERVIEW.md`](../../docs/INVESTOR_OVERVIEW.md) — Investor-facing platform overview
- [`.github/instructions/terminal-discipline.instructions.md`](terminal-discipline.instructions.md) — Already-binding terminal discipline
