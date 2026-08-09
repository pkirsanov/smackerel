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

1. `STRUCTURED` — explicit `mode:` keyword is present WITH concrete spec targets, concrete bug targets, or concrete ops targets. `bubbles.workflow` may continue directly. **NOTE:** If concrete targets are present but NO explicit `mode:` keyword exists, this is NOT structured — classify as `VAGUE` and delegate to `bubbles.super` for intent resolution. The presence of targets alone does not make a request structured.
2. `TARGETLESS_MODE` — explicit `mode:` keyword is present without concrete spec, bug, or ops targets. Mode-only input and `mode:` plus `repositoryRoot` are both `TARGETLESS_MODE`; `repositoryRoot` selects the repository but is not a concrete work target. Repository preflight MUST commit before mode-specific target requirements or discovery are evaluated.
3. `CONTINUATION` — continuation envelopes, run-state, recap/status/handoff packets, or explicit continuation language tied to active workflow state are present. Preserve the active workflow mode when possible. **Binding re-validation (IMP-025 MR3):** when a continuation envelope carries a `provenance` block (`repositoryRoot` / `agentSourceRoot` / `frameworkVersion`), re-validate the repo↔agent binding on resume before mutable work — run `bubbles/scripts/repo-binding-preflight.sh --repo-root <repositoryRoot> --agent-source <agentSourceRoot>` (or `--canonical-source` for framework work). A binding mismatch is a REFUSE: the resumed session is bound to a different workspace root than the handoff assumed; surface the mismatch + remediation instead of editing.
4. `VAGUE` — plain-English goal with no explicit `mode:` keyword, OR spec targets present without `mode:`. Delegate to `bubbles.super` and consume a `RESOLUTION-ENVELOPE`. This includes requests with planning-intent language ("plan", "design", "scope", "create specs", "create bugs", "planning cycle") even when the user names specific specs or features — the intent still needs NL-to-mode translation.
5. `CONTINUE` — generic keep-going language with no recoverable active workflow target. Resolve through `bubbles.super` and route to `bubbles.goal` or `bubbles.iterate`; the workflow runner does not pick unrelated work.
6. `FRAMEWORK` — framework operations such as doctor, hooks, upgrade, status, metrics, lessons, gates, or install. Delegate to `bubbles.super` and consume a `FRAMEWORK-ENVELOPE`.

### Repository Binding Preflight (MANDATORY)

Every classified request MUST complete `bubbles/scripts/repository-binding.sh preflight` and obtain `PREFLIGHT_COMMITTED` before reading repository-local state, expanding a relative target, scanning a repository, selecting work, invoking a repository-owned command, or dispatching a specialist. Classification does not authorize repository-local work. Mode-specific target requirements and any auto-discovery rule are evaluated only after preflight, against the committed `repositoryRoot` and its current actionable packet.

### Work-Boundary Preflight (R6 — anti-wandering)

The binding re-validation in the `CONTINUATION` bucket runs on **resume**. Extend the SAME repo↔agent check to **initial mutable start**, not only on resume: before the FIRST mutable action of any classified request (`STRUCTURED`, or a `VAGUE` request after `bubbles.super` resolution), run `bubbles/scripts/repo-binding-preflight.sh` (`--canonical-source` for framework work) so a fresh session bound to the wrong workspace root REFUSES before editing — the resume path is not the only entry that can be mis-bound.

At **each specialist dispatch**, before handing candidate work to a phase owner, consult the work-boundary resolver against the feature's declared boundary and honor the returned `disposition`:

```
bubbles/scripts/work-boundary-resolve.sh --feature-dir <FEATURE_DIR> --candidate-repo <slug> \
    [--candidate-spec <id>] [--candidate-path <path>] [--strict] [--require-allowed-paths]
```

- `disposition=in-boundary` → dispatch inline as normal.
- `disposition=route-same-repo` → an unrelated same-repo finding: FILE/route it; do not fix it inline in this scope.
- `disposition=route-cross-repo` → a different-repo finding under `crossRepoPolicy: authorized`: route to the owning repo (route-only, never inline).
- `disposition=refuse-cross-repo` → a different-repo finding under the default forbidden policy: REFUSE; surface the boundary + remediation instead of editing the other repo. This is the direct stop for the "started on repo A, wandered into fixing repo B" failure.

**Which invocation form to use (IMP-038 GF-2).** The three forms differ ONLY in how they treat an *undeclared* boundary; a complete boundary classifies identically in all three.

| Caller | Form | Undeclared boundary |
| --- | --- | --- |
| Read-only diagnostic, reporting, legacy caller | no flag | `in-boundary` (permissive) |
| Any **mutable** run, at every specialist dispatch | `--strict` | REFUSE, exit 3 |
| The **first source mutation** of a mutable run | `--strict --require-allowed-paths` | REFUSE, exit 3 or 4 |

A mutable runner MUST NOT authorize a source edit from the permissive form. Permissiveness exists so a pre-IMP-038 spec stays *readable*; it was never a licence to edit with no declared repository, spec, or path reach. Strict mode closes exactly that absence hole: it refuses (exit 3) when `state.json`, `workBoundary`, `repositoryRoots`, `specTargets`, or `crossRepoPolicy` is missing or empty, and `--require-allowed-paths` additionally refuses (exit 4) when no mutation path surface is declared. `--require-allowed-paths` implies `--strict`, so a mutation check can never be weaker than the planning check. A present-but-malformed boundary is still exit 2 in every mode.

**Re-run the resolver for each candidate returned by a specialist** — every repository, spec, and changed path in the result — not only once before dispatch. A specialist's returned surface is the surface that actually needs classifying.

**Deriving the boundary.** Do not hand-author `workBoundary`. Freeze a Goal Contract, then run `bubbles/scripts/goal-contract.sh sync-boundary --session-file <session> --state-file <FEATURE_DIR>/state.json`, which copies the approved boundary verbatim so the enforced boundary and the approved one cannot drift. A planner may **narrow** a frozen boundary without approval and `sync-boundary` writes it. **Widening** is REFUSED (exit 3) and leaves `state.json` untouched — more reach is an expansion, and an expansion belongs in `goal-contract.sh revise --approval-note`, where it carries a recorded operator approval.

The resolver COMPOSES with — does not replace — `repo-binding-preflight.sh`: preflight verifies the repo↔agent binding, the resolver classifies each candidate change against the boundary. There is no `--force` / `--skip` / `--ignore`: widen the declared boundary through a contract revision, never skip the check.

### ⛔ Literal `mode:` Gate (MANDATORY — NON-NEGOTIABLE)

Before applying the classification contract, perform this literal substring check:

1. **Scan the raw user input for the exact token `mode:`**
2. If `mode:` is NOT present anywhere in the input → continue to the no-mode classification rules above. Preserve `CONTINUATION`, `VAGUE`, `CONTINUE`, and `FRAMEWORK` semantics; never classify the request as `STRUCTURED` or `TARGETLESS_MODE`.
3. If `mode:` IS present → classify as `STRUCTURED` only when a concrete spec, bug, or ops target is also present; otherwise classify as `TARGETLESS_MODE`.

**There are ZERO exceptions.** The following do NOT constitute structured input:
- Action verbs: "execute", "plan", "deliver", "implement", "run", "complete", "invoke"
- Spec references: "specs/099-108", "each recommendation", "all features"
- Phase language: "full planning workflow", "planning chain", "planning phases"
- Agent references: "invoke correct workflow/agent"
- Numbered lists of work items

**Known failure pattern:** `bubbles.workflow` receives NL input → skips this gate → self-selects a mode based on keyword matching → proceeds without `bubbles.super` resolution. This is the #1 observed violation and MUST be mechanically prevented by checking for `mode:` FIRST.

### Required Delegation Rules

### Runtime Depth Compatibility Contract

**Measured, not assumed (2026-08-06).** A subagent dispatched on this runtime has **no dispatch tool at all**. An agent invoked via `runSubagent` was asked to enumerate its own tools and to attempt a nested dispatch: its tool list contains no `runSubagent`, no `agent`, and no dispatcher under any name, and a capability search for one returned nothing. The second hop therefore cannot fail loudly, because there is nothing present to fail — which is exactly why it surfaced as silent parent-expansion. Downstream state carries `parent-expanded` **3,951 times** with `runtime lacks runSubagent` **325 times**, covering 13-33% of recorded invocations per repo.

- **A specialist may NEVER dispatch a specialist.** There is one dispatching agent per run: the active top-level runner. A phase owner that needs another phase owner returns `route_required` **upward** to that runner, which performs the next dispatch at depth 1. `route_required` is an upward return, never a lateral call.
- Do not assume a subagent can invoke another subagent. Some host runtimes expose `agent`/`runSubagent` only to the active top-level agent, and the VS Code default measured above is one of them.
- Workflow-running orchestrators MUST NOT invoke another workflow-running orchestrator as a subagent. The active top-level runner resolves the mode itself, verifies its grant in `workflowModeGrants`, invokes the required phase owners directly, and records `executionModel: direct-authorized-runner`.
- Envelope-only utility dispatch remains allowed: `bubbles.super` may return a resolution envelope and `bubbles.iterate` may return a picker-only work envelope because neither path launches a nested workflow.
- A domain orchestrator invoked as a phase owner performs only that phase and returns its result envelope. It may execute its granted workflow modes only when it owns the top-level runtime.
- If the active orchestrator itself lacks `agent`/`runSubagent`, return `blocked`; do not emulate owner work inline and do not claim a delegation happened. Emulating the work and recording it as a specialist run is the failure this contract exists to prevent, and the rate is now counted: `state-transition-guard.sh` emits `parentExpandedPhases` and `gate-hit-log.sh report` prints the expansion rate per repo.

### Frontmatter Dispatch Surface (G064 mechanical enforcement)

The rules above are body prose — the model may ignore them. The VS Code runtime obeys
**frontmatter**, so the same law is additionally expressed there and enforced by
`bubbles/scripts/workflow-runner-grants-lint.sh`. Field semantics (VS Code custom-agent
and subagent specifications, retrieved 2026-07-28):

| Field | Meaning | Bubbles usage |
|---|---|---|
| `handoffs:` | Button rendered **after the turn ends**; the user clicks it and **switches** to the target agent with a pre-filled prompt. Not subagent dispatch. | Retained. Handoffs between orchestrators are legitimate — control returns to the user, who starts a fresh top-level run. |
| `handoffs[].send` | Auto-submits the pre-filled prompt. | MUST remain unset. An auto-submitting handoff converts a human-gated transfer into an automated chain. |
| `agents:` | Subagent allowlist. Omitted = `*` (all). `[]` = none. | Omitted by default. No agent may name a **pure top-level runner**. |
| `disable-model-invocation:` | Prevents *this* agent from being dispatched as a subagent. | `true` on exactly the 6 pure top-level runners. |
| `user-invocable:` | Whether the agent appears in the chat dropdown. | Left at default `true` so every runner stays selectable. |

**Role split** — derived from `workflowModeGrants` (granted runners) intersected with
declared `owner:` phases in `workflows.yaml`:

| Role | Agents | `disable-model-invocation` |
|---|---|---|
| **Pure top-level runner** — never a declared phase owner | `goal`, `propagate`, `sprint`, `train`, `upkeep`, `workflow` | **`true` (required)** |
| **Dual-role** — granted runner AND declared phase owner | `bug`, `iterate`, `journey`, `releases`, `retro`, `stabilize` | **forbidden** — setting it would break their `owner:` phase dispatch |

**Precedence, and why `agents:` matters.** An explicit `agents:` listing **overrides**
`disable-model-invocation: true`. The two are therefore NOT independent layers: the
allowlist wins. The flag stays effective only because no agent names a pure runner in
`agents:` — which the lint enforces. Treat `disable-model-invocation:` as the default
posture and the allowlist prohibition as the control that preserves it.

**Depth assumption, now measured.** This model assumes the VS Code default in which subagents cannot
invoke further subagents, and a 2026-08-06 probe confirmed it directly: a dispatched subagent's tool
list contains no dispatcher at all. Enabling `chat.subagents.allowInvocationsFromSubagents` raises
the limit to depth 5, at which point nested runner dispatch becomes possible and G064
degrades from structurally impossible to convention-only. `bubbles doctor` surfaces the
setting as an advisory; it is operator-owned and cannot be enforced from the repo.

**Residual limitation (do not overstate the guarantee).** `agents:` is *name-based*. It
cannot distinguish "dispatch `bubbles.bug` to own the bugfix phase" from "dispatch
`bubbles.bug` to run the `bugfix-fastlane` mode" — both are `runSubagent(bubbles.bug)`.
For the six dual-role agents the allowlist therefore constrains **who** may be dispatched,
never **what they may be asked to do**. The body-prose contract above and the
`call_runSubagent` body check remain necessary for that half of the law.

See the [`bubbles-vscode-agent-constraints`](../../skills/bubbles-vscode-agent-constraints/SKILL.md)
skill for the full authoring rules and design-review checklist.

- When the request is `VAGUE`, invoke `bubbles.super` as a subagent and require a `## RESOLUTION-ENVELOPE` only.
- When the request is `CONTINUE` and no concrete workflow continuation can be recovered, invoke `bubbles.super` for a `RESOLUTION-ENVELOPE` and route to its `targetAgent`.
- When the request is `FRAMEWORK`, invoke `bubbles.super` as a subagent and require a `## FRAMEWORK-ENVELOPE` only.
- `bubbles.workflow` MUST NOT re-run a second natural-language inference pass after `bubbles.super` has resolved the request.
- `bubbles.workflow` MUST NOT recreate a local intent-to-mode keyword table or a local backlog-priority picker once these delegation paths are available.

### Envelope Consumption Rules

- `RESOLUTION-ENVELOPE` provides the resolved workflow mode, targets, and optional tags for Phase 0.
- `WORK-ENVELOPE` provides the resolved spec, scope, workflow mode, and work type for Phase 0.
- `FRAMEWORK-ENVELOPE` is terminal for framework operations, but it is still repository-sensitive: carry the exact `repositoryRoot`, `repositoryAlias`, and complete `repositoryResolution` unchanged; run `bubbles/scripts/repository-binding.sh validate-packet` and compare with the dispatch packet before reporting. Stale, substituted, malformed, cross-scope, public, or redacted framework envelopes refuse. Only after exact validation may the runner report the result and stop instead of entering the workflow phase engine.

### Continuation Preservation Rules

- Preserve `stochastic-quality-sweep`, `iterate`, and `full-delivery` when continuation context proves one of those modes is still active.
- Treat phrases such as `fix all found`, `fix everything found`, `address rest`, `fix the rest`, `resolve remaining findings`, or `handle remaining issues` as workflow continuation, not as permission to downshift into direct specialist execution.
- If continuation context narrows the remaining work to bug-only, docs-only, or validate-only work, route to the narrower workflow mode instead of echoing raw specialist commands.

### Delegated Intent Resolution Summary

Use this summary before Phase 0 when no explicit `mode:` is present:

1. `STRUCTURED` input (explicit `mode:` + concrete spec, bug, or ops targets) stays inside `bubbles.workflow`.
2. `VAGUE` input (no `mode:` keyword, OR natural-language intent even with spec targets) delegates to `bubbles.super` and consumes only a `RESOLUTION-ENVELOPE`.
3. `CONTINUE` input with no recoverable active workflow delegates resolution to `bubbles.super` and routes to the returned top-level runner.
4. `FRAMEWORK` input delegates to `bubbles.super` and consumes only a `FRAMEWORK-ENVELOPE`.
5. After `bubbles.super` resolves the request, `bubbles.workflow` MUST NOT run a second natural-language inference pass.
6. **The `STRUCTURED` classification requires the literal keyword `mode:` in the input.** Spec targets, feature names, or natural-language descriptions — even when they reference specific specs — are NOT sufficient for `STRUCTURED` classification without `mode:`.