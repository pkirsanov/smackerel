# Design 093 — Admin-Generated Registration Invites (DB-Backed, Single-Use)

**Owner:** `bubbles.design` · **Status:** design complete · **Builds on:** spec.md
(analyst BDD + UX wireframes/flows — the binding requirements + the resolved
UX decisions (c)/(d)).

This document **settles the four design-owned open decisions ((a) hash algo,
(b) TTL, (e) keep-static-secret, (f) consume/create atomicity)** and the
**template-placement** question, then specifies the migration, repo, handlers,
routing, wiring, security model, and test strategy at contract grade so
`/bubbles.plan` and `/bubbles.implement` proceed with zero ambiguity.

---

## Design Brief

**Current State.** Spec 091 ships invite-gated web self-registration. The gate
is a **single static operator secret** `WEB_REGISTRATION_INVITE_TOKEN`
([config.go](../../internal/config/config.go) 524–530, 1533) constant-time
compared in [web_register.go](../../internal/api/web_register.go) Step 3, with a
shared non-enumerating banner, a no-cookie `303 → /login?registered=1` success,
and a value-safe coarse `logRegisterReject` reason enum. Accounts are created via
[`webcreds.PostgresRepo.UpsertPassword(create=true)`](../../internal/auth/webcreds/repo.go)
(argon2id). The richest operator admin web surface behind `webAuthMiddleware` is
`/cards/admin` ([cardrewards.go](../../internal/web/cardrewards.go) `AdminPage` /
`AdminScrapeNow` / `AdminSyncCalendarNow`), rendered with the spec-092
design-token chrome (`{{define "head"}}` / `{{define "cardrewards-nav"}}` /
`{{define "foot"}}` in [cardrewards_templates.go](../../internal/web/cardrewards_templates.go)).
Migration high-water mark is `057_card_rewards.sql`.

**Target State.** A logged-in operator generates **single-use, hashed-at-rest**
registration invites from a new **`GET /cards/admin/invites`** page (same
`webAuthMiddleware` group as `/cards/admin`), sees the **plaintext exactly once**
on the generate response, lists outstanding/used/revoked invites (metadata only),
and revokes unused ones — all without sops surgery. `/register` now accepts a
valid DB invite **OR** the static secret; a successful DB-invite registration
**atomically** marks the invite used in the **same transaction** as account
creation. Zero regression to spec 091.

**Patterns to Follow.**
- The spec-091 gate-first control flow + shared banner + value-safe log in
  [web_register.go](../../internal/api/web_register.go) — extend it, do not rewrite it.
- The [webcreds](../../internal/auth/webcreds/repo.go) `Repo` + `PostgresRepo` +
  hash-excluded `UserRow` projection + `NewPostgresRepo(nil)→error` nil-guard —
  mirror this shape for the new `webinvite` repo.
- The hashed-single-use-token + guarded-`UPDATE` TOCTOU pattern in
  [032_photo_reveal_tokens_secret_hash_and_toctou.sql](../../internal/db/migrations/032_photo_reveal_tokens_secret_hash_and_toctou.sql).
- The additive migration style (`TIMESTAMPTZ DEFAULT now()`, descriptive
  `COMMENT`s) of [044_web_user_credentials.sql](../../internal/db/migrations/044_web_user_credentials.sql),
  and the `id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text` PK convention of
  [019_expense_tracking.sql](../../internal/db/migrations/019_expense_tracking.sql).
- The spec-092 chrome reuse (`{{template "head" .}}` / `cardrewards-nav` /
  `foot`) + the `SetTriggers` late-wiring idiom on `CardRewardsWebHandler`.

**Patterns to Avoid.**
- `callerIsAdmin` ([auth_handlers.go](../../internal/api/auth_handlers.go)
  274–289) — returns `false` for **every** web-login session in production;
  gating generate/list/revoke behind it would lock out the operator. Use
  `webAuthMiddleware`.
- The spec-044 [tokens.html](../../internal/api/admin_ui_static/tokens.html)
  `'unsafe-inline'` + `fetch()` approach — it would force a CSP relaxation on the
  new route. Use the CSP-clean `<form>`-POST style (spec-091 `register.html`).
- A pluggable `InviteSource` provider abstraction for exactly two hard-coded
  checks (YAGNI; multiplies response paths and risks non-enumeration).
- A non-atomic consume (a `SELECT`-then-`UPDATE` without a guarded predicate, or
  consume-then-create with compensation) — use one guarded `UPDATE` inside one tx.

**Resolved Decisions (binding — see "Settled Open Decisions").**
- **(a) Hash = SHA-256 (hex) of the raw token; by-hash lookup.** Not argon2id.
- **(b) TTL = optional expiry, default 7 days at generation, column nullable
  (`NULL` = never).** Consume guard checks `(expires_at IS NULL OR expires_at > now())`.
- **(c) [UX-resolved] dedicated `GET /cards/admin/invites`** in the
  `webAuthMiddleware` group, Admin pill active.
- **(d) [UX-resolved] CSP-clean `<form>`-POST**; generate = POST → **200
  render-once**; revoke = POST → **303** PRG.
- **(e) Keep the static secret as bootstrap.** OR-gate, static checked first;
  **only the DB-invite branch consumes.**
- **(f) One DB transaction** wraps the guarded consume-`UPDATE` + the
  account-`INSERT`; duplicate username rolls the whole tx back (invite **not**
  consumed).
- **Template placement:** new `internal/web/invites.go` + `internal/web/invites_templates.go`;
  handlers are **methods on the existing `CardRewardsWebHandler`**; the
  `webinvite.Repo` is late-wired via a new `SetInvites(...)` setter.

**Open Questions.** None. All six spec open decisions are resolved (c/d by UX;
a/b/e/f here) and the template-placement + route-shape ambiguities are settled
below.

---

## Purpose & Scope

Add a **Registration Invite** lifecycle (generate → outstanding → used | revoked
| expired) with an in-app management surface, and widen the `/register` gate to
accept a live DB invite **or** the static secret. In scope: one migration, one
new `webinvite` repo, three new admin handlers, one modified register handler,
router + wiring changes, and the test suite. Out of scope (per spec Non-Goals):
email/SMS delivery, multi-use invites, RBAC tiers, retiring the static secret,
and any change to PASETO/cookie/`bearerAuthMiddleware`.

---

## Architecture Overview

```
            ┌──────────────────────────── webAuthMiddleware group ───────────────────────────┐
 operator → │  GET  /cards/admin/invites            → CardRewardsWebHandler.AdminInvitesPage  │
            │  POST /cards/admin/invites            → CardRewardsWebHandler.AdminInviteGenerate│ (200 render-once)
            │  POST /cards/admin/invites/{id}/revoke→ CardRewardsWebHandler.AdminInviteRevoke  │ (303 PRG)
            └───────────────┬──────────────────────────────────────────────────────────────────┘
                            │ (internal/web)                 internal/auth/webinvite.Repo
                            ▼                                  ▲           ▲           ▲
                   webinvite.Repo  ──────────────────────────┤ Generate  │ List      │ Revoke
                            ▲                                  │           │           │
 registrant → POST /v1/web/register (PUBLIC, rate-limited) ──┤ IsLive + ConsumeAndCreate (atomic tx)
            (internal/api  Dependencies.HandleWebRegister)    │
                            │ OR-gate                          ▼
                            └── static secret path (unchanged) │  one pgx.Tx:
                                                               │   1) guarded UPDATE … RETURNING id  (claim invite)
                                                               │   2) webcreds.HashAndInsertTx(tx,…) (create account)
                                                               │   commit ⇔ both succeed
                                                               ▼
                                                  PostgreSQL  web_registration_invites + web_user_credentials
```

Two packages touch the invite repo: **`internal/api`** (the public `/register`
consume — `Dependencies.WebInvites`) and **`internal/web`** (the operator admin
UI — `CardRewardsWebHandler` via `SetInvites`). Both reference the **same**
`*webinvite.PostgresRepo` instance constructed once in `cmd/core/wiring.go`.

---

## Settled Open Decisions (BINDING)

### (a) Hash algorithm — SHA-256 (hex) of the raw token; by-hash lookup

**Decision:** the invite token is a **high-entropy random secret**
(`inv_` + `base64.RawURLEncoding(crypto/rand 32 bytes)`, ≥256 bits). Its at-rest
identifier is **`sha256(plaintext)` rendered as lowercase hex** stored in
`token_hash TEXT NOT NULL UNIQUE`. Lookup is **by hash equality**: the gate
computes `sha256(submitted)` and matches the indexed `token_hash`.

**Why not argon2id:** argon2id's slow KDF exists to defend **low-entropy**
human passwords against offline brute force. A 256-bit random invite has no
brute-forceable preimage, so a slow KDF buys nothing and adds latency to **every**
`/register` attempt. A fast cryptographic hash with an **indexed unique by-hash
lookup** is the conventional, correct choice (same reasoning as the spec-044
PASETO at-rest hashing and the migration-032 reveal-secret SHA-256).

**Why plain SHA-256 (not HMAC-keyed):** an attacker with DB read access sees
only 64-char hex digests of 256-bit random preimages — irreversible without the
plaintext. An HMAC key would add key-management burden (storage, rotation,
SST wiring) for negligible marginal benefit at this operator-only scale. Plain
SHA-256 is sufficient and simplest. (If a future threat model demands it, the
`token_hash` column can hold an HMAC without a schema change — out of scope.)

**Constant-time concern (documented):** the static-secret branch keeps
`subtle.ConstantTimeCompare` (it compares the configured **secret** byte-by-byte,
which would otherwise leak via timing). The DB branch does **not** compare the
token byte-by-byte — it computes one SHA-256 and does an **indexed equality
lookup on the digest**, which does not leak the token through compare timing the
way a naive `==` on the raw secret would. Both branches converge on the
**byte-identical** generic banner (`registerGateBanner`), the same `401`, and the
same blank-secret re-render, so the response shape never distinguishes
"bad static secret" from "unknown/used/revoked/expired DB invite" (AC-7,
non-enumeration preserved).

```
token gen:   plaintext = "inv_" + base64.RawURLEncoding.EncodeToString(rand32)
at rest:     token_hash = hex.EncodeToString(sha256.Sum256([]byte(plaintext)))   // 64 hex chars
gate lookup: HashToken(submitted) == token_hash   (indexed UNIQUE)
```

The `inv_` prefix is cosmetic (operator recognizability); the hash covers the
**whole** string including the prefix, and the submitter pastes the whole string.

### (b) TTL — optional expiry, default 7 days, column nullable (`NULL` = never)

**Decision (binding):** `expires_at TIMESTAMPTZ` is **nullable**. `Generate`
takes a `ttl time.Duration`; `ttl > 0` ⇒ `expires_at = now() + ttl`; `ttl <= 0`
⇒ `expires_at = NULL` (never expires). **v1 default at the UI is 7 days**
(`AdminInviteGenerate` calls `Generate(ctx, createdBy, label, 7*24*time.Hour)`).
The UI does **not** expose an expiry picker in v1 (per UX: the expiry control is
"design-deferred… not rendered yet"); a future scope can add the picker + a
"never" (`NULL`) option without a schema change.

**Why a default TTL (not "no TTL"):** an outstanding invite is a live
credential-minting capability. A 7-day default bounds the blast radius of a
leaked/forgotten invite (it self-expires) while staying generous enough for
normal hand-off. Representing "never" as `NULL` keeps the break-glass option open
for a future deliberate choice.

**The consume + gate guards MUST include** `(expires_at IS NULL OR expires_at >
now())`. An expired invite is rejected with the generic banner (UC-4) and renders
the `⚠ Expired` badge in the list.

### (c) UI placement — *UX-resolved; restated as binding*

Dedicated **`GET /cards/admin/invites`**, mounted in the **same
`webAuthMiddleware` group** as `/cards/admin` (a sub-route under the existing
`r.Route("/cards/admin", …)` block), reachable via an **"Account Invites →"**
`.btn .btn-secondary` link on `/cards/admin`, rendered with the spec-092
`cardrewards-nav` chrome with the **Admin pill active**. No new top-level nav
pill. This inherits the correct authorization (NOT `callerIsAdmin`) for free.

### (d) CSP / interaction shape — *UX-resolved; restated as binding*

CSP-clean, server-rendered `<form>`-POST surface (no inline `<script>`, no
`onclick`/`onsubmit`, no `'unsafe-inline'`):

| Action | Method + route | Response |
|--------|----------------|----------|
| **Generate** | `POST /cards/admin/invites` | **HTTP 200**, direct one-time render of the invites page **with** the freshly-minted plaintext in a `role="status"` callout — **NOT** a redirect |
| **Revoke** | `POST /cards/admin/invites/{id}/revoke` | **303 → `GET /cards/admin/invites`** (pure PRG) |
| **List** | `GET /cards/admin/invites` | the page (generate form + metadata-only table) |

**Route-shape reconciliation (design-owned):** the generate route is **`POST
/cards/admin/invites`** (RESTful create on the collection), **not**
`POST /cards/admin/invites/generate`. The spec's resolved UX decision table binds
`POST /cards/admin/invites`, and that single binding (POST to the same path the
GET list lives on, differing only by method) is the authoritative truth source;
the `/generate` suffix that appears in looser paraphrase is reconciled away. This
keeps `GET /` (list) and `POST /` (generate) on one collection path inside the
`/invites` sub-route — chi resolves them by method.

**Generate = POST → render-once (200), not PRG — the single intentional
deviation, required by value-safety:** the one-time plaintext must **never**
travel through a redirect `Location`, a query string, a flash cookie, or a log.
It therefore appears **only** in the immediate POST response body. A browser
refresh on that 200 re-submits the form (standard "resubmit?" prompt) and mints
an additional outstanding (revocable) invite — harmless. No server-side
plaintext is stored to make refresh idempotent (storing it would violate
hashed-at-rest); the resubmit is simply accepted.

### (e) Keep the static secret as bootstrap — OR-gate, static first, only DB consumes

**Decision (binding):** the `/register` gate is **(valid live DB invite) OR
(static `WEB_REGISTRATION_INVITE_TOKEN`)**. The static secret is **kept** as the
operator bootstrap / break-glass (it solves the chicken-and-egg: the first
operator account predates any DB invite). **Branch order:**

1. **Disabled check** (fail-loud, AC-11): if `d.WebCredentials == nil` **OR**
   (`configured == "" ` **AND** `d.WebInvites == nil`) ⇒ generic banner, blank
   username. Registration is never open signup.
2. **Static secret first** (cheap, constant-time): `staticOK = configured != ""
   && subtle.ConstantTimeCompare([]byte(invite), []byte(configured)) == 1`.
3. **DB invite second** (only if `!staticOK && d.WebInvites != nil`):
   `dbLive = d.WebInvites.IsLive(ctx, HashToken(invite))` (a non-mutating gate
   read — see (f)).
4. If `!staticOK && !dbLive` ⇒ generic banner, blank username (covers wrong
   static secret, unknown/used/revoked/expired DB invite, and the disabled case).

**Only the DB-invite branch consumes.** The static-secret branch creates the
account via the **unchanged** `d.WebCredentials.UpsertPassword(ctx, username,
password, true)` and marks **nothing** used — it is reusable bootstrap. The
DB-invite branch creates the account **and** atomically marks the invite used in
one tx (see (f)).

### (f) Consume + account-create atomicity — one pgx transaction in the invite repo

**Decision (binding):** the guarded consume `UPDATE` and the account `INSERT`
execute in **one `pgx.Tx`**, owned by the `webinvite.PostgresRepo` (which owns
the pool). Either both commit or both roll back — no orphan consume, no orphan
account.

The `/register` DB-invite path (after the gate + field validation) is:

```go
hash := webinvite.HashToken(invite)
outcome, err := d.WebInvites.ConsumeAndCreate(ctx, hash, username,
    func(ctx context.Context, tx pgx.Tx) error {
        return webcreds.HashAndInsertTx(ctx, tx, username, password) // argon2id + INSERT on the SAME tx
    })
```

`ConsumeAndCreate` (inside the repo, one tx):

```sql
BEGIN;
UPDATE web_registration_invites
   SET used_at = now(), used_by = $usedBy
 WHERE token_hash = $hash
   AND used_at    IS NULL
   AND revoked_at IS NULL
   AND (expires_at IS NULL OR expires_at > now())
 RETURNING id;            -- 0 rows ⇒ invalid/used/expired/revoked (lost the race) ⇒ ConsumeInvalid, ROLLBACK
-- 1 row ⇒ run onClaimed(tx) = HashAndInsertTx(tx, username, password)
--          onClaimed ok            ⇒ COMMIT  ⇒ ConsumeCreated
--          onClaimed ErrUserExists ⇒ ROLLBACK ⇒ ConsumeRolledBack (invite NOT consumed)  ← duplicate username
--          onClaimed other err     ⇒ ROLLBACK ⇒ ConsumeRolledBack + wrapped err          ← server error
COMMIT;
```

The guarded `UPDATE ... WHERE ... RETURNING id` is the **single-use authority**
(mirrors migration 032): two concurrent registrations with the same invite race
on the row; exactly one `UPDATE` returns a row, the other returns 0 ⇒
`ConsumeInvalid` ⇒ generic banner. No TOCTOU double-spend.

**Duplicate-username case (settled):** `HashAndInsertTx` maps the unique-violation
(`web_user_credentials_pkey`, SQLSTATE `23505`) to `webcreds.ErrUserExists`; the
repo rolls the **whole** tx back, so the invite's `used_at`/`used_by` stay `NULL`
(invite **not** consumed — correct, the registration did not succeed). The handler
maps `ConsumeRolledBack + ErrUserExists` to the spec-091 duplicate banner
(`409`); the operator can retry the same invite with a different username.

**Why `IsLive` exists (gate read) in addition to `ConsumeAndCreate`:** spec-091's
non-enumeration contract shows the **specific** field banners (mismatch / too
short / username invalid) **only** to holders of a valid token. The DB half of
that gate needs a read. `IsLive(ctx, hash)` is a **non-mutating** `SELECT 1 …
WHERE token_hash=$1 AND used_at IS NULL AND revoked_at IS NULL AND (expires_at IS
NULL OR expires_at > now())` used **only** to decide whether to advance to field
validation — exactly mirroring how `staticOK` gates the static path. It is **not**
the single-use authority; the guarded `UPDATE` is. The race window between
`IsLive` and the `UPDATE` collapses safely to `ConsumeInvalid` ⇒ generic banner
(non-enumerating). Password length is *also* enforced inside `HashAndInsertTx`
(via `webcreds.Hash`), so the tx is self-defending even if handler-level
validation were bypassed.

### Template placement — methods on `CardRewardsWebHandler`, new sibling files

**Decision (binding):** the invite admin UI lives in **package `internal/web`** so
it inherits the spec-092 `head`/`cardrewards-nav`/`foot` chrome + the
design-token CSS + the template `FuncMap` **verbatim, with zero duplication**:

- **`internal/web/invites.go`** — the three handler methods on the **existing**
  `CardRewardsWebHandler` (`AdminInvitesPage`, `AdminInviteGenerate`,
  `AdminInviteRevoke`) + the invite view models. Kept in a separate file from
  `cardrewards.go` so the auth-invite concern is visually isolated.
- **`internal/web/invites_templates.go`** — `const cardRewardsInviteTemplates`
  defining `{{define "cardrewards-invites.html"}}` and
  `{{define "cardrewards-invite-reveal.html"}}`, **parsed in
  `NewCardRewardsWebHandler`** right after `cardRewardsInsightsTemplates` so it
  inherits the already-registered `FuncMap` + chrome defines.
- The `webinvite.Repo` is a **new field** on `CardRewardsWebHandler`, **late-wired
  via `SetInvites(repo)`** (mirrors the existing `SetTriggers` idiom) — so the
  constructor signature and the `CardRewardsWebUI` interface are **unchanged**.
  `nil` until wired; when `nil`, the invite sub-pages return `503` (and in
  practice the whole handler only mounts when a Postgres pool exists).

**Why methods on `CardRewardsWebHandler` (not a separate handler):** the chrome
defines (`head`/`nav`/`foot`) live in the **same** big `cardRewardsTemplates`
string as page bodies that reference the card-rewards `FuncMap` (`cents`,
`date`, `pct`, …). A *separate* handler parsing that string would have to
re-register the identical `FuncMap` (duplication) **or** spec-092's chrome would
have to be extracted into a shared const (a refactor that touches the
"unchanged /cards" surface and re-triggers the spec-092 regression). Adding the
two invite templates to the **existing** card-rewards template set + three method
handlers reuses everything with **zero** shared-template edits and **zero**
`FuncMap` duplication — the lowest-risk, least-ambiguous placement. The handler
already owns `/cards/admin`; `/cards/admin/invites` is a natural child.

**Nav active pill:** the invite view model sets **`Title: "Admin"`** so the
existing `cardrewards-nav` define lights the **Admin** pill (the nav keys the
active pill off `.Title`) with **no** edit to the shared chrome. The page body
template hard-codes its own `<h1 class="page-title">Account Invites</h1>` +
subtitle (exactly as every other card-rewards page hard-codes its `h1`). The
browser tab reads "Admin - Smackerel", which is correct — invites is a child of
Admin.

---

## DB Migration — `internal/db/migrations/058_web_registration_invites.sql`

`058` is the next sequential number (high-water `057_card_rewards.sql`;
confirmed via `internal/db/migrations/`). Additive, forward-only, PostgreSQL,
**no plaintext column** — the DB holds only the hash.

```sql
-- 058_web_registration_invites.sql
-- Spec 093 — admin-generated, single-use, DB-backed registration invites.
--
-- Augments spec 091's invite-gated web self-registration. A logged-in operator
-- GENERATES a single-use invite from the web UI; the PLAINTEXT token is shown
-- exactly once and is NEVER persisted — only its SHA-256 (hex) lives here. A
-- new person registers once at /register with the plaintext; on success the
-- account is created and the invite is atomically marked used (single-use,
-- guarded UPDATE — same TOCTOU posture as 032_photo_reveal_tokens). The static
-- WEB_REGISTRATION_INVITE_TOKEN is unchanged and kept as bootstrap.
--
-- id PK convention follows 019_expense_tracking.sql (TEXT + gen_random_uuid()::text)
-- so the {id} revoke route param and the List view-model are plain strings.

CREATE TABLE IF NOT EXISTS web_registration_invites (
    id          TEXT        PRIMARY KEY DEFAULT gen_random_uuid()::text,
    token_hash  TEXT        NOT NULL UNIQUE,
    label       TEXT,
    created_by  TEXT        NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at  TIMESTAMPTZ,
    used_at     TIMESTAMPTZ,
    used_by     TEXT,
    revoked_at  TIMESTAMPTZ
);

COMMENT ON TABLE  web_registration_invites IS
    'Spec 093 — single-use, hashed-at-rest registration invites generated by a logged-in operator. Holds only the SHA-256 (hex) of the plaintext token; the plaintext is shown once at generation and never stored. Augments (does not replace) the static WEB_REGISTRATION_INVITE_TOKEN bootstrap gate.';
COMMENT ON COLUMN web_registration_invites.token_hash IS
    'Lowercase hex SHA-256 of the plaintext invite token (incl. the inv_ prefix). The plaintext is NEVER stored. Lookup + single-use consume is by this hash. UNIQUE so two live invites cannot collide.';
COMMENT ON COLUMN web_registration_invites.label IS
    'Optional operator-facing note (e.g. "for the new analyst"). Metadata only; safe to display in the list.';
COMMENT ON COLUMN web_registration_invites.created_by IS
    'Display identity of the web session that generated the invite (auth.Session.UserID when present, else "operator"). Metadata only — NOT an authorization key.';
COMMENT ON COLUMN web_registration_invites.expires_at IS
    'Optional TTL boundary. NULL = never expires. The consume + gate guards check (expires_at IS NULL OR expires_at > now()). v1 default at generation is now()+7 days.';
COMMENT ON COLUMN web_registration_invites.used_at IS
    'Set atomically (same tx as the account INSERT) when the invite is consumed by a successful /register. NULL while outstanding.';
COMMENT ON COLUMN web_registration_invites.used_by IS
    'Username that consumed the invite (the new account). NULL while outstanding.';
COMMENT ON COLUMN web_registration_invites.revoked_at IS
    'Set when the operator revokes an OUTSTANDING invite. A revoked invite can never register.';

-- The UNIQUE constraint on token_hash already provides the by-hash lookup index.
-- A partial index for the "live" predicate is unnecessary at operator scale.

-- Rollback (manual):
-- DROP TABLE IF EXISTS web_registration_invites;
```

**Derived lifecycle state** (computed in Go, never stored): `revoked_at != NULL`
⇒ `REVOKED`; else `used_at != NULL` ⇒ `USED`; else `expires_at != NULL &&
expires_at <= now()` ⇒ `EXPIRED`; else `OUTSTANDING`.

---

## New Repo — `internal/auth/webinvite/`

Mirrors the `internal/auth/webcreds` shape (interface + `PostgresRepo` +
`NewPostgresRepo(nil)→error` + a hash-excluded projection). Imports only
`crypto/rand`, `crypto/sha256`, `encoding/base64`, `encoding/hex`, `pgx/v5` +
`pgxpool`. **Does NOT import `webcreds`** — the account-create is injected as a
`func(ctx, pgx.Tx) error` callback, keeping the packages decoupled.

### `internal/auth/webinvite/repo.go`

```go
package webinvite

// InviteStatus is the derived lifecycle state (never stored).
type InviteStatus string
const (
    StatusOutstanding InviteStatus = "outstanding"
    StatusUsed        InviteStatus = "used"
    StatusExpired     InviteStatus = "expired"
    StatusRevoked     InviteStatus = "revoked"
)

// InviteRow is the metadata-only projection for List — NO token_hash, NO plaintext.
type InviteRow struct {
    ID        string
    Label     *string
    CreatedBy string
    CreatedAt time.Time
    ExpiresAt *time.Time
    UsedAt    *time.Time
    UsedBy    *string
    RevokedAt *time.Time
    Status    InviteStatus // derived at scan time from now()
}

// ConsumeOutcome is the result of ConsumeAndCreate.
type ConsumeOutcome int
const (
    ConsumeInvalid     ConsumeOutcome = iota // unknown/used/expired/revoked (incl. lost race) — invite untouched
    ConsumeCreated                           // claimed + onClaimed committed — success
    ConsumeRolledBack                        // valid invite claimed in-tx but onClaimed failed → rolled back; see err
)

// RevokeOutcome distinguishes a real transition from a no-op (stale-page race).
type RevokeOutcome int
const (
    RevokeNoop RevokeOutcome = iota // already used/revoked (or unknown id) — nothing to do
    RevokeDone                      // OUTSTANDING → REVOKED
)

type Repo interface {
    // Generate mints a high-entropy token, stores ONLY its SHA-256 (hex) + metadata,
    // and returns the one-time PLAINTEXT to the caller. ttl>0 ⇒ expires_at=now()+ttl; ttl<=0 ⇒ NULL.
    Generate(ctx context.Context, createdBy, label string, ttl time.Duration) (plaintext string, err error)

    // IsLive reports whether token_hash names an OUTSTANDING invite (non-mutating gate read).
    IsLive(ctx context.Context, tokenHash string) (bool, error)

    // ConsumeAndCreate atomically claims the invite (guarded UPDATE … RETURNING id) and runs
    // onClaimed within the SAME tx. Commit ⇔ both succeed. See ConsumeOutcome.
    ConsumeAndCreate(ctx context.Context, tokenHash, usedBy string,
        onClaimed func(ctx context.Context, tx pgx.Tx) error) (ConsumeOutcome, error)

    // List returns metadata-only rows (newest first). NEVER token_hash, NEVER plaintext.
    List(ctx context.Context) ([]InviteRow, error)

    // Revoke sets revoked_at on an OUTSTANDING invite. Guarded so a used/revoked/unknown id is a no-op.
    Revoke(ctx context.Context, id string) (RevokeOutcome, error)
}

// HashToken returns the lowercase-hex SHA-256 of the plaintext (the at-rest identifier).
// Exported so internal/api's /register gate can hash a submitted token for IsLive/ConsumeAndCreate.
func HashToken(plaintext string) string {
    sum := sha256.Sum256([]byte(plaintext))
    return hex.EncodeToString(sum[:])
}

type PostgresRepo struct{ pool *pgxpool.Pool }

// NewPostgresRepo refuses a nil pool (no silent dev no-op) — mirrors webcreds.NewPostgresRepo.
func NewPostgresRepo(pool *pgxpool.Pool) (*PostgresRepo, error) { /* nil-guard */ }
```

**Method bodies (specification):**

- `Generate`: `raw := make([]byte, 32); crypto/rand.Read(raw); plaintext := "inv_"
  + base64.RawURLEncoding.EncodeToString(raw)`. Compute `hash := HashToken(plaintext)`.
  `var expires *time.Time; if ttl > 0 { t := time.Now().Add(ttl); expires = &t }`.
  `INSERT INTO web_registration_invites (token_hash, label, created_by, expires_at)
  VALUES ($1, $2, $3, $4)` (label `NULL` when empty; `created_at`/`id` defaulted).
  Return `plaintext` (the **only** place it ever leaves the process). On the
  ~never unique-hash collision, regenerate once then error.
- `IsLive`: `SELECT 1 FROM web_registration_invites WHERE token_hash=$1 AND
  used_at IS NULL AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at >
  now())`; `pgx.ErrNoRows ⇒ false, nil`.
- `ConsumeAndCreate`: `tx := pool.Begin(ctx)` with `defer tx.Rollback(ctx)`; run
  the guarded `UPDATE … RETURNING id`; `pgx.ErrNoRows ⇒ return ConsumeInvalid,
  nil`; else call `onClaimed(ctx, tx)`; on error `return ConsumeRolledBack, err`
  (deferred rollback fires); on success `tx.Commit(ctx); return ConsumeCreated,
  nil`.
- `List`: `SELECT id, label, created_by, created_at, expires_at, used_at,
  used_by, revoked_at FROM web_registration_invites ORDER BY created_at DESC`;
  derive `Status` per row against `time.Now()`. **token_hash is never selected.**
- `Revoke`: `UPDATE web_registration_invites SET revoked_at=now() WHERE id=$1 AND
  used_at IS NULL AND revoked_at IS NULL RETURNING id`; `pgx.ErrNoRows ⇒
  RevokeNoop, nil`; row ⇒ `RevokeDone, nil`.

### `internal/auth/webcreds/repo.go` — add `HashAndInsertTx` (free function)

A package-level helper (NOT on the `Repo` interface — zero interface churn, no
test-fake breakage) that hashes + inserts on a caller-supplied tx:

```go
// HashAndInsertTx validates the username, argon2id-hashes the password, and INSERTs a
// new web_user_credentials row on the provided tx. Maps a unique-violation (SQLSTATE 23505)
// to ErrUserExists so the caller's transaction can roll back (used by the spec-093 atomic
// invite consume+create). Does NOT commit — the caller owns tx lifecycle.
func HashAndInsertTx(ctx context.Context, tx pgx.Tx, username, password string) error {
    if err := ValidateUsername(username); err != nil { return err }
    hash, err := Hash(password); if err != nil { return err }
    _, err = tx.Exec(ctx,
        `INSERT INTO web_user_credentials (username, password_hash) VALUES ($1, $2)`,
        username, hash)
    var pgErr *pgconn.PgError
    if errors.As(err, &pgErr) && pgErr.Code == "23505" { return ErrUserExists }
    if err != nil { return fmt.Errorf("webcreds: insert in tx: %w", err) }
    return nil
}
```

The existing `UpsertPassword` (static-secret path) is **unchanged**.

---

## Handlers

### `internal/web/invites.go` — admin UI (methods on `CardRewardsWebHandler`)

```go
// SetInvites late-wires the spec-093 invite repo (mirrors SetTriggers). nil ⇒ the
// invite sub-pages return 503. Set in cmd/core/wiring.go after construction.
func (h *CardRewardsWebHandler) SetInvites(r webinvite.Repo) { h.Invites = r }
```

- **`AdminInvitesPage`** (`GET /cards/admin/invites`): if `h.Invites == nil` ⇒
  `503`. `rows, err := h.Invites.List(ctx)`. Render
  `cardrewards-invites.html` with view model `{Title:"Admin", Invites:[…],
  Notice:""}` (Notice carries the optional `?notice=race` warning banner from a
  revoke no-op). No plaintext, no hash.
- **`AdminInviteGenerate`** (`POST /cards/admin/invites`): if `h.Invites == nil`
  ⇒ `503`. Parse the (optional) `label` form field (trim; cap length defensively).
  `createdBy := sessionIdentity(r)` (see below). `plaintext, err :=
  h.Invites.Generate(ctx, createdBy, label, 7*24*time.Hour)`. On error ⇒ render
  the page with a `role="alert"` error banner (`500`, value-safe — never echo
  the token). On success ⇒ re-`List` and render `cardrewards-invite-reveal.html`
  with `{Title:"Admin", Token: plaintext, Label: label, Invites:[…]}` at **HTTP
  200** (render-once; the plaintext is in this body and nowhere else).
- **`AdminInviteRevoke`** (`POST /cards/admin/invites/{id}/revoke`): if `h.Invites
  == nil` ⇒ `503`. `id := chi.URLParam(r, "id")`. `outcome, err :=
  h.Invites.Revoke(ctx, id)`. Always **303 → `/cards/admin/invites`**;
  `RevokeNoop` ⇒ append `?notice=race` so the list shows the non-enumerating
  `alert-warning` "already used or revoked — nothing to do" banner.

```go
// sessionIdentity returns the operator's display identity for created_by:
// auth.Session.UserID when a per-user session is present and non-empty, else "operator"
// (shared-token sessions carry no distinct username — spec 070 "any web user = full admin").
// Value-safe: never a secret; metadata only.
func sessionIdentity(r *http.Request) string {
    if sess, ok := auth.SessionFromContext(r.Context()); ok && sess.UserID != "" {
        return sess.UserID
    }
    return "operator"
}
```

**View models** (in `invites.go`): `inviteListVM{ Title string; Invites
[]webinvite.InviteRow; Notice string }` and `inviteRevealVM{ Title string; Token
string; Label string; Invites []webinvite.InviteRow }`. Both carry `Title` for
the shared head/nav. `Token` exists **only** on `inviteRevealVM` and is rendered
**only** by `cardrewards-invite-reveal.html`.

### `internal/web/invites_templates.go` — the two templates

`const cardRewardsInviteTemplates` defining:

- **`{{define "cardrewards-invites.html"}}`** — `{{template "head" .}}` /
  `{{template "cardrewards-nav" .}}` / page header ("Account Invites") /
  `‹ Back to Admin` `.btn-ghost` / the **generate** `<form method="POST"
  action="/cards/admin/invites">` (optional `label` `.form-control` + `Generate
  invite` `.btn-primary`) / the optional `{{if .Notice}}…alert-warning…{{end}}`
  race banner / the metadata-only `.table-wrap > .cr-table` (Label, Created by,
  Created, **Status badge**, Actions). Each `OUTSTANDING` row carries the
  CSS-only `<details><summary>▸ Revoke</summary><form method="POST"
  action="/cards/admin/invites/{{.ID}}/revoke">…Confirm revoke…</form></details>`.
  Empty list ⇒ `.empty-state`. Closes with `{{template "foot"}}`. **No token,
  no hash anywhere.**
- **`{{define "cardrewards-invite-reveal.html"}}`** — same head/nav/header, then
  the `role="status" aria-live="polite"` success callout: a strong "Copy this
  token now — it will not be shown again." line, a **focusable readonly
  `<input aria-label="One-time invite token">`** (monospace, value=`{{.Token}}`),
  the keyboard copy hint, the label echo, and a `Done — back to invites`
  `.btn-secondary` → `/cards/admin/invites`. Below it, the same metadata-only
  table (the new row shows `● Outstanding`, **no** token). `{{template "foot"}}`.

The status badge mapping (text+glyph carry meaning, color reinforces):
`OUTSTANDING → badge-info "● Outstanding"`, `USED → badge-success "✓ Used"
(+ used by)`, `EXPIRED → badge-warning "⚠ Expired"`, `REVOKED → badge-danger
"✕ Revoked"`. A small `inviteStatusBadge` template-helper or inline
`{{if}}`-ladder renders it from `.Status`.

### `internal/api/web_register.go` — widen the gate (DB invite OR static)

Modify `HandleWebRegister`. The gate (Step 3) becomes the OR-gate from decision
(e); Steps 4–6 (field presence/password/username validation) are **unchanged**
and still run **only after** the gate passes; Step 7 (create) branches by which
gate path succeeded.

```go
// Step 3 — OR-gate (static secret OR live DB invite), gate-first, value-safe.
configured := d.WebRegistrationInviteToken
// Disabled (fail-loud): no creds store, OR neither a static secret nor an invite store.
if d.WebCredentials == nil || (configured == "" && d.WebInvites == nil) {
    d.logRegisterReject(r, "gate")
    renderRegisterError(w, r, sanitizeNext(nextRaw), "", registerGateBanner, http.StatusUnauthorized)
    return
}
staticOK := configured != "" &&
    subtle.ConstantTimeCompare([]byte(invite), []byte(configured)) == 1
dbLive := false
if !staticOK && d.WebInvites != nil {
    if live, err := d.WebInvites.IsLive(r.Context(), webinvite.HashToken(invite)); err == nil {
        dbLive = live
    } // a DB error ⇒ dbLive stays false ⇒ generic banner (value-safe; never leak the error)
}
if !staticOK && !dbLive {
    d.logRegisterReject(r, "gate")
    renderRegisterError(w, r, sanitizeNext(nextRaw), "", registerGateBanner, http.StatusUnauthorized)
    return
}

// Steps 4–6 — UNCHANGED field validation (presence / mismatch / too-short / username),
// reached only past the gate, so the specific banners stay safe (echo username).

// Step 7 — create, branching by gate path.
if staticOK {
    // UNCHANGED spec-091 static-secret path — no consume.
    if err := d.WebCredentials.UpsertPassword(r.Context(), username, password, true); err != nil {
        // ErrUserExists → duplicate banner (409); else server banner (500) — identical to spec 091
    }
} else { // dbLive — atomic consume + create
    outcome, err := d.WebInvites.ConsumeAndCreate(r.Context(), webinvite.HashToken(invite), username,
        func(ctx context.Context, tx pgx.Tx) error {
            return webcreds.HashAndInsertTx(ctx, tx, username, password)
        })
    switch {
    case err != nil && errors.Is(err, webcreds.ErrUserExists):
        d.logRegisterReject(r, "duplicate")
        renderRegisterError(w, r, sanitizeNext(nextRaw), username, registerDuplicateUserMsg, http.StatusConflict)
        return
    case err != nil:
        d.logRegisterReject(r, "server")
        renderRegisterError(w, r, sanitizeNext(nextRaw), username, registerServerErrorMsg, http.StatusInternalServerError)
        return
    case outcome == webinvite.ConsumeInvalid: // lost the race after IsLive
        d.logRegisterReject(r, "gate")
        renderRegisterError(w, r, sanitizeNext(nextRaw), "", registerGateBanner, http.StatusUnauthorized)
        return
    }
    // outcome == ConsumeCreated ⇒ fall through to the shared success 303.
}

// Success — UNCHANGED: 303 → /login?registered=1, NO Set-Cookie (carry sanitized next).
```

`logRegisterReject` reason enum is **unchanged** (`gate | field | duplicate |
server`) — the DB and static paths share the `gate` reason, so the log stays
non-enumerating. The invite plaintext, its hash, and the username **value** are
never logged (only `username_len`).

---

## Router Wiring — `internal/web/cardrewards.go` + `internal/api/router.go`

**`internal/web/cardrewards.go` `RegisterRoutes`** — add the `/invites`
sub-route **inside** the existing `r.Route("/cards/admin", …)` block (the block
shown at cardrewards.go around the `/cards/admin` mount):

```go
r.Route("/cards/admin", func(r chi.Router) {
    r.Get("/", h.AdminPage)
    r.Post("/scrape", h.AdminScrapeNow)
    r.Post("/sync-calendar", h.AdminSyncCalendarNow)
    r.Route("/invites", func(r chi.Router) {        // ← spec 093
        r.Get("/", h.AdminInvitesPage)              // GET  /cards/admin/invites
        r.Post("/", h.AdminInviteGenerate)          // POST /cards/admin/invites (200 render-once)
        r.Post("/{id}/revoke", h.AdminInviteRevoke) // POST /cards/admin/invites/{id}/revoke (303)
    })
})
```

Because `RegisterRoutes` is mounted by `internal/api/router.go` inside
`if deps.CardRewardsWebHandler != nil { r.Group(func(r){ r.Use(deps.webAuthMiddleware);
deps.CardRewardsWebHandler.RegisterRoutes(r) }) }` (router.go around the
"Spec 083 Scope 10 — card-rewards web UI" block), **all three invite routes
inherit `webAuthMiddleware` for free** — the binding authorization (UC-7 / M8;
NOT `callerIsAdmin`). **No change to `internal/api/router.go` is required.**

The **`/register` POST** stays exactly where spec 091 put it — in the public
`r.Group(r.Use(httprate.LimitByIP(20, 1*time.Minute)))` block with `r.Post(
"/v1/web/register", deps.HandleWebRegister)` (router.go ~325–333). **Unchanged.**

**`/cards/admin` entry link** — modify the admin template in
[cardrewards_dashboard_templates.go](../../internal/web/cardrewards_dashboard_templates.go)
(the `{{define "cardrewards-admin.html"}}` body) to add a small **"Account
access"** section with `<a class="btn btn-secondary" href="/cards/admin/invites">
Account Invites →</a>`. Additive only — the existing manual-trigger buttons +
run-history table + all `data-*` hooks are untouched.

---

## Dependency Wiring — `cmd/core/wiring.go`

1. **Add the field** to `Dependencies` (in [health.go](../../internal/api/health.go),
   right after `WebRegistrationInviteToken`):

   ```go
   // Spec 093 — DB-backed single-use registration invites. nil when no Postgres
   // pool (config-validate mode, router unit tests): the /register DB-invite
   // branch is then simply not taken and the static-secret path is unchanged.
   WebInvites webinvite.Repo
   ```

2. **Construct once** and fan out to both consumers in
   `wireCardRewardsHandler` (the existing nil-pool-guarded function, so the repo
   is built only when a pool exists):

   ```go
   func wireCardRewardsHandler(svc *coreServices, deps *api.Dependencies) {
       if svc == nil || svc.pg == nil || svc.pg.Pool == nil { return }
       // … existing store/service/webHandler construction …
       inviteRepo, err := webinvite.NewPostgresRepo(svc.pg.Pool)
       if err != nil { /* fail-fast like webcreds: surface; nil pool already excluded above */ }
       deps.WebInvites = inviteRepo      // ← internal/api /register consume
       webHandler.SetInvites(inviteRepo) // ← internal/web admin UI
       deps.CardRewardsWebHandler = webHandler
       // …
   }
   ```

   (If `wireCardRewardsHandler` cannot return an error, mirror the existing
   pattern by logging-and-returning on the impossible `NewPostgresRepo` error;
   the pool is already non-nil here so the nil-guard never trips.)

**Nil-pool behavior (config-validate / router-unit-test):** `wireCardRewardsHandler`
returns early when `svc.pg.Pool == nil`, so `deps.WebInvites` stays `nil` and
`CardRewardsWebHandler.Invites` stays `nil`. The `/register` handler's disabled
check (`configured == "" && d.WebInvites == nil`) and DB-branch guard
(`d.WebInvites != nil`) both handle `nil` safely; the static-secret path is
entirely unaffected. The invite admin sub-pages return `503` if ever reached
without a repo (they are not mounted at all without the pool).

---

## Security Model

| Control | Mechanism |
|---------|-----------|
| **Token entropy** | `crypto/rand` 32 bytes (256-bit) → `inv_` + base64url. Preimage-resistant. |
| **Hashed at rest** | Only `sha256(plaintext)` (hex) is stored; the plaintext leaves the process **once** (the generate-200 body) and is never persisted/logged/re-shown. |
| **Single-use, atomic** | Guarded `UPDATE … WHERE used_at IS NULL AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > now()) RETURNING id`, in one tx with the account `INSERT`. No TOCTOU double-spend. |
| **Revoke** | Guarded `UPDATE … SET revoked_at=now() WHERE id=$1 AND used_at IS NULL AND revoked_at IS NULL`. Idempotent; stale-page race ⇒ no-op + non-enumerating banner. |
| **TTL** | Default 7 days; nullable (`NULL`=never); enforced in gate + consume. |
| **Non-enumeration** | DB-invalid, static-wrong, used, revoked, expired, and disabled **all** return the byte-identical `registerGateBanner` + `401` + blank-secret re-render. Hash lookup leaks no validity via response shape; the coarse `logRegisterReject` `gate` reason covers all gate failures. |
| **Value-safe** | Plaintext only in the one generate-200 body; never in a redirect `Location`/query/flash/cookie/log/list/hash/refetch. Username **value**, password, token, and hash are never logged (only `username_len`). |
| **Authorization** | Generate/list/revoke behind **`webAuthMiddleware`** (NOT `callerIsAdmin`) by mounting under the existing `/cards/admin` group. `/register` consume stays public + rate-limited (`httprate.LimitByIP(20,1m)`), OUTSIDE `bearerAuthMiddleware`. |
| **CSP** | No new inline `<script>`/handlers and no `'unsafe-inline'`; the invite pages reuse the spec-092 script-free `head` and pass the spec-077 CSP guard (`web/pwa/tests/_support/csp.ts` `attachCSPGuard` / `assertNoCSPViolations`). No CSP relaxation. |
| **Trust band** | A consumed invite creates a spec-070/091 **full-admin** account — no escalation, no reduction. Generating invites is itself a full-admin action. |

Cross-references: spec 091 gate + banner (web_register.go), spec 092 CSP-clean
`/cards` posture + design tokens, spec 077 CSP guard, spec 044 `tokens.html`
"show once" precedent (shape only; its `'unsafe-inline'` is **avoided**).

---

## Capability-Foundation Note (DE4)

Run the proportionality triggers from `capability-foundation.md`. The incidental
vocabulary (the invite now has two **sources**; the words *source* / *single-use*
/ *variant* appear) fires the trigger, but this is **not** a multi-provider /
adapter / strategy / plugin family:

- There is **one** registration capability — no second *kind* of registration.
- The gate is **two concrete checks in one handler** (a constant-time static
  compare **OR** a single hashed-lookup-then-atomic-consume), **not** a pluggable
  `InviteSource` provider seam. Two hard-coded checks do not warrant an adapter
  abstraction (YAGNI), and an extra seam would multiply response paths and
  threaten the non-enumeration invariant.
- The genuinely shared foundations are **reused, not re-invented**: the
  `web_user_credentials` store + argon2id (`internal/auth/webcreds`), the
  gate-first non-enumerating banner (`internal/api/web_register.go`), the spec-092
  chrome (`internal/web/cardrewards_templates.go`), and the
  hashed-single-use-token TOCTOU pattern (migration 032).

### Single-Implementation Justification

This feature adds exactly **one** concrete implementation of each new surface (one
`webinvite.PostgresRepo`, one admin page, one widened `/register` gate). No second
provider/adapter/variant is introduced or anticipated: the invite is a single
DB-backed primitive, and the "two sources" of the gate (static secret + DB invite)
are two literal branches in one handler — deliberately **not** abstracted behind a
provider interface (G094). Should a genuinely second invite *kind* ever appear, a
foundation can be extracted then; introducing it now would be speculative
generality. `webinvite.Repo` is an interface only to mirror `webcreds.Repo` for
nil-guarding + test substitutability, not to host multiple providers.

---

## Complexity Tracking

| Deviation from the simplest viable approach | Simpler alternative considered | Why rejected |
|---------------------------------------------|--------------------------------|--------------|
| `IsLive` gate-read **plus** the guarded consume `UPDATE` (two reads on the happy path) | Single guarded `UPDATE` as the only gate (no `IsLive`) | The single-`UPDATE`-only approach cannot show spec-091's **specific** field banners (mismatch/too-short) **only** to valid-token holders without first knowing the token is live — collapsing all DB-path failures (incl. bad password) to one banner would silently regress spec-091's post-gate UX. `IsLive` is the DB analogue of `staticOK`; the `UPDATE` remains the single-use authority. |
| `ConsumeAndCreate` takes a `func(ctx, pgx.Tx) error` callback | Put a raw `*pgxpool.Pool` on `Dependencies` and orchestrate the tx in the handler | Exposing the pool on `Dependencies` widens the surface and leaks tx mechanics into the api handler; the callback keeps the atomic boundary encapsulated in the repo that already owns the pool, and keeps `webinvite` decoupled from `webcreds`. |
| Invite admin handlers are **methods on `CardRewardsWebHandler`** | A standalone `InviteAdminWebHandler` in its own package/handler | A standalone handler cannot reuse the spec-092 `head`/`nav`/`foot` chrome without either duplicating the card-rewards `FuncMap` or extracting the chrome into a shared const (a refactor that touches the "unchanged /cards" surface). Method-on-existing-handler reuses everything with zero shared-template edits. |
| New `webcreds.HashAndInsertTx` free function (alongside `UpsertPassword`) | Add a tx-aware method to the `webcreds.Repo` **interface** | Adding to the interface forces every implementer/test-fake to change. A package-level free function gives the in-tx insert with zero interface churn and zero fake breakage; the static path keeps using the unchanged `UpsertPassword`. |

Everything else uses the simplest viable approach (one additive migration, a
repo that mirrors `webcreds`, chrome reuse, no new config value, no new CSP
posture, no new nav pill).

---

## Testing & Validation Strategy (hand to `/bubbles.plan`)

Every Gherkin UC and UI-matrix row maps to at least one test. Live-stack
categories (`integration`, `e2e-api`, `e2e-ui`) hit the real running stack via
`./smackerel.sh` against **ephemeral** test storage; no internal mocks.

### Scenario → Test mapping

| Scenario | Test type | Location (indicative) | Key assertion |
|----------|-----------|------------------------|----------------|
| UC-1 generate, plaintext once | unit + e2e-ui | `internal/auth/webinvite/*_test.go`, `web/pwa/tests/cardrewards_invites.spec.ts` | `Generate` returns plaintext; row holds only the hex hash; the 200 reveal body contains the token, the GET list never does |
| UC-2 register once with a live invite | integration + e2e-api | `internal/api/web_register_invite_test.go` | account row created (argon2id) **and** `used_at`/`used_by` set in one tx; 303 → `/login?registered=1`, no `Set-Cookie` |
| UC-3 reuse rejected (single-use) | unit + integration | webinvite + register tests | second consume ⇒ `ConsumeInvalid` ⇒ generic banner; no 2nd account; `used_*` unchanged |
| UC-4 expired rejected | unit | webinvite repo test | invite with past `expires_at` ⇒ `IsLive=false` + `ConsumeAndCreate=ConsumeInvalid` |
| UC-5 static secret still works | integration (regression) | existing spec-091 register tests, unchanged | static path creates account, no consume, 303 |
| UC-6 unknown token ⇒ generic banner | unit + integration | register tests | DB-invalid, static-wrong, disabled all byte-identical `401` + `registerGateBanner` |
| UC-7 anonymous cannot generate | e2e-ui / integration | router auth test | `webAuthMiddleware` 401/redirect; no row created |
| UC-8 list metadata only | unit + e2e-ui | webinvite `List` test + invites spec | no `token_hash`, no plaintext in `List`/the rendered page |
| UC-9 revoke ⇒ cannot register | unit + integration + e2e-ui | webinvite `Revoke` + register + invites spec | `RevokeDone`; subsequent consume ⇒ `ConsumeInvalid` ⇒ generic banner |
| UC-10 spec-091 regression | integration + e2e-* (regression) | existing 091 suites, unchanged | `/register` static + `/login` flows byte-identical |
| **Atomic single-use under concurrency** | integration (DB-backed) | `internal/auth/webinvite/concurrent_consume_test.go` | two goroutines `ConsumeAndCreate` the same hash ⇒ **exactly one** `ConsumeCreated`, one `ConsumeInvalid`; exactly one account row |
| **Duplicate username rolls back consume** | integration | webinvite + register test | live invite + taken username ⇒ `ConsumeRolledBack`+`ErrUserExists` ⇒ duplicate banner; invite `used_*` stays `NULL` |
| CSP-clean surface | e2e-ui | `web/pwa/tests/cardrewards_invites.spec.ts` | `attachCSPGuard` + `assertNoCSPViolations` across generate→reveal→list→revoke (spec-077 guard) |

### Required test categories (baseline)

- **unit** — `webinvite` repo (`Generate`/`IsLive`/`ConsumeAndCreate` outcomes:
  created / invalid / rolled-back / expired / revoked; `List` no-plaintext;
  `Revoke` done/noop; `HashToken` determinism); the register OR-gate branch
  selection (static consumes nothing; DB path consumes; both reject invalid with
  the same banner); `webcreds.HashAndInsertTx` (insert + `ErrUserExists` on 23505).
- **integration (DB-backed, ephemeral)** — atomic single-use under concurrency,
  duplicate-username rollback, the full `/register` DB-invite consume path,
  spec-091 static regression.
- **e2e-ui (Playwright, live stack + CSP guard)** —
  `web/pwa/tests/cardrewards_invites.spec.ts`: generate → one-time reveal (token
  visible once, focusable readonly field) → `Done` → list (row present, **no**
  token) → revoke (`<details>` confirm → 303 → `✕ Revoked`); anonymous blocked;
  the `/cards/admin` "Account Invites →" link; CSP-clean throughout. Adversarial
  guard: assert the token is **absent** from the GET list DOM and that an
  injected CSP violation fails the suite (per the spec-077 `_support/csp.ts`
  pattern).
- **Adversarial regression (grill, per policySnapshot)** — at least one
  non-tautological case: a consume attempt whose invite was revoked **between**
  `IsLive` and the `UPDATE` must yield the generic banner (proves the guarded
  `UPDATE`, not the pre-read, is the authority); a reuse test whose second
  attempt uses a **different** username (proves single-use, not username-dedup).
- **Live deploy proof (end of run)** — the `058` migration runs on deploy
  (`./smackerel.sh up` applies migrations); confirm the table exists and a
  generate→register cycle works on the live stack.

### Run commands (record ≥10-line raw evidence per DoD item)

```
./smackerel.sh check
./smackerel.sh test unit
./smackerel.sh test integration
./smackerel.sh test e2e-ui cardrewards   # includes cardrewards_invites.spec.ts + CSP guard
```

---

## Referenced Surfaces (do not re-discover)

- [internal/api/web_register.go](../../internal/api/web_register.go) — gate-first
  control flow, `registerGateBanner`, `logRegisterReject`, no-cookie 303 (modified).
- [internal/auth/webcreds/repo.go](../../internal/auth/webcreds/repo.go) +
  [hasher.go](../../internal/auth/webcreds/hasher.go) — `Repo`/`PostgresRepo`,
  `UpsertPassword`, `ValidateUsername`, `Hash`, `MinPasswordLength=12`,
  `ErrUserExists` (add `HashAndInsertTx`).
- [internal/web/cardrewards.go](../../internal/web/cardrewards.go) —
  `CardRewardsWebHandler`, `NewCardRewardsWebHandler`, `SetTriggers`,
  `RegisterRoutes` `/cards/admin` block (add `SetInvites` + `/invites` routes).
- [internal/web/cardrewards_templates.go](../../internal/web/cardrewards_templates.go)
  — `head`/`cardrewards-nav`/`foot` defines + design tokens (reused).
- [internal/web/cardrewards_dashboard_templates.go](../../internal/web/cardrewards_dashboard_templates.go)
  — the `/cards/admin` body (add "Account Invites →" link).
- [internal/api/router.go](../../internal/api/router.go) — `webAuthMiddleware`
  group + the `CardRewardsWebHandler` mount (no change) + the public
  rate-limited `/register` block (no change).
- [internal/api/health.go](../../internal/api/health.go) — `Dependencies` struct
  (add `WebInvites`).
- [cmd/core/wiring.go](../../cmd/core/wiring.go) — `wireCardRewardsHandler`
  (construct + fan-out the repo), the webcreds/invite-token wiring at ~418–434.
- [internal/db/migrations/044_web_user_credentials.sql](../../internal/db/migrations/044_web_user_credentials.sql)
  · [032_…toctou.sql](../../internal/db/migrations/032_photo_reveal_tokens_secret_hash_and_toctou.sql)
  · [019_expense_tracking.sql](../../internal/db/migrations/019_expense_tracking.sql)
  — migration + TOCTOU + uuid-PK precedents.
- `web/pwa/tests/_support/csp.ts` + `web/pwa/tests/cardrewards_dashboard.spec.ts`
  — the spec-077 CSP guard pattern for the new e2e-ui spec.

---

*Authored by `bubbles.design`. Scopes, report, and user-validation are owned by
later phases. Next required owner: `bubbles.plan`.*
