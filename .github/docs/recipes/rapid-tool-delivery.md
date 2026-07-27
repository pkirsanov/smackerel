# Recipe: Rapid Tool Delivery (Fast Lane)

> *"In and out, boys. But we still do it right."* — A quick job, done properly.

---

## The Situation

You have a **single, low-risk, build-free tool increment** to ship — a small self-contained utility, script, or helper — and the full planning chain would be pure ceremony. You want fewer phases **without** dropping the integrity contract or shipping something unproven.

`rapid-tool-delivery` is the risk-proportional fast lane for exactly this. It runs a short phase chain but keeps **every universal gate** (anti-fabrication, per-DoD-item raw evidence, tests-for-all-scenarios, implementation-reality scan). It relaxes ONLY the heavyweight mandatory planning chain — never a quality floor.

## Invoke It

**Natural language** (routes through `bubbles.goal`):

```
/bubbles.goal  fast lane this tool
/bubbles.goal  ship a quick tool
/bubbles.goal  small build-free tool
/bubbles.goal  rapid tool delivery
```

**Explicit v6 form:**

```
/bubbles.workflow  implement action:rapid-tool-delivery target:tool for <your tool>
```

**What happens:**

```
select → implement → test → validate → docs → finalize
```

1. Selects the tool increment to build
2. Implements it fully (no stubs, no TODOs, no deferrals)
3. Tests every scenario for the increment
4. `bubbles.validate` derives the achieved assurance from the evidence
5. Publishes docs
6. Finalizes

## The Risk Escalation Guarantee

The fast lane is **low-risk-only**, and that boundary is mechanical, not a promise. Eligibility is resolved by `risk-tier-resolve.sh`. Any high-risk trigger fail-closed-escalates the work to `full-delivery`:

- auth / authorization
- payments
- secrets
- PII
- database migration
- deploy
- prod
- host-singleton
- cross-product

Because escalation is fail-closed (unknown risk resolves *up*, to `full-delivery`), the fast lane **can never shed gates on risky work**. That single resolver is what makes "go faster" safe.

## The Achieved-Assurance Outcome

When the fast lane completes cleanly, its achievement certifies as **`fast` assurance → terminal status `delivered_fast`**. That is the complete integrity chain — implementation complete, full test coverage, all tests passing — **minus** the independent audit (`missingForFull = [independent-audit]`). `delivered_fast` is terminal **only** for `rapid-tool-delivery` (its declared `terminalAlias`).

Assurance is **derived, requestable, and never declarable**:

- `bubbles.validate` DERIVES the level from evidence via `assurance-derive.sh` (fail-closed — any incompleteness derives *down*).
- You can **request** a level; you cannot **self-assign** one. It is not a quality slider you pick.
- The fast lane reaches `done` **only if full assurance is actually achieved** (i.e. an independent audit is present) — otherwise it stops at `delivered_fast`.
- If verification is incomplete or failing, the derived level is `prototype` (→ `delivered_prototype`), which is **never deployable** and never terminal under normal delivery modes.

## Rules

- The fast lane keeps the **full delivery integrity contract** — it relaxes only the heavyweight planning chain, never anti-fabrication, evidence, or test-substance gates.
- It is for **one** low-risk, build-free tool increment. The plan must stay small (the low-risk scope budget is enforced).
- Any high-risk trigger **must** escalate to `full-delivery` — there is no override.
- `delivered_fast` is honest terminal state, not a lowered bar: it means "verified, but no independent audit."
- `prototype` / `delivered_prototype` **never ships**.

## See Also

- [Workflow Modes → Delivery Strategy & Achieved Assurance](../guides/WORKFLOW_MODES.md#delivery-strategy--achieved-assurance)
- [Workflow Modes → rapid-tool-delivery](../guides/WORKFLOW_MODES.md#rapid-tool-delivery)
- [Fix a Bug](fix-a-bug.md) — the bug-focused fast lane (`bugfix-fastlane`)
