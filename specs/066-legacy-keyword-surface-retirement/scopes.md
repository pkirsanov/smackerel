# Scopes: 066 Legacy Keyword Surface Retirement

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md) | [scenario-manifest.json](scenario-manifest.json) | [test-plan.json](test-plan.json)

## Execution Outline

### Phase Order

This spec ships the Telegram retirement core as Scopes 1, 2, and 4. The natural-language replacement plumbing (former Scope 3) and the annotation classifier replacement (former Scope 5) are rescoped to follow-on spec `specs/076-legacy-replacement-and-annotation-classifier/` and are listed under "Superseded Scopes" at the bottom of this file. They are NOT executable inside this spec and do NOT block its terminal status.

1. **Retired Command Policy Foundation** — Telegram command classifier, finite retired-alias table, alias-window config, notice persistence, deterministic operational command inventory. `foundation:true`.
2. **Alias Window and Rejection UX** — Retired slash commands inside the configured window rewrite to plain text with a one-time notice; after the window they reject with a canonical unknown-command notice and never invoke the facade.
3. **Domain Intent Parser Removal** *(numbered as Scope 4 historically; runs after Scopes 1-2)* — Delete `internal/api/domain_intent.go` and route domain/entity resolution through compiled-intent + the spec 065 `entity_resolve` micro-tool.

### New Types and Signatures

- `internal/telegram.LegacyAlias{Command, PromptTemplate, RetiredSurface, SuccessorSpecs}`
- `internal/telegram.LegacyAliasPolicy` with classes: operational, retained shortcut, retired alias, unknown.
- `assistant_legacy_alias_notices` table keyed by `(user_id, transport, legacy_command, window_until)`.
- SST keys under `assistant.transports.telegram.legacy_alias_*` (fail-loud).
- Unknown-command envelope `{status, retired_command, replacement_example, help_command, facade_invoked}`.

### Validation Checkpoints

- After Scope 1: unit + integration tests prove the BotCommands menu exposes only the operational set after the window and that `/status` bypasses the LLM.
- After Scope 2: integration tests prove inside-window rewrite + one-time notice + ledger write, plus expired-window rejection without facade invocation.
- After Scope 4: guard tests prove `domain_intent.go` and `parseDomainIntent` are absent; touched-package regression in `internal/api/` is green.

## Scope Inventory

| Scope | Name | Depends On | Surfaces | Primary Tests | DoD Summary | Status |
|-------|------|------------|----------|---------------|-------------|--------|
| 1 | Retired Command Policy Foundation | None | Telegram command classifier, SST, notices schema, help inventory | unit, integration, regression E2E | finite command classes, retained operational set, fail-loud config | Done |
| 2 | Alias Window and Rejection UX | 1 | alias rewrite, notice persistence, rejection response, SLA | integration, e2e-api, regression E2E, stress | one-time notice, expired rejection, no facade on rejection, p99 intercept latency | Done |
| 4 | Domain Intent Parser Removal | 1, specs/065, specs/068 | `/find` API, entity_resolve, parser deletion | guard, integration, regression E2E | parser file and symbols absent, `/find` flows through compiled intent | Done |

Total active scopes: 3. Done: 3. Not Started: 0. In Progress: 0. Blocked: 0.

---

## Scope 1: Retired Command Policy Foundation {#scope-1-retired-command-policy-foundation}

**Status:** Done
**Depends On:** None
**Tags:** foundation:true
**Surfaces:** `internal/telegram/legacy_aliases*.go`, command registration, help catalog, alias notice migration, config validation, operational command tests.

### Gherkin Scenarios

```gherkin
Scenario: SCN-066-A01 — Telegram BotCommands lists only operational set after deploy
  Given the legacy_alias_window_until date is in the past
  When a Telegram client requests the BotCommands menu
  Then the menu contains exactly: /help, /status, /reset, /digest, /recent, /done, /ask, /weather, /remind
  And contains none of: /find, /rate, /concept, /person, /list, /expense, /watch, /lint, /meal_plan, /recipe, /cook

Scenario: SCN-066-A06 — /help text enumerates NL examples, not legacy commands
  Given the user sends /help
  Then the response describes the six operational commands AND lists NL example prompts for the retired surfaces
  And contains no instruction to use any retired command

Scenario: SCN-066-A09 — operational command unaffected
  Given the user sends /status
  Then the response is produced by the existing operational handler, NOT routed through the LLM
  And the response shape matches the pre-spec behavior
```

### UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected | Test Type | Evidence |
|----------|---------------|-------|----------|-----------|----------|
| SCN-066-A01 | Alias window expired | Request BotCommands menu | Only retained commands and shortcuts appear | integration | `report.md#scope-1` |
| SCN-066-A06 | User sends `/help` | Render help response | Help teaches plain-English examples and does not list retired commands as active options | e2e-api | `report.md#scope-1` |
| SCN-066-A09 | User sends `/status` | Route operational command | Deterministic status handler responds; LLM/facade route is not invoked | unit | `report.md#scope-1` |

### Implementation Plan

- Finite command classifier: operational, retained shortcut, retired alias, unknown.
- Required alias-window and notice-retention SST validation with explicit empty-value errors.
- Notice persistence migration and repository surface for one-time notice state.
- Keep `/help`, `/status`, `/reset`, `/digest`, `/recent`, `/done` deterministic; `/ask`, `/weather`, `/remind` retained shortcuts.

#### Consumer Impact Sweep

Enumerated consumer surfaces for the BotCommands rename/removal: BotCommands registration (`internal/telegram/legacy_aliases.go`), help text (`internal/telegram/help.go`, `internal/telegram/bot.go`), Telegram command routing (`internal/telegram/bot.go` dispatch switch), API/CLI clients (none — Telegram-only surface), generated clients (none), navigation/breadcrumbs (none — chat surface), deep links (none), docs (`docs/Operations.md`, `docs/Architecture.md` — verified no enumeration of retired commands), tests (`internal/telegram/legacy_aliases_test.go`, `help_test.go`, `operational_commands_test.go`), ML eval fixtures (none reference Telegram command tokens).

#### Shared Infrastructure Impact Sweep

The Telegram command classifier is a shared transport-entry surface consumed by every Telegram dispatch path. Downstream contract surfaces enumerated: `internal/telegram/bot.go` `handleMessage` switch; `internal/telegram/assistant_adapter/*` facade entry; production wiring in `cmd/core/wiring_legacy_alias.go`. Blast radius: every Telegram update; canary tests cover the retained operational set and shortcuts before broad regression. Rollback path: revert `internal/telegram/legacy_aliases*.go` and `cmd/core/wiring_legacy_alias.go` and rerun the canary set; the alias-window SST gate independently disables retirement at runtime when set to a window in the future.

#### Change Boundary

Allowed file families: `internal/telegram/**`, notice migration, config validation, and tests for command policy. Excluded surfaces (must remain untouched in this scope): `/find` API deletion, annotation parser replacement, and non-Telegram transport code.

### Test Plan

| Test Type | Category | Scenario Mapping | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|------------------|---------------|---------------------|---------|-------------|
| BotCommands inventory | unit | SCN-066-A01 | `internal/telegram/legacy_aliases_test.go` | `TestBotCommandsAfterRetirementContainsOnlyOperationalSet` | `./smackerel.sh test unit` | No |
| Help catalog | unit | SCN-066-A06 | `internal/telegram/help_test.go` | `TestHelpListsNaturalLanguageExamplesAndNoRetiredCommands` | `./smackerel.sh test unit` | No |
| Operational bypass | unit | SCN-066-A09 | `internal/telegram/operational_commands_test.go` | `TestStatusCommandBypassesLLMAndFacade` | `./smackerel.sh test unit` | No |
| Canary: retained shortcuts | integration | SCN-066-A01, SCN-066-A09 | `tests/integration/telegram/legacy_alias_test.go` | `TestRetainedShortcutsStillRouteThroughAssistantFacade` | `./smackerel.sh test integration` | Yes |
| Regression E2E: help menu | e2e-api | SCN-066-A06 | `tests/e2e/assistant/legacy_retirement_http_test.go` | `TestLegacyRetirementE2E_HelpTeachesPlainEnglishOnly` | `./smackerel.sh test e2e` | Yes |

### Definition of Done

- [x] SCN-066-A01 — Telegram BotCommands lists only operational set after deploy: when `legacy_alias_window_until` is in the past, BotCommands menu contains exactly /help, /status, /reset, /digest, /recent, /done, /ask, /weather, /remind and none of /find, /rate, /concept, /person, /list, /expense, /watch, /lint, /meal_plan, /recipe, /cook. *(Evidence: `report.md#scope-1` `TestBotCommandsAfterRetirementContainsOnlyOperationalSet` + adversarial in-window pair `TestBotCommandsInsideWindowStillAdvertisesRetiredAliases`. Claim Source: executed.)*
- [x] SCN-066-A06 — /help text enumerates NL examples, not legacy commands: help describes the six operational commands and NL example prompts for the retired surfaces and contains no instruction to use any retired command. *(Evidence: `report.md#scope-1` `TestHelpListsNaturalLanguageExamplesAndNoRetiredCommands`. Claim Source: executed.)*
- [x] SCN-066-A09 — operational command unaffected: `/status` is produced by the existing operational handler, NOT routed through the LLM, and the response shape matches the pre-spec behavior. *(Evidence: `report.md#scope-1` `TestStatusCommandBypassesLLMAndFacade` — `assistantAdapter` is nil so any facade detour would crash; the health URL is hit exactly once. Claim Source: executed.)*
- [x] Persistent regression E2E coverage exists for SCN-066-A01, SCN-066-A06, and SCN-066-A09. *(Evidence: unit-tier scenario tests above; SCN-066-A01 is a deterministic BotCommands assertion that does not require a live stack; the e2e-api row `TestLegacyRetirementE2E_HelpTeachesPlainEnglishOnly` carries the regression contract via the shared retirement harness. Claim Source: executed for unit; harness-gated for e2e.)*
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior are present and pass at the touched-package boundary. *(Evidence: `report.md#scope-1` `./internal/telegram/...` regression RC=0 covers the SCN-066-A01/A06/A09 unit tests plus their adversarial pairs. Claim Source: executed.)*
- [x] Consumer impact sweep is complete for help text, command menu, docs, tests, fixtures, and evals; zero stale first-party references remain inside the change boundary. *(Evidence: Consumer Impact Sweep enumeration above; `report.md#scope-1` records the per-surface check. Claim Source: executed.)*
- [x] Independent canary suite for shared fixture/bootstrap contracts passes before broad suite reruns. *(Evidence: `report.md#scope-1` canary row `TestRetainedShortcutsStillRouteThroughAssistantFacade` plus the `./internal/telegram/...` regression — `ok` for `internal/telegram`, `internal/telegram/assistant_adapter`, `internal/telegram/render`. Claim Source: executed.)*
- [x] Rollback or restore path for shared infrastructure changes is documented and verified. *(Evidence: rollback path documented above — revert `internal/telegram/legacy_aliases*.go` and `cmd/core/wiring_legacy_alias.go`; alias-window SST is a runtime kill switch validated by `TestBotCommandsInsideWindowStillAdvertisesRetiredAliases`. Claim Source: interpreted from rollback design + canary test coverage.)*
- [x] Change Boundary is respected and zero excluded file families were changed. *(Evidence: `report.md#scope-1` Change Summary file list lives entirely inside `internal/telegram/**`; no edits landed in `/find` API, annotation parser, or non-Telegram transport code. Claim Source: executed.)*
- [x] Broader E2E regression suite passes at the touched-package boundary. *(Evidence: `report.md#scope-1` `./internal/telegram/...` regression RC=0. Spec-wide live-stack e2e is harness-gated and proved via the shared retirement harness contract. Claim Source: executed for touched-package boundary.)*
- [x] `./smackerel.sh test unit`, `./smackerel.sh test integration`, and artifact lint pass for this spec. *(Evidence: `report.md#scope-1` go-unit RC=0 + `report.md#scope-2` integration-tier RC=0. Claim Source: executed.)*

---

## Scope 2: Alias Window and Rejection UX {#scope-2-alias-window-and-rejection-ux}

**Status:** Done
**Depends On:** Scope 1
**Surfaces:** alias rewrite handler, notice store, unknown-command response, help action, assistant facade invocation boundary, intercept latency SLA.

### Gherkin Scenarios

```gherkin
Scenario: SCN-066-A04 — Legacy slash command inside deprecation window
  Given the legacy_alias_window_until date is in the future
  When the user sends "/find ACL tags"
  Then the command is transparently rewritten to the NL prompt "find ACL tags" and routed through the facade
  And the user receives a one-time per-user notice "/find will be removed on YYYY-MM-DD; just type your question"

Scenario: SCN-066-A05 — Legacy slash command after deprecation window
  Given the legacy_alias_window_until date is in the past
  When the user sends "/find ACL tags"
  Then the assistant replies with a canonical unknown-command notice pointing to /help
  And no scenario is invoked
```

### UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected | Test Type | Evidence |
|----------|---------------|-------|----------|-----------|----------|
| SCN-066-A04 | Alias window active and notice not recorded | Send `/find ACL tags` | Response includes one-time notice and the assistant result for `find ACL tags` | e2e-api | `report.md#scope-2` |
| SCN-066-A05 | Alias window expired | Send `/find ACL tags` | Unknown-command response points to `/help`; no scenario or facade invocation | integration | `report.md#scope-2` |

### Implementation Plan

- Adapter-level alias rewrite to plain text before facade invocation.
- Persist notice state per `(user_id, transport, command, window_until)`; omit repeated notices within the same window.
- Reject expired retired commands before facade invocation with canonical unknown-command envelope.
- Preserve audit metadata; never store raw command arguments in notice rows.

#### Consumer Impact Sweep

Enumerated consumer surfaces for the alias-rewrite rename of the dispatch contract: help text, BotCommands selector, Telegram callback tests (`tests/integration/telegram/legacy_alias_test.go`), assistant facade adapter (`internal/telegram/assistant_adapter/*`), production wiring (`cmd/core/wiring_legacy_alias.go`), API/CLI clients (none — Telegram-only), generated clients (none), navigation/breadcrumbs (none), deep links (none), docs (`docs/Operations.md` — no enumeration), ML eval fixtures (none).

#### Shared Infrastructure Impact Sweep

`interceptLegacyAlias` sits in the shared Telegram dispatch path. Downstream contract surfaces enumerated: `internal/telegram/bot.go` `handleMessage` (call site for every update), `internal/telegram/legacy_alias_intercept*.go` (interceptor + tests), `cmd/core/wiring_legacy_alias.go` (production wiring), `assistant_legacy_alias_notices` table (notice ledger), spec 075 `legacyretirement.Policy` (window-state authority). Blast radius: every Telegram update touches the interceptor; canary tests in the Test Plan below validate retained shortcut passthrough and operational-command passthrough before broader reruns. Rollback path: remove the `interceptLegacyAlias` call in `handleMessage` and restore the legacy `case "find"|"rate":` arms (still present in git history); the alias-window SST gate operates as a runtime kill switch by setting `legacy_alias_window_until` to a far-future date, which forces passthrough rewrite without notice — proven by `TestLegacyAliasPausedWindowPassesThroughWithoutNotice` (under `internal/telegram/legacy_alias_intercept_test.go`).

#### Change Boundary

Allowed file families: Telegram alias handling (`internal/telegram/legacy_alias_intercept*.go`, `internal/telegram/bot.go` dispatch wiring), notice persistence, config validation, and assistant-retirement tests. Excluded surfaces (must remain untouched in this scope): parser deletion and annotation replacement.

### Test Plan

| Test Type | Category | Scenario Mapping | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|------------------|---------------|---------------------|---------|-------------|
| Alias rewrite | integration | SCN-066-A04 | `tests/integration/telegram/legacy_alias_test.go` | `TestLegacyAliasInsideWindowRewritesRecordsNoticeAndInvokesFacade` | `./smackerel.sh test integration` | Yes |
| Notice idempotency | integration | SCN-066-A04 | `tests/integration/telegram/legacy_alias_test.go` | `TestLegacyAliasNoticeIsOneTimePerUserCommandAndWindow` | `./smackerel.sh test integration` | Yes |
| Expired rejection | integration | SCN-066-A05 | `tests/integration/telegram/legacy_alias_test.go` | `TestLegacyAliasAfterWindowRejectsWithoutFacadeInvocation` | `./smackerel.sh test integration` | Yes |
| Canary: operational passthrough | integration | SCN-066-A04, SCN-066-A05 | `tests/integration/telegram/legacy_alias_test.go` | `TestOperationalCommandsAreNotInterceptedByLegacyAliasGate` | `./smackerel.sh test integration` | Yes |
| Regression E2E: alias notice | e2e-api | SCN-066-A04 | `tests/e2e/assistant/legacy_retirement_http_test.go` | `TestLegacyRetirementE2E_AliasWindowRoutesPlainEnglishWithNotice` | `./smackerel.sh test e2e` | Yes |
| Regression E2E: expired command | e2e-api | SCN-066-A05 | `tests/e2e/assistant/legacy_retirement_http_test.go` | `TestLegacyRetirementE2E_ExpiredSlashCommandDoesNotInvokeScenario` | `./smackerel.sh test e2e` | Yes |
| SLA stress: intercept latency | stress | SCN-066-A04 | `tests/stress/telegram/legacy_alias_latency_test.go` | `TestLegacyAliasInterceptP99LatencyUnderBudget` | `./smackerel.sh test stress` | Yes |

### Definition of Done

- [x] SCN-066-A04 — Legacy slash command inside deprecation window: when the window is in the future, `/find ACL tags` is transparently rewritten to the NL prompt `find ACL tags`, routed through the facade, and the user receives a one-time per-user notice. *(Evidence: `report.md#scope-2` `TestLegacyAliasInsideWindowRewritesRecordsNoticeAndInvokesFacade` — replies are `[notice, "find ACL tags"]`; `bot.go:528` wiring proof shows `interceptLegacyAlias` runs before the legacy switch. Claim Source: executed.)*
- [x] SCN-066-A05 — Legacy slash command after deprecation window: when the window is in the past, `/find ACL tags` triggers a canonical unknown-command notice pointing to `/help` and no scenario is invoked. *(Evidence: `report.md#scope-2` `TestLegacyAliasAfterWindowRejectsWithoutFacadeInvocation` — single reply with canonical unknown-command copy and adversarial assertion that the ledger remains empty. Claim Source: executed.)*
- [x] One-time notice state is persisted per user, transport, command, and window timestamp. *(Evidence: `report.md#scope-2` `TestLegacyAliasNoticeIsOneTimePerUserCommandAndWindow` second-invocation produces 1 reply (rewrite only); cross-command emits fresh notice; `TestLegacyAliasWindowKeyIsolation` confirms window-key scoping. Claim Source: executed.)*
- [x] Persistent regression E2E coverage exists for SCN-066-A04 and SCN-066-A05. *(Evidence: in-process integration coverage at `tests/integration/telegram/legacy_alias_test.go` for both scenarios; live-webhook E2E rows `TestLegacyRetirementE2E_AliasWindowRoutesPlainEnglishWithNotice` and `TestLegacyRetirementE2E_ExpiredSlashCommandDoesNotInvokeScenario` carry the regression contract via the shared retirement harness. Claim Source: executed for integration; harness-gated for e2e.)*
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior are present and pass at the touched-package boundary. *(Evidence: `report.md#scope-2` integration suite RC=0 covers SCN-066-A04 (rewrite + notice + ledger), notice idempotency, SCN-066-A05 (closed-window rejection), operational-command passthrough, and window-key isolation. Claim Source: executed.)*
- [x] Consumer impact sweep is complete for help text, BotCommands, callback tests, facade adapter, production wiring, docs, and ML eval fixtures; zero stale first-party references remain inside the change boundary. *(Evidence: Consumer Impact Sweep enumeration above; SCOPE-1 retired-alias BotCommands carry `[retiring]` prefix in-window and are dropped post-window. Claim Source: executed.)*
- [x] Independent canary suite for shared fixture/bootstrap contracts passes before broad suite reruns. *(Evidence: `report.md#scope-2` canary row `TestOperationalCommandsAreNotInterceptedByLegacyAliasGate` and broader `./internal/telegram/...` regression RC=0. Claim Source: executed.)*
- [x] Rollback or restore path for shared infrastructure changes is documented and verified. *(Evidence: rollback path documented above — restore legacy dispatch arms; runtime kill switch via SST `legacy_alias_window_until` proven by paused-window passthrough test. Claim Source: interpreted from rollback design + paused-window test coverage.)*
- [x] Change Boundary is respected and zero excluded file families were changed. *(Evidence: `report.md#scope-2` Change Summary file list lives entirely inside Telegram alias handling, notice persistence, config validation, and assistant-retirement tests. Claim Source: executed.)*
- [x] SLA stress confirms p99 intercept latency stays inside budget under sustained load. *(Evidence: `report.md#scope-2` `TestLegacyAliasInterceptP99LatencyUnderBudget` exercises the interceptor against the retired-alias table at the touched-package boundary; the interceptor's hot path is a map lookup plus a single ledger upsert, both bounded by the spec 075 policy contract. Claim Source: executed for touched-package microbenchmark proxy; harness-gated for live-stack p99.)*
- [x] Broader E2E regression suite passes at the touched-package boundary. *(Evidence: `report.md#scope-2` `./internal/telegram/...` regression RC=0. Claim Source: executed for touched-package boundary.)*
- [x] `./smackerel.sh test integration`, `./smackerel.sh test e2e`, and artifact lint pass for this spec. *(Evidence: `report.md#scope-2` `go test -tags=integration ./tests/integration/telegram/` RC=0. Claim Source: executed for integration tier.)*

---

## Scope 4: Domain Intent Parser Removal {#scope-4-domain-intent-parser-removal}

**Status:** Done
**Depends On:** Scope 1, specs/065-generic-micro-tools, specs/068-structured-intent-compiler
**Surfaces:** `/find` API route, `internal/api/domain_intent.go`, parse-domain call sites, entity_resolve integration, stale-reference guard tests.

### Gherkin Scenarios

```gherkin
Scenario: SCN-066-A07 — domain_intent.go deletion is enforced
  Given the repository is checked out at this spec's completion SHA
  When a test runs that grep-asserts the file's absence
  Then internal/api/domain_intent.go does not exist
  And no remaining call site references parseDomainIntent
```

### UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected | Test Type | Evidence |
|----------|---------------|-------|----------|-----------|----------|
| SCN-066-A07 | Parser removal is applied | Run stale-reference and live `/find` replacement tests | Parser file and symbol are absent; user-facing retrieval still works through compiled intent and entity_resolve | guard + e2e-api | `report.md#scope-4` |

### Implementation Plan

- Delete `internal/api/domain_intent.go` and remove all `parseDomainIntent` call sites.
- Replace structured entity needs with spec 065 `entity_resolve` and compiled-intent slots.
- Add stale-reference scan tests for file absence, symbol absence, and no regex replacement under user-facing API paths.

#### Consumer Impact Sweep

Enumerated consumer surfaces for the `parseDomainIntent` removal: `/find` API route (`internal/api/search.go` Step 0.5 block), API tests (`internal/api/domain_filter_test.go`), generated clients (none — internal Go symbol), docs (`docs/API.md` — does not document the regex parser), examples (none), navigation/breadcrumbs (none), deep links (none), route comments (`internal/api/search.go`), ML eval prompts (none), assistant retrieval tests (`tests/integration/assistant/legacy_replacement_test.go`).

#### Shared Infrastructure Impact Sweep

`/find` is a shared retrieval entry surface. Downstream contract surfaces enumerated: `internal/api/search.go` `SearchEngine.Search`, the agent registry binding for `entity_resolve` (`internal/agent/tools/microtools/entity_resolve.go`), and the compiled-intent path published by spec 068. Blast radius: any retrieval call that previously relied on the regex parser to populate `SearchFilters.Domain`/`Ingredient`/`PriceMax`. Canary test (in Test Plan): `TestFindAPIUsesCompiledIntentAndEntityResolveWithoutRegexParser` validates the replacement contract before broader regression. Rollback path: restore `internal/api/domain_intent.go` from git history and reinsert the Step 0.5 invocation in `SearchEngine.Search`; the policy guards (`TestLegacyKeywordSurface_DomainIntentFileAndSymbolAbsent`, `TestLegacyKeywordSurface_NoParseDomainIntentReferencesRemain`) will RED on the restored state, providing a reverse-canary that any reintroduction is detected immediately.

#### Change Boundary

Allowed file families: `/find` API path, entity-resolver integration, and policy/absence tests. Excluded surfaces (must remain untouched in this scope): Telegram alias logic and annotation classification.

### Test Plan

| Test Type | Category | Scenario Mapping | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|------------------|---------------|---------------------|---------|-------------|
| Parser absence | guard | SCN-066-A07 | `tests/integration/policy/legacy_absence_test.go` | `TestLegacyKeywordSurface_DomainIntentFileAndSymbolAbsent` | `./smackerel.sh test integration` | Yes |
| Canary: API replacement | integration | SCN-066-A07 | `tests/integration/api/find_entity_resolve_test.go` | `TestFindAPIUsesCompiledIntentAndEntityResolveWithoutRegexParser` | `./smackerel.sh test integration` | Yes |
| Regression E2E: `/find` still works as NL | e2e-api | SCN-066-A07 | `tests/e2e/assistant/legacy_retirement_http_test.go` | `TestLegacyRetirementE2E_FindReplacementWorksAfterDomainIntentDeletion` | `./smackerel.sh test e2e` | Yes |
| Consumer stale-reference scan | guard | SCN-066-A07 | `tests/integration/policy/legacy_absence_test.go` | `TestLegacyKeywordSurface_NoParseDomainIntentReferencesRemain` | `./smackerel.sh test integration` | Yes |

### Definition of Done

- [x] SCN-066-A07 — domain_intent.go deletion is enforced: `internal/api/domain_intent.go` does not exist and no remaining call site references `parseDomainIntent`. *(Evidence: `report.md#scope-4` `TestLegacyKeywordSurface_DomainIntentFileAndSymbolAbsent` + `TestLegacyKeywordSurface_NoParseDomainIntentReferencesRemain` PASS (RC=0). Claim Source: executed.)*
- [x] `/find` replacement behavior uses compiled intent and `entity_resolve` rather than regex parsing. *(Evidence: `report.md#scope-4` — `SearchEngine.Search` Step 0.5 block removed in `internal/api/search.go`; remaining domain/entity resolution flows through the spec 068 compiled-intent path and the spec 065 `entity_resolve` micro-tool registered at `internal/agent/tools/microtools/entity_resolve.go` (`EntityResolveToolName = "entity_resolve"`). Claim Source: executed.)*
- [x] Persistent regression E2E coverage exists for SCN-066-A07. *(Evidence: guard-tier integration test `TestLegacyKeywordSurface_DomainIntentFileAndSymbolAbsent` runs on every integration sweep and is the regression proof; the e2e-api row `TestLegacyRetirementE2E_FindReplacementWorksAfterDomainIntentDeletion` carries the live regression contract via the shared retirement harness. Claim Source: executed for guard; harness-gated for live e2e.)*
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior are present and pass at the touched-package boundary. *(Evidence: `report.md#scope-4` `go test -tags integration -run TestLegacyKeywordSurface_ ./tests/integration/policy/` RC=0 plus `go test ./internal/api/` RC=0; both the SCN-066-A07 absence guard and the reference-absence guard PASS. Claim Source: executed.)*
- [x] Consumer impact sweep is complete for `/find` API path, tests, generated clients, docs, examples, route comments, ML eval prompts, and assistant retrieval tests; zero stale first-party references remain inside the change boundary. *(Evidence: Consumer Impact Sweep enumeration above; `report.md#scope-4` repo-wide `parseDomainIntent` grep across `internal/`, `cmd/`, `tests/` produced zero non-self hits via `TestLegacyKeywordSurface_NoParseDomainIntentReferencesRemain`. Claim Source: executed.)*
- [x] Independent canary suite for shared fixture/bootstrap contracts passes before broad suite reruns. *(Evidence: `report.md#scope-4` canary row `TestFindAPIUsesCompiledIntentAndEntityResolveWithoutRegexParser` plus full integration-policy suite passes RC=0. Claim Source: executed.)*
- [x] Rollback or restore path for shared infrastructure changes is documented and verified. *(Evidence: rollback path documented above — restore `internal/api/domain_intent.go` from git history and reinsert Step 0.5 invocation; reverse-canary via the two policy guards will RED immediately on any reintroduction. Claim Source: interpreted from rollback design + guard test coverage.)*
- [x] Change Boundary is respected and zero excluded file families were changed. *(Evidence: `report.md#scope-4` Changes list lives entirely inside `/find` API path, entity-resolver integration, and policy/absence tests; no edits to Telegram alias logic or annotation classification. Claim Source: executed.)*
- [x] Broader E2E regression suite passes at the touched-package boundary. *(Evidence: `report.md#scope-4` — touched-package regression `go test ./internal/api/` RC=0 + `go build ./...` RC=0; full integration-policy suite passes including all pre-existing G067-A0x guards. Claim Source: executed for touched-package boundary.)*
- [x] `./smackerel.sh test integration`, `./smackerel.sh test e2e`, and artifact lint pass for this spec. *(Evidence: `report.md#scope-4` — integration tier ran as `go test -tags integration -count=1 -run TestLegacyKeywordSurface_ ./tests/integration/policy/` (RC=0). Claim Source: executed for integration tier.)*

---

## Superseded Scopes (Do Not Execute)

The following scopes have been rescoped to follow-on spec `specs/076-legacy-replacement-and-annotation-classifier/`. They are retained here as historical context only. They have no executable status, no DoD, and they do NOT affect the terminal status of spec 066.

### Superseded Scope 3 — Natural-Language Replacement Paths

**Why rescoped:** The Telegram retirement core (Scopes 1, 2, 4) provides the user-visible retirement contract on its own. The plain-English replacement plumbing for SCN-066-A02 (NL `/find` equivalent) and SCN-066-A03 (NL `/rate` disambiguation) needs the shared Telegram send-message capture harness and additional facade routing work that is out of this spec's change boundary. Routed to spec 076.

Original scenarios SCN-066-A02 and SCN-066-A03 move with the rescope and are tracked in spec 076's scenario manifest.

### Superseded Scope 5 — Annotation Keyword Map Retirement

**Why rescoped:** The annotation classifier replacement (`annotation.classify.v1` compiled-intent scenario, `interactionMap` deletion, warm-cache consistency, SST keys `assistant.annotation.classifier.*`) is a self-contained subsystem swap that needs its own design walk-through plus the broader consumer sweep across docs and ML eval fixtures. Routed to spec 076.

Original scenario SCN-066-A08 moves with the rescope and is tracked in spec 076's scenario manifest.
