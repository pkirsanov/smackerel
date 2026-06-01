# Design — Spec 070

## Architecture
```
                      browser
                         │
                         │ POST /v1/web/login
                         │   Content-Type: application/x-www-form-urlencoded
                         │   username=operator&password=...
                         ▼
   ┌──────────────────────────────────────────────────┐
   │ HandleWebLogin (internal/api/web_login.go)       │
   │   1. Detect form vs JSON                         │
   │   2. If form && username present → credential    │
   │      verify path (new)                           │
   │   3. Else → existing token verify path           │
   │   4. Either way → set cookie, 303 to next        │
   └─────────────┬────────────────────────────────────┘
                 │ (credential path)
                 ▼
   ┌──────────────────────────────────────────────────┐
   │ webcreds.Repo.VerifyAndTouch(ctx, user, pw)      │
   │   - SELECT password_hash FROM                    │
   │     web_user_credentials WHERE username = $1     │
   │   - argon2id.Compare(hash, pw)                   │
   │   - On match: UPDATE last_login_at = now()       │
   │   - On miss: argon2id.Compare(dummy, pw) for     │
   │     timing parity                                │
   └──────────────────────────────────────────────────┘
```

## §1 Storage
### §1.1 Migration
`internal/db/migrations/044_web_user_credentials.sql`

```sql
-- Spec 070 — web operator credentials. Argon2id PHC strings.
CREATE TABLE IF NOT EXISTS web_user_credentials (
    username       TEXT PRIMARY KEY,
    password_hash  TEXT NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_login_at  TIMESTAMPTZ
);
COMMENT ON TABLE web_user_credentials IS
  'Spec 070 — operator username/password credential layer for the smackerel web UI.';
COMMENT ON COLUMN web_user_credentials.password_hash IS
  'argon2id PHC string per https://github.com/P-H-C/phc-string-format/blob/master/phc-sf-spec.md';
```

Username validation lives in code, NOT in the DB (no CHECK constraint).
The CLI rejects empty/whitespace-only usernames before insert. DB
treats username as opaque text key.

### §1.2 Username format
- Required: non-empty, ≤64 chars, no leading/trailing whitespace, no
  newline / tab / control characters.
- Case-sensitive (no automatic lowercasing) to keep storage = display.

## §2 Hashing
### §2.1 Algorithm
`argon2id` via `golang.org/x/crypto/argon2`.

### §2.2 Parameters
```go
const (
    argonTime    uint32 = 1
    argonMemory  uint32 = 64 * 1024 // 64 MB
    argonThreads uint8  = 4
    argonKeyLen  uint32 = 32
    saltLen      int    = 16
)
```

Hash bytes are encoded as a PHC string:
```
$argon2id$v=19$m=65536,t=1,p=4$<b64-salt>$<b64-hash>
```

### §2.3 Verify timing
On unknown username, the verifier MUST still perform a full argon2id
compare against a precomputed dummy hash, then return generic
`ErrInvalidCredentials`. This makes the wall-clock cost identical
whether the user exists or not.

## §3 Login handler extension
### §3.1 Branch policy
In `HandleWebLogin`, after `isForm` is determined:

```go
if isForm {
    user := strings.TrimSpace(r.PostForm.Get("username"))
    pass := r.PostForm.Get("password") // no Trim on pass
    if user != "" || pass != "" {
        // Credential path (Spec 070)
        if user == "" || pass == "" {
            d.renderLoginError(w, r, dest, "Username and password are required.")
            return
        }
        if err := d.WebCredentials.VerifyAndTouch(ctx, user, pass); err != nil {
            d.renderLoginError(w, r, dest, "Invalid username or password.")
            return
        }
        // Reuse existing cookie/setup with the shared AuthToken.
        token = d.AuthToken
        userID = user
    }
}
```

The existing `token = strings.TrimSpace(r.PostForm.Get("token"))` only
runs when `user == "" && pass == ""`. Either credential present → no
token field is read.

### §3.2 Cookie value
On credential success, the cookie value is `d.AuthToken` (the same
shared token the existing token-form path would set). No new token
type. No mint. Cookie attrs identical to existing path.

### §3.3 No auth backend = no credential path
If `d.AuthToken == "" && !d.AuthConfig.Enabled`, the existing
early-rejection branch fires first ("Login is not available on this
deployment"). Credential path never runs.

## §4 CLI
### §4.1 Command shape
```
smackerel-core users add <username>
smackerel-core users set-password <username>
smackerel-core users list
```

`add` requires the user does NOT exist (refuse to overwrite — use
`set-password` to rotate). `set-password` requires the user DOES
exist (refuse to silently create).

### §4.2 Password input
- Read from TTY using `golang.org/x/term.ReadPassword` (no echo).
- Prompt twice; require match.
- Reject < 12 chars with a warning + abort.
- No upper bound on length (argon2id handles arbitrary length).

### §4.3 Wiring
New file `cmd/core/cli_users.go` with `runUsersCmd(args []string)` invoked
from `main.go` when `os.Args[1] == "users"`. Uses the same `*pgxpool.Pool`
init path as the long-running server — fail-loud on missing
`DATABASE_URL`. Does NOT start the HTTP server, NATS, NTFY etc.

## §5 Login form HTML
`internal/api/admin_ui_static/login.html` is templated. Add:

```html
<form method="POST" action="/v1/web/login">
  <input type="hidden" name="next" value="{{.Next}}">
  <fieldset>
    <legend>Sign in</legend>
    <label>Username
      <input type="text" name="username" autocomplete="username" required>
    </label>
    <label>Password
      <input type="password" name="password" autocomplete="current-password" required>
    </label>
    <button type="submit">Sign in</button>
  </fieldset>
</form>

<details>
  <summary>Machine client login (paste token)</summary>
  <form method="POST" action="/v1/web/login">
    <input type="hidden" name="next" value="{{.Next}}">
    <label>Token
      <input type="text" name="token" autocomplete="off">
    </label>
    <button type="submit">Submit token</button>
  </fieldset>
  </form>
</details>
```

The two forms POST to the same endpoint; the branch policy in §3.1
picks the right verifier based on which fields are present.

## §6 Dependencies wiring
In `cmd/core/main.go` / `cmd/core/wiring.go`, add to
`api.Dependencies`:

```go
WebCredentials webcreds.Repo
```

Initialized as:
```go
deps.WebCredentials = webcreds.NewPostgresRepo(svc.pg.Pool)
```

Before `api.NewRouter(deps)`. No env-var config — table existence is
sufficient.

## §7 Tests
### §7.1 Unit
- `internal/auth/webcreds/hasher_test.go`:
  - Round-trip: hash a password, verify same password → ok.
  - Wrong password → err.
  - Invalid PHC string → err.
  - Tampered hash → err.
- `internal/auth/webcreds/timing_test.go`:
  - Verify unknown-user latency is within ±20% of known-user-wrong-pw
    latency (use median of 50 runs, soak in TempDir).

### §7.2 Integration
- `internal/auth/webcreds/repo_pg_test.go`:
  - UpsertPassword(new user) → row inserted with non-empty PHC.
  - UpsertPassword(existing user) → row updated, created_at preserved.
  - VerifyAndTouch(known user, correct pw) → ok, last_login_at advances.
  - VerifyAndTouch(known user, wrong pw) → ErrInvalidCredentials, no
    last_login_at update.
  - VerifyAndTouch(unknown user, any pw) → ErrInvalidCredentials.

### §7.3 HTTP
- `internal/api/web_login_credential_test.go`:
  - POST form with username+password (matching) → 303 + cookie set
    to AuthToken value.
  - POST form with username+wrong password → 200 + render error,
    no cookie.
  - POST form with unknown username → 200 + render error, no cookie.
  - POST form with token only (no user/pass) → existing token path
    unchanged.
  - POST form with username but no password → 200 + render error.

### §7.4 CLI
- `cmd/core/cli_users_test.go`:
  - `runUsersCmd(["add", "user1"])` with piped password input creates
    a row.
  - `runUsersCmd(["add", "user1"])` second time → exit 2, "user
    already exists".
  - `runUsersCmd(["set-password", "user1"])` rotates hash, row count
    unchanged.
  - `runUsersCmd(["set-password", "ghost"])` → exit 2, "no such
    user".
  - `runUsersCmd(["list"])` prints "user1" plus created/last_login
    columns.

## §8 Operator workflow
1. Deploy new image (existing promote.sh path).
2. `tailscale ssh <operator>@<deploy-host> -- docker exec -it smackerel-<env>-smackerel-core-1 smackerel-core users add operator`
3. Open https://<deploy-host>.<tailnet>.ts.net/login → enter `operator` + the
   password → submitted → redirected to `/`.

## §9 Failure modes
| Symptom | Likely cause | Operator action |
|---------|-------------|-----------------|
| Login form shows "Invalid username or password" for every attempt | User row missing OR password rotated since last knowledge | `users list` to confirm, `users set-password` to rotate |
| `smackerel-core users add` errors `pq: relation "web_user_credentials" does not exist` | Migration 044 didn't run | Check migrate logs; re-run `migrate up` |
| Browser stuck at `/login` after submit | Cookie not set due to `Secure` attr + plain HTTP | Production deploys MUST be HTTPS; home-lab is fronted by Caddy TLS → cookie sets fine |

## §10 Out of scope (anti-pattern catalog)
- Server-side session table (cookie value IS the auth token; same
  model as the existing token path; no new table needed).
- Password reset emails.
- Username case-folding (case-sensitive, by design).
- LDAP/OIDC bridge (separate future spec).
