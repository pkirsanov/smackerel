# Report — Spec 104 Universal `/ask` + Self-Knowledge

## Summary

Planning artifacts authored (spec + design + scopes), grounded in smackerel's real
seams (the `openknowledge.Tool` contract, the `RawArtifactPublisher` ingestion
pipeline, the `artifacts`/pgvector store with `source_id` namespace + `embedding
vector(384)`, the cite-back verifier, and the `/help` command surface). Motivated
by the BUG-061-010 live diagnosis (the bot cannot answer about itself because the
public web knows "smackerel" only as a Super Mario enemy, the personal graph has
no product docs, and the local LLM has no training data on a private product).

Design decision (operator: best for long term, no shortcuts): self-knowledge is a
dedicated `smackerel_self` pgvector namespace, ingested via the existing pipeline
(real embeddings), searched by a new **general** embedding-backed namespace
searcher (resolving the 064 SCOPE-06 deferral) — NOT an in-memory keyword bolt-on.

## Completion Statement

Planning complete (analyze + design + plan). Implementation pending: scopes 1–8 in
dependency order (searcher → corpus → ingest → tool → doc source → /help twin →
trust integration → e2e + deploy).

## Test Evidence

### Scope 1 — general embedding-backed namespace SemanticSearcher {#scope-1}

Built + tested on the home-lab host (local box under OOM pressure). Source SHA a26d9985.

**Unit (`./smackerel.sh test unit --go --go-run SemanticSearcher`) — exit 0:**

```
ok  github.com/smackerel/smackerel/internal/assistant/openknowledge/tools  0.006s
___UNIT_EXIT=0___
```

The `tools` package ran the matched tests (0.006s, not "[no tests to run]"):
`TestPgxSemanticSearcher_ValidationAndEmbedShortCircuit` (all validation +
embedder-error paths short-circuit before any DB access via the queryGuard) and
`TestNewPgxSemanticSearcher_NilArgsPanic`.

**Integration (`./smackerel.sh test integration-light --go-run PgxSemanticSearcher_NamespaceScopedCosine`) — exit 0:**

```
=== RUN   TestPgxSemanticSearcher_NamespaceScopedCosine
--- PASS: TestPgxSemanticSearcher_NamespaceScopedCosine (0.02s)
ok  github.com/smackerel/smackerel/tests/integration/openknowledge  0.032s
PASS: go-integration-light
___INTEG_EXIT=0___
```

Against real pgvector: a row identical to the query vector but in a different
`source_id` namespace is EXCLUDED (isolation, FR-5), and within `smackerel_self`
the nearer embedding ranks first (cosine ordering).

**Build Quality Gate:** the whole Go module compiled clean in both runs;
`format --check` flagged only a pre-existing gofmt drift in
`internal/telegram/assistant_adapter/adapter.go` (a BUG-061-006 doc-comment
reindent that slipped past the pre-push hook, which runs only the knb uniformity
lint) — fixed here.

### Scopes 2–8

Pending implementation.
