---
applyTo: "**"
---

# Smackerel Product Principles Enforcement

> **STATUS**: BINDING. Principles 1–10 were ratified by the owner 2026-06-03; **Principle 11 was ratified 2026-07-29** by owner delegation (recorded via `specs/109-mcp-knowledge-server/spec.md` §18 decision 7). Until 2026-06-03 this file was advisory; from that date forward principles 1–10 are blocking, and Principle 11 is blocking from 2026-07-29.
>
> This file is the agent-facing enforcement layer. When this file disagrees with `Product-Principles.md`, the principles document wins; this file MUST be updated to match. The [`.specify/memory/constitution.md`](../../.specify/memory/constitution.md) engineering principles (C1-C10) remain NON-NEGOTIABLE on their own track.

---

## How This File Works

`docs/Product-Principles.md` is the human-readable product strategy. The constitution defines the binding engineering principles (C1-C10). This file translates each ratified **product principle** (1-11) into:

1. **Spec authoring requirements** (what every new feature spec MUST include)
2. **Enforcement grep checks** (mechanical detection of violations in code)
3. **Blocking patterns** (forbidden anti-patterns that block PR merge)

---

## Spec Authoring Rule

**Every new feature spec under `specs/`** that touches one of the principle areas MUST include a `## Product Principle Alignment` section declaring:

- Which principle(s) (1-11) the feature implements or extends
- If the feature appears to violate a principle, why the deviation is justified
- Evidence linking the feature back to the principle's source document (`docs/Product-Principles.md` and/or `docs/smackerel.md`)

Specs missing this section MUST be rejected by `/bubbles.plan` and `/bubbles.design` before scopes are written. After ratification (2026-06-03), this rule is blocking.

---

## Per-Principle Enforcement (Activated After Ratification — P1–P10 on 2026-06-03, P11 on 2026-07-29)

Each principle below has enforcement actions. After ratification, the actions are blocking and enforced via grep in PR review and pre-push.

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

### Principle 11 — Local-First Data Ownership

Ratified 2026-07-29 (later than P1–P10; see `docs/Product-Principles.md` Surfacing Process).

```bash
# 1. FALSE-CLAIM DEFECT — copy asserting VERIFIED/ENFORCED client-side locality (BLOCKING).
# Smackerel cannot verify where a client's model executes. The only permitted claim shape is
# "Smackerel never sends your knowledge anywhere; a client you explicitly authorize may."
grep -rniE 'verified local|verifies local|enforces local|guaranteed local|locally verified|attests? local|guarantees local' internal/ ml/ web/ docs/

# 2. CLOUD/REMOTE PROCESSING DEFAULT in the SST config (BLOCKING).
# Constitution C1: cloud LLMs may be optional helpers, never the shipped default.
# Current clean state: config/smackerel.yaml carries `provider: "ollama"`.
grep -rniE '^[[:space:]]*(provider|routing|inference_mode)[[:space:]]*:[[:space:]]*["'"'"']?(cloud|remote|openai|anthropic)' config/*.yaml

# 3. REMOTE-EGRESS GRANT WITHOUT AN AUDIT RECORD (BLOCKING).
# Prints any file naming a remote-egress/remote-inference grant that emits no audit entry.
# Empty output = clean.
grep -rlniE 'remote[_-]?inference|remoteInference|remote[_-]?egress' internal/ ml/ web/ config/ 2>/dev/null | xargs -r grep -Lil 'audit'

# 4. EXIT GATED BEHIND AN ENTITLEMENT (BLOCKING) — export/delete/purge must stay unconditional.
# Empty output = clean.
grep -rlniE 'func .*(Export|Delete|Purge|Wipe)' internal/ --include='*.go' | xargs -r grep -lniE 'licen[cs]e|subscription|entitlement|paywall|billing[_ ]?tier'
```

| Action | Status |
|--------|--------|
| Copy MUST NOT claim Smackerel verifies, enforces, guarantees, or attests client-side inference locality; the only permitted claim shape is *"Smackerel never sends your knowledge anywhere; a client you explicitly authorize may."* | BLOCKING (enforced via grep in PR review + pre-push) |
| Cloud/remote processing MUST NOT be the shipped default in the SST config or in code; local inference is the default and cloud is an opt-in helper | BLOCKING (enforced via grep in PR review + pre-push) |
| Any capability that lets an authorized external client read the corpus MUST default to local inference; remote inference MUST be an explicit, per-client, audited operator grant — never global, never a build-time switch, never silent | BLOCKING (enforced via grep in PR review + pre-push) |
| Every remote-egress path MUST emit an audit record naming the granted client | BLOCKING (enforced via grep in PR review + pre-push) |
| Corpus export, relocation, and deletion MUST remain unconditional; accumulated value MUST NOT become a switching barrier | BLOCKING (enforced via grep in PR review + pre-push) |
| A feature that requires a hosted service for core function MUST be rejected | BLOCKING (enforced via grep in PR review + pre-push) |

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
