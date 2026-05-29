# Cross-Spec Packet — Spec 061 → Spec 060

**Routed by:** `bubbles.implement` (spec 061, SCOPE-05, full-delivery convergence round 4)
**Routed at:** 2026-05-30 (artifact authoring time)
**Target owner:** `specs/060-bearer-auth-scope-claim` (PASETO bearer-auth scope catalog + default-grant migration)
**Source owner:** `specs/061-conversational-assistant` (SCOPE-05 wires the first transport for the assistant skills)
**Packet status:** `accepted` (artifact-only — accepted 2026-05-29 by spec 060 owner; see spec 060 report.md → "Accepted Cross-Spec Packets (Spec 061)")

---

## 1. Why this packet exists

Spec 061 §14 ("Authentication & Authorization") and design.md §14 ("Auth"
steps 1+2) require two PASETO scopes that DO NOT YET EXIST in the spec
060 catalog:

| Scope ID | Action | Who issues | Why |
|----------|--------|------------|-----|
| `assistant.skill.retrieval` | read | spec 060 token issuer | SCOPE-06 (retrieval Q&A) — guards `/api/search` + `/api/artifacts/:id` access from the assistant's retrieval tool handler. |
| `assistant.skill.weather` | read | spec 060 token issuer | SCOPE-07 (weather lookup) — guards the weather-tool egress through the existing weather provider. |

Notification write/execute scopes (for SCOPE-08) are NOT in this packet
because SCOPE-08 also depends on spec 054 (notification scheduler
substrate) and will route its own combined packet (spec 060 +
spec 054) when SCOPE-08 implementation begins. Splitting the read-scope
packet now keeps SCOPE-06 + SCOPE-07 unblocked without coupling them to
SCOPE-08's substrate fan-out.

**Cross-reference (spec 061 artifacts):**
- `specs/061-conversational-assistant/spec.md` §14 Authentication & Authorization
- `specs/061-conversational-assistant/design.md` §14 Auth (steps 1–2)
- `specs/061-conversational-assistant/scopes.md` SCOPE-05 DoD item: "Cross-spec packet to spec 060 owner routed and accepted"

---

## 2. Requested changes (verbatim contract for spec 060 owner)

### 2.1 Catalog additions

Add two entries to the spec 060 PASETO scope catalog (typical
location: `internal/auth/scopes/catalog.go` or the analogous registry
file in spec 060's implementation surface). The exact field shape
follows spec 060's existing catalog rows; the entries below specify
the **semantic contract** only.

```text
scope_id:     assistant.skill.retrieval
action:       read
description:  Allows the assistant retrieval tool handler to call
              the internal search + artifact-read surface on behalf
              of an authenticated user.
issued_to:    bot-shared tokens (default-granted per §2.2 below);
              future per-user assistant tokens may also carry it.
revoke_path:  standard spec 060 revoke surface (no spec 061 deviation).

scope_id:     assistant.skill.weather
action:       read
description:  Allows the assistant weather tool handler to call the
              configured weather provider on behalf of an
              authenticated user.
issued_to:    bot-shared tokens (default-granted per §2.2 below);
              future per-user assistant tokens may also carry it.
revoke_path:  standard spec 060 revoke surface (no spec 061 deviation).
```

### 2.2 Default-grant migration

Per design.md §14 step 2, the two scopes above MUST be **default-granted
to every existing bot-shared token** so that v1 assistant installs
"just work" the moment SCOPE-06/07 land. The migration shape:

```sql
-- Migration owned by spec 060; this packet specifies the semantic
-- contract only. Naming/numbering follows spec 060's migration
-- scheme.
--
-- 1. Insert the two catalog rows (if the catalog is table-backed).
-- 2. Add 'assistant.skill.retrieval' and 'assistant.skill.weather'
--    to every existing bot-shared token's scope_set.
-- 3. The migration MUST be idempotent (re-running it MUST be a
--    no-op) and MUST NOT touch per-user (non-bot-shared) tokens.
```

If spec 060 maintains its scope catalog as a Go constant rather than
a DB row set, replace step 1 with the corresponding code-side
catalog edit and bump any catalog-version constant. Step 2 (the
default-grant migration on existing tokens) is required either way.

### 2.3 Documentation

Update spec 060's `docs/` surface (typically `docs/Operations.md`
"PASETO Scopes" table or `docs/API.md` if scopes are documented
near the endpoints they guard) to list the two new scopes in the
same shape as existing rows.

---

## 3. Acceptance criteria for spec 060 owner

The spec 060 owner accepts this packet when ALL of the following are
true and recorded in spec 060's `report.md`:

- [ ] Both scopes (`assistant.skill.retrieval`, `assistant.skill.weather`) exist in the spec 060 PASETO scope catalog.
- [ ] The default-grant migration ran cleanly against the dev DB AND is idempotent (a second `migrate up` is a no-op).
- [ ] Spec 060's docs surface lists both scopes.
- [ ] An acceptance line is appended to spec 061's
      `specs/061-conversational-assistant/report.md` under
      `### SCOPE-05 — cross-spec packet 060 acceptance` referencing
      the spec 060 commit SHA + migration filename.

Until that acceptance line lands, spec 061 SCOPE-06 and SCOPE-07
**MUST NOT** be marked Done (they depend on the scopes existing).
SCOPE-05's own DoD item for this packet is satisfied by the **routed**
status of this file plus the cross-spec acceptance line landing in
spec 061's report.md when spec 060 owner accepts.

---

## 4. Non-goals (out of scope for this packet)

- Per-user assistant tokens (future work — v1 ships on bot-shared
  tokens only).
- Notification write/execute scopes for SCOPE-08 (separate packet,
  routed when SCOPE-08 begins).
- Email-read scope for the v2 email-summarize skill (not in v1).
- Changes to spec 060's claim verifier, transport, or signing
  algorithm — this packet adds catalog rows + a default-grant
  migration only.

---

## 5. Rollback plan (if spec 060 owner rejects)

If spec 060 owner rejects the catalog additions, SCOPE-06 + SCOPE-07
MUST NOT land. Spec 061 owner will then either:

1. Re-route a revised packet matching spec 060's preferred catalog
   shape (e.g. different scope naming, different default-grant
   policy), OR
2. Park SCOPE-06 + SCOPE-07 in `blocked` and drop them from the v1
   skill set per spec 061 §13 (v1 skill set is configurable).

This packet is fully reversible: removing the two catalog rows +
reverting the default-grant migration restores spec 060 to its
pre-packet state.

---

## 6. Receipt log

| Date | Actor | Action | Notes |
|------|-------|--------|-------|
| 2026-05-30 | `bubbles.implement` (spec 061, SCOPE-05) | Authored + routed packet | Status: `routed`. Awaiting spec 060 owner pickup. |
| 2026-05-29 | spec 060 owner (`bubbles.workflow` bugfix-fastlane) | Accepted artifact-only with contract translation | Status: `accepted`. Wire form translated to spec 060's `<surface>:<capability>` regex: `assistant:retrieval`, `assistant:weather`. Code-side surface registration (`"assistant"` added to `internal/auth/scopes.go::RegisteredScopeSurfaces`) deferred to spec 061's commits per spec 060's documented pattern ("additions to RegisteredScopeSurfaces MUST land in the same change set as the spec that introduces the new surface"). Default-grant semantics: spec 060 has NO grant DB; default-grant is operationalized via operator `auth enroll --scope assistant:retrieval --scope assistant:weather` when minting bot-shared tokens (spec 061 owner ships docs/runbook). "Missing scope = mismatch" invariant (spec 060 design.md §4) provides the default-deny base. See spec 060 `report.md` → "Accepted Cross-Spec Packets (Spec 061)" for full acceptance record. |
