# Recipe: End of Day

> *"Have a good one, boys."*

---

## The Situation

You're done for today. Need to save context for tomorrow.

## The Command

```
/bubbles.handoff
```

**What it does:**
1. Validates the current local actionable repository decision before collecting repository state
2. Captures what was done this session
3. Records what's remaining
4. Notes open questions and blockers
5. Saves a post-selection mirror to `.specify/memory/bubbles.session.json`

The repository-local snapshot is continuation context, not repository-selection authority. A local same-session handoff carries the exact session ID, canonical root, decision ID, and control revision from the current validated decision. A public or committed projection redacts the root and is deliberately non-actionable.

## Tomorrow

```
/bubbles.workflow  resume
```

In a distinct or otherwise unbound multi-root session, make the repository explicit:

```
/bubbles.workflow  resume repositoryRoot: <canonical-repository-root>
```

Bubbles does not pick a repository from CWD, editor focus, prompt location, recent files, or workspace order.

Or:

```
/bubbles.status
```

Then pick up wherever you left off.

If you just want the fast talking-head summary before diving back in:

```
/bubbles.recap
```
