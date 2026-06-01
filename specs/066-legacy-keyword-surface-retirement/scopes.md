# Scopes: 066 Legacy Keyword Surface Retirement

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md) | [scenario-manifest.json](scenario-manifest.json) | [test-plan.json](test-plan.json)

## Execution Outline

### Phase Order

1. **Retired Command Policy Foundation** — Define the Telegram command classifier, finite retired-alias table, alias-window config, notice persistence, and deterministic operational command inventory. `foundation:true`.
2. **Alias Window and Rejection UX** — Route retired slash commands inside the configured window as plain text with one-time notice, then reject them after the window without facade invocation.
3. **Natural-Language Replacement Paths** — Prove `/find` and `/rate` equivalents flow through spec 068 compiled intents, facade routing, and disambiguation instead of command handlers.
4. **Domain Intent Parser Removal** — Delete `internal/api/domain_intent.go` and replace `/find` structured extraction with the spec 065 `entity_resolve` path.
5. **Annotation Keyword Map Retirement and Consumer Sweep** — Replace annotation free-text keyword classification with compiled-intent slots and close stale references across docs, tests, fixtures, and help text.

### New Types and Signatures

- `internal/telegram.LegacyAlias` with `Command`, `PromptTemplate`, `RetiredSurface`, and `SuccessorSpecs` fields.
- `internal/telegram.LegacyAliasPolicy` with finite command classification: operational, retained shortcut, retired alias, unknown.
- `assistant_legacy_alias_notices` table keyed by `(user_id, transport, legacy_command, window_until)`.
- Required SST keys under `assistant.transports.telegram.legacy_alias_*`.
- Unknown-command envelope with `status`, `retired_command`, `replacement_example`, `help_command`, and `facade_invoked` fields.

### Validation Checkpoints

- After Scope 1, unit and integration tests prove the command menu exposes only operational commands plus retained shortcuts and that `/status` bypasses the LLM.
- After Scope 2, integration and E2E tests prove inside-window alias rewrite, one-time notice state, expired-command rejection, and no facade invocation on rejection.
- After Scope 3, live HTTP E2E tests prove plain-English `/find` and `/rate` replacements reach the same user outcomes through compiled intents.
- After Scope 4, guard tests prove `domain_intent.go` and `parseDomainIntent` are absent.
- After Scope 5, annotation E2E and stale-reference scans prove no active first-party surface teaches or wires retired command grammar.

## Scope Inventory

| Scope | Name | Depends On | Surfaces | Primary Tests | DoD Summary | Status |
|-------|------|------------|----------|---------------|-------------|--------|
| 1 | Retired Command Policy Foundation | None | Telegram command classifier, SST, notices schema, help inventory | unit, integration, Regression E2E | finite command classes, retained operational set, fail-loud config | Not Started |
| 2 | Alias Window and Rejection UX | 1 | alias rewrite, notice persistence, rejection response | integration, e2e-api, Regression E2E | one-time notice, expired rejection, no facade on rejection | Not Started |
| 3 | Natural-Language Replacement Paths | 1, 2 | facade route, compiler, retrieval/rating actions | e2e-api, integration, Regression E2E | NL replaces command outcomes and disambiguates safely | Not Started |
| 4 | Domain Intent Parser Removal | 3, specs/065, specs/068 | `/find` API, entity_resolve, parser deletion | guard, integration, Regression E2E | old parser file and symbols absent | Not Started |
| 5 | Annotation Keyword Map Retirement and Consumer Sweep | 4 | annotation classifier, docs, fixtures, help text, ML evals | unit, e2e-api, stale-reference scan | compiled slots replace keyword maps; stale references removed | Not Started |

---

## Scope 1: Retired Command Policy Foundation

**Status:** Not Started  
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
| SCN-066-A09 | User sends `/status` | Route operational command | Deterministic status handler responds and LLM/facade route is not invoked | unit | `report.md#scope-1` |

### Implementation Plan

- Create a finite command classifier for operational commands, retained shortcuts, retired aliases, and unknown commands.
- Add required alias-window config validation and notice-retention validation with explicit missing-key errors.
- Add notice persistence migration and repository surface for one-time notice state.
- Keep `/help`, `/status`, `/reset`, `/digest`, `/recent`, and `/done` deterministic; `/ask`, `/weather`, and `/remind` remain retained shortcuts.
- **Consumer Impact Sweep:** enumerate BotCommands registration, help text, Telegram command routing, docs, tests, ML eval fixtures, and examples that reference the old command menu.
- **Shared Infrastructure Impact Sweep:** Telegram command classification is a shared transport entry surface. Canary tests validate existing operational handlers and retained shortcuts before broad suite reruns.
- **Change Boundary:** allowed file families are `internal/telegram/**`, notice migration, config validation, and tests for command policy. Excluded surfaces are `/find` API deletion, annotation parser replacement, and non-Telegram transport code.

### Test Plan

| Test Type | Category | Scenario Mapping | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|------------------|---------------|---------------------|---------|-------------|
| BotCommands inventory | unit | SCN-066-A01 | `internal/telegram/legacy_aliases_test.go` | `TestBotCommandsAfterRetirementContainsOnlyOperationalSet` | `./smackerel.sh test unit` | No |
| Help catalog | unit | SCN-066-A06 | `internal/telegram/help_test.go` | `TestHelpListsNaturalLanguageExamplesAndNoRetiredCommands` | `./smackerel.sh test unit` | No |
| Operational bypass | unit | SCN-066-A09 | `internal/telegram/operational_commands_test.go` | `TestStatusCommandBypassesLLMAndFacade` | `./smackerel.sh test unit` | No |
| Canary: retained shortcuts | integration | SCN-066-A01, SCN-066-A09 | `tests/integration/telegram/legacy_alias_test.go` | `TestRetainedShortcutsStillRouteThroughAssistantFacade` | `./smackerel.sh test integration` | Yes |
| Regression E2E: help menu | e2e-api | SCN-066-A06 | `tests/e2e/assistant/legacy_retirement_http_test.go` | `TestLegacyRetirementE2E_HelpTeachesPlainEnglishOnly` | `./smackerel.sh test e2e` | Yes |

### Definition of Done

- [ ] BotCommands inventory contains only the retained operational set and shortcuts after the alias window.
- [ ] `/help` teaches plain-English examples and contains no active retired-command instructions.
- [ ] `/status` and other operational controls remain deterministic and do not invoke the LLM.
- [ ] Consumer Impact Sweep inventory is complete for help text, command menu, docs, tests, fixtures, and evals.
- [ ] Shared Infrastructure Impact Sweep canary tests pass before broad suite reruns.
- [ ] Scenario-specific E2E regression coverage exists for SCN-066-A01, SCN-066-A06, and SCN-066-A09.
- [ ] Broader E2E regression suite passes.
- [ ] `./smackerel.sh test unit`, `./smackerel.sh test integration`, `./smackerel.sh test e2e`, and artifact lint pass for this spec.

---

## Scope 2: Alias Window and Rejection UX

**Status:** Not Started  
**Depends On:** Scope 1  
**Surfaces:** alias rewrite handler, notice store, unknown-command response, help action, assistant facade invocation boundary.

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
| SCN-066-A05 | Alias window expired | Send `/find ACL tags` | Unknown-command response points to `/help`, no scenario or facade invocation | integration | `report.md#scope-2` |

### Implementation Plan

- Implement adapter-level alias rewrite to plain text before facade invocation.
- Persist notice state per `(user_id, transport, command, window_until)` and omit repeated notices within the same window.
- Reject expired retired commands before facade invocation with canonical unknown-command envelope.
- Ensure alias rewrite preserves audit metadata without exposing raw command arguments in notice storage.
- **Consumer Impact Sweep:** stale command references in help text, BotCommands, Telegram callback tests, docs, eval fixtures, and HTTP contract fixtures are enumerated and removed or converted to plain-English examples.
- **Change Boundary:** allowed file families are Telegram alias handling, notice persistence, config validation, and assistant-retirement tests. Excluded surfaces are parser deletion and annotation replacement.

### Test Plan

| Test Type | Category | Scenario Mapping | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|------------------|---------------|---------------------|---------|-------------|
| Alias rewrite | integration | SCN-066-A04 | `tests/integration/telegram/legacy_alias_test.go` | `TestLegacyAliasInsideWindowRewritesRecordsNoticeAndInvokesFacade` | `./smackerel.sh test integration` | Yes |
| Notice idempotency | integration | SCN-066-A04 | `tests/integration/telegram/legacy_alias_test.go` | `TestLegacyAliasNoticeIsOneTimePerUserCommandAndWindow` | `./smackerel.sh test integration` | Yes |
| Expired rejection | integration | SCN-066-A05 | `tests/integration/telegram/legacy_alias_test.go` | `TestLegacyAliasAfterWindowRejectsWithoutFacadeInvocation` | `./smackerel.sh test integration` | Yes |
| Regression E2E: alias notice | e2e-api | SCN-066-A04 | `tests/e2e/assistant/legacy_retirement_http_test.go` | `TestLegacyRetirementE2E_AliasWindowRoutesPlainEnglishWithNotice` | `./smackerel.sh test e2e` | Yes |
| Regression E2E: expired command | e2e-api | SCN-066-A05 | `tests/e2e/assistant/legacy_retirement_http_test.go` | `TestLegacyRetirementE2E_ExpiredSlashCommandDoesNotInvokeScenario` | `./smackerel.sh test e2e` | Yes |

### Definition of Done

- [ ] Retired commands inside the configured window rewrite to canonical plain-English prompts and route through the facade.
- [ ] One-time notice state is persisted per user, transport, command, and window timestamp.
- [ ] Expired retired commands reject before scenario or facade invocation.
- [ ] Consumer Impact Sweep proves retired commands are not advertised as active actions.
- [ ] Scenario-specific E2E regression coverage exists for SCN-066-A04 and SCN-066-A05.
- [ ] Broader E2E regression suite passes.
- [ ] `./smackerel.sh test integration`, `./smackerel.sh test e2e`, and artifact lint pass for this spec.

---

## Scope 3: Natural-Language Replacement Paths

**Status:** Not Started  
**Depends On:** Scope 1, Scope 2, specs/068-structured-intent-compiler, specs/069-assistant-http-transport  
**Surfaces:** assistant facade route, compiled-intent slots, retrieval scenario, rating/disambiguation flow, HTTP E2E fixtures.

### Gherkin Scenarios

```gherkin
Scenario: SCN-066-A02 — NL replaces /find
  Given the user sends "find my notes about ACL tags"
  When the assistant facade routes the message
  Then the message is matched to retrieval_qa via the intent router (similarity path)
  And the response cites at least one artifact, identical to the previous /find behavior

Scenario: SCN-066-A03 — NL replaces /rate via disambiguation
  Given the user sends "rate that 8 out of 10" with no recent rateable artifact in context
  When the assistant facade routes the message
  Then the user receives a spec 061 disambiguation prompt offering candidate artifacts
  And selecting an artifact persists the rating exactly as /rate previously did
```

### UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected | Test Type | Evidence |
|----------|---------------|-------|----------|-----------|----------|
| SCN-066-A02 | User has relevant saved artifacts | Send plain-English find turn over HTTP | Retrieval answer cites at least one artifact and no slash-command path is used | e2e-api | `report.md#scope-3` |
| SCN-066-A03 | No recent rateable artifact in context | Send rating text and choose a disambiguation candidate | User sees concrete candidate choices, then rating persists after selection | e2e-api | `report.md#scope-3` |

### Implementation Plan

- Route plain-English replacement requests through spec 068 compiled intents and existing facade behavior.
- Ensure retrieval and rating flows use structured context and disambiguation rather than retired command handlers.
- Seed E2E fixtures with owned artifacts so assertions prove persisted response behavior instead of relying on borrowed shared state.
- **Consumer Impact Sweep:** API clients, docs, tests, Telegram examples, ML eval prompts, and scenario fixtures that mention `/find` or `/rate` are converted to plain-English behavior checks.
- **Change Boundary:** allowed file families are assistant facade routing, retrieval/rating tests, and retirement E2E fixtures. Excluded surfaces are command classifier internals already completed in Scopes 1-2 and parser deletion in Scope 4.

### Test Plan

| Test Type | Category | Scenario Mapping | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|------------------|---------------|---------------------|---------|-------------|
| Retrieval replacement | e2e-api | SCN-066-A02 | `tests/e2e/assistant/legacy_retirement_http_test.go` | `TestLegacyRetirementE2E_NaturalLanguageFindCitesArtifacts` | `./smackerel.sh test e2e` | Yes |
| Rating disambiguation | e2e-api | SCN-066-A03 | `tests/e2e/assistant/legacy_retirement_http_test.go` | `TestLegacyRetirementE2E_NaturalLanguageRateDisambiguatesAndPersists` | `./smackerel.sh test e2e` | Yes |
| Retrieval contract | integration | SCN-066-A02 | `tests/integration/assistant/legacy_replacement_test.go` | `TestNaturalLanguageFindUsesRetrievalScenarioNotSlashHandler` | `./smackerel.sh test integration` | Yes |
| Regression E2E: command parity | e2e-api | SCN-066-A02, SCN-066-A03 | `tests/e2e/assistant/legacy_retirement_http_test.go` | `TestLegacyRetirementE2E_RetiredCommandEquivalentsReachSameEndState` | `./smackerel.sh test e2e` | Yes |

### Definition of Done

- [ ] Plain-English find and rating requests route through compiled intents and facade behavior.
- [ ] Rating without context produces a spec 061 disambiguation prompt and persists only after the user chooses.
- [ ] E2E fixtures use agent-owned artifacts and prove persisted outcomes through live API behavior.
- [ ] Consumer Impact Sweep proves `/find` and `/rate` examples are no longer taught as primary actions.
- [ ] Scenario-specific E2E regression coverage exists for SCN-066-A02 and SCN-066-A03.
- [ ] Broader E2E regression suite passes.
- [ ] `./smackerel.sh test integration`, `./smackerel.sh test e2e`, and artifact lint pass for this spec.

---

## Scope 4: Domain Intent Parser Removal

**Status:** Not Started  
**Depends On:** Scope 3, specs/065-generic-micro-tools, specs/068-structured-intent-compiler  
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
- Add stale-reference scan tests for file absence, symbol absence, and no keyword regex replacement under user-facing API paths.
- **Consumer Impact Sweep:** first-party consumers include `/find` API tests, generated clients if present, docs, examples, fixtures, route comments, ML evals, and assistant retrieval tests.
- **Change Boundary:** allowed file families are `/find` API path, entity resolver integration, and policy/absence tests. Excluded surfaces are Telegram alias logic and annotation classification.

### Test Plan

| Test Type | Category | Scenario Mapping | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|------------------|---------------|---------------------|---------|-------------|
| Parser absence | guard | SCN-066-A07 | `tests/integration/policy/legacy_absence_test.go` | `TestLegacyKeywordSurface_DomainIntentFileAndSymbolAbsent` | `./smackerel.sh test integration` | Yes |
| API replacement | integration | SCN-066-A07 | `tests/integration/api/find_entity_resolve_test.go` | `TestFindAPIUsesCompiledIntentAndEntityResolveWithoutRegexParser` | `./smackerel.sh test integration` | Yes |
| Regression E2E: `/find` still works as NL | e2e-api | SCN-066-A07, SCN-066-A02 | `tests/e2e/assistant/legacy_retirement_http_test.go` | `TestLegacyRetirementE2E_FindReplacementWorksAfterDomainIntentDeletion` | `./smackerel.sh test e2e` | Yes |
| Consumer stale-reference scan | guard | SCN-066-A07 | `tests/integration/policy/legacy_absence_test.go` | `TestLegacyKeywordSurface_NoParseDomainIntentReferencesRemain` | `./smackerel.sh test integration` | Yes |

### Definition of Done

- [ ] `internal/api/domain_intent.go` is absent and no first-party code references `parseDomainIntent`.
- [ ] `/find` replacement behavior uses compiled intent and `entity_resolve` rather than regex parsing.
- [ ] Consumer Impact Sweep proves no stale parser references remain in docs, tests, clients, examples, or fixtures.
- [ ] Scenario-specific E2E regression coverage exists for SCN-066-A07.
- [ ] Broader E2E regression suite passes.
- [ ] `./smackerel.sh test integration`, `./smackerel.sh test e2e`, and artifact lint pass for this spec.

---

## Scope 5: Annotation Keyword Map Retirement and Consumer Sweep

**Status:** Not Started  
**Depends On:** Scope 4, specs/068-structured-intent-compiler  
**Surfaces:** `internal/annotation/parser.go`, annotation pipeline, compiled-intent slots, docs/tests/evals that teach command or keyword grammar.

### Gherkin Scenarios

```gherkin
Scenario: SCN-066-A08 — annotation classification uses LLM extraction
  Given the user sends "cooked it last night, was great"
  When the annotation pipeline classifies the interaction
  Then the classification is produced by an LLM tool returning { kind, confidence } above the configured floor
  And the keyword-map code path in internal/annotation/parser.go no longer exists in the runtime classification path
```

### UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected | Test Type | Evidence |
|----------|---------------|-------|----------|-----------|----------|
| SCN-066-A08 | Annotation turn includes natural language interaction | Send `cooked it last night, was great` through assistant HTTP | Compiled intent supplies kind/confidence; borderline cases use disambiguation instead of keyword lookup | e2e-api | `report.md#scope-5` |

### Implementation Plan

- Replace runtime annotation keyword-map classification with spec 068 compiled-intent slots and confidence floor enforcement.
- Preserve domain outcome semantics for `made_it`, rating, note, and target references.
- Add adversarial tests that would fail if a keyword map again selected interaction type from user free text.
- Close the spec-wide Consumer Impact Sweep: navigation/help text, docs, Telegram examples, API clients, generated clients, deep links, config, tests, prompt fixtures, and ML evals.
- **Change Boundary:** allowed file families are annotation classification code, annotation tests, assistant E2E fixtures, and first-party docs/examples that mention retired command grammar. Excluded surfaces are micro-tool implementation and HTTP transport internals.

### Test Plan

| Test Type | Category | Scenario Mapping | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|------------------|---------------|---------------------|---------|-------------|
| Annotation compiled slots | e2e-api | SCN-066-A08 | `tests/e2e/assistant/annotation_intent_test.go` | `TestAnnotationIntentE2E_CookedItClassifiesFromCompiledIntent` | `./smackerel.sh test e2e` | Yes |
| Keyword-map absence | guard | SCN-066-A08 | `tests/integration/policy/legacy_absence_test.go` | `TestLegacyKeywordSurface_NoUserFacingAnnotationKeywordMap` | `./smackerel.sh test integration` | Yes |
| Classification confidence | unit | SCN-066-A08 | `internal/annotation/parser_test.go` | `TestAnnotationClassificationRequiresCompiledIntentConfidence` | `./smackerel.sh test unit` | No |
| Regression E2E: borderline annotation | e2e-api | SCN-066-A08 | `tests/e2e/assistant/annotation_intent_test.go` | `TestAnnotationIntentE2E_BorderlineClassificationDisambiguates` | `./smackerel.sh test e2e` | Yes |
| Consumer stale-reference scan | guard | SCN-066-A01..A09 | `tests/integration/policy/legacy_absence_test.go` | `TestLegacyKeywordSurface_NoRetiredCommandInstructionsRemain` | `./smackerel.sh test integration` | Yes |

### Definition of Done

- [ ] Annotation classification consumes compiled-intent slots and confidence; runtime user free-text keyword maps no longer choose interaction type.
- [ ] Borderline annotation cases route to disambiguation rather than keyword fallback or silent guess.
- [ ] Consumer Impact Sweep proves retired command grammar and annotation keyword-map references are absent from active first-party surfaces.
- [ ] Scenario-specific E2E regression coverage exists for SCN-066-A08.
- [ ] Broader E2E regression suite passes.
- [ ] `./smackerel.sh test unit`, `./smackerel.sh test integration`, `./smackerel.sh test e2e`, and artifact lint pass for this spec.
