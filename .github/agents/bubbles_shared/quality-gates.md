# Quality Gates

Use this file for test taxonomy, evidence, anti-fabrication, and completion-gate expectations.

## Canonical Test Taxonomy

Use these categories based on execution reality:

- `unit`
- `functional`
- `integration`
- `ui-unit`
- `e2e-api`
- `e2e-ui`
- `stress`
- `load`

Do not label mocked tests as live-stack categories.

## Real Implementation And No-Mock Reality

- production code must not rely on stubs, fake data, or placeholder responses
- tests must not mock internal business logic or internal repositories for live-system categories
- real test storage must be used where the category requires it

## Execution Evidence Standard

Pass/fail claims require actual executed evidence.

Minimum bar:

- current-session execution
- raw output
- real exit status or tool result
- enough output to verify what actually happened
- `**Claim Source:**` tag classifying evidence provenance (see evidence-rules.md)

Summaries are not evidence.

## Evidence Provenance Standard

Every evidence block MUST include a `**Claim Source:**` tag. Valid values:

- `executed` — output directly proves the DoD claim without interpretation
- `interpreted` — command ran but DoD conclusion requires agent reasoning about the output (must include `**Interpretation:**` line)
- `not-run` — no command was executed; item MUST stay `[ ]` with Uncertainty Declaration

A missing `**Claim Source:**` tag is treated as `interpreted` by default — never as `executed`. When in doubt about whether evidence is `executed` or `interpreted`, agents MUST use `interpreted`.

Mislabeling `interpreted` evidence as `executed` is a provenance fabrication and triggers the same consequences as other fabrication types.

## Anti-Fabrication Rules

Block completion when any of these occur:

- claims without execution
- fabricated files, commands, or results
- placeholder or template evidence
- fake green status from skipped or noop tests
- stale state claiming more completion than artifacts support
- narrative completion claims without a structured RESULT-ENVELOPE (equivalent to fabrication for tracking purposes)
- unresolved pseudo-completion language in report or scope artifacts (see Gate G040 in state-gates.md)
- **analysis-as-execution (Gate G071):** reading files that a script would check, performing equivalent pattern matching, and reporting predicted findings as command output — even when predictions are accurate (see evidence-rules.md)
- **provenance fabrication:** labeling evidence as `executed` when the DoD claim requires interpretation of the output
- **missing provenance:** evidence blocks without `**Claim Source:**` tags (treated as `interpreted` by default)
- **build-once-deploy-many violation (Gate G081, advisory in framework / blocking in opted-in product repos):** deployment manifests pinned by mutable tag instead of `sha256:<digest>`, CI workflows that fuse build with deploy, adapter `apply.sh` that builds locally or skips signature verification (see state-gates.md and bubbles-deployment-target-adapter skill)
- **execution-narration fabrication (Gate G021, AF-NARRATION):** a first-person execution or delegation claim in agent narration ("dispatched to X", "mid-flight", "resuming from session state", "tests pass") with no matching in-session tool call, dispatch, or file read — fabrication independent of whether any artifact is written. This facet is a self-check / G020 cross-agent-verification obligation; G021 Check 12 stays artifact-scoped.
- **borrowed-context fabrication (Gate G021, AF-BORROWED-CONTEXT):** restating operator-supplied context (pasted screenshots, terminal scrollback, another repository's logs, or another session's state) as the agent's own execution evidence, or using it to infer a work mandate. Such context is diagnostic input only; repository-selection authority is governed separately by G129/IMP-103.

## Test Execution Gate

Before claiming completion for implementation work:

- all required test categories must run
- failures must be fixed, not deferred
- regression coverage must exist for changed behavior
- skipped or proxy tests must be treated as failures for required behavior

## Unbreakable E2E Guardrails

Forbidden patterns for required live-system tests include:

- `if (page.url().includes('/login')) { return; }` or equivalent redirect bailout in an authenticated scenario
- `if (!hasControl) { return; }` or equivalent missing-feature bailout in a required test body
- optional assertions for required behavior that let the test continue without proving the user-visible outcome
- bug regression tests where every fixture already satisfies the broken filter or gate

These patterns convert real failures into silent passes and block completion.

## No Self-Validating Test Setup

Tests must not validate their own fixture data. Every assertion must prove that the **code under test** produced the expected output — not that the test's own hardcoded values round-tripped unchanged.

Prohibited patterns:

- Test injects hardcoded mock data → calls trivial pass-through code → asserts on the same hardcoded values
- Test creates fixture with `value: 0.912` → code returns it unprocessed → test asserts `== 0.912`
- Test constructs expected response → injects it via mock/stub → asserts response matches what was injected
- E2E/integration test seeds exact literal values → asserts on those same literals without verifying that real code processed, computed, persisted, or retrieved them

Detection heuristic: if the code under test were replaced with `return input` or `return hardcodedLiteral`, would the test still pass? If yes, the test is self-validating.

Valid alternatives:

- Assert on computed output: `add(2, 3) == 5` — the value `5` is the product of real logic
- Assert on structural correctness: element exists, has numeric content within valid range, has expected format/type
- Assert on round-trip transformation: write via real API → read back via real API → values match and format is correct
- Assert on behavioral contracts: given known input, the code produces output matching the specification's defined transformation rules

This is a blocking test quality failure for all test categories.

## Adversarial Regression Tests For Bug Fixes

Every bug-fix regression test must include at least one adversarial case that would fail if the bug were reintroduced.

- filter or gate bugs must include data that does not satisfy the buggy condition
- auth or redirect bugs must assert the forbidden redirect or logout does not happen
- persistence or shape bugs must use the edge-case payload that triggered the original defect and verify round-trip behavior

Tautological regressions are invalid even when they execute real code and contain assertions.

## Sequential Completion

Specs and scopes complete in order. Do not advance later required work while earlier required work remains incomplete.

## Cross-Agent Output Verification

When one agent depends on another agent’s result, the downstream agent must verify the result rather than trust an unverified claim.

## Live-State Fixture Ownership

- Any agent that writes to a live stack must provision or identify dedicated owned fixtures before mutation.
- Listing existing entities and mutating the first result is a blocking shared-state violation for write paths.
- Shared defaults, inherited configs, host/global settings, and other baseline records must be treated as protected state.
- Protected-state mutations require baseline capture plus verified restore or explicit isolated fixture scoping.

## Mutation Remediation Gate

- Exploratory or stochastic runs cannot stop at report-only while the runtime state they mutated remains broken.
- If an agent-created or agent-mutated state exposes a blocking failure, the agent must either restore the pre-run baseline or route the issue into the required fix cycle and leave status in progress.
- Cleanup or restore failures are blocking validation failures, not informational notes.

## Specialist Completion Chain

Modes that require specialist phases are not complete until all required specialist phases have actually executed and their outputs satisfy the required gates.

## Phase-Scope Coherence

`execution.completedPhaseClaims`, `certification.certifiedCompletedPhases`, and `certification.completedScopes` must agree with the actual scope files and the actual work performed.

## Implementation Reality Scan

Implemented artifacts must show real execution depth and real consumers where applicable. Placeholder handlers, dead libraries, or unwired surfaces are blocking failures.

## Integration Completeness

Every implemented artifact must be wired into the running system with at least one real consumer or an explicit documented external-only contract.

## Vertical Slice Completeness

For cross-layer work, frontend calls, gateway routing, backend handlers, and persistent behavior must line up end-to-end. Partial cross-layer delivery is not complete.

## Design Readiness Before Implementation

Implementation cannot outrun missing or contradictory design intent. If required business or design artifacts are absent or inconsistent, route to the owning specialist first.

## Findings Artifact Update Protocol

When hardening, gaps, security, or audit work discovers missing work, scope artifacts must be updated so downstream agents have executable follow-up items.

## Cross-Artifact Coherence

`spec.md`, `design.md`, `scopes.md`, `report.md`, `uservalidation.md`, and `state.json` must not contradict each other.

## Quality Work Standards

- no stubs
- no TODO completion claims
- no deferred mandatory work
- no warnings treated as success
- no fake or shallow testing for required behavior

## Fabrication Termination Protocol

When fabrication or equivalent invalid completion behavior is detected:

1. fail the current gate
2. lower completion state immediately
3. record the violation
4. re-run only after real remediation

## Mandatory Completion Checkpoint

Before any final completion claim, confirm:

- artifact state is coherent
- test and evidence gates are satisfied
- no required live-stack gaps remain

## Gate Family Reference (G082–G134)

> **Range:** the canonical gate set runs G001–G134 (G096 is burned; G101 is the
> release-delivery reconciliation gate; G102–G109 is a reserved gap). The
> sections above narrate the foundational gates
> (G001–G081) by topic. This reference covers the later gate families so the
> module stays current with `bubbles/registry/gates.yaml` — which remains the
> single source of truth for each gate's exact name, behavior, and enforcing
> script. Use `bubbles/scripts/gate-meta.sh <id>` for the authoritative entry.

**Convergence & context discipline (G082–G086)** — keep autonomous loops honest:
- **G082** `convergence_cap_enforcement_gate` — caps convergence iterations (default 10) so a loop cannot spin forever (`convergence-cap-guard.sh`).
- **G083** `context_compaction_discipline_gate` — orchestrators compact trailing transition packets within per-spec budgets before the next dispatch (`compaction-discipline-guard.sh`).
- **G084** `pre_existing_deferral_block_gate` — "pre-existing / out of scope" without a filed artifact is forbidden (`pre-existing-deferral-guard.sh`).
- **G085** `framework_dogfood_evidence_gate` — the Bubbles source repo MUST NOT keep persistent `specs/`; framework work lives in `improvements/` (`framework-dogfood-guard.sh`).
- **G086** `orchestrator_persistence_lint_gate` — orchestrators auto-continue after a non-terminal phase; only convergence/cap/stop/impossibility halts (`orchestrator-persistence-lint.sh`).

**Planning & spec integrity (G087–G091)** — protect the plan→implementation chain:
- **G087** `planning_packet_implementation_linkage_gate` — planning packets link to their implementation evidence.
- **G088** `post_certification_spec_edit_gate` — edits to a certified spec re-open revalidation rather than silently mutating done work.
- **G089** `inter_spec_dependency_gate` — declared cross-spec dependencies are revalidated when a dependency changes.
- **G090** `retro_convergence_health_evidence_gate` — the `framework-health` retro emits convergence-health evidence.
- **G091** `planning_workflow_chain_gate` — delivery-capable planning preserves the ordered `analyst → ux → design → plan` chain (`planning-workflow-chain-guard.sh`).

**Terminal status & delivery delta (G092–G093)** — completion honesty:
- **G092** `strict_terminal_status_gate` — new certification writes use only `done` or `blocked`; non-blocking notes attach as observations (`strict-terminal-status-guard.sh`).
- **G093** `delivery_implementation_delta_gate` — done-ceiling delivery modes prove real implementation delta outside planning bookkeeping (`delivery-implementation-delta-guard.sh`).

**Capability & discovery (G094–G095, G097)**:
- **G094** `capability_foundation_gate` — capability-first design when proportionality triggers fire (`capability-foundation-guard.sh`).
- **G095** `discovered_issue_disposition_gate` — every observed issue gets an explicit disposition (fixed/bug-filed/spec-filed/ops-filed/routed) (`discovered-issue-disposition-guard.sh`).
- **G097** `requirement_mechanism_correspondence_gate` — a requirement that names a mechanism (PKCE, mTLS, …) must show that mechanism in code or an explicit justification (`requirement-mechanism-guard.sh`). *(the former G096 slot is burned — never reused.)*

**Observability posture & SLO (G098–G100)** — IMP-001; see `bubbles-observability-adapter` skill:
- **G098** `observability_posture_declared_gate` — every repo declares `traceContracts.observability.posture` (wired/opted-out); undeclared WARNs (`observability-posture-guard.sh`).
- **G099** `observability_opt_out_freshness_gate` — an opt-out is recorded + expiring; an expired `revisitAfter` raises a non-blocking reminder (`observability-opt-out-guard.sh`).
- **G100** `observability_slo_evidence_gate` — BLOCKING when wired + an instrumented scope targets an `slo:`-linked workflow; captured evidence MUST meet the contract target (`observability-slo-guard.sh`).

**Release-delivery reconciliation (G101)** — IMP-006; `bubbles.releases` + `bubbles.goal`/`bubbles.sprint` convergence:
- **G101** `release_delivery_reconciliation_gate` — every `delivery=required` feature in `docs/releases/<phase>/features.md` (machine-bound via `<!-- bubbles:feature id=… spec=… delivery=required -->` plus a `<!-- bubbles:reconciled-packet … -->` header) MUST map to a TERMINAL + validate-certified spec; a promised-but-unspecced, non-terminal, blocked, or implement-self-certified required feature is a finding. WARN-grandfathered without the header; BLOCKING with the header or `--require-coverage` (the goal/sprint release-phase convergence path); a malformed reconciled packet fails loud; framework source EXEMPT. Compile-time twin: `scenario-compile-lint.sh` requires a scenario's `rootOutcome.targetReleasePacket` to cover every required feature with a delivery node (`release-delivery-reconciliation-guard.sh`).

**Release-train & upkeep (G110–G120)** — `bubbles.train` + `bubbles.upkeep`; enforced mainly by `release-train-guard.sh` + `env-pollution-scan.sh`:
- **G110** release-train discipline · **G111** flag default-off on other trains · **G112** backup evidence · **G113** restore-drill evidence · **G114** BCDR evidence · **G115** env-pollution isolation (test code never writes prod monitoring/backup/manifest) · **G116** offsite-backup-required for prod trains · **G117** audit-trail immutability · **G118** backup-retention declared · **G119** secret-rotation recorded (hashes, never values) · **G120** PII classification declared.

**Cross-train propagation (G121–G123)** — `bubbles.propagate`; `propagation-policy-guard.sh`:
- **G121** propagation policy declared · **G122** receiving-train validation required · **G123** propagation ledger recorded (append-only).

**Incident, framework-health, model-tier, capability-consumer, session-cap, repository affinity (G124–G129)**:
- **G124** `incident_severity_declared_gate` — `incident-fastlane` classifies each finding's severity; an incident routes rollback to `bubbles.train`.
- **G125** `framework_health_evidence_gate` — `bubbles.retro target: framework` emits a proposal under `improvements/` and never mutates framework files (`retro-framework-health.sh`).
- **G126** `model_tier_floor_gate` — high-stakes phases enforce a model floor when declared (`model-tier-advisory.sh`).
- **G127** `capability_consumer_freshness_gate` — every `state: shipped` capability in `capability-ledger.yaml` declares a non-empty `consumers:` list whose paths exist (`capability-consumer-freshness.sh`).
- **G128** `session_cap_enforcement_gate` — BLOCKING; the aggregate (whole-session) sibling of G082. Mechanically enforces the `sessionBudget` caps (`maxTotalConvergenceIterations`/`maxWallClockMinutes`/`maxToolCalls`) recorded in `.specify/memory/bubbles.session.json`; default-off (all caps null → no-op), enforced only when a cap is set AND measurable, RFC3339 wall-clock math via jq (GNU/BSD-identical), no `--skip`/`--force` bypass, invoked as state-transition Check 40 (`session-cap-guard.sh`).
- **G129** `repository_binding_classification_discovery_conformance_gate` — BLOCKING; rejects ambient repository inference by requiring an actionable same-session repository decision before repository-local state/discovery/dispatch, normalizing explicit roots before binding, treating the repo-local session snapshot as a post-selection mirror only, refusing unbound multi-root targetless work, confining discovery to `resolvedRepositoryRoot/specs`, preserving exact session/root/decision/revision and scoped goal-node provenance, redacting public projections, and retaining upstream-first ownership. Enforced by `repository-binding.sh` plus `repository-binding-conformance-guard.sh`; no bypass.

**Product-domain correspondence (G130–G131)** — IMP-106; opt-in `domainModel:` block in `.github/bubbles-project.yaml`; advisory-until-configured (the Bubbles source repo, which declares no `domainModel:`, is a clean no-op):
- **G130** `domain_invariant_correspondence_gate` — ADVISORY-UNTIL-CONFIGURED (the domain-invariant sibling of G097); INERT unless a repo declares an optional `domainModel:` block. Where G097 asks "a requirement names PKCE/OAuth2 — does the code implement it?", G130 asks "a `domainModel` invariant (e.g. `Order.status ∈ {created,paid,shipped,refunded}`) — is it mechanically ANCHORED, or just prose?". Each `domainModel.invariants` entry clears by ANY of: code evidence of an `enforcedBy` mechanism token in the scope's declared files, a linked adversarial `provedBy` test that rejects the violating input, or an explicit `## Domain-Invariant Justifications` / `Invariant-Justification:` disclosure. Specs created before 2026-07-27 are WARN-grandfathered. Enforced by `domain-invariant-guard.sh` (state-transition delegated tail Check 41); no `--skip`/`--force` bypass.
- **G131** `domain_model_consistency_gate` — ADVISORY-UNTIL-CONFIGURED consistency NUDGE (the DOM-SST sibling of G130); INERT without the `domainModel:` block. Nudges when a feature's `design.md ## Data Model` declares an entity absent from the shared `domainModel.entities`, or an Outcome-Contract `INV-*` id is absent from `domainModel.invariants` — promote it into the shared SST so one source of truth serves every feature (and G130 can then check enforcement). Advisory by default even when configured; blocks ONLY under an explicit `domainModelConsistencyGuard: block` key. Feeds G044 a structured state-machine model instead of prose-grepping. Enforced by `domain-model-consistency.sh` (delegated tail Check 42); no bypass.

**Claim grounding (G132)** — the mechanical half of the No Phantom References rule in [claim-grounding.md](claim-grounding.md):
- **G132** `reference_existence_gate` — ADVISORY-UNTIL-OPT-IN phantom-reference detector. A phantom reference is a citation to a path that does not exist. It is the most damaging ungrounded claim because it READS as verified evidence and it PROPAGATES: the next agent treats it as established and builds on it. DIVISION OF LABOR: G021 verifies a claimed COMMAND ran, G072 tags HOW an evidence block supports its claim, `gate-id-grep.sh` resolves GATE ID references — none verify that a cited PATH exists. G132 owns path references. Checks relative markdown link targets, resolved against the linking file's directory. Deliberately skips external schemes, absolute paths, bare `#anchor` links, placeholder targets containing `< > { } $ *`, fenced code blocks, inline code spans, and bare backticked paths (a `.github/bubbles/...` path is often a downstream projection that correctly does not exist in a source checkout). Caller-supplied scan surface with NO default. Inline exemption `ref-ok:<reason>`; no `--skip`/`--force`/allowlist bypass. Advisory by default; blocks ONLY under `referenceExistenceGuard: block`. LIMIT: catches dead paths, not a live path whose content fails to support the claim. Enforced by `reference-existence-lint.sh`.

**Test-evidence substance (G133)** — evidence that proves nothing is not evidence:
- **G133** `collected_test_count_gate` — BLOCKING; refuses test evidence whose own output states that ZERO tests ran. This is the gap the ≥10-line raw-output rule left open: a downstream e2e suite collected no tests for 15 days while twelve spec commits recorded passing-looking evidence, because the runner exited non-zero (which reads as ordinary test failure) and raw output from a broken runner is still raw output. The missing assertion was never "is there output" but "did any test actually run". Scans ONLY text inside fenced code blocks — the fence is what separates captured runner output from prose (an earlier prose-scanning version was circular: the sentence "No tests found in this area were affected" supplied its own context word). Inside a fence, an explicit collection-zero signal from jest, playwright, pytest, go, cargo, vitest, mocha or `node --test` is a finding. The pattern set is deliberately small because generic patterns drown the signal (bare `0 total` matched jest's `Snapshots: 0 total` 354 times; bare `0 passed` matched the tail of `10 passed`). Evidence with no recognisable count is REPORTED, not failed — runner formats vary too widely for absence to be proof. RATCHET: pre-existing hits are frozen in a per-repo `<repo>/.specify/collected-test-count-guard.baseline` that the consuming repo generates with `--update-baseline`; it may shrink, never grow. No `--skip`/`--force`/`--ignore` bypass — evidence that proves nothing is fixed by re-running the suite, never by suppressing the check. Enforced by `collected-test-count-guard.sh`.

**Goal fidelity (G134)** — the goal-fidelity capabilities exist, but nothing reaches them unless a boundary calls them:
- **G134** `goal_fidelity_gate` — BLOCKING; makes the IMP-038 goal-fidelity capabilities load-bearing. `goal-contract.sh` can freeze and verify a goal, `work-boundary-resolve.sh` can refuse an undeclared boundary, and the envelope validator can refuse dishonest finding accounting — but none of them is REACHED unless a runner chooses to call it, so a run that simply never called them passed everything. G134 checks the six boundaries where drift actually enters, each independently selectable so a caller pays only for the one it is standing on. `pre-planning`: the contract exists, is schema-valid, is FROZEN (an unfrozen contract can be edited afterwards to match whatever the run did), and names a repository. `post-planning`: spec.md carries a non-empty Outcome Contract, state.json declares a `workBoundary`, and `execution.goalContractRef` points at the SAME goal the session holds. `pre-dispatch`: the candidate repository, spec and path resolve in-boundary under strict mode, and any supplied `goalRef` carries the current digest. `post-finding`: no changed path fell outside the declared boundary — handling a finding never authorizes editing outside the approved reach, which is the specific way a bounded run turns into an unbounded one. `post-compaction`: the resumed `goalRef` still matches, so a resume cannot silently continue against a superseded revision. `pre-certification`: report.md references the declared Success Signal, Hard Constraints were stated, and the spec is not certifying against a revision the session has already superseded. Also carries the G070 repair for ordinary feature work (G070 previously declared `enforcedBy: [ unbound ]` while crediting `artifact-lint.sh` with a presence check it never contained). Exits 0 (all selected boundaries held), 1 (a boundary failed, findings printed one per line), or 2 (usage error). There is NO `--force`/`--skip`/`--ignore`: a boundary that cannot be satisfied is NARROWED, or expanded through `goal-contract.sh revise --approval-note` with recorded operator approval. Hermetic selftest `goal-fidelity-guard-selftest.sh` runs 39 cases, each pairing a green fixture with its drifted variant. Enforced by `goal-fidelity-guard.sh`.

- no fabricated, deferred, or contradictory claims remain