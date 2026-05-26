---
name: bubbles-dod-validation
description: Validate Bubbles Definition of Done items against the Tier 1 (universal) and Tier 2 (role-specific) checks before transitioning a scope to Done. Use when closing out a scope, preparing scope handoff, or auditing whether a scope can legitimately be marked complete. Triggers include marking the final DoD checkbox, running pre-completion self-audit, or composing a "completed_owned" result envelope.
---

# Bubbles DoD Validation

## Goal
Confirm that every DoD checkbox represents real, validated work before marking a scope `Done` and before composing any result envelope claiming `completed_owned`.

## When to use
- About to flip a scope status to `Done`
- About to set `state.json.execution.scopeStatus = Done` for a scope
- About to emit a result envelope with `outcome: completed_owned`
- Reviewing a scope handoff packet during specialist completion chain

## Two-tier check model

### Tier 1 — Universal (every agent, every scope)
Run these in order, all must pass:

1. **All DoD checkboxes `[x]`** — zero unchecked items in the scope's DoD block. No exceptions for "deferred", "future work", or "follow-up" items.
2. **Per-DoD inline evidence** — each `[x]` has its own evidence block (≥10 lines raw output). See `bubbles-evidence-capture` skill.
3. **Test Plan ↔ DoD parity** — every row in the scope's Test Plan table maps to a test-related DoD item.
4. **Test file existence** — every file path mentioned in the Test Plan exists on disk (proven by `ls`, evidence in report.md).
5. **All required test categories executed** — per the Canonical Test Taxonomy, every category applicable to the scope's surface area ran and passed.
6. **No incomplete-work markers** — `grep -r "TODO\|FIXME\|HACK\|STUB\|unimplemented!" <changed-files>` returns zero.
7. **Zero warnings** — build, lint, test, runtime outputs contain zero warnings.
8. **Zero deferrals** — no issue encountered during the scope was skipped, postponed, worked-around, or "out-of-scoped" without an explicit owner packet routed to another agent.
9. **Bug reproduction (bug scopes only)** — reproduced BEFORE the fix and re-verified AFTER, both captured in report.md.

### Tier 2 — Role-specific
Look up the agent's role and load the matching profile from `agents/bubbles_shared/validation-profiles.md`. Examples:
- **implement** — addressedFindings + unresolvedFindings populated in envelope; round-trip verified for data mutations
- **test** — Gherkin scenario coverage 100%, live-stack authenticity (no `page.route()`/`msw`/`nock` in live categories)
- **validate** — scenario replay run, certification.* fields written, lockdown respected
- **audit** — Spot-Check Recommendations issued, finding ledger complete
- **docs** — managed docs current, generated docs regenerated
- **bug** — reproduction evidence, adversarial regression test (would fail if bug reintroduced)

## Mode ceiling pre-flight (Gate G073)
Before ANY non-spec file edit, check `state.json.workflowMode` → `bubbles/workflows.yaml` → `statusCeiling`:

| `statusCeiling` | Files you may edit |
|-----------------|--------------------|
| `done` | All files |
| `specs_hardened` | `specs/` only |
| `specs_scoped` | `specs/` only |
| `docs_updated` | `specs/` and `docs/` only |
| `validated` | `specs/` only |

If the ceiling forbids what your scope requires, return `route_required` with `reason: "mode ceiling does not permit <X>"`. Do not rationalize.

## Sequential spec completion
- A spec cannot be `done` until ALL its scopes are `Done` (Gate G024). Zero in-progress, zero blocked.
- A scope cannot be `Done` until ALL DoD items are `[x]` with inline evidence.
- A DoD item cannot be `[x]` until it was individually validated with raw evidence in this session.

## Authoritative modules
- `agents/bubbles_shared/validation-core.md` — Tier 1/Tier 2 framework
- `agents/bubbles_shared/validation-profiles.md` — role-specific Tier 2 tables
- `agents/bubbles_shared/completion-governance.md` — sequential completion contract
- `agents/bubbles_shared/quality-gates.md` — gate catalog G024–G095+
- `bubbles/scripts/state-transition-guard.sh` — mechanical enforcement
- `bubbles/scripts/artifact-lint.sh` — DoD shape lint
