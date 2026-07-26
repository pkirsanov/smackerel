# Recipe: Resume Work

> *"Way she goes, boys."*

---

## The Situation

You were working on something in a previous session and need to pick up where you left off.

## The Command

```
# Simplest — just say "continue":
/bubbles.workflow  continue

# Or resume explicitly:
/bubbles.workflow  resume

# In a new or unbound multi-root session, name the work repository:
/bubbles.workflow  resume repositoryRoot: <canonical-repository-root>
```

**What happens:**
1. Repository preflight resolves an explicit `repositoryRoot`, the current host session's durable work boundary, or the sole eligible root in a true single-repository workspace
2. Any inherited continuation packet is captured once and validated against the exact current session/root/decision/revision before repository-local reads
3. `continue` tries to resume the active workflow context first when a valid continuation envelope, workflow run-state record, or non-terminal spec state is available under that repository
4. If no active workflow continuation can be recovered safely, workflow falls back to `iterate`, which discovers and ranks work only under `<resolved-repository-root>/specs`
5. `resume` reads `state.json` only from the committed repository and continues from exactly where it stopped

That matters after workflows like `stochastic-quality-sweep`: follow-ups such as `fix all found` or `address the rest` should keep the work inside the active workflow mode instead of downshifting into raw specialist execution.

## Alternative: Check Status First

```
/bubbles.status
```

See what's in progress, what's done, what's remaining. Then:

```
/bubbles.workflow  042-catalog-assistant mode: full-delivery
```

If the next executable action is unclear, feed the recap/status recommendation back into `/bubbles.workflow`; it can consume continuation packets and keep orchestration intact.

If the previous run ended with remaining routed work, you can also say things like:

```
/bubbles.workflow  fix all found
/bubbles.workflow  address the rest
```

Those are continuation-shaped requests. Workflow now resolves them against active continuation context before it ever falls back to generic work-picking.

## Session Boundary And Safe Packets

A successful targeted command establishes or switches the durable work boundary for the current interactive host session. Targetless `continue`, `fix all found`, status, recap, and handoff operations may continue that same decision after validation.

A distinct interactive session does not inherit an old boundary just because `.specify/memory/bubbles.session.json` still exists. In an unbound multi-root session, provide `repositoryRoot: <canonical-repository-root>`; Bubbles refuses rather than choosing from CWD, prompt source, active editor, recent files, or workspace order.

Local continuation packets contain the exact session ID, canonical root, decision ID, and control revision and are actionable only in the same validated session. Packet-file consumers validate and read those immutable captured bytes once. Public or committed projections use `<redacted-local-root>` with `pathVisibility: redacted` and `actionable: false`; they are useful as summaries but cannot resume or dispatch work.

## If the Previous Session Saved a Handoff

Check `.specify/memory/bubbles.session.json` after repository preflight. The handoff agent saves a repository-local mirror there, but the mirror does not select the repository.

```
/bubbles.status  show handoff for 042
```

## Tip

End every session with:

```
/bubbles.handoff
```

This saves a snapshot of what was done, what's next, and any open questions — making the next resume seamless.
This also records the recommended workflow continuation instead of leaving you to reconstruct the next command manually.
