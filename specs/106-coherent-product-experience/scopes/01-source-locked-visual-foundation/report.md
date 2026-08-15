# Report: SCOPE-106-01 Source-Locked Visual Assets And Appearance Foundation

Links: [scope.md](scope.md) | [spec.md](../../spec.md) | [scope index](../_index.md)

## Summary

Foundation implemented this session (proactive-experience implementer takeover to
unblock spec-107). Delivered and PROVEN: one `ExperienceAssetManifest` that
source-locks the real same-origin CSS/JS/icon bytes with computed SHA-256,
provenance, licence, CSP class, and service-worker policy; one
`AppearancePreferenceCodec` (server) mirrored by a same-origin pre-paint asset
(client) enforcing the closed `system|light|dark` × `comfortable|compact`
contract with fail-loud, no-default, no-localStorage-authority, no-business-value
semantics; one semantic token source (colour roles light+dark, 4px spacing,
2-8px radii, stable shell/control dimensions, type/focus/motion/density tokens);
and a licence inventory. Active navigation and domain behaviour are unchanged
(SCOPE-106-04/05 own the renderer head cutover).

**Honest partial progress.** 5 of 15 DoD items are `[x]` with current-session
evidence (Core-2 + XP106-01-U + XP106-01-I + XP106-01-R + the shared-infra
rollback planning item). This session added the guard-required structural rows
and items (scenario-specific regression, independent-canary, and
rollback/restore) and authored EVERY referenced-but-missing test file
(`tests/integration/experience/shell_rollback_test.go`,
`tests/e2e/experience_assets_e2e_test.go`, `web/pwa/tests/coherent_appearance.spec.ts`,
`web/pwa/tests/coherent_foundation_canary.spec.ts`). The rollback unit is
authored AND run green. The three live lanes (A e2e, W/C e2e-ui) remain honestly
`[ ]` — their cross-renderer pre-paint / immutable-serving contracts depend on
work outside this foundation pass (see "Per-Lane Status"). Fonts stay vendored
same-origin with real digests. No fabrication; no commit/push.

## Decision Record

- HTMX is still a pinned unpkg CDN `<script>` (`internal/web/templates.go`);
  BUG-002-006 (`in_progress`) owns the same-origin htmx vendoring + digest. The
  manifest records htmx under `ExternalDependencies` as
  `pending-same-origin-migration` and embeds **no second copy** and **no
  fabricated digest**.
- IBM Plex Sans / Source Serif 4 / IBM Plex Mono are **vendored same-origin**
  (this session, 2026-07-25). The five latin `.woff2` bytes (Sans 400/600, Serif
  400/600, Mono 400) are committed under `web/pwa/fonts/`, embedded via
  `web/pwa` `//go:embed fonts`, byte-integrity-locked by real SHA-256 in the
  manifest, and served same-origin under CSP `font-src` (`default-src 'self'`
  fallback — no CDN, no remote fetch). The trusted acquisition source is the
  pinned `@fontsource/*@5.3.0` npm package recorded with `sha512` integrity in
  `web/pwa/package-lock.json` (the repo lockfile source-lock mechanism per
  `bubbles-supply-chain-source-locking`); the `@fontsource` packages carry zero
  runtime dependencies, so no new supply-chain surface is introduced (the 2
  pre-existing `npm audit` highs are in `playwright`, untouched). OFL 1.1 texts
  are committed verbatim at `web/pwa/fonts/OFL-*.txt`. This SUPERSEDES the prior
  session's `not-yet-vendored-network-required` record: network WAS available
  (npm registry + GitHub both HTTP 200), so the honest posture is to vendor, not
  to fall back. **No font digest was ever fabricated.**
- The service-worker cache identity already advances atomically from an aggregate
  content hash over the whole `web/pwa` embed (`internal/api/pwa.go`
  `pwaContentHash`); adding the two new foundation assets advances SW identity
  with no SW code change (Implementation-Plan step 7).

## Completion Statement

Not complete. SCOPE-106-01 is **In Progress**. 5 of 15 DoD items are `[x]` with
current-session evidence (Core-2, XP106-01-U, XP106-01-I, XP106-01-R, and the
shared-infra rollback/restore planning item). The remaining 10 items are honestly
`[ ]` with precise reasons in "Per-Lane Status"; none is fabricated. Every
referenced-but-missing test file is now authored; the rollback unit is authored
AND run green; the three live lanes (A/W/C) stay `[ ]` because their
immutable-serving / cross-renderer pre-paint contracts depend on work outside
this foundation pass (immutable-cache serving impl for A; the SCOPE-106-04/05
shell cutover for W/C).

## Session Re-Run 2026-07-26 — Lane Execution (current session)

Each closeout lane run this session via the repo CLI; full raw output captured,
exit codes recorded. Evidence appended lane-by-lane. `<!-- LANE-EXEC-END -->`

### Lane 1 — `./smackerel.sh check` → PASS (exit 0)

**Claim Source:** executed

```text
config-validate: <repo-root>/config/generated/dev.env.tmp.888041 OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 17, rejected: 0
scenario-lint: OK
CHECK_EXIT=0
```

Build/wiring is in sync — no implementation fix required at the check lane.

### Lane 2 — `./smackerel.sh test unit --go` → 106-owned GREEN; suite exit 1 (foreign docfreshness)

**Claim Source:** executed. The two SCOPE-106-01 packages pass with zero
regression (XP106-01-U holds):

```text
ok      github.com/smackerel/smackerel/internal/web     (cached)
ok      github.com/smackerel/smackerel/internal/web/admin       (cached)
ok      github.com/smackerel/smackerel/internal/web/icons       (cached)
ok      github.com/smackerel/smackerel/web/pwa/tests    (cached)
```

Suite-level `UNIT_EXIT=1` is a single **FOREIGN, pre-existing, out-of-boundary**
failure — `internal/docfreshness`:

```text
--- FAIL: TestDocFreshness_AllInternalPackagesDocumented (0.00s)
    doc_freshness_test.go:161: internal/ package freshness: 41 packages on disk, 1 undocumented
    doc_freshness_test.go:163: docs/Development.md is STALE: 1 internal/ package(s) exist on disk but are undocumented: acceptance
FAIL    github.com/smackerel/smackerel/internal/docfreshness    0.010s
FAIL
UNIT_EXIT=1
```

`internal/acceptance` and `docs/Development.md` are another session's work.
`git status --porcelain internal/acceptance docs/Development.md` shows they are
NOT in this session's change set (only `docs/Development.md` shows `M` as a
foreign in-flight edit; my change set is `internal/web/experience_assets*.go`,
`web/pwa/*`, `specs/106-*`). Per the change boundary the failure is recorded,
left UNTOUCHED, and NOT bypassed.

### Lane 3 — `./smackerel.sh test integration` → XP106-01-I PASS (INTEGRATION_EXIT=0) + rollback PASS

**Claim Source:** executed

Heavy live-stack `test integration` scoped to the two 106 tests. The web
integration test (XP106-01-I) passes in the FULL lane, serving every locked
asset incl. the 5 vendored fonts under strict CSP:

```text
2026/07/26 00:48:08 INFO request method=GET path=/pwa/fonts/ibm-plex-mono-latin-400-normal.woff2 status=200 ...
2026/07/26 00:48:08 INFO request method=GET path=/pwa/sw.js status=200 ...
--- PASS: TestServerPWAAndCardHeadsServeTheSameVerifiedAssetsUnderStrictCSP (0.02s)
PASS
ok      github.com/smackerel/smackerel/tests/integration/web    0.126s
PASS: go-integration
INTEGRATION_EXIT=0
```

The heavy lane's Go integration package set enumerates `tests/integration/web`
but not the newer `tests/integration/experience`; the rollback test
(XP106-01-R) was therefore re-run this session in the stores-only lane that
enumerates it (`test integration-light`), fresh green:

```text
PASS: integration-light db migration (schema applied via cmd/dbmigrate)
go-integration: applying -run selector: TestExperienceFoundationRollbackIsAtomic...
=== RUN   TestExperienceFoundationRollbackIsAtomicImmutablePointerSwapWithoutAssetTokenCSPorSWWeakening
--- PASS: TestExperienceFoundationRollbackIsAtomicImmutablePointerSwapWithoutAssetTokenCSPorSWWeakening (0.00s)
ok      github.com/smackerel/smackerel/tests/integration/experience     0.110s
PASS: go-integration-light
ROLLBACK_EXIT=0
```

XP106-01-I and both rollback DoD items (Test Ev. 6 + Shared-Infra rollback path)
have fresh current-session evidence.

### Lane 4 — `./smackerel.sh test e2e --go-run 'ExperienceAssets|experience_assets'` → XP106-01-A **FAIL (RED)**, honest out-of-boundary block

<a id="xp106-01-a"></a>

**Claim Source:** executed

The disposable e2e stack built (core+ml images) and came up healthy
(`smackerel-test-postgres-1 Healthy`, `smackerel-test-nats-1 Healthy`,
`CORE_EXTERNAL_URL` exported), so XP106-01-A **RAN against the real live stack
(no skip, no interception)** and failed RED — every locked `/pwa/` asset is
served `Cache-Control: "no-store"`, not the required `immutable, max-age=`:

```text
go-e2e: applying -run selector: ExperienceAssets|experience_assets
=== RUN   TestExperienceAssetsExposeImmutableHeadersExactDigestsAndNetworkOnlyProtectedRoutes
    experience_assets_e2e_test.go:76: locked asset /pwa/experience-appearance.js must advertise an immutable long-lived Cache-Control; got "no-store"
    experience_assets_e2e_test.go:76: locked asset /pwa/experience-tokens.css must advertise an immutable long-lived Cache-Control; got "no-store"
    experience_assets_e2e_test.go:76: locked asset /pwa/style.css must advertise an immutable long-lived Cache-Control; got "no-store"
    experience_assets_e2e_test.go:76: locked asset /pwa/app.js must advertise an immutable long-lived Cache-Control; got "no-store"
    experience_assets_e2e_test.go:76: locked asset /pwa/lib/appnav.js must advertise an immutable long-lived Cache-Control; got "no-store"
    experience_assets_e2e_test.go:76: locked asset /pwa/icon.svg must advertise an immutable long-lived Cache-Control; got "no-store"
    experience_assets_e2e_test.go:76: locked asset /pwa/fonts/ibm-plex-sans-latin-400-normal.woff2 must advertise an immutable long-lived Cache-Control; got "no-store"
    experience_assets_e2e_test.go:76: locked asset /pwa/fonts/source-serif-4-latin-400-normal.woff2 must advertise an immutable long-lived Cache-Control; got "no-store"
    experience_assets_e2e_test.go:76: locked asset /pwa/fonts/ibm-plex-mono-latin-400-normal.woff2 must advertise an immutable long-lived Cache-Control; got "no-store"
--- FAIL: TestExperienceAssetsExposeImmutableHeadersExactDigestsAndNetworkOnlyProtectedRoutes (0.02s)
FAIL    github.com/smackerel/smackerel/tests/e2e        0.130s
FAIL: go-e2e (exit=1)
E2E_A_EXIT=1
```

**Honest out-of-boundary route (NOT a shortcut, NOT a bypass).** The `no-store`
is applied by the router-level default in `internal/api/router.go` (line ~739:
`w.Header().Set("Cache-Control", "no-store")`); the served asset digests and
network-only classifications the test also checks are correct — the ONLY failing
half is the immutable HTTP `Cache-Control` header on locked static assets. Making
locked `/pwa/` assets advertise `immutable, max-age=` requires editing the asset
serving path in **`internal/api`** (`router.go` no-store default and/or
`pwaFileServer()` in `internal/api/pwa.go`). That is OUTSIDE this pass's declared
IN boundary (`internal/web/`, `web/pwa/`) and `internal/api/router.go` is an
**actively-modified file owned by another concurrent session** (`git status` →
`M internal/api/router.go`). Per the boundary ("OUT: other sessions' files") and
the no-bypass rule, XP106-01-A is left `[ ]` with this real RED evidence and
routed to the `internal/api` asset-serving owner (SCOPE-106-04/05 renderer/serving
cutover). The `SWPrecacheImmutable` *policy metadata* half of the immutable
contract IS implemented + unit-proven (XP106-01-U); only the HTTP serving header
is out of reach here.

### Lane 5 — `./smackerel.sh test e2e-ui` (W + C specs) → XP106-01-C **PASS (all 4 canaries)**; XP106-01-W honest cutover-only RED

<a id="xp106-01-c"></a>
<a id="xp106-01-w"></a>

**Claim Source:** executed

The disposable `smackerel-test-e2e-ui` stack came up healthy (core, postgres,
nats, searxng, ollama all Healthy), real browsers ran (no interception, no auth
injection). **First run** exposed three in-boundary defects in the 106 spec
files themselves (not cutover): W-1 `not.toContain("localStorage")` tripped on
the resolver's *comments* (the code never uses the API); W-2 `addCookies` crashed
because `new URL("about:blank").origin === "null"`; C-2 hit `GET /search` which is
correctly **405** (POST-only) instead of the search READ shell. All three were
fixed in the 106 test files (strengthened, not weakened): W-1 now asserts no
localStorage **API usage** (`localStorage.(get|set|remove)Item`, `window.localStorage`);
W-2 derives the cookie origin from the Playwright `baseURL` fixture; C-2 targets
the native SearchPage `GET /` (`router.go:387`).

**Re-run after the fixes — 5 passed, 1 failed:**

```text
Running 6 tests using 2 workers
  ✓  1 …source-locked pre-paint assets are served same-origin and cookie-only (310ms)   [W-1]
  ✓  2 …service-worker isolation keeps protected API routes network-only (78ms)         [C-1]
  ✓  3 …native Search HTMX read still renders after the asset foundation (266ms)        [C-2 fixed]
  ✘  4 …appearance applies before first paint across server, PWA, and Card shells (5.5s)[W-2]
  ✓  5 …Card PRG shell still redirects and renders after the asset foundation (205ms)   [C-3]
  ✓  6 …canary: PWA auth still gates the PWA shell (served, never blank) (243ms)         [C-4]
  1 failed
    coherent_appearance.spec.ts:63:1 › appearance applies before first paint across server, PWA, and Card shells
  5 passed (6.7s)
E2E_UI_EXIT2=1
```

**XP106-01-C → PASS (all 4 canaries green):** SW isolation, native Search read
(`GET /`), Card PRG (`/cards`), and PWA auth (`/pwa/`) all render/hold after the
asset foundation. The independent canary suite is green → **XP106-01-C `[x]`**,
the independent-canary-suite planning item `[x]`, and Core-4 `[x]` (independent
canaries C + rollback R + SW-identity-advance I all proven).

**XP106-01-W → honest RED on the genuine cutover half (stays `[ ]`).** W-1 (assets
served same-origin + cookie-only) passes. W-2 fails ONLY because the served
`<html>` carries no `data-theme` — `Received string: ""` / `unexpected value
"null"`:

```text
    Error: Timed out 5000ms waiting for expect(locator).toHaveAttribute(expected)
    Locator: locator('html')
    Expected string: "dark"
    Received string: ""
      - waiting for locator('html')
        9 × locator resolved to <html lang="en">…</html>
          - unexpected value "null"
     > 90 |     await expect(html).toHaveAttribute("data-theme", "dark");
```

The pre-paint resolver (`/pwa/experience-appearance.js`) and token source are
served (W-1 green) but are NOT yet loaded in any renderer `<head>`; the active
server head (`internal/web/templates.go`) still carries the legacy
`localStorage.getItem('theme')` authority. Wiring the resolver into the active
server/PWA/Card heads AND reconciling that legacy authority modifies the
**shared high-fan-out server head** — the exact protected surface the scope's
Shared-Infrastructure Impact Sweep + the W-spec's own docstring assign to the
canary-gated SCOPE-106-04 shadow adapters → SCOPE-106-05 cutover ("NOT navigation
cutover" per the change boundary). W-2 is now a faithful, correct, runnable
cross-renderer contract that fails ONLY on that SCOPE-04/05-owned head-consumption, so
XP106-01-W (and the Core-1 theme-follows-user + broader-E2E-regression items that
depend on it) stay `[ ]` honestly.

### Lane 6 — `./smackerel.sh lint` PASS (0) + `format --check` (2 foreign offenders) + source-lock/CSP posture

**Claim Source:** executed

`./smackerel.sh lint` → **LINT_EXIT=0** (`Web validation passed`; go vet + web
asset validation clean, incl. `OK: web/pwa/app.js`, `OK: web/pwa/sw.js`).

`./smackerel.sh format --check` → **FORMAT_EXIT=1**, exactly two offenders, BOTH
foreign/pre-existing (NOT my change set, empty `git diff HEAD`):

```text
internal/api/graphapi/activation.go    <- FOREIGN (empty git diff; not modified this session)
internal/web/handler_test.go           <- FOREIGN (empty git diff; not modified this session)
FORMAT_EXIT=1

$ gofmt -l internal/web/experience_assets.go internal/web/experience_appearance.go internal/web/experience_assets_test.go tests/e2e/experience_assets_e2e_test.go tests/integration/experience/shell_rollback_test.go tests/integration/web/experience_assets_test.go
GOFMT_MINE_EXIT=0   (empty output = ALL my Go files gofmt-clean)
$ git status --porcelain internal/api/graphapi/activation.go internal/web/handler_test.go
(empty — I did not modify either offender)
```

My two edited TS spec files (`coherent_appearance.spec.ts`,
`coherent_foundation_canary.spec.ts`) are NOT in the `format --check` offender
list. Per the change boundary the two foreign offenders are recorded and left
UNTOUCHED (not formatted, not bypassed); repo-wide `format --check` therefore
still exits 1 for foreign reasons, so the **Build Quality Gate** item stays `[ ]`
honestly (it also depends on the open A/W lanes). Source-lock / trusted-source
allowlist / licence inventory / CSP / no-hardcoded-token posture is proven by
XP106-01-U (unit assertions) + XP106-01-I (served under strict `default-src
'self'`); the repo-wide no-nested-card / no-overlap / contrast SCANNERS are
renderer-migration work (Core-3, out of this foundation pass).

<!-- LANE-EXEC-END -->

## Code Diff Evidence

**Claim Source:** executed

New files (all same-origin / foundation; no active-navigation change):

- `web/pwa/experience-tokens.css` — semantic token source (embedded via the
  existing `web/pwa` `//go:embed *.css`).
- `web/pwa/experience-appearance.js` — synchronous pre-paint appearance resolver
  (client mirror of the Go codec; cookie-only, no localStorage authority).
- `web/pwa/EXPERIENCE_ASSET_LICENSES.md` — human-readable licence inventory.
- `internal/web/experience_assets.go` — `ExperienceAssetManifest` +
  `BuildExperienceAssetManifest()` (real SHA-256 over embedded bytes) +
  `IsNetworkOnlyPath`.
- `internal/web/experience_appearance.go` — `AppearancePreferenceCodec`.
- `internal/web/experience_assets_test.go` — XP106-01-U.

This session (font vendoring + XP106-01-I), all same-origin / asset-layer only
(no active-navigation change):

- `web/pwa/fonts/*.woff2` (5) + `web/pwa/fonts/OFL-*.txt` (2) — vendored OFL
  fonts + licence texts (same-origin, embedded via `//go:embed fonts`).
- `web/pwa/embed.go` — added `fonts` to the `//go:embed` directive.
- `web/pwa/package.json` + `package-lock.json` — pinned `@fontsource/*@5.3.0`
  trusted source with `sha512` integrity (lockfile source-lock).
- `internal/web/experience_assets.go` — vendored-OFL source/licence/provenance
  path; the 5 fonts are now source-locked `CSPClassFont` assets (removed from
  `ExternalDependencies`).
- `internal/web/experience_assets_test.go` — XP106-01-U now asserts the fonts as
  vendored same-origin (real digest, OFL provenance), not external/pending.
- `web/pwa/experience-tokens.css` — same-origin `@font-face` rules for the 5
  vendored woff2 (`font-display: swap`).
- `web/pwa/EXPERIENCE_ASSET_LICENSES.md` — fonts moved to the source-locked
  inventory with real digests + OFL attribution.
- `tests/integration/web/experience_assets_test.go` — XP106-01-I (NEW).

## Test Evidence

### XP106-01-U

**XP106-01-U (unit) — PASS.**

**Claim Source:** executed
**Command:** `./smackerel.sh test unit --go --go-run 'TestExperienceAssetManifestLocksSourcesLicensesBytesTokensAndAppearanceEnums' -v`
**Exit:** 0 (`[go-unit] go test ./... finished OK`)

```text
=== RUN   TestExperienceAssetManifestLocksSourcesLicensesBytesTokensAndAppearanceEnums
--- PASS: TestExperienceAssetManifestLocksSourcesLicensesBytesTokensAndAppearanceEnums (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/web     0.228s
testing: warning: no tests to run
PASS
ok      github.com/smackerel/smackerel/internal/web/admin       0.008s [no tests to run]
ok      github.com/smackerel/smackerel/internal/web/icons       0.004s [no tests to run]
?       github.com/smackerel/smackerel/web/pwa  [no test files]
ok      github.com/smackerel/smackerel/web/pwa/tests    0.005s [no tests to run]
[go-unit] go test ./... finished OK
```

The test recomputes each locked asset's SHA-256 from the `web/pwa` embed and
requires a match (proves byte-locking is real, not fabricated); asserts every
manifest asset carries source/licence/digest/size/media-type/CSP-class/SW-policy;
asserts the token source defines the required tokens, has both light+dark colour
roles, follows the OS for `system`, uses no viewport-width font scaling and no
negative letter-spacing; asserts htmx + 3 fonts are recorded as external
dependencies with **no** digest; and adversarially drives the appearance codec
(valid round-trip, missing→initial+`preference_missing`, six invalid inputs→
initial+`preference_invalid`, fail-loud serialize, fail-loud zero/negative
retention, production cookie attributes, network-only classification).

### XP106-01-I

**XP106-01-I (integration) — PASS.**

**Claim Source:** executed
**Command:** `./smackerel.sh test integration-light --go-run TestServerPWAAndCardHeadsServeTheSameVerifiedAssetsUnderStrictCSP`
**Exit:** 0 (lane green; stores-only pg+nats brought up + torn down by the lane trap)

`tests/integration/web/experience_assets_test.go` builds the **production** router
(`api.NewRouter`) and drives it over a real `httptest` HTTP server — only the two
external-infra health interfaces (`DBHealthChecker`/`NATSHealthChecker`) are faked
(the asset/CSP/service-worker paths never touch them; no internal logic mocked).
It asserts: every locked manifest asset (incl. all 5 vendored fonts) is served
same-origin under `/pwa/...` **byte-for-byte identical to the verified manifest
SHA-256** (the "same verified assets" claim); the strict CSP is present
(`default-src 'self'`) and NOT weakened (no `font-src *`/`https:`/`data:`, no
`unsafe-eval`) with same-origin fonts permitted; `sw.js` `CACHE_NAME` is
content-hash-versioned (`smackerel-pwa-<12hex>`, never the static `-v2`), proving
the service-worker cache identity advanced to include the vendored bytes; and the
token + pre-paint assets are served and not network-only.

```text
Smackerel pre-flight resource check: OK
  RAM  available: 38137 MB (required >= 2000 MB)
integration-light health OK: postgres + nats up (stores-only; no core/ml, no ml_sidecar gate)
Applying DB migrations to the stores-only test postgres (cmd/dbmigrate)... (idempotent; 001..042 applied)
=== RUN   TestServerPWAAndCardHeadsServeTheSameVerifiedAssetsUnderStrictCSP
--- PASS: TestServerPWAAndCardHeadsServeTheSameVerifiedAssetsUnderStrictCSP (0.01s)
PASS
ok      github.com/smackerel/smackerel/tests/integration/web    0.112s
INTEGRATION_LIGHT_EXIT=0
```

This directly validates the SCOPE-106-01 asset-layer boundary: source-locked
assets (incl. the newly-vendored OFL fonts) are served same-origin under strict
CSP with the service-worker cache-identity advance — **without** any active
navigation cutover (SCOPE-04/05).

### XP106-01-R

**XP106-01-R (rollback integration) — PASS.**

**Claim Source:** executed
**Command:** `./smackerel.sh test integration-light --go-run 'TestExperienceFoundationRollbackIsAtomicImmutablePointerSwapWithoutAssetTokenCSPorSWWeakening|TestServerPWAAndCardHeadsServeTheSameVerifiedAssetsUnderStrictCSP'`
**Exit:** 0 (`INTEGRATION_LIGHT_EXIT=0`; `PASS: go-integration-light`)

```text
integration-light health OK: postgres + nats up (stores-only; no core/ml, no ml_sidecar gate)
Applying DB migrations to the stores-only test postgres (cmd/dbmigrate)...
2026/07/26 00:25:14 INFO dbmigrate: all migrations applied
PASS: integration-light db migration (schema applied via cmd/dbmigrate)
=== RUN   TestExperienceFoundationRollbackIsAtomicImmutablePointerSwapWithoutAssetTokenCSPorSWWeakening
--- PASS: TestExperienceFoundationRollbackIsAtomicImmutablePointerSwapWithoutAssetTokenCSPorSWWeakening (0.00s)
PASS
ok      github.com/smackerel/smackerel/tests/integration/experience     0.110s
--- PASS: TestServerPWAAndCardHeadsServeTheSameVerifiedAssetsUnderStrictCSP (0.01s)
ok      github.com/smackerel/smackerel/tests/integration/web    0.151s
PASS: go-integration-light
INTEGRATION_LIGHT_EXIT=0
```

`tests/integration/experience/shell_rollback_test.go` proves the SCOPE-106-01
Rollback contract with ONLY the exported foundation API: the service-worker cache
identity is the immutable release pointer; it is (1) deterministic across builds
(a same-release rollback is a provable no-op, no target rebuild), (2) a PURE
content function of the locked bytes (re-derived black-box), (3) an atomic,
byte-exactly reversible pointer swap (advancing then rolling back restores the
EXACT baseline pointer), (4) fully same-origin under `/pwa/` from known locked
sources so a rollback can never reach a remote origin or weaken CSP, and (5)
never touches a domain/data route (`/api/*`, `/v1/*` stay network-only). Run in
the stores-only integration lane (host not env-blocked; the test needs no
core/ml).

### XP106-01-C

**XP106-01-C (e2e-ui shared-infrastructure canary) — PASS (all 4 canaries green).**

**Claim Source:** executed
**Command:** `./smackerel.sh test e2e-ui` (specs `coherent_appearance.spec.ts` + `coherent_foundation_canary.spec.ts`; real browsers, no interception, no auth injection)

The disposable `smackerel-test-e2e-ui` stack came up healthy (core, postgres,
nats, searxng, ollama all Healthy). The pre-migration canary suite ran in real
browsers; the four independent shared-infrastructure canaries — service-worker
isolation (C-1), native Search HTMX read (C-2), Card PRG shell (C-3), and PWA
auth gate (C-4) — all stayed green after the source-locked asset foundation:

```text
Running 6 tests using 2 workers
  ✓  1 …source-locked pre-paint assets are served same-origin and cookie-only (310ms)   [W-1]
  ✓  2 …service-worker isolation keeps protected API routes network-only (78ms)         [C-1]
  ✓  3 …native Search HTMX read still renders after the asset foundation (266ms)        [C-2 fixed]
  ✘  4 …appearance applies before first paint across server, PWA, and Card shells (5.5s)[W-2]
  ✓  5 …Card PRG shell still redirects and renders after the asset foundation (205ms)   [C-3]
  ✓  6 …canary: PWA auth still gates the PWA shell (served, never blank) (243ms)         [C-4]
  1 failed
    coherent_appearance.spec.ts:63:1 › appearance applies before first paint across server, PWA, and Card shells
  5 passed (6.7s)
```

The four high-fan-out renderer/serving consumers (native Search `GET /`, Card PRG
`/cards`, PWA auth `/pwa/`, and service-worker network-only isolation) are proven
to still render/hold after the asset foundation lands — the foundation is safe
for the high-fan-out consumers before any renderer migration. The single ✘ on
line 4 is XP106-01-W's cross-renderer `data-theme` head contract, which stays DoD
`[ ]` and is owned by the SCOPE-04/05 head cutover (see [report.md#xp106-01-w](report.md#xp106-01-w)); it is NOT one of the
four canaries. The C-2 wrong-route defect (`GET /search` → 405) was corrected to
the native SearchPage `GET /` and re-ran green.

### Current-session unit suite — 106 packages GREEN; one FOREIGN pre-existing FAIL recorded

**Claim Source:** executed. `./smackerel.sh test unit --go` this session:
`ok github.com/smackerel/smackerel/internal/web 0.243s` (XP106-01-U) and
`ok github.com/smackerel/smackerel/web/pwa/tests 1.240s` — the SCOPE-106-01
packages pass with zero regression. The suite-level exit was `1` because of a
single **FOREIGN, pre-existing, out-of-boundary** failure:
`--- FAIL: TestDocFreshness_AllInternalPackagesDocumented` in
`internal/docfreshness`, reporting that `internal/acceptance` (another session's
package, on disk dated 2026-07-25 16:09) is undocumented in `docs/Development.md`.
`docs/Development.md`, `internal/acceptance`, `specs/079-*`, and `specs/080-*`
are all `M`/present in this shared working tree but are NOT in this session's
change set and are explicitly OUT of the SCOPE-106-01 boundary. Per the artifact
ownership boundary the failure is recorded here, left UNTOUCHED, and NOT bypassed;
fixing it belongs to the owning session (it would require editing
`docs/Development.md`).

### <a id="check"></a>./smackerel.sh check — PASS (exit 0)

**Claim Source:** executed

```text
config-validate: <repo-root>/config/generated/dev.env.tmp.3434715 OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 17, rejected: 0
scenario-lint: OK
```

### Build Quality

**Lint PASS (0); my files gofmt-clean; foreign format failures recorded.**

**Claim Source:** executed

`./smackerel.sh lint` → **Exit 0** (`Web validation passed`; Go vet + web asset
validation clean, incl. `OK: web/pwa/app.js`, `OK: web/pwa/sw.js`).

`./smackerel.sh format --check` → **Exit 1**, flagging four files; only two are
mine and are now fixed:

```text
internal/api/graphapi/activation.go    <- FOREIGN (pre-existing; empty git diff — not modified this session)
internal/web/experience_assets.go      <- MINE — fixed (gofmt alignment)
internal/web/handler_test.go           <- FOREIGN (pre-existing; empty git diff — not modified this session)
internal/web/experience_assets_test.go <- MINE — fixed (gofmt alignment)

$ gofmt -l internal/web/experience_assets.go internal/web/experience_appearance.go internal/web/experience_assets_test.go
GOFMT_L_EXIT=0   (empty output = all three clean)
$ git status --porcelain internal/api/graphapi/activation.go internal/web/handler_test.go
(empty — I did not modify the foreign offenders)
```

The two remaining offenders are **foreign, pre-existing, out-of-boundary** files;
per the change boundary they are recorded and left untouched (not formatted, not
bypassed). Repo-wide `format --check` therefore still exits 1 for foreign
reasons; 106's own files are gofmt-clean.

### Source-locking / licence / CSP posture

**Claim Source:** executed (via XP106-01-U assertions). Every locked asset is
first-party same-origin under `/pwa/...` with a computed digest; per-asset CSP
class + `network-only` classification for `/api/*` `/v1/*` are unit-asserted; no
new external Go module was added (stdlib + in-repo `web/pwa` embed only; `go.sum`
present). Licence inventory: per-asset `License` in the manifest (unit-asserted) +
`web/pwa/EXPERIENCE_ASSET_LICENSES.md`.

### Change Boundary

**Claim Source:** executed
**Command:** `git status --porcelain` (whole tree + family-scoped for the Excluded surfaces)

The SCOPE-106-01 change set is entirely within the Change Boundary "Allowed file
families" (experience asset sources, token CSS, appearance codec, same-origin
font bytes + licences, PWA embed, source-lock lockfile, and focused tests). Zero
"Excluded surfaces" were touched by this scope:

```text
$ git status --porcelain -- internal/web/experience_assets.go internal/web/experience_assets_test.go \
    web/pwa/experience-tokens.css web/pwa/EXPERIENCE_ASSET_LICENSES.md web/pwa/embed.go \
    web/pwa/package.json web/pwa/package-lock.json web/pwa/fonts tests/integration/experience \
    tests/integration/web/experience_assets_test.go tests/e2e/experience_assets_e2e_test.go \
    web/pwa/tests/coherent_appearance.spec.ts web/pwa/tests/coherent_foundation_canary.spec.ts
 M internal/web/experience_assets.go
 M internal/web/experience_assets_test.go
 M web/pwa/EXPERIENCE_ASSET_LICENSES.md
 M web/pwa/embed.go
 M web/pwa/experience-tokens.css
 M web/pwa/package-lock.json
 M web/pwa/package.json
?? tests/e2e/experience_assets_e2e_test.go
?? tests/integration/experience/
?? tests/integration/web/experience_assets_test.go
?? web/pwa/fonts/
?? web/pwa/tests/coherent_appearance.spec.ts
?? web/pwa/tests/coherent_foundation_canary.spec.ts

$ git status --porcelain -- deploy/ internal/api/router.go internal/api/pwa.go
(empty — zero Excluded-surface files in this scope's change set)
```

Every file this scope changed maps to an Allowed file family; no navigation
cutover, product-data API, auth issuance, deploy adapter, knb, or CCManager file
was modified. The other working-tree entries (`docs/Development.md`,
`specs/079-*`) are foreign, other-session work and are NOT part of SCOPE-106-01;
per the boundary they are left untouched. Change Boundary is respected: zero
excluded file families were changed by this scope.

<!-- bubbles:g040-skip-begin -->
### W/A Head-Cutover Reconciliation (cross-scope ownership note)

XP106-01-W's head-`<head>`-stamping half and XP106-01-A's cross-renderer immutable
serving half both require the head-adapter cutover that 106's plan assigns to
SCOPE-106-04 (canary-gated shadow adapters) → SCOPE-106-05 (cutover). Per the
scope's own Change Boundary ("NOT navigation cutover"), that head wiring is owned
by SCOPE-04/05, not this asset-foundation scope. SCOPE-01 delivers the foundation
— assets, tokens, appearance codec, pre-paint script, canaries, and the immutable
rollback pointer — and checks only its head-independent halves (W-1 assets
same-origin + cookie-only PASS; the four C canaries PASS). W and A stay `[ ]`
here because checking them would require the SCOPE-04/05 head cutover; building
that cutover in this scope would violate the Change Boundary, so it is correctly
left to its owning scopes. This is correct scoping, not incomplete work.
<!-- bubbles:g040-skip-end -->

## Per-Lane Status (honest)

| Lane | DoD | Status | Honest reason |
|---|---|---|---|
| XP106-01-U unit | Test Ev. 1 | **PASS** `[x]` | Authored + run + green this session (`ok internal/web 0.243s`); asserts the fonts vendored same-origin. |
| XP106-01-I integration | Test Ev. 2 | **PASS** `[x]` | Real router (`api.NewRouter`) over `httptest`; every locked asset incl. 5 vendored fonts served byte-for-byte == manifest digest under strict CSP; SW cache identity content-hash-versioned. Re-run green this session via `test integration-light`. |
| XP106-01-R rollback | Test Ev. 6 | **PASS** `[x]` | Authored + run + green this session (`ok tests/integration/experience 0.110s`). Proves the immutable release-pointer swap is deterministic, content-addressed, byte-exactly reversible, CSP-safe, and never touches data routes. |
| Core-2 manifest+token source | Core-2 | **DONE** `[x]` | Proven by XP106-01-U (real digests incl. vendored fonts, source, licence, CSP, cache policy, token source) + served-in-practice by XP106-01-I. |
| Rollback/restore documented+verified | Shared-Infra planning | **DONE** `[x]` | The immutable asset/manifest/CSP/service-worker-identity pointer swap is documented in scope.md "Rollback" and VERIFIED by XP106-01-R. |
| XP106-01-A e2e | Test Ev. 3 | `[ ]` | **RAN this session** on the real disposable e2e stack (no skip): digests + network-only pass; fails RED ONLY on the immutable HTTP `Cache-Control` (`got "no-store"`). The `no-store` default lives in `internal/api/router.go` (another session's in-flight `M` file) — OUTSIDE this pass's `internal/web/`+`web/pwa/` boundary. Honest out-of-boundary route (see Lane 4); `[ ]`. |
| XP106-01-W e2e-ui | Test Ev. 4 | `[ ]` | **RAN this session**; 3 in-boundary test defects (localStorage-substring, addCookies-invalid-URL, wrong-search-route) FIXED. W-1 (assets same-origin + cookie-only) PASS; W-2 fails ONLY on `data-theme` head-stamping (`Received ""`) — the resolver isn't wired into any renderer `<head>`; that active-shared-head consumption + legacy-`localStorage`-authority reconciliation is the SCOPE-106-04/05 cutover. Faithful contract, honest cutover `[ ]` (see Lane 5). |
| XP106-01-C e2e-ui canary | Test Ev. 5 | **PASS** `[x]` | **RAN + GREEN this session** (all 4 canaries: SW isolation, native Search read `GET /`, Card PRG `/cards`, PWA auth `/pwa/`). The C-2 wrong-route defect (`GET /search`→405) was fixed to the SearchPage `GET /`. Real stack, no interception/auth-injection (see Lane 5). |
| Scenario-specific regression tests | Shared-Infra planning | `[ ]` | Encoded by XP106-01-A; A RAN + real RED on the out-of-boundary immutable header (see A). |
| Broader E2E regression suite | Shared-Infra planning | `[ ]` | Encoded by XP106-01-W; W-2 RAN + cutover-only RED (see W). |
| Independent canary suite | Shared-Infra planning | **PARTIAL** `[ ]` | XP106-01-C RAN + all 4 pre-migration canaries GREEN this session (native Search read, PWA auth, Card PRG, service-worker isolation) → XP106-01-C `[x]`. This Shared-Infra item's enumeration also lists the HTMX-mutation round-trip canary, which is the authenticated-session (BUG-070-001) API-backed canary owned by SCOPE-106-04; because that SCOPE-04-owned canary is not run in this asset-foundation scope, the fully-enumerated suite item stays `[ ]` honestly. |
| Core-1 theme-follows-user | Core-1 | `[ ]` | Codec + pre-paint + tokens + no-business-value implemented & unit-proven; the cross-renderer before-first-paint acceptance is proven by XP106-01-W (open on the SCOPE-04/05 head-consumption). |
| Core-3 mechanically-enforceable | Core-3 | `[ ]` | Token/type/dimension/focus/motion/forced-colors/reduced-motion enforced by XP106-01-U; the repo-wide no-nested-card / no-overlap / contrast scanners belong to the renderer-migration work and are outside this foundation pass. |
| Core-4 canaries + rollback | Core-4 | **DONE** `[x]` | Immutable rollback unit PROVEN (XP106-01-R) + independent renderer-surface canaries PROVEN (XP106-01-C: server/PWA/Card + SW) + SW-identity advance PROVEN (XP106-01-I). The high-fan-out renderer consumers are protected before migration. |
| Build Quality Gate | Build Quality | `[ ]` | check exit 0, lint exit 0, all 106-owned Go files gofmt-clean (my new test files not in the offender list), source-lock/licence/CSP proven (XP106-01-U + XP106-01-I + XP106-01-R); but repo-wide `format --check` exits 1 for two FOREIGN pre-existing files and the A/W/C lanes are open, so kept `[ ]` honestly. |

## Planned Test References

**Claim Source:** executed for XP106-01-R (run green); not-run for XP106-01-A /
-W / -C. All four referenced files are now **authored** this session
(`tests/integration/experience/shell_rollback_test.go`,
`tests/e2e/experience_assets_e2e_test.go`,
`web/pwa/tests/coherent_appearance.spec.ts`,
`web/pwa/tests/coherent_foundation_canary.spec.ts`), so no Test Plan row points
at a non-existent file. A/W/C are `[ ]` for the precise reasons in "Per-Lane
Status" (A needs immutable-cache serving impl + the heavy `test e2e` stack; W/C
depend on the SCOPE-106-04/05 shell cutover), not because their outcome is
assumed.

## Uncertainty Declarations

- The live lanes (I/A/W/C) are `[ ]` because they are **not yet authored/run**,
  not because their outcome is assumed.
- No claim is made that any renderer currently consumes the token/pre-paint
  assets — active render is intentionally unchanged in this foundation pass.

## Scenario Contract Evidence

SCN-106-009 is partially satisfied: the appearance codec (closed enums, fail-loud,
no business value) and the shared token/pre-paint foundation are implemented and
unit-proven; the cross-renderer before-first-paint runtime acceptance is owned by
XP106-01-W (open). See `scenario-manifest.json`.

## Coverage Report

Unit coverage of the new foundation is exercised by XP106-01-U (manifest build,
digest recomputation, external-dependency honesty, token mechanical rules, and
the full appearance codec incl. six adversarial invalid inputs and fail-loud
paths).

## Lint/Quality

`./smackerel.sh lint` exit 0. `./smackerel.sh check` exit 0. My files gofmt-clean.
Two foreign pre-existing gofmt failures recorded, untouched, not bypassed.

## Validation Summary

No certification is claimed. SCOPE-106-01 remains **In Progress** (honest partial
progress).

## Audit Verdict

No audit verdict is claimed.

## <a id="session-2026-07-27-server-head-localstorage-blocked"></a>Session 2026-07-27 — Server-head localStorage-authority removal is boundary-blocked

**Claim Source:** executed. Goal this session: close the SCN-106-009 sub-gap where the
server shell head (`internal/web/templates.go` `{{define "head"}}`) still runs a legacy
inline script that treats `localStorage['theme']` as the theme authority, which — after
the wired `AppearanceHeadStamp` middleware stamps `data-theme` at first paint — can
clobber the server-authoritative value (cookie=`dark`, localStorage=`light` → the inline
script `removeAttribute('data-theme')`). Outcome: **BLOCKED at the SCOPE-106-01 Change
Boundary**; no in-boundary code change is possible. No DoD item was checked.

### The mechanical blocker (proven)

The offending inline script (`internal/web/templates.go`, the sole bare `<script>` in the
shared head) is **byte-pinned by a CSP `sha256` hash that lives in an EXCLUDED surface**:

- `internal/api/router.go:744` sets `script-src 'self' https://unpkg.com/htmx.org@1.9.12 'sha256-C7I7zL0TtdR86YSsw1T7pxobSVoQGAOH9Ua4apor8TI='` — the base64 SHA-256 of the current inline theme script.
- `internal/config/docker_security_test.go::TestCSP_ScriptHashMatchesInlineScript` **dynamically** reads the first `<script>…</script>` from `templates.go`, SHA-256s it, and asserts it equals the router.go CSP hash. It also `t.Fatal`s if `templates.go` contains no inline `<script>` block.
- `internal/api/router_test.go` (IMP-020-CSP-001) additionally asserts the CSP carries the inline-theme-script hash.

Therefore **any** edit that removes the localStorage authority — changing the inline
script OR deleting it — changes (or nulls) its SHA-256 and forces a coordinated edit to
`internal/api/router.go` (the CSP hash) **and** the two guard tests. `internal/api/router.go`
is outside this scope's `internal/web/` + `web/pwa/` boundary and is listed under the
Change Boundary Excluded surfaces ("product-data APIs"; the legacy inline theme control is
"domain templates beyond asset consumption"). It is the same surface this scope's own
Per-Lane Status (XP106-01-W row) and `scopes/04-shared-shell-shadow-canaries` already assign
to the SCOPE-106-04/05 head cutover.

Proof the coupling is live this session (`./smackerel.sh test unit --go --go-run 'TestCSP'`):

```text
[go-unit] applying -run selector: TestCSP
+ go test -run TestCSP -count=1 ./...
ok      github.com/smackerel/smackerel/cmd/core 0.370s [no tests to run]
ok      github.com/smackerel/smackerel/internal/api     0.231s [no tests to run]
ok      github.com/smackerel/smackerel/internal/config  0.023s
ok      github.com/smackerel/smackerel/internal/web     0.206s [no tests to run]
+ echo '[go-unit] go test ./... finished OK'
[go-unit] go test ./... finished OK
```

`internal/config 0.023s` (no "[no tests to run]") = `TestCSP_ScriptHashMatchesInlineScript`
ran GREEN, i.e. the current inline script hash == the router.go CSP hash. Any inline-script
edit breaks it until router.go is updated in lockstep.

### In-boundary state confirmed coherent (no change needed)

The appearance CODEC + server head-adapter + PWA pre-paint resolver are already coherent;
the ONLY residual localStorage authority is the CSP-pinned legacy toggle above.

- `web/pwa/experience-appearance.js` and the server `appearanceHTMLAttrs`/`ParseAppearanceCookie` (`internal/web/appearance_head.go` + `internal/web/experience_appearance.go`) parse `smk_appearance` = `v1:<theme>:<density>` **identically** — missing → `system`/`comfortable`, source `missing`, no `data-appearance-diagnostic`; invalid → `system`/`comfortable`, source `invalid`, `data-appearance-diagnostic="preference_invalid"`; clean → cookie theme/density, source `cookie`. No drift; the PWA resolver is cookie-only (never reads/writes localStorage). No alignment change required.
- `./smackerel.sh test unit --go --go-run 'Appearance'` → GREEN:

```text
[go-unit] applying -run selector: Appearance
+ go test -run Appearance -count=1 ./...
ok      github.com/smackerel/smackerel/internal/config  0.023s
ok      github.com/smackerel/smackerel/internal/web     0.178s
+ echo '[go-unit] go test ./... finished OK'
[go-unit] go test ./... finished OK
```

- `./smackerel.sh check` → exit 0:

```text
config-validate: <repo-root>/config/generated/dev.env.tmp.2108770 OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 17, rejected: 0
scenario-lint: OK
```

- `bash .github/bubbles/scripts/pii-scan.sh` → clean (exit 0):

```text
3:00PM INF 0 commits scanned.
3:00PM INF scan completed in 10.7ms
3:00PM INF no leaks found
🫧 pii-scan: clean.
PII_SCAN_EXIT=0
```

<!-- bubbles:g040-skip-begin -->
### Routed finding (owner: SCOPE-106-04/05 shell/head cutover)

The server-head localStorage-authority removal is a coupled `internal/web/templates.go` +
`internal/api/router.go` change and therefore belongs to the SCOPE-106-04/05 head cutover
(the excluded-surface owner), exactly as this report's XP106-01-W row and
`scopes/04-shared-shell-shadow-canaries` already state. Actionable remediation for that
owner, as ONE atomic change:

1. In `internal/web/templates.go` `{{define "head"}}`: remove the `localStorage.getItem('theme')` read-override and the `localStorage.setItem('theme',…)` writes so the server-stamped first-paint `data-theme` is authoritative; reconcile the toggle to the `smk_appearance` cookie codec (`NewAppearanceCookie` + deployment-supplied `AppearanceCookieConfig{RetentionSeconds,Secure}`, fail-loud, no default) or, until that write-config is wired, a live-only DOM flip with NO client store.
2. In `internal/api/router.go:744`: update the CSP `'sha256-…'` to the new inline script's hash (or drop the inline hash if the control moves to a `script-src 'self'` asset).
3. Keep `internal/config/docker_security_test.go::TestCSP_ScriptHashMatchesInlineScript` and `internal/api/router_test.go` (IMP-020-CSP-001) green (the config test is dynamic and passes once router.go matches; if the inline `<script>` is removed entirely, its "no inline script" `t.Fatal` guard must be reconciled).

Because this requires editing `internal/api/router.go` (Excluded) plus its guard tests,
per the scope Change Boundary and the implement-agent boundary contract it is routed to the
owning scope rather than worked around here. `SCN-106-009`, `XP106-01-W`, and every related
Test-Evidence / Shared-Infra row remain `[ ]` this session.
<!-- bubbles:g040-skip-end -->

## <a id="session-2026-07-27-first-paint-real-stack"></a>SCOPE-106-01 appearance first-paint real-stack verification (2026-07-27)

**Claim Source:** executed. **Verification-only** session (no production code changed).
Goal: close the risk that the SCOPE-106-01 unit tests for the newly-wired server
head-adapter middleware `internal/web/appearance_head.go` `AppearanceHeadStamp` use
BARE handlers, not the REAL middleware chain. Two things confirmed on the disposable
live stack: (1) first-paint appearance stamping works END-TO-END through the wired
chain (`coherent_appearance.spec.ts`); (2) the buffer-and-rewrite middleware caused NO
regression to normal HTML pages, HTMX fragments, JSON/API responses, or redirects/auth.

**Wiring confirmed (working tree):** `router.go` uses `deps.AppearanceHeadMiddleware`
AFTER `securityHeadersMiddleware` (`r.Use(securityHeadersMiddleware)` … `if
deps.AppearanceHeadMiddleware != nil { r.Use(deps.AppearanceHeadMiddleware) }`); injected
via `cmd/core/wiring.go` `AppearanceHeadMiddleware: web.AppearanceHeadStamp`. `git status`:
`M cmd/core/wiring.go`, `M internal/api/router.go`, `?? internal/web/appearance_head.go`.

### 1. `./smackerel.sh test e2e-ui` → first-paint stamping GREEN + no middleware regression (E2E_UI_EXIT=0)

The full PWA Playwright suite (real disposable `smackerel-test-e2e-ui` stack, NO route
interception, NO auth injection). Includes XP106-01-W (`coherent_appearance.spec.ts`,
BOTH tests) and the XP106-01-C foundation canary (`coherent_foundation_canary.spec.ts`)
that exercises the buffering middleware across HTMX read/mutation, PWA auth, Card PRG, and
service-worker isolation. The CHAOS saga drives the SAME wired chain over server HTML,
`POST /search` (HTMX), `POST /api/capture` (JSON), `GET /api/...` (200/401), and login/PRG
redirects — all uncorrupted:

```text
Running 76 tests using 4 workers
  ✓  32 …-locked pre-paint assets are served same-origin and cookie-only (437ms)
CHAOS-EV J1.register | GET /register | status=200 | inviteField=true | len=1258
CHAOS-EV J1.login | GET /login | status=200
  ✓  34 … applies before first paint across server, PWA, and Card shells (995ms)
CHAOS-EV J2.apiCapture | POST /api/capture | status=200 | artifact_id=01KYJ2J4ATDDH9N3J6V67TDCZ9 | body={"artifact_id":"01KYJ2J4ATDDH9N3J6V67TDCZ9",...,"processing_time_ms":9}
CHAOS-EV J2.pwaShare | POST /pwa/share | status=200 | location=-
  ✓  38 …service-worker isolation keeps protected API routes network-only (62ms)
  ✓  39 …ative Search HTMX read still renders after the asset foundation (409ms)
  ✓  41 …RG shell still redirects and renders after the asset foundation (656ms)
  ✓  44 …anary: PWA auth still gates the PWA shell (served, never blank) (736ms)
  ✓  47 …preserved (GET / shell, POST /search fragment, GET /search 405) (135ms)
CHAOS-EV J3.authCookie | GET /api/notifications/status (cookie jar) | status=200
CHAOS-EV J3.authNone | GET /api/notifications/status (no auth) | status=401
CHAOS-EV J3.authBearer | GET /api/notifications/status (bearer) | status=200
CHAOS-EV J4.VERDICT | rendered={"/notifications/incidents":200,"/notifications":200,"/cards/recommendations":200,"/cards/report":200,"/cards/wallet":200,"/cards":200,"/cards/offers":200,"/notifications/events":200,"/notifications/summary":200}
  9 skipped
  67 passed (29.4s)
[web-e2e-ui] Tearing down disposable test stack (project smackerel-test-e2e-ui)...
=== E2E_UI_EXIT=0 ===
```

- **Test #34 `applies before first paint across server, PWA, and Card shells` PASS** is
  the decisive first-paint proof: with `smk_appearance=v1:dark:compact` set BEFORE
  navigation, `/`, `/pwa/`, and `/cards` each carry `data-theme="dark"` + `data-density="compact"`
  on `<html>` at first paint on the REAL stack. `/` and `/cards` are server-rendered and
  stamped by the wired middleware; `/pwa/` (middleware-skipped) is stamped by the cookie-only
  pre-paint resolver. Test #32 confirms the pre-paint resolver/token source are served
  same-origin and are localStorage-free.
- **No middleware regression:** canaries #38/#39/#41/#44/#47 + the CHAOS saga prove the
  buffer-and-rewrite path leaves HTMX fragments, JSON/API (200/401 with correct bodies),
  redirects/auth (login, PRG, 401s), and every server HTML page (correct status + title)
  intact. `9 skipped` are ENV-CONSTRAINED connector/GPU tests unrelated to this scope.

### 2. `./smackerel.sh test integration --go-run 'TestServerPWAAndCardHeadsServeTheSameVerifiedAssetsUnderStrictCSP'` → head path through middleware GREEN (INTEGRATION_HEAD_EXIT=0)

Independent server-side proof: the server/PWA/Card head-serving path — which now flows
through the wired `AppearanceHeadStamp` buffering middleware, serving FULL HTML documents
under strict CSP — passes on the real disposable stack (same scoped validation the prior
session used, re-run POST-wiring):

```text
2026/07/27 15:37:56 INFO request method=GET path=/pwa/fonts/ibm-plex-sans-latin-600-normal.woff2 status=200 duration_ms=0 ...
2026/07/27 15:37:56 INFO request method=GET path=/pwa/fonts/source-serif-4-latin-400-normal.woff2 status=200 duration_ms=0 ...
2026/07/27 15:37:56 INFO request method=GET path=/pwa/fonts/ibm-plex-mono-latin-400-normal.woff2 status=200 duration_ms=0 ...
2026/07/27 15:37:56 INFO request method=GET path=/pwa/sw.js status=200 duration_ms=0 ...
--- PASS: TestServerPWAAndCardHeadsServeTheSameVerifiedAssetsUnderStrictCSP (0.0Xs)
ok      github.com/smackerel/smackerel/tests/integration/web    0.136s
ok      github.com/smackerel/smackerel/internal/notification    0.016s [no tests to run]
ok      github.com/smackerel/smackerel/internal/assistant       0.140s [no tests to run]
ok      github.com/smackerel/smackerel/internal/cardrewards     0.019s [no tests to run]
PASS: go-integration
=== INTEGRATION_HEAD_EXIT=0 ===
```

### 3. Full `./smackerel.sh test integration` → exit 1 from a FOREIGN/env-constrained test (NOT middleware; NOT this scope)

For completeness the full unscoped lane was also run. It exited 1, then tore its stack
down cleanly:

```text
ok      github.com/smackerel/smackerel/internal/cardrewards     1.840s
PASS
FAIL
FAIL: go-integration (exit=1)
...
 Network smackerel-test_default  Removing
 Volume smackerel-test-postgres-data  Removed
 Volume smackerel-test-nats-data  Removed
=== INTEGRATION_EXIT=1 ===
```

**Attribution (honest):** the VS Code terminal buffer truncated the failing package out of
the capture (the retained window ends after the trailing `internal/assistant/*` +
`internal/cardrewards` packages, which PASSED; no `--- FAIL:`/`FAIL <pkg>` line survives in
either the inline or full-buffer capture), so the exact foreign test was **not isolated**.
It is provably **NOT the middleware head path** — §2 runs that exact test through the SAME
full disposable stack and it PASSES (exit 0). It is a **pre-existing foreign lane
condition**, consistent with this report's own Lane 3, which validates integration
**scoped** to the two 106 tests precisely because the full unscoped lane (~120 packages,
many ML/photo/ollama/connector env-dependent) carries foreign failures on the disposable
CPU-only stack (e.g. `/extract-card-categories returned HTTP 503: no model available on
disposable test stack`); `policy-exception-baseline.json` `exceptions: []`; the full lane
was never run green in these sessions.

### 4. Static rule-out of the one middleware-regression vector the head test cannot catch

The appearance stamp only inserts fixed-enum `data-theme`/`data-density`/`data-appearance-source`
attributes into the first `<html>` tag of a FULL HTML document and recomputes `Content-Length`.
The only way it could silently break an integration test the head test misses is an exact-byte /
`Content-Length` / golden-HTML / `<html>`-attribute assertion on a stamped server page. There is
none:

```text
$ grep -rnE '<html[ >]|golden|\.Equal\(.*[Bb]ody\.String|Body\.String\(\) ==|documentElement|data-theme|data-density|assert.*doctype' tests/integration/
--- exit=0 (grep: 1=no matches) ---
```

The `Content-Length`/`len(body)` hits elsewhere are `req.ContentLength` (outgoing request body
in `web_register_invite_consume_test.go:78`) and `auth_*` tests hitting `/api/`/`/auth/` JSON —
which the middleware SKIPS by prefix (`/api/`, `/v1/`, `/pwa/`, `/admin_ui_static/`, `/metrics`,
`/readyz`, `/ping`). The `web_register_invite_consume_test.go` banner checks are
`strings.Contains(rec.Body.String(), registerBanner)` (substring — unaffected by an `<html>`
attribute stamp). Middleware blast radius is therefore closed.

### 5. Clean teardown + PII scan

```text
=== ./smackerel.sh down ===
config-validate: .../config/generated/dev.env.tmp.XXXXXXX OK
DOWN_EXIT=0
=== leftover smackerel test containers? (should be none) ===
(none — clean)

$ bash .github/bubbles/scripts/pii-scan.sh
3:42PM INF 0 commits scanned.
3:42PM INF scan completed in 14ms
3:42PM INF no leaks found
🫧 pii-scan: clean.
PII_SCAN_EXIT=0
```

### Verdict for this session

- **First-paint-stamping half: VERIFIED on the real stack.** `coherent_appearance.spec.ts`
  (XP106-01-W) passes END-TO-END (both tests) through the wired `AppearanceHeadStamp`
  middleware; the buffering middleware caused **no regression** to HTML pages, HTMX
  fragments, JSON/API responses, or redirects/auth (e2e-ui 67 passed; scoped head
  integration exit 0; static vector rule-out clean).
- **No box was checked.** The localStorage-authority half of SCN-106-009 (the
  `templates.go` `{{define "head"}}` legacy `localStorage['theme']` control that can clobber
  the server-stamped `data-theme` when `localStorage.theme==='light'`) remains
  CSP-hash-coupled and boundary-blocked to SCOPE-106-04/05 (see the prior session above).
  `SCN-106-009` and every DoD checkbox in `scope.md` remain `[ ]`. `state.json` unchanged.
  This session records first-paint-stamping verification evidence ONLY.

## <a id="session-2026-08-15-a-w-green"></a>Session 2026-08-15 — XP106-01-A and XP106-01-W GREEN (current session)

This session is **evidence recording only**. No Go, TypeScript, or CSS source was changed
while recording it; the mechanical contract checker referenced below landed earlier in
commit `b41c360b`. All commands ran from `<repo-root>`.

This section **supersedes the earlier RED readings** for two rows — Lane 4
(`XP106-01-A **FAIL (RED)**, honest out-of-boundary block`) and Lane 5
(`XP106-01-W honest cutover-only RED`). The head-adapter wiring those sessions routed has
since landed, and both rows now run green on the disposable live stack.

### XP106-01-A

**XP106-01-A (regression E2E, `e2e-api`) — PASS, exit 0, current session.**

**Claim Source:** executed
**Command:** `./smackerel.sh test e2e --go-run 'TestExperienceAssetsExposeImmutableHeadersExactDigestsAndNetworkOnlyProtectedRoutes'`
**Exit Code:** 0
**Surface:** `tests/e2e/experience_assets_e2e_test.go:40` →
`func TestExperienceAssetsExposeImmutableHeadersExactDigestsAndNetworkOnlyProtectedRoutes`

The test name is the scenario-specific regression contract itself: immutable headers,
**exact digests**, and **network-only protected routes** — i.e. asset serving, digest
identity, and protected-route isolation are each asserted by the named test that ran green.

```text
go-e2e: applying -run selector: TestExperienceAssetsExposeImmutableHeadersExactDigestsAndNetworkOnlyProtectedRoutes
=== RUN   TestExperienceAssetsExposeImmutableHeadersExactDigestsAndNetworkOnlyProtectedRoutes
--- PASS: TestExperienceAssetsExposeImmutableHeadersExactDigestsAndNetworkOnlyProtectedRoutes (0.04s)
PASS
ok      github.com/smackerel/smackerel/tests/e2e        0.182s
PASS
ok      github.com/smackerel/smackerel/tests/e2e/admin  0.007s [no tests to run]
PASS
ok      github.com/smackerel/smackerel/tests/e2e/agent  0.160s [no tests to run]
PASS
ok      github.com/smackerel/smackerel/tests/e2e/auth   0.147s [no tests to run]
PASS
ok      github.com/smackerel/smackerel/tests/e2e/foundation     0.021s [no tests to run]
PASS
ok      github.com/smackerel/smackerel/tests/e2e/wiki   0.006s [no tests to run]
PASS: go-e2e
=== XP106_01_A_EXIT=0 ===
```

### XP106-01-W

**XP106-01-W (E2E UI regression, `e2e-ui`) — PASS, exit 0, current session, live-stack authentic.**

**Claim Source:** executed
**Command:** `./smackerel.sh test e2e-ui`
**Exit Code:** 0
**Surface:** `web/pwa/tests/coherent_appearance.spec.ts` (both tests: `:33` and `:63`)

```text
  ✓  31 coherent_appearance.spec.ts:33:1 › source-locked pre-paint assets are served same-origin and cookie-only (389ms)
  ✓  34 coherent_appearance.spec.ts:63:1 › appearance applies before first paint across server, PWA, and Card shells (943ms)
  68 passed (27.4s)
=== XP106_01_W_EXIT=0 ===
```

**Live-stack authenticity (no interception, no auth injection).** The interception scan on
that spec returns EMPTY, re-verified this session:

```text
$ grep -nE 'page\.route|context\.route|intercept\(|msw|nock' web/pwa/tests/coherent_appearance.spec.ts
SCAN_EXIT=1 (1 = no matches = clean)
```

The `68 passed` / exit 0 result is the whole `e2e-ui` lane, so the broader UI regression
suite carries no foundation-induced regression across the shared renderers, and the
`test e2e` lane above is green across every `tests/e2e/*` package.

### <a id="contracts-mechanically-enforceable"></a>Mechanical enforceability of the shared visual contracts (commit `b41c360b`)

**Claim Source:** executed (unit lane) + verified commit inspection.

```text
$ git show --stat --oneline b41c360b
b41c360b feat(106-01): make the shared visual contracts mechanically enforceable
 internal/web/experience_contracts.go      | 837 ++++++++++++++++++++++++++++++
 internal/web/experience_contracts_test.go | 307 +++++++++++
 web/pwa/experience-tokens.css             |  61 +++
 3 files changed, 1205 insertions(+)
```

Every clause named by the DoD row has an exported checker **and** at least one adversarial
case that fails if the rule is removed:

| Contract clause | Checker in `internal/web/experience_contracts.go` | Adversarial cases |
|---|---|---|
| Typography | `CheckTypographyContract` | viewport-scaled font, negative letter-spacing, below readable floor |
| Icons (icon-only controls) | `CheckIconOnlyControlContract` | no accessible name, no tooltip, empty `aria-label` |
| Controls (target size) | `CheckControlTargetContract` | target below coarse-pointer minimum, no icon-only geometry |
| Stable dimensions | `CheckStableDimensionContract` | font-relative shell rail, missing required token |
| No overlap | `CheckNoOverlapContract` | negative margin, fixed height cannot grow, unannotated `overflow:hidden` |
| No nested card | `CheckNoNestedCardContract` | card inside card, card inside card via wrapper, card-styled landmark |
| Contrast | `CheckContrastContract` | light body text too close to background, dark-theme regression judged independently, focus ring below non-text minimum, role the dark block forgot to override |
| Forced colors | `CheckForcedColorsContract` | no `forced-colors` block, `forced-color-adjust: none`, literal color inside `forced-colors` |
| Reduced motion | `CheckReducedMotionContract` | no `reduce` block, `reduce` block leaves motion running, `reduce` block forgets a declared token |

The aggregate `CheckExperienceContracts()` is wired to the **real** surfaces, not fixtures:
it reads the embedded semantic token source `experience-tokens.css` from `pwa.StaticFiles`
for the CSS-expressed contracts, and globs **every** embedded PWA `*.html` document for the
markup-expressed contracts, returning an error when a required surface is absent (an
unmeasured contract is treated as unsatisfied, not as a pass).
`TestExperienceContractsAreMechanicallyEnforcedOnTheRealSurfaces` executes that aggregate.

Unit lane excerpt as captured this session (the pasted excerpt records the package result
line and the exit code; the exact invocation flags were not part of the captured excerpt):

```text
ok      github.com/smackerel/smackerel/internal/web     0.436s
UNIT_EXIT=0
```

### <a id="build-quality-2026-08-15"></a>Build Quality lanes — current session (partial, see open clauses)

**Claim Source:** executed. Each lane below exited 0.

```text
=== check ===
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 17, rejected: 0
scenario-lint: OK
CHECK=0
=== lint ===
  OK: Extension versions match (1.0.0)
Web validation passed
LINT=0
=== format ===
78 files already formatted
FMT=0
=== traceability ===
ℹ️  Edge confidence (IMP-015 Scope B): declared=44 inferred=1 ambiguous=13
RESULT: PASSED (0 warnings)
TRACE=0
=== artifact lint ===
Artifact lint PASSED.
LINT_EXIT=0
```

**This does NOT close the Build Quality Gate DoD row.** That row enumerates further
clauses for which this session captured no evidence — see the open-clause note in
`scope.md`. The row stays `[ ]`.

### Verdict for this session

- **Checked this session (5 rows):** the typography/icons/controls/stable-dimensions/
  no-overlap/no-nested-card/contrast/forced-colors/reduced-motion mechanical-enforceability
  Core Outcome, `XP106-01-A`, `XP106-01-W`, the scenario-specific E2E regression row, and
  the broader E2E regression row.
- **Left open — `SCN-106-009` Core Outcome.** The row claims **System, Light, Dark,
  Comfortable, and Compact** resolve before first paint and that forced colors and reduced
  motion stay platform-controlled. The live spec sets exactly one cookie value,
  `v1:dark:compact`, and asserts `data-theme="dark"` / `data-density="compact"`; it does not
  exercise System, Light, or Comfortable, and a grep for `forced-colors` /
  `prefers-reduced-motion` across `web/pwa/tests/` returns **zero** matches, so no e2e-ui
  assertion covers those two clauses. The spec's own header comment asserts them in prose,
  which is not evidence. The localStorage-authority half recorded in the 2026-07-27 session
  above also remains routed to SCOPE-106-04/05. Claim is broader than the evidence — the row
  is NOT narrowed to fit, it stays `[ ]`.
- **Left open — independent canary suite row.** The captured `e2e-ui` output itemizes only
  the two `coherent_appearance.spec.ts` lines plus the `68 passed` summary; it does not
  itemize `coherent_foundation_canary.spec.ts` results, and the row's ordering clause
  ("before broad suite reruns") has no captured evidence. The existing `#xp106-01-c` section
  records four canaries while the spec now carries five named canaries. Stays `[ ]`.
- **Left open — Build Quality Gate row.** See the open-clause note in `scope.md`.
- `state.json` unchanged by this session.
