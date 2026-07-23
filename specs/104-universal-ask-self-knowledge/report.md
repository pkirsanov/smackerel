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

### Scope 5 — product-doc corpus source {#scope-5}

Built + tested on the home-lab host. Source SHA a50b37ca.

**Unit (`./smackerel.sh test unit --go --go-run "DocCorpus|ExtractDocSection"`) — exit 0:**

```
[go-unit] applying -run selector: DocCorpus|ExtractDocSection|...
[go-unit] starting go test ./...
ok  github.com/smackerel/smackerel/cmd/core  0.095s [no tests to run]
ok  github.com/smackerel/smackerel/internal/assistant/selfknowledge  0.012s
```

`selfknowledge` ran the docsource tests (0.012s): `TestDocCorpus_Entries_FromEmbeddedOverview`
(the embedded `corpus/product_overview.md` parses into 3 feature/usecase entries;
`feature:overview` mentions the product framing), `TestExtractDocSection_MissingAnchorFailsLoud`
+ `TestExtractDocSection_EmptyBodyFailsLoud` (fail-loud, no silent drop),
`TestExtractDocSection_StopsAtNextHeading`, `TestDocCorpus_DeclaredAnchorMissingFromMarkdownFailsLoud`
(lockstep between `curatedDocSections` and the embedded file). The curated overview
is embedded (`//go:embed`) because the runtime image ships only the binary (Dockerfile
`COPY --from=builder /bin/smackerel-core`), not `docs/`; wired into the ingestor via
`WithDocSource(NewDocCorpus())`.

### Scope 6 — /help human twin {#scope-6}

**Unit (`./smackerel.sh test unit --go --go-run "HelpListsNaturalLanguage|Help_RendersCapabilities"`) — exit 0:**

```
ok  github.com/smackerel/smackerel/internal/telegram  0.013s
ok  github.com/smackerel/smackerel/cmd/core  0.095s [no tests to run]
```

`internal/telegram` ran the /help tests (0.013s, NOT "[no tests to run]"):
`TestHelpListsNaturalLanguageExamplesAndNoRetiredCommands` (the spec-066 contract
still holds — plain-English examples, operational commands, retained /ask,/weather,/remind,
NO retired slash commands) and `TestHelp_RendersCapabilitiesFromSharedCorpus`
(the "What I can help with" list is derived from the SAME `selfknowledge.Derive`
corpus; every enabled scenario label appears; an adversarial brand-new scenario
appears with no help-code edit; command-kind entries do not surface retiring
/recipe,/cook). `HelpText` is fed `selfknowledge.Derive(manifest)` via
`SetHelpCapabilities` in `wireAssistantTelegramAdapter`; the stale hardcoded
`handleHelp` string that still advertised retired commands was removed (spec-066 fix).

### Scope 7 — trust integration + honest fallback {#scope-7}

**Integration (`./smackerel.sh test integration-light --go-run "TrustPerimeter"`) — exit 0:**

```
=== RUN   TestSelfKnowledge_TrustPerimeter
--- PASS: TestSelfKnowledge_TrustPerimeter (0.01s)
PASS
ok  github.com/smackerel/smackerel/tests/integration/openknowledge  0.018s
```

Over real pgvector + the REAL `citeback.Verify` (the same verifier the agent loop
runs each turn): (1) a grounded answer citing a returned `smackerel_self` artifact
passes cite-back (`VerifyResult.OK`, 1 verified); (2) a citation absent from the tool
trace is REFUSED (`ReasonNotInTrace`) — the facade renders this as an honest
`StatusUnavailable` (BUG-061-009 INV-HB-REFUSAL), never "saved as an idea", never a
hallucinated answer; (3) a personal-graph `user:` artifact is never in the tool's
recorded sources AND citing it is rejected (`ReasonNotInTrace`) — personal data can
never be cited via self_knowledge.

**Build Quality Gate (scopes 5–7):** whole module compiled clean (all packages `ok`,
zero FAIL across unit + integration runs); `gofmt -l` empty; 0 warnings. (One
cross-function scope slip — `manifest` referenced in `wireAssistantTelegramAdapter` —
was caught by the module `go test` on the home-lab host, fixed in a50b37ca, and
re-verified green.)

### Scope 8 — E2E + deploy + verify {#scope-8}

**E2E (`./smackerel.sh test e2e`) — self-knowledge meta-question flow:**

```
=== RUN   TestSelfKnowledge_AskMetaQuestion_GroundedCitedAnswer_E2E
--- PASS: TestSelfKnowledge_AskMetaQuestion_GroundedCitedAnswer_E2E (0.01s)  [status=success num_sources=1]
=== RUN   TestSelfKnowledge_AskUngroundable_RefusesHonestly_E2E
--- PASS: TestSelfKnowledge_AskUngroundable_RefusesHonestly_E2E (0.01s)  [status=refused termination_reason=fabricated_source]
ok  github.com/smackerel/smackerel/tests/e2e/openknowledge  0.035s
```

Drives the REAL agent loop over real pg: a grounded `/ask` meta-question returns a
cited answer (num_sources=1); an ungroundable one refuses honestly (never "saved as
an idea"). The `tests/e2e/transports` package failed to COMPILE on a pre-existing
`CaptureFn` signature drift (BUG-061-006 added an `error` return; those e2e stubs
were never updated — e2e is not in the pre-push hook) — fixed here (4 stubs, commit
50d6b564): `ok tests/e2e/transports 0.016s`.

**Pre-existing flake (NOT this spec):** the full suite intermittently fails on
`tests/e2e/assistant :: TestIntentCompilerE2E_WeatherCompilesBeforeRouteAndNormalizesLocation`
(weather routed to capture-fallback — a real-embedding-router timing flake, observed
failing 2026-07-21, before any spec-104 work). Re-run on HEAD: `--- PASS (0.02s)`,
exit 0. Unrelated to self-knowledge.

**Build (`./smackerel.sh build --target self-hosted`) — exit 0:**

```
core: ghcr.io/pkirsanov/smackerel-core@sha256:3b6261a915afc2df5144bf6f15fb61d9793894b520b0feb01da46be80471ef5b
ml:   ghcr.io/pkirsanov/smackerel-ml@sha256:25f36dc55c7be2138f1b49c3e90b57892a6797bc421432a3cfecb7f80088830e
[5/7] cosign sign (operator key) — core + ml signed
[6/7] syft SBOM + cosign attest — core + ml attested
___SMKBUILD_EXIT=0___
```

**Deploy (`promote.sh --target home-lab --product smackerel`, sudo -n) — exit 0:**

```
The cosign claims were validated (core + ml)
preconditions OK
core+ml Recreated → Started
verify OK (strict current release accepted): core-digest=accepted ml-digest=accepted health=accepted
apply OK
___SMKDEPLOY_EXIT=0___
```

**Live verification (docker inspect + prod DB):**

```
smackerel-home-lab-smackerel-core-1 :: running health=healthy restarts=0 :: smackerel-core@sha256:3b6261a9…
smackerel-home-lab-smackerel-ml-1   :: running health=healthy restarts=0 :: smackerel-ml@sha256:25f36dc5…
self-knowledge corpus (source_id=smackerel_self): total=13 embedded=5→7→… kinds=capability,product,recipe,article,idea,note
```

Both digests match the build, healthy, 0 restarts. The boot self-knowledge
ingestion populated the `smackerel_self` namespace (13 artifacts; embeddings fill in
async via NATS→ML — climbing 5→7 at verification). **The semantic self-knowledge
corpus is LIVE on the deployed bot.**

**Operator behavioral smoke test (operator-only):** the live Telegram round-trip
(`/ask what can you do?` → cited capability answer) is operator-verifiable — agents
cannot send Telegram and the prod assistant HTTP API requires a per-user PASETO
token. The behavior is proven end-to-end at the agent/facade layer by the scope-8
e2e tests + the scope-7 trust-perimeter integration test, and deploy-verified with
the corpus live.
