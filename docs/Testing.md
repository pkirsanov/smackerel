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
- **ntfy source adapter tests**: explicit `NTFY_SOURCES_JSON` validation, webhook receiver registration, valid ntfy message ingest through `SourceEventSink`, malformed payload dead-lettering, reconnect state, topic lag, replay-through-source-sink idempotency, multi-topic/multi-instance provenance, and no output-channel coupling

## Current Validation Surface

The repository now exposes a sanctioned CLI-owned runtime test surface for the foundation scaffold while retaining the Bubbles framework checks for governance work.

| Test type | Command | Required when |
|-----------|---------|---------------|
| Go + Python unit | `./smackerel.sh test unit` | Runtime code changes |
| Integration | `./smackerel.sh test integration` | Runtime lifecycle or health changes |
| Integration (stores-only, light) | `./smackerel.sh test integration-light` | Integration tests needing ONLY postgres+nats (in-process Go against the stores); LIGHT preflight floor (2000 MB / 8 GB) |
| End-to-end | `./smackerel.sh test e2e` | Runtime, compose, or config changes |
| End-to-end UI (PWA browser) | `./smackerel.sh test e2e-ui` | PWA `.spec.ts` under `web/pwa/tests/` changes, login/auth UI, CSP, or served-route shape changes |
| Stress smoke | `./smackerel.sh test stress` | Runtime health, lifecycle, or stress env handoff changes |
| Chrome extension e2e (MV3) | `./smackerel.sh test e2e-ext` | `extensions/chrome-bridge/` MV3 background/content/transport code or its Playwright e2e harness changes (self-contained; real headless Chromium, no live stack) |
| Chrome extension supply-chain | `./smackerel.sh test extension-supplychain` | Chrome-bridge sideload-zip packaging or its LOCAL cosign sign / `verify-blob` proof changes (offline ephemeral key; no public-Rekor upload) |
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
| Integration (stores-only, light) | `integration-light` | `./smackerel.sh test integration-light` |
| End-to-end API | `e2e-api` | `./smackerel.sh test e2e` |
| End-to-end UI | `e2e-ui` | `./smackerel.sh test e2e` (web UI paths included) |
| End-to-end UI (PWA browser) | `e2e-ui` | `./smackerel.sh test e2e-ui` |
| Stress | `stress` | `./smackerel.sh test stress` |
| Chrome extension e2e (MV3) | `e2e-ext` | `./smackerel.sh test e2e-ext` |
| Chrome extension supply-chain | `extension-supplychain` | `./smackerel.sh test extension-supplychain` |

### Stores-Only Integration-Light Lane (`test integration-light`, OPS-005 F-RUNBOOK)

`./smackerel.sh test integration-light [--go-run <regex>]` is a fast, low-RAM
sibling of `test integration` for integration tests that need ONLY the two
durable stores — postgres + nats — exercised in-process by Go (e.g. a
`pipeline.Processor` writing to Postgres + publishing to NATS). It brings up
ONLY postgres + nats from the generated test stack: **NO core/ml image build,
NO `ml_sidecar` health gate**. The two stores are health-gated via their compose
healthchecks (postgres `pg_isready` + `SELECT 1`; nats `/healthz`), and the
stack is torn down on ANY exit (success or failure) by the lane's teardown trap.

- **Resource floor (LIGHT profile):** gated FIRST by the LIGHT preflight pair
  `PREFLIGHT_MIN_AVAILABLE_RAM_MB_LIGHT` / `PREFLIGHT_MIN_AVAILABLE_DISK_GB_LIGHT`
  = **2000 MB / 8 GB** (vs the heavy lane's 6000 MB / 15 GB). On a host below the
  floor the lane refuses cleanly and starts NO stack.
- **When to use the heavy lane instead:** tests that need `smackerel-core` or
  `smackerel-ml` (HTTP endpoints, the `ml_sidecar`, core-created JetStream
  streams, or core-applied DB migrations) MUST use the heavy
  `./smackerel.sh test integration`.

### PWA Browser e2e-ui Harness (Spec 077)

Spec 077 ships a Playwright-driven browser harness for the PWA. It runs
under the dedicated disposable Compose project `smackerel-test-e2e-ui`
so it cannot collide with the persistent dev stack or the
`smackerel-test` lane used by Go integration/e2e/stress.

| Concept | Value |
|---------|-------|
| Dispatcher subcommand | `./smackerel.sh test e2e-ui` |
| Compose project | `smackerel-test-e2e-ui` |
| Spec discovery convention | `web/pwa/tests/*.spec.ts` (auto-discovered via `testDir: "tests"` + `testMatch: "**/*.spec.ts"` in `web/pwa/playwright.config.ts`; `_support/` is excluded) |
| Helpers directory | `web/pwa/tests/_support/` (e.g. `env.ts`, `csp.ts`) — never picked up as specs |
| SST consumer | `SMACKEREL_BASE_URL` (derived from `CORE_EXTERNAL_URL` in `config/generated/test.env`); fail-loud if unset |
| Proof-of-life smoke | `web/pwa/tests/proof_of_life.spec.ts` (asserts HTTP 200 OR 401 against `/` — the disposable stack is auth-gated) |
| Artifact paths on failure | `web/pwa/test-results/`, `web/pwa/playwright-report/` |
| CI workflow | `.github/workflows/e2e-ui.yml` (runs on every push to `main` and PR; uploads artifacts on failure) |

Adding a new browser test is as simple as dropping a `*.spec.ts` file
under `web/pwa/tests/` — the discovery convention is the only contract
and is asserted by `tests/unit/web/spec_077_discovery_convention_test.sh`.

Note on Chromium sandboxing in CI: this repo currently runs the
harness with the default Playwright Chromium sandbox enabled. If a
future CI runner cannot enable user namespaces (for example, a
container without `CAP_SYS_ADMIN`) the workflow may need to pass
`--no-sandbox` to Chromium. That flag drops a defense-in-depth layer
and MUST be justified inline in the workflow if ever introduced — see
spec 077 Hard Constraint 7.

### Spec 055 — ntfy Source Adapter Test Surface

Spec 055 test coverage is focused on the adapter boundary: ntfy transport and
payload translation live in `internal/notification/source/ntfy`, while the core
notification package remains source-neutral. Tests must prove that accepted
ntfy events enter through `SourceEventSink`, malformed or rejected events are
dead-lettered with redaction, the first accepted replay goes back through the
source sink, repeated replay is side-effect idempotent, and the adapter never
performs output dispatch.

| Test type | Command | Primary coverage |
|-----------|---------|------------------|
| Unit | `./smackerel.sh test unit --go --go-run 'TestNtfy\|TestValidate_DBMaxConns_Missing' --verbose` | Config validation, explicit `auth_mode=none`, secret-reference validation for credential-backed modes, mapper field preservation, lifecycle handling, health transitions, dead-letter eligibility, replay boundaries, webhook route unit coverage, and no-output-coupling guards. |
| Integration | `./smackerel.sh test integration --go-run 'TestNtfy'` | Runtime startup from `NTFY_SOURCES_JSON`, webhook receiver registration, raw/normalized persistence through the source sink, invalid enabled source health, reconnect/lag state, replay attempts, replay burst idempotency, and multi-topic/multi-instance provenance. |
| E2E API/UI | `./smackerel.sh test e2e --go-run 'TestNtfy'` | Authenticated webhook ingest, malformed payload rejection, source detail, reconnect, dead-letter list/detail/replay, idempotent replay API responses, operator web source pages, replay confirmation copy, redaction, and source/output boundary markers. |
| Stress | `./smackerel.sh test stress --go-run 'TestNtfy'` | Concurrent webhook burst acceptance, duplicate source-event provenance, malformed-payload burst dead-lettering, reconnect control under load, and redaction of secret-like malformed content. |
| Format | `./smackerel.sh format --check` | Go/Python formatting stays clean after ntfy changes. |
| Lint | `./smackerel.sh lint` | Go vet, Python ruff, and web asset validation stay clean after ntfy changes. |
| Artifact lint | `bash .github/bubbles/scripts/artifact-lint.sh specs/055-notification-source-ntfy-adapter` | Spec packet structure remains valid. |
| Traceability guard | `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/055-notification-source-ntfy-adapter` | Scenario manifest, tests, report evidence references, and DoD mappings remain coherent. |

Current concrete ntfy test files:

| File | Coverage |
|------|----------|
| `internal/api/notifications_ntfy_test.go` | Production webhook route dispatches to the registered receiver and rejects malformed, wrong-topic, not-running, and non-webhook-source cases. |
| `internal/notification/source/ntfy/config_test.go` | Enabled instances require explicit identity, form/mode, endpoint identity, topic set, config hash, redacted metadata, and credential refs except explicit `auth_mode=none`. |
| `internal/notification/source/ntfy/mapper_test.go` | ntfy fields, unknown safe fields, lifecycle events, priority/tag hints, and raw JSON mapping. |
| `internal/notification/source/ntfy/health_test.go` | Connected/degraded/disconnected transitions, retry count, lag thresholds, and redacted error categories. |
| `internal/notification/source/ntfy/dead_letter_test.go` | Redacted dead-letter causes, payload hashes/previews, replay eligibility, and bounded sink retry. |
| `internal/notification/source/ntfy/provenance_test.go` | Topic/source-instance provenance and loop metadata preservation. |
| `internal/notification/source/ntfy/no_output_coupling_test.go` | Static guard that ntfy production files do not import output-channel packages or dispatch paths. |
| `internal/notification/no_ntfy_core_dependency_test.go` | Static guard that core notification production code has no ntfy-specific dependency. |
| `internal/notification/source/ntfy/*_integration_test.go` | Live PostgreSQL-backed adapter integration for raw/normalized records, invalid source health, reconnect state, replay, replay burst idempotency, and provenance. |
| `tests/integration/notification_ntfy_runtime_test.go` | Runtime startup from JSON and webhook receiver registration. |
| `tests/e2e/notification_ntfy_source_api_test.go` | Live API workflow for webhook ingest, reconnect, dead-letter list/detail, replay, idempotent repeat replay responses, invalid limit, and redaction. |
| `tests/e2e/notification_ntfy_source_ui_test.go` | Operator web pages for source list, ntfy detail, dead letters, replay confirmation, and boundary text. |
| `tests/stress/notification_ntfy_source_stress_test.go` | Burst webhook and malformed/dead-letter/reconnect stress coverage. |

Evidence for spec 055 closeout should be recorded in
`specs/055-notification-source-ntfy-adapter/report.md` with `**Claim Source:**
tags. For docs-only alignment, rerun artifact lint and traceability guard after
the documentation edits so the spec packet still passes its governance checks.

### Go Package Coverage

All 38 Go packages have tests:

- `internal/api` — capture, search, health, digest, recent, annotations, lists handlers
- `internal/annotation` — freeform parser, store CRUD, materialized summary, Telegram message mapping
- `internal/auth` — OAuth2 provider (token exchange + encrypted token storage); spec 044 per-user PASETO v4.public bearer auth (issue/verify/hash, rotation grace window, `Session` context helpers, startup fail-loud guard, SST grep guard); revocation cache + NATS broadcaster (`internal/auth/revocation`)
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

### Per-User Bearer Auth Test Surface (Spec 044)

Spec 044 Scope 01 ships the per-user PASETO v4.public bearer-auth foundation
(SST keys, token issue/verify/hash, revocation cache + NATS broadcaster, CLI
subcommands, admin HTTP handlers, DB migration 033, startup fail-loud guard).
Scope 02 wires the per-user `bearerAuthMiddleware` onto the API hot path,
registers the four admin HTTP routes, and closes three cross-spec body-actor
trust-boundary issues in production mode (MIT-040-S-008 photos mint/reveal,
MIT-038-S-003 cloud-drive Connect, MIT-027-TRACE-001 actor-source segment for
annotations). Test coverage matches scope DoD:

| Test type | Files | Coverage |
|-----------|-------|----------|
| unit | `internal/config/validate_test.go` (8 sub-tests), `internal/auth/issue_test.go`, `internal/auth/verify_test.go`, `internal/auth/startup_test.go` (8 sub-cases), `internal/auth/sst_grep_guard_test.go` (+ adversarial + allowlist), `internal/auth/revocation/cache_test.go`, `internal/api/router_auth_middleware_test.go` (Scope 02 — production PASETO + dev/test shared-token + empty-token bypass branches), `internal/api/auth_actor_grep_guard_test.go` (Scope 02 AC-11 grep guard with adversarial fixture) | Loader and runtime fail-loud branches (production+enabled+empty-signing-key, +empty-key-id, +empty-hashing-key, +hashing-key==signing-key per OQ-8); PASETO sign/verify round-trip; rotation grace window honoring prior key (incl. forged-kid adversarial); revocation cache bootstrap → propagation → idempotency; SST grep guard for hardcoded auth values across `internal/` and `cmd/`; middleware mode-branch coverage; AC-11 grep guard (zero production-applicable header-trust paths) |
| integration | `tests/integration/auth_bootstrap_test.go`, `tests/integration/auth_mintreveal_test.go`, `tests/integration/auth_drive_connect_test.go`, `tests/integration/auth_annotation_test.go`, `tests/integration/auth_rotation_test.go`, `tests/integration/auth_revocation_test.go` (all build tag `integration`) | Live test-stack bootstrap on a fresh production-mode DB: `Enroll` → `IssueToken` → `HashToken` → `PersistToken` round-trip; `auth_users.user_id` UNIQUE adversarial; PASETO public-hex derivation. Scope 02 MIT-closure verification: photos mint/reveal rejects body `actor_id`, drive `Connect` rejects body `owner_user_id`, annotation create rejects body `actor_source` — all `HTTP 400` with the documented error codes against the live test-stack core (host port 45001) backed by postgres 47001 + NATS 47002. Rotation grace timeline: prior token admits during the grace window then rejects after `expires_at`. Revocation propagation: revoke broadcasts on the configured NATS subject and the next request returns `HTTP 401`; NATS-down fallback exercises `Cache.Refresh(ctx, store)` against `BearerStore.LoadRevokedTokenIDs` to close the staleness window. |
| stress / chaos | `tests/integration/auth_chaos_test.go` (Scope 01 — 7 stress scenarios + 1 informational benchmark), `tests/integration/auth_chaos_scope02_test.go` (Scope 02 — 11 behaviors C2-B01..C2-B11 covering concurrent middleware-verify, verify-vs-revoke race, concurrent mint/reveal under MIT-040-S-008 closure, concurrent drive `Connect` under MIT-038-S-003 closure, concurrent annotation create under MIT-027-TRACE-001 closure, rotation under load, revocation under load, admin endpoint stress, malformed-Authorization-header storm, `-race -count=20` stress loop, pure-CPU middleware benchmark) | Concurrent enrollment with UNIQUE rejection; concurrent rotate-vs-verify across the grace window; revocation broadcaster race; cache bootstrap under concurrent load; broadcaster malformed-payload defensive handling; migration idempotency; token boundary conditions; pure-CPU verify benchmark vs NFR-AUTH-001 5 ms hot-path budget; Scope 02 hot-path race-clean stress + body-key adversarial fixtures + admin-endpoint contention. |

Required adversarial cases:

- Hashing key equal to the signing private key MUST fail loud at startup
  (`internal/auth/startup_test.go` covers all four production-mode fail-loud
  branches per spec 044 OQ-8).
- A token signed with a `kid` not matching either the active or prior key MUST
  surface `auth.ErrUnknownKeyID`.
- A second `Enroll` for the same `user_id` MUST surface a uniqueness error
  (UNIQUE constraint on `auth_users.user_id`).
- The SST grep guard MUST detect a fresh hardcoded PASETO key inserted into the
  source tree (verified by the adversarial sub-test injecting a literal pattern
  outside the allowlist).
- Production mode MUST reject body-supplied `actor_id` / `owner_user_id` /
  `actor_source` on the photos mint/reveal, drive `Connect`, and annotation
  create handlers respectively — required adversarial integration tests
  `TestMintReveal_BodyActorIDInProduction_Returns400_FailsLoudly`,
  `TestDriveConnect_OwnerInBody_Production_Returns400`,
  `TestAnnotation_BodyActorSourceInProduction_Rejected`.
- Dev/test mode MUST continue to honor body-supplied actor identifiers and the
  `X-Actor-Id` header so existing local-dev fixtures work unchanged (covered by
  the same MIT-closure integration tests via mode-branch sub-cases).
- After token rotation, the prior token MUST be rejected once `expires_at`
  passes (`TestRotation_AfterGraceWindow_OldTokenRejected` with `expired` /
  `exp claim` / `signature` / `verify` body-content adversarials).
- A revoked token MUST be rejected on the next request with `HTTP 401` and the
  401 body MUST NOT leak `revoked` / `revocation` / `cache hit` strings
  (`TestRevocation_RevokedTokenRejectedOnNextRequest` with NFR-AUTH-007 body
  content adversarial).

Run live integration coverage (test stack must be up — host ports postgres
47001, NATS 47002, smackerel-ml 45002, smackerel-core 45001):

```bash
./smackerel.sh --env test up
go test -count=1 -tags=integration -v -timeout=180s \
  -run 'Test(Auth|MintReveal|DriveConnect|Annotation|Rotation|Revocation)' \
  ./tests/integration/...
```

Scope 02 `bearerAuthMiddleware` integration is exercised end-to-end by the
files above. Scope 03 (PWA / extension / Telegram bridge / admin UI) test
inventory is documented in the next subsection. Scope 04 (deprecation +
metrics + F02 closure) test inventory is in the subsection after that and
is fully tracked under
`specs/044-per-user-bearer-auth/scenario-manifest.json`.

### Per-User Bearer Auth — Scope 03 Test Inventory (Spec 044)

Scope 03 lands four caller-side surfaces (PWA cookie session, browser
extension Authorization header, Telegram per-user bridge, admin
token-management UI) plus the Scope 03 chaos suite. Test files (zero mocks,
zero `t.Skip()`):

| Surface | Test files | Build tag | Coverage |
|---|---|---|---|
| PWA cookie-derived session E2E | `tests/e2e/auth/pwa_per_user_test.go` (4 tests + 5 sub-tests) | `e2e` | `TestE2E_PWAAuth_Production_PerUserSession` (login → cookie → authenticated photos round-trip), `TestE2E_PWAAuth_Production_LoginRejectsMissingToken` (3 sub-tests: empty body, empty token, whitespace token), `TestE2E_PWAAuth_Production_LoginRejectsInvalidToken` (2 sub-tests: random garbage, foreign-signed PASETO), `TestE2E_PWAAuth_Production_AuthorizationHeaderStillWorks` (header path preserved alongside cookie). Discharges `FINALIZE-PREREQ-044-V7-001`. |
| Browser extension bearer forward (live) | `tests/integration/auth_extension_test.go` (3 tests + 4 sub-tests) | `integration` | `TestExtensionAuth_PerUserPASETO_AdmitsAndAttachesSession` (mint via `auth.IssueToken` → header forward → admit + session attach), `TestExtensionAuth_MalformedBearer_Production_Returns401` (4 sub-tests: empty / garbage / missing-space / wrong-scheme), `TestExtensionAuth_RevokedPerUserToken_Returns401` (`BearerStore.RevokeToken` + `RevocationCache.MarkRevoked` propagation). |
| Telegram per-user bridge (live) | `tests/integration/auth_telegram_e2e_test.go` (3 tests) | `integration` | `TestTelegramBridge_MintsPerUserBearer_AdmitsRequest` (mint via `PerUserTokenMinter.MintForChat` → bearer-authenticated annotation create succeeds), `TestTelegramBridge_UnmappedChat_MinterRefusesAndCallerCannotProceed` (production unmapped chat → `ErrNoUserMappingForChat` → caller drops with no API call), `TestTelegramBridge_BodyClaimedActorRejected` (Telegram-minted PASETO admits middleware but body-claimed `actor_source: "telegram"` is rejected by the production handler defense from Scope 02 — closes MIT-027-TRACE-001 actor-source contract end-to-end through the Telegram path). |
| Admin token-management UI (live) | `tests/integration/auth_admin_ui_test.go` (3 tests + 3 sub-tests) | `integration` | `TestAdminUI_WithBearer_Returns200HTML` (5 functional content markers + `Content-Type` + `Cache-Control: no-store` + `X-Content-Type-Options: nosniff` + non-empty CSP), `TestAdminUI_WithoutBearer_Production_Returns401`, `TestAdminUI_DisallowedMethods_Return405` (POST / PUT / DELETE sub-tests). |
| Web login handler unit | `internal/api/web_login_test.go` (11 tests with sub-tests) | (none) | Production PASETO accept / foreign-signed reject / revoked reject; dev shared accept / wrong-token reject; dev-bypass `RefusesLogin`; body validation (5 cases); method not-allowed; logout cookie clear (production + dev); `extractBearerToken` cookie fallback (5 cases). |
| Per-user token minter unit | `internal/telegram/per_user_token_test.go` (8 tests) | (none) | `TestNewPerUserTokenMinter_Validates`, `TestNewPerUserTokenMinter_DefaultsAppliedWhenZero`, `TestMintForChat_Production_MappedChat_ProducesVerifiableToken` (round-trip via `auth.VerifyAndParse`), `TestMintForChat_Production_UnmappedChat_ReturnsError`, `TestMintForChat_Production_EmptyMapping_RejectsAll`, `TestMintForChat_Dev_UnmappedChat_ReturnsZeroAndNil`, `TestMintForChat_Dev_MappedChat_StillMintsForCorrectness`, `TestMintForUser_RejectsEmptyUserID`, `TestMintForChat_AdversarialNoBodyTrust` (chat-id never leaks into PASETO claims), `TestMintForChat_FreshTokenIDPerCall` (token id regenerated per call). |
| User mapping parser + resolver unit | `internal/telegram/user_mapping_test.go` (6 tests) | (none) | `TestParseUserMapping` (12 sub-tests: empty / single / two / whitespace / negative chat-id / missing colon / missing user_id / missing chat_id / non-numeric / duplicate / empty pair), `TestResolveActorUserID_Production_RejectsUnmappedChat`, `TestResolveActorUserID_Production_AcceptsMappedChat`, `TestResolveActorUserID_Production_EmptyMappingRejectsAll`, `TestResolveActorUserID_Dev_AllowsMappedAndUnmapped`, `TestResolveActorUserID_Production_CaseInsensitiveEnv`, `TestResolveActorUserID_NilBot`. |
| Scope 03 chaos behaviors (live) | `tests/integration/auth_chaos_scope03_test.go` (5 tests + 1 hot-path benchmark) | `integration` | `TestAuthChaos_S03_PWALoginCookieJarChurn_NoSessionInterleave` (50 jars × 10 reuses; distinct synthetic `RemoteAddr` per jar to bypass per-IP login rate-limit; 0 jar leaks), `TestAuthChaos_S03_ExtensionTokenRotationRace_GraceWindowSurvives` (3-cohort classify pattern; `authReject == 0`; chi `Throttle(100)` 503s classified orthogonal; adversarial lower-bound prevents false-pass via 100% throttle), `TestAuthChaos_S03_TelegramMappingConcurrentReads_NoRaceNoLeak` (100 mapped + 100 unmapped + 20 parser allocations per iter), `TestAuthChaos_S03_AdminUIUnderRevocationRace_HTMLOrCleanReject` (revoker injected mid-burst on real test-stack NATS subject; every response 200+HTML or 401-clean; zero panic/torn/leak/cache-leak), `TestAuthChaos_S03_TelegramMintUnderDBPressure_AllSucceed` (50 concurrent mints under DB pressure; all unique TokenIDs — validates design §11 mint-path-DB-independent invariant). All 5 PASS 20/20 under `-race -count=20`; `TOTAL-FAIL-COUNT=0`; `RACE-MARKERS=0`. Hot-path benchmark `BenchmarkAuthChaos_S03_PWACookieDerivedSession_HotPath`: **1,477,561 ns/op** / **20,782 B/op** / **200 allocs/op** at b.N=10000 single-threaded against the live test stack (full DB roundtrip + chi middleware chain + PASETO verify + bearer cache + handler) — well below NFR-AUTH-001 ≤5 ms p99 budget. |

Required adversarial cases for Scope 03 (12 cases tabulated under
[`specs/044-per-user-bearer-auth/report.md` → Test Evidence (Scope 03) → Adversarial Coverage Summary](../specs/044-per-user-bearer-auth/report.md)):

- Foreign-signed PASETO accepted in production →
  `TestE2E_PWAAuth_Production_LoginRejectsInvalidToken/foreign-signed_paseto`.
- Whitespace-only token accepted →
  `TestE2E_PWAAuth_Production_LoginRejectsMissingToken/whitespace_token`.
- Missing token in body accepted →
  `TestE2E_PWAAuth_Production_LoginRejectsMissingToken/empty_body` +
  `/empty_token`.
- Random garbage token accepted →
  `TestE2E_PWAAuth_Production_LoginRejectsInvalidToken/random_garbage`.
- Body-claimed `actor_source` overrides session-derived in Telegram path →
  `TestTelegramBridge_BodyClaimedActorRejected` (closes MIT-027-TRACE-001
  end-to-end through the Telegram entry point).
- Unmapped Telegram chat ID minted any token in production →
  `TestMintForChat_Production_UnmappedChat_ReturnsError` +
  `TestTelegramBridge_UnmappedChat_MinterRefusesAndCallerCannotProceed`.
- Empty Telegram user mapping admits all chats →
  `TestMintForChat_Production_EmptyMapping_RejectsAll`.
- Body-claimed `actor_id` not verified through full path →
  `TestMintForChat_AdversarialNoBodyTrust`.
- Malformed bearer header (4 cases: empty / garbage / missing-space /
  wrong-scheme) admitted →
  `TestExtensionAuth_MalformedBearer_Production_Returns401`.
- Revoked per-user token still admits requests →
  `TestExtensionAuth_RevokedPerUserToken_Returns401`.
- Admin UI loads without bearer in production →
  `TestAdminUI_WithoutBearer_Production_Returns401`.
- Admin UI accepts non-GET methods (POST / PUT / DELETE) →
  `TestAdminUI_DisallowedMethods_Return405`.

Each adversarial test would FAIL if the underlying invariant were weakened —
zero tautological regressions; zero `t.Skip()`; zero bailout returns.

Run Scope 03 live integration coverage (test stack must be up — host ports
postgres `47001`, NATS `47002`, smackerel-ml `45002`, smackerel-core `45001`):

```bash
./smackerel.sh --env test up
./smackerel.sh test integration --go-run '^TestExtensionAuth_|^TestTelegramBridge_|^TestAdminUI_'
./smackerel.sh test integration --go-run '^TestAuthChaos_S03_'
./smackerel.sh test e2e --go-run '^TestE2E_PWAAuth_'
```

### Per-User Bearer Auth — Scope 04 Test Inventory (Spec 044)

Scope 04 closes F02 (Telegram-bridge per-user PASETO wiring) and ships
the `smackerel_auth_*` metrics surface. Test files (zero mocks, zero
`t.Skip()`):

| Surface | Test files | Build tag | Coverage |
|---|---|---|---|
| Auth metrics surface | `internal/metrics/auth_test.go` (8 tests) | (none) | `TestAuthMetrics_EmitsAllExpectedSeries` (all 7 series surface from `prometheus.DefaultGatherer` after seeding via `seedAllAuthMetrics`), `TestAuthIssuance_IncrementsBySource` (3 sources: `admin_api`, `bootstrap_cli`, `telegram_bridge`), `TestAuthRotation_Increments`, `TestAuthRevocation_NormalizesReason` (11 cases incl. an adversarial Bobby-Tables SQL-injection-like input — proves the bucket stays in the closed set), `TestAuthRevocation_IncrementsBucketed`, `TestAuthValidationLatency_RecordsObservation` (verified via `histogramSampleCount` helper), `TestAuthValidationOutcome_AcceptsClosedSetLabels` (5 results × 2 sources), `TestAuthLegacyFallbackUsed_OperatorVisibility`, `TestAuthFailure_AcceptsClosedSetLabels` (6 reasons), `TestAuthMetrics_NamesUseCanonicalPrefix` (every metric name starts with `smackerel_auth_`). |
| F02 wiring (in-package unit) | `internal/telegram/bot_wiring_test.go` (8 tests) | (none) | `TestBot_bearerForChat_NilMinter_FallsBackToSharedToken` (legacy dev path), `TestBot_bearerForChat_NilMinter_EmptyAuthToken_ReturnsEmpty` (dev empty-token bypass), `TestBot_bearerForChat_WithMinter_MappedChat_ReturnsPerUserPASETO` (production happy path; PASETO `v4.public.` prefix verified), `TestBot_bearerForChat_WithMinter_DevUnmappedChat_FallsBackToShared` (dev fallback contract), `TestBot_bearerForChat_WithMinter_ProdUnmappedChat_PropagatesError` (production safety: `ErrNoUserMappingForChat` propagated, no shared-bearer leak), `TestBot_setBearerHeader_NilMinter_AppliesSharedToken`, `TestBot_setBearerHeader_EmptyToken_LeavesHeaderUnset` (dev bypass — header MUST be unset), `TestBot_setBearerHeader_ProdUnmappedChat_PropagatesError` (helper-level safety propagation). |
| F02 wiring (live integration) | `tests/integration/auth_telegram_f02_wiring_test.go` (2 tests) | `integration` | `TestF02Wiring_SetPerUserTokenMinter_HappyPath` (Bot.SetPerUserTokenMinter wired against live test-stack pool → outbound request prepared via setBearerHeader carries fresh `v4.public.` PASETO bearer → bearerAuthMiddleware admits with HTTP 200 → `smackerel_auth_token_issuance_total{source="telegram_bridge"}` ticks by exactly 1 — sentinel `WRONG-shared-bearer-DO-NOT-USE-IN-F02-PATH` planted in `b.authToken` to catch silent fallback regressions), `TestF02Wiring_SetPerUserTokenMinter_ProductionUnmappedRefuses` (production unmapped chat → setBearerHeader errors → Authorization header unset → metric counter delta = 0). |

Required adversarial cases for Scope 04:

- Free-text revocation reason must NOT escape the closed bucket set →
  `TestAuthRevocation_NormalizesReason/bobby-tables-sql-style` (asserts
  `NormalizeRevocationReason("compromise; DROP TABLE auth_tokens; --")`
  buckets to `compromise`, not the raw string).
- Production unmapped chat must NOT silently fall back to the shared
  bearer when a minter is wired →
  `TestBot_bearerForChat_WithMinter_ProdUnmappedChat_PropagatesError`
  (sentinel `WRONG-shared-bearer-DO-NOT-USE` planted in `b.authToken`).
- F02 wiring observable through live HTTP path →
  `TestF02Wiring_SetPerUserTokenMinter_HappyPath` proves the bearer
  attached is a fresh PASETO (verified by `v4.public.` prefix +
  middleware admit) AND the metric ticks; an inverse case
  (`TestF02Wiring_SetPerUserTokenMinter_ProductionUnmappedRefuses`)
  proves the metric counter delta is exactly 0 when the bot refuses.

Run Scope 04 coverage:

```bash
go test ./internal/metrics/ -count=1
go test ./internal/telegram/ -count=1 -run '^TestBot_bearerForChat|^TestBot_setBearerHeader'
./smackerel.sh --env test up
DATABASE_URL='postgres://smackerel:smackerel@127.0.0.1:47001/smackerel?sslmode=disable' \
  go test -tags integration -count=1 -run '^TestF02Wiring_' ./tests/integration/
```

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

#### Scope 5 Test Surface (Spec 041)

Scope 5 hardens credential rotation, safety boundaries, the symmetric
`smackerel_qf_*` metric set, and Cross-Product Audit Envelope v1. It adds
the following live-stack test surface on top of Scopes 1-4:

| Test type | Test file | Functions / coverage |
|-----------|-----------|----------------------|
| unit | `internal/connector/qfdecisions/credentials_test.go` | `TestPlanCredentialRotationSelectsNewestValidCredentialAndPreservesState`, `TestPlanCredentialRotationRejectsInvalidCredentialBoundaries` (24h overlap rule, newest-by-`not_before` selection, cursor / evidence-export / capability state preservation, diagnostics for all reject paths) |
| unit | `internal/connector/qfdecisions/boundary_test.go` | `EnforceQFActionBoundary` rejects all 8 pre-MVP forbidden action types (`approval`, `execution`, `mandate_change`, `emergency_stop`, `watch_creation`, `watch_evaluation`, `callback_acceptance`, `qf_trust_reconstruction`) and emits a boundary-kick audit envelope per attempt |
| unit | `internal/connector/qfdecisions/audit_test.go` | `BuildCrossProductAuditEnvelopeV1` field shape, default-fill for `actor_ref` / `surface`, RFC3339 timestamp normalization, per-event field projection |
| unit | `internal/connector/qfdecisions/metrics_test.go` + `internal/metrics/metrics_test.go` | All 12 `smackerel_qf_*` metrics declared and registered exactly once with the label sets contracted by QF design 063 |
| integration | `tests/integration/qf_credential_rotation_test.go` | `TestQFCredentialRotationOverlapPreservesCursorExportIdempotencyCapabilityDiagnosticsAndAudit` — full rotation against the live disposable stack with an httptest QF stub; asserts cursor + evidence-export preservation, capability re-read with the new bearer token, and the `capability_handshake ok → credential_rotation ok → capability_handshake ok` audit envelope sequence |
| integration | `tests/integration/qf_scope5_observability_test.go` | Live-stack proof that the 12-metric set emits with QF design 063 label parity across sync / render / export / boundary kick paths |
| integration | `tests/integration/qf_audit_envelope_test.go` | Live-stack proof that the 8 Smackerel-side emission points (`packet_ingest`, `evidence_export_attempt`, `evidence_revocation`, `deep_link_render`, `capability_handshake`, `action_boundary_kick`, plus the two pre-MVP helpers `engagement_signal_flush` / `callback_attempt`) build envelopes with the always-required field set |
| e2e | `tests/e2e/qf_scope5_safety_observability_test.go` | `TestQFCredentialRotationPreservesCursorAndEvidenceStateThroughLiveSurface` (line 352, SCN-SM-041-019), `TestQFSafetyBoundaryAndMetricSetThroughLiveSyncRenderExportSurface` (line 664, SCN-SM-041-020), `TestQFAuditEnvelopeV1RecordedForRequiredBridgeEventsThroughLiveSurface` (line 1118, SCN-SM-041-021) — each runs against the live disposable test stack (core 45001, ml 45002, postgres 47001, nats 47002, ollama 47004) plus an httptest QF stub |
| stress | `tests/stress/qf_freshness_test.go::TestQFDecisionsFreshnessSLAP95RenderAndCombined` | Render-stage and combined (ingest → render) freshness gauges meet the p95 SLA with the operator-documented headroom |

Run locally against the disposable test stack with the standard repo CLI:

```bash
./smackerel.sh --env test test integration
./smackerel.sh --env test test e2e --go-run 'QFScope5|QFCredentialRotation|QFSafetyBoundary|QFAuditEnvelopeV1'
./smackerel.sh --env test test stress
```

Each command stands the test stack up on the disposable ports listed above, runs
the matching suite, then tears the stack down with `--volumes`. No dev state is
touched.

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

### SearxNG Test Container (Spec 064 SCOPE-07)

The open-ended knowledge agent web search provider is backed by a self-hosted SearxNG container behind the `searxng` Compose profile. The `test` env auto-enables the profile (`environments.test.searxng_enabled: true`) so `./smackerel.sh test integration` brings up `searxng/searxng:2026.5.30-bd863f16b` alongside the rest of the test stack and injects `OPEN_KNOWLEDGE_SEARXNG_URL=http://searxng:8080` into the Go test runner. `dev` and `home-lab` ship with the profile disabled; developers can opt in for ad-hoc work by setting `environments.dev.searxng_enabled: true` and regenerating config, then running `docker compose --profile searxng up -d searxng`. `deploy/compose.deploy.yml` intentionally does NOT include SearxNG — operators who want a self-hosted instance overlay it via the deploy adapter. The image tag is pinned via `assistant.open_knowledge.searxng.image` (SST); bump it deliberately and never use `:latest`. SearxNG settings (including required JSON output format) live in [`config/searxng/settings.yml`](../config/searxng/settings.yml) and are mounted read-only.

**Fabricated-cite fixture mode (pending).** The spec 064 SCOPE-17 adversarial regression for the cite-back verifier requires the ML sidecar `/llm/chat` route to support a `fixture-fabricated-cite` test mode that returns a planner response whose citations are deliberately absent from the per-turn `ToolResultStore`. That fixture knob is routed as finding #3 under [`specs/064-open-ended-knowledge-agent/route-packets/PKT-WORKFLOW-A.md`](../specs/064-open-ended-knowledge-agent/route-packets/PKT-WORKFLOW-A.md). When the fixture lands, `TestOpenKnowledgeE2E_A06_FabricatedSourceRejected` activates without modification and asserts `open_knowledge_refusal_total{cause="fabricated-source-blocked"}` increments exactly once.

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

## Spec 053 Delivery Evidence Requirements

When a spec transitions to done under full-delivery workflow mode, its report must include phase-owned evidence sections for validation, audit, and chaos.

- Validation evidence section: include executed command output and the phase marker for bubbles.validate.
- Audit evidence section: include executed command output and the phase marker for bubbles.audit.
- Chaos evidence section: include executed command output and the phase marker for bubbles.chaos.

## Tier-gated live-stack tests

Some live-stack e2e tests require accelerator hardware (GPU/NPU) to meet
their latency contracts and would deterministically fail on cpu-only
dev hosts even though the production retrieval path passes on accel
hardware. These tests are gated at the test-script level via the
`SMACKEREL_HARDWARE_TIER` environment variable (required; fail-loud per
SST policy — no default).

Behavior:

| `SMACKEREL_HARDWARE_TIER` | Test behavior | Exit |
|---------------------------|---------------|------|
| `cpu` | Emits `SKIP: <name> — ...` line and exits 0 | 0 |
| `accel` | Runs the test normally | test result |
| unset | Fails loud via `${VAR:?}` substitution | non-zero |
| any other value | Emits `[<name>] unknown SMACKEREL_HARDWARE_TIER=...` to stderr | 2 |

The `SKIP:` prefix is intentional and parseable: CI/dev/regression review
can grep for `^SKIP:` to distinguish deliberate tier-gated skips from
PASS/FAIL outcomes. A skip is **not** a pass — it is a recorded
deferral to accel-tier validation.

Currently tier-gated tests:

- BS-002 — [`tests/e2e/assistant_bs002_test.sh`](../tests/e2e/assistant_bs002_test.sh) (retrieval Q&A happy-path)
- BS-007 — [`tests/e2e/assistant_bs007_test.sh`](../tests/e2e/assistant_bs007_test.sh) (retrieval refusal)

Shared helper: `skip_unless_accel_tier <test-name>` in
[`tests/e2e/lib/helpers.sh`](../tests/e2e/lib/helpers.sh). New tier-gated
tests MUST reuse this helper rather than open-coding the gate.

Tier-gated tests REMAIN in the default e2e suite — do **not** exclude
them via runner flags. The gate runs at test-script level so the test
runner sees exit 0 on cpu hosts and the actual test outcome on accel
hosts.

Rationale and evidence:

- [`specs/061-conversational-assistant/design.md`](../specs/061-conversational-assistant/design.md) §5.1 — PACKET 3 operator-defer block
- [`specs/061-conversational-assistant/scopes.md`](../specs/061-conversational-assistant/scopes.md) SCOPE-06c DoD #5/#6
- [`specs/061-conversational-assistant/report.md`](../specs/061-conversational-assistant/report.md) Round 74 plan-triage `010a45d4`; Round 71e latency evidence

## Assistant Evaluation Harness

The spec 061 conversational assistant ships with an offline evaluation
harness that gives the §17 acceptance contract teeth in CI without
requiring a live LLM endpoint. The harness scores routing accuracy and
capture-fallback coverage against a fixed labelled corpus so silent
classifier or corpus drift becomes a loud test failure.

### Corpus

[`tests/eval/assistant/corpus.yaml`](../tests/eval/assistant/corpus.yaml)
holds 150 rows distributed 30 per intent label across the closed
vocabulary defined in
[`tests/eval/assistant/harness.go`](../tests/eval/assistant/harness.go):

| Label | Meaning |
|-------|---------|
| `retrieval` | Question stems that should route to the retrieval skill. |
| `weather` | Weather lexemes or weather-adjective + place / temporal context. |
| `notifications` | Imperative reminder phrasing with a concrete subject. |
| `capture` | Declarative messages with a capture marker (the always-on capture path). |
| `ambiguous-borderline` | Under-specified messages that MUST default to capture (design §3.2 "default-to-capture"). |

Structural invariants (≥150 rows, ≥30 per label, no duplicate IDs,
closed-vocabulary intents) are enforced by
[`tests/eval/assistant/corpus_validation_test.go`](../tests/eval/assistant/corpus_validation_test.go)
on every `go test` pass — a corpus mutation that drops below quota fails
fast.

### Acceptance Gate

SST-resolved thresholds (literals in
[`config/smackerel.yaml`](../config/smackerel.yaml) under
`assistant.eval:`):

| Key | Floor | Last shipped run |
|-----|-------|------------------|
| `routing_accuracy_min` | `0.85` | `1.0000` (150 / 150 rows) |
| `capture_fallback_min` | `1.0` | `1.0000` (60 / 60 capture-expected rows) |

The acceptance test
[`tests/eval/assistant/acceptance_test.go`](../tests/eval/assistant/acceptance_test.go)
(build tag `integration`, function
`TestAcceptanceGate_RoutingAccuracyAndCaptureFallback`) reads both
floors via `os.Getenv` from `ASSISTANT_EVAL_ROUTING_ACCURACY_MIN` and
`ASSISTANT_EVAL_CAPTURE_FALLBACK_MIN`. Both vars are REQUIRED:
`mustFloatEnv` calls `t.Fatalf` with the literal message `"SST contract
violation: <KEY> is empty"` if either is missing — fail-loud per
[`smackerel-no-defaults`](../.github/instructions/smackerel-no-defaults.instructions.md).

### How To Run

The env vars are emitted into `config/generated/<env>.env` by
[`scripts/commands/config.sh`](../scripts/commands/config.sh) from the
`assistant.eval:` block in `config/smackerel.yaml`. The standard
invocation is:

```bash
./smackerel.sh test integration --go-run TestAcceptanceGate_RoutingAccuracyAndCaptureFallback
```

Direct invocation (when the env file is already sourced into the shell)
is:

```bash
go test -count=1 -tags integration -run TestAcceptanceGate_RoutingAccuracyAndCaptureFallback ./tests/eval/assistant/...
```

The acceptance test logs the full per-row report on pass (useful as
SCOPE-10 evidence) and on failure includes the full report in the
failure message so an operator can see which rows drifted.

### How To Add A Scenario

1. Append a row to `tests/eval/assistant/corpus.yaml` with `id`, `text`,
   `ground_truth_intent` (one of the five labels above), and
   `ground_truth_capture_expected` (and `ground_truth_slots` when
   relevant).
2. Use a unique `id` — duplicates fail
   `TestCorpus_NoDuplicateIDs`.
3. Keep ≥30 rows per label — the per-label floor is enforced by
   `TestCorpus_PerLabelFloor`.
4. Run the harness:
   `go test -count=1 -tags integration -run TestAcceptanceGate_RoutingAccuracyAndCaptureFallback ./tests/eval/assistant/...`.
5. If the gate now fails, either tune the corpus row or adjust the
   classifier in `harness.go` to match the rules the production agent
   router is expected to follow.

### Determinism And Anti-Tautology Guards

The harness classifier is a deterministic keyword function (no RNG, no
external calls), so two consecutive runs against the same corpus
produce identical results — asserted by `TestRun_Deterministic`.

The harness also ships an explicit anti-tautology guard:
`TestRun_AdversarialFailureSurfaces` runs the harness with ground truth
deliberately mismatched against every classifier output and asserts
`IntentCorrect == 0`, `RoutingAccuracy == 0.0`, and a populated
`Failures` slice. This proves the harness CAN fail when classifier and
corpus disagree — a regression that silently passes every row would be
caught by this guard.

### Per-Behavioral-Scenario Regression Files

Spec 061 BS-001..BS-010 each get a dedicated regression slot under
[`tests/e2e/assistant_regression/`](../tests/e2e/assistant_regression/):

| Slot | File | State |
|------|------|-------|
| BS-001 | `bs_001_capture_fallback.sh` | Delegates to `tests/e2e/test_telegram_assistant_bs001.sh`. |
| BS-002 | `bs_002_retrieval_qa.sh` | `skip-77` pending substrate blocker (graph seeding). |
| BS-003 | `bs_003_weather_happy_path.sh` | Delegates to `tests/e2e/assistant_bs003_test.sh`. |
| BS-004 | `bs_004_notification_confirm.sh` | `skip-77` pending substrate blocker (notification proposal fixture). |
| BS-005 | `bs_005_ambiguous_disambig.sh` | `skip-77` pending substrate blocker (borderline seeding). |
| BS-006 | `bs_006_weather_outage.sh` | Delegates to `tests/e2e/assistant_bs006_test.sh`. |
| BS-007 | `bs_007_provenance_violation.sh` | `skip-77` pending substrate blocker (LLM no-source stub). |
| BS-008 | `bs_008_disabled_skill.sh` | `skip-77` pending substrate blocker (manifest hot-flip harness). |
| BS-009 | `bs_009_sst_missing_boot_failure.sh` | `skip-77` pending substrate blocker (boot-failure harness). |
| BS-010 | `bs_010_telegram_e2e.sh` | Delegates to `tests/e2e/test_telegram_assistant_bs001.sh` as the always-available Telegram adapter probe. |The skip-77 fixtures use the convention from
`tests/e2e/assistant_regression/lib/regression_helpers.sh`
(`reg_skip_with_blocker`) and each documents the full executed
assertion body in an inline `:<<'EXECUTED_PATTERN'` heredoc, so the
round that unblocks the named substrate flips a single line from
`reg_skip_with_blocker` to the in-tree assertion. Live-stack
verification across the full BS-001..BS-010 set remains pending and is
tracked by the open SCOPE-10 findings
`SCOPE-10-TELEGRAM-SMOKE-LIVE-STACK-VERIFICATION-PENDING` and
`SCOPE-10-PER-BS-REGRESSION-LIVE-STACK-VERIFICATION-PENDING` in
[`specs/061-conversational-assistant/state.json`](../specs/061-conversational-assistant/state.json).

### Tier-gated live-stack tests (BS-002 / BS-007)

Per spec 061 SCOPE-06c PACKET 3 (ratified in
[`design.md`](../specs/061-conversational-assistant/design.md) §5.1
under "Live-stack verification policy (operator-defer per SCOPE-06c
Round 74)"), the retrieval-Q&A live-stack fixtures
[`tests/e2e/assistant_bs002_test.sh`](../tests/e2e/assistant_bs002_test.sh)
and
[`tests/e2e/assistant_bs007_test.sh`](../tests/e2e/assistant_bs007_test.sh)
are **tier-gated** by the `SMACKEREL_HARDWARE_TIER` SST switch:

| Tier  | BS-002 / BS-007 fixture behavior | Notes |
|-------|----------------------------------|-------|
| `cpu`   | **SKIP** with exit code **77** and structured `RESULT: SKIPPED` / `SKIP_REASON=cpu-tier-operator-defer-per-SCOPE-06c-PACKET-3` record | Correct behavior. CPU-only inference cannot satisfy the 15 s interactive budget for a full retrieval-qa cited-JSON turn; production retrieval runs on accel hardware only. |
| `accel` | Fixture proceeds past the skip block and runs the full live-stack assertion (webhook POST → slog scrape → adversarial `jq` assertions) | Acceptance verification path. |
| unset / other | Fixture exits non-zero with `[F061-HARDWARE-TIER-MISSING]`-style fail-loud error (per [`smackerel-no-defaults`](../.github/instructions/smackerel-no-defaults.instructions.md)) | No implicit default. |

**Exit 77 SKIP convention.** Exit code 77 is the e2e-harness-wide
convention for "intentional skip with documented reason" (see also the
`skip-77` regression slots in the table above and
[`tests/e2e/assistant_regression/lib/regression_helpers.sh`](../tests/e2e/assistant_regression/lib/regression_helpers.sh)).
A 77 exit MUST be treated as PASS by any aggregating runner; the
structured `SKIP_REASON=…` record is the audit trail.

**Operator workflow.**

- *Local dev loop* — `.smackerel.local.env` (gitignored) sets
  `SMACKEREL_HARDWARE_TIER=cpu`; running
  `bash tests/e2e/assistant_bs002_test.sh` (and `…bs007…`) emits the
  structured SKIP record and exits 77. The remaining live-stack
  fixtures (BS-001 / BS-003 / BS-006 / BS-010) run as usual.
- *Acceptance verification on accel hardware* — operator exports
  `SMACKEREL_HARDWARE_TIER=accel` in the deploy-overlay overlay env
  (or runs on a tier-accel host where `.smackerel.local.env` is set to
  `accel`) and re-runs BS-002 / BS-007. Both then proceed past the
  skip block and execute the full live-stack assertion. Operator
  records PASS evidence under
  `specs/061-conversational-assistant/report.md#scope-06c-accel-live-stack-operator-pass`
  per SCOPE-06c DoD #6.

A SKIP on the cpu tier is **not** a failure of BS-002 / BS-007 coverage
— the capability-layer legs (SCOPE-06 DoD #4a / #5a) already verify
the underlying retrieval skill end-to-end on real PostgreSQL via Go
integration tests, and the adapter-composition leg is verified on
accel hardware per the matrix above.

This requirement is enforced by the artifact lint and state transition guards, and prevents done-level promotions that only contain planning artifacts without delivery-phase evidence provenance.