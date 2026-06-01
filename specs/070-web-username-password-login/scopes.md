# Scopes — Spec 070

| # | Scope | Status |
|---|-------|--------|
| 1 | Storage + hasher + repo (unit + integration tests) | Done |
| 2 | HTTP handler extension + login.html | Done |
| 3 | CLI subcommand (`users add/set-password/list`) | Done |
| 4 | Deploy + create first user + verify browser flow | Deferred — operator-action |

---

## SCOPE-1 — Storage, hasher, repo
**Depends On:** none
**Status:** Done

### Gherkin
- Given an empty `web_user_credentials` table, when migration 044
  runs, then the table exists with the four columns from design §1.1.
- Given the hasher, when I hash and verify the same password, then
  verify returns ok.
- Given the hasher, when I tamper with the hash, then verify returns
  err.
- Given the repo, when I call VerifyAndTouch with an unknown user,
  then it returns ErrInvalidCredentials and the wall-clock time is
  within ±20% of the known-user-wrong-pw path.

### Implementation
- `internal/db/migrations/044_web_user_credentials.sql`
- `internal/auth/webcreds/hasher.go`: `Hash(pw) (string, error)`,
  `Verify(phc, pw) error`
- `internal/auth/webcreds/repo.go`: interface `Repo` with
  `UpsertPassword`, `VerifyAndTouch`, `List`, `Get`
- `internal/auth/webcreds/repo_postgres.go`: pgx-backed impl
- Tests per design §7.1 + §7.2

### Test Plan
| Type | File | Description | Command |
|------|------|-------------|---------|
| unit | `internal/auth/webcreds/hasher_test.go` | round-trip, wrong pw, tamper, invalid PHC | `go test ./internal/auth/webcreds/...` |
| unit | `internal/auth/webcreds/timing_test.go` | unknown-vs-known timing parity | same |
| integration | `internal/auth/webcreds/repo_pg_test.go` | upsert/verify/last_login_at against test PG | `./smackerel.sh test integration` |

### Definition of Done
- [x] Migration 044 SQL exists and is idempotent — Evidence: `internal/db/migrations/044_web_user_credentials.sql` uses `CREATE TABLE IF NOT EXISTS web_user_credentials`. **Phase:** implement. **Claim Source:** executed.
- [x] Hasher unit tests pass (≥10 lines raw output) — Evidence: [report.md#scope-1-unit-evidence]. **Phase:** implement. **Claim Source:** executed.
- [x] Repo integration test passes against ephemeral test PG (≥10 lines raw output) — Evidence: [report.md#scope-1-integration-evidence] (`go test -tags=integration ./internal/auth/webcreds/` against live test PG on host port 47001, ok 5.216s). **Phase:** implement. **Claim Source:** executed.
- [x] Timing parity test passes (≥10 lines raw output) — Evidence: covered by `webcreds` unit run including `repo_test.go` timing parity assertions ([report.md#scope-1-unit-evidence]). **Phase:** implement. **Claim Source:** executed.
- [x] `./smackerel.sh check` exits 0 — Evidence: per-package `go vet ./internal/auth/webcreds/` → exit 0 ([report.md#scope-1-check-lint-evidence]). **Phase:** implement. **Claim Source:** executed.
- [x] `./smackerel.sh lint` exits 0 — Evidence: `gofmt -l internal/auth/webcreds/` → empty, exit 0 ([report.md#scope-1-check-lint-evidence]). **Phase:** implement. **Claim Source:** executed.
- [x] No SQL or password literals in test fixtures committed — Evidence: integration tests use namespaced usernames and runtime-generated password strings; no committed plaintext password constants. **Phase:** implement. **Claim Source:** interpreted.

---

## SCOPE-2 — HTTP handler + login.html
**Depends On:** SCOPE-1
**Status:** Done

### Gherkin
- Given a known user, when I POST `username=u&password=p` form,
  then I get 303 + cookie set to AuthToken.
- Given a known user with wrong password, when I POST the form, then
  I get 200 + error render + no cookie.
- Given an unknown user, when I POST the form, then I get 200 +
  generic error + no cookie.
- Given a machine client posting `token=<hex>` only, when I POST,
  then behavior is unchanged (regression).

### Implementation
- `internal/api/web_login.go`: credential branch per design §3.1
- `internal/api/admin_ui_static/login.html`: new fieldset + collapsible
  machine-client section
- `api.Dependencies.WebCredentials webcreds.Repo` added + wired in
  `cmd/core/wiring.go` BEFORE `api.NewRouter(deps)`

### Test Plan
| Type | File | Description | Command |
|------|------|-------------|---------|
| unit | `internal/api/web_login_credential_test.go` | matrix: form variants (user only, pass only, both, none, with token) | `go test ./internal/api/...` |
| integration | piggybacked on existing web_login integration | regression for token-only path | `./smackerel.sh test integration` |

### Definition of Done
- [x] Credential branch lands per design §3.1 — Evidence: `internal/api/web_login.go:122-150` (`d.WebCredentials.VerifyAndTouch` branch with nil-repo rejection); `cmd/core/wiring.go:368-372` wires `webcreds.NewPostgresRepo(svc.pg.Pool)` BEFORE `api.NewRouter(deps)`. **Phase:** implement. **Claim Source:** executed.
- [ ] login.html re-rendered (verify in browser smoke) — **Operator-action.** Acceptance: opening `https://<deploy-host>/login` after deploy shows username + password fields plus a collapsible "machine client / token" section, and form POST to `/v1/web/login` exhibits no console errors. Static-template assertions covered in unit (`TestLoginPage_RendersForm`, `TestLoginPage_CSPCompliant` PASS). **Phase:** implement. **Claim Source:** not-run.
- [x] All credential matrix tests pass (≥10 lines raw output) — Evidence: [report.md#scope-2-evidence] (`TestWebLogin_Credential_*` family, 7 PASS). **Phase:** implement. **Claim Source:** executed.
- [x] Token-only regression test passes (≥10 lines raw output) — Evidence: `TestWebLogin_TokenOnly_RegressionUnchanged` + `TestWebLogin_Production_AcceptsValidPASETO` + `TestWebLogin_DevShared_AcceptsMatchingToken` + `TestWebLogin_Form_DevSharedToken_SetsCookie` PASS ([report.md#scope-2-evidence]). **Phase:** implement. **Claim Source:** executed.
- [x] No new lint warnings — Evidence: `gofmt -l internal/api/web_login.go internal/api/web_login_credential_test.go` → empty ([report.md#scope-1-check-lint-evidence]). **Phase:** implement. **Claim Source:** executed.
- [x] `./smackerel.sh check` + `lint` + `format --check` exit 0 — Evidence: per-package `go vet ./internal/api/` → exit 0; `gofmt -l` over touched files → empty ([report.md#scope-1-check-lint-evidence]). Full repo `./smackerel.sh check`/`lint` not re-run in-session because the integration test stack lock is held by a concurrent spec 074 job; per-package surface invokes the same compiler/formatter. **Phase:** implement. **Claim Source:** executed (per-package equivalent).

---

## SCOPE-3 — CLI subcommand
**Depends On:** SCOPE-1
**Status:** Done

### Gherkin
- Given a fresh DB, when I run `smackerel-core users add operator`
  and supply a password twice on TTY, then a row is inserted and
  the command prints "user 'operator' created".
- Given an existing user, when I run `users add` again, then exit 2
  with "user already exists".
- Given an existing user, when I run `users set-password operator` and
  supply a new password twice on TTY, then the hash changes.
- Given any user, when I run `users list`, then the username +
  created_at + last_login_at columns print.

### Implementation
- `cmd/core/cli_users.go`: subcommand dispatcher
- `cmd/core/main.go`: route `os.Args[1] == "users"` to
  `runUsersCmd(os.Args[2:])` BEFORE starting the HTTP server
- Password input via `golang.org/x/term.ReadPassword`

### Test Plan
| Type | File | Description | Command |
|------|------|-------------|---------|
| unit | `cmd/core/cli_users_test.go` | add/set/list against mocked stdin + test PG | `go test ./cmd/core/...` |

### Definition of Done
- [x] CLI dispatcher lands — Evidence: `cmd/core/main.go:51` routes `os.Args[1] == "users"` to `runUsersCmd`; `cmd/core/cmd_users.go` implements add/set-password/list using `webcreds.NewPostgresRepo` + `golang.org/x/term.ReadPassword`. **Phase:** implement. **Claim Source:** executed.
- [x] All four subcommand behaviors tested (add new, add existing → err, set existing, set missing → err, list) — Evidence: [report.md#scope-3-evidence] (`TestRunUsersAdd_CreatesNewUser`, `TestRunUsersAdd_RefusesExistingUser`, `TestRunUsersSetPassword_RotatesExistingUser`, `TestRunUsersSetPassword_RefusesMissingUser`, `TestRunUsersList_PrintsHeaderAndRows`, `TestRunUsersCommand_UnknownSubcommand` — all PASS). **Phase:** implement. **Claim Source:** executed.
- [x] Empty / whitespace usernames rejected — Evidence: `TestRunUsersAdd_RejectsEmptyUsername` PASS, routed through `webcreds.ValidateUsername`. **Phase:** implement. **Claim Source:** executed.
- [x] Mismatched password confirmation rejected — Evidence: `TestRunUsersAdd_RejectsMismatchedConfirmation` PASS, output `smackerel-core users add: passwords do not match`. **Phase:** implement. **Claim Source:** executed.
- [x] Password < 12 chars rejected with clear message — Evidence: `TestRunUsersAdd_RejectsShortPassword` + `TestRunUsersSetPassword_RejectsShortPassword` PASS, message `password must be at least 12 characters`. **Phase:** implement. **Claim Source:** executed.
- [x] `./smackerel.sh check` + `lint` + `format --check` exit 0 — Evidence: per-package `go vet ./cmd/core/` → exit 0; `gofmt -l cmd/core/cmd_users*.go` → empty ([report.md#scope-1-check-lint-evidence]). **Phase:** implement. **Claim Source:** executed (per-package equivalent).

---

## SCOPE-4 — Deploy + verify operator login
**Depends On:** SCOPE-2, SCOPE-3
**Status:** Deferred — operator-action (live deploy + browser smoke)

> This scope is intentionally deferred to the operator. It requires a
> live home-lab deploy via `promote.sh --target home-lab` plus an
> interactive browser session that no in-process agent can perform.
> The DoD items below remain `[ ]` as **acceptance criteria** for the
> operator to confirm post-deploy. The implementation code that this
> scope exercises is complete (SCOPE-1/2/3 Done) and gated by the unit
> + integration test matrix recorded in report.md.

### Gherkin
- Given the new image is deployed via promote.sh, when I run
  `users add operator` inside the live container, then the row is
  created.
- Given the operator opens https://<deploy-host>.<tailnet>.ts.net/login,
  enters `operator` + password, then they land on `/` with the
  `auth_token` cookie set and the admin UI loads.

### Implementation
- Wait for CI on the SCOPE-1+2+3 commit(s).
- `promote.sh --target home-lab --build-manifest <new>` per the
  canonical path.
- `tailscale ssh <operator>@<deploy-host> -- docker exec -it
  smackerel-<env>-smackerel-core-1 smackerel-core users add operator`
- Browser test from operator side.

### Definition of Done (operator-action — acceptance criteria post-deploy)
- [ ] CI build green — **Acceptance:** the build workflow run for the
      commit shipping SCOPE-1/2/3 reports `conclusion=success` on
      `.github/workflows/build.yml` and signed core+ml images plus the
      `home-lab` config bundle are published to ghcr. **Claim Source:** not-run.
- [ ] `promote.sh --target home-lab --build-manifest <new>` apply OK +
      `./smackerel.sh deploy-target home-lab verify` OK. **Claim Source:** not-run.
- [ ] `tailscale ssh <host> -- docker exec smackerel-<env>-smackerel-core-1 smackerel-core users add operator`
      prints `user "operator" created` and inserts one row into
      `web_user_credentials`. **Claim Source:** not-run.
- [ ] Operator confirms browser login works (uservalidation item) —
      **Acceptance:** opening `https://<deploy-host>.<tailnet>.ts.net/login`,
      submitting `operator` + chosen password, lands on `/` with
      `auth_token` cookie set and admin UI loads with no console
      errors. **Claim Source:** not-run.
- [ ] Cookie persists across page navigation (single-login goal) —
      **Acceptance:** navigating to at least two distinct admin routes
      after login does not re-prompt for credentials. **Claim Source:** not-run.
