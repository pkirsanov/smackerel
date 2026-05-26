# Spec Review: 055 Notification Source ntfy Adapter

**Agent:** bubbles.spec-review
**Reviewed At:** 2026-05-26T07:19:13Z
**Scope:** post-docs/governance cleanup G088 recertification
**Trust Level:** CURRENT
**Behavioral Verdict:** PASS
**Artifact Verdict:** CURRENT_FOR_BLOCKED_CERTIFICATION

## Summary

Spec-review recertification checked the current dirty parent planning truth in `spec.md`, `design.md`, and `scopes.md` after the docs/governance cleanup. The active planning truth is now coherent with the implemented ntfy adapter behavior and the cleaned artifact state: the feature is behaviorally complete, Scopes 1-9 are Done in the planning artifact, and parent state remains blocked only for validate-owned final certification metadata and promotion reruns.

No runtime source, config, deploy, docs, scripts, web, ML, Docker, or proposal artifacts were modified by this recertification pass.

## Artifact Assessment

| Artifact | Assessment | Finding |
|----------|------------|---------|
| `spec.md` | CURRENT | Active Status and Current Capability Map now describe the implemented adapter, source-neutral boundary, and final certification-only blocker. |
| `design.md` | CURRENT | Active Design Brief says the ntfy adapter, config parsing, runtime startup, stream/webhook handling, health, DLQ/replay store, APIs, source views, and focused coverage exist. The old missing-adapter text is retained only inside an explicitly superseded hidden duplicate-history block. |
| `scopes.md` | CURRENT | Planning Status, scope inventory, scope statuses, DoD checkboxes, and evidence refs agree that Scopes 1-9 are Done while parent top-level certification remains validate-owned and blocked. |
| `state.json` | CURRENT_FOR_BLOCKED_STATE | Parent remains blocked for final validation certification; child BUG-CHAOS-20260524-001 is done; this spec-review records the current recertification without editing certification-owned fields. |

## Implementation Reality

Verified implementation surfaces remain aligned with the active contract:

| Surface | Current file(s) | Verification |
|---------|-----------------|--------------|
| ntfy adapter package | `internal/notification/source/ntfy/` | Config validation, event parsing, mapper, runtime startup from `NTFY_SOURCES_JSON`, stream/webhook handling, topic health, dead-letter/replay store, redaction-state decode handling, and no-output-coupling tests are present. |
| ntfy API routes | `internal/api/notifications_ntfy.go`, `internal/api/router.go` | Authenticated source detail, webhook, reconnect, dead-letter list/detail, and replay handlers are mounted under notification source routes and preserve the source/output boundary. |
| ntfy schema | `internal/db/migrations/038_notification_ntfy_source_adapter.sql` | Adapter-owned subscription state, dead-letter, and replay attempt persistence matches the planning contract. |
| BUG-CHAOS replay fix | `internal/notification/source/ntfy/store.go` | `ReplayDeadLetter` locks the dead-letter row, returns the existing replay attempt for already replayed records, and submits through `SourceEventSink` only for the first eligible replay. |
| Tests | `internal/api/notifications_ntfy_test.go`, `internal/notification/source/ntfy/*_test.go`, `tests/e2e/notification_ntfy_source_api_test.go`, `tests/e2e/notification_ntfy_source_ui_test.go`, `tests/stress/notification_ntfy_source_stress_test.go` | Focused coverage exists for config, runtime startup, webhook route, mapping, health, DLQ/replay, provenance, redaction, source/output boundary, UI, E2E API, and stress behavior. |

## Trust Classification

**CURRENT**: The active parent `spec.md`, `design.md`, and `scopes.md` can be used as current planning truth for the implemented ntfy source adapter. Spec-review finds no behavior drift and no remaining active stale planning-truth prose in those three artifacts after the cleanup.

The parent state still intentionally remains `blocked` because validate owns final promotion: top-level `certifiedAt`, certification metadata, blocker clearance, and final done-mode guard reruns are not spec-review-owned.

## Next Route

```text
agent: bubbles.validate
mode: final-certification
spec: specs/055-notification-source-ntfy-adapter
reason: spec-review:CURRENT-after-docs-governance-cleanup
actions:
  - Rerun final parent promotion checks from the current blocked state.
  - Own any certification.blockers cleanup, certifiedAt update, certification status promotion, and final state-transition evidence.
  - Do not recreate .github/bubbles-project/proposals/*.md.
  - Do not treat spec-review as authorization to bypass G088 if the done-state guard still reports dirty certified planning truth.
```