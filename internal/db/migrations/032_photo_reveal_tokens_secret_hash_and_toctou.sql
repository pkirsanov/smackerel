-- 032_photo_reveal_tokens_secret_hash_and_toctou.sql
-- Spec 040 hardening — close MIT-040-S-001 + MIT-040-S-007.
--
-- MIT-040-S-001 (LOW): the original photo_reveal_tokens table stored
--   only id/photo_id/actor_id/expires_at/consumed_at/created_at. The
--   wire format documented in internal/connector/photos/sensitivity.go
--   was `<uuid>.<secret>`, but ConsumeRevealToken validated only the
--   `<uuid>` half; the 24-byte crypto/rand secret was decorative.
--   `hashRevealSecret` was declared but discarded via
--   `var _ = hashRevealSecret`. Add `secret_hash bytea` so MintRevealToken
--   can persist a SHA-256 of the random secret and ConsumeRevealToken
--   can constant-time compare the presented secret against it. The
--   wire blob shape stays `<uuid>.<secret>` — only the server-side
--   validation changes. Existing rows survive the ALTER via the
--   transient empty default; future inserts MUST supply a hash.
--
-- MIT-040-S-007 (MEDIUM): ConsumeRevealToken's transactional SELECT
--   lacked FOR UPDATE and the UPDATE lacked a `WHERE consumed_at IS NULL`
--   predicate, so two concurrent reveal requests presenting the same
--   token could both pass the `consumed_at IS NULL` Go check and both
--   run the UPDATE — bypassing the documented single-use guarantee.
--   The Go code now serializes via FOR UPDATE and adds the predicate to
--   the UPDATE; this migration adds the unique-on-hash index so the
--   single-use invariant also holds at the storage layer (no two live
--   tokens can collide on hash).

ALTER TABLE photo_reveal_tokens
    ADD COLUMN IF NOT EXISTS secret_hash bytea NOT NULL DEFAULT ''::bytea;

ALTER TABLE photo_reveal_tokens
    ALTER COLUMN secret_hash DROP DEFAULT;

CREATE UNIQUE INDEX IF NOT EXISTS uq_photo_reveal_tokens_secret_hash
    ON photo_reveal_tokens (secret_hash)
    WHERE secret_hash <> ''::bytea;

-- Rollback (manual):
-- DROP INDEX IF EXISTS uq_photo_reveal_tokens_secret_hash;
-- ALTER TABLE photo_reveal_tokens DROP COLUMN IF EXISTS secret_hash;
