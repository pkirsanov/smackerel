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
PHC, and `TestVerify_TimingParityWithinConstantFactor` — timing parity
for known-wrong-password vs unknown-user paths) and `repo_test.go`
(`TestValidateUsername` table, 13 sub-cases). Exit 0.
**Claim Source:** executed.

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
--- PASS: TestWebLogin_Credential_ValidMatch_RedirectsAndSetsCookie (0.10s)
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
exercised by `TestWebLogin_Credential_ValidMatch_RedirectsAndSetsCookie`
(302/303 redirect + `Set-Cookie: auth_token=...`). **Claim Source:** executed.

### scope-3 evidence

Command (round-2 harden remediation re-run via the sanctioned runner;
cmd/core package section):
`./smackerel.sh test unit --go --go-run 'TestRunUsers|TestDispatchUsersSubcommand|TestRunUsersCommand_MissingArgs_Exit2' --verbose`

```
=== RUN   TestRunUsersAdd_CreatesNewUser
user "operator" created
--- PASS: TestRunUsersAdd_CreatesNewUser (0.34s)
=== RUN   TestRunUsersAdd_RefusesExistingUser
smackerel-core users add: user "operator" already exists (use `set-password` to rotate)
--- PASS: TestRunUsersAdd_RefusesExistingUser (0.23s)
=== RUN   TestRunUsersAdd_UsageWhenMissingArg
usage: smackerel-core users add <username>
--- PASS: TestRunUsersAdd_UsageWhenMissingArg (0.00s)
=== RUN   TestRunUsersAdd_RejectsEmptyUsername
smackerel-core users add: username must not be empty
smackerel-core users add: username must not be empty
--- PASS: TestRunUsersAdd_RejectsEmptyUsername (0.00s)
=== RUN   TestRunUsersAdd_RejectsShortPassword
smackerel-core users add: webcreds: password must be at least 12 characters
--- PASS: TestRunUsersAdd_RejectsShortPassword (0.00s)
=== RUN   TestRunUsersAdd_RejectsMismatchedConfirmation
smackerel-core users add: passwords do not match
--- PASS: TestRunUsersAdd_RejectsMismatchedConfirmation (0.00s)
=== RUN   TestRunUsersSetPassword_RotatesExistingUser
password for "operator" rotated
--- PASS: TestRunUsersSetPassword_RotatesExistingUser (0.25s)
=== RUN   TestRunUsersSetPassword_RefusesMissingUser
smackerel-core users set-password: no such user "ghost" (use `add` to create)
--- PASS: TestRunUsersSetPassword_RefusesMissingUser (0.11s)
=== RUN   TestRunUsersSetPassword_RejectsShortPassword
smackerel-core users set-password: webcreds: password must be at least 12 characters
--- PASS: TestRunUsersSetPassword_RejectsShortPassword (0.11s)
=== RUN   TestRunUsersList_PrintsHeaderAndRows
--- PASS: TestRunUsersList_PrintsHeaderAndRows (0.17s)
=== RUN   TestRunUsersCommand_MissingArgs_Exit2
usage: smackerel-core users <add|set-password|list> [args...]
usage: smackerel-core users <add|set-password|list> [args...]
--- PASS: TestRunUsersCommand_MissingArgs_Exit2 (0.00s)
=== RUN   TestDispatchUsersSubcommand_UnknownSubcommand_Exit2
smackerel-core users: unknown subcommand "bogus" (want add|set-password|list)
--- PASS: TestDispatchUsersSubcommand_UnknownSubcommand_Exit2 (0.00s)
=== RUN   TestDispatchUsersSubcommand_RoutesToKnownSubcommands
user "operator" created
password for "operator" rotated
--- PASS: TestDispatchUsersSubcommand_RoutesToKnownSubcommands (0.28s)
PASS
ok      github.com/smackerel/smackerel/cmd/core 1.977s
```

`cmd/core/main.go:54` routes `os.Args[1] == "users"` to
`runUsersCommand(ctx, os.Args[2:])` before the HTTP server starts.
Password input uses `golang.org/x/term.ReadPassword`; tests inject a
scripted prompter and route through the `dispatchUsersSubcommand` seam,
so unknown-subcommand (exit 2) and missing-args (exit 2) branches are
exercised without a live Postgres. The round-2 harden remediation
replaced the prior no-op `TestRunUsersCommand_UnknownSubcommand`
(which only asserted `MinPasswordLength==12`) with the real dispatch
tests shown above. **Claim Source:** executed.

### scope-4 evidence
Deferred — operator-action. See SCOPE-4 acceptance criteria in
`scopes.md`. No in-process evidence to capture until the operator runs
`promote.sh --target self-hosted`, executes `users add operator` in the
live container, and confirms browser login.

## Completion Statement
SCOPE-1, SCOPE-2, and SCOPE-3 are implementation-complete with
executed unit + integration evidence (see Test Evidence above) and
per-package `go vet` + `gofmt` clean across `internal/auth/webcreds/`,
`internal/api/`, and `cmd/core/`. SCOPE-4 is intentionally deferred to
the operator: it requires a live self-hosted deploy (`promote.sh
--target self-hosted`) plus an interactive browser smoke that no
in-process agent can perform. SCOPE-4 DoD items remain `[ ]` as
explicit acceptance criteria; matching `uservalidation.md` items also
remain `[ ]` until the operator runs through the login flow.

Spec status is `done` (certified 2026-06-06T17:00Z, auditVerdict
`passed-with-known-drift`). SCOPE-1/2/3 are certified-complete; the five
SCOPE-4 operator-action DoD items are recorded as
`certification.unresolvedFindings` (OP-070-SCOPE4-*) with explicit
operator acceptance criteria, per the BUG-045-002 precedent. SCOPE-4
and its matching `uservalidation.md` items remain `[ ]` until the
operator runs the live deploy + browser smoke.

Round-2 harden remediation (this run) corrected documentation and
test-name drift (F1–F7) and reconciled the `## Status` header plus the
login rate-limit number in `spec.md` to match the code and the certified
state. Because those two edits touch protected planning artifacts after
`certifiedAt`, `state.json.requiresRevalidation` is set to `true` so
bubbles.validate / spec-review must re-certify.

## Validation Evidence

### round-2 recert validation (2026-06-07, bubbles.validate, mapped mode harden-to-doc)

Independent G020/G071 re-certification of the round-2 harden remediation
(7 findings remediated by bubbles.implement). **Verdict: BLOCKED — a fresh
validate `done` re-certification is NOT mechanically achievable.** The
remediation itself (F1–F7) is verified GREEN; the blocker is that the
changed-profile done-spec audit holds this *touched + recertified* spec to
current full-delivery policy, which it fails on 39 pre-existing structural
known-drift items (8 missing specialist phases, missing report sections,
SCOPE-4 deferral, completedScopes count mismatch, etc.). Gate G088 alignment
(state-transition-guard Check 30) PASSES — the protected-artifact edits are
consistent with cert state. **Claim Source:** executed.

Remediation re-verification (all green):

```
$ ./smackerel.sh test unit --go --go-run 'TestLoginPage_RendersCredentialFields|TestDispatchUsersSubcommand|TestRunUsersCommand_MissingArgs|TestVerify_TimingParityWithinConstantFactor|TestWebLogin_Credential|TestHash|TestVerify|TestValidateUsername' --verbose
--- PASS: TestRunUsersCommand_MissingArgs_Exit2 (0.00s)
--- PASS: TestDispatchUsersSubcommand_UnknownSubcommand_Exit2 (0.00s)
--- PASS: TestDispatchUsersSubcommand_RoutesToKnownSubcommands (0.60s)
ok      github.com/smackerel/smackerel/cmd/core 1.125s
--- PASS: TestWebLogin_Credential_ValidMatch_RedirectsAndSetsCookie (0.00s)
--- PASS: TestWebLogin_Credential_WrongPassword_NoCookie (0.00s)
--- PASS: TestWebLogin_Credential_UnknownUser_NoCookie_SameError (0.00s)
--- PASS: TestWebLogin_Credential_MissingPassword (0.00s)
--- PASS: TestWebLogin_Credential_MissingUsername (0.00s)
--- PASS: TestWebLogin_Credential_NilRepo_RejectedWithError (0.00s)
--- PASS: TestLoginPage_RendersCredentialFields (0.00s)
ok      github.com/smackerel/smackerel/internal/api     0.313s
    hasher_test.go:113: median timings: known-wrong=165.203095ms unknown=116.423525ms
--- PASS: TestVerify_TimingParityWithinConstantFactor (9.64s)   [ratio 0.70, within 0.5..2.0]
--- PASS: TestValidateUsername (0.02s)  [13 sub-cases]
ok      github.com/smackerel/smackerel/internal/auth/webcreds   11.904s
[go-unit] go test ./... finished OK
```

```
$ ./smackerel.sh check
Config is in sync with SST
env_file drift guard: OK
scenario-lint: OK
CHECK_EXIT=0
```

Mechanical gate results (G071 — executed, real exit codes):

```
$ bash .github/bubbles/scripts/state-transition-guard.sh specs/070-web-username-password-login
🔴 TRANSITION BLOCKED: 39 failure(s), 2 warning(s)
  Check 30 (Gate G088 post-cert spec edit): ✅ PASS (alignment OK)
  Dominant failures (all pre-existing full-delivery known-drift, NOT remediation-caused):
   - Gate G022: 8 specialist phases missing (regression/simplify/gaps/harden/stabilize/security/docs/chaos) + spec-review
   - Check 4 / G040 / G041: SCOPE-4 operator-action — 6 unchecked DoD + 'Deferred — operator-action' non-canonical status + deferral language
   - Check 5 / Check 15 (G027): completedScopes count (1) != artifact Done count (3)
   - Check 8A/8C: regression-E2E + shared-infra planning rows missing
   - Check 13/13B: artifact-lint fail + missing '### Code Diff Evidence' (G053)
   - Check 17/21/31: missing structured commit msg + spec-review phase + inter-spec dep (G089)
GUARD_EXIT=1

$ bash .github/bubbles/scripts/artifact-lint.sh specs/070-web-username-password-login
Artifact lint FAILED with 30 issue(s).   [8 missing phases + missing report sections + 4 short evidence blocks]
LINT_EXIT=1

$ bash .github/bubbles/scripts/done-spec-audit.sh --profile changed specs/070-web-username-password-login
Current-policy failures for changed/reopened/newly promoted specs:
- specs/070-web-username-password-login
DONE_AUDIT_EXIT=1

$ bash .github/bubbles/scripts/inter-spec-dependency-guard.sh specs/070-web-username-password-login
G089: status 'done' while requiresRevalidation:true unresolved — demote or recertify after revalidation
G089: specDependsOn dangling: specs/057-form-encoded-web-login (no state.json)
DEP_EXIT=1
```

Finding verification (F1–F7 — all confirmed-fixed by independent source read + executed tests):

| Finding | What it fixed | Independent verification | Status |
|---------|---------------|--------------------------|--------|
| F1 | no-op `TestRunUsersCommand_UnknownSubcommand` (only asserted `MinPasswordLength==12`) replaced with real dispatch tests | 3 new tests PASS; routing+rotation+list assertions confirmed in diff | confirmed-fixed |
| F2 | AC-6 login-page credential-field render coverage gap | `TestLoginPage_RendersCredentialFields` PASS (username/password render, token demoted to `<details>`, ordering guard) | confirmed-fixed |
| F3 | misleading timing test name `…WithinTwentyPercent` (band was already 0.5..2.0) | renamed `…WithinConstantFactor` + honest comment; PASS, ratio 0.70 | confirmed-fixed |
| F4 | `dispatchUsersSubcommand` seam extraction enabling F1 | diff confirms seam; tests exercise it without live PG | confirmed-fixed |
| F5 | 5 cross-cutting `spec 063`→`Spec 070` comment fixes | diff = exactly 5 (health.go, web_login.go, hasher.go, migration ×2); zero `063` left in internal/+cmd; QF/scheduler 063 untouched (no QF/scheduler file in diff) | confirmed-fixed |
| F6 | `spec.md` `## Status` draft→done | spec.md shows `done`; matches cert state | confirmed-fixed |
| F7 | `spec.md` login rate-limit 10/min→20/min | router.go:308 `/v1/web/login` is inside `LimitByIP(20, 1*time.Minute)` group (line 307); spec.md matches | confirmed-fixed |

**Source-claim cross-checks (G020):** `cmd/core/main.go:54` routes `users` →
`runUsersCommand` (confirmed); `internal/auth/webcreds/hasher.go` short-password
message is `webcreds: password must be at least 12 characters` (confirmed);
`internal/api/web_login.go` credential branch matches design §3.1 (confirmed).

**Blocker disposition:** verdict BLOCKED. `requiresRevalidation` left `true`,
`status` left `done`, `certifiedAt` NOT advanced (no clean cert occurred). The
remediation is sound; the blocker is governance-level (touched done-spec held to
current full-delivery policy it never satisfied — certified originally under
`passed-with-known-drift`). Routed to bubbles.workflow / bubbles.spec-review for
the known-drift recert decision; scopes.md/state.json structural fixes (SCOPE-3
citation drift, non-canonical SCOPE-4 status, completedScopes count, planning
rows, missing report sections, specs/057 state.json) routed to bubbles.plan /
bubbles.test / bubbles.docs if the full current-policy remediation path is taken.
**Claim Source:** executed.

## Stabilize Pass (stochastic-quality-sweep Round 5, 2026-06-07)

Diagnostic stability probe by `bubbles.stabilize` (role: diagnostic). Scope
of the probe: the web username/password login path
(`internal/auth/webcreds/`, `internal/api/web_login.go`,
`cmd/core/cmd_users.go`, router wiring). Protected artifacts (`spec.md`,
`design.md`, `scopes.md`) were NOT edited — G088 known-drift basis kept
intact. Verdict: 🟢 STABLE (0 findings; 2 info-level observations).

### Build + unit-test health

Command + result (whole-module compile succeeded; every targeted spec-070
unit test passed):

```
$ ./smackerel.sh test unit --go \
  --go-run 'TestWebLogin_Credential|TestWebLogin_TokenOnly|TestRunUsers|TestDispatchUsersSubcommand|TestValidateUsername|TestHash_|TestVerify_|TestDummyHash' \
  --verbose
[go-unit] go test ./... finished OK
ok  github.com/smackerel/smackerel/cmd/core                2.042s
ok  github.com/smackerel/smackerel/internal/api            0.532s
ok  github.com/smackerel/smackerel/internal/auth/webcreds  11.244s
```

Timing-parity probe (AC-5 user-enumeration guard) result:
`median timings: known-wrong=138.960444ms unknown=138.331146ms`
(ratio ≈ 0.995, well inside the documented 0.5..2.0 band).
**Claim Source:** executed.

### Operational-surface verifications (independent source read)

- Rate limiting: `/v1/web/login` is inside the
  `httprate.LimitByIP(20, 1*time.Minute)` group — `internal/api/router.go`
  lines 307-310 (confirmed by read).
- argon2id cost is bounded and not recomputed per request: the unknown-user
  timing-parity dummy hash is precomputed once in `webcreds.init()`
  (`internal/auth/webcreds/hasher.go`), so each login attempt performs exactly
  one ~138ms / 64MB argon2id evaluation regardless of user existence
  (confirmed by read).
- Resource lifecycle: the Postgres repo is constructed once over the shared
  `svc.pg.Pool` at wiring time with fail-loud error propagation
  (`cmd/core/wiring.go:417`); the `users` CLI opens and `defer`-closes its own
  pool (`cmd/core/cmd_users.go:52-55`). No per-request pool creation, no
  goroutine/connection leak (synchronous handler, no spawned goroutines).
- SST / fail-loud: the CLI rejects empty `DATABASE_URL` with exit 1
  (`cmd/core/cmd_users.go:48-50`); `NewPostgresRepo(nil)` returns an error
  (refuses silent no-op). No new env config and no `${VAR:-default}` /
  `unwrap_or` fallback introduced by this feature (confirmed by read).
- Request hardening: form body capped at 64KB via
  `http.MaxBytesReader(w, r.Body, 64*1024)` and DB calls are request-context
  bound (`internal/api/web_login.go`).
- Migration `044_web_user_credentials.sql` present; PG repo tests are
  `//go:build integration` (correctly excluded from the unit profile and
  skip when `DATABASE_URL` is unset).
**Claim Source:** executed.

### Observations (G092 — non-blocking, NOT findings)

- **OBS-070-A (info, observability):** On a Postgres outage,
  `PostgresRepo.VerifyAndTouch` returns a wrapped DB error which
  `HandleWebLogin` collapses into the generic "Invalid username or password."
  render plus an INFO-level `web_login_credential_fail` log; the underlying DB
  error is not surfaced at WARN/ERROR on this path, so a real outage reads as a
  flood of failed logins. This is safe-by-design (fails closed, no enumeration
  leak) and is an operability nicety, not a defect. If ever pursued it is a
  code-behavior change (distinguish `errors.Is(err, webcreds.ErrInvalidCredentials)`
  from transient DB errors and log the latter at WARN) → owner `bubbles.implement`.
  Not routed now: low/info, current behavior is correct and secure.
- **OBS-070-B (info, resource):** Each login attempt costs ~138ms CPU + ~64MB
  transient memory (argon2id m=64MB). `httprate` caps request *rate*
  (20/min/IP) but not *concurrency*, so a simultaneous burst within one window
  could spike memory (N×64MB). This is the spec's documented, accepted posture
  (`spec.md` Security Model: "No account lockout (Caddy / future spec can layer
  rate limiting)") and the tailnet-edge bind limits the surface to trusted
  peers. Tightening it (concurrency semaphore around argon2 or parameter
  retune) would be a design change → owner `bubbles.design` / `bubbles.plan`,
  and is out of the stabilize lane. Recorded only; not a finding.
**Claim Source:** executed.

## Test Pass (stochastic-quality-sweep Round 12, 2026-06-07)

Test-diagnostic + gap-closure probe by `bubbles.test` (role: test-diagnostic).
Protected artifacts (`spec.md`, `design.md`, `scopes.md`) were NOT edited —
G088 known-drift basis kept intact. Verdict: 🟢 1 finding, CLOSED with a real
adversarial test (RED→GREEN). Round-2 remediations (AC-6 render test, CLI
dispatch seam + exit-code tests, timing-parity name fix) were re-verified
present and passing; not re-reported.

### Finding F-070-R12-001 (medium) — login rate-limit had no executable guard

Spec 070 Security Model documents `/v1/web/login` form posts as rate-limited by
the `httprate.LimitByIP(20, 1*time.Minute)` group in `internal/api/router.go`.
The OAuth entry points have router-level rate-limit regression tests
(`TestOAuthStart_RateLimited`, `TestOAuthCallback_RateLimited`,
`TestSecR30_OAuthRateLimit_*`), but `/v1/web/login` had NONE: the credential /
form unit tests call `deps.HandleWebLogin` directly (bypassing router
middleware), and the Scope-03 chaos test deliberately gives each goroutine a
distinct `RemoteAddr` so the limiter never engages. A refactor moving the route
out of the group would silently weaken brute-force protection with zero test
failing. Closed by new file `internal/api/web_login_ratelimit_test.go` (2 tests)
driving the real `NewRouter(deps)`.

GREEN — both tests pass against the real router; the 20/min/IP budget engages on
request 21 (firstFail=20) and a fresh IP is still admitted:

```
$ ./smackerel.sh test unit --go --go-run '^TestWebLogin_RateLimit' --verbose
=== RUN   TestWebLogin_RateLimited_PerIP
    web_login_ratelimit_test.go:100: statuses (one IP, in order)=[401 401 401 401 401 401 401 401 401 401 401 401 401 401 401 401 401 401 401 401 429] firstFail=20
2026/06/07 18:03:13 INFO request method=POST path=/v1/web/login status=429 duration_ms=0 request_id=3cc282e5197f/xWp9ymUEyv-000021
--- PASS: TestWebLogin_RateLimited_PerIP (0.00s)
--- PASS: TestWebLogin_RateLimit_PerIP_FreshIPAdmitted (0.00s)
ok      github.com/smackerel/smackerel/internal/api     0.569s
```

RED (teeth proof) — `/v1/web/login` TEMPORARILY moved out of the `LimitByIP`
group in `router.go` (reverted immediately; `git diff internal/api/router.go`
empty afterward). Every POST is then admitted and both tests FAIL on the exact
regression they guard:

```
$ ./smackerel.sh test unit --go --go-run '^TestWebLogin_RateLimit' --verbose
    web_login_ratelimit_test.go:100: statuses (one IP, in order)=[401 401 401 401 401 401 401 401 401 401 401 401 401 401 401 401 401 401 401 401 401 401 401 401 401 401 401 401 401 401] firstFail=-1
    web_login_ratelimit_test.go:103: expected 429 after exceeding the 20/min/IP budget on /v1/web/login, but 30 consecutive POSTs were all admitted ... the login route is no longer inside the httprate.LimitByIP(20, 1*time.Minute) group
--- FAIL: TestWebLogin_RateLimited_PerIP (0.00s)
    web_login_ratelimit_test.go:131: precondition failed: IP-A never hit 429 within 40 requests — the /v1/web/login limiter is not engaging at all
--- FAIL: TestWebLogin_RateLimit_PerIP_FreshIPAdmitted (0.00s)
FAIL    github.com/smackerel/smackerel/internal/api     0.331s
```

**Claim Source:** executed.

### Full Go unit suite (broad regression)

The spec-070 surface packages each pass (`internal/api`, `internal/auth/webcreds`,
`cmd/core`). The suite exit is 1 due to THREE pre-existing failures in unrelated
packages — none touches package `api`, and an additive test file cannot cause
them: spec 032 docs drift (`docs/Development.md` missing 5 prompt contracts),
spec 073 canary (`node`/`dart` not installed in this container), and a
`cmd/config-validate` drive-config fixture drift (`DRIVE_PROVIDER_GOOGLE_*`):

```
$ ./smackerel.sh test unit --go ; echo "EXIT_CODE=$?"
ok      github.com/smackerel/smackerel/cmd/core 1.767s
ok      github.com/smackerel/smackerel/internal/api     8.045s
ok      github.com/smackerel/smackerel/internal/auth/webcreds   8.191s
    doc_freshness_test.go:205: docs/Development.md is STALE: 5 prompt contract(s) on disk are undocumented
    render_descriptor_canary_test.go:125: node not on PATH; the spec 073 cross-language renderer canary requires both node and dart
    main_test.go:160: expected exit 0 with fixture-model override, got 1; stderr="ERROR: missing or invalid required drive configuration: DRIVE_PROVIDER_GOOGLE_HTTP_RESPONSE_HEADER_TIMEOUT_SECONDS"
EXIT_CODE=1
```

**Claim Source:** executed.

## Gaps Pass (stochastic-quality-sweep, 2026-06-17)

Gap-diagnostic + closure probe by `bubbles.workflow` mapped mode `gaps-to-doc`
(role: gap-diagnostic; `executionModel: parent-expanded-child-mode`). Protected
artifacts (`spec.md`, `design.md`, `scopes.md`) were NOT edited — the G088
known-drift basis is preserved exactly as in the Round-5 stabilize and Round-12
test passes. Spec status left `done` (gaps-to-doc ceiling is `docs_updated`; no
promotion attempted). Verdict: 🟢 1 finding, CLOSED with a real adversarial test
(RED→GREEN). Only spec-070's owned surface was touched.

### Coverage cross-reference (spec requirement → guarding test)

| Spec requirement | Guarding test(s) | Status |
|------------------|------------------|--------|
| UC-1 / AC-2 — form login → 303 + cookie = shared AuthToken | `TestWebLogin_Credential_ValidMatch_RedirectsAndSetsCookie` | covered |
| UC-5 — wrong password → error, no cookie | `TestWebLogin_Credential_WrongPassword_NoCookie` | covered |
| UC-6 / AC-5 — unknown user → same generic error + timing parity | `TestWebLogin_Credential_UnknownUser_NoCookie_SameError`, `TestVerify_TimingParityWithinConstantFactor` | covered |
| UC-7 / AC-4 — token-only form regression unchanged | `TestWebLogin_TokenOnly_RegressionUnchanged` | covered |
| AC-2 — missing-field handling | `TestWebLogin_Credential_MissingPassword`, `…_MissingUsername` | covered |
| §3.3 — credential post with nil repo rejected (no silent fall-through) | `TestWebLogin_Credential_NilRepo_RejectedWithError` | covered |
| AC-6 — login form renders username+password, token demoted | `TestLoginPage_RendersCredentialFields` (Round-2 F2) | covered |
| Security Model — `/v1/web/login` rate-limited 20/min/IP | `TestWebLogin_RateLimited_PerIP`, `…_FreshIPAdmitted` (Round-12) | covered |
| UC-3 / AC-3 — `users add` creates row; refuses existing; guards | `TestRunUsersAdd_*`, `TestDispatchUsersSubcommand_RoutesToKnownSubcommands` | covered |
| UC-4 / AC-3 — `users set-password` rotates; refuses missing | `TestRunUsersSetPassword_RotatesExistingUser`, `…_RefusesMissingUser` | covered |
| CLI invocation contract (missing-arg / unknown-subcommand exit codes) | `TestRunUsersCommand_MissingArgs_Exit2`, `TestDispatchUsersSubcommand_UnknownSubcommand_Exit2` | covered |
| AC-1 — table + 4 columns; created_at preserved on rotate; last_login bump | `internal/auth/webcreds/repo_pg_test.go` (integration) | covered |
| §2 hashing — round-trip / wrong-pw / tamper / malformed PHC | `TestHash_RoundTrip`, `TestVerify_WrongPassword`, `…_TamperedHash`, `…_MalformedPHC` | covered |
| §2.2 — **per-hash random salt (saltLen=16, crypto/rand)** | **none → `TestHash_SaltIsUniquePerInvocation`** | **GAP — CLOSED this round** |

### Finding F-070-GAPS-001 (medium) — per-hash salt uniqueness had no executable guard

Spec 070 §2.2 / Security Model specifies argon2id with a 16-byte salt read from
`crypto/rand` on every `Hash` call. Per-hash random salt is the property that
stops identical passwords from producing identical stored hashes (password-reuse
leak across users + offline precompute). The existing hasher suite had **zero**
coverage for it: `TestHash_RoundTrip`, `TestVerify_WrongPassword`,
`TestVerify_TamperedHash`, `TestVerify_MalformedPHC` and the timing-parity test
all hold whether the salt is random or a fixed/zero block, and the integration
`TestPostgresRepo_UpsertRotatePreservesCreatedAt` only compares hashes of
*different* passwords. A regression dropping the `rand.Read(salt)` CSPRNG read
(zero salt, shared salt, or a deterministic stub) would have shipped undetected.
Closed by an additive test (`internal/auth/webcreds/hasher_test.go`, +39 lines,
0 deletions) asserting that hashing the same password 16× yields all-distinct,
each-still-verifiable PHC strings.

GREEN — new test passes against the real random-salt `Hash` (full hasher set via
the sanctioned runner; the spec-094 `internal/config` RED tests are excluded by
the `-run` selector, so they do not execute):

```
$ ./smackerel.sh test unit --go --go-run 'TestHash|TestVerify|TestDummyHash' --verbose
=== RUN   TestHash_RoundTrip
--- PASS: TestHash_RoundTrip (0.22s)
=== RUN   TestHash_RejectsShortPasswords
--- PASS: TestHash_RejectsShortPasswords (0.00s)
=== RUN   TestHash_SaltIsUniquePerInvocation
--- PASS: TestHash_SaltIsUniquePerInvocation (5.28s)
=== RUN   TestVerify_WrongPassword
--- PASS: TestVerify_WrongPassword (0.67s)
=== RUN   TestVerify_TamperedHash
--- PASS: TestVerify_TamperedHash (0.21s)
=== RUN   TestVerify_MalformedPHC
--- PASS: TestVerify_MalformedPHC (0.00s)
=== RUN   TestDummyHash_VerifiesAgainstNothing
--- PASS: TestDummyHash_VerifiesAgainstNothing (0.32s)
=== RUN   TestVerify_TimingParityWithinConstantFactor
    hasher_test.go:152: median timings: known-wrong=132.785858ms unknown=122.6541ms
--- PASS: TestVerify_TimingParityWithinConstantFactor (7.58s)
PASS
ok      github.com/smackerel/smackerel/internal/auth/webcreds   15.122s
[go-unit] go test ./... finished OK
GREEN_EXIT=0
```

RED (teeth proof) — `Hash` in `internal/auth/webcreds/hasher.go` TEMPORARILY
forced to a constant salt (`for i := range salt { salt[i] = 0x42 }` after the
`rand.Read`; reverted immediately, `git diff internal/auth/webcreds/hasher.go`
empty afterward). The new test FAILS on the exact regression it guards, while
**every** pre-existing hasher/verify test still PASSES — proving the gap was
real and the test is non-tautological:

```
$ ./smackerel.sh test unit --go --go-run 'TestHash_|TestVerify_' --verbose
=== RUN   TestHash_RoundTrip
--- PASS: TestHash_RoundTrip (0.31s)
=== RUN   TestHash_RejectsShortPasswords
--- PASS: TestHash_RejectsShortPasswords (0.00s)
=== RUN   TestHash_SaltIsUniquePerInvocation
    hasher_test.go:54: Hash("correct-horse-battery-staple") produced a DUPLICATE PHC string on attempt #1 — salt is not unique per invocation (constant/zero-salt regression). Identical passwords MUST hash to distinct PHC strings.
--- FAIL: TestHash_SaltIsUniquePerInvocation (0.30s)
=== RUN   TestVerify_WrongPassword
--- PASS: TestVerify_WrongPassword (1.00s)
=== RUN   TestVerify_TamperedHash
--- PASS: TestVerify_TamperedHash (0.45s)
=== RUN   TestVerify_MalformedPHC
--- PASS: TestVerify_MalformedPHC (0.00s)
=== RUN   TestVerify_TimingParityWithinConstantFactor
--- PASS: TestVerify_TimingParityWithinConstantFactor (8.72s)
FAIL
FAIL    github.com/smackerel/smackerel/internal/auth/webcreds   11.141s
RED_EXIT=1
```

Restore confirmed (`hasher.go` returned to its exact committed state via the
edit tool, NOT `git restore`):

```
$ git --no-pager diff -- internal/auth/webcreds/hasher.go ; echo "exit=$?"
exit=0   # empty diff — constant-salt mutation fully reverted
```

Post-restore GREEN re-confirmation against the real-salt `Hash`:

```
$ ./smackerel.sh test unit --go --go-run 'TestHash_SaltIsUniquePerInvocation|TestHash_RoundTrip|TestDummyHash' --verbose
=== RUN   TestHash_RoundTrip
--- PASS: TestHash_RoundTrip (0.39s)
=== RUN   TestHash_SaltIsUniquePerInvocation
--- PASS: TestHash_SaltIsUniquePerInvocation (3.08s)
=== RUN   TestDummyHash_VerifiesAgainstNothing
--- PASS: TestDummyHash_VerifiesAgainstNothing (0.24s)
ok      github.com/smackerel/smackerel/internal/auth/webcreds   3.970s
[go-unit] go test ./... finished OK
FINAL_GREEN_EXIT=0
```

**Claim Source:** executed.

### Foreign attribution (NOT spec 070)

The working tree carries large uncommitted cross-spec sweep work. The known
pre-existing RED baseline in `internal/config` (spec-094 weather added required
`ASSISTANT_SKILLS_WEATHER_*` SST keys without updating the shared
`internal/config/validate_test.go::setRequiredEnv` fixture) is a **spec-094
defect**, outside spec 070's owned surface — attributed, NOT fixed here. All
spec-070 evidence above used a `-run` selector that excludes `internal/config`,
so those tests never executed and contributed nothing to the captured results.
No other spec's uncommitted files were modified, reverted, committed, or stashed.

**Claim Source:** executed.
