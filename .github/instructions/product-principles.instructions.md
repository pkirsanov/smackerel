---
applyTo: "**"
---

# Smackerel Product Principles Enforcement

> **STATUS**: Companion enforcement file for [`docs/Product-Principles.md`](../../docs/Product-Principles.md). Becomes binding **only after** the principles in that document are ratified by the owner.
>
> **Until ratification**: This file is advisory. The [`.specify/memory/constitution.md`](../../.specify/memory/constitution.md) remains the only NON-NEGOTIABLE engineering authority.
>
> **After ratification**: This file is the agent-facing enforcement layer. When this file disagrees with `Product-Principles.md`, the principles document wins; this file MUST be updated to match.

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

Specs missing this section MUST be rejected by `/bubbles.plan` and `/bubbles.design` before scopes are written. Until ratification, this rule is advisory.

---

## Per-Principle Enforcement (Activated After Ratification)

Each principle below has enforcement actions. Until the corresponding principle is ratified in `Product-Principles.md`, the actions are advisory. After ratification, they are blocking.

### Principle 1 — Observe First, Ask Second

```bash
# Detect features adding "tag at capture" / "classify at capture" UX (BLOCKING after ratification)
grep -rn 'requireTag\|requireClassification\|tagAtCapture\|categorize.*at.*capture' internal/ web/ ml/

# Detect features that block on user input before passive ingestion can proceed
grep -rn 'blockingUserInput\|requireUserChoice\|awaitUserClassification' internal/
```

| Action | Status |
|--------|--------|
| Spec MUST justify why inference cannot replace user input at capture time | Advisory until ratified |
| Features requiring user organization/tagging at capture MUST justify why observation cannot infer | Advisory until ratified |

### Principle 2 — Vague In, Precise Out

```bash
# Detect retrieval features that require exact field/date/tag input (BLOCKING after ratification)
grep -rn 'exactMatch\|requireExactDate\|requireExactTag\|requireFieldName' internal/

# Verify semantic search is the default retrieval path
grep -rn 'pgvector\|semanticSearch\|llmRerank' internal/
```

| Action | Status |
|--------|--------|
| Retrieval features requiring exact metadata MUST be reclassified as auxiliary, not primary | Advisory until ratified |
| Semantic search via pgvector + LLM re-ranking remains the primary retrieval contract | Advisory until ratified |

### Principle 3 — Knowledge Breathes (Lifecycle, Not Static)

```bash
# Detect new persisted artifact types that don't participate in lifecycle (BLOCKING after ratification)
grep -rn 'CREATE TABLE' internal/db/migrations/ | grep -v 'lifecycle_state\|topic_state\|state'

# Verify topic lifecycle states implemented
grep -rn 'emerging\|active\|hot\|cooling\|dormant\|archived' internal/
```

| Action | Status |
|--------|--------|
| Every new artifact type MUST declare its lifecycle (promotion/decay path) | Advisory until ratified |
| Permanent state without lifecycle management MUST be rejected | Advisory until ratified |

### Principle 4 — Source-Qualified Processing

```bash
# Detect connectors that strip source metadata (BLOCKING after ratification)
grep -rn 'fn.*Connector\|func.*Connector' internal/connectors/ | xargs grep -L 'metadata\|sourceMetadata\|labels'
```

| Action | Status |
|--------|--------|
| Every connector spec MUST declare what source metadata it preserves | Advisory until ratified |
| Connectors stripping metadata for "simplicity" MUST be rejected | Advisory until ratified |

### Principle 5 — One Graph, Many Views

```bash
# Detect new artifact types creating parallel data stores (BLOCKING after ratification)
grep -rn 'CREATE TABLE' internal/db/migrations/ | grep -v 'artifact\|graph\|topic\|entity\|connection'

# Detect parallel search index attempts
grep -rn 'elasticsearch\|opensearch\|meilisearch\|tantivy' internal/
```

| Action | Status |
|--------|--------|
| New artifact types MUST extend the existing knowledge graph | Advisory until ratified |
| Parallel storage/search backends MUST be rejected without explicit cross-graph integration | Advisory until ratified |

### Principle 6 — Invisible By Default, Felt Not Heard

```bash
# Detect notification additions (BLOCKING after ratification — must clear actionability bar)
grep -rn 'sendNotification\|pushAlert\|notifyUser' internal/ ml/

# Detect status-update prompts (forbidden by default)
grep -rn 'processedItems\|capturedToday\|ingestedThisWeek' internal/ web/
```

| Action | Status |
|--------|--------|
| New notifications MUST clear actionability bar (per spec authoring section in spec.md) | Advisory until ratified |
| Status-update prompts ("we processed X") MUST be rejected | Advisory until ratified |
| System-initiated prompts MUST honor the < 3 per week budget (per design doc §1.4) | Advisory until ratified |

### Principle 7 — Small, Frequent, Actionable Output

```bash
# Detect long-form output features (BLOCKING after ratification — must justify)
grep -rn 'multiPageDigest\|longFormSynthesis\|weeklyEssay' internal/ ml/

# Verify digest output length targets
grep -rn 'maxDigestLength\|maxDigestItems\|phoneScreenFit' internal/
```

| Action | Status |
|--------|--------|
| Long-form output features MUST justify why phone-screen-fit version cannot deliver the value | Advisory until ratified |
| Daily digest read time MUST honor the < 2 minute target (per design doc §1.4) | Advisory until ratified |

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
| Returning UX MUST default to "ask system what mattered while away" — no backlog screen | Advisory until ratified |
| Unread/missed counters that punish absence MUST be rejected | Advisory until ratified |

### Principle 10 — QF Companion Boundary (NON-NEGOTIABLE Cross-Product)

```bash
# Detect financial-action features in Smackerel (BLOCKING after ratification — cross-product principal review required)
grep -rn 'approveTrade\|changeMandate\|executeOrder\|financialAdvice' internal/ ml/ web/

# Verify QF packet metadata preservation
grep -rn 'QFDecisionPacket\|CalibrationBadge\|DataProvenanceBadge' internal/
```

| Action | Status |
|--------|--------|
| Smackerel MUST NOT initiate trade approval, mandate change, execution, or financial advice | Advisory until ratified |
| QF packet metadata (`CalibrationBadge`, `DataProvenanceBadge`, packet IDs, intent/scenario IDs, trace IDs, deep links) MUST be preserved without modification | Advisory until ratified |
| `PersonalEvidenceBundle` exports MUST include source, sensitivity, consent, provenance metadata | Advisory until ratified |
| Cross-product schema changes MUST update QF spec 063 FIRST, then Smackerel | Advisory until ratified |

---

## Pre-Ratification Checklist

Before flipping this file from advisory to binding:

- [ ] Owner has reviewed every principle in `docs/Product-Principles.md` (1-10)
- [ ] Owner has ratified each principle (replaced "Surfaced for owner approval" with "Ratified YYYY-MM-DD")
- [ ] Each enforcement action above has a corresponding test or grep check
- [ ] Existing codebase has been scanned with each grep check; existing violations are either fixed or documented as exemptions
- [ ] This file's "Status: advisory until ratified" markers are removed and replaced with "BLOCKING"

Until every checkbox above is checked, this file is advisory and the constitution remains the sole NON-NEGOTIABLE authority.

---

## Cross-References

- [`docs/Product-Principles.md`](../../docs/Product-Principles.md) — Full principle text (surfaced for owner approval)
- [`.specify/memory/constitution.md`](../../.specify/memory/constitution.md) — Engineering principles (NON-NEGOTIABLE; C1-C10)
- [`docs/smackerel.md`](../../docs/smackerel.md) — Authoritative product and architecture design (source for all surfaced principles)
- [`docs/INVESTOR_OVERVIEW.md`](../../docs/INVESTOR_OVERVIEW.md) — Investor-facing platform overview
- [`.github/instructions/terminal-discipline.instructions.md`](terminal-discipline.instructions.md) — Already-binding terminal discipline
