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

Scope and report artifacts must not contain unresolved pseudo-completion language or unresolved deferred-work prose when the spec/bug status is `done` or transitioning to `done`.

G040 has two enforcement points in `state-transition-guard.sh`:

### Enforcement Point 1: Pseudo-completion narrative scan

Blocking phrases (outside quoted historical evidence blocks):
- `Next Steps` (as heading or bullet leader)
- `Recommended routing:` / `Recommended resolution:`
- `Ready for /bubbles.` / `Re-run /bubbles.validate`
- `Commit the fix` / `Record DoD evidence` / `Run full E2E suite`
- `[PENDING` / `header only initially`

Enforced by: `artifact-lint.sh` (report.md scan) and `state-transition-guard.sh` (report.md scan).

### Enforcement Point 2: Deferral Language Scan (Check 18)

Scope and report artifacts MUST NOT contain raw deferred-work prose (e.g. "deferred to next sprint", "skip for now", "punted to Phase 3", "out of scope for this iteration") when status is `done`. The check excludes:

1. **Schema-canonical follow-up field names** mandated by completion-governance.md: `followUpOwner`, `followUpAction`, `followUpTarget`, `followUps` (case-insensitive). These are the structured mechanism for tracking concerns under `done_with_concerns`, not deferred work.
2. **The canonical section heading** `## Follow-Up Narrative` (and `## Follow-Up Section`). The schema-allowed container heading is exempt.
3. **Code-fenced content** (```` ``` ```` blocks). Test descriptions, examples, and historical evidence inside fences are not scanned.
4. **Sentinel-marker-bracketed regions**. Authors may wrap prose between `<!-- bubbles:g040-skip-begin -->` and `<!-- bubbles:g040-skip-end -->` markers to exempt schema-allowed quoted material that would otherwise trip the deferral pattern. Marker lines themselves are stripped before scanning.

**`done_with_concerns` skip:** When `state.json.status == "done_with_concerns"`, Check 18 is skipped entirely with an INFO line. The completion-governance schema explicitly permits follow-up narrative under that outcome state, and the structured `concerns:`/`followUps:` schema fields are the contract for tracking those follow-ups.

If any unmatched deferral hit is found under `status == "done"`, the transition is blocked.

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

## State-Snapshot Fabrication Gates (G075–G078)

These four gates harden `state.json` against the most common fabrication patterns observed in downstream product repos. They are mechanically enforced by `state-transition-guard.sh` (Checks 2B, 5B, 5C, 7A/7B) and `bubbles/scripts/batch-promotion-lint.sh` and are listed in `bubbles/workflows.yaml` `gates:` for canonical reference.

| Gate | Name | Purpose | Enforced by |
|------|------|---------|-------------|
| G075 | `scope_index_parity_gate` | Per-scope-directory layout only. The `scopes/_index.md` status column MUST match the `**Status:**` line of every linked `scopes/NN-name/scope.md`. Detects fabricated batch promotions that update individual scope files to `Done` while leaving `_index.md` showing `In Progress`. | `state-transition-guard.sh` Check 5B |
| G076 | `phantom_scope_detection_gate` | Every entry in `state.json` `completedScopes` (and `certification.completedScopes`) MUST map to a real scope artifact on disk (a `scopes/NN-name/` directory or a `## Scope N:` heading in `scopes.md`). Phantom entries naming scopes that don't exist are a documented fabrication pattern. | `state-transition-guard.sh` Check 5C |
| G077 | `execution_history_plausibility_gate` | `executionHistory` entries MUST have plausible timestamps: no identical-interval clusters (3+ runs spaced exactly the same), no zero-duration non-trivial entries, no overlapping entries. Also enforces `certification.lockdownState.round` ≤ implement-phase run count and `lastCleanRound` ≤ `round`. | `state-transition-guard.sh` Checks 7A and 7B |
| G078 | `batch_promotion_limit_gate` | A single git commit (or push range) MUST NOT promote more than one spec's `state.json` `status` to `done` without explicit operator override. Mass promotions are a documented fabrication pattern (e.g., the QF 2026-03-15 batch that promoted 33 specs in one commit). | `batch-promotion-lint.sh` and the `state-transition-guard` GitHub Actions workflow |

These gates have no associated agent prose elsewhere — `bubbles/workflows.yaml` and this section are the authoritative description.

## Build-Once Deploy-Many Integrity Gate (G079)

**Status in framework:** Advisory. Becomes BLOCKING when a downstream product repo opts in via its own `copilot-instructions.md`.

When a project ships images to multiple environments (dev, staging, prod, home-lab, cloud), the build-once-deploy-many invariant MUST hold: a single git SHA produces one immutable application image and a per-environment family of immutable config bundles. The same image digest is then deployed to every target by pairing it with the matching environment's bundle.

Blocked patterns:
- Any deployment manifest (`deploy/<target>/manifest.yaml`, runtime Compose file, runtime Kubernetes spec, systemd unit ExecStart) referencing an image by mutable tag (`:latest`, `:staging-latest`, `:prod-latest`, `:main`, branch names) instead of by `sha256:<digest>`
- CI workflow that performs `apply`, `deploy`, `ssh`, or any host mutation after image publish
- CI workflow that does NOT publish a `build-manifest-<sourceSha>.yaml` listing image digest(s), bundle hashes, and attestation refs
- Adapter `apply.sh` that invokes `docker build`, `docker buildx build`, `cargo build`, `npm run build`, or any compile step
- Adapter `apply.sh` that falls back to local build if registry pull fails
- Adapter `apply.sh` that does NOT verify the image's cosign signature against a transparency log before container start
- Adapter `apply.sh` that does NOT verify the config bundle hash before extraction
- Adapter `rollback.sh` that rebuilds anything (rollback MUST be a pure pointer-swap on `previousManifest`)
- Config bundle generated on the deploy target instead of in CI
- Config bundle that embeds plaintext secrets instead of secret references
- Config bundle that is non-deterministic (timestamps, random ordering, varying uid/gid produce different hashes for the same SST + sourceSha)
- Two targets sharing the same `manifest.yaml` (each adapter MUST own its own manifest)

Enforced by: `bubbles-deployment-target-adapter` skill (Build-Once Deploy-Many Pattern, CI ↔ Adapter Handshake, Anti-Patterns table), `bubbles-deployment-target.instructions.md` (Build-Once Deploy-Many section), `bubbles-config-sst` skill (Config Bundle Artifact section).

Downstream enforcement (when a product repo declares G079 BLOCKING in its `copilot-instructions.md`): pre-push hook scans for mutable-tag patterns in deployment manifests; CI workflow lint scans for build/deploy fusion; adapter `apply.sh` audit confirms cosign verification call site exists.
