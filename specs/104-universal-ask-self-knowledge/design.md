# Design 104 — Universal `/ask` + Self-Knowledge Grounding

> Grounds the spec-104 requirements in smackerel's **existing** seams. No parallel
> machinery: the same `openknowledge.Tool` contract, the same ingestion pipeline,
> the same `artifacts`/pgvector store, the same cite-back + provenance trust
> perimeter, the same `/help` command surface.

## 1. Architecture at a glance

```
                         ┌───────────────────────── /ask (open_knowledge agent loop) ──────────────────────────┐
  user ──"what can       │  planner (LLM) picks tools by description                                            │
   smackerel do?"──────▶ │    ├── internal_retrieval  → SemanticSearcher(ns="user:<id>")   (personal graph)     │
                         │    ├── web_search          → searxng/brave/tavily               (world)              │
                         │    ├── calculator / unit_convert                                (compute)            │
                         │    └── self_knowledge  ★NEW → SemanticSearcher(ns="smackerel_self")  (product)       │
                         │  → cite-back verifier (every citation hash-matches a tool result)                    │
                         │  → provenance gate (requires_provenance) → StatusAnswered w/ Sources                 │
                         │  → else BUG-061-009 honest refusal (StatusUnavailable, never "saved as an idea")     │
                         └──────────────────────────────────────────────────────────────────────────────────────┘

  deploy/boot ──▶ SelfKnowledgeIngestor ──derives──▶ CapabilityEntry[] ──as RawArtifact(source_id="smackerel_self")──▶
                     PublishRawArtifact ──▶ artifacts table (embedding vector(384) via NATS→ML) ──▶ pgvector
                     (idempotent: content_hash dedup)

  /help  ──reads the SAME CapabilityEntry[] (SST-derived)──▶ human-facing capability menu
```

Two planes, one corpus:
- **Ingest plane** (deploy/boot): derive capability entries from SSTs → ingest as
  `smackerel_self` artifacts (embedded, deduped).
- **Answer plane** (per `/ask`): the planner routes a product meta-question to
  `self_knowledge`, which semantically searches the `smackerel_self` namespace and
  returns cited `Source`s.

## 2. The `self_knowledge` tool (`internal/assistant/openknowledge/tools/self_knowledge.go`)

Implements the existing contract verbatim (mirrors `internal_retrieval.go`):

```go
type Tool interface {
    Name() string                 // "self_knowledge"
    Description() string          // planner-facing (see below)
    ParamsSchema() json.RawMessage // { query:string, k:int<=25 }
    Execute(ctx, params) (*ToolResult, error) // → {Snippets, Sources{Kind: SourceArtifact}}
}
```

- **Name:** `self_knowledge`.
- **Description (planner routing signal):** *"Answer questions about smackerel
  itself — what it can do, its skills and scenarios, slash commands, recipes,
  features, and how to use it. Use this for any question about the product,
  its abilities, or how it works. Returns cited entries from smackerel's own
  capability registry."*
- **Execute:** delegates to a `SemanticSearcher` bound to the `smackerel_self`
  namespace; maps hits to `Source{Kind: SourceArtifact, ID: <artifactID>, Title,
  …}` + `Snippet`s — exactly the shape `internal_retrieval` already returns, so the
  cite-back verifier and the Telegram/WhatsApp renderers accept it with zero
  renderer changes.
- **Always allowlisted:** wired into the `tool_allowlist` unconditionally (FR-1).
  Unlike `web_search` (operator-gateable), self-knowledge is an essential skill.

## 3. Corpus derivation — fresh-by-construction (`internal/assistant/selfknowledge/`)

A new package assembles `CapabilityEntry[]` from the **live SSTs** (FR-2). No
hand-maintained doc.

```go
type CapabilityEntry struct {
    Kind    string // "scenario" | "command" | "recipe" | "feature" | "usecase"
    ID      string // stable, e.g. "scenario:open_knowledge", "command:/help"
    Title   string
    Body    string // the searchable/answerable text
    SourceRef string // provenance back to the SST (file#anchor)
}
```

| Facet | SST source (grounded) | Derivation |
|---|---|---|
| scenarios / skills | `config/assistant/scenarios.yaml` + `internal/assistant/skills_manifest.go` | one entry per scenario: `user_facing_label`, description, `slash_shortcut`, examples, `requires_provenance` |
| slash commands | `internal/assistant/shortcuts.go` + `SetMyCommands` inventory (`cmd/core/wiring.go`) + existing `/help` set | one entry per command: name, what it does, the scenario it maps to |
| recipes | the `recipe_search` catalog / retrieval substrate | one entry per recipe: name + summary |
| features / use-cases / overview | curated sections of `docs/smackerel.md` + `README.md` | a **small reviewed allow-list of section anchors** (overview + "what it does" + capabilities); chunked to artifact-sized `Body` |

Only the last facet is partly-curated (a bounded list of doc-section anchors,
`config`-declared, not free-form); everything else is 100% auto-derived from the
registries, so adding a scenario/command/recipe updates self-knowledge with **no
edit** to this package (FR-2, NFR-2).

## 4. Ingestion — real pipeline, dedicated namespace (`internal/assistant/selfknowledge/ingestor.go`)

`SelfKnowledgeIngestor` maps each `CapabilityEntry` to a `connector.RawArtifact`
and calls the **existing** `RawArtifactPublisher.PublishRawArtifact` (FR-3):

```go
RawArtifact{
    SourceID:   "smackerel_self",      // the namespace discriminator (artifacts.source_id)
    SourceRef:  entry.ID,              // stable per-entry id → dedup key
    RawContent: entry.Body,            // → content_hash dedup + embedded
    Metadata:   { "kind": entry.Kind, "title": entry.Title, "source_ref": entry.SourceRef },
}
```

- **Embeddings for free:** `PublishRawArtifact` stores the artifact and publishes
  to NATS; the ML sidecar computes the `vector(384)` embedding — the same path
  every connector artifact takes. Self-knowledge is genuinely semantic, not a
  keyword bolt-on.
- **Idempotent (NFR-2):** dedup is `(source_url/source_ref, content_hash)`. An
  unchanged entry re-ingests as a no-op; a changed SST yields a new
  `content_hash` → the entry updates. Stale entries (a removed scenario) are
  reconciled by a namespace sweep: after publishing the current set, delete
  `smackerel_self` artifacts whose `source_ref` is not in the current entry ids.
- **Lifecycle (FR-9):** the ingestor runs at **deploy/boot** (invoked from
  `cmd/core` wiring after migrations, before the assistant serves) and is safe to
  re-run. No operator step.

### Why a separate `smackerel_self` namespace (FR-5, non-goal)
`artifacts.source_id = "smackerel_self"` keeps product docs **out** of the user's
personal graph and personal captures **out** of product meta-answers. It is a
system-owned corpus, rebuilt from SSTs on deploy — never mixed with user data.

## 5. Search — the general embedding-backed namespace searcher (`internal/assistant/openknowledge/tools/semantic_searcher.go`)

This spec lands the embedding-backed searcher **deferred by 064 SCOPE-06** as a
**general, namespace-parameterised** capability (FR-4), not a one-off:

```go
type SemanticSearcher interface {
    // Namespace-scoped pgvector cosine search over artifacts.embedding.
    Search(ctx, namespace, query string, k int) ([]GraphArtifact, error)
}
```

- Implementation: embed `query` via the same ML-sidecar embedder used at ingest,
  then `SELECT id, title, summary FROM artifacts WHERE source_id = $namespace
  ORDER BY embedding <=> $queryVec LIMIT $k` (pgvector cosine distance).
- `self_knowledge` consumes it with `namespace = "smackerel_self"`.
- **Reuse path (no fork):** `internal_retrieval` today uses the text-similarity
  `PgxGraphSearcher` (064 SCOPE-06 deferral). The new `SemanticSearcher` is the
  general replacement; `internal_retrieval` adopting it for the user namespace is
  a low-risk **follow-on** (out of scope here, FR-4 note) — but both then share
  ONE embedding-backed search path (NFR-4). No lesser keyword matcher for
  self-knowledge.
- **Graceful degradation:** if the embedder is transiently unavailable, the
  searcher returns a typed error → the agent surfaces the honest BUG-061-009
  refusal (never a fake answer). It does NOT silently fall back to keyword search.

## 6. Trust integration — cite-back + provenance unchanged (FR-8, NFR-1)

- `self_knowledge` returns real `Source{Kind: SourceArtifact}` entries → the
  cite-back verifier (`internal/assistant/openknowledge/citeback`) hash-matches
  each citation to a tool result → the provenance gate passes → `StatusAnswered`
  with citations.
- If `self_knowledge` (and every other tool) returns no citeable source, the
  synthesis is ungrounded → cite-back/gate refuse → the **BUG-061-009 honest
  refusal** (`StatusUnavailable` + "I don't have a sourced answer for that.",
  never "saved as an idea"). The local LLM's untrained guess about a private
  product is exactly what this correctly suppresses.
- **Nothing about the trust perimeter is weakened.** Self-knowledge answers are
  *more* trustworthy than web answers: they cite the product's own registry.

## 7. The `/help` human twin (FR-6)

The existing `/help` command (`internal/telegram/help*.go`, `SetMyCommands`
inventory in `cmd/core/wiring.go`) is refactored to render from the **same**
`CapabilityEntry[]` the ingestor derives. One SST-derived source → both the human
menu and the agent's `self_knowledge` corpus. They cannot drift (a new scenario
appears in `/help` and in `/ask` answers from the same derivation). Optionally a
`/capabilities` alias surfaces the richer list.

## 8. The general command-as-source seam (FR-7)

`self_knowledge` is the first demonstration of a uniform pattern: **any** command
or data surface becomes an `/ask` answer source by implementing
`openknowledge.Tool` and registering it in the allowlist. The design records the
recipe:
1. wrap the surface behind `Tool.Execute` returning `ToolResult{Sources}`,
2. give it a clear planner-facing `Description()`,
3. register in `registry.go` + add to `tool_allowlist`.
Future surfaces (a `catalog` tool, a `status` tool, a connector-data tool) follow
the same three steps — "`/ask` can use any other command for answering"
generalises without bespoke wiring.

## 9. Data model / schema touchpoints

- **No new table.** Reuses `artifacts` (`source_id`, `artifact_type`,
  `content_hash`, `embedding vector(384)`), gated by `source_id="smackerel_self"`.
- Optional: a partial index `CREATE INDEX … ON artifacts (source_id) WHERE
  source_id = 'smackerel_self'` for scoped-search performance (small corpus, so
  optional).

## 10. Wiring (`cmd/core/wiring_assistant_openknowledge.go` + boot)

1. Construct `SemanticSearcher` (embedder + pool).
2. Construct `self_knowledge` tool bound to `namespace="smackerel_self"`; register
   it and add to the effective `tool_allowlist`.
3. At boot (after migrations, before serving): run `SelfKnowledgeIngestor` once
   (idempotent) so the corpus reflects the deployed SSTs.
4. Refactor `/help` to read the shared `CapabilityEntry[]`.

## 11. Failure modes

| Condition | Behaviour |
|---|---|
| Embedder down at ingest | ingestion retries/logs; artifacts persist and get embedded when the sidecar returns (existing NATS path) |
| Embedder down at query | `self_knowledge` returns typed error → honest refusal (never keyword fallback, never fake answer) |
| SST parse error at ingest | fail-loud at boot (G028 no-defaults) — a malformed scenarios.yaml is a deploy error, not a silent empty corpus |
| Meta-question with no match | honest BUG-061-009 refusal |
| Stale entry (removed scenario) | namespace sweep deletes it on next ingest |

## 12. Alternatives considered (and rejected)

- **In-memory keyword index** (the earlier v1 idea): rejected as a shortcut —
  keyword-only recall, a parallel non-pgvector path, and no reuse of the real
  retrieval substrate. It would not generalise to the broader "answer from
  smackerel + online + other sources" vision.
- **Ingest into the personal graph:** rejected — pollutes personal captures and
  personal retrieval (FR-5).
- **Let the LLM answer meta-questions ungrounded with a caveat:** rejected — the
  local model has no real knowledge of a private product; the "answer" would be a
  hallucination (NFR-1, non-goal).

## 13. Scope boundary

In scope: the `self_knowledge` tool, the SST-derived corpus + ingestor, the
general namespace `SemanticSearcher`, the `smackerel_self` namespace, the `/help`
twin, cite-back/provenance integration tests, e2e, deploy. Out of scope:
switching `internal_retrieval` to the new searcher (follow-on), other
products/tenants, and any provenance relaxation.
