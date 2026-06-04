# Stochastic Quality Sweep — Round 4 — SIMP-070 Closure

> Ops-quality work. NOT tracked under spec 070 (terminal `done`).
> Source: `bubbles.simplify` round-4 envelope against the web
> username/password login surface introduced by spec 070.
> Implementer: `bubbles.implement`. Date: 2026-06-04.

## Scope

Six low/medium simplify findings against the web login surface:

| ID            | Class                       | Targets                                                                                  |
| ------------- | --------------------------- | ---------------------------------------------------------------------------------------- |
| SIMP-070-001  | duplicated-logic (medium)   | `cmd/core/cmd_users.go`, `cmd/core/cmd_users_test.go`                                    |
| SIMP-070-002  | duplicated-logic (medium)   | `internal/api/web_login.go`                                                              |
| SIMP-070-003  | dead-branch readability     | `internal/api/web_login.go`                                                              |
| SIMP-070-004  | trivial wrapper             | `internal/api/web_login_page.go`, `internal/api/router.go`                               |
| SIMP-070-005  | silent telemetry            | `internal/auth/webcreds/repo.go`                                                         |
| SIMP-070-006  | duplicated string literals  | `internal/api/web_login.go`, `internal/api/web_login_page.go`, `internal/api/sanitize_next.go` |

## Per-finding Closure

### SIMP-070-001 — Drop CLI-local min-password enforcement

**Status:** addressed.

- Deleted `MinPasswordLength`, `errPasswordTooShort`, and
  `enforceMinPasswordLength` from `cmd/core/cmd_users.go`.
- Removed the two `enforceMinPasswordLength(password)` guards in
  `runUsersAdd` / `runUsersSetPassword`. The minimum length is now
  enforced exclusively by `webcreds.Hash`, called from
  `(*PostgresRepo).UpsertPassword`, and its `webcreds: password must
  be at least N characters` error propagates naturally through the
  existing CLI error path (which prefixes the subcommand name and
  exits 1).
- Updated the in-memory test repo in `cmd/core/cmd_users_test.go` so
  `memRepo.UpsertPassword` also calls `webcreds.Hash` to enforce the
  same contract — `TestRunUsersAdd_RejectsShortPassword` and
  `TestRunUsersSetPassword_RejectsShortPassword` continue to assert
  exit code 1, and they still match because `webcreds.Hash("short")`
  returns the password-length error.
- Replaced the dead `MinPasswordLength == 12` assertion in
  `TestRunUsersCommand_UnknownSubcommand` with a direct check against
  `webcreds.MinPasswordLength`.

### SIMP-070-002 — Helper extraction in `web_login.go`

**Status:** addressed.

- Added three unexported helpers in `internal/api/web_login.go`:
  - `isFormContentType(r *http.Request) bool` (replaces the two
    inline `strings.HasPrefix(strings.ToLower(strings.TrimSpace(...)))`
    blocks in `HandleWebLogin` and `HandleWebLogout`).
  - `(d *Dependencies) authCookie(value string, clear bool) *http.Cookie`
    builds the session cookie with the canonical attributes (HttpOnly,
    SameSite=Lax, Path=/, Secure when production). Logout passes
    `clear=true` which sets `MaxAge: -1` — preserved bit-for-bit.
  - `(d *Dependencies) loginAuthEnabled() bool` replaces the inlined
    `d.AuthConfig.Enabled || d.AuthToken != ""` checks in
    `renderLoginError` and `HandleLoginPage`.

### SIMP-070-003 — Flip credentialVerified branch

**Status:** addressed.

- Reworked the post-credentials branch in `HandleWebLogin` from the
  empty-then-else-if-else shape into `if !credentialVerified { … }`.
  The dead "skip token verify" branch is gone; the explanatory
  comment now sits above the outer guard.

### SIMP-070-004 — Inline `loginStaticFS`

**Status:** addressed.

- Deleted `loginStaticFS()` from `internal/api/web_login_page.go`.
- Updated the sole caller in `internal/api/router.go`:
  `http.FileServer(loginStaticFS())` → `http.FileServer(http.FS(loginUIFS))`.
  `loginUIFS` is already package-visible.

### SIMP-070-005 — Warn-log touch failure

**Status:** addressed.

- In `(*PostgresRepo).VerifyAndTouch` the silent `return nil` on
  `last_login_at` UPDATE failure now emits
  `slog.Warn("webcreds: last_login_at update failed",
  "username_len", len(username), "err", err)` before returning. The
  username itself is never logged — only its length is exposed as a
  non-identifying signal. Added `log/slog` to the package imports.

### SIMP-070-006 — Replace duplicated string literals

**Status:** addressed.

- Added two unexported package constants in
  `internal/api/web_login.go`:
  `authCookieName = "auth_token"` and `loginPath = "/login"`.
- `authCookieName` is consumed by `authCookie`, which replaces both
  inline `Name: "auth_token"` occurrences (formerly L221, L255).
- `loginPath` replaces the inline `"/login"` in
  `HandleWebLogout`'s redirect and in `sanitize_next.go`'s login-loop
  guard (formerly L56 in that file).

## Evidence

### `go vet` — affected packages

```text
$ go vet ./internal/api/... ./cmd/core/... ./internal/auth/webcreds/...
(no output — clean)
```

**Claim Source:** executed.

### `./smackerel.sh build`

Full container build (smackerel-core + smackerel-ml) completed.
Tail:

```text
#41 [smackerel-core] exporting to image
#41 writing image sha256:fc080d0783e041374b819c3f0ffff3b7ab1865b6ef490348da82c73b0a71b50f done
#41 DONE 0.3s
 smackerel-core  Built
 smackerel-ml  Built
```

Exit code: 0.

**Claim Source:** executed.

### `./smackerel.sh test unit`

Full Go unit-test suite ran. Affected packages all pass:

```text
ok      github.com/smackerel/smackerel/cmd/core                    1.240s
ok      github.com/smackerel/smackerel/internal/api               10.158s
ok      github.com/smackerel/smackerel/internal/api/admin/extensiondevices    (cached)
ok      github.com/smackerel/smackerel/internal/api/connectors/extension      (cached)
ok      github.com/smackerel/smackerel/internal/api/graphapi      (cached)
ok      github.com/smackerel/smackerel/internal/auth/webcreds      4.208s
```

Two unrelated pre-existing failures observed in the same run:

- `internal/deploy` `TestBundleSecretContract_*` — fail with
  `ERROR: searxng settings file not found:
  …/config/searxng/settings.yml`. Environmental: the test tmpdir
  isn't seeded with the searxng settings file. Not touched by these
  edits.
- `tests/unit/clients` `TestRenderDescriptorV1_*` — fail with
  `node not on PATH` / `dart not on PATH`. Environmental: cross-
  language canary requires node + dart toolchains. Not touched by
  these edits.

Affected-package re-run for clarity:

```text
$ go test ./cmd/core/... ./internal/api/... ./internal/auth/webcreds/...
ok      github.com/smackerel/smackerel/cmd/core                    1.240s
ok      github.com/smackerel/smackerel/internal/api               10.158s
ok      github.com/smackerel/smackerel/internal/api/admin/extensiondevices    (cached)
ok      github.com/smackerel/smackerel/internal/api/connectors/extension      (cached)
ok      github.com/smackerel/smackerel/internal/api/graphapi      (cached)
ok      github.com/smackerel/smackerel/internal/auth/webcreds      4.208s
```

Exit code: 0.

**Claim Source:** executed.

## Unresolved

None for the routed finding set.
