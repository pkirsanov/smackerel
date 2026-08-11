-- 063_auth_token_granted_scopes.sql
-- Spec 108 design.md §10 (F-108-UX-ROSTER-01, DECIDED) — record the issued
-- scope set on the token row so a principal's grants are readable
-- server-side without possessing the wire token.
--
-- Why the column lives on auth_tokens and not auth_users: this is a RECORD
-- OF ISSUANCE, not an intent store. It is written by IssueAndPersistToken
-- from the same variable that becomes the PASETO `scope` claim, in the same
-- function, for the same token — so the row cannot disagree with the token
-- by construction. A per-principal intent column would diverge the moment
-- intent were edited without a rotation (§10.3, option b-users REJECTED).
-- 033_auth_per_user_bearer.sql:19-20 deferred "per-user permission scopes"
-- to a later spec; this is that continuation.
--
-- Nullable, NO DB-side DEFAULT (G028 / NO-DEFAULTS). The nullability carries
-- meaning and the three states are DISTINCT (§10.4):
--
--   NULL          — UNKNOWN. Issued before grant recording existed.
--                   Produced by THIS MIGRATION ONLY, never by the write path.
--   '{}'          — RECORDED AS NONE. Issued with no scope claim: either no
--                   --scope was supplied, the `--scope ""` demote sentinel was
--                   used, or the admin API minted it (that endpoint has no
--                   scope parameter — F-108-UX-ADMINUI-01).
--   '{corpus:read,…}' — RECORDED SET. Exactly the claim carried in the token.
--
-- Conflating NULL with '{}' would assert that a pre-existing token was issued
-- with no grants — a claim about authority that nobody made. spec.md S7
-- already forbids the UI equivalent ("rendering a guess, a default, or an
-- empty set would be fabricated authority state"); the same prohibition
-- applies to the column that feeds it. A NOT NULL DEFAULT '{}' column would
-- commit exactly that error at migration time for every existing row.
--
-- NO BACKFILL (§10.5). Existing rows stay NULL. Backfilling from a role
-- default would both fabricate and widen; backfilling '{}' would assert
-- "issued with no grants" for tokens that demonstrably carry grants. Unknown
-- fails closed everywhere it is consumed, and the ratified remedy is
-- proactive rotation of every unknowable principal (uservalidation.md item 9),
-- with the operator issuing a deliberate grant set rather than recovering an
-- unreadable one.
--
-- Additive and independently revertible: nothing reads this column yet.

ALTER TABLE auth_tokens ADD COLUMN IF NOT EXISTS granted_scopes text[];

COMMENT ON COLUMN auth_tokens.granted_scopes IS
    'Spec 108 §10 — the scope set recorded at issuance, written by IssueAndPersistToken from the same value that becomes the PASETO `scope` claim (no DB-side default — NO-DEFAULTS / G028). NULL = UNKNOWN (issued before recording existed; produced by migration 063 only, never by the write path). ''{}'' = RECORDED AS NONE (issued with no scope claim, including every admin-API mint). Non-empty = the exact claim in the token. NULL and ''{}'' are distinct and MUST NOT be conflated: NULL is "nobody recorded", ''{}'' is "recorded as no grants".';

-- Rollback (manual):
-- ALTER TABLE auth_tokens DROP COLUMN IF EXISTS granted_scopes;
