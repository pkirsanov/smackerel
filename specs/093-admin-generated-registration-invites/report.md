# Report — Spec 093 (Admin-Generated Registration Invites, DB-Backed, Single-Use)

**Scopes:** [scopes.md](scopes.md) · **Spec:** [spec.md](spec.md) · **Design:** [design.md](design.md) · **User acceptance:** [uservalidation.md](uservalidation.md)

> **Status:** planning complete — implementation + evidence pending. This report is the evidence skeleton authored by the plan phase. Every anchored subsection below corresponds to a Definition-of-Done item in [scopes.md](scopes.md); the implement/test phase fills each with **verbatim terminal / live-stack output (≥10 lines)** tagged `**Claim Source:**` at the time the command is actually run. No evidence is pre-filled and none is fabricated.

## Summary

Spec 093 gives the spec-091 registration invite a real lifecycle and an in-app management surface: a logged-in operator generates **single-use, hashed-at-rest, DB-backed** registration invites from a new `/cards/admin/invites` page (one-time plaintext reveal), lists/revokes them, and `/register` now accepts a live DB invite **OR** the static `WEB_REGISTRATION_INVITE_TOKEN` (kept as bootstrap), consuming the DB invite atomically with account creation. Four scopes (see [scopes.md](scopes.md) for the DAG + Test-Plan↔DoD parity): (01) migration `058` + the `internal/auth/webinvite` repo + `webcreds.HashAndInsertTx`; (02) the `/register` OR-gate DB-invite consume + wiring; (03) the CSP-clean admin invites UI; (04) consolidated verification + live home-lab deploy proof.

## Completion Statement (MANDATORY)

**Planning complete; implementation and evidence are pending.** This is the plan-phase evidence skeleton — no scope is Done and no DoD box in [scopes.md](scopes.md) is checked. The implement/test phase delivers SCOPE-01..03 and records per-DoD-item raw output under the anchors below; SCOPE-04 consolidates the full suites and the live home-lab deploy proof. The spec must not be marked `done` until every scope is Done with ≥10-line raw evidence per `[x]` item, the spec-091/092 regression is proven byte-identical, the atomic single-use is proven under concurrency, the CSP guard is green, and `artifact-lint` + the state-transition guard exit 0.

## Test Evidence

ALL test types required per the [scopes.md](scopes.md) Test Plans. Each scope's per-DoD-item evidence is recorded under its anchored subsection below — verbatim terminal output, ≥10 lines per item, each tagged `**Claim Source:**` (`executed` | `interpreted` | `not-run`). Evidence-block format per item:

```
**Phase:** <phase-name>
**Command:** <exact command executed>
**Exit Code:** <actual exit code>
**Claim Source:** <executed | interpreted | not-run>
<raw output, ≥10 lines>
```

---

## SCOPE-01 — Migration `058` + `webinvite` repo + `webcreds.HashAndInsertTx`

#### scope-01-impl
**Phase:** implement
**Command:** `git status --short` + `git diff --stat -- internal/auth/webcreds/repo.go` + symbol-landing grep across migration / webinvite repo / webcreds.HashAndInsertTx
**Exit Code:** 0
**Claim Source:** executed
```text
=== git status (SCOPE-01 surface) ===
 M internal/auth/webcreds/repo.go
?? internal/auth/webcreds/hashandinsert_test.go
?? internal/auth/webinvite/
?? internal/db/migrations/058_web_registration_invites.sql
?? tests/integration/web_registration_invite_test.go
=== git diff --stat ===
 internal/auth/webcreds/repo.go | 34 ++++++++++++++++++++++++++++++++++
 1 file changed, 34 insertions(+)
=== symbol landing grep (internal/auth/webinvite/repo.go) ===
86:type Repo interface {
115:func HashToken(plaintext string) string {
141:func NewPostgresRepo(pool *pgxpool.Pool) (*PostgresRepo, error) {
150:func (r *PostgresRepo) Generate(ctx context.Context, createdBy, label string, ttl time.Duration) (string, error) {
190:func (r *PostgresRepo) IsLive(ctx context.Context, tokenHash string) (bool, error) {
212:func (r *PostgresRepo) ConsumeAndCreate(ctx context.Context, tokenHash, usedBy string,
255:func (r *PostgresRepo) List(ctx context.Context) ([]InviteRow, error) {
285:func (r *PostgresRepo) Revoke(ctx context.Context, id string) (RevokeOutcome, error) {
=== internal/auth/webcreds/repo.go ===
148:func HashAndInsertTx(ctx context.Context, tx pgx.Tx, username, password string) error {
=== internal/db/migrations/058_web_registration_invites.sql ===
15:CREATE TABLE IF NOT EXISTS web_registration_invites (
17:    token_hash  TEXT        NOT NULL UNIQUE,
```
Migration `058` + the `internal/auth/webinvite` package (`Generate`/`IsLive`/`ConsumeAndCreate`/`List`/`Revoke`/`HashToken`/`NewPostgresRepo` nil-guard) + `webcreds.HashAndInsertTx` landed per design.md; the hash-only table has NO plaintext column, `UpsertPassword` is unchanged, and the `webcreds.Repo` interface is untouched (additive free function).

#### scope-01-generate
**Phase:** implement
**Command:** `./smackerel.sh test unit --go --verbose --go-run 'TestWebInvite_Generate|TestHashToken_Deterministic|TestNewPostgresRepo_NilGuard'`
**Exit Code:** 0
**Claim Source:** executed
```text
[go-unit] applying -run selector: TestWebInvite_Generate|TestHashToken_Deterministic|TestNewPostgresRepo_NilGuard
[go-unit] starting go test ./...
ok      github.com/smackerel/smackerel/internal/auth/webcreds   0.210s [no tests to run]
=== RUN   TestWebInvite_GenerateTokenShapeAndHash
--- PASS: TestWebInvite_GenerateTokenShapeAndHash (0.00s)
=== RUN   TestHashToken_Deterministic
--- PASS: TestHashToken_Deterministic (0.00s)
=== RUN   TestNewPostgresRepo_NilGuard
--- PASS: TestNewPostgresRepo_NilGuard (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/auth/webinvite  0.014s
```
The token format (`inv_` + 43-char RawURLEncoding of 32 random bytes, total 47, no padding/`=+/`, non-repeating across 64 mints), `HashToken` determinism (lowercase-hex SHA-256 covering the whole string incl. the `inv_` prefix), and the `NewPostgresRepo(nil)` refusal are validated without a DB. The DB-backed `Generate` hashed-at-rest + plaintext-once half is proven in #scope-01-consume-singleuse's lane run (`TestWebInvite_Generate_StoresHashOnly`).

#### scope-01-consume-singleuse
**Phase:** implement
**Command:** `./smackerel.sh test integration --go-run 'TestWebInvite'` (live ephemeral Postgres; migration 058 applied; all 8 SCOPE-01 DB-backed scenarios ran in this one lane invocation — full verdict below, referenced by the sibling integration anchors)
**Exit Code:** 0
**Claim Source:** executed

This item proves `TestWebInvite_ConsumeAndCreate_SingleUse`: the atomic claim+create sets `used_at`/`used_by`, and a SECOND consume of the same hash (different username) returns `ConsumeInvalid` with no second account and `used_by` unchanged.
```text
go-integration: applying -run selector: TestWebInvite
=== RUN   TestWebInvite_Generate_StoresHashOnly
--- PASS: TestWebInvite_Generate_StoresHashOnly (0.04s)
=== RUN   TestWebInvite_ConsumeAndCreate_SingleUse
--- PASS: TestWebInvite_ConsumeAndCreate_SingleUse (0.15s)
=== RUN   TestWebInvite_ConcurrentConsume
--- PASS: TestWebInvite_ConcurrentConsume (0.07s)
=== RUN   TestWebInvite_Expired
--- PASS: TestWebInvite_Expired (0.04s)
=== RUN   TestWebInvite_DuplicateUsernameRollsBack
--- PASS: TestWebInvite_DuplicateUsernameRollsBack (0.07s)
=== RUN   TestWebInvite_List
--- PASS: TestWebInvite_List (0.08s)
=== RUN   TestWebInvite_Revoke
--- PASS: TestWebInvite_Revoke (0.04s)
=== RUN   TestWebInvite_Migration058Applies
--- PASS: TestWebInvite_Migration058Applies (0.04s)
ok      github.com/smackerel/smackerel/tests/integration        0.646s
PASS: go-integration
```

#### scope-01-concurrent
**Phase:** implement
**Command:** `./smackerel.sh test integration --go-run 'TestWebInvite'`
**Exit Code:** 0
**Claim Source:** executed

This item proves `TestWebInvite_ConcurrentConsume`: two goroutines race `ConsumeAndCreate` on one invite hash (released together via a start channel); exactly one returns `ConsumeCreated` and one `ConsumeInvalid`, and exactly one `web_user_credentials` row exists — no TOCTOU double-spend. (Same lane invocation that validated all 8 SCOPE-01 DB scenarios.)
```text
go-integration: applying -run selector: TestWebInvite
=== RUN   TestWebInvite_ConcurrentConsume
--- PASS: TestWebInvite_ConcurrentConsume (0.07s)
=== RUN   TestWebInvite_Expired
--- PASS: TestWebInvite_Expired (0.04s)
=== RUN   TestWebInvite_DuplicateUsernameRollsBack
--- PASS: TestWebInvite_DuplicateUsernameRollsBack (0.07s)
=== RUN   TestWebInvite_List
--- PASS: TestWebInvite_List (0.08s)
=== RUN   TestWebInvite_Revoke
--- PASS: TestWebInvite_Revoke (0.04s)
=== RUN   TestWebInvite_Migration058Applies
--- PASS: TestWebInvite_Migration058Applies (0.04s)
ok      github.com/smackerel/smackerel/tests/integration        0.646s
PASS: go-integration
```

#### scope-01-expired
**Phase:** implement
**Command:** `./smackerel.sh test integration --go-run 'TestWebInvite'`
**Exit Code:** 0
**Claim Source:** executed

This item proves `TestWebInvite_Expired`: an invite directly seeded with `expires_at = now() - interval '1 hour'` reports `IsLive=false` AND `ConsumeAndCreate` returns `ConsumeInvalid` and creates no account. (Same lane invocation that validated all 8 SCOPE-01 DB scenarios.)
```text
go-integration: applying -run selector: TestWebInvite
=== RUN   TestWebInvite_Generate_StoresHashOnly
--- PASS: TestWebInvite_Generate_StoresHashOnly (0.04s)
=== RUN   TestWebInvite_ConsumeAndCreate_SingleUse
--- PASS: TestWebInvite_ConsumeAndCreate_SingleUse (0.15s)
=== RUN   TestWebInvite_Expired
--- PASS: TestWebInvite_Expired (0.04s)
=== RUN   TestWebInvite_DuplicateUsernameRollsBack
--- PASS: TestWebInvite_DuplicateUsernameRollsBack (0.07s)
=== RUN   TestWebInvite_Revoke
--- PASS: TestWebInvite_Revoke (0.04s)
ok      github.com/smackerel/smackerel/tests/integration        0.646s
PASS: go-integration
```

#### scope-01-dup-rollback
**Phase:** implement
**Command:** `./smackerel.sh test integration --go-run 'TestWebInvite'`
**Exit Code:** 0
**Claim Source:** executed

This item proves `TestWebInvite_DuplicateUsernameRollsBack`: a live invite + a taken username makes `HashAndInsertTx` return `ErrUserExists`; `ConsumeAndCreate` returns `ConsumeRolledBack`, the whole tx unwinds, the invite's `used_at` stays NULL, and `IsLive` is still true (retriable). (Same lane invocation that validated all 8 SCOPE-01 DB scenarios.)
```text
go-integration: applying -run selector: TestWebInvite
=== RUN   TestWebInvite_ConsumeAndCreate_SingleUse
--- PASS: TestWebInvite_ConsumeAndCreate_SingleUse (0.15s)
=== RUN   TestWebInvite_ConcurrentConsume
--- PASS: TestWebInvite_ConcurrentConsume (0.07s)
=== RUN   TestWebInvite_Expired
--- PASS: TestWebInvite_Expired (0.04s)
=== RUN   TestWebInvite_DuplicateUsernameRollsBack
--- PASS: TestWebInvite_DuplicateUsernameRollsBack (0.07s)
=== RUN   TestWebInvite_List
--- PASS: TestWebInvite_List (0.08s)
ok      github.com/smackerel/smackerel/tests/integration        0.646s
PASS: go-integration
```

#### scope-01-list
**Phase:** implement
**Command:** `./smackerel.sh test integration --go-run 'TestWebInvite'`
**Exit Code:** 0
**Claim Source:** executed

This item proves `TestWebInvite_List`: across mixed-state invites (outstanding/used/revoked/expired) every `InviteRow` carries the correct derived `Status`, and an adversarial scan asserts NO row field equals any live plaintext OR its hash (the projection never selects `token_hash`). (Same lane invocation that validated all 8 SCOPE-01 DB scenarios.)
```text
go-integration: applying -run selector: TestWebInvite
=== RUN   TestWebInvite_Expired
--- PASS: TestWebInvite_Expired (0.04s)
=== RUN   TestWebInvite_DuplicateUsernameRollsBack
--- PASS: TestWebInvite_DuplicateUsernameRollsBack (0.07s)
=== RUN   TestWebInvite_List
--- PASS: TestWebInvite_List (0.08s)
=== RUN   TestWebInvite_Revoke
--- PASS: TestWebInvite_Revoke (0.04s)
=== RUN   TestWebInvite_Migration058Applies
--- PASS: TestWebInvite_Migration058Applies (0.04s)
ok      github.com/smackerel/smackerel/tests/integration        0.646s
PASS: go-integration
```

#### scope-01-revoke
**Phase:** implement
**Command:** `./smackerel.sh test integration --go-run 'TestWebInvite'`
**Exit Code:** 0
**Claim Source:** executed

This item proves `TestWebInvite_Revoke`: an OUTSTANDING invite revokes to `RevokeDone` with `revoked_at` set and `IsLive=false`; a repeat revoke and an unknown-id revoke both return `RevokeNoop` and change nothing. (Same lane invocation that validated all 8 SCOPE-01 DB scenarios.)
```text
go-integration: applying -run selector: TestWebInvite
=== RUN   TestWebInvite_DuplicateUsernameRollsBack
--- PASS: TestWebInvite_DuplicateUsernameRollsBack (0.07s)
=== RUN   TestWebInvite_List
--- PASS: TestWebInvite_List (0.08s)
=== RUN   TestWebInvite_Revoke
--- PASS: TestWebInvite_Revoke (0.04s)
=== RUN   TestWebInvite_Migration058Applies
--- PASS: TestWebInvite_Migration058Applies (0.04s)
ok      github.com/smackerel/smackerel/tests/integration        0.646s
PASS: go-integration
```

#### scope-01-hashandinsert
**Phase:** implement
**Command:** `./smackerel.sh test unit --go --verbose --go-run 'TestHashAndInsertTx'`
**Exit Code:** 0
**Claim Source:** executed
```text
[go-unit] applying -run selector: TestHashAndInsertTx
[go-unit] starting go test ./...
ok      github.com/smackerel/smackerel/internal/auth/revocation 0.014s [no tests to run]
=== RUN   TestHashAndInsertTx_Success
--- PASS: TestHashAndInsertTx_Success (0.23s)
=== RUN   TestHashAndInsertTx_MapsUniqueViolation
--- PASS: TestHashAndInsertTx_MapsUniqueViolation (0.24s)
=== RUN   TestHashAndInsertTx_WrapsOtherError
--- PASS: TestHashAndInsertTx_WrapsOtherError (0.23s)
=== RUN   TestHashAndInsertTx_RejectsBadUsernameBeforeExec
--- PASS: TestHashAndInsertTx_RejectsBadUsernameBeforeExec (0.00s)
=== RUN   TestHashAndInsertTx_RejectsShortPasswordBeforeExec
--- PASS: TestHashAndInsertTx_RejectsShortPasswordBeforeExec (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/auth/webcreds   0.996s
```
The unit layer fakes ONLY the external `pgx.Tx` boundary: `23505` → `ErrUserExists`, a non-23505 error is wrapped (not mapped), success returns nil, and a bad username / short password is rejected BEFORE `Exec` runs. The REAL Postgres `23505` round-trip is additionally proven by `TestWebInvite_DuplicateUsernameRollsBack` (#scope-01-dup-rollback).

#### scope-01-migration
**Phase:** implement
**Command:** `./smackerel.sh test integration --go-run 'TestWebInvite'`
**Exit Code:** 0
**Claim Source:** executed

This item proves `TestWebInvite_Migration058Applies`: `db.Migrate` applies `058` on the live test DB; `information_schema.columns` confirms the table exists with all 9 expected columns, `token_hash` is `is_nullable = NO`, and NO plaintext-bearing column (`token`/`plaintext`/`token_plain`) exists. (Same lane invocation that validated all 8 SCOPE-01 DB scenarios.)
```text
go-integration: applying -run selector: TestWebInvite
=== RUN   TestWebInvite_Generate_StoresHashOnly
--- PASS: TestWebInvite_Generate_StoresHashOnly (0.04s)
=== RUN   TestWebInvite_List
--- PASS: TestWebInvite_List (0.08s)
=== RUN   TestWebInvite_Revoke
--- PASS: TestWebInvite_Revoke (0.04s)
=== RUN   TestWebInvite_Migration058Applies
--- PASS: TestWebInvite_Migration058Applies (0.04s)
ok      github.com/smackerel/smackerel/tests/integration        0.646s
PASS: go-integration
```

#### scope-01-build-gate
**Phase:** implement
**Command:** `./smackerel.sh check` + `./smackerel.sh lint` + `./smackerel.sh format --check` + `bash .github/bubbles/scripts/artifact-lint.sh specs/093-admin-generated-registration-invites`
**Exit Code:** 0 (all four)
**Claim Source:** executed
```text
$ ./smackerel.sh check
config-validate: .../config/generated/dev.env.tmp OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 16, rejected: 0
scenario-lint: OK

$ ./smackerel.sh lint
All checks passed!            # ruff (python ML)
=== Validating web manifests ===  OK
=== Validating JS syntax ===      OK
Web validation passed            # (go vet ran first and was clean — set -e reached web stage)

$ ./smackerel.sh format --check
65 files already formatted

$ bash .github/bubbles/scripts/artifact-lint.sh specs/093-admin-generated-registration-invites
Artifact lint PASSED.
ARTIFACT_LINT_EXIT=0
```
No `${VAR:-default}` / `${VAR-default}` fallback introduced — SCOPE-01 adds Go + one forward SQL migration only (no Compose/env-file edits). Zero warnings, zero deferrals.

---

## SCOPE-02 — `/register` OR-gate (DB-invite consume) + wiring

#### scope-02-impl
**Phase:** implement
**Command:** `git diff --stat` + OR-gate / DB-branch / field / wiring symbol grep across web_register.go / health.go / wiring.go / router.go
**Exit Code:** 0
**Claim Source:** executed
```text
=== git diff --stat (SCOPE-02 surface) ===
 cmd/core/wiring.go           | 13 +++++++
 internal/api/health.go       |  8 ++++
 internal/api/web_register.go | 90 ++++++++++++++++++++++++++++++++++----------
 3 files changed, 92 insertions(+), 19 deletions(-)
=== OR-gate + DB-branch landing (web_register.go) ===
88:  // Step 3 — invite gate FIRST (OR-gate: static secret OR live DB invite),
110: staticOK := configured != "" &&
117: dbLive := false
118: if !staticOK && d.WebInvites != nil {
119:   if live, err := d.WebInvites.IsLive(r.Context(), webinvite.HashToken(invite)); err == nil {
163: if staticOK {
181:   outcome, err := d.WebInvites.ConsumeAndCreate(r.Context(), webinvite.HashToken(invite), username,
183:     return webcreds.HashAndInsertTx(ctx, tx, username, password)
194:   case outcome == webinvite.ConsumeInvalid:
=== Dependencies.WebInvites field (health.go) ===
187: WebInvites webinvite.Repo
=== wiring fan-out (wiring.go) ===
921: inviteRepo, err := webinvite.NewPostgresRepo(svc.pg.Pool)
926: deps.WebInvites = inviteRepo
=== router.go spec-093 refs (must be 0 — unchanged) ===
0
```
The OR-gate (`staticOK OR dbLive`, disabled fail-loud, value-safe IsLive), the Step-7 DB-invite atomic branch (`ConsumeAndCreate` + `HashAndInsertTx`, outcome switch), the `Dependencies.WebInvites` field, and the `cmd/core/wiring.go` fan-out landed per design.md; `logRegisterReject` enum unchanged; `internal/api/router.go` has zero spec-093 references (unchanged).

#### scope-02-gate-branch
**Phase:** implement
**Command:** `./smackerel.sh test unit --go --verbose --go-run 'WebRegister'`
**Exit Code:** 0
**Claim Source:** executed
```text
=== RUN   TestWebRegister_OrGate
=== RUN   TestWebRegister_OrGate/static-first-consumes-nothing
=== RUN   TestWebRegister_OrGate/db-second-when-no-static-configured
=== RUN   TestWebRegister_OrGate/db-second-when-static-mismatch
=== RUN   TestWebRegister_OrGate/disabled-nil-credentials-store
=== RUN   TestWebRegister_OrGate/disabled-empty-static-and-no-invite-store
--- PASS: TestWebRegister_OrGate (0.00s)
    --- PASS: TestWebRegister_OrGate/static-first-consumes-nothing (0.00s)
    --- PASS: TestWebRegister_OrGate/db-second-when-no-static-configured (0.00s)
    --- PASS: TestWebRegister_OrGate/db-second-when-static-mismatch (0.00s)
    --- PASS: TestWebRegister_OrGate/disabled-nil-credentials-store (0.00s)
    --- PASS: TestWebRegister_OrGate/disabled-empty-static-and-no-invite-store (0.00s)
ok      github.com/smackerel/smackerel/internal/api     0.215s
```
Static-first (the configured secret is matched and consumes NOTHING — `ConsumeAndCreate` call count 0), DB-second (taken when no static is configured AND when a static IS configured but the submitted token isn't it — routed to `ConsumeAndCreate` with `HashToken(invite)` + the username), and both disabled paths (nil credentials store; empty static + no invite store) all behave per design.md branch order.

#### scope-02-nonenum
**Phase:** implement
**Command:** `./smackerel.sh test unit --go --verbose --go-run 'WebRegister'`
**Exit Code:** 0
**Claim Source:** executed
```text
=== RUN   TestWebRegister_NonEnumerating
--- PASS: TestWebRegister_NonEnumerating (0.00s)
=== RUN   TestWebRegister_OrGate
--- PASS: TestWebRegister_OrGate (0.00s)
=== RUN   TestWebRegister_Gate
--- PASS: TestWebRegister_Gate (0.00s)
    --- PASS: TestWebRegister_Gate/wrong-token (0.00s)
    --- PASS: TestWebRegister_Gate/missing-token (0.00s)
    --- PASS: TestWebRegister_Gate/empty-configured (0.00s)
    --- PASS: TestWebRegister_Gate/empty-configured-empty-submitted (0.00s)
    --- PASS: TestWebRegister_Gate/nil-store (0.00s)
ok      github.com/smackerel/smackerel/internal/api     0.215s
```
`TestWebRegister_NonEnumerating` asserts the response bodies for DB-invalid, static-wrong-with-invite-store, static-wrong-without-store, and disabled are ALL byte-identical (`bodyX == dbInvalidBody`) — same 401, same `registerGateBanner`, same blank-secret re-render. The response shape never distinguishes a bad DB invite from a wrong static secret from the disabled case.

#### scope-02-consume
**Phase:** implement
**Command:** `./smackerel.sh test integration --go-run 'TestWebRegisterIntegration'` (live ephemeral Postgres; real webcreds + webinvite repos driving HandleWebRegister via httptest)
**Exit Code:** 0
**Claim Source:** executed

This item proves `TestWebRegisterIntegration_DBInviteConsumes`: a new person registers once with a live DB invite — the account row is created with an `$argon2id$v=19$` hash AND the invite's `used_at`/`used_by` are set in the SAME tx; the response is 303 → `/login?registered=1` with NO `Set-Cookie`.
```text
go-integration: applying -run selector: TestWebRegisterIntegration
=== RUN   TestWebRegisterIntegration_DBInviteConsumes
--- PASS: TestWebRegisterIntegration_DBInviteConsumes (0.14s)
=== RUN   TestWebRegisterIntegration_ReusedInviteRejected
--- PASS: TestWebRegisterIntegration_ReusedInviteRejected (0.07s)
=== RUN   TestWebRegisterIntegration_DuplicateUsernameRollsBack
--- PASS: TestWebRegisterIntegration_DuplicateUsernameRollsBack (0.11s)
=== RUN   TestWebRegisterIntegration_StaticSecretConsumesNothing
--- PASS: TestWebRegisterIntegration_StaticSecretConsumesNothing (0.06s)
ok      github.com/smackerel/smackerel/tests/integration        0.514s
PASS: go-integration
```

#### scope-02-reuse
**Phase:** implement
**Command:** `./smackerel.sh test integration --go-run 'TestWebRegisterIntegration'`
**Exit Code:** 0
**Claim Source:** executed

This item proves `TestWebRegisterIntegration_ReusedInviteRejected`: after one successful registration consumes the invite, a SECOND register with the same token + a different username returns 401 + the generic banner, creates NO second account, and leaves `used_by` = the first user. (Same lane invocation that validated all 4 SCOPE-02 register scenarios.)
```text
go-integration: applying -run selector: TestWebRegisterIntegration
=== RUN   TestWebRegisterIntegration_DBInviteConsumes
--- PASS: TestWebRegisterIntegration_DBInviteConsumes (0.14s)
=== RUN   TestWebRegisterIntegration_ReusedInviteRejected
--- PASS: TestWebRegisterIntegration_ReusedInviteRejected (0.07s)
=== RUN   TestWebRegisterIntegration_DuplicateUsernameRollsBack
--- PASS: TestWebRegisterIntegration_DuplicateUsernameRollsBack (0.11s)
ok      github.com/smackerel/smackerel/tests/integration        0.514s
PASS: go-integration
```

#### scope-02-dup
**Phase:** implement
**Command:** `./smackerel.sh test integration --go-run 'TestWebRegisterIntegration'`
**Exit Code:** 0
**Claim Source:** executed

This item proves `TestWebRegisterIntegration_DuplicateUsernameRollsBack`: registering on the DB-invite path with an already-taken username returns 409 + the spec-091 duplicate banner, the invite's `used_at` stays NULL (whole-tx rollback), and the SAME invite then succeeds with a fresh username (retriable — 303). (Same lane invocation that validated all 4 SCOPE-02 register scenarios.)
```text
go-integration: applying -run selector: TestWebRegisterIntegration
=== RUN   TestWebRegisterIntegration_ReusedInviteRejected
--- PASS: TestWebRegisterIntegration_ReusedInviteRejected (0.07s)
=== RUN   TestWebRegisterIntegration_DuplicateUsernameRollsBack
--- PASS: TestWebRegisterIntegration_DuplicateUsernameRollsBack (0.11s)
=== RUN   TestWebRegisterIntegration_StaticSecretConsumesNothing
--- PASS: TestWebRegisterIntegration_StaticSecretConsumesNothing (0.06s)
ok      github.com/smackerel/smackerel/tests/integration        0.514s
PASS: go-integration
```

#### scope-02-static
**Phase:** implement
**Command:** `./smackerel.sh test integration --go-run 'TestWebRegisterIntegration'`
**Exit Code:** 0
**Claim Source:** executed

This item proves `TestWebRegisterIntegration_StaticSecretConsumesNothing`: with a configured static secret, registering via the static token creates the account (303) while a co-existing OUTSTANDING DB invite is left completely UNTOUCHED (its `used_at` stays NULL and `IsLive` is still true) — the static path is reusable bootstrap that consumes no invite. (Same lane invocation that validated all 4 SCOPE-02 register scenarios.)
```text
go-integration: applying -run selector: TestWebRegisterIntegration
=== RUN   TestWebRegisterIntegration_DBInviteConsumes
--- PASS: TestWebRegisterIntegration_DBInviteConsumes (0.14s)
=== RUN   TestWebRegisterIntegration_DuplicateUsernameRollsBack
--- PASS: TestWebRegisterIntegration_DuplicateUsernameRollsBack (0.11s)
=== RUN   TestWebRegisterIntegration_StaticSecretConsumesNothing
--- PASS: TestWebRegisterIntegration_StaticSecretConsumesNothing (0.06s)
ok      github.com/smackerel/smackerel/tests/integration        0.514s
PASS: go-integration
```

#### scope-02-091-regression
**Phase:** implement
**Command:** `./smackerel.sh test unit --go --verbose --go-run 'WebRegister'`
**Exit Code:** 0
**Claim Source:** executed

The spec-091 register suite (`internal/api/web_register_test.go` + `web_register_ratelimit_test.go`) is unit-tier (httptest + the in-memory webcreds fake, no DB) and runs in the `test unit` lane. With `WebInvites == nil` the widened OR-gate degrades to exactly the spec-091 static behaviour — every existing scenario passes byte-identically after the gate edit, proving zero regression.
```text
--- PASS: TestWebRegister_Success (0.00s)
--- PASS: TestWebRegister_Gate (0.00s)
    --- PASS: TestWebRegister_Gate/wrong-token (0.00s)
    --- PASS: TestWebRegister_Gate/missing-token (0.00s)
    --- PASS: TestWebRegister_Gate/empty-configured (0.00s)
    --- PASS: TestWebRegister_Gate/empty-configured-empty-submitted (0.00s)
    --- PASS: TestWebRegister_Gate/nil-store (0.00s)
--- PASS: TestWebRegister_Duplicate (0.00s)
--- PASS: TestWebRegister_FieldValidation (0.00s)
--- PASS: TestWebRegister_NonEnumeration (0.00s)
--- PASS: TestWebRegister_ValueSafeLog (0.00s)
--- PASS: TestWebRegister_MethodGuard (0.00s)
--- PASS: TestWebRegister_RateLimited_PerIP (0.00s)
--- PASS: TestWebRegister_RateLimit_PerIP_FreshIPAdmitted (0.00s)
ok      github.com/smackerel/smackerel/internal/api     0.215s
```

#### scope-02-build-gate
**Phase:** implement
**Command:** `./smackerel.sh lint` + `./smackerel.sh format --check` + value-safe `slog` audit + `artifact-lint.sh`
**Exit Code:** 0 (all)
**Claim Source:** executed
```text
$ ./smackerel.sh lint
... go vet clean; ruff All checks passed!; Web validation passed
LINT_EXIT=0

$ ./smackerel.sh format --check
65 files already formatted

$ value-safe slog audit (token/hash/password must NEVER be logged)
internal/api/web_register.go:241:  slog.Info("web registration rejected", ... username_len ... reason)  # no token/hash/password
internal/auth/webinvite/repo.go:   (no slog calls)
cmd/core/wiring.go:923:  slog.Error("card-rewards: invite repo construction failed", "error", err)  # non-secret

$ bash .github/bubbles/scripts/artifact-lint.sh specs/093-admin-generated-registration-invites
Artifact lint PASSED.
```
Value-safe: the invite plaintext, its hash, and the password are NEVER logged — the only register-path `slog` is the unchanged `logRegisterReject` (`username_len` + coarse `reason` enum). No `${VAR:-default}` introduced (Go-only edits, no Compose/env). Zero warnings, zero deferrals.

---

## SCOPE-03 — Admin invites UI (generate / list / revoke)

#### scope-03-impl
**Phase:** implement
**Command:** `git status --short` + handler/route/template/link/CSP landing grep
**Exit Code:** 0
**Claim Source:** executed
```text
=== git status (SCOPE-03 surface) ===
 M cmd/core/wiring.go
 M internal/web/cardrewards.go
 M internal/web/cardrewards_dashboard_templates.go
?? internal/web/invites.go
?? internal/web/invites_templates.go
?? internal/web/invites_test.go
?? web/pwa/tests/cardrewards_invites.spec.ts
=== handler methods + SetInvites (invites.go) ===
99:func (h *CardRewardsWebHandler) SetInvites(r webinvite.Repo) { h.Invites = r }
103:func (h *CardRewardsWebHandler) AdminInvitesPage(w http.ResponseWriter, r *http.Request) {
127:func (h *CardRewardsWebHandler) AdminInviteGenerate(w http.ResponseWriter, r *http.Request) {
178:func (h *CardRewardsWebHandler) AdminInviteRevoke(w http.ResponseWriter, r *http.Request) {
200:func sessionIdentity(r *http.Request) string {
=== /invites sub-route inside the existing /cards/admin block (cardrewards.go) ===
143:    template.Must(t.Parse(cardRewardsInviteTemplates))
206:    r.Get("/", h.AdminInvitesPage)              // GET  /cards/admin/invites
207:    r.Post("/", h.AdminInviteGenerate)          // POST /cards/admin/invites (200 render-once)
208:    r.Post("/{id}/revoke", h.AdminInviteRevoke) // POST /cards/admin/invites/{id}/revoke (303 PRG)
=== /cards/admin link + SetInvites wiring ===
246:  <a class="btn btn-secondary" href="/cards/admin/invites" data-action="account-invites">Account Invites &rarr;</a>
927:    webHandler.SetInvites(inviteRepo)
=== router.go spec-093 refs (must be 0) + inline <script>/onclick/onsubmit in invite templates (must be 0) ===
0
0
```
`internal/web/invites.go` (3 handler methods + `SetInvites` + `sessionIdentity` + view models), `internal/web/invites_templates.go` (chrome-reusing list + reveal + shared table; token only on reveal), the `/invites` sub-route in the existing `/cards/admin` block, the `/cards/admin` “Account Invites →” link, and the `webHandler.SetInvites` wiring landed per design.md; CSP-clean (zero inline scripts/handlers); `internal/api/router.go` has zero spec-093 references.

#### scope-03-page
**Phase:** implement
**Command:** `./smackerel.sh test unit --go --verbose --go-run 'TestAdminInvite'`
**Exit Code:** 0
**Claim Source:** executed
```text
=== RUN   TestAdminInvitesPage
--- PASS: TestAdminInvitesPage (0.01s)
    --- PASS: TestAdminInvitesPage/503-when-nil (0.00s)
    --- PASS: TestAdminInvitesPage/metadata-only-render (0.00s)
    --- PASS: TestAdminInvitesPage/empty-state (0.00s)
    --- PASS: TestAdminInvitesPage/race-notice (0.00s)
ok      github.com/smackerel/smackerel/internal/web     0.244s
```
`metadata-only-render` asserts the list contains the labels / created-by / status badges (outstanding/used/revoked, “by newcomer-x”), contains NO `inv_` token, has exactly ONE revoke form (only the outstanding row), and is CSP-clean (no `<script`/`onclick`/`onsubmit`). `503-when-nil` proves the guard when `Invites == nil`. `empty-state` + `race-notice` cover the no-invites and `?notice=race` paths.

#### scope-03-generate
**Phase:** implement
**Command:** `./smackerel.sh test unit --go --verbose --go-run 'TestAdminInvite'`
**Exit Code:** 0
**Claim Source:** executed
```text
=== RUN   TestAdminInviteGenerate
--- PASS: TestAdminInviteGenerate (0.01s)
    --- PASS: TestAdminInviteGenerate/200-one-time-reveal (0.00s)
    --- PASS: TestAdminInviteGenerate/value-safe-error (0.00s)
    --- PASS: TestAdminInviteGenerate/503-when-nil (0.00s)
ok      github.com/smackerel/smackerel/internal/web     0.244s
```
`200-one-time-reveal` proves POST generate ⇒ HTTP 200 (NOT a redirect — no `Location`), the `data-onetime-token-reveal` callout, and the token appearing EXACTLY once in a `readonly` `data-onetime-token` field, CSP-clean. `value-safe-error` proves the error path is 500 and echoes NO token (`inv_` absent) while showing the value-safe banner.

#### scope-03-revoke
**Phase:** implement
**Command:** `./smackerel.sh test unit --go --verbose --go-run 'TestAdminInvite'`
**Exit Code:** 0
**Claim Source:** executed
```text
=== RUN   TestAdminInviteRevoke
--- PASS: TestAdminInviteRevoke (0.00s)
    --- PASS: TestAdminInviteRevoke/done-303-prg (0.00s)
    --- PASS: TestAdminInviteRevoke/noop-303-with-race-notice (0.00s)
    --- PASS: TestAdminInviteRevoke/503-when-nil (0.00s)
ok      github.com/smackerel/smackerel/internal/web     0.244s
```
`done-303-prg` proves POST revoke ⇒ 303 PRG with `Location: /cards/admin/invites` and the `{id}` chi URL param reaching the repo (`lastRevoke == "abc-123"`). `noop-303-with-race-notice` proves a `RevokeNoop` redirects to `/cards/admin/invites?notice=race` (non-enumerating). `503-when-nil` covers the nil guard.

#### scope-03-anon
**Phase:** implement
**Command:** `./smackerel.sh test unit --go --verbose --go-run 'TestAdminInvite'` (Go group-gating) + `./smackerel.sh test e2e-ui cardrewards` (REAL webAuthMiddleware, live stack)
**Exit Code:** 0
**Claim Source:** executed
```text
=== RUN   TestAdminInvites_AnonymousBlocked
--- PASS: TestAdminInvites_AnonymousBlocked (0.00s)
    --- PASS: TestAdminInvites_AnonymousBlocked/anonymous-get-list (0.00s)
    --- PASS: TestAdminInvites_AnonymousBlocked/anonymous-post-generate (0.00s)
    --- PASS: TestAdminInvites_AnonymousBlocked/anonymous-post-revoke (0.00s)
    --- PASS: TestAdminInvites_AnonymousBlocked/authenticated-reaches-handler (0.00s)
ok      github.com/smackerel/smackerel/internal/web     0.244s
```
The Go test mounts `RegisterRoutes` behind a faithful auth gate and proves all THREE invite routes are 401 when anonymous and reach the handler (503 nil-Invites, NOT 401) when authenticated — catching a mis-registration outside the gated group. The REAL `webAuthMiddleware` rejecting anonymous live is additionally proven by the e2e-ui `SCN-093-16` step (see #scope-03-e2e: `anonymous /cards/admin/invites must be rejected`).

#### scope-03-e2e
**Phase:** implement
**Command:** `./smackerel.sh test e2e-ui cardrewards` (live disposable `smackerel-test-e2e-ui` stack, freshly rebuilt core image)
**Exit Code:** 0
**Claim Source:** executed

This item proves the live-stack journey `SCN-093-13/14/15/16/17` (line 8): login → click the `/cards/admin` “Account Invites →” link → generate → one-time reveal (token once, `readOnly`) → Done → list (row `data-invite-status="outstanding"`, token ABSENT from the DOM) → `<details>` revoke → row `data-invite-status="revoked"` → anonymous `/cards/admin/invites` rejected.
```text
Running 20 tests using 4 workers
  ✓  8 …dmin link → generate → reveal → list → revoke → anonymous-blocked (3.2s)
  ✓  9 … CSP) › SCOPE-01-C — CSP-clean across representative /cards pages (1.8s)
  ✓  1 …083-K07 — scrape now runs the refresh pipeline and logs a new run (2.4s)
  ✓  4 …onsive nav: mobile scroll-strip, desktop wrap, 44px pills, sticky (2.6s)
  ✓ 20 … Card Rewards Wallet › SCN-083-J05 — toggle card activation off (696ms)
  20 passed (15.6s)
```

#### scope-03-csp
**Phase:** implement
**Command:** `./smackerel.sh test e2e-ui cardrewards`
**Exit Code:** 0
**Claim Source:** executed

The spec-093 journey (line 8) ends with `assertNoCSPViolations(page)` after the full generate → reveal → list → revoke walk — the spec-077 CSP guard recorded ZERO violations (the test passed). The adversarial token-absent assertion (`expect(await page.content()).not.toContain(token)` on the GET list) is part of the same passing test. The cross-cutting spec-077 CSP sweep over the /cards pages (line 9) also passed.
```text
Running 20 tests using 4 workers
  ✓  8 …dmin link → generate → reveal → list → revoke → anonymous-blocked (3.2s)
  ✓  9 … CSP) › SCOPE-01-C — CSP-clean across representative /cards pages (1.8s)
  ✓  6 …-B — dark-mode token application differs from light (adversarial) (1.6s)
  20 passed (15.6s)
```
(`grep -cE '<script|onclick|onsubmit' internal/web/invites_templates.go` returns 0 — the invite templates are inline-script-free by construction.)

#### scope-03-092-regression
**Phase:** implement
**Command:** `./smackerel.sh test e2e-ui cardrewards`
**Exit Code:** 0
**Claim Source:** executed

The spec-092 `/cards` design-system suite (dashboard, chrome, CSP, responsive nav, dark-mode tokens) renders UNCHANGED after the additive invite template/route edits — every existing cardrewards e2e passes in the same run as the new invites journey.
```text
Running 20 tests using 4 workers
  ✓  2 … hooks and a width-correct progress bar; update-progress sets met (4.1s)
  ✓  4 …onsive nav: mobile scroll-strip, desktop wrap, 44px pills, sticky (2.6s)
  ✓  6 …-B — dark-mode token application differs from light (adversarial) (1.6s)
  ✓  7 …board shows recommendations, active rotating, and pending actions (1.7s)
  ✓  9 … CSP) › SCOPE-01-C — CSP-clean across representative /cards pages (1.8s)
  20 passed (15.6s)
```

#### scope-03-build-gate
**Phase:** implement
**Command:** `./smackerel.sh lint` + `./smackerel.sh format --check` + value-safe `slog` audit + `artifact-lint.sh`
**Exit Code:** 0 (all)
**Claim Source:** executed
```text
$ ./smackerel.sh lint
... go vet clean; ruff All checks passed!; Web validation passed
LINT_DONE_EXIT=0

$ ./smackerel.sh format --check
65 files already formatted

$ value-safe slog audit (invites.go)
invites.go: ZERO slog/log calls (token/hash never logged)

$ grep -cE '<script|onclick|onsubmit' internal/web/invites_templates.go
0          # CSP-clean: no inline scripts / event handlers

$ bash .github/bubbles/scripts/artifact-lint.sh specs/093-admin-generated-registration-invites
Artifact lint PASSED.
```
CSP-clean (no inline `<script>`/handlers in the invite templates), value-safe (the invites handler makes ZERO `slog` calls — the one-time token leaves the process only in the generate-200 body), no `${VAR:-default}` introduced (Go + html/template + a Playwright spec only). Zero warnings, zero deferrals.

---

## SCOPE-04 — Consolidated verification + live home-lab deploy proof

#### scope-04-unit
_Evidence recorded at implement time (test 1 — full unit suite green)._

#### scope-04-integration
_Evidence recorded at implement time (test 2 — full integration suite green: atomic single-use + dup rollback + DB-invite consume + 091 static regression)._

#### scope-04-e2eui
_Evidence recorded at implement time (test 3 — full card-rewards e2e-ui green: invites + CSP guard + 092 dashboard)._

#### scope-04-regression
_Evidence recorded at implement time (test 4 — spec-091 + spec-092 byte-identical regression)._

#### scope-04-live
_Evidence recorded at implement time (test 5 — live home-lab deploy: migration 058 applied; generate→register→used→reuse-rejected; value-safe; live-stack)._

#### scope-04-build-gate
_Evidence recorded at implement time (check + lint + format --check + artifact-lint exit 0; live-stack authenticity; value-safe; no ${VAR:-default})._

---

## Security Review

**Reviewer:** `bubbles.security` · **Phase:** SECURITY (parent-expanded full-delivery run) · **Surface:** SCOPE-01..03 admin-generated single-use registration invites.
**Verdict:** 🔒 **SECURE** — 12/12 threat-model checks PASS. One **Informational** observation (pre-existing shared helper; not a spec-093 defect; not routed). No critical/high/medium findings. Do NOT block on this review.

**Scope reviewed:** `internal/db/migrations/058_web_registration_invites.sql`, `internal/auth/webinvite/repo.go`, `internal/auth/webcreds/repo.go` (`HashAndInsertTx`), `internal/api/web_register.go`, `internal/web/invites.go`, `internal/web/invites_templates.go`, `internal/web/cardrewards.go` (routing + chrome parse), `cmd/core/wiring.go` (fan-out), `internal/api/router.go` (`webAuthMiddleware` mount).

### Executed evidence (Claim Source: executed)

`./smackerel.sh test unit --go --go-run 'WebInvite|AdminInvite|WebRegister|HashAndInsertTx'` — all spec-093 security-relevant packages green:

```
+ go test -run 'WebInvite|AdminInvite|WebRegister|HashAndInsertTx' -count=1 ./...
[go-unit] applying -run selector: WebInvite|AdminInvite|WebRegister|HashAndInsertTx
[go-unit] starting go test ./...
ok      github.com/smackerel/smackerel/internal/api             0.218s
ok      github.com/smackerel/smackerel/internal/auth/webcreds   0.542s
ok      github.com/smackerel/smackerel/internal/auth/webinvite  0.017s
ok      github.com/smackerel/smackerel/internal/web             0.291s
[go-unit] go test ./... finished OK
```

Covering tests exercised: `TestWebInvite_GenerateTokenShapeAndHash`, `TestAdminInvitesPage`, `TestAdminInviteGenerate`, `TestAdminInviteRevoke`, `TestAdminInvites_AnonymousBlocked`, `TestWebRegister_OrGate`, `TestWebRegister_NonEnumerating`, `TestWebRegister_FieldValidation`, `TestWebRegister_ValueSafeLog`, `TestWebRegister_MethodGuard`, `TestHashAndInsertTx_{Success,MapsUniqueViolation,WrapsOtherError,RejectsBadUsernameBeforeExec,RejectsShortPasswordBeforeExec}`.

### Threat-model verdicts (Claim Source: interpreted code-read, confirmed by executed tests above)

| # | Threat | Verdict | Evidence (file:line) |
|---|--------|---------|----------------------|
| 1 | **Single-use is race-safe (no double-spend)** | ✅ PASS | The guarded `UPDATE … SET used_at=now(),used_by=$2 WHERE token_hash=$1 AND used_at IS NULL AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > now()) RETURNING id` in `webinvite/repo.go:212` (SQL at `:225`) is the **sole** claim authority — atomic, not check-then-act. `pgx.ErrNoRows ⇒ ConsumeInvalid` (lost race → generic banner). Account `INSERT` runs via `onClaimed(ctx, tx)` on the **same** tx; on error → `ConsumeRolledBack` with deferred `tx.Rollback` (invite stays usable). `IsLive` (`:190`) is explicitly **not** the authority (web_register.go:181 + design (f)). |
| 2 | **Token at rest (hash-only, high-entropy)** | ✅ PASS | `newPlaintextToken` (`webinvite/repo.go:123`) uses `crypto/rand.Read` (`:125`, NOT math/rand) over 32 bytes (256-bit) → `"inv_"+base64.RawURLEncoding`. `HashToken` (`:115`) stores `hex(sha256(plaintext))`. Migration `058_web_registration_invites.sql:16` declares `token_hash TEXT NOT NULL UNIQUE` with **no plaintext column**. Unsalted SHA-256 is correct for a 256-bit random preimage (no brute-forceable preimage; design (a)). |
| 3 | **Value-safety (plaintext never leaks)** | ✅ PASS | Plaintext returned **only** by `Generate` (`webinvite/repo.go:150`); rendered **only** by `AdminInviteGenerate` → `h.render("cardrewards-invite-reveal.html", …)` at HTTP **200** (`internal/web/invites.go`), **not** a redirect (revoke is the only 303). `List` SQL (`:255/:257`) excludes `token_hash`; `InviteRow`/`inviteRowView` carry no hash/token field. No `slog`/`log`/`fmt.Print` of the token in `invites.go`, `web_register.go`, or `repo.go`; `logRegisterReject` (`web_register.go:233`) logs only `remote_addr`, `username_len`, coarse `reason`. Generate error path re-renders **without** the token. |
| 4 | **Authorization (logged-in operator; POST-only mutate)** | ✅ PASS | All 3 endpoints mount inside `r.Route("/cards/admin", …)` → `/invites` (`internal/web/cardrewards.go:205-208`), which is grouped under `r.Use(deps.webAuthMiddleware)` (`internal/api/router.go:430-433`). `webAuthMiddleware` (`router.go:721-748`) rejects anonymous with `401` in production (`AuthToken != ""`). **Not** behind `callerIsAdmin` (correct — would lock out prod operators). Generate/revoke are `r.Post` only; list is `r.Get` (a GET cannot mint/revoke). Proven by `TestAdminInvites_AnonymousBlocked` (green). |
| 5 | **Non-enumeration** | ✅ PASS | Every gate failure — wrong static secret, unknown/used/revoked/expired DB invite, disabled — returns byte-identical `registerGateBanner` + `401` + **blank** username: disabled check `web_register.go:100-101`, `!staticOK && !dbLive` at `:123-124`, `ConsumeInvalid` race branch at `:197`. Shared coarse `gate` log reason. Proven by `TestWebRegister_NonEnumerating`. Documented residual (design (a)): static path is constant-time compare vs DB path's hash-indexed lookup — not a practical enumeration vector (SHA-256 irreversible; 256-bit space unenumerable). |
| 6 | **No invite burn on bad input** | ✅ PASS | Field validation (presence/mismatch/too-short/username) runs at `web_register.go:138/145/150/157` — **after** the gate (`:100-124`) and **before** `ConsumeAndCreate` (`:181`). A bad-password attempt returns at `:145` so the invite is never claimed. Defense-in-depth: `HashAndInsertTx`→`webcreds.Hash` re-enforces `MinPasswordLength` inside the tx, and a failed `onClaimed` rolls back the claim. Proven by `TestWebRegister_FieldValidation`. |
| 7 | **OR-gate correctness (static never consumes)** | ✅ PASS | `dbLive` computed only `if !staticOK` (`web_register.go:117-122`). Step 7 branches `if staticOK { UpsertPassword(…create) }` (`:163`, no consume) `else { ConsumeAndCreate }` (`:181`). Static path never references `webinvite`. Mutually exclusive. Proven by `TestWebRegister_OrGate`. |
| 8 | **Revoke safety (idempotent, no un-consume)** | ✅ PASS | `Revoke` (`webinvite/repo.go:285`, SQL `:289`): `UPDATE … SET revoked_at=now() WHERE id=$1 AND used_at IS NULL AND revoked_at IS NULL RETURNING id`. Used/already-revoked/unknown → `ErrNoRows` → `RevokeNoop` (idempotent no-op; cannot un-consume an account). Proven by `TestAdminInviteRevoke`. |
| 9 | **CSP / XSS** | ✅ PASS | `invites_templates.go`: no inline `<script>`, no `onclick`/`onsubmit` (uses `data-action` hooks + CSS-only `<details>` revoke-confirm). Engine is `html/template` (`cardrewards.go:27`); invite templates parsed onto that set (`cardrewards.go:144`). Reflected values (`{{.Token}}` readonly input, `{{deref .Label}}`, `{{.CreatedBy}}`, `{{deref .UsedBy}}`, `{{.ID}}`) are auto-escaped; **no** `template.HTML` on user input. |
| 10 | **SQL injection** | ✅ PASS | All `webinvite` queries (Generate INSERT, IsLive SELECT, ConsumeAndCreate UPDATE `:225`, List SELECT `:257`, Revoke UPDATE `:289`) and `webcreds.HashAndInsertTx` `INSERT … VALUES ($1,$2)` use bound parameters. No string-concatenated SQL anywhere; `token_hash`/`id`/`username` are always bound. |
| 11 | **TTL** | ✅ PASS | `Generate` sets `expires_at = now()+ttl` when `ttl>0` (`webinvite/repo.go:150`); UI default `inviteTTL = 7*24h` (`internal/web/invites.go`). Both the `IsLive` gate (`:190`) and the `ConsumeAndCreate` guard (`:225`) include `(expires_at IS NULL OR expires_at > now())`; `deriveStatus` renders `Expired` for past `expires_at`. |
| 12 | **Supply-chain / SST** | ✅ PASS | `webinvite` imports only stdlib (`crypto/rand`, `crypto/sha256`, `encoding/base64`, `encoding/hex`, `errors`, `fmt`, `time`) + already-vendored `pgx/v5` (`pgx`, `pgconn`, `pgxpool`). **No new external dependency.** `inviteTTL` is a compile-time business constant, not a `${VAR:-default}` runtime fallback (smackerel-no-defaults targets runtime config). Migration carries no plaintext/secret; no secret committed. |

### Informational observation (NOT a spec-093 finding; NOT routed; NOT blocking)

**OBS-1 — shared `fail` helper echoes `err.Error()` to the HTTP response body.** `CardRewardsWebHandler.fail` (`internal/web/cardrewards.go:1068`) writes `what+": "+err.Error()` into the response. The spec-093 invite handlers reuse it (`AdminInvitesPage` list error, `AdminInviteRevoke` revoke error, `AdminInviteGenerate` parse/double-fault). This is a generic verbose-error disclosure (OWASP A09/A05).

Why this is **Informational**, not a finding:
- **Pre-existing, cross-cutting.** `fail` is the shared spec-083 card-rewards helper used by ~30 handlers; spec 093 introduces no new disclosure surface — it reuses the established pattern consistently.
- **Operator-only audience.** All `/cards/admin/*` routes sit behind `webAuthMiddleware`; under spec 070's binding trust model "any web user = full admin", so the only reader is the system owner.
- **No secret in the disclosure.** Verified: the invite plaintext never flows into any `err` passed to `fail` (`Generate` returns `"", err` on failure; the token only exists on the success return). No token, hash, password, or static secret is exposed. The value-safety invariant (threat #3) holds.

Disposition: surfaced for operator awareness only. Hardening the shared `fail` helper to log-internally/return-generic is a **separate cross-cutting hygiene item** against the card-rewards web surface, not a spec-093 blocker. Inflating it into a routed 093 finding would be scope creep onto a spec-083 helper and severity inflation. The spec-093 surface remains SECURE.

### Result

12/12 threat-model checks PASS; 0 critical/high/medium findings; 1 informational observation (out-of-scope, not routed). The spec-093 admin-generated single-use registration-invite surface is **SECURE**. Security review introduced **no** artifact changes beyond this `## Security Review` subsection; `spec.md`, `design.md`, `scopes.md`, `uservalidation.md`, and `state.json` are untouched. Next required owner: **`bubbles.regression`**.

---

## Regression Review

**Agent:** `bubbles.regression` (Steve French) · **Phase:** regression · **Run date:** 2026-06-14 · **Target:** zero regression to the DEPLOYED specs **091** (web self-registration) + **092** (card-rewards UI elevation) after spec-093 SCOPE-01..03 landed in the working tree (uncommitted; baseline = `HEAD` = commit `aa4efe1b` "spec(092) finalize…done").

### Verdict

**⚠️ REGRESSION_DETECTED — but scoped: the DEPLOYED-spec surface (091 + 092) is REGRESSION_FREE.**

- **091/092 runtime behavior:** zero regressions. Every previously-passing spec-091 register/login test and spec-092 card-rewards render/e2e test still passes byte-for-byte. The `/register` OR-gate preserves the static-secret path; the `/cards` admin-template edit is purely additive.
- **Broad unit baseline (check 3):** **2 previously-green META/governance tests now FAIL** — `TestDocFreshness_AllMigrationsDocumented` and `TestScopesPathRefDrift_NonIncreasing`. **Both are spec-093's OWN artifact/doc-completeness gaps (the new migration 058 + the new `scopes.md`), NOT 091/092 runtime regressions.** They block a green `./smackerel.sh test unit --go` and therefore block SCOPE-04, and are routed below.

### Per-check verdict

| # | Check | Verdict | Evidence anchor |
|---|-------|---------|-----------------|
| 1 | Spec 091 `/register` static-secret path UNCHANGED | 🟢 CLEAN | #reg-check1 |
| 2 | Spec 092 `/cards` UI render UNCHANGED (15 pages + CSP + data-hooks) | 🟢 CLEAN | #reg-check2 |
| 3 | Full Go unit baseline | ⚠️ 2 meta-tests regressed (spec-093 self-gaps; NOT 091/092) | #reg-check3 |
| 4 | e2e-ui full card-rewards suite (19 existing + new invite spec + CSP) | 🟢 CLEAN | #reg-check4 |
| 5 | Router/auth posture unchanged | 🟢 CLEAN | #reg-check5 |
| 6 | data-* contract for `/cards` preserved | 🟢 CLEAN | #reg-check6 |
| 7 | Adversarial: 091 + 092 guards non-tautological | 🟢 CONFIRMED | #reg-check7 |

#### reg-check1
**Phase:** regression
**Command:** `./smackerel.sh test unit --go --verbose --go-run 'TestWebRegister|TestWebLogin|TestLoginPage'`
**Exit Code:** 0
**Claim Source:** executed
```text
=== RUN   TestWebRegister_OrGate
=== RUN   TestWebRegister_OrGate/static-first-consumes-nothing
=== RUN   TestWebRegister_OrGate/db-second-when-no-static-configured
=== RUN   TestWebRegister_OrGate/db-second-when-static-mismatch
=== RUN   TestWebRegister_OrGate/disabled-nil-credentials-store
=== RUN   TestWebRegister_OrGate/disabled-empty-static-and-no-invite-store
--- PASS: TestWebRegister_OrGate (0.00s)
    --- PASS: TestWebRegister_OrGate/static-first-consumes-nothing (0.00s)
    --- PASS: TestWebRegister_OrGate/db-second-when-static-mismatch (0.00s)
    --- PASS: TestWebRegister_OrGate/disabled-empty-static-and-no-invite-store (0.00s)
--- PASS: TestWebRegister_Gate (0.00s)        (wrong-token, missing-token, empty-configured, empty-configured-empty-submitted, nil-store)
--- PASS: TestWebRegister_Duplicate (0.00s)
--- PASS: TestWebRegister_RateLimited_PerIP (0.00s)
--- PASS: TestWebRegister_RateLimit_PerIP_FreshIPAdmitted (0.00s)
--- PASS: TestWebRegister_Success (0.00s)
--- PASS: TestWebRegister_NonEnumerating (0.00s)
--- PASS: TestWebLogin_* (all 21 login scenarios)  --- PASS
--- PASS: TestLoginPage_* (all 9 page scenarios incl CSPCompliant)  --- PASS
PASS
ok      github.com/smackerel/smackerel/internal/api     0.268s
[go-unit] go test ./... finished OK
```
Static-secret path is byte-identical: `static-first-consumes-nothing` (303 + account created + 0 invite consume), `Gate/wrong-token`, `Gate/empty-configured-empty-submitted` (open-signup trap still guarded), `Duplicate`, `RateLimited_PerIP`, and all `/login` + `/login` page tests pass exactly as spec-091 shipped them. **CLEAN.**

#### reg-check2
**Phase:** regression
**Command:** `./smackerel.sh test unit --go --verbose --go-run 'CardRewards|TestCardRewardsTemplates'`
**Exit Code:** 0
**Claim Source:** executed
```text
--- PASS: TestCardRewardsTemplates_ParseAndRenderAllPages (0.01s)
    (15 sub-pages incl cardrewards-admin.html — the link-edited template)
--- PASS: TestCardRewardsTemplates_PartialsRenderCSPClean (0.00s)
--- PASS: TestCardRewardsTemplates_ElevatedMarkersAndDataHooks (0.01s)
    (all 15 pages incl cardrewards-admin.html retain elevated markers + data-* hooks)
ok      github.com/smackerel/smackerel/internal/web     0.178s
--- PASS: TestLoadCardRewardsConfig_* (config suite)  ok internal/config 0.042s
--- PASS: TestCardRewards* scheduler suite            ok internal/scheduler 0.041s
[go-unit] go test ./... finished OK
```
All 15 spec-092 design-system pages render, the admin page (whose template gained the Account Invites link) still renders + keeps its hooks, and CSP-clean partials pass. **CLEAN.**

#### reg-check3
**Phase:** regression
**Command:** `./smackerel.sh test unit --go` (full baseline) + verbose re-run of the 2 failing packages
**Exit Code:** 0 (wrapper) — `go test ./...` reported 2 FAIL packages
**Claim Source:** executed
```text
ok packages:    125
FAIL packages:  2 distinct  (internal/docfreshness, internal/scopesdriftguard)
no-test pkgs:   16
ok  github.com/smackerel/smackerel/internal/api      (PASS — 091/092 register/login)
ok  github.com/smackerel/smackerel/internal/web      0.245s (PASS — 092 cards)
ok  github.com/smackerel/smackerel/internal/config / internal/scheduler  (PASS)

--- FAIL: TestDocFreshness_AllMigrationsDocumented (0.00s)
    doc_freshness_test.go:184: docs/Development.md is STALE: 1 migration file(s) on disk
      are undocumented: 058_web_registration_invites.sql
      spec 032 requires the Database Migrations table to list every migration on disk.
FAIL    github.com/smackerel/smackerel/internal/docfreshness

--- FAIL: TestScopesPathRefDrift_NonIncreasing (0.16s)
    scopes_drift_guard_test.go:264: scopes.md drift scan: 271 broken file references found
      (ratchet ceiling: 270)
    scopes_drift_guard_test.go:279: DRIFT RATCHET EXCEEDED: 271 > maxAllowedBrokenPaths=270
          093-admin-generated-registration-invites: 2 broken
FAIL    github.com/smackerel/smackerel/internal/scopesdriftguard
```
**Both failures are spec-093's OWN incomplete artifacts**, NOT regressions to 091/092 runtime code (both 091/092 code packages `internal/api` + `internal/web` PASS). Both were green on `HEAD` (spec-092) and flipped because spec-093 (a) added migration `058` without updating the `docs/Development.md` migrations table, and (b) added a `scopes.md` carrying 2 broken path-references (271 > ratchet 270). Routed below as F-093-R1 / F-093-R2.

#### reg-check4
**Phase:** regression
**Command:** `./smackerel.sh test e2e-ui cardrewards`
**Exit Code:** 0
**Claim Source:** executed
```text
Running 20 tests using 4 workers
  ✓  cardrewards_chrome.spec.ts  SCOPE-01-A responsive nav 44px pills sticky      (092)
  ✓  cardrewards_chrome.spec.ts  SCOPE-01-B dark-mode token differs from light    (092 adversarial)
  ✓  cardrewards_chrome.spec.ts  SCOPE-01-C CSP-clean across representative /cards pages  (092/077 CSP)
  ✓  cardrewards_bonuses.spec.ts SCOPE-02-B bonus data-* hooks + progress bar     (092)
  ✓  cardrewards_invites.spec.ts SCN-093-13/14/15/16/17 admin link → generate →
        reveal → list → revoke → anonymous-blocked                                (093 NEW)
  ✓  cardrewards_admin / dashboard / categories / offers_selections /
        recommendations / rotating_verify / wallet (Spec 083 Scope 10+11)
  20 passed (17.1s)
E2EUI_EXIT=0
```
All 19 pre-existing 091/092/083 specs pass, the new `cardrewards_invites.spec.ts` passes (incl `anonymous-blocked` = auth proof), and the CSP-clean guard (`SCOPE-01-C`) stays green — the new invite page is CSP-clean. **CLEAN.**

#### reg-check5
**Phase:** regression
**Command:** `git status --short` (router.go absent from modified set) + `grep -nE 'webAuthMiddleware|/register|/login|/cards' internal/api/router.go` + `sed -n '198,212p' internal/web/cardrewards.go`
**Exit Code:** 0
**Claim Source:** executed
```text
git status: internal/api/router.go is NOT in the modified ('M') set — auth wiring unchanged.
router.go:338  r.Get("/login",  …)        PUBLIC (mirrors /register)
router.go:342  r.Get("/register", …)      PUBLIC (spec 091, OUTSIDE webAuthMiddleware)
router.go:331  r.Post("/v1/web/register") PUBLIC (OUTSIDE bearerAuthMiddleware) — unchanged
router.go:430  if deps.CardRewardsWebHandler != nil {
router.go:432      r.Use(deps.webAuthMiddleware)        <-- whole /cards tree authed
router.go:433      deps.CardRewardsWebHandler.RegisterRoutes(r) }
cardrewards.go:205  r.Route("/invites", …)  -- NESTED inside the existing authed r.Route("/cards/admin", …)
```
The 3 new invite routes are nested strictly inside the pre-existing authed `/cards/admin` group; `router.go` is unmodified; no public route became protected and no protected route became public. The e2e `anonymous-blocked` assertion (reg-check4) confirms the new routes reject anonymous. **CLEAN.**

#### reg-check6
**Phase:** regression
**Command:** `git show HEAD:internal/web/cardrewards_dashboard_templates.go | grep -oE 'data-[a-z-]+=' | sort | uniq -c` vs working tree, then `diff`
**Exit Code:** 0
**Claim Source:** executed
```text
diff /tmp/dh_head.txt /tmp/dh_wt.txt
1c1
<       7 data-action=
---
>       8 data-action=
(every other data-* token byte-identical: data-badge=3, data-catalog=2,
 data-citation-source=1, data-confidence=1, data-empty=7, data-events-written=1,
 data-manual-override=1, data-needs-verification=1, data-rec-card-id=1,
 data-rec-category=2, data-rec-starred-badge=1, data-rec-starred=2,
 data-report-category=1, data-rotating-id=1, data-run-id=1, data-run-status=1,
 data-run-trigger=1, data-run-type=1, data-triggers=1)
```
The ONLY delta is `data-action=` 7→8 — the new `data-action="account-invites"` link was **added**; zero hooks dropped or renamed. **CLEAN.**

#### reg-check7
**Phase:** regression
**Command:** `read internal/api/web_register_invite_test.go:83-98` + `grep -nE 'not\.toBe|44|getComputedStyle' web/pwa/tests/cardrewards_chrome.spec.ts` + `grep -nE 'style=|<script|onclick' internal/web/invites_templates.go`
**Exit Code:** 0
**Claim Source:** executed
```text
# 7a — spec-091 static path is NON-tautological (web_register_invite_test.go:89-96):
  if rec.Code != http.StatusSeeOther { t.Fatalf("static path status=%d want 303") }   # gate+create
  if _, ok := creds.creds["boot-op"]; !ok { t.Errorf("static path did not create the account") }
  if invites.consumeCalls != 0 { t.Errorf("static path consumed an invite; must consume NOTHING") }
  -> breaking the static path (gate OR create OR wrong DB-route) flips this test to FAIL.

# 7b — spec-092 design-loss guard is NON-tautological (cardrewards_chrome.spec.ts):
  61:  expect(pillHeight).toBeGreaterThanOrEqual(44);
  90:  expect(darkBg).not.toBe(lightBg);    # a /cards page losing its design tokens => dark==light => FAIL

# new invite templates: no inline style=/onclick/<script (only a code COMMENT mentions "NO inline <script>")
internal/web/invites_templates.go:7:  // … there are NO inline <script> blocks and NO inline …
```
Both adversarial guards assert concrete behavior that would break if the protected behavior regressed — neither is a tautology. **CONFIRMED.**

### Findings (routed — NOT 091/092 regressions; spec-093 self-completion gaps blocking SCOPE-04)

| ID | Severity | Finding | Owner | Disposition |
|----|----------|---------|-------|-------------|
| F-093-R1 | P1 (blocks green unit baseline / SCOPE-04) | `docs/Development.md` Database Migrations table does not list the new `058_web_registration_invites.sql` → `TestDocFreshness_AllMigrationsDocumented` FAIL | **bubbles.implement** | Add the `058` row to the migrations table in `docs/Development.md` (spec-032 doc-freshness contract), then re-run `./smackerel.sh test unit --go`. |
| F-093-R2 | P1 (blocks green unit baseline / SCOPE-04) | spec-093 `scopes.md` introduces 2 broken file-path references → `TestScopesPathRefDrift_NonIncreasing` FAIL (271 > ratchet 270) | **bubbles.plan** | Fix the 2 broken path-refs in `specs/093-admin-generated-registration-invites/scopes.md` (do NOT raise the ratchet — the guard says fix the new refs), then re-run the guard. |

Neither finding is a regression to deployed spec 091 or 092 — they are spec-093's own incomplete artifacts surfaced by the broad baseline. The spec MUST NOT be marked `done` and SCOPE-04 MUST NOT proceed to deploy until both are resolved and `./smackerel.sh test unit --go` is green.

### Result

091 + 092 deployed-spec surface: **REGRESSION_FREE** (checks 1, 2, 4, 5, 6, 7 all CLEAN; adversarial guards confirmed real). Broad unit baseline (check 3): **2 spec-093 self-gaps** (F-093-R1, F-093-R2) routed to `bubbles.implement` + `bubbles.plan`. Regression review introduced **no** artifact changes beyond this `## Regression Review` subsection; `spec.md`, `design.md`, `scopes.md`, `uservalidation.md`, and `state.json` are untouched; no commit was made. Next required owner: **`bubbles.implement`** (F-093-R1) + **`bubbles.plan`** (F-093-R2); then re-run `bubbles.regression`; then **`bubbles.workflow`** for SCOPE-04 deploy once the unit baseline is green.

## Remediation — F-093-R1 (docfreshness migration doc) — bubbles.implement

**Finding:** `internal/docfreshness` `TestDocFreshness_AllMigrationsDocumented` FAILed because the new migration `internal/db/migrations/058_web_registration_invites.sql` (SCOPE-01) was not listed in the Database Migrations table of `docs/Development.md`. The test asserts every `internal/db/migrations/*.sql` filename appears in the doc (`undocumented(doc, files, fileNeedle)`; `fileNeedle(name) = name` → substring presence of the base filename).

**Fix (docs/Development.md ONLY — scopes.md NOT touched):**
- Added the `058` row to the Database Migrations table, matching the surrounding row format (pipe-table, backticked file + table/column names, em-dash lead, `IF NOT EXISTS` idempotency note):
  > `| 058 | `058_web_registration_invites.sql` | Spec 093 admin-generated single-use registration invites — `web_registration_invites` table (`token_hash` lowercase-hex SHA-256, `label`, `created_by`, `created_at`, `expires_at`, `used_at`, `used_by`, `revoked_at`); hashed-at-rest with no plaintext column, `UNIQUE(token_hash)` lookup, atomically marked used on a successful `/register` (single-use, TOCTOU-guarded UPDATE), augmenting spec 091's static `WEB_REGISTRATION_INVITE_TOKEN` bootstrap gate; `CREATE TABLE IF NOT EXISTS` self-idempotent |`
- Updated the prose freshness line from "Database migrations through `057_card_rewards.sql`" → "through `058_web_registration_invites.sql`" to keep it accurate.

### impl-F1a — targeted TestDocFreshness now green
**Phase:** implement
**Command:** `./smackerel.sh test unit --go --go-run 'TestDocFreshness'`
**Exit Code:** 0
**Claim Source:** executed
```text
[go-unit] applying -run selector: TestDocFreshness
[go-unit] starting go test ./...
+ go test -run TestDocFreshness -count=1 ./...
ok      github.com/smackerel/smackerel/internal/docfreshness    0.041s
ok      github.com/smackerel/smackerel/internal/scopesdriftguard        0.022s [no tests to run]
ok      github.com/smackerel/smackerel/internal/web     0.652s [no tests to run]
ok      github.com/smackerel/smackerel/web/pwa/tests    0.023s [no tests to run]
+ echo '[go-unit] go test ./... finished OK'
[go-unit] go test ./... finished OK
```
`internal/docfreshness` now RUNS (not "[no tests to run]") and is `ok` — the migration-documented assertion passes. (`scopesdriftguard` shows "[no tests to run]" because `-run TestDocFreshness` excludes it; it is exercised in the broad run below.)

### impl-F1b — broad Go unit baseline: only F-093-R2 (scopesdriftguard) remains red
**Phase:** implement
**Command:** `./smackerel.sh test unit --go`
**Exit Code:** 1 (single pre-routed failure — F-093-R2, plan-owned)
**Claim Source:** executed
```text
ok      github.com/smackerel/smackerel/internal/docfreshness    0.033s
...
--- FAIL: TestScopesPathRefDrift_NonIncreasing (0.26s)
    scopes_drift_guard_test.go:264: scopes.md drift scan: 271 broken file references found (ratchet ceiling: 270)
    scopes_drift_guard_test.go:279: DRIFT RATCHET EXCEEDED: found 271 broken file references in specs/*/scopes.md, but maxAllowedBrokenPaths=270.
        Breakdown:
          093-admin-generated-registration-invites: 2 broken
            - internal/auth/webinvite/repo_test.go
            - internal/auth/webinvite/concurrent_consume_test.go
          036-meal-planning: 41 broken
          034-expense-tracking: 80 broken
          035-recipe-enhancements: 61 broken
          ... (all other breakdown rows are PRE-EXISTING drift below the ratchet)
FAIL
FAIL    github.com/smackerel/smackerel/internal/scopesdriftguard        0.276s
ok      github.com/smackerel/smackerel/web/pwa/tests    1.285s
FAIL
```
Every other package is `ok`/`[no test files]`. The **sole** remaining failure is `internal/scopesdriftguard` (`TestScopesPathRefDrift_NonIncreasing`), driven by spec-093's 2 new broken `scopes.md` path-refs (`internal/auth/webinvite/repo_test.go`, `internal/auth/webinvite/concurrent_consume_test.go`) pushing the ratchet from ≤270 → 271. That is **F-093-R2, plan-owned** — `scopes.md` was deliberately NOT modified by this implement remediation. F-093-R1 is RESOLVED.

**Ownership note:** This remediation modified only `docs/Development.md` (implement-permitted doc) and appended this evidence to `report.md` (implement-owned). `scopes.md`, `spec.md`, `design.md`, `uservalidation.md`, and `state.json` are untouched; no commit was made; status remains `in_progress`. Next required owner for the remaining red: **`bubbles.plan`** (F-093-R2).
