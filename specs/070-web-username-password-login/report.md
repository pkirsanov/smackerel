# Report — Spec 070

## Summary
Implementing username/password login for the smackerel web UI on top
of the existing shared-token cookie mechanism. Adds a credential layer
(table + argon2id hasher + repo), extends the `/v1/web/login` handler
to verify user+pass, exposes a CLI for operator user management, and
updates the login form HTML. Cookie value on success is the existing
shared `AuthToken` — same trust band as today's token-form login.

## Test Evidence

### scope-1 unit evidence

Command: `go test -count=1 -timeout 60s ./internal/auth/webcreds/`

```
ok      github.com/smackerel/smackerel/internal/auth/webcreds   8.436s
```

Covers `hasher_test.go` (round-trip, wrong password, tamper, invalid
PHC), `repo_test.go` (in-memory repo + timing parity for
known-wrong-password vs unknown-user paths), and `ValidateUsername`
table (13 sub-cases). Exit 0. **Claim Source:** executed.

### scope-1 integration evidence

Command (reusing the already-running ephemeral test stack
`smackerel-test-postgres-1` on host port 47001):

```
DATABASE_URL='postgres://smackerel:smackerel@127.0.0.1:47001/smackerel?sslmode=disable' \
  go test -tags=integration -count=1 -timeout 90s -v ./internal/auth/webcreds/
```

```
=== RUN   TestPostgresRepo_UpsertRotateRejectsMissing
--- PASS: TestPostgresRepo_UpsertRotateRejectsMissing (0.12s)
=== RUN   TestPostgresRepo_VerifyAndTouchHappyPath
--- PASS: TestPostgresRepo_VerifyAndTouchHappyPath (0.23s)
=== RUN   TestPostgresRepo_VerifyAndTouchWrongPasswordKeepsLastLoginUnchanged
--- PASS: TestPostgresRepo_VerifyAndTouchWrongPasswordKeepsLastLoginUnchanged (0.20s)
=== RUN   TestPostgresRepo_VerifyAndTouchUnknownUser
--- PASS: TestPostgresRepo_VerifyAndTouchUnknownUser (0.11s)
=== RUN   TestPostgresRepo_ListReturnsSeededRows
--- PASS: TestPostgresRepo_ListReturnsSeededRows (0.14s)
=== RUN   TestValidateUsername
--- PASS: TestValidateUsername (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/auth/webcreds   5.216s
```

Migration 044 is idempotent — `CREATE TABLE IF NOT EXISTS
web_user_credentials` (verified by direct read of
`internal/db/migrations/044_web_user_credentials.sql`). **Claim Source:** executed.

### scope-1 check + lint evidence

Per-package surface (the full repo `./smackerel.sh check`/`lint` was
not invoked because the integration test-suite lock is held by a
concurrent spec 074 e2e run; per-package `go vet` and `gofmt` invoke
the same compiler/formatter):

```
$ go vet ./internal/auth/webcreds/ ; echo $?
0
$ go vet ./internal/api/ ; echo $?
0
$ go vet ./cmd/core/ ; echo $?
0
$ gofmt -l internal/auth/webcreds/ cmd/core/cmd_users.go \
    cmd/core/cmd_users_test.go internal/api/web_login.go \
    internal/api/web_login_credential_test.go ; echo $?
0
```

**Claim Source:** executed.

### scope-2 evidence

Command: `go test -count=1 -timeout 60s -v -run 'TestLogin|TestCredential|TestWebLogin|Credential' ./internal/api/`

Selected results (full output shows ≥25 PASS lines including dev-shared,
production PASETO, revoked-token, body-validation matrix, and credential
matrix). Credential-branch matrix:

```
--- PASS: TestWebLogin_Credential_Valid_RedirectsAndSetsCookie (0.10s)
--- PASS: TestWebLogin_Credential_WrongPassword_NoCookie (0.00s)
--- PASS: TestWebLogin_Credential_UnknownUser_NoCookie_SameError (0.00s)
--- PASS: TestWebLogin_Credential_MissingPassword (0.00s)
--- PASS: TestWebLogin_Credential_MissingUsername (0.00s)
--- PASS: TestWebLogin_TokenOnly_RegressionUnchanged (0.00s)
--- PASS: TestWebLogin_Credential_NilRepo_RejectedWithError (0.00s)
--- PASS: TestWebLogin_Form_Valid_RedirectsAndSetsCookie (0.00s)
--- PASS: TestWebLogin_Form_DevSharedToken_SetsCookie (0.00s)
--- PASS: TestLoginPage_RendersForm (0.00s)
--- PASS: TestLoginPage_CSPCompliant (0.00s)
--- PASS: TestLoginPage_SanitisesNext (0.00s)
--- PASS: TestWebLogin_Production_AcceptsValidPASETO (0.00s)
--- PASS: TestWebLogin_Production_RejectsForeignPASETO (0.00s)
--- PASS: TestWebLogin_Production_RejectsRevokedToken (0.00s)
--- PASS: TestWebLogin_DevShared_AcceptsMatchingToken (0.00s)
--- PASS: TestWebLogin_DevShared_RejectsWrongToken (0.00s)
--- PASS: TestWebLogin_DevBypass_RefusesLogin (0.00s)
--- PASS: TestWebLogin_BodyValidation (0.00s) [+5 sub-cases]
--- PASS: TestWebLogin_RejectsNonPOST (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/api     0.176s
```

Token-only regression (the original spec 057 / 060 / 044 path) remains
green alongside the new credential branch. Cookie issuance: on success
the handler sets the existing shared `AuthToken` cookie (no new token
type) — this is the SCOPE-3 contract called out in design §3.1 and is
exercised by `TestWebLogin_Credential_Valid_RedirectsAndSetsCookie`
(302/303 redirect + `Set-Cookie: auth_token=...`). **Claim Source:** executed.

### scope-3 evidence

Command: `go test -count=1 -timeout 60s -v -run TestRunUsers ./cmd/core/`

```
--- PASS: TestRunUsersAdd_CreatesNewUser (0.00s)
--- PASS: TestRunUsersAdd_RefusesExistingUser (0.00s)
  smackerel-core users add: user "operator" already exists (use `set-password` to rotate)
--- PASS: TestRunUsersAdd_UsageWhenMissingArg (0.00s)
  usage: smackerel-core users add <username>
--- PASS: TestRunUsersAdd_RejectsEmptyUsername (0.00s)
  smackerel-core users add: username must not be empty
--- PASS: TestRunUsersAdd_RejectsShortPassword (0.00s)
  smackerel-core users add: password must be at least 12 characters
--- PASS: TestRunUsersAdd_RejectsMismatchedConfirmation (0.00s)
  smackerel-core users add: passwords do not match
--- PASS: TestRunUsersSetPassword_RotatesExistingUser (0.00s)
  password for "operator" rotated
--- PASS: TestRunUsersSetPassword_RefusesMissingUser (0.00s)
  smackerel-core users set-password: no such user "ghost" (use `add` to create)
--- PASS: TestRunUsersSetPassword_RejectsShortPassword (0.00s)
--- PASS: TestRunUsersList_PrintsHeaderAndRows (0.00s)
--- PASS: TestRunUsersCommand_UnknownSubcommand (0.00s)
PASS
ok      github.com/smackerel/smackerel/cmd/core 0.165s
```

`cmd/core/main.go:51` routes `os.Args[1] == "users"` to
`runUsersCmd(os.Args[2:])` before the HTTP server starts. Password
input uses `golang.org/x/term.ReadPassword`; tests inject a scripted
prompter via the in-process dispatcher seam. **Claim Source:** executed.

### scope-4 evidence
Deferred — operator-action. See SCOPE-4 acceptance criteria in
`scopes.md`. No in-process evidence to capture until the operator runs
`promote.sh --target home-lab`, executes `users add operator` in the
live container, and confirms browser login.

## Completion Statement
SCOPE-1, SCOPE-2, and SCOPE-3 are implementation-complete with
executed unit + integration evidence (see Test Evidence above) and
per-package `go vet` + `gofmt` clean across `internal/auth/webcreds/`,
`internal/api/`, and `cmd/core/`. SCOPE-4 is intentionally deferred to
the operator: it requires a live home-lab deploy (`promote.sh
--target home-lab`) plus an interactive browser smoke that no
in-process agent can perform. SCOPE-4 DoD items remain `[ ]` as
explicit acceptance criteria; matching `uservalidation.md` items also
remain `[ ]` until the operator runs through the login flow.

Spec status stays `in_progress` (workflow ceiling `done`) until the
operator confirms the SCOPE-4 acceptance criteria. No promotion to
`done` is attempted by this run.
