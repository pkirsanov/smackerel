# BUG-028-004 — Routing for Deferred Findings

These findings from stochastic-quality-sweep round 9 are out of scope for this
bug because they touch planning, schema-contract, or policy surfaces that are
NOT owned by `bubbles.implement`. They are routed to the artifact owners.

## F5 — pg_trgm migration for fuzzy search on list_items.content

- **Class:** schema / index / migration
- **Owner chain:** `bubbles.analyst` (does fuzzy search belong in the spec?) →
  `bubbles.plan` (scope a migration adding `pg_trgm` + GIN index) →
  `bubbles.design` (extension enablement, index strategy, query rewrite) →
  `bubbles.implement` (apply via `internal/db/migrations/`).
- **Rationale:** Adding a Postgres extension changes the migration baseline,
  affects backup/restore drill (G115), and changes query planning. This is a
  multi-artifact change, not a stabilize fix.

## F6 — Pagination contract on `GET /api/lists`

- **Class:** API contract
- **Owner chain:** `bubbles.analyst` (decide cursor vs offset, total count
  semantics, `next_offset` vs `next_cursor` field, max page size policy) →
  `bubbles.design` (response envelope shape) → `bubbles.plan` (scope) →
  `bubbles.implement`.
- **Rationale:** The current handler caps `limit ≤ 100` and returns
  `{lists, count}` with no pagination envelope. Changing to a stable cursor
  contract is a breaking API change that must flow through spec/design and
  trigger consumer impact sweep (web UI, telegram bot, future MCP clients).

## F8 — Auth scope decision on list mutation endpoints

- **Class:** authn/authz policy
- **Owner chain:** `bubbles.analyst` (which actor scopes can create / mutate /
  archive lists; does telegram bot share user scope; can a service token
  bypass) → `bubbles.design` (token claim shape, middleware wiring) →
  `bubbles.plan` → `bubbles.implement`.
- **Rationale:** Spec 028 didn't enumerate per-endpoint auth scopes; current
  handlers rely on whatever the upstream chi router decided. Picking the
  scope model is a product/policy decision, not a stabilize edit.

## F10 — FK contract: `list_items.list_id` ON DELETE behavior

- **Class:** schema / migration
- **Owner chain:** `bubbles.plan` (decide CASCADE vs RESTRICT vs SET NULL;
  affects archive vs hard-delete semantics) → `bubbles.design` (migration
  + rollback plan) → `bubbles.implement`.
- **Rationale:** Changing FK action requires a migration that locks the
  `list_items` table, and it changes the meaning of `ArchiveList` /
  `DeleteList` (if added). Cannot be patched in stabilize.

## How to action this routing

The next planning-cycle agent picking up spec 028 follow-up work should open
this file, file a `bubbles.analyst` invocation per finding (F5–F10 may be
batched into one analyst session since they all touch the same spec), and
write the resulting scope updates into `specs/028-actionable-lists/scopes.md`
under a new "Round 9 follow-up" section.

This bug (BUG-028-004) is closed independently of those follow-ups; the
deferred findings are tracked here, not held open against this bug.
