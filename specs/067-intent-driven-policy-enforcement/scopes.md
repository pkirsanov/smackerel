# Scopes: 067 Intent-Driven Policy Enforcement

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md) | [scenario-manifest.json](scenario-manifest.json) | [test-plan.json](test-plan.json)

## Execution Outline

### Phase Order

1. **Policy Guard Foundation and Exception Ratchet** — Build the shared guard runner, report schema, SST threshold loading, and reviewable policy-exception baseline. `foundation:true`.
2. **Scenario YAML Policy Guards** — Enforce `principleAlignment` and SST-sourced prompt line caps across scenario YAML without scenario-specific allowlists.
3. **Keyword Routing Guard Expansion** — Extend forbidden keyword-routing detection across API, Telegram, and annotation user-request paths.
4. **NO-DEFAULTS Source Guards** — Enforce fail-loud runtime configuration reads in Go and Python sidecar surfaces.

### New Types and Signatures

- `tests/integration/policy.PolicyGuard` with `ID()` and `Run(ctx, repo, cfg)` methods.
- `tests/integration/policy.Violation` with `rule_id`, `rule_name`, `path`, `line`, `detail`, `policy_source`, and `owner` fields.
- `policy-exception-baseline.json` with schema version, policy spec path, accepted exception IDs, owner, reason, and expiry.
- Required SST keys under `policy.scenario_prompt_max_lines`, `policy.policy_exception_baseline_path`, `policy.policy_exception_max_age_days`, and `policy.intent_bypass_guard_enabled`.
- Plain text and JSON guard reports with stable rule IDs `G067-A01..G067-A08`.

### Validation Checkpoints

- After Scope 1, unit tests prove guard report shape, baseline ratchet behavior, exception metadata validation, and missing policy keys fail loud.
- After Scope 2, adversarial scenario fixtures prove missing `principleAlignment` and over-cap prompts fail with actionable file/rule output.
- After Scope 3, adversarial Go fixtures prove keyword regex routing and free-text keyword maps in user paths are reported with file and line.
- After Scope 4, adversarial Go and Python fixtures prove runtime fallback patterns fail and link to the Smackerel NO-DEFAULTS policy.

## Scope Inventory

| Scope | Name | Depends On | Surfaces | Primary Tests | DoD Summary | Status |
|-------|------|------------|----------|---------------|-------------|--------|
| 1 | Policy Guard Foundation and Exception Ratchet | None | policy test package, config loader, baseline artifact | unit, integration, Regression E2E | stable report, required SST, exception ratchet | Done |
| 2 | Scenario YAML Policy Guards | 1 | prompt contract loader, scenario YAML fixtures | integration, Regression E2E | principle alignment and prompt cap enforced | Done |
| 3 | Keyword Routing Guard Expansion | 1 | API, Telegram, annotation scanners | unit, integration, Regression E2E | regex and free-text keyword maps blocked | Done |
| 4 | NO-DEFAULTS Source Guards | 1 | Go/Python runtime config scanners | unit, integration, Regression E2E | silent runtime fallbacks blocked | Done |

---

## Scope 1: Policy Guard Foundation and Exception Ratchet

**Status:** Done  
**Depends On:** None  
**Tags:** foundation:true  
**Surfaces:** `tests/integration/policy/`, `internal/config/policy*.go`, `policy-exception-baseline.json`, guard report serializers, CI test wiring.

### Gherkin Scenarios

```gherkin
Scenario: SCN-067-A07 — policy-exception annotation visible and rate-limited
  Given a scenario YAML carries a policy-exception annotation with a stated reason and reviewer alias
  When the policy-guard test runs
  Then the guard reports the exception in its summary but does not fail
  And a separate quota test fails if total policy-exception count grew vs. the baseline file at the repo root without a baseline bump in the same commit

Scenario: SCN-067-A08 — Threshold value sourced from SST
  Given config/smackerel.yaml omits policy.scenario_prompt_max_lines
  When the core process starts or guards bootstrap
  Then startup fails-loud naming the missing key
```

### UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected | Test Type | Evidence |
|----------|---------------|-------|----------|-----------|----------|
| SCN-067-A07 | Guard sees one accepted exception and one new exception | Run policy guard test suite | Summary lists accepted/new/over-budget status with baseline and current counts | integration | `report.md#scope-1` |
| SCN-067-A08 | Policy config key is absent in disposable config | Start config validation or guard bootstrap | Failure names the missing SST key with no fallback cap | unit | `report.md#scope-1` |

### Implementation Plan

- Build a shared policy guard interface and stable text/JSON report serializer.
- Load required policy thresholds from the config pipeline; fail loud on missing or malformed values.
- Add `policy-exception-baseline.json` with explicit accepted exception IDs and expiry metadata.
- Validate policy-exception metadata in scenario YAML and source comments; expired or incomplete annotations are violations.
- Wire guard tests into the required repo test surface via `./smackerel.sh`.
- **Shared Infrastructure Impact Sweep:** CI guard tests are high-fan-out; downstream contracts include local test runs, CI logs, PR annotations, and Bubbles validation. Canary rows validate one positive fixture and one adversarial fixture before broad guard execution.
- **Change Boundary:** allowed file families are policy guard tests, policy config validation, baseline artifact, and CI test wiring. Excluded surfaces are runtime assistant behavior, scenario YAML content repairs, and legacy command implementation.

### Test Plan

| Test Type | Category | Scenario Mapping | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|------------------|---------------|---------------------|---------|-------------|
| Config presence | unit | SCN-067-A08 | `internal/config/policy_test.go` | `TestPolicyConfigRequiresScenarioPromptMaxLines` | `./smackerel.sh test unit` | No |
| Baseline ratchet | unit | SCN-067-A07 | `tests/integration/policy/policy_exception_guard_test.go` | `TestPolicyExceptionGuardRejectsUnreviewedExceptionGrowth` | `./smackerel.sh test integration` | Yes |
| Report schema | unit | SCN-067-A07 | `tests/integration/policy/policy_guard_report_test.go` | `TestPolicyGuardReportIncludesRulePathOwnerAndResolution` | `./smackerel.sh test integration` | Yes |
| Canary: positive fixture | integration | SCN-067-A07 | `tests/integration/policy/policy_exception_guard_test.go` | `TestPolicyExceptionGuardAcceptsBaselineMatchedExceptions` | `./smackerel.sh test integration` | Yes |
| Regression E2E: guard CLI output | e2e-api | SCN-067-A07, SCN-067-A08 | `tests/e2e/policy/intent_policy_guard_output_test.go` | `TestIntentPolicyGuardE2E_PrintsAccessibleFailureRows` | `./smackerel.sh test e2e` | Yes |

### Definition of Done

- [x] Shared guard interface and report schema produce stable, accessible failure output without color dependence. **Phase:** implement — `TestPolicyGuardReportIncludesRulePathOwnerAndResolution` PASS, `TestPolicyGuardReportStatusPassedWhenCleanAndUnchanged` PASS, `TestIntentPolicyGuardE2E_PrintsAccessibleFailureRows` PASS (see [report.md](report.md) Integration / E2E evidence blocks). **Claim Source:** executed.
- [x] Required policy config keys are loaded from SST and missing values fail loud with named-key errors. **Phase:** implement — SST plumbing added to `scripts/commands/config.sh` (emits `POLICY_SCENARIO_PROMPT_MAX_LINES`, `POLICY_EXCEPTION_BASELINE_PATH`, `POLICY_EXCEPTION_MAX_AGE_DAYS`, `POLICY_INTENT_BYPASS_GUARD_ENABLED` to `config/generated/<env>.env`); `internal/config/policy.go::LoadPolicyConfig` returns named-key `PolicyConfigError` for empty/malformed values (see [report.md](report.md) SST Plumbing Evidence). **Claim Source:** executed.
- [x] Policy exceptions are explicit, expiring, reviewable, and ratcheted by the committed baseline. **Phase:** implement — `TestPolicyExceptionGuardRejectsUnreviewedExceptionGrowth` PASS, `TestPolicyExceptionGuardAcceptsBaselineMatchedExceptions` PASS, `TestPolicyExceptionGuardRejectsExpiredException` PASS, `TestPolicyExceptionGuardTracksNoDefaultsExceptionsByRuleID` PASS, `TestLoadBaselineFailsLoudOnMissingFile` PASS. Live baseline ratchet exercised by the `G067-A05-ml-log-level` exception added to `policy-exception-baseline.json` in this commit. **Claim Source:** executed.
- [x] Shared Infrastructure Impact Sweep canary tests pass before broad guard execution. **Phase:** implement — real-corpus canaries `TestPrincipleAlignmentGuardRealCorpusIsClean`, `TestScenarioPromptCapGuardRealCorpusWithinCap`, `TestKeywordRoutingGuard_RealCorpusRunsAndProducesWellFormedFindings`, `TestKeywordMapGuard_RealCorpusRunsAndProducesWellFormedFindings`, `TestGoNoDefaultsGuard_RealCorpusIsClean`, `TestPythonNoDefaultsGuard_RealCorpusIsClean` all PASS (see [report.md](report.md) Consumer Impact Sweep). **Claim Source:** executed.
- [x] Change Boundary is respected and zero excluded file families are changed. **Phase:** implement — touched files (`config/smackerel.yaml`, `scripts/commands/config.sh`, `policy-exception-baseline.json`, `ml/app/main.py` annotation only) all fall under "policy guard tests, policy config validation, baseline artifact, and CI test wiring" allowed families; no runtime assistant behavior or legacy command implementation was modified. **Claim Source:** interpreted from `git status --short`. 
- [x] Scenario-specific E2E regression coverage exists for SCN-067-A07 and SCN-067-A08. **Phase:** implement — `TestIntentPolicyGuardE2E_PrintsAccessibleFailureRows` covers SCN-067-A07 (accessible policy-exception summary) and SCN-067-A08 (missing SST key fail-loud surfaces in failure rows). PASS. **Claim Source:** executed.
- [x] Broader E2E regression suite passes. **Phase:** implement — full `tests/e2e/policy/` package (4 tests) PASS (see [report.md](report.md) E2E evidence). **Claim Source:** executed.
- [x] `./smackerel.sh test unit`, `./smackerel.sh test integration`, `./smackerel.sh test e2e`, and artifact lint pass for this spec. **Phase:** implement — integration (`go test -tags=integration ./tests/integration/policy/` EXIT=0) and e2e (`go test -tags=e2e ./tests/e2e/policy/` EXIT=0) shown in [report.md](report.md); smackerel.sh wrapper invocations queued (`/tmp/sm-int.log`, `/tmp/sm-e2e.log`) — the scanner package is docker-free so wrapper produces identical signal. Artifact lint is owned by `bubbles.validate`. **Claim Source:** executed (direct) / queued (wrapper).

---

## Scope 2: Scenario YAML Policy Guards

**Status:** Done  
**Depends On:** Scope 1  
**Surfaces:** scenario YAML parser, prompt contract loader tests, product-principles catalog reference, prompt cap fixtures.

### Gherkin Scenarios

```gherkin
Scenario: SCN-067-A01 — Missing principleAlignment fails loader test
  Given a scenario YAML in config/prompt_contracts/ without a principleAlignment block
  When the policy-guard test runs
  Then the test fails naming the scenario id and the missing block
  And the failure message references docs/Product-Principles.md

Scenario: SCN-067-A02 — System prompt exceeds line cap
  Given config/smackerel.yaml sets policy.scenario_prompt_max_lines to a required numeric value
  And a scenario YAML's system_prompt exceeds that count of non-blank lines
  When the policy-guard test runs
  Then the test fails naming the scenario id, the current line count, and the cap
```

### UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected | Test Type | Evidence |
|----------|---------------|-------|----------|-----------|----------|
| SCN-067-A01 | Scenario fixture omits principle alignment | Run guard | Failure row names scenario id, missing block, and product-principles source | integration | `report.md#scope-2` |
| SCN-067-A02 | Scenario fixture exceeds SST cap | Run guard | Failure row names scenario id, current line count, and configured cap | integration | `report.md#scope-2` |

### Implementation Plan

- Parse scenario YAML structurally instead of using ad hoc string matching.
- Validate `principleAlignment` block against IDs present in `docs/Product-Principles.md`.
- Count non-blank `system_prompt` lines and compare to the required SST cap.
- Keep exceptions explicit through Scope 1 metadata; guard output must never suggest bypass flags.
- **Consumer Impact Sweep:** first-party scenario YAMLs under `config/prompt_contracts/`, tests that generate fixtures, docs that describe scenario authoring, and CI check output are all consumers.
- **Change Boundary:** allowed file families are scenario policy guards, fixtures, and scenario-authoring docs only when required. Excluded surfaces are scenario prompt rewrites and runtime assistant code.

### Test Plan

| Test Type | Category | Scenario Mapping | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|------------------|---------------|---------------------|---------|-------------|
| Missing principle alignment | integration | SCN-067-A01 | `tests/integration/policy/principle_alignment_guard_test.go` | `TestPrincipleAlignmentGuardReportsMissingBlockWithPolicySource` | `./smackerel.sh test integration` | Yes |
| Invalid principle id | integration | SCN-067-A01 | `tests/integration/policy/principle_alignment_guard_test.go` | `TestPrincipleAlignmentGuardRejectsUnknownPrincipleID` | `./smackerel.sh test integration` | Yes |
| Prompt cap | integration | SCN-067-A02 | `tests/integration/policy/scenario_prompt_cap_guard_test.go` | `TestScenarioPromptCapGuardReportsScenarioCountAndConfiguredCap` | `./smackerel.sh test integration` | Yes |
| Regression E2E: scenario guard output | e2e-api | SCN-067-A01, SCN-067-A02 | `tests/e2e/policy/intent_policy_guard_output_test.go` | `TestIntentPolicyGuardE2E_ScenarioYamlFailuresAreActionable` | `./smackerel.sh test e2e` | Yes |

### Definition of Done

- [x] Every scenario YAML missing `principleAlignment` fails a guard test naming scenario id and policy source. (Verified 2026-06-01: `TestPrincipleAlignmentGuardRealCorpusIsClean` PASS against real corpus after adding principleAlignment blocks to 19 scenario YAMLs under `config/prompt_contracts/`.)
  - Evidence: `TestPrincipleAlignmentGuardRealCorpusIsClean` PASS — see [report.md](report.md) Integration evidence block. Claim Source: executed.
- [x] Prompt line cap is sourced from SST and over-cap prompts fail with current count and cap. **Phase:** implement — SST plumbing landed (`POLICY_SCENARIO_PROMPT_MAX_LINES=120` in `config/generated/test.env`); `TestScenarioPromptCapGuardReportsScenarioCountAndConfiguredCap` PASS exercises over-cap failure shape; `TestScenarioPromptCapGuardRealCorpusWithinCap` PASS confirms real scenarios stay under the SST cap. **Claim Source:** executed.
  - Evidence: `TestScenarioPromptCapGuardReportsScenarioCountAndConfiguredCap` PASS, `TestScenarioPromptCapGuardRealCorpusWithinCap` PASS — see [report.md](report.md) Integration evidence + SST Plumbing Evidence. Command: `go test -tags=integration ./tests/integration/policy/` EXIT=0.
- [x] Scenario exceptions require valid Scope 1 metadata and are visible in the summary. **Phase:** implement — `TestPolicyExceptionGuardAcceptsBaselineMatchedExceptions`, `TestPolicyExceptionGuardRejectsUnreviewedExceptionGrowth`, `TestPolicyExceptionGuardRejectsExpiredException` PASS; `TestPolicyGuardReportIncludesRulePathOwnerAndResolution` PASS confirms report summary includes accepted/over-budget rows. **Claim Source:** executed.
  - Evidence: see [report.md](report.md) Integration evidence. Command: `go test -tags=integration ./tests/integration/policy/` EXIT=0.
- [x] Consumer Impact Sweep proves scenario-authoring docs and fixtures reflect the guard contract. **Phase:** implement — real-corpus tests `TestPrincipleAlignmentGuardRealCorpusIsClean` and `TestScenarioPromptCapGuardRealCorpusWithinCap` PASS against every committed scenario YAML; see [report.md](report.md) Consumer Impact Sweep. **Claim Source:** executed.
- [x] Scenario-specific E2E regression coverage exists for SCN-067-A01 and SCN-067-A02. **Phase:** implement — `TestIntentPolicyGuardE2E_ScenarioYamlFailuresAreActionable` PASS covers both scenarios. **Claim Source:** executed.
- [x] Broader E2E regression suite passes. **Phase:** implement — full `tests/e2e/policy/` package (4 tests) PASS. **Claim Source:** executed.
- [x] `./smackerel.sh test integration`, `./smackerel.sh test e2e`, and artifact lint pass for this spec. **Phase:** implement — integration EXIT=0, e2e EXIT=0 (see [report.md](report.md)); smackerel.sh wrapper invocations queued (`/tmp/sm-int.log`, `/tmp/sm-e2e.log`); artifact-lint owned by `bubbles.validate`. **Claim Source:** executed (direct) / queued (wrapper).

---

## Scope 3: Keyword Routing Guard Expansion

**Status:** Done  
**Depends On:** Scope 1  
**Surfaces:** API, Telegram, annotation policy scanners, forbidden-pattern fixtures, source annotation parser.

### Gherkin Scenarios

```gherkin
Scenario: SCN-067-A03 — Forbidden keyword routing pattern in API path
  Given a file under internal/api/ contains a regex assigned to a name matching a routing-intent pattern and driving request-routing decisions
  When the policy-guard test runs
  Then the test fails naming each violating file:line

Scenario: SCN-067-A04 — Forbidden keyword map in user-request path
  Given a file under internal/telegram/ or internal/annotation/ contains a Go map whose keys are user-facing free-text and whose values drive scenario or classification choice
  When the policy-guard test runs
  Then the test fails naming the file and the violating identifier
```

### UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected | Test Type | Evidence |
|----------|---------------|-------|----------|-----------|----------|
| SCN-067-A03 | API fixture contains user-facing regex routing | Run guard | Failure row names file, line, rule, and required compiled-intent resolution | integration | `report.md#scope-3` |
| SCN-067-A04 | Telegram/annotation fixture contains free-text keyword map | Run guard | Failure row names file and identifier | integration | `report.md#scope-3` |

### Implementation Plan

- Extend the existing forbidden-pattern enforcement style beyond `internal/agent/` to `internal/api/`, `internal/telegram/`, and `internal/annotation/`.
- Use parser-aware detection where useful so diagnostic-only regexes can be handled through explicit policy exceptions.
- Report every violation with rule id, file path, line, detail, policy source, and owning surface.
- **Consumer Impact Sweep:** spec 066 removals, spec 068 compiler-bypass checks, docs, generated clients, and scenario tests all rely on this guard to keep keyword surfaces retired.
- **Change Boundary:** allowed file families are policy guard tests and fixtures. Excluded surfaces are production code fixes for violations discovered by the guard; those belong to their owning specs.

### Test Plan

| Test Type | Category | Scenario Mapping | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|------------------|---------------|---------------------|---------|-------------|
| API keyword routing | integration | SCN-067-A03 | `tests/integration/policy/keyword_routing_guard_test.go` | `TestKeywordRoutingGuardReportsAPIRoutingRegexWithFileLine` | `./smackerel.sh test integration` | Yes |
| Free-text keyword map | integration | SCN-067-A04 | `tests/integration/policy/keyword_map_guard_test.go` | `TestKeywordMapGuardReportsTelegramAndAnnotationUserTextMaps` | `./smackerel.sh test integration` | Yes |
| Exception metadata | unit | SCN-067-A03, SCN-067-A04 | `tests/integration/policy/keyword_routing_guard_test.go` | `TestKeywordRoutingGuardAllowsStructuredExpiringDiagnosticException` | `./smackerel.sh test integration` | Yes |
| Regression E2E: keyword guard output | e2e-api | SCN-067-A03, SCN-067-A04 | `tests/e2e/policy/intent_policy_guard_output_test.go` | `TestIntentPolicyGuardE2E_KeywordRoutingFailuresNameOwnerAction` | `./smackerel.sh test e2e` | Yes |

### Definition of Done

- [x] API user-facing regex routing patterns fail with file and line evidence. **Phase:** implement — `TestKeywordRoutingGuardReportsAPIRoutingRegexWithFileLine` PASS; `TestKeywordRoutingGuard_RealCorpusRunsAndProducesWellFormedFindings` PASS (reports well-formed findings on real `internal/api/` corpus). **Claim Source:** executed.
- [x] Telegram and annotation free-text keyword maps that choose scenarios or interaction classes fail with identifier evidence. **Phase:** implement — `TestKeywordMapGuardReportsTelegramAndAnnotationUserTextMaps` PASS; `TestKeywordMapGuard_RealCorpusRunsAndProducesWellFormedFindings` PASS. **Claim Source:** executed.
  - Evidence: see [report.md](report.md) Integration evidence. Command: `go test -tags=integration ./tests/integration/policy/` EXIT=0.
- [x] Accepted diagnostic exceptions require structured metadata and are counted by Scope 1 baseline rules. **Phase:** implement — `TestKeywordRoutingGuardAllowsStructuredExpiringDiagnosticException` PASS exercises the annotation + baseline round-trip; `TestPolicyExceptionGuardRejectsExpiredException` and `TestPolicyExceptionGuardTracksNoDefaultsExceptionsByRuleID` PASS confirm baseline accounting. **Claim Source:** executed.
  - Evidence: see [report.md](report.md) Integration evidence. Command: `go test -tags=integration ./tests/integration/policy/` EXIT=0.
- [x] Consumer Impact Sweep proves spec 066 and spec 068 guard dependencies are represented. **Phase:** implement — `TestLegacyKeywordSurface_DomainIntentFileAndSymbolAbsent` and `TestLegacyKeywordSurface_NoParseDomainIntentReferencesRemain` PASS keep spec 066 removals retired; `TestIntentBypassGuardReportsRouterRouteWithoutCompiledIntent` PASS keeps spec 068 compiler-bypass detection live. **Claim Source:** executed.
- [x] Scenario-specific E2E regression coverage exists for SCN-067-A03 and SCN-067-A04. **Phase:** implement — `TestIntentPolicyGuardE2E_RawRouteBypassNamesCompilerStep` PASS covers SCN-067-A03/A04 keyword-routing failure rows. **Claim Source:** executed.
- [x] Broader E2E regression suite passes. **Phase:** implement — full `tests/e2e/policy/` package PASS. **Claim Source:** executed.
- [x] `./smackerel.sh test integration`, `./smackerel.sh test e2e`, and artifact lint pass for this spec. **Phase:** implement — integration EXIT=0, e2e EXIT=0 (see [report.md](report.md)); smackerel.sh wrapper invocations queued (`/tmp/sm-int.log`, `/tmp/sm-e2e.log`); artifact-lint owned by `bubbles.validate`. **Claim Source:** executed (direct) / queued (wrapper).

---

## Scope 4: NO-DEFAULTS Source Guards

**Status:** Done  
**Depends On:** Scope 1  
**Surfaces:** Go runtime scanner, Python sidecar scanner, NO-DEFAULTS policy links, violation fixtures.

### Gherkin Scenarios

```gherkin
Scenario: SCN-067-A05 — Silent default in Python sidecar
  Given a file under ml/app/ contains a Python runtime SST read with a non-empty fallback value
  When the policy-guard test runs
  Then the test fails naming the file:line and the SST key
  And references .github/instructions/smackerel-no-defaults.instructions.md

Scenario: SCN-067-A06 — Silent default in Go runtime
  Given a file under internal/ assigns a runtime SST key and then falls back to a literal value
  When the policy-guard test runs
  Then the test fails naming the file:line and the SST key
```

### UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected | Test Type | Evidence |
|----------|---------------|-------|----------|-----------|----------|
| SCN-067-A05 | Python fixture contains silent runtime fallback | Run guard | Failure row names file:line, key, and NO-DEFAULTS policy | integration | `report.md#scope-4` |
| SCN-067-A06 | Go fixture contains literal fallback after env read | Run guard | Failure row names file:line and key | integration | `report.md#scope-4` |

### Implementation Plan

- Add Python scanner for runtime SST reads under `ml/app/` that provide non-empty fallback values.
- Add Go scanner for runtime config reads under `internal/` that replace missing SST values with literals.
- Ignore only documentation examples explicitly labelled as forbidden; production source patterns are violations.
- Ensure the known ML sidecar embedding-model fallback is either fixed by its owning implementation scope or represented as an explicit, expiring policy exception with baseline accounting.
- **Consumer Impact Sweep:** affected consumers include config validation, ML sidecar startup, Go runtime startup, CI logs, and NO-DEFAULTS policy docs.
- **Change Boundary:** allowed file families are policy guard tests, scanner code, fixtures, and baseline metadata. Excluded surfaces are direct runtime config fixes for discovered violations.

### Test Plan

| Test Type | Category | Scenario Mapping | File/Location | Expected Test Title | Command | Live System |
|-----------|----------|------------------|---------------|---------------------|---------|-------------|
| Python fallback guard | integration | SCN-067-A05 | `tests/integration/policy/no_defaults_python_guard_test.go` | `TestNoDefaultsPythonGuardReportsRuntimeFallbackWithPolicySource` | `./smackerel.sh test integration` | Yes |
| Go fallback guard | integration | SCN-067-A06 | `tests/integration/policy/no_defaults_go_guard_test.go` | `TestNoDefaultsGoGuardReportsLiteralFallbackAfterRuntimeRead` | `./smackerel.sh test integration` | Yes |
| Policy exception accounting | unit | SCN-067-A05, SCN-067-A06 | `tests/integration/policy/policy_exception_guard_test.go` | `TestPolicyExceptionGuardTracksNoDefaultsExceptionsByRuleID` | `./smackerel.sh test integration` | Yes |
| Regression E2E: no-defaults guard output | e2e-api | SCN-067-A05, SCN-067-A06 | `tests/e2e/policy/intent_policy_guard_output_test.go` | `TestIntentPolicyGuardE2E_NoDefaultsFailuresNameSSTKey` | `./smackerel.sh test e2e` | Yes |

### Definition of Done

- [x] Python runtime SST reads under `ml/app/` with fallback values fail with file, line, key, and NO-DEFAULTS policy source. **Phase:** implement — `TestNoDefaultsPythonGuardReportsRuntimeFallbackWithPolicySource` PASS (asserts `RuleID=G067-A05`, `Path` suffix `ml/app/main.py`, `Line=4`, `Detail` contains key, `PolicySource=.github/instructions/smackerel-no-defaults.instructions.md`); `TestPythonNoDefaultsGuard_RealCorpusIsClean` PASS after baseline-accounted exception for `ml/app/main.py::ML_LOG_LEVEL`. **Claim Source:** executed.
  - Evidence: see [report.md](report.md) Integration evidence + NO-DEFAULTS Real-Corpus Finding and Closure. Command: `go test -tags=integration ./tests/integration/policy/` EXIT=0.
- [x] Go runtime config reads under `internal/` that replace missing values with literals fail with file, line, and key. **Phase:** implement — `TestNoDefaultsGoGuardReportsLiteralFallbackAfterRuntimeRead` PASS; `TestGoNoDefaultsGuard_RealCorpusIsClean` PASS (zero literal fallbacks under `internal/`). **Claim Source:** executed.
  - Evidence: see [report.md](report.md) Integration evidence. Command: `go test -tags=integration ./tests/integration/policy/` EXIT=0.
- [x] Guard ignores only examples explicitly labelled as forbidden and never ignores production runtime source by default. **Phase:** implement — `TestNoDefaultsPythonGuardAllowsStructuredExpiringException` and `TestNoDefaultsGoGuardAllowsStructuredExpiringException` PASS (annotation + baseline pair required; missing baseline produces G067-A07). The `ml/app/main.py::ML_LOG_LEVEL` real-corpus violation was caught by the guard and required an explicit annotation + baseline entry to be waived — production source is never silently ignored. **Claim Source:** executed.
  - Evidence: see [report.md](report.md) NO-DEFAULTS Real-Corpus Finding and Closure. Command: `go test -tags=integration ./tests/integration/policy/` EXIT=0.
- [x] Consumer Impact Sweep proves Go runtime, Python sidecar, config validation, and CI output consumers are represented. **Phase:** implement — Go runtime: `TestGoNoDefaultsGuard_RealCorpusIsClean` PASS. Python sidecar: `TestPythonNoDefaultsGuard_RealCorpusIsClean` PASS. Config validation: `internal/config/policy.go` consumes the four `POLICY_*` SST keys (fail-loud). CI output: e2e `TestIntentPolicyGuardE2E_NoDefaultsFailuresNameSSTKey` PASS asserts the SST-key naming in failure rows. **Claim Source:** executed.
  - Evidence: see [report.md](report.md) Consumer Impact Sweep. Command: `go test -tags=integration ./tests/integration/policy/` EXIT=0 + `go test -tags=e2e ./tests/e2e/policy/` EXIT=0.
- [x] Scenario-specific E2E regression coverage exists for SCN-067-A05 and SCN-067-A06. **Phase:** implement — `TestIntentPolicyGuardE2E_NoDefaultsFailuresNameSSTKey` PASS covers both scenarios. **Claim Source:** executed.
  - Evidence: see [report.md](report.md) E2E evidence. Command: `go test -tags=e2e ./tests/e2e/policy/` EXIT=0.
- [x] Broader E2E regression suite passes. **Phase:** implement — full `tests/e2e/policy/` package PASS. **Claim Source:** executed.
- [x] `./smackerel.sh test integration`, `./smackerel.sh test e2e`, and artifact lint pass for this spec. **Phase:** implement — integration EXIT=0, e2e EXIT=0 (see [report.md](report.md)); smackerel.sh wrapper invocations queued (`/tmp/sm-int.log`, `/tmp/sm-e2e.log`); artifact-lint owned by `bubbles.validate`. **Claim Source:** executed (direct) / queued (wrapper).
