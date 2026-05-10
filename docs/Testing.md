# Smackerel Testing Guide

This guide defines Smackerel's CLI-owned test surface and the isolation rules for runtime testing.

## Current Test Coverage

| Language | Packages/Files | Test Count | Type |
|----------|---------------|------------|------|
| Go | 38 packages | 3334+ test functions | Unit (behavioral + structural) |
| Python | 18 test files | 177+ tests | Unit |
| Shell (E2E) | 70 scripts | End-to-end | Live-stack |
| Shell (Stress) | 2 scripts | Stress | Live-stack |

### Key Test Areas

- **Security tests**: auth token validation (placeholder rejection, min length), SSRF URL blocking (private IPs, IPv6, metadata), ILIKE wildcard escaping, config validation (PORT, CRON)
- **Search tests**: temporal intent parsing (8 patterns), search request handling
- **Connector tests**: IMAP tier assignment, CalDAV event parsing, YouTube engagement tiers, RSS/Atom parsing, bookmarks parsing, browser dwell time, Keep takeout parsing + label mapping + qualifiers, Hospitable PAT auth + pagination + rate-limit retry + multi-resource sync + cursor management + sender classification + rating precision, Discord channel monitoring + token validation + message classification, Twitter archive parsing + thread reconstruction + entity extraction, Weather location config + coordinate rounding + WMO code mapping, Government Alerts haversine proximity + magnitude filtering + deduplication, Financial Markets watchlist parsing + rate limiting + alert threshold
- **Telegram tests**: share capture, forward metadata, conversation assembly (timer/overflow/concurrent), media group assembly
- **Intelligence tests**: synthesis insights, alert lifecycle, resurfacing scoring
- **Knowledge tests**: concept store CRUD, entity profiles, contract validation, lint auditing (stale/orphan/contradiction detection), upsert conflict resolution
- **Domain extraction tests**: schema registry lookup, content-type matching, URL qualifier matching
- **Annotation tests**: freeform parser (ratings, tags, notes, interactions), store CRUD, materialized summary view, Telegram message-artifact mapping
- **Actionable list tests**: list CRUD, item completion (done/skipped/substituted), domain-aware aggregation (recipe ingredients, reading lists, product comparisons)
- **Observability tests**: Prometheus metric registration, counter increments, histogram recording, W3C traceparent header injection/extraction
- **Pipeline tests**: dedup, processing, embedding format, tier assignment, synthesis subscriber

## Current Validation Surface

The repository now exposes a sanctioned CLI-owned runtime test surface for the foundation scaffold while retaining the Bubbles framework checks for governance work.

| Test type | Command | Required when |
|-----------|---------|---------------|
| Go + Python unit | `./smackerel.sh test unit` | Runtime code changes |
| Integration | `./smackerel.sh test integration` | Runtime lifecycle or health changes |
| End-to-end | `./smackerel.sh test e2e` | Runtime, compose, or config changes |
| Stress smoke | `./smackerel.sh test stress` | Runtime health, lifecycle, or stress env handoff changes |
| Framework doctor | `bash .github/bubbles/scripts/cli.sh doctor` | Project-owned bootstrap docs change |
| Framework validate | `timeout 1200 bash .github/bubbles/scripts/cli.sh framework-validate` | Before claiming bootstrap health |
| Artifact lint | `bash .github/bubbles/scripts/artifact-lint.sh specs/<feature>` | Spec or bug artifacts change |
| Traceability guard | `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/<feature>` | Traceability-sensitive artifact content changes |
| Regression baseline guard | `timeout 600 bash .github/bubbles/scripts/regression-baseline-guard.sh specs/<feature> --verbose` | Managed docs or competitive baseline content changes |

## Current Runtime Test Matrix

The current CLI-owned runtime surface exposes these categories today:

| Test type | Category | Required command |
|-----------|----------|------------------|
| Go unit | `unit` | `./smackerel.sh test unit --go` |
| Python unit | `unit` | `./smackerel.sh test unit --python` |
| Integration | `integration` | `./smackerel.sh test integration` |
| End-to-end API | `e2e-api` | `./smackerel.sh test e2e` |
| End-to-end UI | `e2e-ui` | `./smackerel.sh test e2e` (web UI paths included) |
| Stress | `stress` | `./smackerel.sh test stress` |

### Go Package Coverage

All 38 Go packages have tests:

- `internal/api` — capture, search, health, digest, recent, annotations, lists handlers
- `internal/annotation` — freeform parser, store CRUD, materialized summary, Telegram message mapping
- `internal/auth` — OAuth2 provider, token exchange
- `internal/config` — validation, placeholder rejection, env parsing
- `internal/connector` — framework, registry, backoff, supervisor
- `internal/connector/bookmarks` — Chrome/Netscape parsing
- `internal/connector/browser` — dwell time, skip list, privacy
- `internal/connector/caldav` — event sync, attendee extraction, tier assignment
- `internal/connector/discord` — channel monitoring, token validation, message classification
- `internal/connector/imap` — email sync, tier qualification, action item extraction
- `internal/connector/keep` — Takeout parsing, normalization, labels, qualifiers
- `internal/connector/hospitable` — PAT auth, pagination, Retry-After, active reservation sync, normalizer (4 types), cursor round-trip, partial failure, sender classification, URL population, rating precision
- `internal/connector/maps` — activity classification, trail detection, GeoJSON
- `internal/connector/markets` — watchlist parsing, rate limiting, alert threshold
- `internal/connector/rss` — RSS + Atom feed parsing
- `internal/connector/alerts` — haversine proximity, magnitude filtering, deduplication
- `internal/connector/twitter` — archive parsing, thread reconstruction, entity extraction
- `internal/connector/weather` — location config, coordinate rounding, WMO code mapping
- `internal/connector/youtube` — video sync, engagement tiers
- `internal/db` — migration system
- `internal/digest` — generation, formatting
- `internal/domain` — schema registry, content-type matching, URL qualifier matching
- `internal/extract` — readability, SSRF protection, content hashing
- `internal/graph` — similarity, entity, topic, temporal linking
- `internal/intelligence` — synthesis, alerts, commitments, resurfacing
- `internal/knowledge` — concept store, entity profiles, contract validation, lint auditing, upsert conflict resolution
- `internal/list` — list CRUD, item completion, domain-aware aggregation (recipe, reading, product)
- `internal/metrics` — Prometheus metric registration, counter/histogram/gauge verification, W3C trace header propagation
- `internal/nats` — JetStream client, stream management
- `internal/pipeline` — processing, dedup, tier assignment
- `internal/scheduler` — cron scheduling
- `internal/telegram` — bot routing, share capture, forwarding, conversation assembly, media groups, format
- `internal/topics` — lifecycle management
- `internal/web` — handler, search, artifact detail, status
- `internal/web/icons` — SVG validation

### Recommendation Runtime Test Surface (Spec 039)

| Test type | File | Purpose |
|-----------|------|---------|
| unit | `internal/recommendation/store/redact_test.go` | Verifies serialized recommendation logs/traces never leak provider keys, raw payloads, exact GPS, or sensitive graph prompt text (SCN-039-053) |
| integration | `tests/integration/recommendation_metrics_test.go` | Verifies all eight `smackerel_recommendation_*` metrics are emitted with bounded labels (SCN-039-050) |
| integration | `tests/integration/recommendation_watch_audit_test.go` | Verifies per-watch operator counts come from joining `recommendation_watch_runs` on `watch_id` rather than from a high-cardinality Prometheus label (SCN-039-051) |
| stress | `tests/stress/recommendations_test.go` | Drives 50 concurrent warm reactive requests for 5 minutes against the live dev stack and asserts the spec-039 NFR (warm p95 ≤ 10s) is met (SCN-039-052) |
| e2e-api | `tests/e2e/recommendations_full_regression_test.go` | Broad regression covering reactive + watch detail + feedback + why paths, including redaction smoke checks (SCN-039-050..053) |

### Cloud Drives Test Surface (Spec 038)

The cloud-drives surface (Google Drive in scope today) MUST exercise the OAuth web flow, scan + monitor loop on the `DRIVE` NATS stream, Save Rules + Save Service + confirmation flow, and the search/artifact-detail surface.

| Test type | Coverage |
|-----------|----------|
| unit | `DriveProvider` interface compliance per provider, OAuth nonce lifecycle, scan-rule include/exclude + max_depth, Save Rules first-stable-match + conflict audit, sensitivity policy decision matrix, cursor invalidation + bounded-rescan strategy, low-confidence confirmation envelope, agent retrieval tools |
| integration | Live Google Drive fixture against the test stack (`tests/integration/drive/`): `BeginConnect`/`FinalizeConnect` end-to-end with a `drive_oauth_states` row, `drive_connections` row with healthy status + scope JSON + bearer `credentials_ref`, scan/monitor producing `drive_files` rows, change-monitor cursor durability, Save Service routing through provider writers, exactly-once confirmation across web + Telegram |
| e2e-api | Drive-side journeys against the live test stack: Connect → scan progress → search hit → artifact detail with folder context, Save Request → confirmation → provider write, sensitive content blocked at retrieval until reveal/confirm |
| e2e-ui | Screen 2 (provider selector + OAuth handoff), Screen 3 (scan progress + counters), Screen 4 (skipped/blocked grouping), Screen 7 (rule audit + recent saves), Screen 8 (rule dry-run), Screen 11 (low-confidence confirmation) |
| stress | Bulk scan of synthetic large-folder fixtures, sustained change-monitor throughput, and Save Rules evaluation under burst load — all against the disposable test stack |

Required adversarial cases:

- A Save Rule with overlapping conditions must surface every conflicting match in the audit feed, not silently pick first match without provenance.
- A `drive_cursors` row marked "cursor invalid" must trigger a bounded rescan that re-emits only true deltas — duplicates are forbidden.
- A sensitive artifact must NOT appear in retrieval responses without a confirmation/reveal step.
- An OAuth `state` nonce reuse must be rejected at `FinalizeConnect`.

### Cloud Photo Libraries Test Surface (Spec 040)

The cloud-photos surface (Immich and PhotoPrism providers) MUST exercise the provider-neutral `photolib.PhotoLibrary` contract, the unified upload pipeline shared by Telegram/PWA/web, the action-token mint+confirm flow, sensitivity reveal, and the capability taxonomy SST.

| Test type | Coverage |
|-----------|----------|
| unit | `photolib.PhotoLibrary` shape per adapter, capability taxonomy `CheckCapability` + `LimitationDescriptorFor`, cross-provider duplicate signal (strict-hash + weak fallback), lifecycle/dedupe/removal analyzers, scope-hash `PhotoActionToken` mint and drift detection, ConfirmedWriter guard, sensitivity reveal token mint + single-use enforcement, photo routing target classification |
| integration | Live Immich fixture against the test stack (`tests/integration/`): connect/scope/scan, incremental changes, skip-ledger visibility, capability canary (`TestPhotosCapabilityTaxonomyCanary_GoRegistryMatchesPWALimitationCodes`), provider-neutrality canary against the second adapter, lifecycle/dedupe/removal, unified upload pipeline (`TestPhotosUpload_TelegramMobileWebEnterSamePipeline`), document-scan multi-page grouping, sensitivity reveal + audit, `409 PROVIDER_LIMITATION` mapping for unsupported writes |
| e2e-api | Telegram + mobile + web upload → classify → search → retrieve, cross-feature routing produces downstream artifacts (receipt → expense, recipe photo → recipe, document → document), Telegram does NOT auto-send sensitive photos, action plan does NOT mutate before confirm, cross-provider duplicate returned once across two providers |
| e2e-ui | PWA Screens 1–5 (connectors list, add wizard, connector detail, search, photo detail), Photo Health dashboards (lifecycle, duplicates, removal, quality), Photo Health limitations dashboard (8 `data-limitation-code` anchors), confirm-destructive-action screen, mobile docscan |
| stress | `tests/stress/`: synthetic 15,000-photo library ingest with cross-provider duplicates, search p95 budget under burst load |

Required adversarial cases:

- A destructive action must NOT be executed when the action token's scope hash differs from the confirmed scope (drift detection).
- A second adapter (PhotoPrism) marking face-cluster rename UNSUPPORTED MUST return a stable `LimitationCode` from the shared taxonomy, not an ad-hoc string.
- The same photo content present in two providers MUST surface as a single canonical artifact in cross-provider search results.
- A reveal token MUST fail closed on reuse, after TTL, or when used by a different actor than the one who minted it.
- Sensitive search results MUST omit preview URLs and set `requires_reveal=true`.

### QF Companion Connector Test Surface (Spec 041)

Spec 041 adds a pre-MVP connector and companion-surface contract. Current Scope 1 is implemented but not certified complete: the active tests cover connector configuration, read-contract validation, schema-mismatch degradation, health reporting, and the guarantee that Scope 1 publishes no QF artifacts. Later scopes expand the matrix to packet ingest, search/detail surfacing, UI rendering, evidence export, and replay/cursor behavior.

Current Scope 1 coverage:

| Test type | Current Scope 1 coverage |
|-----------|--------------------------|
| unit | Config parsing requires explicit fields; QF client uses read-only `GET` requests; schema mismatch maps to degraded health; auth failures map to error health; DTO JSON names mirror the QF contract |
| integration | Connector registry/health integration, QF read-contract validation, auth failure, schema mismatch, and zero QF artifact publication |
| e2e-api | Live API health includes `connector:qf-decisions`; schema-mismatch sync through `/settings/connectors/qf-decisions/sync` records the bridge error and publishes no trusted artifacts |

| Test type | Required coverage |
|-----------|-------------------|
| unit | Later-scope cursor parsing, packet normalization, required ID/badge validation, and evidence-bundle serialization |
| integration | Later-scope sync against a QF-compatible test read surface, preserving packet IDs, badges, trace IDs, approval state, and degraded-state behavior |
| e2e-api | Later-scope ingest of a QF packet, retrieval through search/recent/detail APIs, and consent-scoped `PersonalEvidenceBundle` export |
| e2e-ui | Later-scope web and Telegram/digest surfaces show QF packet content as read-only with QF source, trust badges, trace/deep links, and no execution controls |
| stress | Later-scope connector sync cycles remain bounded under repeated packet updates and do not duplicate artifacts or lose cursor state |

Required adversarial cases:

- A packet missing calibration or provenance metadata must be degraded rather than silently trusted.
- A packet with a stale cursor must not duplicate an existing QF packet.
- A local Smackerel synthesis must not rewrite the QF thesis or approval state.
- A context export without explicit consent or source provenance must fail.

### Ollama-Backed Agent E2E Test Lane (Spec 043)

Spec 043 closes MIT-037-OLLAMA-001 by adding an opt-in test lane that drives the production NATS + Python sidecar + LiteLLM + Ollama path against a real local model. The lane is gated by an environment variable so the default `./smackerel.sh test e2e` run stays Ollama-free.

| Test type | File | Purpose |
|-----------|------|---------|
| e2e-api (opt-in) | `tests/e2e/agent/happy_path_test.go` | `TestAgentHappyPath_PlanToolSynthesis` (full agent loop against the SST-pinned test model), `TestAgentHappyPath_DeterministicOutput` (3-run byte-identical synthesis under fixed determinism options), `TestOllamaUnreachable_FailsLoudly` (BS-014 fail-loud regression) |
| unit guard | `tests/e2e/agent/no_skip_guard_test.go` | `TestNoSkipBailoutInAgentE2E`, `TestNoSkipBailout_HappyPathTestExplicitlyForbidden`, `TestNoSkipBailout_AdversarialFinding` — enforce that no test in `tests/e2e/agent/` reaches for `t.Skip*` to silently bypass an Ollama-unavailable failure |

#### Running the lane

```bash
SMACKEREL_TEST_OLLAMA=1 ./smackerel.sh test e2e
```

Setting `SMACKEREL_TEST_OLLAMA=1` (or `true`) at the `./smackerel.sh test e2e` entry point causes the runner to:

1. Source the `OLLAMA_TEST_*` env vars from `config/generated/test.env`.
2. Invoke `scripts/commands/ollama-test-pull.sh` to ensure the test model is present in the live test-stack Ollama container's catalog.
3. Run `go test -tags=e2e_ollama ./tests/e2e/agent/...` against the live test stack.

Without `SMACKEREL_TEST_OLLAMA=1`, the runner skips both the pull script and the `e2e_ollama`-tagged tests with an explicit log message; the rest of the E2E suite continues.

#### Required env vars (sourced from `config/generated/test.env`)

| Env var | SST key | Purpose |
|---------|---------|---------|
| `OLLAMA_URL` | `infrastructure.ollama_url` | Base URL for the live test-stack Ollama HTTP API |
| `OLLAMA_TEST_MODEL` | `infrastructure.ollama.test.model` | Pinned test model tag (`qwen2.5:0.5b-instruct`) |
| `OLLAMA_TEST_PULL_TIMEOUT_SECONDS` | `infrastructure.ollama.test.pull_timeout_seconds` | Wall-clock ceiling enforced via `timeout(1)` |
| `OLLAMA_TEST_REQUEST_TEMPERATURE` | `infrastructure.ollama.test.request_temperature` | Determinism: temperature (default `0.0`) |
| `OLLAMA_TEST_REQUEST_TOP_P` | `infrastructure.ollama.test.request_top_p` | Determinism: top-p (default `1.0`) |
| `OLLAMA_TEST_REQUEST_TOP_K` | `infrastructure.ollama.test.request_top_k` | Determinism: top-k (default `1`) |
| `OLLAMA_TEST_REQUEST_SEED` | `infrastructure.ollama.test.request_seed` | Determinism: PRNG seed (default `42`) |
| `OLLAMA_TEST_REQUEST_NUM_PREDICT` | `infrastructure.ollama.test.request_num_predict` | Determinism: max tokens (default `256`) |

Dev and home-lab environments emit these `OLLAMA_TEST_*` keys as empty strings; `ml/app/agent.resolve_ollama_determinism_options()` reads them only when set and passes them to `litellm.acompletion` as `extra_kwargs` only when the routed provider is `ollama` (no-op for `openai` / `anthropic` / etc.).

#### Cold-pull workflow

`scripts/commands/ollama-test-pull.sh` POSTs `/api/pull` to the test-stack Ollama container (host port `47004`) and verifies `/api/tags` after. Exit codes:

| Exit | Meaning |
|------|---------|
| `0` | Pull completed and the model is present in the daemon's catalog |
| `1` | Missing or empty required env var (SST violation) |
| `2` | HTTP error from `/api/pull` (non-2xx, or curl transport failure) |
| `3` | Pull timed out before the daemon reported success |
| `4` | Model still missing from `/api/tags` after the pull reported success |

The first invocation against an empty `smackerel-test-ollama-data` volume incurs a one-time download of the test model (~397 MB). Subsequent runs warm-cache against the same volume; the test compose lifecycle preserves it across `./smackerel.sh down` (only `postgres` and `nats` test volumes are removed by default, so the Ollama warm-cache survives — use `./smackerel.sh clean full` to drop it explicitly).

#### Per-environment model selection

Spec 043 introduces a per-environment override for the agent `fast` provider model so the test lane can pin a small, deterministic model without affecting dev/home-lab routing:

| Environment | `agent_provider_fast_model` | Source |
|-------------|----------------------------|--------|
| `dev` | `gpt-oss:20b` | `config/smackerel.yaml::environments.dev.agent_provider_fast_model` (top-level default) |
| `test` | `qwen2.5:0.5b-instruct` | `config/smackerel.yaml::environments.test.agent_provider_fast_model` (per-env override) |
| `home-lab` | `gpt-oss:20b` | `config/smackerel.yaml::environments.home-lab.agent_provider_fast_model` (matches dev) |

After editing the per-env value, regenerate every environment so the override propagates:

```bash
for env in dev test home-lab; do ./smackerel.sh --env "$env" config generate; done
```

`environments.<env>.ollama_enabled` follows the same per-env pattern: `true` for `test` (the only environment that auto-starts the `ollama` profile when running E2E), `false` for `dev` and `home-lab` (operators opt in locally per `docs/Development.md` runtime contract).

#### Required adversarial cases

- `happy_path_test.go` MUST NOT contain any `t.Skip` / `SkipNow` / `Skipf` call — `TestNoSkipBailout_HappyPathTestExplicitlyForbidden` enforces this; bypass would let an Ollama outage silently pass the lane.
- `TestOllamaUnreachable_FailsLoudly` MUST fail (not skip) when the test-stack Ollama container is unreachable; this is the BS-014 fail-loud contract.
- `ollama-test-pull.sh` MUST exit non-zero on every failure path (missing env var, HTTP error, timeout, or post-pull `/api/tags` mismatch); silent success would mask a corrupt model cache.

## Environment Isolation Rules

### Development State Is Sacred

The persistent development stack exists for manual work only.

- It uses named volumes.
- It must survive CLI restarts.
- It must never be the target for automated E2E, integration, chaos, or validation writes.

### Test State Must Be Disposable

The automated test environment must use ephemeral storage.

- PostgreSQL test data should use project-scoped test volumes that are removed during test cleanup.
- JetStream or queue state used by tests should use test-scoped volumes removed during cleanup.
- Extracted artifact scratch data and temp uploads should be disposable.
- Tests should create uniquely identifiable synthetic fixtures.

### Validation And Chaos Must Be Isolated

Certification, validation, and chaos runs must use isolated runtime state.

- Use a separate Compose project name.
- Use disposable stores.
- Never tear down another active session's runtime implicitly.

## E2E Requirements

Smackerel must adopt the same live-stack standards as the stronger repos.

### Live Stack Only

- `integration`, `e2e-api`, and `e2e-ui` must hit the real running stack.
- Request interception in live categories is forbidden.
- If a test uses interception or canned responses, it must be reclassified out of live categories.

### E2E Uses The Test Stack Only

`./smackerel.sh test e2e` must boot or attach to the ephemeral test stack, never the persistent dev stack.

Required behavior:

- Start disposable test storage.
- Run migrations or schema setup against the test store.
- Seed only synthetic test data.
- Start the runtime against the test environment.
- Execute live-stack E2E coverage.
- Tear down or reset disposable state safely.

### Bug Fixes Need Adversarial Regressions

Every bug fix regression must include at least one case that would fail if the bug returned.

- Tautological fixtures are forbidden.
- Silent-pass bailout logic is forbidden.
- Missing required controls or redirects must fail loudly.

## Verification Standards

Smackerel inherits the Bubbles evidence rules:

- Pass/fail claims require executed commands.
- Test evidence must include raw command output, not summaries.
- Long-running commands must use explicit timeouts.

When new runtime categories are added, update this file, the command registry, and copilot instructions in the same change set so the documented test surface matches reality.