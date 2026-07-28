---
name: bubbles-vscode-agent-constraints
description: Design agents that fit the VS Code Copilot Chat runtime instead of fighting it. Use when authoring or editing any `*.agent.md`; when adding `tools:`, `agents:`, `handoffs:`, `model:`, `user-invocable:`, or `disable-model-invocation:` frontmatter; when designing an orchestrator that dispatches specialists; when a design implies agent-calls-agent-calls-agent chaining; when a subagent dispatch silently no-ops or an agent cannot be selected in the dropdown; or when writing a governance rule about delegation that the runtime cannot actually enforce. Prevents multi-level-subagent designs, prose-only dispatch laws, and frontmatter/body contradictions.
---

# VS Code Agent Runtime Constraints

## Mental Model

An `.agent.md` file is **two contracts, not one**:

| Layer | Read by | Enforceable? |
|---|---|---|
| **Frontmatter** | the VS Code runtime | **Yes** — the runtime obeys it |
| **Body prose** | the model | **No** — the model may ignore it |

A delegation rule written only in the body is a *convention*. The same rule expressed
in frontmatter is an *invariant*. When a governance requirement can be expressed in
frontmatter, expressing it in prose instead is a design defect — the framework's own
thesis (mechanical verdict over prose claim) applies to its own agent files.

> Platform behavior below reflects the VS Code custom-agent and subagent
> specifications as of **2026-07-28**. Re-verify before relying on a detail;
> the `agents:` restriction is still marked **Experimental** and `infer:` is
> already deprecated.

## Constraint 1 — Dispatch Is One Level Deep

By default **subagents cannot invoke further subagents**. The only shape available is:

```
user → main agent          (depth 0)
     → subagent            (depth 1)
     → [depth 2 refused]
```

`chat.subagents.allowInvocationsFromSubagents` (default `false`) lifts this to a maximum
nesting depth of 5, but it is an **operator-owned editor setting** — never assume it.

**Therefore:** an orchestrator must resolve its own plan and invoke each worker
**directly**. Never design `orchestrator → orchestrator → worker`. The middle agent would
land at depth 1 with no ability to dispatch, and its workers would silently never run.

| ❌ Broken design | ✅ Correct design |
|---|---|
| Runner A dispatches Runner B, which dispatches phase owners | Runner A resolves the mode itself and dispatches each phase owner directly |
| "Delegate the whole sub-workflow to the specialist orchestrator" | "Execute the sub-workflow's phases in this runtime, invoking each owner" |
| Recursive fan-out that assumes arbitrary depth | Flat fan-out from one coordinator; iterate in the coordinator's own loop |

## Constraint 2 — Frontmatter Field Semantics

These four fields are routinely confused. They do different things:

| Field | What it actually does | Default |
|---|---|---|
| `handoffs:` | Renders a **button after the turn ends**. The user clicks it and **switches** to the target agent with a pre-filled prompt. **Not** subagent dispatch — the current turn is already over. | none |
| `handoffs[].send` | Auto-submits the pre-filled prompt instead of waiting for the user to press enter. | `false` |
| `agents:` | **Subagent allowlist.** Names which agents this agent may dispatch. `*` = all, `[]` = none. | `*` |
| `disable-model-invocation:` | Prevents *this* agent from being invoked **as a subagent** by others. | `false` |
| `user-invocable:` | Whether the agent appears in the chat agents dropdown. `false` = subagent-only. | `true` |

**A handoff between two orchestrators is legitimate** — control returns to the user, who
then starts a fresh top-level run. That is the correct way to chain orchestrators under
Constraint 1. A *subagent dispatch* between two orchestrators is not.

## Constraint 3 — `agents:` Overrides `disable-model-invocation:`

Explicitly listing an agent in another agent's `agents:` array **overrides** that agent's
`disable-model-invocation: true`.

**Therefore:** the two fields are **not independent layers**. Do not describe them as
"defense in depth" — the allowlist wins. Treat `disable-model-invocation:` as a *default
posture marker*, and the `agents:` allowlist as the actual control. Lint the exact
intended set rather than assuming one field backstops the other.

## Constraint 4 — Dual-Role Agents Cannot Be Invocation-Blocked

An agent that is **both** a top-level runner **and** a worker other runners dispatch
(a "phase owner") must remain subagent-invocable. Setting `disable-model-invocation: true`
on it breaks every dispatch that depends on it.

Before applying the flag, compute the role split:

```bash
# agents that are BOTH a declared phase owner AND a granted workflow runner
comm -12 \
  <(grep -E '^\s+owner:' bubbles/workflows.yaml | sed 's/.*owner:[[:space:]]*//' \
     | grep '^bubbles\.' | sort -u) \
  <(sed -n '/workflowModeGrants:/,/^[a-zA-Z]/p' bubbles/agent-capabilities.yaml \
     | grep -oE 'bubbles\.[a-z-]+' | sort -u)
```

Apply `disable-model-invocation: true` **only** to agents absent from that intersection.

## Constraint 5 — Frontmatter `tools:` Gates Runtime Capability

A body-level "TOOL ALLOWLIST" block is governance documentation. The runtime reads
`tools:`. Two consequences:

- An agent whose body says it dispatches subagents **must** include the `agent` tool
  alias in frontmatter `tools:`, or `runSubagent` is simply unavailable at runtime.
- Tools that are unavailable are **silently ignored**, not reported. A capability that
  never fires may be a missing frontmatter entry, not a model failure.

Keep the body allowlist and frontmatter `tools:` in agreement; when they disagree,
frontmatter is what happens.

## Constraint 6 — Subagents Return Summaries, Not State

A subagent runs with **isolated context** and returns a summary to the parent. It does not
share the parent's conversation, and the parent does not observe its intermediate steps.

**Therefore:** design the dispatch prompt to be self-contained, and design the return
value as a structured envelope the parent can act on. Never assume a subagent inherited
context the parent never passed, and never rely on a subagent mutating parent state
in-place.

## Constraint 7 — Model Selection Has a Cost Ceiling

Model resolution order is: explicit `runSubagent` parameter → the agent's own `model:`
frontmatter → the parent conversation's model. **A subagent cannot exceed the cost tier of
the main model**; a more expensive request silently falls back.

**Therefore:** never design a cheap orchestrator that "escalates" a subagent to a more
capable model. Escalation must happen at the top level.

## Pitfall → Correct Form

| ❌ Pitfall | ✅ Correct form |
|---|---|
| Orchestrator dispatches another orchestrator | Orchestrator resolves the mode and dispatches phase owners directly |
| Design assumes arbitrary nesting depth | Flat dispatch; loop in the coordinator |
| `handoffs:` treated as automated chaining | Handoff = user-clicked switch; keep `send:` unset |
| `handoffs[].send: true` on a governance-critical transition | Leave `send` unset so a human gates the transition |
| Delegation law written only in body prose | Express it in `agents:`; keep prose for what the field cannot encode |
| `disable-model-invocation:` assumed absolute | It is overridable by `agents:`; lint the exact intended set |
| `disable-model-invocation: true` on a dual-role agent | Apply only to agents that are never dispatched as workers |
| Body claims subagent dispatch, frontmatter omits `agent` tool | Add the `agent` alias to frontmatter `tools:` |
| Subagent expected to see parent conversation | Pass everything needed in the dispatch prompt |
| Subagent escalated to a pricier model | Escalate at the top level; subagents are capped by the parent tier |
| `infer:` used to control visibility | `user-invocable:` and `disable-model-invocation:` |

## Authoring Rules

1. **Express delegation limits in frontmatter first.** Prose is the fallback for what
   `agents:` cannot encode — e.g. *why* an agent is dispatched, which the name-based
   allowlist cannot distinguish.
2. **State residual limits honestly.** `agents:` constrains *who* may be dispatched, never
   *what they are asked to do*. If a rule depends on intent, say so rather than implying
   the field closes the gap.
3. **Never contradict frontmatter in the body.** If the body forbids something the
   frontmatter permits, the frontmatter is what the runtime does.
4. **Generate frontmatter from the registry** where a framework already has one, so an
   ownership change re-derives the dispatch topology instead of requiring every agent
   file to be hand-edited in step.
5. **Assume unknown keys are ignored.** Older VS Code builds skip frontmatter they do not
   recognize, so adding these fields is backward-safe — but never *depend* on a new field
   for a safety property without a non-frontmatter backstop.
6. **Re-verify the platform contract before relying on a detail.** Record the retrieved-on
   date next to any behavior claim.

## Design Review Checklist

Before shipping an agent definition, confirm:

- [ ] No path in the design requires depth ≥ 2 subagent dispatch.
- [ ] Every agent the body says it dispatches appears in frontmatter `agents:`.
- [ ] `agent` tool alias present in `tools:` for any dispatching agent.
- [ ] No agent lists a pure top-level runner in its `agents:` allowlist.
- [ ] `disable-model-invocation:` set only on agents never dispatched as workers.
- [ ] No `handoffs[].send: true` on a transition that should stay human-gated.
- [ ] Body prose and frontmatter agree on tools and dispatch targets.
- [ ] Dispatch prompts are self-contained; return values are structured envelopes.
- [ ] No design depends on a subagent using a costlier model than its parent.

## Verification

```bash
# agents declaring a dispatch target in frontmatter handoffs
grep -l 'handoffs:' agents/*.agent.md

# agents that can dispatch but never declare an allowlist
grep -L '^agents:' agents/*.agent.md

# auto-submitting handoffs (should normally be empty)
grep -rn 'send:[[:space:]]*true' agents/*.agent.md

# dispatching agents missing the runtime tool alias
grep -l 'runSubagent' agents/*.agent.md \
  | xargs grep -L "tools:.*\bagent\b"

# is recursive nesting enabled in this environment?
grep -rn 'allowInvocationsFromSubagents' ~/.config/Code/User/settings.json 2>/dev/null \
  || echo 'not set — depth-1 default applies'
```

## See Also

- [`bubbles-agents.instructions.md`](../../instructions/bubbles-agents.instructions.md) — agent file format and required sections
- [`workflow-delegation-core.md`](../../agents/bubbles_shared/workflow-delegation-core.md) — who may run a workflow mode and how dispatch is recorded
- [`bubbles-workflow-execution-loops`](../bubbles-workflow-execution-loops/SKILL.md) — per-round dispatch-and-wait execution policy
- [`bubbles-result-envelope`](../bubbles-result-envelope/SKILL.md) — the structured return value a dispatched agent must produce
- VS Code docs: *Custom agents* and *Subagents* — the authoritative platform contract
