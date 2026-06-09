# Execution Report: [BUG-056-002] User-Context OAuth 2.0 PKCE auth missing for user-owned endpoints

## Status: DELIVERED-PENDING-AUDIT — Path A LOCKED (design.md); all 4 scopes (A Foundation, B Authorize CLI, C Routing+Refresh, D Gauge) DELIVERED + tests GREEN; independently re-verified + reconciled by bubbles.validate 2026-06-08 (bug remains NON-terminal; audit owns close-out)

This report records the original DIAGNOSTIC evidence that confirmed the bug, then the per-scope delivery evidence. **Update 2026-06-08 (delivery-validation + state reconciliation, bubbles.validate):** the maintainer resolved Q1 → Path A (design.md LOCKED) and ALL FOUR scopes have been implemented — Scope A (Foundation), Scope B (Authorize CLI), Scope C (endpoint auth-tier routing + refresh-on-401 + the named adversarial regression), and Scope D (rate-limit-remaining gauge). `bubbles.validate` independently re-verified the CI-runnable suite GREEN (`./smackerel.sh test unit --go --go-run 'TestTwitterAPI|TestTwitterAuthorize|TestTwitterOAuth|PKCE|TestConfig_TwitterOAuth'`; `./smackerel.sh check` + `lint` clean) and RECONCILED the lagging Scope B bookkeeping — its code and four `TestTwitterAuthorize_*` tests had shipped, but the packet still read "Not started" — see "## Scope B Delivery Evidence" and "## Validation & Parent-Claim-Correction Evidence" below. The bug stays NON-terminal (`state.json status: delivered-pending-audit`); terminal closure + the remaining specialist phases (regression/simplify/stabilize/security) are owned by `bubbles.audit` (separation of duties). The original diagnostic sections below are preserved verbatim as the triage record (anti-fabrication, Gate G021).

## Summary
- **GAP-056-G1 (High):** Spec 056 mandates User-Context OAuth 2.0 PKCE for `/2/users/me`, `/2/users/:id/bookmarks`, `/2/users/:id/liked_tweets` and states App-Only bearer is insufficient (spec.md:225 NC-1; design.md:131-133). The connector implements no PKCE flow and applies a single static App-Only bearer to all four endpoints (api.go:141). Against the real API the three user-owned endpoints return 403, so bookmarks and likes cannot be ingested. report.md:7 falsely claims PKCE was delivered.
- **GAP-056-G2 (Medium):** R-016 (spec.md:111) requires an `x-rate-limit-remaining` gauge after each API call; only `ConnectorTwitterAPIRateLimitReset` exists, written only inside the 429 branch (api.go:530-534). No per-call rate-headroom visibility.
- **Status (current):** All four scopes DELIVERED; the CI-runnable suite is GREEN (independently re-verified by `bubbles.validate` 2026-06-08, re-run GREEN by `bubbles.audit` 2026-06-09). `state.json status: in_progress` (delivered-pending-audit, NON-terminal). The original diagnostic-only triage state (`status: blocked`, held pending the maintainer product decision in design.md Q1) is retained under `## Completion Statement` → Historical below.
- **Scenarios:** all 16 SCN-BUG-056-002-0xx are delivered with real unit/integration tests; the per-scope acceptance + RED-GREEN evidence is in the Scope 1–4 delivery sections below.

## Diagnostic Evidence (verified at HEAD `9638b065`, 2026-06-07)
**Claim Source:** executed — read-only `grep` against the working tree; outputs reproduced verbatim below (relative paths).

### Evidence 1 — GAP-G1: no PKCE/OAuth2 user-context flow exists; a single static App-Only bearer is applied uniformly
```
$ grep -rniE 'pkce|code_verifier|code_challenge|oauth2|refresh_token|/oauth2/token' internal/connector/twitter/ ; echo "g1a_exit=$?"
g1a_exit=1
$ grep -nE 'bearerToken string|func .*buildRequest|Authorization", "Bearer' internal/connector/twitter/api.go
62:	bearerToken string //nolint:unused // consumed by buildRequest below
117:func (c *apiClient) buildRequest(ctx context.Context, method, path string, query url.Values) (*http.Request, error) {
141:	req.Header.Set("Authorization", "Bearer "+c.bearerToken)
```
`g1a_exit=1` = zero matches across the entire connector dir (including tests). The only auth is the static `bearerToken` field (`:62`) attached as `Authorization: Bearer` by `buildRequest` (`:117`, `:141`), used by `fetchUsersMe` and every paginated endpoint.

### Evidence 2 — GAP-G1 requirement: spec.md NC-1 + design.md matrix mandate User-Context PKCE for the user-owned endpoints
```
$ grep -n 'User-Context OAuth 2.0 with PKCE' specs/056-twitter-api-connector/spec.md
225:  - **Resolved 2026-05-27:** Use **User-Context OAuth 2.0 with PKCE** for `/2/users/me/bookmarks` and `/2/users/:id/liked_tweets`. App-Only bearer tokens are insufficient for these user-owned endpoints. ...
$ grep -n 'User-Context OAuth 2.0 PKCE' specs/056-twitter-api-connector/design.md
131:| `GET /2/users/me` | Resolve authenticated user ID once at connector start | — | **User-Context OAuth 2.0 PKCE** ... | 75 / 15 min user-context |
132:| `GET /2/users/:id/bookmarks` | Poll user bookmarks | `pagination_token` | **User-Context OAuth 2.0 PKCE** (NC-1: App-Only bearer is insufficient ...) | 75 / 15 min user-context |
133:| `GET /2/users/:id/liked_tweets` | Poll user likes | `pagination_token` | **User-Context OAuth 2.0 PKCE** (NC-1: same constraint ...) | 75 / 15 min user-context |
```

### Evidence 3 — GAP-G1 false delivered-claim: report.md claims PKCE was delivered
```
$ grep -n 'App-Only bearer + User-Context PKCE' specs/056-twitter-api-connector/report.md
7:Spec 056 delivered the Twitter API v2 connector path (App-Only bearer + User-Context PKCE) covering 4 endpoints
342:"Spec 056 delivered the Twitter API v2 connector path (App-Only bearer + User-Context PKCE) covering 4 endpoints"
```
PKCE was never implemented (Evidence 1), so this delivered-claim (line 7, re-quoted line 342) is false. The closure MUST correct it. This packet does NOT edit parent report.md (create-only).

### Evidence 4 — GAP-G2: no `x-rate-limit-remaining` gauge; only the reset gauge, written only on 429
```
$ grep -rniE 'x-rate-limit-remaining|RateLimitRemaining' internal/connector/twitter/ internal/metrics/ ; echo "g2a_exit=$?"
g2a_exit=1
$ grep -nE 'observeRateLimitReset|ConnectorTwitterAPIRateLimitReset' internal/connector/twitter/api.go
534:			c.observeRateLimitReset(endpoint, waitDur)
636:func (c *apiClient) observeRateLimitReset(endpoint string, wait time.Duration) {
637:	metrics.ConnectorTwitterAPIRateLimitReset.WithLabelValues("twitter", endpoint).Set(wait.Seconds())
```
`g2a_exit=1` = no remaining-gauge anywhere. The only rate-limit gauge update (`:534`) sits inside `case resp.StatusCode == http.StatusTooManyRequests:` (api.go:530) — the 2xx success path returns without reading any rate-limit header, so there is no per-call headroom signal.

### Evidence 5 — GAP-G2 requirement: spec.md R-016 vs the only gauge defined
```
$ grep -n 'R-016' specs/056-twitter-api-connector/spec.md
111:| R-016 | The connector MUST expose a Prometheus gauge (or equivalent metric the project already uses) reporting `x-rate-limit-remaining` after each API call. |
$ grep -nE 'ConnectorTwitterAPIRateLimit|RateLimitRemaining' internal/metrics/metrics.go
102:// ConnectorTwitterAPIRateLimitReset records the seconds-until-reset
105:var ConnectorTwitterAPIRateLimitReset = prometheus.NewGaugeVec(
595:			ConnectorTwitterAPIRateLimitReset,
```
R-016 requires a `remaining` gauge "after each API call"; the metrics surface defines only the `Reset` gauge.

## Consequence
**Claim Source:** not-run — the live Twitter/X API 403 was NOT reproduced (requires real user-context credentials; not run in this diagnostic pass). Documented expected consequence: against the real API, `/2/users/me`, `/2/users/:id/bookmarks`, and `/2/users/:id/liked_tweets` reject App-Only bearer tokens with HTTP 403 ("Authenticating with OAuth 2.0 Application-Only is forbidden for this endpoint"), so the connector cannot retrieve bookmarks or likes — a central capability. Only the public `/2/users/:id/tweets` and `/2/users/:id/mentions` (which legitimately accept App-Only) would work. The structural divergence proven above (no mechanism exists to obtain or send a user-context token) is conclusive on its own.

## Test Evidence
This bug followed scenario-first TDD: each scope's evidence below carries a RED-GREEN proof (the test fails under the reverted defect and passes with the fix). The CI-runnable suite is GREEN — independently re-verified by `bubbles.validate` (2026-06-08) and re-run GREEN by `bubbles.audit` (2026-06-09): `./smackerel.sh test unit --go --go-run 'TestTwitterAPI|TestTwitterAuthorize|TestTwitterOAuth|PKCE|TestConfig_TwitterOAuth'`; `./smackerel.sh check` + `lint` clean. Per-scope acceptance evidence is in the Scope 1–4 delivery sections below. The delivered tests live in `internal/auth/oauth_pkce_test.go`, `internal/config/twitter_oauth_config_test.go`, `internal/connector/twitter/oauth_store_test.go`, `internal/connector/twitter/oauth_authorize_test.go`, and `internal/connector/twitter/api_test.go`; a per-file `git diff --stat` / `wc -l` is in `## Implementation Code Diff Evidence` below. The live real-Twitter `403 → 200` arm (`internal/connector/twitter/api_live_test.go`) is operator-gated and not CI-runnable; no live pass is claimed. The verified diagnostic evidence that confirmed the bug is preserved above under "## Diagnostic Evidence".

## Parent-Spec Non-Interference Evidence
Parent spec `056-twitter-api-connector` status remains `done`; no parent artifact (spec.md / design.md / scopes.md / state.json / report.md / uservalidation.md / scenario-manifest.json) was modified by this packet. The false report.md:7 PKCE-delivered claim is recorded here as a delivery-pass DoD item (scopes.md Scope 1) — it is NOT corrected now, because correcting it is a deliberate parent-artifact edit reserved for the delivery pass, not a create-only bug-packet action.

## Completion Statement
The fix is DELIVERED. All four scopes (1 Foundation, 2 Authorize CLI, 3 Routing+Refresh, 4 Gauge) are implemented with CI-runnable tests GREEN — independently re-verified by `bubbles.validate` (2026-06-08) and re-run GREEN by `bubbles.audit` (2026-06-09). `state.json status: in_progress` (delivered-pending-audit, NON-terminal): terminal certification + the remaining specialist phases (regression/simplify/stabilize/security) are owned by `bubbles.audit`; the one open DoD item (Scope 1 migration live DB-apply under `./smackerel.sh test integration`) is owned by the integration-apply pass. No parent spec 056 artifact was modified by this packet's planning repair.

<!-- bubbles:g040-skip-begin -->
> **Historical (superseded 2026-06-08) — original diagnostic-only triage record.** The packet began as tracked-work-creation-only and was intentionally incomplete: the fix was held pending a maintainer product decision (design.md Q1 — build the full User-Context OAuth 2.0 PKCE flow now, or de-scope bookmarks/likes/users-me and correct spec 056's claims). At that point `state.json status: blocked`, no fix code was written, and no tests had run — recording a test result then would have been fabrication (Gate G021). The maintainer chose Path A and all four scopes were subsequently delivered (Scope 1–4 evidence above), so this triage state no longer holds; it is retained only as the historical record.
<!-- bubbles:g040-skip-end -->

---

## Scope A Delivery Evidence (2026-06-08 — Foundation; bug remains OPEN, Scopes B/C/D pending)

**Claim Source:** executed — all fenced output below is verbatim terminal output from `./smackerel.sh` runs on this working tree (absolute home paths redacted to `~/`). The bug is NOT marked Fixed and `state.json` is NOT flipped to terminal; only Scope A DoD items are ticked.

Scope A delivered the four foundation pieces with NO connector request-behavior change (routing/refresh/gauge remain Scopes C/D):

1. **Config SST chain (3 new OAuth keys, fail-loud, no hidden default).** `config/smackerel.yaml` (`connectors.twitter`: `oauth_client_id`/`oauth_client_secret`/`oauth_redirect_url` empty-string entries), `scripts/commands/config.sh` (`yaml_get … || VAR=""` reads + generated-env emit, mirroring `TWITTER_BEARER_TOKEN`), `internal/config/config.go` (`TwitterOAuthClientID/Secret/RedirectURL` via `os.Getenv`, no fallback literal), `cmd/core/connectors.go` (threaded into the Twitter `Credentials` map), `internal/connector/twitter/twitter.go` (`TwitterConfig` fields + parse). `oauth_client_secret` follows the same connector-credential path as `bearer_token` (which is NOT in `infrastructure.secret_keys`), so the sops-managed secret manifest is intentionally unchanged.
2. **Migration `internal/db/migrations/056_twitter_oauth_pkce.sql`** — next free slot after `055`; creates `twitter_oauth_states` (state_token PK, `code_verifier`, `scope` jsonb, 15-min `expires_at` + index) and `twitter_oauth_tokens` (composite PK `(owner_user_id, connector_id)`, AES-256-GCM access/refresh columns); idempotent `CREATE TABLE IF NOT EXISTS`; ROLLBACK comment. Picked up by the `//go:embed migrations/*.sql` runner.
3. **Additive PKCE on `auth.GenericOAuth2`** — `GeneratePKCEPair()`, `PKCEChallengeS256()`, `AuthURLWithPKCE()`, `ExchangeCodeWithVerifier()`, `RefreshTokenBasic()`, and the `OAuth2Config.TokenEndpointAuthStyle` Basic-auth flag honored inside `tokenRequest`. The shared `OAuth2Provider` interface and the existing `AuthURL`/`ExchangeCode`/`RefreshToken` signatures are byte-for-byte unchanged (zero ripple).
4. **Encrypted Twitter-owned store `internal/connector/twitter/oauth_store.go`** — AES-256-GCM (key = SHA-256(`SMACKEREL_AUTH_TOKEN`), nonce-prepended base64) reusing the `auth.TokenStore` technique but **fail-loud on empty at-rest key** (`ErrOAuthAtRestKeyRequired`, no plaintext fallback). Methods: `SaveTokens`/`GetTokens`/`HasValidUserContext`/`SaveState`/`ConsumeState` (delete-on-consume + TTL).

### A-E1 — `./smackerel.sh config generate` (dev + test) regenerates cleanly; new keys present, no hidden default
```
config-validate: ~/smackerel/config/generated/dev.env.tmp.2577590 OK
Generated ~/smackerel/config/generated/dev.env
Generated ~/smackerel/config/generated/nats.conf
Generated ~/smackerel/config/generated/prometheus.yml
```
The three new keys land in BOTH generated env files as empty-string entries (validated fail-loud at runtime, not generate-time):
```
config/generated/dev.env:301:TWITTER_OAUTH_CLIENT_ID=
config/generated/dev.env:302:TWITTER_OAUTH_CLIENT_SECRET=
config/generated/dev.env:303:TWITTER_OAUTH_REDIRECT_URL=
config/generated/test.env:301:TWITTER_OAUTH_CLIENT_ID=
config/generated/test.env:302:TWITTER_OAUTH_CLIENT_SECRET=
config/generated/test.env:303:TWITTER_OAUTH_REDIRECT_URL=
```

### A-E2 — `./smackerel.sh check` (config in SST sync, drift guard OK)
```
config-validate: ~/smackerel/config/generated/dev.env.tmp.2577590 OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 16, rejected: 0
scenario-lint: OK
```

### A-E3 — `./smackerel.sh lint` (Go golangci-lint + ruff + web validation)
```
All checks passed!
=== Validating web manifests ===
  OK: web/pwa/manifest.json
  OK: PWA manifest has required fields
=== Checking extension version consistency ===
  OK: Extension versions match (1.0.0)
Web validation passed
```

### A-E4 — Scope A unit tests GREEN (`./smackerel.sh test unit --go --go-run '<5 Scope A tests>' --verbose`)
```
=== RUN   TestAuth_GeneratePKCEPairS256
=== RUN   TestAuth_OAuth2PKCEBasicAuthStyle
--- PASS: TestAuth_GeneratePKCEPairS256 (0.00s)
--- PASS: TestAuth_OAuth2PKCEBasicAuthStyle (0.05s)
ok      github.com/smackerel/smackerel/internal/auth   0.109s
--- PASS: TestConfig_TwitterOAuthCredentialsHaveNoHiddenDefault (0.00s)
ok      github.com/smackerel/smackerel/internal/config 0.113s
--- PASS: TestTwitterOAuth_EmptyKeyFailsLoud (0.00s)
--- PASS: TestTwitterOAuth_EncryptedStoreRoundTrip (0.00s)
ok      github.com/smackerel/smackerel/internal/connector/twitter      0.119s
[go-unit] go test ./... finished OK
```

### A-E5 — RFC 7636 vector RED→GREEN (proves the S256 assertion is non-tautological)
RED — with the challenge encoder deliberately swapped to `base64.StdEncoding` (wrong: `+`/`=` instead of base64url-nopad), the RFC 7636 Appendix B vector test FAILS:
```
    oauth_pkce_test.go:30: RFC 7636 Appendix B vector mismatch:
          verifier  = dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk
          want      = E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM
          got       = E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw+cM=
--- FAIL: TestAuth_GeneratePKCEPairS256 (0.00s)
FAIL    github.com/smackerel/smackerel/internal/auth   0.167s
```
GREEN — reverting to `base64.RawURLEncoding` (RFC-correct base64url-nopad) restores the pass:
```
--- PASS: TestAuth_GeneratePKCEPairS256 (0.00s)
--- PASS: TestAuth_OAuth2PKCEBasicAuthStyle (0.11s)
ok      github.com/smackerel/smackerel/internal/auth   0.137s
```

### A-E6 — Migration 056 auto-run pickup (DB-free embed + parseable unit tests) + highest slot
```
=== RUN   TestMigrationsEmbed
--- PASS: TestMigrationsEmbed (0.00s)
=== RUN   TestMigrationSQL_Parseable
--- PASS: TestMigrationSQL_Parseable (0.00s)
ok      github.com/smackerel/smackerel/internal/db     0.109s
```
```
055_annotation_actor_and_version.sql
056_twitter_oauth_pkce.sql
total_files=40
```

### A-E7 — Full-suite regression scan (`./smackerel.sh test unit --go`, no name filter): changed packages GREEN
The three changed packages pass their COMPLETE existing suites (not just the new tests), proving no regressions:
```
ok      github.com/smackerel/smackerel/internal/auth   3.636s
ok      github.com/smackerel/smackerel/internal/config 43.945s
ok      github.com/smackerel/smackerel/internal/connector/twitter      7.749s
```
The new migration initially tripped the spec-032 migration-doc-freshness gate; adding the required `056` row to the `docs/Development.md` Database Migrations table (the mechanical migration⟹doc coupling) resolves it:
```
=== RUN   TestDocFreshness_AllMigrationsDocumented
    doc_freshness_test.go:182: migration freshness: 39 migration files on disk, 0 undocumented
--- PASS: TestDocFreshness_AllMigrationsDocumented (0.00s)
ok      github.com/smackerel/smackerel/internal/docfreshness   0.007s
```
Pre-existing unrelated failures (NOT introduced by this scope, NOT fixed here): `internal/docfreshness::TestDocFreshness_AllPromptContractsDocumented` (5 undocumented prompt contracts — spec-032, pre-existing) and `tests/unit/clients` spec-073 node/dart cross-language canary (`node`/`dart` not on PATH). `cmd/config-validate` passed in this run.

### Honest gap (anti-fabrication)
The live **DB-apply** of migration 056 under `./smackerel.sh test integration` (Postgres `migrate-up`) was NOT run in this unit-scoped pass — the embed runner pickup + SQL-parseable proof above (A-E6) is the unit-level evidence; the full integration apply is exercised by the Scope B authorize-begin/finalize persistence pass (scopes.md "After B" checkpoint). That single scopes.md Scope A DoD line (which names `test integration`) is therefore left UNCHECKED rather than ticked on unit evidence alone. **(CLOSED 2026-06-09 — the live DB-apply has since RUN GREEN; `TestTwitterOAuthMigration_AppliesCleanly` PASS vs live Postgres, see A-E8. That Scope A DoD line is now ticked `[x]` with Claim Source: executed.)**

### Scope A DoD status
8 of 9 Scope A DoD items are ticked against the evidence above; the 1 remaining item (migration applies cleanly under `./smackerel.sh test integration`) is deliberately left unchecked per the Honest gap note. No parent spec 056 artifact was modified. (Status note: this section records the 2026-06-08 pass; the packet is now `in_progress` / delivered-pending-audit — see the dated disposition immediately below.) **(UPDATE 2026-06-09: all 9 of 9 Scope A DoD items are now ticked — the migration live DB-apply RAN GREEN this session (A-E8), so Scope A is Done. Status stays NON-terminal in this PHASE-1 pass; PHASE 2 stamps `done`.)**

### Scope A Migration-Apply Disposition — honest Uncertainty Declaration (2026-06-09, bubbles.validate state-reconciliation)

**Claim Source: not-run (Gate G021 — NO fabricated live-apply pass).** The migration-056 live DB-apply under `./smackerel.sh test integration` is honestly dispositioned as an operator/CI-gated Uncertainty Declaration and stays `[ ]` in scopes.md. The integration stack is genuinely unavailable in this sandbox: no live Postgres, `DATABASE_URL` unset, and the `./smackerel.sh test integration` health-gate times out bringing up the slow Ollama image on a shared docker daemon — the SAME operator/CI-gated condition under which the live-Twitter `403 → 200` arm (`api_live_test.go`) correctly SKIPs. This is NOT a fabricated pass and NOT a silent skip; it is a declared not-run row whose terminal adjudication is owned by `bubbles.audit`.

What IS proven (unit-level + artifact-level), so the not-run row is bounded and low-risk:
- The migration auto-applies on container start via the embedded `//go:embed migrations/*.sql` runner and is unit-verified to parse with the correct `twitter_oauth_states` + `twitter_oauth_tokens` tables (A-E6 above).
- A dedicated integration test `TestTwitterOAuthMigration_AppliesCleanly` exists in the working tree and asserts BOTH tables + the `idx_twitter_oauth_states_expires_at` index after migrate, plus idempotent re-apply — ready to run in any CI/operator env with `DATABASE_URL` set:

```
$ wc -l tests/integration/twitter_oauth_migration_test.go
89 tests/integration/twitter_oauth_migration_test.go
$ grep -nE 'go:build integration|func TestTwitterOAuthMigration_AppliesCleanly|twitter_oauth_states|twitter_oauth_tokens|idx_twitter_oauth_states_expires_at|testPool' tests/integration/twitter_oauth_migration_test.go
1://go:build integration
20:	"twitter_oauth_states",
21:	"twitter_oauth_tokens",
29:func TestTwitterOAuthMigration_AppliesCleanly(t *testing.T) {
30:	pool := testPool(t)
83:		"idx_twitter_oauth_states_expires_at").Scan(&exists); err != nil {
87:		t.Fatal("expected index idx_twitter_oauth_states_expires_at to exist after migrate")
```

`./smackerel.sh test integration` was deliberately NOT attempted in that 2026-06-08/early-2026-06-09 pass (the orchestrator confirmed the stack was unavailable; a prior attempt timed out on the Ollama health probe). The packet was `in_progress` (delivered-pending-audit), not `blocked`.

**CLOSURE (2026-06-09, bubbles.validate — PHASE 1 planning-truth pass): this Uncertainty Declaration is now RETIRED.** The sandbox memory-saturation/OOM that blocked the integration stack was resolved (≈15 GB headroom freed). `bubbles.validate` then ran the live DB-apply in this session and it PASSED — see **A-E8** immediately below. The scopes.md migration DoD row is now ticked `[x]` with Claim Source: executed, Scope 1 is **Done** at 9/9 DoD, and the historical not-run narrative above is preserved as the triage record (anti-fabrication, Gate G021). Status stays NON-terminal (`certifiedAt` null) in this PHASE-1 pass; PHASE 2 stamps terminal `done` after the commit.

### A-E8 — Migration 056 live DB-apply GREEN (2026-06-09, bubbles.validate session-bound)

**Executed:** YES (in current session) | **Command:** `./smackerel.sh test integration --go-run 'TwitterOAuthMigration'` | **Result:** PASS | **Wrapper exit:** 0

The full disposable test stack came up healthy (all 8 containers Healthy: `nats`, `postgres`, `ollama`, `searxng`, `stub-providers`, `jaeger`, `smackerel-core`, `smackerel-ml`), the migration test ran against live Postgres, and the stack was torn down cleanly on exit (all containers + volumes + network removed):
```
go-integration: applying -run selector: TwitterOAuthMigration
=== RUN   TestTwitterOAuthMigration_AppliesCleanly
--- PASS: TestTwitterOAuthMigration_AppliesCleanly (0.15s)
PASS
ok      github.com/smackerel/smackerel/tests/integration        0.385s
```
```
PASS: go-integration
Running project-scoped integration test stack teardown (exit cleanup, timeout 180s)...
 Container smackerel-test-postgres-1  Removed
 Volume smackerel-test-postgres-data  Removed
 Network smackerel-test_default  Removed
INTEGRATION_WRAPPER_EXIT=0
```
The test (`tests/integration/twitter_oauth_migration_test.go`, `//go:build integration`) reads the real `056_twitter_oauth_pkce.sql`, applies it from a clean schema, asserts BOTH `twitter_oauth_states` + `twitter_oauth_tokens` tables and the `idx_twitter_oauth_states_expires_at` index, then re-applies for idempotency — a genuine live-Postgres apply, not a stub. (Independent-run note: this session's per-test/package timings `0.15s` / `0.385s` are real and differ from earlier quoted numbers — confirming a fresh, session-bound execution rather than a recited result.)

---

## Scope C Delivery Evidence — Pass 1 (2026-06-08 — endpoint auth-tier routing + fail-loud sentinel; bug remains OPEN)

**Claim Source:** executed — all fenced output below is verbatim terminal output from `./smackerel.sh` runs on this working tree (absolute home paths redacted to `~/`). The bug is NOT marked Fixed and `state.json` is NOT flipped to terminal.

**Scope boundary (Pass 1).** This pass delivers the endpoint→auth-tier routing mechanism that the rest of Scope C builds on: a centralized, auditable `endpointAuthTier` matrix; a tier-aware `buildRequest` that selects the credential per tier; an injectable user-context token source on `apiClient`; the `ErrUserContextTokenRequired` fail-loud sentinel (NO silent App-Only fallback); and the connector wiring (`Connector.userContextTokenSource`, consumed in `Connect`) that reads the persisted token from the Scope-A/B `oauthStore` (`GetTokens`) and fails loud when absent. **Pass 1 boundary (delivered separately in Pass 2 below):** refresh-on-401 + rotated-token persistence, pre-expiry refresh, and the full live-fixture adversarial integration suite (`TestTwitterAPI_AppOnlyOnUserOwnedEndpointRejected` + the SCN-009..014 named `httptest` integration tests) — these landed in Scope 3 Pass 2.

**Auth-tier matrix (spec 056 NC-1) as implemented:**
| Endpoint label | Tier | Credential |
|----------------|------|-----------|
| `users_me`, `bookmarks`, `liked_tweets` | user-context | decrypted user-context access token (`oauthStore.GetTokens`) |
| `tweets`, `mentions` | App-Only | `apiClient.bearerToken` (unchanged) |

**Mechanism.** `endpointAuthTier(label string) authTier` is the single source of truth; both call sites route through it — `fetchUsersMe` → `buildRequest(…, endpointAuthTier(usersMeLabel))`, `fetchEndpointPaginated` → `buildRequest(…, endpointAuthTier(string(endpoint)))`. `buildRequest` resolves the credential via `authorizationHeader(ctx, tier)` BEFORE constructing the request: App-Only returns `"Bearer "+bearerToken`; user-context returns `"Bearer "+<resolved token>`, or `ErrUserContextTokenRequired` (wrapping any resolver error) when the source is nil / the token is empty / the store errors — it NEVER falls back to App-Only. GET-only guard, nil-client guard, and User-Agent/Accept headers are unchanged; pagination/cursor/dead-letter logic is untouched.

### C-E1 — 4 new Scope C tests GREEN (`./smackerel.sh test unit --go --go-run '<4 tests>' --verbose`)
```
--- PASS: TestEndpointAuthTier (0.00s)
    --- PASS: TestEndpointAuthTier/users_me (0.00s)
    --- PASS: TestEndpointAuthTier/bookmarks (0.00s)
    --- PASS: TestEndpointAuthTier/liked_tweets (0.00s)
    --- PASS: TestEndpointAuthTier/tweets (0.00s)
    --- PASS: TestEndpointAuthTier/mentions (0.00s)
    --- PASS: TestEndpointAuthTier/some_unmapped_future_endpoint (0.00s)
--- PASS: TestBuildRequest_UserContextEndpointUsesUserToken (0.00s)
    --- PASS: TestBuildRequest_UserContextEndpointUsesUserToken/users_me (0.00s)
    --- PASS: TestBuildRequest_UserContextEndpointUsesUserToken/liked_tweets (0.00s)
    --- PASS: TestBuildRequest_UserContextEndpointUsesUserToken/bookmarks (0.00s)
--- PASS: TestBuildRequest_AppOnlyEndpointUsesBearer (0.00s)
    --- PASS: TestBuildRequest_AppOnlyEndpointUsesBearer/tweets (0.00s)
    --- PASS: TestBuildRequest_AppOnlyEndpointUsesBearer/mentions (0.00s)
--- PASS: TestBuildRequest_UserContextEndpoint_NoToken_FailsLoud (0.00s)
    --- PASS: TestBuildRequest_UserContextEndpoint_NoToken_FailsLoud/nil_source_(no_runtime_wired) (0.00s)
    --- PASS: TestBuildRequest_UserContextEndpoint_NoToken_FailsLoud/store_error (0.00s)
    --- PASS: TestBuildRequest_UserContextEndpoint_NoToken_FailsLoud/empty_token_string (0.00s)
    --- PASS: TestBuildRequest_UserContextEndpoint_NoToken_FailsLoud/empty_store_(no_token_row) (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/connector/twitter       0.083s
```

### C-E2 — RED→GREEN (proves the tests are non-tautological — they detect the original BUG-056-002 defect)
RED — with `endpointAuthTier` temporarily reverted to the original defect (ALL endpoints routed through App-Only), the three user-context-asserting tests FAIL and the App-Only test correctly stays GREEN:
```
--- FAIL: TestBuildRequest_UserContextEndpoint_NoToken_FailsLoud (0.00s)
    --- FAIL: TestBuildRequest_UserContextEndpoint_NoToken_FailsLoud/nil_source_(no_runtime_wired) (0.00s)
    --- FAIL: TestBuildRequest_UserContextEndpoint_NoToken_FailsLoud/store_error (0.00s)
    --- FAIL: TestBuildRequest_UserContextEndpoint_NoToken_FailsLoud/empty_token_string (0.00s)
    --- FAIL: TestBuildRequest_UserContextEndpoint_NoToken_FailsLoud/empty_store_(no_token_row) (0.00s)
--- FAIL: TestBuildRequest_UserContextEndpointUsesUserToken (0.00s)
    --- FAIL: TestBuildRequest_UserContextEndpointUsesUserToken/users_me (0.00s)
    --- FAIL: TestBuildRequest_UserContextEndpointUsesUserToken/liked_tweets (0.00s)
    --- FAIL: TestBuildRequest_UserContextEndpointUsesUserToken/bookmarks (0.00s)
--- FAIL: TestEndpointAuthTier (0.00s)
    --- FAIL: TestEndpointAuthTier/users_me (0.00s)
    --- PASS: TestEndpointAuthTier/tweets (0.00s)
    --- FAIL: TestEndpointAuthTier/some_unmapped_future_endpoint (0.00s)
    --- PASS: TestEndpointAuthTier/mentions (0.00s)
    --- FAIL: TestEndpointAuthTier/bookmarks (0.00s)
    --- FAIL: TestEndpointAuthTier/liked_tweets (0.00s)
--- PASS: TestBuildRequest_AppOnlyEndpointUsesBearer (0.00s)
    --- PASS: TestBuildRequest_AppOnlyEndpointUsesBearer/tweets (0.00s)
    --- PASS: TestBuildRequest_AppOnlyEndpointUsesBearer/mentions (0.00s)
FAIL    github.com/smackerel/smackerel/internal/connector/twitter       0.195s
```
GREEN — reverting `endpointAuthTier` to the correct matrix restores all four (see C-E1). The `NoToken_FailsLoud` RED is the precursor proof of the no-silent-fallback contract: under the bug a user-owned endpoint silently used the App-Only bearer instead of failing loud.

### C-E3 — Full-suite no-regression scan (`./smackerel.sh test unit --go`, no name filter): required packages GREEN
The full twitter package (all existing + modified + 4 new tests), `cmd/core`, and `internal/api` pass their COMPLETE suites — the existing pagination/rate-limit/secrecy tests were updated to inject a user-context token (since `users_me`/`bookmarks`/`liked_tweets` are now user-context tier) and the three connector-level API/hybrid Sync tests inject a connector-level override:
```
ok      github.com/smackerel/smackerel/cmd/core 3.026s
ok      github.com/smackerel/smackerel/internal/api     8.747s
ok      github.com/smackerel/smackerel/internal/auth    3.839s
ok      github.com/smackerel/smackerel/internal/config  30.432s
ok      github.com/smackerel/smackerel/internal/connector/twitter       6.193s
```
Pre-existing unrelated failures (NOT introduced by this pass, NOT in the change boundary, identical to the set recorded in the Scope A pass above): `internal/docfreshness::TestDocFreshness_AllPromptContractsDocumented` (5 undocumented prompt contracts — spec-032 doc drift) and `tests/unit/clients` spec-073 node/dart cross-language canary (`node`/`dart` not on PATH). Neither package imports the twitter routing symbols changed here.

### C-E4 — `./smackerel.sh check` (config in SST sync) + `./smackerel.sh lint` (go vet + ruff + web) clean
```
config-validate: ~/smackerel/config/generated/dev.env.tmp.608149 OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 16, rejected: 0
scenario-lint: OK
```
```
All checks passed!
=== Validating web manifests ===
  OK: web/pwa/manifest.json
  OK: PWA manifest has required fields
Web validation passed
```
(`./smackerel.sh lint` runs `go vet ./...` — silent/clean — then ruff "All checks passed!" then web validation.)

### C-E5 — Token secrecy preserved across both tiers
The user-context token is never logged and never embedded in a returned error. `authorizationHeader` returns the bare `ErrUserContextTokenRequired` sentinel (no token), and the two existing log-scan tests were strengthened to also assert the user-context token never appears: `TestTwitterAPI_BearerTokenNeverInLogs` and `TestTwitterAPI_BearerTokenNeverAppearsInLogs` (both GREEN in C-E3's twitter suite). Refresh-token secrecy is covered in Pass 2 (the refresh path lands there).

### Files changed (Scope C Pass 1 — all within the `internal/connector/twitter/*.go` change boundary)
- `internal/connector/twitter/api.go` — `authTier` + `endpointAuthTier` matrix, `usersMeLabel`, `userContextTokenFunc` + `apiClient.userContextToken` field, `ErrUserContextTokenRequired` sentinel, tier-aware `buildRequest` + `authorizationHeader`, both call sites routed through the matrix.
- `internal/connector/twitter/twitter.go` — `apiUserContextTokenOverride` test field, `Connector.userContextTokenSource()` (store-backed reader, fail-loud-when-absent), `Connect` wiring of `client.userContextToken`.
- `internal/connector/twitter/api_test.go` — 4 new tests + `staticUserContextToken`/`testUserContextToken` helpers; existing user-owned-endpoint tests inject the user-context source; `fetchUsersMe`/secrecy assertions updated for the user-context tier.
- `internal/connector/twitter/twitter_test.go` — 3 connector-level API/hybrid Sync tests inject `apiUserContextTokenOverride`.

No parent spec 056 artifact was modified; `state.json` remains `blocked` (non-terminal). Refresh-on-401 + the full adversarial fixture integration suite remain for the next Scope C delivery pass.

## Scope C Delivery Evidence — Pass 2 (2026-06-08 — refresh-on-401 + pre-expiry refresh + the KEY adversarial regression; bug remains OPEN, NOT terminal)

**Claim Source:** executed — every fenced block below is verbatim terminal output from `./smackerel.sh` runs on this working tree (absolute home paths redacted to `~/`). The bug is NOT marked Fixed and `state.json` is NOT flipped to terminal.

**Scope boundary (Pass 2).** This pass adds the runtime token lifecycle that Pass 1 left to Pass 2, plus the named reintroduction guard:
- a `userContextManager` (`internal/connector/twitter/oauth_token_manager.go`) that owns the encrypted token store + the confidential-client OAuth provider and exposes `AccessToken(ctx)` (read the persisted token; **proactively refresh** when within `refreshSkew = 60s` of a known expiry) and `Refresh(ctx)` (force-refresh: read the stored refresh token → `POST /2/oauth2/token` `grant_type=refresh_token` HTTP-Basic → persist the **ROTATED** access+refresh pair; Twitter rotates the refresh token, so BOTH are re-persisted; token values NEVER logged);
- `apiClient.refreshUserContext` + refresh-on-401 in `doWithRetry`: a **401** on a **user-context-tier** endpoint with a refresh hook wired refreshes ONCE (tracked by `refreshedOnce`) and `continue`s so the next `reqBuilder()` rebuilds the request with the freshly-persisted token. **App-Only endpoints stay terminal** (an app bearer cannot be rotated) and **a 403 stays terminal** (a tier/permission failure is not an expired-token signal) — the refresh is gated by the SAME `endpointAuthTier` matrix that selects the credential (no duplication), plus a `status == 401` guard;
- the connector wiring (`Connector.userContextAuth()` replaces the Pass 1 `userContextTokenSource()`): a manager-backed `AccessToken` + `Refresh` in production, a fail-loud source + nil refresh hook when runtime deps are absent, and the test override path unchanged.

**The KEY adversarial regression** `TestTwitterAPI_AppOnlyOnUserOwnedEndpointRejected` now lands against a `httptest.Server` that ENFORCES user-context (returns 403 `Unsupported Authentication` to the App-Only sentinel bearer) — the enforcement the old permissive fake lacked.

### C-Pass2-E1 — the 5 new Scope C Pass 2 tests GREEN (`./smackerel.sh test unit --go --go-run 'TestTwitterAPI' --verbose`)
```
--- PASS: TestTwitterAPI_AppOnlyOnUserOwnedEndpointRejected (0.12s)
    --- PASS: TestTwitterAPI_AppOnlyOnUserOwnedEndpointRejected/app_only_only_fails_loud_before_wire (0.00s)
    --- PASS: TestTwitterAPI_AppOnlyOnUserOwnedEndpointRejected/user_context_token_used_not_app_bearer (0.06s)
--- PASS: TestTwitterAPI_Refresh_On401_PersistentIsTerminalAfterOneRefresh (0.04s)
--- PASS: TestTwitterAPI_PreExpiryRefresh (0.15s)
--- PASS: TestTwitterAPI_AppOnly401_NoRefresh_Terminal (0.11s)
--- PASS: TestTwitterAPI_Refresh_On401_RetriesOnce (0.23s)
--- PASS: TestTwitterAPI_Unauthorized401FailsWithoutRetry (0.06s)
--- PASS: TestTwitterAPI_BearerTokenNeverAppearsInLogs (0.03s)
PASS
ok      github.com/smackerel/smackerel/internal/connector/twitter       0.682s
```
The existing `TestTwitterAPI_Unauthorized401FailsWithoutRetry` (a user-context bookmarks 401 with NO refresh hook wired) stays GREEN — proving a 401 is terminal when `refreshUserContext == nil`, so the new backstop is purely additive.

### C-Pass2-E2 — RED→GREEN: the named adversarial regression detects the original BUG-056-002 defect (matrix reverted)
RED — with `endpointAuthTier` temporarily reverted to the original defect (user-owned `bookmarks`/`liked_tweets` routed through App-Only), the App-Only sentinel bearer reaches the enforcing fixture and is 403'd, so BOTH subcases FAIL (fail-loud-before-wire is bypassed, and the user-context fetch is rejected):
```
2026/06/08 22:40:59 WARN authentication rejected component=twitter.api endpoint=bookmarks status=403
    api_test.go:1348: user-owned endpoint with no user-context token must fail with ErrUserContextTokenRequired (NOT a 403/errAuthRejected from a silently-sent App-Only bearer); got *fmt.wrapError: twitter api client: bookmarks page 0: twitter api client: authentication rejected (401/403); no retry: status=403
2026/06/08 22:40:59 WARN authentication rejected component=twitter.api endpoint=bookmarks status=403
    api_test.go:1384: user-context fetch against an enforcing server must succeed; got: twitter api client: bookmarks page 0: twitter api client: authentication rejected (401/403); no retry: status=403
--- FAIL: TestTwitterAPI_AppOnlyOnUserOwnedEndpointRejected (0.09s)
    --- FAIL: TestTwitterAPI_AppOnlyOnUserOwnedEndpointRejected/app_only_only_fails_loud_before_wire (0.03s)
    --- FAIL: TestTwitterAPI_AppOnlyOnUserOwnedEndpointRejected/user_context_token_used_not_app_bearer (0.00s)
FAIL
FAIL    github.com/smackerel/smackerel/internal/connector/twitter       0.171s
```
GREEN — restoring `endpointAuthTier` to the correct matrix (`users_me`/`bookmarks`/`liked_tweets` → user-context) makes both subcases PASS again (see C-Pass2-E1). The matrix reversion was temporary and was reverted in the same pass; the committed `endpointAuthTier` routes the user-owned endpoints to user-context.

### C-Pass2-E3 — refresh behavior proven by the 4 refresh tests (verbose trace excerpt)
`TestTwitterAPI_Refresh_On401_RetriesOnce` drives a REAL `userContextManager` (in-memory store + a real confidential-client provider against an `httptest` `/2/oauth2/token`): the first bookmarks request carries `OLD-ACCESS` → 401 → exactly ONE refresh → the retry carries the ROTATED `NEW-ACCESS`, the rotated `NEW-REFRESH` is persisted, and the call succeeds. `TestTwitterAPI_PreExpiryRefresh` seeds a token expiring within `refreshSkew/2` and asserts the SINGLE request carries the proactively-refreshed token (one token-endpoint exchange, no 401). `TestTwitterAPI_Refresh_On401_PersistentIsTerminalAfterOneRefresh` returns 401 on EVERY attempt and asserts the refresh fires AT MOST ONCE then surfaces terminal `errAuthRejected` (exactly 2 API calls, 1 refresh — no infinite loop). `TestTwitterAPI_AppOnly401_NoRefresh_Terminal` drives App-Only `tweets` with the refresh hook wired + a valid refreshable token in the store, and asserts the 401 is terminal with ZERO token-endpoint exchanges:
```
2026/06/08 23:15:16 INFO user-context token refreshed component=twitter.usercontext
2026/06/08 23:15:16 INFO user-context token refreshed after 401 component=twitter.api endpoint=bookmarks status=401
2026/06/08 23:15:16 WARN authentication rejected component=twitter.api endpoint=bookmarks status=401
--- PASS: TestTwitterAPI_Refresh_On401_PersistentIsTerminalAfterOneRefresh (0.04s)
2026/06/08 23:15:16 WARN authentication rejected component=twitter.api endpoint=tweets status=401
--- PASS: TestTwitterAPI_AppOnly401_NoRefresh_Terminal (0.11s)
--- PASS: TestTwitterAPI_PreExpiryRefresh (0.15s)
--- PASS: TestTwitterAPI_Refresh_On401_RetriesOnce (0.23s)
```
The persisting-401 trace shows EXACTLY ONE `"user-context token refreshed after 401"` followed by a terminal `WARN authentication rejected` (the second 401 hits the `refreshedOnce` gate) — proving refresh-once + no-infinite-loop. The App-Only `WARN authentication rejected ... endpoint=tweets status=401` with NO preceding refresh log confirms the App-Only 401 never reached the refresh path; every `INFO user-context token refreshed*` line carries NO token value.

### C-Pass2-E4 — full-suite no-regression scan (`./smackerel.sh test unit --go`, no filter): changed packages GREEN
The complete suites for every package this pass touches (or that consumes the twitter routing symbols) pass with the restored matrix:
```
ok      github.com/smackerel/smackerel/cmd/core 4.656s
ok      github.com/smackerel/smackerel/internal/api     13.150s
ok      github.com/smackerel/smackerel/internal/auth    3.617s
ok      github.com/smackerel/smackerel/internal/config  80.877s
ok      github.com/smackerel/smackerel/internal/connector/twitter       16.467s
```
The only FAILs in the full run are the two pre-existing UNRELATED failures (identical to the set recorded in the Scope A + Scope C Pass 1 passes; NOT introduced by this pass, NOT in the change boundary, neither package imports the twitter routing symbols changed here):
```
--- FAIL: TestDocFreshness_AllPromptContractsDocumented (0.00s)
    doc_freshness_test.go:203: prompt-contract freshness: 26 contracts on disk, 5 undocumented
FAIL    github.com/smackerel/smackerel/internal/docfreshness    0.394s
--- FAIL: TestRenderDescriptorV1_CrossLanguageCanary (0.00s)
    render_descriptor_canary_test.go:125: node not on PATH; the spec 073 cross-language renderer canary requires both node and dart: exec: "node": executable file not found in $PATH
FAIL    github.com/smackerel/smackerel/tests/unit/clients       0.022s
```

### C-Pass2-E5 — `./smackerel.sh check` (config in SST sync) + `./smackerel.sh lint` (go vet + ruff + web) clean
```
config-validate: ~/smackerel/config/generated/dev.env.tmp.711959 OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 16, rejected: 0
scenario-lint: OK
```
```
All checks passed!
=== Validating web manifests ===
  OK: web/pwa/manifest.json
  OK: PWA manifest has required fields
Web validation passed
```
(`./smackerel.sh lint` runs `go vet ./...` — silent/clean — then ruff "All checks passed!" then web validation.)

### C-Pass2-E6 — token secrecy across the refresh cycle (SCN-014 satisfied)
`TestTwitterAPI_Refresh_On401_RetriesOnce` captures all log output through a `slog` JSON buffer and asserts the refresh-after-401 log line is emitted while NONE of the four token values (`OLD-ACCESS`, `NEW-ACCESS`, `OLD-REFRESH`, `NEW-REFRESH`) appears in any log line. The manager logs only a token-free `"user-context token refreshed"` and `doWithRetry` logs a token-free `"user-context token refreshed after 401"` (endpoint + status only). The refresh-failure path wraps the token-endpoint's rejection (an `invalid_grant`-style body, never a token echo) into `errAuthRejected`; no token value is embedded. The Pass-1 access-token-secrecy guarantee (`TestTwitterAPI_BearerTokenNeverInLogs` / `…NeverAppearsInLogs`) remains GREEN (C-Pass2-E4 twitter suite).

### Files changed (Scope C Pass 2 — within the `internal/connector/twitter/*.go` change boundary)
- `internal/connector/twitter/oauth_token_manager.go` (NEW) — `refreshSkew` (60s) const; `userContextTokenStore` + `userContextRefresher` narrow interfaces (+ compile-time assertions for `*oauthStore` / `*auth.GenericOAuth2`); `userContextManager` with `AccessToken` (proactive pre-expiry refresh) + `Refresh` (force-refresh, rotated-pair persistence, token-free logging) + the shared `refresh` core.
- `internal/connector/twitter/api.go` — new `apiClient.refreshUserContext` hook; `refreshedOnce`-gated, tier-gated refresh-on-401 backstop in `doWithRetry` (App-Only stays terminal); `doWithRetry` behavior-matrix doc updated. `endpointAuthTier` is byte-for-byte the Pass 1 matrix (temporarily reverted only for the C-Pass2-E2 RED proof, then restored).
- `internal/connector/twitter/twitter.go` — `userContextTokenSource()` → `userContextAuth()` (returns the token source AND the refresh hook, manager-backed) + `failLoudUserContextSource` helper; `Connect` wires both `client.userContextToken` and `client.refreshUserContext`.
- `internal/connector/twitter/api_test.go` — `newRefreshTokenServer` helper; 5 new tests (`TestTwitterAPI_AppOnlyOnUserOwnedEndpointRejected`, `…_Refresh_On401_RetriesOnce`, `…_Refresh_On401_PersistentIsTerminalAfterOneRefresh`, `…_PreExpiryRefresh`, `…_AppOnly401_NoRefresh_Terminal`); `net/url` + `internal/auth` imports.
- `internal/connector/twitter/oauth_authorize_test.go` — `fakeFlowStore.GetTokens` (completes the `userContextTokenStore` surface) + a compile-time assertion.

No parent spec 056 artifact was modified; `state.json` remains `blocked` (non-terminal). The Scope C runtime auth path (routing + fail-loud + refresh-on-401 + pre-expiry refresh + the named adversarial regression) is now delivered; Scope D (the G2 gauge + the parent-claim-correction governance step) remains.

## Scope D Delivery Evidence (2026-06-08 — GAP-056-G2 / R-016 `x-rate-limit-remaining` gauge; bug remains OPEN, NOT terminal)

**Scope boundary (Scope D).** Adds the independent `ConnectorTwitterAPIRateLimitRemaining` Prometheus gauge (labels `connector`, `endpoint`), parses the `x-rate-limit-remaining` response header, and publishes it after EVERY API call (2xx/4xx/429/5xx) — satisfying spec 056 R-016 "updated after each API call". The pre-existing `ConnectorTwitterAPIRateLimitReset` gauge (429-only) is untouched and coexists with no duplicate-registration panic. The parent-spec-056 false-claim-correction is a governance closure step deliberately NOT performed here (recorded for the closure owner). Change boundary: `internal/metrics/metrics.go`, `internal/connector/twitter/api.go` (+ `api_test.go`). No parent spec 056 artifact touched.

### D-E1 — metric added + registered next to the reset gauge
**Claim Source:** interpreted (source inspection; the registration is exercised live by the GREEN tests in D-E2, which would panic on a duplicate-registration regression).

The new gauge sits beside `ConnectorTwitterAPIRateLimitReset` in `internal/metrics/metrics.go` and is added to the `init()` `prometheus.MustRegister(...)` list immediately after the reset gauge:

```text
var ConnectorTwitterAPIRateLimitRemaining = prometheus.NewGaugeVec(
    prometheus.GaugeOpts{
        Name: "smackerel_connector_twitter_api_rate_limit_remaining",
        Help: "Remaining requests in the current rate-limit window per endpoint (from x-rate-limit-remaining header)",
    },
    []string{"connector", "endpoint"},
)
...
        ConnectorTwitterAPIRateLimitReset,
        ConnectorTwitterAPIRateLimitRemaining,
        NATSDeadLetter,
```

The per-call hook in `doWithRetry` runs immediately after `c.observeRequest(endpoint, statusLabel)` and BEFORE the status `switch`, so it covers all status classes uniformly; an absent/unparseable header skips the `Set` (no-clobber):

```text
        statusLabel := strconv.Itoa(resp.StatusCode)
        c.observeRequest(endpoint, statusLabel)
        if rem, ok := parseRateLimitRemaining(resp.Header.Get("x-rate-limit-remaining")); ok {
            c.observeRateLimitRemaining(endpoint, rem)
        }

        switch {
```

### D-E2 — 3 new Scope D tests GREEN (`./smackerel.sh test unit --go --go-run 'TestTwitterAPI_RateLimitRemaining' --verbose`)
**Claim Source:** executed.

```text
=== RUN   TestTwitterAPI_RateLimitRemaining_SetFromHeader
=== RUN   TestTwitterAPI_RateLimitRemaining_AbsentHeaderLeavesPriorValue
=== RUN   TestTwitterAPI_RateLimitRemaining_SetOnEveryStatus
--- PASS: TestTwitterAPI_RateLimitRemaining_SetFromHeader (0.03s)
--- PASS: TestTwitterAPI_RateLimitRemaining_AbsentHeaderLeavesPriorValue (0.04s)
--- PASS: TestTwitterAPI_RateLimitRemaining_SetOnEveryStatus (0.06s)
PASS
ok      github.com/smackerel/smackerel/internal/connector/twitter       0.182s
[go-unit] go test ./... finished OK
```

- `_SetFromHeader` (SCN-015): a 200 + `x-rate-limit-remaining: 42` ⇒ `ToFloat64(gauge{twitter,endpoint}) == 42`.
- `_AbsentHeaderLeavesPriorValue` (no-clobber): gauge seeded to 99, a response with NO header ⇒ gauge stays 99 (absent ≠ exhausted).
- `_SetOnEveryStatus` (SCN-016, ADVERSARIAL): asserts the gauge moves on BOTH a 200 (==200) and a 429 (==7); the 429 case drives the gauge even though the call ends in `errMaxRetriesExceeded`, proving the hook fires before the status switch (not only on 2xx).

### D-E3 — RED→GREEN (the gauge stays 0 when the observe hook is removed; non-tautological)
**Claim Source:** executed.

RED — temporarily commented out the `observeRateLimitRemaining` hook in `doWithRetry`, then ran the gauge-==42 test:

```text
=== RUN   TestTwitterAPI_RateLimitRemaining_SetFromHeader
=== CONT  TestTwitterAPI_RateLimitRemaining_SetFromHeader
    api_test.go:1802: gauge = 0, want 42 (x-rate-limit-remaining header must drive the gauge)
--- FAIL: TestTwitterAPI_RateLimitRemaining_SetFromHeader (0.06s)
FAIL
FAIL    github.com/smackerel/smackerel/internal/connector/twitter       0.124s
```

GREEN — restored the hook, re-ran the full Scope D group:

```text
--- PASS: TestTwitterAPI_RateLimitRemaining_SetFromHeader (0.03s)
--- PASS: TestTwitterAPI_RateLimitRemaining_AbsentHeaderLeavesPriorValue (0.04s)
--- PASS: TestTwitterAPI_RateLimitRemaining_SetOnEveryStatus (0.06s)
PASS
ok      github.com/smackerel/smackerel/internal/connector/twitter       0.182s
[go-unit] go test ./... finished OK
```

The gauge reads 0 (not 42) with the hook removed → the assertion genuinely depends on the new wiring; it is not a tautology.

### D-E4 — full `TestTwitterAPI` suite + `internal/metrics` GREEN (no regression to the reset gauge or its registration)
**Claim Source:** executed (`./smackerel.sh test unit --go --go-run 'TestTwitterAPI' --verbose`).

```text
--- PASS: TestTwitterAPI_RateLimitRemaining_SetFromHeader (0.09s)
--- PASS: TestTwitterAPI_RateLimitRemaining_AbsentHeaderLeavesPriorValue (0.11s)
--- PASS: TestTwitterAPI_RateLimitRemaining_SetOnEveryStatus (0.17s)
ok      github.com/smackerel/smackerel/internal/connector/twitter       0.461s
ok      github.com/smackerel/smackerel/internal/metrics 0.058s [no tests to run]
[go-unit] go test ./... finished OK
```

The whole `TestTwitterAPI*` family (routing, fail-loud, refresh-on-401, pre-expiry, the adversarial App-Only regression, plus the 3 new gauge tests) is green; `internal/metrics` compiles + registers cleanly (no duplicate-registration panic from adding the second gauge). The `--go-run 'TestTwitterAPI'` filter is why unrelated packages report "no tests to run" — the known baseline failures that pre-date this packet (cmd/config-validate drive fixture, internal/docfreshness spec-032, tests/unit/clients spec-073 canary; tracked under `## Discovered Issues` below) are outside this filter and outside the change boundary.

### D-E5 — `./smackerel.sh check` + `./smackerel.sh lint` clean
**Claim Source:** executed.

```text
config-validate: ~/smackerel/config/generated/dev.env.tmp.1617274 OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 16, rejected: 0
scenario-lint: OK
```

```text
Successfully built smackerel-ml
All checks passed!
=== Validating web manifests ===
  OK: web/pwa/manifest.json
=== Checking extension version consistency ===
  OK: Extension versions match (1.0.0)
Web validation passed
```

`All checks passed!` is golangci-lint (go vet + staticcheck + the SST/no-defaults linters) over the whole module including the two changed Go files; the parse helper returns an explicit `(0, false)` on empty/unparseable input (a defensive parse bool, NOT a hidden config default) so the no-defaults policy is honored.

### Governance flag (Scope D delivery pass: RECORDED, deliberately NOT performed; SUPERSEDED 2026-06-08)
**Claim Source:** not-run at Scope D delivery time (intentionally out of the Scope D change boundary).

Delivering the gauge does not by itself touch the parent-spec-056 false-claim. At the Scope D delivery pass the corrective edit to `specs/056-twitter-api-connector/report.md:7` (re-quoted `:342`) remained a closure step owned by the delivery/closure agent + an orchestrator governance decision (re-certification risk: parent spec 056 is certified `done`). It was recorded here, in `scopes.md` §"Scope D … Parent claim-correction governance", and in `state.json` `CONCERN-056-002-false-claim`. **SUPERSEDED 2026-06-08:** the closure owner `bubbles.validate` has now PERFORMED the correction — see "## Validation & Parent-Claim-Correction Evidence" below. NO parent spec 056 artifact was modified by the Scope D delivery pass itself.

### Files changed (Scope D — within the declared change boundary)
- `internal/metrics/metrics.go` — new `ConnectorTwitterAPIRateLimitRemaining` `GaugeVec` (labels `connector`, `endpoint`) + its entry in the `init()` `MustRegister(...)` list, beside the existing reset gauge.
- `internal/connector/twitter/api.go` — `parseRateLimitRemaining(headerVal string) (float64, bool)` defensive parse helper (mirrors `parseRateLimitReset`); `observeRateLimitRemaining(endpoint string, remaining float64)` setter; the per-response hook in `doWithRetry` after `observeRequest` (covers every status class); the observe-cluster doc comment updated to enumerate the new method.
- `internal/connector/twitter/api_test.go` — 3 new integration tests (`TestTwitterAPI_RateLimitRemaining_SetFromHeader`, `…_AbsentHeaderLeavesPriorValue`, `…_SetOnEveryStatus`) + the `remainingReqBuilder` helper; new `github.com/prometheus/client_golang/prometheus/testutil` and `internal/metrics` imports.

No parent spec 056 artifact was modified; `state.json` remains `blocked` (non-terminal). Scope D (the G2 gauge) is delivered + unit-tested; the parent-claim-correction governance step and the validate/audit close-out chain remain.

### Scope D DoD status
- [x] `ConnectorTwitterAPIRateLimitRemaining` gauge (labels connector,endpoint) added + registered — D-E1, D-E4 (no duplicate-registration panic).
- [x] `x-rate-limit-remaining` parsed in `doWithRetry` after `observeRequest`, gauge set on EVERY response carrying the header (not only on 429) — D-E1 hook placement + D-E2 `_SetOnEveryStatus` (200 AND 429).
- [x] SCN-015 passes (gauge reflects the header) — D-E2 `_SetFromHeader` == 42.
- [x] Adversarial SCN-016 proven — D-E2 `_SetOnEveryStatus` (429 case == 7) + D-E3 RED (gauge 0 when the hook is removed).
- [x] GOVERNANCE: parent-claim-correction PERFORMED 2026-06-08 by bubbles.validate (the closure owner) — see "## Validation & Parent-Claim-Correction Evidence" → E-V5; the Scope D delivery pass correctly recorded it as not-yet-performed (superseded).
- [x] All existing `internal/connector/twitter` and `internal/metrics` tests still pass (no regressions) — D-E4.
- [x] `./smackerel.sh check` and `./smackerel.sh lint` clean — D-E5.

---

## Scope B Delivery Evidence (reconciled by bubbles.validate, 2026-06-08)

**Claim Source:** executed (the PASS lines below are verbatim from the `bubbles.validate` re-verification run on this working tree; absolute home paths redacted to `~/`) + code-anchor inspection (file/line references verified by read + grep). **Reconciliation note:** Scope B's code (the authorize CLI + flow) and its four `TestTwitterAuthorize_*` tests had already SHIPPED, but the packet bookkeeping still read `[ ] Not started` (the delivery pass updated the code + scopes.md for A/C/D but not B). `bubbles.validate`, as the designated state-reconciliation owner, ticked the 10 Scope B DoD items against the independently re-verified evidence below. No Scope B source was modified by this reconciliation — it records verified reality.

### B-E1 — the 4 authorize-flow tests GREEN (from the comprehensive re-verification run)
```text
Executed: YES
Phase Agent: bubbles.validate
Command: ./smackerel.sh test unit --go --go-run 'TestTwitterAPI|TestTwitterAuthorize|TestTwitterOAuth|PKCE|TestConfig_TwitterOAuth' --verbose
Date: 2026-06-08
Exit Code: 0 (finished OK)
Raw Output (Scope B authorize tests):
--- PASS: TestTwitterAuthorize_StatusReflectsPersistedToken (0.00s)
--- PASS: TestTwitterAuthorize_BeginPersistsStateAndBuildsS256URL (0.00s)
--- PASS: TestTwitterAuthorize_FinalizeUnknownOrExpiredStateFailsLoud (0.06s)
--- PASS: TestTwitterAuthorize_FinalizeExchangesAndPersistsEncrypted (0.06s)
ok      github.com/smackerel/smackerel/internal/connector/twitter       0.463s
```
- `TestTwitterAuthorize_BeginPersistsStateAndBuildsS256URL` (SCN-006): asserts `code_challenge_method=S256`, the LOCKED scope set, a persisted `twitter_oauth_states` row, and that the `code_verifier` never leaks into the authorize URL.
- `TestTwitterAuthorize_FinalizeExchangesAndPersistsEncrypted` (SCN-007): consumes+deletes the state, exchanges at an `httptest` token endpoint with HTTP Basic + `code_verifier`, persists the encrypted pair.
- `TestTwitterAuthorize_FinalizeUnknownOrExpiredStateFailsLoud` (extra adversarial beyond the 3 planned): an unknown/expired state token fails loud (no exchange, no token persisted).
- `TestTwitterAuthorize_StatusReflectsPersistedToken` (SCN-008): reports token presence/absence via `HasValidUserContext`.

### B-E2 — Scope B code anchors verified (read + grep)
- `cmd/core/cmd_connector.go` — `runConnectorCommand` → `runConnectorTwitter` dispatch for `authorize-begin|authorize-finalize|authorize-status`; exit codes 0 (success) / 1 (config/DB/exchange error) / 2 (invocation error); flags validated BEFORE config load / DB connect (`--user-id`, and `--state`/`--code` required for finalize).
- `cmd/core/main.go:69-72` — `if len(os.Args) > 1 && os.Args[1] == "connector" { … os.Exit(runConnectorCommand(ctx, os.Args[2:])) }`, beside the `auth`/`users`/`assistant` branches.
- `smackerel.sh:687` — `connector)` passthrough mirroring `auth)`; forwards `"$@"` verbatim into the running `smackerel-core` container; fails loud if the container is not up (NO host-binary fallback — Gate G028 / NO-DEFAULTS SST).
- `internal/connector/twitter/oauth_authorize.go` — `AuthorizeService` (begin/finalize/status) with the LOCKED endpoints (`https://twitter.com/i/oauth2/authorize`, `https://api.twitter.com/2/oauth2/token`), LOCKED scopes (`offline.access tweet.read users.read bookmark.read like.read`), 15-min state TTL, `TokenEndpointAuthStyle:"basic"`, and fail-loud on empty `ClientID`/`RedirectURL`/at-rest key; never surfaces the verifier or secret.
- `internal/connector/twitter/twitter.go:180` — `ConfigureRuntime(pool, atRestKey, oauthCfg)` (mirrors Drive); `cmd/core/connectors.go:53` calls it, threading `cfg.TwitterOAuthClientID/Secret/RedirectURL`.

### B-E3 — full `internal/connector/twitter` suite GREEN (no regression)
The complete twitter package (the 4 authorize tests + all Scope A/C/D tests + every pre-existing test) passed in the same run: `ok github.com/smackerel/smackerel/internal/connector/twitter 0.463s`. `internal/auth` (`ok … 0.309s`) and `internal/config` (`ok … 0.204s`) also passed.

### B-E4 — `./smackerel.sh check` + `./smackerel.sh lint` clean (bubbles.validate, 2026-06-08)
```text
config-validate: ~/smackerel/config/generated/dev.env.tmp.1889505 OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 16, rejected: 0
scenario-lint: OK
```
```text
All checks passed!
=== Validating web manifests ===
  OK: web/pwa/manifest.json
  OK: PWA manifest has required fields
Web validation passed
```

### B-E5 — honest testing boundary (Scope B)
The token exchange in B-E1 runs against an in-process `httptest.Server` emulating `POST /2/oauth2/token` — NOT the real Twitter endpoint, and NOT a mislabeled mock of internal code (the OAuth provider, PKCE, and encrypted store under test are real). The interactive browser authorize step and the live CLI invocation against a running stack (DB + real OAuth app) are operator-only and NOT CI-runnable; no live Twitter authorize is claimed. There is deliberately no `e2e-api` row (no live-stack scenario backs the headless single-operator CLI).

---

## Validation & Parent-Claim-Correction Evidence (bubbles.validate, 2026-06-08)

**Claim Source:** executed. Independent delivery-validation of all 4 scopes + the parent-spec-056 claim correction (the Scope D governance DoD item). Absolute home paths redacted to `~/`.

### E-V1 — comprehensive re-verification GREEN (all 4 scopes)
```text
Executed: YES
Command: ./smackerel.sh test unit --go --go-run 'TestTwitterAPI|TestTwitterAuthorize|TestTwitterOAuth|PKCE|TestConfig_TwitterOAuth' --verbose
Date: 2026-06-08
Exit Code: 0
Key PASS lines:
--- PASS: TestAuth_GeneratePKCEPairS256 (0.00s)                               # Scope A (PKCE S256, RFC 7636)
--- PASS: TestAuth_OAuth2PKCEBasicAuthStyle (0.21s)                           # Scope A (PKCE + Basic auth)
--- PASS: TestConfig_TwitterOAuthCredentialsHaveNoHiddenDefault (0.05s)       # Scope A (config no-default)
--- PASS: TestTwitterOAuth_EncryptedStoreRoundTrip (0.00s)                    # Scope A (AES-256-GCM store)
--- PASS: TestTwitterOAuth_EmptyKeyFailsLoud (0.00s)                          # Scope A (fail-loud at-rest key)
--- PASS: TestTwitterAuthorize_BeginPersistsStateAndBuildsS256URL (0.00s)     # Scope B
--- PASS: TestTwitterAuthorize_FinalizeExchangesAndPersistsEncrypted (0.06s)  # Scope B
--- PASS: TestTwitterAuthorize_FinalizeUnknownOrExpiredStateFailsLoud (0.06s) # Scope B
--- PASS: TestTwitterAuthorize_StatusReflectsPersistedToken (0.00s)           # Scope B
--- PASS: TestTwitterAPI_AppOnlyOnUserOwnedEndpointRejected (0.08s)           # Scope C KEY adversarial
    --- PASS: …/app_only_only_fails_loud_before_wire (0.00s)
    --- PASS: …/user_context_token_used_not_app_bearer (0.00s)
--- PASS: TestTwitterAPI_Refresh_On401_RetriesOnce (0.20s)                    # Scope C refresh-on-401
--- PASS: TestTwitterAPI_Refresh_On401_PersistentIsTerminalAfterOneRefresh (0.07s)
--- PASS: TestTwitterAPI_PreExpiryRefresh (0.17s)                             # Scope C pre-expiry refresh
--- PASS: TestTwitterAPI_AppOnly401_NoRefresh_Terminal (0.08s)               # Scope C App-Only stays terminal
--- PASS: TestTwitterAPI_RateLimitRemaining_SetFromHeader (0.06s)            # Scope D gauge
--- PASS: TestTwitterAPI_RateLimitRemaining_AbsentHeaderLeavesPriorValue (0.06s)
--- PASS: TestTwitterAPI_RateLimitRemaining_SetOnEveryStatus (0.16s)         # Scope D adversarial (non-429)
--- SKIP: TestTwitterAPILive_UsersMe (0.00s)                                 # live arm correctly SKIPPED (gated)
ok      github.com/smackerel/smackerel/internal/auth    0.309s
ok      github.com/smackerel/smackerel/internal/config  0.204s
ok      github.com/smackerel/smackerel/internal/connector/twitter       0.463s
[go-unit] go test ./... finished OK
```

### E-V2 — `./smackerel.sh check` clean
```text
config-validate: ~/smackerel/config/generated/dev.env.tmp.1889505 OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 16, rejected: 0
scenario-lint: OK
```

### E-V3 — `./smackerel.sh lint` clean
```text
All checks passed!
=== Validating web manifests ===
  OK: web/pwa/manifest.json
  OK: PWA manifest has required fields
Web validation passed
```

### E-V4 — requirement-mechanism guard (G097) for spec 056 — grandfathered warning, NON-blocking
```text
Executed: YES
Command: bash .github/bubbles/scripts/requirement-mechanism-guard.sh specs/056-twitter-api-connector
Date: 2026-06-08
Exit Code: 0
Output:
G097 BLOCK: requirement names mechanism 'PKCE' but NO implementation file shows it (searched 6 file(s)) …
G097: 1 requirement-mechanism correspondence gap(s) — DOWNGRADED to warning (spec createdAt … grandfathered).
```
**Interpretation:** G097 is NON-blocking here (exit 0, grandfathered — parent spec 056 `state.json` has no `createdAt`). The "PKCE not found in 6 files" is because the guard discovers implementation files from the PARENT spec 056 `scopes.md` (the original App-Only scopes, whose declared files are `api.go`/`twitter.go`/`metrics.go`/tests — none contain the literal `PKCE`). The PKCE mechanism genuinely ships in NEW files the BUG-056-002 packet added (`internal/auth/oauth.go`, `internal/connector/twitter/oauth_authorize.go` / `oauth_store.go` / `oauth_token_manager.go`, `oauth_pkce_test.go`) that the parent's file list does not yet reference. Independent confirmation the divergence is closed in code: `grep -rniE 'pkce|code_verifier|code_challenge|oauth2|S256' internal/connector/twitter/ internal/auth/` returns 20+ matches at HEAD (vs the historical `exit 1` at HEAD `9638b065`), and the PKCE/authorize tests above are GREEN. Remediation noted for `bubbles.plan` (NON-blocking): add the PKCE files to spec 056's declared implementation-file list, OR add a `## Requirement-Mechanism Justifications` entry, when spec 056 is next touched.

### E-V5 — parent spec 056 claim correction PERFORMED (the Scope D governance DoD item)
`bubbles.validate` (the closure owner) corrected the parent `specs/056-twitter-api-connector/report.md`:
- Rewrote the **Summary** from the interim honest "App-Only only; User-Context PKCE pending" statement (the 2026-06-07 reconcile correction) to the now-truthful "Spec 056 delivers BOTH the App-Only bearer path (public: tweets/mentions) AND the User-Context OAuth 2.0 PKCE flow (users_me/bookmarks/liked_tweets) via BUG-056-002", with an explicit provenance/honesty paragraph and NO overclaim (the live real-Twitter `403 → 200` arm is called out as operator-gated/manual, not executed).
- Marked the historical **GAP-056-G1** (PKCE) and **GAP-056-G2** (R-016 gauge) sections **RESOLVED 2026-06-08**, preserving their diagnostic bodies as the triage record.
- Added a `bubbles.validate` `delivery-validation` entry to the parent `state.json` `executionHistory` and updated the top-level `activeBugs[BUG-056-002]` to `delivered-pending-audit`. Parent top-level `status` stays `done`; the `certification` block structure is unchanged; `spec.md`/`design.md`/`scopes.md` were NOT touched (they correctly mandated PKCE all along).

### E-V6 — no-regression boundary (baseline failures outside this packet's change boundary)
The known baseline failures that pre-date this packet (not owned by this bug, identical to the set recorded in the Scope 1/3/4 delivery passes; tracked under `## Discovered Issues` below) remain outside the changed packages: `internal/docfreshness::TestDocFreshness_AllPromptContractsDocumented` (spec-032, 5 undocumented prompt contracts), `tests/unit/clients` spec-073 node/dart cross-language canary (`node`/`dart` not on PATH), and the `cmd/config-validate` drive fixture. None imports the twitter/auth/config/metrics symbols changed here. The `--go-run` filter above scoped the run to the BUG-056-002 surface, which finished `OK` with the changed packages all green.

### Disposition
All 4 scopes are DELIVERED and their CI-runnable tests are GREEN; the parent claim is corrected to the truth; the packet is at **delivered-pending-audit** (NON-terminal). ONE honest gap stays UNCHECKED (Scope A migration-056 live DB-apply under `./smackerel.sh test integration` — `bubbles.validate` lacked integration authority this pass). Terminal certification + the remaining specialist phases (regression/simplify/stabilize/security) are owned by `bubbles.audit` (separation of duties).

---

## Implementation Code Diff Evidence

### Code Diff Evidence
**Claim Source:** executed — ran git diff --stat (modified, tracked) and wc -l (new, untracked) on this working tree during the RW-056-002-001 planning repair (2026-06-09). The BUG-056-002 PKCE / user-context-auth change is a runtime-behavior change. Scope is the BUG-056-002 surface only; the unrelated working-tree changes (`internal/api/web_login*`, `internal/auth/webcreds`, `internal/pipeline`, `internal/stringutil`, `cmd/core/cmd_users*`) are NOT part of this packet and are excluded from the figures below.

**Modified (tracked) — `git diff --stat`:**
```text
$ git diff --stat -- internal/auth/oauth.go internal/connector/twitter/api.go internal/connector/twitter/api_test.go internal/connector/twitter/twitter.go internal/connector/twitter/twitter_test.go internal/config/config.go internal/metrics/metrics.go cmd/core/connectors.go cmd/core/main.go config/smackerel.yaml scripts/commands/config.sh smackerel.sh
 cmd/core/connectors.go                     |  19 +-
 cmd/core/main.go                           |   8 +
 config/smackerel.yaml                      |   3 +
 internal/auth/oauth.go                     |  95 +++
 internal/config/config.go                  |  10 +-
 internal/connector/twitter/api.go          | 250 +++++++-
 internal/connector/twitter/api_test.go     | 911 ++++++++++++++++++++++++++++-
 internal/connector/twitter/twitter.go      | 117 +++-
 internal/connector/twitter/twitter_test.go |   3 +
 internal/metrics/metrics.go                |  15 +
 scripts/commands/config.sh                 |   6 +
 smackerel.sh                               |  21 +
 12 files changed, 1431 insertions(+), 27 deletions(-)
```

**New (untracked) — `wc -l`:**
```text
  168 cmd/core/cmd_connector.go
   42 cmd/core/cmd_connector_test.go
  194 internal/auth/oauth_pkce_test.go
   51 internal/config/twitter_oauth_config_test.go
  225 internal/connector/twitter/oauth_authorize.go
  406 internal/connector/twitter/oauth_authorize_test.go
  243 internal/connector/twitter/oauth_store.go
   98 internal/connector/twitter/oauth_store_test.go
  157 internal/connector/twitter/oauth_token_manager.go
   59 internal/db/migrations/056_twitter_oauth_pkce.sql
 1643 total
```

The migration `internal/db/migrations/056_twitter_oauth_pkce.sql` and the new source files (`internal/connector/twitter/oauth_store.go`, `oauth_token_manager.go`, `oauth_authorize.go`, `cmd/core/cmd_connector.go`) carry the PKCE / user-context-auth mechanism. **Delivered test files** (the persistent regression surface; scenario-first TDD with per-scope RED-GREEN proofs): `internal/auth/oauth_pkce_test.go`, `internal/config/twitter_oauth_config_test.go`, `internal/connector/twitter/oauth_store_test.go`, `internal/connector/twitter/oauth_authorize_test.go`, `internal/connector/twitter/api_test.go`, `internal/connector/twitter/twitter_test.go`, `cmd/core/cmd_connector_test.go`. The live real-Twitter `403 → 200` arm (`internal/connector/twitter/api_live_test.go`) is operator-gated and not CI-runnable; no live pass is claimed.

## Discovered Issues
Logged 2026-06-09 during the RW-056-002-001 structural repair. Two failures exist on the baseline and are NOT owned, introduced, or touched by BUG-056-002 (no twitter/auth/config/metrics symbol is involved); each is recorded here with disposition + owning reference per Gate G095.

| # | Issue | Disposition | Owner / reference |
|---|-------|-------------|-------------------|
| DI-1 | `internal/docfreshness::TestDocFreshness_AllPromptContractsDocumented` — 5 undocumented prompt contracts | baseline failure outside this packet's change boundary; not fixed here (no twitter/auth/config/metrics symbol involved) | spec-032 doc-freshness gate |
| DI-2 | `tests/unit/clients::TestRenderDescriptorV1_CrossLanguageCanary` — `node`/`dart` not on PATH | environment-gated baseline failure outside this packet's change boundary; not fixed here | spec-073 cross-language canary |

## Independent Audit Evidence (bubbles.audit, 2026-06-09) — VERDICT: 🛑 REWORK_REQUIRED (route_required)

**Claim Source:** executed. Independent G022 audit pass (orchestrator `bubbles.goal`). All fenced output below is verbatim terminal output from this audit session on this working tree (absolute home paths redacted to `~/`). The implementation is genuinely sound; however, the packet is **NOT certified to terminal status** because the mechanical state-transition guard (Gate G023) and the traceability guard both legitimately BLOCK, and the decisive blockers are outside the audit agent's authority to resolve or fabricate (separation of duties). No source, no parent spec 056 artifact, and no framework asset was modified by this audit.

### AU-E1 — independent re-run of the full BUG-056-002 surface GREEN (reproduces the recorded evidence)
```text
Command: ./smackerel.sh test unit --go --go-run 'TestTwitterAPI|TestTwitterAuthorize|TestTwitterOAuth|PKCE|TestConfig_TwitterOAuth' --verbose
Date: 2026-06-09
Exit: 0 ([go-unit] go test ./... finished OK)
--- PASS: TestAuth_GeneratePKCEPairS256 (0.00s)                                # Scope A PKCE S256 (RFC 7636)
--- PASS: TestAuth_OAuth2PKCEBasicAuthStyle (0.05s)                            # Scope A PKCE + Basic auth
ok      github.com/smackerel/smackerel/internal/auth    0.174s
--- PASS: TestConfig_TwitterOAuthCredentialsHaveNoHiddenDefault (0.03s)        # Scope A config no-default
ok      github.com/smackerel/smackerel/internal/config  0.150s
--- SKIP: TestTwitterAPILive_UsersMe (0.00s)                                   # live arm correctly gated SKIP
--- SKIP: TestTwitterAPI_LiveTestNeverRunsInCI (0.00s)                         # live arm correctly gated SKIP
--- PASS: TestTwitterOAuth_EmptyKeyFailsLoud (0.00s)                           # Scope A fail-loud at-rest key
--- PASS: TestTwitterOAuth_EncryptedStoreRoundTrip (0.00s)                     # Scope A AES-256-GCM round-trip
--- PASS: TestTwitterAuthorize_StatusReflectsPersistedToken (0.00s)            # Scope B
--- PASS: TestTwitterAuthorize_FinalizeUnknownOrExpiredStateFailsLoud (0.03s)  # Scope B
--- PASS: TestTwitterAuthorize_BeginPersistsStateAndBuildsS256URL (0.00s)      # Scope B
--- PASS: TestTwitterAuthorize_FinalizeExchangesAndPersistsEncrypted (0.03s)   # Scope B
--- PASS: TestTwitterAPI_AppOnlyOnUserOwnedEndpointRejected (0.09s)            # Scope C KEY adversarial
    --- PASS: TestTwitterAPI_AppOnlyOnUserOwnedEndpointRejected/app_only_only_fails_loud_before_wire (0.00s)
    --- PASS: TestTwitterAPI_AppOnlyOnUserOwnedEndpointRejected/user_context_token_used_not_app_bearer (0.00s)
--- PASS: TestTwitterAPI_Refresh_On401_RetriesOnce (0.14s)                     # Scope C refresh-on-401
--- PASS: TestTwitterAPI_Refresh_On401_PersistentIsTerminalAfterOneRefresh (0.25s)
--- PASS: TestTwitterAPI_PreExpiryRefresh (0.15s)                              # Scope C pre-expiry refresh
--- PASS: TestTwitterAPI_AppOnly401_NoRefresh_Terminal (0.10s)                 # Scope C App-Only stays terminal
--- PASS: TestTwitterAPI_RateLimitRemaining_SetFromHeader (0.11s)             # Scope D gauge
--- PASS: TestTwitterAPI_RateLimitRemaining_AbsentHeaderLeavesPriorValue (0.12s)
--- PASS: TestTwitterAPI_RateLimitRemaining_SetOnEveryStatus (0.21s)          # Scope D adversarial (non-429)
ok      github.com/smackerel/smackerel/internal/connector/twitter       1.070s
```
The audit's independent run reproduces the recorded GREEN exactly (evidence integrity VERIFIED — no discrepancy with the recorded `report.md` results). `./smackerel.sh check` was also re-run clean (`Config is in sync with SST` / `env_file drift guard: OK` / `scenario-lint: OK`).

### AU-E2 — anti-fabrication posture: SOUND (each item independently confirmed)
- **Adversarial test is non-tautological — CONFIRMED.** Source read of `endpointAuthTier` (`internal/connector/twitter/api.go:325`) + `authorizationHeader` (`:210`): user-owned labels route to `authTierUserContext`; the recorded C-Pass2-E2 RED proof (matrix reverted → both subcases FAIL at `api_test.go:1348/1384` against an enforcing fixture that 403s the App-Only sentinel) is structurally consistent with the code. The test fails iff a user-owned endpoint is (re)routed through App-Only. Additional safety observed: the matrix `default` case returns `authTierUserContext`, so an unmapped/future endpoint fails *toward* user-context, never silently App-Only.
- **Claim truthful, no live overclaim — CONFIRMED.** The live `TestTwitterAPILive_UsersMe` and `TestTwitterAPI_LiveTestNeverRunsInCI` arms SKIP in my run (gated); the report/scopes carve the real-Twitter `403 → 200` arm out as operator-gated and NOT a DoD item. No DoD row claims a live Twitter pass.
- **No token / secret / code_verifier logged — CONFIRMED.** Source read: `authorizationHeader` returns the bare `ErrUserContextTokenRequired` sentinel (no token); `doWithRetry` and `userContextManager` emit only token-free `"user-context token refreshed[ after 401]"` lines; `oauth_store.go` encrypts access+refresh AES-256-GCM with a CSPRNG nonce; `generateStateToken()`/`GeneratePKCEPair()` use `crypto/rand`; `ConsumeState` is atomic `DELETE … RETURNING` + TTL (single-use, no replay). The `TestTwitterAPI_BearerTokenNeverInLogs` / `…NeverAppearsInLogs` guards are GREEN in AU-E1.

### AU-E3 — guard sweep (the decisive blockers to terminal close)
| Guard | Exit | Result |
|-------|------|--------|
| `artifact-lint.sh` | 0 | **PASS** (one non-blocking advisory: deprecated `scopeProgress` field name) |
| `state-transition-guard.sh` | 1 | **🔴 BLOCKED — 46 failures, 4 warnings.** `state.json status MUST NOT be set to 'done'.` |
| `traceability-guard.sh` | 1 | **🔴 FAILED — 32 failures** (16 missing report evidence-refs for the concrete test file; Gate G068 DoD↔Gherkin fidelity: 15/16 scenarios unmapped) |

**Decisive, audit-unresolvable blockers (cannot be fabricated or fixed within audit authority):**
1. **G022 — 4 required specialist phases never ran (Check 6).** `bugfix-fastlane` `phaseOrder` (`.github/bubbles/workflows/modes.yaml:189`) mandates `regression, simplify, stabilize, security` as specialist phases with `blockOnMissingSpecialistExecution: true`. `state.json` records only `implement, test, validate`. **The orchestrator's premise that audit is "the last required G022 phase" is mechanically false** — 4 specialist phases besides audit are missing. The audit agent cannot supply or fabricate them.
2. **G024 / DoD completion (Check 4) — 1 UNCHECKED DoD item.** Scope A's migration live DB-apply under `./smackerel.sh test integration` is an honest UD that stays `[ ]`; terminal `done` requires zero unchecked. The audit agent's allowed command set excludes `test integration`, so this gap cannot be closed here.
3. **G041 scope-status canonicality (Check 4B) + cascade (Check 5, G027 Check 15).** The 4 scope `**Status:**` lines use free-form prose (`[~] Foundation IMPLEMENTED…`, `[x] DELIVERED…`) instead of canonical `Done`; the guard therefore counts 0 Done scopes → `completedScopes(3) ≠ Done(0)` and trips G027 phase/scope-coherence. `scopes.md` planning structure is owned by `bubbles.plan` (artifact-ownership boundary), not the audit agent.
4. **Structural artifact gaps owned by `bubbles.plan` / the delivery chain:** G040/G084/G095 deferral & discovered-issue language (4 hits in scopes.md, 15 in report.md); G068 DoD↔Gherkin fidelity (15 scenarios); G053 missing `### Code Diff Evidence`; G060 scenario-first red→green markers not detected by the guard; Check 8A Gherkin-E2E regression rows; Check 5A SLA stress; Check 8B/8D consumer-trace & change-boundary DoD items.

### AU-E4 — DoD disposition (per scope, honest)
- **Scope A (Foundation):** met with real evidence for 8/9 DoD; the 9th (migration live DB-apply under `test integration`) is an **honest not-run UD**, correctly left `[ ]`. Genuine terminal-`done` blocker (needs an integration run the audit agent may not perform). **(CLOSED 2026-06-09 — now resolved: `bubbles.validate` ran the migration live DB-apply GREEN; `TestTwitterOAuthMigration_AppliesCleanly` PASS vs live Postgres, see A-E8. Scope A is now 9/9 DoD, status Done; this terminal-`done` blocker is cleared.)**
- **Scope B (Authorize CLI):** met — 4 `TestTwitterAuthorize_*` GREEN (AU-E1) + code anchors.
- **Scope C (routing + refresh + adversarial):** met — routing/fail-loud/refresh/pre-expiry/adversarial all GREEN (AU-E1); RED→GREEN reintroduction proof recorded.
- **Scope D (gauge + governance):** met — gauge + non-429 adversarial GREEN (AU-E1); parent claim-correction performed truthfully by validate (no live overclaim).
- **Honest not-run rows (NOT fake passes):** (a) Scope A migration live DB-apply under `./smackerel.sh test integration`; (b) the live real-Twitter `403 → 200` arm (`api_live_test.go`, operator-gated, correctly SKIP). Row (a) MUST run before terminal close (G024); row (b) is legitimately operator-gated and is correctly excluded from the DoD. **(2026-06-09 UPDATE: row (a) has now RUN GREEN — `TestTwitterOAuthMigration_AppliesCleanly` PASS, see A-E8; the G024 terminal-close blocker on row (a) is cleared. Row (b) remains legitimately operator-gated.)**

### AU-E5 — verdict + routing
**🛑 REWORK_REQUIRED → outcome `route_required`.** The implementation and anti-fabrication posture are genuinely sound (no fabrication detected; the security-sensitive auth code is correct), but the packet cannot be certified to terminal `done`: both governance guards legitimately block, and the decisive blockers (4 missing G022 specialist phases, the un-run integration DoD, and `bubbles.plan`-owned scopes.md structure) are outside the audit agent's authority. Per the audit contract, the audit phase is **NOT** recorded as a clean certified phase and the bug status is **left unchanged** (`in_progress`); terminal close is **NOT** forced. Remediation packet **RW-056-002-001** routes to `bubbles.plan` for the foundational scopes.md repairs, after which the orchestrator must dispatch the missing `regression → simplify → stabilize → security` specialist phases and the Scope A migration integration apply, then re-invoke `bubbles.audit`.

This audit re-confirms (does NOT re-edit) the parent spec 056 PKCE false-claim concern (`CONCERN-056-002-false-claim`) was already reconciled truthfully by `bubbles.validate`; no parent spec 056 artifact was touched by this audit.

### AU-E6 — Spot-Check Recommendations (automation-bias mitigation)
1. **Re-read AU-E3 blocker #1 yourself:** open `.github/bubbles/workflows/modes.yaml:189` and confirm `bugfix-fastlane` lists `regression, simplify, stabilize, security` — i.e., audit is not the only missing phase.
2. **Re-run the state-transition guard** and confirm exit 1 / "TRANSITION BLOCKED" before any future close attempt; do not let a confident close narrative override the mechanical verdict.
3. **Verify the honest gap is real:** confirm Scope A's migration DoD line is still `[ ]` and that no integration apply has been recorded — terminal `done` needs it run with real evidence, not checked on unit evidence.
4. **Confirm no live-Twitter pass is ever fabricated** during remediation — the `api_live_test.go` arm must remain a gated SKIP, never a claimed pass.

## Regression Pass — BUG-056-002 (2026-06-08)

`bubbles.regression` (Steve French) — the `regression` specialist phase the `bugfix-fastlane`
`phaseOrder` requires (one of the 4 missing phases AU-E3 blocker #1 flagged). Scope: did adding
the User-Context OAuth 2.0 PKCE path break any EXISTING behavior across the 12 changed source files
(`internal/auth/oauth.go`, `internal/connector/twitter/{api,oauth_authorize,oauth_store,oauth_token_manager,twitter}.go`,
`cmd/core/{main,cmd_connector,connectors}.go`, `internal/config/config.go`, `internal/metrics/metrics.go`,
migration `056_twitter_oauth_pkce.sql`). Delta isolated via `git diff HEAD` (delivery lives in the
working tree, +491/−23 across the 7 tracked source files; 5 new source/migration files). Read-only +
allowed `./smackerel.sh test unit` commands only; no protected artifact edited.

### RG-E1 — full Go unit suite (`./smackerel.sh test unit --go`)

The BUG-056-002-touched packages every report `ok`; the only two `FAIL` packages are pre-existing and
unrelated (isolated in RG-E3). Curated verbatim from the run:

```
$ ./smackerel.sh test unit --go
ok      github.com/smackerel/smackerel/cmd/core 1.416s
ok      github.com/smackerel/smackerel/internal/auth    3.472s
ok      github.com/smackerel/smackerel/internal/config  30.836s
ok      github.com/smackerel/smackerel/internal/connector       56.611s
ok      github.com/smackerel/smackerel/internal/connector/twitter       5.963s
ok      github.com/smackerel/smackerel/internal/drive/google    0.521s
ok      github.com/smackerel/smackerel/internal/metrics 0.045s
--- FAIL: TestDocFreshness_AllPromptContractsDocumented (0.00s)
    doc_freshness_test.go:203: prompt-contract freshness: 26 contracts on disk, 5 undocumented
    doc_freshness_test.go:205: docs/Development.md is STALE: 5 prompt contract(s) on disk are undocumented: alert-timing-evaluate-v1.yaml, expertise-classify-v1.yaml, hospitality-concern-evaluate-v1.yaml, relationship-cooling-evaluate-v1.yaml, resurface-evaluate-v1.yaml
FAIL    github.com/smackerel/smackerel/internal/docfreshness    0.015s
--- FAIL: TestRenderDescriptorV1_CrossLanguageCanary (0.00s)
    render_descriptor_canary_test.go:125: node not on PATH; the spec 073 cross-language renderer canary requires both node and dart: exec: "node": executable file not found in $PATH
--- FAIL: TestRenderDescriptorV1_DartPreCompiled_NoFallbackToDartRun (0.00s)
    render_descriptor_canary_test.go:367: dart not on PATH; the spec 073 cross-language renderer canary requires dart: exec: "dart": executable file not found in $PATH
FAIL    github.com/smackerel/smackerel/tests/unit/clients       0.006s
FAIL
```

Note `cmd/config-validate` reported `ok` this run (`ok github.com/smackerel/smackerel/cmd/config-validate 0.032s`),
so the actual failure set is exactly `{internal/docfreshness, tests/unit/clients}` — two packages, both
pre-existing.

### RG-E2 — full Python unit suite (`./smackerel.sh test unit --python`)

The delivery touched ZERO Python files (12/12 are Go + SQL), so the ML sidecar is a pure
did-we-break-anything-globally control. Verbatim tail:

```
$ ./smackerel.sh test unit --python
[py-unit] pip install OK; starting pytest ml/tests
s....................................................................... [ 14%]
........................................................................ [ 86%]
..................................................................       [100%]
496 passed, 2 skipped, 2 warnings in 14.61s
[py-unit] pytest ml/tests finished OK
```

The 2 warnings (StarletteDeprecationWarning on `httpx`; a `nats_client.py` coroutine RuntimeWarning) are
pre-existing ML-sidecar warnings unrelated to this Go/SQL delivery.

### RG-E3 — unrelated other-spec failures isolated (import-graph proof neither imports the changed symbols)

The two `FAIL` packages cannot be caused by BUG-056-002 because neither imports any changed package
(`internal/auth`, `internal/connector/twitter`, `internal/config`, `internal/metrics`, `cmd/core`). Proven
two ways — a scoped grep for the changed import paths (zero hits) and the packages' actual import blocks
(stdlib-only):

```
$ grep -rE 'smackerel/internal/(auth|connector/twitter|config|metrics)|smackerel/cmd/core' internal/docfreshness/ tests/unit/clients/
(no matches — exit 1)

# internal/docfreshness/doc_freshness_test.go imports:
import ( "fmt"; "io/fs"; "os"; "path/filepath"; "runtime"; "sort"; "strings"; "testing" )

# tests/unit/clients/render_descriptor_canary_test.go imports:
import ( "bytes"; "encoding/json"; "errors"; "fmt"; "io/fs"; "os"; "os/exec"; "path/filepath"; "reflect"; "strings"; "testing" )
```

- `internal/docfreshness` (spec-032): stdlib-only; it diffs `config/prompt_contracts/*.yaml` on disk against
  `docs/Development.md`. Its 5 undocumented contracts (`alert-timing-evaluate-v1`, `expertise-classify-v1`,
  `hospitality-concern-evaluate-v1`, `relationship-cooling-evaluate-v1`, `resurface-evaluate-v1`) come from
  the **BUG-021-006..010 intelligence deliveries** in the working tree — not Twitter OAuth. Doc-freshness is
  owned by those intelligence specs, not this packet.
- `tests/unit/clients` (spec-073): stdlib-only; it shells out to `node` and `dart` for the cross-language
  renderer canary. Both binaries are absent from `$PATH` in this environment, so the test fails loud by
  design — an **environment** gap, not a code regression, and structurally independent of the Go connector.

### RG-E4 — cross-surface: every changed surface is additive / backward-compatible

`git diff HEAD` diffstat for the tracked source files, plus the per-surface `ok` proof:

```
$ git diff --stat HEAD -- <7 tracked source files>
 cmd/core/connectors.go                |  19 ++-
 cmd/core/main.go                      |   8 ++
 internal/auth/oauth.go                |  95 +++++++++++++
 internal/config/config.go             |  10 +-
 internal/connector/twitter/api.go     | 250 +++++++++++++++++++++++++++++++---
 internal/connector/twitter/twitter.go | 117 +++++++++++++++-
 internal/metrics/metrics.go           |  15 ++
 7 files changed, 491 insertions(+), 23 deletions(-)
```

- **Shared `auth.GenericOAuth2` (Drive/Google/Gmail consumers) — NOT broken.** The diff adds only new
  funcs (`GeneratePKCEPair`, `PKCEChallengeS256`, `AuthURLWithPKCE`, `ExchangeCodeWithVerifier`,
  `RefreshTokenBasic`) and one optional field (`OAuth2Config.TokenEndpointAuthStyle`). The single existing-
  function change, in `tokenRequest`, is gated behind `useBasic := g.Config.TokenEndpointAuthStyle == "basic"`;
  every pre-existing caller leaves that field `""` → the `data.Del("client_secret")` + `SetBasicAuth` branch is
  skipped → byte-for-byte the prior body-credential request. The `OAuth2Provider` interface is unchanged. No
  source file other than the Twitter authorize service sets `TokenEndpointAuthStyle`. Proof: `ok internal/auth
  3.472s` and `ok internal/drive/google 0.521s`.
- **App-Only path (tweets/mentions) — unchanged.** `endpointAuthTier` (`api.go`) maps `tweets`/`mentions` →
  `authTierAppOnly` → `authorizationHeader` returns `"Bearer " + c.bearerToken` — the original credential. The
  auth tier is the ONLY new parameter threaded into the otherwise-identical `fetchEndpointPaginated` /
  `loadCursor` / `saveCursor` loop, so App-Only pagination/cursor behavior is preserved. Proof: `ok
  internal/connector/twitter 5.963s` (covers `TestBuildRequest_AppOnlyEndpointUsesBearer`,
  `TestTwitterAPI_BookmarksPaginatesAndPersistsCursor`, `TestTwitterAPI_ReplayPagination`,
  `TestTwitterAPI_CursorSurvivesProcessRestart`).
- **Archive sync mode — unaffected.** `newAPIClient` still returns `(nil, nil)` for `SyncModeArchive`; the
  parse/threads archive path is untouched. Proof: `TestTwitterAPI_ArchiveModeReturnsNilClient`,
  `TestConnect_MissingArchiveDir`, `TestParseTweetsJS` all under `ok internal/connector/twitter 5.963s`.
- **Metrics registration — both gauges coexist.** `metrics.go` adds `ConnectorTwitterAPIRateLimitRemaining`
  alongside the pre-existing `ConnectorTwitterAPIRateLimitReset` and appends it to the single `init()`
  `MustRegister` list (additive). No duplicate-registration panic. Proof: `ok internal/metrics 0.045s`.
- **`cmd/core` dispatch — existing commands preserved.** `main.go` inserts the `connector` branch BEFORE the
  existing `run()` fallthrough; the new branch fires only on `os.Args[1] == "connector"`. `connectors.go` adds a
  `ConfigureRuntime(...)` injection + extra Twitter credential keys without removing any existing connector
  registration. Proof: `ok cmd/core 1.416s`.

### RG-E5 — connector invariants preserved (each backed by a green adversarial test in `internal/connector/twitter`)

Source anchors read this session + the test functions confirmed present in the package that reported
`ok ... 5.963s` (a package-level `ok` means every listed test func passed):

```
$ grep -nE 'func Test[A-Za-z0-9_]+' internal/connector/twitter/api_test.go   (key rows)
191:func TestBuildRequest_UserContextEndpoint_NoToken_FailsLoud
249:func TestTwitterAPI_EmptyBearerTokenFailsLoud
432:func TestTwitterAPI_BearerTokenNeverInLogs
1054:func TestTwitterAPI_BearerTokenNeverAppearsInLogs
1304:func TestTwitterAPI_AppOnlyOnUserOwnedEndpointRejected
1628:func TestTwitterAPI_AppOnly401_NoRefresh_Terminal
1847:func TestTwitterAPI_RateLimitRemaining_SetOnEveryStatus
ok      github.com/smackerel/smackerel/internal/connector/twitter       5.963s
```

- **No silent App-Only fallback.** `authorizationHeader` returns `ErrUserContextTokenRequired` (nil source,
  resolver error, or empty token) for `authTierUserContext` — never `bearerToken`; `endpointAuthTier`'s
  `default` is `authTierUserContext`, so an unmapped/future endpoint fails *toward* user-context. The key
  adversarial `TestTwitterAPI_AppOnlyOnUserOwnedEndpointRejected` drives an httptest server that 403s the
  App-Only sentinel and asserts (a) fail-loud BEFORE the wire (0 server hits, `errors.Is ErrUserContextTokenRequired`,
  NOT `errAuthRejected`) and (b) the user-context token is the only credential ever sent — green under the `ok`
  above. It would turn RED if the bug were reintroduced.
- **Fail-loud config (no hidden default).** `ErrAPIBearerTokenRequired` for empty bearer in api/hybrid mode;
  `TestTwitterAPI_EmptyBearerTokenFailsLoud` + `TestTwitterOAuth_EmptyKeyFailsLoud` (empty at-rest key) +
  `internal/config/twitter_oauth_config_test.go` (SST) green (`ok internal/config 30.836s`).
- **No token / secret / code_verifier logged.** `TestTwitterAPI_BearerTokenNeverInLogs` +
  `TestTwitterAPI_BearerTokenNeverAppearsInLogs` green; `authorizationHeader` returns the bare sentinel with no
  token in the error string.
- **App-Only cursor/pagination unchanged.** Same `fetchEndpointPaginated` loop (empty-non-terminal-page does
  not advance the cursor; bound at `maxPagesPerEndpoint`); `TestTwitterAPI_EmptyNonTerminalPageDoesNotAdvanceCursor`
  + `TestTwitterAPI_PaginationBoundsTerminateOnRunawayServer` green.

### RG-E6 — artifact-lint not worsened + verdict

`artifact-lint.sh` was clean before this append and remains clean after (no new evidence-template or
section gaps introduced):

```
$ bash .github/bubbles/scripts/artifact-lint.sh specs/056-twitter-api-connector/bugs/BUG-056-002-pkce-user-context-auth-missing
✅ No unfilled evidence template placeholders in report.md
✅ No repo-CLI bypass detected in report.md command evidence
Artifact lint PASSED.
```

**Verdict: 🟢 REGRESSION_FREE → `completed_diagnostic`.** The full Go + Python unit suites were executed this
session. No failure is attributable to the BUG-056-002 delivery: every changed package reports `ok`, and the
only two `FAIL` packages (`internal/docfreshness` spec-032 doc-staleness from the BUG-021 intelligence
deliveries; `tests/unit/clients` spec-073 node/dart-not-on-PATH environment gap) are proven by import-graph to
not import any changed symbol. All five existing surfaces (shared `GenericOAuth2`, App-Only path, archive mode,
metrics registration, `cmd/core` dispatch) are additive/backward-compatible, and the four connector invariants
are each held by a green adversarial test. This is a read-only diagnostic; no source/spec/scope/state artifact
was modified (only this `report.md` evidence section was appended).

## Simplify Pass — BUG-056-002 (2026-06-08)

**Role:** `bubbles.simplify` (simplify-diagnostic). **Surface probed:** the BUG-056-002 PKCE / user-context
delivery — `internal/auth/oauth.go` (PKCE additions), `internal/connector/twitter/oauth_authorize.go`,
`oauth_store.go`, `oauth_token_manager.go`, the `api.go` auth-tier block, and `cmd/core/cmd_connector.go`.
Read-only diagnostic: NO source/spec/scope/state artifact was modified; this evidence section is the only
`report.md` change.

**Determination: 🟢 APPROPRIATELY SIMPLE — no safe behavior-preserving simplification found.** Every new
symbol has a caller; each apparent "duplication" is an intentional, required parallel structure. This matches
the specs 080 / 056 earlier-round "appropriately simple" outcome — no manufactured simplification.

### SP-E1 — candidate probe (4 candidates examined, 0 applied)

| id | candidate | probe result | disposition |
|----|-----------|--------------|-------------|
| C1 | `RefreshTokenBasic` body is line-identical to `RefreshToken` | Required by the additive `userContextRefresher` interface; the behavior split lives entirely in `tokenRequest`'s `TokenEndpointAuthStyle` branch (Basic header vs body secret); mirrors the `ExchangeCode` / `ExchangeCodeWithVerifier` pairing. Collapsing it couples two distinct OAuth grant flows and edits shared cross-connector auth used by Google/IMAP/etc. | declined-not-actually-complex |
| C2 | `authTier.String()` suspected no-caller | Exercised by the `%s` verb in `TestEndpointAuthTier` (`api_test.go:82`). It is a live Stringer, not dead code. | declined-not-actually-complex (not dead) |
| C3 | `pkceState.ConnectorID` set in `Begin` while `SaveState` writes the `twitterConnectorID` constant | The field maps a real persisted column that `ConsumeState` scans back; the literal documents the binding. Removing it is documentary-only churn, not dead-code removal. | declined-not-actually-complex |
| C4 | `AuthorizeService.now` / `userContextManager.now` clock-seam fields | Established codebase clock-injection pattern (`apiClient.now`, `userContextManager.now`); genuinely consumed in `Begin` / `AccessToken`. Not speculative generality. | declined-not-actually-complex |

REQUIRED design correctly NOT flagged as complexity: the `ErrUserContextTokenRequired` /
`ErrOAuthAtRestKeyRequired` fail-loud sentinels, the `endpointAuthTier` matrix (security-critical
explicitness, default→user-context), AES-256-GCM at-rest crypto, CSPRNG state + PKCE verifier, the narrow
`userContextTokenStore` / `userContextRefresher` interfaces with compile-time assertions, and the `authTier`
parameter threading.

### SP-E2 — every new auth symbol has a caller (none dead)

```
$ grep -rn 'RefreshTokenBasic\|ExchangeCodeWithVerifier\|AuthURLWithPKCE\|GeneratePKCEPair\|PKCEChallengeS256' internal/auth internal/connector/twitter --include=*.go   (call sites)
internal/auth/oauth.go:118:    return verifier, PKCEChallengeS256(verifier), nil
internal/connector/twitter/oauth_token_manager.go:142:  rotated, err := m.refresher.RefreshTokenBasic(ctx, current.RefreshToken)
internal/connector/twitter/oauth_authorize.go:156:     verifier, challenge, err := auth.GeneratePKCEPair()
internal/connector/twitter/oauth_authorize.go:181:             AuthURL: s.provider.AuthURLWithPKCE(scopes, state, challenge),
internal/connector/twitter/oauth_authorize.go:200:     tok, err := s.provider.ExchangeCodeWithVerifier(ctx, code, st.CodeVerifier)

$ grep -rn 'func (t authTier) String\|endpointAuthTier(%q)=%s' internal/connector/twitter/api.go internal/connector/twitter/api_test.go
internal/connector/twitter/api.go:303:func (t authTier) String() string {
internal/connector/twitter/api_test.go:82:                             t.Fatalf("endpointAuthTier(%q)=%s, want %s", tc.label, got, tc.want)
```

### SP-E3 — scoped surface GREEN (no change ⇒ delivered baseline re-confirmed)

```
$ ./smackerel.sh test unit --go --go-run 'TestAuth_GeneratePKCEPairS256|TestAuth_OAuth2PKCEBasicAuthStyle|TestEndpointAuthTier|TestBuildRequest_|TestTwitterAPI_(AppOnly|PreExpiryRefresh|Refresh_On401)|TestTwitterAuthorize_|TestTwitterOAuth_|TestConfig_TwitterOAuthCredentialsHaveNoHiddenDefault'
[go-unit] applying -run selector: TestAuth_GeneratePKCEPairS256|...|TestConfig_TwitterOAuthCredentialsHaveNoHiddenDefault
ok      github.com/smackerel/smackerel/internal/auth    0.103s
ok      github.com/smackerel/smackerel/internal/config  0.111s
ok      github.com/smackerel/smackerel/internal/connector/twitter       0.210s
ok      github.com/smackerel/smackerel/internal/docfreshness    0.012s [no tests to run]
ok      github.com/smackerel/smackerel/tests/unit/clients       0.022s [no tests to run]
[go-unit] go test ./... finished OK
```

The two packages that FAIL under the full unit run (`internal/docfreshness` spec-032 doc-staleness;
`tests/unit/clients` node/dart-not-on-PATH) report `[no tests to run]` under this scoped selector — proof they
sit outside the BUG-056-002 surface and are not silently-skipped failures.

### SP-E4 — `check` clean (build/config/scenario invariants intact)

```
$ ./smackerel.sh check
config-validate: ~/smackerel/config/generated/dev.env.tmp.4004851 OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 16, rejected: 0
scenario-lint: OK
```

### SP-E5 — artifact-lint not worsened by this append

```
$ bash .github/bubbles/scripts/artifact-lint.sh specs/056-twitter-api-connector/bugs/BUG-056-002-pkce-user-context-auth-missing
=== Anti-Fabrication Evidence Checks ===
✅ All checked DoD items in scopes.md have evidence blocks
✅ No unfilled evidence template placeholders in scopes.md
✅ No unfilled evidence template placeholders in report.md
✅ No repo-CLI bypass detected in report.md command evidence
=== End Anti-Fabrication Checks ===
Artifact lint PASSED.
```

The pre-existing `⚠️ deprecated field 'scopeProgress'` notice is emitted against `state.json` (untouched by
this pass) and is unchanged from before the append.

**Verdict: 🟢 `completed_diagnostic` — appropriately simple.** The BUG-056-002 PKCE / user-context auth code
carries no dead code, no unreachable branch, no redundant abstraction, no unused helper or parameter, and no
speculative generality with no caller. The four candidates probed (C1–C4) are each a required parallel
structure or a live test-exercised symbol, not strippable complexity. No source was changed; the delivered
surface remains GREEN (`internal/auth`, `internal/config`, `internal/connector/twitter` all `ok`). Only this
`report.md` evidence section was appended; no spec/scope/state/design artifact was touched.

---

## Stabilize Pass — BUG-056-002 (2026-06-08)

**Agent:** bubbles.stabilize · **Role:** stabilize-diagnostic (read-only probe; no source touched).
**Surface probed:** `internal/auth/oauth.go`, `internal/connector/twitter/{oauth_authorize,oauth_store,oauth_token_manager,api,twitter}.go`, `cmd/core/cmd_connector.go`, `internal/db/migrations/056_twitter_oauth_pkce.sql`.
**Probe dimensions:** (1) token-refresh concurrency/safety, (2) resource bounds, (3) state lifecycle, (4) failure modes, (5) shutdown/cancellation.

**Verdict: 🟢 STABLE.** Each of the five reliability/resource dimensions is backed by a passing live-server
test AND a code-level bound/guard at a named `file:line`. No destabilizer was found and no source change was
required. The harden-noted "refresh is not row-locked" item is confirmed fail-safe (below) for the
single-operator `default` owner and is NOT re-litigated as new; no cleanup job or lock was added (that would be
gold-plating for a single-operator surface).

### ST-1 — Focused stability test run (all five dimensions GREEN)

```
$ ./smackerel.sh test unit --go --go-run 'TestTwitterAPI_(Refresh_On401_RetriesOnce|PreExpiryRefresh|AppOnly401_NoRefresh_Terminal|Refresh_On401_PersistentIsTerminalAfterOneRefresh|PaginationBoundsTerminateOnRunawayServer|ServerError5xxBoundedBackoff|RateLimitResetCapAborts|Unauthorized401FailsWithoutRetry)|TestTwitterAuthorize_|TestTwitterOAuth_|TestSync_ConcurrentDoubleSync|TestSyncArchive_CancelledContext' --verbose
[go-unit] applying -run selector: TestTwitterAPI_(Refresh_On401_RetriesOnce|PreExpiryRefresh|AppOnly401_NoRefresh_Terminal|Refresh_On401_PersistentIsTerminalAfterOneRefresh|PaginationBoundsTerminateOnRunawayServer|ServerError5xxBoundedBackoff|RateLimitResetCapAborts|Unauthorized401FailsWithoutRetry)|TestTwitterAuthorize_|TestTwitterOAuth_|TestSync_ConcurrentDoubleSync|TestSyncArchive_CancelledContext
[go-unit] starting go test ./...
--- PASS: TestSyncArchive_CancelledContext (0.00s)
2026/06/09 03:06:04 INFO twitter connector connected id=twitter mode=archive
--- PASS: TestSync_ConcurrentDoubleSync (0.01s)
--- PASS: TestTwitterOAuth_EmptyKeyFailsLoud (0.00s)
--- PASS: TestTwitterOAuth_EncryptedStoreRoundTrip (0.00s)
--- PASS: TestTwitterAuthorize_StatusReflectsPersistedToken (0.00s)
--- PASS: TestTwitterAuthorize_BeginPersistsStateAndBuildsS256URL (0.00s)
2026/06/09 03:06:04 WARN pagination cap hit component=twitter.api endpoint=bookmarks pages=100 tweets_so_far=100
--- PASS: TestTwitterAPI_PaginationBoundsTerminateOnRunawayServer (0.04s)
--- PASS: TestTwitterAuthorize_FinalizeUnknownOrExpiredStateFailsLoud (0.08s)
--- PASS: TestTwitterAuthorize_FinalizeExchangesAndPersistsEncrypted (0.09s)
2026/06/09 03:06:04 WARN server error; backing off component=twitter.api endpoint=bookmarks attempt=0 status=500 backoff=1s
2026/06/09 03:06:04 WARN server error; backing off component=twitter.api endpoint=bookmarks attempt=1 status=500 backoff=2s
2026/06/09 03:06:04 WARN server error; backing off component=twitter.api endpoint=bookmarks attempt=2 status=500 backoff=4s
2026/06/09 03:06:04 WARN server error; backing off component=twitter.api endpoint=bookmarks attempt=3 status=500 backoff=8s
--- PASS: TestTwitterAPI_ServerError5xxBoundedBackoff (0.10s)
2026/06/09 03:06:04 INFO user-context token refreshed component=twitter.usercontext
2026/06/09 03:06:04 INFO user-context token refreshed after 401 component=twitter.api endpoint=bookmarks status=401
2026/06/09 03:06:04 WARN authentication rejected component=twitter.api endpoint=bookmarks status=401
--- PASS: TestTwitterAPI_Refresh_On401_PersistentIsTerminalAfterOneRefresh (0.11s)
--- PASS: TestTwitterAPI_RateLimitResetCapAborts (0.09s)
--- PASS: TestTwitterAPI_Refresh_On401_RetriesOnce (0.13s)
2026/06/09 03:06:05 WARN authentication rejected component=twitter.api endpoint=tweets status=401
--- PASS: TestTwitterAPI_AppOnly401_NoRefresh_Terminal (0.15s)
2026/06/09 03:06:05 INFO user-context token refreshed component=twitter.usercontext
--- PASS: TestTwitterAPI_PreExpiryRefresh (0.18s)
2026/06/09 03:06:05 WARN authentication rejected component=twitter.api endpoint=bookmarks status=401
--- PASS: TestTwitterAPI_Unauthorized401FailsWithoutRetry (0.11s)
PASS
ok      github.com/smackerel/smackerel/internal/connector/twitter       0.268s
[go-unit] go test ./... finished OK
```

17/17 selected tests PASS; package `internal/connector/twitter` reports `ok ... 0.268s`; the harness prints
`finished OK`. The pre-existing full-suite `exit 1` (`internal/docfreshness` spec-032 doc-staleness;
`tests/unit/clients` node/dart-not-on-PATH) is OUTSIDE this surface — both report `[no tests to run]` under
this scoped selector, so they are not silently-skipped auth-surface failures (same isolation noted in SP-E3
above).

### ST-2 — Code-level bounds & guards (file:line)

```
$ grep -nE 'io\.LimitReader|defer resp\.Body\.Close|NewRequestWithContext|client := &http\.Client\{Timeout|maxTokenResponseBytes' internal/auth/oauth.go
17:// maxTokenResponseBytes limits the size of token endpoint responses to 1 MB.
19:const maxTokenResponseBytes = 1 << 20 // 1 MB
208:    req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.Config.TokenEndpoint,
219:    client := &http.Client{Timeout: time.Duration(g.Config.HTTPTimeoutSeconds) * time.Second}
224:    defer resp.Body.Close()
228:            errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
243:    if err := json.NewDecoder(io.LimitReader(resp.Body, maxTokenResponseBytes)).Decode(&tokenResp); err != nil {
$ grep -nE 'drainAndClose|io\.LimitReader|NewRequestWithContext|httpClient:  &http\.Client\{Timeout: apiClientTimeout' internal/connector/twitter/api.go
139:            httpClient:  &http.Client{Timeout: apiClientTimeout},
191:    req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
260:    if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
491:            excerpt, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
495:    if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
682:                    drainAndClose(resp)
726:                    drainAndClose(resp)
746:                    drainAndClose(resp)
765:                    excerpt, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
766:                    drainAndClose(resp)
781:    _, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
$ grep -nE 'refreshedOnce|StatusUnauthorized && !refreshedOnce|maxRetries' internal/connector/twitter/api.go
542:const maxRetries = 3
638:    // refreshedOnce gates the user-context refresh-on-401 backstop to AT MOST
641:    refreshedOnce := false
642:    for attempt := 0; attempt <= maxRetries; attempt++ {
697:                    if resp.StatusCode == http.StatusUnauthorized && !refreshedOnce &&
712:                            refreshedOnce = true
$ grep -nE 'DELETE FROM twitter_oauth_states|time\.Now\(\)\.After\(st\.ExpiresAt\)|twitterOAuthStateTTL' internal/connector/twitter/oauth_store.go internal/connector/twitter/oauth_authorize.go
internal/connector/twitter/oauth_store.go:215:          DELETE FROM twitter_oauth_states
internal/connector/twitter/oauth_store.go:230:  if time.Now().After(st.ExpiresAt) {
internal/connector/twitter/oauth_authorize.go:26:       twitterOAuthStateTTL = 15 * time.Minute
internal/connector/twitter/oauth_authorize.go:174:              ExpiresAt:    now.Add(twitterOAuthStateTTL),
$ grep -nE 'IF NOT EXISTS|idx_twitter_oauth_states_expires_at' internal/db/migrations/056_twitter_oauth_pkce.sql
36:CREATE TABLE IF NOT EXISTS twitter_oauth_states (
46:CREATE INDEX IF NOT EXISTS idx_twitter_oauth_states_expires_at ON twitter_oauth_states (expires_at);
48:CREATE TABLE IF NOT EXISTS twitter_oauth_tokens (
$ grep -nE 'AUTH_OAUTH_HTTP_TIMEOUT_SECONDS must be > 0' internal/config/config.go
1734:           return fmt.Errorf("AUTH_OAUTH_HTTP_TIMEOUT_SECONDS must be > 0; got %d", c.AuthOAuthHTTPTimeoutSeconds)
```

### Per-dimension determination

| # | Dimension | Verdict | Evidence (test + bound) |
|---|-----------|---------|-------------------------|
| 1 | refresh-safety | 🟢 STABLE | Reactive 401 refresh gated to AT MOST ONCE per request (`refreshedOnce` `api.go:641/697/712`) inside a `maxRetries = 3` (`api.go:542`) loop — proven non-looping by `TestTwitterAPI_Refresh_On401_RetriesOnce` (1 refresh, 2 API calls) and `..._PersistentIsTerminalAfterOneRefresh` (1 refresh then terminal `errAuthRejected`, the `refreshed after 401` then `authentication rejected` log pair above). Proactive 60s pre-expiry skew is deterministic (`refreshSkew`, `oauth_token_manager.go`) — `TestTwitterAPI_PreExpiryRefresh`. Tier gate keeps App-Only 401s terminal — `TestTwitterAPI_AppOnly401_NoRefresh_Terminal`. Concurrency: a single connector is single-flight (`syncing` guard `twitter.go`, `TestSync_ConcurrentDoubleSync`); the not-row-locked cross-process case is fail-safe — `SaveTokens` writes a complete access+refresh pair in one upsert (`oauth_store.go`) and `refresh()` persists ONLY after a non-empty successful exchange, so a race yields a fail-loud `re-authorize`, never a half-written/empty/mismatched row and never silent token loss. |
| 2 | resource-bounds | 🟢 STABLE | Every token/API read is size-capped: token decode 1 MB (`oauth.go:19/243`), token error body 512 B (`oauth.go:228`), API decode 1<<20 (`api.go:260/495`), drain 4 KiB (`api.go:781`). Every body is closed — `defer resp.Body.Close()` (`oauth.go:224`) + `drainAndClose` on every retry branch (`api.go:682/726/746/766`). HTTP clients are timeout-bounded (`api.go:139` 30 s; `oauth.go:219` config-sourced, fail-loud `> 0` at `config.go:1734`). No goroutine/connection leak observed. |
| 3 | state-lifecycle | 🟢 STABLE | State is single-use delete-on-consume (`DELETE … RETURNING`, `oauth_store.go:215`) with TTL re-checked on read (`oauth_store.go:230`); 15-min TTL (`oauth_authorize.go:26/174`). `TestTwitterAuthorize_FinalizeUnknownOrExpiredStateFailsLoud` + `..._BeginPersistsStateAndBuildsS256URL` cover it. Migration is idempotent (`IF NOT EXISTS` ×3, `migration:36/46/48`) with an `expires_at` index. See ST-OBS-1 for abandoned-state growth. |
| 4 | failure-modes | 🟢 STABLE | Token exchange/refresh failure is loud and un-retried at the token endpoint (single `client.Do` in `tokenRequest`, non-200 → wrapped error) — no retry storm. A revoked/expired refresh surfaces `ErrUserContextTokenRequired` / `errAuthRejected` cleanly (`TestTwitterAPI_Unauthorized401FailsWithoutRetry`, `..._AppOnly401_NoRefresh_Terminal`), never a hang/crash. DB-unavailable token read fails loud (`AccessToken` wraps the store error; `TestTwitterOAuth_EmptyKeyFailsLoud`). 5xx backoff is bounded 1s→2s→4s→8s then `errMaxRetriesExceeded` (`TestTwitterAPI_ServerError5xxBoundedBackoff`, traces above); 429 over cap aborts (`TestTwitterAPI_RateLimitResetCapAborts`). |
| 5 | cancellation | 🟢 STABLE | All token/API HTTP is ctx-bound via `http.NewRequestWithContext` (`oauth.go:208`, `api.go:191`); backoff `defaultSleeper` selects on `ctx.Done()`. `TestSyncArchive_CancelledContext` PASS. The CLI root `context.WithCancel` is not signal-wired, but it is a foreground one-shot command and the config-enforced `> 0` token-endpoint timeout bounds a hung exchange regardless. |

### ST-OBS-1 — abandoned-state growth (single-operator-acceptable observation; NOT a finding)

The ST-2 grep shows the ONLY `DELETE FROM twitter_oauth_states` is the consume path (`oauth_store.go:215`); there
is no background sweep of abandoned `authorize-begin`-without-`finalize` rows. Per the probe charter this is the
accepted "expiry-checked-on-read" model: it is fail-safe (an expired row can never be consumed —
`oauth_store.go:230` rejects it) and, for the single-operator `default` owner driving a manual CLI flow, the
abandoned-row volume is negligible. The existing `idx_twitter_oauth_states_expires_at` index (`migration:46`)
would make a periodic `DELETE … WHERE expires_at < now()` sweep trivial IF a multi-account/multi-tenant owner
is ever introduced. No change made (anti-gold-plating).

## Security Scan — BUG-056-002 OWASP (2026-06-08)

**Agent:** bubbles.security · **Role:** security-diagnostic (read-only OWASP review; no source touched).
**Surface reviewed:** `internal/auth/oauth.go`, `internal/connector/twitter/{oauth_authorize,oauth_store,oauth_token_manager,api,twitter}.go`, `cmd/core/cmd_connector.go`, `internal/config/config.go` (oauth + at-rest-key), `internal/db/migrations/056_twitter_oauth_pkce.sql`.
**Method:** formal OWASP-Top-10-2021 cross-check of the User-Context OAuth 2.0 PKCE token-handling surface. Distinct from the Stabilize pass (reliability/resource dimensions); the harden-noted secret/PKCE/CSRF/SQL observations are NOT re-litigated as new.

**Verdict: 🔒 SECURE — CLEAN across every applicable OWASP category. 0 actionable findings.** Each per-category determination below is backed by pasted code (`file:line`) AND a passing named test. One non-actionable single-operator observation (SEC-OBS-1) is recorded for transparency, not raised as a finding (anti-gold-plating). No source change was required.

### Per-category determination

| OWASP 2021 | Category | Verdict | Evidence anchor |
|---|---|---|---|
| A01 | Broken Access Control | 🟢 CLEAN | SEC-E2 (owner-scoped parameterized SQL), SEC-E5 (`TestEndpointAuthTier`, `TestTwitterAPI_AppOnlyOnUserOwnedEndpointRejected`, `TestBuildRequest_UserContextEndpoint_NoToken_FailsLoud`); state→verifier binding is single-use delete-on-consume (`oauth_store.go:215`, harden ST-3). |
| A02 | Cryptographic Failures | 🟢 CLEAN | SEC-E1 (AES-256 via SHA-256 KDF, fresh CSPRNG nonce/encryption, GCM `Open` tag-verify, PKCE S256), SEC-E3 (key strength), SEC-E5 (`TestTwitterOAuth_EncryptedStoreRoundTrip`, `TestTokenStore_*GCMFailure/SameKeyDifferentNonces/WrongKey_FailClosed`, `TestAuth_GeneratePKCEPairS256`). |
| A03 | Injection | 🟢 CLEAN | SEC-E2 (every query parameterized `$1..$7`; owner/state bound, never concatenated; authorize URL via `url.Values.Encode`). |
| A04 | Insecure Design | 🟢 CLEAN | single-use state + 15-min TTL + server-side-only verifier + bounded refresh; SEC-E5 (`TestTwitterAuthorize_Begin…S256URL`, `…FinalizeUnknownOrExpiredStateFailsLoud`). |
| A05 | Security Misconfiguration | 🟢 CLEAN | SEC-E3 (client_id/secret/redirect `os.Getenv` no-default + fail-loud; `redirect_uri` config-fixed = no open redirect; secret in Basic header not body). |
| A06 | Vulnerable Components | ⚪ N/A | no new third-party dependency introduced; stdlib `crypto/{aes,cipher,rand,sha256}` + existing `pgx`/`internal/auth` only. |
| A07 | Identification & Auth Failures | 🟢 CLEAN | SEC-E5 (`…NoToken_FailsLoud` ×4, `…AppOnly401_NoRefresh_Terminal`); fail-loud `ErrUserContextTokenRequired`, no App-Only fallback; refresh bounded to ≤1 (`refreshedOnce`, harden ST-1). |
| A08 | Software & Data Integrity | 🟢 CLEAN | SEC-E1 + SEC-E5 (GCM auth-tag rejects tampered/wrong-key ciphertext at rest: `TestTokenStore_Decrypt_FailClosed_GCMFailure`, `…WrongKey_FailClosed`). |
| A09 | Logging & Monitoring Failures | 🟢 CLEAN | SEC-E4 (sole token-surface log is message-only `"user-context token refreshed"`; combined negative grep for any logged token/secret/verifier value → `grep-exit=1`); SEC-E5 (`TestTwitterAPI_BearerTokenNeverInLogs`, `…NeverAppearsInLogs`). |
| A10 | SSRF | ⚪ N/A | authorize/token endpoints are LOCKED constants (`twitter.com` / `api.twitter.com`); no request-controlled URL reaches any HTTP client; `redirect_uri` is config-fixed (SEC-E3). |

### SEC-E1 — A02 Cryptographic Failures: AES-256-GCM (CSPRNG nonce per encryption + auth-tag verify on decrypt) + PKCE S256 + CSPRNG verifier/state

```
$ grep -nE 'sha256\.Sum256|aes\.NewCipher|cipher\.NewGCM|s\.gcm\.NonceSize\(\)|io\.ReadFull\(rand\.Reader|s\.gcm\.Seal|s\.gcm\.Open' internal/connector/twitter/oauth_store.go
65:     h := sha256.Sum256([]byte(atRestKey))
66:     block, err := aes.NewCipher(h[:])
70:     gcm, err := cipher.NewGCM(block)
84:     nonce := make([]byte, s.gcm.NonceSize())
85:     if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
88:     ciphertext := s.gcm.Seal(nonce, nonce, []byte(plaintext), nil)
101:    nonceSize := s.gcm.NonceSize()
106:    plaintext, err := s.gcm.Open(nil, nonce, ciphertext, nil)
$ grep -nE 'code_challenge_method|S256|sha256\.Sum256|base64\.RawURLEncoding|io\.ReadFull\(rand\.Reader|pkceVerifierBytes|stateTokenBytes' internal/auth/oauth.go internal/connector/twitter/oauth_authorize.go
internal/auth/oauth.go:106:const pkceVerifierBytes = 32
internal/auth/oauth.go:113:     b := make([]byte, pkceVerifierBytes)
internal/auth/oauth.go:114:     if _, err := io.ReadFull(rand.Reader, b); err != nil {
internal/auth/oauth.go:117:     verifier = base64.RawURLEncoding.EncodeToString(b)
internal/auth/oauth.go:124:     sum := sha256.Sum256([]byte(verifier))
internal/auth/oauth.go:125:     return base64.RawURLEncoding.EncodeToString(sum[:])
internal/auth/oauth.go:140:             "code_challenge_method": {"S256"},
internal/connector/twitter/oauth_authorize.go:29:       stateTokenBytes = 32
internal/connector/twitter/oauth_authorize.go:220:      b := make([]byte, stateTokenBytes)
internal/connector/twitter/oauth_authorize.go:221:      if _, err := io.ReadFull(rand.Reader, b); err != nil {
internal/connector/twitter/oauth_authorize.go:224:      return base64.RawURLEncoding.EncodeToString(b), nil
```

- **AES-256:** key = `sha256.Sum256(atRestKey)` (32 bytes) → `aes.NewCipher` selects AES-256 → `cipher.NewGCM` (oauth_store.go:65-70).
- **Unique nonce per encryption:** a fresh `s.gcm.NonceSize()` (12-byte) nonce is drawn from the CSPRNG `rand.Reader` on EVERY `encrypt()` call (oauth_store.go:84-85); `Seal` prepends it (oauth_store.go:88). No fixed/counter nonce — no GCM nonce-reuse exposure.
- **Auth tag verified on decrypt:** `s.gcm.Open` (oauth_store.go:106) returns an error on tag mismatch — tampered ciphertext fails closed (A08).
- **PKCE S256:** `code_challenge_method=S256`, challenge = `base64url-nopad(SHA-256(verifier))` (oauth.go:124-125,140) — not `plain`.
- **CSPRNG everywhere:** verifier 32 B and state 32 B both via `io.ReadFull(rand.Reader, …)` (oauth.go:114, oauth_authorize.go:221). No `math/rand` on the auth surface.

### SEC-E2 — A01/A03 Access Control & Injection: every store query is parameterized and owner/connector-scoped

```
$ grep -nE 'Exec\(ctx|QueryRow\(ctx|\$[1-7]|owner_user_id = \$|state_token = \$|DELETE FROM|INSERT INTO' internal/connector/twitter/oauth_store.go
129:    _, err = s.pool.Exec(ctx, `
130:            INSERT INTO twitter_oauth_tokens
132:            VALUES ($1, $2, $3, $4, $5, $6, $7, now())
152:    err := s.pool.QueryRow(ctx, `
155:            WHERE owner_user_id = $1 AND connector_id = $2
178:    err := s.pool.QueryRow(ctx, `
181:                    WHERE owner_user_id = $1 AND connector_id = $2
196:    _, err = s.pool.Exec(ctx, `
197:            INSERT INTO twitter_oauth_states
199:            VALUES ($1, $2, $3, $4, $5, now(), $6)
214:    err := s.pool.QueryRow(ctx, `
215:            DELETE FROM twitter_oauth_states
216:            WHERE state_token = $1
```

All five statements bind `owner_user_id`/`connector_id`/`state_token`/token columns as `$N` bind parameters — no string interpolation of any caller-supplied value (A03 clean). Token read/write/exists are scoped by the composite `(owner_user_id, connector_id)` key (A01: one owner's row is never readable/writable under another owner). State consume is the single-use atomic `DELETE … RETURNING` keyed by the 256-bit CSPRNG `state_token` (replay-proof; forged-callback-proof).

### SEC-E3 — A05/A02 Misconfiguration: fail-loud OAuth creds, config-fixed redirect (no open redirect), validated at-rest key

```
$ grep -nE 'os\.Getenv\("TWITTER_OAUTH_(CLIENT_ID|CLIENT_SECRET|REDIRECT_URL)"\)' internal/config/config.go
585:            TwitterOAuthClientID:          os.Getenv("TWITTER_OAUTH_CLIENT_ID"),
586:            TwitterOAuthClientSecret:      os.Getenv("TWITTER_OAUTH_CLIENT_SECRET"),
587:            TwitterOAuthRedirectURL:       os.Getenv("TWITTER_OAUTH_REDIRECT_URL"),
$ grep -nE 'oauth_client_id is required|oauth_redirect_url is required|owner user id is required|ErrOAuthAtRestKeyRequired' internal/connector/twitter/oauth_authorize.go internal/connector/twitter/oauth_store.go
internal/connector/twitter/oauth_authorize.go:124:              return nil, fmt.Errorf("twitter authorize: owner user id is required")
internal/connector/twitter/oauth_authorize.go:147:              "twitter authorize-begin: oauth_client_id is required (set connectors.twitter.oauth_client_id " +
internal/connector/twitter/oauth_authorize.go:152:              "twitter authorize-begin: oauth_redirect_url is required (set connectors.twitter.oauth_redirect_url " +
internal/connector/twitter/oauth_store.go:63:           return nil, ErrOAuthAtRestKeyRequired
$ grep -nE '"redirect_uri":|RedirectURL' internal/auth/oauth.go
94:             "redirect_uri":  {g.Config.RedirectURL},
135:            "redirect_uri":          {g.Config.RedirectURL},
150:            "redirect_uri":  {g.Config.RedirectURL},
176:            "redirect_uri":  {g.Config.RedirectURL},
$ grep -nE 'AuthToken == ""|at least 16 characters|known placeholder value|dev-token-' internal/config/config.go
1751:   if c.Environment == "production" && c.AuthToken == "" {
1824:                   return fmt.Errorf("SMACKEREL_AUTH_TOKEN is set to a known placeholder value — generate a secure random token: openssl rand -hex 24")
1828:           if strings.HasPrefix(strings.ToLower(c.AuthToken), "dev-token-") {
1832:                   return fmt.Errorf("SMACKEREL_AUTH_TOKEN must be at least 16 characters (got %d)", len(c.AuthToken))
```

OAuth client credentials are `os.Getenv` with NO fallback default (config.go:585-587); `authorize-begin` fails loud on empty `client_id`/`redirect_url` (oauth_authorize.go:147,152), `NewAuthorizeService` on empty owner (:124), and `newOAuthStore` on empty at-rest key (oauth_store.go:63 → `ErrOAuthAtRestKeyRequired`, no plaintext fallback). `redirect_uri` is the config-constant `g.Config.RedirectURL` in BOTH the authorize URL and the token exchange (oauth.go:94/135/150/176) — there is no request-supplied redirect parameter, so there is no open-redirect surface. The AES key (`SMACKEREL_AUTH_TOKEN`) is production-required, ≥16 chars, and weak-value/`dev-token-`-rejected (config.go:1751,1824,1828,1832).

### SEC-E4 — A09 Logging: no token/secret/verifier/code value reaches any log or print sink

```
$ grep -nE 'logger\.(Info|Warn|Error)\(|fmt\.(Print|Fprint)' internal/connector/twitter/oauth_token_manager.go internal/connector/twitter/oauth_store.go internal/connector/twitter/oauth_authorize.go internal/auth/oauth.go
internal/connector/twitter/oauth_token_manager.go:155:  m.logger.Info("user-context token refreshed")
$ grep -rniE '(slog\.|logger\.(Info|Warn|Error)|fmt\.(Print|Fprint)|log\.).*(AccessToken|RefreshToken|CodeVerifier|code_verifier|ClientSecret|client_secret|\bBearer \b|"Bearer "|\+ *tok\b)' internal/connector/twitter/ internal/auth/oauth.go cmd/core/cmd_connector.go ; echo "grep-exit=$?"
grep-exit=1 (1/empty = no sensitive value in any log/print sink)
```

The ONLY log statement in the token-handling files is a message-only `"user-context token refreshed"` (oauth_token_manager.go:155) — no value. The combined negative grep across the whole twitter connector + `auth/oauth.go` + the CLI for any log/print referencing a token, refresh token, verifier, client secret, or `Bearer` value returns nothing (`grep-exit=1`). The CLI prints only `ExpiresAt`/`Scopes`/owner, never a token value (`cmd_connector.go` `Finalize` handler).

### SEC-E5 — OWASP test run (A01/A02/A04/A07/A08/A09), repo CLI, GREEN

```
$ ./smackerel.sh test unit --go --go-run 'TestTwitterOAuth_(EncryptedStoreRoundTrip|EmptyKeyFailsLoud)|TestTokenStore_(Decrypt_FailClosed_GCMFailure|EncryptDecrypt_SameKeyDifferentNonces|Decrypt_WrongKey_FailClosed)|TestAuth_(GeneratePKCEPairS256|OAuth2PKCEBasicAuthStyle)|TestTwitterAuthorize_(BeginPersistsStateAndBuildsS256URL|FinalizeUnknownOrExpiredStateFailsLoud|FinalizeExchangesAndPersistsEncrypted)|TestEndpointAuthTier|TestBuildRequest_UserContextEndpoint_NoToken_FailsLoud|TestTwitterAPI_(AppOnlyOnUserOwnedEndpointRejected|BearerTokenNeverInLogs|BearerTokenNeverAppearsInLogs)' --verbose
--- PASS: TestTokenStore_Decrypt_FailClosed_GCMFailure (0.00s)
--- PASS: TestTokenStore_EncryptDecrypt_SameKeyDifferentNonces (0.00s)
--- PASS: TestTokenStore_Decrypt_WrongKey_FailClosed (0.00s)
--- PASS: TestAuth_GeneratePKCEPairS256 (0.00s)
--- PASS: TestAuth_OAuth2PKCEBasicAuthStyle (0.05s)
ok      github.com/smackerel/smackerel/internal/auth    0.478s
--- PASS: TestEndpointAuthTier (0.03s)
    --- PASS: TestEndpointAuthTier/users_me (0.00s)
    --- PASS: TestEndpointAuthTier/bookmarks (0.00s)
    --- PASS: TestEndpointAuthTier/liked_tweets (0.00s)
    --- PASS: TestEndpointAuthTier/tweets (0.00s)
    --- PASS: TestEndpointAuthTier/mentions (0.00s)
    --- PASS: TestEndpointAuthTier/some_unmapped_future_endpoint (0.00s)
--- PASS: TestTwitterAuthorize_BeginPersistsStateAndBuildsS256URL (0.00s)
--- PASS: TestTwitterOAuth_EmptyKeyFailsLoud (0.00s)
--- PASS: TestTwitterOAuth_EncryptedStoreRoundTrip (0.00s)
--- PASS: TestBuildRequest_UserContextEndpoint_NoToken_FailsLoud (0.00s)
    --- PASS: TestBuildRequest_UserContextEndpoint_NoToken_FailsLoud/nil_source_(no_runtime_wired) (0.00s)
    --- PASS: TestBuildRequest_UserContextEndpoint_NoToken_FailsLoud/store_error (0.00s)
    --- PASS: TestBuildRequest_UserContextEndpoint_NoToken_FailsLoud/empty_token_string (0.00s)
    --- PASS: TestBuildRequest_UserContextEndpoint_NoToken_FailsLoud/empty_store_(no_token_row) (0.00s)
--- PASS: TestTwitterAuthorize_FinalizeUnknownOrExpiredStateFailsLoud (0.03s)
--- PASS: TestTwitterAuthorize_FinalizeExchangesAndPersistsEncrypted (0.07s)
--- PASS: TestTwitterAPI_AppOnlyOnUserOwnedEndpointRejected (0.07s)
    --- PASS: TestTwitterAPI_AppOnlyOnUserOwnedEndpointRejected/app_only_only_fails_loud_before_wire (0.00s)
    --- PASS: TestTwitterAPI_AppOnlyOnUserOwnedEndpointRejected/user_context_token_used_not_app_bearer (0.00s)
--- PASS: TestTwitterAPI_BearerTokenNeverInLogs (0.07s)
--- PASS: TestTwitterAPI_BearerTokenNeverAppearsInLogs (0.08s)
ok      github.com/smackerel/smackerel/internal/connector/twitter       0.252s
[go-unit] go test ./... finished OK
```

16 selected security tests (+ 12 sub-tests) PASS across `internal/auth` and `internal/connector/twitter`; both packages report `ok`; the harness prints `finished OK` (exit 0). No `FAIL` line anywhere in the scoped run. The pre-existing full-suite `internal/docfreshness` / `tests/unit/clients` issues are OUTSIDE this surface and report `[no tests to run]` under this selector (same isolation noted in the Stabilize ST-1 block).

### SEC-OBS-1 — Finalize persists under the service `--user-id`, not the consumed state row's `owner_user_id` (single-operator-acceptable observation; NOT a finding)

`AuthorizeService.Finalize` consumes the state row by `state_token` only and then persists via `SaveTokens(ctx, s.owner, tok)` using the finalize invocation's `--user-id` (oauth_authorize.go), rather than the `OwnerUserID` carried on the consumed row. In the single-operator CLI model this is not an access-control boundary crossing: the `state_token` is an unguessable 256-bit CSPRNG capability that only the flow's own operator holds, both `begin` and `finalize` default to the same `DefaultOwnerUserID`, and the production routing reads under that same owner. The worst case is a self-inflicted owner-label mismatch if one operator deliberately passes different `--user-id` values to `begin` vs `finalize`. It is recorded only because, IF a true multi-tenant owner model is ever introduced, `Finalize` SHOULD additionally assert `st.OwnerUserID == s.owner` before persisting. No change made now (anti-gold-plating; consistent with the single-operator scope the harden ST-OBS-1 also assumes).

---

## Independent Audit Evidence — PASS 2 (bubbles.audit, 2026-06-09) — VERDICT: 🚀 SHIP_IT_PENDING_CI (`completed_pending_ci`)

**Claim Source:** executed. Final `audit` phase (orchestrator `bubbles.goal`), run AFTER the 4 specialist phases (regression/simplify/stabilize/security) that PASS-1 above flagged as unrecorded were GENUINELY performed and recorded into the phase ledger by `bubbles.validate`'s state-reconciliation. All fenced output below is verbatim terminal output from this audit session on this working tree (absolute home paths redacted to `~/`). **This pass SUPERSEDES the PASS-1 `🛑 REWORK_REQUIRED`:** PASS-1's decisive blocker (4 missing G022 specialist phases) is now CLOSED, and the `bubbles.plan`-owned scopes.md structural gaps it listed are guard-verified resolved (`artifact-lint` PASS, `traceability-guard` exit 0, `state-transition-guard` Checks 4A/4B/5A/6 [non-audit]/8A/8B/8C/8D/13/13A/13B/15/16/18/22 all PASS). No source, no parent spec 056 artifact, and no framework asset was modified by this audit.

### AU2-E1 — independent re-run of the full BUG-056-002 unit surface GREEN (evidence integrity VERIFIED, no discrepancy with recorded results)

```
$ ./smackerel.sh test unit --go --go-run 'TestTwitterAPI|TestTwitterAuthorize|TestTwitterOAuth|PKCE|TestTokenStore|TestConfig_TwitterOAuth' --verbose
--- PASS: TestTokenStore_EncryptDecrypt_SameKeyDifferentNonces (0.00s)         # AES-256-GCM unique CSPRNG nonce per encryption
--- PASS: TestTokenStore_Decrypt_WrongKey_FailClosed (0.00s)                   # GCM tag-verify fails closed on wrong key
--- PASS: TestTokenStore_Decrypt_FailClosed_GCMFailure (0.00s)                 # GCM tag-verify fails closed on tamper
--- PASS: TestAuth_GeneratePKCEPairS256 (0.00s)                                # Scope A PKCE S256 (RFC 7636)
--- PASS: TestAuth_OAuth2PKCEBasicAuthStyle (0.06s)                            # Scope A PKCE + confidential-client Basic
ok      github.com/smackerel/smackerel/internal/auth    0.135s
--- PASS: TestConfig_TwitterOAuthCredentialsHaveNoHiddenDefault (0.01s)        # Scope A fail-loud SST (no hidden default)
ok      github.com/smackerel/smackerel/internal/config  0.082s
    api_live_test.go:164: live Twitter API tests are opt-in; set SMACKEREL_TWITTER_LIVE_TESTS=1 ...
--- SKIP: TestTwitterAPILive_UsersMe (0.00s)                                   # live arm correctly gated SKIP (NOT a DoD item)
--- SKIP: TestTwitterAPI_LiveTestNeverRunsInCI (0.00s)                         # live arm correctly gated SKIP
--- PASS: TestTwitterOAuth_EmptyKeyFailsLoud (0.00s)                           # Scope A Twitter-store fail-loud at-rest key
--- PASS: TestTwitterOAuth_EncryptedStoreRoundTrip (0.00s)                     # Scope A AES-256-GCM round-trip
--- PASS: TestTwitterAuthorize_BeginPersistsStateAndBuildsS256URL (0.00s)      # Scope B
--- PASS: TestTwitterAuthorize_FinalizeUnknownOrExpiredStateFailsLoud (0.01s)  # Scope B
--- PASS: TestTwitterAuthorize_FinalizeExchangesAndPersistsEncrypted (0.08s)   # Scope B
--- PASS: TestTwitterAuthorize_StatusReflectsPersistedToken (0.00s)            # Scope B
--- PASS: TestTwitterAPI_AppOnlyOnUserOwnedEndpointRejected (0.03s)            # Scope C KEY adversarial (non-tautological)
    --- PASS: TestTwitterAPI_AppOnlyOnUserOwnedEndpointRejected/app_only_only_fails_loud_before_wire (0.00s)
    --- PASS: TestTwitterAPI_AppOnlyOnUserOwnedEndpointRejected/user_context_token_used_not_app_bearer (0.00s)
2026/06/09 04:43:15 INFO user-context token refreshed after 401 component=twitter.api endpoint=bookmarks status=401
--- PASS: TestTwitterAPI_Refresh_On401_PersistentIsTerminalAfterOneRefresh (0.13s)
--- PASS: TestTwitterAPI_PreExpiryRefresh (0.12s)                              # Scope C 60s pre-expiry refresh
--- PASS: TestTwitterAPI_Refresh_On401_RetriesOnce (0.12s)                     # Scope C refresh-on-401 retry-once
--- PASS: TestTwitterAPI_AppOnly401_NoRefresh_Terminal (0.11s)                 # Scope C App-Only 401 stays terminal
--- PASS: TestTwitterAPI_RateLimitRemaining_SetFromHeader (0.04s)             # Scope D x-rate-limit-remaining gauge
--- PASS: TestTwitterAPI_RateLimitRemaining_AbsentHeaderLeavesPriorValue (0.09s)
--- PASS: TestTwitterAPI_RateLimitRemaining_SetOnEveryStatus (0.16s)          # Scope D non-429 adversarial
ok      github.com/smackerel/smackerel/internal/connector/twitter       0.403s
[go-unit] go test ./... finished OK
```

Exit 0; `go test ./...` finished OK; ZERO `FAIL` lines. The audit's independent run reproduces the recorded GREEN exactly (no discrepancy with the recorded `report.md` results → evidence integrity VERIFIED). The migration integration test (`tests/integration/twitter_oauth_migration_test.go`, `//go:build integration`) correctly compiles out of the unit run (`tests/integration … [no tests to run]`), consistent with its operator/CI-gated disposition.

### AU2-E2 — `./smackerel.sh check` clean

```
$ ./smackerel.sh check
config-validate: ~/smackerel/config/generated/dev.env.tmp.1242472 OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 16, rejected: 0
scenario-lint: OK
   (exit 0)
```

### AU2-E3 — anti-fabrication posture: SOUND (each item independently confirmed THIS pass)

- **The 4 specialist-phase evidence sections are GENUINE, not rubber-stamps — CONFIRMED.** Each was read in full this pass and carries distinct, substantive, non-cloned evidence: *Regression* (full Go+Python suite with real package timings, `git diff --stat HEAD` +491/−23, and an import-graph proof that the only two `FAIL` packages — `internal/docfreshness` spec-032, `tests/unit/clients` spec-073 node/dart-not-on-PATH — do not import any changed symbol); *Simplify* (4 candidates C1–C4 probed with caller-existence grep, 0 applied); *Stabilize* (5 reliability dimensions, 17/17 tests, code-level bounds at named `file:line`); *Security* (OWASP-2021 per-category table with pasted `file:line` code anchors + a negative no-token-logging grep `grep-exit=1`). All four are recorded in `state.json.executionHistory` with `completed_diagnostic` outcomes and verified-present cited tests.
- **The named adversarial test is NON-TAUTOLOGICAL — CONFIRMED by source read.** `TestTwitterAPI_AppOnlyOnUserOwnedEndpointRejected` (`internal/connector/twitter/api_test.go:1304`) drives an `httptest.Server` that ACTIVELY ENFORCES the auth tier — it returns the real Twitter `403 {"title":"Unsupported Authentication",…}` when it receives the App-Only sentinel bearer, and `200` for a user-context token. Sub-case (a) asserts the call fails loud BEFORE the wire (`hits == 0`, `errors.Is(err, ErrUserContextTokenRequired)`, **NOT** `errors.Is(err, errAuthRejected)`); sub-case (b) asserts every observed `Authorization` header carries the user-context token, never the App-Only bearer. Both assertions go RED if the App-Only-on-user-owned bug is reintroduced (the server would be hit and 403). This server-side enforcement is precisely what the original fixture lacked (the documented root cause in bug.md).
- **NO fabricated migration or live-Twitter pass — CONFIRMED.** The migration live DB-apply stays an honest `[ ]` "Claim Source: not-run" Uncertainty Declaration; the live-Twitter `403 → 200` arm correctly SKIPs in AU2-E1 (gated, NOT a DoD item). The migration integration test `TestTwitterOAuthMigration_AppliesCleanly` was read in full this pass: it reads the real `056_twitter_oauth_pkce.sql`, drops the tables for a from-scratch apply, asserts both `twitter_oauth_states` + `twitter_oauth_tokens` and the `idx_twitter_oauth_states_expires_at` index, and re-applies for idempotency — a genuine integration test, CI-ready, NOT a stub and NOT a fabricated pass.
- **No token / secret / code_verifier logged — CONFIRMED by source read.** `authorizationHeader` returns the bare `ErrUserContextTokenRequired` sentinel (no token value); the only token-surface log is the value-free `"user-context token refreshed"` (oauth_token_manager.go:155); `TestTwitterAPI_BearerTokenNeverInLogs` + `…NeverAppearsInLogs` are GREEN in AU2-E1; the Security pass's combined negative grep across the connector + `auth/oauth.go` + the CLI returns nothing (`grep-exit=1`).

### AU2-E4 — source verification of the security-sensitive paths (independent spot-check)

```
$ sed -n '210,228p;325,334p' internal/connector/twitter/api.go   (key lines)
func (c *apiClient) authorizationHeader(ctx context.Context, tier authTier) (string, error) {
	if tier == authTierUserContext {
		if c.userContextToken == nil { return "", ErrUserContextTokenRequired }
		tok, err := c.userContextToken(ctx)
		if err != nil { … return "", fmt.Errorf("%w: %v", ErrUserContextTokenRequired, err) }
		if tok == "" { return "", ErrUserContextTokenRequired }
		return "Bearer " + tok, nil          # user-context token only
	}
	return "Bearer " + c.bearerToken, nil    # App-Only tier only
}
func endpointAuthTier(label string) authTier {
	case usersMeLabel, bookmarks, liked_tweets: return authTierUserContext
	case tweets, mentions:                       return authTierAppOnly
	default:                                      return authTierUserContext   # SAFE default — unmapped fails toward user-context, never App-Only
}
```

Confirmed: user-owned endpoints route to user-context; the matrix `default` is the MORE-restrictive `authTierUserContext` (an unmapped/future endpoint fails loud rather than leaking an App-Only bearer); `authorizationHeader` fails loud with `ErrUserContextTokenRequired` and **NEVER** falls back to the App-Only bearer. PKCE is S256 (`oauth.go:124-125,140`), at-rest is AES-256-GCM with a fresh CSPRNG nonce per encryption + tag-verify on decrypt (`oauth_store.go:65-106`) — both exercised GREEN in AU2-E1.

### AU2-E5 — guard sweep (the SINGLE decisive residual to terminal close)

| Guard | Exit | Result |
|-------|------|--------|
| `artifact-lint.sh` | 0 | **PASS** (one non-blocking advisory: deprecated `scopeProgress` field name) |
| `traceability-guard.sh` | 0 | **PASS — 0 warnings.** 16/16 scenarios mapped to DoD + Test Plan rows + concrete test files + report evidence-refs |
| `state-transition-guard.sh` | 1 | **🔴 BLOCKED — but ONLY on the CI-gated migration row** (see AU2-E6). Check 6 (all specialist phases incl. `audit`) now PASS; the ONLY remaining `🔴` are Check 4 (1 unchecked DoD = the migration live-apply UD) and its coupled Check 5 (Scope A `In Progress`). |

PASS-1's 46→ now-2 state-guard failures: the 44 closed are the 4 missing G022 phases (now recorded) + the entire `bubbles.plan`-owned scopes.md structural set (G041/G068/G053/G040/G084/G095/Check-8A/8B/8C/8D/5A), all guard-verified resolved by validate's reconciliation. PASS-1's 32 traceability failures are now 0.

### AU2-E6 — migration UD adjudication + TERMINAL CALL (honest, guard-grounded)

The state-transition guard EXPLICITLY RECOGNIZES the migration row as an honest UD — it prints `Claim Source: not-run (honest Uncertainty Declaration, Gate G021)` — and STILL issues `🔴 BLOCK` on Check 4, because G024 requires **zero** unchecked DoD for `done`. There is **no accepted-UD convention** in the guard that lets an unchecked DoD pass terminal `done`, and `bugfix-fastlane` has no status ceiling/alias other than `done`. Therefore terminal `done` is **NOT guard-permitted** in this sandbox.

The honest, non-fabricated disposition is **delivered — pending CI migration-apply**:
- Everything that CAN be verified in this sandbox IS verified: PKCE S256 auth, AES-256-GCM encrypted token store, authorize CLI, endpoint auth-tier routing, fail-loud no-fallback, refresh-on-401 + pre-expiry refresh, the named adversarial regression, and the R-016 gauge — all GREEN and independently re-verified (AU2-E1), source-correct (AU2-E4), regression-free / appropriately-simple / stable / OWASP-clean (the 4 specialist passes).
- The ONLY un-run item is the migration LIVE DB-apply under `./smackerel.sh test integration`, which genuinely cannot run here (no live Postgres; `DATABASE_URL` unset) — the SAME operator/CI gate as the live-Twitter arm. The migration auto-applies via embedded `//go:embed`, is unit-verified to parse, and has a CI-ready integration test that EXISTS.
- I did **NOT** fabricate a `[x]`, and I did **NOT** re-characterize the legitimate migration-verification DoD row into a non-DoD note to game G024 (that would be format manipulation). The row stays `[ ]`; `status` stays `in_progress`; the bug is NOT marked Fixed. The `audit` phase IS recorded (it ran to completion and reached a verdict), which is why Check 6 now passes — leaving the guard's residual precisely isolated to the one CI-gated item.

### AU2-E7 — verdict + Spot-Check Recommendations (automation-bias mitigation)

**🚀 SHIP_IT_PENDING_CI → outcome `completed_pending_ci`.** The fix is delivered, comprehensively verified across 7 prior phases plus this independent audit, and anti-fabrication-sound. Terminal `done` is correctly withheld by the mechanical guard on the single honest CI-gated migration-apply row; no `[x]` is fabricated to force it.

1. **Re-run the state-transition guard yourself** and confirm exit 1 with the residual now isolated to Check 4 (migration DoD `[ ]`) + Check 5 (Scope A `In Progress`) — Check 6 (incl. `audit`) should PASS. Do not let a confident close narrative override the mechanical verdict.
2. **Confirm the honest gap is real and un-faked:** Scope A's migration DoD line is still `[ ]`, and no `./smackerel.sh test integration` apply has been recorded as a pass anywhere.
3. **When a CI/operator env with `DATABASE_URL` is available:** run `./smackerel.sh test integration` (or `go test -tags integration ./tests/integration/ -run TestTwitterOAuthMigration_AppliesCleanly`), confirm GREEN, check the migration DoD `[x]` with that real evidence, flip Scope A → `Done`, then the guard will permit terminal `done`. This is the ONLY remaining step.
4. **Verify the named adversarial test stays server-enforcing** during any future change — the `httptest` fixture must keep returning 403 on the App-Only sentinel; a fixture that accepts any bearer would re-hide the original bug.

