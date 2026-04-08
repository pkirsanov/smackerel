## Workflow Delegation Core

Use this module to keep routing responsibilities separated across the Bubbles front door.

### Delegation Boundaries

- `bubbles.super` is the ONLY natural-language dispatcher. It owns plain-English translation into workflow parameters, exact slash-command guidance, and framework-operation routing. `bubbles.workflow` and `bubbles.iterate` MUST NOT maintain duplicate intent-to-mode mapping tables.
- `bubbles.iterate` is the ONLY highest-priority work picker. It owns backlog/work discovery, next-action selection, and `WORK-ENVELOPE` output. `bubbles.workflow` MUST NOT maintain its own work-priority heuristic when iterate is available.
- `bubbles.workflow` owns execution only. It may parse structured `mode:` and spec targets, recover continuation packets, consume `RESOLUTION-ENVELOPE` or `WORK-ENVELOPE` outputs, and then run the selected workflow phases. It must delegate vague routing to `bubbles.super` and generic work discovery to `bubbles.iterate`.

### Input Classification Contract

Classify incoming workflow requests into exactly one of these buckets before Phase 0:

1. `STRUCTURED` — explicit `mode:` and/or concrete spec targets are present. `bubbles.workflow` may continue directly.
2. `CONTINUATION` — continuation envelopes, run-state, recap/status/handoff packets, or explicit continuation language tied to active workflow state are present. Preserve the active workflow mode when possible.
3. `VAGUE` — plain-English goal with no structured mode/target. Delegate to `bubbles.super` and consume a `RESOLUTION-ENVELOPE`.
4. `CONTINUE` — generic keep-going language with no recoverable active workflow target. Delegate to `bubbles.iterate` and consume a `WORK-ENVELOPE`.
5. `FRAMEWORK` — framework operations such as doctor, hooks, upgrade, status, metrics, lessons, gates, or install. Delegate to `bubbles.super` and consume a `FRAMEWORK-ENVELOPE`.

### Required Delegation Rules

- When the request is `VAGUE`, invoke `bubbles.super` as a subagent and require a `## RESOLUTION-ENVELOPE` only.
- When the request is `CONTINUE` and no concrete workflow continuation can be recovered, invoke `bubbles.iterate` as a subagent and require a `## WORK-ENVELOPE` only.
- When the request is `FRAMEWORK`, invoke `bubbles.super` as a subagent and require a `## FRAMEWORK-ENVELOPE` only.
- `bubbles.workflow` MUST NOT re-run a second natural-language inference pass after `bubbles.super` or `bubbles.iterate` has already resolved the request.
- `bubbles.workflow` MUST NOT recreate a local intent-to-mode keyword table or a local backlog-priority picker once these delegation paths are available.

### Envelope Consumption Rules

- `RESOLUTION-ENVELOPE` provides the resolved workflow mode, targets, and optional tags for Phase 0.
- `WORK-ENVELOPE` provides the resolved spec, scope, workflow mode, and work type for Phase 0.
- `FRAMEWORK-ENVELOPE` is terminal for framework operations; report the result and stop instead of entering the workflow phase engine.

### Continuation Preservation Rules

- Preserve `stochastic-quality-sweep`, `iterate`, and `delivery-lockdown` when continuation context proves one of those modes is still active.
- Treat phrases such as `fix all found`, `fix everything found`, `address rest`, `fix the rest`, `resolve remaining findings`, or `handle remaining issues` as workflow continuation, not as permission to downshift into direct specialist execution.
- If continuation context narrows the remaining work to bug-only, docs-only, or validate-only work, route to the narrower workflow mode instead of echoing raw specialist commands.

### Delegated Intent Resolution Summary

Use this summary before Phase 0 when no explicit `mode:` is present:

1. `STRUCTURED` input stays inside `bubbles.workflow`.
2. `VAGUE` input delegates to `bubbles.super` and consumes only a `RESOLUTION-ENVELOPE`.
3. `CONTINUE` input with no recoverable active workflow delegates to `bubbles.iterate` and consumes only a `WORK-ENVELOPE`.
4. `FRAMEWORK` input delegates to `bubbles.super` and consumes only a `FRAMEWORK-ENVELOPE`.
5. After `bubbles.super` or `bubbles.iterate` resolves the request, `bubbles.workflow` MUST NOT run a second natural-language inference pass.