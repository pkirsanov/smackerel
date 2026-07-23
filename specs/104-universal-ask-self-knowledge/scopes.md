# Scopes 104 — Universal `/ask` + Self-Knowledge Grounding

Eight sequential scopes. Substrate first (searcher → corpus → ingest), then the
tool + doc source + human twin, then trust integration, then e2e + deploy. Every
live-category test uses the ephemeral test stack (no prod writes).

Commands: build/test/lint via `./smackerel.sh`; build+deploy on the home-lab host
per the BUG-061-009 recipe (local box has OOM pressure — do NOT build locally).

---

## Scope 1 (P0): General embedding-backed namespace `SemanticSearcher`

**Status:** Not Started
**Depends On:** —
**FR:** FR-4, NFR-4

Lands the embedding-backed searcher deferred by 064 SCOPE-06 as a general,
namespace-parameterised capability. `internal/assistant/openknowledge/tools/semantic_searcher.go`:
`SemanticSearcher.Search(ctx, namespace, query, k)` → embed query via the ML
sidecar embedder, `SELECT … FROM artifacts WHERE source_id=$namespace ORDER BY
embedding <=> $vec LIMIT $k`. Typed error on embedder failure (NO keyword
fallback).

```gherkin
Scenario: namespace-scoped cosine search returns only in-namespace artifacts
  Given artifacts exist under source_id "smackerel_self" and "user:u1" with embeddings
  When SemanticSearcher.Search(ns="smackerel_self", "capabilities", k=5) runs
  Then only smackerel_self artifacts are returned, ordered by cosine similarity
```

### Test Plan
| Test Type | Category | File | Description | Command | Live |
|---|---|---|---|---|---|
| Unit | `unit` | `.../tools/semantic_searcher_test.go` | param validation; embedder-error → typed error (no fallback) | `./smackerel.sh test unit --go` | No |
| Integration | `integration` | `tests/integration/openknowledge/semantic_searcher_test.go` | real pgvector: seed 2 namespaces + embeddings, assert scoped ordering | `./smackerel.sh test integration` | Yes |

### Definition of Done
- [ ] `SemanticSearcher` implemented; namespace-scoped pgvector cosine; embedder-failure returns typed error (no keyword fallback)
- [ ] Unit + integration tests pass (ephemeral pg); scoped isolation asserted
- [ ] Build Quality Gate: `./smackerel.sh check`, `lint`, `format --check` clean; 0 warnings

---

## Scope 2 (P0): Self-knowledge corpus derivation (fresh-by-construction)

**Status:** Not Started
**Depends On:** —
**FR:** FR-2

`internal/assistant/selfknowledge/`: `CapabilityEntry` + derivation from the live
SSTs — `config/assistant/scenarios.yaml` + `skills_manifest` (scenarios/skills),
`shortcuts.go` + `SetMyCommands` inventory (commands), `recipe_search` catalog
(recipes). Fail-loud on malformed SST (G028).

```gherkin
Scenario: adding a scenario yields a new capability entry with no hand edit
  Given config/assistant/scenarios.yaml lists scenario "open_knowledge" (/ask)
  When the corpus is derived
  Then a CapabilityEntry{kind:scenario, id:"scenario:open_knowledge"} exists
    carrying its user_facing_label, description, and slash_shortcut
```

### Test Plan
| Test Type | Category | File | Description | Command | Live |
|---|---|---|---|---|---|
| Unit | `unit` | `internal/assistant/selfknowledge/derive_test.go` | derive from real scenarios.yaml + shortcuts; count/shape asserts; malformed SST → fail-loud | `./smackerel.sh test unit --go` | No |

### Definition of Done
- [ ] `CapabilityEntry` + derivation from scenarios.yaml + shortcuts + skills_manifest + recipe catalog
- [ ] Auto-derived facets require zero hand-maintenance (adding a scenario/command/recipe updates the corpus)
- [ ] Malformed SST fails loud at derive time (no silent empty corpus)
- [ ] Unit tests pass against the real committed SSTs
- [ ] Build Quality Gate clean

---

## Scope 3 (P0): Self-knowledge ingestion + `smackerel_self` namespace + boot lifecycle

**Status:** Not Started
**Depends On:** Scope 2
**FR:** FR-3, FR-5, FR-9, NFR-2

`SelfKnowledgeIngestor`: map each `CapabilityEntry` → `connector.RawArtifact{SourceID:"smackerel_self", SourceRef:entry.ID, RawContent:entry.Body, Metadata}` →
`RawArtifactPublisher.PublishRawArtifact` (real pipeline → embedding + dedup).
Idempotent; stale-entry namespace sweep. Wired at boot after migrations.

```gherkin
Scenario: idempotent ingestion with stale sweep
  Given the corpus is ingested once under smackerel_self
  When it is ingested again unchanged
  Then no duplicate artifacts are created (content_hash dedup)
  And a removed entry's artifact is swept from the namespace
```

### Test Plan
| Test Type | Category | File | Description | Command | Live |
|---|---|---|---|---|---|
| Integration | `integration` | `tests/integration/selfknowledge/ingest_test.go` | ingest → artifacts present under smackerel_self, embedded; re-ingest no-op; stale sweep | `./smackerel.sh test integration` | Yes |

### Definition of Done
- [ ] Ingestor uses the existing `PublishRawArtifact` (no bespoke insert); artifacts land under `source_id="smackerel_self"` with embeddings
- [ ] Idempotent (content_hash dedup); stale entries swept; boot wiring runs once, safe to re-run
- [ ] Integration test proves ingest + dedup + sweep on ephemeral pg
- [ ] Build Quality Gate clean

---

## Scope 4 (P0): `self_knowledge` tool + allowlist + planner routing

**Status:** Not Started
**Depends On:** Scope 1, Scope 3
**FR:** FR-1, FR-5

`internal/assistant/openknowledge/tools/self_knowledge.go` implementing
`openknowledge.Tool`, bound to `namespace="smackerel_self"` via the Scope-1
searcher; registered in `registry.go` + added to the effective `tool_allowlist`
(always-on). Planner-facing `Description()` per design §2.

```gherkin
Scenario: the tool returns only cited smackerel_self sources
  When self_knowledge.Execute({query:"what can smackerel do", k:5}) runs
  Then it returns Source{Kind:SourceArtifact} entries all from smackerel_self
```

### Test Plan
| Test Type | Category | File | Description | Command | Live |
|---|---|---|---|---|---|
| Unit | `unit` | `.../tools/self_knowledge_test.go` | contract: name/description/schema; Execute maps hits → Sources; namespace isolation | `./smackerel.sh test unit --go` | No |
| Integration | `integration` | `tests/integration/openknowledge/self_knowledge_tool_test.go` | tool over real pg+embeddings returns cited self sources | `./smackerel.sh test integration` | Yes |

### Definition of Done
- [ ] `self_knowledge` implements the Tool contract; wired into registry + always in the allowlist
- [ ] Returns `Source{Kind:SourceArtifact}` only from smackerel_self (isolation)
- [ ] Planner description present + clear
- [ ] Unit + integration tests pass
- [ ] Build Quality Gate clean

---

## Scope 5 (P1): Product-doc corpus source (features / use-cases / overview)

**Status:** Not Started
**Depends On:** Scope 2, Scope 3
**FR:** FR-2 (curated facet)

A `config`-declared, bounded allow-list of `docs/smackerel.md` + `README.md`
section anchors ingested as `kind:feature`/`kind:usecase` entries (chunked to
artifact size). This is the only partly-curated facet — kept minimal + SST-anchored.

```gherkin
Scenario: the "how does smackerel work" overview is ingested and answerable
  Given the curated overview section anchors are declared
  When the corpus is ingested
  Then a smackerel_self artifact carries the overview text and is retrievable
```

### Test Plan
| Test Type | Category | File | Description | Command | Live |
|---|---|---|---|---|---|
| Unit | `unit` | `internal/assistant/selfknowledge/docsource_test.go` | declared anchors resolve to chunked entries; missing anchor fails loud | `./smackerel.sh test unit --go` | No |
| Integration | `integration` | (extends Scope 3 test) | overview artifact present + embedded under smackerel_self | `./smackerel.sh test integration` | Yes |

### Definition of Done
- [ ] Bounded, config-declared doc-section allow-list → chunked capability entries
- [ ] Missing/renamed anchor fails loud (no silent drop)
- [ ] Tests pass
- [ ] Build Quality Gate clean

---

## Scope 6 (P1): `/help` human twin (shared corpus)

**Status:** Not Started
**Depends On:** Scope 2
**FR:** FR-6

Refactor the existing `/help` (`internal/telegram/help*.go` + `SetMyCommands`
inventory) to render from the SAME `CapabilityEntry[]` the ingestor derives, so
menu and `/ask` answers cannot diverge.

```gherkin
Scenario: /help lists capabilities from the SST-derived corpus
  When the user sends "/help"
  Then the rendered list is derived from the same CapabilityEntry set
```

### Test Plan
| Test Type | Category | File | Description | Command | Live |
|---|---|---|---|---|---|
| Unit | `unit` | `internal/telegram/help_test.go` (extend) | /help renders from the shared corpus; new scenario appears without a hand edit | `./smackerel.sh test unit --go` | No |

### Definition of Done
- [ ] `/help` reads the shared `CapabilityEntry[]` (no separate hand-maintained list)
- [ ] A newly-added scenario appears in `/help` with no help-code edit
- [ ] Tests pass
- [ ] Build Quality Gate clean

---

## Scope 7 (P0): Trust integration + honest-fallback (cite-back / provenance)

**Status:** Not Started
**Depends On:** Scope 4, Scope 5
**FR:** FR-8, NFR-1

Prove the trust perimeter is intact end-to-end at the facade: a grounded meta-question
answers with citations to smackerel_self and passes the gate; an ungroundable
meta-question refuses honestly (BUG-061-009); personal captures never leak.

```gherkin
Scenario: grounded meta-answer is cited; ungroundable refuses honestly
  When /ask "what can smackerel do?" runs with self-knowledge ingested
  Then StatusAnswered with smackerel_self citations (cite-back + gate pass)
  When /ask a meta-question with no self-knowledge match runs
  Then StatusUnavailable honest refusal, never "saved as an idea", never hallucinated
```

### Test Plan
| Test Type | Category | File | Description | Command | Live |
|---|---|---|---|---|---|
| Integration | `integration` | `tests/integration/openknowledge/self_knowledge_provenance_test.go` | grounded→cited+gate-pass; ungroundable→honest refusal; no personal leak | `./smackerel.sh test integration` | Yes |

### Definition of Done
- [ ] Grounded meta-answer cites smackerel_self and passes cite-back + provenance
- [ ] Ungroundable meta-question → honest refusal (never saved-as-idea, never hallucinated)
- [ ] Personal-graph artifacts never cited in a self-knowledge answer
- [ ] Tests pass
- [ ] Build Quality Gate clean

---

## Scope 8 (P0): E2E + deploy + verify

**Status:** Not Started
**Depends On:** Scope 6, Scope 7
**FR:** all (end-to-end)

E2E `/ask` meta-question flows against the live stack; build + deploy on-host
(local-operator) to the home-lab bot; verify running digests + healthy + the
self-knowledge corpus ingested.

```gherkin
Scenario: live /ask about smackerel answers with citations
  When a user asks the deployed bot "/ask what can you do?"
  Then the reply is a real, cited capability answer (not a refusal, not saved-as-idea)
```

### Test Plan
| Test Type | Category | File | Description | Command | Live |
|---|---|---|---|---|---|
| E2E | `e2e-api`/`e2e-ui` | `tests/e2e/openknowledge/self_knowledge_ask_test.go` | full-stack /ask meta-question → cited answer | `./smackerel.sh test e2e` | Yes |
| Stress | `stress` | (extend openknowledge p95) | self_knowledge search within budget | per existing | Yes |

### Definition of Done
- [ ] E2E meta-question flow passes on the ephemeral stack (cited answer)
- [ ] Built + operator-cosign-signed + deployed on-host; running digests healthy; corpus ingested (verified)
- [ ] Operator behavioral smoke test recorded (or noted operator-only)
- [ ] Build Quality Gate clean
