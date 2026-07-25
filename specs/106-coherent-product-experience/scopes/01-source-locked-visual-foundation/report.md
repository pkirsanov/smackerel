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

**Honest partial progress.** 2 of 10 DoD items are `[x]` with current-session
evidence (Core-2 + XP106-01-U). The four live-stack test lanes (I/A/W/C) and the
cross-renderer core items are honestly `[ ]` — see "Per-Lane Status" for the
precise, non-fabricated reason each remains open. No fabrication; no commit/push.

## Decision Record

- HTMX is still a pinned unpkg CDN `<script>` (`internal/web/templates.go`);
  BUG-002-006 (`in_progress`) owns the same-origin htmx vendoring + digest. The
  manifest records htmx under `ExternalDependencies` as
  `pending-same-origin-migration` and embeds **no second copy** and **no
  fabricated digest**.
- IBM Plex Sans / Source Serif 4 / IBM Plex Mono are **not** vendored; same-origin
  `.woff2` byte vendoring requires a network fetch unavailable in this build
  environment. Recorded as `not-yet-vendored-network-required` with the family
  names declared (platform fallback) in the token source. **No font digest is
  fabricated.**
- The service-worker cache identity already advances atomically from an aggregate
  content hash over the whole `web/pwa` embed (`internal/api/pwa.go`
  `pwaContentHash`); adding the two new foundation assets advances SW identity
  with no SW code change (Implementation-Plan step 7).

## Completion Statement

Not complete. SCOPE-106-01 is **In Progress**. 2 of 10 DoD items are `[x]` with
current-session evidence (Core-2, XP106-01-U). The remaining 8 items are honestly
`[ ]` with precise reasons in "Per-Lane Status"; none is fabricated.

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

## Test Evidence

### <a id="xp106-01-u"></a>XP106-01-U (unit) — PASS

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

### Full unit-suite regression — GREEN (no regression)

**Claim Source:** executed. The whole `go test ./...` reported `ok`/`PASS` for
every package and **zero `FAIL`** (`[go-unit] go test ./... finished OK`). The
foundation did not regress any existing package.

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

### <a id="build-quality"></a>Build Quality — lint PASS (0); my files gofmt-clean; foreign format failures recorded

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

## Per-Lane Status (honest)

| Lane | DoD | Status | Honest reason |
|---|---|---|---|
| XP106-01-U unit | Test Ev. 1 | **PASS** `[x]` | Authored + run + green this session. |
| Core-2 manifest+token source | Core-2 | **DONE** `[x]` | Fully proven by XP106-01-U (real digests, source, licence, CSP, cache policy, token source). |
| XP106-01-I integration | Test Ev. 2 | `[ ]` | Live-stack test not yet authored. Docker is available (not env-blocked); authoring + running the full live-stack assertion is open implementation not started in this session. |
| XP106-01-A e2e | Test Ev. 3 | `[ ]` | Live-stack e2e test not yet authored (same as above). |
| XP106-01-W e2e-ui | Test Ev. 4 | `[ ]` | Requires wiring the pre-paint + token **head adapters** into the server, PWA, and Card renderers so appearance applies before first paint — the renderer-head migration owned by the SCOPE-04/05-adjacent work. Not started in this foundation-only pass (active render intentionally unchanged). |
| XP106-01-C e2e-ui canary | Test Ev. 5 | `[ ]` | Requires the independent canary scaffolding (native Search, HTMX read, HTMX mutation, PWA auth, Card PRG, SW isolation) the design mandates **before** any renderer migration; depends on the head wiring above. |
| Core-1 theme-follows-user | Core-1 | `[ ]` | Codec + pre-paint + tokens + no-business-value implemented & unit-proven, but the cross-renderer before-first-paint acceptance is proven by XP106-01-W (open). |
| Core-3 mechanically-enforceable | Core-3 | `[ ]` | Token/type/dimension/focus/motion/forced-colors/reduced-motion enforced by XP106-01-U; the repo-wide no-nested-card / no-overlap / contrast scanners belong to the renderer-migration work (SCOPE-04/05 adjacent) and are outside this foundation pass. |
| Core-4 canaries + rollback | Core-4 | `[ ]` | The immutable `CacheIdentity` rollback unit exists and is unit-proven, but the independent consumer canaries (XP106-01-C) are open. |
| Build Quality Gate | Build Quality | `[ ]` | lint exit 0 and 106's own files gofmt-clean, but repo-wide `format --check` exits 1 for two FOREIGN pre-existing files and the test lanes above are open; kept `[ ]` honestly. |

## Planned Test References

**Claim Source:** not-run. XP106-01-I / -A / -W / -C are not yet authored/run; see
"Per-Lane Status" for the precise reason each is open. Docker is available, so the
block is unstarted implementation work (and, for W/C, renderer-head wiring), not
the environment.

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
