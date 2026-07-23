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

### Scope 2 — self-knowledge corpus derivation {#scope-2}

Built + tested on the home-lab host. Source SHA ea3762f5.

**Unit (`./smackerel.sh test unit --go --go-run "Derive|SelfKnowledge"`) — exit 0:**

```
[go-unit] applying -run selector: Derive|SelfKnowledge
[go-unit] starting go test ./...
ok  github.com/smackerel/smackerel/cmd/core  0.083s [no tests to run]
ok  github.com/smackerel/smackerel/internal/assistant/openknowledge/tools  0.006s
ok  github.com/smackerel/smackerel/internal/assistant/selfknowledge  0.017s
```

`internal/assistant/selfknowledge` ran the matched tests (0.017s, NOT "[no tests to
run]"): `TestDerive_FromRealScenariosYAML` (loads the real
`config/assistant/scenarios.yaml` + shortcuts; asserts `scenario:open_knowledge`
carries the `/ask` shortcut + label, and the `command:/ask` + `command:/reset`
bodies), `TestDerive_Deterministic` (stable order), `TestDerive_NilManifest`
(fail-safe). Recipes are represented via the `recipe_search` scenario +
`/recipe`,`/cook` command entries (no separate recipe-catalog SST exists in the
repo). `cmd/core` compiled clean with the new boot wiring.

### Scope 3 — self-knowledge ingestion + smackerel_self namespace {#scope-3}

**Integration (`./smackerel.sh test integration-light --go-run "Ingestor|SelfKnowledgeTool"`) — exit 0:**

```
go-integration: applying -run selector: Ingestor|SelfKnowledgeTool
=== RUN   TestIngestor_IdempotentWithStaleSweep
--- PASS: TestIngestor_IdempotentWithStaleSweep (0.02s)
PASS
ok  github.com/smackerel/smackerel/tests/integration/selfknowledge  0.025s
```

Against real pgvector: first ingest publishes each entry under
`source_id="smackerel_self"` via the shared `PublishRawArtifact` (content-hash
dedup); re-ingest publishes 0 + sweeps 0 (idempotent); an injected stale row is
swept (`content_hash <> ALL(current)`). Boot wiring
(`cmd/core/wiring_selfknowledge.go`) runs once after migrations, gated on
`open_knowledge.enabled`, and compiled clean (the `cmd/core` line above).

### Scope 4 — self_knowledge tool + always-on allowlist {#scope-4}

**Unit** — the `openknowledge/tools` package ran the matched `SelfKnowledge` tests
(the 0.006s line above): `TestSelfKnowledge_Contract`,
`TestSelfKnowledge_ExecuteMapsCitedSources` (asserts namespace `smackerel_self`
searched + `Source{Kind:SourceArtifact}` mapping), `TestSelfKnowledge_ExecuteErrorPaths`
(9 validation/backend cases), `TestNewSelfKnowledge_NilArgsPanic`.

**Integration (`./smackerel.sh test integration-light --go-run "...SelfKnowledgeTool"`) — exit 0:**

```
=== RUN   TestSelfKnowledgeTool_CitesOnlySmackerelSelf
--- PASS: TestSelfKnowledgeTool_CitesOnlySmackerelSelf (0.01s)
PASS
ok  github.com/smackerel/smackerel/tests/integration/openknowledge  0.019s
```

The tool over a real `PgxSemanticSearcher`+pgvector returns cited
`Source{Kind:SourceArtifact}` entries drawn ONLY from `smackerel_self` (a closer
personal-graph `user:` row is EXCLUDED — isolation), cosine-ordered, with 1:1
snippets. Registered always-on into the effective `tool_allowlist` in
`wireOpenKnowledge` (FR-1).

**Build Quality Gate (scopes 2–4):** whole module compiled clean (all packages
`ok`, zero FAIL across unit + integration runs); `gofmt -l` on all 10 changed
files returned empty (format clean); 0 warnings.

### Scopes 5–8

Pending implementation.
