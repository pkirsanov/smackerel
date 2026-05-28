# Cross-Spec Packet — spec 060 (PASETO Scope Catalog) — Write Scope

**Source:** spec 061 (Conversational Assistant), SCOPE-08 — Notifications skill (v1 #3)
**Target owner:** spec 060 (PASETO Scope Catalog) owner
**Routed by:** bubbles.implement during spec 061 SCOPE-08 substrate landing
**Date routed:** 2026-05-28
**Change shape:** Catalog addition + owner-only default grant.

---

## 1. Why this packet exists

spec 061 design.md §14 step 3 and scopes.md SCOPE-08 cross-spec dependency table call for adding a new PASETO scope to spec 060's catalog so the `notification_execute` write-side tool can be authorized per request:

> spec 060 PASETO scope catalog: add `assistant.skill.notifications.write` (write) | SCOPE-08 (notifications scenario dispatch) | Catalog-addition packet + owner-only grant migration SQL (design §14 step 3)

This is the third assistant-side scope. The two read scopes (`assistant.skill.retrieval`, `assistant.skill.weather`) were routed during SCOPE-06/SCOPE-07 implementation. This packet is the write counterpart — and per design.md §14 step 3 it MUST NOT be default-granted to all users (notifications can register externally-visible side-effects, so the grant requires explicit owner approval).

## 2. Requested change

### 2.1 Add catalog entry

In spec 060's scope catalog (whichever artifact ratifies the canonical list):

| Scope | Kind | Default-grant policy | Owner | Notes |
|-------|------|----------------------|-------|-------|
| `assistant.skill.notifications.write` | write | **owner-only** (NOT default-granted) | spec 061 | Authorizes `notification_execute` tool to register a reminder via the spec 054 scheduler |

### 2.2 Ship migration SQL

In whatever migration owner spec 060 maintains for the scope catalog, add (final ordering decided by spec 060 owner — sketch below):

```sql
-- spec 060 owner files this migration when the catalog packet lands.
-- The grant rows below intentionally cover only the owner user; the
-- migration MUST NOT default-grant this scope to every user.

INSERT INTO paseto_scope_catalog (scope, kind, description)
VALUES (
    'assistant.skill.notifications.write',
    'write',
    'Authorizes spec 061 notification_execute tool to register reminders via the spec 054 scheduler.'
)
ON CONFLICT (scope) DO NOTHING;

-- Owner-only grant (spec 060 owner SHOULD ship a parameterized form;
-- the literal user ID below is illustrative only.)
-- INSERT INTO paseto_scope_grants (user_id, scope, granted_at)
-- VALUES (
--     '<owner-user-id>',
--     'assistant.skill.notifications.write',
--     NOW()
-- )
-- ON CONFLICT (user_id, scope) DO NOTHING;
```

The exact table names + final column shapes are spec 060's call; the spec 061 implementer will follow whatever spec 060 ratifies.

## 3. Backward-compat guarantees the spec 061 implementer relies on

- Adding a new catalog row MUST NOT change behavior for users who do not hold the scope. Their existing assistant turns continue to work; `notification_execute` simply fails authorization with a canonical error.
- spec 060's authorization check MUST treat the missing-grant case as "scope not held → 403", NOT as "scope unknown → 500".
- The new scope MUST NOT be retroactively granted to existing users by default. Notifications are an explicit-opt-in surface per design.md §14 step 3.

## 4. Tests the spec 060 owner is asked to add

- Catalog test: `assistant.skill.notifications.write` appears in the canonical catalog enumeration with `kind=write`, default-grant=false.
- Authorization test: a token without the scope receives 403 from `notification_execute`-bound routes.
- Authorization test: a token with the scope receives 200 + the normal response.
- Owner-grant migration test: applying the migration in a clean DB grants the scope to the owner-user-id row and nobody else.

## 5. Acceptance criteria for this packet (closed when ALL met)

- [ ] spec 060 owner reviews and approves the catalog addition.
- [ ] spec 060 ships the catalog row + tests + owner-only grant migration.
- [ ] spec 061's SCOPE-08 wiring can call into the spec 060 authorization layer to check this scope before invoking `notification_execute`.
- [ ] spec 061 BS-004 e2e can authenticate as the owner user and successfully schedule a reminder.

## 6. Current state of the spec 061 side (so spec 060 owner knows what is waiting)

- The notification tool handlers already exist and are gated behind `assistant.skills.notifications.enabled`. The spec 060 authorization check sits one layer up — the router/adapter MUST check the PASETO scope before dispatching `notification_execute`.
- `internal/agent/tools/notification/services.go` + `internal/agent/tools/notification/pg_confirm_store.go` are production-ready (verified by `TestMachinePg*` + `TestPgConfirmStore*` integration tests).
- The scheduler binding is also pending — see `packet-054-scheduler.md` in this folder. Both packets must land before BS-004 e2e can run end-to-end.

## 7. What spec 061 will NOT change while this packet is open

- spec 061 will NOT default-grant the new scope. Operators who opt-in must grant the scope manually until spec 060 ships the catalog row + migration.
- spec 061 will NOT bypass the spec 060 authorization layer for `notification_execute`. The temporary `notificationSchedulerStub` ensures any unauthorized attempt fails loud at the scheduler boundary in addition to the authorization boundary.

## 8. Open questions for the spec 060 owner

1. Does spec 060 prefer a different scope name (e.g., `notifications.write`, `assistant.notifications`)? spec 061 will follow the ratified name and update its router check accordingly.
2. Does spec 060 maintain a separate "default-grant" registry, or is the absence of a grant row the implicit "not default-granted" signal?
3. Are read scopes for the assistant skills (`assistant.skill.retrieval`, `assistant.skill.weather`) already in the catalog from prior SCOPE-06/SCOPE-07 packets, or are those still pending?

---

**Routing status:** packet authored 2026-05-28 by `bubbles.implement` during spec 061 SCOPE-08 substrate landing. spec 060 owner ownership transfer pending. No spec 060 artifacts modified by this packet — it is a routed request, not an applied change.
