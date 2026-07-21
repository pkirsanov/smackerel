## Workflow Delegation Core

Use this module to keep routing responsibilities separated across the Bubbles front door.

### Delegation Boundaries

- `bubbles.super` is the ONLY natural-language dispatcher. It owns plain-English translation into workflow parameters, exact slash-command guidance, and framework-operation routing. `bubbles.workflow` and `bubbles.iterate` MUST NOT maintain duplicate intent-to-mode mapping tables.
- `bubbles.iterate` is the ONLY highest-priority work picker. It owns backlog/work discovery, next-action selection, and `WORK-ENVELOPE` output. `bubbles.workflow` MUST NOT maintain its own work-priority heuristic when iterate is available.
- `bubbles.goal` is the universal goal endpoint. It may achieve one outcome through zero, one, or several authorized workflow modes plus direct specialist dispatch.
- `bubbles.workflow` is a narrow single-mode runner. It accepts one explicit `mode:` or one mode resolved by `bubbles.super`, then executes exactly one resolved workflow mode. It does not decompose broad goals, prioritize unrelated work, or compose multiple root modes.
- `bubbles.sprint` is the time-bounded multi-goal runner. It prioritizes a goal queue and applies the goal execution contract directly within the active sprint runtime.

### Input Classification Contract

Classify incoming workflow requests into exactly one of these buckets before Phase 0:

1. `STRUCTURED` — explicit `mode:` keyword is present WITH concrete spec targets. `bubbles.workflow` may continue directly. **NOTE:** If spec targets are present but NO explicit `mode:` keyword exists, this is NOT structured — classify as `VAGUE` and delegate to `bubbles.super` for intent resolution. The presence of spec targets alone does not make a request structured.
2. `CONTINUATION` — continuation envelopes, run-state, recap/status/handoff packets, or explicit continuation language tied to active workflow state are present. Preserve the active workflow mode when possible. **Binding re-validation (IMP-025 MR3):** when a continuation envelope carries a `provenance` block (`repositoryRoot` / `agentSourceRoot` / `frameworkVersion`), re-validate the repo↔agent binding on resume before mutable work — run `bubbles/scripts/repo-binding-preflight.sh --repo-root <repositoryRoot> --agent-source <agentSourceRoot>` (or `--canonical-source` for framework work). A binding mismatch is a REFUSE: the resumed session is bound to a different workspace root than the handoff assumed; surface the mismatch + remediation instead of editing.
3. `VAGUE` — plain-English goal with no explicit `mode:` keyword, OR spec targets present without `mode:`. Delegate to `bubbles.super` and consume a `RESOLUTION-ENVELOPE`. This includes requests with planning-intent language ("plan", "design", "scope", "create specs", "create bugs", "planning cycle") even when the user names specific specs or features — the intent still needs NL-to-mode translation.
4. `CONTINUE` — generic keep-going language with no recoverable active workflow target. Resolve through `bubbles.super` and route to `bubbles.goal` or `bubbles.iterate`; the workflow runner does not pick unrelated work.
5. `FRAMEWORK` — framework operations such as doctor, hooks, upgrade, status, metrics, lessons, gates, or install. Delegate to `bubbles.super` and consume a `FRAMEWORK-ENVELOPE`.

### Work-Boundary Preflight (R6 — anti-wandering)

The binding re-validation in the `CONTINUATION` bucket runs on **resume**. Extend the SAME repo↔agent check to **initial mutable start**, not only on resume: before the FIRST mutable action of any classified request (`STRUCTURED`, or a `VAGUE` request after `bubbles.super` resolution), run `bubbles/scripts/repo-binding-preflight.sh` (`--canonical-source` for framework work) so a fresh session bound to the wrong workspace root REFUSES before editing — the resume path is not the only entry that can be mis-bound.

At **each specialist dispatch**, before handing candidate work to a phase owner, consult the work-boundary resolver against the feature's declared boundary and honor the returned `disposition`:

```
bubbles/scripts/work-boundary-resolve.sh --feature-dir <FEATURE_DIR> --candidate-repo <slug> \
    [--candidate-spec <id>] [--candidate-path <path>]
```

- `disposition=in-boundary` → dispatch inline as normal.
- `disposition=route-same-repo` → an unrelated same-repo finding: FILE/route it; do not fix it inline in this scope.
- `disposition=route-cross-repo` → a different-repo finding under `crossRepoPolicy: authorized`: route to the owning repo (route-only, never inline).
- `disposition=refuse-cross-repo` → a different-repo finding under the default forbidden policy: REFUSE; surface the boundary + remediation instead of editing the other repo. This is the direct stop for the "started on repo A, wandered into fixing repo B" failure.

Backward-compatible: a feature with no declared `workBoundary` (or no `state.json`) resolves `in-boundary` (no behavior change); a present-but-malformed boundary exits 2 (fail-closed). The resolver COMPOSES with — does not replace — `repo-binding-preflight.sh`: preflight verifies the repo↔agent binding, the resolver classifies each candidate change against the boundary.

### ⛔ Literal `mode:` Gate (MANDATORY — NON-NEGOTIABLE)

Before applying the classification contract, perform this literal substring check:

1. **Scan the raw user input for the exact token `mode:`**
2. If `mode:` is NOT present anywhere in the input → classify as `VAGUE` → delegate to `bubbles.super`
3. If `mode:` IS present → continue to the classification rules above

**There are ZERO exceptions.** The following do NOT constitute structured input:
- Action verbs: "execute", "plan", "deliver", "implement", "run", "complete", "invoke"
- Spec references: "specs/099-108", "each recommendation", "all features"
- Phase language: "full planning workflow", "planning chain", "planning phases"
- Agent references: "invoke correct workflow/agent"
- Numbered lists of work items

**Known failure pattern:** `bubbles.workflow` receives NL input → skips this gate → self-selects a mode based on keyword matching → proceeds without `bubbles.super` resolution. This is the #1 observed violation and MUST be mechanically prevented by checking for `mode:` FIRST.

### Required Delegation Rules

### Runtime Depth Compatibility Contract

- Do not assume a subagent can invoke another subagent. Some host runtimes expose `agent`/`runSubagent` only to the active top-level agent.
- Workflow-running orchestrators MUST NOT invoke another workflow-running orchestrator as a subagent. The active top-level runner resolves the mode itself, verifies its grant in `workflowModeGrants`, invokes the required phase owners directly, and records `executionModel: direct-authorized-runner`.
- Envelope-only utility dispatch remains allowed: `bubbles.super` may return a resolution envelope and `bubbles.iterate` may return a picker-only work envelope because neither path launches a nested workflow.
- A domain orchestrator invoked as a phase owner performs only that phase and returns its result envelope. It may execute its granted workflow modes only when it owns the top-level runtime.
- If the active orchestrator itself lacks `agent`/`runSubagent`, return `blocked`; do not emulate owner work inline and do not claim a delegation happened.

- When the request is `VAGUE`, invoke `bubbles.super` as a subagent and require a `## RESOLUTION-ENVELOPE` only.
- When the request is `CONTINUE` and no concrete workflow continuation can be recovered, invoke `bubbles.super` for a `RESOLUTION-ENVELOPE` and route to its `targetAgent`.
- When the request is `FRAMEWORK`, invoke `bubbles.super` as a subagent and require a `## FRAMEWORK-ENVELOPE` only.
- `bubbles.workflow` MUST NOT re-run a second natural-language inference pass after `bubbles.super` has resolved the request.
- `bubbles.workflow` MUST NOT recreate a local intent-to-mode keyword table or a local backlog-priority picker once these delegation paths are available.

### Envelope Consumption Rules

- `RESOLUTION-ENVELOPE` provides the resolved workflow mode, targets, and optional tags for Phase 0.
- `WORK-ENVELOPE` provides the resolved spec, scope, workflow mode, and work type for Phase 0.
- `FRAMEWORK-ENVELOPE` is terminal for framework operations; report the result and stop instead of entering the workflow phase engine.

### Continuation Preservation Rules

- Preserve `stochastic-quality-sweep`, `iterate`, and `full-delivery` when continuation context proves one of those modes is still active.
- Treat phrases such as `fix all found`, `fix everything found`, `address rest`, `fix the rest`, `resolve remaining findings`, or `handle remaining issues` as workflow continuation, not as permission to downshift into direct specialist execution.
- If continuation context narrows the remaining work to bug-only, docs-only, or validate-only work, route to the narrower workflow mode instead of echoing raw specialist commands.

### Delegated Intent Resolution Summary

Use this summary before Phase 0 when no explicit `mode:` is present:

1. `STRUCTURED` input (explicit `mode:` + spec targets) stays inside `bubbles.workflow`.
2. `VAGUE` input (no `mode:` keyword, OR natural-language intent even with spec targets) delegates to `bubbles.super` and consumes only a `RESOLUTION-ENVELOPE`.
3. `CONTINUE` input with no recoverable active workflow delegates resolution to `bubbles.super` and routes to the returned top-level runner.
4. `FRAMEWORK` input delegates to `bubbles.super` and consumes only a `FRAMEWORK-ENVELOPE`.
5. After `bubbles.super` resolves the request, `bubbles.workflow` MUST NOT run a second natural-language inference pass.
6. **The `STRUCTURED` classification requires the literal keyword `mode:` in the input.** Spec targets, feature names, or natural-language descriptions — even when they reference specific specs — are NOT sufficient for `STRUCTURED` classification without `mode:`.