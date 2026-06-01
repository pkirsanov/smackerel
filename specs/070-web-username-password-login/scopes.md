# Scopes — Spec 070

| # | Scope | Status |
|---|-------|--------|
| 1 | Storage + hasher + repo (unit + integration tests) | Not Started |
| 2 | HTTP handler extension + login.html | Not Started |
| 3 | CLI subcommand (`users add/set-password/list`) | Not Started |
| 4 | Deploy + create first user + verify browser flow | Not Started |

---

## SCOPE-1 — Storage, hasher, repo
**Depends On:** none
**Status:** Not Started

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
- [ ] Migration 044 SQL exists and is idempotent
- [ ] Hasher unit tests pass (≥10 lines raw output)
- [ ] Repo integration test passes against ephemeral test PG (≥10 lines raw output)
- [ ] Timing parity test passes (≥10 lines raw output)
- [ ] `./smackerel.sh check` exits 0
- [ ] `./smackerel.sh lint` exits 0
- [ ] No SQL or password literals in test fixtures committed

---

## SCOPE-2 — HTTP handler + login.html
**Depends On:** SCOPE-1
**Status:** Not Started

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
- [ ] Credential branch lands per design §3.1
- [ ] login.html re-rendered (verify in browser smoke)
- [ ] All credential matrix tests pass (≥10 lines raw output)
- [ ] Token-only regression test passes (≥10 lines raw output)
- [ ] No new lint warnings
- [ ] `./smackerel.sh check` + `lint` + `format --check` exit 0

---

## SCOPE-3 — CLI subcommand
**Depends On:** SCOPE-1
**Status:** Not Started

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
- [ ] CLI dispatcher lands
- [ ] All four subcommand behaviors tested (add new, add existing →
      err, set existing, set missing → err, list)
- [ ] Empty / whitespace usernames rejected
- [ ] Mismatched password confirmation rejected
- [ ] Password < 12 chars rejected with clear message
- [ ] `./smackerel.sh check` + `lint` + `format --check` exit 0

---

## SCOPE-4 — Deploy + verify operator login
**Depends On:** SCOPE-2, SCOPE-3
**Status:** Not Started

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

### Definition of Done
- [ ] CI build green
- [ ] promote.sh apply OK + verify OK
- [ ] `users add` succeeds with raw output
- [ ] Operator confirms browser login works (uservalidation item)
- [ ] Cookie persists across page navigation (single-login goal)
