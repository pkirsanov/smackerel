# State Gates

Purpose: compact state/completion rules that must remain authoritative for all agents.

## Completion Chain
- A DoD item becomes `[x]` only after real validation evidence exists inline.
- A scope becomes `Done` (or `Done with Concerns` when all gates pass but agent flags observational risks) only when every DoD item is valid.
- A spec becomes `done` (or `done_with_concerns`) only when every scope is `Done` or `Done with Concerns`.
- `Done with Concerns` is a done-equivalent for all gate checks (G024, G027, G023). Gates treat it identically to `Done`.

## Read / Loop Discipline
- Max 3 consecutive reads before action.
- Max 3 docs per tier before action.
- No redundant rereads without a new reason.
- A reread is allowed when the file changed, the active phase changed, or a newly triggered gate requires re-checking it.
- No hunt loops for missing files.

## State Integrity
- Never inflate `certification.completedScopes`, `execution.completedPhaseClaims`, `certification.certifiedCompletedPhases`, or final status beyond artifact reality.
- Do not batch-complete DoD items.
- Do not bypass gates by reformatting DoD or status fields.
- Only `bubbles.validate` may write `certification.*` fields (Gate G056).
- `policySnapshot` must record effective mode settings with provenance (Gate G055).
- `transitionRequests` and `reworkQueue` must be empty before certification (Gate G061).
- Diagnostic and certification agents must route foreign-owned remediation instead of fixing inline (Gate G042).
- Agent and child-workflow invocations must end with a concrete result outcome, not narrative-only findings (Gate G063).
- Only orchestrators may invoke child workflows, and nesting depth must remain bounded (Gate G064).
- Phase claims in `completedPhaseClaims` must have matching agent provenance in `executionHistory` (Gate G066). An agent may only record its own phase name; cross-phase impersonation is fabrication.

## Mechanical Gates
- `state-transition-guard.sh` — DoD, scope status, certification/execution coherence, policy provenance (G055), certification state (G056), scenario manifest (G057), lockdown/regression (G058/G059), TDD evidence (G060), transition/rework closure (G061), packet/result integrity and framework contract enforcement (G042/G063/G064), phase-claim provenance (G066), source code edit lockout (G073)
- `artifact-lint.sh` — schema validation (v2 + v3), phase coherence, scope parity, specialist completion
- `implementation-reality-scan.sh` — stub/fake/hardcoded data detection
- `regression-quality-guard.sh` — silent-pass bailout detection plus adversarial regression heuristics for bug-fix tests
- `artifact-freshness-guard.sh` — superseded content isolation (G052)
- `traceability-guard.sh` — Gherkin-to-test-to-evidence linkage, scenario manifest cross-check (G057/G059)
- `done-spec-audit.sh` — post-completion audit running state-transition-guard + artifact-lint + traceability-guard for all `done` specs
- `agent-ownership-lint.sh` — ownership/capability registry validation plus owner-only remediation, result-envelope, and child-workflow policy checks (G042/G042/G063/G064)

## Pseudo-Completion Language Gate (G040)

Scope and report artifacts must not contain unresolved pseudo-completion language when the spec/bug status is `done` or transitioning to `done`.

Blocking phrases (outside quoted historical evidence blocks):
- `Next Steps` (as heading or bullet leader)
- `Recommended routing:` / `Recommended resolution:`
- `Ready for /bubbles.` / `Re-run /bubbles.validate`
- `Commit the fix` / `Record DoD evidence` / `Run full E2E suite`
- `[PENDING` / `header only initially`

Enforced by: `artifact-lint.sh` (report.md scan) and `state-transition-guard.sh` (report.md scan).

If any match is found, the transition to `done` is blocked.

## Analysis-As-Execution Gate (G071)

Validation, audit, and test agents must produce evidence from actual terminal command execution, not from reading the files those commands would inspect and predicting findings. Even accurate predictions are fabrication because:

- The canonical script is the source of truth for its own logic.
- An agent's pattern matching may miss or hallucinate issues the real script wouldn't.
- File analysis cannot replicate version checks, cross-file correlations, or stateful path resolution in scripts.

Blocked patterns:
- Reporting lint/guard/test findings without a corresponding `run_in_terminal` invocation
- Producing a numbered issue list by reading artifacts manually instead of running `artifact-lint.sh`
- Predicting `traceability-guard.sh` output by manually grepping scenario/test mappings
- Claiming test pass/fail by reading test source files instead of executing the test runner

When a command cannot be executed, the correct report is `NOT RUN` with reason — never substitute file analysis.

Enforced by: evidence-rules.md (analysis-as-execution section), quality-gates.md (anti-fabrication rules), validation-core.md (rule 5).

## Evidence Provenance Gate (G072)

Every evidence block attached to a DoD item MUST include a `**Claim Source:**` tag with a valid value: `executed`, `interpreted`, or `not-run`.

Blocked patterns:
- Evidence block without a `**Claim Source:**` tag (treated as `interpreted` by default, but missing tag is a lint failure)
- Evidence labeled `**Claim Source:** executed` where the DoD claim is not directly readable in the raw output (provenance fabrication)
- Evidence labeled `**Claim Source:** interpreted` without an `**Interpretation:**` line explaining the reasoning
- DoD item marked `[x]` with `**Claim Source:** not-run` evidence (not-run cannot support completion)
- DoD item left `[ ]` after agent work without an Uncertainty Declaration explaining what was attempted

Enforced by: evidence-rules.md (Evidence Provenance Taxonomy), quality-gates.md (Evidence Provenance Standard), audit-core.md (Evidence provenance review).

## Source Code Edit Lockout Gate (G073)

When a workflow mode's `statusCeiling` is below `done` (e.g., `specs_hardened`, `docs_updated`, `validated`), NO source code files may be modified. The guard script checks `git diff` (staged + working tree) for files matching implementation extensions (`.go`, `.rs`, `.py`, `.ts`, `.tsx`, `.js`, `.jsx`, `.sql`, `.proto`, `.yaml`, `.yml`, `.toml`, `.json`, `.css`, `.scss`, `.html`) outside allowed paths (`specs/`, `docs/`, `.github/`, `.specify/`).

Blocked patterns:
- Any source code file staged or modified in the working tree when the active `workflowMode` has `statusCeiling` below `done`
- Commits containing source code changes under a planning-only mode (detected as warnings on last commit)

Enforced by: `state-transition-guard.sh` (Check 3B), agent-common.md (Mode Ceiling Pre-Flight), bubbles.implement (Mode Ceiling Pre-Flight behavioral rule), bubbles.bug (Phase 5 Mode Ceiling Gate).
